package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestIPLimiter_BurstThenBlock(t *testing.T) {
	// burst 3 → 3 request pertama OK, request ke-4 langsung ditolak.
	l := newIPLimiter(1000, 3)

	for i := 0; i < 3; i++ {
		if !l.allow("203.0.113.10") {
			t.Fatalf("request %d should be allowed (burst)", i+1)
		}
	}
	if l.allow("203.0.113.10") {
		t.Fatal("4th request should be blocked (burst exhausted)")
	}

	// IP berbeda tidak terpengaruh.
	if !l.allow("198.51.100.5") {
		t.Fatal("different IP should not be affected")
	}
}

func TestIPLimiter_RefillAfterElapsed(t *testing.T) {
	// rate 2/s, burst 2 → habiskan burst, tunggu 0.5s → 1 token refill.
	l := newIPLimiter(2, 2)
	ip := "203.0.113.20"

	if !l.allow(ip) || !l.allow(ip) {
		t.Fatal("first two should be allowed")
	}
	if l.allow(ip) {
		t.Fatal("third should be blocked")
	}

	time.Sleep(550 * time.Millisecond)
	if !l.allow(ip) {
		t.Fatal("after 0.5s at rate 2/s, one token should be available")
	}
	if l.allow(ip) {
		t.Fatal("only one token was refilled")
	}
}

func TestRateLimitMiddleware(t *testing.T) {
	// burst 2: 2 request sukses, ke-3 dapat 429.
	h := RateLimit(1000, 2)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for i := 0; i < 2; i++ {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/test", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d: got %d, want 200", i+1, rec.Code)
		}
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/test", nil))
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("3rd request: got %d, want 429", rec.Code)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Fatal("expected Retry-After header on 429")
	}
}

func TestSecurityHeaders(t *testing.T) {
	h := SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/test", nil))

	checks := []string{
		"X-Content-Type-Options",
		"X-Frame-Options",
		"Referrer-Policy",
		"X-XSS-Protection",
		"Permissions-Policy",
		"Content-Security-Policy",
	}
	for _, header := range checks {
		if rec.Header().Get(header) == "" {
			t.Errorf("missing security header: %s", header)
		}
	}
	if got := rec.Header().Get("X-Frame-Options"); got != "DENY" {
		t.Errorf("X-Frame-Options = %q, want DENY", got)
	}
}

func TestRequestIDMiddleware(t *testing.T) {
	h := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := RequestIDFrom(r.Context())
		if id == "" {
			t.Error("request id missing from context")
		}
		w.Header().Set("X-Echo", id)
		w.WriteHeader(http.StatusNoContent)
	}))

	// Tanpa header: generate baru & konsisten (context vs header).
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/test", nil))
	if rec.Header().Get("X-Request-Id") == "" {
		t.Error("expected generated X-Request-Id header")
	}
	if rec.Header().Get("X-Request-Id") != rec.Header().Get("X-Echo") {
		t.Error("context id != header id")
	}

	// Dengan header: hormati client-supplied (trace propagation).
	rec2 := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("X-Request-Id", "trace-abc-123")
	h.ServeHTTP(rec2, req)
	if rec2.Header().Get("X-Request-Id") != "trace-abc-123" {
		t.Error("expected client-supplied request id to be honored")
	}
}
