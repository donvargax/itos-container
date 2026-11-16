package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/creack/pty"
	"github.com/donvargax/itos-container/internal/cli"
	ac "github.com/donvargax/itos-container/internal/engine/approval/canonical"
	ad "github.com/donvargax/itos-container/internal/engine/approval/domain"
	ap "github.com/donvargax/itos-container/internal/engine/approval/port"
	af "github.com/donvargax/itos-container/internal/engine/approval/port/porttest"
	au "github.com/donvargax/itos-container/internal/engine/approval/presentation"
	"github.com/donvargax/itos-container/internal/engine/manifest/jsonc"
	mf "github.com/donvargax/itos-container/internal/engine/manifest/port/porttest"
	"github.com/donvargax/itos-container/internal/engine/manifest/reference"
	"github.com/donvargax/itos-container/internal/engine/policy/idna"
	pf "github.com/donvargax/itos-container/internal/engine/policy/port/porttest"
	"github.com/donvargax/itos-container/internal/engine/policy/yaml"
	rdisk "github.com/donvargax/itos-container/internal/engine/readiness/disk"
	rd "github.com/donvargax/itos-container/internal/engine/readiness/domain"
	rp "github.com/donvargax/itos-container/internal/engine/readiness/port"
	rf "github.com/donvargax/itos-container/internal/engine/readiness/port/porttest"
	rref "github.com/donvargax/itos-container/internal/engine/readiness/reference"
	rsource "github.com/donvargax/itos-container/internal/engine/readiness/source"
	approve "github.com/donvargax/itos-container/internal/slices/approve/command"
	doctor "github.com/donvargax/itos-container/internal/slices/doctor/command"
	"github.com/donvargax/itos-container/internal/slices/doctor/inspection"
	runcommand "github.com/donvargax/itos-container/internal/slices/run/command"
	rundomain "github.com/donvargax/itos-container/internal/slices/run/domain"
)

type credentialAdmissionRequest struct {
	Argv, Policy, Manifest, Root, State string
	Component, Checkpoint               string
	Pin                                 rp.CheckoutIdentity
	Files                               rf.CredentialFiles
	Approved                            string
}

// Recording wraps the real presenter; it supplies no readiness or consent result.
type credentialAdmissionPresentation struct {
	au.UI
	view ap.View
}

func (p *credentialAdmissionPresentation) Print(v ap.View, j bool) int {
	p.view = v
	return p.UI.Print(v, j)
}
func (p *credentialAdmissionPresentation) Report(v ap.View) ap.Report {
	p.view = v
	return p.UI.Report(v)
}

type credentialAdmissionOutput struct {
	Stdout, Stderr string
	Exit           int
	Calls          map[string]int
	Pin            rp.CheckoutIdentity
	Observed       rp.CredentialNode
	Report         ap.Report
	Problems       []ap.Problem
	Extra          map[string]any
}

