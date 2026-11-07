package features

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"

	"github.com/cucumber/godog"
)

func (w *world) controlBootstrapSteps(sc *godog.ScenarioContext) {
	sc.Step(`^a first-approval control-state fixture with HOME equal to the account home and XDG_STATE_HOME unset and no state directories$`, func() error {
		return w.controlSetup("absent", "first approval", "shared daemon")
	})
	sc.Step(`^default-tree bootstrap has fault "([^"]*)" while ordinary approval writes remain healthy$`, func(fault string) { w.control.p.doctor.env["SLICE_21_FAULT"] = fault })
	sc.Step(`^P consents with its independently calculated exact fingerprint in a fresh process$`, func() { w.control.tracedApproval() })
	sc.Step(`^P validates private bootstrap entries before taking private coordination exclusion through approval publication and completes durable self-binding before any ordinary state write$`, func() error { return w.control.bootstrapOrdering() })
	sc.Step(`^P reports matching approval with exit 0 and exactly generation 1 beneath the default tree bound in its own catalog$`, func() error {
		f := w.control
		return errors.Join(f.err, f.p.approvalSuccess(), f.p.approvalStored("1"), f.selfBinding())
	})
	sc.Step(`^only the fixed bootstrap chain self-binding and P approval were created with private ownership and no engine mutation or checkout execution$`, func() error { return w.control.bootstrapFootprint(true) })
	sc.Step(`^the approval process has exited and fresh P doctor and matching approve use the same default layout$`, func() { w.control.bootstrapReuse() })
	sc.Step(`^both report matching approval with exit 0 while the completed self-binding and approval keep their bytes identities and generation$`, func() error {
		f := w.control
		if len(f.results) != 2 {
			return errors.New("missing fresh doctor and matching approval observations")
		}
		for _, r := range f.results {
			var report map[string]any
			if r.exit != 0 || json.Unmarshal([]byte(r.stdout), &report) != nil || object(report["approval"])["status"] != "matching" {
				return errors.New("fresh default-tree command did not report matching approval")
			}
		}
		return errors.Join(f.retainedBootstrap(), f.p.approvalStored("1"))
	})
	sc.Step(`^the fresh doctor writes nothing and the matching approve uses the same private coordination lock without creating a duplicate location binding$`, func() error {
		f := w.control
		if err := errors.Join(f.err, f.catalogOnlySelf()); err != nil {
			return err
		}
		events, err := controlEvents(f.traces[len(f.traces)-1].journal)
		if err != nil {
			return err
		}
		for _, e := range events {
			if e.Op == "flock" && e.Path == filepath.Join(f.coordination(), "control.lock") && e.Phase == "end" && e.Success && e.Flags&2 != 0 {
				return nil
			}
		}
		return errors.New("matching reuse did not acquire the original private coordination lock")
	})
	sc.Step(`^P reports approval.state.write with exit 75 and no approval directory record or temp file was created$`, func() error {
		f := w.control
		return errors.Join(f.err, f.p.approvalProblem("approval.state.write", 75, ""), f.noApprovalEntry())
	})
	sc.Step(`^only safe bootstrap entries may remain with no engine mutation or checkout change$`, func() error {
		f := w.control
		if err := f.bootstrapFootprint(false); err != nil {
			return err
		}
		return f.captureBootstrap()
	})
	sc.Step(`^the failed process has exited and the fixture clears only the fault before fresh P doctor and exact-fingerprint approval$`, func() {
		f := w.control
		delete(f.p.doctor.env, "SLICE_21_FAULT")
		f.inspect("doctor", "JSON")
		f.err = errors.Join(f.err, f.p.exitsWith(0), f.p.approvalStatus("missing"), f.unchanged())
		f.tracedApproval()
	})
	sc.Step(`^doctor reports missing approval with exit 0 and writes nothing before approval completes durable bootstrap and publishes generation 1$`, func() error {
		f := w.control
		return errors.Join(f.err, f.p.approvalSuccess(), f.p.approvalStored("1"), f.selfBinding(), f.bootstrapOrdering())
	})
	sc.Step(`^the retry preserves surviving bootstrap identities and completed binding bytes while redoing required fsyncs before any ordinary state write$`, func() error {
		return errors.Join(w.control.retainedBootstrap(), w.control.bootstrapOrdering())
	})
}

func (f *controlFixture) fixedRoot() string {
	return filepath.Join(f.home, ".local/state/itos-container")
}
func (f *controlFixture) coordination() string { return filepath.Join(f.fixedRoot(), "coordination") }

