package main

import (
	"debug/elf"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"syscall"
	"testing"
	"unsafe"

	"golang.org/x/sys/unix"
)

// rulesetLink is the exact readlink target Linux gives a ruleset descriptor.
const rulesetLink = "anon_inode:[landlock-ruleset]"

// measureABI is the test's own raw call, independent of the init's: the
// Landlock ABI, or 0 where Landlock is unavailable.
func measureABI() int {
	r, _, e := unix.Syscall(unix.SYS_LANDLOCK_CREATE_RULESET, 0, 0, unix.LANDLOCK_CREATE_RULESET_VERSION)
	if e != 0 {
		return 0
	}
	return int(r)
}

// checkStatic holds the init to a static ELF any approved image can mount.
func checkStatic(t *testing.T, path string) {
	t.Helper()
	f, err := elf.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	for _, prog := range f.Progs {
		if prog.Type == elf.PT_INTERP {
			t.Error("the init has a PT_INTERP: it is dynamically linked")
		}
	}
	if needed, err := f.DynString(elf.DT_NEEDED); err == nil && len(needed) != 0 {
		t.Errorf("the init needs shared libraries %v", needed)
	}
}

// probeReport is what the restricted workload saw.
type probeReport struct {
	Connect, Sendto, Own string
	NoNewPrivs           bool
	Control, After       []string
}

// leafReport is what a child of the workload saw.
type leafReport struct{ Connect, Sendto string }

type argvReport struct{ Args, Env [][]byte }

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func rulesetFDs() ([]string, error) {
	entries, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		return nil, err
	}
	var found []string
	for _, entry := range entries {
		if target, err := os.Readlink("/proc/self/fd/" + entry.Name()); err == nil && target == rulesetLink {
			found = append(found, entry.Name())
		}
	}
	return found, nil
}

func connectTo(path string) error {
	fd, err := unix.Socket(unix.AF_UNIX, unix.SOCK_STREAM|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		return err
	}
	defer func() { _ = unix.Close(fd) }()
	return unix.Connect(fd, &unix.SockaddrUnix{Name: path})
}

func sendTo(path string) error {
	fd, err := unix.Socket(unix.AF_UNIX, unix.SOCK_DGRAM|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		return err
	}
	defer func() { _ = unix.Close(fd) }()
	return unix.Sendto(fd, []byte("x"), 0, &unix.SockaddrUnix{Name: path})
}

