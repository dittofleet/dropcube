package cmd

import (
	"fmt"
	"strings"
)

// extractBoolFlag walks args, removes the named flag (`--name`) wherever it
// appears, and returns the remaining args plus whether the flag was set.
// Flag position relative to positional args doesn't matter.
//
// Any other `--*` token is left in place so callers can still detect it
// as an unknown flag.
func extractBoolFlag(args []string, name string) (rest []string, set bool) {
	rest = make([]string, 0, len(args))
	flag := "--" + name
	for _, a := range args {
		if a == flag {
			set = true
			continue
		}
		rest = append(rest, a)
	}
	return rest, set
}

// rejectUnknownFlags errors on the first remaining `--*` token. Call it
// after the known flags have been extracted.
func rejectUnknownFlags(args []string, usage string) error {
	for _, a := range args {
		if strings.HasPrefix(a, "--") {
			return fmt.Errorf("unknown flag: %s\n%s", a, usage)
		}
	}
	return nil
}