func (f *controlFixture) tracedApproval() {
	if f.bootstrapBefore == nil {
		var err error
		f.bootstrapBefore, err = f.inventory()
		f.err = errors.Join(f.err, err)
	}
	p := f.traceCommand("approve", "--json", "--fingerprint", f.p.approval.fingerprint)
	f.traces = append(f.traces, p)
	p.wait()
	f.err = errors.Join(f.err, p.err)
	f.p.exit, f.p.stdout, f.p.stderr = p.exit, string(p.out.bytes()), string(p.errout.bytes())
}

func (f *controlFixture) bootstrapOrdering() error {
	if f.err != nil {
		return f.err
	}
	if len(f.traces) == 0 {
		return errors.New("no bootstrap syscall observations")
	}
	events, err := controlEvents(f.traces[len(f.traces)-1].journal)
	if err != nil {
		return err
	}
	locked, fileSynced, bound, durable, ordinary := false, false, false, false, false
	created, dirSynced := map[string]bool{}, map[string]bool{}
	for _, e := range events {
		if e.Op == "flock" && e.Path == f.home && e.Flags&3 != 0 {
			return errors.New("command still locks the publicly readable home inode")
		}
		if e.Op == "flock" && e.Path == filepath.Join(f.coordination(), "control.lock") && e.Phase == "end" && e.Success {
			if e.Flags&2 != 0 {
				locked = true
			}
			if e.Flags&8 != 0 {
				locked = false
			}
		}
		if (e.Op == "mkdir" || e.Op == "create" || e.Op == "rename" || e.Op == "write" || e.Op == "fsync") && e.Phase == "begin" && strings.HasPrefix(e.Path, f.fixedRoot()+"/") && !locked && !f.bootstrapOnly(e) {
			return errors.New("managed-state mutation occurred outside private coordination exclusion")
		}
		if e.Phase == "end" && e.Success && e.Op == "mkdir" {
			if !locked && !f.bootstrapOnly(e) {
				return errors.New("non-bootstrap directory was created before private exclusion")
			}
			created[e.Path] = true
			dirSynced[e.Path], dirSynced[filepath.Dir(e.Path)] = false, false
		}
		if e.Phase == "end" && e.Success && e.Op == "fsync" {
			if e.Directory {
				dirSynced[e.Path] = true
				if bound && e.Path == f.coordination() {
					durable = true
				}
			} else if strings.HasPrefix(e.Path, f.coordination()+"/") {
				fileSynced = true
				if filepath.Base(e.Path) == "self.json" {
					bound = true
				}
			}
		}
		if e.Op == "rename" && e.Phase == "end" && e.Success && e.Path == filepath.Join(f.coordination(), "self.json") {
			bound = fileSynced
		}
		if e.Phase == "begin" && (e.Op == "mkdir" || e.Op == "create") && (e.Path == filepath.Join(f.fixedRoot(), "approvals") || strings.Contains(e.Path, "/approvals/")) {
			ordinary = true
			if !locked || !durable {
				return errors.New("ordinary approval storage preceded durable catalog self-binding under the private lock")
			}
			for path := range created {
				if strings.Contains(path, "/approvals") {
					continue
				}
				if !dirSynced[path] || !dirSynced[filepath.Dir(path)] {
					return fmt.Errorf("bootstrap component or parent was not synced before approval: %s", path)
				}
			}
		}
	}
	if !ordinary || !durable {
		return errors.New("missing durable self-binding before ordinary approval publication")
	}
	return nil
}

// Independently reconstruct each path/UID/inode binding from fixture metadata.
func controlBinding(path, home string, complete bool) (map[string]any, error) {
	var ancestors []any
	current, end := string(filepath.Separator), path
	if path == filepath.Join(home, ".local/state/itos-container") {
		end = filepath.Join(path, "coordination")
	}
	for _, part := range strings.Split(strings.TrimPrefix(end, string(filepath.Separator)), string(filepath.Separator)) {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) && !complete {
			break
		}
		if err != nil {
			return nil, err
		}
		stat := reflect.Indirect(reflect.ValueOf(info.Sys()))
		ancestors = append(ancestors, map[string]any{"path": current, "device": fmt.Sprint(stat.FieldByName("Dev")), "inode": fmt.Sprint(stat.FieldByName("Ino"))})
	}
	return map[string]any{"schema": 1, "uid": strconv.Itoa(os.Geteuid()), "path": path, "complete": complete, "ancestors": ancestors}, nil
}

func (f *controlFixture) selfBinding() error {
	want, err := controlBinding(f.fixedRoot(), f.home, true)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(filepath.Join(f.coordination(), "self.json"))
	if err != nil {
		return err
	}
	if !recoveryJSONEqual(string(data), want) {
		return errors.New("catalog self-binding differs from independently observed whole-tree identity")
	}
	return f.catalogOnlySelf()
}

