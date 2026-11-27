package features

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/cucumber/godog"
)

// slice-60: the next control-lock holder removes orphaned publisher temporaries.
// No Given invokes the product: every run, reader and writer starts in a When.
// "Keeps" compares one entry's type, mode, owner, bytes, device, inode, link count
// and mtime. A directory keeps its identity and mode only: a removal inside it
// moves its mtime, and nothing here claims otherwise.
type sweepEntry struct {
	kind, mode, owner, dev, ino, nlink, mtime, size, sum, raw string
}

type sweepState struct {
	location, command string
	subject           *world
	run               *overlapRun
	root              string
	temporaries       []string          // every planted temporary, in the order planted
	twins             map[string]string // linked temporary -> the record it is a second link of
	genuine           []string
	listed            []string
	outside           string
	atPlant, atWriter map[string]sweepEntry
	doctor            doctorResult
	status            recoveryEntry
	statusErr         error
	boundary          string
	pausedName        string
	pausedDir         string
	paused, record    string
	beside, admission string
	capturedPaused    map[string]sweepEntry
	writer            *overlapRun
	releasedAt        time.Time
}

func (w *world) controlSweepSteps(sc *godog.ScenarioContext) {
	sc.Step(`^a control-state fixture for a sweep of the "([^"]*)" session tree by P "([^"]*)"$`, w.sweepSetup)
	sc.Step(`^the "([^"]*)" project's fixture run finishes with complete cleanup and the fixture plants a lone and a linked publisher temporary in each of its admission collection, intent directory and session directory$`, func(location string) error {
		return w.control.sweepRunAndPlant(location, (*sweepState).plantSix)
	})
	sc.Step(`^a fresh P doctor and a fresh status of that stopped session read the tree$`, func() error { return w.control.sweepRead() })
	sc.Step(`^both readers succeed and every planted temporary and record keeps its bytes inode link count and mtime$`, func() error { return w.control.sweepReadersKept() })
	sc.Step(`^P runs "([^"]*)" as the next writer of the control lock$`, func(command string) error { return w.control.sweepWrite(command) })
	sc.Step(`^the writer succeeds and only the six planted temporaries are gone from the swept tree$`, func() error { return w.control.sweepGone(nil) })
	sc.Step(`^each record that had a planted second link keeps its bytes inode and mtime with one link$`, func() error { return w.control.sweepTwinsKept() })
	sc.Step(`^a fresh status reports the stopped session unchanged with complete cleanup$`, func() error { return w.control.sweepStatusUnchanged() })
	sc.Step(`^the "([^"]*)" project's fixture run finishes with complete cleanup and the fixture plants these entries Publish never leaves:$`, func(location string, table *godog.Table) error {
		return w.control.sweepRunAndPlant(location, func(s *sweepState) error { return s.plantListed(table) })
	})
	sc.Step(`^the fixture also plants a genuine lone publisher temporary in W's admission collection and in W's intent directory$`, func() error { return w.control.sweepGenuine() })
	sc.Step(`^the writer succeeds and only the two genuine lone temporaries are gone from the swept tree$`, func() error {
		return w.control.sweepGone(w.control.sweep.genuine)
	})
	sc.Step(`^every listed entry keeps its name type mode bytes inode link count and mtime$`, func() error { return w.control.sweepListedKept() })
	sc.Step(`^a control-state fixture whose W run will pause before its first "([^"]*)" replacement after registration once that replacement's temporary is durable$`, w.sweepPauseSetup)
	sc.Step(`^W pauses there and the fixture plants a lone publisher temporary in W's admission collection and another beside W's paused temporary$`, func() error { return w.control.sweepPause() })
	sc.Step(`^P approves with its exact fingerprint as the next writer of the control lock while W stays paused$`, func() error { return w.control.sweepWrite("approve") })
	sc.Step(`^the writer succeeds and the temporary planted in W's admission collection is gone$`, func() error { return w.control.sweepPausedWriter() })
	sc.Step(`^W's paused temporary its record and the temporary planted beside them keep their bytes inode link count and mtime$`, func() error { return w.control.sweepPausedKept() })
	sc.Step(`^the fixture releases W and ends W's workload$`, func() error { return w.control.sweepRelease() })
	sc.Step(`^W's paused rename evidence shows its "([^"]*)" published with the paused temporary's bytes device and inode and one link$`, func(record string) error { return w.control.sweepPausedEvidence(record) })
	sc.Step(`^W left no publisher temporary of its own in its intent directory or session directory$`, func() error { return w.control.sweepNoOwnTemporaries() })
	sc.Step(`^W's run exits zero with complete cleanup$`, func() error { return w.control.sweepPausedRunEnded() })
}

