package disk

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/donvargax/itos-container/internal/engine/approval/port"
)

func TestAdmissionPublicationExcludesOtherProjectsUntilDurable(t *testing.T) {
	s, c, _ := diskFixture(t)
	entered, release := make(chan struct{}), make(chan struct{})
	r := Registry{Store: s, IO: SnapshotIO{FileSync: func(file *os.File) error { close(entered); <-release; return file.Sync() }}}
	d, f := r.Admission(t.Context(), c)
	if f != "" {
		t.Fatal(f)
	}
	done := make(chan port.Failure, 1)
	go func() { done <- d.Publish(strings.Repeat("a", 64)+".json", nil, []byte("reservation")); d.Close() }()
	<-entered
	other := c
	other.Project = strings.Repeat("b", 64)
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	contender, f := (Registry{Store: s}).Admission(ctx, other)
	cancel()
	if contender != nil {
		contender.Close()
	}
	close(release)
	if failure := <-done; failure != "" {
		t.Fatal(failure)
	}
	if f != "state.unavailable" || contender != nil {
		t.Fatal("publication barrier did not exclude another project", f)
	}
	d, f = (Registry{Store: s}).Admission(t.Context(), other)
	if f != "" {
		t.Fatal(f)
	}
	defer d.Close()
	if names, f := d.Names(); f != "" || len(names) != 1 {
		t.Fatal("durable reservation lost after lock release", names, f)
	}
}
func TestAdmissionRefusesUnsafeState(t *testing.T) {
	for _, fault := range []string{"relative state", "invalid project", "lock", "unknown name", "symlink record", "hardlink record", "record mode", "unsafe index", "unknown index", "unknown session", "unsafe session", "record missing", "record directory", "temporary", "read directory", "changed directory"} {
		t.Run(fault, func(t *testing.T) {
			s, c, _ := diskFixture(t)
			r := Registry{Store: s}
			if fault == "relative state" {
				r.Store.StateHome = "relative"
			}
			if fault == "invalid project" {
				c.Project = "invalid"
			}
			if fault == "lock" {
				ctx, cancel := context.WithCancel(t.Context())
				cancel()
				if d, f := r.Admission(ctx, c); d != nil || f == "" {
					t.Fatal("canceled lock acquired", f)
				}
				return
			}
			d, f := r.Admission(t.Context(), c)
			if fault == "relative state" || fault == "invalid project" {
				if d != nil || f == "" {
					t.Fatal("invalid admission location", f)
				}
				return
			}
			if f != "" {
				t.Fatal(f)
			}
			defer d.Close()
			real := d.(*admissionDirectory)
			name := strings.Repeat("a", 64) + ".json"
			if strings.Contains(fault, "record") && fault != "record missing" && fault != "record directory" {
				if f := d.Publish(name, nil, []byte("reservation")); f != "" {
					t.Fatal(f)
				}
			}
			path := filepath.Join(real.pin.path, name)
			switch fault {
			case "unknown name":
				if err := os.WriteFile(filepath.Join(real.pin.path, "unexpected"), nil, 0600); err != nil {
					t.Fatal(err)
				}
			case "temporary":
				if err := os.WriteFile(filepath.Join(real.pin.path, ".intent-dead.tmp"), nil, 0600); err != nil {
					t.Fatal(err)
				}
			case "symlink record":
				if err := os.Rename(path, path+"-old"); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(path+"-old", path); err != nil {
					t.Fatal(err)
				}
			case "hardlink record":
				if err := os.Link(path, path+"-link"); err != nil {
					t.Fatal(err)
				}
			case "record mode":
				if err := os.Chmod(path, 0644); err != nil {
					t.Fatal(err)
				}
			case "read directory":
				if err := real.pin.dir.Close(); err != nil {
					t.Fatal(err)
				}
			case "changed directory":
				if err := os.Chmod(real.pin.path, 0755); err != nil {
					t.Fatal(err)
				}
			default:
				index := c.Project
				id := strings.Repeat("b", 64)
				if fault == "unknown index" {
					index = "invalid"
				}
				if fault == "unknown session" {
					id = "invalid"
				}
				p := filepath.Join(real.sessions, index, id)
				if err := os.MkdirAll(p, 0700); err != nil {
					t.Fatal(err)
				}
				if fault == "unsafe index" {
					if err := os.Chmod(filepath.Dir(p), 0755); err != nil {
						t.Fatal(err)
					}
				}
				if fault == "unsafe session" {
					if err := os.Chmod(p, 0755); err != nil {
						t.Fatal(err)
					}
				}
				if fault == "record directory" {
					if err := os.Mkdir(filepath.Join(p, "record.json"), 0700); err != nil {
						t.Fatal(err)
					}
				}
			}
			switch fault {
			case "unknown name", "read directory", "changed directory", "temporary":
				names, f := d.Names()
				if fault == "temporary" {
					if f != "" || len(names) != 0 {
						t.Fatal(names, f)
					}
				} else if f == "" {
					t.Fatal("unsafe admission names accepted", names)
				}
			case "symlink record", "hardlink record", "record mode":
				if _, f := d.Read(name); f == "" {
					t.Fatal("unsafe admission record accepted")
				}
			default:
				records, f := d.Sessions()
				if fault == "record missing" {
					if f != "" || len(records) != 1 || records[0].Data != nil {
						t.Fatal(records, f)
					}
				} else if f == "" {
					t.Fatal("unsafe registration scan accepted", records)
				}
			}
		})
	}
}
func TestAdmissionScanRefusesDisappearingPathsAndReadFailures(t *testing.T) {
	s, c, _ := diskFixture(t)
	d, f := (Registry{Store: s}).Admission(t.Context(), c)
	if f != "" {
		t.Fatal(f)
	}
	defer d.Close()
	a := d.(*admissionDirectory)
	for _, path := range []string{filepath.Join(a.sessions, c.Project, strings.Repeat("a", 64))} {
		if _, f := a.sessionRecord(a.store, path); f != "state.path" {
			t.Fatal("missing path accepted", f)
		}
	}
	if _, f := a.projectSessions(a.store, c.Project); f != "state.path" {
		t.Fatal("missing project accepted", f)
	}
	ops := *a.store.ops
	ops.openRoot = func(string) (*os.Root, error) { return nil, errors.New("unavailable") }
	a.store.ops = &ops
	if _, f := a.Sessions(); f != "state.unavailable" {
		t.Fatal(f)
	}
	if _, f := a.projectSessions(a.store, c.Project); f != "state.unavailable" {
		t.Fatal(f)
	}
	if _, f := a.sessionRecord(a.store, c.Project); f != "state.unavailable" {
		t.Fatal(f)
	}
}

