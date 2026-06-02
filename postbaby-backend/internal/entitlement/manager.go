package entitlement

import (
	"context"
	"errors"

	"postbaby-backend/internal/store"
)

type Manager struct {
	store store.EntitlementStore
}

func NewManager(entitlementStore store.EntitlementStore) *Manager {
	return &Manager{
		store: entitlementStore,
	}
}

func (m *Manager) HostedSyncGranted(ctx context.Context, userID int64) (bool, error) {
	entitlement, err := m.store.GetAccountEntitlement(ctx, userID, store.EntitlementKeyHostedSync)
	if err != nil {
		if errors.Is(err, store.ErrEntitlementNotFound) {
			return false, nil
		}
		return false, err
	}

	return entitlement.Status == store.EntitlementStatusActive, nil
}

func (m *Manager) HostedSyncEntitlement(ctx context.Context, userID int64) (store.AccountEntitlement, bool, error) {
	entitlement, err := m.store.GetAccountEntitlement(ctx, userID, store.EntitlementKeyHostedSync)
	if err != nil {
		if errors.Is(err, store.ErrEntitlementNotFound) {
			return store.AccountEntitlement{}, false, nil
		}
		return store.AccountEntitlement{}, false, err
	}

	return entitlement, true, nil
}
