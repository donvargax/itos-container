package command

import (
	"strings"
	"testing"
)

func TestHelpNamesTheGappedRefusalLogExit(t *testing.T) {
	help := (CLI{}).Help()
	for _, text := range []string{"no positional", "--json", "--no-json", "ITOS_JSON",
		"recovery.refusal-log-gap", "cannot restore", "cleanup may be complete alongside it",
		"2 usage/invalid state", "3 unavailable", "70 internal failure", "75 temporary uncertainty"} {
		if !strings.Contains(help, text) {
			t.Errorf("help lacks %q", text)
		}
	}
}
