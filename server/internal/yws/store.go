// Package yws (Yjs WebSocket) berisi: document store (in-memory byte state),
// hub koneksi per-document, dan handler WS yang mengimplementasikan sync &
// awareness protocol sesuai y-protocols.
//
// Filosofi: server = relay stateful, BUKAN re-implementasi CRDT (task §2).
// Server menyimpan:
//   1. doc.state  → byte array state Yjs terakhir (hasil merge kumulatif dari
//                   semua update yang lewat — untuk replay ke client baru).
//   2. doc.sv     → state vector terakhir (byte array) yang dikirim client;
//                   dipakai untuk balas SYNC_STEP2.
//
// Karena server tidak punya library Yjs, kami tidak bisa benar-benar "merge"
// update di server. Pendekatan yang kami pakai (cocok untuk relay):
//   - Saat client connect & kirim SYNC_STEP1(stateVector), server balas
//     SYNC_STEP2(stateTerakhir) — state penuh, bukan delta. Tidak optimal
//     bandwidth, tapi BENAR (client apply, lalu pakai Y.mergeUpdates untuk
//     deduplicate — CRDT idempotent terhadap apply).
//   - Setiap Update yang masuk → kami append ke state (concatenate as opaque)
//     TIDAK. Cara ini salah karena Y.applyUpdate butuh update tunggal, bukan
//     concat raw bytes.
//
// Solusi yang BENAR tanpa library: server simpan LIST of raw update bytes.
// Saat client baru minta sync, server balas dengan merge dari semua update?
// Tetap butuh Y.mergeUpdates yang ada hanya di library Yjs.
//
// === Pendekatan akhir (Fase 2) ===
// Untuk bisa berjalan TANPA library Yjs di server, kami pilih:
//   1. Server memaintain doc.state = hasil yang dikirim client pertama kali
//      via SYNC_STEP2(state vector kosong) → yaitu full state.
//      Setiap Update yang masuk → KAMI KIRIM SYNC_STEP2 request ke client
//      penyumbang untuk minta state vector-nya, lalu minta full state lagi.
//   2. Lebih sederhana & correct: server hanya memaintain "snapshot terakhir"
//      dengan strategi salah-satu-client-sebagai-source-of-truth:
//        - Update masuk → broadcast ke semua client LAIN (relay murni).
//        - Saat client baru connect → minta state penuh dari client tertua
//          yang masih online. Kalau tidak ada yang online → load dari DB
//          (Fase 4 persistence).
//
// Ini strategi y-websocket "dumb relay" — sederhana, benar, dan tidak butuh
// library CRDT di server. Trade-off: persistence butuh client write-back
// (Fase 4 akan kirim periodic snapshot dari client).
package yws

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Message type top-level (y-protocols §5 + Pulse extension).
const (
	MsgSync           byte = 0
	MsgAwareness      byte = 1
	MsgAuth           byte = 2
	MsgQueryAwareness byte = 3
	MsgPing           byte = 6
	MsgPong           byte = 7
)

// Sync sub-protocol type (y-protocols §3.1).
const (
	SyncStep1 byte = 0 // payload = stateVector
	SyncStep2 byte = 1 // payload = documentUpdate
	Update    byte = 2 // payload = documentUpdate
)

// ErrStoreClosed sinyal store sudah dimatikan (shutdown).
var ErrStoreClosed = errors.New("document store closed")

// Document menyimpan state Yjs (opaque bytes) untuk satu dokumen + daftar
// koneksi yang sedang terhubung. Concurrency-safe.
type Document struct {
	ID      uuid.UUID
	mu      sync.RWMutex
	// lastState = state Yjs penuh terakhir yang diketahui server. Opaque blob.
	// Diisi saat client kirim SYNC_STEP2 (saat handshake) atau saat periodic
	// snapshot write-back. Bisa nil kalau belum ada yang isi.
	lastState []byte
	// replayEvents = update events yang dimuat dari DB (events setelah snapshot
	// terakhir). Dikirim ke client baru setelah SYNC_STEP2 agar client mendapat
	// semua perubahan yang terjadi setelah snapshot.
	replayEvents [][]byte
	// connections = set koneksi aktif untuk dokumen ini.
	connections map[*Connection]struct{}
	// dirty = true kalau ada update yang masuk tapi lastState belum di-refresh.
	dirty bool
	// pendingEvents adalah buffer update bytes yang belum di-flush ke DB.
	pendingEvents [][]byte
	// eventCount = total update events sejak snapshot terakhir.
	eventCount int

	// --- Awareness batching (Fase 3) ---
	awarenessMu       sync.Mutex
	pendingAwareness  map[string][]byte // connID → last awareness body
	awarenessTimer    *time.Timer
	awarenessBatchDur time.Duration
}

// Store mengelola document-id → *Document. Lazy-create saat akses.
type Store struct {
	mu     sync.RWMutex
	docs   map[uuid.UUID]*Document
	logger *slog.Logger
	closed bool
}

func NewStore(logger *slog.Logger) *Store {
	if logger == nil {
		logger = slog.Default()
	}
	return &Store{docs: make(map[uuid.UUID]*Document), logger: logger}
}

