package command

import (
	"strings"
	"testing"
)

func TestHelpNamesTheGappedRefusalLogExit(t *testing.T) {
	help := (CLI{}).Help()
	for _, text := range []string{"64-digit", "--json", "--no-json", "ITOS_JSON",
		"recovery.refusal-log-gap", "cannot restore", "already be complete alongside it",
		"repeated stop preserves the diagnosis", "2 means usage/invalid state",
		"3 unavailable", "70 internal failure", "75 temporary uncertainty",
		"retry in the same effective project"} {
		if !strings.Contains(help, text) {
			t.Errorf("help lacks %q", text)
		}
	}
}
