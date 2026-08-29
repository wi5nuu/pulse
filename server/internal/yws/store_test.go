package yws

import (
	"testing"

	"github.com/google/uuid"
)

func newTestStore() *Store {
	return NewStore(nil)
}

func TestStore_GetOrCreate_SameDocSameInstance(t *testing.T) {
	s := newTestStore()
	id := uuid.New()
	a := s.GetOrCreate(id)
	b := s.GetOrCreate(id)
	if a != b {
		t.Fatal("GetOrCreate must return the same Document instance")
	}
}

func TestStore_SetState(t *testing.T) {
	s := newTestStore()
	d := s.GetOrCreate(uuid.New())

	d.SetState([]byte("state-A"))
	state, ok := d.State()
	if !ok || string(state) != "state-A" {
		t.Fatalf("got state %q, ok=%v", state, ok)
	}
	if !d.StateFresh() {
		t.Fatal("state should be fresh after SetState with no pending events")
	}
	if d.NeedsWriteBack() {
		t.Fatal("no write-back needed when no pending events")
	}
}

func TestStore_SetState_DoesNotClearPendingEvents(t *testing.T) {
	// FIX C3: SetState TIDAK boleh mengosongkan pendingEvents. Update yang
	// in-flight bisa tidak tercakup state yang dikirim client — jika dibuang,
	// update hilang permanen.
	s := newTestStore()
	d := s.GetOrCreate(uuid.New())

	d.AddPendingEvent([]byte("event-1"))
	if d.PendingEventCount() != 1 {
		t.Fatal("expected 1 pending event")
	}

	// Client kirim full state — state-nya mungkin TIDAK memuat event-1.
	d.SetState([]byte("state-without-event-1"))

	if d.PendingEventCount() != 1 {
		t.Fatalf("SetState cleared pending events: got %d, want 1", d.PendingEventCount())
	}
	if d.StateFresh() {
		t.Fatal("state must NOT be fresh while pending events exist")
	}
	if !d.NeedsWriteBack() {
		t.Fatal("write-back still needed while pending events exist")
	}
}

func TestStore_GetReplayEvents_DoesNotClear(t *testing.T) {
	// FIX M1: GetReplayEvents harus TIDAK mengosongkan buffer — tiap koneksi
	// baru perlu menerima event yang sama (dua koneksi = dua kali replay).
	s := newTestStore()
	d := s.GetOrCreate(uuid.New())

	d.SetReplayEvents([][]byte{[]byte("ev-1"), []byte("ev-2")})

	first := d.GetReplayEvents()
	if len(first) != 2 {
		t.Fatalf("first replay: got %d events, want 2", len(first))
	}
	second := d.GetReplayEvents()
	if len(second) != 2 {
		t.Fatalf("second replay: got %d events, want 2 (buffer must not be cleared)", len(second))
	}
}

func TestStore_GetReplayEvents_ReturnsCopy(t *testing.T) {
	s := newTestStore()
	d := s.GetOrCreate(uuid.New())
	d.SetReplayEvents([][]byte{[]byte("original")})

	got := d.GetReplayEvents()
	got[0][0] = 'X'

	again := d.GetReplayEvents()
	if string(again[0]) != "original" {
		t.Fatalf("replay events must be defensive copies, got %q", again[0])
	}
}

func TestStore_AddPendingEvent_MarksDirty(t *testing.T) {
	s := newTestStore()
	d := s.GetOrCreate(uuid.New())

	d.SetState([]byte("clean-state"))
	if !d.StateFresh() {
		t.Fatal("precondition: state fresh")
	}

	d.AddPendingEvent([]byte("new-event"))

	if d.StateFresh() {
		t.Fatal("event must make state stale")
	}
	if !d.NeedsWriteBack() {
		t.Fatal("event must set needsWriteBack")
	}
	if !d.SnapshotDue() {
		t.Fatal("event must set snapshotDue")
	}
	if d.SinceLastSnapshot() != 1 {
		t.Fatalf("sinceLastSnapshot = %d, want 1", d.SinceLastSnapshot())
	}
}

func TestStore_GetAndClearPendingEvents_Restore(t *testing.T) {
	s := newTestStore()
	d := s.GetOrCreate(uuid.New())

	d.AddPendingEvent([]byte("a"))
	d.AddPendingEvent([]byte("b"))

	events, count := d.GetAndClearPendingEvents()
	if count != 2 || len(events) != 2 {
		t.Fatalf("got %d events (count=%d), want 2", len(events), count)
	}
	if d.PendingEventCount() != 0 {
		t.Fatal("buffer should be empty after GetAndClear")
	}

	// Simulasi insert DB gagal → restore supaya tidak hilang.
	d.RestorePendingEvents(events, count)
	if d.PendingEventCount() != 2 {
		t.Fatalf("after restore: got %d pending, want 2", d.PendingEventCount())
	}
}

func TestStore_EvictStale(t *testing.T) {
	// FIX M2: dokumen tanpa koneksi + sudah punya state + tidak ada pending
	// events → harus bisa di-evict (sebelumnya terjebak selamanya karena
	// needsWriteBack/snapshotDue tidak pernah dibersihkan).
	s := newTestStore()
	clean := s.GetOrCreate(uuid.New())
	clean.SetState([]byte("state"))
	clean.AddPendingEvent([]byte("flushed"))
	clean.GetAndClearPendingEvents() // worker sudah flush → no pending
	// needsWriteBack & snapshotDue tetap true (SetState belum dipanggil lagi)

	staleDirty := s.GetOrCreate(uuid.New())
	staleDirty.SetState([]byte("state"))
	staleDirty.AddPendingEvent([]byte("unflushed")) // masih pending → jangan evict

	evicted := s.EvictStale()
	if evicted != 1 {
		t.Fatalf("expected exactly 1 eviction, got %d", evicted)
	}

	if _, ok := s.docs[clean.ID]; ok {
		t.Fatal("clean doc should have been evicted")
	}
	if _, ok := s.docs[staleDirty.ID]; !ok {
		t.Fatal("doc with pending events must NOT be evicted")
	}
}

func TestStore_MaybeEvict(t *testing.T) {
	s := newTestStore()
	id := uuid.New()
	d := s.GetOrCreate(id)
	d.SetState([]byte("state"))

	// Tanpa koneksi & clean → evict.
	s.MaybeEvict(id)
	if _, ok := s.docs[id]; ok {
		t.Fatal("clean doc should be evicted by MaybeEvict")
	}

	// Dengan pending events → jangan evict.
	id2 := uuid.New()
	d2 := s.GetOrCreate(id2)
	d2.AddPendingEvent([]byte("x"))
	s.MaybeEvict(id2)
	if _, ok := s.docs[id2]; !ok {
		t.Fatal("doc with pending events must not be evicted")
	}
}

func TestStore_SetReplayEvents_DefensiveCopy(t *testing.T) {
	s := newTestStore()
	d := s.GetOrCreate(uuid.New())

	original := [][]byte{[]byte("replay-1")}
	d.SetReplayEvents(original)
	original[0][0] = 'X'

	got := d.GetReplayEvents()
	if string(got[0]) != "replay-1" {
		t.Fatalf("SetReplayEvents must copy input, got %q", got[0])
	}
}
