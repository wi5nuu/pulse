package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/pulse/server/internal/auth"
)

// RequireAuth middleware: memvalidasi access token di header Authorization.
// Kalau valid, identitas user ditempel ke context. Kalau tidak, 401.
//
// Format header: "Authorization: Bearer <token>". Kami tolak kalau bukan Bearer.
func RequireAuth(jwt *auth.JWTService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, ok := bearerToken(r)
			if !ok {
				writeError(w, http.StatusUnauthorized, CodeUnauthorized, "missing or malformed Authorization header")
				return
			}
			claims, err := jwt.Verify(token)
			if err != nil {
				// Jangan bocorkan detail kenapa token invalid — cukup "invalid token".
				// Detail di log server saja (untuk debugging).
				writeError(w, http.StatusUnauthorized, CodeUnauthorized, "invalid or expired token")
				return
			}
			r = r.WithContext(withUser(r.Context(), claims.UserID, claims.Email, claims.Name))
			next.ServeHTTP(w, r)
		})
	}
}

// bearerToken meng-ekstrak token dari header Authorization (case-insensitive scheme).
func bearerToken(r *http.Request) (string, bool) {
	h := r.Header.Get("Authorization")
	if h == "" {
		return "", false
	}
	const prefix = "bearer "
	if !strings.HasPrefix(strings.ToLower(h), prefix) {
		return "", false
	}
	token := strings.TrimSpace(h[len(prefix):])
	if token == "" {
		return "", false
	}
	return token, true
}

// isAuthError mengecek apakah error dari refresh service adalah auth-related.
func isAuthError(err error) bool {
	return errors.Is(err, auth.ErrRefreshInvalid) ||
		errors.Is(err, auth.ErrRefreshExpired) ||
		errors.Is(err, auth.ErrRefreshReused)
}
