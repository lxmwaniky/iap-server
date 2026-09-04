package domain

import "time"

type User struct {
	ID string
}

type AnonymousSession struct {
	UserID    string    `json:"user_id"`
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

type PurchaseType string

const (
	PurchaseTypeOneTime      PurchaseType = "one_time"
	PurchaseTypeSubscription PurchaseType = "subscription"
)

type PurchaseStatus string

const (
	PurchaseStatusPending  PurchaseStatus = "PENDING"
	PurchaseStatusActive   PurchaseStatus = "ACTIVE"
	PurchaseStatusCanceled PurchaseStatus = "CANCELED"
	PurchaseStatusExpired  PurchaseStatus = "EXPIRED"
	PurchaseStatusRevoked  PurchaseStatus = "REVOKED"
	PurchaseStatusUnknown  PurchaseStatus = "UNKNOWN"
)

type EntitlementStatus string

const (
	EntitlementStatusActive   EntitlementStatus = "ACTIVE"
	EntitlementStatusInactive EntitlementStatus = "INACTIVE"
)

type Purchase struct {
	ID             int64          `json:"id"`
	UserID         string         `json:"user_id"`
	ProductID      string         `json:"product_id"`
	PurchaseToken  string         `json:"purchase_token"`
	Type           PurchaseType   `json:"type"`
	Status         PurchaseStatus `json:"status"`
	OrderID        string         `json:"order_id,omitempty"`
	ObfuscatedID   string         `json:"obfuscated_account_id,omitempty"`
	ExpiresAt      *time.Time     `json:"expires_at,omitempty"`
	AcknowledgedAt *time.Time     `json:"acknowledged_at,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

type Entitlement struct {
	ID        int64             `json:"id"`
	UserID    string            `json:"user_id"`
	ProductID string            `json:"product_id"`
	Status    EntitlementStatus `json:"status"`
	Source    PurchaseType      `json:"source"`
	ExpiresAt *time.Time        `json:"expires_at,omitempty"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
}

type VerifiedPurchase struct {
	ProductID            string
	PurchaseToken        string
	Type                 PurchaseType
	Status               PurchaseStatus
	OrderID              string
	ObfuscatedID         string
	ExpiresAt            *time.Time
	NeedsAcknowledgement bool
}

type VerifyPurchaseRequest struct {
	UserID        string
	ProductID     string
	PurchaseToken string
	Type          PurchaseType
}

type VerifyPurchaseResult struct {
	Entitled  bool           `json:"entitled"`
	Status    PurchaseStatus `json:"status"`
	ProductID string         `json:"product_id"`
	ExpiresAt *time.Time     `json:"expires_at,omitempty"`
}
