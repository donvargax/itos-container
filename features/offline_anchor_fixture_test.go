package features

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"
)

type anchorReport struct {
	Phase, Identity, Nonce, CWD string
	Denied, Controls            map[string]bool
}

type anchorVolume struct {
	name            string
	labels, options map[string]any
	observed        map[string]any
	beforeWorker    bool
	absent          bool
	retained        map[string]any
}

type anchorWorld struct {
	control                           *world
	ready, release                    chan struct{}
	held, reached                     bool
	pid                               int
	original, replacement, originalID string
	nonce, otherNonce                 string
	volumes                           map[string]*anchorVolume
	records, checkedRecords           map[string]sessionRecord
	replacementBefore                 map[string]string
	before, running                   map[string]any
	reports                           []anchorReport
	nested, nestedOK                  bool
	err                               error
	retention                         bool
	child                             int
	workerAfter                       map[string]any
	retainedRecords                   map[string]sessionRecord
	originalBefore, protectedBefore   map[string]string
	protectedAfter                    map[string]string
}

func anchorIdentity(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	stat := reflect.Indirect(reflect.ValueOf(info.Sys()))
	if !stat.IsValid() || stat.Kind() != reflect.Struct || !stat.FieldByName("Dev").IsValid() || !stat.FieldByName("Ino").IsValid() {
		return "", errors.New("native identity unavailable")
	}
	return fmt.Sprintf("%v:%v", stat.FieldByName("Dev"), stat.FieldByName("Ino")), nil
}

func (w *world) anchorFixture() error {
	if err := doctorRealGuard(); err != nil {
		return err
	}
	if err := w.prepareAnchor(""); err != nil {
		return err
	}
	control := &world{root: w.root, bin: w.bin, coverdir: w.coverdir, dir: filepath.Join(w.dir, "slice-17-control")}
	w.anchor.control = control // preserve partially prepared fixture for cleanup
	if err := os.Mkdir(control.dir, 0700); err != nil {
		return err
	}
	return control.prepareAnchor(filepath.Join(w.anchor.original, "slice-17-probe"))
}

func (w *world) prepareAnchor(probe string) error {
	if err := w.offlineFixture("shell"); err != nil {
		return err
	}
	s := &anchorWorld{nonce: isolationNonce(), otherNonce: isolationNonce(), volumes: map[string]*anchorVolume{}}
	w.anchor = s
	// The replaceable ancestor contains only the checkout, never host state or
	// the recorder. Substitution therefore cannot interrupt fixture controls.
	parent := filepath.Join(w.dir, "slice-17-parent")
	if err := os.Mkdir(parent, 0700); err != nil {
		return err
	}
	s.original = filepath.Join(parent, "checkout")
	if err := os.Rename(w.doctor.checkout, s.original); err != nil {
		return err
	}
	w.doctor.checkout, w.doctor.cwd = s.original, s.original
	s.replacement = filepath.Join(w.dir, "slice-17-replacement", "checkout")
	if err := os.MkdirAll(filepath.Join(s.replacement, "subdir"), 0700); err != nil {
		return err
	}
	if probe == "" {
		probe = filepath.Join(s.original, "slice-17-probe")
		if err := w.buildAnchorProbe(probe); err != nil {
			return err
		}
	}
	data, err := os.ReadFile(probe)
	if err != nil {
		return err
	}
	for _, root := range []string{s.original, s.replacement} {
		if err = os.WriteFile(filepath.Join(root, "slice-17-probe"), data, 0700); err != nil {
			return err
		}
	}
	for path, value := range map[string]string{
		filepath.Join(s.original, "slice-17-nonce"):          s.nonce,
		filepath.Join(s.replacement, "slice-17-nonce"):       s.otherNonce,
		filepath.Join(s.replacement, "slice-17-replacement"): s.otherNonce,
	} {
		if err = os.WriteFile(path, []byte(value), 0600); err != nil {
			return err
		}
	}
	s.originalID, err = anchorIdentity(s.original)
	if err != nil {
		return err
	}
	otherID, err := anchorIdentity(s.replacement)
	if err != nil || otherID == s.originalID {
		return errors.New("original and replacement identities must differ")
	}
	source := filepath.Join(w.doctor.host, "slice-17-nested-source")
	target := filepath.Join(s.original, "slice-17-nested")
	if err = os.Mkdir(source, 0700); err != nil {
		return err
	}
	if err = os.Mkdir(target, 0700); err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(source, "marker"), []byte(s.nonce), 0600); err != nil {
		return err
	}
	// This CI fixture mount creates the hostile nested source; it is not a
	// controller operation or evidence that the product has mount authority.
	if err = isolationSudo("mount", "--bind", source, target); err != nil {
		return err
	}
	s.nested = true
	if value, readErr := os.ReadFile(filepath.Join(target, "marker")); readErr != nil || string(value) != s.nonce {
		return errors.New("nested mount positive control failed")
	}
	if err = w.approvalSeed("1", false); err != nil {
		return err
	}
	s.replacementBefore, err = anchorTree(s.replacement)
	if err != nil {
		return err
	}
	return w.offlineCheckpoint()
}

