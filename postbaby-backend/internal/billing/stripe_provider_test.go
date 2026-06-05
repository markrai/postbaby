package billing

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"postbaby-backend/internal/store"
)

func TestStripeProviderCreateCheckoutSessionUsesSubscriptionCheckout(t *testing.T) {
	t.Parallel()

	var capturedAuth string
	var capturedForm url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/checkout/sessions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		capturedAuth = r.Header.Get("Authorization")
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		capturedForm = r.PostForm
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"url":"https://checkout.stripe.test/session"}`))
	}))
	defer server.Close()

	provider := NewStripeProvider(StripeProviderOptions{
		SecretKey:     "sk_test_123",
		WebhookSecret: "whsec_123",
		PriceID:       "price_123",
		APIBaseURL:    server.URL,
		HTTPClient:    server.Client(),
	})

	redirectURL, err := provider.CreateCheckoutSession(context.Background(), CheckoutSessionInput{
		User:               &store.User{ID: 42, Username: "cloud-user"},
		ProviderCustomerID: "cus_123",
		SuccessURL:         "http://127.0.0.1:8080/?billing=success",
		CancelURL:          "http://127.0.0.1:8080/?billing=canceled",
		ClientReferenceID:  "42",
	})
	if err != nil {
		t.Fatalf("create checkout session: %v", err)
	}

	if redirectURL != "https://checkout.stripe.test/session" {
		t.Fatalf("unexpected redirect URL %q", redirectURL)
	}
	if capturedAuth != "Basic "+base64.StdEncoding.EncodeToString([]byte("sk_test_123:")) {
		t.Fatalf("unexpected Authorization header %q", capturedAuth)
	}
	if capturedForm.Get("customer") != "cus_123" || capturedForm.Get("mode") != "subscription" {
		t.Fatalf("unexpected checkout form: %+v", capturedForm)
	}
	if capturedForm.Get("line_items[0][price]") != "price_123" || capturedForm.Get("line_items[0][quantity]") != "1" {
		t.Fatalf("unexpected line item form values: %+v", capturedForm)
	}
	if capturedForm.Get("subscription_data[metadata][user_id]") != "42" {
		t.Fatalf("expected subscription metadata user_id, got %+v", capturedForm)
	}
}

func TestStripeProviderCreatePortalSessionUsesCustomerPortal(t *testing.T) {
	t.Parallel()

	var capturedForm url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/billing_portal/sessions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		capturedForm = r.PostForm
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"url":"https://billing.stripe.test/session"}`))
	}))
	defer server.Close()

	provider := NewStripeProvider(StripeProviderOptions{
		SecretKey:     "sk_test_123",
		WebhookSecret: "whsec_123",
		PriceID:       "price_123",
		APIBaseURL:    server.URL,
		HTTPClient:    server.Client(),
	})

	redirectURL, err := provider.CreatePortalSession(context.Background(), PortalSessionInput{
		User:               &store.User{ID: 42, Username: "cloud-user"},
		ProviderCustomerID: "cus_123",
		ReturnURL:          "http://127.0.0.1:8080/?billing=manage",
	})
	if err != nil {
		t.Fatalf("create portal session: %v", err)
	}

	if redirectURL != "https://billing.stripe.test/session" {
		t.Fatalf("unexpected redirect URL %q", redirectURL)
	}
	if capturedForm.Get("customer") != "cus_123" || capturedForm.Get("return_url") != "http://127.0.0.1:8080/?billing=manage" {
		t.Fatalf("unexpected portal form: %+v", capturedForm)
	}
}

func TestStripeProviderParseWebhookMapsCheckoutCompletionToCustomerLinked(t *testing.T) {
	t.Parallel()

	provider := NewStripeProvider(StripeProviderOptions{
		SecretKey:     "sk_test_123",
		WebhookSecret: "whsec_test",
		PriceID:       "price_123",
	})

	payload := map[string]any{
		"id":   "evt_checkout",
		"type": "checkout.session.completed",
		"data": map[string]any{
			"object": map[string]any{
				"customer":            "cus_123",
				"subscription":        "sub_123",
				"client_reference_id": "42",
				"metadata": map[string]string{
					"user_id": "42",
				},
			},
		},
	}
	rawBody, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	signatureHeader := signedStripeHeader(t, "whsec_test", rawBody, time.Now().UTC().Unix())

	event, err := provider.ParseWebhook(context.Background(), rawBody, http.Header{"Stripe-Signature": []string{signatureHeader}})
	if err != nil {
		t.Fatalf("parse webhook: %v", err)
	}

	if event.Kind != WebhookEventKindCustomerLinked || event.RawType != "checkout.session.completed" || event.UserID != 42 || event.ProviderCustomerID != "cus_123" || event.ProviderSubscriptionID != "sub_123" {
		t.Fatalf("unexpected parsed event: %+v", event)
	}
}

