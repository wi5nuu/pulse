package boards

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("board not found")
var ErrVersionConflict = errors.New("task version conflict")

type Board struct {
	ID          uuid.UUID
	WorkspaceID uuid.UUID
	Name        string
	CreatedBy   uuid.UUID
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type Column struct {
	ID       uuid.UUID
	BoardID  uuid.UUID
	Title    string
	Position float64
}

type Task struct {
	ID          uuid.UUID
	ColumnID    uuid.UUID
	Title       string
	Description *string
	AssigneeID  *uuid.UUID
	Position    float64
	Version     int
	CreatedBy   uuid.UUID
	UpdatedAt   time.Time
}

type Repo struct {
	pool *pgxpool.Pool
}

func NewRepo(pool *pgxpool.Pool) *Repo {
	return &Repo{pool: pool}
}

func (r *Repo) CreateBoard(ctx context.Context, wsID uuid.UUID, name string, createdBy uuid.UUID) (*Board, error) {
	row := r.pool.QueryRow(ctx, `
		INSERT INTO boards (workspace_id, name, created_by)
		VALUES ($1, $2, $3)
		RETURNING id, workspace_id, name, created_by, created_at, updated_at`,
		wsID, name, createdBy,
	)
	return scanBoard(row)
}

func (r *Repo) GetBoard(ctx context.Context, id uuid.UUID) (*Board, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, workspace_id, name, created_by, created_at, updated_at
		FROM boards WHERE id = $1`, id)
	return scanBoard(row)
}

func (r *Repo) ListBoards(ctx context.Context, wsID uuid.UUID) ([]*Board, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, workspace_id, name, created_by, created_at, updated_at
		FROM boards WHERE workspace_id = $1
		ORDER BY created_at`, wsID)
	if err != nil {
		return nil, fmt.Errorf("list boards: %w", err)
	}
	defer rows.Close()
	var out []*Board
	for rows.Next() {
		b, err := scanBoardRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (r *Repo) CreateColumn(ctx context.Context, boardID uuid.UUID, title string, position float64) (*Column, error) {
	row := r.pool.QueryRow(ctx, `
		INSERT INTO board_columns (board_id, title, position)
		VALUES ($1, $2, $3)
		RETURNING id, board_id, title, position`,
		boardID, title, position,
	)
	return scanColumn(row)
}

func (r *Repo) UpdateColumn(ctx context.Context, id uuid.UUID, title *string, position *float64) error {
	ct, err := r.pool.Exec(ctx, `
		UPDATE board_columns SET
			title = COALESCE($2, title),
			position = COALESCE($3, position)
		WHERE id = $1`, id, title, position)
	if err != nil {
		return fmt.Errorf("update column: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repo) DeleteColumn(ctx context.Context, id uuid.UUID) error {
	ct, err := r.pool.Exec(ctx, `DELETE FROM board_columns WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete column: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repo) ListColumns(ctx context.Context, boardID uuid.UUID) ([]*Column, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, board_id, title, position
		FROM board_columns WHERE board_id = $1
		ORDER BY position`, boardID)
	if err != nil {
		return nil, fmt.Errorf("list columns: %w", err)
	}
	defer rows.Close()
	var out []*Column
	for rows.Next() {
		c, err := scanColumnRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *Repo) ListColumnsAndTasks(ctx context.Context, boardID uuid.UUID) ([]*Column, []*Task, error) {
	columns, err := r.ListColumns(ctx, boardID)
	if err != nil {
		return nil, nil, err
	}
	tasks, err := r.ListTasks(ctx, boardID)
	if err != nil {
		return nil, nil, err
	}
	return columns, tasks, nil
}

// BoardWorkspaceID mengembalikan workspace ID untuk sebuah board.
func (r *Repo) BoardWorkspaceID(ctx context.Context, boardID uuid.UUID) (uuid.UUID, error) {
	var wsID uuid.UUID
	err := r.pool.QueryRow(ctx, `
		SELECT workspace_id FROM boards WHERE id = $1`, boardID).Scan(&wsID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, ErrNotFound
		}
		return uuid.Nil, fmt.Errorf("workspace id by board: %w", err)
	}
	return wsID, nil
}

// ColumnWorkspaceID mengembalikan workspace ID untuk sebuah column (via board).
func (r *Repo) ColumnWorkspaceID(ctx context.Context, columnID uuid.UUID) (uuid.UUID, error) {
	var wsID uuid.UUID
	err := r.pool.QueryRow(ctx, `
		SELECT b.workspace_id FROM board_columns bc
		JOIN boards b ON b.id = bc.board_id
		WHERE bc.id = $1`, columnID).Scan(&wsID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, ErrNotFound
		}
		return uuid.Nil, fmt.Errorf("workspace id by column: %w", err)
	}
	return wsID, nil
}

// TaskWorkspaceID mengembalikan workspace ID untuk sebuah task (via column → board).
func (r *Repo) TaskWorkspaceID(ctx context.Context, taskID uuid.UUID) (uuid.UUID, error) {
	var wsID uuid.UUID
	err := r.pool.QueryRow(ctx, `
		SELECT b.workspace_id FROM tasks t
		JOIN board_columns bc ON bc.id = t.column_id
		JOIN boards b ON b.id = bc.board_id
		WHERE t.id = $1`, taskID).Scan(&wsID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, ErrNotFound
		}
		return uuid.Nil, fmt.Errorf("workspace id by task: %w", err)
	}
	return wsID, nil
}

// ListTasksByColumn mengembalikan task dalam satu column.
// BoardIDByColumn mengembalikan board ID untuk sebuah column.
func (r *Repo) BoardIDByColumn(ctx context.Context, columnID uuid.UUID) (uuid.UUID, error) {
	var boardID uuid.UUID
	err := r.pool.QueryRow(ctx, `
		SELECT board_id FROM board_columns WHERE id = $1`, columnID).Scan(&boardID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, ErrNotFound
		}
		return uuid.Nil, fmt.Errorf("board id by column: %w", err)
	}
	return boardID, nil
}

// BoardIDByTask mengembalikan board ID untuk sebuah task.
func (r *Repo) BoardIDByTask(ctx context.Context, taskID uuid.UUID) (uuid.UUID, error) {
	var boardID uuid.UUID
	err := r.pool.QueryRow(ctx, `
		SELECT bc.board_id FROM tasks t
		JOIN board_columns bc ON bc.id = t.column_id
		WHERE t.id = $1`, taskID).Scan(&boardID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, ErrNotFound
		}
		return uuid.Nil, fmt.Errorf("board id by task: %w", err)
	}
	return boardID, nil
}

