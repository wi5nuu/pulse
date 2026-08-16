package httpapi

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"

	"github.com/pulse/server/internal/models"
	"github.com/pulse/server/internal/workspaces"
)

type MemberHandlers struct {
	wsRepo   *workspaces.Repo
	validate *validator.Validate
}

func NewMemberHandlers(wsRepo *workspaces.Repo) *MemberHandlers {
	return &MemberHandlers{
		wsRepo:   wsRepo,
		validate: validator.New(validator.WithRequiredStructEnabled()),
	}
}

type memberDTO struct {
	UserID uuid.UUID `json:"userId"`
	Name   string    `json:"name"`
	Email  string    `json:"email"`
	Role   string    `json:"role"`
}

type inviteRequest struct {
	Email string `json:"email" validate:"required,email"`
	Role  string `json:"role" validate:"required,oneof=editor viewer"`
}

type updateRoleRequest struct {
	Role string `json:"role" validate:"required,oneof=editor viewer"`
}

// requireMember memastikan user adalah anggota workspace (role apapun).
// Menulis error response dan return false jika bukan anggota.
func (h *MemberHandlers) requireMember(w http.ResponseWriter, r *http.Request, wsID uuid.UUID) bool {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, CodeUnauthorized, "no user")
		return false
	}
	_, err := h.wsRepo.GetMemberRole(r.Context(), wsID, uid)
	if err != nil {
		writeError(w, http.StatusForbidden, CodeForbidden, "not a workspace member")
		return false
	}
	return true
}

// requireOwner memastikan user adalah owner workspace. Menulis error response
// dan return false jika bukan owner.
func (h *MemberHandlers) requireOwner(w http.ResponseWriter, r *http.Request, wsID uuid.UUID) bool {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, CodeUnauthorized, "no user")
		return false
	}
	role, err := h.wsRepo.GetMemberRole(r.Context(), wsID, uid)
	if err != nil {
		writeError(w, http.StatusForbidden, CodeForbidden, "not a workspace member")
		return false
	}
	if role != models.RoleOwner {
		writeError(w, http.StatusForbidden, CodeForbidden, "only the owner can do that")
		return false
	}
	return true
}

// ListMembers workspace.
func (h *MemberHandlers) ListMembers(w http.ResponseWriter, r *http.Request) {
	wsID, err := uuid.Parse(chi.URLParam(r, "workspaceID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, CodeBadRequest, "invalid workspace id")
		return
	}
	if !h.requireMember(w, r, wsID) {
		return
	}
	memberList, err := h.wsRepo.ListMembers(r.Context(), wsID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, CodeInternal, "could not list members")
		return
	}
	out := make([]memberDTO, 0, len(memberList))
	for _, m := range memberList {
		out = append(out, memberDTO{UserID: m.UserID, Name: m.Name, Email: m.Email, Role: m.Role})
	}
	writeJSON(w, http.StatusOK, map[string]any{"members": out})
}

// InviteMember membuat invite token.
func (h *MemberHandlers) InviteMember(w http.ResponseWriter, r *http.Request) {
	wsID, err := uuid.Parse(chi.URLParam(r, "workspaceID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, CodeBadRequest, "invalid workspace id")
		return
	}
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, CodeUnauthorized, "no user")
		return
	}
	// Authz: owner/editor boleh invite; viewer tidak (FEATURES.md §6).
	role, err := h.wsRepo.GetMemberRole(r.Context(), wsID, uid)
	if err != nil {
		writeError(w, http.StatusForbidden, CodeForbidden, "not a workspace member")
		return
	}
	if role == models.RoleViewer {
		writeError(w, http.StatusForbidden, CodeForbidden, "viewers cannot invite")
		return
	}
	var req inviteRequest
	if !decodeValid(w, r, h.validate, &req) {
		return
	}
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		writeError(w, http.StatusInternalServerError, CodeInternal, "could not generate token")
		return
	}
	token := hex.EncodeToString(tokenBytes)
	expiresAt := time.Now().Add(7 * 24 * time.Hour)

	if err := h.wsRepo.CreateInvite(r.Context(), wsID, req.Email, req.Role, token, expiresAt, uid); err != nil {
		writeError(w, http.StatusInternalServerError, CodeInternal, "could not create invite")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"invite": map[string]any{
			"email": req.Email,
			"role":  req.Role,
			"token": token,
		},
	})
}

