// Package models berisi plain struct domain yang dipakai lintas layer
// (repository → handler). Tidak ada logic bisnis di sini — hanya data shape.
//
// Konvensi: field time selalu time.Time (UTC), bukan string. Konversi ke
// ISO-8601 string hanya di layer HTTP (handler/DTO).
package models

import (
	"time"

	"github.com/google/uuid"
)

// Role yang mungkin untuk anggota workspace. String (bukan custom type)
// supaya mudah di-scan dari enum Postgres; validasi tetap di repository.
const (
	RoleOwner  = "owner"
	RoleEditor = "editor"
	RoleViewer = "viewer"
)

type User struct {
	ID           uuid.UUID
	Email        string
	Name         string
	PasswordHash string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type Workspace struct {
	ID        uuid.UUID
	Name      string
	Slug      string
	CreatedBy uuid.UUID
	CreatedAt time.Time
	UpdatedAt time.Time
}

// WorkspaceMember adalah row di workspace_members. Role sebagai string
// karena datang dari enum Postgres; repository yang memvalidasi nilainya.
type WorkspaceMember struct {
	WorkspaceID uuid.UUID
	UserID      uuid.UUID
	Role        string
	CreatedAt   time.Time
}

type Document struct {
	ID          uuid.UUID
	WorkspaceID uuid.UUID
	Title       string
	CreatedBy   uuid.UUID
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// DocumentSnapshot dipakai di Fase 4. Struct sudah disiapkan sekarang.
type DocumentSnapshot struct {
	ID         int64
	DocumentID uuid.UUID
	State      []byte
	Version    int
	EventCount int
	CreatedBy  *uuid.UUID // nullable (snapshot otomatis tidak punya user)
	CreatedAt  time.Time
}

// DocumentEvent dipakai di Fase 4.
type DocumentEvent struct {
	ID         int64
	DocumentID uuid.UUID
	Update     []byte
	Origin     []byte // nullable
	CreatedBy  *uuid.UUID
	CreatedAt  time.Time
}

// RefreshToken dipakai internal oleh auth service. Tidak pernah diserialisasi
// ke client (hanya token plaintext yang dikirim via cookie, sekali).
type RefreshToken struct {
	ID                uuid.UUID
	UserID            uuid.UUID
	TokenHash         string
	FamilyID          uuid.UUID
	CreatedByRotation *uuid.UUID
	ExpiresAt         time.Time
	RevokedAt         *time.Time
	ReusedAt          *time.Time
	CreatedAt         time.Time
	UserAgent         *string
	IPAddress         *string
}