func (w *world) buildAnchorProbe(destination string) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "build", "-trimpath", "-o", destination, filepath.Join(w.root, "features", "testdata", "slice-17", "probe.go"))
	cmd.Dir, cmd.Env = w.root, sessionChildEnvironment(os.Environ())
	for k, v := range map[string]string{"CGO_ENABLED": "0", "GOOS": "linux", "GOARCH": "amd64", "GOFLAGS": "", "GOWORK": "off"} {
		cmd.Env = setEnv(cmd.Env, k, v)
	}
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("compile slice-17 probe: %w\n%s", err, output)
	}
	return nil
}

func (w *world) anchorRace(replacement string) error {
	w.anchorExecute(replacement)
	w.anchor.control.anchorExecute("")
	return nil // all action failures are observations for Then
}

func (w *world) anchorSubstitute(kind string) error {
	s := w.anchor
	root := w.doctor.checkout
	if strings.HasPrefix(kind, "checkout ancestor") {
		oldParent := filepath.Dir(root)
		moved := filepath.Join(w.dir, "slice-17-moved-parent")
		if err := os.Rename(oldParent, moved); err != nil {
			return err
		}
		s.original = filepath.Join(moved, "checkout")
		if strings.HasSuffix(kind, "symlink") {
			return os.Symlink(filepath.Dir(s.replacement), oldParent)
		}
		if err := os.Rename(filepath.Dir(s.replacement), oldParent); err != nil {
			return err
		}
		s.replacement = root
		return nil
	}
	moved := filepath.Join(w.dir, "slice-17-moved-checkout")
	if err := os.Rename(root, moved); err != nil {
		return err
	}
	s.original = moved
	if strings.HasSuffix(kind, "symlink") {
		return os.Symlink(s.replacement, root)
	}
	if err := os.Rename(s.replacement, root); err != nil {
		return err
	}
	s.replacement = root
	return nil
}

