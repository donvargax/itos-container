package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRecoveryUsageJSONPreferences(t *testing.T) {
	for _, command := range []string{"status", "stop"} {
		for _, tc := range []struct {
			name, inherited string
			flags           []string
			wantJSON        bool
		}{
			{"default", "", nil, false},
			{"inherited", "1", nil, true},
			{"other environment value", "true", nil, false},
			{"enable", "", []string{"--json"}, true},
			{"disable", "1", []string{"--no-json"}, false},
			{"equals true", "", []string{"--json=true"}, true},
			{"equals false", "1", []string{"--json=false"}, false},
			{"last enable", "", []string{"--no-json", "--json=true"}, true},
			{"last disable", "1", []string{"--json=true", "--no-json"}, false},
			{"literal enable", "", []string{"--", "--json=true"}, false},
			{"literal disable", "1", []string{"--", "--json=false"}, true},
		} {
			t.Run(command+"/"+tc.name, func(t *testing.T) {
				t.Setenv("ITOS_JSON", tc.inherited)
				args := append([]string{command}, tc.flags...)
				args = append(args, "--invalid-option")
				var stdout, stderr strings.Builder
				if code := run(args, strings.NewReader(""), &stdout, &stderr); code != 2 || !strings.Contains(stderr.String(), "recovery.usage") {
					t.Fatalf("usage refusal = %d, %q", code, stderr.String())
				}
				if !tc.wantJSON {
					if stdout.Len() != 0 {
						t.Fatalf("plain usage wrote stdout: %q", stdout.String())
					}
					return
				}
				var report struct {
					Schema   int
					OK       bool
					Problems []struct {
						Rule string
						Exit int
					}
					Sessions []json.RawMessage
				}
				if err := json.Unmarshal([]byte(stdout.String()), &report); err != nil || report.Schema != 1 || report.OK || len(report.Problems) != 1 || report.Problems[0].Rule != "recovery.usage" || report.Problems[0].Exit != 2 || report.Sessions == nil || len(report.Sessions) != 0 {
					t.Fatalf("JSON usage refusal = %q (%v)", stdout.String(), err)
				}
			})
		}
	}
}
