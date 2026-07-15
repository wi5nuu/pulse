-- +goose Up
ALTER TABLE workspace_invites ADD COLUMN invited_by UUID REFERENCES users(id);

-- +goose Down
ALTER TABLE workspace_invites DROP COLUMN IF EXISTS invited_by;
