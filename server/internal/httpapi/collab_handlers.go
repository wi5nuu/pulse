package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"

	"github.com/pulse/server/internal/comments"
	"github.com/pulse/server/internal/documents"
)

// CollabHandlers: komentar (fiturwajibada I) + link share (fiturwajibada H.168).
// Semua mutasi mem-broadcast event via WS supaya real-time ke kolaborator lain.
type CollabHandlers struct {
	comRepo  *comments.Repo
	docRepo  *documents.Repo
	validate *validator.Validate
}

func NewCollabHandlers(comRepo *comments.Repo, docRepo *documents.Repo) *CollabHandlers {
	return &CollabHandlers{
		comRepo:  comRepo,
		docRepo:  docRepo,
		validate: validator.New(validator.WithRequiredStructEnabled()),
	}
}

// --- Komentar ---

type createCommentRequest struct {
	Anchor   string  `json:"anchor" validate:"required,min=2,max=64"` // JSON {"from":n,"to":n}
	Body     string  `json:"body" validate:"required,min=1,max=2000"`
	ParentID *string `json:"parentId"`
}

func (h *CollabHandlers) ListComments(w http.ResponseWriter, r *http.Request) {
	docID, ok := parseDocAndAuthorize(w, r, h.docRepo, false)
	if !ok {
		return
	}
	cs, err := h.comRepo.ListComments(r.Context(), docID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, CodeInternal, "could not list comments")
		return
	}
	out := make([]map[string]any, 0, len(cs))
	for _, c := range cs {
		out = append(out, commentDTO(c))
	}
	writeJSON(w, http.StatusOK, map[string]any{"comments": out})
}

func (h *CollabHandlers) CreateComment(w http.ResponseWriter, r *http.Request) {
	docID, uid, ok := parseDocAndAuthorizeUser(w, r, h.docRepo, true)
	if !ok {
		return
	}
	var req createCommentRequest
	if !decodeValid(w, r, h.validate, &req) {
		return
	}
	var parentID *uuid.UUID
	if req.ParentID != nil {
		pid, err := uuid.Parse(*req.ParentID)
		if err != nil {
			writeError(w, http.StatusBadRequest, CodeBadRequest, "invalid parent id")
			return
		}
		parentID = &pid
	}
	// Validasi anchor JSON sederhana (harus berisi from/to).
	var anchor struct {
		From int `json:"from"`
		To   int `json:"to"`
	}
	if err := json.Unmarshal([]byte(req.Anchor), &anchor); err != nil || anchor.From < 0 || anchor.To < anchor.From {
		writeError(w, http.StatusBadRequest, CodeBadRequest, "invalid anchor")
		return
	}

	c, err := h.comRepo.CreateComment(r.Context(), docID, uid, req.Anchor, req.Body, parentID)
	if err != nil {
		if errors.Is(err, comments.ErrNotFound) {
			writeError(w, http.StatusNotFound, CodeNotFound, "parent comment not found")
			return
		}
		writeError(w, http.StatusInternalServerError, CodeInternal, "could not create comment")
		return
	}
	// Realtime: beri tahu kolaborator lain.
	DocEventBroadcast(docID, commentEvent("comment-created", c))
	writeJSON(w, http.StatusCreated, map[string]any{"comment": commentDTO(c)})
}

type resolveCommentRequest struct {
	Resolved bool `json:"resolved"`
}

