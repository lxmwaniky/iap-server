package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/lxmwaniky/iap-server/internal/domain"
)

type PlayGateway interface {
	VerifyPurchase(ctx context.Context, productID, purchaseToken string, purchaseType domain.PurchaseType) (*domain.VerifiedPurchase, error)
	AcknowledgePurchase(ctx context.Context, productID, purchaseToken string, purchaseType domain.PurchaseType) error
}

type iapUsecase struct {
	repo    domain.PurchaseRepository
	gateway PlayGateway
}

func NewIAPUsecase(repo domain.PurchaseRepository, gateway PlayGateway) domain.IAPUsecase {
	return &iapUsecase{
		repo:    repo,
		gateway: gateway,
	}
}

func (u *iapUsecase) VerifyPurchase(ctx context.Context, req domain.VerifyPurchaseRequest) (*domain.VerifyPurchaseResult, error) {
	if req.UserID == "" || req.ProductID == "" || req.PurchaseToken == "" {
		return nil, domain.ErrInvalidPurchase
	}
	if req.Type != domain.PurchaseTypeOneTime && req.Type != domain.PurchaseTypeSubscription {
		return nil, domain.ErrInvalidPurchase
	}

	existing, err := u.repo.FindPurchaseByToken(ctx, req.PurchaseToken)
	if err != nil {
		return nil, fmt.Errorf("failed to find purchase by token: %w", err)
	}
	if existing != nil && existing.UserID != req.UserID {
		return nil, domain.ErrPurchaseOwnedByAnotherUser
	}

	verified, err := u.gateway.VerifyPurchase(ctx, req.ProductID, req.PurchaseToken, req.Type)
	if err != nil {
		return nil, fmt.Errorf("failed to verify purchase: %w", err)
	}
	if verified.ProductID != "" && verified.ProductID != req.ProductID {
		return nil, domain.ErrInvalidPurchase
	}
	if verified.PurchaseToken != "" && verified.PurchaseToken != req.PurchaseToken {
		return nil, domain.ErrInvalidPurchase
	}

	now := time.Now().UTC()
	purchase := &domain.Purchase{
		UserID:        req.UserID,
		ProductID:     req.ProductID,
		PurchaseToken: req.PurchaseToken,
		Type:          req.Type,
		Status:        verified.Status,
		OrderID:       verified.OrderID,
		ObfuscatedID:  verified.ObfuscatedID,
		ExpiresAt:     verified.ExpiresAt,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	entitled := isEntitlingPurchase(verified)
	var entitlement *domain.Entitlement
	if entitled {
		entitlement = &domain.Entitlement{
			UserID:    req.UserID,
			ProductID: req.ProductID,
			Status:    domain.EntitlementStatusActive,
			Source:    req.Type,
			ExpiresAt: verified.ExpiresAt,
			CreatedAt: now,
			UpdatedAt: now,
		}
	}

	if err := u.repo.SaveVerifiedPurchase(ctx, purchase, entitlement); err != nil {
		return nil, fmt.Errorf("failed to save verified purchase: %w", err)
	}

	if entitled && verified.NeedsAcknowledgement {
		if err := u.gateway.AcknowledgePurchase(ctx, req.ProductID, req.PurchaseToken, req.Type); err != nil {
			return nil, fmt.Errorf("failed to acknowledge purchase: %w", err)
		}
	}

	return &domain.VerifyPurchaseResult{
		Entitled:  entitled,
		Status:    verified.Status,
		ProductID: req.ProductID,
		ExpiresAt: verified.ExpiresAt,
	}, nil
}

func (u *iapUsecase) GetEntitlement(ctx context.Context, userID, productID string) (*domain.Entitlement, error) {
	if userID == "" || productID == "" {
		return nil, domain.ErrInvalidPurchase
	}
	return u.repo.FindEntitlement(ctx, userID, productID)
}

func (u *iapUsecase) ProcessNotification(ctx context.Context, notification *domain.PlayNotification) error {
	if notification == nil || notification.PurchaseToken == "" {
		return domain.ErrInvalidPurchase
	}
	created, err := u.repo.RecordNotification(ctx, notification)
	if err != nil {
		return fmt.Errorf("failed to record play notification: %w", err)
	}
	if !created {
		return nil
	}

	existing, err := u.repo.FindPurchaseByToken(ctx, notification.PurchaseToken)
	if err != nil {
		return fmt.Errorf("failed to find purchase by token: %w", err)
	}
	if existing == nil {
		return nil
	}

	productID := notification.ProductID
	if productID == "" {
		productID = existing.ProductID
	}
	verified, err := u.gateway.VerifyPurchase(ctx, productID, notification.PurchaseToken, existing.Type)
	if err != nil {
		return fmt.Errorf("failed to verify notified purchase: %w", err)
	}

	now := time.Now().UTC()
	purchase := &domain.Purchase{
		UserID:         existing.UserID,
		ProductID:      productID,
		PurchaseToken:  notification.PurchaseToken,
		Type:           existing.Type,
		Status:         verified.Status,
		OrderID:        verified.OrderID,
		ObfuscatedID:   verified.ObfuscatedID,
		ExpiresAt:      verified.ExpiresAt,
		AcknowledgedAt: existing.AcknowledgedAt,
		CreatedAt:      existing.CreatedAt,
		UpdatedAt:      now,
	}
	entitlement := &domain.Entitlement{
		UserID:    existing.UserID,
		ProductID: productID,
		Status:    domain.EntitlementStatusInactive,
		Source:    existing.Type,
		ExpiresAt: verified.ExpiresAt,
		UpdatedAt: now,
	}
	if isEntitlingPurchase(verified) {
		entitlement.Status = domain.EntitlementStatusActive
	}
	return u.repo.SaveVerifiedPurchase(ctx, purchase, entitlement)
}

func (u *iapUsecase) Ping(ctx context.Context) error {
	return u.repo.Ping(ctx)
}

func isEntitlingPurchase(purchase *domain.VerifiedPurchase) bool {
	if purchase == nil {
		return false
	}
	if purchase.Status != domain.PurchaseStatusActive {
		return false
	}
	if purchase.Type == domain.PurchaseTypeSubscription && purchase.ExpiresAt != nil && purchase.ExpiresAt.Before(time.Now().UTC()) {
		return false
	}
	return true
}
