-- +goose Up
-- Document shares table untuk granular document-level permissions
-- Memungkinkan owner share document ke specific users tanpa perlu invite ke workspace

CREATE TABLE document_shares (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    document_id     UUID NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    shared_with_id  UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    shared_by_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    permission      TEXT NOT NULL CHECK (permission IN ('view', 'edit')),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    
    -- Prevent duplicate shares
    UNIQUE(document_id, shared_with_id)
);

-- Indexes for fast lookup
CREATE INDEX idx_document_shares_document ON document_shares(document_id);
CREATE INDEX idx_document_shares_user ON document_shares(shared_with_id);

-- +goose Down
DROP TABLE IF EXISTS document_shares;
