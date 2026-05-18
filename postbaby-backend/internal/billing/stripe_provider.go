package billing

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"postbaby-backend/internal/store"
)

const (
	defaultStripeAPIBaseURL       = "https://api.stripe.com"
	defaultStripeWebhookTolerance = 5 * time.Minute
)

type StripeProviderOptions struct {
	SecretKey     string
	WebhookSecret string
	PriceID       string
	APIBaseURL    string
	HTTPClient    *http.Client
}

type StripeProvider struct {
	secretKey        string
	webhookSecret    string
	priceID          string
	apiBaseURL       string
	httpClient       *http.Client
	webhookTolerance time.Duration
}

func NewStripeProvider(options StripeProviderOptions) *StripeProvider {
	apiBaseURL := strings.TrimRight(strings.TrimSpace(options.APIBaseURL), "/")
	if apiBaseURL == "" {
		apiBaseURL = defaultStripeAPIBaseURL
	}

	httpClient := options.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	return &StripeProvider{
		secretKey:        strings.TrimSpace(options.SecretKey),
		webhookSecret:    strings.TrimSpace(options.WebhookSecret),
		priceID:          strings.TrimSpace(options.PriceID),
		apiBaseURL:       apiBaseURL,
		httpClient:       httpClient,
		webhookTolerance: defaultStripeWebhookTolerance,
	}
}

func (p *StripeProvider) Name() string {
	return "stripe"
}

func (p *StripeProvider) Available() bool {
	return p.secretKey != "" && p.webhookSecret != "" && p.priceID != ""
}

func (p *StripeProvider) CreateCustomer(ctx context.Context, user *store.User) (string, error) {
	form := url.Values{}
	form.Set("name", user.Username)
	form.Set("metadata[user_id]", strconv.FormatInt(user.ID, 10))
	form.Set("metadata[username]", user.Username)

	var response struct {
		ID string `json:"id"`
	}
	if err := p.postForm(ctx, "/v1/customers", form, &response); err != nil {
		return "", err
	}
	if strings.TrimSpace(response.ID) == "" {
		return "", fmt.Errorf("stripe create customer returned empty id")
	}

	return response.ID, nil
}

func (p *StripeProvider) CreateCheckoutSession(ctx context.Context, input CheckoutSessionInput) (string, error) {
	form := url.Values{}
	form.Set("customer", input.ProviderCustomerID)
	form.Set("mode", "subscription")
	form.Set("line_items[0][price]", p.priceID)
	form.Set("line_items[0][quantity]", "1")
	form.Set("success_url", input.SuccessURL)
	form.Set("cancel_url", input.CancelURL)
	form.Set("client_reference_id", input.ClientReferenceID)
	form.Set("metadata[user_id]", strconv.FormatInt(input.User.ID, 10))
	form.Set("metadata[username]", input.User.Username)
	form.Set("subscription_data[metadata][user_id]", strconv.FormatInt(input.User.ID, 10))
	form.Set("subscription_data[metadata][username]", input.User.Username)

	var response struct {
		URL string `json:"url"`
	}
	if err := p.postForm(ctx, "/v1/checkout/sessions", form, &response); err != nil {
		return "", err
	}
	if strings.TrimSpace(response.URL) == "" {
		return "", fmt.Errorf("stripe create checkout session returned empty url")
	}

	return response.URL, nil
}

func (p *StripeProvider) CreatePortalSession(ctx context.Context, input PortalSessionInput) (string, error) {
	form := url.Values{}
	form.Set("customer", input.ProviderCustomerID)
	form.Set("return_url", input.ReturnURL)

	var response struct {
		URL string `json:"url"`
	}
	if err := p.postForm(ctx, "/v1/billing_portal/sessions", form, &response); err != nil {
		return "", err
	}
	if strings.TrimSpace(response.URL) == "" {
		return "", fmt.Errorf("stripe create portal session returned empty url")
	}

	return response.URL, nil
}

