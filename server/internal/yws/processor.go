package yws

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/pulse/server/internal/models"
	"github.com/pulse/server/internal/ycodec"
)

// SyncProcessor mengimplementasikan MessageProcessor untuk sync protocol.
//
// Saat pesan masuk:
//   message := varUint(messageType) • payload
//
// messageType:
//   0 = Sync      → payload = varUint(syncType) • varBuffer(data)
//   1 = Awareness → payload = raw awareness update (Fase 3)
//   2 = Auth      → Fase 7
//   3 = QueryAwareness → Fase 3
type SyncProcessor struct {
	store  *Store
	logger *slog.Logger
}

func NewSyncProcessor(store *Store, logger *slog.Logger) *SyncProcessor {
	if logger == nil {
		logger = slog.Default()
	}
	return &SyncProcessor{store: store, logger: logger}
}

// Process: decode top-level message, route ke handler sub-protocol.
func (p *SyncProcessor) Process(ctx context.Context, c *Connection, payload []byte) error {
	if len(payload) == 0 {
		return fmt.Errorf("empty message")
	}
	msgType, n, err := ycodec.ReadVarUint(payload)
	if err != nil {
		return fmt.Errorf("read message type: %w", err)
	}
	body := payload[n:]

	switch uint64(msgType) {
	case uint64(MsgSync):
		return p.handleSync(ctx, c, body)
	case uint64(MsgAwareness):
		// Awareness: batch di server dengan window 50ms sebelum broadcast.
		// Client sudah throttle ~20 updates/detik; server side batching
		// mengurangi jumlah broadcast ke N koneksi.
		p.batchAwareness(c.doc, body, c)
		return nil
	case uint64(MsgQueryAwareness):
		// Fase 3: balas dengan awareness saat ini. Untuk sekarang no-op.
		return nil
	case uint64(MsgAuth):
		// Fase 7: token auth tambahan (optional). Skip sekarang.
		return nil
	case uint64(MsgPing):
		// Heartbeat: balas dengan Pong.
		reply := make([]byte, 1)
		reply[0] = byte(MsgPong)
		c.Send(reply)
		return nil
	case uint64(MsgPong):
		// Pong dari client (saat kita jadi pengirim ping). No-op.
		return nil
	case uint64(MsgDocEvent):
		// Relay event dokumen (mis. komentar dibuat/diresolve) ke koneksi
		// LAIN. Server tidak menginterpretasi isi — client yang render.
		// Sender mendapat konfirmasi dari REST response-nya sendiri.
		c.doc.Broadcast(append([]byte{byte(MsgDocEvent)}, body...), c)
		return nil
	default:
		// Pesan tidak dikenali → abaikan (forward-compat). Jangan putuskan
		// koneksi; client versi baru mungkin kirim pesan yang server lama tidak
		// tahu — relay tolerance.
		p.logger.Debug("unknown message type, ignoring",
			"type", msgType, "doc", c.doc.ID, "user", c.UserID)
		return nil
	}
}

// batchAwareness memasukkan awareness ke buffer per-document untuk di-batch
// dan di-broadcast periodik (window 50ms). Client-side sudah throttle ~20/s.
func (p *SyncProcessor) batchAwareness(doc *Document, body []byte, sender *Connection) {
	doc.QueueAwareness(sender, body)
}

// isReadOnly: true jika user hanya boleh membaca (workspace viewer ATAU
// document share dengan permission "view"). Fix IDOR: string "view" (share)
// sebelumnya tidak cocok dengan check `Role == models.RoleViewer` sehingga
// viewer via share bisa menulis — bypass read-only.
func isReadOnly(role string) bool {
	return role == models.RoleViewer || role == "view"
}

// handleSync: decode sub-protocol & route.
//
// sync payload := varUint(syncType) • varBuffer(data)
func (p *SyncProcessor) handleSync(ctx context.Context, c *Connection, body []byte) error {
	syncType, n, err := ycodec.ReadVarUint(body)
	if err != nil {
		return fmt.Errorf("read sync type: %w", err)
	}
	data, _, err := ycodec.ReadVarBuffer(body[n:])
	if err != nil {
		return fmt.Errorf("read sync data: %w", err)
	}

	switch uint64(syncType) {
	case uint64(SyncStep1):
		// Client kirim stateVector-nya; minta missing updates dari server.
		// Server balas SYNC_STEP2 berisi state penuh (kami tidak bisa compute
		// delta tanpa library Yjs — relay strategy, lihat package doc).
		return p.handleSyncStep1(c, data)
	case uint64(SyncStep2):
		// Client kirim documentUpdate → ini adalah state yang server minta,
		// ATAU reply SYNC_STEP2 dari client saat server initiate.
		// Update lastState server & broadcast ke client lain.
		if isReadOnly(c.Role) {
			// Viewer ikut membalas SYNC_STEP1 yang dikirim persistence worker
			// saat request snapshot. Jangan putuskan koneksi — cukup abaikan;
			// snapshot hanya boleh berasal dari editor/owner.
			p.logger.Debug("ignoring viewer sync step2", "user", c.UserID, "doc", c.doc.ID)
			return nil
		}
		return p.handleSyncStep2(c, data)
	case uint64(Update):
		// Pembaruan inkremental dari client → broadcast ke client lain.
		// Server TIDAK merge ke lastState (tidak bisa tanpa library Yjs).
		if isReadOnly(c.Role) {
			// Viewers seharusnya tidak pernah mengirim Update (client
			// enforces read-only). Kalau terjadi, drop saja — jangan
			// memutus koneksi, lebih robust terhadap bug client.
			p.logger.Warn("dropping update from viewer", "user", c.UserID, "doc", c.doc.ID)
			return nil
		}
		return p.handleUpdate(c, data)
	default:
		return nil
	}
}

