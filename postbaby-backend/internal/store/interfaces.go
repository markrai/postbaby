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
	GetUserByUsername(ctx context.Context, username string) (User, error)
	CreateSession(ctx context.Context, userID int64, tokenHash string, expiresAt time.Time) (Session, error)
	GetSessionUserByTokenHash(ctx context.Context, tokenHash string) (SessionUser, error)
	DeleteSessionByTokenHash(ctx context.Context, tokenHash string) error
	TouchSession(ctx context.Context, sessionID int64, lastSeenAt time.Time) error
}

var (
	_ DocumentStore = (*Store)(nil)
	_ IdentityStore = (*Store)(nil)
)
