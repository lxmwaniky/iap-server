package domain

import (
	"context"
	"errors"
)

var (
	ErrInvalidPurchase            = errors.New("invalid purchase")
	ErrPurchaseOwnedByAnotherUser = errors.New("purchase token is owned by another user")
)

type PurchaseRepository interface {
	SaveVerifiedPurchase(ctx context.Context, purchase *Purchase, entitlement *Entitlement) error
	FindPurchaseByToken(ctx context.Context, purchaseToken string) (*Purchase, error)
	FindEntitlement(ctx context.Context, userID, productID string) (*Entitlement, error)
	RecordNotification(ctx context.Context, notification *PlayNotification) (bool, error)
	Ping(ctx context.Context) error
}
