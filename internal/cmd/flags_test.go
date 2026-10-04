package cmd

import (
	"strings"
	"testing"
)

func TestExtractValueFlag(t *testing.T) {
	for _, c := range []struct {
		args       []string
		rest, want string
	}{
		{[]string{"--to", "work", "a.txt"}, "a.txt", "work"},
		{[]string{"a.txt", "--to=work", "b.txt"}, "a.txt b.txt", "work"},
		{[]string{"a.txt"}, "a.txt", ""},
	} {
		rest, got, err := extractValueFlag(c.args, "to")
		if err != nil || got != c.want || strings.Join(rest, " ") != c.rest {
			t.Errorf("%q: rest %q, value %q, err %v", c.args, rest, got, err)
		}
	}
	for _, args := range [][]string{
		{"a.txt", "--to"},
		{"--to=", "a.txt"},
		{"--to", "", "a.txt"},
		{"--to", "--yes", "a.txt"},
		{"--to", "a", "--to=b", "a.txt"},
	} {
		if _, _, err := extractValueFlag(args, "to"); err == nil {
			t.Errorf("%q: should be an error", args)
		}
	}
}
