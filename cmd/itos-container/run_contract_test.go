package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/donvargax/itos-container/internal/engine/approval/canonical"
	"github.com/donvargax/itos-container/internal/engine/approval/disk"
	approval "github.com/donvargax/itos-container/internal/engine/approval/domain"
	ap "github.com/donvargax/itos-container/internal/engine/approval/port"
	af "github.com/donvargax/itos-container/internal/engine/approval/port/porttest"
	manifest "github.com/donvargax/itos-container/internal/engine/manifest/port"
	policy "github.com/donvargax/itos-container/internal/engine/policy/port"
	readiness "github.com/donvargax/itos-container/internal/engine/readiness/port"
	command "github.com/donvargax/itos-container/internal/slices/run/command"
	rundomain "github.com/donvargax/itos-container/internal/slices/run/domain"
	rp "github.com/donvargax/itos-container/internal/slices/run/port"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

type runContractInput struct {
	result         readiness.Result
	reads, engines int
	selected       bool
	after          int
}

func (r *runContractInput) Inspect() readiness.Result {
	r.reads++
	r.engines++
	if r.selected {
		r.after++
	}
	return r.result
}
func (r *runContractInput) InspectBeforeEngine(check func(readiness.Result) bool) readiness.Result {
	r.reads++
	if r.selected {
		r.after++
	}
	local := r.result
	local.Engine = nil
	local.Image = nil
	if !check(local) {
		return local
	}
	r.engines++
	return r.result
}

type runContractApproval struct {
	rp.Approval
	before, after func()
}

func (a runContractApproval) Freeze(v ap.View) (ap.Frozen, ap.Failure) {
	if a.before != nil {
		a.before()
	}
	f, e := a.Approval.Freeze(v)
	if e == "" && a.after != nil {
		a.after()
	}
	return f, e
}

type runContractResult struct {
	Handoff                 ap.Durable
	Rule                    string
	Exit                    int
	Message, Fix            string
	EngineCalls, AfterReads int
	Published               ap.Frozen
	Visible                 ap.Frozen
	Events                  []string
	Bytes                   string
	Inode                   string
	Mtime                   int64
}

func contractResult(h ap.Durable, err error, input *runContractInput) runContractResult {
	result := runContractResult{Handoff: h, EngineCalls: input.engines, AfterReads: input.after}
	if err != nil {
		result.Exit, _ = command.Classify(err)
		_, problems := command.Classify(err)
		if len(problems) > 0 {
			result.Rule = problems[0].Rule
			result.Message = problems[0].Message
			result.Fix = problems[0].Fix
		}
	}
	return result
}

// Only a go test subprocess can enter this assembly, never a product flag.
func TestRunPreflightContractProbe(t *testing.T) {
	path := os.Getenv("SLICE_8_PROBE_OUTPUT")
	if path == "" {
		t.Skip("feature contract subprocess only")
	}
	kind, condition := os.Getenv("SLICE_8_PROBE_KIND"), os.Getenv("SLICE_8_PROBE_CONDITION")
	uid, gid := os.Getenv("SLICE_8_PROBE_UID"), os.Getenv("SLICE_8_PROBE_GID")
	out := map[string]any{}
	if kind == "identity" {
		input := &runContractInput{result: af.Ready()}
		memory := &af.Store{}
		snapshot := &af.Snapshots{}
		service := approval.Service{Readiness: input, Host: &af.Host{UID: uid, GID: gid, Stamp: "socket"}, Codec: canonical.JSON{}, Store: memory}
		h, err := rundomain.Preflight(approval.Preflight{Service: service, Local: input, Canonical: canonical.JSON{}}, snapshot)
		result := contractResult(h, err, input)
		out["identity"] = result
		out["snapshot_count"] = len(snapshot.Values)
	} else {
		for _, backend := range []string{"memory", "disk"} {
			out[backend] = runContractBackend(t, kind, condition, backend)
		}
	}
	data, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}
