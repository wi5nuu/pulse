package httpapi

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"

	"github.com/pulse/server/internal/documents"
	"github.com/pulse/server/internal/models"
	"github.com/pulse/server/internal/workspaces"
)

type DocumentShareHandlers struct {
	docRepo  *documents.Repo
	wsRepo   *workspaces.Repo
	validate *validator.Validate
}

func NewDocumentShareHandlers(docRepo *documents.Repo, wsRepo *workspaces.Repo) *DocumentShareHandlers {
	return &DocumentShareHandlers{
		docRepo:  docRepo,
		wsRepo:   wsRepo,
		validate: validator.New(validator.WithRequiredStructEnabled()),
	}
}

type shareDocumentRequest struct {
	UserID     string `json:"userId" validate:"required,uuid"`
	Permission string `json:"permission" validate:"required,oneof=view edit"`
}

// ShareDocument shares a document with a user
func (h *DocumentShareHandlers) ShareDocument(w http.ResponseWriter, r *http.Request) {
	docID, err := uuid.Parse(chi.URLParam(r, "documentID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, CodeBadRequest, "invalid document id")
		return
	}

	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, CodeUnauthorized, "no user")
		return
	}

	// Get document
	doc, err := h.docRepo.GetByID(r.Context(), docID)
	if err != nil {
		if errors.Is(err, documents.ErrNotFound) {
			writeError(w, http.StatusNotFound, CodeNotFound, "document not found")
			return
		}
		writeError(w, http.StatusInternalServerError, CodeInternal, "could not get document")
		return
	}

	// Check if user is owner or has edit permission in workspace
	role, err := h.wsRepo.GetMemberRole(r.Context(), doc.WorkspaceID, uid)
	if err != nil || (role != models.RoleOwner && role != models.RoleEditor) {
		writeError(w, http.StatusForbidden, CodeForbidden, "only workspace owners/editors can share documents")
		return
	}

	var req shareDocumentRequest
	if !decodeValid(w, r, h.validate, &req) {
		return
	}

	sharedWithID, err := uuid.Parse(req.UserID)
	if err != nil {
		writeError(w, http.StatusBadRequest, CodeBadRequest, "invalid user id")
		return
	}

	// Don't allow sharing with self
	if sharedWithID == uid {
		writeError(w, http.StatusBadRequest, CodeBadRequest, "cannot share document with yourself")
		return
	}

	// Share document

	// Share document
	if err := h.docRepo.ShareDocument(r.Context(), docID, sharedWithID, uid, req.Permission); err != nil {
		writeError(w, http.StatusInternalServerError, CodeInternal, "could not share document")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"ok": true})
}

// UnshareDocument removes document share access
func (h *DocumentShareHandlers) UnshareDocument(w http.ResponseWriter, r *http.Request) {
	docID, err := uuid.Parse(chi.URLParam(r, "documentID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, CodeBadRequest, "invalid document id")
		return
	}

	sharedWithID, err := uuid.Parse(chi.URLParam(r, "userID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, CodeBadRequest, "invalid user id")
		return
	}

	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, CodeUnauthorized, "no user")
		return
	}

	// Get document
	doc, err := h.docRepo.GetByID(r.Context(), docID)
	if err != nil {
		if errors.Is(err, documents.ErrNotFound) {
			writeError(w, http.StatusNotFound, CodeNotFound, "document not found")
			return
		}
		writeError(w, http.StatusInternalServerError, CodeInternal, "could not get document")
		return
	}

	// Check if user is owner or has edit permission
	role, err := h.wsRepo.GetMemberRole(r.Context(), doc.WorkspaceID, uid)
	if err != nil || (role != models.RoleOwner && role != models.RoleEditor) {
		writeError(w, http.StatusForbidden, CodeForbidden, "only workspace owners/editors can unshare documents")
		return
	}

	if err := h.docRepo.UnshareDocument(r.Context(), docID, sharedWithID); err != nil {
		if errors.Is(err, documents.ErrNotFound) {
			writeError(w, http.StatusNotFound, CodeNotFound, "share not found")
			return
		}
		writeError(w, http.StatusInternalServerError, CodeInternal, "could not unshare document")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ListDocumentShares lists all users who have access to a document
func (h *DocumentShareHandlers) ListDocumentShares(w http.ResponseWriter, r *http.Request) {
	docID, err := uuid.Parse(chi.URLParam(r, "documentID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, CodeBadRequest, "invalid document id")
		return
	}

	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, CodeUnauthorized, "no user")
		return
	}

	// Get document
	doc, err := h.docRepo.GetByID(r.Context(), docID)
	if err != nil {
		if errors.Is(err, documents.ErrNotFound) {
			writeError(w, http.StatusNotFound, CodeNotFound, "document not found")
			return
		}
		writeError(w, http.StatusInternalServerError, CodeInternal, "could not get document")
		return
	}

	// Check if user has access to this document
	hasAccess, _, err := h.docRepo.HasDocumentAccess(r.Context(), docID, uid)
	if err != nil {
		writeError(w, http.StatusInternalServerError, CodeInternal, "could not check access")
		return
	}
	if !hasAccess {
		writeError(w, http.StatusForbidden, CodeForbidden, "no access to this document")
		return
	}

	shares, err := h.docRepo.ListDocumentShares(r.Context(), docID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, CodeInternal, "could not list shares")
		return
	}

	result := make([]map[string]any, 0, len(shares))
	for _, s := range shares {
		result = append(result, map[string]any{
			"id":              s.ID,
			"documentId":      s.DocumentID,
			"sharedWithId":    s.SharedWithID,
			"sharedWithName":  s.SharedWithName,
			"sharedWithEmail": s.SharedWithEmail,
			"permission":      s.Permission,
			"createdAt":       s.CreatedAt,
		})
	}

	// Also include workspace members
	role, _ := h.wsRepo.GetMemberRole(r.Context(), doc.WorkspaceID, uid)
	writeJSON(w, http.StatusOK, map[string]any{
		"shares":     result,
		"canManage":  role == "owner" || role == "editor",
		"documentId": docID,
	})
}
