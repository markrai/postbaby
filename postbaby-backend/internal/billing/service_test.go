package billing

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"postbaby-backend/internal/store"
)

func TestServiceReportsUnavailableWithoutProvider(t *testing.T) {
	t.Parallel()

	sqliteStore := openBillingTestStore(t)
	service := NewServiceWithProvider(sqliteStore, nil, "http://127.0.0.1:8080")

	if service.Available() {
		t.Fatal("expected billing service to be unavailable without provider")
	}
}

func TestServiceCheckoutCreatesBillingCustomerMapping(t *testing.T) {
	t.Parallel()

	sqliteStore := openBillingTestStore(t)
	user := createBillingTestUser(t, sqliteStore, "checkout-user")
	provider := &fakeProvider{
		available:        true,
		name:             "stripe",
		createCustomerID: "cus_checkout",
		checkoutURL:      "https://checkout.stripe.test/session",
	}
	service := NewServiceWithProvider(sqliteStore, provider, "http://127.0.0.1:8080")

	redirectURL, err := service.CreateCheckoutSession(context.Background(), &user)
	if err != nil {
		t.Fatalf("create checkout session: %v", err)
	}

	if redirectURL != "https://checkout.stripe.test/session" {
		t.Fatalf("expected checkout URL, got %q", redirectURL)
	}
	if provider.createCustomerCalls != 1 {
		t.Fatalf("expected create customer call, got %d", provider.createCustomerCalls)
	}
	if provider.checkoutInput == nil || provider.checkoutInput.ProviderCustomerID != "cus_checkout" {
		t.Fatalf("unexpected checkout input: %+v", provider.checkoutInput)
	}

	customer, err := sqliteStore.GetBillingCustomer(context.Background(), user.ID, "stripe")
	if err != nil {
		t.Fatalf("load billing customer: %v", err)
	}
	if customer.ProviderCustomerID != "cus_checkout" {
		t.Fatalf("unexpected billing customer mapping: %+v", customer)
	}
}

func TestServicePortalRequiresExistingCustomer(t *testing.T) {
	t.Parallel()

	sqliteStore := openBillingTestStore(t)
	user := createBillingTestUser(t, sqliteStore, "portal-user")
	service := NewServiceWithProvider(sqliteStore, &fakeProvider{
		available: true,
		name:      "stripe",
	}, "http://127.0.0.1:8080")

	if _, err := service.CreatePortalSession(context.Background(), &user); !errors.Is(err, ErrBillingCustomerNotFound) {
		t.Fatalf("expected billing customer not found, got %v", err)
	}
}

func TestServiceHandleWebhookUpdatesEntitlementAndMappings(t *testing.T) {
	t.Parallel()

	sqliteStore := openBillingTestStore(t)
	user := createBillingTestUser(t, sqliteStore, "webhook-user")
	service := NewServiceWithProvider(sqliteStore, &fakeProvider{
		available: true,
		name:      "stripe",
		webhookEvent: WebhookEvent{
			ID:                     "evt_1",
			Type:                   "customer.subscription.updated",
			UserID:                 user.ID,
			ProviderCustomerID:     "cus_webhook",
			ProviderSubscriptionID: "sub_webhook",
			Status:                 "active",
			ValidUntil:             stripeTimestampPointer(time.Date(2026, time.May, 20, 12, 0, 0, 0, time.UTC).Unix()),
		},
	}, "http://127.0.0.1:8080")

	if err := service.HandleWebhook(context.Background(), []byte(`{}`), "sig"); err != nil {
		t.Fatalf("handle webhook: %v", err)
	}

	customer, err := sqliteStore.GetBillingCustomer(context.Background(), user.ID, "stripe")
	if err != nil {
		t.Fatalf("load billing customer: %v", err)
	}
	if customer.ProviderCustomerID != "cus_webhook" {
		t.Fatalf("unexpected billing customer: %+v", customer)
	}

	subscription, err := sqliteStore.GetBillingSubscriptionByProviderSubscriptionID(context.Background(), "stripe", "sub_webhook")
	if err != nil {
		t.Fatalf("load billing subscription: %v", err)
	}
	if subscription.Status != store.EntitlementStatusActive {
		t.Fatalf("expected active billing subscription status, got %+v", subscription)
	}

	entitlement, err := sqliteStore.GetAccountEntitlement(context.Background(), user.ID, store.EntitlementKeyHostedSync)
	if err != nil {
		t.Fatalf("load entitlement: %v", err)
	}
	if entitlement.Status != store.EntitlementStatusActive || entitlement.Source != store.EntitlementSourceStripe {
		t.Fatalf("unexpected entitlement: %+v", entitlement)
	}
}