func (w *world) sweepSetup(location, command string) error {
	if location != "enrolled" && location != "selected" {
		return fmt.Errorf("unknown swept tree %q", location)
	}
	if command != "approve" && command != "run" {
		return fmt.Errorf("unknown writer %q", command)
	}
	if err := w.controlSafe("state path shares only a textual prefix with W"); err != nil {
		return err
	}
	f := w.control
	f.sweep = &sweepState{location: location, command: command, twins: map[string]string{}}
	f.workers.engines["W"].cleanExit, f.workers.engines["P"].cleanExit = true, true
	// P runs only where it is the writer, or where its own tree is the one swept.
	if command == "run" || location == "selected" {
		if err := f.p.approvalSeed("1", false); err != nil {
			return err
		}
		return f.p.sessionSeedSnapshot()
	}
	return nil
}

// sweepRunAndPlant ends the project's fixture run, with its session stopped, and
// then plants into the tree that run made. The planting is the fixture's, never
// a product's, and begins only after the run has ended.
func (f *controlFixture) sweepRunAndPlant(location string, plant func(*sweepState) error) error {
	s := f.sweep
	if s == nil || s.location != location {
		return fmt.Errorf("sweep fixture is for %v, not %q", s, location)
	}
	name := "W"
	if location == "selected" {
		name = "P"
	}
	s.subject = f.workers.worlds[name]
	s.run = f.workers.start(name, "", "")
	f.workers.observe(s.run, false)
	f.workers.checkpoint(s.run)
	if err := errors.Join(f.err, f.workers.err, f.workers.finishLocalWorkloads()); err != nil {
		return err
	}
	if err := overlapAwait("fixture run exits", 40*time.Second, s.run.ended); err != nil {
		return err
	}
	if got := s.run.result(); got.exit != 0 || s.run.process.err != nil {
		return fmt.Errorf("fixture run did not finish: %+v %v", got, s.run.process.err)
	}
	s.root = filepath.Join(s.subject.doctor.env["XDG_STATE_HOME"], "itos-container")
	if err := plant(s); err != nil {
		return err
	}
	var err error
	s.atPlant, err = sweepInventory(s.root)
	return err
}

func (s *sweepState) admissionDir() string { return filepath.Join(s.root, "admission") }
func (s *sweepState) intentDir() string {
	return filepath.Join(s.root, "intents", s.subject.approval.project)
}
func (s *sweepState) sessionDir() string {
	return filepath.Join(s.root, "sessions", s.subject.recoveryIndexKey(), s.run.id)
}

// claim is the one admission record the project's registration published.
func (s *sweepState) claim() (string, error) {
	entries, err := os.ReadDir(s.admissionDir())
	if err != nil {
		return "", err
	}
	for _, e := range entries {
		if len(e.Name()) == 69 && strings.HasSuffix(e.Name(), ".json") {
			return filepath.Join(s.admissionDir(), e.Name()), nil
		}
	}
	return "", errors.New("the fixture run left no admission claim")
}

func sweepTemporaryName() string {
	var nonce [16]byte
	_, _ = rand.Read(nonce[:])
	return ".intent-" + hex.EncodeToString(nonce[:]) + ".tmp"
}

