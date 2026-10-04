package cmd

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/dittofleet/dropcube/internal/config"
)

// recorder is a stand-in worker that remembers the requests it gets.
type recorder struct {
	*httptest.Server
	got []string
}

func newRecorder(t *testing.T) *recorder {
	r := &recorder{}
	r.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		r.got = append(r.got, req.Method+" "+req.URL.EscapedPath()+" "+req.Header.Get("Authorization"))
	}))
	t.Cleanup(r.Close)
	return r
}

func deployment(t *testing.T, endpoint, token string) config.Deployment {
	u, err := url.Parse(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	return config.Deployment{Endpoint: endpoint, Token: token, URL: u}
}

func TestActOnLinkRoutesToTheLinksDeployment(t *testing.T) {
	main, private := newRecorder(t), newRecorder(t)
	priv := deployment(t, private.URL, "tok-private")
	cfg := &config.Config{
		Deployment:  deployment(t, main.URL, "tok-main"),
		Deployments: map[string]*config.Deployment{"private": &priv},
	}

	for _, c := range []struct{ link, action string }{
		{main.URL + "/f/0123456789abcdef/a%20b.txt", "keep"},
		{private.URL + "/f/fedcba9876543210/", "remove"},
	} {
		if err := actOnLink(http.DefaultClient, cfg, c.link, c.action); err != nil {
			t.Fatalf("%s %s: %v", c.action, c.link, err)
		}
	}

	want := []string{"POST /keep/0123456789abcdef/a%20b.txt Bearer tok-main"}
	if strings.Join(main.got, "\n") != strings.Join(want, "\n") {
		t.Errorf("main got %q, want %q", main.got, want)
	}
	want = []string{"POST /remove/fedcba9876543210 Bearer tok-private"}
	if strings.Join(private.got, "\n") != strings.Join(want, "\n") {
		t.Errorf("private got %q, want %q", private.got, want)
	}
}

func TestActOnLinkRefusesOtherLinks(t *testing.T) {
	main := newRecorder(t)
	cfg := &config.Config{Deployment: deployment(t, main.URL, "tok-main")}
	for link, want := range map[string]string{
		"https://elsewhere.example/f/0123456789abcdef/a.txt": "not a link from",
		main.URL + "/other/0123456789abcdef":                 "not a dropcube link",
		main.URL + "/f/":                                     "not a dropcube link",
		"no link at all":                                     "not a link",
	} {
		err := actOnLink(http.DefaultClient, cfg, link, "remove")
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: err = %v, want %q", link, err, want)
		}
	}
	if len(main.got) != 0 {
		t.Errorf("refused links still reached the worker: %q", main.got)
	}
}
