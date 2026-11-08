package features

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"time"
)

type runEntry struct {
	Data                        string
	Mode                        os.FileMode
	Owner, Links, Device, Inode any
	Modified                    int64
}
type runObservation struct {
	approvalErr error
	result      doctorResult
	journal     []doctorRequest
	connections int64
	data        []byte
	info        os.FileInfo
	snapshotErr error
	pid         int
}
type runWorld struct {
	before              map[string]runEntry
	mask                string
	imageObservation    bool
	results             []runObservation
	original            []byte
	originalInfo        os.FileInfo
	contract            map[string]any
	uid, gid, condition string
	partial             error
}

func runTree(root string) (map[string]runEntry, error) {
	out := map[string]runEntry{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		e := runEntry{Mode: info.Mode(), Modified: info.ModTime().UnixNano()}
		st := reflect.Indirect(reflect.ValueOf(info.Sys()))
		if st.IsValid() && st.Kind() == reflect.Struct {
			for key, target := range map[string]*any{"Uid": &e.Owner, "Nlink": &e.Links, "Dev": &e.Device, "Ino": &e.Inode} {
				v := st.FieldByName(key)
				if v.IsValid() {
					*target = v.Interface()
				}
			}
		}
		if info.Mode().IsRegular() {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			e.Data = string(data)
		} else if info.Mode()&os.ModeSymlink != 0 {
			e.Data, err = os.Readlink(path)
			if err != nil {
				return err
			}
		}
		out[path] = e
		return nil
	})
	return out, err
}
func (w *world) runSnapshotPath() string {
	return filepath.Join(filepath.Dir(filepath.Dir(filepath.Dir(w.approval.record))), "snapshots", w.approval.project, w.approval.generation+".json")
}
func (w *world) runCheckpoint() error {
	r := w.runFixture()
	var err error
	r.before, err = runTree(w.dir)
	if err != nil {
		return err
	}
	return w.approvalCapture()
}
func (w *world) runSameTree(allowSnapshot bool) error {
	got, err := runTree(w.dir)
	if err != nil {
		return err
	}
	before := w.runFixture().before
	root := filepath.Dir(filepath.Dir(w.runSnapshotPath()))
	registry := map[string]bool{}
	if allowSnapshot {
		registry, err = w.localRegistryPaths(got)
		if err != nil {
			return err
		}
	}
	for path, want := range before {
		actual, ok := got[path]
		if !ok {
			return fmt.Errorf("removed existing entry %s", path)
		}
		if allowSnapshot && want.Mode.IsDir() && (path == root || strings.HasPrefix(root, path+string(filepath.Separator)) || registry[path]) {
			actual.Modified = want.Modified
			actual.Links = want.Links
		}
		if !reflect.DeepEqual(actual, want) {
			return fmt.Errorf("changed existing entry %s", path)
		}
	}
	allowed := map[string]bool{root: true, filepath.Dir(w.runSnapshotPath()): true, w.runSnapshotPath(): true}
	for path := range got {
		if _, ok := before[path]; !ok && (!allowSnapshot || (!allowed[path] && !w.localIntentPath(path) && !registry[path])) {
			return fmt.Errorf("unexpected new entry %s", path)
		}
	}
	return nil
}
func (w *world) runSnapshot(generation string) error {
	data, err := os.ReadFile(w.runSnapshotPath())
	if err != nil {
		return fmt.Errorf("snapshot missing/unreadable: %w", err)
	}
	return w.runValidateSnapshot(data, generation)
}
func (w *world) runValidateSnapshot(data []byte, generation string) error {
	a := w.approval
	want := approvalJSON(map[string]any{"schema": 1, "project": a.project, "generation": generation, "fingerprint": a.fingerprint, "settings": a.settings})
	var obj map[string]any
	if err := json.Unmarshal(data, &obj); err != nil {
		return fmt.Errorf("snapshot not a complete JSON object: %w", err)
	}
	if string(data) != string(want) {
		return fmt.Errorf("snapshot differs from independently calculated canonical envelope")
	}
	for path := w.runSnapshotPath(); ; path = filepath.Dir(path) {
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		mode := os.FileMode(0700)
		if path == w.runSnapshotPath() {
			mode = 0600
			if !info.Mode().IsRegular() {
				return fmt.Errorf("snapshot not regular")
			}
		}
		st := reflect.Indirect(reflect.ValueOf(info.Sys()))
		if info.Mode().Perm() != mode || fmt.Sprint(st.FieldByName("Uid").Interface()) != a.uid || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("unsafe snapshot entry %s", path)
		}
		if path == w.runSnapshotPath() && fmt.Sprint(st.FieldByName("Nlink").Interface()) != "1" {
			return fmt.Errorf("snapshot has extra links")
		}
		if filepath.Base(path) == "itos-container" {
			break
		}
	}
	return nil
}
func (w *world) runProblem(rule string, code int, subject string) error {
	if w.exit != code || w.stdout != "" {
		return fmt.Errorf("preflight result expected exit %d and empty stdout: %s", code, w.report())
	}
	switch subject {
	case "checkout path":
		subject = w.doctor.checkout
	case "requested image":
		subject = w.doctor.image
	case "snapshot path":
		subject = w.runSnapshotPath()
	}
	if !strings.Contains(w.stderr, "itos-container:") || !strings.Contains(w.stderr, rule) || !strings.Contains(w.stderr, subject) || strings.Contains(w.stderr, "y/N") || strings.Contains(w.stderr, "approval matched") {
		return fmt.Errorf("missing safe preflight cause/subject or premature success: %s", w.report())
	}
	// Require explanation and actionable guidance beyond just echoing the rule.
	causes := map[string][]string{
		"policy.missing": {"policy", "create"}, "policy.network_mode": {"offline", "use"}, "manifest.missing": {"manifest", "create"}, "manifest.unsupported": {"supported", "remove"}, "manifest.duplicate": {"repeated", "once"},
		"approval.identity.mismatch": {"effective host UID", "omit"}, "doctor.checkout.path": {"checkout", "use"}, "doctor.checkout.changed": {"changed", "inspect again"}, "doctor.engine.changed": {"changed", "inspecting again"},
		"doctor.engine.identity": {"identity", "engine"}, "doctor.engine.unsupported": {"engine", "Docker"}, "doctor.engine.platform": {"linux", "amd64"}, "doctor.image.missing": {"image", "host"}, "doctor.image.digest": {"digest", "image"}, "doctor.image.platform": {"linux", "amd64"}, "doctor.image.inspect": {"image", "host operator"}, "manifest.metadata": {"metadata", "strict JSON"},
		"run.snapshot.unsafe": {"unsafe", "state"}, "run.snapshot.invalid": {"invalid", "state"},
	}
	for _, word := range causes[rule] {
		if !strings.Contains(strings.ToLower(w.stderr), strings.ToLower(word)) {
			return fmt.Errorf("missing cause/fix %q: %s", word, w.report())
		}
	}
	return nil
}
func (w *world) runJournal(journal []doctorRequest, class string, image bool) error {
	if class == "empty" {
		if len(journal) != 0 {
			return fmt.Errorf("unexpected engine requests: %v", journal)
		}
		return nil
	}
	seen := map[string]bool{}
	for _, r := range journal {
		if !doctorAllowedRequest(r, w.doctor.image, true) {
			return fmt.Errorf("forbidden API request: %v", r)
		}
		seen[doctorRequestPath(r.Path)] = true
	}
	required := []string{"/version", "/info"}
	if image {
		required = append(required, "/images/"+w.doctor.image+"/json")
	}
	if class == "complete" {
		required = append(required, "/_ping")
	}
	for _, path := range required {
		if !seen[path] {
			return fmt.Errorf("missing inspection %s in %v", path, journal)
		}
	}
	return nil
}