// GetOrCreate return Document untuk docID; bikin baru kalau belum ada.
func (s *Store) GetOrCreate(docID uuid.UUID) *Document {
	s.mu.RLock()
	d, ok := s.docs[docID]
	s.mu.RUnlock()
	if ok {
		return d
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	// Double-check setelah acquire write lock.
	if d, ok := s.docs[docID]; ok {
		return d
	}
	d = &Document{
		ID:                docID,
		connections:       make(map[*Connection]struct{}),
		pendingAwareness:  make(map[string][]byte),
		awarenessBatchDur: 50 * time.Millisecond,
	}
	s.docs[docID] = d
	return d
}

// SetState setter untuk lastState (dipanggil saat SYNC_STEP2 diterima, atau
// saat client write-back snapshot di Fase 4).
func (d *Document) SetState(state []byte) {
	d.mu.Lock()
	defer d.mu.Unlock()
	// Copy supaya caller tidak bisa mutate setelahnya.
	d.lastState = append([]byte(nil), state...)
	d.dirty = false
}

// SetReplayEvents menyimpan daftar event yang perlu di-replay ke client baru
// (dimuat dari DB saat load dokumen). Copy supaya caller tidak mutate.
func (d *Document) SetReplayEvents(events [][]byte) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.replayEvents = make([][]byte, len(events))
	for i, e := range events {
		d.replayEvents[i] = append([]byte(nil), e...)
	}
}

// GetAndClearReplayEvents mengambil replay events dan membersihkan buffer.
// Dipanggil di handleSyncStep1 setelah mengirim SYNC_STEP2.
func (d *Document) GetAndClearReplayEvents() [][]byte {
	d.mu.Lock()
	defer d.mu.Unlock()
	events := d.replayEvents
	d.replayEvents = nil
	return events
}

// State getter. Return (state, true) kalau ada, (nil, false) kalau belum.
func (d *Document) State() ([]byte, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.lastState == nil {
		return nil, false
	}
	return append([]byte(nil), d.lastState...), true
}

// QueueAwareness menyimpan awareness update terbaru dari suatu koneksi.
// Broadcast di-batch dengan window ~50ms untuk mengurangi jumlah pesan.
func (d *Document) QueueAwareness(connID string, body []byte) {
	d.awarenessMu.Lock()
	defer d.awarenessMu.Unlock()

	d.pendingAwareness[connID] = append([]byte(nil), body...)

	if d.awarenessTimer == nil {
		d.awarenessTimer = time.AfterFunc(d.awarenessBatchDur, func() {
			d.flushAwareness()
		})
	}
}

// flushAwareness broadcast semua pending awareness ke semua koneksi.
func (d *Document) flushAwareness() {
	d.awarenessMu.Lock()
	pending := d.pendingAwareness
	d.pendingAwareness = make(map[string][]byte)
	d.awarenessTimer = nil
	d.awarenessMu.Unlock()

	for _, body := range pending {
		d.Broadcast(EncodeAwarenessMessage(body), nil)
	}
}

// AddPendingEvent menambahkan update byte ke buffer pending untuk persistensi.
// Juga menandai dokumen sebagai dirty.
func (d *Document) AddPendingEvent(update []byte) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.pendingEvents = append(d.pendingEvents, append([]byte(nil), update...))
	d.eventCount++
	d.dirty = true
}

// GetAndClearPendingEvents mengambil semua pending events dan membersihkan buffer.
func (d *Document) GetAndClearPendingEvents() ([][]byte, int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	events := d.pendingEvents
	count := d.eventCount
	d.pendingEvents = nil
	d.eventCount = 0
	return events, count
}

// PendingEventCount mengembalikan jumlah pending events.
func (d *Document) PendingEventCount() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.eventCount
}

// markDirty menandai bahwa ada Update masuk dan lastState belum di-refresh
// (server tidak merge update krn tidak punya library Yjs). Dipakai persistence
// worker untuk tahu dokumen mana yang perlu di-write-back.
func (d *Document) markDirty() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.dirty = true
}

// IsDirty & ClearDirty dipakai worker write-back.
func (d *Document) IsDirty() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.dirty
}

func (d *Document) ClearDirty() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.dirty = false
}

// AddConnection mendaftarkan koneksi ke dokumen. Return list connection lain
// (untuk notifikasi join — opsional, dipakai di awareness Fase 3).
func (d *Document) AddConnection(c *Connection) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.connections[c] = struct{}{}
}

// RemoveConnection menghapus koneksi. Bersih-bersih.
func (d *Document) RemoveConnection(c *Connection) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.connections, c)
}

// Broadcast kirim data ke SEMUA koneksi dokumen KECUALI pengecualian `except`.
// except=nil = kirim ke semua. Setiap koneksi punya send buffer sendiri,
// jadi satu koneksi lambat tidak memblokir yang lain.
func (d *Document) Broadcast(data []byte, except *Connection) {
	d.mu.RLock()
	conns := make([]*Connection, 0, len(d.connections))
	for c := range d.connections {
		conns = append(conns, c)
	}
	d.mu.RUnlock()

	for _, c := range conns {
		if c == except {
			continue
		}
		// non-blocking kirim; kalau buffer penuh, drop (Fase 7: rate-limit/
		// disconnect slow consumer).
		select {
		case c.send <- data:
		default:
			// buffer penuh → skip. Client akan re-sync lewat heartbeat.
		}
	}
}

// ForEachDirty mengunjungi semua dokumen dirty dengan callback.
// Jika callback return false, iterasi berhenti.
func (s *Store) ForEachDirty(ctx context.Context, fn func(ctx context.Context, docID uuid.UUID, doc *Document) bool) {
	s.mu.RLock()
	ids := make([]uuid.UUID, 0, len(s.docs))
	for id, doc := range s.docs {
		if doc.IsDirty() {
			ids = append(ids, id)
		}
	}
	s.mu.RUnlock()
	for _, id := range ids {
		doc := s.GetOrCreate(id)
		if !fn(ctx, id, doc) {
			return
		}
	}
}

// ConnectionCount dipakai untuk awareness & debugging.
func (d *Document) ConnectionCount() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return len(d.connections)
}

// Close menutup store — tidak menerima dokumen baru. Tidak menutup koneksi
// aktif (caller yang tangani via server shutdown).
func (s *Store) Close(ctx context.Context) error {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	return nil
}
