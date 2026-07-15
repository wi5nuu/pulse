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
	defer flushTicker.Stop()
	defer snapTicker.Stop()

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
		}
	}
}

func (w *Worker) Done() <-chan struct{} {
	return w.done
}

func (w *Worker) flushPending(ctx context.Context) {
	w.store.ForEachDirty(ctx, func(ctx context.Context, docID uuid.UUID, doc *yws.Document) bool {
		events, count := doc.GetAndClearPendingEvents()
		if len(events) == 0 {
			return true
		}
		if err := w.batchInsertEvents(ctx, docID, events); err != nil {
			w.logger.Error("flush events failed", "doc", docID, "error", err)
			return true
		}
		w.logger.Debug("flushed events", "doc", docID, "count", count)
		if count >= w.cfg.MaxEventsBeforeSnapshot {
			if state, ok := doc.State(); ok {
				if err := w.snapRepo.SaveSnapshot(ctx, docID, state, count, nil); err != nil {
					w.logger.Error("save snapshot failed", "doc", docID, "error", err)
				} else {
					w.logger.Info("snapshot saved (event threshold)", "doc", docID, "count", count)
				}
			} else {
				w.requestSnapshot(ctx, docID, doc)
			}
		}
		_ = w.docRepo.TouchUpdated(ctx, docID)
		return true
	})
}

func (w *Worker) batchInsertEvents(ctx context.Context, docID uuid.UUID, events [][]byte) error {
	if len(events) == 0 {
		return nil
	}
	tx, err := w.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	for _, event := range events {
		if _, err := tx.Exec(ctx,
			`INSERT INTO document_events (document_id, update) VALUES ($1, $2)`,
			docID, event,
		); err != nil {
			return fmt.Errorf("insert event: %w", err)
		}
	}
	return tx.Commit(ctx)
}

func (w *Worker) checkSnapshots(ctx context.Context) {
	w.store.ForEachDirty(ctx, func(ctx context.Context, docID uuid.UUID, doc *yws.Document) bool {
		if state, ok := doc.State(); ok {
			events, _ := doc.GetAndClearPendingEvents()
			totalCount := len(events)
			if totalCount == 0 {
				return true
			}
			if err := w.batchInsertEvents(ctx, docID, events); err != nil {
				w.logger.Error("checkSnapshot flush failed", "doc", docID, "error", err)
				return true
			}
			if err := w.snapRepo.SaveSnapshot(ctx, docID, state, totalCount, nil); err != nil {
				w.logger.Error("checkSnapshot save failed", "doc", docID, "error", err)
			} else {
				w.logger.Info("periodic snapshot saved", "doc", docID, "count", totalCount)
			}
			_ = w.docRepo.TouchUpdated(ctx, docID)
		} else if doc.ConnectionCount() > 0 {
			w.requestSnapshot(ctx, docID, doc)
		}
		return true
	})
}

func (w *Worker) requestSnapshot(ctx context.Context, docID uuid.UUID, doc *yws.Document) {
	w.logger.Info("requesting snapshot from client", "doc", docID)
	emptySV := []byte{}
	msg := yws.BuildSyncStep1Message(emptySV)
	doc.Broadcast(msg, nil)
}
