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
	ErrEntitlementNotFound = errors.New("entitlement not found")
)

const (
	EntitlementKeyHostedSync = "hosted_sync"

	EntitlementStatusNone     = "none"
	EntitlementStatusActive   = "active"
	EntitlementStatusPastDue  = "past_due"
	EntitlementStatusCanceled = "canceled"
	EntitlementStatusExpired  = "expired"

	EntitlementSourceStripe = "stripe"
	EntitlementSourceManual = "manual"
	EntitlementSourceAdmin  = "admin"
)

type AccountEntitlement struct {
	UserID         int64
	EntitlementKey string
	Status         string
	Source         string
	ValidUntil     *time.Time
	UpdatedAt      time.Time
}

func (s *Store) GetAccountEntitlement(ctx context.Context, userID int64, entitlementKey string) (AccountEntitlement, error) {
	started := time.Now()
	ctx, cancel := withDBTimeout(ctx)
	defer cancel()

	row := s.db.QueryRowContext(
		ctx,
		`SELECT user_id, entitlement_key, status, source, valid_until, updated_at
		FROM account_entitlements
		WHERE user_id = ? AND entitlement_key = ?`,
		userID,
		entitlementKey,
	)

	entitlement, err := scanAccountEntitlement(row)
	if err != nil {
		if errors.Is(err, ErrEntitlementNotFound) {
			return AccountEntitlement{}, err
		}
		return AccountEntitlement{}, s.wrapDBError("read_account_entitlement", started, err)
	}

	s.logDBOperation("read_account_entitlement", started, nil)
	return entitlement, nil
}

func (s *Store) PutAccountEntitlement(ctx context.Context, userID int64, entitlementKey, status, source string, validUntil *time.Time) (AccountEntitlement, error) {
	started := time.Now()
	ctx, cancel := withDBTimeout(ctx)
	defer cancel()

	updatedAt := time.Now().UTC()
	var validUntilValue any
	if validUntil != nil {
		validUntilUTC := validUntil.UTC().Format(time.RFC3339)
		validUntilValue = validUntilUTC
	}

	if _, err := s.db.ExecContext(
		ctx,
		`INSERT INTO account_entitlements (user_id, entitlement_key, status, source, valid_until, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(user_id, entitlement_key) DO UPDATE SET
			status = excluded.status,
			source = excluded.source,
			valid_until = excluded.valid_until,
			updated_at = excluded.updated_at`,
		userID,
		entitlementKey,
		status,
		source,
		validUntilValue,
		updatedAt.Format(time.RFC3339),
	); err != nil {
		return AccountEntitlement{}, s.wrapDBError("write_account_entitlement", started, fmt.Errorf("write account entitlement: %w", err))
	}

	s.logDBOperation("write_account_entitlement", started, nil)
	return AccountEntitlement{
		UserID:         userID,
		EntitlementKey: entitlementKey,
		Status:         status,
		Source:         source,
		ValidUntil:     cloneTimePointer(validUntil),
		UpdatedAt:      updatedAt,
	}, nil
}

func scanAccountEntitlement(row interface {
	Scan(dest ...any) error
}) (AccountEntitlement, error) {
	var entitlement AccountEntitlement
	var validUntil sql.NullString
	var updatedAt string
	if err := row.Scan(
		&entitlement.UserID,
		&entitlement.EntitlementKey,
		&entitlement.Status,
		&entitlement.Source,
		&validUntil,
		&updatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return AccountEntitlement{}, ErrEntitlementNotFound
		}
		return AccountEntitlement{}, fmt.Errorf("scan account entitlement: %w", err)
	}

	entitlement.UpdatedAt = mustParseTimestamp(updatedAt)
	if validUntil.Valid && strings.TrimSpace(validUntil.String) != "" {
		parsed := mustParseTimestamp(validUntil.String)
		entitlement.ValidUntil = &parsed
	}

	return entitlement, nil
}

func cloneTimePointer(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}

	copied := value.UTC()
	return &copied
}
