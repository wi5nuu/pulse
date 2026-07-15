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
	closed   bool
	closeMu  sync.Mutex
	closeOnce sync.Once

	// Identitas pengguna untuk presence/awareness & authorization (Fase 7).
	UserID   string
	UserName string
	Role     string // owner/editor/viewer

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
		UserID:      cfg.UserID,
		UserName:    cfg.UserName,
		Role:        cfg.Role,
		writeTimeout: cfg.WriteTimeout,
	}
}

// Send menambahkan data ke buffer kirim. Non-blocking: return false kalau
// buffer penuh (caller bisa drop atau disconnect). Dipakai oleh Hub.Broadcast.
func (c *Connection) Send(data []byte) bool {
	select {
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
		case msg, ok := <-c.send:
			if !ok {
				// channel di-close → sinyal shutdown
				_ = c.ws.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
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
	// NOTE: read deadline TIDAK di-set di sini karena koneksi WebSocket idle
	// selama user tidak ngetik. Heartbeat (ping/pong) di-handle oleh gorilla
	// default. Kalau perlu hardening, set pong handler di upgrade.
	defer c.Close()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
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
func (c *Connection) Close() {
	c.closeOnce.Do(func() {
		c.closeMu.Lock()
		c.closed = true
		c.closeMu.Unlock()
		close(c.send)
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
