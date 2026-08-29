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
	MsgRole           byte = 5 // Pulse extension: kirim role user ke client
	MsgPing           byte = 6
	MsgPong           byte = 7
	MsgDocEvent       byte = 8 // Pulse extension: relay event dokumen (komentar, dll)
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
	// pendingEvents adalah buffer update bytes yang belum di-flush ke DB.
	pendingEvents [][]byte
	// eventCount = jumlah update di buffer pending (reset saat flush).
	eventCount int
	// stateFresh = true kalau lastState memuat SEMUA event yang sudah di-flush
	// ke DB (yaitu full-state write-back terjadi setelah event terakhir).
	// Snapshot hanya boleh disimpan saat stateFresh — kalau tidak, snapshot
	// menyimpan state basi dan restore kehilangan edit.
	stateFresh bool
	// needsWriteBack = ada event yang sudah di-flush tapi lastState belum
	// di-refresh (butuh request full state dari client).
	needsWriteBack bool
	// snapshotDue = ada event sejak snapshot terakhir, jadi snapshot perlu
	// disimpan begitu state fresh tersedia. Dipakai ForEachDirty supaya
	// dokumen tetap dikunjungi worker walau pendingEvents kosong.
	snapshotDue bool
	// sinceLastSnapshot = jumlah event sejak snapshot terakhir disimpan.
	// TIDAK di-reset saat flush — hanya di-reset saat snapshot disimpan.
	sinceLastSnapshot int

	// --- Awareness batching (Fase 3) ---
	awarenessMu       sync.Mutex
	pendingAwareness  map[*Connection][]byte // connection → last awareness body
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
		pendingAwareness:  make(map[*Connection][]byte),
		awarenessBatchDur: 50 * time.Millisecond,
	}
	s.docs[docID] = d
	return d
}

// SetState setter untuk lastState (dipanggil saat SYNC_STEP2 diterima, saat
// client write-back snapshot, atau saat restore snapshot).
//
// PENTING (fix C3): TIDAK mengosongkan pendingEvents. Update yang sedang
// in-flight bisa saja TIDAK tercakup dalam state yang dikirim client (race:
// client membalas write-back sebelum menerima broadcast update terbaru).
// Jika pending events dibuang di sini, update itu hilang permanen.
// Worker tetap flush pendingEvents ke DB — duplikat tidak berbahaya karena
// Yjs update idempotent saat di-apply.
func (d *Document) SetState(state []byte) {
	d.mu.Lock()
	defer d.mu.Unlock()
	// Copy supaya caller tidak bisa mutate setelahnya.
	d.lastState = append([]byte(nil), state...)
	// stateFresh hanya jika tidak ada pending events yang belum ter-flush.
	// Kalau masih ada, needsWriteBack tetap true → worker minta write-back
	// lagi sampai semua event tercakup.
	d.stateFresh = d.eventCount == 0
	d.needsWriteBack = d.eventCount > 0
	// Replay events dari DB tidak diperlukan lagi: lastState adalah state
	// lengkap terbaru dari client. (Event lama tetap aman di DB.)
	d.replayEvents = nil
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

// GetReplayEvents mengambil replay events TANPA membersihkan buffer.
// Dipakai di handleSyncStep1 — setiap koneksi perlu menerima event yang sama
// (fix M1: sebelumnya buffer di-clear global, koneksi kedua kehilangan events).
func (d *Document) GetReplayEvents() [][]byte {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if len(d.replayEvents) == 0 {
		return nil
	}
	out := make([][]byte, len(d.replayEvents))
	for i, e := range d.replayEvents {
		out[i] = append([]byte(nil), e...)
	}
	return out
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
// Key = *Connection (bukan UserID) supaya dua tab dari user yang sama punya
// presence terpisah.
func (d *Document) QueueAwareness(conn *Connection, body []byte) {
	d.awarenessMu.Lock()
	defer d.awarenessMu.Unlock()

	d.pendingAwareness[conn] = append([]byte(nil), body...)

	if d.awarenessTimer == nil {
		d.awarenessTimer = time.AfterFunc(d.awarenessBatchDur, func() {
			d.flushAwareness()
		})
	}
}

// flushAwareness broadcast semua pending awareness ke koneksi lain
// (sender TIDAK menerima echo awareness-nya sendiri).
func (d *Document) flushAwareness() {
	d.awarenessMu.Lock()
	pending := d.pendingAwareness
	d.pendingAwareness = make(map[*Connection][]byte)
	d.awarenessTimer = nil
	d.awarenessMu.Unlock()

	for sender, body := range pending {
		d.Broadcast(EncodeAwarenessMessage(body), sender)
	}
}

// AddPendingEvent menambahkan update byte ke buffer pending untuk persistensi.
// Event baru membuat lastState basi (stateFresh=false) dan menandai bahwa
// snapshot perlu disimpan nanti (snapshotDue).
func (d *Document) AddPendingEvent(update []byte) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.pendingEvents = append(d.pendingEvents, append([]byte(nil), update...))
	d.eventCount++
	d.sinceLastSnapshot++
	d.stateFresh = false
	d.needsWriteBack = true
	d.snapshotDue = true
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

// RestorePendingEvents mengembalikan event ke buffer (dipakai worker kalau
// insert ke DB gagal — supaya update tidak hilang dan bisa dicoba lagi).
func (d *Document) RestorePendingEvents(events [][]byte, count int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	restored := make([][]byte, 0, len(events)+len(d.pendingEvents))
	restored = append(restored, events...)
	restored = append(restored, d.pendingEvents...)
	d.pendingEvents = restored
	d.eventCount += count
}

// PendingEventCount mengembalikan jumlah pending events.
func (d *Document) PendingEventCount() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.eventCount
}

// StateFresh: true kalau lastState memuat semua event yang sudah di-flush.
func (d *Document) StateFresh() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.stateFresh
}