// GetInvite mengembalikan detail undangan (tanpa auth — publik via token).
func (h *MemberHandlers) GetInvite(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	if token == "" {
		writeError(w, http.StatusBadRequest, CodeBadRequest, "missing token")
		return
	}
	invite, err := h.wsRepo.GetInviteByToken(r.Context(), token)
	if err != nil {
		if errors.Is(err, workspaces.ErrNotFound) {
			writeError(w, http.StatusNotFound, CodeNotFound, "invite not found or expired")
			return
		}
		writeError(w, http.StatusInternalServerError, CodeInternal, "could not get invite")
		return
	}
	if invite.ExpiresAt.Before(time.Now()) {
		writeError(w, http.StatusNotFound, CodeNotFound, "invite expired")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"invite": map[string]any{
			"workspaceId":   invite.WorkspaceID,
			"workspaceName": invite.WorkspaceName,
			"role":          invite.Role,
			"email":         invite.Email,
			"accepted":      invite.Accepted,
			"invitedByName": invite.InvitedByName,
			"expiresAt":     invite.ExpiresAt,
		},
	})
}

// AcceptInvite menerima undangan.
func (h *MemberHandlers) AcceptInvite(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, CodeUnauthorized, "no user")
		return
	}
	token := chi.URLParam(r, "token")
	if token == "" {
		writeError(w, http.StatusBadRequest, CodeBadRequest, "missing token")
		return
	}
	if err := h.wsRepo.AcceptInvite(r.Context(), token, uid); err != nil {
		if errors.Is(err, workspaces.ErrNotFound) {
			writeError(w, http.StatusNotFound, CodeNotFound, "invite not found or expired")
			return
		}
		if errors.Is(err, workspaces.ErrInviteAccepted) {
			writeError(w, http.StatusConflict, CodeConflict, "invite already accepted")
			return
		}
		writeError(w, http.StatusInternalServerError, CodeInternal, "could not accept invite")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ListPendingInvites mengembalikan semua pending invites untuk user yang sedang login.
func (h *MemberHandlers) ListPendingInvites(w http.ResponseWriter, r *http.Request) {
	email, ok := emailFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, CodeUnauthorized, "no email in context")
		return
	}
	invites, err := h.wsRepo.ListPendingInvites(r.Context(), email)
	if err != nil {
		writeError(w, http.StatusInternalServerError, CodeInternal, "could not list invites")
		return
	}
	
	result := make([]map[string]any, 0, len(invites))
	for _, inv := range invites {
		result = append(result, map[string]any{
			"id":             inv.ID,
			"workspaceId":    inv.WorkspaceID,
			"workspaceName":  inv.WorkspaceName,
			"role":           inv.Role,
			"token":          inv.Token,
			"invitedByName":  inv.InvitedByName,
			"expiresAt":      inv.ExpiresAt,
			"createdAt":      inv.CreatedAt,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"invites": result})
}

