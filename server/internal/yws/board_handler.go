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
	closeOnce sync.Once
	UserID   string
	UserName string
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
		select {
		case c.send <- data:
		default:
		}
	}
}

// BoardWSHandler menangani upgrade WS untuk board.
type BoardWSHandler struct {
	hub      *BoardHub
	boardRepo *boards.Repo
	logger   *slog.Logger
	upgrader websocket.Upgrader
}

func NewBoardWSHandler(hub *BoardHub, boardRepo *boards.Repo, allowedOrigin string, logger *slog.Logger) *BoardWSHandler {
	if logger == nil {
		logger = slog.Default()
	}
	allowed := map[string]bool{allowedOrigin: true}
	return &BoardWSHandler{
		hub:       hub,
		boardRepo: boardRepo,
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

	ws, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	room := h.hub.GetOrCreate(boardID)
	conn := &BoardConn{
		ws:       ws,
		room:     room,
		send:     make(chan []byte, 64),
		UserID:   claims.UserID.String(),
		UserName: claims.Name,
	}
	room.Add(conn)

	h.logger.Info("board ws connected", "board", boardID, "user", claims.UserID)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Write pump
	pumpDone := make(chan struct{})
	go func() {
		defer close(pumpDone)
		defer ws.Close()
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-conn.send:
				if !ok {
					ws.WriteMessage(websocket.CloseMessage, []byte{})
					return
				}
				ws.SetWriteDeadline(time.Now().Add(10 * time.Second))
				if err := ws.WriteMessage(websocket.TextMessage, msg); err != nil {
					return
				}
			}
		}
	}()

	// Read pump
	for {
		_, payload, err := ws.ReadMessage()
		if err != nil {
			break
		}
		_ = payload // Board events server saat ini hanya relay; tidak perlu proses payload
	}

	room.Remove(conn)
	conn.Close()
	<-pumpDone

	h.logger.Info("board ws disconnected", "board", boardID, "user", claims.UserID)
}

func (c *BoardConn) Close() {
	c.closeOnce.Do(func() {
		close(c.send)
		c.ws.Close()
	})
}
