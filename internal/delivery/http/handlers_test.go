package http

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/lxmwaniky/iap-server/internal/auth"
	"github.com/lxmwaniky/iap-server/internal/domain"
)

type fakeAuthenticator struct {
	user *domain.User
	err  error
}

func (f *fakeAuthenticator) Authenticate(ctx context.Context, token string) (*domain.User, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.user, nil
}

type fakeIAPUsecase struct {
	verifyReq domain.VerifyPurchaseRequest
	verifyRes *domain.VerifyPurchaseResult
}

func (f *fakeIAPUsecase) VerifyPurchase(ctx context.Context, req domain.VerifyPurchaseRequest) (*domain.VerifyPurchaseResult, error) {
	f.verifyReq = req
	return f.verifyRes, nil
}

func (f *fakeIAPUsecase) GetEntitlement(ctx context.Context, userID, productID string) (*domain.Entitlement, error) {
	return nil, nil
}

func (f *fakeIAPUsecase) ProcessNotification(ctx context.Context, notification *domain.PlayNotification) error {
	return nil
}

func (f *fakeIAPUsecase) Ping(ctx context.Context) error {
	return nil
}

func TestVerifyPurchase_UsesAuthenticatedUserID(t *testing.T) {
	usecase := &fakeIAPUsecase{
		verifyRes: &domain.VerifyPurchaseResult{
			Entitled:  true,
			Status:    domain.PurchaseStatusActive,
			ProductID: "pro_monthly",
		},
	}
	handler := NewHandler(usecase, &fakeAuthenticator{
		user: &domain.User{ID: "trusted-user"},
	})

	body, _ := json.Marshal(map[string]string{
		"user_id":        "attacker-controlled",
		"product_id":     "pro_monthly",
		"purchase_token": "purchase-token-1",
		"type":           string(domain.PurchaseTypeSubscription),
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/iap/verify", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer valid-token")
	rec := httptest.NewRecorder()

	handler.VerifyPurchase(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if usecase.verifyReq.UserID != "trusted-user" {
		t.Fatalf("expected trusted user id, got %s", usecase.verifyReq.UserID)
	}
}

func TestVerifyPurchase_RejectsMissingBearerToken(t *testing.T) {
	usecase := &fakeIAPUsecase{}
	handler := NewHandler(usecase, &fakeAuthenticator{
		user: &domain.User{ID: "trusted-user"},
	})

	body, _ := json.Marshal(map[string]string{
		"product_id":     "pro_monthly",
		"purchase_token": "purchase-token-1",
		"type":           string(domain.PurchaseTypeSubscription),
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/iap/verify", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	handler.VerifyPurchase(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", rec.Code)
	}
	if usecase.verifyReq.UserID != "" {
		t.Fatalf("expected usecase not to be called, got user id %s", usecase.verifyReq.UserID)
	}
}

func TestVerifyPurchase_AnonymousModeUsesIAPUserIDHeader(t *testing.T) {
	authenticator := auth.NewAnonymousAuthenticator("secret", time.Hour)
	session, err := authenticator.NewSession()
	if err != nil {
		t.Fatalf("unexpected session error: %v", err)
	}
	usecase := &fakeIAPUsecase{
		verifyRes: &domain.VerifyPurchaseResult{
			Entitled:  true,
			Status:    domain.PurchaseStatusActive,
			ProductID: "pro_monthly",
		},
	}
	handler := NewHandler(usecase, authenticator)

	body, _ := json.Marshal(map[string]string{
		"product_id":     "pro_monthly",
		"purchase_token": "purchase-token-1",
		"type":           string(domain.PurchaseTypeSubscription),
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/iap/verify", bytes.NewReader(body))
	req.Header.Set("X-IAP-User-Token", session.Token)
	rec := httptest.NewRecorder()

	handler.VerifyPurchase(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if usecase.verifyReq.UserID != session.UserID {
		t.Fatalf("expected anonymous user id from header, got %s", usecase.verifyReq.UserID)
	}
}

func TestCreateAnonymousSession_MintsSignedUserToken(t *testing.T) {
	authenticator := auth.NewAnonymousAuthenticator("secret", time.Hour)
	handler := NewHandler(&fakeIAPUsecase{}, authenticator)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/iap/anonymous-sessions", nil)
	rec := httptest.NewRecorder()

	handler.CreateAnonymousSession(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d", rec.Code)
	}
	var body struct {
		UserID string `json:"user_id"`
		Token  string `json:"token"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.UserID == "" || body.Token == "" {
		t.Fatalf("expected user id and token, got %+v", body)
	}
	user, err := authenticator.Authenticate(context.Background(), body.Token)
	if err != nil {
		t.Fatalf("expected minted token to authenticate: %v", err)
	}
	if user.ID != body.UserID {
		t.Fatalf("expected authenticated user id %s, got %s", body.UserID, user.ID)
	}
}

func TestHandleRTDN_RequiresBearerToken(t *testing.T) {
	handler := NewHandler(&fakeIAPUsecase{}, &fakeAuthenticator{user: &domain.User{ID: "user-1"}})
	body := pubsubPayload(t, map[string]any{
		"packageName": "com.example.app",
		"subscriptionNotification": map[string]any{
			"notificationType": 1,
			"purchaseToken":    "purchase-token-1",
			"subscriptionId":   "pro_monthly",
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/iap/rtdn?token=secret", bytes.NewReader(body))
	req = req.WithContext(context.WithValue(req.Context(), rtdnTokenKey{}, "secret"))
	rec := httptest.NewRecorder()

	handler.HandleRTDN(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", rec.Code)
	}
}

func pubsubPayload(t *testing.T, playNotification map[string]any) []byte {
	t.Helper()
	rawPlayNotification, err := json.Marshal(playNotification)
	if err != nil {
		t.Fatalf("marshal play notification: %v", err)
	}
	body, err := json.Marshal(map[string]any{
		"message": map[string]any{
			"messageId": "message-1",
			"data":      base64.StdEncoding.EncodeToString(rawPlayNotification),
		},
	})
	if err != nil {
		t.Fatalf("marshal pubsub payload: %v", err)
	}
	return body
}