func TestAdmissionScanDetectsUnreadableAndChangedDirectories(t *testing.T) {
	for _, fault := range []string{"collection read", "project read", "expired admission", "changed project"} {
		t.Run(fault, func(t *testing.T) {
			s, c, _ := diskFixture(t)
			d, f := (Registry{Store: s}).Admission(t.Context(), c)
			if f != "" {
				t.Fatal(f)
			}
			defer d.Close()
			a := d.(*admissionDirectory)
			project := filepath.Join(a.sessions, c.Project)
			if err := os.MkdirAll(project, 0700); err != nil {
				t.Fatal(err)
			}
			ops := *a.store.ops
			open := ops.openDir
			ops.openDir = func(root *os.Root, name string) (*os.File, error) {
				file, err := open(root, name)
				if err == nil && ((fault == "collection read" && root.Name() == a.sessions) || (fault == "project read" && root.Name() == project)) {
					_ = file.Close()
				}
				return file, err
			}
			if fault == "expired admission" {
				ctx, cancel := context.WithCancel(t.Context())
				cancel()
				a.ctx = ctx
			}
			if fault == "changed project" {
				stat := ops.statRoot
				observations := 0
				ops.statRoot = func(root *os.Root, name string) (os.FileInfo, error) {
					info, err := stat(root, name)
					if root.Name() == project {
						observations++
						if observations == 3 {
							if err := os.Chmod(project, 0755); err != nil {
								t.Fatal(err)
							}
						}
					}
					return info, err
				}
			}
			a.store.ops = &ops
			_, f = a.Sessions()
			want := port.Failure("state.path")
			if strings.HasSuffix(fault, "read") {
				want = "state.unavailable"
			}
			if f != want {
				t.Fatal("scan retained changed or unreadable authority", f, want)
			}
		})
	}
}
