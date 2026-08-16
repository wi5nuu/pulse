package httpapi

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"

	"github.com/pulse/server/internal/boards"
	"github.com/pulse/server/internal/documents"
	"github.com/pulse/server/internal/models"
	"github.com/pulse/server/internal/workspaces"
)

type WorkspaceHandlers struct {
	wsRepo    *workspaces.Repo
	docsRepo  *documents.Repo
	boardRepo *boards.Repo
	validate  *validator.Validate
}

func NewWorkspaceHandlers(wsRepo *workspaces.Repo, docsRepo *documents.Repo, boardRepo *boards.Repo) *WorkspaceHandlers {
	return &WorkspaceHandlers{
		wsRepo:    wsRepo,
		docsRepo:  docsRepo,
		boardRepo: boardRepo,
		validate:  validator.New(validator.WithRequiredStructEnabled()),
	}
}

type createWorkspaceRequest struct {
	Name string `json:"name" validate:"required,min=1,max=100"`
}

type workspaceDTO struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
	Slug string    `json:"slug"`
}

// List workspaces milik user.
func (h *WorkspaceHandlers) List(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, CodeUnauthorized, "no user")
		return
	}
	list, err := h.wsRepo.ListByUser(r.Context(), uid)
	if err != nil {
		writeError(w, http.StatusInternalServerError, CodeInternal, "could not list workspaces")
		return
	}
	out := make([]workspaceDTO, 0, len(list))
	for _, ws := range list {
		out = append(out, workspaceDTO{ID: ws.ID, Name: ws.Name, Slug: ws.Slug})
	}
	writeJSON(w, http.StatusOK, map[string]any{"workspaces": out})
}

// Create workspace baru.
func (h *WorkspaceHandlers) Create(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, CodeUnauthorized, "no user")
		return
	}
	var req createWorkspaceRequest
	if !decodeValid(w, r, h.validate, &req) {
		return
	}
	ws, err := h.wsRepo.Create(r.Context(), req.Name, uid)
	if err != nil {
		writeError(w, http.StatusInternalServerError, CodeInternal, "could not create workspace")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"workspace": workspaceDTO{ID: ws.ID, Name: ws.Name, Slug: ws.Slug},
	})
}

// requireMember memastikan user adalah anggota workspace (role apapun) ATAU
// memiliki document share dalam workspace tersebut. Menulis error response dan
// return false jika keduanya tidak terpenuhi.
func (h *WorkspaceHandlers) requireMember(w http.ResponseWriter, r *http.Request, wsID uuid.UUID) bool {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, CodeUnauthorized, "no user")
		return false
	}
	_, err := h.wsRepo.GetMemberRole(r.Context(), wsID, uid)
	if err == nil {
		return true
	}
	shared, sErr := h.docsRepo.HasSharedDocInWorkspace(r.Context(), wsID, uid)
	if sErr == nil && shared {
		return true
	}
	writeError(w, http.StatusForbidden, CodeForbidden, "no access to workspace")
	return false
}

// Get workspace by ID.
func (h *WorkspaceHandlers) Get(w http.ResponseWriter, r *http.Request) {
	wsID, err := uuid.Parse(chi.URLParam(r, "workspaceID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, CodeBadRequest, "invalid id")
		return
	}
	if !h.requireMember(w, r, wsID) {
		return
	}
	ws, err := h.wsRepo.GetByID(r.Context(), wsID)
	if err != nil {
		if errors.Is(err, workspaces.ErrNotFound) {
			writeError(w, http.StatusNotFound, CodeNotFound, "workspace not found")
			return
		}
		writeError(w, http.StatusInternalServerError, CodeInternal, "could not load workspace")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"workspace": workspaceDTO{ID: ws.ID, Name: ws.Name, Slug: ws.Slug},
	})
}

// ListDocuments in a workspace.
// Untuk member: tampilkan semua dokumen workspace.
// Untuk non-member dengan document share: tampilkan hanya dokumen yang di-share.
func (h *WorkspaceHandlers) ListDocuments(w http.ResponseWriter, r *http.Request) {
	wsID, err := uuid.Parse(chi.URLParam(r, "workspaceID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, CodeBadRequest, "invalid id")
		return
	}
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, CodeUnauthorized, "no user")
		return
	}
	if !h.requireMember(w, r, wsID) {
		return
	}

	_, roleErr := h.wsRepo.GetMemberRole(r.Context(), wsID, uid)
	isMember := roleErr == nil

	var docs []*models.Document
	if isMember {
		docs, err = h.docsRepo.ListByWorkspace(r.Context(), wsID, 50)
	} else {
		docs, err = h.docsRepo.ListSharedInWorkspace(r.Context(), wsID, uid, 50)
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, CodeInternal, "could not list documents")
		return
	}
	out := make([]documentDTO, 0, len(docs))
	for _, d := range docs {
		out = append(out, toDocDTO(d))
	}
	writeJSON(w, http.StatusOK, map[string]any{"documents": out})
}

// ListSharedDocuments lists documents shared directly with the user
// (document-level sharing, without workspace membership).
func (h *WorkspaceHandlers) ListSharedDocuments(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, CodeUnauthorized, "no user")
		return
	}
	docs, err := h.docsRepo.ListSharedWithUser(r.Context(), uid, 50)
	if err != nil {
		writeError(w, http.StatusInternalServerError, CodeInternal, "could not list shared documents")
		return
	}
	out := make([]documentDTO, 0, len(docs))
	for _, d := range docs {
		out = append(out, toDocDTO(d))
	}
	writeJSON(w, http.StatusOK, map[string]any{"documents": out})
}

// CreateDocument in a workspace.
func (h *WorkspaceHandlers) CreateDocument(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, CodeUnauthorized, "no user")
		return
	}
	wsID, err := uuid.Parse(chi.URLParam(r, "workspaceID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, CodeBadRequest, "invalid id")
		return
	}
	// FIX IDOR (kritis): sebelumnya handler tidak memverifikasi keanggotaan
	// workspace — user terautentikasi mana pun bisa membuat dokumen di
	// workspace orang lain. Hanya member (role apapun) yang boleh.
	if !h.requireMember(w, r, wsID) {
		return
	}
	var req createDocRequest
	if !decodeValid(w, r, h.validate, &req) {
		return
	}
	doc, err := h.docsRepo.Create(r.Context(), wsID, req.Title, uid)
	if err != nil {
		writeError(w, http.StatusInternalServerError, CodeInternal, "could not create document")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"document": toDocDTO(doc)})
}
