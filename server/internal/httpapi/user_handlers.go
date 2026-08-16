package httpapi

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"

	"github.com/pulse/server/internal/users"
)

type UserHandlers struct {
	repo     *users.Repo
	validate *validator.Validate
}

func NewUserHandlers(repo *users.Repo) *UserHandlers {
	return &UserHandlers{
		repo:     repo,
		validate: validator.New(validator.WithRequiredStructEnabled()),
	}
}

type updateProfileRequest struct {
	Name string `json:"name" validate:"required,min=1,max=100"`
}

// UpdateProfile mengubah display name user.
func (h *UserHandlers) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, CodeUnauthorized, "no user")
		return
	}
	var req updateProfileRequest
	if !decodeValid(w, r, h.validate, &req) {
		return
	}
	if err := h.repo.UpdateName(r.Context(), uid, req.Name); err != nil {
		if errors.Is(err, users.ErrNotFound) {
			writeError(w, http.StatusNotFound, CodeNotFound, "user not found")
			return
		}
		writeError(w, http.StatusInternalServerError, CodeInternal, "could not update profile")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// GetUserByEmail returns user info by email (for sharing documents)
func (h *UserHandlers) GetUserByEmail(w http.ResponseWriter, r *http.Request) {
	email := chi.URLParam(r, "email")
	if email == "" {
		writeError(w, http.StatusBadRequest, CodeBadRequest, "email is required")
		return
	}

	// Must be authenticated
	_, ok := userIDFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, CodeUnauthorized, "no user")
		return
	}

	user, err := h.repo.GetByEmail(r.Context(), email)
	if err != nil {
		if errors.Is(err, users.ErrNotFound) {
			writeError(w, http.StatusNotFound, CodeNotFound, "user not found")
			return
		}
		writeError(w, http.StatusInternalServerError, CodeInternal, "could not get user")
		return
	}

	// Return only safe user info (no password hash)
	writeJSON(w, http.StatusOK, map[string]any{
		"id":    user.ID,
		"name":  user.Name,
		"email": user.Email,
	})
}
