package cmd

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	clikit "github.com/dittofleet/go-cli-kit"
	"github.com/dittofleet/go-cli-kit/postinstall"
	"golang.org/x/term"

	"github.com/dittofleet/dropcube/internal/config"
)

// Postinstall is the install script's first-time setup. The endpoint and
// token come from DROPCUBE_ENDPOINT and DROPCUBE_TOKEN (making a fresh
// remote machine a single curl-pipe), or from asking when there is a
// terminal and no config yet. Without them a starter config is written to
// fill in by hand. A config already there is only replaced from the
// environment.
func Postinstall(a clikit.App) error {
	return postinstall.Run(a, func() error {
		path := config.Path()
		_, err := os.Stat(path)
		exists := err == nil

		endpoint, token := os.Getenv("DROPCUBE_ENDPOINT"), os.Getenv("DROPCUBE_TOKEN")
		stdin := int(os.Stdin.Fd())
		if endpoint == "" && token == "" && !exists && term.IsTerminal(stdin) {
			fmt.Print("Endpoint URL (empty to fill in later): ")
			line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
			if endpoint = strings.TrimSpace(line); endpoint != "" {
				fmt.Print("API token: ")
				b, err := term.ReadPassword(stdin)
				fmt.Println()
				if err != nil {
					return err
				}
				token = strings.TrimSpace(string(b))
			}
		}

		complete := endpoint != "" && token != ""
		if exists && !complete {
			return nil
		}
		if err := writeConfig(path, endpoint, token); err != nil {
			return err
		}
		if complete {
			fmt.Printf("Wrote config to %s\n", path)
		} else {
			fmt.Printf("Created starter config at %s, fill in endpoint and token\n", path)
		}
		return nil
	})
}

// writeConfig writes a config with the given endpoint and token, or the
// starter placeholders for whichever is missing.
func writeConfig(path, endpoint, token string) error {
	var starter struct {
		SchemaVersion int    `json:"schemaVersion"`
		Endpoint      string `json:"endpoint"`
		Token         string `json:"token"`
	}
	if err := json.Unmarshal([]byte(config.StarterConfig), &starter); err != nil {
		return err
	}
	if endpoint != "" {
		starter.Endpoint = endpoint
	}
	if token != "" {
		starter.Token = token
	}
	// Not escaped, so the placeholders read as <your dropcube domain>.
	var data bytes.Buffer
	enc := json.NewEncoder(&data)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(starter); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data.Bytes(), 0o600)
}
