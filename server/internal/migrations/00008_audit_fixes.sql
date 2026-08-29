-- +goose Up
-- Fix migration audit:
--   1. Hapus duplicate indexes (kolom UNIQUE sudah otomatis punya index).
--   2. Hapus index duplikat invites token.
--   3. FK snapshots/events created_by → ON DELETE SET NULL (menghapus user
--      yang pernah membuat snapshot/event tidak boleh gagal).
--   4. Hapus document_events yang lebih lama dari snapshot terbaru (m3:
--      tabel events tidak boleh tumbuh tanpa batas).

-- 1. Duplicate index: users.email sudah UNIQUE (00001), index terpisah redundan.
DROP INDEX IF EXISTS users_email_idx;

-- 2. Duplicate index: workspaces.slug sudah UNIQUE (00001), index terpisah redundan.
DROP INDEX IF EXISTS workspaces_slug_idx;

-- 3. Duplicate index: invites.token sudah UNIQUE (00003), index terpisah redundan.
DROP INDEX IF EXISTS invites_token_idx;
DROP INDEX IF EXISTS idx_workspace_invites_token;

-- 4. FK dibuat saat tabel init (00001) tanpa ON DELETE → RESTRICT default.
--    Ubah ke SET NULL supaya penghapusan user yang pernah create snapshot/event
--    tetap bisa jalan (audit trail dipertahankan, user_id jadi NULL).
ALTER TABLE document_snapshots
    DROP CONSTRAINT IF EXISTS document_snapshots_created_by_fkey,
    ADD CONSTRAINT document_snapshots_created_by_fkey
        FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL;

ALTER TABLE document_events
    DROP CONSTRAINT IF EXISTS document_events_created_by_fkey,
    ADD CONSTRAINT document_events_created_by_fkey
        FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL;

-- 5. (m3) Prune document_events: hapus event yang sudah tercakup snapshot
--    terbaru dokumen. Jaga window: event harus lebih OLD dari snapshot
--    terbaru, dan snapshot terbaru harus lebih baru dari snapshot kedua-terbaru
--    (artinya snapshot yang terakhir memang snapshot terbaru yang valid).
--    Query ini berjalan sekali saat migration; runtime pruning dilakukan worker.
DELETE FROM document_events e
USING document_snapshots s
WHERE s.document_id = e.document_id
  AND s.created_at > e.created_at
  AND s.id = (
      SELECT id FROM document_snapshots s2
      WHERE s2.document_id = s.document_id
      ORDER BY created_at DESC LIMIT 1
  );

-- Index pendukung runtime pruning: find events yang lebih tua dari snapshot
-- terbaru per dokumen.
CREATE INDEX idx_document_events_snapshot_prune
    ON document_events (document_id, created_at);

-- +goose Down
-- Restore index (best-effort; index redundant tidak berdampak fungsional).
CREATE INDEX IF NOT EXISTS users_email_idx ON users(email);
CREATE INDEX IF NOT EXISTS workspaces_slug_idx ON workspaces(slug);
CREATE INDEX IF NOT EXISTS invites_token_idx ON workspace_invites(token);
CREATE INDEX IF NOT EXISTS idx_workspace_invites_token ON workspace_invites(token);

DROP INDEX IF EXISTS idx_document_events_snapshot_prune;