type runProcess struct {
	approval map[string]runEntry
	cmd      *exec.Cmd
	cancel   context.CancelFunc
	done     chan struct{}
	out, err strings.Builder
	start    time.Time
	count    int64
	coverage string
}

func (w *world) startRun(env []string, args ...string) (*runProcess, error) {
	originalArgs := append([]string{}, args...)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	bin := w.bin
	if w.doctor.engine.offline != nil && len(args) > 0 && args[0] == "run" {
		var err error
		bin, err = w.sessionProbeBinary()
		if err != nil {
			cancel()
			return nil, err
		}
		args = append([]string{"-test.run=^TestLocalRunCommand$", "-test.count=1", "--"}, args...)
		env = append(env, "SLICE_17_LOCAL_RUN=1", "SLICE_17_LOCAL_MOUNT_ROOT="+w.doctor.checkout)
	}
	r := w.runFixture()
	if r.mask != "" {
		helper, err := os.Executable()
		if err != nil {
			cancel()
			return nil, err
		}
		args = append([]string{"-test.run=^TestRunMaskHelper$", "--", "slice-8-mask", r.mask, bin}, args...)
		bin = helper
	}
	approvalBefore, err := runTree(filepath.Dir(w.approval.record))
	if err != nil && !os.IsNotExist(err) {
		cancel()
		return nil, err
	}
	p := &runProcess{approval: approvalBefore, cancel: cancel, done: make(chan struct{}), start: time.Now(), count: w.doctor.engine.listener.connections.Load()}
	if cover := os.Getenv("GOCOVERDIR"); cover != "" && r.mask != "" {
		dir, err := os.MkdirTemp(cover, "slice-8-mask-")
		if err != nil {
			cancel()
			return nil, err
		}
		p.coverage = dir
		env = setEnv(env, "GOCOVERDIR", dir)
	}
	p.cmd = exec.CommandContext(ctx, bin, args...)
	p.cmd.Env = env
	p.cmd.Dir = w.doctor.cwd
	p.cmd.Stdout = &p.out
	p.cmd.Stderr = &p.err
	p.cmd.WaitDelay = time.Second
	if err := w.startLocalCommand(p.cmd); err != nil {
		cancel()
		return nil, err
	}
	if a := w.doctor.engine.offline; a != nil {
		for i, arg := range originalArgs {
			if arg == "--" {
				a.mu.Lock()
				a.expected[p.cmd.Process.Pid] = append([]string{}, originalArgs[i+1:]...)
				a.mu.Unlock()
				break
			}
		}
	}
	go func() {
		_ = p.cmd.Wait()
		unlock := w.lockLocalAPI()
		defer unlock()
		close(p.done)
	}()
	return p, nil
}
func (w *world) finishRun(p *runProcess) (runObservation, error) {
	<-p.done
	p.cancel()
	r := runObservation{result: doctorResult{stdout: p.out.String(), stderr: p.err.String(), exit: p.cmd.ProcessState.ExitCode(), elapsed: time.Since(p.start)}, pid: p.cmd.Process.Pid, connections: w.doctor.engine.listener.connections.Load() - p.count}
	for _, request := range w.doctor.engine.requests() {
		if request.PID == r.pid {
			r.journal = append(r.journal, request)
		}
	}
	after, afterErr := runTree(filepath.Dir(w.approval.record))
	if afterErr != nil && !os.IsNotExist(afterErr) {
		r.approvalErr = afterErr
	} else if !reflect.DeepEqual(after, p.approval) {
		r.approvalErr = fmt.Errorf("run rewrote approval state")
	}
	r.info, r.snapshotErr = os.Lstat(w.runSnapshotPath())
	if r.snapshotErr == nil && r.info.Mode().IsRegular() {
		r.data, r.snapshotErr = os.ReadFile(w.runSnapshotPath())
	} else if r.snapshotErr == nil {
		r.snapshotErr = fmt.Errorf("snapshot is not a regular file")
	}
	if p.coverage != "" {
		defer func() { _ = os.RemoveAll(p.coverage) }()
		if err := exportApprovalKernelCoverage(p.coverage, os.Getenv("GOCOVERDIR")); err != nil {
			return r, err
		}
	}
	return r, nil
}
func (w *world) runPreflightProcess(env []string, args ...string) error {
	p, err := w.startRun(env, args...)
	if err != nil {
		return err
	}
	r, err := w.finishRun(p)
	w.run.results = append(w.run.results, r)
	w.stdout, w.stderr, w.exit = r.result.stdout, r.result.stderr, r.result.exit
	return err
}
func (w *world) runObservedSnapshot(index int, g string) error {
	r := w.runFixture()
	if len(r.results) <= index {
		return fmt.Errorf("missing invocation %d", index+1)
	}
	o := r.results[index]
	if o.snapshotErr != nil {
		return fmt.Errorf("invocation %d snapshot: %w", index+1, o.snapshotErr)
	}
	return w.runValidateSnapshot(o.data, g)
}
func (w *world) runRace(identity string) error {
	p, err := w.startRun(w.env(), "run", "--", "fixture-workload")
	if err != nil {
		return err
	}
	select {
	case <-p.done:
	case <-w.doctor.engine.reached:
		w.doctor.engine.mu.Lock()
		err = w.approvalEdit(identity)
		w.doctor.engine.mu.Unlock()
		if err == nil {
			err = w.runCheckpoint()
		}
		w.doctor.engine.unpause()
	case <-time.After(6 * time.Second):
		w.doctor.engine.unpause()
	}
	o, finish := w.finishRun(p)
	w.run.results = append(w.run.results, o)
	w.stdout, w.stderr, w.exit = o.result.stdout, o.result.stderr, o.result.exit
	return errors.Join(err, finish)
}
func (w *world) runTwice(change string) error {
	if err := w.runPreflightProcess(w.env(), "run", "--", "fixture-workload"); err != nil {
		return err
	}
	o := w.run.results[0]
	w.run.original = append([]byte{}, o.data...)
	w.run.originalInfo = o.info
	var err error
	switch change {
	case "active approval is removed":
		err = os.Remove(w.approval.record)
	case "active approval is truncated":
		err = os.WriteFile(w.approval.record, []byte(`{"schema":`), 0600)
	case "literal manifest environment":
		err = w.approvalEdit("change the literal application environment")
	case "manifest requests another digest":
		err = w.runChangeImage()
	case "daemon ID at the same endpoint":
		err = w.approvalEdit("daemon ID")
	case "cached image config digest":
		err = w.approvalEdit("the selected image config ID")
	case "requested image becomes absent":
		w.doctor.engine.imageStatus = 404
	default:
		err = fmt.Errorf("unknown change %q", change)
	}
	if err != nil {
		return err
	}
	if err = w.runCheckpoint(); err != nil {
		return err
	}
	return w.runPreflightProcess(w.env(), "run", "--", "fixture-workload")
}
func (w *world) runSecondProblem(rule string, exit int, journal string) error {
	if len(w.run.results) != 2 {
		return fmt.Errorf("missing runs")
	}
	o := w.run.results[1]
	if o.result.exit != exit || o.result.stdout != "" || !strings.Contains(o.result.stderr, rule) || strings.Contains(o.result.stderr, "approval matched") {
		return fmt.Errorf("second result: %+v", o.result)
	}
	if journal == "empty" && o.connections != 0 {
		return fmt.Errorf("second run connected to daemon")
	}
	return w.runJournal(o.journal, journal, true)
}
func (w *world) runUnchangedRecords() error {
	for _, r := range w.run.results {
		if r.approvalErr != nil {
			return r.approvalErr
		}
	}
	data, err := os.ReadFile(w.runSnapshotPath())
	if err != nil {
		return err
	}
	info, err := os.Lstat(w.runSnapshotPath())
	if err != nil {
		return err
	}
	if w.run.originalInfo == nil || string(data) != string(w.run.original) || !os.SameFile(info, w.run.originalInfo) || !info.ModTime().Equal(w.run.originalInfo.ModTime()) {
		return fmt.Errorf("retained snapshot rewritten")
	}
	return w.approvalRecordUnchanged()
}
func (w *world) runNoEffects() error {
	for _, o := range w.run.results {
		for _, r := range o.journal {
			if !doctorAllowedRequest(r, w.doctor.image, true) {
				return fmt.Errorf("forbidden request %v", r)
			}
		}
	}
	return w.doctorNoMarker()
}
func (w *world) runSnapshotDefect(defect string) error {
	w.runFixture()
	path := w.runSnapshotPath()
	if err := writeManifestFixture(path, w.approval.recordBefore, 0600); err != nil {
		return err
	}
	target := filepath.Join(w.dir, "slice-8-outside-target")
	switch defect {
	case "snapshots directory is a symlink":
		path = filepath.Dir(filepath.Dir(path))
		fallthrough
	case "project directory is a symlink", "final record is a symlink":
		if defect == "project directory is a symlink" {
			path = filepath.Dir(path)
		}
		if err := os.Rename(path, target); err != nil {
			return err
		}
		return os.Symlink(target, path)
	case "project directory has mode 0777":
		return os.Chmod(filepath.Dir(path), 0777)
	case "final record is a FIFO":
		if err := os.Remove(path); err != nil {
			return err
		}
		return approvalFIFO(path)
	case "final record has another hard link":
		return os.Link(path, target)
	case "final record has mode 0644":
		return os.Chmod(path, 0644)
	case "final record is truncated JSON":
		return os.WriteFile(path, []byte(`{"schema":`), 0600)
	case "final record contains other settings":
		settings := approvalClone(w.approval.settings)
		settings["manifest"].(map[string]any)["containerEnv"] = map[string]any{"APP": "other"}
		obj := map[string]any{"schema": 1, "generation": w.approval.generation, "project": w.approval.project, "fingerprint": "sha256:" + approvalHash(settings), "settings": settings}
		return os.WriteFile(path, approvalJSON(obj), 0600)
	default:
		return fmt.Errorf("unknown snapshot defect %q", defect)
	}
}
func (w *world) runThree(schedule string) error {
	stop, stopped := make(chan struct{}), make(chan struct{})
	expected := approvalJSON(map[string]any{"schema": 1, "project": w.approval.project, "generation": w.approval.generation, "fingerprint": w.approval.fingerprint, "settings": w.approval.settings})
	go func() {
		defer close(stopped)
		ticker := time.NewTicker(50 * time.Microsecond)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				info, err := os.Lstat(w.runSnapshotPath())
				if os.IsNotExist(err) {
					continue
				}
				if err != nil {
					w.run.partial = err
					return
				}
				if !info.Mode().IsRegular() {
					w.run.partial = fmt.Errorf("published snapshot not regular")
					return
				}
				data, err := os.ReadFile(w.runSnapshotPath())
				if err != nil || string(data) != string(expected) {
					w.run.partial = fmt.Errorf("observed incomplete snapshot: %v", err)
					return
				}
			}
		}
	}()
	defer func() { close(stop); <-stopped }()

	var children []*runProcess
	var mu sync.Mutex
	reached := make(chan struct{})
	release := make(chan struct{})
	seen := map[int]bool{}
	if schedule == "concurrently" {
		w.doctor.engine.runBarrier = func(pid int) {
			mu.Lock()
			seen[pid] = true
			if len(seen) == 2 {
				select {
				case <-reached:
				default:
					close(reached)
				}
			}
			mu.Unlock()
			<-release
		}
	}
	for range 2 {
		p, err := w.startRun(w.env(), "run", "--", "fixture-workload")
		if err != nil {
			return err
		}
		children = append(children, p)
		if schedule == "sequentially" {
			o, err := w.finishRun(p)
			if err != nil {
				return err
			}
			w.run.results = append(w.run.results, o)
		}
	}
	if schedule == "concurrently" {
		bothDone := make(chan struct{})
		go func() {
			for _, p := range children {
				<-p.done
			}
			close(bothDone)
		}()
		select {
		case <-reached:
		case <-bothDone:
		case <-time.After(5 * time.Second):
		}
		close(release)
		for _, p := range children {
			o, err := w.finishRun(p)
			if err != nil {
				return err
			}
			w.run.results = append(w.run.results, o)
		}
		w.doctor.engine.runBarrier = nil
	}
	return w.runPreflightProcess(w.env(), "run", "--", "fixture-workload")
}
func (w *world) runSnapshotStable() error {
	if len(w.run.results) != 3 {
		return fmt.Errorf("missing runs")
	}
	first := w.run.results[0]
	for _, o := range w.run.results {
		if o.info == nil || first.info == nil || string(o.data) != string(first.data) || !os.SameFile(o.info, first.info) || !o.info.ModTime().Equal(first.info.ModTime()) {
			return fmt.Errorf("snapshot changed across runs")
		}
	}
	return w.run.partial
}
