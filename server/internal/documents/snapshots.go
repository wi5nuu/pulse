package documents

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pulse/server/internal/models"
)

// SnapshotRepo mengelola document_snapshots.
type SnapshotRepo struct {
	pool *pgxpool.Pool
}

func NewSnapshotRepo(pool *pgxpool.Pool) *SnapshotRepo {
	return &SnapshotRepo{pool: pool}
}

type SnapshotInfo struct {
	ID         int64     `json:"id"`
	CreatedAt  string    `json:"createdAt"`
	EventCount int       `json:"eventCount"`
}

func (r *SnapshotRepo) ListByDocument(ctx context.Context, docID uuid.UUID) ([]SnapshotInfo, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, event_count, created_at
		FROM document_snapshots
		WHERE document_id = $1
		ORDER BY created_at DESC
		LIMIT 50`, docID)
	if err != nil {
		return nil, fmt.Errorf("list snapshots: %w", err)
	}
	defer rows.Close()
	var out []SnapshotInfo
	for rows.Next() {
		var s SnapshotInfo
		if err := rows.Scan(&s.ID, &s.EventCount, &s.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan snapshot: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// SaveSnapshot menyimpan snapshot baru.
func (r *SnapshotRepo) SaveSnapshot(ctx context.Context, docID uuid.UUID, state []byte, eventCount int, createdBy *uuid.UUID) error {
	// Use Serializable to prevent concurrent version increment race.
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return fmt.Errorf("begin snapshot tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// Get current max version under lock.
	var maxVersion int
	err = tx.QueryRow(ctx, `
		SELECT COALESCE(MAX(version), 0) FROM document_snapshots WHERE document_id = $1`,
		docID).Scan(&maxVersion)
	if err != nil {
		return fmt.Errorf("get max version: %w", err)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO document_snapshots (document_id, state, version, event_count, created_by)
		VALUES ($1, $2, $3, $4, $5)`,
		docID, state, maxVersion+1, eventCount, createdBy)
	if err != nil {
		return fmt.Errorf("save snapshot: %w", err)
	}

	return tx.Commit(ctx)
}

// GetByID mengambil snapshot berdasarkan ID.
func (r *SnapshotRepo) GetByID(ctx context.Context, snapshotID int64) (*models.DocumentSnapshot, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, document_id, state, version, event_count, created_by, created_at
		FROM document_snapshots
		WHERE id = $1`, snapshotID)
	s := &models.DocumentSnapshot{}
	err := row.Scan(&s.ID, &s.DocumentID, &s.State, &s.Version, &s.EventCount, &s.CreatedBy, &s.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get snapshot by id: %w", err)
	}
	return s, nil
}

// GetLatestSnapshot mengambil snapshot terbaru untuk dokumen.
func (r *SnapshotRepo) GetLatestSnapshot(ctx context.Context, docID uuid.UUID) (*models.DocumentSnapshot, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, document_id, state, version, event_count, created_by, created_at
		FROM document_snapshots
		WHERE document_id = $1
		ORDER BY version DESC
		LIMIT 1`, docID)
	s := &models.DocumentSnapshot{}
	err := row.Scan(&s.ID, &s.DocumentID, &s.State, &s.Version, &s.EventCount, &s.CreatedBy, &s.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get latest snapshot: %w", err)
	}
	return s, nil
}

// LoadEventsSince mengambil update events dari document_events yang dibuat setelah
// timestamp tertentu. Dipakai untuk replay events setelah snapshot saat load dokumen.
func (r *SnapshotRepo) LoadEventsSince(ctx context.Context, docID uuid.UUID, since time.Time) ([][]byte, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT update FROM document_events
		WHERE document_id = $1 AND created_at > $2
		ORDER BY created_at`, docID, since)
	if err != nil {
		return nil, fmt.Errorf("load events since: %w", err)
	}
	defer rows.Close()
	var out [][]byte
	for rows.Next() {
		var update []byte
		if err := rows.Scan(&update); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}
		out = append(out, update)
	}
	return out, rows.Err()
}

// PruneEventsBeforeSnapshot menghapus document_events yang lebih OLD dari
// snapshot terbaru dokumen. Dipanggil setelah snapshot disimpan (m3 fix:
// tabel events tidak boleh tumbuh tanpa batas — snapshot sudah menangkap
// semua state, event lama tidak perlu lagi).
func (r *SnapshotRepo) PruneEventsBeforeSnapshot(ctx context.Context, docID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `
		DELETE FROM document_events e
		WHERE e.document_id = $1
		  AND e.created_at < COALESCE((
		      SELECT created_at FROM document_snapshots
		      WHERE document_id = $1
		      ORDER BY version DESC LIMIT 1
		  ), '-infinity'::timestamptz)`,
		docID)
	if err != nil {
		return fmt.Errorf("prune events: %w", err)
	}
	return nil
}


