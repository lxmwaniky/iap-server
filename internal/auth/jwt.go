package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/lxmwaniky/iap-server/internal/domain"
)

var ErrInvalidToken = errors.New("invalid token")

type JWTAuthenticator struct {
	secret []byte
}

func NewJWTAuthenticator(secret string) *JWTAuthenticator {
	return &JWTAuthenticator{secret: []byte(secret)}
}

func (a *JWTAuthenticator) Authenticate(ctx context.Context, token string) (*domain.User, error) {
	_ = ctx
	claims, err := a.verify(token)
	if err != nil {
		return nil, err
	}
	userID, _ := claims["sub"].(string)
	if userID == "" {
		return nil, ErrInvalidToken
	}
	return &domain.User{ID: userID}, nil
}

func (a *JWTAuthenticator) verify(token string) (map[string]any, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, ErrInvalidToken
	}

	header, err := decodeSegment(parts[0])
	if err != nil {
		return nil, ErrInvalidToken
	}
	alg, _ := header["alg"].(string)
	if alg != "HS256" {
		return nil, ErrInvalidToken
	}

	signingInput := parts[0] + "." + parts[1]
	expectedMAC := hmac.New(sha256.New, a.secret)
	_, _ = expectedMAC.Write([]byte(signingInput))
	expectedSig := expectedMAC.Sum(nil)

	gotSig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, ErrInvalidToken
	}
	if !hmac.Equal(gotSig, expectedSig) {
		return nil, ErrInvalidToken
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, ErrInvalidToken
	}

	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, ErrInvalidToken
	}
	exp, ok := claims["exp"].(float64)
	if !ok || exp == 0 {
		return nil, ErrInvalidToken
	}
	if time.Now().Unix() >= int64(exp) {
		return nil, fmt.Errorf("token expired: %w", ErrInvalidToken)
	}
	return claims, nil
}

func decodeSegment(segment string) (map[string]any, error) {
	raw, err := base64.RawURLEncoding.DecodeString(segment)
	if err != nil {
		return nil, err
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, err
	}
	return decoded, nil
}
