package httpapi

import (
	"context"

	"github.com/google/uuid"
)

// contextKey adalah tipe unik untuk key context supaya tidak collision dengan
// library lain. Pendekatan standar Go (lihat context docs).
type contextKey int

const (
	keyUserID contextKey = iota
	keyUserEmail
	keyUserName
)

// withUser menempelkan identitas user hasil verifikasi JWT ke context request.
func withUser(ctx context.Context, id uuid.UUID, email, name string) context.Context {
	ctx = context.WithValue(ctx, keyUserID, id)
	ctx = context.WithValue(ctx, keyUserEmail, email)
	ctx = context.WithValue(ctx, keyUserName, name)
	return ctx
}

func userIDFrom(ctx context.Context) (uuid.UUID, bool) {
	v, ok := ctx.Value(keyUserID).(uuid.UUID)
	return v, ok
}

func emailFrom(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(keyUserEmail).(string)
	return v, ok
}

func userEmailFrom(ctx context.Context) string {
	v, _ := ctx.Value(keyUserEmail).(string)
	return v
}

func userNameFrom(ctx context.Context) string {
	v, _ := ctx.Value(keyUserName).(string)
	return v
}
