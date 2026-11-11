package cmd

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestUnknownExtensionsGoByContents guards text opening in the browser when
// the system doesn't know its extension, without the check costing the
// upload the start of the file.
func TestUnknownExtensionsGoByContents(t *testing.T) {
	type upload struct{ contentType, body string }
	var got upload
	worker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		got = upload{r.Header.Get("Content-Type"), string(body)}
	}))
	t.Cleanup(worker.Close)
	dep := deployment(t, worker.URL, "tok")
	dir := t.TempDir()

	for _, tc := range []struct{ name, body, want string }{
		{"build.dropcubetest", "line one\nline two\n", "text/plain; charset=utf-8"},
		{"long.dropcubetest", strings.Repeat("words ", 1000), "text/plain; charset=utf-8"},
		{"utf16.dropcubetest", "\xff\xfeh\x00i\x00", "text/plain; charset=utf-16le"},
		{"tag.dropcubetest", `<p align="center">hi</p>`, "text/plain; charset=utf-8"},
		{"blob.dropcubetest", "\x00\x01\x02binary", ""},
		{"empty.dropcubetest", "", "text/plain; charset=utf-8"},
		{"page.html", "plain words", "text/html; charset=utf-8"},
	} {
		file := filepath.Join(dir, tc.name)
		os.WriteFile(file, []byte(tc.body), 0o600)
		if _, err := sendFile(http.DefaultClient, &dep, http.MethodPut, worker.URL, file, http.StatusOK); err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if want := (upload{tc.want, tc.body}); got != want {
			t.Errorf("%s: got %+v, want %+v", tc.name, got, want)
		}
	}
}

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
