package yws

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"github.com/pulse/server/internal/boards"
	"github.com/pulse/server/internal/httpapi"
	"github.com/pulse/server/internal/workspaces"
)

// BoardRoom menyimpan koneksi untuk satu board.
type BoardRoom struct {
	ID          uuid.UUID
	mu          sync.RWMutex
	connections map[*BoardConn]struct{}
}

// BoardConn membungkus koneksi WS untuk board channel.
type BoardConn struct {
	ws       *websocket.Conn
	room     *BoardRoom
	send     chan []byte
	done     chan struct{}
	closeOnce sync.Once
	UserID   string
	UserName string
	Role     string
}

// BoardHub mengelola board-id → BoardRoom.
type BoardHub struct {
	mu     sync.RWMutex
	rooms  map[uuid.UUID]*BoardRoom
	logger *slog.Logger
}

func NewBoardHub(logger *slog.Logger) *BoardHub {
	if logger == nil {
		logger = slog.Default()
	}
	return &BoardHub{
		rooms:  make(map[uuid.UUID]*BoardRoom),
		logger: logger,
	}
}

// Broadcast mengirim data ke semua client di room boardID (string).
func (h *BoardHub) Broadcast(boardIDStr string, data []byte) {
	boardID, err := uuid.Parse(boardIDStr)
	if err != nil {
		return
	}
	h.mu.RLock()
	r, ok := h.rooms[boardID]
	h.mu.RUnlock()
	if ok {
		r.Broadcast(data, nil)
	}
}

func (h *BoardHub) GetOrCreate(boardID uuid.UUID) *BoardRoom {
	h.mu.RLock()
	r, ok := h.rooms[boardID]
	h.mu.RUnlock()
	if ok {
		return r
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if r, ok := h.rooms[boardID]; ok {
		return r
	}
	r = &BoardRoom{
		ID:          boardID,
		connections: make(map[*BoardConn]struct{}),
	}
	h.rooms[boardID] = r
	return r
}

func (r *BoardRoom) Add(c *BoardConn) {
	r.mu.Lock()
	r.connections[c] = struct{}{}
	r.mu.Unlock()
}

func (r *BoardRoom) Remove(c *BoardConn) {
	r.mu.Lock()
	delete(r.connections, c)
	r.mu.Unlock()
}

// MaybeEvict membuang room kosong dari hub (mencegah memory leak).
func (h *BoardHub) MaybeEvict(boardID uuid.UUID) {
	h.mu.Lock()
	defer h.mu.Unlock()
	r, ok := h.rooms[boardID]
	if !ok {
		return
	}
	r.mu.Lock()
	empty := len(r.connections) == 0
	r.mu.Unlock()
	if empty {
		delete(h.rooms, boardID)
	}
}

// Broadcast kirim data ke semua koneksi di room kecuali pengecualian.
func (r *BoardRoom) Broadcast(data []byte, except *BoardConn) {
	r.mu.RLock()
	conns := make([]*BoardConn, 0, len(r.connections))
	for c := range r.connections {
		conns = append(conns, c)
	}
	r.mu.RUnlock()

	for _, c := range conns {
		if c == except {
			continue
		}
		// Non-blocking. Channel `send` tidak pernah di-close (sinyal shutdown
		// via `done`) → tidak ada risiko panic "send on closed channel".
		select {
		case <-c.done:
		case c.send <- data:
		default:
		}
	}
}

// BoardWSHandler menangani upgrade WS untuk board.
type BoardWSHandler struct {
	hub       *BoardHub
	boardRepo *boards.Repo
	wsRepo    *workspaces.Repo
	logger    *slog.Logger
	upgrader  websocket.Upgrader
}

func NewBoardWSHandler(hub *BoardHub, boardRepo *boards.Repo, wsRepo *workspaces.Repo, allowedOrigin string, logger *slog.Logger) *BoardWSHandler {
	if logger == nil {
		logger = slog.Default()
	}
	allowed := map[string]bool{allowedOrigin: true}
	return &BoardWSHandler{
		hub:       hub,
		boardRepo: boardRepo,
		wsRepo:    wsRepo,
		logger:    logger,
		upgrader: websocket.Upgrader{
			ReadBufferSize:   4096,
			WriteBufferSize:  4096,
			CheckOrigin:      func(r *http.Request) bool { return allowed[r.Header.Get("Origin")] },
			HandshakeTimeout: 10 * time.Second,
		},
	}
}

func (h *BoardWSHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	boardIDStr := chi.URLParam(r, "boardID")
	boardID, err := uuid.Parse(boardIDStr)
	if err != nil {
		httpapi.NewWSWriteError(w, http.StatusBadRequest, "invalid board id")
		return
	}

	token := r.URL.Query().Get("token")
	if token == "" {
		httpapi.NewWSWriteError(w, http.StatusUnauthorized, "missing token")
		return
	}
	claims, err := httpapi.VerifyWSAccessToken(r.Context(), token)
	if err != nil {
		httpapi.NewWSWriteError(w, http.StatusUnauthorized, "invalid token")
		return
	}
	userID := claims.UserID

	// Authorization: pastikan user adalah anggota workspace board.
	wsID, err := h.boardRepo.BoardWorkspaceID(r.Context(), boardID)
	if err != nil {
		httpapi.NewWSWriteError(w, http.StatusForbidden, "forbidden")
		return
	}
	role, err := h.wsRepo.GetMemberRole(r.Context(), wsID, userID)
	if err != nil {
		h.logger.Warn("board ws role check failed", "board", boardID, "user", userID, "error", err)
		httpapi.NewWSWriteError(w, http.StatusForbidden, "forbidden")
		return
	}

	ws, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	room := h.hub.GetOrCreate(boardID)
	conn := &BoardConn{
		ws:       ws,
		room:     room,
		send:     make(chan []byte, 64),
		done:     make(chan struct{}),
		UserID:   userID.String(),
		UserName: claims.Name,
		Role:     role,
	}
	room.Add(conn)

	h.logger.Info("board ws connected", "board", boardID, "user", userID, "role", role)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Hardening: batasi ukuran frame & deteksi koneksi mati senyap.
	ws.SetReadLimit(16 << 20) // 16 MB
	_ = ws.SetReadDeadline(time.Now().Add(90 * time.Second))

	// Write pump
	pumpDone := make(chan struct{})
	go func() {
		defer close(pumpDone)
		defer ws.Close()
		for {
			select {
			case <-ctx.Done():
				return
			case <-conn.done:
				ws.WriteMessage(websocket.CloseMessage, []byte{})
				return
			case msg := <-conn.send:
				ws.SetWriteDeadline(time.Now().Add(10 * time.Second))
				if err := ws.WriteMessage(websocket.TextMessage, msg); err != nil {
					return
				}
			}
		}
	}()

	// Read pump
	for {
		_ = ws.SetReadDeadline(time.Now().Add(90 * time.Second))
		_, payload, err := ws.ReadMessage()
		if err != nil {
			break
		}
		_ = payload // Board events server saat ini hanya relay; tidak perlu proses payload
	}

	room.Remove(conn)
	conn.Close()
	<-pumpDone
	h.hub.MaybeEvict(boardID)

	h.logger.Info("board ws disconnected", "board", boardID, "user", userID)
}

func (c *BoardConn) Close() {
	c.closeOnce.Do(func() {
		close(c.done)
		c.ws.Close()
	})
}
