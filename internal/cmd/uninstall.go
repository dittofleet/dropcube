package cmd

import (
	"fmt"
	"os"

	"github.com/dittofleet/dropcube/internal/app"
	clikit "github.com/dittofleet/go-cli-kit"
	"github.com/dittofleet/go-cli-kit/uninstall"
	"github.com/dittofleet/go-cli-kit/xdg"
)

const uninstallUsage = "usage: dropcube uninstall [--yes]"

// Uninstall removes the dropcube binary, config directory, and data
// directory.
func Uninstall(args []string, a clikit.App) error {
	args, yes := extractBoolFlag(args, "yes")
	if err := rejectUnknownFlags(args, uninstallUsage); err != nil {
		return err
	}
	if len(args) > 0 {
		return fmt.Errorf("unexpected arguments: %v\n%s", args, uninstallUsage)
	}
	return uninstall.Run(a, yes, uninstall.Plan{
		Items: []uninstall.Item{
			{Label: "Cache", Path: xdg.DataDir(app.Name), Remove: os.RemoveAll},
			{Label: "Config", Path: xdg.ConfigDir(app.Name), Note: "endpoint and API token", Remove: os.RemoveAll},
		},
		Notice: "Note: the Cloudflare worker, bucket, and uploaded files are NOT touched.",
	})
}
