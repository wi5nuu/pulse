// Package config memuat konfigurasi aplikasi dari environment variables.
//
// Filosofi: konfigurasi adalah input, bukan hardcoded default tersebar di kode.
// Semua nilai yang bisa berubah antar environment (dev/prod) harus lewat sini.
// Satu struct Config ditaruh di root context aplikasi — tidak ada global var.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// Config menggabungkan seluruh pengaturan runtime aplikasi.
type Config struct {
	Env           string
	Port          string
	DatabaseURL   string
	RedisURL      string
	JWTSecret     string
	JWTAccessTTL  time.Duration
	RefreshTTL    time.Duration
	CORSOrigin    string
	IsDev         bool
}

// Load membaca .env (opsional, hanya jika ada) lalu parse environment vars.
// .env di-load dengan __silent__ mode: kalau tidak ada (mis. di produksi yang
// pakai env var asli), tidak error — kami hanya mengabaikannya.
func Load() (Config, error) {
	// godotenv.Overload akan membaca .env jika ada; di produksi biasanya tidak ada
	// dan environment variabel di-set langsung oleh orchestrator (Docker/k8s).
	_ = godotenv.Overload() // best-effort, ignore error

	c := Config{
		Env:         getenv("ENV", "dev"),
		Port:        getenv("PORT", "8080"),
		DatabaseURL: os.Getenv("DATABASE_URL"),
		RedisURL:    os.Getenv("REDIS_URL"),
		JWTSecret:   os.Getenv("JWT_SECRET"),
		CORSOrigin:  getenv("CORS_ORIGIN", "http://localhost:3000"),
	}
	c.IsDev = strings.EqualFold(c.Env, "dev")

	// Validasi wajib. Lebih baik gagal cepat di startup daripada runtime error
	// samar saat koneksi DB pertama kali.
	if c.DatabaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL wajib di-set")
	}
	if c.RedisURL == "" {
		return Config{}, fmt.Errorf("REDIS_URL wajib di-set")
	}
	if c.JWTSecret == "" {
		return Config{}, fmt.Errorf("JWT_SECRET wajib di-set")
	}
	// Defense: di produksi, secret pendek = red flag. Kami tolak di startup.
	if !c.IsDev && len(c.JWTSecret) < 32 {
		return Config{}, fmt.Errorf("JWT_SECRET harus minimal 32 karakter di produksi (got %d)", len(c.JWTSecret))
	}

	accessTTL, err := time.ParseDuration(getenv("JWT_ACCESS_TTL", "15m"))
	if err != nil {
		return Config{}, fmt.Errorf("JWT_ACCESS_TTL invalid: %w", err)
	}
	c.JWTAccessTTL = accessTTL

	refreshTTL, err := time.ParseDuration(getenv("REFRESH_TTL", "720h"))
	if err != nil {
		return Config{}, fmt.Errorf("REFRESH_TTL invalid: %w", err)
	}
	c.RefreshTTL = refreshTTL

	return c, nil
}

// getenv mengembalikan nilai env atau fallback jika kosong/tidak ada.
func getenv(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

// getenvInt helper untuk nilai numerik (cadangan untuk config masa depan).
func getenvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}
