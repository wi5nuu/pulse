package auth

// VerifyWSAccessToken memvalidasi JWT access token untuk koneksi WebSocket.
// Thin wrapper di atas JWTService.Verify — dipisah dari handler HTTP biasa
// karena WebSocket tidak bisa pakai middleware chi.
//
// Token dikirim via ?token= query param (browser WS API tidak support custom
// header Authorization). Lihat catatan security di yws/handler.go.
func (s *JWTService) VerifyWSAccessToken(token string) (*Claims, error) {
	return s.Verify(token)
}
