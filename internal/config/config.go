package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config is a fully resolved environment with credentials applied. Returned
// from Loaded.Config for a specific environment name.
type Config struct {
	Name          string
	EnvironmentID string
	// Exactly one of PlatformToken or (ClientID+ClientSecret) is set after Load.
	PlatformToken string
	ClientID      string
	ClientSecret  string
	Scopes        []string
}

// Loaded is the parsed config file. It owns the ordered list of environment
// names and lets the caller materialise a Config for any of them.
type Loaded struct {
	Path     string
	Names    []string // ordered as in the file
	Selected string   // resolved selection (CLI flag → `default:` → first)
	specs    map[string]envSpec
}

type envSpec struct {
	EnvironmentID string `yaml:"environment_id"`
	PlatformToken string `yaml:"platform_token"`
	OAuth         struct {
		ClientID     string   `yaml:"client_id"`
		ClientSecret string   `yaml:"client_secret"`
		Scopes       []string `yaml:"scopes"`
	} `yaml:"oauth"`
}

type fileShape struct {
	// New multi-environment shape. yaml.Node so we can preserve key order.
	Environments yaml.Node `yaml:"environments"`
	Default      string    `yaml:"default"`

	// Legacy single-environment shape — synthesised into a single env named
	// "default" when `environments` is absent.
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

// Load parses the YAML at path (or DefaultPath when empty) and resolves which
// environment is active. Selection priority: explicit selectedEnv (CLI flag),
// then `default:` in the file, then the first environment in the file.
func Load(path, selectedEnv string) (*Loaded, error) {
	if path == "" {
		p, err := DefaultPath()
		if err != nil {
			return nil, err
		}
		path = p
	}

	var f fileShape
	if data, err := os.ReadFile(path); err == nil {
		if err := yaml.Unmarshal(data, &f); err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	names, specs, err := decodeEnvs(f.Environments)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	// Back-compat: synthesise a single "default" env from legacy top-level fields.
	if len(names) == 0 && (f.EnvironmentID != "" || f.PlatformToken != "" || f.OAuth.ClientID != "") {
		spec := envSpec{
			EnvironmentID: f.EnvironmentID,
			PlatformToken: f.PlatformToken,
		}
		spec.OAuth.ClientID = f.OAuth.ClientID
		spec.OAuth.ClientSecret = f.OAuth.ClientSecret
		spec.OAuth.Scopes = f.OAuth.Scopes
		names = []string{"default"}
		specs = map[string]envSpec{"default": spec}
	}

	loaded := &Loaded{Path: path, Names: names, specs: specs}

	pick := selectedEnv
	if pick == "" {
		pick = f.Default
	}
	if pick == "" && len(names) > 0 {
		pick = names[0]
	}
	if pick != "" {
		if _, ok := specs[pick]; !ok && len(names) > 0 {
			return nil, fmt.Errorf("environment %q not found in config (have: %s)", pick, strings.Join(names, ", "))
		}
	}
	loaded.Selected = pick
	return loaded, nil
}

// Config returns a fully resolved config for the named environment, applying
// DT_* env-var overrides on top. Pass an empty name to use loaded.Selected.
func (l *Loaded) Config(name string) (Config, error) {
	if name == "" {
		name = l.Selected
	}
	cfg := Config{Name: name, Scopes: defaultScopes}
	if spec, ok := l.specs[name]; ok {
		cfg.EnvironmentID = spec.EnvironmentID
		cfg.PlatformToken = spec.PlatformToken
		cfg.ClientID = spec.OAuth.ClientID
		cfg.ClientSecret = spec.OAuth.ClientSecret
		if len(spec.OAuth.Scopes) > 0 {
			cfg.Scopes = spec.OAuth.Scopes
		}
	} else if name != "" {
		return Config{}, fmt.Errorf("environment %q not found in config", name)
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
		return Config{}, fmt.Errorf("missing environment_id for %q (set DT_ENVIRONMENT_ID or %s)", name, l.Path)
	}
	hasPT := cfg.PlatformToken != ""
	hasOAuth := cfg.ClientID != "" && cfg.ClientSecret != ""
	if !hasPT && !hasOAuth {
		return Config{}, fmt.Errorf("missing credentials for %q: set platform_token (DT_PLATFORM_TOKEN) or oauth.client_id/client_secret (DT_OAUTH_CLIENT_ID/SECRET) in %s", name, l.Path)
	}
	if cfg.Name == "" {
		cfg.Name = "default"
	}
	return cfg, nil
}

// decodeEnvs walks the `environments:` mapping node and preserves key order.
// Returns nil/nil when the node is empty (no `environments:` block in the file).
func decodeEnvs(node yaml.Node) ([]string, map[string]envSpec, error) {
	if node.Kind == 0 {
		return nil, nil, nil
	}
	if node.Kind != yaml.MappingNode {
		return nil, nil, fmt.Errorf("`environments` must be a mapping, got kind %d", node.Kind)
	}
	names := make([]string, 0, len(node.Content)/2)
	specs := make(map[string]envSpec, len(node.Content)/2)
	for i := 0; i+1 < len(node.Content); i += 2 {
		key := node.Content[i].Value
		var spec envSpec
		if err := node.Content[i+1].Decode(&spec); err != nil {
			return nil, nil, fmt.Errorf("environment %q: %w", key, err)
		}
		names = append(names, key)
		specs[key] = spec
	}
	return names, specs, nil
}
