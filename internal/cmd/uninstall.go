package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"

	"github.com/dittofleet/dropcube/internal/app"
	clikit "github.com/dittofleet/go-cli-kit"
	"github.com/dittofleet/go-cli-kit/xdg"
	"golang.org/x/term"
)

const uninstallUsage = "usage: dropcube uninstall [--yes]"

// Uninstall removes the dropcube binary, config directory, and data
// directory. Order is data → config → binary so a failure leaves a
// tool to retry with.
func Uninstall(args []string, a clikit.App) error {
	args, yes := extractBoolFlag(args, "yes")
	if err := rejectUnknownFlags(args, uninstallUsage); err != nil {
		return err
	}
	if len(args) > 0 {
		return fmt.Errorf("unexpected arguments: %v\n%s", args, uninstallUsage)
	}

	if a.IsDev() {
		return errors.New("cannot uninstall a dev build")
	}

	binaryPath, err := clikit.Executable()
	if err != nil {
		return fmt.Errorf("cannot determine binary path: %w", err)
	}

	configDir := xdg.ConfigDir(app.Name)
	dataDir := xdg.DataDir(app.Name)

	fmt.Println("This will remove:")
	fmt.Printf("  - Binary:  %s\n", binaryPath)
	fmt.Printf("  - Config:  %s  (endpoint and API token)\n", configDir)
	fmt.Printf("  - Cache:   %s\n", dataDir)
	fmt.Println()
	fmt.Println("Note: the Cloudflare worker, bucket, and uploaded files are NOT touched.")
	fmt.Println()

	if !yes {
		if !term.IsTerminal(int(os.Stdin.Fd())) {
			return errors.New("refusing to uninstall non-interactively without --yes")
		}
		fmt.Print("Proceed? [y/N]: ")
		reader := bufio.NewReader(os.Stdin)
		line, _ := reader.ReadString('\n')
		answer := strings.ToLower(strings.TrimSpace(line))
		if answer != "y" && answer != "yes" {
			fmt.Println("Aborted.")
			return nil
		}
	}

	steps := []struct {
		label string
		path  string
		fn    func(string) error
	}{
		{"cache directory", dataDir, os.RemoveAll},
		{"config directory", configDir, os.RemoveAll},
		{"binary", binaryPath, os.Remove},
	}
	var removed []string
	for _, s := range steps {
		err := s.fn(s.path)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			if len(removed) > 0 {
				fmt.Fprintf(os.Stderr, "Removed before failure: %s\n", strings.Join(removed, ", "))
			}
			return fmt.Errorf("failed to remove %s (%s): %w", s.label, s.path, err)
		}
		removed = append(removed, s.label)
	}

	fmt.Println("Uninstalled dropcube.")
	return nil
}
