package process

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/donvargax/itos-container/internal/slices/workerinit/port"
	"github.com/donvargax/itos-container/internal/slices/workerinit/port/porttest"
)

const (
	modeEnv   = "T65_EXEC_MODE"
	argvEnv   = "T65_ARGV"
	nameEnv   = "T65_NAME"
	outEnv    = "T65_OUT"
	reportEnv = "T65_REPORT"
)

// report is what the exec target saw, as bytes: JSON strings would not hold a
// non-UTF-8 byte.
type report struct{ Args, Env [][]byte }

func toBytes(list []string) [][]byte {
	out := make([][]byte, len(list))
	for i, s := range list {
		out[i] = []byte(s)
	}
	return out
}

func toStrings(list [][]byte) []string {
	out := make([]string, len(list))
	for i, b := range list {
		out[i] = string(b)
	}
	return out
}

// TestExecChild is the real adapter's side of the tests below. Run on its
// own it does nothing. It never forks: on success the process becomes the
// target, so the parent sees the target's own report.
func TestExecChild(t *testing.T) {
	mode := os.Getenv(modeEnv)
	if mode == "" {
		return
	}
	runtime.LockOSThread()
	a := New()
	switch mode {
	case "exec":
		var raw [][]byte
		if err := json.Unmarshal([]byte(os.Getenv(argvEnv)), &raw); err != nil {
			t.Fatal(err)
		}
		argv := toStrings(raw)
		path, err := a.Lookup(argv[0])
		if err != nil {
			t.Fatalf("Lookup(%q) = %v", argv[0], err)
		}
		t.Fatalf("Exec(%q) returned %v", path, a.Exec(path, argv))
	case "probe":
		name := os.Getenv(nameEnv)
		var lines []string
		path, err := a.Lookup(name)
		switch {
		case errors.Is(err, port.ErrNotFound):
			lines = append(lines, "lookup=notfound")
		case err != nil:
			lines = append(lines, "lookup=error")
		default:
			lines = append(lines, "lookup=ok:"+path)
			switch err := a.Exec(path, []string{name}); {
			case errors.Is(err, port.ErrNotFound):
				lines = append(lines, "exec=notfound")
			case err != nil:
				lines = append(lines, "exec=other")
			default:
				lines = append(lines, "exec=nil")
			}
		}
		if err := os.WriteFile(os.Getenv(outEnv), []byte(strings.Join(lines, "\n")), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// TestExecTarget is what the child becomes: it writes what it was given.
func TestExecTarget(t *testing.T) {
	path := os.Getenv(reportEnv)
	if path == "" {
		return
	}
	data, err := json.Marshal(report{Args: toBytes(os.Args), Env: toBytes(os.Environ())})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

// runChild runs this test binary as the adapter's child in dir, with exactly
// env, and returns its error and output.
func runChild(t *testing.T, dir string, env []string) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	args := []string{"-test.run=^TestExecChild$"}
	if cover := flag.Lookup("test.gocoverdir").Value.String(); cover != "" {
		args = append(args, "-test.gocoverdir="+cover)
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(ctx, self, args...)
	command.Dir, command.Env = dir, env
	out, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w\n%s", err, out)
	}
	return nil
}

// workspace makes a short private directory with bin/t65-target, a link to
// this test binary, and returns the directory and bin.
func workspace(t *testing.T) (root, bin string) {
	t.Helper()
	root = t.TempDir()
	bin = filepath.Join(root, "bin")
	if err := os.Mkdir(bin, 0o700); err != nil {
		t.Fatal(err)
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"t65-target", "t65-rel"} {
		if err := os.Symlink(self, filepath.Join(bin, name)); err != nil {
			t.Fatal(err)
		}
	}
	return root, bin
}

func testRealProcess(t *testing.T) {
	_, bin := workspace(t)
	t.Setenv("PATH", bin)
	porttest.ProcessContract(t, func() (port.Process, porttest.ProcessWant) {
		return New(), porttest.ProcessWant{
			Bare: "t65-target", Resolved: filepath.Join(bin, "t65-target"),
			Missing: "t65-absent", MissingPath: filepath.Join(bin, "nope"),
		}
	})
}

func testExec(t *testing.T) {
	root, bin := workspace(t)
	rest := []string{"-test.run=^TestExecTarget$", "", "a b", "tab\there", "new\nline", "-leading", "--", "--help", "bad\xffbyte", "--", "x=y"}
	for _, row := range []struct{ name, argv0, dir string }{
		{"bare name through PATH", "t65-target", root},
		{"absolute path", filepath.Join(bin, "t65-target"), root},
		{"relative path with a slash", "./t65-target", bin},
	} {
		t.Run(row.name, func(t *testing.T) {
			argv := append([]string{row.argv0}, rest...)
			encoded, err := json.Marshal(toBytes(argv))
			if err != nil {
				t.Fatal(err)
			}
			reportPath := filepath.Join(root, "report")
			_ = os.Remove(reportPath)
			env := []string{"PATH=" + bin, modeEnv + "=exec", argvEnv + "=" + string(encoded), reportEnv + "=" + reportPath,
				"EQ=a=b=c", "EMPTY=", "SPACES=a b  c", "NL=x\ny", "RAW=\xfe\xff"}
			if err := runChild(t, row.dir, env); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(reportPath)
			if err != nil {
				t.Fatalf("the target left no report: %v", err)
			}
			var got report
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(toStrings(got.Args), argv) {
				t.Errorf("argv = %q; want %q", toStrings(got.Args), argv)
			}
			gotEnv, wantEnv := toStrings(got.Env), slices.Clone(env)
			slices.Sort(gotEnv)
			slices.Sort(wantEnv)
			if !slices.Equal(gotEnv, wantEnv) {
				t.Errorf("env = %q; want %q", gotEnv, wantEnv)
			}
		})
	}

	rel := filepath.Join(root, "rel")
	if err := os.MkdirAll(filepath.Join(rel, "bin"), 0o700); err != nil {
		t.Fatal(err)
	}
	self, _ := os.Executable()
	if err := os.Symlink(self, filepath.Join(rel, "bin", "t65-rel")); err != nil {
		t.Fatal(err)
	}
	plain := filepath.Join(bin, "plain")
	if err := os.WriteFile(plain, []byte("#!/bin/sh\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	missingPath := filepath.Join(bin, "nope")
	for _, row := range []struct {
		name, nameArg, path, dir string
		godebug                  string
		want                     string
	}{
		{"bare name not on PATH", "t65-absent", bin, root, "", "lookup=notfound"},
		{"bare name only in a relative PATH entry", "t65-rel", "bin", rel, "", "lookup=notfound"},
		{"relative PATH entry with execerrdot=0", "t65-rel", "bin", rel, "execerrdot=0", "lookup=notfound"},
		{"missing path with a slash", missingPath, bin, root, "", "lookup=ok:" + missingPath + "\nexec=notfound"},
		{"non-executable explicit path", plain, bin, root, "", "lookup=ok:" + plain + "\nexec=other"},
	} {
		t.Run(row.name, func(t *testing.T) {
			out := filepath.Join(root, "out")
			_ = os.Remove(out)
			env := []string{"PATH=" + row.path, modeEnv + "=probe", nameEnv + "=" + row.nameArg, outEnv + "=" + out}
			if row.godebug != "" {
				env = append(env, "GODEBUG="+row.godebug)
			}
			if err := runChild(t, row.dir, env); err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(out)
			if err != nil || string(got) != row.want {
				t.Errorf("probe = %q, %v; want %q", got, err, row.want)
			}
		})
	}
}
