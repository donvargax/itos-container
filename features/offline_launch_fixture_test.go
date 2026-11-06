package features

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type offlineTrace struct {
	Operation, ID, Failure string
	Record, Binding        map[string]any
	Argv                   []string
	Observation            sessionMemoryObservation
	Status, Exit           int
	Writes                 []sessionEvent
}

type offlineLaunchWorld struct {
	barrier, replaced, cause string
	child                    int
	faultActive, reached     bool
	watch                    bool
	outside, moved           string
	outsideBefore            map[string]string
	manifestB, approvalB     []byte
	settingsB                map[string]any
	first                    *world
	snapshotA                sessionRecord
}

func (w *world) offlineLaunchFixture() error {
	if err := w.offlineLocalFixture("", false); err != nil {
		return err
	}
	return w.sessionEach(func(w *world) error {
		for _, current := range []*world{w, w.session.offlineControl} {
			current.session.launch = &offlineLaunchWorld{}
			for _, name := range []string{"a", "b"} {
				if err := os.Mkdir(filepath.Join(current.doctor.checkout, name), 0700); err != nil {
					return err
				}
			}
			current.doctor.manifest["workspaceFolder"] = "/workspace/a"
			current.doctor.manifest["containerEnv"] = map[string]any{"APP": "A literal;$HOME slice-13-private-env", "DEFAULT": "overlaid"}
			if err := current.approvalManifest(); err != nil {
				return err
			}
			current.doctor.engine.config()["Env"] = []string{"DEFAULT=image", "IMAGE_ONLY=retained", "HOME=/image-home", "TMPDIR=/image-tmp"}
			current.doctor.engine.config()["Entrypoint"] = []string{"image-entrypoint-must-not-run"}
			current.doctor.engine.config()["Cmd"] = []string{"image-default-must-not-run"}
			if err := current.approvalSeed("1", false); err != nil {
				return err
			}
			foreign := strings.Repeat("e", 64)
			current.session.api.local[foreign] = map[string]any{
				"Id": foreign, "Name": "/unrelated-worker", "Image": current.doctor.imageID,
				"Config": map[string]any{"Labels": map[string]any{"io.itos-container.intent": strings.Repeat("d", 64)}},
				"State":  map[string]any{"Status": "created", "Running": false, "Pid": float64(0), "StartedAt": "0001-01-01T00:00:00Z"},
			}
		}
		return nil
	})
}

func (w *world) offlineFrozenFixture() error {
	if err := w.offlineLaunchFixture(); err != nil {
		return err
	}
	return w.sessionEach(func(w *world) error {
		l, a := w.session.launch, w.approval
		l.watch = true
		manifest := approvalClone(w.doctor.manifest)
		manifest["workspaceFolder"] = "/workspace/b"
		manifest["containerEnv"] = map[string]any{"APP": "B literal;$(never) slice-13-private-env", "DEFAULT": "B overlay"}
		l.manifestB = approvalJSON(manifest)
		l.settingsB = approvalClone(a.settings)
		object(l.settingsB["manifest"])["workspaceFolder"] = manifest["workspaceFolder"]
		object(l.settingsB["manifest"])["containerEnv"] = manifest["containerEnv"]
		l.approvalB = approvalJSON(map[string]any{"schema": 1, "project": a.project, "generation": "2", "fingerprint": "sha256:" + approvalHash(l.settingsB), "settings": l.settingsB})
		return nil
	})
}

func (w *world) offlineSubstitutionFixture(boundary string) error {
	if err := w.offlineLaunchFixture(); err != nil {
		return err
	}
	return w.sessionEach(func(w *world) error {
		l := w.session.launch
		l.barrier = boundary
		l.outside = filepath.Join(w.dir, "slice-13-outside")
		if err := os.Mkdir(l.outside, 0700); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(l.outside, "sentinel"), []byte("outside must remain untouched"), 0640); err != nil {
			return err
		}
		var err error
		l.outsideBefore, err = doctorTree(l.outside)
		return err
	})
}

func (w *world) offlineCleanupFixture(child int, cause string) error {
	if err := w.offlineLaunchFixture(); err != nil {
		return err
	}
	return w.sessionEach(func(w *world) error {
		w.session.launch.child, w.session.launch.cause = child, cause
		w.session.offlineControl.session.launch.child = child
		return nil
	})
}

