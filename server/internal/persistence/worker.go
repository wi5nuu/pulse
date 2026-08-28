package persistence

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pulse/server/internal/documents"
	"github.com/pulse/server/internal/yws"
)

type WorkerConfig struct {
	FlushInterval          time.Duration
	SnapshotInterval       time.Duration
	MaxEventsBeforeSnapshot int
}

func DefaultConfig() WorkerConfig {
	return WorkerConfig{
		FlushInterval:           5 * time.Second,
		SnapshotInterval:        3 * time.Minute,
		MaxEventsBeforeSnapshot: 100,
	}
}

type Worker struct {
	logger   *slog.Logger
	cfg      WorkerConfig
	done     chan struct{}
	store    *yws.Store
	snapRepo *documents.SnapshotRepo
	docRepo  *documents.Repo
	pool     *pgxpool.Pool
}

func NewWorker(
	logger *slog.Logger,
	cfg WorkerConfig,
	store *yws.Store,
	snapRepo *documents.SnapshotRepo,
	docRepo *documents.Repo,
	pool *pgxpool.Pool,
) *Worker {
	if logger == nil {
		logger = slog.Default()
	}
	return &Worker{
		logger:   logger.With("component", "persistence"),
		cfg:      cfg,
		done:     make(chan struct{}),
		store:    store,
		snapRepo: snapRepo,
		docRepo:  docRepo,
		pool:     pool,
	}
}

func (w *Worker) Start(ctx context.Context) {
	w.logger.Info("persistence worker started",
		"flushInterval", w.cfg.FlushInterval,
		"snapshotInterval", w.cfg.SnapshotInterval,
	)

	flushTicker := time.NewTicker(w.cfg.FlushInterval)
	snapTicker := time.NewTicker(w.cfg.SnapshotInterval)
	evictTicker := time.NewTicker(5 * time.Minute)
	defer flushTicker.Stop()
	defer snapTicker.Stop()
	defer evictTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			w.logger.Info("persistence worker stopped")
			close(w.done)
			return
		case <-flushTicker.C:
			w.flushPending(ctx)
		case <-snapTicker.C:
			w.checkSnapshots(ctx)
		case <-evictTicker.C:
			// Fix M2: evict dokumen yang sudah tidak punya koneksi + punya
			// state (bisa di-rebuild dari DB). Mencegah memory leak.
			evicted := w.store.EvictStale()
			if evicted > 0 {
				w.logger.Info("evicted stale documents", "count", evicted)
			}
		}
	}
}

func (w *Worker) Done() <-chan struct{} {
	return w.done
}

// FlushNow menyiram semua pending events ke DB secara sinkron. Dipanggil saat
// shutdown (fix M4): sebelum worker berhenti, sisa pending events (≤5 detik
// terakhir) harus masuk DB supaya tidak hilang saat restart.
func (w *Worker) FlushNow(ctx context.Context) {
	w.store.ForEachDirty(ctx, func(ctx context.Context, docID uuid.UUID, doc *yws.Document) bool {
		events, count := doc.GetAndClearPendingEvents()
		if len(events) > 0 {
			if err := w.batchInsertEvents(ctx, docID, events); err != nil {
				// Shutdown: tidak bisa retry lagi — kembalikan buffer, log.
				doc.RestorePendingEvents(events, count)
				w.logger.Error("final flush failed", "doc", docID, "error", err)
				return true
			}
			w.logger.Info("final flush complete", "doc", docID, "count", count)
			_ = w.docRepo.TouchUpdated(ctx, docID)
		}
		return true
	})
	w.store.EvictStale()
}

func (w *Worker) flushPending(ctx context.Context) {
	w.store.ForEachDirty(ctx, func(ctx context.Context, docID uuid.UUID, doc *yws.Document) bool {
		events, count := doc.GetAndClearPendingEvents()
		if len(events) > 0 {
			if err := w.batchInsertEvents(ctx, docID, events); err != nil {
				// Jangan kehilangan update: kembalikan ke buffer, coba lagi
				// di tick berikutnya.
				doc.RestorePendingEvents(events, count)
				w.logger.Error("flush events failed", "doc", docID, "error", err)
				return true
			}
			w.logger.Debug("flushed events", "doc", docID, "count", count)
			_ = w.docRepo.TouchUpdated(ctx, docID)
		}
		w.handleSnapshot(ctx, docID, doc)
		return true
	})
}

func (w *Worker) checkSnapshots(ctx context.Context) {
	w.store.ForEachDirty(ctx, func(ctx context.Context, docID uuid.UUID, doc *yws.Document) bool {
		events, count := doc.GetAndClearPendingEvents()
		if len(events) > 0 {
			if err := w.batchInsertEvents(ctx, docID, events); err != nil {
				doc.RestorePendingEvents(events, count)
				w.logger.Error("periodic flush failed", "doc", docID, "error", err)
				return true
			}
			_ = w.docRepo.TouchUpdated(ctx, docID)
		}
		w.handleSnapshot(ctx, docID, doc)
		return true
	})
}

func (w *Worker) batchInsertEvents(ctx context.Context, docID uuid.UUID, events [][]byte) error {
	if len(events) == 0 {
		return nil
	}
	
	// Use COPY for better performance and atomicity
	_, err := w.pool.CopyFrom(ctx, pgx.Identifier{"document_events"}, []string{"document_id", "update"},
		pgx.CopyFromSlice(len(events), func(i int) ([]interface{}, error) {
			return []interface{}{docID, events[i]}, nil
		}))
	if err != nil {
		return fmt.Errorf("batch insert events: %w", err)
	}
	return nil
}

// handleSnapshot menyimpan snapshot bila aman:
//   - snapshotDue (ada event sejak snapshot terakhir) DAN
//   - stateFresh (lastState memuat semua event yang sudah di-flush).
//
// Kalau state masih basi, minta full state dari client (write-back); snapshot
// disimpan di tick berikutnya setelah client membalas. Kalau tidak ada state
// sama sekali dan ada koneksi, minta state awal.
func (w *Worker) handleSnapshot(ctx context.Context, docID uuid.UUID, doc *yws.Document) {
	if !doc.SnapshotDue() {
		return
	}
	if state, ok := doc.State(); ok && doc.StateFresh() {
		eventCount := doc.SinceLastSnapshot()
		if err := w.snapRepo.SaveSnapshot(ctx, docID, state, eventCount, nil); err != nil {
			w.logger.Error("save snapshot failed", "doc", docID, "error", err)
			return
		}
		// m3 fix: prune events yang sudah tercakup snapshot terbaru supaya
		// tabel document_events tidak tumbuh tanpa batas.
		if err := w.snapRepo.PruneEventsBeforeSnapshot(ctx, docID); err != nil {
			w.logger.Warn("prune events failed", "doc", docID, "error", err)
		}
		doc.ClearSnapshotDue()
		w.logger.Info("snapshot saved", "doc", docID, "eventCount", eventCount)
		return
	}
	if doc.ConnectionCount() > 0 {
		w.requestSnapshot(ctx, docID, doc)
	}
}

func (w *Worker) requestSnapshot(ctx context.Context, docID uuid.UUID, doc *yws.Document) {
	w.logger.Debug("requesting snapshot from client", "doc", docID)
	emptySV := []byte{}
	msg := yws.BuildSyncStep1Message(emptySV)
	doc.Broadcast(msg, nil)
}
