package httpapi

import (
	"database/sql"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"
	"github.com/redis/go-redis/v9"

	"github.com/pulse/server/internal/auth"
	"github.com/pulse/server/internal/boards"
	"github.com/pulse/server/internal/documents"
	"github.com/pulse/server/internal/health"
	"github.com/pulse/server/internal/users"
	"github.com/pulse/server/internal/workspaces"
)

// RouterDeps menggabungkan dependency yang dibutuhkan router.
type RouterDeps struct {
	IsDev          bool
	CORSOrig       string
	DB             *sql.DB
	Redis          *redis.Client
	Jwt            *auth.JWTService
	Refresh        *auth.RefreshService
	UsersRepo      *users.Repo
	WsRepo         *workspaces.Repo
	DocsRepo       *documents.Repo
	BoardRepo      *boards.Repo
	SnapRepo       *documents.SnapshotRepo
	WSHandler      http.Handler
	BoardWSHandler http.Handler
	AccessTTL      time.Duration
	RefreshTTL     time.Duration
}

// NewRouter membangun http.Handler dengan semua route.
func NewRouter(d RouterDeps) http.Handler {
	r := chi.NewRouter()

	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{d.CORSOrig},
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Authorization", "Content-Type"},
		ExposedHeaders:   []string{"Set-Cookie"},
		AllowCredentials: true,
		MaxAge:           300,
	}))
	r.Use(chiMiddlewareRecoverer())

	r.Get("/health", healthHandler(d.DB, d.Redis))

	// --- Auth ---
	authH := NewAuthHandlers(AuthDeps{
		UsersRepo:  d.UsersRepo,
		WsRepo:     d.WsRepo,
		Jwt:        d.Jwt,
		Refresh:    d.Refresh,
		IsDev:      d.IsDev,
		AccessTTL:  d.AccessTTL,
		RefreshTTL: d.RefreshTTL,
	})
	r.Route("/auth", func(r chi.Router) {
		r.Post("/register", authH.Register)
		r.Post("/login", authH.Login)
		r.Post("/refresh", authH.Refresh)
		r.With(RequireAuth(d.Jwt)).Post("/logout", authH.Logout)
	})

	// --- User profile ---
	r.With(RequireAuth(d.Jwt)).Get("/me", meHandler)
	userH := NewUserHandlers(d.UsersRepo)
	r.With(RequireAuth(d.Jwt)).Patch("/users/me", userH.UpdateProfile)

	// --- API group (semua butuh auth) ---
	r.Route("/api", func(r chi.Router) {
		r.Use(RequireAuth(d.Jwt))

		// Workspace: List & Create
		wsH := NewWorkspaceHandlers(d.WsRepo, d.DocsRepo, d.BoardRepo)
		r.Get("/workspaces", wsH.List)
		r.Post("/workspaces", wsH.Create)

		// Workspace by ID
		r.Route("/workspaces/{workspaceID}", func(r chi.Router) {
			r.Get("/", wsH.Get)
			r.Get("/documents", wsH.ListDocuments)
			r.Post("/documents", wsH.CreateDocument)
			r.Get("/boards", NewBoardHandlers(d.BoardRepo, d.WsRepo).ListBoards)
			r.Post("/boards", NewBoardHandlers(d.BoardRepo, d.WsRepo).CreateBoard)

			// Members
			memH := NewMemberHandlers(d.WsRepo)
			r.Get("/members", memH.ListMembers)
			r.Post("/invites", memH.InviteMember)
			r.Patch("/members/{userID}", memH.UpdateMemberRole)
			r.Delete("/members/{userID}", memH.RemoveMember)
		})

		// Documents (by ID)
		r.Route("/documents/{documentID}", func(r chi.Router) {
			docH := NewDocHandlers(d.DocsRepo)
			r.Patch("/", docH.Rename)
			r.Delete("/", docH.Delete)
			// Snapshots
			snapH := NewSnapshotHandlers(d.SnapRepo, d.DocsRepo)
			r.Get("/snapshots", snapH.ListSnapshots)
			r.Post("/snapshots/{snapshotID}/restore", snapH.RestoreSnapshot)
		})

		// Boards
		r.Route("/boards/{boardID}", func(r chi.Router) {
			bH := NewBoardHandlers(d.BoardRepo, d.WsRepo)
			r.Get("/", bH.GetBoard)
			r.Post("/columns", bH.CreateColumn)
		})

		// Columns
		r.Route("/columns/{columnID}", func(r chi.Router) {
			bH := NewBoardHandlers(d.BoardRepo, d.WsRepo)
			r.Patch("/", bH.UpdateColumn)
			r.Delete("/", bH.DeleteColumn)
			r.Post("/tasks", bH.CreateTask)
		})

		// Tasks
		r.Route("/tasks/{taskID}", func(r chi.Router) {
			bH := NewBoardHandlers(d.BoardRepo, d.WsRepo)
			r.Patch("/", bH.UpdateTask)
			r.Delete("/", bH.DeleteTask)
		})
	})

	// Invite detail (public — cukup token).
	memH := NewMemberHandlers(d.WsRepo)
	r.Get("/invites/{token}", memH.GetInvite)
	// Invite accept (butuh auth).
	r.With(RequireAuth(d.Jwt)).Post("/invites/{token}/accept", memH.AcceptInvite)

	// WebSocket endpoints.
	if d.WSHandler != nil {
		r.Get("/ws/doc/{documentID}", d.WSHandler.ServeHTTP)
	}
	if d.BoardWSHandler != nil {
		r.Get("/ws/board/{boardID}", d.BoardWSHandler.ServeHTTP)
	}

	return r
}

func meHandler(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, CodeUnauthorized, "no user in context")
		return
	}
	writeJSON(w, http.StatusOK, userDTO{
		ID:    uid,
		Email: userEmailFrom(r.Context()),
		Name:  userNameFrom(r.Context()),
	})
}

func healthHandler(db *sql.DB, rdb *redis.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		report := health.CheckAll(r.Context(), db, rdb)
		status := http.StatusOK
		if report.Status != health.StatusOK {
			status = http.StatusServiceUnavailable
		}
		writeJSON(w, status, report)
	}
}
