package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"

	"github.com/pulse/server/internal/boards"
	"github.com/pulse/server/internal/models"
	"github.com/pulse/server/internal/workspaces"
)

type BoardHandlers struct {
	repo     *boards.Repo
	wsRepo   *workspaces.Repo
	validate *validator.Validate
}

func NewBoardHandlers(repo *boards.Repo, wsRepo *workspaces.Repo) *BoardHandlers {
	return &BoardHandlers{
		repo:     repo,
		wsRepo:   wsRepo,
		validate: validator.New(validator.WithRequiredStructEnabled()),
	}
}

// requireEditor memeriksa apakah user memiliki role editor atau owner di workspace.
// Menulis error response dan return false jika bukan editor.
func (h *BoardHandlers) requireEditor(w http.ResponseWriter, r *http.Request, workspaceID uuid.UUID) bool {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, CodeUnauthorized, "no user")
		return false
	}
	role, err := h.wsRepo.GetMemberRole(r.Context(), workspaceID, uid)
	if err != nil {
		writeError(w, http.StatusForbidden, CodeForbidden, "not a workspace member")
		return false
	}
	if role == models.RoleViewer {
		writeError(w, http.StatusForbidden, CodeForbidden, "viewers cannot modify")
		return false
	}
	return true
}

// requireMember memastikan user adalah anggota workspace (role apapun).
// Menulis error response dan return false jika bukan anggota.
func (h *BoardHandlers) requireMember(w http.ResponseWriter, r *http.Request, workspaceID uuid.UUID) bool {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, CodeUnauthorized, "no user")
		return false
	}
	_, err := h.wsRepo.GetMemberRole(r.Context(), workspaceID, uid)
	if err != nil {
		writeError(w, http.StatusForbidden, CodeForbidden, "not a workspace member")
		return false
	}
	return true
}

// broadcastEvent mengirim event board ke semua client via WebSocket hub.
func (h *BoardHandlers) broadcastEvent(boardID uuid.UUID, eventType string, data interface{}) {
	evt := map[string]interface{}{
		"type": eventType,
		"data": data,
	}
	payload, err := json.Marshal(evt)
	if err != nil {
		return
	}
	BoardBroadcastEvent(boardID.String(), payload)
}

type createBoardRequest struct {
	Name string `json:"name" validate:"required,min=1,max=100"`
}

type boardDTO struct {
	ID          uuid.UUID `json:"id"`
	WorkspaceID uuid.UUID `json:"workspaceId"`
	Name        string    `json:"name"`
}

type columnDTO struct {
	ID       uuid.UUID `json:"id"`
	BoardID  uuid.UUID `json:"boardId"`
	Title    string    `json:"title"`
	Position float64   `json:"position"`
}

type taskDTO struct {
	ID          uuid.UUID  `json:"id"`
	ColumnID    uuid.UUID  `json:"columnId"`
	Title       string     `json:"title"`
	Description *string    `json:"description"`
	AssigneeID  *uuid.UUID `json:"assigneeId"`
	Position    float64    `json:"position"`
	Version     int        `json:"version"`
}

type createColumnRequest struct {
	Title    string   `json:"title" validate:"required,min=1,max=100"`
	Position *float64 `json:"position,omitempty"`
}

type createTaskRequest struct {
	Title       string     `json:"title" validate:"required,min=1,max=500"`
	Description *string    `json:"description,omitempty"`
	AssigneeID  *uuid.UUID `json:"assigneeId,omitempty"`
	Position    *float64   `json:"position,omitempty"`
}

type updateColumnRequest struct {
	Title    *string  `json:"title,omitempty"`
	Position *float64 `json:"position,omitempty"`
}

type updateTaskRequest struct {
	Title       *string    `json:"title,omitempty"`
	Description *string    `json:"description,omitempty"`
	ColumnID    *uuid.UUID `json:"columnId,omitempty"`
	Position    *float64   `json:"position,omitempty"`
	Version     int        `json:"version"`
}

// ListBoards di workspace.
func (h *BoardHandlers) ListBoards(w http.ResponseWriter, r *http.Request) {
	wsID, err := uuid.Parse(chi.URLParam(r, "workspaceID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, CodeBadRequest, "invalid workspace id")
		return
	}
	if !h.requireMember(w, r, wsID) {
		return
	}
	list, err := h.repo.ListBoards(r.Context(), wsID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, CodeInternal, "could not list boards")
		return
	}
	out := make([]boardDTO, 0, len(list))
	for _, b := range list {
		out = append(out, boardDTO{ID: b.ID, WorkspaceID: b.WorkspaceID, Name: b.Name})
	}
	writeJSON(w, http.StatusOK, map[string]any{"boards": out})
}