func TestCredentialAdmissionProcess(t *testing.T) {
	path := os.Getenv("SLICE_30_ADMISSION_INPUT")
	if path == "" {
		t.Skip("feature admission assembly only")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var request credentialAdmissionRequest
	if err := json.Unmarshal(data, &request); err != nil {
		t.Fatal(err)
	}
	output := credentialAdmissionCommand(t, request)
	encoded, err := json.Marshal(output)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(os.Getenv("SLICE_30_ADMISSION_OUTPUT"), encoded, 0600); err != nil {
		t.Fatal(err)
	}
}

func credentialAdmissionCommand(t *testing.T, request credentialAdmissionRequest) credentialAdmissionOutput {
	t.Helper()
	files := &request.Files
	pair := append([]string(nil), files.MountsText...)
	for range 3 {
		files.MountsText = append(files.MountsText, pair...)
	}
	files.Parser = rdisk.ParseCredentialMounts
	files.Calls = map[string]int{}
	pin := &rf.Checkout{Identity: request.Pin}
	policyReads := 0
	policyFiles := &pf.Files{Data: []byte(request.Policy), ReadCount: &policyReads}
	policies := inspection.Policies{Source: yaml.Source{Files: policyFiles}, Names: idna.Names{}}
	manifest := &mf.Files{Data: map[string][]byte{"worker.json": []byte(request.Manifest)}}
	image := rp.Image{ID: "sha256:" + strings.Repeat("b", 64), OS: "linux", Architecture: "amd64", ConfigPresent: true, RepoDigests: []string{"worker.test/agent@sha256:" + strings.Repeat("a", 64)}}
	session := &rf.Session{Address: "unix:///engine.sock", VersionData: rp.Version{Product: "Docker Engine - Community", OS: "linux", Architecture: "amd64", Minimum: "1.44", Maximum: "1.56", Negotiated: "1.56", SDKMinimum: "1.44", SDKMaximum: "1.56"}, InfoData: rp.Info{ID: "engine-A", OS: "linux", Architecture: "amd64"}, Images: map[string]rp.Image{"worker.test/agent@sha256:" + strings.Repeat("a", 64): image}}
	engine := &rf.Engine{Session: session}
	change := func() {
		node := files.Nodes[request.Root]
		switch request.Component {
		case "device":
			node.Device++
		case "inode":
			node.Inode++
		}
		files.Nodes[request.Root] = node
	}
	if request.Checkpoint == "before engine inspection" || request.Checkpoint == "before source inspection" {
		change()
	}
	if request.Checkpoint == "after engine inspection" {
		session.AfterImage = change
	}
	input := rp.Input{Root: request.Root, Host: "unix:///engine.sock", OS: "linux", Architecture: "amd64"}
	inspector := rd.Inspector{AllowVersion3: true, Sources: rsource.Credentials{Checkout: pin, FS: files, Input: rp.CredentialInput{Root: request.Root, State: request.State, UID: 1000, OS: "linux", Architecture: "amd64"}, Account: &af.Account{Home: "/account"}}, Checkout: pin, Policy: policies.Source, Names: idna.Names{}, Files: manifest, Parser: jsonc.Parser{}, References: reference.References{}, Digests: rref.Digests{}, Engine: engine}
	readiness := inspection.Readiness{Inspector: inspector, Input: input}
	codec := ac.JSON{}
	store := &af.Store{}
	service := ad.Service{Readiness: readiness, Host: &af.Host{UID: "1000", GID: "1000", Stamp: "socket"}, Codec: codec, Store: store}
	if request.Argv == "preflight" {
		approved := credentialIndependentCandidate(request.Approved)
		store.Records = map[string]ap.Record{approved.Project: {Candidate: approved, Generation: "4"}}
		snapshots := &af.Snapshots{}
		_, err := rundomain.Preflight(ad.Preflight{Service: service, Local: readiness, Canonical: codec}, snapshots)
		exit, problems := runcommand.Classify(err)
		return credentialAdmissionOutput{Exit: exit, Extra: map[string]any{"problems": problems, "engines": engine.Opens, "writes": store.Writes, "snapshots": len(snapshots.Values)}}
	}
	var stdout, stderr bytes.Buffer
	presentation := &credentialAdmissionPresentation{UI: au.UI{Out: &stdout, Err: &stderr}}
	ui := &cli.UI{Stdout: &stdout, Stderr: &stderr}
	command := doctor.CLI{JSON: true, Approval: service, Presentation: presentation, Policies: policies, RootPolicies: policies,
		Credentials: inspection.Credentials{Checkout: pin, Input: rp.CredentialInput{Root: request.Root, State: request.State, UID: 1000, OS: "linux", Architecture: "amd64"}, Observer: inspector.Sources}}
	code := 0
	if strings.Contains(request.Argv, "--credentials-only") {
		command.CredentialsOnly = true
		code = command.Run(ui)
	} else if strings.HasPrefix(request.Argv, "terminal approve") {
		master, slave, err := pty.Open()
		if err != nil {
			t.Fatal(err)
		}
		presentation.In, presentation.Out = slave, slave
		capture := &credentialAdmissionCapture{}
		drained := make(chan struct{})
		go func() { _, _ = io.Copy(capture, master); close(drained) }()
		done := make(chan int, 1)
		go func() { done <- (approve.CLI{Service: service, Presentation: presentation}).Run(ui) }()
		deadline := time.NewTimer(8 * time.Second)
		tick := time.NewTicker(time.Millisecond)
		answered := false
	loop:
		for {
			select {
			case code = <-done:
				break loop
			case <-tick.C:
				if !answered && strings.Contains(capture.text(), "y/N") {
					if request.Checkpoint == "after terminal consent" {
						change()
					}
					_, _ = master.Write([]byte("yes\n"))
					answered = true
				}
			case <-deadline.C:
				t.Fatal("admission terminal command did not finish")
			}
		}
		tick.Stop()
		deadline.Stop()
		_ = slave.Close()
		<-drained
		_ = master.Close()
		stdout.WriteString(capture.text())
	} else {
		code = command.Run(ui)
	}
	calls := files.Calls
	calls["policy reads"] = policyReads
	calls["policy pins"] = len(pin.Roots)
	calls["checkout inspections"] = pin.Inspections
	calls["engine opens"] = engine.Opens
	calls["approval writes"] = store.Writes
	calls["frozen publications"] = 0 // No snapshot capability belongs to doctor/approve.
	calls["source data reads"] = 0   // The raw metadata port cannot open/read source data.
	calls["prompts"] = strings.Count(stdout.String(), "y/N")
	return credentialAdmissionOutput{Stdout: stdout.String(), Stderr: stderr.String(), Exit: code, Calls: calls, Pin: request.Pin, Observed: files.Nodes[request.Root].CredentialNode, Report: presentation.Report(presentation.view), Problems: presentation.Problems(presentation.view)}
}

type credentialAdmissionCapture struct {
	mu sync.Mutex
	b  strings.Builder
}

func (c *credentialAdmissionCapture) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.b.Write(p)
}
func (c *credentialAdmissionCapture) text() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.b.String()
}
