// Package httpapi berisi handler HTTP, router, middleware, dan DTO.
//
// Konvensi response:
//   - Sukses: 2xx, body JSON plain (lihat handler masing-masing).
//   - Error:   { "error": { "code": "MACHINE_CODE", "message": "human text", "fields"?: {...} } }
//     `code` selalu SCREAMING_SNAKE. `fields` hanya untuk VALIDATION_FAILED.
package httpapi

import (
	"encoding/json"
	"net/http"
)

// Error code standar. Tambah sesuai kebutuhan, jangan reuse lintas makna.
const (
	CodeBadRequest         = "BAD_REQUEST"
	CodeValidationFailed   = "VALIDATION_FAILED"
	CodeUnauthorized       = "UNAUTHORIZED"
	CodeForbidden          = "FORBIDDEN"
	CodeNotFound           = "NOT_FOUND"
	CodeConflict           = "CONFLICT"
	CodeInternal           = "INTERNAL_ERROR"
	CodeTooManyRequests    = "RATE_LIMITED"
)

type errorBody struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Fields  map[string]string `json:"fields,omitempty"`
}

// writeJSON helper: encode v ke ResponseWriter dengan content-type JSON.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError menulis error response dengan code & message konsisten.
func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, errorBody{Error: errorDetail{Code: code, Message: message}})
}

// writeValidationError menulis 422 dengan map field → pesan error.
func writeValidationError(w http.ResponseWriter, fields map[string]string) {
	writeJSON(w, http.StatusUnprocessableEntity, errorBody{
		Error: errorDetail{
			Code:    CodeValidationFailed,
			Message: "request validation failed",
			Fields:  fields,
		},
	})
}

// NewWSWriteError menulis error JSON ke ResponseWriter (dipakai oleh yws
// sebelum WS upgrade untuk menolak handshake).
func NewWSWriteError(w http.ResponseWriter, status int, message string) {
	writeError(w, status, CodeForStatus(status), message)
}

// CodeForStatus memetakan HTTP status ke error code.
func CodeForStatus(status int) string {
	switch status {
	case http.StatusBadRequest:
		return CodeBadRequest
	case http.StatusUnauthorized:
		return CodeUnauthorized
	case http.StatusForbidden:
		return CodeForbidden
	case http.StatusNotFound:
		return CodeNotFound
	case http.StatusConflict:
		return CodeConflict
	default:
		return CodeInternal
	}
}