// handleSyncStep1: client minta sync.
//
// Strategi relay:
//   - Kalau server punya lastState (state penuh yang sudah di-write-back):
//     balas SYNC_STEP2(lastState) → client apply. BENAR & idempotent (CRDT).
//     Lalu replay events dari DB (events setelah snapshot) sebagai UPDATE.
//     Selanjutnya minta state vector client via SYNC_STEP1 balik — client
//     yang punya update offline akan membalas SYNC_STEP2 (fix C2: edit yang
//     dibuat saat putus koneksi TIDAK boleh hilang).
//   - Kalau TIDAK punya lastState (dokumen baru / belum di-persist):
//     balas SYNC_STEP1 kosong ke client → client akan reply dengan
//     SYNC_STEP2(full state) yang server simpan sebagai lastState.
//     Ini membalik handshake arah tanpa library Yjs.
func (p *SyncProcessor) handleSyncStep1(c *Connection, stateVector []byte) error {
	if state, ok := c.doc.State(); ok {
		// Punya state → balas SYNC_STEP2.
		reply := buildSyncMessage(SyncStep2, state)
		c.Send(reply)

		// Replay events yang dimuat dari DB (events setelah snapshot).
		// Per-koneksi (fix M1): setiap koneksi menerima semua events,
		// bukan hanya koneksi pertama setelah server restart.
		if !c.replaySent {
			for _, event := range c.doc.GetReplayEvents() {
				c.Send(buildSyncMessage(Update, event))
			}
			c.replaySent = true
		}

		// Fix C2: minta state client balik supaya update yang dibuat saat
		// offline tidak hilang. Client yang punya local changes akan membalas
		// SYNC_STEP2(full state) → server SetState (merge CRDT di client).
		// State vector client yang dikirim via parameter dipakai sebagai
		// sinyal: jika client mengirim SV kosong (dokumen baru), tidak perlu
		// tanya balik.
		if len(stateVector) > 0 {
			c.Send(buildSyncMessage(SyncStep1, stateVector))
		}

		return nil
	}
	// Tidak punya state → minta ke client. Kirim SYNC_STEP1 dengan state
	// vector kosong → client balas SYNC_STEP2(state penuh).
	emptySV := []byte{}
	reply := buildSyncMessage(SyncStep1, emptySV)
	c.Send(reply)
	return nil
}

// handleSyncStep2: client mengirim documentUpdate.
//
//   - Bisa jadi ini reply dari SYNC_STEP1 yang server kirim (full state dari
//     client) → simpan sebagai lastState.
//   - Bisa juga ini reply dari SYNC_STEP1 client-ke-server pada handshake awal
//     (lihat §3.3: client kirim SYNC_STEP1 dulu, server balas SYNC_STEP2;
//     lalu server kirim SYNC_STEP1, client balas SYNC_STEP2).
//
// Untuk relay: anggap SYNC_STEP2 = "state penuh dari client" → save sebagai
// lastState, LALU broadcast ke client LAIN supaya mereka juga sync.
// Exclude sender (c) to avoid echo.
func (p *SyncProcessor) handleSyncStep2(c *Connection, update []byte) error {
	// Save sebagai snapshot state (ini asumsi: client kirim full state saat
	// reply SYNC_STEP1 dari server — benar menurut Yjs docs).
	c.doc.SetState(update)

	// Relay ke client LAIN: mereka mungkin sudah punya state parsial.
	// Mereka akan apply & CRDT resolve duplikat. Aman.
	// Exclude sender to avoid echo.
	msg := buildSyncMessage(SyncStep2, update)
	c.doc.Broadcast(msg, c)
	return nil
}

// handleUpdate: pembaruan inkremental dari client (real-time editing).
//
// Relay-only: teruskan ke client lain. Server tidak modify lastState.
// lastState akan di-refresh di Fase 4 via periodic snapshot write-back dari
// salah satu client.
//
// UPDATE: untuk persistence yang benar di Fase 4, server tetap harus simpan
// sesuatu. Strategi: set doc.dirty=true supaya worker tahu dokumen perlu
// di-write-back. Write-back sendiri (kirim query ke client "tolong kirim full
// state") di-implement di Fase 4.
func (p *SyncProcessor) handleUpdate(c *Connection, update []byte) error {
	msg := buildSyncMessage(Update, update)
	c.doc.Broadcast(msg, c)
	// Simpan pending event untuk persistence worker flush ke document_events.
	c.doc.AddPendingEvent(update)
	return nil
}

// buildSyncMessage: helper encode pesan sync top-level.
//
//	out := varUint(MsgSync) • varUint(syncType) • varBuffer(data)
func buildSyncMessage(syncType byte, data []byte) []byte {
	body := ycodec.WriteVarUint(nil, uint64(syncType))
	body = ycodec.WriteVarBuffer(body, data)
	return EncodeSyncMessage(body)
}

// ErrInvalidRole dipakai untuk signaling (Fase 7); disiapkan di sini.
var ErrInvalidRole = errors.New("viewer cannot modify document")