// UpdateMemberRole.
func (h *MemberHandlers) UpdateMemberRole(w http.ResponseWriter, r *http.Request) {
	wsID, err := uuid.Parse(chi.URLParam(r, "workspaceID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, CodeBadRequest, "invalid workspace id")
		return
	}
	userID, err := uuid.Parse(chi.URLParam(r, "userID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, CodeBadRequest, "invalid user id")
		return
	}
	// Hanya owner yang bisa mengubah role member (FEATURES.md §6).
	if !h.requireOwner(w, r, wsID) {
		return
	}
	var req updateRoleRequest
	if !decodeValid(w, r, h.validate, &req) {
		return
	}
	if err := h.wsRepo.UpdateMemberRole(r.Context(), wsID, userID, req.Role); err != nil {
		if errors.Is(err, workspaces.ErrNotFound) {
			writeError(w, http.StatusNotFound, CodeNotFound, "member not found or is owner")
			return
		}
		writeError(w, http.StatusInternalServerError, CodeInternal, "could not update role")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// RemoveMember menghapus anggota.
func (h *MemberHandlers) RemoveMember(w http.ResponseWriter, r *http.Request) {
	wsID, err := uuid.Parse(chi.URLParam(r, "workspaceID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, CodeBadRequest, "invalid workspace id")
		return
	}
	userID, err := uuid.Parse(chi.URLParam(r, "userID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, CodeBadRequest, "invalid user id")
		return
	}
	// Hanya owner yang bisa menghapus member (FEATURES.md §6).
	if !h.requireOwner(w, r, wsID) {
		return
	}
	if err := h.wsRepo.RemoveMember(r.Context(), wsID, userID); err != nil {
		if errors.Is(err, workspaces.ErrNotFound) {
			writeError(w, http.StatusNotFound, CodeNotFound, "member not found or is owner")
			return
		}
		writeError(w, http.StatusInternalServerError, CodeInternal, "could not remove member")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// RejectInvite menolak undangan.
func (h *MemberHandlers) RejectInvite(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, CodeUnauthorized, "no user")
		return
	}
	token := chi.URLParam(r, "token")
	if token == "" {
		writeError(w, http.StatusBadRequest, CodeBadRequest, "missing token")
		return
	}
	if err := h.wsRepo.RejectInvite(r.Context(), token, uid); err != nil {
		if errors.Is(err, workspaces.ErrNotFound) {
			writeError(w, http.StatusNotFound, CodeNotFound, "invite not found or expired")
			return
		}
		if errors.Is(err, workspaces.ErrInviteAccepted) {
			writeError(w, http.StatusConflict, CodeConflict, "invite already accepted")
			return
		}
		writeError(w, http.StatusInternalServerError, CodeInternal, "could not reject invite")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ListWorkspaceInvites mengembalikan semua invite untuk workspace (owner only).
func (h *MemberHandlers) ListWorkspaceInvites(w http.ResponseWriter, r *http.Request) {
	wsID, err := uuid.Parse(chi.URLParam(r, "workspaceID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, CodeBadRequest, "invalid workspace id")
		return
	}
	// Hanya owner yang bisa melihat semua invite workspace.
	if !h.requireOwner(w, r, wsID) {
		return
	}
	invites, err := h.wsRepo.ListWorkspaceInvites(r.Context(), wsID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, CodeInternal, "could not list invites")
		return
	}

	result := make([]map[string]any, 0, len(invites))
	for _, inv := range invites {
		result = append(result, map[string]any{
			"id":            inv.ID,
			"workspaceId":   inv.WorkspaceID,
			"workspaceName": inv.WorkspaceName,
			"email":         inv.Email,
			"role":          inv.Role,
			"token":         inv.Token,
			"invitedByName": inv.InvitedByName,
			"accepted":      inv.Accepted,
			"expiresAt":     inv.ExpiresAt,
			"createdAt":     inv.CreatedAt,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"invites": result})
}

// DeleteInvite menghapus/membatalkan invite (owner only).
func (h *MemberHandlers) DeleteInvite(w http.ResponseWriter, r *http.Request) {
	wsID, err := uuid.Parse(chi.URLParam(r, "workspaceID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, CodeBadRequest, "invalid workspace id")
		return
	}
	inviteID, err := uuid.Parse(chi.URLParam(r, "inviteID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, CodeBadRequest, "invalid invite id")
		return
	}
	// Hanya owner yang bisa menghapus invite.
	if !h.requireOwner(w, r, wsID) {
		return
	}
	if err := h.wsRepo.DeleteInvite(r.Context(), wsID, inviteID); err != nil {
		if errors.Is(err, workspaces.ErrNotFound) {
			writeError(w, http.StatusNotFound, CodeNotFound, "invite not found")
			return
		}
		writeError(w, http.StatusInternalServerError, CodeInternal, "could not delete invite")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
