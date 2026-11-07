package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/donvargax/itos-container/internal/cli"
	canonical "github.com/donvargax/itos-container/internal/engine/approval/canonical"
	ap "github.com/donvargax/itos-container/internal/engine/approval/port"
	af "github.com/donvargax/itos-container/internal/engine/approval/port/porttest"
	parser "github.com/donvargax/itos-container/internal/engine/manifest/jsonc"
	rp "github.com/donvargax/itos-container/internal/engine/readiness/port"
	rf "github.com/donvargax/itos-container/internal/engine/readiness/port/porttest"
	codec "github.com/donvargax/itos-container/internal/engine/session/codec"
	disk "github.com/donvargax/itos-container/internal/engine/session/disk"
	"github.com/donvargax/itos-container/internal/engine/session/domain"
	sp "github.com/donvargax/itos-container/internal/engine/session/port"
	sf "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
	"github.com/donvargax/itos-container/internal/engine/session/presentation"
	runcommand "github.com/donvargax/itos-container/internal/slices/run/command"
	runfake "github.com/donvargax/itos-container/internal/slices/run/port/porttest"
	status "github.com/donvargax/itos-container/internal/slices/status/command"
	stop "github.com/donvargax/itos-container/internal/slices/stop/command"
)

// This matrix is also invoked from each "through each local backend" scenario.
// Test-process I/O carries the verdict only: every product dependency below is
// a real codec/binding adapter or a shared-contract, I/O-free port fake.
func TestPublicRecoveryFakeBackend(t *testing.T) {
	t.Run("lifecycle", TestPublicRecoveryCommandsWithPortFakes)
	t.Run("classifications", TestPublicRecoveryCommandRefusalsWithPortFakes)
	t.Run("empty-and-partial", publicFakeDiscovery)
	t.Run("protected-material", publicFakeMaterial)
	t.Run("ownership", publicFakeOwnership)
	t.Run("registration", publicFakeRegistration)
	t.Run("deadlines", publicFakeDeadlines)
}

type publicMemory struct {
	registry disk.Registry
	files    *af.Registry
	storage  *af.Storage
	host     *af.Host
	checkout *rf.Checkout
	c        ap.Candidate
	b        sp.Binding
	e        *sf.RunEngine
}

func publicMemoryFixture(t *testing.T) publicMemory {
	t.Helper()
	ready := af.Ready()
	// Memory ports model Linux paths even when this test runs on another OS.
	c := canonical.JSON{}.Candidate(ready, "1000", "1000")
	frozen := ap.Frozen{Project: c.Project, Generation: "1", Fingerprint: c.Fingerprint, Settings: c.Canonical}
	jsonCodec := codec.JSON{Approval: canonical.JSON{}, Parser: parser.Parser{}}
	b, failure := jsonCodec.Frozen(frozen)
	if failure != "" {
		t.Fatal(failure)
	}
	storage := &af.Storage{Directories: map[string]*af.Directory{
		"snapshots/" + c.Project: {Files: map[string][]byte{"1.json": canonical.JSON{}.Encode(ap.Record{Candidate: c, Generation: "1"})}},
		"intents/" + c.Project:   {Files: map[string][]byte{}},
	}}
	host, checkout := &af.Host{UID: "1000", GID: "1000"}, &rf.Checkout{Identity: *ready.Checkout}
	store := disk.Store{Storage: storage, Codec: jsonCodec, Host: host, Checkout: checkout, Input: rp.Input{Root: c.Root, OS: "linux", Architecture: "amd64"}}
	files := &af.Registry{Sessions: map[string]*af.SessionFiles{}}
	registry := disk.Registry{Store: store, Storage: files, Codec: jsonCodec, Identity: canonical.JSON{}}
	_, memory, engine := sf.Fixture()
	engine.Endpoint, engine.DaemonID = b.Endpoint, b.EngineID
	engine.Store = memory
	return publicMemory{registry, files, storage, host, checkout, c, b, &sf.RunEngine{Engine: engine, WaitFailure: sp.ExecutionFailed}}
}

func publicIndexKey(root, uid string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte("itos-container/session-index/v1\x00"+uid+"\x00"+root)))
}

func (m publicMemory) index() ap.Candidate {
	c := m.c
	c.Root = m.registry.Store.Input.Root
	c.Project = publicIndexKey(c.Root, m.host.UID)
	return c
}