func runContractBackend(t *testing.T, kind, condition, backend string) map[string]any {
	t.Helper()
	root := t.TempDir()
	codec := canonical.JSON{}
	uid := strconv.Itoa(os.Geteuid())
	ready := af.Ready()
	ready.Checkout.Root = filepath.Join(root, "checkout")
	a := codec.Candidate(ready, uid, "1000")
	readyB := af.Ready()
	readyB.Checkout = ready.Checkout
	readyB.Manifest.Effective.ContainerEnv = map[string]string{"APP": "B"}
	b := codec.Candidate(readyB, uid, "1000")
	// Seed through an independently constructed envelope, never an approve command.
	memory := &af.Store{Records: map[string]ap.Record{a.Project: {Candidate: a, Generation: "1"}}}
	state := filepath.Join(root, "state")
	store := disk.Store{StateHome: state, Records: codec}
	var active ap.Store = memory
	if backend == "disk" {
		active = store
		contractSeed(t, state, a, "1")
	}
	snapshots := &af.Snapshots{Values: map[string]ap.Frozen{}}
	var events []string
	diskSnapshots := disk.Snapshots{Store: store, Records: codec}
	failed := true
	fault := errors.New("slice-8 publication fault")
	// Every non-faulting operation actually executes against pinned file handles.
	diskSnapshots.IO = disk.SnapshotIO{
		Create: func(r *os.Root, n string) (*os.File, error) {
			events = append(events, "create")
			if failed && condition == "private file creation" {
				return nil, fault
			}
			return r.OpenFile(n, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		},
		Write: func(f *os.File, b []byte) (int, error) {
			events = append(events, "write")
			if failed && condition == "short write" {
				return f.Write(b[:len(b)-1])
			}
			return f.Write(b)
		},
		FileSync: func(f *os.File) error {
			events = append(events, "file fsync")
			if failed && condition == "file fsync" {
				return fault
			}
			return f.Sync()
		},
		Rename: func(r *os.Root, a, b string) error {
			events = append(events, "rename")
			if failed && condition == "rename" {
				return fault
			}
			return r.Rename(a, b)
		},
		ParentSync: func(f *os.File) error {
			events = append(events, "parent directory fsync")
			if failed && condition == "parent directory fsync" {
				return fault
			}
			return f.Sync()
		},
		DirectorySync: func(f *os.File) error {
			events = append(events, "new ancestor fsync")
			if failed && condition == "new ancestor fsync" {
				return fault
			}
			return f.Sync()
		},
	}
	var publisher ap.Snapshots = snapshots
	if backend == "disk" {
		publisher = diskSnapshots
	}
	input := &runContractInput{result: ready}
	service := approval.Service{Readiness: input, Host: &af.Host{UID: uid, GID: "1000", Stamp: "socket"}, Codec: codec, Store: active}
	preflight := approval.Preflight{Service: service, Local: input, Canonical: codec}
	wrapped := runContractApproval{Approval: preflight}
	replace := func() {
		memory.Records[a.Project] = ap.Record{Candidate: b, Generation: "2"}
		if backend == "disk" {
			contractSeed(t, state, b, "2")
		}
	}
	if kind == "superseded" {
		wrapped.before = replace
	}
	if kind == "independence" {
		wrapped.after = func() {
			input.selected = true
			switch condition {
			case "policy now requests forbidden online networking":
				input.result.Policy = &policy.Result{Issues: []policy.Issue{{Kind: policy.NetworkMode, Path: []string{"network", "mode"}}}}
			case "manifest now requests a forbidden extra mount":
				input.result.Manifest = &manifest.Result{Issues: []manifest.Issue{{Kind: manifest.Unsupported, Path: []string{"mounts"}}}}
			case "active approval is replaced by B":
				replace()
			}
		}
	}
	if kind == "fault" {
		snapshots.Failure = condition
	}
	outside := filepath.Join(root, "outside-target")
	if err := os.WriteFile(outside, []byte("preserve outside target"), 0600); err != nil {
		t.Fatal(err)
	}
	prior := filepath.Join(state, "itos-container", "snapshots", strings.Repeat("e", 64), "9.json")
	if err := os.MkdirAll(filepath.Dir(prior), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(prior, []byte(`{"prior":"preserve independently"}`), 0600); err != nil {
		t.Fatal(err)
	}
	before, err := contractProtected(root, a.Project)
	if err != nil {
		t.Fatal(err)
	}
	h, e := rundomain.Preflight(wrapped, publisher)
	first := contractResult(h, e, input)
	observe := func(result *runContractResult) {
		generation := result.Handoff.Frozen.Generation
		if backend == "memory" {
			result.Published = snapshots.Values[a.Project+"/"+generation]
		} else if generation != "" {
			name := filepath.Join(state, "itos-container", "snapshots", a.Project, generation+".json")
			if data, err := os.ReadFile(name); err == nil {
				candidate := a
				if generation == "2" {
					candidate = b
				}
				if record, failure := codec.Decode(candidate, data); failure == "" {
					result.Published = ap.Frozen{Project: record.Project, Generation: record.Generation, Fingerprint: record.Fingerprint, Settings: record.Canonical}
				}
			}
		}

		if backend == "memory" {
			result.Visible = snapshots.Values[a.Project+"/1"]
			result.Events = append([]string{}, snapshots.Events...)
			result.Bytes = result.Visible.Settings
			result.Inode = "memory-1"
			return
		}
		name := filepath.Join(state, "itos-container", "snapshots", a.Project, "1.json")
		data, err := os.ReadFile(name)
		if err != nil {
			return
		}
		r, f := codec.Decode(a, data)
		if f == "" {
			result.Visible = ap.Frozen{Project: r.Project, Generation: r.Generation, Fingerprint: r.Fingerprint, Settings: r.Canonical}
		}
		result.Bytes = string(data)
		info, _ := os.Stat(name)
		if info != nil {
			result.Inode = fmt.Sprintf("%v:%v", reflect.Indirect(reflect.ValueOf(info.Sys())).FieldByName("Dev"), reflect.Indirect(reflect.ValueOf(info.Sys())).FieldByName("Ino"))
			result.Mtime = info.ModTime().UnixNano()
		}
		result.Events = append([]string{}, events...)
	}
	observe(&first)
	retained := h
	if bytes := h.Frozen.Bytes(); len(bytes) > 0 {
		bytes[0] = 'X'
	}
	h.Frozen.Settings = "caller mutation"
	input.selected = false
	input.reads = 0
	input.engines = 0
	input.after = 0
	if kind == "fault" {
		failed = false
		snapshots.Failure = ""
		snapshots.Events = nil
		events = nil
		input.result = ready
	} else {
		replace()
		input.result = readyB
	}
	secondH, secondErr := rundomain.Preflight(preflight, publisher)
	second := contractResult(secondH, secondErr, input)
	observe(&second)
	if bytes := secondH.Frozen.Bytes(); len(bytes) > 0 {
		bytes[0] = 'Y'
	}
	after, err := contractProtected(root, a.Project)
	if err != nil {
		t.Fatal(err)
	}
	return map[string]any{"first": first, "second": second, "retained": retained, "a": a, "b": b, "protected_before": before, "protected_after": after}
}
func contractSeed(t *testing.T, state string, c ap.Candidate, g string) {
	t.Helper()
	var settings any
	if err := json.Unmarshal([]byte(c.Canonical), &settings); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(map[string]any{"schema": 1, "project": c.Project, "generation": g, "fingerprint": c.Fingerprint, "settings": settings})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(state, "itos-container", "approvals", c.Project, "current.json")
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}
func contractProtected(state, project string) (map[string]string, error) {
	out := map[string]string{}
	err := filepath.WalkDir(state, func(path string, e os.DirEntry, err error) error {
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if e.IsDir() {
			return nil
		}
		if filepath.Dir(path) == filepath.Join(state, "state", "itos-container", "snapshots", project) && (filepath.Base(path) == "1.json" || filepath.Base(path) == "2.json") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		info, err := e.Info()
		if err != nil {
			return err
		}
		stat := reflect.Indirect(reflect.ValueOf(info.Sys()))
		out[path] = fmt.Sprintf("%s:%v:%v:%v:%v:%v:%d", data, info.Mode(), stat.FieldByName("Uid"), stat.FieldByName("Nlink"), stat.FieldByName("Dev"), stat.FieldByName("Ino"), info.ModTime().UnixNano())
		return nil
	})
	return out, err
}

func TestRunSnapshotStoreContracts(t *testing.T) {
	for _, backend := range []string{"memory", "disk"} {
		t.Run(backend, func(t *testing.T) {
			if backend == "disk" && runtime.GOOS != "linux" {
				t.Skip("Linux protected-store target")
			}
			af.SnapshotContract(t, func() (ap.Snapshots, ap.Frozen, ap.Frozen) {
				codec := canonical.JSON{}
				uid := strconv.Itoa(os.Geteuid())
				if uid == "-1" {
					uid = "1000"
				}
				a := codec.Candidate(af.Ready(), uid, "1000")
				b := codec.Candidate(af.Ready(), uid, "2000")
				frozenA := ap.Frozen{Project: a.Project, Generation: "1", Fingerprint: a.Fingerprint, Settings: a.Canonical}
				frozenB := ap.Frozen{Project: b.Project, Generation: "2", Fingerprint: b.Fingerprint, Settings: b.Canonical}
				if backend == "memory" {
					return &af.Snapshots{}, frozenA, frozenB
				}
				return disk.Snapshots{Store: disk.Store{StateHome: t.TempDir(), Records: codec}, Records: codec}, frozenA, frozenB
			})
		})
	}
}
