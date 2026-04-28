package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	EnvironmentID string
	// Exactly one of PlatformToken or (ClientID+ClientSecret) is set after Load.
	PlatformToken string
	ClientID      string
	ClientSecret  string
	Scopes        []string
}

type fileShape struct {
	EnvironmentID string `yaml:"environment_id"`
	PlatformToken string `yaml:"platform_token"`
	OAuth         struct {
		ClientID     string   `yaml:"client_id"`
		ClientSecret string   `yaml:"client_secret"`
		Scopes       []string `yaml:"scopes"`
	} `yaml:"oauth"`
}

var defaultScopes = []string{"storage:logs:read", "storage:buckets:read"}

func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "dynatrace-tui", "config.yaml"), nil
}

// Load reads the YAML at path (if present) and overlays env vars on top.
// An empty path falls back to DefaultPath().
func Load(path string) (Config, error) {
	if path == "" {
		p, err := DefaultPath()
		if err != nil {
			return Config{}, err
		}
		path = p
	}

	cfg := Config{Scopes: defaultScopes}

	if data, err := os.ReadFile(path); err == nil {
		var f fileShape
		if err := yaml.Unmarshal(data, &f); err != nil {
			return Config{}, fmt.Errorf("parse %s: %w", path, err)
		}
		cfg.EnvironmentID = f.EnvironmentID
		cfg.PlatformToken = f.PlatformToken
		cfg.ClientID = f.OAuth.ClientID
		cfg.ClientSecret = f.OAuth.ClientSecret
		if len(f.OAuth.Scopes) > 0 {
			cfg.Scopes = f.OAuth.Scopes
		}
	} else if !os.IsNotExist(err) {
		return Config{}, fmt.Errorf("read %s: %w", path, err)
	}

	if v := os.Getenv("DT_ENVIRONMENT_ID"); v != "" {
		cfg.EnvironmentID = v
	}
	if v := os.Getenv("DT_PLATFORM_TOKEN"); v != "" {
		cfg.PlatformToken = v
	}
	if v := os.Getenv("DT_OAUTH_CLIENT_ID"); v != "" {
		cfg.ClientID = v
	}
	if v := os.Getenv("DT_OAUTH_CLIENT_SECRET"); v != "" {
		cfg.ClientSecret = v
	}

	if cfg.EnvironmentID == "" {
		return Config{}, fmt.Errorf("missing environment_id (set DT_ENVIRONMENT_ID or %s)", path)
	}
	hasPT := cfg.PlatformToken != ""
	hasOAuth := cfg.ClientID != "" && cfg.ClientSecret != ""
	if !hasPT && !hasOAuth {
		return Config{}, fmt.Errorf("missing credentials: set platform_token (DT_PLATFORM_TOKEN) or oauth.client_id/client_secret (DT_OAUTH_CLIENT_ID/SECRET) in %s", path)
	}
	return cfg, nil
}