func writeJSON(t *testing.T, path string, v any) {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

// runWorkload is the workload side of every enforcement row, and the helper
// of the layer-exhaustion row. It makes its marker first, except the helper,
// which is not a workload.
func runWorkload(t *testing.T) {
	mode := os.Getenv(modeEnv)
	if mode != "exhaust" {
		if err := os.WriteFile(os.Getenv(markerEnv), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	switch mode {
	case "report":
		writeJSON(t, os.Getenv(reportEnv), argvReport{Args: toBytes(os.Args), Env: toBytes(os.Environ())})
	case "probe":
		probe(t)
	case "leaf":
		stream, dgram := os.Getenv(streamEnv), os.Getenv(dgramEnv)
		writeJSON(t, os.Getenv(leafEnv), leafReport{Connect: errText(connectTo(stream)), Sendto: errText(sendTo(dgram))})
	case "exhaust":
		exhaust(t)
	}
}

// probe reports what the restricted workload can and cannot reach, and
// starts a child to report the same.
func probe(t *testing.T) {
	stream, dgram := os.Getenv(streamEnv), os.Getenv(dgramEnv)
	var report probeReport
	report.Connect, report.Sendto = errText(connectTo(stream)), errText(sendTo(dgram))

	own := filepath.Join(filepath.Dir(stream), "own-"+fmt.Sprint(os.Getpid()))
	fd, err := unix.Socket(unix.AF_UNIX, unix.SOCK_STREAM|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := unix.Bind(fd, &unix.SockaddrUnix{Name: own}); err != nil {
		t.Fatal(err)
	}
	if err := unix.Listen(fd, 4); err != nil {
		t.Fatal(err)
	}
	report.Own = errText(connectTo(own))
	_ = unix.Close(fd)

	status, err := os.ReadFile("/proc/self/status")
	if err != nil {
		t.Fatal(err)
	}
	report.NoNewPrivs = strings.Contains(string(status), "\nNoNewPrivs:\t1\n")

	// The positive control: the probe must see a ruleset descriptor this
	// workload opened itself before its absence means anything.
	attr := unix.LandlockRulesetAttr{Handled_access_fs: unix.LANDLOCK_ACCESS_FS_EXECUTE}
	ruleset, _, e := unix.Syscall(unix.SYS_LANDLOCK_CREATE_RULESET, uintptr(unsafe.Pointer(&attr)), unsafe.Sizeof(attr), 0)
	if e != 0 {
		t.Fatalf("control ruleset: %v", e)
	}
	if report.Control, err = rulesetFDs(); err != nil {
		t.Fatal(err)
	}
	if err := unix.Close(int(ruleset)); err != nil {
		t.Fatal(err)
	}
	if report.After, err = rulesetFDs(); err != nil {
		t.Fatal(err)
	}
	writeJSON(t, os.Getenv(reportEnv), report)

	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	leaf := exec.Command(self, "-test.run=^TestInitWorkload$")
	leaf.Env = append(slices.Clone(os.Environ()), modeEnv+"=leaf")
	if out, err := leaf.CombinedOutput(); err != nil {
		t.Fatalf("leaf: %v\n%s", err, out)
	}
}

// exhaust is the layer-exhaustion helper. On one locked thread it sets
// no_new_privs and restricts itself with the init's own ruleset until the
// kernel's layer limit answers E2BIG, which fills whatever slots remain, and
// then it executes the init from that same thread.
func exhaust(t *testing.T) {
	runtime.LockOSThread()
	if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
		t.Fatal(err)
	}
	attr := unix.LandlockRulesetAttr{Handled_access_fs: unix.LANDLOCK_ACCESS_FS_RESOLVE_UNIX}
	full := false
	for call := 0; call < 17 && !full; call++ {
		fd, _, e := unix.Syscall(unix.SYS_LANDLOCK_CREATE_RULESET, uintptr(unsafe.Pointer(&attr)), unsafe.Sizeof(attr), 0)
		if e != 0 {
			t.Fatalf("create ruleset: %v", e)
		}
		_, _, e = unix.Syscall(unix.SYS_LANDLOCK_RESTRICT_SELF, fd, 0, 0)
		_ = unix.Close(int(fd))
		switch {
		case errors.Is(e, syscall.E2BIG):
			full = true
		case e != 0:
			t.Fatalf("restrict: %v", e)
		}
	}
	if !full {
		t.Fatal("17 restrictions without E2BIG")
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	initPath := os.Getenv(initEnv)
	t.Fatalf("exec: %v", syscall.Exec(initPath, []string{"itos-container-init", "--", self, "-test.run=^TestInitWorkload$"}, os.Environ()))
}

// enforcement is the init's rows on a host with Landlock ABI 9 or later.
func enforcement(t *testing.T, binary, root string, withCover func([]string) []string) {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(root, "bin")
	if err := os.Mkdir(bin, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"t65-workload", "t65-rel"} {
		if err := os.Symlink(self, filepath.Join(bin, name)); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("argv and environment", func(t *testing.T) {
		rest := []string{"-test.run=^TestInitWorkload$", "", "a b", "tab\there", "new\nline", "-leading", "--", "--help", "bad\xffbyte", "--", "x=y"}
		for _, argv0 := range []string{"t65-workload", filepath.Join(bin, "t65-workload")} {
			marker, report := filepath.Join(root, "m-report"), filepath.Join(root, "r-report")
			_ = os.Remove(marker)
			_ = os.Remove(report)
			env := []string{"PATH=" + bin, modeEnv + "=report", markerEnv + "=" + marker, reportEnv + "=" + report,
				"EQ=a=b=c", "EMPTY=", "SPACES=a b  c", "NL=x\ny", "RAW=\xfe\xff"}
			argv := append([]string{argv0}, rest...)
			code, _, stderr := runInit(t, binary, invocation{args: append([]string{"--"}, argv...), env: env, dir: root})
			if code != 0 || strings.Contains(stderr, "itos-container-init:") {
				t.Errorf("init with argv[0] %q = %d, %q; want the workload's own exit 0 and no init line", argv0, code, stderr)
			}
			var got argvReport
			readJSON(t, report, &got)
			if !slices.Equal(toStrings(got.Args), argv) {
				t.Errorf("argv = %q; want %q", toStrings(got.Args), argv)
			}
			gotEnv, wantEnv := toStrings(got.Env), slices.Clone(env)
			slices.Sort(gotEnv)
			slices.Sort(wantEnv)
			if !slices.Equal(gotEnv, wantEnv) {
				t.Errorf("env = %q; want %q", gotEnv, wantEnv)
			}
		}
	})

	t.Run("restriction", func(t *testing.T) {
		// The sockets this process made, which the workload must not reach.
		short := socketDir(t)
		stream, dgram := filepath.Join(short, "s"), filepath.Join(short, "d")
		for _, s := range []struct {
			kind int
			path string
		}{{unix.SOCK_STREAM, stream}, {unix.SOCK_DGRAM, dgram}} {
			fd, err := unix.Socket(unix.AF_UNIX, s.kind|unix.SOCK_CLOEXEC, 0)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = unix.Close(fd) }()
			if err := unix.Bind(fd, &unix.SockaddrUnix{Name: s.path}); err != nil {
				t.Fatal(err)
			}
			if s.kind == unix.SOCK_STREAM {
				if err := unix.Listen(fd, 4); err != nil {
					t.Fatal(err)
				}
			}
		}
		if err := connectTo(stream); err != nil {
			t.Fatalf("the test's own connect = %v; the sockets must be reachable outside the init", err)
		}
		if err := sendTo(dgram); err != nil {
			t.Fatalf("the test's own sendto = %v", err)
		}
		report, leaf := filepath.Join(root, "probe"), filepath.Join(root, "leaf")
		env := []string{"PATH=" + bin, modeEnv + "=probe", markerEnv + "=" + filepath.Join(root, "m-probe"), reportEnv + "=" + report,
			leafEnv + "=" + leaf, streamEnv + "=" + stream, dgramEnv + "=" + dgram}
		code, _, stderr := runInit(t, binary, invocation{args: []string{"--", "t65-workload", "-test.run=^TestInitWorkload$"}, env: env, dir: root})
		if code != 0 {
			t.Fatalf("init = %d, %q; want the workload's exit 0", code, stderr)
		}
		var got probeReport
		readJSON(t, report, &got)
		denied := syscall.EACCES.Error()
		if got.Connect != denied || got.Sendto != denied || got.Own != "" {
			t.Errorf("workload connect %q, sendto %q, own socket %q; want %q, %q, success", got.Connect, got.Sendto, got.Own, denied, denied)
		}
		if !got.NoNewPrivs {
			t.Error("the workload's /proc/self/status does not show NoNewPrivs 1")
		}
		if len(got.Control) != 1 || len(got.After) != 0 {
			t.Errorf("ruleset descriptors: with one open %v, after closing it %v; want one, then none", got.Control, got.After)
		}
		var child leafReport
		readJSON(t, leaf, &child)
		if child.Connect != denied || child.Sendto != denied {
			t.Errorf("the workload's child: connect %q, sendto %q; want %q for both", child.Connect, child.Sendto, denied)
		}
	})

	t.Run("layer exhaustion", func(t *testing.T) {
		marker := filepath.Join(root, "m-exhaust")
		env := []string{"PATH=" + bin, modeEnv + "=exhaust", markerEnv + "=" + marker, initEnv + "=" + binary}
		code, stdout, stderr := runInit(t, self, invocation{args: []string{"-test.run=^TestInitWorkload$"}, env: withCover(env), dir: root})
		if code != 125 || stdout != "" || stderr != "itos-container-init: landlock-failed\n" {
			t.Errorf("init with every layer used = %d, %q, %q; want 125, landlock-failed", code, stdout, stderr)
		}
		absent(t, marker)
	})

	t.Run("not found and exec failures", func(t *testing.T) {
		rel := filepath.Join(root, "rel")
		if err := os.MkdirAll(filepath.Join(rel, "bin"), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(self, filepath.Join(rel, "bin", "t65-rel")); err != nil {
			t.Fatal(err)
		}
		plain := filepath.Join(bin, "plain")
		if err := os.WriteFile(plain, []byte("#!/bin/sh\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		for _, row := range []struct {
			name, path, dir string
			argv            []string
			extra           []string
			code            int
			line            string
		}{
			{"missing bare name", bin, root, []string{"t65-absent"}, nil, 127, "not-found"},
			{"bare name in a relative PATH entry", "bin", rel, []string{"t65-rel"}, nil, 127, "not-found"},
			{"relative PATH entry with execerrdot=0", "bin", rel, []string{"t65-rel"}, []string{"GODEBUG=execerrdot=0"}, 127, "not-found"},
			{"missing path with a slash", bin, root, []string{filepath.Join(bin, "nope")}, nil, 127, "not-found"},
			{"non-executable explicit path", bin, root, []string{plain}, nil, 126, "exec-failed"},
		} {
			marker := filepath.Join(root, "m-nf")
			_ = os.Remove(marker)
			env := append([]string{"PATH=" + row.path, modeEnv + "=report", markerEnv + "=" + marker}, row.extra...)
			code, stdout, stderr := runInit(t, binary, invocation{args: append([]string{"--"}, row.argv...), env: withCover(env), dir: row.dir})
			if code != row.code || stdout != "" || stderr != "itos-container-init: "+row.line+"\n" {
				t.Errorf("%s = %d, %q, %q; want %d, %s", row.name, code, stdout, stderr, row.code, row.line)
			}
			absent(t, marker)
		}
	})
}
