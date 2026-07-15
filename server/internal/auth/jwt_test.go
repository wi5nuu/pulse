package auth

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestJWT_IssueAndVerify(t *testing.T) {
	svc := NewJWTService("test-secret-thats-long-enough-for-test", 15*time.Minute)

	userID := uuid.New()
	token, expiresAt, err := svc.Issue(userID, "test@example.com", "Test User")
	if err != nil {
		t.Fatal("issue:", err)
	}
	if token == "" {
		t.Fatal("token is empty")
	}
	if expiresAt.Before(time.Now()) {
		t.Fatal("expiresAt in past")
	}

	claims, err := svc.Verify(token)
	if err != nil {
		t.Fatal("verify:", err)
	}
	if claims.UserID != userID {
		t.Errorf("expected user %s, got %s", userID, claims.UserID)
	}
	if claims.Email != "test@example.com" {
		t.Errorf("expected email test@example.com, got %s", claims.Email)
	}
	if claims.Name != "Test User" {
		t.Errorf("expected name Test User, got %s", claims.Name)
	}
}

func TestJWT_ExpiredToken(t *testing.T) {
	// Token dengan TTL 0 → langsung expired
	svc := NewJWTService("test-secret-2", 0)

	token, _, err := svc.Issue(uuid.New(), "a@b.com", "A")
	if err != nil {
		t.Fatal("issue:", err)
	}

	// Tunggu 1ms supaya expired
	time.Sleep(time.Millisecond)

	_, err = svc.Verify(token)
	if err == nil {
		t.Fatal("expected error for expired token, got nil")
	}
}

func TestJWT_InvalidSignature(t *testing.T) {
	svc1 := NewJWTService("secret-a", 15*time.Minute)
	svc2 := NewJWTService("secret-b", 15*time.Minute)

	token, _, err := svc1.Issue(uuid.New(), "a@b.com", "A")
	if err != nil {
		t.Fatal("issue:", err)
	}

	// Verify dengan secret berbeda
	_, err = svc2.Verify(token)
	if err == nil {
		t.Fatal("expected error for wrong secret, got nil")
	}
}

func TestJWT_MalformedToken(t *testing.T) {
	svc := NewJWTService("test-secret-3", 15*time.Minute)

	_, err := svc.Verify("this.is.not.a.jwt")
	if err == nil {
		t.Fatal("expected error for malformed token, got nil")
	}
}

func TestJWT_AlgNone(t *testing.T) {
	// Cek bahwa "alg: none" attack tidak lolos
	svc := NewJWTService("test-secret-4", 15*time.Minute)

	// Token buatan dengan alg:none
	malicious := "eyJhbGciOiJub25lIiwidHlwIjoiSldUIn0.eyJzdWIiOiIxMjM0NTY3ODkwIn0."
	_, err := svc.Verify(malicious)
	if err == nil {
		t.Fatal("expected error for alg:none token, got nil")
	}
}
