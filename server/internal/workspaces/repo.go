// Package workspaces berisi repository untuk workspaces dan workspace_members.
package workspaces

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pulse/server/internal/models"
)

var ErrNotFound = errors.New("workspace not found")

type MemberWithUser struct {
	WorkspaceID uuid.UUID
	UserID      uuid.UUID
	Role        string
	JoinedAt    time.Time
	Name        string
	Email       string
}

type Repo struct {
	pool *pgxpool.Pool
}

func NewRepo(pool *pgxpool.Pool) *Repo { return &Repo{pool: pool} }

func (r *Repo) CreatePersonalWorkspace(ctx context.Context, ownerID uuid.UUID, name string) (*models.Workspace, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	slug := uniqueSlug(ctx, tx, slugify(name))

	row := tx.QueryRow(ctx, `
		INSERT INTO workspaces (name, slug, created_by)
		VALUES ($1, $2, $3)
		RETURNING id, name, slug, created_by, created_at, updated_at`,
		name, slug, ownerID,
	)
	var ws models.Workspace
	if err := row.Scan(&ws.ID, &ws.Name, &ws.Slug, &ws.CreatedBy, &ws.CreatedAt, &ws.UpdatedAt); err != nil {
		return nil, fmt.Errorf("insert workspace: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO workspace_members (workspace_id, user_id, role)
		VALUES ($1, $2, 'owner')`, ws.ID, ownerID); err != nil {
		return nil, fmt.Errorf("insert owner membership: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit workspace: %w", err)
	}
	return &ws, nil
}

// ListByUser mengembalikan semua workspace di mana user adalah anggota.
func (r *Repo) ListByUser(ctx context.Context, userID uuid.UUID) ([]*models.Workspace, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT w.id, w.name, w.slug, w.created_by, w.created_at, w.updated_at
		FROM workspaces w
		JOIN workspace_members wm ON wm.workspace_id = w.id
		WHERE wm.user_id = $1
		ORDER BY w.created_at DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("list workspaces by user: %w", err)
	}
	defer rows.Close()
	var out []*models.Workspace
	for rows.Next() {
		ws := &models.Workspace{}
		if err := rows.Scan(&ws.ID, &ws.Name, &ws.Slug, &ws.CreatedBy, &ws.CreatedAt, &ws.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan workspace: %w", err)
		}
		out = append(out, ws)
	}
	return out, rows.Err()
}

// Create workspace baru (beyond personal).
func (r *Repo) Create(ctx context.Context, name string, ownerID uuid.UUID) (*models.Workspace, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	slug := uniqueSlug(ctx, tx, slugify(name))
	row := tx.QueryRow(ctx, `
		INSERT INTO workspaces (name, slug, created_by)
		VALUES ($1, $2, $3)
		RETURNING id, name, slug, created_by, created_at, updated_at`,
		name, slug, ownerID)
	ws := &models.Workspace{}
	if err := row.Scan(&ws.ID, &ws.Name, &ws.Slug, &ws.CreatedBy, &ws.CreatedAt, &ws.UpdatedAt); err != nil {
		return nil, fmt.Errorf("insert workspace: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO workspace_members (workspace_id, user_id, role)
		VALUES ($1, $2, 'owner')`, ws.ID, ownerID); err != nil {
		return nil, fmt.Errorf("insert owner: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}
	return ws, nil
}

// GetByID load workspace by ID.
func (r *Repo) GetByID(ctx context.Context, id uuid.UUID) (*models.Workspace, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, name, slug, created_by, created_at, updated_at
		FROM workspaces WHERE id = $1`, id)
	ws := &models.Workspace{}
	err := row.Scan(&ws.ID, &ws.Name, &ws.Slug, &ws.CreatedBy, &ws.CreatedAt, &ws.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get workspace: %w", err)
	}
	return ws, nil
}

// ListMembers workspace dengan info user (name, email).
func (r *Repo) ListMembers(ctx context.Context, workspaceID uuid.UUID) ([]MemberWithUser, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT wm.workspace_id, wm.user_id, wm.role, wm.created_at,
			   u.name, u.email
		FROM workspace_members wm
		JOIN users u ON u.id = wm.user_id
		WHERE wm.workspace_id = $1
		ORDER BY wm.created_at`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list members: %w", err)
	}
	defer rows.Close()
	var out []MemberWithUser
	for rows.Next() {
		var m MemberWithUser
		if err := rows.Scan(&m.WorkspaceID, &m.UserID, &m.Role, &m.JoinedAt, &m.Name, &m.Email); err != nil {
			return nil, fmt.Errorf("scan member: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// UpdateMemberRole mengubah role anggota workspace (hanya owner).
func (r *Repo) UpdateMemberRole(ctx context.Context, workspaceID, userID uuid.UUID, role string) error {
	ct, err := r.pool.Exec(ctx, `
		UPDATE workspace_members SET role = $1
		WHERE workspace_id = $2 AND user_id = $3 AND role != 'owner'`,
		role, workspaceID, userID)
	if err != nil {
		return fmt.Errorf("update role: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// RemoveMember hapus anggota dari workspace.
func (r *Repo) RemoveMember(ctx context.Context, workspaceID, userID uuid.UUID) error {
	ct, err := r.pool.Exec(ctx, `
		DELETE FROM workspace_members
		WHERE workspace_id = $1 AND user_id = $2 AND role != 'owner'`,
		workspaceID, userID)
	if err != nil {
		return fmt.Errorf("remove member: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

type InviteDetail struct {
	ID            uuid.UUID
	WorkspaceID   uuid.UUID
	WorkspaceName string
	Email         string
	Role          string
	Token         string
	InvitedByID   *uuid.UUID
	InvitedByName *string
	Accepted      bool
	ExpiresAt     time.Time
	CreatedAt     time.Time
}

// CreateInvite buat invite link token.
func (r *Repo) CreateInvite(ctx context.Context, workspaceID uuid.UUID, email, role string, token string, expiresAt time.Time, invitedBy uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO workspace_invites (workspace_id, email, role, token, expires_at, invited_by)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		workspaceID, email, role, token, expiresAt, invitedBy)
	if err != nil {
		return fmt.Errorf("create invite: %w", err)
	}
	return nil
}

// GetInviteByToken mengembalikan detail undangan (workspace + inviter).
func (r *Repo) GetInviteByToken(ctx context.Context, token string) (*InviteDetail, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT wi.id, wi.workspace_id, w.name, wi.email, wi.role, wi.token,
			   wi.invited_by, u.name, wi.accepted, wi.expires_at, wi.created_at
		FROM workspace_invites wi
		JOIN workspaces w ON w.id = wi.workspace_id
		LEFT JOIN users u ON u.id = wi.invited_by
		WHERE wi.token = $1`, token)
	d := &InviteDetail{}
	err := row.Scan(&d.ID, &d.WorkspaceID, &d.WorkspaceName, &d.Email, &d.Role, &d.Token,
		&d.InvitedByID, &d.InvitedByName, &d.Accepted, &d.ExpiresAt, &d.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get invite: %w", err)
	}
	return d, nil
}

// AcceptInvite menerima undangan + add member.
func (r *Repo) AcceptInvite(ctx context.Context, token string, userID uuid.UUID) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var wsID uuid.UUID
	var role string
	var accepted bool
	err = tx.QueryRow(ctx, `
		SELECT workspace_id, role, accepted FROM workspace_invites
		WHERE token = $1 AND expires_at > now()
		FOR UPDATE`, token).Scan(&wsID, &role, &accepted)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("select invite: %w", err)
	}
	if accepted {
		return fmt.Errorf("invite already accepted")
	}

	if _, err := tx.Exec(ctx, `
		UPDATE workspace_invites SET accepted = true WHERE token = $1`, token); err != nil {
		return fmt.Errorf("accept invite: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO workspace_members (workspace_id, user_id, role)
		VALUES ($1, $2, $3)
		ON CONFLICT DO NOTHING`, wsID, userID, role); err != nil {
		return fmt.Errorf("add member: %w", err)
	}
	return tx.Commit(ctx)
}

func (r *Repo) GetMemberRole(ctx context.Context, workspaceID, userID uuid.UUID) (string, error) {
	var role string
	err := r.pool.QueryRow(ctx, `
		SELECT role FROM workspace_members
		WHERE workspace_id = $1 AND user_id = $2`,
		workspaceID, userID,
	).Scan(&role)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", fmt.Errorf("get member role: %w", err)
	}
	return role, nil
}

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	b.Grow(len(s))
	prevDash := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevDash = false
		default:
			if !prevDash && b.Len() > 0 {
				b.WriteRune('-')
				prevDash = true
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		out = "workspace"
	}
	return out
}

func uniqueSlug(ctx context.Context, tx pgx.Tx, base string) string {
	slug := base
	for i := 2; i < 1000; i++ {
		var exists bool
		err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM workspaces WHERE slug = $1)`, slug).Scan(&exists)
		if err != nil {
			return base
		}
		if !exists {
			return slug
		}
		slug = fmt.Sprintf("%s-%d", base, i)
	}
	return slug
}
