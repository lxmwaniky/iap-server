package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lxmwaniky/iap-server/internal/domain"
)

const AnonymousUserTokenHeader = "X-IAP-User-Token"

type AnonymousAuthenticator struct {
	secret []byte
	ttl    time.Duration
}

func NewAnonymousAuthenticator(secret string, ttl time.Duration) *AnonymousAuthenticator {
	return &AnonymousAuthenticator{
		secret: []byte(secret),
		ttl:    ttl,
	}
}

func (a *AnonymousAuthenticator) NewSession() (*domain.AnonymousSession, error) {
	userID, err := newUUID()
	if err != nil {
		return nil, fmt.Errorf("failed to create anonymous user id: %w", err)
	}
	expiresAt := time.Now().UTC().Add(a.ttl)
	payload := anonymousTokenPayload{
		Subject:   userID,
		ExpiresAt: expiresAt.Unix(),
	}
	token, err := a.sign(payload)
	if err != nil {
		return nil, err
	}
	return &domain.AnonymousSession{
		UserID:    userID,
		Token:     token,
		ExpiresAt: expiresAt,
	}, nil
}

func (a *AnonymousAuthenticator) Authenticate(ctx context.Context, token string) (*domain.User, error) {
	_ = ctx
	payload, err := a.verify(strings.TrimSpace(token))
	if err != nil {
		return nil, err
	}
	if _, err := uuid.Parse(payload.Subject); err != nil {
		return nil, ErrInvalidToken
	}
	return &domain.User{ID: payload.Subject}, nil
}

type anonymousTokenPayload struct {
	Subject   string `json:"sub"`
	ExpiresAt int64  `json:"exp"`
	Nonce     string `json:"nonce"`
}

func (a *AnonymousAuthenticator) sign(payload anonymousTokenPayload) (string, error) {
	if payload.Nonce == "" {
		nonce := make([]byte, 16)
		if _, err := rand.Read(nonce); err != nil {
			return "", fmt.Errorf("failed to create anonymous token nonce: %w", err)
		}
		payload.Nonce = base64.RawURLEncoding.EncodeToString(nonce)
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal anonymous token: %w", err)
	}
	encodedBody := base64.RawURLEncoding.EncodeToString(body)
	mac := hmac.New(sha256.New, a.secret)
	_, _ = mac.Write([]byte(encodedBody))
	return "anon.v1." + encodedBody + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

func (a *AnonymousAuthenticator) verify(token string) (*anonymousTokenPayload, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 4 || parts[0] != "anon" || parts[1] != "v1" {
		return nil, ErrInvalidToken
	}
	mac := hmac.New(sha256.New, a.secret)
	_, _ = mac.Write([]byte(parts[2]))
	expectedSig := mac.Sum(nil)
	gotSig, err := base64.RawURLEncoding.DecodeString(parts[3])
	if err != nil {
		return nil, ErrInvalidToken
	}
	if !hmac.Equal(gotSig, expectedSig) {
		return nil, ErrInvalidToken
	}
	body, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, ErrInvalidToken
	}
	var payload anonymousTokenPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, ErrInvalidToken
	}
	if payload.Subject == "" || payload.ExpiresAt == 0 || time.Now().Unix() >= payload.ExpiresAt {
		return nil, ErrInvalidToken
	}
	return &payload, nil
}

func newUUID() (string, error) {
	id, err := uuid.NewRandom()
	if err != nil {
		return "", err
	}
	return id.String(), nil
}
