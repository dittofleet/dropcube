package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/dittofleet/dropcube/internal/xdg"
)

const SchemaVersion = 1

// DefaultName names the top-level deployment, the one used unless another
// is asked for.
const DefaultName = "default"

type Config struct {
	SchemaVersion int `json:"schemaVersion"`
	Deployment

	// Deployments are further workers, chosen by name with
	// `upload --to <name>`. Each token defaults to the top-level one.
	Deployments map[string]*Deployment `json:"deployments,omitempty"`
}

// Deployment is one dropcube worker: where it is, the token it takes, and
// what it is for.
type Deployment struct {
	Endpoint string `json:"endpoint"`
	Token    string `json:"token,omitempty"`
	// Description tells agents when to pick this deployment, e.g. "Needs
	// the user's login to view. Use for private files."
	Description string `json:"description,omitempty"`

	// Name and URL are set by Load: the deployment's key in the config
	// (DefaultName for the top-level one) and Endpoint parsed.
	Name string   `json:"-"`
	URL  *url.URL `json:"-"`
}

// All lists every deployment, the default first and the rest by name.
func (c *Config) All() []*Deployment {
	all := []*Deployment{&c.Deployment}
	for _, name := range slices.Sorted(maps.Keys(c.Deployments)) {
		all = append(all, c.Deployments[name])
	}
	return all
}

// Find returns the deployment with the given name.
func (c *Config) Find(name string) (*Deployment, error) {
	if name == DefaultName {
		return &c.Deployment, nil
	}
	if d, ok := c.Deployments[name]; ok {
		return d, nil
	}
	var names []string
	for _, d := range c.All() {
		names = append(names, d.Name)
	}
	return nil, fmt.Errorf("no deployment named %q (have: %s)", name, strings.Join(names, ", "))
}

func Path() string {
	return filepath.Join(xdg.ConfigDir("dropcube"), "config.json")
}

// StarterConfig is the recommended starter content for a fresh install.
// main prints this when Load returns a NotConfiguredError.
const StarterConfig = `{
  "schemaVersion": 1,
  "endpoint": "https://<your dropcube domain>",
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
// DROPCUBE_TOKEN env vars on top. The file is optional when the
// environment supplies the values.
func Load() (*Config, error) {
	path := Path()
	cfg := &Config{}

	data, err := os.ReadFile(path)
	fileExists := err == nil
	switch {
	case fileExists:
		// Unknown keys are errors rather than ignored, so a misspelt or
		// outdated section (like v0.4.0's "private") fails loudly instead
		// of its deployment silently going missing.
		dec := json.NewDecoder(bytes.NewReader(data))
		dec.DisallowUnknownFields()
		if err := dec.Decode(cfg); err != nil {
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
	c.Name = DefaultName
	if err := c.Deployment.validate(path, ""); err != nil {
		return err
	}
	// keep and remove pick a deployment by the host a link came from, so
	// two deployments on one origin would leave them guessing which token
	// to send.
	origins := map[string]string{Origin(c.URL): DefaultName}
	for _, name := range slices.Sorted(maps.Keys(c.Deployments)) {
		d := c.Deployments[name]
		field := "deployments." + name
		switch {
		case name == "" || name == DefaultName:
			return fmt.Errorf("invalid %s:\n  - %s: not a usable deployment name", path, field)
		case d == nil:
			return fmt.Errorf("invalid %s:\n  - %s: missing endpoint", path, field)
		}
		d.Name = name
		if d.Token == "" {
			d.Token = c.Token
		}
		if err := d.validate(path, field+"."); err != nil {
			return err
		}
		if other, ok := origins[Origin(d.URL)]; ok {
			return fmt.Errorf("invalid %s:\n  - %s: same endpoint as %s", path, field, other)
		}
		origins[Origin(d.URL)] = name
	}
	return nil
}

// Origin reduces a URL to the form the worker builds its links from (the JS
// URL.origin): lowercased host, and a port only when it is not the scheme's
// default. Comparing raw hosts would reject links from an endpoint written
// as https://Drop.example or https://drop.example:443.
func Origin(u *url.URL) string {
	host, port := strings.ToLower(u.Hostname()), u.Port()
	if (u.Scheme == "https" && port == "443") || (u.Scheme == "http" && port == "80") {
		port = ""
	}
	if port != "" {
		host = net.JoinHostPort(host, port)
	}
	return u.Scheme + "://" + host
}

func (d *Deployment) validate(path, field string) error {
	// Real endpoints and tokens never contain angle brackets, so any
	// <...> span means an unfilled placeholder, whichever starter text
	// (install.sh, README, StarterConfig) it was copied from.
	if strings.ContainsAny(d.Endpoint, "<>") || strings.ContainsAny(d.Token, "<>") {
		// The starter config has no deployments section, so pointing at
		// it would not help. Name the section instead.
		if field != "" {
			return fmt.Errorf("invalid %s:\n  - %s: still has placeholder values", path, strings.TrimSuffix(field, "."))
		}
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
	// `dropcube deployments` prints one deployment per line.
	d.Description = strings.Join(strings.Fields(d.Description), " ")
	return nil
}
