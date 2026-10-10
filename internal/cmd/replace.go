package cmd

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/dittofleet/dropcube/internal/config"
)

const replaceUsage = "usage: dropcube replace <link> <file>"

// Replace swaps the file behind a link for another, so the same link shows
// the new contents.
func Replace(args []string) error {
	if err := rejectUnknownFlags(args, replaceUsage); err != nil {
		return err
	}
	if len(args) != 2 {
		return errors.New(replaceUsage)
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if err := replace(cfg, args[0], args[1]); err != nil {
		return fmt.Errorf("replace %s: %w", args[0], err)
	}
	return nil
}

func replace(cfg *config.Config, link, path string) error {
	dep, target, err := actionTarget(cfg, link, "replace")
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: uploadTimeout}
	_, err = sendFile(client, dep, http.MethodPost, target, path, http.StatusOK)
	return err
}
