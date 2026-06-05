package billing

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"strconv"
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
		name:             "portablepay",
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

	customer, err := sqliteStore.GetBillingCustomer(context.Background(), user.ID, "portablepay")
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
		name:      "portablepay",
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
		name:      "portablepay",
		webhookEvent: WebhookEvent{
			Kind:                   WebhookEventKindSubscriptionStateChanged,
			ID:                     "evt_1",
			RawType:                "portable.subscription.updated",
			UserID:                 user.ID,
			ProviderCustomerID:     "cus_webhook",
			ProviderSubscriptionID: "sub_webhook",
			EntitlementStatus:      store.EntitlementStatusActive,
			ValidUntil:             stripeTimestampPointer(time.Date(2026, time.May, 20, 12, 0, 0, 0, time.UTC).Unix()),
		},
	}, "http://127.0.0.1:8080")

	if err := service.HandleWebhook(context.Background(), []byte(`{}`), http.Header{"X-Provider-Signature": []string{"sig"}}); err != nil {
		t.Fatalf("handle webhook: %v", err)
	}

	customer, err := sqliteStore.GetBillingCustomer(context.Background(), user.ID, "portablepay")
	if err != nil {
		t.Fatalf("load billing customer: %v", err)
	}
	if customer.ProviderCustomerID != "cus_webhook" {
		t.Fatalf("unexpected billing customer: %+v", customer)
	}

	subscription, err := sqliteStore.GetBillingSubscriptionByProviderSubscriptionID(context.Background(), "portablepay", "sub_webhook")
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
	if entitlement.Status != store.EntitlementStatusActive || entitlement.Source != "portablepay" {
		t.Fatalf("unexpected entitlement: %+v", entitlement)
	}
}

func TestServiceHandleActiveSubscriptionActivatesProvisionalUser(t *testing.T) {
	t.Parallel()

	sqliteStore := openBillingTestStore(t)
	expiresAt := time.Now().UTC().Add(time.Hour)
	user, err := sqliteStore.CreateProvisionalUser(context.Background(), "provisional-user", "argon-hash", "provisional-owner", expiresAt)
	if err != nil {
		t.Fatalf("create provisional user: %v", err)
	}
	service := NewServiceWithProvider(sqliteStore, &fakeProvider{
		available: true,
		name:      "portablepay",
		webhookEvent: WebhookEvent{
			Kind:                   WebhookEventKindSubscriptionStateChanged,
			ID:                     "evt_activate",
			RawType:                "portable.subscription.created",
			UserID:                 user.ID,
			ProviderCustomerID:     "cus_activate",
			ProviderSubscriptionID: "sub_activate",
			EntitlementStatus:      store.EntitlementStatusActive,
		},
	}, "http://127.0.0.1:8080")

	if err := service.HandleWebhook(context.Background(), []byte(`{}`), http.Header{}); err != nil {
		t.Fatalf("handle webhook: %v", err)
	}

	loaded, err := sqliteStore.GetUserByUsername(context.Background(), "provisional-user")
	if err != nil {
		t.Fatalf("load activated user: %v", err)
	}
	if loaded.AccountStatus != store.AccountStatusActive || loaded.CheckoutExpiresAt != nil {
		t.Fatalf("expected active user after subscription event, got %+v", loaded)
	}
}

func TestServiceHandleInvoicePaidWebhookUpdatesEntitlementFromCustomerMapping(t *testing.T) {
	t.Parallel()

	sqliteStore := openBillingTestStore(t)
	user := createBillingTestUser(t, sqliteStore, "invoice-paid-user")
	if _, err := sqliteStore.PutBillingCustomer(context.Background(), user.ID, "portablepay", "cus_invoice"); err != nil {
		t.Fatalf("seed billing customer: %v", err)
	}
	service := NewServiceWithProvider(sqliteStore, &fakeProvider{
		available: true,
		name:      "portablepay",
		webhookEvent: WebhookEvent{
			Kind:                   WebhookEventKindSubscriptionStateChanged,
			ID:                     "evt_invoice_paid",
			RawType:                "portable.invoice.paid",
			ProviderCustomerID:     "cus_invoice",
			ProviderSubscriptionID: "sub_invoice",
			EntitlementStatus:      store.EntitlementStatusActive,
			ValidUntil:             stripeTimestampPointer(time.Date(2026, time.June, 20, 12, 0, 0, 0, time.UTC).Unix()),
		},
	}, "http://127.0.0.1:8080")

	if err := service.HandleWebhook(context.Background(), []byte(`{}`), http.Header{}); err != nil {
		t.Fatalf("handle invoice webhook: %v", err)
	}

	subscription, err := sqliteStore.GetBillingSubscriptionByProviderSubscriptionID(context.Background(), "portablepay", "sub_invoice")
	if err != nil {
		t.Fatalf("load billing subscription: %v", err)
	}
	if subscription.UserID != user.ID || subscription.Status != store.EntitlementStatusActive {
		t.Fatalf("unexpected billing subscription: %+v", subscription)
	}

	entitlement, err := sqliteStore.GetAccountEntitlement(context.Background(), user.ID, store.EntitlementKeyHostedSync)
	if err != nil {
		t.Fatalf("load entitlement: %v", err)
	}
	if entitlement.Status != store.EntitlementStatusActive || entitlement.Source != "portablepay" {
		t.Fatalf("unexpected entitlement: %+v", entitlement)
	}
}