// CreateBoard di workspace.
func (h *BoardHandlers) CreateBoard(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, CodeUnauthorized, "no user")
		return
	}
	wsID, err := uuid.Parse(chi.URLParam(r, "workspaceID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, CodeBadRequest, "invalid workspace id")
		return
	}
	if !h.requireEditor(w, r, wsID) {
		return
	}
	var req createBoardRequest
	if !decodeValid(w, r, h.validate, &req) {
		return
	}
	b, err := h.repo.CreateBoard(r.Context(), wsID, req.Name, uid)
	if err != nil {
		writeError(w, http.StatusInternalServerError, CodeInternal, "could not create board")
		return
	}
	h.broadcastEvent(b.ID, "board_created", map[string]any{
		"id": b.ID, "name": b.Name,
	})
	writeJSON(w, http.StatusCreated, map[string]any{
		"board": boardDTO{ID: b.ID, WorkspaceID: b.WorkspaceID, Name: b.Name},
	})
}

// GetBoard dengan columns & tasks.
func (h *BoardHandlers) GetBoard(w http.ResponseWriter, r *http.Request) {
	boardID, err := uuid.Parse(chi.URLParam(r, "boardID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, CodeBadRequest, "invalid board id")
		return
	}
	// Authz: cek apakah board ada dulu, lalu cek keanggotaan workspace.
	wsID, err := h.repo.BoardWorkspaceID(r.Context(), boardID)
	if err != nil {
		if errors.Is(err, boards.ErrNotFound) {
			writeError(w, http.StatusNotFound, CodeNotFound, "board not found")
			return
		}
		writeError(w, http.StatusInternalServerError, CodeInternal, "could not load board")
		return
	}
	if !h.requireMember(w, r, wsID) {
		return
	}
	columns, tasks, err := h.repo.ListColumnsAndTasks(r.Context(), boardID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, CodeInternal, "could not load board")
		return
	}
	colOut := make([]columnDTO, 0, len(columns))
	for _, c := range columns {
		colOut = append(colOut, columnDTO{ID: c.ID, BoardID: c.BoardID, Title: c.Title, Position: c.Position})
	}
	taskOut := make([]taskDTO, 0, len(tasks))
	for _, t := range tasks {
		taskOut = append(taskOut, taskDTO{
			ID: t.ID, ColumnID: t.ColumnID, Title: t.Title,
			Description: t.Description, AssigneeID: t.AssigneeID,
			Position: t.Position, Version: t.Version,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"columns": colOut,
		"tasks":   taskOut,
	})
}

// CreateColumn di board.
func (h *BoardHandlers) CreateColumn(w http.ResponseWriter, r *http.Request) {
	boardID, err := uuid.Parse(chi.URLParam(r, "boardID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, CodeBadRequest, "invalid board id")
		return
	}
	wsID, err := h.repo.BoardWorkspaceID(r.Context(), boardID)
	if err != nil {
		if errors.Is(err, boards.ErrNotFound) {
			writeError(w, http.StatusNotFound, CodeNotFound, "board not found")
			return
		}
		writeError(w, http.StatusInternalServerError, CodeInternal, "could not load board")
		return
	}
	if !h.requireEditor(w, r, wsID) {
		return
	}
	var req createColumnRequest
	if !decodeValid(w, r, h.validate, &req) {
		return
	}
	position := 0.0
	if req.Position != nil {
		position = *req.Position
	} else {
		cols, err := h.repo.ListColumns(r.Context(), boardID)
		if err == nil && len(cols) > 0 {
			position = cols[len(cols)-1].Position + 1.0
		}
	}
	c, err := h.repo.CreateColumn(r.Context(), boardID, req.Title, position)
	if err != nil {
		writeError(w, http.StatusInternalServerError, CodeInternal, "could not create column")
		return
	}
	// Broadcast supaya client lain melihat kolom baru real-time.
	h.broadcastEvent(boardID, "column_created", map[string]any{
		"id": c.ID, "title": c.Title, "position": c.Position,
	})
	writeJSON(w, http.StatusCreated, map[string]any{
		"column": columnDTO{ID: c.ID, BoardID: c.BoardID, Title: c.Title, Position: c.Position},
	})
}

// UpdateColumn rename / reorder.
func (h *BoardHandlers) UpdateColumn(w http.ResponseWriter, r *http.Request) {
	colID, err := uuid.Parse(chi.URLParam(r, "columnID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, CodeBadRequest, "invalid column id")
		return
	}
	wsID, err := h.repo.ColumnWorkspaceID(r.Context(), colID)
	if err != nil {
		if errors.Is(err, boards.ErrNotFound) {
			writeError(w, http.StatusNotFound, CodeNotFound, "column not found")
			return
		}
		writeError(w, http.StatusInternalServerError, CodeInternal, "could not load column")
		return
	}
	if !h.requireEditor(w, r, wsID) {
		return
	}
	var req updateColumnRequest
	if !decodeValid(w, r, h.validate, &req) {
		return
	}
	if err := h.repo.UpdateColumn(r.Context(), colID, req.Title, req.Position); err != nil {
		if errors.Is(err, boards.ErrNotFound) {
			writeError(w, http.StatusNotFound, CodeNotFound, "column not found")
			return
		}
		writeError(w, http.StatusInternalServerError, CodeInternal, "could not update column")
		return
	}
	if boardID, e := h.repo.BoardIDByColumn(r.Context(), colID); e == nil {
		updatedFields := map[string]any{"id": colID}
		if req.Title != nil {
			updatedFields["title"] = *req.Title
		}
		if req.Position != nil {
			updatedFields["position"] = *req.Position
		}
		h.broadcastEvent(boardID, "column_updated", updatedFields)
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// DeleteColumn.
func (h *BoardHandlers) DeleteColumn(w http.ResponseWriter, r *http.Request) {
	colID, err := uuid.Parse(chi.URLParam(r, "columnID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, CodeBadRequest, "invalid column id")
		return
	}
	wsID, err := h.repo.ColumnWorkspaceID(r.Context(), colID)
	if err != nil {
		if errors.Is(err, boards.ErrNotFound) {
			writeError(w, http.StatusNotFound, CodeNotFound, "column not found")
			return
		}
		writeError(w, http.StatusInternalServerError, CodeInternal, "could not load column")
		return
	}
	if !h.requireEditor(w, r, wsID) {
		return
	}
	// Ambil board + daftar task SEBELUM delete, supaya client lain bisa
	// membersihkan task yang ikut terhapus via ON DELETE CASCADE.
	boardID, boardErr := h.repo.BoardIDByColumn(r.Context(), colID)
	var staleTasks []*boards.Task
	if boardErr == nil {
		if tasks, e := h.repo.ListTasksByColumn(r.Context(), colID); e == nil {
			staleTasks = tasks
		}
	}
	if err := h.repo.DeleteColumn(r.Context(), colID); err != nil {
		if errors.Is(err, boards.ErrNotFound) {
			writeError(w, http.StatusNotFound, CodeNotFound, "column not found")
			return
		}
		writeError(w, http.StatusInternalServerError, CodeInternal, "could not delete column")
		return
	}
	if boardErr == nil {
		// Broadcast task deletions dulu, baru column deletion — urutan penting
		// agar client tidak render task orphan saat menghapus kolom.
		for _, t := range staleTasks {
			h.broadcastEvent(boardID, "task_deleted", map[string]any{"id": t.ID})
		}
		h.broadcastEvent(boardID, "column_deleted", map[string]any{"id": colID})
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// CreateTask di column.
func (h *BoardHandlers) CreateTask(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, CodeUnauthorized, "no user")
		return
	}
	colID, err := uuid.Parse(chi.URLParam(r, "columnID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, CodeBadRequest, "invalid column id")
		return
	}
	wsID, err := h.repo.ColumnWorkspaceID(r.Context(), colID)
	if err != nil {
		if errors.Is(err, boards.ErrNotFound) {
			writeError(w, http.StatusNotFound, CodeNotFound, "column not found")
			return
		}
		writeError(w, http.StatusInternalServerError, CodeInternal, "could not load column")
		return
	}
	if !h.requireEditor(w, r, wsID) {
		return
	}
	var req createTaskRequest
	if !decodeValid(w, r, h.validate, &req) {
		return
	}
	position := 0.0
	if req.Position != nil {
		position = *req.Position
	} else {
		tasks, err := h.repo.ListTasksByColumn(r.Context(), colID)
		if err == nil && len(tasks) > 0 {
			position = tasks[len(tasks)-1].Position + 1.0
		}
	}
	t, err := h.repo.CreateTask(r.Context(), colID, req.Title, position, uid, req.Description, req.AssigneeID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, CodeInternal, "could not create task")
		return
	}
	if boardID, e := h.repo.BoardIDByColumn(r.Context(), colID); e == nil {
		// Pakai taskDTO (camelCase) supaya konsisten dengan payload REST lain.
		h.broadcastEvent(boardID, "task_created", taskDTO{
			ID: t.ID, ColumnID: t.ColumnID, Title: t.Title,
			Description: t.Description, AssigneeID: t.AssigneeID,
			Position: t.Position, Version: t.Version,
		})
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"task": taskDTO{
			ID: t.ID, ColumnID: t.ColumnID, Title: t.Title,
			Description: t.Description, AssigneeID: t.AssigneeID,
			Position: t.Position, Version: t.Version,
		},
	})
}

// UpdateTask dengan version check.
func (h *BoardHandlers) UpdateTask(w http.ResponseWriter, r *http.Request) {
	taskID, err := uuid.Parse(chi.URLParam(r, "taskID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, CodeBadRequest, "invalid task id")
		return
	}
	wsID, err := h.repo.TaskWorkspaceID(r.Context(), taskID)
	if err != nil {
		if errors.Is(err, boards.ErrNotFound) {
			writeError(w, http.StatusNotFound, CodeNotFound, "task not found")
			return
		}
		writeError(w, http.StatusInternalServerError, CodeInternal, "could not load task")
		return
	}
	if !h.requireEditor(w, r, wsID) {
		return
	}
	var req updateTaskRequest
	if !decodeValid(w, r, h.validate, &req) {
		return
	}
	err = h.repo.UpdateTask(r.Context(), taskID, req.Title, req.Description, req.ColumnID, req.Position, req.Version)
	if err != nil {
		if errors.Is(err, boards.ErrVersionConflict) {
			writeError(w, http.StatusConflict, CodeConflict, "task was modified by another user, please refresh")
			return
		}
		if errors.Is(err, boards.ErrNotFound) {
			writeError(w, http.StatusNotFound, CodeNotFound, "task not found")
			return
		}
		writeError(w, http.StatusInternalServerError, CodeInternal, "could not update task")
		return
	}
	if boardID, e := h.repo.BoardIDByTask(r.Context(), taskID); e == nil {
		updatedFields := map[string]any{"id": taskID}
		if req.Title != nil {
			updatedFields["title"] = *req.Title
		}
		if req.Description != nil {
			updatedFields["description"] = *req.Description
		}
		if req.ColumnID != nil {
			updatedFields["columnId"] = *req.ColumnID
		}
		if req.Position != nil {
			updatedFields["position"] = *req.Position
		}
		h.broadcastEvent(boardID, "task_updated", updatedFields)
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// DeleteTask.
func (h *BoardHandlers) DeleteTask(w http.ResponseWriter, r *http.Request) {
	taskID, err := uuid.Parse(chi.URLParam(r, "taskID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, CodeBadRequest, "invalid task id")
		return
	}
	wsID, err := h.repo.TaskWorkspaceID(r.Context(), taskID)
	if err != nil {
		if errors.Is(err, boards.ErrNotFound) {
			writeError(w, http.StatusNotFound, CodeNotFound, "task not found")
			return
		}
		writeError(w, http.StatusInternalServerError, CodeInternal, "could not load task")
		return
	}
	if !h.requireEditor(w, r, wsID) {
		return
	}
	boardID, boardErr := h.repo.BoardIDByTask(r.Context(), taskID)
	deleteErr := h.repo.DeleteTask(r.Context(), taskID)
	if deleteErr != nil {
		if errors.Is(deleteErr, boards.ErrNotFound) {
			writeError(w, http.StatusNotFound, CodeNotFound, "task not found")
			return
		}
		writeError(w, http.StatusInternalServerError, CodeInternal, "could not delete task")
		return
	}
	if boardErr == nil {
		h.broadcastEvent(boardID, "task_deleted", map[string]any{"id": taskID})
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
