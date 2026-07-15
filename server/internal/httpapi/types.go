package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5/middleware"
)

// chiMiddlewareRecoverer membungkus middleware.Recoverer chi supaya namanya
// eksplisit di router.go. Tangani panic → 500, jangan crash proses server.
func chiMiddlewareRecoverer() func(http.Handler) http.Handler {
	return middleware.Recoverer
}
