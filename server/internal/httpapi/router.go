package httpapi

import (
	"database/sql"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"
	"github.com/redis/go-redis/v9"

	"github.com/pulse/server/internal/auth"
	"github.com/pulse/server/internal/boards"
	"github.com/pulse/server/internal/comments"
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
	CommentRepo    *comments.Repo
	BoardRepo      *boards.Repo
	SnapRepo       *documents.SnapshotRepo
	WSHandler      http.Handler
	BoardWSHandler http.Handler
	AccessTTL      time.Duration
	RefreshTTL     time.Duration
	Logger         *slog.Logger
}

// NewRouter membangun http.Handler dengan semua route.
func NewRouter(d RouterDeps) http.Handler {
	r := chi.NewRouter()

	// Middleware berurutan:
	//   1. RequestID — ID unik per request (logging & debugging).
	//   2. RequestLogger — structured log semua request (tanpa query string
	//      supaya token WS di URL tidak bocor ke log).
	//   3. SecurityHeaders — baseline security headers (CSP, nosniff, dll).
	//   4. Recoverer — tangkap panic, kembalikan 500 (bukan crash proses).
	//   5. CORS.
	r.Use(RequestID)
	r.Use(RequestLogger(d.Logger))
	r.Use(SecurityHeaders)
	r.Use(chiMiddlewareRecoverer())

	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{d.CORSOrig},
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Authorization", "Content-Type"},
		ExposedHeaders:   []string{"Set-Cookie"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	r.Get("/health", healthHandler(d.DB, d.Redis))

	// --- Auth (rate-limited keras: brute-force protection) ---
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
		// 3 request/detik, burst 10 per IP — cukup untuk user normal, blokir
		// script brute-force. Logout tidak di-rate-limit ketat (sesekali).
		r.With(RateLimit(3, 10)).Post("/register", authH.Register)
		r.With(RateLimit(3, 10)).Post("/login", authH.Login)
		r.With(RateLimit(3, 10)).Post("/refresh", authH.Refresh)
		r.With(RequireAuth(d.Jwt)).Post("/logout", authH.Logout)
	})

	// --- User profile ---
	r.With(RequireAuth(d.Jwt)).Get("/me", meHandler)
	userH := NewUserHandlers(d.UsersRepo)
	// FIX: frontend memakai `/api/users/me` — registrasi di kedua path untuk
	// backward-compat (dulu hanya `/users/me` yang ada → profil update 404).
	r.With(RequireAuth(d.Jwt)).Patch("/users/me", userH.UpdateProfile)
	r.With(RequireAuth(d.Jwt)).Patch("/api/users/me", userH.UpdateProfile)
	// Rate-limit lookup email (mitigasi user enumeration).
	r.With(RequireAuth(d.Jwt), RateLimit(5, 20)).Get("/api/users/by-email/{email}", userH.GetUserByEmail)

	// --- API group (semua butuh auth) ---
	r.Route("/api", func(r chi.Router) {
		r.Use(RequireAuth(d.Jwt))

		// Workspace: List & Create
		wsH := NewWorkspaceHandlers(d.WsRepo, d.DocsRepo, d.BoardRepo)
		r.Get("/workspaces", wsH.List)
		r.Post("/workspaces", wsH.Create)
		r.Get("/documents/shared", wsH.ListSharedDocuments)

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
		r.With(RateLimit(5, 10)).Post("/invites", memH.InviteMember)
		r.Get("/invites", memH.ListWorkspaceInvites)
		r.Delete("/invites/{inviteID}", memH.DeleteInvite)
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
			// Document Sharing
			shareH := NewDocumentShareHandlers(d.DocsRepo, d.WsRepo)
			r.Get("/shares", shareH.ListDocumentShares)
			r.Post("/shares", shareH.ShareDocument)
			r.Delete("/shares/{userID}", shareH.UnshareDocument)
			// Komentar & link share (fiturwajibada I & H.168)
			collabH := NewCollabHandlers(d.CommentRepo, d.DocsRepo)
			r.Get("/comments", collabH.ListComments)
			r.Post("/comments", collabH.CreateComment)
			r.Patch("/comments/{commentID}", collabH.ResolveComment)
			r.Delete("/comments/{commentID}", collabH.DeleteComment)
			r.Get("/linkshare", collabH.ListLinkShares)
			r.Post("/linkshare", collabH.CreateLinkShare)
			r.Delete("/linkshare/{shareID}", collabH.DeleteLinkShare)
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

	// Invite detail (public — cukup token). Rate-limited: endpoint publik
	// tanpa auth, token 64-hex → brute-force lambat, tapi tetap batasi.
	memH := NewMemberHandlers(d.WsRepo)
	r.With(RateLimit(5, 20)).Get("/invites/{token}", memH.GetInvite)
	// Link share resolve (public — "Anyone with the link"). Rate-limited keras
	// (mitigasi token brute-force & DoS pada endpoint tanpa auth).
	collabPub := NewCollabHandlers(d.CommentRepo, d.DocsRepo)
	r.With(RateLimit(5, 20)).Get("/api/linkshare/{token}", collabPub.GetLinkSharePublic)
	// Invite accept (butuh auth).
	r.With(RequireAuth(d.Jwt), RateLimit(10, 20)).Post("/invites/{token}/accept", memH.AcceptInvite)
	// Invite reject (butuh auth).
	r.With(RequireAuth(d.Jwt), RateLimit(10, 20)).Post("/invites/{token}/reject", memH.RejectInvite)
	// List pending invites untuk user yang login.
	r.With(RequireAuth(d.Jwt), RateLimit(10, 30)).Get("/invites/pending", memH.ListPendingInvites)

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
