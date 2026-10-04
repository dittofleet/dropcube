package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeConfig points XDG_CONFIG_HOME at a fresh directory holding body as
// the config file, and clears the env overrides so the host's own settings
// cannot leak into the test.
func writeConfig(t *testing.T, body string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	for _, k := range []string{"DROPCUBE_ENDPOINT", "DROPCUBE_TOKEN", "DROPCUBE_PRIVATE_ENDPOINT", "DROPCUBE_PRIVATE_TOKEN"} {
		t.Setenv(k, "")
	}
	if err := os.MkdirAll(filepath.Join(dir, "dropcube"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "dropcube", "config.json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestPrivateTokenDefaultsToMain(t *testing.T) {
	writeConfig(t, `{"schemaVersion":1,"endpoint":"https://a.example","token":"main","private":{"endpoint":"https://b.example"}}`)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Private.Token != "main" || cfg.Private.URL.Host != "b.example" {
		t.Fatalf("private = %+v", cfg.Private)
	}
	if len(cfg.Deployments()) != 2 {
		t.Fatalf("deployments = %d", len(cfg.Deployments()))
	}
}

func TestPrivateFromEnv(t *testing.T) {
	writeConfig(t, `{"schemaVersion":1,"endpoint":"https://a.example","token":"main"}`)
	t.Setenv("DROPCUBE_PRIVATE_ENDPOINT", "https://b.example")
	t.Setenv("DROPCUBE_PRIVATE_TOKEN", "priv")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Private.Token != "priv" || cfg.Private.URL.Host != "b.example" || cfg.Token != "main" {
		t.Fatalf("cfg = %+v, private = %+v", cfg.Deployment, cfg.Private)
	}
}

func TestPrivateTokenWithoutEndpointIsAnError(t *testing.T) {
	writeConfig(t, `{"schemaVersion":1,"endpoint":"https://a.example","token":"main"}`)
	t.Setenv("DROPCUBE_PRIVATE_TOKEN", "priv")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "private.endpoint") {
		t.Fatalf("err = %v", err)
	}
}

func TestPrivatePlaceholderNamesTheSection(t *testing.T) {
	writeConfig(t, `{"schemaVersion":1,"endpoint":"https://a.example","token":"main","private":{"endpoint":"https://<your private domain>"}}`)
	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "private: still has placeholder values") {
		t.Fatalf("err = %v", err)
	}
	if _, ok := err.(*NotConfiguredError); ok {
		t.Fatal("a private placeholder should not print the starter config")
	}
}
