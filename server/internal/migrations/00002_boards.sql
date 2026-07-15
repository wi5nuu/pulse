-- +goose Up
-- Boards, columns, dan tasks untuk Task Board (Tier 2).
-- Desain fractional indexing: position DOUBLE PRECISION.
-- Version column untuk optimistic concurrency check.

CREATE TABLE boards (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    created_by  UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX boards_workspace_idx ON boards(workspace_id);

CREATE TRIGGER boards_touch_updated_at
    BEFORE UPDATE ON boards
    FOR EACH ROW EXECUTE FUNCTION touch_updated_at();

CREATE TABLE board_columns (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    board_id    UUID NOT NULL REFERENCES boards(id) ON DELETE CASCADE,
    title       TEXT NOT NULL,
    position    DOUBLE PRECISION NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX board_columns_board_idx ON board_columns(board_id);

CREATE TABLE tasks (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    column_id   UUID NOT NULL REFERENCES board_columns(id) ON DELETE CASCADE,
    title       TEXT NOT NULL,
    description TEXT,
    assignee_id UUID REFERENCES users(id) ON DELETE SET NULL,
    position    DOUBLE PRECISION NOT NULL,
    version     INTEGER NOT NULL DEFAULT 1,
    created_by  UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX tasks_column_idx ON tasks(column_id);
CREATE INDEX tasks_assignee_idx ON tasks(assignee_id);

-- +goose Down
DROP TABLE IF EXISTS tasks;
DROP TABLE IF EXISTS board_columns;
DROP TABLE IF EXISTS boards;
