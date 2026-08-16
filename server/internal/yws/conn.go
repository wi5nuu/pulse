package yws

import (
	"context"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/pulse/server/internal/ycodec"
)

// Connection membungkus satu koneksi WebSocket yang sudah ter-autentikasi
// ke satu dokumen. Concurrency-safe untuk read/write paralel dari goroutine
// read pump & write pump.
type Connection struct {
	ws       *websocket.Conn
	doc      *Document
	send     chan []byte // buffered; write pump konsumsi
	done     chan struct{} // ditutup saat Close(); sinyal shutdown write pump
	closed   bool
	closeMu  sync.Mutex
	closeOnce sync.Once

	// Identitas pengguna untuk presence/awareness & authorization (Fase 7).
	UserID   string
	UserName string
	Role     string // owner/editor/viewer/view

	// replaySent: true setelah replay events dari DB sudah dikirim ke koneksi
	// ini. Per-koneksi (fix M1) — buffer replay di store tidak lagi di-clear
	// global, jadi tiap koneksi hanya menerima sekali.
	replaySent bool

	// writeTimeout: write WS tidak boleh blocking tak terhingga.
	writeTimeout time.Duration
}

type ConnConfig struct {
	WS           *websocket.Conn
	Doc          *Document
	UserID       string
	UserName     string
	Role         string
	SendBuffer  int
	WriteTimeout time.Duration
}

func NewConnection(cfg ConnConfig) *Connection {
	if cfg.SendBuffer <= 0 {
		cfg.SendBuffer = 64
	}
	if cfg.WriteTimeout <= 0 {
		cfg.WriteTimeout = 10 * time.Second
	}
	return &Connection{
		ws:          cfg.WS,
		doc:         cfg.Doc,
		send:        make(chan []byte, cfg.SendBuffer),
		done:        make(chan struct{}),
		UserID:      cfg.UserID,
		UserName:    cfg.UserName,
		Role:        cfg.Role,
		writeTimeout: cfg.WriteTimeout,
	}
}

// Send menambahkan data ke buffer kirim. Non-blocking: return false kalau
// buffer penuh atau koneksi sudah ditutup (caller bisa drop atau disconnect).
// Dipakai oleh Hub.Broadcast.
//
// Penting: channel `send` TIDAK pernah di-close — penutupan dilakukan via
// `done` channel. Ini menghindari panic "send on closed channel" saat
// Broadcast yang sedang berjalan bersaing dengan Close().
func (c *Connection) Send(data []byte) bool {
	select {
	case <-c.done:
		return false
	case c.send <- data:
		return true
	default:
		return false
	}
}

// WritePump: goroutine tunggal yang mengirim dari send chan → WebSocket.
// Penting: gorilla/websocket melarang concurrent writes — harus 1 writer only.
// Jalankan sebagai goroutine; return saat koneksi ditutup.
func (c *Connection) WritePump(ctx context.Context) {
	defer c.ws.Close()
	for {
		select {
		case <-ctx.Done():
			return
		case <-c.done:
			_ = c.ws.WriteMessage(websocket.CloseMessage, []byte{})
			return
		case msg := <-c.send:
			_ = c.ws.SetWriteDeadline(time.Now().Add(c.writeTimeout))
			if err := c.ws.WriteMessage(websocket.BinaryMessage, msg); err != nil {
				return
			}
		}
	}
}

// ReadPump: goroutine yang baca pesan masuk & proses. Return saat koneksi
// ditutup atau error. Setelah return, caller harus RemoveConnection dari Doc.
func (c *Connection) ReadPump(ctx context.Context, proc MessageProcessor) error {
	// Hardening: batasi ukuran frame (Yjs update dokumen normal < 1MB) supaya
	// client jahat tidak bisa OOM server dengan frame raksasa.
	c.ws.SetReadLimit(16 << 20) // 16 MB
	defer c.Close()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		// Client kirim ping setiap 30s → pakai deadline 90s untuk deteksi
		// koneksi mati senyap (network drop tanpa close frame).
		_ = c.ws.SetReadDeadline(time.Now().Add(90 * time.Second))
		_, payload, err := c.ws.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err,
				websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				return err
			}
			return nil
		}
		if err := proc.Process(ctx, c, payload); err != nil {
			return err
		}
	}
}

// Close menutup koneksi (idempotent). Aman dipanggil berkali-kali.
// Tidak menutup channel `send` — sinyal shutdown via `done` supaya Send()
// dari goroutine lain tidak panic.
func (c *Connection) Close() {
	c.closeOnce.Do(func() {
		c.closeMu.Lock()
		c.closed = true
		c.closeMu.Unlock()
		close(c.done)
		_ = c.ws.Close()
	})
}

// IsClosed dipakai oleh Hub untuk skip cleanup double.
func (c *Connection) IsClosed() bool {
	c.closeMu.Lock()
	defer c.closeMu.Unlock()
	return c.closed
}

// MessageProcessor: strategi pemrosesan pesan masuk. Implementasi nyata
// di handler.go — di-mock untuk test nanti.
type MessageProcessor interface {
	Process(ctx context.Context, c *Connection, payload []byte) error
}

// EncodeSyncMessage membungkus payload sync jadi pesan top-level.
// message := varUint(MsgSync) • payload
func EncodeSyncMessage(payload []byte) []byte {
	out := ycodec.WriteVarUint(nil, uint64(MsgSync))
	return append(out, payload...)
}

// EncodeAwarenessMessage membungkus payload awareness jadi pesan top-level.
func EncodeAwarenessMessage(payload []byte) []byte {
	out := ycodec.WriteVarUint(nil, uint64(MsgAwareness))
	return append(out, payload...)
}

// BuildSyncStep1Message membuat pesan SYNC_STEP1 dengan state vector tertentu.
// Dipakai oleh persistence worker untuk request full state dari client.
func BuildSyncStep1Message(stateVector []byte) []byte {
	return buildSyncMessage(SyncStep1, stateVector)
}

// BuildSyncStep2Message membuat pesan SYNC_STEP2 dengan full state.
// Dipakai untuk restore snapshot ke client.
func BuildSyncStep2Message(state []byte) []byte {
	return buildSyncMessage(SyncStep2, state)
}

// EncodeRoleMessage membungkus pesan role: varUint(MsgRole) + role string.
// Dipakai server untuk memberi tahu client role user (read-only vs editor).
func EncodeRoleMessage(role string) []byte {
	out := ycodec.WriteVarUint(nil, uint64(MsgRole))
	out = append(out, []byte(role)...)
	return out
}

// EncodeDocEventMessage membungkus payload event dokumen (JSON komentar dll):
// varUint(MsgDocEvent) + payload. Dipakai relay realtime — server hanya
// meneruskan ke koneksi lain, tidak menginterpretasi isinya.
func EncodeDocEventMessage(payload []byte) []byte {
	out := ycodec.WriteVarUint(nil, uint64(MsgDocEvent))
	return append(out, payload...)
}
