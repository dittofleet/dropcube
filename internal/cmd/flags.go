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

// extractValueFlag removes `--name value` or `--name=value` from args and
// returns the remaining args plus the value ("" when the flag is absent).
// A flag given with an empty value is an error rather than reading as
// absent: `--to "$UNSET"` must not quietly mean the default.
func extractValueFlag(args []string, name string) (rest []string, value string, err error) {
	rest = make([]string, 0, len(args))
	flag := "--" + name
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == flag:
			if i+1 == len(args) || args[i+1] == "" {
				return nil, "", fmt.Errorf("%s needs a value", flag)
			}
			i++
			value = args[i]
		case strings.HasPrefix(a, flag+"="):
			if value = strings.TrimPrefix(a, flag+"="); value == "" {
				return nil, "", fmt.Errorf("%s needs a value", flag)
			}
		default:
			rest = append(rest, a)
		}
	}
	return rest, value, nil
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
