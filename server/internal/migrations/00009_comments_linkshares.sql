-- +goose Up
-- Fitur kolaborasi lanjutan (dari fiturwajibada.md):
--   1. document_comments — komentar pada dokumen (anchor posisi via posFrom/posTo
--      yang disimpan sebagai string "from:to" agar stabil terhadap edit Yjs).
--   2. document_link_shares — share via link (Anyone with the link, Google Docs
--      H.168): token acak, permission view/edit, optional expiry.

CREATE TABLE document_comments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    document_id UUID NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    author_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    -- Anchor posisi dalam dokumen. Simpan sebagai JSON string {"from":n,"to":n}
    -- dengan offset dari awal dokumen. Bukan FK — posisi tidak di-normalisasi
    -- karena ProseMirror offset berubah saat dokumen diedit.
    anchor TEXT NOT NULL,
    body TEXT NOT NULL CHECK (length(body) > 0),
    parent_id UUID REFERENCES document_comments(id) ON DELETE CASCADE,
    resolved BOOLEAN NOT NULL DEFAULT FALSE,
    resolved_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_document_comments_doc ON document_comments (document_id, created_at);
CREATE INDEX idx_document_comments_parent ON document_comments (parent_id);

CREATE TABLE document_link_shares (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    document_id UUID NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    token TEXT UNIQUE NOT NULL,
    permission TEXT NOT NULL CHECK (permission IN ('view','edit')),
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_document_link_shares_doc ON document_link_shares (document_id);

-- +goose Down
DROP TABLE IF EXISTS document_link_shares;
DROP TABLE IF EXISTS document_comments;