func sweepWriteFile(path string, mode os.FileMode) error {
	var data [16]byte
	_, _ = rand.Read(data[:])
	if err := os.WriteFile(path, data[:], 0600); err != nil {
		return err
	}
	return os.Chmod(path, mode)
}

func (s *sweepState) lone(dir string) (string, error) {
	path := filepath.Join(dir, sweepTemporaryName())
	s.temporaries = append(s.temporaries, path)
	return path, sweepWriteFile(path, 0600)
}

func (s *sweepState) linked(dir, record string) error {
	path := filepath.Join(dir, sweepTemporaryName())
	s.temporaries = append(s.temporaries, path)
	s.twins[path] = record
	return os.Link(record, path)
}

func (s *sweepState) plantSix() error {
	claim, err := s.claim()
	if err != nil {
		return err
	}
	for dir, record := range map[string]string{s.admissionDir(): claim, s.intentDir(): s.run.intent, s.sessionDir(): s.run.record} {
		if _, err := s.lone(dir); err != nil {
			return err
		}
		if err := s.linked(dir, record); err != nil {
			return err
		}
	}
	return nil
}

func (s *sweepState) plantListed(table *godog.Table) error {
	claim, err := s.claim()
	if err != nil {
		return err
	}
	dirs := map[string]string{
		"admission collection": s.admissionDir(), "intent directory": s.intentDir(), "session directory": s.sessionDir(),
		"approval directory": filepath.Dir(s.subject.approval.record), "snapshot directory": filepath.Dir(s.subject.sessionSnapshotPath()),
		"managed root": s.root, "intents collection": filepath.Join(s.root, "intents"),
	}
	s.outside = filepath.Join(s.subject.dir, "slice-60-outside")
	for _, row := range table.Rows[1:] {
		dir, name, shape := dirs[row.Cells[0].Value], row.Cells[1].Value, row.Cells[2].Value
		if dir == "" {
			return fmt.Errorf("unknown directory %q", row.Cells[0].Value)
		}
		path := filepath.Join(dir, name)
		var err error
		switch shape {
		case "lone 0600 file", "0600 file":
			err = sweepWriteFile(path, 0600)
		case "lone 0644 file":
			err = sweepWriteFile(path, 0644)
		case "lone 0400 file":
			err = sweepWriteFile(path, 0400)
		case "symlink to the admission claim":
			err = os.Symlink(claim, path)
		case "empty 0700 directory":
			if err = os.Mkdir(path, 0700); err == nil {
				err = os.Chmod(path, 0700)
			}
		case "FIFO":
			err = syscall.Mkfifo(path, 0600)
		case "second link of the outside file":
			if _, statErr := os.Lstat(s.outside); statErr != nil {
				if err = sweepWriteFile(s.outside, 0600); err != nil {
					return err
				}
			}
			err = os.Link(s.outside, path)
		case "second link of linked":
			err = os.Link(filepath.Join(dir, "linked"), path)
		default:
			err = fmt.Errorf("unknown shape %q", shape)
		}
		if err != nil {
			return err
		}
		s.listed = append(s.listed, path)
	}
	return nil
}

func (f *controlFixture) sweepGenuine() error {
	s := f.sweep
	for _, dir := range []string{s.admissionDir(), s.intentDir()} {
		path, err := s.lone(dir)
		if err != nil {
			return err
		}
		s.genuine = append(s.genuine, path)
	}
	var err error
	s.atPlant, err = sweepInventory(s.root)
	return err
}

