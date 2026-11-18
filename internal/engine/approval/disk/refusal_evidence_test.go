package disk

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/donvargax/itos-container/internal/engine/approval/port"
)

func TestSessionEvidenceReadsOnlyPrivatePublishedFiles(t *testing.T) {
	s, c, _ := diskFixture(t)
	r := Registry{Store: s}
	id, dir := stagedSession(t, r, c)
	path := filepath.Join(dir, stagedLogName)
	check := func(ctx context.Context, id string, want port.Failure) port.SessionEvidence {
		t.Helper()
		e, failure := r.Evidence(ctx, c, id)
		if failure != want {
			t.Fatalf("evidence read returned %q, want %q", failure, want)
		}
		if failure != "" && (e.Log != nil || e.Report != nil) {
			t.Fatal("refused evidence exposed unverified bytes")
		}
		return e
	}
	check(t.Context(), "invalid", "state.invalid")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	check(ctx, id, "state.unavailable")
	check(t.Context(), strings.Repeat("b", 64), "")
	r.Store.StateHome = "."
	check(t.Context(), id, "state.location")
	r.Store = s
	if e := check(t.Context(), id, ""); e.Log != nil || e.Report != nil || e.Path != path {
		t.Fatal("empty session lost its destination or invented evidence", e)
	}

	staged, failure := r.Stage(t.Context(), c, id)
	if failure != "" || staged.Path() != path {
		t.Fatal("stage", failure)
	}
	if staged.Write([]byte("line\n")) != "" || staged.Attach([]byte("sidecar")) != "" || staged.Publish() != "" {
		t.Fatal("publication failed")
	}
	if e := check(t.Context(), id, ""); string(e.Log) != "line\n" || string(e.Report) != "sidecar" || e.Path != path {
		t.Fatal("committed pair did not round trip", e)
	}

	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	check(t.Context(), id, "state.unsafe")

	// A replacement after both absent-file observations must still invalidate the pin.
	for _, name := range []string{stagedLogName, stagedReportName} {
		if err := os.Remove(filepath.Join(dir, name)); err != nil {
			t.Fatal(err)
		}
	}
	ops := s.operations()
	lstat := ops.lstat
	reads := 0
	ops.lstat = func(root *os.Root, name string) (os.FileInfo, error) {
		info, err := lstat(root, name)
		if name == stagedLogName || name == stagedReportName {
			reads++
			if reads == 2 {
				if err := os.Rename(dir, dir+"-moved"); err != nil {
					t.Fatal(err)
				}
			}
		}
		return info, err
	}
	r.Store.ops = &ops
	check(t.Context(), id, "state.path")
}

func TestEvidenceConfirmsPairDurabilityBeforeCommitting(t *testing.T) {
	s, c, _ := diskFixture(t)
	r := Registry{Store: s}
	id, dir := stagedSession(t, r, c)
	stage, failure := r.Stage(t.Context(), c, id)
	if failure != "" {
		t.Fatal(failure)
	}
	l := stage.(*stagedLog)
	defer l.Close()
	bad := errors.New("report durability fault")
	sync := l.pin.ops.syncParent
	l.pin.ops.syncParent = func(f *os.File) error {
		if _, err := os.Stat(filepath.Join(dir, stagedReportName)); err == nil {
			return bad
		}
		return sync(f)
	}
	if l.Write([]byte("line\n")) != "" || l.Attach([]byte("report")) != "" {
		t.Fatal("staging failed")
	}
	if f := l.Publish(); f != "state.write" {
		t.Fatalf("publish = %q, want the report parent sync failure", f)
	}
	for _, name := range []string{stagedLogName, stagedReportName} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("failed publication left %s invisible: %v", name, err)
		}
	}

	// The pair is visible but its report rename was never confirmed durable.
	// Evidence must refuse it while the directory sync cannot be re-run, and
	// serve it only after one sync confirms the pair's directory entry. A
	// registry snapshots the store at construction, so the faulted operations
	// reach it only through a registry built after they are installed.
	ops := s.operations()
	var attempted, confirmed int
	ops.syncParent = func(f *os.File) error {
		attempted++
		return bad
	}
	s.ops = &ops
	refused := Registry{Store: s}
	e, failure := refused.Evidence(t.Context(), c, id)
	if failure != "state.write" || e.Log != nil || e.Report != nil {
		t.Fatalf("unconfirmed pair served as evidence: %q %+v", failure, e)
	}
	ops.syncParent = func(f *os.File) error {
		confirmed++
		return sync(f)
	}
	e, failure = refused.Evidence(t.Context(), c, id)
	if failure != "" || string(e.Log) != "line\n" || string(e.Report) != "report" {
		t.Fatalf("confirmed pair did not round trip: %q %+v", failure, e)
	}
	if attempted != 1 || confirmed != 1 {
		t.Fatalf("evidence attempted %d and confirmed %d syncs, want one of each", attempted, confirmed)
	}
}

