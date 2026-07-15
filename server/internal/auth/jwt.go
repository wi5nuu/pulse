package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// Claims adalah payload JWT access token. Di-keep ramping: hanya identitas
// yang dibutuhkan untuk authorize request. Role TIDAK di taruh di sini karena
// role bisa berubah (admin demote user) — selalu query DB di endpoint yang
// butuh authorize role spesifik.
type Claims struct {
	UserID uuid.UUID `json:"uid"`
	Email  string    `json:"email"`
	Name   string    `json:"name"`
	jwt.RegisteredClaims
}

// JWTService menandatangani & memverifikasi access token (HS256).
//
// Kenapa HS256 bukan RS256? Pulse single-deployment: tidak ada microservice
// lain yang perlu verify token tanpa secret bersama. RS256 worth it kalau ada
// banyak verifier independen (microservices / 3rd-party). HS256 = 1 secret,
// simpler ops. Bisa di-rotate ke RS256 tanpa breaking change nanti.
type JWTService struct {
	secret []byte
	ttl    time.Duration
}

func NewJWTService(secret string, ttl time.Duration) *JWTService {
	return &JWTService{secret: []byte(secret), ttl: ttl}
}

// Issue menghasilkan access token baru untuk user.
func (s *JWTService) Issue(userID uuid.UUID, email, name string) (string, time.Time, error) {
	now := time.Now().UTC()
	expiresAt := now.Add(s.ttl)

	claims := Claims{
		UserID: userID,
		Email:  email,
		Name:   name,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID.String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			NotBefore: jwt.NewNumericDate(now),
			Issuer:    "pulse",
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(s.secret)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("sign jwt: %w", err)
	}
	return signed, expiresAt, nil
}

// Verify mem-parse & validasi signature + expiry. Return claims jika valid.
func (s *JWTService) Verify(tokenStr string) (*Claims, error) {
	var claims Claims
	// jwt.ParseWithClaims butuh keyfunc untuk validasi signature + algoritma.
	// Kami eksplisit menolak algoritma selain HS256 (defense against
	// "alg: none" attack yang terkenal di JWT history).
	parser := jwt.NewParser(jwt.WithValidMethods([]string{"HS256"}))
	token, err := parser.ParseWithClaims(tokenStr, &claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return s.secret, nil
	})
	if err != nil {
		return nil, fmt.Errorf("parse jwt: %w", err)
	}
	if !token.Valid {
		return nil, errors.New("invalid token")
	}
	return &claims, nil
}
