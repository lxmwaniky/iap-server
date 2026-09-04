package auth

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestAnonymousAuthenticator_RejectsBareUUID(t *testing.T) {
	authenticator := NewAnonymousAuthenticator("secret", time.Hour)

	_, err := authenticator.Authenticate(context.Background(), "9f455c68-566f-4b8b-b92c-5ff2269d3724")
	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected invalid token error, got %v", err)
	}
}

func TestAnonymousAuthenticator_AuthenticatesMintedToken(t *testing.T) {
	authenticator := NewAnonymousAuthenticator("secret", time.Hour)
	session, err := authenticator.NewSession()
	if err != nil {
		t.Fatalf("unexpected mint error: %v", err)
	}

	user, err := authenticator.Authenticate(context.Background(), session.Token)
	if err != nil {
		t.Fatalf("unexpected auth error: %v", err)
	}
	if user.ID != session.UserID {
		t.Fatalf("expected user id %s, got %s", session.UserID, user.ID)
	}
}
