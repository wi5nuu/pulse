package httpapi

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"
)

// ============================================================
// Middleware enterprise: security headers, request ID, logging,
// rate limiting. Dipakai di router.go secara berurutan.
// ============================================================

type ctxKey string

const ctxKeyRequestID ctxKey = "request_id"

// RequestIDFrom mengambil request ID dari context (untuk logging).
func RequestIDFrom(ctx context.Context) string {
	if v, ok := ctx.Value(ctxKeyRequestID).(string); ok {
		return v
	}
	return ""
}

// SecurityHeaders menambahkan security headers ke semua response (fix audit:
// sebelumnya tidak ada sama sekali).
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("X-XSS-Protection", "1; mode=block")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		// CSP: app ini SPA — izinkan self + inline styles (Tailwind).
		// React membutuhkan 'unsafe-inline' untuk style & script di dev;
		// di produksi tetap longgar karena tanpa nonce infra. Ini baseline
		// yang menghalangi banyak injection vektor.
		h.Set("Content-Security-Policy",
			"default-src 'self'; "+
				"script-src 'self' 'unsafe-inline'; "+
				"style-src 'self' 'unsafe-inline'; "+
				"img-src 'self' data: blob:; "+
				"connect-src 'self' ws: wss:; "+
				"font-src 'self' data:; "+
				"frame-ancestors 'none'")
		if r.TLS != nil {
			h.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
}

// RequestID middleware: generate UUID per request, expose via header
// X-Request-Id, dan simpan di context untuk logging terstruktur.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-Id")
		if id == "" || len(id) > 128 {
			var buf [16]byte
			_, _ = rand.Read(buf[:])
			id = hex.EncodeToString(buf[:])
		}
		w.Header().Set("X-Request-Id", id)
		ctx := context.WithValue(r.Context(), ctxKeyRequestID, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequestLogger mencatat setiap request: method, path, status, durasi.
// PENTING: JANGAN log query string (token WS ada di query) & jangan log
// header Authorization.
func RequestLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(sw, r)
			logger.Info("request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", sw.status,
				"duration_ms", time.Since(start).Milliseconds(),
				"request_id", RequestIDFrom(r.Context()),
				"remote", clientIPSimple(r),
			)
		})
	}
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

// Flush implements http.Flusher supaya streaming response (SSE dll) berjalan
// melalui statusWriter tanpa di-buffer.
func (w *statusWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Hijack implements http.Hijacker supaya WebSocket upgrade yang melewati
// statusWriter tetap bisa mengambil alih koneksi TCP mentah.
func (w *statusWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, http.ErrNotSupported
	}
	return h.Hijack()
}

// clientIPSimple mengambil IP tanpa X-Forwarded-For (yang bisa di-spoof).
// Untuk logging saja; audit log IP memakai ini.
func clientIPSimple(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// ============================================================
// Rate limiter (token bucket per IP) — fix audit: tidak ada rate
// limiting sama sekali sebelumnya. Dipakai pada endpoint auth
// (login/register/refresh) dengan limit ketat.
// ============================================================

type ipLimiter struct {
	mu       sync.Mutex
	buckets  map[string]*bucket
	rate     float64 // token per detik
	burst    int
	lastGC   time.Time
}

type bucket struct {
	tokens float64
	last   time.Time
}

func newIPLimiter(rate float64, burst int) *ipLimiter {
	return &ipLimiter{
		buckets: make(map[string]*bucket),
		rate:    rate,
		burst:   burst,
		lastGC:  time.Now(),
	}
}

// allow mengecek apakah request dari IP diizinkan. Memakai token bucket:
// tiap IP punya burst token; token refill `rate` per detik.
func (l *ipLimiter) allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()

	// GC bucket yang sudah lama tidak dipakai (mencegah map membengkak).
	if now.Sub(l.lastGC) > 10*time.Minute {
		for k, b := range l.buckets {
			if now.Sub(b.last) > 10*time.Minute {
				delete(l.buckets, k)
			}
		}
		l.lastGC = now
	}

	b, ok := l.buckets[ip]
	if !ok {
		b = &bucket{tokens: float64(l.burst), last: now}
		l.buckets[ip] = b
	}
	// Refill berdasarkan elapsed time.
	elapsed := now.Sub(b.last).Seconds()
	refilled := b.tokens + elapsed*l.rate
	if refilled > float64(l.burst) {
		refilled = float64(l.burst)
	}
	b.tokens = refilled
	b.last = now

	if b.tokens >= 1 {
		b.tokens--
		return true
	}
	return false
}

// RateLimit middleware: batasi request per IP.
//   - Auth endpoints: rate=2/s, burst=10 (brute-force protection).
//   - API umum: rate=20/s, burst=60 (default).
func RateLimit(rate float64, burst int) func(http.Handler) http.Handler {
	limiter := newIPLimiter(rate, burst)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := clientIPSimple(r)
			if !limiter.allow(ip) {
				w.Header().Set("Retry-After", "1")
				writeError(w, http.StatusTooManyRequests, CodeTooManyRequests, "too many requests, try again later")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
