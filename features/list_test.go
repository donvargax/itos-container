package features

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
)

var scenarioListOutput = flag.String("scenario-list-output", "", "write eligible live scenario TSV for code proof without executing steps")

// TestScenarioList is the entry point of tools/bin/scenario-list. The execution
// selector supplies eligible IDs; core itos still supplies files/fingerprints.
func TestScenarioList(t *testing.T) {
	if *scenarioListOutput == "" {
		t.Skip("invoked only by tools/bin/scenario-list")
	}
	env := readFeatureEnvironment(os.Getenv, runtime.GOOS, runtime.GOARCH)
	if env.redRequested() {
		t.Fatal("code proof must list live scenarios, never manual red selections")
	}
	selection, err := selectScenarios("", ".", env)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("itos", "tests", "list", "scenario", "--tsv")
	cmd.Stderr = os.Stderr
	listed, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	output, err := eligibleScenarioRows(listed, selection.ids)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(*scenarioListOutput, output, 0600); err != nil {
		t.Fatal(err)
	}
}

func eligibleScenarioRows(listed []byte, ids []string) ([]byte, error) {
	pending := make(map[string]bool, len(ids))
	for _, id := range ids {
		pending[strings.TrimPrefix(id, "@")] = true
	}
	var output bytes.Buffer
	for line := range strings.SplitSeq(strings.TrimSuffix(string(listed), "\n"), "\n") {
		id, _, ok := strings.Cut(line, "\t")
		if !ok {
			return nil, fmt.Errorf("invalid scenario TSV row %q", line)
		}
		if pending[id] {
			output.WriteString(line + "\n")
			delete(pending, id)
		}
	}
	if len(pending) != 0 {
		return nil, fmt.Errorf("itos listing lacks %d eligible scenario IDs", len(pending))
	}
	return output.Bytes(), nil
}

func TestScenarioListMatchesExecution(t *testing.T) {
	dir := writeFeatureFixture(t, `Feature: discovery
  @ID-MEMORY-40
  Scenario: portable
    Then record "portable"
  @ID-MEMORY-41 @local-engine-fixture
  Scenario: local socket
    Then record "local"
  @ID-MEMORY-42 @unsupported-host @manual-platform
  Scenario: native non-Linux
    Then record "unsupported"
  @ID-MEMORY-43 @engine
  Scenario: actual engine
    Then record "engine"
  @ID-MEMORY-44 @wip
  Scenario: pending
    Then record "pending"
`)
	for _, tc := range []struct {
		env  featureEnvironment
		want []string
	}{
		{featureEnvironment{os: "linux", arch: "amd64"}, []string{"@ID-MEMORY-40", "@ID-MEMORY-41"}},
		{featureEnvironment{os: "darwin", arch: "arm64"}, []string{"@ID-MEMORY-40", "@ID-MEMORY-42"}},
		{featureEnvironment{os: "windows", arch: "amd64"}, []string{"@ID-MEMORY-40", "@ID-MEMORY-42"}},
		{featureEnvironment{os: "linux", arch: "amd64", engine: "1", ci: "true", actions: "true"}, []string{"@ID-MEMORY-43"}},
	} {
		t.Run(tc.env.os+tc.env.engine, func(t *testing.T) {
			selection, err := selectScenarios("", dir, tc.env)
			if err != nil {
				t.Fatal(err)
			}
			var listed, want strings.Builder
			for _, id := range []string{"@ID-MEMORY-40", "@ID-MEMORY-41", "@ID-MEMORY-42", "@ID-MEMORY-43"} {
				fmt.Fprintf(&listed, "%s\tfeatures/fixture.feature\tfingerprint-%s\n", id[1:], id)
			}
			for _, id := range tc.want {
				fmt.Fprintf(&want, "%s\tfeatures/fixture.feature\tfingerprint-%s\n", id[1:], id)
			}
			got, err := eligibleScenarioRows([]byte(listed.String()), selection.ids)
			if err != nil || string(got) != want.String() {
				t.Fatalf("list = %q, %v; want %q", got, err, want.String())
			}
			seen := executeMemory(t, dir, selection, nil)
			if len(seen.steps) != len(tc.want) {
				t.Fatalf("discovery and execution disagree: %+v", seen)
			}
		})
	}
}

func TestScenarioListRefusesMissingInventory(t *testing.T) {
	for _, data := range []string{"broken", "ID-MEMORY-40\tfeatures/fixture.feature\tfingerprint\n"} {
		if got, err := eligibleScenarioRows([]byte(data), []string{"@ID-MEMORY-41"}); err == nil || got != nil {
			t.Fatalf("incomplete listing returned %q, %v", got, err)
		}
	}
}
