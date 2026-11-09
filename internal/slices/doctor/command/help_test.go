package command

import (
	"strings"
	"testing"
)

func TestHelpNamesControlStateExitCodes(t *testing.T) {
	help := (CLI{}).Help()
	for _, text := range []string{"Exit codes:", "0  policy validation succeeded", "2  invalid policy or command line", "3  policy cannot be read",
		"exposed to a worker checkout", "retained control state", "approval.state.busy"} {
		if !strings.Contains(help, text) {
			t.Errorf("help lacks %q", text)
		}
	}
}
