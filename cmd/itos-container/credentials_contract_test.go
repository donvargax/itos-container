package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
	"github.com/donvargax/itos-container/internal/cli"
	"github.com/donvargax/itos-container/internal/engine/manifest/jsonc"
	manifestfake "github.com/donvargax/itos-container/internal/engine/manifest/port/porttest"
	"github.com/donvargax/itos-container/internal/engine/manifest/reference"
	policyidna "github.com/donvargax/itos-container/internal/engine/policy/idna"
	policyfake "github.com/donvargax/itos-container/internal/engine/policy/port/porttest"
	policyyaml "github.com/donvargax/itos-container/internal/engine/policy/yaml"
	readinessdisk "github.com/donvargax/itos-container/internal/engine/readiness/disk"
	readiness "github.com/donvargax/itos-container/internal/engine/readiness/domain"
	readinessport "github.com/donvargax/itos-container/internal/engine/readiness/port"
	readinessfake "github.com/donvargax/itos-container/internal/engine/readiness/port/porttest"
	"github.com/donvargax/itos-container/internal/slices/doctor/inspection"
)

// The process exposes only test input and a separate operation journal. Both
// file-only and metadata scopes go through the same real command dispatcher.
func TestCredentialContractProcess(t *testing.T) {
	if os.Getenv("SLICE_29_CONTRACT") != "1" {
		t.Skip("used only by the credential observation scenarios")
	}
	var request struct {
		Policy, Manifest, Argv string
		readinessfake.CredentialFiles
	}
	if err := json.NewDecoder(os.Stdin).Decode(&request); err != nil {
		t.Fatal(err)
	}
	request.Parser = readinessdisk.ParseCredentialMounts
	request.Calls = map[string]int{}
	code := credentialContractCommand(request.Policy, request.Manifest, request.Argv, &request.CredentialFiles)
	log, err := json.Marshal(struct {
		Calls     map[string]int
		Completed string
	}{request.Calls, request.Completed})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(os.Getenv("SLICE_29_OBSERVATIONS"), log, 0o600); err != nil {
		t.Fatal(err)
	}
	os.Exit(code)
}

func credentialContractCommand(policy, manifest, argv string, files *readinessfake.CredentialFiles) int {
	ui := &cli.UI{Stdout: os.Stdout, Stderr: os.Stderr}
	var c commandLine
	parser, err := kong.New(&c, append(cli.Flags(), kong.Writers(os.Stdout, os.Stderr))...)
	if err != nil {
		return ui.Fail(err, true)
	}
	if _, err := parser.Parse(strings.Fields(argv)); err != nil {
		return ui.Usage("doctor", err, true)
	}
	c.Doctor.Policies = inspection.Policies{Source: policyyaml.Source{Files: policyfake.Files{Data: []byte(policy)}}, Names: policyidna.Names{}}
	c.Doctor.Manifests = inspection.Manifests{
		Files:  &manifestfake.Files{Data: map[string][]byte{".devcontainer/devcontainer.json": []byte(manifest)}},
		Parser: jsonc.Parser{}, References: reference.References{},
	}
	c.Doctor.Credentials = readiness.Credentials{FS: files, Input: readinessport.CredentialInput{Root: "/checkout", State: "/state", OS: "linux", Architecture: "amd64", UID: 1000}}
	return c.Doctor.Run(ui)
}
