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
	for _, k := range []string{"DROPCUBE_ENDPOINT", "DROPCUBE_TOKEN"} {
		t.Setenv(k, "")
	}
	if err := os.MkdirAll(filepath.Join(dir, "dropcube"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "dropcube", "config.json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestDeploymentsDefaultToTheMainToken(t *testing.T) {
	writeConfig(t, `{"schemaVersion":1,"endpoint":"https://a.example","token":"main",
		"deployments":{"work":{"endpoint":"https://c.example","token":"w"},"private":{"endpoint":"https://b.example","description":"login"}}}`)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, d := range cfg.All() {
		got = append(got, d.Name+" "+d.URL.Host+" "+d.Token+" "+d.Description)
	}
	want := []string{"default a.example main ", "private b.example main login", "work c.example w "}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("got %q, want %q", got, want)
	}
	if d, err := cfg.Find("work"); err != nil || d.Token != "w" {
		t.Fatalf("Find(work) = %+v, %v", d, err)
	}
	if _, err := cfg.Find("nope"); err == nil || !strings.Contains(err.Error(), "default, private, work") {
		t.Fatalf("Find(nope) err = %v", err)
	}
}

func TestDeploymentPlaceholderNamesTheSection(t *testing.T) {
	writeConfig(t, `{"schemaVersion":1,"endpoint":"https://a.example","token":"main","deployments":{"private":{"endpoint":"https://<your private domain>"}}}`)
	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "deployments.private: still has placeholder values") {
		t.Fatalf("err = %v", err)
	}
	if _, ok := err.(*NotConfiguredError); ok {
		t.Fatal("a deployment placeholder should not print the starter config")
	}
}

func TestUnknownKeysAreErrors(t *testing.T) {
	for _, body := range []string{
		`{"schemaVersion":1,"endpoint":"https://a.example","token":"main","private":{"endpoint":"https://b.example"}}`,
		`{"schemaVersion":1,"endpoint":"https://a.example","token":"main","deploymnets":{}}`,
	} {
		writeConfig(t, body)
		if _, err := Load(); err == nil || !strings.Contains(err.Error(), "unknown field") {
			t.Errorf("%s: err = %v", body, err)
		}
	}
}

func TestDeploymentsCannotShareAnOrigin(t *testing.T) {
	writeConfig(t, `{"schemaVersion":1,"endpoint":"https://a.example","token":"main","deployments":{"work":{"endpoint":"https://A.example:443/x"}}}`)
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "same endpoint as default") {
		t.Fatalf("err = %v", err)
	}
}

func TestNullDeploymentAndMultilineDescription(t *testing.T) {
	writeConfig(t, `{"schemaVersion":1,"endpoint":"https://a.example","token":"main","deployments":{"work":null}}`)
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "deployments.work: missing endpoint") {
		t.Fatalf("err = %v", err)
	}
	writeConfig(t, `{"schemaVersion":1,"endpoint":"https://a.example","token":"main","description":"Public.\n\tUse  freely."}`)
	cfg, err := Load()
	if err != nil || cfg.Description != "Public. Use freely." {
		t.Fatalf("description = %q, err = %v", cfg.Description, err)
	}
}

func TestDefaultIsNotADeploymentName(t *testing.T) {
	writeConfig(t, `{"schemaVersion":1,"endpoint":"https://a.example","token":"main","deployments":{"default":{"endpoint":"https://b.example"}}}`)
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "deployments.default") {
		t.Fatalf("err = %v", err)
	}
}