// Race every armed barrier with subprocess exit and its finite deadline. Early
// returns remain results for Then; they do not hang or become Given failures.
func (w *world) offlineAwaitBarrier(ctx context.Context, cancel context.CancelFunc, done chan error, path string, stdin io.Writer) error {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	released := false
	for {
		select {
		case err := <-done:
			return err
		case <-ctx.Done():
			return <-done
		case <-ticker.C:
			if released {
				continue
			}
			data, err := os.ReadFile(path)
			var stage string
			if err != nil || json.Unmarshal(data, &stage) != nil {
				continue
			}
			if err := w.offlineChangeAtBarrier(stage); err != nil {
				cancel()
				<-done
				return err
			}
			if err := json.NewEncoder(stdin).Encode("continue"); err != nil {
				cancel()
				<-done
				return err
			}
			released = true
		}
	}
}

func (w *world) offlineChangeAtBarrier(stage string) error {
	s, l := w.session, w.session.launch
	l.reached = true
	if stage == "after exit" {
		s.api.mu.Lock()
		defer s.api.mu.Unlock()
		l.faultActive = true
		if s.backend == "disk" {
			switch l.cause {
			case "configuration":
				return os.WriteFile(w.sessionRecordPath(), []byte(`{"invalid":`), 0600)
			case "environment":
				return os.Remove(strings.TrimPrefix(w.doctor.env["DOCKER_HOST"], "unix://"))
			}
		}
		return nil
	}
	if stage != l.barrier {
		return fmt.Errorf("unexpected launch barrier %q", stage)
	}
	if s.backend == "disk" {
		path := w.doctor.checkout
		switch l.replaced {
		case "embedded .git":
			path = filepath.Join(path, ".git")
		case "workspace subdirectory":
			path = filepath.Join(path, "a")
		}
		l.moved = filepath.Join(w.dir, "slice-13-original")
		if err := os.Rename(path, l.moved); err != nil {
			return err
		}
		if err := os.Symlink(l.outside, path); err != nil {
			return err
		}
		w.doctor.cleanup = append(w.doctor.cleanup, func() {
			_ = os.Remove(path)
			_ = os.Rename(l.moved, path)
		})
	}
	// Account for fixture writes before observing subsequent product effects.
	return w.sessionCaptureBefore()
}

func (w *world) offlineLaunchAction() error {
	return w.sessionEach(func(w *world) error {
		s := w.session
		s.fixtureError = w.offlineLocalAction(false)
		control := s.offlineControl
		control.session.fixtureError = control.offlineLocalAction(false)
		return nil
	})
}

func (w *world) offlineFrozenAction() error {
	return w.sessionEach(func(w *world) error {
		w.session.fixtureError = w.offlineFrozenPair()
		return nil
	})
}

func (w *world) offlineFrozenPair() error {
	s, l := w.session, w.session.launch
	var err error
	s.create, err = w.sessionProbe("run", "", func() error {
		l.snapshotA, err = sessionReadRecord(w.sessionSnapshotPath())
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(w.doctor.checkout, ".devcontainer/devcontainer.json"), l.manifestB, 0600); err != nil {
			return err
		}
		if err := os.WriteFile(w.approval.record, l.approvalB, 0600); err != nil {
			return err
		}
		return w.sessionCaptureBefore()
	})
	if err != nil {
		return err
	}
	s.cleanup = sessionResult{Outcome: s.create.Cleanup, Reference: s.create.Reference}
	if err := w.sessionCaptureAfter(); err != nil {
		return err
	}
	api := s.api
	s.api = &sessionAPI{requests: api.requests, created: api.created, observed: api.observed, local: api.local, absent: api.absent}
	l.first = &world{root: w.root, bin: w.bin, dir: w.dir, doctor: w.doctor, approval: w.approval, session: s}
	// A fresh token and generation are independently supplied fixture data.
	w.session = &sessionWorld{api: api, backend: s.backend, umask: s.umask, offline: true, journal: filepath.Join(w.dir, "slice-13-second-io.jsonl"), offlineControl: s.offlineControl, launch: l, token: approvalHash(map[string]any{"next": s.token})}
	w.session.reference = "intents/" + w.approval.project + "/" + w.session.token + ".json"
	api.requests, api.created = nil, nil
	api.observed, api.absent = map[string]map[string]any{}, map[string]bool{}
	// Keep the original snapshot and intent as protected, unrelated state.
	a := *w.approval
	a.settings, a.generation, a.fingerprint = l.settingsB, "2", "sha256:"+approvalHash(l.settingsB)
	w.approval = &a
	if err := os.WriteFile(w.session.journal, nil, 0600); err != nil {
		return err
	}
	if err := w.offlineLocalAction(false); err != nil {
		return err
	}
	return nil
}
