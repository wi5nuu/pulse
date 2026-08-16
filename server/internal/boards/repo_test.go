package boards

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pulse/server/internal/users"
	"github.com/pulse/server/internal/workspaces"
)

// skipIfNoDB skips test jika tidak ada koneksi DB.
// Untuk CI/local: set DATABASE_URL env.
func skipIfNoDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := "postgres://pulse:pulse@localhost:5433/pulse?sslmode=disable"
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Skip("no database available:", err)
	}
	return pool
}

// setupEnv membuat user + workspace nyata supaya FK boards terpenuhi.
func setupEnv(t *testing.T, pool *pgxpool.Pool) (userID, wsID uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	u, err := users.NewRepo(pool).Create(ctx, fmt.Sprintf("test-%s@pulse.test", uuid.NewString()), "Test User", "x")
	if err != nil {
		t.Fatal("create user:", err)
	}
	ws, err := workspaces.NewRepo(pool).CreatePersonalWorkspace(ctx, u.ID, "Test WS "+uuid.NewString())
	if err != nil {
		t.Fatal("create workspace:", err)
	}
	return u.ID, ws.ID
}

// Fractional indexing: insert di tengah, posisi baru adalah rata-rata tetangga.
func TestFractionalIndex_Midpoint(t *testing.T) {
	pool := skipIfNoDB(t)
	defer pool.Close()
	repo := NewRepo(pool)
	ctx := context.Background()

	// Setup: buat board & column
	userID, wsID := setupEnv(t, pool)
	board, err := repo.CreateBoard(ctx, wsID, "Test Board", userID)
	if err != nil {
		t.Fatal("create board:", err)
	}
	col, err := repo.CreateColumn(ctx, board.ID, "Test Column", 1000)
	if err != nil {
		t.Fatal("create column:", err)
	}

	// Insert task pertama di posisi 1000
	t1, err := repo.CreateTask(ctx, col.ID, "Task 1", 1000, userID, nil, nil)
	if err != nil {
		t.Fatal("create task 1:", err)
	}
	// Insert task kedua di posisi 2000
	t2, err := repo.CreateTask(ctx, col.ID, "Task 2", 2000, userID, nil, nil)
	if err != nil {
		t.Fatal("create task 2:", err)
	}
	// Insert task di tengah: posisi harus antara 1000 dan 2000
	mid := (t1.Position + t2.Position) / 2
	t3, err := repo.CreateTask(ctx, col.ID, "Task Mid", mid, userID, nil, nil)
	if err != nil {
		t.Fatal("create task mid:", err)
	}
	if t3.Position <= t1.Position || t3.Position >= t2.Position {
		t.Errorf("mid position %f should be between %f and %f", t3.Position, t1.Position, t2.Position)
	}
}

// Insert di ujung: posisi increment dari yang terakhir.
func TestFractionalIndex_End(t *testing.T) {
	pool := skipIfNoDB(t)
	defer pool.Close()
	repo := NewRepo(pool)
	ctx := context.Background()

	userID, wsID := setupEnv(t, pool)
	board, err := repo.CreateBoard(ctx, wsID, "Test Board 2", userID)
	if err != nil {
		t.Fatal("create board:", err)
	}
	col, err := repo.CreateColumn(ctx, board.ID, "Col", 0)
	if err != nil {
		t.Fatal("create column:", err)
	}

	t1, err := repo.CreateTask(ctx, col.ID, "Task 1", 0, userID, nil, nil)
	if err != nil {
		t.Fatal("create task 1:", err)
	}
	// Task baru di ujung: posisi lebih besar dari t1
	t2, err := repo.CreateTask(ctx, col.ID, "Task 2", t1.Position+1, userID, nil, nil)
	if err != nil {
		t.Fatal("create task 2:", err)
	}
	if t2.Position <= t1.Position {
		t.Errorf("end position %f should be > %f", t2.Position, t1.Position)
	}
}

// Insert berulang: pastikan tidak error dalam skenario wajar.
func TestFractionalIndex_Repeated(t *testing.T) {
	pool := skipIfNoDB(t)
	defer pool.Close()
	repo := NewRepo(pool)
	ctx := context.Background()

	userID, wsID := setupEnv(t, pool)
	board, err := repo.CreateBoard(ctx, wsID, "Test Board 3", userID)
	if err != nil {
		t.Fatal("create board:", err)
	}
	col, err := repo.CreateColumn(ctx, board.ID, "Col", 0)
	if err != nil {
		t.Fatal("create column:", err)
	}

	for i := 0; i < 10; i++ {
		tasks, err := repo.ListTasksByColumn(ctx, col.ID)
		if err != nil {
			t.Fatal("list tasks:", err)
		}
		pos := float64(i * 1000)
		if len(tasks) > 0 {
			pos = tasks[len(tasks)-1].Position + 1000
		}
		_, err = repo.CreateTask(ctx, col.ID, "Task", pos, userID, nil, nil)
		if err != nil {
			t.Fatalf("create task %d: %v", i, err)
		}
	}
}

