// Package documents berisi repository untuk tabel documents + helpers untuk
// authorization dokumen (menentukan role user terhadap dokumen via workspace).
package documents

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pulse/server/internal/models"
)

var (
	ErrNotFound = errors.New("document not found")
)

type Repo struct {
	pool *pgxpool.Pool
}

func NewRepo(pool *pgxpool.Pool) *Repo { return &Repo{pool: pool} }

// Create dokumen baru dalam workspace. Caller harus pastikan user adalah anggota
// workspace dengan role yang boleh membuat dokumen (editor/owner).
func (r *Repo) Create(ctx context.Context, workspaceID uuid.UUID, title string, createdBy uuid.UUID) (*models.Document, error) {
	row := r.pool.QueryRow(ctx, `
		INSERT INTO documents (workspace_id, title, created_by)
		VALUES ($1, $2, $3)
		RETURNING id, workspace_id, title, created_by, created_at, updated_at`,
		workspaceID, title, createdBy,
	)
	return scanDocument(row)
}

// GetByID load dokumen + workspace_id (untuk authorization check).
func (r *Repo) GetByID(ctx context.Context, id uuid.UUID) (*models.Document, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, workspace_id, title, created_by, created_at, updated_at
		FROM documents WHERE id = $1`, id)
	return scanDocument(row)
}

// ListByWorkspace: dokumen dalam workspace, terbaru dulu. Dipakai sidebar UI.
func (r *Repo) ListByWorkspace(ctx context.Context, workspaceID uuid.UUID, limit int) ([]*models.Document, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, workspace_id, title, created_by, created_at, updated_at
		FROM documents
		WHERE workspace_id = $1
		ORDER BY updated_at DESC
		LIMIT $2`, workspaceID, limit)
	if err != nil {
		return nil, fmt.Errorf("list documents: %w", err)
	}
	defer rows.Close()
	var out []*models.Document
	for rows.Next() {
		d, err := scanDocumentRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// UpdateTitle: rename dokumen. Dipakai di UI editor.
func (r *Repo) UpdateTitle(ctx context.Context, id uuid.UUID, title string) error {
	ct, err := r.pool.Exec(ctx, `
		UPDATE documents SET title = $1 WHERE id = $2`, title, id)
	if err != nil {
		return fmt.Errorf("update document title: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// Delete menghapus dokumen.
func (r *Repo) Delete(ctx context.Context, id uuid.UUID) error {
	ct, err := r.pool.Exec(ctx, `DELETE FROM documents WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete document: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// TouchUpdated: increment updated_at saat dokumen diedit (dipakai Fase 4 worker
// setelah write-back, supaya sorting di sidebar akurat).
func (r *Repo) TouchUpdated(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `UPDATE documents SET updated_at = now() WHERE id = $1`, id)
	return err
}

// MemberRole: cari role user terhadap dokumen (lewati workspace). Return
// ("", ErrNotFound) kalau user bukan anggota workspace dokumen tsb.
// Dipakai di WS handshake untuk authorization.
func (r *Repo) MemberRole(ctx context.Context, docID, userID uuid.UUID) (string, error) {
	var role string
	err := r.pool.QueryRow(ctx, `
		SELECT wm.role
		FROM documents d
		JOIN workspace_members wm ON wm.workspace_id = d.workspace_id
		WHERE d.id = $1 AND wm.user_id = $2`,
		docID, userID,
	).Scan(&role)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", fmt.Errorf("query member role: %w", err)
	}
	return role, nil
}

// WorkspaceOfUser: workspace pertama user (untuk Fase 2 simple UX —
// nanti support multi-workspace). Dipakai handler list dokumen.
func (r *Repo) WorkspaceOfUser(ctx context.Context, userID uuid.UUID) (uuid.UUID, error) {
	var wsID uuid.UUID
	err := r.pool.QueryRow(ctx, `
		SELECT workspace_id FROM workspace_members
		WHERE user_id = $1 ORDER BY created_at LIMIT 1`, userID,
	).Scan(&wsID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, ErrNotFound
		}
		return uuid.Nil, fmt.Errorf("query user workspace: %w", err)
	}
	return wsID, nil
}

// --- scan helpers ---

type rowScanner interface {
	Scan(dest ...any) error
}

func scanDocument(row rowScanner) (*models.Document, error) {
	d := &models.Document{}
	err := row.Scan(&d.ID, &d.WorkspaceID, &d.Title, &d.CreatedBy, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("scan document: %w", err)
	}
	return d, nil
}

func scanDocumentRow(row interface {
	Scan(dest ...any) error
	Next() bool
}) (*models.Document, error) {
	d := &models.Document{}
	if err := row.Scan(&d.ID, &d.WorkspaceID, &d.Title, &d.CreatedBy, &d.CreatedAt, &d.UpdatedAt); err != nil {
		return nil, fmt.Errorf("scan document row: %w", err)
	}
	return d, nil
}