func (w *world) anchorExecute(replacement string) {
	s, o := w.anchor, w.offline
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, w.bin, "run", "--", "/workspace/slice-17-probe", strconv.Itoa(s.child))
	cmd.Dir, cmd.Env = w.doctor.checkout, w.env()
	for k, v := range w.doctor.env {
		cmd.Env = setEnv(cmd.Env, k, v)
	}
	cmd.Env = setEnv(cmd.Env, "ITOS_ROOT", w.doctor.checkout)
	var out, errout offlineBuffer
	cmd.Stdout, cmd.Stderr = &out, &errout
	stdin, err := cmd.StdinPipe()
	if err != nil {
		s.err = err
		return
	}
	defer func() { _ = stdin.Close() }()
	if err = cmd.Start(); err != nil {
		s.err = err
		return
	}
	o.api.mu.Lock()
	s.pid = cmd.Process.Pid
	o.api.mu.Unlock()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	ready, checked := false, false
	startReady := s.ready
	tick := time.NewTicker(5 * time.Millisecond)
	defer tick.Stop()
	waiting := true
	for waiting {
		select {
		case <-startReady:
			s.reached = true
			startReady = nil
			s.records, err = w.anchorRecords()
			s.err = errors.Join(s.err, err)
			if replacement != "" {
				s.err = errors.Join(s.err, w.anchorSubstitute(replacement))
			}
			close(s.release)
		case err = <-done:
			waiting = false
		case <-ctx.Done():
			err = <-done
			s.err = errors.Join(s.err, errors.New("native anchor action exceeded its deadline"))
			waiting = false
		case <-tick.C:
			reports := anchorReports(out.bytes())
			if len(reports) > 0 && !ready {
				ready = true
				if replacement == "" || s.retention {
					s.records, err = w.anchorRecords()
					s.err = errors.Join(s.err, err)
				}
				o.api.mu.Lock()
				if len(o.api.created) == 1 {
					s.running, err = o.api.inspect(o.api.created[0])
					s.err = errors.Join(s.err, err)
				}
				o.api.mu.Unlock()
				if s.retention {
					s.originalBefore, err = anchorTree(s.original)
					s.err = errors.Join(s.err, err)
					s.protectedBefore, err = w.anchorSelectedRecords()
					s.err = errors.Join(s.err, err)
					if replacement != "" {
						s.err = errors.Join(s.err, w.anchorSubstitute(replacement))
					}
					s.reached = true
				}
				paths := make([]string, 0, len(s.records))
				for path := range s.records {
					paths = append(paths, path)
				}
				slices.Sort(paths)
				s.err = errors.Join(s.err, json.NewEncoder(stdin).Encode(paths))
			}
			if len(reports) > 1 && !checked {
				checked = true
				s.checkedRecords, err = w.anchorRecords()
				s.err = errors.Join(s.err, err, json.NewEncoder(stdin).Encode(true))
				_ = stdin.Close()
			}
		}
	}
	o.exit = 0
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			o.exit = exit.ExitCode()
		} else {
			s.err = errors.Join(s.err, err)
		}
	}
	o.stdout, o.stderr = out.bytes(), errout.bytes()
	s.reports = anchorReports(o.stdout)
	s.err = errors.Join(s.err, o.api.observeAfter())
	o.resourcesAfter, err = w.doctor.real.snapshotResources()
	s.err = errors.Join(s.err, err)
	for _, v := range s.volumes {
		v.absent = w.doctor.real.read("/volumes/"+v.name, nil, 404) == nil
		if s.retention && !v.absent {
			v.retained, err = o.api.anchorInspect(v)
			s.err = errors.Join(s.err, err)
		}
	}
	value, readErr := os.ReadFile(filepath.Join(s.original, "slice-17-nested", "marker"))
	s.nestedOK = readErr == nil && string(value) == s.nonce
	if s.retention {
		s.retainedRecords, err = w.anchorRecords()
		s.err = errors.Join(s.err, err)
		s.protectedAfter, err = w.anchorSelectedRecords()
		s.err = errors.Join(s.err, err)
		o.api.mu.Lock()
		if len(o.api.created) == 1 && !o.api.absent[o.api.created[0]] {
			s.workerAfter, err = o.api.inspect(o.api.created[0])
			s.err = errors.Join(s.err, err)
		}
		o.api.mu.Unlock()
	}
}

func anchorReports(data []byte) []anchorReport {
	var reports []anchorReport
	for _, line := range bytes.Split(data, []byte{'\n'}) {
		var r anchorReport
		if json.Unmarshal(line, &r) == nil && (r.Phase == "ready" || r.Phase == "checked") {
			reports = append(reports, r)
		}
	}
	return reports
}

func (w *world) anchorRecords() (map[string]sessionRecord, error) {
	w.offline.api.mu.Lock()
	var names []string
	for name := range w.anchor.volumes {
		names = append(names, name)
	}
	w.offline.api.mu.Unlock()
	records := map[string]sessionRecord{}
	root := filepath.Join(w.doctor.env["XDG_STATE_HOME"], "itos-container")
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.Type().IsRegular() || filepath.Ext(path) != ".json" {
			return nil
		}
		record, err := sessionReadRecord(path)
		if err != nil {
			return err
		}
		for _, name := range names {
			if strings.Contains(record.Bytes, name) {
				records[path] = record
				break
			}
		}
		return nil
	})
	return records, err
}

func (w *world) closeAnchor() error {
	if w.anchor == nil {
		return nil
	}
	s := w.anchor
	var err error
	if control := s.control; control != nil {
		err = errors.Join(control.closeOfflineRuntime(), control.closeAnchor(), control.closeSession())
		control.closeDoctor()
	}
	if s.nested {
		err = errors.Join(err, isolationSudo("umount", filepath.Join(s.original, "slice-17-nested")))
	}
	return err
}
