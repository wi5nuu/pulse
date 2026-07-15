-- +goose Up
-- Workspace invite table untuk invitation flow.

CREATE TABLE workspace_invites (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    email       TEXT NOT NULL,
    role        TEXT NOT NULL CHECK (role IN ('editor', 'viewer')),
    token       TEXT UNIQUE NOT NULL,
    expires_at  TIMESTAMPTZ NOT NULL,
    accepted    BOOLEAN NOT NULL DEFAULT FALSE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX invites_token_idx ON workspace_invites(token);
CREATE INDEX invites_workspace_idx ON workspace_invites(workspace_id);

-- +goose Down
DROP TABLE IF EXISTS workspace_invites;
