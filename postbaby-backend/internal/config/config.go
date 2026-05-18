package config

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	defaultAddr           = ":8080"
	defaultDBPath         = "./data/postbaby.db"
	defaultStatic         = "../"
	defaultSessionTTL     = 30 * 24 * time.Hour
	defaultDeploymentMode = DeploymentModeStaticLocal
)

type DeploymentMode string

const (
	DeploymentModeStaticLocal          DeploymentMode = "static_local"
	DeploymentModeSelfHostedSingleUser DeploymentMode = "selfhosted_single_user"
	DeploymentModeCloudMultiUser       DeploymentMode = "cloud_multi_user"
)

type Config struct {
	Addr                string
	DBPath              string
	StaticDir           string
	CookieSecure        bool
	SessionTTL          time.Duration
	DeploymentMode      DeploymentMode
	BillingProvider     string
	PublicBaseURL       string
	StripeSecretKey     string
	StripeWebhookSecret string
	StripePriceID       string
}

func Load() (Config, error) {
	dbPath := strings.TrimSpace(os.Getenv("POSTBABY_DB_PATH"))
	if dbPath == "" {
		dbPath = defaultDBPath
	}

	addr := strings.TrimSpace(os.Getenv("POSTBABY_ADDR"))
	if addr == "" {
		addr = defaultAddr
	}

	staticDir := strings.TrimSpace(os.Getenv("POSTBABY_STATIC_DIR"))
	if staticDir == "" {
		staticDir = defaultStatic
	}

	deploymentMode, err := parseDeploymentModeEnv("POSTBABY_DEPLOYMENT_MODE", defaultDeploymentMode)
	if err != nil {
		return Config{}, err
	}

	cookieSecure, err := parseBoolEnv("POSTBABY_COOKIE_SECURE", false)
	if err != nil {
		return Config{}, err
	}

	sessionTTL := defaultSessionTTL
	sessionTTLRaw := strings.TrimSpace(os.Getenv("POSTBABY_SESSION_TTL"))
	if sessionTTLRaw != "" {
		parsedTTL, parseErr := time.ParseDuration(sessionTTLRaw)
		if parseErr != nil {
			return Config{}, fmt.Errorf("parse POSTBABY_SESSION_TTL: %w", parseErr)
		}
		if parsedTTL <= 0 {
			return Config{}, fmt.Errorf("POSTBABY_SESSION_TTL must be greater than zero")
		}
		sessionTTL = parsedTTL
	}

	billingProvider, err := parseBillingProviderEnv("POSTBABY_BILLING_PROVIDER")
	if err != nil {
		return Config{}, err
	}

	publicBaseURL := strings.TrimSpace(os.Getenv("POSTBABY_PUBLIC_BASE_URL"))
	if publicBaseURL != "" {
		normalizedBaseURL, normalizeErr := normalizePublicBaseURL(publicBaseURL)
		if normalizeErr != nil {
			return Config{}, fmt.Errorf("parse POSTBABY_PUBLIC_BASE_URL: %w", normalizeErr)
		}
		publicBaseURL = normalizedBaseURL
	}

	stripeSecretKey := strings.TrimSpace(os.Getenv("POSTBABY_STRIPE_SECRET_KEY"))
	stripeWebhookSecret := strings.TrimSpace(os.Getenv("POSTBABY_STRIPE_WEBHOOK_SECRET"))
	stripePriceID := strings.TrimSpace(os.Getenv("POSTBABY_STRIPE_PRICE_ID"))

	if billingProvider == "stripe" {
		if stripeSecretKey == "" {
			return Config{}, fmt.Errorf("POSTBABY_STRIPE_SECRET_KEY is required when POSTBABY_BILLING_PROVIDER=stripe")
		}
		if stripeWebhookSecret == "" {
			return Config{}, fmt.Errorf("POSTBABY_STRIPE_WEBHOOK_SECRET is required when POSTBABY_BILLING_PROVIDER=stripe")
		}
		if stripePriceID == "" {
			return Config{}, fmt.Errorf("POSTBABY_STRIPE_PRICE_ID is required when POSTBABY_BILLING_PROVIDER=stripe")
		}
		if publicBaseURL == "" {
			return Config{}, fmt.Errorf("POSTBABY_PUBLIC_BASE_URL is required when POSTBABY_BILLING_PROVIDER=stripe")
		}
	}

	return Config{
		Addr:                addr,
		DBPath:              dbPath,
		StaticDir:           staticDir,
		CookieSecure:        cookieSecure,
		SessionTTL:          sessionTTL,
		DeploymentMode:      deploymentMode,
		BillingProvider:     billingProvider,
		PublicBaseURL:       publicBaseURL,
		StripeSecretKey:     stripeSecretKey,
		StripeWebhookSecret: stripeWebhookSecret,
		StripePriceID:       stripePriceID,
	}, nil
}

func (c Config) EnsureDBDir() error {
	dir := filepath.Dir(c.DBPath)
	if dir == "" {
		return nil
	}

	return os.MkdirAll(dir, 0o755)
}

func (c Config) ResolveStaticDir() (string, error) {
	resolved, err := filepath.Abs(c.StaticDir)
	if err != nil {
		return "", fmt.Errorf("resolve static dir: %w", err)
	}

	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("stat static dir: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("static dir is not a directory: %s", resolved)
	}

	indexPath := filepath.Join(resolved, "index.html")
	indexInfo, err := os.Stat(indexPath)
	if err != nil {
		return "", fmt.Errorf("stat index.html in static dir: %w", err)
	}
	if indexInfo.IsDir() {
		return "", fmt.Errorf("index.html is a directory: %s", indexPath)
	}

	return resolved, nil
}

func parseBoolEnv(name string, defaultValue bool) (bool, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return defaultValue, nil
	}

	switch strings.ToLower(raw) {
	case "1", "true", "yes", "on":
		return true, nil
	case "0", "false", "no", "off":
		return false, nil
	default:
		return false, fmt.Errorf("%s must be a boolean value", name)
	}
}

func parseDeploymentModeEnv(name string, defaultValue DeploymentMode) (DeploymentMode, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return defaultValue, nil
	}

	switch DeploymentMode(strings.ToLower(raw)) {
	case DeploymentModeStaticLocal:
		return DeploymentModeStaticLocal, nil
	case DeploymentModeSelfHostedSingleUser:
		return DeploymentModeSelfHostedSingleUser, nil
	case DeploymentModeCloudMultiUser:
		return DeploymentModeCloudMultiUser, nil
	default:
		return "", fmt.Errorf("%s must be one of %q, %q, or %q", name, DeploymentModeStaticLocal, DeploymentModeSelfHostedSingleUser, DeploymentModeCloudMultiUser)
	}
}

func parseBillingProviderEnv(name string) (string, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return "", nil
	}

	switch strings.ToLower(raw) {
	case "stripe":
		return "stripe", nil
	default:
		return "", fmt.Errorf("%s must be blank or %q", name, "stripe")
	}
}

func normalizePublicBaseURL(raw string) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("must include scheme and host")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	parsed.RawPath = strings.TrimRight(parsed.RawPath, "/")
	return parsed.String(), nil
}
