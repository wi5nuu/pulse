// Package comments berisi repository untuk fitur kolaborasi lanjutan:
//   - document_comments      — komentar + reply pada dokumen (fiturwajibada I)
//   - document_link_shares   — share via link "Anyone with the link" (fiturwajibada H.168)
package comments

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound          = errors.New("comment/link share not found")
	ErrExpired           = errors.New("link share expired")
	ErrAlreadyExists     = errors.New("comment already in that state")
	ErrLinkShareDowngrade = errors.New("cannot downgrade link share permission")
)

type Repo struct {
	pool *pgxpool.Pool
}

func NewRepo(pool *pgxpool.Pool) *Repo { return &Repo{pool: pool} }

// Comment adalah komentar pada dokumen. anchor = JSON {"from":n,"to":n}
// (offset ProseMirror). Nil TIDAK di-normalisasi karena offset berubah
// saat dokumen diedit — client yang menyesuaikan posisi saat render.
type Comment struct {
	ID         uuid.UUID
	DocumentID uuid.UUID
	AuthorID   uuid.UUID
	AuthorName string
	AuthorEmail string
	Anchor     string
	Body       string
	ParentID   *uuid.UUID
	Resolved   bool
	ResolvedBy *uuid.UUID
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// ListComments mengambil semua komentar dokumen (root + reply) terurut
// kronologis. Dipakai panel komentar & render anchor.
func (r *Repo) ListComments(ctx context.Context, documentID uuid.UUID) ([]*Comment, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT c.id, c.document_id, c.author_id, u.name, u.email,
		       c.anchor, c.body, c.parent_id, c.resolved, c.resolved_by,
		       c.created_at, c.updated_at
		FROM document_comments c
		JOIN users u ON u.id = c.author_id
		WHERE c.document_id = $1
		ORDER BY c.created_at ASC, c.id ASC`, documentID)
	if err != nil {
		return nil, fmt.Errorf("list comments: %w", err)
	}
	defer rows.Close()
	var out []*Comment
	for rows.Next() {
		c, err := scanComment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// CreateComment membuat komentar baru (root atau reply via ParentID).
func (r *Repo) CreateComment(ctx context.Context, documentID, authorID uuid.UUID, anchor, body string, parentID *uuid.UUID) (*Comment, error) {
	if parentID != nil {
		// Validasi parent milik dokumen yang sama.
		var exists bool
		err := r.pool.QueryRow(ctx, `
			SELECT EXISTS(SELECT 1 FROM document_comments WHERE id = $1 AND document_id = $2)`,
			*parentID, documentID).Scan(&exists)
		if err != nil {
			return nil, fmt.Errorf("check parent comment: %w", err)
		}
		if !exists {
			return nil, ErrNotFound
		}
	}
	row := r.pool.QueryRow(ctx, `
		INSERT INTO document_comments (document_id, author_id, anchor, body, parent_id)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, document_id, author_id, anchor, body, parent_id, resolved, resolved_by, created_at, updated_at`,
		documentID, authorID, anchor, body, parentID)
	c, err := scanComment(row)
	if err != nil {
		return nil, fmt.Errorf("create comment: %w", err)
	}
	// Lampirkan nama/email author (join manual — nilai sudah dimiliki caller).
	return c, nil
}

// SetResolved menandai komentar selesai/batal. Return comment terbaru.
func (r *Repo) SetResolved(ctx context.Context, commentID, actorID uuid.UUID, resolved bool) (*Comment, error) {
	row := r.pool.QueryRow(ctx, `
		UPDATE document_comments
		SET resolved = $1,
		    resolved_by = CASE WHEN $1 THEN $2 ELSE NULL END,
		    updated_at = now()
		WHERE id = $3
		RETURNING id, document_id, author_id, anchor, body, parent_id, resolved, resolved_by, created_at, updated_at`,
		resolved, actorID, commentID)
	c, err := scanComment(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("set comment resolved: %w", err)
	}
	return c, nil
}

// DeleteComment menghapus komentar (reply ikut terhapus via ON DELETE CASCADE).
func (r *Repo) DeleteComment(ctx context.Context, commentID uuid.UUID) error {
	ct, err := r.pool.Exec(ctx, `DELETE FROM document_comments WHERE id = $1`, commentID)
	if err != nil {
		return fmt.Errorf("delete comment: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// GetComment retrieves a single comment by ID.
func (r *Repo) GetComment(ctx context.Context, commentID uuid.UUID) (*Comment, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT c.id, c.document_id, c.author_id, u.name, u.email,
		       c.anchor, c.body, c.parent_id, c.resolved, c.resolved_by,
		       c.created_at, c.updated_at
		FROM document_comments c
		JOIN users u ON u.id = c.author_id
		WHERE c.id = $1`, commentID)
	c, err := scanComment(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get comment: %w", err)
	}
	return c, nil
}

