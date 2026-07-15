-- +goose Up
-- Schema awal Pulse (Fase 1). 7 tabel.
--
-- Catatan desain:
--  * CITEXT untuk email → lookup case-insensitive di DB level (tidak ada bug
--    "Foo@x.com" ≠ "foo@x.com"). Butuh extension citext.
--  * workspace_role enum → type-safe; mencegah role invalid walau app buggy.
--  * BIGSERIAL untuk snapshots/events → antisipasi jutaan baris (CRDT high-write).
--  * Tabel snapshots & events dibuat sekarang walau Fase 1 belum menulisnya,
--    supaya schema final di awal (menghindari migration besar di Fase 4).

CREATE EXTENSION IF NOT EXISTS citext;
-- gen_random_uuid() tersedia built-in sejak PG13; extension pgcrypto tidak wajib,
-- tapi kami pasang untuk kompatibilitas mundur dengan tooling yang mungkin memanggilnya.
CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- ============================================================================
-- 1. users
-- ============================================================================
CREATE TABLE users (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email         CITEXT UNIQUE NOT NULL,
    name          TEXT NOT NULL,
    password_hash TEXT NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX users_email_idx ON users(email);

-- updated_at auto-touch via trigger supaya konsisten tanpa bergantung pada app.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION touch_updated_at() RETURNS trigger AS $$
BEGIN
    NEW.updated_at := now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER users_touch_updated_at
    BEFORE UPDATE ON users
    FOR EACH ROW EXECUTE FUNCTION touch_updated_at();

-- ============================================================================
-- 2. workspaces
-- ============================================================================
CREATE TABLE workspaces (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT NOT NULL,
    slug        TEXT UNIQUE NOT NULL,
    created_by  UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX workspaces_slug_idx ON workspaces(slug);
CREATE INDEX workspaces_created_by_idx ON workspaces(created_by);

CREATE TRIGGER workspaces_touch_updated_at
    BEFORE UPDATE ON workspaces
    FOR EACH ROW EXECUTE FUNCTION touch_updated_at();

-- ============================================================================
-- 3. workspace_members
-- ============================================================================
-- +goose StatementBegin
DO $$ BEGIN
    CREATE TYPE workspace_role AS ENUM ('owner', 'editor', 'viewer');
EXCEPTION WHEN duplicate_object THEN NULL; END $$;
-- +goose StatementEnd

CREATE TABLE workspace_members (
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    user_id      UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role         workspace_role NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (workspace_id, user_id)
);
CREATE INDEX workspace_members_user_idx ON workspace_members(user_id);

-- ============================================================================
-- 4. documents
-- ============================================================================
CREATE TABLE documents (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    title        TEXT NOT NULL DEFAULT 'Untitled',
    created_by   UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX documents_workspace_idx ON documents(workspace_id);
CREATE INDEX documents_updated_idx ON documents(updated_at DESC);

CREATE TRIGGER documents_touch_updated_at
    BEFORE UPDATE ON documents
    FOR EACH ROW EXECUTE FUNCTION touch_updated_at();

-- ============================================================================
-- 5. document_snapshots — state Yjs lengkap (binary) pada titik waktu tertentu.
--    Diisi oleh persistence worker di Fase 4.
-- ============================================================================
CREATE TABLE document_snapshots (
    id          BIGSERIAL PRIMARY KEY,
    document_id UUID NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    state       BYTEA NOT NULL,        -- Y.encodeStateAsUpdate output
    version     INTEGER NOT NULL,      -- nomor snapshot yang naik monoton
    event_count INTEGER NOT NULL DEFAULT 0,  -- jumlah event sejak snapshot sebelumnya
    created_by  UUID REFERENCES users(id),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX snapshots_doc_version_idx ON document_snapshots(document_id, version DESC);

-- ============================================================================
-- 6. document_events — log incremental update Yjs.
--    Dipakai untuk replay saat load (snapshot + events setelahnya).
-- ============================================================================
CREATE TABLE document_events (
    id          BIGSERIAL PRIMARY KEY,
    document_id UUID NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    update      BYTEA NOT NULL,        -- raw Yjs update bytes
    origin      BYTEA,                 -- client-defined origin (undo per-user Fase 5)
    created_by  UUID REFERENCES users(id),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX events_doc_created_idx ON document_events(document_id, created_at);

-- ============================================================================
-- 7. refresh_tokens — refresh token rotation & revocation.
--    Tambahan terhadap daftar task §5; lihat docs/phase-01-summary.md alasan.
-- ============================================================================
CREATE TABLE refresh_tokens (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    -- token_hash = SHA-256 dari token plaintext. Kami tidak simpan plaintext:
    -- kalau DB bocor, attacker tidak bisa langsung memakai token.
    token_hash  TEXT NOT NULL UNIQUE,
    -- family_id mengelompokkan rantai token hasil rotasi dari satu login.
    -- Reuse token di tengah rantai → revoke seluruh family (token theft defense).
    family_id   UUID NOT NULL,
    -- created_by_rotation NULL = token dari login awal (root of family).
    -- Selain itu = token hasil rotasi sebelumnya, untuk audit.
    created_by_rotation UUID REFERENCES refresh_tokens(id) ON DELETE SET NULL,
    expires_at  TIMESTAMPTZ NOT NULL,
    revoked_at  TIMESTAMPTZ,           -- NULL = masih aktif
    reused_at   TIMESTAMPTZ,           -- set kalau token ini dipakai lagi (anomali)
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    user_agent  TEXT,
    ip_address  TEXT
);
CREATE INDEX refresh_tokens_user_idx ON refresh_tokens(user_id);
CREATE INDEX refresh_tokens_family_idx ON refresh_tokens(family_id);
CREATE INDEX refresh_tokens_hash_idx ON refresh_tokens(token_hash);
-- Cleanup index: query "hapus token expired" jadi cepat.
CREATE INDEX refresh_tokens_expires_idx ON refresh_tokens(expires_at);

-- +goose Down
-- Rollback schema awal. Urutan terbalik dari up untuk menghormati FK.

DROP TABLE IF EXISTS refresh_tokens;
DROP TABLE IF EXISTS document_events;
DROP TABLE IF EXISTS document_snapshots;
DROP TABLE IF EXISTS documents;
DROP TABLE IF EXISTS workspace_members;
DROP TYPE IF EXISTS workspace_role;
DROP TABLE IF EXISTS workspaces;
DROP TABLE IF EXISTS users;

DROP FUNCTION IF EXISTS touch_updated_at() CASCADE;
DROP EXTENSION IF EXISTS pgcrypto;
DROP EXTENSION IF EXISTS citext;
