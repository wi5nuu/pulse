-- +goose Up
-- Add CASCADE DELETE to document_shares and document_link_shares foreign keys
-- to ensure related data is cleaned up when a document is deleted.

-- First, drop existing foreign key constraints
ALTER TABLE document_shares DROP CONSTRAINT IF EXISTS document_shares_document_id_fkey;
ALTER TABLE document_link_shares DROP CONSTRAINT IF EXISTS document_link_shares_document_id_fkey;
ALTER TABLE document_comments DROP CONSTRAINT IF EXISTS document_comments_document_id_fkey;
ALTER TABLE document_snapshots DROP CONSTRAINT IF EXISTS document_snapshots_document_id_fkey;
ALTER TABLE document_events DROP CONSTRAINT IF EXISTS document_events_document_id_fkey;

-- Recreate with CASCADE DELETE
ALTER TABLE document_shares ADD CONSTRAINT document_shares_document_id_fkey 
    FOREIGN KEY (document_id) REFERENCES documents(id) ON DELETE CASCADE;
ALTER TABLE document_link_shares ADD CONSTRAINT document_link_shares_document_id_fkey 
    FOREIGN KEY (document_id) REFERENCES documents(id) ON DELETE CASCADE;
ALTER TABLE document_comments ADD CONSTRAINT document_comments_document_id_fkey 
    FOREIGN KEY (document_id) REFERENCES documents(id) ON DELETE CASCADE;
ALTER TABLE document_snapshots ADD CONSTRAINT document_snapshots_document_id_fkey 
    FOREIGN KEY (document_id) REFERENCES documents(id) ON DELETE CASCADE;
ALTER TABLE document_events ADD CONSTRAINT document_events_document_id_fkey 
    FOREIGN KEY (document_id) REFERENCES documents(id) ON DELETE CASCADE;

-- +goose Down
-- Revert to RESTRICT (default behavior)
ALTER TABLE document_shares DROP CONSTRAINT IF EXISTS document_shares_document_id_fkey;
ALTER TABLE document_link_shares DROP CONSTRAINT IF EXISTS document_link_shares_document_id_fkey;
ALTER TABLE document_comments DROP CONSTRAINT IF EXISTS document_comments_document_id_fkey;
ALTER TABLE document_snapshots DROP CONSTRAINT IF EXISTS document_snapshots_document_id_fkey;
ALTER TABLE document_events DROP CONSTRAINT IF EXISTS document_events_document_id_fkey;

ALTER TABLE document_shares ADD CONSTRAINT document_shares_document_id_fkey 
    FOREIGN KEY (document_id) REFERENCES documents(id) ON DELETE RESTRICT;
ALTER TABLE document_link_shares ADD CONSTRAINT document_link_shares_document_id_fkey 
    FOREIGN KEY (document_id) REFERENCES documents(id) ON DELETE RESTRICT;
ALTER TABLE document_comments ADD CONSTRAINT document_comments_document_id_fkey 
    FOREIGN KEY (document_id) REFERENCES documents(id) ON DELETE RESTRICT;
ALTER TABLE document_snapshots ADD CONSTRAINT document_snapshots_document_id_fkey 
    FOREIGN KEY (document_id) REFERENCES documents(id) ON DELETE RESTRICT;
ALTER TABLE document_events ADD CONSTRAINT document_events_document_id_fkey 
    FOREIGN KEY (document_id) REFERENCES documents(id) ON DELETE RESTRICT;