// --- Link share ---

type LinkShare struct {
	ID         uuid.UUID
	DocumentID uuid.UUID
	Token      string
	Permission string
	CreatedBy  *uuid.UUID
	ExpiresAt  *time.Time
	CreatedAt  time.Time
}

// CreateLinkShare membuat link share baru. Token acak (crypto/rand hex).
// Kembalikan ErrAlreadyExists jika doc sudah punya link share aktif untuk
// permission yang sama — client bisa pakai yang lama.
func (r *Repo) CreateLinkShare(ctx context.Context, documentID uuid.UUID, permission string, createdBy uuid.UUID, expiresAt *time.Time) (*LinkShare, error) {
	var existingID uuid.UUID
	var existingPerm string
	err := r.pool.QueryRow(ctx, `
		SELECT id, permission FROM document_link_shares
		WHERE document_id = $1 AND (expires_at IS NULL OR expires_at > now())`,
		documentID).Scan(&existingID, &existingPerm)
	if err == nil {
		// Block permission downgrade: existing "edit" cannot be replaced with "view"
		if existingPerm == "edit" && permission == "view" {
			return nil, ErrLinkShareDowngrade
		}
		// Same permission = already exists
		if existingPerm == permission {
			return nil, ErrAlreadyExists
		}
		// Permission upgrade: delete old and create new
		_, _ = r.pool.Exec(ctx, `DELETE FROM document_link_shares WHERE id = $1`, existingID)
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("check existing link share: %w", err)
	}

	row := r.pool.QueryRow(ctx, `
		INSERT INTO document_link_shares (document_id, token, permission, created_by, expires_at)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, document_id, token, permission, created_by, expires_at, created_at`,
		documentID, newToken(), permission, createdBy, expiresAt)
	s, err := scanLinkShare(row)
	if err != nil {
		return nil, fmt.Errorf("create link share: %w", err)
	}
	return s, nil
}

// ListLinkShares mengambil semua link share dokumen (hanya yang aktif/belum expired).
func (r *Repo) ListLinkShares(ctx context.Context, documentID uuid.UUID) ([]*LinkShare, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, document_id, token, permission, created_by, expires_at, created_at
		FROM document_link_shares
		WHERE document_id = $1 AND (expires_at IS NULL OR expires_at > now())
		ORDER BY created_at DESC`, documentID)
	if err != nil {
		return nil, fmt.Errorf("list link shares: %w", err)
	}
	defer rows.Close()
	var out []*LinkShare
	for rows.Next() {
		s, err := scanLinkShare(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// GetByToken: cari link share by token. Validasi expiry di sini — expired
// dianggap tidak ditemukan (client tidak boleh tahu bedanya).
func (r *Repo) GetByToken(ctx context.Context, token string) (*LinkShare, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, document_id, token, permission, created_by, expires_at, created_at
		FROM document_link_shares
		WHERE token = $1`, token)
	s, err := scanLinkShare(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get link share by token: %w", err)
	}
	if s.ExpiresAt != nil && s.ExpiresAt.Before(time.Now()) {
		return nil, ErrExpired
	}
	return s, nil
}

// DeleteLinkShare menghapus link share (hanya jika milik dokumen yang diminta).
func (r *Repo) DeleteLinkShare(ctx context.Context, shareID, documentID uuid.UUID) error {
	ct, err := r.pool.Exec(ctx, `DELETE FROM document_link_shares WHERE id = $1 AND document_id = $2`, shareID, documentID)
	if err != nil {
		return fmt.Errorf("delete link share: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanComment(row rowScanner) (*Comment, error) {
	c := &Comment{}
	var parentID, resolvedBy *uuid.UUID
	err := row.Scan(&c.ID, &c.DocumentID, &c.AuthorID, &c.AuthorName, &c.AuthorEmail,
		&c.Anchor, &c.Body, &parentID, &c.Resolved, &resolvedBy, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return nil, err
	}
	c.ParentID = parentID
	c.ResolvedBy = resolvedBy
	return c, nil
}

func scanLinkShare(row rowScanner) (*LinkShare, error) {
	s := &LinkShare{}
	var createdBy *uuid.UUID
	err := row.Scan(&s.ID, &s.DocumentID, &s.Token, &s.Permission, &createdBy, &s.ExpiresAt, &s.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	s.CreatedBy = createdBy
	return s, nil
}
