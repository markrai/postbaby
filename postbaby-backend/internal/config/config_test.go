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
