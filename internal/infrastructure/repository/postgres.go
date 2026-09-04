package repository

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/lxmwaniky/iap-server/internal/domain"
)

//go:embed schema.sql
var schemaSQL string

type PostgresDB struct {
	DB *sql.DB
}

func NewPostgresDB(databaseURL string) (*PostgresDB, error) {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to open postgres connection: %w", err)
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(25)
	db.SetConnMaxLifetime(5 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to ping postgres: %w", err)
	}
	if _, err := db.ExecContext(ctx, schemaSQL); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to apply postgres schema: %w", err)
	}

	return &PostgresDB{DB: db}, nil
}

func (p *PostgresDB) Close() error {
	return p.DB.Close()
}

type postgresPurchaseRepository struct {
	db *sql.DB
}

func NewPostgresPurchaseRepository(db *sql.DB) domain.PurchaseRepository {
	return &postgresPurchaseRepository{db: db}
}

func (r *postgresPurchaseRepository) SaveVerifiedPurchase(ctx context.Context, purchase *domain.Purchase, entitlement *domain.Entitlement) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin purchase transaction: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	purchaseQuery := `
		INSERT INTO purchases (user_id, product_id, purchase_token, purchase_type, status, order_id, obfuscated_account_id, expires_at, acknowledged_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (purchase_token) DO UPDATE SET
			user_id = EXCLUDED.user_id,
			product_id = EXCLUDED.product_id,
			purchase_type = EXCLUDED.purchase_type,
			status = EXCLUDED.status,
			order_id = EXCLUDED.order_id,
			obfuscated_account_id = EXCLUDED.obfuscated_account_id,
			expires_at = EXCLUDED.expires_at,
			acknowledged_at = COALESCE(purchases.acknowledged_at, EXCLUDED.acknowledged_at),
			updated_at = EXCLUDED.updated_at
		WHERE purchases.user_id = EXCLUDED.user_id
		RETURNING id
	`
	err = tx.QueryRowContext(ctx, purchaseQuery,
		purchase.UserID,
		purchase.ProductID,
		purchase.PurchaseToken,
		purchase.Type,
		purchase.Status,
		nullString(purchase.OrderID),
		nullString(purchase.ObfuscatedID),
		purchase.ExpiresAt,
		purchase.AcknowledgedAt,
		purchase.CreatedAt,
		purchase.UpdatedAt,
	).Scan(&purchase.ID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.ErrPurchaseOwnedByAnotherUser
		}
		return fmt.Errorf("failed to upsert purchase: %w", err)
	}

	if entitlement != nil {
		entitlementQuery := `
			INSERT INTO entitlements (user_id, product_id, status, source, expires_at, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			ON CONFLICT (user_id, product_id) DO UPDATE SET
				status = EXCLUDED.status,
				source = EXCLUDED.source,
				expires_at = EXCLUDED.expires_at,
				updated_at = EXCLUDED.updated_at
			RETURNING id, created_at
		`
		err = tx.QueryRowContext(ctx, entitlementQuery,
			entitlement.UserID,
			entitlement.ProductID,
			entitlement.Status,
			entitlement.Source,
			entitlement.ExpiresAt,
			entitlement.CreatedAt,
			entitlement.UpdatedAt,
		).Scan(&entitlement.ID, &entitlement.CreatedAt)
		if err != nil {
			return fmt.Errorf("failed to upsert entitlement: %w", err)
		}
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit purchase transaction: %w", err)
	}
	return nil
}

func (r *postgresPurchaseRepository) FindPurchaseByToken(ctx context.Context, purchaseToken string) (*domain.Purchase, error) {
	query := `
		SELECT id, user_id, product_id, purchase_token, purchase_type, status, COALESCE(order_id, ''), COALESCE(obfuscated_account_id, ''), expires_at, acknowledged_at, created_at, updated_at
		FROM purchases
		WHERE purchase_token = $1
	`
	var purchase domain.Purchase
	err := r.db.QueryRowContext(ctx, query, purchaseToken).Scan(
		&purchase.ID,
		&purchase.UserID,
		&purchase.ProductID,
		&purchase.PurchaseToken,
		&purchase.Type,
		&purchase.Status,
		&purchase.OrderID,
		&purchase.ObfuscatedID,
		&purchase.ExpiresAt,
		&purchase.AcknowledgedAt,
		&purchase.CreatedAt,
		&purchase.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to find purchase by token: %w", err)
	}
	return &purchase, nil
}

func (r *postgresPurchaseRepository) FindEntitlement(ctx context.Context, userID, productID string) (*domain.Entitlement, error) {
	query := `
		SELECT id, user_id, product_id, status, source, expires_at, created_at, updated_at
		FROM entitlements
		WHERE user_id = $1 AND product_id = $2
	`
	var entitlement domain.Entitlement
	err := r.db.QueryRowContext(ctx, query, userID, productID).Scan(
		&entitlement.ID,
		&entitlement.UserID,
		&entitlement.ProductID,
		&entitlement.Status,
		&entitlement.Source,
		&entitlement.ExpiresAt,
		&entitlement.CreatedAt,
		&entitlement.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to find entitlement: %w", err)
	}
	return &entitlement, nil
}

func (r *postgresPurchaseRepository) RecordNotification(ctx context.Context, notification *domain.PlayNotification) (bool, error) {
	query := `
		INSERT INTO play_notifications (message_id, package_name, event_type, product_id, purchase_token, received_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (message_id) DO NOTHING
	`
	result, err := r.db.ExecContext(ctx, query,
		notification.MessageID,
		notification.PackageName,
		notification.EventType,
		nullString(notification.ProductID),
		notification.PurchaseToken,
		notification.ReceivedAt,
	)
	if err != nil {
		return false, fmt.Errorf("failed to record play notification: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("failed to read notification insert result: %w", err)
	}
	return rows == 1, nil
}

func (r *postgresPurchaseRepository) Ping(ctx context.Context) error {
	return r.db.PingContext(ctx)
}

func nullString(value string) sql.NullString {
	if value == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: value, Valid: true}
}