// NeedsWriteBack: ada event yang belum tercakup lastState (perlu full state
// baru dari client).
func (d *Document) NeedsWriteBack() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.needsWriteBack
}

// SnapshotDue: ada event sejak snapshot terakhir.
func (d *Document) SnapshotDue() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.snapshotDue
}

// SinceLastSnapshot: jumlah event sejak snapshot terakhir.
func (d *Document) SinceLastSnapshot() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.sinceLastSnapshot
}

// ClearSnapshotDue dipanggil worker setelah snapshot berhasil disimpan.
func (d *Document) ClearSnapshotDue() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.snapshotDue = false
	d.sinceLastSnapshot = 0
}

// AddConnection mendaftarkan koneksi ke dokumen. Return list connection lain
// (untuk notifikasi join — opsional, dipakai di awareness Fase 3).
func (d *Document) AddConnection(c *Connection) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.connections[c] = struct{}{}
}

// RemoveConnection menghapus koneksi. Bersih-bersih termasuk presence-nya
// supaya user yang disconnect tidak "hantu" di daftar online.
func (d *Document) RemoveConnection(c *Connection) {
	d.mu.Lock()
	delete(d.connections, c)
	d.mu.Unlock()

	d.awarenessMu.Lock()
	delete(d.pendingAwareness, c)
	d.awarenessMu.Unlock()
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

// ForEachDirty mengunjungi semua dokumen yang butuh perhatian worker:
// ada pending events, butuh write-back, atau ada snapshot yang harus disimpan.
// Jika callback return false, iterasi berhenti.
func (s *Store) ForEachDirty(ctx context.Context, fn func(ctx context.Context, docID uuid.UUID, doc *Document) bool) {
	s.mu.RLock()
	ids := make([]uuid.UUID, 0, len(s.docs))
	for id, doc := range s.docs {
		if doc.PendingEventCount() > 0 || doc.NeedsWriteBack() || doc.SnapshotDue() {
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

// MaybeEvict membuang dokumen dari store kalau sudah tidak ada koneksi dan
// tidak ada data yang belum dipersist (pending events / write-back pending).
// Mencegah memory leak saat dokumen dibuka sekali lalu ditinggalkan.
// Dipanggil oleh handler setelah RemoveConnection.
func (s *Store) MaybeEvict(docID uuid.UUID) {
	s.mu.RLock()
	d, ok := s.docs[docID]
	s.mu.RUnlock()
	if !ok {
		return
	}

	d.mu.RLock()
	empty := len(d.connections) == 0
	hasPending := len(d.pendingEvents) > 0 || d.eventCount > 0
	needWB := d.needsWriteBack
	due := d.snapshotDue
	d.mu.RUnlock()

	if !empty || hasPending || needWB || due {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	// Double-check di bawah lock store (koneksi baru bisa saja masuk).
	dd, ok := s.docs[docID]
	if !ok {
		return
	}
	dd.mu.RLock()
	stillEmpty := len(dd.connections) == 0
	stillClean := len(dd.pendingEvents) == 0 && dd.eventCount == 0 && !dd.needsWriteBack && !dd.snapshotDue
	dd.mu.RUnlock()
	if stillEmpty && stillClean {
		delete(s.docs, docID)
	}
}

// Close menutup store — tidak menerima dokumen baru. Tidak menutup koneksi
// aktif (caller yang tangani via server shutdown).
func (s *Store) Close(ctx context.Context) error {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	return nil
}

// EvictStale membuang dokumen yang tidak punya koneksi aktif dan sudah
// "menyerah" pada write-back — yaitu snapshotDue / needsWriteBack masih true
// TAPI lastState sudah tersimpan (stateFresh dari write-back terakhir).
// Fix M2: dokumen yang diedit lalu ditinggalkan tidak pernah bisa di-evict
// karena needsWriteBack/snapshotDue hanya bisa dibersihkan via SetState yang
// butuh client online. Data TIDAK hilang: pending events sudah di-flush
// worker ke DB, dan lastState sudah tersimpan — dokumen akan di-rebuild dari
// DB saat ada client baru (lazy-load via GetOrCreate + snapshot/events).
func (s *Store) EvictStale() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	evicted := 0
	for id, d := range s.docs {
		d.mu.RLock()
		empty := len(d.connections) == 0
		hasState := d.lastState != nil
		noPending := len(d.pendingEvents) == 0 && d.eventCount == 0
		d.mu.RUnlock()

		// Dokumen tanpa koneksi + sudah punya state (bisa di-rebuild dari DB)
		// + tidak ada pending events yang belum di-flush → aman di-evict.
		// needsWriteBack/snapshotDue yang masih true tidak masalah: DB punya
		// semua events + lastState (via write-back), sehingga rebuild dari
		// snapshot/events tetap konsisten.
		if empty && hasState && noPending {
			delete(s.docs, id)
			evicted++
		}
	}
	return evicted
}
