package cmd

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// TestUploadToSendsToTheNamedDeployment guards the routing that keeps a
// file meant for one deployment off another: --to picks the deployment,
// and its token falls back to the top-level one.
func TestUploadToSendsToTheNamedDeployment(t *testing.T) {
	worker := func(got *[]string) *httptest.Server {
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			io.Copy(io.Discard, r.Body)
			*got = append(*got, r.Method+" "+r.URL.Path+" "+r.Header.Get("Authorization"))
			w.WriteHeader(http.StatusCreated)
			fmt.Fprintf(w, "http://%s/f/0123456789abcdef%s\n", r.Host, r.URL.Path)
		}))
		t.Cleanup(s.Close)
		return s
	}
	var mainGot, privateGot []string
	main, private := worker(&mainGot), worker(&privateGot)

	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("DROPCUBE_ENDPOINT", "")
	t.Setenv("DROPCUBE_TOKEN", "")
	os.MkdirAll(filepath.Join(dir, "dropcube"), 0o755)
	cfg := fmt.Sprintf(`{"schemaVersion":1,"endpoint":%q,"token":"tok-main","deployments":{"private":{"endpoint":%q}}}`, main.URL, private.URL)
	os.WriteFile(filepath.Join(dir, "dropcube", "config.json"), []byte(cfg), 0o600)
	file := filepath.Join(dir, "a.txt")
	os.WriteFile(file, []byte("hi"), 0o600)

	for _, args := range [][]string{{"--to", "private", file}, {file}} {
		if err := Upload(args); err != nil {
			t.Fatalf("%q: %v", args, err)
		}
	}
	if want := "PUT /a.txt Bearer tok-main"; len(privateGot) != 1 || privateGot[0] != want {
		t.Errorf("private got %q, want [%q]", privateGot, want)
	}
	if want := "PUT /a.txt Bearer tok-main"; len(mainGot) != 1 || mainGot[0] != want {
		t.Errorf("main got %q, want [%q]", mainGot, want)
	}
}
