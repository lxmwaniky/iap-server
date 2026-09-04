package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestJWTAuthenticator_RejectsMissingExp(t *testing.T) {
	token := signJWT(t, "secret", map[string]any{
		"sub": "user-123",
	})
	authenticator := NewJWTAuthenticator("secret")

	_, err := authenticator.Authenticate(context.Background(), token)
	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected invalid token error, got %v", err)
	}
}

func TestJWTAuthenticator_RejectsUnsupportedAlgorithm(t *testing.T) {
	token := signJWTWithHeader(t, "secret", map[string]any{
		"alg": "none",
		"typ": "JWT",
	}, map[string]any{
		"sub": "user-123",
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	authenticator := NewJWTAuthenticator("secret")

	_, err := authenticator.Authenticate(context.Background(), token)
	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected invalid token error, got %v", err)
	}
}

func signJWT(t *testing.T, secret string, claims map[string]any) string {
	t.Helper()
	return signJWTWithHeader(t, secret, map[string]any{
		"alg": "HS256",
		"typ": "JWT",
	}, claims)
}

func signJWTWithHeader(t *testing.T, secret string, header map[string]any, claims map[string]any) string {
	t.Helper()
	headerJSON, err := json.Marshal(header)
	if err != nil {
		t.Fatalf("marshal header: %v", err)
	}
	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("marshal claims: %v", err)
	}
	signingInput := base64.RawURLEncoding.EncodeToString(headerJSON) + "." + base64.RawURLEncoding.EncodeToString(claimsJSON)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(signingInput))
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
