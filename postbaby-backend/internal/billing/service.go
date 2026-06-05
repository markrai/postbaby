package billing

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"postbaby-backend/internal/config"
	"postbaby-backend/internal/store"
)

var (
	ErrBillingUnavailable      = errors.New("billing unavailable")
	ErrBillingCustomerNotFound = errors.New("billing customer not found")
	ErrInvalidWebhookSignature = errors.New("invalid webhook signature")
)

type Service struct {
	store         store.BillingStore
	provider      Provider
	publicBaseURL string
}

type Provider interface {
	Name() string
	Available() bool
	CreateCustomer(ctx context.Context, user *store.User) (string, error)
	CreateCheckoutSession(ctx context.Context, input CheckoutSessionInput) (string, error)
	CreatePortalSession(ctx context.Context, input PortalSessionInput) (string, error)
	ParseWebhook(ctx context.Context, rawBody []byte, headers http.Header) (WebhookEvent, error)
}

type CheckoutSessionInput struct {
	User               *store.User
	ProviderCustomerID string
	SuccessURL         string
	CancelURL          string
	ClientReferenceID  string
}

type PortalSessionInput struct {
	User               *store.User
	ProviderCustomerID string
	ReturnURL          string
}

type WebhookEvent struct {
	Kind                   string
	ID                     string
	RawType                string
	UserID                 int64
	ProviderCustomerID     string
	ProviderSubscriptionID string
	EntitlementStatus      string
	ValidUntil             *time.Time
}

const (
	WebhookEventKindIgnore                   = "ignore"
	WebhookEventKindCustomerLinked           = "customer_linked"
	WebhookEventKindSubscriptionStateChanged = "subscription_state_changed"
)

func NewService(billingStore store.BillingStore, cfg config.Config) (*Service, error) {
	provider, err := newProviderFromConfig(cfg)
	if err != nil {
		return nil, err
	}

	return NewServiceWithProvider(billingStore, provider, cfg.PublicBaseURL), nil
}

func NewServiceWithProvider(billingStore store.BillingStore, provider Provider, publicBaseURL string) *Service {
	return &Service{
		store:         billingStore,
		provider:      provider,
		publicBaseURL: strings.TrimRight(publicBaseURL, "/"),
	}
}

func (s *Service) Available() bool {
	return s != nil && s.provider != nil && s.provider.Available()
}

func (s *Service) ProviderName() string {
	if !s.Available() {
		return ""
	}
	return s.provider.Name()
}

func newProviderFromConfig(cfg config.Config) (Provider, error) {
	switch cfg.BillingProvider {
	case "":
		return nil, nil
	case "stripe":
		return NewStripeProvider(StripeProviderOptions{
			SecretKey:     cfg.StripeSecretKey,
			WebhookSecret: cfg.StripeWebhookSecret,
			PriceID:       cfg.StripePriceID,
		}), nil
	default:
		return nil, fmt.Errorf("unsupported billing provider %q", cfg.BillingProvider)
	}
}

func (s *Service) CreateCheckoutSession(ctx context.Context, user *store.User) (string, error) {
	if !s.Available() {
		return "", ErrBillingUnavailable
	}

	customerID, err := s.ensureProviderCustomer(ctx, user)
	if err != nil {
		return "", err
	}

	return s.provider.CreateCheckoutSession(ctx, CheckoutSessionInput{
		User:               user,
		ProviderCustomerID: customerID,
		SuccessURL:         s.publicURLWithQuery("billing", "success"),
		CancelURL:          s.publicURLWithQuery("billing", "canceled"),
		ClientReferenceID:  strconv.FormatInt(user.ID, 10),
	})
}

func (s *Service) CreatePortalSession(ctx context.Context, user *store.User) (string, error) {
	if !s.Available() {
		return "", ErrBillingUnavailable
	}

	customer, err := s.store.GetBillingCustomer(ctx, user.ID, s.provider.Name())
	if err != nil {
		if errors.Is(err, store.ErrBillingCustomerNotFound) {
			return "", ErrBillingCustomerNotFound
		}
		return "", err
	}

	return s.provider.CreatePortalSession(ctx, PortalSessionInput{
		User:               user,
		ProviderCustomerID: customer.ProviderCustomerID,
		ReturnURL:          s.publicURLWithQuery("billing", "manage"),
	})
}

func (s *Service) HandleWebhook(ctx context.Context, rawBody []byte, headers http.Header) error {
	if !s.Available() {
		return ErrBillingUnavailable
	}

	event, err := s.provider.ParseWebhook(ctx, rawBody, headers)
	if err != nil {
		return err
	}

	switch event.Kind {
	case WebhookEventKindCustomerLinked:
		return s.applyCustomerLinked(ctx, event)
	case WebhookEventKindSubscriptionStateChanged:
		return s.applySubscriptionStateChanged(ctx, event)
	case "", WebhookEventKindIgnore:
		log.Printf("billing webhook ignored kind=%s raw_type=%s id=%s", strings.TrimSpace(event.Kind), event.logType(), event.ID)
		return nil
	default:
		log.Printf("billing webhook ignored kind=%s raw_type=%s id=%s", event.Kind, event.logType(), event.ID)
		return nil
	}
}