func sweepStat(path string) (sweepEntry, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return sweepEntry{}, err
	}
	stat := reflect.Indirect(reflect.ValueOf(info.Sys()))
	e := sweepEntry{kind: info.Mode().Type().String(), mode: info.Mode().String(), owner: fmt.Sprint(stat.FieldByName("Uid")), dev: fmt.Sprint(stat.FieldByName("Dev")), ino: fmt.Sprint(stat.FieldByName("Ino"))}
	switch {
	case info.IsDir():
	case info.Mode().IsRegular():
		data, err := os.ReadFile(path)
		if err != nil {
			return e, err
		}
		sum := sha256.Sum256(data)
		e.sum, e.raw, e.size = hex.EncodeToString(sum[:]), hex.EncodeToString(data), fmt.Sprint(len(data))
		e.nlink, e.mtime = fmt.Sprint(stat.FieldByName("Nlink")), info.ModTime().String()
	case info.Mode()&os.ModeSymlink != 0:
		target, err := os.Readlink(path)
		if err != nil {
			return e, err
		}
		e.sum = target
		e.nlink, e.mtime = fmt.Sprint(stat.FieldByName("Nlink")), info.ModTime().String()
	default:
		// A FIFO or device is never opened.
		e.nlink, e.mtime = fmt.Sprint(stat.FieldByName("Nlink")), info.ModTime().String()
	}
	return e, nil
}

func sweepInventory(root string) (map[string]sweepEntry, error) {
	result := map[string]sweepEntry{}
	err := filepath.WalkDir(root, func(path string, _ os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		e, err := sweepStat(path)
		result[path] = e
		return err
	})
	return result, err
}

func (f *controlFixture) sweepRead() error {
	s := f.sweep
	f.inspect("doctor", "JSON")
	s.doctor = f.results[len(f.results)-1]
	p := s.subject.recoveryStart("status", "--json")
	p.wait()
	report, err := recoveryReadReport(p)
	s.statusErr = err
	for _, e := range report.Sessions {
		if e.ID == s.run.id {
			s.status = e
		}
	}
	return nil
}

func (s *sweepState) stopped(e recoveryEntry) error {
	if e.ID != s.run.id || e.State != "stopped" || e.Resource != "absent" || e.Cleanup != "complete" || e.Project != s.subject.approval.project {
		return fmt.Errorf("status does not report the stopped session with complete cleanup: %+v", e)
	}
	return nil
}

func (f *controlFixture) sweepReadersKept() error {
	s := f.sweep
	if err := errors.Join(f.err, s.statusErr); err != nil {
		return err
	}
	if s.doctor.exit != 0 {
		return fmt.Errorf("doctor exit = %d\n%s", s.doctor.exit, s.doctor.stderr)
	}
	if err := s.stopped(s.status); err != nil {
		return err
	}
	now, err := sweepInventory(s.root)
	if err != nil {
		return err
	}
	return sweepSame(s.atPlant, now, s.root, "a reader changed")
}

// sweepSame compares two inventories entry by entry, ignoring directories' mtime
// (they are recorded without one) and the subtrees the writer itself rewrites.
func sweepSame(before, now map[string]sweepEntry, root, what string) error {
	var paths []string
	for path := range before {
		paths = append(paths, path)
	}
	for path := range now {
		if _, ok := before[path]; !ok {
			paths = append(paths, path)
		}
	}
	slices.Sort(paths)
	for _, path := range paths {
		name := strings.TrimPrefix(path, root)
		was, existed := before[path]
		got, exists := now[path]
		switch {
		case existed && !exists:
			return fmt.Errorf("%s: %s is gone", what, name)
		case !existed:
			return fmt.Errorf("%s: %s is present and should not be", what, name)
		case got != was:
			return fmt.Errorf("%s: %s was %+v and became %+v", what, name, was, got)
		}
	}
	return nil
}