func (m publicMemory) seed(letter string) (string, string) {
	id, token, worker := strings.Repeat(letter, 64), strings.Repeat(letter, 63)+"1", strings.Repeat(letter, 63)+"2"
	i := m.b.Intent(token)
	i.Stage, i.ContainerID = "owned", worker
	m.storage.Directories["intents/"+m.c.Project].Files[token+".json"] = m.registry.Store.Codec.Encode(i)
	p := sp.PublicRecord{Schema: 1, ID: id, Project: m.c.Project, Intent: sp.Reference(m.c.Project, token), State: "failed"}
	m.files.Sessions[m.index().Project+"/"+id] = &af.SessionFiles{Data: m.registry.Codec.EncodePublic(p)}
	m.e.Resources[worker] = sp.Observation{ID: worker, Name: i.Name, ImageID: i.ImageID, Token: token, Project: i.Project, Fingerprint: i.Fingerprint, StatePresent: true, Status: "created"}
	return id, worker
}

type publicMemoryReport struct {
	Schema   int
	OK       bool
	Problems []presentation.Problem
	Sessions []sp.RecoveryEntry
}

func publicMemoryCommand(t *testing.T, service sp.Recovery, command, id string, expected int) publicMemoryReport {
	t.Helper()
	var out, errout bytes.Buffer
	ui := presentation.UI{Out: &out, Err: &errout}
	var code int
	if command == "status" {
		code = (status.CLI{Service: service, Presentation: ui, JSON: true}).Run()
	} else {
		code = (stop.CLI{Service: service, Presentation: ui, JSON: true, Args: []string{id}}).Run()
	}
	var report publicMemoryReport
	if json.Unmarshal(out.Bytes(), &report) != nil || report.Schema != 1 || code != expected || report.OK != (expected == 0) || report.Problems == nil || report.Sessions == nil {
		t.Fatal(code, out.String(), errout.String())
	}
	return report
}

func (m publicMemory) service() domain.Recovery {
	return domain.Recovery{Registry: m.registry, Engine: m.e}
}

func publicFakeDiscovery(t *testing.T) {
	m := publicMemoryFixture(t)
	r := publicMemoryCommand(t, m.service(), "status", "", 0)
	var plain bytes.Buffer
	code := (status.CLI{Service: m.service(), Presentation: presentation.UI{Out: &plain, Err: &plain}}).Run()
	if code != 0 || len(r.Sessions) != 0 || len(m.e.Events) != 0 || len(m.files.Events) != 0 || !strings.Contains(plain.String(), "No owned sessions") {
		t.Fatal(r, plain.String())
	}
	for _, letter := range []string{"e", "d", "c", "b", "a"} {
		m.seed(letter)
	}
	m.files.Sessions[m.index().Project+"/"+strings.Repeat("c", 64)].Data = []byte("{invalid")
	m.files.Sessions[m.index().Project+"/"+strings.Repeat("d", 64)].Unsafe = true
	o := m.e.Resources[strings.Repeat("e", 63)+"2"]
	o.StatePresent = false
	m.e.Resources[o.ID] = o
	r = publicMemoryCommand(t, m.service(), "status", "", 75)
	var ids, rules []string
	for _, e := range r.Sessions {
		ids = append(ids, e.ID[:1])
		if e.WorkloadExit != nil || e.Project != m.c.Project || e.Fingerprint != m.c.Fingerprint {
			t.Fatal(e)
		}
	}
	for _, p := range r.Problems {
		rules = append(rules, p.Session[:1]+"/"+p.Rule)
	}
	if !reflect.DeepEqual(ids, []string{"a", "b", "e"}) || !reflect.DeepEqual(rules, []string{"c/recovery.invalid-state", "d/recovery.unsafe-state", "e/recovery.temporary"}) || len(m.files.Events) != 0 {
		t.Fatal(r)
	}
	if last := r.Sessions[2]; last.State != "failed" || last.Cleanup != "unknown" || last.Resource != "unknown" {
		t.Fatal(last)
	}
}

