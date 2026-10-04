package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/dittofleet/dropcube/internal/config"
)

const deploymentsUsage = "usage: dropcube deployments"

// Deployments lists the configured deployments, one per line: the name to
// pass to `upload --to`, the host its links come from, and its description.
func Deployments(args []string) error {
	if len(args) > 0 {
		return fmt.Errorf("unexpected arguments\n%s", deploymentsUsage)
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	for _, d := range cfg.All() {
		fmt.Fprintf(w, "%s\t%s\t%s\n", d.Name, d.URL.Host, d.Description)
	}
	return w.Flush()
}
