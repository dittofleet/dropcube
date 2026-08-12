package cmd

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const removeUsage = "usage: dropcube remove <link>..."

const removeTimeout = 30 * time.Second

// Remove deletes uploads by their view links. The link is the capability,
// so no config or token is involved.
func Remove(args []string) error {
	if err := rejectUnknownFlags(args, removeUsage); err != nil {
		return err
	}
	if len(args) == 0 {
		return fmt.Errorf("no links given\n%s", removeUsage)
	}

	client := &http.Client{Timeout: removeTimeout}
	var failures []error
	for _, link := range args {
		if err := removeLink(client, link); err != nil {
			failures = append(failures, fmt.Errorf("remove %s: %w", link, err))
		}
	}
	return errors.Join(failures...)
}

func removeLink(client *http.Client, link string) error {
	u, err := url.Parse(strings.TrimSpace(link))
	if err != nil || !u.IsAbs() || u.Host == "" {
		return fmt.Errorf("not a link")
	}

	resp, err := client.Get(strings.TrimSuffix(u.String(), "/") + "/remove")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}
