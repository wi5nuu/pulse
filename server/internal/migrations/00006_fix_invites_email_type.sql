-- +goose Up
-- Fix workspace_invites.email type mismatch: TEXT -> CITEXT for case-insensitive comparison
-- This ensures email comparison works correctly with users.email which is CITEXT

ALTER TABLE workspace_invites ALTER COLUMN email TYPE CITEXT;

-- +goose Down
ALTER TABLE workspace_invites ALTER COLUMN email TYPE TEXT;