func TestServiceHandleInvoicePaidWebhookUpdatesEntitlementFromUserMetadataWithoutCustomerMapping(t *testing.T) {
	t.Parallel()

	sqliteStore := openBillingTestStore(t)
	user := createBillingTestUser(t, sqliteStore, "invoice-first-user")
	service := NewServiceWithProvider(sqliteStore, NewStripeProvider(StripeProviderOptions{
		SecretKey:     "sk_test_123",
		WebhookSecret: "whsec_test",
		PriceID:       "price_123",
	}), "http://127.0.0.1:8080")
	payload := map[string]any{
		"id":   "evt_invoice_first",
		"type": "invoice.payment_succeeded",
		"data": map[string]any{
			"object": map[string]any{
				"id":       "in_invoice_first",
				"customer": "cus_invoice_first",
				"status":   "paid",
				"paid":     true,
				"parent": map[string]any{
					"subscription_details": map[string]any{
						"subscription": "sub_invoice_first",
						"metadata": map[string]string{
							"user_id": strconv.FormatInt(user.ID, 10),
						},
					},
				},
				"lines": map[string]any{
					"data": []map[string]any{
						{
							"period": map[string]any{
								"end": time.Date(2026, time.July, 20, 12, 0, 0, 0, time.UTC).Unix(),
							},
						},
					},
				},
			},
		},
	}
	rawBody, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal invoice-first payload: %v", err)
	}

	if err := service.HandleWebhook(context.Background(), rawBody, http.Header{"Stripe-Signature": []string{signedStripeHeader(t, "whsec_test", rawBody, time.Now().UTC().Unix())}}); err != nil {
		t.Fatalf("handle invoice-first webhook: %v", err)
	}

	customer, err := sqliteStore.GetBillingCustomer(context.Background(), user.ID, "stripe")
	if err != nil {
		t.Fatalf("load billing customer: %v", err)
	}
	if customer.ProviderCustomerID != "cus_invoice_first" {
		t.Fatalf("unexpected billing customer: %+v", customer)
	}

	subscription, err := sqliteStore.GetBillingSubscriptionByProviderSubscriptionID(context.Background(), "stripe", "sub_invoice_first")
	if err != nil {
		t.Fatalf("load billing subscription: %v", err)
	}
	if subscription.UserID != user.ID || subscription.Status != store.EntitlementStatusActive {
		t.Fatalf("unexpected billing subscription: %+v", subscription)
	}

	entitlement, err := sqliteStore.GetAccountEntitlement(context.Background(), user.ID, store.EntitlementKeyHostedSync)
	if err != nil {
		t.Fatalf("load entitlement: %v", err)
	}
	if entitlement.Status != store.EntitlementStatusActive || entitlement.Source != store.EntitlementSourceStripe {
		t.Fatalf("unexpected entitlement: %+v", entitlement)
	}
}

func TestServiceHandleNonActiveSubscriptionDoesNotActivateProvisionalUser(t *testing.T) {
	t.Parallel()

	sqliteStore := openBillingTestStore(t)
	expiresAt := time.Now().UTC().Add(time.Hour)
	user, err := sqliteStore.CreateProvisionalUser(context.Background(), "provisional-past-due", "argon-hash", "provisional-owner-past-due", expiresAt)
	if err != nil {
		t.Fatalf("create provisional user: %v", err)
	}
	service := NewServiceWithProvider(sqliteStore, &fakeProvider{
		available: true,
		name:      "portablepay",
		webhookEvent: WebhookEvent{
			Kind:                   WebhookEventKindSubscriptionStateChanged,
			ID:                     "evt_past_due",
			RawType:                "portable.subscription.updated",
			UserID:                 user.ID,
			ProviderCustomerID:     "cus_past_due",
			ProviderSubscriptionID: "sub_past_due",
			EntitlementStatus:      store.EntitlementStatusPastDue,
			ValidUntil:             stripeTimestampPointer(time.Date(2026, time.August, 20, 12, 0, 0, 0, time.UTC).Unix()),
		},
	}, "http://127.0.0.1:8080")

	if err := service.HandleWebhook(context.Background(), []byte(`{}`), http.Header{}); err != nil {
		t.Fatalf("handle webhook: %v", err)
	}

	loaded, err := sqliteStore.GetUserByUsername(context.Background(), "provisional-past-due")
	if err != nil {
		t.Fatalf("load provisional user: %v", err)
	}
	if loaded.AccountStatus != store.AccountStatusCheckoutPending {
		t.Fatalf("expected provisional user to remain checkout_pending, got %+v", loaded)
	}
	if loaded.CheckoutExpiresAt == nil {
		t.Fatalf("expected provisional checkout expiry to remain set, got %+v", loaded)
	}

	subscription, err := sqliteStore.GetBillingSubscriptionByProviderSubscriptionID(context.Background(), "portablepay", "sub_past_due")
	if err != nil {
		t.Fatalf("load billing subscription: %v", err)
	}
	if subscription.Status != store.EntitlementStatusPastDue {
		t.Fatalf("expected past_due billing subscription status, got %+v", subscription)
	}

	entitlement, err := sqliteStore.GetAccountEntitlement(context.Background(), user.ID, store.EntitlementKeyHostedSync)
	if err != nil {
		t.Fatalf("load entitlement: %v", err)
	}
	if entitlement.Status != store.EntitlementStatusPastDue || entitlement.Source != "portablepay" {
		t.Fatalf("unexpected entitlement: %+v", entitlement)
	}
}

