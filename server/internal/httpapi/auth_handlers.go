package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/pulse/server/internal/auth"
	"github.com/pulse/server/internal/models"
	"github.com/pulse/server/internal/users"
	"github.com/pulse/server/internal/workspaces"
)

const cookieRefreshName = "pulse_refresh"

// AuthHandlers menampung dependency untuk endpoint auth.
type AuthHandlers struct {
	usersRepo    *users.Repo
	wsRepo       *workspaces.Repo
	jwt          *auth.JWTService
	refresh      *auth.RefreshService
	validate     *validator.Validate
	isDev        bool
	accessTTL    time.Duration
	refreshTTL   time.Duration
}

type AuthDeps struct {
	UsersRepo  *users.Repo
	WsRepo     *workspaces.Repo
	Jwt        *auth.JWTService
	Refresh    *auth.RefreshService
	IsDev      bool
	AccessTTL  time.Duration
	RefreshTTL time.Duration
}

func NewAuthHandlers(d AuthDeps) *AuthHandlers {
	return &AuthHandlers{
		usersRepo:  d.UsersRepo,
		wsRepo:     d.WsRepo,
		jwt:        d.Jwt,
		refresh:    d.Refresh,
		validate:   validator.New(validator.WithRequiredStructEnabled()),
		isDev:      d.IsDev,
		accessTTL:  d.AccessTTL,
		refreshTTL: d.RefreshTTL,
	}
}

// --- DTO ---

type registerRequest struct {
	Email    string `json:"email" validate:"required,email,max=254"`
	Name     string `json:"name" validate:"required,min=1,max=100"`
	Password string `json:"password" validate:"required,min=8,max=72"`
}

type loginRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required"`
}

type authResponse struct {
	AccessToken string  `json:"accessToken"`
	ExpiresIn   int     `json:"expiresIn"` // detik
	User        userDTO `json:"user"`
}

type userDTO struct {
	ID    uuid.UUID `json:"id"`
	Email string    `json:"email"`
	Name  string    `json:"name"`
}

// --- Handlers ---

// Register: buat user + auto-create personal workspace + issue token pair.
func (h *AuthHandlers) Register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if !decodeValid(w, r, h.validate, &req) {
		return
	}

	// Pre-flight email check → 409 yang jelas (vs unique constraint violation).
	taken, err := h.usersRepo.EmailTaken(r.Context(), req.Email)
	if err != nil {
		writeError(w, http.StatusInternalServerError, CodeInternal, "could not check email")
		return
	}
	if taken {
		writeError(w, http.StatusConflict, CodeConflict, "email already registered")
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, CodeInternal, "could not hash password")
		return
	}

	user, err := h.usersRepo.Create(r.Context(), req.Email, req.Name, hash)
	if err != nil {
		// Handle TOCTOU race: EmailTaken returned false but another request
		// created the user between our check and Create. Handle unique
		// constraint violation gracefully.
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			writeError(w, http.StatusConflict, CodeConflict, "email already registered")
			return
		}
		writeError(w, http.StatusInternalServerError, CodeInternal, "could not create user")
		return
	}

	// Auto-create workspace personal supaya onboarding langsung bisa mulai bikin dokumen.
	if _, werr := h.wsRepo.CreatePersonalWorkspace(r.Context(), user.ID, req.Name+"'s Workspace"); werr != nil {
		// Tidak fatal: user tetap terdaftar. Di produksi: log warning.
		_ = werr
	}

	h.issueTokens(w, r, user, http.StatusCreated)
}

// Login: verifikasi kredensial, issue token pair.
func (h *AuthHandlers) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if !decodeValid(w, r, h.validate, &req) {
		return
	}

	user, err := h.usersRepo.GetByEmail(r.Context(), req.Email)
	if err != nil {
		if errors.Is(err, users.ErrNotFound) {
			// Pesan generik — jangan bocorkan apakah email terdaftar.
			writeError(w, http.StatusUnauthorized, CodeUnauthorized, "invalid email or password")
			return
		}
		writeError(w, http.StatusInternalServerError, CodeInternal, "could not load user")
		return
	}

	if err := auth.VerifyPassword(user.PasswordHash, req.Password); err != nil {
		writeError(w, http.StatusUnauthorized, CodeUnauthorized, "invalid email or password")
		return
	}

	h.issueTokens(w, r, user, http.StatusOK)
}

