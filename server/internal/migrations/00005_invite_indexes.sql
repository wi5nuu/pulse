-- +goose Up
-- Add indexes for workspace_invites table to improve query performance
CREATE INDEX IF NOT EXISTS idx_workspace_invites_token ON workspace_invites(token);
CREATE INDEX IF NOT EXISTS idx_workspace_invites_email ON workspace_invites(email);
CREATE INDEX IF NOT EXISTS idx_workspace_invites_workspace_email ON workspace_invites(workspace_id, email) WHERE accepted = false;
CREATE INDEX IF NOT EXISTS idx_workspace_invites_expires ON workspace_invites(expires_at) WHERE accepted = false;

-- +goose Down
DROP INDEX IF EXISTS idx_workspace_invites_token;
DROP INDEX IF EXISTS idx_workspace_invites_email;
DROP INDEX IF EXISTS idx_workspace_invites_workspace_email;
DROP INDEX IF EXISTS idx_workspace_invites_expires;
