package config

import "testing"

func TestLoadDefaultsToStaticLocalMode(t *testing.T) {
	t.Setenv("POSTBABY_DEPLOYMENT_MODE", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.DeploymentMode != DeploymentModeStaticLocal {
		t.Fatalf("expected default deployment mode %q, got %q", DeploymentModeStaticLocal, cfg.DeploymentMode)
	}
}

func TestLoadAcceptsSelfHostedSingleUserMode(t *testing.T) {
	t.Setenv("POSTBABY_DEPLOYMENT_MODE", string(DeploymentModeSelfHostedSingleUser))

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.DeploymentMode != DeploymentModeSelfHostedSingleUser {
		t.Fatalf("expected deployment mode %q, got %q", DeploymentModeSelfHostedSingleUser, cfg.DeploymentMode)
	}
}

func TestLoadAcceptsCloudMultiUserMode(t *testing.T) {
	t.Setenv("POSTBABY_DEPLOYMENT_MODE", string(DeploymentModeCloudMultiUser))

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.DeploymentMode != DeploymentModeCloudMultiUser {
		t.Fatalf("expected deployment mode %q, got %q", DeploymentModeCloudMultiUser, cfg.DeploymentMode)
	}
}

func TestLoadRejectsInvalidDeploymentMode(t *testing.T) {
	t.Setenv("POSTBABY_DEPLOYMENT_MODE", "invalid-mode")

	if _, err := Load(); err == nil {
		t.Fatal("expected invalid deployment mode error")
	}
}

func TestLoadDefaultsBillingToDisabled(t *testing.T) {
	t.Setenv("POSTBABY_BILLING_PROVIDER", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.BillingProvider != "" {
		t.Fatalf("expected billing provider to be disabled, got %q", cfg.BillingProvider)
	}
	if cfg.PublicBaseURL != "" {
		t.Fatalf("expected public base URL to be blank when unset, got %q", cfg.PublicBaseURL)
	}
}

func TestLoadAcceptsStripeBillingConfig(t *testing.T) {
	t.Setenv("POSTBABY_BILLING_PROVIDER", "stripe")
	t.Setenv("POSTBABY_STRIPE_SECRET_KEY", "sk_test_CHANGE_ME")
	t.Setenv("POSTBABY_STRIPE_WEBHOOK_SECRET", "whsec_CHANGE_ME")
	t.Setenv("POSTBABY_STRIPE_PRICE_ID", "price_CHANGE_ME")
	t.Setenv("POSTBABY_PUBLIC_BASE_URL", "http://127.0.0.1:8080/")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.BillingProvider != "stripe" {
		t.Fatalf("expected billing provider stripe, got %q", cfg.BillingProvider)
	}
	if cfg.PublicBaseURL != "http://127.0.0.1:8080" {
		t.Fatalf("expected normalized public base URL, got %q", cfg.PublicBaseURL)
	}
	if cfg.StripeSecretKey != "sk_test_CHANGE_ME" || cfg.StripeWebhookSecret != "whsec_CHANGE_ME" || cfg.StripePriceID != "price_CHANGE_ME" {
		t.Fatalf("unexpected stripe config: %+v", cfg)
	}
}

func TestLoadRejectsInvalidBillingProvider(t *testing.T) {
	t.Setenv("POSTBABY_BILLING_PROVIDER", "bogus")

	if _, err := Load(); err == nil {
		t.Fatal("expected invalid billing provider error")
	}
}

func TestLoadRejectsIncompleteStripeBillingConfig(t *testing.T) {
	t.Setenv("POSTBABY_BILLING_PROVIDER", "stripe")
	t.Setenv("POSTBABY_STRIPE_SECRET_KEY", "sk_test_CHANGE_ME")
	t.Setenv("POSTBABY_STRIPE_WEBHOOK_SECRET", "")
	t.Setenv("POSTBABY_STRIPE_PRICE_ID", "price_CHANGE_ME")
	t.Setenv("POSTBABY_PUBLIC_BASE_URL", "http://127.0.0.1:8080")

	if _, err := Load(); err == nil {
		t.Fatal("expected incomplete stripe billing config error")
	}
}
