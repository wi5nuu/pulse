package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pulse/server/internal/models"
)

// Refresh errors. Dibuat var supaya handler bisa matching dengan errors.Is.
var (
	// ErrRefreshInvalid = token tidak dikenali (tidak ada di DB atau sudah di-revoke).
	ErrRefreshInvalid = errors.New("refresh token invalid")
	// ErrRefreshExpired = token ditemukan tapi sudah lewat expiry.
	ErrRefreshExpired = errors.New("refresh token expired")
	// ErrRefreshReused = token yang sudah di-rotate dipakai lagi → indikasi token theft.
	// Saat ini terdeteksi, seluruh family di-revoke (defense in depth).
	ErrRefreshReused = errors.New("refresh token reuse detected")
)

// RefreshService mengelola siklus hidup refresh token di DB.
//
// Model "rotating refresh tokens with reuse detection" adalah best practice
// industri (lihat Auth0, OAuth 2.0 BCP). Intinya:
//   1. Login → buat token A, simpan dengan family_id baru.
//   2. Refresh dengan A → revoke A, buat B dengan family_id yang sama, catat
//      created_by_rotation=A.
//   3. Refresh dengan B → revoke B, buat C, dst.
//   4. Kalau A dipakai lagi padahal sudah revoked → ANOMALI. Kemungkinan:
//      attacker mencuri A dan user asli juga masih punya A. Revoke seluruh
//      family (A, B, C, ...) supaya attacker tidak bisa lanjut.
type RefreshService struct {
	pool *pgxpool.Pool
	ttl  time.Duration
}

func NewRefreshService(pool *pgxpool.Pool, ttl time.Duration) *RefreshService {
	return &RefreshService{pool: pool, ttl: ttl}
}

// Issue membuat token baru (root family — dari login awal). Return token
// plaintext (yang dikirim ke client) SEKALI; DB hanya menyimpan hash.
func (s *RefreshService) Issue(ctx context.Context, userID uuid.UUID, userAgent, ipAddress string) (plaintext string, record *models.RefreshToken, err error) {
	familyID := uuid.New()
	return s.mint(ctx, userID, familyID, nil, userAgent, ipAddress)
}

// mint menghasilkan token baru. familyID dan createdByRotation dikontrol caller:
//   - Issue (login): familyID baru, createdByRotation=nil.
//   - Rotate (refresh): familyID lama, createdByRotation=token lama.
func (s *RefreshService) mint(ctx context.Context, userID, familyID uuid.UUID, createdByRotation *uuid.UUID, userAgent, ipAddress string) (string, *models.RefreshToken, error) {
	plaintext, hash, err := generateToken()
	if err != nil {
		return "", nil, err
	}

	now := time.Now().UTC()
	expiresAt := now.Add(s.ttl)
	id := uuid.New()

	var ua *string
	if userAgent != "" {
		ua = &userAgent
	}
	var ip *string
	if ipAddress != "" {
		ip = &ipAddress
	}

	row := s.pool.QueryRow(ctx, `
		INSERT INTO refresh_tokens
			(id, user_id, token_hash, family_id, created_by_rotation,
			 expires_at, user_agent, ip_address)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING created_at`,
		id, userID, hash, familyID, createdByRotation, expiresAt, ua, ip,
	)
	var createdAt time.Time
	if err := row.Scan(&createdAt); err != nil {
		return "", nil, fmt.Errorf("insert refresh token: %w", err)
	}

	rec := &models.RefreshToken{
		ID:                id,
		UserID:            userID,
		TokenHash:         hash,
		FamilyID:          familyID,
		CreatedByRotation: createdByRotation,
		ExpiresAt:         expiresAt,
		CreatedAt:         createdAt,
		UserAgent:         ua,
		IPAddress:         ip,
	}
	return plaintext, rec, nil
}