func (s *Service) ensureProviderCustomer(ctx context.Context, user *store.User) (string, error) {
	customer, err := s.store.GetBillingCustomer(ctx, user.ID, s.provider.Name())
	if err == nil {
		return customer.ProviderCustomerID, nil
	}
	if !errors.Is(err, store.ErrBillingCustomerNotFound) {
		return "", err
	}

	providerCustomerID, err := s.provider.CreateCustomer(ctx, user)
	if err != nil {
		return "", err
	}

	if _, err := s.store.PutBillingCustomer(ctx, user.ID, s.provider.Name(), providerCustomerID); err != nil {
		return "", err
	}

	return providerCustomerID, nil
}

func (s *Service) applyCustomerLinked(ctx context.Context, event WebhookEvent) error {
	if event.ProviderCustomerID == "" {
		log.Printf("billing customer-linked webhook missing customer id raw_type=%s id=%s", event.logType(), event.ID)
		return nil
	}

	userID, err := s.resolveUserID(ctx, event)
	if err != nil {
		return fmt.Errorf("resolve billing user for customer-linked webhook raw_type=%s id=%s customer_id=%s: %w", event.logType(), event.ID, event.ProviderCustomerID, err)
	}

	if _, err := s.store.PutBillingCustomer(ctx, userID, s.provider.Name(), event.ProviderCustomerID); err != nil {
		return err
	}

	log.Printf("billing customer linked raw_type=%s id=%s user_id=%d provider=%s customer_id=%s", event.logType(), event.ID, userID, s.provider.Name(), event.ProviderCustomerID)
	return nil
}

func (s *Service) applySubscriptionStateChanged(ctx context.Context, event WebhookEvent) error {
	if event.ProviderSubscriptionID == "" {
		log.Printf("billing subscription-state webhook missing subscription id raw_type=%s id=%s customer_id=%s", event.logType(), event.ID, event.ProviderCustomerID)
		return nil
	}

	entitlementStatus := strings.TrimSpace(event.EntitlementStatus)
	if entitlementStatus == "" {
		log.Printf("billing subscription-state webhook missing entitlement status raw_type=%s id=%s customer_id=%s subscription_id=%s", event.logType(), event.ID, event.ProviderCustomerID, event.ProviderSubscriptionID)
		return nil
	}
	if !isValidEntitlementStatus(entitlementStatus) {
		return fmt.Errorf("invalid normalized entitlement status %q for billing event kind=%s raw_type=%s id=%s", entitlementStatus, event.Kind, event.logType(), event.ID)
	}

	userID, err := s.resolveUserID(ctx, event)
	if err != nil {
		return fmt.Errorf("resolve billing user for subscription-state webhook raw_type=%s id=%s customer_id=%s subscription_id=%s: %w", event.logType(), event.ID, event.ProviderCustomerID, event.ProviderSubscriptionID, err)
	}

	if event.ProviderCustomerID != "" {
		if _, err := s.store.PutBillingCustomer(ctx, userID, s.provider.Name(), event.ProviderCustomerID); err != nil {
			return err
		}
	}

	if entitlementStatus == store.EntitlementStatusActive {
		if err := s.store.ActivateUser(ctx, userID); err != nil {
			return err
		}
	}
	if _, err := s.store.PutBillingSubscription(ctx, userID, s.provider.Name(), event.ProviderSubscriptionID, entitlementStatus, event.ValidUntil); err != nil {
		return err
	}
	if _, err := s.store.PutAccountEntitlement(ctx, userID, store.EntitlementKeyHostedSync, entitlementStatus, s.provider.Name(), event.ValidUntil); err != nil {
		return err
	}

	log.Printf("billing entitlement updated raw_type=%s id=%s user_id=%d provider=%s customer_id=%s subscription_id=%s entitlement_status=%s", event.logType(), event.ID, userID, s.provider.Name(), event.ProviderCustomerID, event.ProviderSubscriptionID, entitlementStatus)
	return nil
}

func (s *Service) resolveUserID(ctx context.Context, event WebhookEvent) (int64, error) {
	if event.UserID > 0 {
		return event.UserID, nil
	}
	if event.ProviderCustomerID != "" {
		customer, err := s.store.GetBillingCustomerByProviderCustomerID(ctx, s.provider.Name(), event.ProviderCustomerID)
		if err == nil {
			return customer.UserID, nil
		}
		if !errors.Is(err, store.ErrBillingCustomerNotFound) {
			return 0, err
		}
	}
	if event.ProviderSubscriptionID != "" {
		subscription, err := s.store.GetBillingSubscriptionByProviderSubscriptionID(ctx, s.provider.Name(), event.ProviderSubscriptionID)
		if err == nil {
			return subscription.UserID, nil
		}
		if !errors.Is(err, store.ErrBillingSubscriptionNotFound) {
			return 0, err
		}
	}

	return 0, fmt.Errorf("could not resolve billing user for event %q", event.ID)
}
func (s *Service) publicURLWithQuery(key, value string) string {
	parsed, err := url.Parse(s.publicBaseURL)
	if err != nil {
		return s.publicBaseURL
	}
	if parsed.Path == "" {
		parsed.Path = "/"
	}
	query := parsed.Query()
	query.Set(key, value)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func (e WebhookEvent) logType() string {
	if strings.TrimSpace(e.RawType) != "" {
		return e.RawType
	}
	if strings.TrimSpace(e.Kind) != "" {
		return e.Kind
	}
	return "unknown"
}

func isValidEntitlementStatus(status string) bool {
	switch strings.TrimSpace(status) {
	case store.EntitlementStatusActive,
		store.EntitlementStatusPastDue,
		store.EntitlementStatusCanceled,
		store.EntitlementStatusExpired:
		return true
	default:
		return false
	}
}
