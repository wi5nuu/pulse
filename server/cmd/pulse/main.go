// Command pulse adalah entrypoint server Pulse.
//
// Bootstrap order:
//   1. Load config (env)
//   2. Buka DB pool + jalankan migrasi goose (embed)
//   3. Buka Redis client
//   4. Wire auth services (JWT + refresh)
//   5. Wire repositories (users, workspaces)
//   6. Bangun router
//   7. Start HTTP server dengan graceful shutdown
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/pulse/server/internal/auth"
	"github.com/pulse/server/internal/boards"
	"github.com/pulse/server/internal/documents"
	pulsedb "github.com/pulse/server/internal/db"
	"github.com/pulse/server/internal/httpapi"
	"github.com/pulse/server/internal/migrations"
	"github.com/pulse/server/internal/persistence"
	"github.com/pulse/server/internal/users"
	"github.com/pulse/server/internal/workspaces"
	"github.com/pulse/server/internal/yws"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	cfg, err := loadConfig()
	if err != nil {
		logger.Error("config load failed", "error", err)
		os.Exit(1)
	}
	logger.Info("config loaded",
		"env", cfg.Env, "port", cfg.Port,
		"db", pulsedb.DSNForLog(cfg.DatabaseURL),
		"redis", cfg.RedisURL,
	)

	// ctx startup — terpisah dari ctx request. Timeout supaya startup tidak hang.
	startupCtx, startupCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer startupCancel()

	// --- DB pool + migrations ---
	db, err := pulsedb.New(startupCtx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("db open failed", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := pulsedb.Migrate(db, migrations.FS, "."); err != nil {
		logger.Error("migration failed", "error", err)
		os.Exit(1)
	}
	logger.Info("migrations applied")

	// pgxpool dipakai langsung oleh repositories & refresh service untuk
	// ergonomi & performance. Kita parse ulang config dari URL yang sama.
	poolCfg, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		logger.Error("pgxpool parse failed", "error", err)
		os.Exit(1)
	}
	pool, err := pgxpool.NewWithConfig(startupCtx, poolCfg)
	if err != nil {
		logger.Error("pgxpool open failed", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	// --- Redis ---
	rdb := redis.NewClient(&redis.Options{Addr: redisAddr(cfg.RedisURL)})
	defer rdb.Close()
	if err := rdb.Ping(startupCtx).Err(); err != nil {
		logger.Error("redis ping failed", "error", err)
		os.Exit(1)
	}

	// --- Auth services ---
	jwtSvc := auth.NewJWTService(cfg.JWTSecret, cfg.JWTAccessTTL)
	refreshSvc := auth.NewRefreshService(pool, cfg.RefreshTTL)

	// --- Repositories ---
	usersRepo := users.NewRepo(pool)
	wsRepo := workspaces.NewRepo(pool)
	docsRepo := documents.NewRepo(pool)
	boardRepo := boards.NewRepo(pool)
	snapRepo := documents.NewSnapshotRepo(pool)

	// --- Yjs WebSocket realtime layer ---
	httpapi.SetWSJWTVerifier(jwtSvc)
	ywsStore := yws.NewStore(logger.With("component", "yws-store"))
	wsHandler := yws.NewWSHandler(ywsStore, docsRepo, snapRepo, cfg.CORSOrigin, logger.With("component", "ws"))

	// --- Board WebSocket hub ---
	boardHub := yws.NewBoardHub(logger.With("component", "board-ws"))
	boardWSHandler := yws.NewBoardWSHandler(boardHub, boardRepo, cfg.CORSOrigin, logger.With("component", "board-ws"))
	httpapi.SetBoardEventBroadcaster(boardHub.Broadcast)

	// --- Doc state broadcaster untuk snapshot restore ---
	httpapi.SetDocStateBroadcaster(func(docID uuid.UUID, data []byte) {
		doc := ywsStore.GetOrCreate(docID)
		syncMsg := yws.BuildSyncStep2Message(data)
		doc.Broadcast(syncMsg, nil)
	})

	// --- Persistence worker ---
	persistWorker := persistence.NewWorker(
		logger.With("component", "persistence"),
		persistence.DefaultConfig(),
		ywsStore,
		snapRepo,
		docsRepo,
		pool,
	)
	persistCtx, persistCancel := context.WithCancel(context.Background())
	defer persistCancel()
	go persistWorker.Start(persistCtx)

	// --- Router ---
	handler := httpapi.NewRouter(httpapi.RouterDeps{
		IsDev:          cfg.IsDev,
		CORSOrig:       cfg.CORSOrigin,
		DB:             db,
		Redis:          rdb,
		Jwt:            jwtSvc,
		Refresh:        refreshSvc,
		UsersRepo:      usersRepo,
		WsRepo:         wsRepo,
		DocsRepo:       docsRepo,
		BoardRepo:      boardRepo,
		SnapRepo:       snapRepo,
		WSHandler:      wsHandler,
		BoardWSHandler: boardWSHandler,
		AccessTTL:      cfg.JWTAccessTTL,
		RefreshTTL:     cfg.RefreshTTL,
	})

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		// ReadTimeout/WriteTimeout tidak dipasang di sini karena Fase 2+ akan
		// punya koneksi WebSocket (long-lived) — timeout global akan memutusnya.
		// Per-endpoint timeout akan ditangani via context.
		IdleTimeout:       60 * time.Second,
	}

	// --- Graceful shutdown ---
	// Dengarkan SIGINT/SIGTERM. Saat sinyal masuk, beri waktu 15 detik untuk
	// menyelesaikan in-flight request sebelum exit.
	done := make(chan struct{})
	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
		<-sig
		logger.Info("shutdown signal received")

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			logger.Error("shutdown error", "error", err)
		}
		close(done)
	}()

	logger.Info("server starting", "addr", srv.Addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("server failed", "error", err)
		os.Exit(1)
	}
	<-done
	logger.Info("server stopped cleanly")
}