func (r *Repo) ListTasksByColumn(ctx context.Context, columnID uuid.UUID) ([]*Task, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, column_id, title, description, assignee_id, position, version, created_by, updated_at
		FROM tasks
		WHERE column_id = $1
		ORDER BY position`, columnID)
	if err != nil {
		return nil, fmt.Errorf("list tasks by column: %w", err)
	}
	defer rows.Close()
	var out []*Task
	for rows.Next() {
		t, err := scanTaskRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (r *Repo) ListTasks(ctx context.Context, boardID uuid.UUID) ([]*Task, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT t.id, t.column_id, t.title, t.description, t.assignee_id, t.position, t.version, t.created_by, t.updated_at
		FROM tasks t
		JOIN board_columns bc ON bc.id = t.column_id
		WHERE bc.board_id = $1
		ORDER BY t.position`, boardID)
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	defer rows.Close()
	var out []*Task
	for rows.Next() {
		t, err := scanTaskRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (r *Repo) CreateTask(ctx context.Context, columnID uuid.UUID, title string, position float64, createdBy uuid.UUID, description *string, assigneeID *uuid.UUID) (*Task, error) {
	row := r.pool.QueryRow(ctx, `
		INSERT INTO tasks (column_id, title, description, assignee_id, position, created_by)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, column_id, title, description, assignee_id, position, version, created_by, updated_at`,
		columnID, title, description, assigneeID, position, createdBy,
	)
	return scanTask(row)
}

func (r *Repo) UpdateTask(ctx context.Context, id uuid.UUID, title *string, description *string, columnID *uuid.UUID, position *float64, version int) error {
	ct, err := r.pool.Exec(ctx, `
		UPDATE tasks SET
			title = COALESCE($2, title),
			description = COALESCE($3, description),
			column_id = COALESCE($4, column_id),
			position = COALESCE($5, position),
			version = version + 1
		WHERE id = $1 AND version = $6`,
		id, title, description, columnID, position, version,
	)
	if err != nil {
		return fmt.Errorf("update task: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrVersionConflict
	}
	return nil
}

func nullableStr(s *string) *string {
	if s == nil || *s == "" {
		return nil
	}
	return s
}

func (r *Repo) DeleteTask(ctx context.Context, id uuid.UUID) error {
	ct, err := r.pool.Exec(ctx, `DELETE FROM tasks WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete task: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanBoard(row rowScanner) (*Board, error) {
	b := &Board{}
	err := row.Scan(&b.ID, &b.WorkspaceID, &b.Name, &b.CreatedBy, &b.CreatedAt, &b.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("scan board: %w", err)
	}
	return b, nil
}

func scanBoardRow(rows interface {
	Scan(dest ...any) error
	Next() bool
}) (*Board, error) {
	b := &Board{}
	if err := rows.Scan(&b.ID, &b.WorkspaceID, &b.Name, &b.CreatedBy, &b.CreatedAt, &b.UpdatedAt); err != nil {
		return nil, fmt.Errorf("scan board row: %w", err)
	}
	return b, nil
}

func scanColumn(row rowScanner) (*Column, error) {
	c := &Column{}
	err := row.Scan(&c.ID, &c.BoardID, &c.Title, &c.Position)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("scan column: %w", err)
	}
	return c, nil
}

func scanColumnRow(rows interface {
	Scan(dest ...any) error
	Next() bool
}) (*Column, error) {
	c := &Column{}
	if err := rows.Scan(&c.ID, &c.BoardID, &c.Title, &c.Position); err != nil {
		return nil, fmt.Errorf("scan column row: %w", err)
	}
	return c, nil
}

func scanTask(row rowScanner) (*Task, error) {
	t := &Task{}
	err := row.Scan(&t.ID, &t.ColumnID, &t.Title, &t.Description, &t.AssigneeID, &t.Position, &t.Version, &t.CreatedBy, &t.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("scan task: %w", err)
	}
	return t, nil
}

func scanTaskRow(rows interface {
	Scan(dest ...any) error
	Next() bool
}) (*Task, error) {
	t := &Task{}
	if err := rows.Scan(&t.ID, &t.ColumnID, &t.Title, &t.Description, &t.AssigneeID, &t.Position, &t.Version, &t.CreatedBy, &t.UpdatedAt); err != nil {
		return nil, fmt.Errorf("scan task row: %w", err)
	}
	return t, nil
}
