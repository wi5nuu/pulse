package comments

import (
	"crypto/rand"
	"encoding/hex"
)

// newToken menghasilkan token acak 32 byte hex (64 karakter).
// Kekuatan 256-bit — tidak bisa ditebak / brute-force.
func newToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand praktis tidak pernah gagal; kalau sampai gagal,
		// crash lebih aman daripada membuat token lemah.
		panic("crypto/rand failed: " + err.Error())
	}
	return hex.EncodeToString(b)
}