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
	SchemaVersion int `json:"schemaVersion"`
	Deployment

	// Private is an optional second deployment, the one `upload --private`
	// sends to. Its token defaults to the main one.
	Private *Deployment `json:"private,omitempty"`
}

// Deployment is one dropcube worker: where it is and the token it takes.
type Deployment struct {
	Endpoint string `json:"endpoint"`
	Token    string `json:"token,omitempty"`

	// URL is Endpoint parsed and validated, set by Load.
	URL *url.URL `json:"-"`
}

// Deployments lists every configured deployment, the main one first.
func (c *Config) Deployments() []*Deployment {
	if c.Private == nil {
		return []*Deployment{&c.Deployment}
	}
	return []*Deployment{&c.Deployment, c.Private}
}

func Path() string {
	return filepath.Join(xdg.ConfigDir("dropcube"), "config.json")
}

// StarterConfig is the recommended starter content for a fresh install.
// main prints this when Load returns a NotConfiguredError.
const StarterConfig = `{
  "schemaVersion": 1,
  "endpoint": "https://dropcube.<your-subdomain>.workers.dev",
  "token": "<API token>"
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
// DROPCUBE_TOKEN env vars on top, and DROPCUBE_PRIVATE_ENDPOINT /
// DROPCUBE_PRIVATE_TOKEN for the private deployment. The file is optional when the
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
	if v := os.Getenv("DROPCUBE_PRIVATE_ENDPOINT"); v != "" {
		if cfg.Private == nil {
			cfg.Private = &Deployment{}
		}
		cfg.Private.Endpoint = v
	}
	if v := os.Getenv("DROPCUBE_PRIVATE_TOKEN"); v != "" && cfg.Private != nil {
		cfg.Private.Token = v
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
	if err := c.Deployment.validate(path, ""); err != nil {
		return err
	}
	if c.Private != nil {
		if c.Private.Token == "" {
			c.Private.Token = c.Token
		}
		if err := c.Private.validate(path, "private."); err != nil {
			return err
		}
	}
	return nil
}

func (d *Deployment) validate(path, field string) error {
	// Real endpoints and tokens never contain angle brackets, so any
	// <...> span means an unfilled placeholder, whichever starter text
	// (install.sh, README, StarterConfig) it was copied from.
	if strings.ContainsAny(d.Endpoint, "<>") || strings.ContainsAny(d.Token, "<>") {
		return &NotConfiguredError{Path: path, Placeholder: true}
	}
	u, err := url.Parse(d.Endpoint)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("invalid %s:\n  - %sendpoint: must be an http(s) URL", path, field)
	}
	if d.Token == "" {
		return fmt.Errorf("invalid %s:\n  - %stoken: missing", path, field)
	}
	d.URL = u
	return nil
}
