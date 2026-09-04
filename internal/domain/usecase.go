package domain

import "context"

type IAPUsecase interface {
	VerifyPurchase(ctx context.Context, req VerifyPurchaseRequest) (*VerifyPurchaseResult, error)
	GetEntitlement(ctx context.Context, userID, productID string) (*Entitlement, error)
	ProcessNotification(ctx context.Context, notification *PlayNotification) error
	Ping(ctx context.Context) error
}