func TestServiceHandleWebhookIsIdempotentEnoughForRepeatedEvents(t *testing.T) {
	t.Parallel()

	sqliteStore := openBillingTestStore(t)
	user := createBillingTestUser(t, sqliteStore, "repeat-webhook-user")
	provider := &fakeProvider{
		available: true,
		name:      "stripe",
		webhookEvent: WebhookEvent{
			ID:                     "evt_repeat",
			Type:                   "customer.subscription.updated",
			UserID:                 user.ID,
			ProviderCustomerID:     "cus_repeat",
			ProviderSubscriptionID: "sub_repeat",
			Status:                 "past_due",
		},
	}
	service := NewServiceWithProvider(sqliteStore, provider, "http://127.0.0.1:8080")

	if err := service.HandleWebhook(context.Background(), []byte(`{}`), "sig"); err != nil {
		t.Fatalf("first handle webhook: %v", err)
	}
	if err := service.HandleWebhook(context.Background(), []byte(`{}`), "sig"); err != nil {
		t.Fatalf("second handle webhook: %v", err)
	}

	entitlement, err := sqliteStore.GetAccountEntitlement(context.Background(), user.ID, store.EntitlementKeyHostedSync)
	if err != nil {
		t.Fatalf("load entitlement: %v", err)
	}
	if entitlement.Status != store.EntitlementStatusPastDue {
		t.Fatalf("expected past_due entitlement, got %+v", entitlement)
	}
}

type fakeProvider struct {
	available           bool
	name                string
	createCustomerID    string
	checkoutURL         string
	portalURL           string
	webhookEvent        WebhookEvent
	webhookErr          error
	createCustomerCalls int
	checkoutInput       *CheckoutSessionInput
	portalInput         *PortalSessionInput
}

func (p *fakeProvider) Name() string {
	return p.name
}

func (p *fakeProvider) Available() bool {
	return p.available
}

func (p *fakeProvider) CreateCustomer(ctx context.Context, user *store.User) (string, error) {
	p.createCustomerCalls += 1
	return p.createCustomerID, nil
}

func (p *fakeProvider) CreateCheckoutSession(ctx context.Context, input CheckoutSessionInput) (string, error) {
	copied := input
	p.checkoutInput = &copied
	return p.checkoutURL, nil
}

func (p *fakeProvider) CreatePortalSession(ctx context.Context, input PortalSessionInput) (string, error) {
	copied := input
	p.portalInput = &copied
	return p.portalURL, nil
}

func (p *fakeProvider) ParseWebhook(ctx context.Context, rawBody []byte, signatureHeader string) (WebhookEvent, error) {
	if p.webhookErr != nil {
		return WebhookEvent{}, p.webhookErr
	}
	return p.webhookEvent, nil
}

func openBillingTestStore(t *testing.T) *store.Store {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "postbaby-billing-test.db")
	sqliteStore, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open test store: %v", err)
	}

	t.Cleanup(func() {
		if err := sqliteStore.Close(); err != nil {
			t.Fatalf("close test store: %v", err)
		}
	})

	return sqliteStore
}

func createBillingTestUser(t *testing.T, sqliteStore *store.Store, username string) store.User {
	t.Helper()

	user, err := sqliteStore.CreateUser(context.Background(), username, "argon-hash", username+"-owner")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	return user
}
