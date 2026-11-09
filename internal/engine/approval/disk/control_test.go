package disk

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/donvargax/itos-container/internal/engine/approval/port"
	"github.com/donvargax/itos-container/internal/engine/approval/port/porttest"
)

func controlFixture(t *testing.T) (Control, port.Candidate, *porttest.Reservations, string, string) {
	t.Helper()
	store, c, _ := diskFixture(t)
	base := store.StateHome
	home := filepath.Join(base, "account")
	if err := os.Mkdir(home, 0700); err != nil {
		t.Fatal(err)
	}
	store.StateHome = filepath.Join(base, "selected")
	r := &porttest.Reservations{RootsByLocation: map[string][]string{}}
	return Control{Account: &porttest.Account{Home: home}, Store: store, Reservations: r}, c, r, home, filepath.Join(store.StateHome, "itos-container")
}
func controlSnapshot(t *testing.T, root string) string {
	t.Helper()
	var snapshot strings.Builder
	err := filepath.WalkDir(root, func(path string, e os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := e.Info()
		if err != nil {
			return err
		}
		fmt.Fprintln(&snapshot, path, info.Mode())
		if info.Mode().IsRegular() {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			fmt.Fprintln(&snapshot, string(data), info.ModTime(), reflect.Indirect(reflect.ValueOf(info.Sys())).FieldByName("Ino"))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return snapshot.String()
}
func TestProtectedControlContract(t *testing.T) {
	porttest.ControlContract(t, func() porttest.ControlFixture {
		first, c, r, home, selected := controlFixture(t)
		second := first
		second.Store.StateHome = filepath.Join(filepath.Dir(home), "other")
		other := filepath.Join(second.Store.StateHome, "itos-container")
		fault := false
		ops := first.Store.operations()
		write := ops.write
		ops.write = func(file *os.File, data []byte) (int, error) {
			if fault {
				return 0, errors.New("publication interrupted")
			}
			return write(file, data)
		}
		first.Store.ops = &ops
		return porttest.ControlFixture{First: first, Second: second, Candidate: c, Location: selected,
			Expose: func(on bool) {
				if on {
					tx, f := second.Begin(t.Context(), c, true)
					if f != "" {
						t.Fatal(f)
					}
					tx.Close()
					r.RootsByLocation[other] = []string{filepath.Dir(selected)}
				} else {
					delete(r.RootsByLocation, other)
				}
			},
			WriteFault: func(on bool) { fault = on }, Snapshot: func() string { return controlSnapshot(t, filepath.Dir(home)) },
			Durable: func() bool {
				data, err := os.ReadFile(filepath.Join(home, ".local/state/itos-container/coordination", locationName(selected)))
				var b locationBinding
				return err == nil && json.Unmarshal(data, &b) == nil && b.Complete
			},
			Replace: func() {
				if err := os.Rename(selected, selected+"-old"); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(selected, 0700); err != nil {
					t.Fatal(err)
				}
			},
		}
	})
}
func TestControlAccountAndPrerequisites(t *testing.T) {
	for _, kind := range []string{"lookup", "relative account", "root account", "missing home", "unsafe home", "home stat failure", "missing account port", "missing reservations port", "relative selection", "own selection", "own anchor", "reservation read"} {
		t.Run(kind, func(t *testing.T) {
			control, c, r, home, _ := controlFixture(t)
			want := port.Failure("state.unavailable")
			switch kind {
			case "lookup":
				control.Account = &porttest.Account{Failure: "state.unavailable"}
			case "relative account":
				control.Account = &porttest.Account{Home: "relative"}
			case "root account":
				control.Account = &porttest.Account{Home: "/"}
			case "missing home":
				control.Account = &porttest.Account{Home: home + "-absent"}
			case "unsafe home":
				if err := os.Chmod(home, 0770); err != nil {
					t.Fatal(err)
				}
				want = "state.unsafe"
			case "home stat failure":
				ops := control.Store.operations()
				ops.statFile = func(*os.File) (os.FileInfo, error) { return nil, os.ErrPermission }
				control.Store.ops = &ops
				want = "state.unsafe"
			case "missing account port":
				control.Account = nil
			case "missing reservations port":
				control.Reservations = nil
			case "relative selection":
				control.Store.StateHome = "relative"
				want = "state.location"
			case "own selection":
				c.Root = control.Store.StateHome
				want = "state.path"
			case "own anchor":
				c.Root = home
				want = "state.path"
			case "reservation read":
				r.Failure = "state.unavailable"
			}
			tx, f := control.Begin(t.Context(), c, false)
			if tx != nil {
				tx.Close()
			}
			if f != want {
				t.Fatal(f, want)
			}
		})
	}
}
func TestControlBootstrapWriteFaultsRetainSafeRetry(t *testing.T) {
	for _, kind := range []string{"mkdir", "directory sync", "nonce", "create", "chmod", "write", "short write", "file sync", "close", "rename", "parent sync"} {
		t.Run(kind, func(t *testing.T) {
			control, c, _, _, _ := controlFixture(t)
			ops := control.Store.operations()
			bad := errors.New("interrupted bootstrap")
			switch kind {
			case "mkdir":
				ops.mkdir = func(*os.Root, string, os.FileMode) error { return bad }
			case "directory sync":
				ops.syncDir = func(*os.File) error { return bad }
			case "nonce":
				ops.nonce = func([]byte) (int, error) { return 0, bad }
			case "create":
				ops.create = func(*os.Root, string) (*os.File, error) { return nil, bad }
			case "chmod":
				ops.chmod = func(*os.File, os.FileMode) error { return bad }
			case "write":
				ops.write = func(*os.File, []byte) (int, error) { return 0, bad }
			case "short write":
				ops.write = func(*os.File, []byte) (int, error) { return 0, nil }
			case "file sync":
				ops.syncFile = func(*os.File) error { return bad }
			case "close":
				ops.closeFile = func(*os.File) error { return bad }
			case "rename":
				ops.rename = func(*os.Root, string, string) error { return bad }
			case "parent sync":
				ops.syncParent = func(*os.File) error { return bad }
			}
			control.Store.ops = &ops
			tx, f := control.Begin(t.Context(), c, true)
			if tx != nil {
				tx.Close()
			}
			if f != "state.write" {
				t.Fatal(f)
			}
			control.Store.ops = nil
			tx, f = control.Begin(t.Context(), c, false)
			if f != "" {
				t.Fatal("inspection after interrupted bootstrap", f)
			}
			tx.Close()
			tx, f = control.Begin(t.Context(), c, true)
			if f != "" {
				t.Fatal("retry", f)
			}
			tx.Close()
		})
	}
}
func TestControlBindingsRefuseMalformedOrReplacedEvidence(t *testing.T) {
	for _, kind := range []string{"json", "unknown field", "duplicate uid", "duplicate ancestor", "missing complete", "trailing data", "schema", "uid", "relative path", "noncanonical path", "mismatched name", "empty ancestors", "wrong ancestor path", "bad device", "bad inode", "changed device", "changed inode", "incomplete self", "short complete", "missing root", "symlink", "hardlink", "mode", "unknown name", "missing self", "oversized", "read failure"} {
		t.Run(kind, func(t *testing.T) {
			control, c, _, home, selected := controlFixture(t)
			tx, f := control.Begin(t.Context(), c, true)
			if f != "" {
				t.Fatal(f)
			}
			tx.Close()
			catalog := filepath.Join(home, ".local/state/itos-container/coordination")
			path := filepath.Join(catalog, locationName(selected))
			if kind == "incomplete self" {
				path = filepath.Join(catalog, "self.json")
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var b locationBinding
			if err = json.Unmarshal(data, &b); err != nil {
				t.Fatal(err)
			}
			want := port.Failure("state.invalid")
			switch kind {
			case "json":
				data = []byte("{")
			case "unknown field":
				data = append(data[:len(data)-1], []byte(`,"extra":true}`)...)
			case "duplicate uid":
				data = append(data[:len(data)-1], []byte(`,"uid":"`+c.UID+`"}`)...)
			case "duplicate ancestor":
				data = []byte(strings.Replace(string(data), `"device":`, `"device":"0","device":`, 1))
			case "missing complete":
				data = []byte(strings.Replace(string(data), `"complete":true,`, "", 1))
			case "trailing data":
				data = append(data, []byte(" {}")...)
			case "schema":
				b.Schema = 2
			case "uid":
				b.UID = "other"
			case "relative path":
				b.Path = "relative"
			case "noncanonical path":
				b.Path += "/../itos-container"
			case "mismatched name":
				b.Path += "-other"
			case "empty ancestors":
				b.Ancestors = nil
			case "wrong ancestor path":
				b.Ancestors[0].Path = "/another"
			case "bad device":
				b.Ancestors[0].Device = "invalid"
			case "bad inode":
				b.Ancestors[0].Inode = "invalid"
			case "changed device":
				b.Ancestors[0].Device = "999999999"
				want = "state.path"
			case "changed inode":
				b.Ancestors[0].Inode = "999999999"
				want = "state.path"
			case "incomplete self":
				b.Complete = false
			case "short complete":
				b.Ancestors = b.Ancestors[:len(b.Ancestors)-1]
			case "missing root":
				if err := os.Rename(selected, selected+"-old"); err != nil {
					t.Fatal(err)
				}
				want = "state.unavailable"
			case "symlink":
				// Keep the target outside the catalog so an unknown entry cannot
				// mask the symlink refusal in filesystem-dependent listing order.
				target := filepath.Join(home, "binding-target.json")
				if err := os.Rename(path, target); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, path); err != nil {
					t.Fatal(err)
				}
				want = "state.path"
			case "hardlink":
				if err := os.Link(path, filepath.Join(home, "other")); err != nil {
					t.Fatal(err)
				}
				want = "state.unsafe"
			case "mode":
				if err := os.Chmod(path, 0644); err != nil {
					t.Fatal(err)
				}
				want = "state.unsafe"
			case "unknown name":
				if err := os.WriteFile(filepath.Join(catalog, "unknown"), nil, 0600); err != nil {
					t.Fatal(err)
				}
			case "missing self":
				if err := os.Remove(filepath.Join(catalog, "self.json")); err != nil {
					t.Fatal(err)
				}
			case "oversized":
				data = make([]byte, 1024*1024+1)
			case "read failure":
				ops := control.Store.operations()
				ops.readAll = func(io.Reader) ([]byte, error) { return nil, errors.New("unreadable") }
				control.Store.ops = &ops
				want = "state.unavailable"
			}
			switch kind {
			case "schema", "uid", "relative path", "noncanonical path", "mismatched name", "empty ancestors", "wrong ancestor path", "bad device", "bad inode", "changed device", "changed inode", "incomplete self", "short complete":
				data, _ = json.Marshal(b)
			}
			switch kind {
			case "symlink", "hardlink", "mode", "unknown name", "missing self", "missing root", "read failure":
			default:
				if err := os.WriteFile(path, data, 0600); err != nil {
					t.Fatal(err)
				}
			}
			tx, f = control.Begin(t.Context(), c, false)
			if tx != nil {
				tx.Close()
			}
			if f != want {
				t.Fatal(f, want)
			}
		})
	}
}

func TestApprovalPublicationRevalidatesAccountHome(t *testing.T) {
	for _, boundary := range []string{"before temporary", "after file sync", "after parent sync"} {
		t.Run(boundary, func(t *testing.T) {
			control, c, _, home, _ := controlFixture(t)
			store := control.Store
			store.Control = control
			if _, f := store.Commit(c, ""); f != "" {
				t.Fatal(f)
			}
			next := c
			next.Canonical = "changed"
			replace := func() {
				if err := os.Rename(home, home+"-old"); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(home, 0700); err != nil {
					t.Fatal(err)
				}
			}
			ops := store.operations()
			switch boundary {
			case "before temporary":
				ops.beforePublish = replace
			case "after file sync":
				ops.syncFile = func(f *os.File) error { err := f.Sync(); replace(); return err }
			case "after parent sync":
				ops.syncParent = func(f *os.File) error { err := f.Sync(); replace(); return err }
			}
			store.ops = &ops
			if _, f := store.Commit(next, "1"); f != "state.path" {
				t.Fatal("replaced account granted publication", f)
			}
			raw := store
			raw.Control = nil
			raw.ops = nil
			r, f := raw.Read(c)
			if f != "" {
				t.Fatal(f)
			}
			generation := "1"
			if boundary == "after parent sync" {
				generation = "2"
			}
			if r.Generation != generation {
				t.Fatal("publication was not complete old or new", r.Generation)
			}
		})
	}
}

func TestControlCatalogReadFailures(t *testing.T) {
	for _, kind := range []string{"metadata", "open", "stat", "contents", "after read", "directory unavailable"} {
		t.Run(kind, func(t *testing.T) {
			control, c, _, _, _ := controlFixture(t)
			tx, f := control.Begin(t.Context(), c, true)
			if f != "" {
				t.Fatal(f)
			}
			tx.Close()
			ops := control.Store.operations()
			bad := errors.New("unavailable catalog")
			want := port.Failure("state.unavailable")
			switch kind {
			case "metadata":
				original := ops.lstat
				ops.lstat = func(r *os.Root, n string) (os.FileInfo, error) {
					if strings.HasSuffix(n, ".json") {
						return nil, bad
					}
					return original(r, n)
				}
			case "open":
				ops.openRecord = func(*os.Root, string, int, os.FileMode) (*os.File, error) { return nil, bad }
			case "stat":
				original := ops.statFile
				ops.statFile = func(file *os.File) (os.FileInfo, error) {
					if strings.HasSuffix(file.Name(), ".json") {
						return nil, bad
					}
					return original(file)
				}
				want = "state.path"
			case "contents":
				ops.readAll = func(io.Reader) ([]byte, error) { return nil, bad }
			case "after read":
				original := ops.readAll
				ops.readAll = func(reader io.Reader) ([]byte, error) {
					data, err := original(reader)
					if file, ok := reader.(*io.LimitedReader); ok {
						if f, ok := file.R.(*os.File); ok {
							if err := os.Chmod(f.Name(), 0644); err != nil {
								t.Fatal(err)
							}
						}
					}
					return data, err
				}
				want = "state.path"
			case "directory unavailable":
				original := ops.openDir
				ops.openDir = func(root *os.Root, name string) (*os.File, error) {
					if strings.HasSuffix(root.Name(), "/coordination") {
						return nil, bad
					}
					return original(root, name)
				}
			}
			control.Store.ops = &ops
			tx, f = control.Begin(t.Context(), c, false)
			if tx != nil {
				tx.Close()
			}
			if f != want {
				t.Fatal(f, want)
			}
		})
	}
}