// Optimistic concurrency: update dengan version yang salah harus conflict.
func TestOptimisticConcurrency_VersionConflict(t *testing.T) {
	pool := skipIfNoDB(t)
	defer pool.Close()
	repo := NewRepo(pool)
	ctx := context.Background()

	userID, wsID := setupEnv(t, pool)
	board, err := repo.CreateBoard(ctx, wsID, "Test Board 4", userID)
	if err != nil {
		t.Fatal("create board:", err)
	}
	col, err := repo.CreateColumn(ctx, board.ID, "Col", 0)
	if err != nil {
		t.Fatal("create column:", err)
	}

	task, err := repo.CreateTask(ctx, col.ID, "Task", 1000, userID, nil, nil)
	if err != nil {
		t.Fatal("create task:", err)
	}
	// Update dengan version benar
	err = repo.UpdateTask(ctx, task.ID, "Updated", nil, nil, nil, task.Version)
	if err != nil {
		t.Fatal("update with correct version:", err)
	}
	// Update dengan version lama → harus conflict
	err = repo.UpdateTask(ctx, task.ID, "Conflict", nil, nil, nil, task.Version)
	if err != ErrVersionConflict {
		t.Errorf("expected ErrVersionConflict, got %v", err)
	}
}

// UpdateColumn dengan position baru.
func TestUpdateColumn_Position(t *testing.T) {
	pool := skipIfNoDB(t)
	defer pool.Close()
	repo := NewRepo(pool)
	ctx := context.Background()

	userID, wsID := setupEnv(t, pool)
	board, err := repo.CreateBoard(ctx, wsID, "Test Board", userID)
	if err != nil {
		t.Fatal("create board:", err)
	}
	col, err := repo.CreateColumn(ctx, board.ID, "Col A", 1000)
	if err != nil {
		t.Fatal("create column:", err)
	}

	pos := 500.0
	err = repo.UpdateColumn(ctx, col.ID, nil, &pos)
	if err != nil {
		t.Fatal("update column position:", err)
	}

	cols, err := repo.ListColumns(ctx, board.ID)
	if err != nil {
		t.Fatal("list columns:", err)
	}
	if len(cols) != 1 || cols[0].Position != 500 {
		t.Errorf("expected position 500, got %f", cols[0].Position)
	}
}

// UpdateColumn dengan title baru (position tetap).
func TestUpdateColumn_Title(t *testing.T) {
	pool := skipIfNoDB(t)
	defer pool.Close()
	repo := NewRepo(pool)
	ctx := context.Background()

	userID, wsID := setupEnv(t, pool)
	board, err := repo.CreateBoard(ctx, wsID, "Test Board", userID)
	if err != nil {
		t.Fatal("create board:", err)
	}
	col, err := repo.CreateColumn(ctx, board.ID, "Old Title", 1000)
	if err != nil {
		t.Fatal("create column:", err)
	}

	newTitle := "New Title"
	err = repo.UpdateColumn(ctx, col.ID, &newTitle, nil)
	if err != nil {
		t.Fatal("update column title:", err)
	}

	cols, err := repo.ListColumns(ctx, board.ID)
	if err != nil {
		t.Fatal("list columns:", err)
	}
	if len(cols) != 1 || cols[0].Title != "New Title" || cols[0].Position != 1000 {
		t.Errorf("expected title 'New Title' position 1000, got %q %f", cols[0].Title, cols[0].Position)
	}
}

// BoardWorkspaceID, ColumnWorkspaceID, TaskWorkspaceID.
func TestWorkspaceIDLookup(t *testing.T) {
	pool := skipIfNoDB(t)
	defer pool.Close()
	repo := NewRepo(pool)
	ctx := context.Background()

	userID, wsID := setupEnv(t, pool)
	board, err := repo.CreateBoard(ctx, wsID, "Test Board", userID)
	if err != nil {
		t.Fatal("create board:", err)
	}
	col, err := repo.CreateColumn(ctx, board.ID, "Col", 1000)
	if err != nil {
		t.Fatal("create column:", err)
	}

	task, err := repo.CreateTask(ctx, col.ID, "Task", 1000, userID, nil, nil)
	if err != nil {
		t.Fatal("create task:", err)
	}

	// Board → workspace
	foundWS, err := repo.BoardWorkspaceID(ctx, board.ID)
	if err != nil {
		t.Fatal("BoardWorkspaceID:", err)
	}
	if foundWS != wsID {
		t.Errorf("expected workspace %v, got %v", wsID, foundWS)
	}

	// Column → workspace
	foundWS, err = repo.ColumnWorkspaceID(ctx, col.ID)
	if err != nil {
		t.Fatal("ColumnWorkspaceID:", err)
	}
	if foundWS != wsID {
		t.Errorf("expected workspace %v, got %v", wsID, foundWS)
	}

	// Task → workspace
	foundWS, err = repo.TaskWorkspaceID(ctx, task.ID)
	if err != nil {
		t.Fatal("TaskWorkspaceID:", err)
	}
	if foundWS != wsID {
		t.Errorf("expected workspace %v, got %v", wsID, foundWS)
	}

	// Not found cases
	if _, err = repo.BoardWorkspaceID(ctx, uuid.New()); err != ErrNotFound {
		t.Error("expected ErrNotFound for missing board")
	}
	if _, err = repo.ColumnWorkspaceID(ctx, uuid.New()); err != ErrNotFound {
		t.Error("expected ErrNotFound for missing column")
	}
	if _, err = repo.TaskWorkspaceID(ctx, uuid.New()); err != ErrNotFound {
		t.Error("expected ErrNotFound for missing task")
	}
}
