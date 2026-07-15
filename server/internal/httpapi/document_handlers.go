package httpapi

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"

	"github.com/pulse/server/internal/documents"
	"github.com/pulse/server/internal/models"
)

// DocHandlers: endpoint REST untuk dokumen (di luar WS).
type DocHandlers struct {
	repo     *documents.Repo
	validate *validator.Validate
}

func NewDocHandlers(repo *documents.Repo) *DocHandlers {
	return &DocHandlers{
		repo:     repo,
		validate: validator.New(validator.WithRequiredStructEnabled()),
	}
}

type createDocRequest struct {
	Title string `json:"title" validate:"required,min=1,max=200"`
}

type documentDTO struct {
	ID          uuid.UUID `json:"id"`
	WorkspaceID uuid.UUID `json:"workspaceId"`
	Title       string    `json:"title"`
	CreatedBy   uuid.UUID `json:"createdBy"`
}

// List: dokumen di workspace user (workspace pertama user — simple UX Fase 2).
//
// GET /documents
func (h *DocHandlers) List(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, CodeUnauthorized, "no user")
		return
	}
	wsID, err := h.repo.WorkspaceOfUser(r.Context(), uid)
	if err != nil {
		if errors.Is(err, documents.ErrNotFound) {
			writeJSON(w, http.StatusOK, map[string]any{"documents": []any{}})
			return
		}
		writeError(w, http.StatusInternalServerError, CodeInternal, "could not load workspace")
		return
	}
	docs, err := h.repo.ListByWorkspace(r.Context(), wsID, 50)
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

// Create: bikin dokumen baru di workspace user.
//
// POST /documents
func (h *DocHandlers) Create(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, CodeUnauthorized, "no user")
		return
	}
	var req createDocRequest
	if !decodeValid(w, r, h.validate, &req) {
		return
	}
	wsID, err := h.repo.WorkspaceOfUser(r.Context(), uid)
	if err != nil {
		writeError(w, http.StatusInternalServerError, CodeInternal, "no workspace for user")
		return
	}
	doc, err := h.repo.Create(r.Context(), wsID, req.Title, uid)
	if err != nil {
		writeError(w, http.StatusInternalServerError, CodeInternal, "could not create document")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"document": toDocDTO(doc)})
}

// Rename: update title dokumen.
//
// PATCH /documents/{id}
func (h *DocHandlers) Rename(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, CodeUnauthorized, "no user")
		return
	}
	docID, err := uuid.Parse(chi.URLParam(r, "documentID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, CodeBadRequest, "invalid document id")
		return
	}
	// Authorization: cek role. Viewer tidak boleh rename.
	role, err := h.repo.MemberRole(r.Context(), docID, uid)
	if err != nil {
		writeError(w, http.StatusForbidden, CodeForbidden, "forbidden")
		return
	}
	if role == "viewer" {
		writeError(w, http.StatusForbidden, CodeForbidden, "viewers cannot edit")
		return
	}
	var req createDocRequest // reuse: {title}
	if !decodeValid(w, r, h.validate, &req) {
		return
	}
	if err := h.repo.UpdateTitle(r.Context(), docID, req.Title); err != nil {
		if errors.Is(err, documents.ErrNotFound) {
			writeError(w, http.StatusNotFound, CodeNotFound, "document not found")
			return
		}
		writeError(w, http.StatusInternalServerError, CodeInternal, "could not rename")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// Delete menghapus dokumen.
func (h *DocHandlers) Delete(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, CodeUnauthorized, "no user")
		return
	}
	docID, err := uuid.Parse(chi.URLParam(r, "documentID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, CodeBadRequest, "invalid document id")
		return
	}
	role, err := h.repo.MemberRole(r.Context(), docID, uid)
	if err != nil {
		writeError(w, http.StatusForbidden, CodeForbidden, "forbidden")
		return
	}
	if role == "viewer" {
		writeError(w, http.StatusForbidden, CodeForbidden, "viewers cannot delete")
		return
	}
	if err := h.repo.Delete(r.Context(), docID); err != nil {
		if errors.Is(err, documents.ErrNotFound) {
			writeError(w, http.StatusNotFound, CodeNotFound, "document not found")
			return
		}
		writeError(w, http.StatusInternalServerError, CodeInternal, "could not delete document")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func toDocDTO(d *models.Document) documentDTO {
	return documentDTO{
		ID:          d.ID,
		WorkspaceID: d.WorkspaceID,
		Title:       d.Title,
		CreatedBy:   d.CreatedBy,
	}
}
