package store

import (
	"context"
	"encoding/json"
	"time"
)

type DocumentStore interface {
	Health(ctx context.Context) error
	GetDocument(ctx context.Context, ownerKey, appID string) (Document, error)
	GetDocumentMeta(ctx context.Context, ownerKey, appID string) (DocumentMeta, error)
	PutDocument(ctx context.Context, ownerKey, appID string, body json.RawMessage, expectedVersion *int64) (Document, error)
}

type IdentityStore interface {
	UsersExist(ctx context.Context) (bool, error)
	BootstrapOwnerKey(ctx context.Context) (string, error)
	CreateInitialUser(ctx context.Context, username, passwordHash, ownerKey string) (User, error)
	CreateUser(ctx context.Context, username, passwordHash, ownerKey string) (User, error)
	CreateProvisionalUser(ctx context.Context, username, passwordHash, ownerKey string, checkoutExpiresAt time.Time) (User, error)
	ActivateUser(ctx context.Context, userID int64) error
	DeleteExpiredProvisionalUsers(ctx context.Context, now time.Time) (int64, error)
	GetUserByUsername(ctx context.Context, username string) (User, error)
	CreateSession(ctx context.Context, userID int64, tokenHash string, expiresAt time.Time) (Session, error)
	GetSessionUserByTokenHash(ctx context.Context, tokenHash string) (SessionUser, error)
	DeleteSessionByTokenHash(ctx context.Context, tokenHash string) error
	TouchSession(ctx context.Context, sessionID int64, lastSeenAt time.Time) error
}

type EntitlementStore interface {
	GetAccountEntitlement(ctx context.Context, userID int64, entitlementKey string) (AccountEntitlement, error)
	PutAccountEntitlement(ctx context.Context, userID int64, entitlementKey, status, source string, validUntil *time.Time) (AccountEntitlement, error)
}

type BillingStore interface {
	EntitlementStore
	IdentityStore
	GetBillingCustomer(ctx context.Context, userID int64, provider string) (BillingCustomer, error)
	GetBillingCustomerByProviderCustomerID(ctx context.Context, provider, providerCustomerID string) (BillingCustomer, error)
	PutBillingCustomer(ctx context.Context, userID int64, provider, providerCustomerID string) (BillingCustomer, error)
	GetBillingSubscriptionByProviderSubscriptionID(ctx context.Context, provider, providerSubscriptionID string) (BillingSubscription, error)
	PutBillingSubscription(ctx context.Context, userID int64, provider, providerSubscriptionID, status string, validUntil *time.Time) (BillingSubscription, error)
}

var (
	_ DocumentStore    = (*Store)(nil)
	_ IdentityStore    = (*Store)(nil)
	_ EntitlementStore = (*Store)(nil)
	_ BillingStore     = (*Store)(nil)
)