func (f *controlFixture) catalogOnlySelf() error {
	entries, err := os.ReadDir(f.coordination())
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.Name() != "self.json" && entry.Name() != "control.lock" && (!strings.HasPrefix(entry.Name(), ".") || !strings.HasSuffix(entry.Name(), ".tmp")) {
			return errors.New("default tree acquired an extra catalog binding")
		}
	}
	return nil
}

func (f *controlFixture) noApprovalEntry() error {
	_, err := os.Lstat(filepath.Join(f.fixedRoot(), "approvals"))
	if !errors.Is(err, os.ErrNotExist) {
		return errors.New("failed bootstrap created ordinary approval state")
	}
	return nil
}

func (f *controlFixture) bootstrapFootprint(approval bool) error {
	if err := f.inspectionOnly(); err != nil {
		return err
	}
	now, err := f.inventory()
	if err != nil {
		return err
	}
	allowed := map[string]bool{}
	for _, path := range []string{filepath.Join(f.home, ".local"), filepath.Join(f.home, ".local/state"), f.fixedRoot(), f.coordination(), filepath.Join(f.coordination(), "self.json"), filepath.Join(f.coordination(), "control.lock")} {
		allowed[path] = true
	}
	for path, value := range now {
		if old, ok := f.bootstrapBefore[path]; ok && old == value {
			continue
		}
		privateTemp := filepath.Dir(path) == f.coordination() && strings.HasPrefix(filepath.Base(path), ".") && strings.HasSuffix(path, ".tmp")
		ordinary := approval && (path == filepath.Join(f.fixedRoot(), "approvals") || strings.HasPrefix(path, filepath.Join(f.fixedRoot(), "approvals")+"/"))
		if !allowed[path] && !privateTemp && !ordinary {
			return fmt.Errorf("bootstrap changed an unauthorized entry: %s", path)
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		mode := os.FileMode(0600)
		if info.IsDir() {
			mode = 0700
		}
		stat := reflect.Indirect(reflect.ValueOf(info.Sys()))
		if info.Mode().Perm() != mode || stat.FieldByName("Uid").Uint() != uint64(os.Geteuid()) || (!info.IsDir() && (!info.Mode().IsRegular() || stat.FieldByName("Nlink").Uint() != 1)) {
			return errors.New("bootstrap retained an unsafe entry")
		}
	}
	for path := range f.bootstrapBefore {
		if _, ok := now[path]; !ok {
			return fmt.Errorf("bootstrap deleted %s", path)
		}
	}
	return nil
}

func (f *controlFixture) captureBootstrap() error {
	f.surviving = map[string]sessionRecord{}
	return filepath.WalkDir(f.home, func(path string, e os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if e.IsDir() {
			info, err := e.Info()
			if err != nil {
				return err
			}
			stat := reflect.Indirect(reflect.ValueOf(info.Sys()))
			f.surviving[path] = sessionRecord{Bytes: "directory:" + fmt.Sprint(stat.FieldByName("Dev"), ":", stat.FieldByName("Ino"))}
			return nil
		}
		r, err := sessionReadRecord(path)
		f.surviving[path] = r
		return err
	})
}

func (f *controlFixture) retainedBootstrap() error {
	before := f.surviving
	if err := f.captureBootstrap(); err != nil {
		return err
	}
	for path, record := range before {
		if f.surviving[path] != record {
			return fmt.Errorf("retry or matching reuse replaced prior bootstrap evidence: %s", path)
		}
	}
	return nil
}

func (f *controlFixture) bootstrapReuse() {
	f.err = errors.Join(f.err, f.captureBootstrap())
	f.inspect("doctor", "JSON")
	f.err = errors.Join(f.err, f.unchanged())
	f.tracedApproval()
	f.results = append(f.results, doctorResult{exit: f.p.exit, stdout: f.p.stdout, stderr: f.p.stderr})
}

// Only mkdir/sync of the fixed private chain and creation of its lock inode may
// precede exclusion. Binding writes and all selected-tree mutations still wait.
func (f *controlFixture) bootstrapOnly(e controlEvent) bool {
	if e.Op == "mkdir" || (e.Op == "fsync" && e.Directory) {
		for _, path := range []string{f.home, filepath.Join(f.home, ".local"), filepath.Join(f.home, ".local/state"), f.fixedRoot(), f.coordination()} {
			if e.Path == path {
				return true
			}
		}
	}
	return e.Op == "create" && e.Path == filepath.Join(f.coordination(), "control.lock")
}
