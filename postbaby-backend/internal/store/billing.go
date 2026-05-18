package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrBillingCustomerNotFound     = errors.New("billing customer not found")
	ErrBillingSubscriptionNotFound = errors.New("billing subscription not found")
)

type BillingCustomer struct {
	UserID             int64
	Provider           string
	ProviderCustomerID string
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type BillingSubscription struct {
	UserID                 int64
	Provider               string
	ProviderSubscriptionID string
	Status                 string
	ValidUntil             *time.Time
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

func (s *Store) GetBillingCustomer(ctx context.Context, userID int64, provider string) (BillingCustomer, error) {
	started := time.Now()
	ctx, cancel := withDBTimeout(ctx)
	defer cancel()

	row := s.db.QueryRowContext(
		ctx,
		`SELECT user_id, provider, provider_customer_id, created_at, updated_at
		FROM billing_customers
		WHERE user_id = ? AND provider = ?`,
		userID,
		provider,
	)

	customer, err := scanBillingCustomer(row)
	if err != nil {
		if errors.Is(err, ErrBillingCustomerNotFound) {
			return BillingCustomer{}, err
		}
		return BillingCustomer{}, s.wrapDBError("read_billing_customer", started, err)
	}

	s.logDBOperation("read_billing_customer", started, nil)
	return customer, nil
}

func (s *Store) GetBillingCustomerByProviderCustomerID(ctx context.Context, provider, providerCustomerID string) (BillingCustomer, error) {
	started := time.Now()
	ctx, cancel := withDBTimeout(ctx)
	defer cancel()

	row := s.db.QueryRowContext(
		ctx,
		`SELECT user_id, provider, provider_customer_id, created_at, updated_at
		FROM billing_customers
		WHERE provider = ? AND provider_customer_id = ?`,
		provider,
		providerCustomerID,
	)

	customer, err := scanBillingCustomer(row)
	if err != nil {
		if errors.Is(err, ErrBillingCustomerNotFound) {
			return BillingCustomer{}, err
		}
		return BillingCustomer{}, s.wrapDBError("read_billing_customer_by_provider_id", started, err)
	}

	s.logDBOperation("read_billing_customer_by_provider_id", started, nil)
	return customer, nil
}

func (s *Store) PutBillingCustomer(ctx context.Context, userID int64, provider, providerCustomerID string) (BillingCustomer, error) {
	started := time.Now()
	ctx, cancel := withDBTimeout(ctx)
	defer cancel()

	timestamp := time.Now().UTC().Format(time.RFC3339)
	if _, err := s.db.ExecContext(
		ctx,
		`INSERT INTO billing_customers (user_id, provider, provider_customer_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(user_id, provider) DO UPDATE SET
			provider_customer_id = excluded.provider_customer_id,
			updated_at = excluded.updated_at`,
		userID,
		provider,
		providerCustomerID,
		timestamp,
		timestamp,
	); err != nil {
		return BillingCustomer{}, s.wrapDBError("write_billing_customer", started, fmt.Errorf("write billing customer: %w", err))
	}

	customer, err := s.GetBillingCustomer(ctx, userID, provider)
	if err != nil {
		return BillingCustomer{}, err
	}

	s.logDBOperation("write_billing_customer", started, nil)
	return customer, nil
}

func (s *Store) GetBillingSubscriptionByProviderSubscriptionID(ctx context.Context, provider, providerSubscriptionID string) (BillingSubscription, error) {
	started := time.Now()
	ctx, cancel := withDBTimeout(ctx)
	defer cancel()

	row := s.db.QueryRowContext(
		ctx,
		`SELECT user_id, provider, provider_subscription_id, status, valid_until, created_at, updated_at
		FROM billing_subscriptions
		WHERE provider = ? AND provider_subscription_id = ?`,
		provider,
		providerSubscriptionID,
	)

	subscription, err := scanBillingSubscription(row)
	if err != nil {
		if errors.Is(err, ErrBillingSubscriptionNotFound) {
			return BillingSubscription{}, err
		}
		return BillingSubscription{}, s.wrapDBError("read_billing_subscription_by_provider_id", started, err)
	}

	s.logDBOperation("read_billing_subscription_by_provider_id", started, nil)
	return subscription, nil
}

func (s *Store) PutBillingSubscription(ctx context.Context, userID int64, provider, providerSubscriptionID, status string, validUntil *time.Time) (BillingSubscription, error) {
	started := time.Now()
	ctx, cancel := withDBTimeout(ctx)
	defer cancel()

	timestamp := time.Now().UTC().Format(time.RFC3339)
	var validUntilValue any
	if validUntil != nil {
		validUntilValue = validUntil.UTC().Format(time.RFC3339)
	}

	if _, err := s.db.ExecContext(
		ctx,
		`INSERT INTO billing_subscriptions (user_id, provider, provider_subscription_id, status, valid_until, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(provider, provider_subscription_id) DO UPDATE SET
			user_id = excluded.user_id,
			status = excluded.status,
			valid_until = excluded.valid_until,
			updated_at = excluded.updated_at`,
		userID,
		provider,
		providerSubscriptionID,
		status,
		validUntilValue,
		timestamp,
		timestamp,
	); err != nil {
		return BillingSubscription{}, s.wrapDBError("write_billing_subscription", started, fmt.Errorf("write billing subscription: %w", err))
	}

	subscription, err := s.GetBillingSubscriptionByProviderSubscriptionID(ctx, provider, providerSubscriptionID)
	if err != nil {
		return BillingSubscription{}, err
	}

	s.logDBOperation("write_billing_subscription", started, nil)
	return subscription, nil
}

func scanBillingCustomer(row interface {
	Scan(dest ...any) error
}) (BillingCustomer, error) {
	var customer BillingCustomer
	var createdAt string
	var updatedAt string
	if err := row.Scan(
		&customer.UserID,
		&customer.Provider,
		&customer.ProviderCustomerID,
		&createdAt,
		&updatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return BillingCustomer{}, ErrBillingCustomerNotFound
		}
		return BillingCustomer{}, fmt.Errorf("scan billing customer: %w", err)
	}

	customer.CreatedAt = mustParseTimestamp(createdAt)
	customer.UpdatedAt = mustParseTimestamp(updatedAt)
	return customer, nil
}

func scanBillingSubscription(row interface {
	Scan(dest ...any) error
}) (BillingSubscription, error) {
	var subscription BillingSubscription
	var validUntil sql.NullString
	var createdAt string
	var updatedAt string
	if err := row.Scan(
		&subscription.UserID,
		&subscription.Provider,
		&subscription.ProviderSubscriptionID,
		&subscription.Status,
		&validUntil,
		&createdAt,
		&updatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return BillingSubscription{}, ErrBillingSubscriptionNotFound
		}
		return BillingSubscription{}, fmt.Errorf("scan billing subscription: %w", err)
	}

	if validUntil.Valid && strings.TrimSpace(validUntil.String) != "" {
		parsed := mustParseTimestamp(validUntil.String)
		subscription.ValidUntil = &parsed
	}
	subscription.CreatedAt = mustParseTimestamp(createdAt)
	subscription.UpdatedAt = mustParseTimestamp(updatedAt)
	return subscription, nil
}