func (p *StripeProvider) ParseWebhook(ctx context.Context, rawBody []byte, signatureHeader string) (WebhookEvent, error) {
	if err := p.verifyWebhookSignature(rawBody, signatureHeader); err != nil {
		return WebhookEvent{}, err
	}

	var envelope struct {
		ID   string `json:"id"`
		Type string `json:"type"`
		Data struct {
			Object json.RawMessage `json:"object"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rawBody, &envelope); err != nil {
		return WebhookEvent{}, fmt.Errorf("decode stripe webhook event: %w", err)
	}

	switch envelope.Type {
	case "checkout.session.completed":
		var session struct {
			Customer          any               `json:"customer"`
			Subscription      any               `json:"subscription"`
			ClientReferenceID string            `json:"client_reference_id"`
			Metadata          map[string]string `json:"metadata"`
		}
		if err := json.Unmarshal(envelope.Data.Object, &session); err != nil {
			return WebhookEvent{}, fmt.Errorf("decode stripe checkout.session.completed: %w", err)
		}

		return WebhookEvent{
			ID:                     envelope.ID,
			Type:                   envelope.Type,
			UserID:                 parseStripeUserID(session.ClientReferenceID, session.Metadata),
			ProviderCustomerID:     decodeStripeID(session.Customer),
			ProviderSubscriptionID: decodeStripeID(session.Subscription),
		}, nil
	case "customer.subscription.created", "customer.subscription.updated", "customer.subscription.deleted":
		var subscription struct {
			ID               string            `json:"id"`
			Customer         any               `json:"customer"`
			Status           string            `json:"status"`
			CurrentPeriodEnd int64             `json:"current_period_end"`
			Metadata         map[string]string `json:"metadata"`
		}
		if err := json.Unmarshal(envelope.Data.Object, &subscription); err != nil {
			return WebhookEvent{}, fmt.Errorf("decode stripe subscription event: %w", err)
		}

		status := subscription.Status
		if envelope.Type == "customer.subscription.deleted" && strings.TrimSpace(status) == "" {
			status = "deleted"
		}

		return WebhookEvent{
			ID:                     envelope.ID,
			Type:                   envelope.Type,
			UserID:                 parseStripeUserID("", subscription.Metadata),
			ProviderCustomerID:     decodeStripeID(subscription.Customer),
			ProviderSubscriptionID: strings.TrimSpace(subscription.ID),
			Status:                 status,
			ValidUntil:             stripeTimestampPointer(subscription.CurrentPeriodEnd),
		}, nil
	default:
		return WebhookEvent{
			ID:   envelope.ID,
			Type: envelope.Type,
		}, nil
	}
}

func (p *StripeProvider) postForm(ctx context.Context, path string, form url.Values, dest any) error {
	endpoint := p.apiBaseURL + path
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.SetBasicAuth(p.secretKey, "")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("stripe request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read stripe response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("stripe request failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	if err := json.Unmarshal(body, dest); err != nil {
		return fmt.Errorf("decode stripe response: %w", err)
	}

	return nil
}

func (p *StripeProvider) verifyWebhookSignature(rawBody []byte, signatureHeader string) error {
	if strings.TrimSpace(signatureHeader) == "" {
		return ErrInvalidWebhookSignature
	}

	var timestamp string
	var signatures []string
	for _, component := range strings.Split(signatureHeader, ",") {
		pair := strings.SplitN(strings.TrimSpace(component), "=", 2)
		if len(pair) != 2 {
			continue
		}
		switch pair[0] {
		case "t":
			timestamp = pair[1]
		case "v1":
			signatures = append(signatures, pair[1])
		}
	}

	if timestamp == "" || len(signatures) == 0 {
		return ErrInvalidWebhookSignature
	}

	timestampUnix, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return ErrInvalidWebhookSignature
	}

	now := time.Now().UTC().Unix()
	if now-timestampUnix > int64(p.webhookTolerance.Seconds()) || timestampUnix-now > int64(p.webhookTolerance.Seconds()) {
		return ErrInvalidWebhookSignature
	}

	signedPayload := timestamp + "." + string(rawBody)
	mac := hmac.New(sha256.New, []byte(p.webhookSecret))
	_, _ = mac.Write([]byte(signedPayload))
	expectedSignature := hex.EncodeToString(mac.Sum(nil))

	for _, signature := range signatures {
		if subtle.ConstantTimeCompare([]byte(signature), []byte(expectedSignature)) == 1 {
			return nil
		}
	}

	return ErrInvalidWebhookSignature
}

func parseStripeUserID(clientReferenceID string, metadata map[string]string) int64 {
	if metadata != nil {
		if parsed, err := strconv.ParseInt(strings.TrimSpace(metadata["user_id"]), 10, 64); err == nil && parsed > 0 {
			return parsed
		}
	}
	if parsed, err := strconv.ParseInt(strings.TrimSpace(clientReferenceID), 10, 64); err == nil && parsed > 0 {
		return parsed
	}
	return 0
}

func decodeStripeID(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case map[string]any:
		if id, ok := typed["id"].(string); ok {
			return strings.TrimSpace(id)
		}
	}
	return ""
}

func stripeTimestampPointer(unixSeconds int64) *time.Time {
	if unixSeconds <= 0 {
		return nil
	}

	value := time.Unix(unixSeconds, 0).UTC()
	return &value
}