func (h *CollabHandlers) ResolveComment(w http.ResponseWriter, r *http.Request) {
	docID, uid, ok := parseDocAndAuthorizeUser(w, r, h.docRepo, true)
	if !ok {
		return
	}
	commentID, err := uuid.Parse(chi.URLParam(r, "commentID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, CodeBadRequest, "invalid comment id")
		return
	}
	var req resolveCommentRequest
	if !decodeValid(w, r, h.validate, &req) {
		return
	}
	c, err := h.comRepo.SetResolved(r.Context(), commentID, uid, req.Resolved)
	if err != nil {
		if errors.Is(err, comments.ErrNotFound) {
			writeError(w, http.StatusNotFound, CodeNotFound, "comment not found")
			return
		}
		writeError(w, http.StatusInternalServerError, CodeInternal, "could not update comment")
		return
	}
	DocEventBroadcast(docID, commentEvent("comment-updated", c))
	writeJSON(w, http.StatusOK, map[string]any{"comment": commentDTO(c)})
}

func (h *CollabHandlers) DeleteComment(w http.ResponseWriter, r *http.Request) {
	docID, _, ok := parseDocAndAuthorizeUser(w, r, h.docRepo, true)
	if !ok {
		return
	}
	commentID, err := uuid.Parse(chi.URLParam(r, "commentID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, CodeBadRequest, "invalid comment id")
		return
	}
	
	// Get comment first to include parentId in broadcast for proper cleanup
	c, err := h.comRepo.GetComment(r.Context(), commentID)
	if err != nil {
		if errors.Is(err, comments.ErrNotFound) {
			writeError(w, http.StatusNotFound, CodeNotFound, "comment not found")
			return
		}
		writeError(w, http.StatusInternalServerError, CodeInternal, "could not get comment")
		return
	}
	
	if err := h.comRepo.DeleteComment(r.Context(), commentID); err != nil {
		if errors.Is(err, comments.ErrNotFound) {
			writeError(w, http.StatusNotFound, CodeNotFound, "comment not found")
			return
		}
		writeError(w, http.StatusInternalServerError, CodeInternal, "could not delete comment")
		return
	}
	// Broadcast with parentId so clients can clean up replies properly
	DocEventBroadcast(docID, commentEvent("comment-deleted", c))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// --- Link share ---

type createLinkShareRequest struct {
	Permission string `json:"permission" validate:"required,oneof=view edit"`
	ExpiresAt  string `json:"expiresAt"` // RFC3339, opsional
}

func (h *CollabHandlers) CreateLinkShare(w http.ResponseWriter, r *http.Request) {
	docID, uid, ok := parseDocAndAuthorizeUser(w, r, h.docRepo, true)
	if !ok {
		return
	}
	var req createLinkShareRequest
	if !decodeValid(w, r, h.validate, &req) {
		return
	}
	var expiresAt *time.Time
	if req.ExpiresAt != "" {
		t, err := time.Parse(time.RFC3339, req.ExpiresAt)
		if err != nil {
			writeError(w, http.StatusBadRequest, CodeBadRequest, "invalid expiresAt")
			return
		}
		expiresAt = &t
	}
	s, err := h.comRepo.CreateLinkShare(r.Context(), docID, req.Permission, uid, expiresAt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, CodeInternal, "could not create link share")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"share": linkShareDTO(s)})
}

func (h *CollabHandlers) ListLinkShares(w http.ResponseWriter, r *http.Request) {
	docID, _, ok := parseDocAndAuthorizeUser(w, r, h.docRepo, false)
	if !ok {
		return
	}
	ss, err := h.comRepo.ListLinkShares(r.Context(), docID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, CodeInternal, "could not list link shares")
		return
	}
	out := make([]map[string]any, 0, len(ss))
	for _, s := range ss {
		out = append(out, linkShareDTO(s))
	}
	writeJSON(w, http.StatusOK, map[string]any{"shares": out})
}

