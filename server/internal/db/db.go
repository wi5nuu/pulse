// Package db menyediakan *sql.DB pool dan util migrasi goose.
//
// Driver: pgx v5. Kami pakai pgxpool untuk connection pooling di bawah hood,
// lalu diekspos sebagai *sql.DB standar supaya kompatibel dengan goose dan
// repository pattern berbasis database/sql.
package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"net/url"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

// New membuka pool koneksi Postgres dengan parameter yang aman untuk app server:
// batas max conns, lifetime, dan idle timeout. Pool dikelola pgxpool, lalu
// diekspos sebagai *sql.DB agar kompatibel dengan database/sql & goose.
func New(ctx context.Context, databaseURL string) (*sql.DB, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}

	// 25 max conns cukup untuk dev & small prod. Naikkan kalau load test (Fase 8)
	// menunjukkan kebutuhan —koneksi idle tidak berguna untuk app real-time.
	cfg.MaxConns = 25
	cfg.MinConns = 2
	cfg.MaxConnLifetime = time.Hour
	cfg.MaxConnIdleTime = 15 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("open pool: %w", err)
	}

	// Healthcheck awal: gagal cepat di startup kalau DB tidak reachable.
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	// Konversi pgxpool → *sql.DB via stdlib adapter resmi pgx v5.
	return stdlib.OpenDBFromPool(pool), nil
}

// Migrate menjalankan goose.Up terhadap embed.FS migration.
func Migrate(db *sql.DB, migrationsFS embed.FS, dir string) error {
	goose.SetLogger(goose.NopLogger())

	sub, err := fs.Sub(migrationsFS, dir)
	if err != nil {
		return fmt.Errorf("sub migrations fs: %w", err)
	}
	// goose.SetBaseFS tidak return error di versi ini — kita Set, lalu Up.
	goose.SetBaseFS(sub)
	if err := goose.Up(db, "."); err != nil {
		return fmt.Errorf("goose up: %w", err)
	}
	return nil
}

// DSNForLog mengembalikan DSN dengan password di-mask supaya tidak bocor ke log.
func DSNForLog(databaseURL string) string {
	u, err := url.Parse(databaseURL)
	if err != nil {
		return "[unparseable dsn]"
	}
	if u.User != nil {
		if _, has := u.User.Password(); has {
			u.User = url.UserPassword(u.User.Username(), "****")
		}
	}
	return u.String()
}
