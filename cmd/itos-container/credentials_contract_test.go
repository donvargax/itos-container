package main

import (
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
	"github.com/donvargax/itos-container/internal/cli"
	approvalfake "github.com/donvargax/itos-container/internal/engine/approval/port/porttest"
	"github.com/donvargax/itos-container/internal/engine/manifest/jsonc"
	manifestfake "github.com/donvargax/itos-container/internal/engine/manifest/port/porttest"
	"github.com/donvargax/itos-container/internal/engine/manifest/reference"
	policyidna "github.com/donvargax/itos-container/internal/engine/policy/idna"
	policyfake "github.com/donvargax/itos-container/internal/engine/policy/port/porttest"
	policyyaml "github.com/donvargax/itos-container/internal/engine/policy/yaml"
	readinessdisk "github.com/donvargax/itos-container/internal/engine/readiness/disk"
	readinessport "github.com/donvargax/itos-container/internal/engine/readiness/port"
	readinessfake "github.com/donvargax/itos-container/internal/engine/readiness/port/porttest"
	readinesssource "github.com/donvargax/itos-container/internal/engine/readiness/source"
	"github.com/donvargax/itos-container/internal/slices/doctor/inspection"
)

// The process exposes only test input and a separate operation journal. Both
// file-only and metadata scopes go through the same real command dispatcher.
func TestCredentialContractProcess(t *testing.T) {
	if os.Getenv("SLICE_29_CONTRACT") != "1" {
		t.Skip("used only by the credential observation scenarios")
	}
	var request struct {
		Policy, Manifest, Argv, Root string
		OS, Architecture             string
		readinessfake.CredentialFiles
	}
	if err := json.NewDecoder(os.Stdin).Decode(&request); err != nil {
		t.Fatal(err)
	}
	request.Parser = readinessdisk.ParseCredentialMounts
	request.Calls = map[string]int{}
	code := credentialContractCommand(request.Policy, request.Manifest, request.Argv, request.Root, request.OS, request.Architecture, &request.CredentialFiles)
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

func credentialContractCommand(policy, manifest, argv, root, hostOS, architecture string, files *readinessfake.CredentialFiles) int {
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
	if root == "" {
		root = "/checkout"
	}
	// The in-memory policy has no second location, so its root pin always holds.
	c.Doctor.RootPolicies = c.Doctor.Policies
	node := files.Nodes[root]
	pin := &readinessfake.Checkout{Identity: readinessport.CheckoutIdentity{Root: root, Device: strconv.FormatUint(node.Device, 10), Inode: strconv.FormatUint(node.Inode, 10)}}
	if hostOS == "" {
		hostOS = "linux"
	}
	if architecture == "" {
		architecture = "amd64"
	}
	input := readinessport.CredentialInput{Root: root, State: "/state", OS: hostOS, Architecture: architecture, UID: 1000}
	c.Doctor.Credentials = inspection.Credentials{Input: input, Checkout: pin, Observer: readinesssource.Credentials{FS: files, Input: input, Account: &approvalfake.Account{Home: "/account"}, Checkout: pin}}
	code := c.Doctor.Run(ui)
	files.Calls["pinned checkout identity"] = pin.PinnedReads
	return code
}