func TestReadEvidenceRejectsUnverifiedFiles(t *testing.T) {
	s, c, _ := diskFixture(t)
	id, dir := stagedSession(t, Registry{Store: s}, c)
	p, failure := s.pinPath(c, false, dir, 4)
	if failure != "" || p == nil {
		t.Fatal("pin", id, failure)
	}
	defer p.close()
	const name = "evidence"
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("data"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		fault string
		want  port.Failure
	}{
		{"at limit", ""}, {"over limit", "state.unsafe"}, {"lstat", "state.unavailable"},
		{"open", "state.path"}, {"stat", "state.path"}, {"read", "state.unavailable"},
		{"growth after stat", "state.path"}, {"symlink", "state.path"}, {"mode", "state.unsafe"},
		{"second link", "state.unsafe"}, {"pipe", "state.unsafe"},
	} {
		t.Run(tc.fault, func(t *testing.T) {
			ops := s.operations()
			limit := int64(4)
			switch tc.fault {
			case "over limit":
				limit--
			case "lstat":
				ops.lstat = func(*os.Root, string) (os.FileInfo, error) { return nil, os.ErrPermission }
			case "pipe":
				reader, writer, err := os.Pipe()
				if err != nil {
					t.Fatal(err)
				}
				defer func() { _ = reader.Close() }()
				defer func() { _ = writer.Close() }()
				ops.lstat = func(*os.Root, string) (os.FileInfo, error) { return reader.Stat() }
				ops.openRecord = func(*os.Root, string, int, os.FileMode) (*os.File, error) {
					t.Fatal("attempted to open a pipe as a refusal log")
					return nil, os.ErrInvalid
				}
			case "open":
				ops.openRecord = func(*os.Root, string, int, os.FileMode) (*os.File, error) { return nil, os.ErrPermission }
			case "stat":
				ops.statFile = func(*os.File) (os.FileInfo, error) { return nil, os.ErrPermission }
			case "read":
				ops.readAll = func(io.Reader) ([]byte, error) { return nil, os.ErrPermission }
			case "growth after stat":
				read := ops.readAll
				ops.readAll = func(r io.Reader) ([]byte, error) {
					if err := os.Truncate(path, 6); err != nil {
						t.Fatal(err)
					}
					defer func() {
						if err := os.Truncate(path, 4); err != nil {
							t.Error(err)
						}
					}()
					return read(r)
				}
			case "symlink", "second link":
				link := os.Link
				if tc.fault == "symlink" {
					link = os.Symlink
				}
				if err := link(path, path+"-link"); err != nil {
					t.Fatal(err)
				}
				defer func() {
					if err := os.Remove(path + "-link"); err != nil {
						t.Error(err)
					}
				}()
			case "mode":
				if err := os.Chmod(path, 0640); err != nil {
					t.Fatal(err)
				}
				defer func() {
					if err := os.Chmod(path, 0600); err != nil {
						t.Error(err)
					}
				}()
			}
			observed := name
			if tc.fault == "symlink" {
				observed += "-link"
			}
			copy := *p
			copy.ops = ops
			data, failure := copy.readEvidence(observed, limit)
			if failure != tc.want || (failure != "" && data != nil) || (failure == "" && string(data) != "data") {
				t.Fatalf("read = %q, %s; want %s", data, failure, tc.want)
			}
		})
	}
}

func TestAttachRefusalReportFailuresReleaseStage(t *testing.T) {
	for _, fault := range []string{"closed", "poisoned", "duplicate", "nonce", "create", "chmod", "short write", "write", "fsync", "report close", "report rename", "report parent sync"} {
		t.Run(fault, func(t *testing.T) {
			s, c, _ := diskFixture(t)
			id, dir := stagedSession(t, Registry{Store: s}, c)
			stage, failure := (Registry{Store: s}).Stage(t.Context(), c, id)
			if failure != "" {
				t.Fatal(failure)
			}
			l := stage.(*stagedLog)
			defer l.Close()
			before := []string{"controller.lock", "operation.lock", "record.json"}
			bad := errors.New("report storage fault")
			ops := &l.pin.ops
			want := port.Failure("state.write")
			switch fault {
			case "closed":
				l.Close()
			case "poisoned":
				ops.write = func(*os.File, []byte) (int, error) { return 0, bad }
				if l.Write([]byte("data")) == "" {
					t.Fatal("failed write succeeded")
				}
			case "duplicate":
				if f := l.Attach([]byte("first")); f != "" {
					t.Fatal(f)
				}
				want = "state.conflict"
			case "nonce":
				ops.nonce = func([]byte) (int, error) { return 0, bad }
			case "create":
				ops.create = func(*os.Root, string) (*os.File, error) { return nil, bad }
			case "chmod":
				ops.chmod = func(*os.File, os.FileMode) error { return bad }
			case "short write":
				ops.write = func(*os.File, []byte) (int, error) { return 1, nil }
			case "write":
				ops.write = func(*os.File, []byte) (int, error) { return 0, bad }
			case "fsync":
				ops.syncFile = func(*os.File) error { return bad }
			default:
				want = ""
			}
			if got := l.Attach([]byte("report")); got != want {
				t.Fatalf("attach = %q, want %q", got, want)
			}
			if want == "" {
				switch fault {
				case "report close":
					ops.closeFile = func(f *os.File) error {
						if f == l.report {
							return bad
						}
						return f.Close()
					}
				case "report rename":
					old := ops.rename
					ops.rename = func(root *os.Root, from, to string) error {
						if to == stagedReportName {
							return bad
						}
						return old(root, from, to)
					}
				case "report parent sync":
					ops.syncParent = func(f *os.File) error {
						if _, err := os.Stat(filepath.Join(dir, stagedReportName)); err == nil {
							return bad
						}
						return f.Sync()
					}
					before = append(before, stagedReportName)
				}
				if f := l.Publish(); f != "state.write" {
					t.Fatalf("publish = %q", f)
				}
				before = append(before, stagedLogName)
			}
			l.Close()
			slices.Sort(before)
			if names := sessionNames(t, dir); !slices.Equal(names, before) {
				t.Fatalf("after %s: %v, want %v", fault, names, before)
			}
		})
	}
}
