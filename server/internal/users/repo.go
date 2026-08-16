// Package users berisi repository untuk tabel users. Tidak ada logic bisnis —
// hanya CRUD & query. Validation dan hashing dilakukan di layer service/handler.
package users

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pulse/server/internal/models"
)

// ErrNotFound dipakai handler untuk membedakan "tidak ada" vs error lain.
var ErrNotFound = errors.New("user not found")

// Repo — pgxpool dipakai langsung (bukan *sql.DB) karena pgx native lebih
// ergonomis & performan untuk query dengan banyak parameter/scan.
type Repo struct {
	pool *pgxpool.Pool
}

func NewRepo(pool *pgxpool.Pool) *Repo { return &Repo{pool: pool} }

// Create menyimpan user baru. Password harus sudah di-hash oleh caller.
func (r *Repo) Create(ctx context.Context, email, name, passwordHash string) (*models.User, error) {
	row := r.pool.QueryRow(ctx, `
		INSERT INTO users (email, name, password_hash)
		VALUES ($1, $2, $3)
		RETURNING id, email, name, password_hash, created_at, updated_at`,
		email, name, passwordHash,
	)
	var u models.User
	if err := row.Scan(&u.ID, &u.Email, &u.Name, &u.PasswordHash, &u.CreatedAt, &u.UpdatedAt); err != nil {
		return nil, fmt.Errorf("insert user: %w", err)
	}
	return &u, nil
}

// GetByEmail untuk lookup saat login. CITEXT di DB memastikan case-insensitive.
func (r *Repo) GetByEmail(ctx context.Context, email string) (*models.User, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, email, name, password_hash, created_at, updated_at
		FROM users WHERE email = $1`, email)
	var u models.User
	if err := row.Scan(&u.ID, &u.Email, &u.Name, &u.PasswordHash, &u.CreatedAt, &u.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get user by email: %w", err)
	}
	return &u, nil
}

// GetByID untuk load profil user (dipakai middleware setelah parse JWT).
func (r *Repo) GetByID(ctx context.Context, id uuid.UUID) (*models.User, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, email, name, password_hash, created_at, updated_at
		FROM users WHERE id = $1`, id)
	var u models.User
	if err := row.Scan(&u.ID, &u.Email, &u.Name, &u.PasswordHash, &u.CreatedAt, &u.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get user by id: %w", err)
	}
	return &u, nil
}

// EmailTaken mengecek apakah email sudah dipakai. Dipakai pre-flight di register
// supaya error message jelas (bukan rely pada unique constraint violation).
func (r *Repo) EmailTaken(ctx context.Context, email string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE email = $1)`, email).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check email taken: %w", err)
	}
	return exists, nil
}

// UpdateName mengubah display name user.
func (r *Repo) UpdateName(ctx context.Context, id uuid.UUID, name string) error {
	ct, err := r.pool.Exec(ctx, `UPDATE users SET name = $1 WHERE id = $2`, name, id)
	if err != nil {
		return fmt.Errorf("update name: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// GetEmailByID mendapatkan email user berdasarkan ID.
func (r *Repo) GetEmailByID(ctx context.Context, id uuid.UUID) (string, error) {
	var email string
	err := r.pool.QueryRow(ctx, `SELECT email FROM users WHERE id = $1`, id).Scan(&email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", fmt.Errorf("get email by id: %w", err)
	}
	return email, nil
}
