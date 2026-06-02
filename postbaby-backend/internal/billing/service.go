package billing

import (
	"context"
	"errors"
	"fmt"
	"log"
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
	ParseWebhook(ctx context.Context, rawBody []byte, signatureHeader string) (WebhookEvent, error)
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
	ID                     string
	Type                   string
	UserID                 int64
	ProviderCustomerID     string
	ProviderSubscriptionID string
	Status                 string
	ValidUntil             *time.Time
}

func NewService(billingStore store.BillingStore, cfg config.Config) (*Service, error) {
	switch cfg.BillingProvider {
	case "":
		return NewServiceWithProvider(billingStore, nil, cfg.PublicBaseURL), nil
	case "stripe":
		return NewServiceWithProvider(billingStore, NewStripeProvider(StripeProviderOptions{
			SecretKey:     cfg.StripeSecretKey,
			WebhookSecret: cfg.StripeWebhookSecret,
			PriceID:       cfg.StripePriceID,
		}), cfg.PublicBaseURL), nil
	default:
		return nil, fmt.Errorf("unsupported billing provider %q", cfg.BillingProvider)
	}
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

func (s *Service) HandleWebhook(ctx context.Context, rawBody []byte, signatureHeader string) error {
	if !s.Available() {
		return ErrBillingUnavailable
	}

	event, err := s.provider.ParseWebhook(ctx, rawBody, signatureHeader)
	if err != nil {
		return err
	}

	switch event.Type {
	case "checkout.session.completed":
		return s.applyCheckoutCompleted(ctx, event)
	case "customer.subscription.created", "customer.subscription.updated", "customer.subscription.deleted", "invoice.paid", "invoice.payment_succeeded":
		return s.applySubscriptionEvent(ctx, event)
	default:
		log.Printf("billing webhook ignored type=%s id=%s", event.Type, event.ID)
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

func (s *Service) applyCheckoutCompleted(ctx context.Context, event WebhookEvent) error {
	if event.ProviderCustomerID == "" {
		log.Printf("billing checkout webhook missing customer id type=%s id=%s", event.Type, event.ID)
		return nil
	}

	userID, err := s.resolveUserID(ctx, event)
	if err != nil {
		return fmt.Errorf("resolve billing user for checkout webhook type=%s id=%s customer_id=%s: %w", event.Type, event.ID, event.ProviderCustomerID, err)
	}

	if _, err := s.store.PutBillingCustomer(ctx, userID, s.provider.Name(), event.ProviderCustomerID); err != nil {
		return err
	}

	log.Printf("billing checkout customer linked type=%s id=%s user_id=%d provider=%s customer_id=%s", event.Type, event.ID, userID, s.provider.Name(), event.ProviderCustomerID)
	return nil
}

func (s *Service) applySubscriptionEvent(ctx context.Context, event WebhookEvent) error {
	if event.ProviderSubscriptionID == "" {
		log.Printf("billing subscription webhook missing subscription id type=%s id=%s customer_id=%s", event.Type, event.ID, event.ProviderCustomerID)
		return nil
	}

	userID, err := s.resolveUserID(ctx, event)
	if err != nil {
		return fmt.Errorf("resolve billing user for subscription webhook type=%s id=%s customer_id=%s subscription_id=%s: %w", event.Type, event.ID, event.ProviderCustomerID, event.ProviderSubscriptionID, err)
	}

	if event.ProviderCustomerID != "" {
		if _, err := s.store.PutBillingCustomer(ctx, userID, s.provider.Name(), event.ProviderCustomerID); err != nil {
			return err
		}
	}

	entitlementStatus := mapSubscriptionStatusToEntitlement(event.Status)
	if entitlementStatus == store.EntitlementStatusActive {
		if err := s.store.ActivateUser(ctx, userID); err != nil {
			return err
		}
	}
	if _, err := s.store.PutBillingSubscription(ctx, userID, s.provider.Name(), event.ProviderSubscriptionID, entitlementStatus, event.ValidUntil); err != nil {
		return err
	}
	if _, err := s.store.PutAccountEntitlement(ctx, userID, store.EntitlementKeyHostedSync, entitlementStatus, store.EntitlementSourceStripe, event.ValidUntil); err != nil {
		return err
	}

	log.Printf("billing entitlement updated type=%s id=%s user_id=%d provider=%s customer_id=%s subscription_id=%s subscription_status=%s entitlement_status=%s", event.Type, event.ID, userID, s.provider.Name(), event.ProviderCustomerID, event.ProviderSubscriptionID, event.Status, entitlementStatus)
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

func mapSubscriptionStatusToEntitlement(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "active", "trialing":
		return store.EntitlementStatusActive
	case "past_due":
		return store.EntitlementStatusPastDue
	case "canceled":
		return store.EntitlementStatusCanceled
	case "unpaid", "incomplete", "incomplete_expired", "paused", "deleted":
		return store.EntitlementStatusExpired
	default:
		return store.EntitlementStatusExpired
	}
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
