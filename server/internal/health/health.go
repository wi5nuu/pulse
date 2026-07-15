// Package health menyediakan pemeriksaan kesehatan dependensi (DB, Redis).
// Dipisah dari handler supaya bisa dipanggil dari startup check juga.
package health

import (
	"context"
	"database/sql"
	"time"

	"github.com/redis/go-redis/v9"
)

type Status string

const (
	StatusOK   Status = "ok"
	StatusDown Status = "down"
)

type ComponentStatus struct {
	Status Status `json:"status"`
	LatencyMS int64 `json:"latency_ms,omitempty"`
	Error  string `json:"error,omitempty"`
}

type Report struct {
	Status       Status                    `json:"status"`
	Dependencies map[string]ComponentStatus `json:"dependencies"`
}

// CheckAll mengecek DB & Redis. Timeout per komponen 2 detik — kalau lebih,
// dianggap down (laporan status, bukan blocking).
func CheckAll(ctx context.Context, db *sql.DB, rdb *redis.Client) Report {
	deps := make(map[string]ComponentStatus)
	overall := StatusOK

	deps["postgres"] = checkDB(ctx, db)
	if deps["postgres"].Status != StatusOK {
		overall = StatusDown
	}

	deps["redis"] = checkRedis(ctx, rdb)
	if deps["redis"].Status != StatusOK {
		overall = StatusDown
	}

	return Report{Status: overall, Dependencies: deps}
}

func checkDB(ctx context.Context, db *sql.DB) ComponentStatus {
	cctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	start := time.Now()
	if err := db.PingContext(cctx); err != nil {
		return ComponentStatus{Status: StatusDown, Error: err.Error()}
	}
	return ComponentStatus{Status: StatusOK, LatencyMS: time.Since(start).Milliseconds()}
}

func checkRedis(ctx context.Context, rdb *redis.Client) ComponentStatus {
	cctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	start := time.Now()
	if err := rdb.Ping(cctx).Err(); err != nil {
		return ComponentStatus{Status: StatusDown, Error: err.Error()}
	}
	return ComponentStatus{Status: StatusOK, LatencyMS: time.Since(start).Milliseconds()}
}
