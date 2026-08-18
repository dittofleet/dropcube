package cmd

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Remove deletes uploads by their view links. The link is the capability,
// so no config or token is involved.
func Remove(args []string) error {
	return linkAction(args, "remove", 30*time.Second)
}

// Keep stops uploads from expiring. Same capability, same link afterwards:
// only where the file is stored changes. Copying the file server-side takes
// longer than a removal does, so allow for it.
func Keep(args []string) error {
	return linkAction(args, "keep", 10*time.Minute)
}

// linkAction runs one of the worker's link actions over every given link,
// each a GET of the link plus "/<action>". The action names itself in the
// usage line and in every error, so the two cannot drift apart.
func linkAction(args []string, action string, timeout time.Duration) error {
	usage := fmt.Sprintf("usage: dropcube %s <link>...", action)
	if err := rejectUnknownFlags(args, usage); err != nil {
		return err
	}
	if len(args) == 0 {
		return fmt.Errorf("no links given\n%s", usage)
	}

	client := &http.Client{Timeout: timeout}
	var failures []error
	for _, link := range args {
		if err := actOnLink(client, link, action); err != nil {
			failures = append(failures, fmt.Errorf("%s %s: %w", action, link, err))
		}
	}
	return errors.Join(failures...)
}

func actOnLink(client *http.Client, link, action string) error {
	u, err := url.Parse(strings.TrimSpace(link))
	if err != nil || !u.IsAbs() || u.Host == "" {
		return fmt.Errorf("not a link")
	}

	resp, err := client.Get(strings.TrimSuffix(u.String(), "/") + "/" + action)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if body := readBody(resp); resp.StatusCode != http.StatusOK {
		return httpError(resp.StatusCode, body)
	}
	return nil
}
