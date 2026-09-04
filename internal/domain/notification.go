package domain

import "time"

type PlayNotification struct {
	MessageID     string    `json:"message_id"`
	PackageName   string    `json:"package_name"`
	EventType     string    `json:"event_type"`
	ProductID     string    `json:"product_id,omitempty"`
	PurchaseToken string    `json:"purchase_token"`
	ReceivedAt    time.Time `json:"received_at"`
}
