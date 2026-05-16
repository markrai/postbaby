package config

import (
	"fmt"
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
	Addr           string
	DBPath         string
	StaticDir      string
	CookieSecure   bool
	SessionTTL     time.Duration
	DeploymentMode DeploymentMode
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

	return Config{
		Addr:           addr,
		DBPath:         dbPath,
		StaticDir:      staticDir,
		CookieSecure:   cookieSecure,
		SessionTTL:     sessionTTL,
		DeploymentMode: deploymentMode,
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
