package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/dittofleet/dropcube/internal/app"
	"github.com/dittofleet/dropcube/internal/cmd"
	"github.com/dittofleet/dropcube/internal/config"
	clikit "github.com/dittofleet/go-cli-kit"
	"github.com/dittofleet/go-cli-kit/selfupdate"
	"github.com/dittofleet/go-cli-kit/updatecheck"
)

var errUnknownCommand = errors.New("unknown command")

var version = "dev"

const usage = `Usage: dropcube <command>

Commands:
  upload [--to <deployment>] <file>...
                     Upload files, printing one view link per line
                     (links and files expire after 30 days). --to picks
                     a deployment other than the default
  deployments        List the deployments --to can pick, with what each is for
  keep <link>...     Stop uploads expiring, keeping the same links
  remove <link>...   Delete uploads by their view links
  version            Print the installed version
  update             Download and install the latest version
  uninstall [--yes]  Remove binary, config, and update cache
  help               Print this help message
`

func printUsage() {
	fmt.Print(usage)
}

func main() {
	args := os.Args[1:]

	if len(args) == 0 {
		printUsage()
		os.Exit(0)
	}

	dropcube := app.New(version)
	if err := dispatch(dropcube, args); err != nil {
		var notConfigured *config.NotConfiguredError
		switch {
		case errors.Is(err, errUnknownCommand):
			printUsage()
		case errors.As(err, &notConfigured):
			fmt.Fprintln(os.Stderr, "Error:", err)
			fmt.Fprintln(os.Stderr, "It should look like this, with your endpoint and token filled in (or set DROPCUBE_ENDPOINT and DROPCUBE_TOKEN):")
			fmt.Fprintln(os.Stderr)
			fmt.Fprintln(os.Stderr, config.StarterConfig)
		default:
			fmt.Fprintln(os.Stderr, "Error:", err)
		}
		os.Exit(1)
	}

	updatecheck.MaybeCheck(dropcube, args[0])
}

func dispatch(dropcube clikit.App, args []string) error {
	switch args[0] {
	case "upload":
		return cmd.Upload(args[1:])
	case "keep":
		return cmd.Keep(args[1:])
	case "remove":
		return cmd.Remove(args[1:])
	case "deployments":
		return cmd.Deployments(args[1:])
	case "update":
		_, err := selfupdate.Run(dropcube)
		return err
	case "uninstall":
		return cmd.Uninstall(args[1:], dropcube)
	case "version", "--version", "-v":
		fmt.Println(version)
		return nil
	case "help", "--help", "-h":
		printUsage()
		return nil
	default:
		return errUnknownCommand
	}
}