func (f *controlFixture) sweepWrite(command string) error {
	s := f.sweep
	if s == nil || s.command != command {
		return fmt.Errorf("sweep fixture is not for writer %q", command)
	}
	var err error
	s.atWriter, err = sweepInventory(s.root)
	if err != nil {
		return err
	}
	if command == "approve" {
		if s.location == "selected" {
			// P's own tree: a changed containerEnv makes approve publish the next generation.
			f.p.doctor.manifest["containerEnv"] = map[string]string{"APP": "new"}
			if err := errors.Join(f.p.approvalManifest(), f.p.approvalOracle()); err != nil {
				return err
			}
		}
		f.err = errors.Join(f.err, f.p.runDoctor(f.p.env(), "approve", "--fingerprint", f.p.approval.fingerprint, "--json"))
		return nil
	}
	s.writer = f.workers.start("P", "", "")
	f.workers.observe(s.writer, false)
	f.err = errors.Join(f.err, f.workers.finishLocalWorkloads())
	f.err = errors.Join(f.err, overlapAwait("P run exits", 40*time.Second, s.writer.ended))
	return nil
}

func (f *controlFixture) sweepSucceeded() error {
	s := f.sweep
	if err := errors.Join(f.err, f.workers.err); err != nil {
		return err
	}
	if s.command == "approve" {
		return f.p.approvalSuccess()
	}
	if got := s.writer.result(); got.exit != 0 || s.writer.process.err != nil {
		return fmt.Errorf("P run did not succeed: %+v %v", got, s.writer.process.err)
	}
	return nil
}

// sweepGone checks the swept tree against what it held before the writer: the
// named temporaries are gone, a record that was a temporary's second link has one
// link, and nothing else differs. A selected tree is also P's own, so the writer's
// approval and snapshot generation are not part of the comparison.
func (f *controlFixture) sweepGone(removed []string) error {
	s := f.sweep
	if err := f.sweepSucceeded(); err != nil {
		return err
	}
	if removed == nil {
		removed = s.temporaries
	}
	now, err := sweepInventory(s.root)
	if err != nil {
		return err
	}
	expected := map[string]sweepEntry{}
	for path, e := range s.atWriter {
		expected[path] = e
	}
	for _, path := range removed {
		delete(expected, path)
		if record, twin := s.twins[path]; twin {
			e := expected[record]
			e.nlink = "1"
			expected[record] = e
		}
	}
	skip := func(path string) bool {
		if s.location != "selected" {
			return false
		}
		for _, name := range []string{"approvals", "snapshots"} {
			if strings.HasPrefix(path, filepath.Join(s.root, name)) {
				return true
			}
		}
		return false
	}
	for _, m := range []map[string]sweepEntry{expected, now} {
		for path := range m {
			if skip(path) {
				delete(m, path)
			}
		}
	}
	return sweepSame(expected, now, s.root, "the writer's sweep of")
}

func (f *controlFixture) sweepTwinsKept() error {
	s := f.sweep
	now, err := sweepInventory(s.root)
	if err != nil {
		return err
	}
	for _, record := range s.twins {
		want, got := s.atPlant[record], now[record]
		want.nlink = "1"
		if got != want {
			return fmt.Errorf("%s: %+v became %+v, want one link and nothing else changed", record, s.atPlant[record], got)
		}
	}
	return nil
}

func (f *controlFixture) sweepStatusUnchanged() error {
	s := f.sweep
	p := s.subject.recoveryStart("status", "--json")
	p.wait()
	report, err := recoveryReadReport(p)
	if err != nil {
		return err
	}
	for _, e := range report.Sessions {
		if e.ID == s.run.id {
			if err := s.stopped(e); err != nil {
				return err
			}
			if !reflect.DeepEqual(e, s.status) {
				return fmt.Errorf("status changed from %+v to %+v", s.status, e)
			}
			return nil
		}
	}
	return errors.New("status lost the stopped session")
}

func (f *controlFixture) sweepListedKept() error {
	s := f.sweep
	if err := f.sweepSucceeded(); err != nil {
		return err
	}
	for _, path := range s.listed {
		want, ok := s.atWriter[path]
		got, err := sweepStat(path)
		if !ok || err != nil || got != want {
			return fmt.Errorf("%s: %+v became %+v (%v)", path, want, got, err)
		}
	}
	return nil
}