func TestServiceHandleWebhookRejectsInvalidNormalizedEntitlementStatus(t *testing.T) {
	t.Parallel()

	sqliteStore := openBillingTestStore(t)
	user := createBillingTestUser(t, sqliteStore, "invalid-status-user")
	service := NewServiceWithProvider(sqliteStore, &fakeProvider{
		available: true,
		name:      "portablepay",
		webhookEvent: WebhookEvent{
			Kind:                   WebhookEventKindSubscriptionStateChanged,
			ID:                     "evt_invalid_status",
			RawType:                "portable.subscription.updated",
			UserID:                 user.ID,
			ProviderCustomerID:     "cus_invalid_status",
			ProviderSubscriptionID: "sub_invalid_status",
			EntitlementStatus:      "bogus_status",
		},
	}, "http://127.0.0.1:8080")

	if err := service.HandleWebhook(context.Background(), []byte(`{}`), http.Header{}); err == nil {
		t.Fatal("expected invalid normalized entitlement status to be rejected")
	}

	if _, err := sqliteStore.GetAccountEntitlement(context.Background(), user.ID, store.EntitlementKeyHostedSync); !errors.Is(err, store.ErrEntitlementNotFound) {
		t.Fatalf("expected no hosted sync entitlement for invalid normalized status, got %v", err)
	}
	if _, err := sqliteStore.GetBillingSubscriptionByProviderSubscriptionID(context.Background(), "portablepay", "sub_invalid_status"); !errors.Is(err, store.ErrBillingSubscriptionNotFound) {
		t.Fatalf("expected no billing subscription for invalid normalized status, got %v", err)
	}
}

func TestServiceHandlePaidInvoiceWithoutSubscriptionDoesNotGrantHostedSync(t *testing.T) {
	t.Parallel()

	sqliteStore := openBillingTestStore(t)
	user := createBillingTestUser(t, sqliteStore, "invoice-no-subscription-user")
	service := NewServiceWithProvider(sqliteStore, &fakeProvider{
		available: true,
		name:      "portablepay",
		webhookEvent: WebhookEvent{
			Kind:               WebhookEventKindSubscriptionStateChanged,
			ID:                 "evt_invoice_no_subscription",
			RawType:            "portable.invoice.paid",
			UserID:             user.ID,
			ProviderCustomerID: "cus_no_subscription",
			EntitlementStatus:  store.EntitlementStatusActive,
		},
	}, "http://127.0.0.1:8080")

	if err := service.HandleWebhook(context.Background(), []byte(`{}`), http.Header{}); err != nil {
		t.Fatalf("handle non-subscription invoice webhook: %v", err)
	}

	if _, err := sqliteStore.GetAccountEntitlement(context.Background(), user.ID, store.EntitlementKeyHostedSync); !errors.Is(err, store.ErrEntitlementNotFound) {
		t.Fatalf("expected no hosted sync entitlement for invoice without subscription, got %v", err)
	}
	if _, err := sqliteStore.GetBillingCustomer(context.Background(), user.ID, "portablepay"); !errors.Is(err, store.ErrBillingCustomerNotFound) {
		t.Fatalf("expected no billing customer for invoice without subscription, got %v", err)
	}
}

func TestServiceHandleWebhookIsIdempotentEnoughForRepeatedEvents(t *testing.T) {
	t.Parallel()

	sqliteStore := openBillingTestStore(t)
	user := createBillingTestUser(t, sqliteStore, "repeat-webhook-user")
	provider := &fakeProvider{
		available: true,
		name:      "portablepay",
		webhookEvent: WebhookEvent{
			Kind:                   WebhookEventKindSubscriptionStateChanged,
			ID:                     "evt_repeat",
			RawType:                "portable.subscription.updated",
			UserID:                 user.ID,
			ProviderCustomerID:     "cus_repeat",
			ProviderSubscriptionID: "sub_repeat",
			EntitlementStatus:      store.EntitlementStatusPastDue,
		},
	}
	service := NewServiceWithProvider(sqliteStore, provider, "http://127.0.0.1:8080")

	if err := service.HandleWebhook(context.Background(), []byte(`{}`), http.Header{}); err != nil {
		t.Fatalf("first handle webhook: %v", err)
	}
	if err := service.HandleWebhook(context.Background(), []byte(`{}`), http.Header{}); err != nil {
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

func (p *fakeProvider) ParseWebhook(ctx context.Context, rawBody []byte, headers http.Header) (WebhookEvent, error) {
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