// Refresh: rotate refresh token (cookie), issue access token baru.
func (h *AuthHandlers) Refresh(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(cookieRefreshName)
	if err != nil || cookie.Value == "" {
		writeError(w, http.StatusUnauthorized, CodeUnauthorized, "missing refresh token")
		return
	}

	newPlaintext, rec, userID, err := h.refresh.Rotate(r.Context(), cookie.Value, r.UserAgent(), clientIP(r))
	if err != nil {
		if isAuthError(err) {
			// Reuse atau invalid: clear cookie supaya client tidak retry loop.
			h.clearRefreshCookie(w)
			writeError(w, http.StatusUnauthorized, CodeUnauthorized, "refresh token invalid or expired")
			return
		}
		writeError(w, http.StatusInternalServerError, CodeInternal, "could not refresh")
		return
	}

	user, err := h.usersRepo.GetByID(r.Context(), userID)
	if err != nil {
		// User terhapus di tengah → revoke semua tokennya.
		_ = h.refresh.RevokeAll(r.Context(), userID)
		writeError(w, http.StatusUnauthorized, CodeUnauthorized, "user no longer exists")
		return
	}

	accessToken, _, err := h.jwt.Issue(user.ID, user.Email, user.Name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, CodeInternal, "could not issue access token")
		return
	}

	h.setRefreshCookie(w, newPlaintext, rec.ExpiresAt)
	writeJSON(w, http.StatusOK, authResponse{
		AccessToken: accessToken,
		ExpiresIn:   int(h.accessTTL.Seconds()),
		User:        userDTO{ID: user.ID, Email: user.Email, Name: user.Name},
	})
}

// Logout: revoke family token (single device) + clear cookie.
func (h *AuthHandlers) Logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(cookieRefreshName); err == nil && cookie.Value != "" {
		_ = h.refresh.RevokeFamily(r.Context(), cookie.Value)
	}
	h.clearRefreshCookie(w)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// --- helpers ---

// issueTokens: helper bersama untuk Register & Login.
func (h *AuthHandlers) issueTokens(w http.ResponseWriter, r *http.Request, user *models.User, status int) {
	accessToken, _, err := h.jwt.Issue(user.ID, user.Email, user.Name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, CodeInternal, "could not issue access token")
		return
	}
	plaintext, rec, err := h.refresh.Issue(r.Context(), user.ID, r.UserAgent(), clientIP(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, CodeInternal, "could not issue refresh token")
		return
	}
	h.setRefreshCookie(w, plaintext, rec.ExpiresAt)
	writeJSON(w, status, authResponse{
		AccessToken: accessToken,
		ExpiresIn:   int(h.accessTTL.Seconds()),
		User:        userDTO{ID: user.ID, Email: user.Email, Name: user.Name},
	})
}

// decodeValid: decode JSON body + validasi. Return false jika sudah menulis error.
func decodeValid(w http.ResponseWriter, r *http.Request, v *validator.Validate, dst any) bool {
	// Hardening: batasi ukuran body (fix audit: tanpa MaxBytesReader, client
	// bisa kirim body raksasa → DoS memori). 1 MB cukup untuk semua payload
	// app ini (title, email, invite, dll).
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		writeError(w, http.StatusBadRequest, CodeBadRequest, "malformed JSON body")
		return false
	}
	if err := v.Struct(dst); err != nil {
		var ve validator.ValidationErrors
		if errors.As(err, &ve) {
			fields := map[string]string{}
			for _, fe := range ve {
				fields[fe.Field()] = msgForTag(fe.Tag())
			}
			writeValidationError(w, fields)
			return false
		}
		writeError(w, http.StatusBadRequest, CodeBadRequest, "request validation failed")
		return false
	}
	return true
}

func msgForTag(tag string) string {
	switch tag {
	case "required":
		return "is required"
	case "email":
		return "must be a valid email"
	case "min":
		return "is too short"
	case "max":
		return "is too long"
	default:
		return "is invalid"
	}
}

// setRefreshCookie: httpOnly cookie. Secure=false di dev (localhost HTTP).
func (h *AuthHandlers) setRefreshCookie(w http.ResponseWriter, value string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieRefreshName,
		Value:    value,
		Path:     "/",
		Expires:  expires,
		MaxAge:   int(h.refreshTTL.Seconds()),
		HttpOnly: true,
		Secure:   !h.isDev,
		SameSite: http.SameSiteLaxMode,
	})
}

func (h *AuthHandlers) clearRefreshCookie(w http.ResponseWriter) {
	// Path HARUS sama dengan setRefreshCookie ("/") — kalau beda, browser
	// menganggap ini cookie berbeda dan cookie lama tidak pernah terhapus.
	http.SetCookie(w, &http.Cookie{
		Name:     cookieRefreshName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   !h.isDev,
		SameSite: http.SameSiteLaxMode,
	})
}

// clientIP: ambil IP client. XFF hanya dipakai jika ada (asumsi: di belakang
// proxy terpercaya — di-hardening di Fase 7).
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.Index(xff, ","); i > 0 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	if i := strings.LastIndex(r.RemoteAddr, ":"); i > 0 {
		return r.RemoteAddr[:i]
	}
	return r.RemoteAddr
}