func publicFakeDeadlines(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m := publicMemoryFixture(t)
		id, _ := m.seed("a")
		locked, f := m.files.Open(context.Background(), m.index(), id, false)
		if f != "" {
			t.Fatal(f)
		}
		defer locked.Close()
		for _, operation := range []string{"status", "stop"} {
			start := time.Now()
			r := publicMemoryCommand(t, m.service(), operation, id, 75)
			want := 10 * time.Second
			if operation == "stop" {
				want = 30 * time.Second
			}
			if time.Since(start) != want || len(r.Problems) != 1 || r.Problems[0].Rule != "recovery.temporary" || len(m.e.Events) != 0 || len(m.files.Events) != 0 {
				t.Fatal(r, time.Since(start))
			}
		}
	})
}

func publicFakeRegistration(t *testing.T) {
	for _, fault := range []string{"", "entropy", "occupied", "write", "file fsync", "atomic publication", "parent fsync", "lock object durability"} {
		t.Run(fault, func(t *testing.T) {
			m := publicMemoryFixture(t)
			id := strings.Repeat("a", 64)
			m.files.Fault = fault
			entropy := &publicMemoryEntropy{}
			if fault == "entropy" {
				entropy.Failure = sp.IdentityFailed
			}
			if fault == "occupied" {
				m.files.Sessions[m.index().Project+"/"+id] = &af.SessionFiles{Data: []byte("existing")}
			}
			m.e.WaitFailure = ""
			runner := domain.Runner{Store: m.registry.Store, Registry: m.registry, Engine: m.e, Workspace: &sf.Workspace{}, Entropy: entropy}
			created := 0
			m.e.After = func(event string) {
				if event != "create" {
					return
				}
				created++
				if len(m.files.Sessions) != created || len(m.storage.Directories["intents/"+m.c.Project].Files) != created {
					t.Fatal("create preceded registration/intent")
				}
				for _, files := range m.files.Sessions {
					p, f := m.registry.Codec.DecodePublic(files.Data)
					if f != "" || p.ID == strings.TrimSuffix(strings.Split(p.Intent, "/")[2], ".json") {
						t.Fatal("public ID reused operation token")
					}
				}
				last := m.files.Events[len(m.files.Events)-1]
				if last.Operation != "parent fsync" || last.Failed {
					t.Fatal("create preceded durable registration")
				}
			}
			invoke := func() (int, string) {
				var out, errout bytes.Buffer
				approval := &runfake.Approval{View: ap.View{Inspection: af.Ready(), Candidate: m.c, Record: ap.Record{Candidate: m.c, Generation: "1"}, Status: "matching"}}
				code := (runcommand.CLI{Approval: approval, Snapshots: &af.Snapshots{}, Runner: runner, Args: []string{"--", "probe"}}).Run(&cli.UI{Stdout: &out, Stderr: &errout, Classify: runcommand.Classify})
				return code, errout.String()
			}
			code, diagnostic := invoke()
			if fault == "" {
				m.e.CreateID = strings.Repeat("d", 64)
				second, _ := invoke()
				r := publicMemoryCommand(t, m.service(), "status", "", 0)
				if code != 0 || second != 0 || created != 2 || len(r.Sessions) != 2 || entropy.Draws != 4 {
					t.Fatal(code, second, diagnostic, r)
				}
				for _, e := range r.Sessions {
					if e.Cleanup != "complete" || e.Project != m.c.Project || e.Fingerprint != m.c.Fingerprint {
						t.Fatal(e)
					}
				}
				return
			}
			want, cause := 75, "temporary"
			if fault == "occupied" {
				want, cause = 1, "ownership"
			}
			if fault == "entropy" {
				want, cause = 70, "internal"
			}
			if code != want || created != 0 || !strings.Contains(diagnostic, "run.session-registration-"+cause) || strings.Contains(diagnostic, "itos container stop ") || entropy.Draws > 2 {
				t.Fatal(code, diagnostic, entropy.Draws)
			}
			if fault == "occupied" && string(m.files.Sessions[m.index().Project+"/"+id].Data) != "existing" {
				t.Fatal("occupied ID replaced")
			}
		})
	}
}

type publicMemoryEntropy struct {
	Draws   int
	Failure sp.Outcome
}

func (e *publicMemoryEntropy) Token() (string, sp.Outcome) {
	e.Draws++
	return strings.Repeat(string("abcdef"[e.Draws-1]), 64), e.Failure
}
