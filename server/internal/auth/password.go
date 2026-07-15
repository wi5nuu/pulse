// Package auth berisi utilitas otentikasi: hashing password, JWT (access token),
// dan service refresh token (rotation + revocation). Dipisah dari repository
// user supaya auth bisa ditest independen dan mudah di-rotate secret-nya.
package auth

import (
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

// bcrypt cost 12 = ~250ms hash time di hardware modern. Cukup lambat untuk
// membatasi brute-force offline, cukup cepat untuk tidak menyebalkan di login.
// Kalau performa login jadi bottleneck di Fase 8, turunkan ke 11 atau naikkan
// ke 13 kalau ancaman dictionary attack meningkat.
const bcryptCost = 12

// HashPassword meng-hash password plaintext. Tidak return error untuk input
// panjang berlebih secara eksplisit — bcrypt membatasi 72 byte; kalau lebih,
// dia akan truncate (dengan warning di bcrypt v2). Validasi panjang password
// dilakukan di layer DTO/handler sebelum sampai sini.
func HashPassword(plain string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(plain), bcryptCost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	return string(b), nil
}

// VerifyPassword membandingkan hash dengan plaintext. Return error jika tidak
// cocok — handler menerjemahkan ini ke 401, JANGAN bocorkan apakah user ada.
func VerifyPassword(hash, plain string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain))
}