// Rotate: revoke oldToken, mint token baru di family yang sama.
// Return ErrRefreshReused kalau token lama sudah di-revoke sebelumnya.
// Saat reuse terdeteksi, seluruh family di-revoke untuk memutus rantai.
//
//	returns: (newPlaintext, newRecord, userID, err)
func (s *RefreshService) Rotate(ctx context.Context, oldPlaintext, userAgent, ipAddress string) (string, *models.RefreshToken, uuid.UUID, error) {
	hash := hashToken(oldPlaintext)

	// Transaksi: cek status token lama → revoke → mint baru → (atau revoke family).
	// Semua atomic supaya tidak ada race antara dua refresh paralel di token sama.
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return "", nil, uuid.Nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // rollback aman setelah commit

	var (
		oldID      uuid.UUID
		userID     uuid.UUID
		familyID   uuid.UUID
		revokedAt  *time.Time
		expiresAt  time.Time
	)
	err = tx.QueryRow(ctx, `
		SELECT id, user_id, family_id, revoked_at, expires_at
		FROM refresh_tokens
		WHERE token_hash = $1
		FOR UPDATE`, hash,
	).Scan(&oldID, &userID, &familyID, &revokedAt, &expiresAt)

	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return "", nil, uuid.Nil, ErrRefreshInvalid
	case err != nil:
		return "", nil, uuid.Nil, fmt.Errorf("select refresh token: %w", err)
	}

	// Anomali 1: token sudah di-revoke (pernah di-rotate). → reuse → revoke family.
	if revokedAt != nil {
		if _, err := tx.Exec(ctx, `
			UPDATE refresh_tokens SET revoked_at = now()
			WHERE family_id = $1 AND revoked_at IS NULL`, familyID); err != nil {
			return "", nil, uuid.Nil, fmt.Errorf("revoke family on reuse: %w", err)
		}
		if err := tx.Commit(ctx); err != nil {
			return "", nil, uuid.Nil, fmt.Errorf("commit reuse-revoke: %w", err)
		}
		return "", nil, uuid.Nil, ErrRefreshReused
	}

	// Anomali 2: token expired (tapi belum ke-scheduled cleanup). Tolak.
	if time.Now().UTC().After(expiresAt) {
		return "", nil, uuid.Nil, ErrRefreshExpired
	}

	// Happy path: revoke token lama, mint token baru di family yang sama.
	now := time.Now().UTC()
	if _, err := tx.Exec(ctx, `
		UPDATE refresh_tokens SET revoked_at = $1 WHERE id = $2`, now, oldID); err != nil {
		return "", nil, uuid.Nil, fmt.Errorf("revoke old token: %w", err)
	}

	newPlaintext, newHash, err := generateToken()
	if err != nil {
		return "", nil, uuid.Nil, err
	}
	newID := uuid.New()
	newExpires := now.Add(s.ttl)
	var ua *string
	if userAgent != "" {
		ua = &userAgent
	}
	var ip *string
	if ipAddress != "" {
		ip = &ipAddress
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO refresh_tokens
			(id, user_id, token_hash, family_id, created_by_rotation,
			 expires_at, user_agent, ip_address)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		newID, userID, newHash, familyID, &oldID, newExpires, ua, ip,
	); err != nil {
		return "", nil, uuid.Nil, fmt.Errorf("insert rotated token: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return "", nil, uuid.Nil, fmt.Errorf("commit rotation: %w", err)
	}

	rec := &models.RefreshToken{
		ID:                newID,
		UserID:            userID,
		TokenHash:         newHash,
		FamilyID:          familyID,
		CreatedByRotation: &oldID,
		ExpiresAt:         newExpires,
		CreatedAt:         now,
		UserAgent:         ua,
		IPAddress:         ip,
	}
	return newPlaintext, rec, userID, nil
}

// RevokeAll mematikan semua token aktif milik user (dipakai di logout "all devices").
func (s *RefreshService) RevokeAll(ctx context.Context, userID uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE refresh_tokens SET revoked_at = now()
		WHERE user_id = $1 AND revoked_at IS NULL`, userID)
	if err != nil {
		return fmt.Errorf("revoke all: %w", err)
	}
	return nil
}

// RevokeFamily mematikan semua token dalam family (dipakai saat logout single device).
func (s *RefreshService) RevokeFamily(ctx context.Context, plaintext string) error {
	hash := hashToken(plaintext)
	_, err := s.pool.Exec(ctx, `
		UPDATE refresh_tokens SET revoked_at = now()
		WHERE family_id = (SELECT family_id FROM refresh_tokens WHERE token_hash = $1)
		AND revoked_at IS NULL`, hash)
	if err != nil {
		return fmt.Errorf("revoke family: %w", err)
	}
	return nil
}

// CleanupExpired menghapus token yang sudah expired > 7 hari. Dipanggil periodik
// oleh background job (nanti, di Fase 4 worker). Skrg tidak di-wire untuk jaga
// Fase 1 minimal.
func (s *RefreshService) CleanupExpired(ctx context.Context) (int64, error) {
	ct, err := s.pool.Exec(ctx, `
		DELETE FROM refresh_tokens
		WHERE expires_at < now() - interval '7 days'`)
	if err != nil {
		return 0, fmt.Errorf("cleanup expired: %w", err)
	}
	return ct.RowsAffected(), nil
}

// generateToken menghasilkan token plaintext acak + hash SHA-256-nya.
// Kami simpan HANYA hash di DB. Plaintext diberikan ke client via cookie.
// 32 byte random = 256 bit entropy → tahan brute-force sampai panjang umur alam semesta.
func generateToken() (plaintext, hash string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", fmt.Errorf("generate token bytes: %w", err)
	}
	plaintext = base64.RawURLEncoding.EncodeToString(b)
	hash = hashToken(plaintext)
	return plaintext, hash, nil
}

// hashToken: SHA-256 hex dari plaintext. Cepat & deterministic.
// SHA-256 cukup di sini karena token punya 256-bit entropy — tidak mungkin
// di-brute-force jadi preimage. Bcrypt tidak perlu (tidak ada "salt" per token
// karena tiap token sudah unik random).
func hashToken(plaintext string) string {
	sum := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(sum[:])
}
