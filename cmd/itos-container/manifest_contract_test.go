package main

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/donvargax/itos-container/internal/cli"
	policy "github.com/donvargax/itos-container/internal/engine/policy/port"
	doctor "github.com/donvargax/itos-container/internal/slices/doctor/command"
	"github.com/donvargax/itos-container/internal/slices/doctor/port/porttest"
)

// TestManifestContractProcess is a test-binary assembly, never a product input
// channel. The feature runner sends raw data and reads the real presenter's
// output. At the red baseline only policy inspection exists; this assembly will
// bind the manifest validator when its production ports and adapters land.
func TestManifestContractProcess(t *testing.T) {
	if os.Getenv("SLICE_2_CONTRACT") != "1" {
		t.Skip("used only by the manifest contract scenarios")
	}
	var input struct {
		Path     string
		Manifest string
		Label    *string
		JSON     bool
	}
	if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil {
		t.Fatal(err)
	}
	command := doctor.CLI{
		Policies: &porttest.Policies{Result: policy.Result{Policy: &policy.Policy{
			Version: 1, Manifest: input.Path, Network: policy.Network{Mode: "offline"},
		}}},
		JSON: input.JSON,
	}
	os.Exit(command.Run(&cli.UI{Stdout: os.Stdout, Stderr: os.Stderr}))
}
