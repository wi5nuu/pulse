package httpapi

import (
	"context"
	"errors"
	"sync"

	"github.com/pulse/server/internal/auth"
)

var errNoVerifier = errors.New("JWT verifier not initialized")

var (
	wsJWT     *auth.JWTService
	wsJWTOnce sync.Once
)

// SetWSJWTVerifier menyimpan JWTService untuk verifikasi token WebSocket.
// Dipanggil sekali di main.go saat bootstrap.
func SetWSJWTVerifier(svc *auth.JWTService) {
	wsJWTOnce.Do(func() {
		wsJWT = svc
	})
}

// VerifyWSAccessToken memverifikasi JWT access token untuk koneksi WebSocket.
// Token dikirim via ?token= query param.
func VerifyWSAccessToken(_ context.Context, token string) (*auth.Claims, error) {
	if wsJWT == nil {
		return nil, errNoVerifier
	}
	return wsJWT.Verify(token)
}
