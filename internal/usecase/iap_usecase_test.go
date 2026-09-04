package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/lxmwaniky/iap-server/internal/domain"
)

type fakePurchaseRepository struct {
	purchases    map[string]*domain.Purchase
	entitlements map[string]*domain.Entitlement
	saveErr      error
}

func newFakePurchaseRepository() *fakePurchaseRepository {
	return &fakePurchaseRepository{
		purchases:    make(map[string]*domain.Purchase),
		entitlements: make(map[string]*domain.Entitlement),
	}
}

func (f *fakePurchaseRepository) SaveVerifiedPurchase(ctx context.Context, purchase *domain.Purchase, entitlement *domain.Entitlement) error {
	if f.saveErr != nil {
		return f.saveErr
	}
	f.purchases[purchase.PurchaseToken] = purchase
	if entitlement != nil {
		f.entitlements[entitlement.UserID+":"+entitlement.ProductID] = entitlement
	}
	return nil
}

func (f *fakePurchaseRepository) FindPurchaseByToken(ctx context.Context, purchaseToken string) (*domain.Purchase, error) {
	purchase, ok := f.purchases[purchaseToken]
	if !ok {
		return nil, nil
	}
	return purchase, nil
}

func (f *fakePurchaseRepository) FindEntitlement(ctx context.Context, userID, productID string) (*domain.Entitlement, error) {
	entitlement, ok := f.entitlements[userID+":"+productID]
	if !ok {
		return nil, nil
	}
	return entitlement, nil
}

func (f *fakePurchaseRepository) RecordNotification(ctx context.Context, notification *domain.PlayNotification) (bool, error) {
	return true, nil
}

func (f *fakePurchaseRepository) Ping(ctx context.Context) error {
	return nil
}

type fakePlayGateway struct {
	result              *domain.VerifiedPurchase
	verifyErr           error
	acknowledgedTokens  []string
	acknowledgeErr      error
	verificationCalls   int
	acknowledgeCallTime time.Time
	saveCallTime        *time.Time
}

func (f *fakePlayGateway) VerifyPurchase(ctx context.Context, productID, purchaseToken string, purchaseType domain.PurchaseType) (*domain.VerifiedPurchase, error) {
	f.verificationCalls++
	if f.verifyErr != nil {
		return nil, f.verifyErr
	}
	return f.result, nil
}

func (f *fakePlayGateway) AcknowledgePurchase(ctx context.Context, productID, purchaseToken string, purchaseType domain.PurchaseType) error {
	f.acknowledgedTokens = append(f.acknowledgedTokens, purchaseToken)
	f.acknowledgeCallTime = time.Now()
	return f.acknowledgeErr
}

func TestVerifyPurchase_ActiveSubscriptionSavesEntitlementBeforeAcknowledgement(t *testing.T) {
	expiresAt := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	repo := newFakePurchaseRepository()
	gateway := &fakePlayGateway{
		result: &domain.VerifiedPurchase{
			ProductID:            "pro_monthly",
			PurchaseToken:        "purchase-token-1",
			Type:                 domain.PurchaseTypeSubscription,
			Status:               domain.PurchaseStatusActive,
			OrderID:              "GPA.1234-5678-9012-34567",
			ExpiresAt:            &expiresAt,
			NeedsAcknowledgement: true,
		},
	}

	usecase := NewIAPUsecase(repo, gateway)
	result, err := usecase.VerifyPurchase(context.Background(), domain.VerifyPurchaseRequest{
		UserID:        "user-123",
		ProductID:     "pro_monthly",
		PurchaseToken: "purchase-token-1",
		Type:          domain.PurchaseTypeSubscription,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Entitled {
		t.Fatal("expected verified subscription to grant entitlement")
	}
	if result.Status != domain.PurchaseStatusActive {
		t.Errorf("expected status %s, got %s", domain.PurchaseStatusActive, result.Status)
	}
	if len(gateway.acknowledgedTokens) != 1 || gateway.acknowledgedTokens[0] != "purchase-token-1" {
		t.Fatalf("expected purchase token to be acknowledged once, got %v", gateway.acknowledgedTokens)
	}

	purchase := repo.purchases["purchase-token-1"]
	if purchase == nil {
		t.Fatal("expected verified purchase to be saved")
	}
	entitlement := repo.entitlements["user-123:pro_monthly"]
	if entitlement == nil {
		t.Fatal("expected entitlement to be saved")
	}
	if entitlement.ExpiresAt == nil || !entitlement.ExpiresAt.Equal(expiresAt) {
		t.Fatalf("expected entitlement expiry %s, got %v", expiresAt, entitlement.ExpiresAt)
	}
}

func TestVerifyPurchase_DoesNotAcknowledgeWhenSaveFails(t *testing.T) {
	repo := newFakePurchaseRepository()
	repo.saveErr = errors.New("database unavailable")
	gateway := &fakePlayGateway{
		result: &domain.VerifiedPurchase{
			ProductID:            "lifetime",
			PurchaseToken:        "purchase-token-2",
			Type:                 domain.PurchaseTypeOneTime,
			Status:               domain.PurchaseStatusActive,
			NeedsAcknowledgement: true,
		},
	}

	usecase := NewIAPUsecase(repo, gateway)
	_, err := usecase.VerifyPurchase(context.Background(), domain.VerifyPurchaseRequest{
		UserID:        "user-123",
		ProductID:     "lifetime",
		PurchaseToken: "purchase-token-2",
		Type:          domain.PurchaseTypeOneTime,
	})

	if err == nil {
		t.Fatal("expected save failure")
	}
	if len(gateway.acknowledgedTokens) != 0 {
		t.Fatalf("expected acknowledgement to be skipped when save fails, got %v", gateway.acknowledgedTokens)
	}
}

func TestVerifyPurchase_RejectsTokenOwnedByDifferentUser(t *testing.T) {
	repo := newFakePurchaseRepository()
	repo.purchases["purchase-token-3"] = &domain.Purchase{
		UserID:        "original-user",
		ProductID:     "lifetime",
		PurchaseToken: "purchase-token-3",
		Type:          domain.PurchaseTypeOneTime,
		Status:        domain.PurchaseStatusActive,
	}
	gateway := &fakePlayGateway{
		result: &domain.VerifiedPurchase{
			ProductID:            "lifetime",
			PurchaseToken:        "purchase-token-3",
			Type:                 domain.PurchaseTypeOneTime,
			Status:               domain.PurchaseStatusActive,
			NeedsAcknowledgement: true,
		},
	}

	usecase := NewIAPUsecase(repo, gateway)
	_, err := usecase.VerifyPurchase(context.Background(), domain.VerifyPurchaseRequest{
		UserID:        "other-user",
		ProductID:     "lifetime",
		PurchaseToken: "purchase-token-3",
		Type:          domain.PurchaseTypeOneTime,
	})

	if !errors.Is(err, domain.ErrPurchaseOwnedByAnotherUser) {
		t.Fatalf("expected owned-by-another-user error, got %v", err)
	}
	if gateway.verificationCalls != 0 {
		t.Fatalf("expected Google verification to be skipped, got %d calls", gateway.verificationCalls)
	}
	if len(gateway.acknowledgedTokens) != 0 {
		t.Fatalf("expected acknowledgement to be skipped, got %v", gateway.acknowledgedTokens)
	}
}
