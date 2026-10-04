package cmd

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/dittofleet/dropcube/internal/config"
)

// Remove deletes uploads by their view links. A link alone only grants
// viewing, so this also sends the configured token.
func Remove(args []string) error {
	return linkAction(args, "remove", 30*time.Second)
}

// Keep stops uploads from expiring. Same link afterwards: only where the
// file is stored changes. Copying the file server-side takes
// longer than a removal does, so allow for it.
func Keep(args []string) error {
	return linkAction(args, "keep", 10*time.Minute)
}

// linkAction runs one of the worker's link actions over every given link.
// A link's /f/<key> becomes an authorized POST to /<action>/<key> on the
// deployment the link came from. The action names itself in the
// usage line and in every error, so the two cannot drift apart.
func linkAction(args []string, action string, timeout time.Duration) error {
	usage := fmt.Sprintf("usage: dropcube %s <link>...", action)
	if err := rejectUnknownFlags(args, usage); err != nil {
		return err
	}
	if len(args) == 0 {
		return fmt.Errorf("no links given\n%s", usage)
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	client := &http.Client{Timeout: timeout}
	var failures []error
	for _, link := range args {
		if err := actOnLink(client, cfg, link, action); err != nil {
			failures = append(failures, fmt.Errorf("%s %s: %w", action, link, err))
		}
	}
	return errors.Join(failures...)
}

func actOnLink(client *http.Client, cfg *config.Config, link, action string) error {
	u, err := url.Parse(strings.TrimSpace(link))
	if err != nil || !u.IsAbs() || u.Host == "" {
		return fmt.Errorf("not a link")
	}
	key, ok := strings.CutPrefix(strings.TrimSuffix(u.EscapedPath(), "/"), "/f/")
	if !ok || key == "" {
		return fmt.Errorf("not a dropcube link")
	}
	// The token goes along with the request, so only ever send it to the
	// worker it belongs to, never to whatever host a pasted link names.
	dep, err := deploymentFor(cfg, u)
	if err != nil {
		return err
	}

	target := origin(dep.URL) + "/" + action + "/" + key
	req, err := http.NewRequest(http.MethodPost, target, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+dep.Token)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if body := readBody(resp); resp.StatusCode != http.StatusOK {
		return httpError(resp.StatusCode, body)
	}
	return nil
}

func deploymentFor(cfg *config.Config, link *url.URL) (*config.Deployment, error) {
	var hosts []string
	for _, d := range cfg.Deployments() {
		if origin(d.URL) == origin(link) {
			return d, nil
		}
		hosts = append(hosts, d.URL.Host)
	}
	return nil, fmt.Errorf("not a link from %s", strings.Join(hosts, " or "))
}

// origin reduces a URL to the form the worker builds its links from (the JS
// URL.origin): lowercased host, and a port only when it is not the scheme's
// default. Comparing raw hosts would reject links from an endpoint written
// as https://Drop.example or https://drop.example:443.
func origin(u *url.URL) string {
	host, port := strings.ToLower(u.Hostname()), u.Port()
	if (u.Scheme == "https" && port == "443") || (u.Scheme == "http" && port == "80") {
		port = ""
	}
	if port != "" {
		host = net.JoinHostPort(host, port)
	}
	return u.Scheme + "://" + host
}
