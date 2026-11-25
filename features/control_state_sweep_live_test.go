package features

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// ID-CONTROL-29 exercises the sweep while a publisher is paused.
func (w *world) sweepPauseSetup(record string) error {
	boundary := map[string]string{"session intent": "intent replacement", "public session record": "public record replacement"}[record]
	if boundary == "" {
		return fmt.Errorf("unknown record %q", record)
	}
	if err := w.controlSafe("state path shares only a textual prefix with W"); err != nil {
		return err
	}
	f := w.control
	f.sweep = &sweepState{location: "enrolled", command: "approve", boundary: boundary, twins: map[string]string{}}
	f.workers.engines["W"].cleanExit, f.workers.engines["P"].cleanExit = true, true
	f.sweep.subject = f.workers.worlds["W"]
	f.sweep.root = filepath.Join(f.sweep.subject.doctor.env["XDG_STATE_HOME"], "itos-container")
	return nil
}

func (f *controlFixture) sweepPause() error {
	s := f.sweep
	s.run = f.workers.start("W", s.boundary, "")
	f.workers.first = s.run
	if s.run.probe == nil {
		return errors.Join(f.workers.err, errors.New("W did not start"))
	}
	ready := s.run.probe.ready
	if err := overlapAwait("W pauses before the replacement", 40*time.Second, func() bool {
		data, _ := os.ReadFile(ready)
		return len(data) != 0 || s.run.ended()
	}); err != nil {
		return err
	}
	data, _ := os.ReadFile(ready)
	s.pausedName = string(data)
	if !strings.HasPrefix(s.pausedName, ".intent-") {
		return fmt.Errorf("W ended without pausing: %+v", s.run.result())
	}
	var dir string
	_ = filepath.WalkDir(s.root, func(path string, d os.DirEntry, err error) error {
		if err == nil && d.Name() == s.pausedName {
			dir = filepath.Dir(path)
		}
		return nil
	})
	if dir == "" {
		return errors.New("W's paused temporary is not in its state tree")
	}
	s.pausedDir, s.paused = dir, filepath.Join(dir, s.pausedName)
	s.record = filepath.Join(dir, "record.json")
	if s.boundary == "intent replacement" {
		entries, _ := os.ReadDir(dir)
		for _, e := range entries {
			if len(e.Name()) == 69 && strings.HasSuffix(e.Name(), ".json") {
				s.record = filepath.Join(dir, e.Name())
			}
		}
	}
	var err error
	if s.admission, err = s.lone(s.admissionDir()); err != nil {
		return err
	}
	if s.beside, err = s.lone(dir); err != nil {
		return err
	}
	s.capturedPaused = map[string]sweepEntry{}
	for _, path := range []string{s.paused, s.record, s.beside} {
		if s.capturedPaused[path], err = sweepStat(path); err != nil {
			return err
		}
	}
	return nil
}

func (f *controlFixture) sweepPausedWriter() error {
	s := f.sweep
	if err := errors.Join(f.err, f.workers.err); err != nil {
		return err
	}
	if err := f.p.approvalSuccess(); err != nil {
		return err
	}
	if _, err := os.Lstat(s.admission); !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("the temporary planted in W's admission collection remains: %v", err)
	}
	return nil
}

func (f *controlFixture) sweepPausedKept() error {
	s := f.sweep
	for path, want := range s.capturedPaused {
		got, err := sweepStat(path)
		if err != nil || got != want {
			return fmt.Errorf("%s: %+v became %+v (%v)", path, want, got, err)
		}
	}
	return nil
}

func (f *controlFixture) sweepRelease() error {
	s := f.sweep
	if err := os.WriteFile(s.run.probe.release, []byte("continue"), 0600); err != nil {
		return err
	}
	s.releasedAt = time.Now()
	if err := overlapAwait("W's replacement returns", 15*time.Second, func() bool {
		_, err := os.Lstat(s.run.probe.evidence)
		return err == nil || s.run.ended()
	}); err != nil {
		return err
	}
	f.workers.observe(s.run, false)
	f.err = errors.Join(f.err, f.workers.finishLocalWorkloads())
	return overlapAwait("W's run exits", 40*time.Second, s.run.ended)
}

func (f *controlFixture) sweepPausedEvidence(record string) error {
	s := f.sweep
	data, err := os.ReadFile(s.run.probe.evidence)
	if err != nil {
		return fmt.Errorf("W's replacement left no evidence for %q: %w (W: %+v)", record, err, s.run.result())
	}
	var evidence struct {
		Source, Target       string
		TimedOut             bool
		Error                string
		Device, Inode, Links string
		Bytes                string
	}
	if err := json.Unmarshal(data, &evidence); err != nil {
		return err
	}
	if evidence.TimedOut {
		return fmt.Errorf("W's replacement for %q timed out before the fixture released it", record)
	}
	if evidence.Error != "" {
		return fmt.Errorf("W's replacement for %q failed: %s", record, evidence.Error)
	}
	paused := s.capturedPaused[s.paused]
	switch {
	case evidence.Source != s.pausedName:
		return fmt.Errorf("W renamed %q for %q, not the paused temporary %q", evidence.Source, record, s.pausedName)
	case evidence.Target != filepath.Base(s.record):
		return fmt.Errorf("W published %q for %q, not %q", evidence.Target, record, filepath.Base(s.record))
	case evidence.Device != paused.dev || evidence.Inode != paused.ino:
		return fmt.Errorf("W's evidence for %q names %s:%s, not the paused temporary's %s:%s", record, evidence.Device, evidence.Inode, paused.dev, paused.ino)
	case evidence.Links != "1":
		return fmt.Errorf("W's evidence for %q reports %s links, want one", record, evidence.Links)
	case evidence.Bytes != paused.raw:
		return fmt.Errorf("W's evidence for %q carries other bytes than the paused temporary", record)
	}
	return nil
}

// sweepNoOwnTemporaries reads the two directories W publishes into after its run
// has exited. Only the fixture's temporary beside the paused one may remain.
func (f *controlFixture) sweepNoOwnTemporaries() error {
	s := f.sweep
	var left []string
	for _, dir := range []string{s.intentDir(), s.sessionDir()} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			name := entry.Name()
			path := filepath.Join(dir, name)
			if strings.HasPrefix(name, ".intent-") && strings.HasSuffix(name, ".tmp") && path != s.beside {
				left = append(left, path)
			}
		}
	}
	if len(left) != 0 {
		slices.Sort(left)
		return fmt.Errorf("W left a temporary of its own: %v", left)
	}
	return nil
}

func (f *controlFixture) sweepPausedRunEnded() error {
	s := f.sweep
	got := s.run.result()
	if err := errors.Join(f.err, f.workers.err, s.run.process.err); err != nil {
		return err
	}
	if got.exit != 0 {
		return fmt.Errorf("W's run exited %d\n%s", got.exit, got.stderr)
	}
	s.run.discover()
	p := s.subject.recoveryStart("status", "--json")
	p.wait()
	report, err := recoveryReadReport(p)
	if err != nil {
		return err
	}
	for _, e := range report.Sessions {
		if e.ID == s.run.id {
			return s.stopped(e)
		}
	}
	return errors.New("status lost W's session")
}
