// Package ycodec mengimplementasikan encoding/decoding primitif lib0
// yang dipakai oleh y-protocols sync & awareness.
//
// Spesifikasi: lihat y-protocols/PROTOCOL.md §2.
//
// Penting: Go tidak punya library resmi Yjs. Tapi karena server Pulse adalah
// RELAY (bukan re-implementasi CRDT — lihat task §2), kami hanya butuh:
//   - Parse varUint & varBuffer untuk membaca header pesan.
//   - Membangun ulang pesan untuk broadcast (untuk SYNC_STEP2 kami butuh
//     inject state vector di reply; itu tetap hanya memanipulasi bytes opaque).
//
// Server TIDAK pernah decode isi Yjs update — itu tetap opaque blob yang
// diteruskan ke client lain. Client (browser) yang punya Yjs library penuh.
package ycodec

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// ErrIncomplete dipakai saat buffer tidak cukup byte untuk decode.
var ErrIncomplete = errors.New("ycodec: incomplete buffer")

// --- varUint (LEB128 unsigned 53-bit) ---

// ReadVarUint membaca varUint dari b, return (value, bytesConsumed, error).
// Implementasi mengikuti lib0/decoding.readVarUint — 7 bit payload per byte,
// high bit = continuation, least-significant first.
func ReadVarUint(b []byte) (uint64, int, error) {
	var num uint64
	var bits uint
	for i, by := range b {
		if i >= 8 {
			// VarUint di lib0 maksimal 53 bit → ≤ 8 byte. Lebih dari itu invalid.
			return 0, 0, fmt.Errorf("ycodec: varUint exceeds 8 bytes")
		}
		num |= uint64(by&0x7f) << bits
		bits += 7
		consumed := i + 1
		if by&0x80 == 0 {
			return num, consumed, nil
		}
	}
	return 0, 0, ErrIncomplete
}

// WriteVarUint menulis n sebagai varUint ke dst (append, return new slice).
func WriteVarUint(dst []byte, n uint64) []byte {
	// Lib0 implementation: loop writing 7-bit chunks.
	for {
		b := byte(n & 0x7f)
		n >>= 7
		if n == 0 {
			return append(dst, b)
		}
		// set continuation bit
		dst = append(dst, b|0x80)
	}
}

// --- varBuffer (length-prefixed) ---

// ReadVarBuffer membaca varBuffer: varUint length + length bytes payload.
// Return (payload, bytesConsumed, error).
func ReadVarBuffer(b []byte) ([]byte, int, error) {
	length, n, err := ReadVarUint(b)
	if err != nil {
		return nil, 0, err
	}
	if uint64(len(b)-n) < length {
		return nil, 0, ErrIncomplete
	}
	end := n + int(length)
	return b[n:end], end, nil
}

// WriteVarBuffer menulis payload sebagai varBuffer ke dst.
func WriteVarBuffer(dst, payload []byte) []byte {
	dst = WriteVarUint(dst, uint64(len(payload)))
	return append(dst, payload...)
}

// ReadUint8 membaca 1 byte. Dipakai untuk message type headers.
func ReadUint8(b []byte) (byte, int, error) {
	if len(b) < 1 {
		return 0, 0, ErrIncomplete
	}
	return b[0], 1, nil
}

// WriteUint8 append 1 byte.
func WriteUint8(dst []byte, v byte) []byte {
	return append(dst, v)
}

// ReadVarString membaca varString (varBuffer interpreted as UTF-8). Dipakai di
// awareness json state. Kami return raw bytes; caller yang decode UTF-8.
func ReadVarString(b []byte) ([]byte, int, error) {
	return ReadVarBuffer(b)
}

// --- io helpers (untuk streaming parse) ---

// byteOrder untuk uint32/uint64 yang mungkin dipakai di protocol extensions.
var byteOrder = binary.BigEndian

// Sealed: pastikan io tidak unused (digunakan oleh consumer untuk EOF check).
var _ = io.EOF
