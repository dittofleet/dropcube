package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/dittofleet/dropcube/internal/cmd"
	"github.com/dittofleet/dropcube/internal/config"
	"github.com/dittofleet/dropcube/internal/update"
)

var errUnknownCommand = errors.New("unknown command")

var version = "dev"

const usage = `Usage: dropcube <command>

Commands:
  upload <file>...   Upload files, printing one view link per line
                     (links and files expire after 30 days)
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

	if err := dispatch(args); err != nil {
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

	if args[0] != "uninstall" {
		update.MaybeCheck(version)
	}
}

func dispatch(args []string) error {
	switch args[0] {
	case "upload":
		return cmd.Upload(args[1:])
	case "remove":
		return cmd.Remove(args[1:])
	case "update":
		return cmd.SelfUpdate(version)
	case "uninstall":
		return cmd.Uninstall(args[1:], version)
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
