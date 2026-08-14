package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/dittofleet/dropcube/internal/xdg"
)

const SchemaVersion = 1

type Config struct {
	SchemaVersion int    `json:"schemaVersion"`
	Endpoint      string `json:"endpoint"`
	Token         string `json:"token"`

	// URL is Endpoint parsed and validated, set by Load.
	URL *url.URL `json:"-"`
}

func Path() string {
	return filepath.Join(xdg.ConfigDir("dropcube"), "config.json")
}

// StarterConfig is the recommended starter content for a fresh install.
// main prints this when Load returns a NotConfiguredError.
const StarterConfig = `{
  "schemaVersion": 1,
  "endpoint": "https://dropcube.<your-subdomain>.workers.dev",
  "token": "<upload token>"
}`

// NotConfiguredError is returned by Load when no usable config exists,
// either because the file is absent (and the environment provides
// nothing) or because it still holds unfilled placeholder values.
// main detects it via errors.As to print starter UX.
type NotConfiguredError struct {
	Path        string
	Placeholder bool
}

func (e *NotConfiguredError) Error() string {
	if e.Placeholder {
		return fmt.Sprintf("config at %s still has placeholder values", e.Path)
	}
	return fmt.Sprintf("config not found at %s", e.Path)
}

// Load reads the config file and overlays the DROPCUBE_ENDPOINT /
// DROPCUBE_TOKEN env vars on top. The file is optional when the
// environment supplies the values.
func Load() (*Config, error) {
	path := Path()
	cfg := &Config{}

	data, err := os.ReadFile(path)
	fileExists := err == nil
	switch {
	case fileExists:
		if err := json.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("failed to parse %s: %w", path, err)
		}
		if cfg.SchemaVersion != SchemaVersion {
			return nil, fmt.Errorf("invalid %s:\n  - schemaVersion: expected %d, got %d", path, SchemaVersion, cfg.SchemaVersion)
		}
	case !errors.Is(err, fs.ErrNotExist):
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	if v := os.Getenv("DROPCUBE_ENDPOINT"); v != "" {
		cfg.Endpoint = v
	}
	if v := os.Getenv("DROPCUBE_TOKEN"); v != "" {
		cfg.Token = v
	}
	if !fileExists && cfg.Endpoint == "" && cfg.Token == "" {
		return nil, &NotConfiguredError{Path: path}
	}
	if err := cfg.validate(path); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Config) validate(path string) error {
	// Real endpoints and tokens never contain angle brackets, so any
	// <...> span means an unfilled placeholder, whichever starter text
	// (install.sh, README, StarterConfig) it was copied from.
	if strings.ContainsAny(c.Endpoint, "<>") || strings.ContainsAny(c.Token, "<>") {
		return &NotConfiguredError{Path: path, Placeholder: true}
	}
	u, err := url.Parse(c.Endpoint)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("invalid %s:\n  - endpoint: must be an http(s) URL", path)
	}
	if c.Token == "" {
		return fmt.Errorf("invalid %s:\n  - token: missing", path)
	}
	c.URL = u
	return nil
}
