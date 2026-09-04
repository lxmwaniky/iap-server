package playbilling

import (
	"context"
	"fmt"
	"time"

	"github.com/lxmwaniky/iap-server/internal/domain"
	"google.golang.org/api/androidpublisher/v3"
)

type Client struct {
	service     *androidpublisher.Service
	packageName string
}

func NewClient(ctx context.Context, packageName string) (*Client, error) {
	service, err := androidpublisher.NewService(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create android publisher client: %w", err)
	}
	return &Client{service: service, packageName: packageName}, nil
}

func (c *Client) VerifyPurchase(ctx context.Context, productID, purchaseToken string, purchaseType domain.PurchaseType) (*domain.VerifiedPurchase, error) {
	switch purchaseType {
	case domain.PurchaseTypeSubscription:
		return c.verifySubscription(ctx, productID, purchaseToken)
	case domain.PurchaseTypeOneTime:
		return c.verifyOneTime(ctx, productID, purchaseToken)
	default:
		return nil, domain.ErrInvalidPurchase
	}
}

func (c *Client) AcknowledgePurchase(ctx context.Context, productID, purchaseToken string, purchaseType domain.PurchaseType) error {
	switch purchaseType {
	case domain.PurchaseTypeSubscription:
		req := &androidpublisher.SubscriptionPurchasesAcknowledgeRequest{}
		return c.service.Purchases.Subscriptions.Acknowledge(c.packageName, productID, purchaseToken, req).Context(ctx).Do()
	case domain.PurchaseTypeOneTime:
		req := &androidpublisher.ProductPurchasesAcknowledgeRequest{}
		return c.service.Purchases.Products.Acknowledge(c.packageName, productID, purchaseToken, req).Context(ctx).Do()
	default:
		return domain.ErrInvalidPurchase
	}
}

func (c *Client) verifySubscription(ctx context.Context, productID, purchaseToken string) (*domain.VerifiedPurchase, error) {
	purchase, err := c.service.Purchases.Subscriptionsv2.Get(c.packageName, purchaseToken).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("failed to get subscription purchase: %w", err)
	}

	expiresAt := latestSubscriptionExpiry(purchase)
	verifiedProductID := productID
	orderID := ""
	if len(purchase.LineItems) > 0 {
		verifiedProductID = purchase.LineItems[0].ProductId
		orderID = purchase.LineItems[0].LatestSuccessfulOrderId
	}

	obfuscatedID := ""
	if purchase.ExternalAccountIdentifiers != nil {
		obfuscatedID = purchase.ExternalAccountIdentifiers.ObfuscatedExternalAccountId
	}

	return &domain.VerifiedPurchase{
		ProductID:            verifiedProductID,
		PurchaseToken:        purchaseToken,
		Type:                 domain.PurchaseTypeSubscription,
		Status:               subscriptionStatus(purchase.SubscriptionState, expiresAt),
		OrderID:              orderID,
		ObfuscatedID:         obfuscatedID,
		ExpiresAt:            expiresAt,
		NeedsAcknowledgement: purchase.AcknowledgementState == "ACKNOWLEDGEMENT_STATE_PENDING",
	}, nil
}

func (c *Client) verifyOneTime(ctx context.Context, productID, purchaseToken string) (*domain.VerifiedPurchase, error) {
	purchase, err := c.service.Purchases.Productsv2.Getproductpurchasev2(c.packageName, purchaseToken).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("failed to get product purchase: %w", err)
	}

	verifiedProductID := productID
	if len(purchase.ProductLineItem) > 0 {
		verifiedProductID = purchase.ProductLineItem[0].ProductId
	}

	status := domain.PurchaseStatusUnknown
	if purchase.PurchaseStateContext != nil {
		status = productStatus(purchase.PurchaseStateContext.PurchaseState)
	}

	return &domain.VerifiedPurchase{
		ProductID:            verifiedProductID,
		PurchaseToken:        purchaseToken,
		Type:                 domain.PurchaseTypeOneTime,
		Status:               status,
		OrderID:              purchase.OrderId,
		ObfuscatedID:         purchase.ObfuscatedExternalAccountId,
		NeedsAcknowledgement: purchase.AcknowledgementState == "ACKNOWLEDGEMENT_STATE_PENDING",
	}, nil
}

func latestSubscriptionExpiry(purchase *androidpublisher.SubscriptionPurchaseV2) *time.Time {
	var latest *time.Time
	for _, item := range purchase.LineItems {
		if item.ExpiryTime == "" {
			continue
		}
		parsed, err := time.Parse(time.RFC3339, item.ExpiryTime)
		if err != nil {
			continue
		}
		if latest == nil || parsed.After(*latest) {
			latest = &parsed
		}
	}
	return latest
}

func subscriptionStatus(state string, expiresAt *time.Time) domain.PurchaseStatus {
	switch state {
	case "SUBSCRIPTION_STATE_ACTIVE", "SUBSCRIPTION_STATE_IN_GRACE_PERIOD":
		if expiresAt != nil && expiresAt.Before(time.Now().UTC()) {
			return domain.PurchaseStatusExpired
		}
		return domain.PurchaseStatusActive
	case "SUBSCRIPTION_STATE_PENDING":
		return domain.PurchaseStatusPending
	case "SUBSCRIPTION_STATE_CANCELED", "SUBSCRIPTION_STATE_PENDING_PURCHASE_CANCELED":
		return domain.PurchaseStatusCanceled
	case "SUBSCRIPTION_STATE_EXPIRED", "SUBSCRIPTION_STATE_ON_HOLD", "SUBSCRIPTION_STATE_PAUSED":
		return domain.PurchaseStatusExpired
	default:
		return domain.PurchaseStatusUnknown
	}
}

func productStatus(state string) domain.PurchaseStatus {
	switch state {
	case "PURCHASED":
		return domain.PurchaseStatusActive
	case "PENDING":
		return domain.PurchaseStatusPending
	case "CANCELLED":
		return domain.PurchaseStatusCanceled
	default:
		return domain.PurchaseStatusUnknown
	}
}
