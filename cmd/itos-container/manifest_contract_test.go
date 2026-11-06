package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/donvargax/itos-container/internal/cli"
	"github.com/donvargax/itos-container/internal/engine/manifest/domain"
	"github.com/donvargax/itos-container/internal/engine/manifest/jsonc"
	"github.com/donvargax/itos-container/internal/engine/manifest/port"
	"github.com/donvargax/itos-container/internal/engine/manifest/port/porttest"
	"github.com/donvargax/itos-container/internal/engine/manifest/reference"
	doctor "github.com/donvargax/itos-container/internal/slices/doctor/command"
)

// TestManifestContractProcess is a test-binary assembly, never a product input
// channel. The feature runner sends raw data and reads the real presenter's
// output. File data is in memory; the parser, reference validator and domain are
// the same production components used by doctor. No engine or metadata flag is
// exposed by the product binary.
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
	parser := jsonc.Parser{}
	result := domain.Inspect(input.Path, &porttest.Files{Data: map[string][]byte{input.Path: []byte(input.Manifest)}}, parser, reference.References{})
	label := port.Label{Present: input.Label != nil}
	if input.Label != nil {
		label.Text = *input.Label
	}
	result = domain.WithMetadata(result, label, parser)
	os.Exit(doctor.PresentManifest(&cli.UI{Stdout: os.Stdout, Stderr: os.Stderr}, result, input.JSON))
}

func TestOrdinaryDoctorRefusesMissingSelectedManifest(t *testing.T) {
	root := t.TempDir()
	// The subject rejects aliased checkout ancestors. macOS temp directories
	// commonly arrive through /var; give this missing-manifest case a real root.
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("ITOS_ROOT", root)
	if err := os.WriteFile(filepath.Join(root, "itos-container.yaml"), []byte("version: 1\nmanifest: selected.json\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	var stdout, stderr bytes.Buffer
	if code := run([]string{"doctor", "--json"}, nil, &stdout, &stderr); code != 2 {
		t.Fatalf("exit %d: %s", code, stdout.String())
	}
	var result struct {
		Scope    string
		Manifest struct{ Valid bool }
		Problems []struct{ Rule, File string }
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Scope != "manifest" || result.Manifest.Valid || len(result.Problems) != 1 || result.Problems[0].Rule != "manifest.missing" || result.Problems[0].File != "selected.json" || !strings.Contains(stderr.String(), "selected.json") {
		t.Fatalf("%#v, %s", result, stderr.String())
	}
}
