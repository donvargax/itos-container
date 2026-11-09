package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	ap "github.com/donvargax/itos-container/internal/engine/approval/port"
	af "github.com/donvargax/itos-container/internal/engine/approval/port/porttest"
	"github.com/donvargax/itos-container/internal/engine/session/domain"
	sp "github.com/donvargax/itos-container/internal/engine/session/port"
	"github.com/donvargax/itos-container/internal/engine/session/presentation"
	status "github.com/donvargax/itos-container/internal/slices/status/command"
	stop "github.com/donvargax/itos-container/internal/slices/stop/command"
)

type anchorMemoryReport struct {
	Sessions []map[string]any
	Problems []struct {
		Rule string
		Exit int
	}
}

func anchorMemoryInvoke(service domain.Recovery, command, id string) (int, anchorMemoryReport) {
	var out, errout bytes.Buffer
	ui := presentation.UI{Out: &out, Err: &errout}
	code := 0
	if command == "status" {
		code = (status.CLI{Service: service, Presentation: ui, JSON: true}).Run()
	} else {
		code = (stop.CLI{Service: service, Presentation: ui, JSON: true, Args: []string{id}}).Run()
	}
	var r anchorMemoryReport
	if json.Unmarshal(out.Bytes(), &r) != nil {
		return 70, r
	}
	return code, r
}
func anchorMemoryProblem(t *testing.T, code int, r anchorMemoryReport, want int, rule string) {
	t.Helper()
	if code != want {
		t.Fatalf("anchor command exit %d want %d: %+v", code, want, r)
	}
	found := false
	for _, p := range r.Problems {
		if p.Rule == rule && p.Exit == want {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing %s: %+v", rule, r)
	}
}

func anchorRecoveryMemoryFaultCase(t *testing.T, q map[string]string, m publicMemory, e *anchorRecoveryMemoryEngine, s domain.Recovery, id, worker string, r sp.Record, p sp.PublicRecord) {
	t.Helper()
	mode, fault := q["mode"], q["fault"]
	if mode == "defect" {
		anchorMemoryDefect(t, q, m, e, s, id, worker, r)
		return
	}
	if mode == "barrier" || mode == "concurrent" {
		anchorMemoryRace(t, q, m, e, s, id, worker, r, p)
		return
	}
	dir := m.storage.Directories["intents/"+m.c.Project]
	var restore func()
	reached := false
	active := true
	if mode == "worker" {
		switch fault {
		case "create reply lost and exact-name lookup absent":
			saved := m.e.Resources[worker]
			delete(m.e.Resources, worker)
			r.Stage, r.ContainerID = "intent", ""
			dir.Files[r.NameToken()+".json"] = m.registry.Store.Codec.Encode(r)
			restore = func() { m.e.Resources[worker] = saved }
		case "worker removal fails with the full ID still present":
			e.removeWorker = func(context.Context, string) bool { reached = true; return !active }
		case "worker removal and full-ID absence remain unknown":
			e.removeWorker = func(ctx context.Context, id string) bool {
				reached = true
				if active {
					m.e.Following = "unavailable"
				}
				return true
			}
			restore = func() { m.e.Following = "" }
		}
	}
	if mode == "cleanup" {
		e.removeAnchor = func(ctx context.Context, name string) bool {
			reached = true
			if !active {
				return true
			}
			switch fault {
			case "volume deletion fails and the owned volume remains", "volume deletion reports in use", "volume delete acknowledged but volume still present":
				return false
			default:
				return true
			}
		}
		e.inspectAnchor = func(ctx context.Context, name string, o sp.AnchorObservation, state sp.Inspection) (sp.AnchorObservation, sp.Inspection) {
			if active && reached && fault == "volume delete reply lost and inspection unavailable" {
				return sp.AnchorObservation{}, sp.Unavailable
			}
			return o, state
		}
		failWrite := func(data []byte) ap.Failure {
			if !active {
				return ""
			}
			var v struct {
				State  string
				Anchor struct{ Stage string }
			}
			_ = json.Unmarshal(data, &v)
			if strings.HasPrefix(fault, "anchor absence") && v.Anchor.Stage == "absent" || strings.HasPrefix(fault, "session completion") && v.State == "stopped" {
				reached = true
				return "state.write"
			}
			return ""
		}
		store := m.registry.Store
		store.Storage = anchorMemoryStorage{Storage: store.Storage, fault: failWrite, published: strings.Contains(fault, "parent fsync")}
		reg := m.registry
		reg.Store = store
		reg.Storage = anchorMemoryRegistry{RegistryStorage: reg.Storage, fault: failWrite, published: strings.Contains(fault, "parent fsync")}
		s.Registry = reg
	}
	command := "stop"
	if mode == "budget" {
		command = "stop"
		if fault == "status anchor inspect" {
			command = "status"
		}
		if command == "stop" {
			lock, f := m.files.Open(context.Background(), m.index(), id, false)
			if f != "" {
				t.Fatal(f)
			}
			go func() { time.Sleep(25 * time.Second); lock.Close() }()
		} else {
			// Model the preceding engine inspection within the same total.
			e.delay = 8 * time.Second
		}
		e.inspectAnchor = func(ctx context.Context, name string, o sp.AnchorObservation, state sp.Inspection) (sp.AnchorObservation, sp.Inspection) {
			if active && strings.HasSuffix(fault, "inspect") {
				reached = true
				<-ctx.Done()
				return sp.AnchorObservation{}, sp.Unavailable
			}
			return o, state
		}
		e.removeAnchor = func(ctx context.Context, name string) bool {
			if active && strings.HasSuffix(fault, "delete") {
				reached = true
				<-ctx.Done()
				return false
			}
			return true
		}
	}
	start := time.Now()
	code, report := anchorMemoryInvoke(s, command, id)
	anchorMemoryProblem(t, code, report, 75, "recovery.temporary")
	if mode != "worker" && !reached {
		t.Fatal("named anchor fault never reached")
	}
	if mode == "worker" && e.deletions != 0 {
		t.Fatal("worker uncertainty released anchor")
	}
	for _, entry := range report.Sessions {
		if entry["id"] == id && (entry["cleanup"] == "complete" || entry["retry"] == nil) {
			t.Fatal("uncertainty claimed completion", entry)
		}
	}
	if mode == "budget" {
		limit := 30 * time.Second
		if command == "status" {
			limit = 10 * time.Second
		}
		if time.Since(start) > limit {
			t.Fatal("reset recovery total", time.Since(start))
		}
	}
	active = false
	e.delay = 0
	if restore != nil {
		restore()
	}
	r.ContainerID, r.Stage = worker, "owned"
	anchorRecoveryMemoryStop(t, m, e, s, id, worker, r, p)
	if mode == "cleanup" {
		anchorRecoveryMemoryCase(t, map[string]string{"mode": "stop", "phase": "failed", "worker": "created", "volume": "present", "completion": "pending", "exit": "7", "root": "/independent" + m.c.Root, "letter": "b"})
	}
}

type anchorMemoryStorage struct {
	ap.Storage
	fault     func([]byte) ap.Failure
	published bool
}

func (s anchorMemoryStorage) Open(ctx context.Context, c ap.Candidate, collection string, create bool) (ap.Directory, ap.Failure) {
	d, f := s.Storage.Open(ctx, c, collection, create)
	if f != "" || d == nil {
		return d, f
	}
	return anchorMemoryDirectory{Directory: d, fault: s.fault, published: s.published}, ""
}

type anchorMemoryDirectory struct {
	ap.Directory
	fault     func([]byte) ap.Failure
	published bool
}

func (d anchorMemoryDirectory) Publish(name string, old, next []byte) ap.Failure {
	f := d.fault(next)
	if f != "" && !d.published {
		return f
	}
	if err := d.Directory.Publish(name, old, next); err != "" {
		return err
	}
	return f
}
func (d anchorMemoryDirectory) Sync(name string, expected []byte) ap.Failure {
	if f := d.fault(expected); f != "" {
		return f
	}
	return d.Directory.Sync(name, expected)
}

type anchorMemoryRegistry struct {
	ap.RegistryStorage
	fault     func([]byte) ap.Failure
	published bool
}

func (s anchorMemoryRegistry) Open(ctx context.Context, c ap.Candidate, id string, readOnly bool) (ap.SessionDirectory, ap.Failure) {
	d, f := s.RegistryStorage.Open(ctx, c, id, readOnly)
	if f != "" || d == nil {
		return d, f
	}
	return anchorMemorySessionDirectory{SessionDirectory: d, fault: s.fault, published: s.published}, ""
}

type anchorMemorySessionDirectory struct {
	ap.SessionDirectory
	fault     func([]byte) ap.Failure
	published bool
}

func (d anchorMemorySessionDirectory) Publish(name string, old, next []byte) ap.Failure {
	f := d.fault(next)
	if f != "" && !d.published {
		return f
	}
	if err := d.SessionDirectory.Publish(name, old, next); err != "" {
		return err
	}
	return f
}
func (d anchorMemorySessionDirectory) Sync(name string, expected []byte) ap.Failure {
	if f := d.fault(expected); f != "" {
		return f
	}
	return d.SessionDirectory.Sync(name, expected)
}

func anchorMemoryRace(t *testing.T, q map[string]string, m publicMemory, e *anchorRecoveryMemoryEngine, s domain.Recovery, id, worker string, r sp.Record, p sp.PublicRecord) {
	reached := false
	ready, release := make(chan struct{}), make(chan struct{})
	e.inspectAnchor = func(ctx context.Context, name string, o sp.AnchorObservation, state sp.Inspection) (sp.AnchorObservation, sp.Inspection) {
		if _, present := m.e.Resources[worker]; present || reached {
			return o, state
		}
		reached = true
		if q["mode"] == "concurrent" {
			close(ready)
			<-release
			return o, state
		}
		switch q["fault"] {
		case "selected daemon changed":
			m.e.DaemonID = "foreign"
		case "pinned socket inode replaced":
			m.e.Replaced = true
		case "same-named volume tuple changed":
			v := e.volumes.Resources[name]
			v.Created = "foreign"
			e.volumes.Resources[name] = v
		case "protected anchor record replaced":
			next := r
			next.Anchor.Source = "/proc/123/fd/19"
			m.storage.Directories["intents/"+m.c.Project].Files[r.NameToken()+".json"] = m.registry.Store.Codec.Encode(next)
		}
		return o, state
	}
	if q["mode"] == "barrier" {
		code, report := anchorMemoryInvoke(s, "stop", id)
		anchorMemoryProblem(t, code, report, 1, "recovery.not-owned")
		if !reached || e.deletions != 0 {
			t.Fatal("release did not revalidate anchor authority", reached, e.deletions)
		}
	} else {
		done := make(chan int, 2)
		go func() { code, _ := anchorMemoryInvoke(s, "stop", id); done <- code }()
		synctest.Wait()
		if !reached {
			t.Fatal("concurrent anchor boundary not reached")
		}
		<-ready
		go func() { code, _ := anchorMemoryInvoke(s, "stop", id); done <- code }()
		synctest.Wait()
		anchorRecoveryMemoryCase(t, map[string]string{"mode": "stop", "phase": "failed", "worker": "created", "volume": "present", "completion": "pending", "exit": "7", "root": "/independent" + m.c.Root, "letter": "b"})
		close(release)
		code, report := anchorMemoryInvoke(s, "stop", strings.Repeat("b", 64))
		anchorMemoryProblem(t, code, report, 1, "recovery.not-owned")
		if a, b := <-done, <-done; a != 0 || b != 0 || e.deletions != 1 {
			t.Fatal("concurrent recovery did not serialize", a, b, e.deletions)
		}
	}
	anchorRecoveryMemoryCase(t, map[string]string{"mode": "stop", "phase": "failed", "worker": "created", "volume": "present", "completion": "pending", "exit": "7", "root": "/independent" + m.c.Root, "letter": "b"})
}

func anchorMemoryDefect(t *testing.T, q map[string]string, m publicMemory, e *anchorRecoveryMemoryEngine, s domain.Recovery, id, worker string, r sp.Record) {
	oldIndex := m.index().Project
	fault := q["fault"]
	v := e.volumes.Resources[r.Anchor.Name]
	dir := m.storage.Directories["intents/"+m.c.Project]
	rule, code := "recovery.not-owned", 1
	switch fault {
	case "selected daemon differs":
		m.e.DaemonID = "foreign"
	case "anchor belongs to a different host user":
		m.host.UID = "1001"
	case "anchor belongs to a different project":
		v.Project = strings.Repeat("e", 64)
	case "anchor session label differs":
		v.Session = strings.Repeat("e", 64)
	case "anchor operation label differs":
		v.Token = strings.Repeat("e", 64)
	case "anchor fingerprint label differs":
		v.Fingerprint = "foreign"
	case "anchor driver differs":
		v.Driver = "foreign"
	case "anchor bind type differs":
		v.Type = "tmpfs"
	case "anchor bind options differ":
		v.Options = "rbind"
	case "anchor device names a reused foreign descriptor":
		v.Source = "/proc/123/fd/17"
	case "recorded volume name now holds a foreign volume":
		v.Created = "foreign"
	case "worker mount names a different anchor":
		o := m.e.Resources[worker]
		data, _ := json.Marshal(o)
		var fields map[string]any
		_ = json.Unmarshal(data, &fields)
		fields["AnchorName"] = "foreign"
		data, _ = json.Marshal(fields)
		_ = json.Unmarshal(data, &o)
		m.e.Resources[worker] = o
	case "required protected anchor record missing":
		r.Anchor = sp.Anchor{}
		dir.Files[r.NameToken()+".json"] = m.registry.Store.Codec.Encode(r)
		rule, code = "recovery.invalid-state", 2
	case "unsupported mount-bearing record version":
		r.Schema = 3
		dir.Files[r.NameToken()+".json"] = m.registry.Store.Codec.Encode(r)
		rule, code = "recovery.invalid-state", 2
	case "duplicate anchor ownership field":
		data := string(dir.Files[r.NameToken()+".json"])
		dir.Files[r.NameToken()+".json"] = []byte(strings.Replace(data, `"anchor":{`, `"anchor":{"session":"duplicate",`, 1))
		rule, code = "recovery.invalid-state", 2
	case "anchor record symlink", "anchor record hard link", "anchor state ancestor writable by others":
		dir.ReadFailure = "state.unsafe"
		rule = "recovery.unsafe-state"
	default:
		t.Fatal("unknown anchor defect", fault)
	}
	e.volumes.Resources[r.Anchor.Name] = v
	if oldIndex != m.index().Project {
		for key, files := range m.files.Sessions {
			delete(m.files.Sessions, key)
			m.files.Sessions[strings.Replace(key, oldIndex, m.index().Project, 1)] = files
		}
	}
	safe := anchorMemorySafeEntry(t, m)
	for _, command := range []string{"status", "stop"} {
		actual, report := anchorMemoryInvoke(s, command, id)
		anchorMemoryProblem(t, actual, report, code, rule)
		if command == "status" {
			found := false
			for _, entry := range report.Sessions {
				if entry["id"] == safe {
					found = true
					if entry["anchor"] != "absent" || entry["cleanup"] != "complete" {
						t.Error("safe independent entry lost", entry)
					}
				}
			}
			if !found {
				t.Error("safe independent entry omitted")
			}
		}
	}
	if e.deletions != 0 {
		t.Fatal("defective authority deleted volume")
	}
	anchorRecoveryMemoryCase(t, map[string]string{"mode": "stop", "phase": "failed", "worker": "created", "volume": "present", "completion": "pending", "exit": "7", "root": "/independent" + m.c.Root, "letter": "b"})
}

func anchorMemorySafeEntry(t *testing.T, m publicMemory) string {
	t.Helper()
	var settings map[string]any
	if err := json.Unmarshal([]byte(m.c.Canonical), &settings); err != nil {
		t.Fatal(err)
	}
	checkout := settings["checkout"].(map[string]any)
	checkout["inode"] = fmt.Sprint(checkout["inode"]) + "1"
	host := settings["host"].(map[string]any)
	host["uid"], host["gid"] = m.host.UID, m.host.GID
	manifest := settings["manifest"].(map[string]any)
	manifest["containerUser"], manifest["remoteUser"] = m.host.UID, m.host.UID
	settings["engine"].(map[string]any)["id"] = m.e.DaemonID
	hash := func(value any) string { data, _ := json.Marshal(value); return fmt.Sprintf("%x", sha256.Sum256(data)) }
	project := hash(map[string]any{"schema": 1, "uid": m.host.UID, "checkout": checkout})
	fingerprint := "sha256:" + hash(settings)
	data, _ := json.Marshal(map[string]any{"schema": 1, "project": project, "generation": "2", "fingerprint": fingerprint, "settings": settings})
	b, f := m.registry.Store.Codec.Snapshot(data)
	if f != "" {
		t.Fatal("safe independent snapshot fixture", f)
	}
	m.storage.Directories["snapshots/"+project] = &af.Directory{Files: map[string][]byte{"2.json": data}}
	id, token, worker := strings.Repeat("c", 64), strings.Repeat("c", 63)+"1", strings.Repeat("c", 63)+"2"
	r := b.Intent(token)
	r.Schema, r.Stage, r.ContainerID = 2, "absent", worker
	r.Anchor = sp.Anchor{Session: id, Name: "itos-container-anchor-" + token, Source: "/proc/2147483646/fd/9", Device: b.Checkout.Device, Inode: b.Checkout.Inode, Created: "2026-11-07T16:00:00Z", Stage: "absent"}
	m.storage.Directories["intents/"+project] = &af.Directory{Files: map[string][]byte{token + ".json": m.registry.Store.Codec.Encode(r)}}
	p := sp.PublicRecord{Schema: 1, ID: id, Project: project, Intent: sp.Reference(project, token), State: "stopped"}
	m.files.Sessions[m.index().Project+"/"+id] = &af.SessionFiles{Data: m.registry.Codec.EncodePublic(p)}
	return id
}