func TestStripeProviderParseWebhookVerifiesSignatureAndMapsSubscriptionEvent(t *testing.T) {
	t.Parallel()

	provider := NewStripeProvider(StripeProviderOptions{
		SecretKey:     "sk_test_123",
		WebhookSecret: "whsec_test",
		PriceID:       "price_123",
	})

	payload := map[string]any{
		"id":   "evt_123",
		"type": "customer.subscription.updated",
		"data": map[string]any{
			"object": map[string]any{
				"id":                 "sub_123",
				"customer":           "cus_123",
				"status":             "active",
				"current_period_end": int64(1770000000),
				"metadata": map[string]string{
					"user_id": "42",
				},
			},
		},
	}
	rawBody, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	signatureHeader := signedStripeHeader(t, "whsec_test", rawBody, time.Now().UTC().Unix())

	event, err := provider.ParseWebhook(context.Background(), rawBody, http.Header{"Stripe-Signature": []string{signatureHeader}})
	if err != nil {
		t.Fatalf("parse webhook: %v", err)
	}

	if event.Kind != WebhookEventKindSubscriptionStateChanged || event.RawType != "customer.subscription.updated" || event.UserID != 42 || event.ProviderCustomerID != "cus_123" || event.ProviderSubscriptionID != "sub_123" || event.EntitlementStatus != store.EntitlementStatusActive {
		t.Fatalf("unexpected parsed event: %+v", event)
	}
	if event.ValidUntil == nil || event.ValidUntil.UTC().Unix() != 1770000000 {
		t.Fatalf("unexpected valid_until: %+v", event.ValidUntil)
	}
}

func TestStripeProviderParseWebhookMapsInvoicePaidEvent(t *testing.T) {
	t.Parallel()

	provider := NewStripeProvider(StripeProviderOptions{
		SecretKey:     "sk_test_123",
		WebhookSecret: "whsec_test",
		PriceID:       "price_123",
	})

	payload := map[string]any{
		"id":   "evt_invoice_paid",
		"type": "invoice.paid",
		"data": map[string]any{
			"object": map[string]any{
				"id":       "in_123",
				"customer": "cus_123",
				"status":   "paid",
				"paid":     true,
				"parent": map[string]any{
					"subscription_details": map[string]any{
						"subscription": "sub_123",
						"metadata": map[string]string{
							"user_id": "42",
						},
					},
				},
				"lines": map[string]any{
					"data": []map[string]any{
						{
							"period": map[string]any{
								"end": int64(1775000000),
							},
						},
					},
				},
			},
		},
	}
	rawBody, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	signatureHeader := signedStripeHeader(t, "whsec_test", rawBody, time.Now().UTC().Unix())

	event, err := provider.ParseWebhook(context.Background(), rawBody, http.Header{"Stripe-Signature": []string{signatureHeader}})
	if err != nil {
		t.Fatalf("parse webhook: %v", err)
	}

	if event.Kind != WebhookEventKindSubscriptionStateChanged || event.RawType != "invoice.paid" || event.UserID != 42 || event.ProviderCustomerID != "cus_123" || event.ProviderSubscriptionID != "sub_123" || event.EntitlementStatus != store.EntitlementStatusActive {
		t.Fatalf("unexpected parsed event: %+v", event)
	}
	if event.ValidUntil == nil || event.ValidUntil.UTC().Unix() != 1775000000 {
		t.Fatalf("unexpected valid_until: %+v", event.ValidUntil)
	}
}

func TestStripeProviderRejectsInvalidWebhookSignature(t *testing.T) {
	t.Parallel()

	provider := NewStripeProvider(StripeProviderOptions{
		SecretKey:     "sk_test_123",
		WebhookSecret: "whsec_test",
		PriceID:       "price_123",
	})

	if _, err := provider.ParseWebhook(context.Background(), []byte(`{"id":"evt","type":"checkout.session.completed","data":{"object":{}}}`), http.Header{"Stripe-Signature": []string{"t=1,v1=bogus"}}); !errors.Is(err, ErrInvalidWebhookSignature) {
		t.Fatalf("expected invalid signature error, got %v", err)
	}
}

func signedStripeHeader(t *testing.T, secret string, rawBody []byte, timestamp int64) string {
	t.Helper()

	payload := fmt.Sprintf("%d.%s", timestamp, string(rawBody))
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(payload))
	signature := hex.EncodeToString(mac.Sum(nil))
	return fmt.Sprintf("t=%d,v1=%s", timestamp, signature)
}