func (h *CollabHandlers) DeleteLinkShare(w http.ResponseWriter, r *http.Request) {
	docID, _, ok := parseDocAndAuthorizeUser(w, r, h.docRepo, true)
	if !ok {
		return
	}
	shareID, err := uuid.Parse(chi.URLParam(r, "shareID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, CodeBadRequest, "invalid share id")
		return
	}
	if err := h.comRepo.DeleteLinkShare(r.Context(), shareID, docID); err != nil {
		if errors.Is(err, comments.ErrNotFound) {
			writeError(w, http.StatusNotFound, CodeNotFound, "link share not found")
			return
		}
		writeError(w, http.StatusInternalServerError, CodeInternal, "could not delete link share")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// GetLinkSharePublic: resolve token link share (TANPA auth — "Anyone with
// the link"). Balas info dokumen + permission. Dipakai halaman pembuka link.
func (h *CollabHandlers) GetLinkSharePublic(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	if token == "" {
		writeError(w, http.StatusBadRequest, CodeBadRequest, "missing token")
		return
	}
	s, err := h.comRepo.GetByToken(r.Context(), token)
	if err != nil {
		if errors.Is(err, comments.ErrNotFound) || errors.Is(err, comments.ErrExpired) {
			writeError(w, http.StatusNotFound, CodeNotFound, "link share not found or expired")
			return
		}
		writeError(w, http.StatusInternalServerError, CodeInternal, "could not resolve link share")
		return
	}
	doc, err := h.docRepo.GetByID(r.Context(), s.DocumentID)
	if err != nil {
		if errors.Is(err, documents.ErrNotFound) {
			writeError(w, http.StatusNotFound, CodeNotFound, "document not found")
			return
		}
		writeError(w, http.StatusInternalServerError, CodeInternal, "could not get document")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"documentId":   s.DocumentID,
		"title":        doc.Title,
		"workspaceId":  doc.WorkspaceID,
		"permission":   s.Permission,
		"expiresAt":    s.ExpiresAt,
	})
}

// --- DTO & helpers ---

func commentDTO(c *comments.Comment) map[string]any {
	return map[string]any{
		"id":          c.ID,
		"documentId":  c.DocumentID,
		"authorId":    c.AuthorID,
		"authorName":  c.AuthorName,
		"authorEmail": c.AuthorEmail,
		"anchor":      c.Anchor,
		"body":        c.Body,
		"parentId":    c.ParentID,
		"resolved":    c.Resolved,
		"createdAt":   c.CreatedAt,
		"updatedAt":   c.UpdatedAt,
	}
}

func linkShareDTO(s *comments.LinkShare) map[string]any {
	return map[string]any{
		"id":         s.ID,
		"documentId": s.DocumentID,
		"token":      s.Token,
		"permission": s.Permission,
		"createdBy":  s.CreatedBy,
		"expiresAt":  s.ExpiresAt,
		"createdAt":  s.CreatedAt,
	}
}

// isReadOnlyPermission: true jika permission dari HasDocumentAccess berarti
// read-only ("viewer" dari workspace, atau "view" dari document share).
func isReadOnlyPermission(permission string) bool {
	return permission == "viewer" || permission == "view"
}

// commentEvent: payload JSON yang di-broadcast via WS ke kolaborator lain.
func commentEvent(evt string, c *comments.Comment) []byte {
	payload := map[string]any{"event": evt, "comment": commentDTO(c)}
	b, err := json.Marshal(payload)
	if err != nil {
		return []byte(`{"event":"comment-created"}`)
	}
	return b
}

// parseDocAndAuthorizeUser: parse docID + user, cek akses dokumen.
// editOnly=true → user harus punya akses edit (owner/editor/view share "edit").
func parseDocAndAuthorizeUser(w http.ResponseWriter, r *http.Request, docRepo *documents.Repo, editOnly bool) (uuid.UUID, uuid.UUID, bool) {
	docID, err := uuid.Parse(chi.URLParam(r, "documentID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, CodeBadRequest, "invalid document id")
		return uuid.Nil, uuid.Nil, false
	}
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, CodeUnauthorized, "no user")
		return uuid.Nil, uuid.Nil, false
	}
	hasAccess, permission, err := docRepo.HasDocumentAccess(r.Context(), docID, uid)
	if err != nil {
		writeError(w, http.StatusInternalServerError, CodeInternal, "could not check access")
		return uuid.Nil, uuid.Nil, false
	}
	if !hasAccess {
		writeError(w, http.StatusForbidden, CodeForbidden, "no access to this document")
		return uuid.Nil, uuid.Nil, false
	}
	if editOnly && isReadOnlyPermission(permission) {
		writeError(w, http.StatusForbidden, CodeForbidden, "viewers cannot modify comments")
		return uuid.Nil, uuid.Nil, false
	}
	return docID, uid, true
}

func parseDocAndAuthorize(w http.ResponseWriter, r *http.Request, docRepo *documents.Repo, editOnly bool) (uuid.UUID, bool) {
	docID, _, ok := parseDocAndAuthorizeUser(w, r, docRepo, editOnly)
	return docID, ok
}