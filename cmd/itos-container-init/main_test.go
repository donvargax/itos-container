package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

const (
	modeEnv   = "T65_MODE"
	markerEnv = "T65_MARKER"
	reportEnv = "T65_REPORT"
	leafEnv   = "T65_LEAF"
	streamEnv = "T65_STREAM"
	dgramEnv  = "T65_DGRAM"
	initEnv   = "T65_INIT"
)

// workloadRun is the workload's own test: the init's argv names this test
// binary, which writes a marker, then the report its mode asks for. Run on
// its own it does nothing.
func TestInitWorkload(t *testing.T) {
	if os.Getenv(modeEnv) == "" {
		return
	}
	runWorkload(t)
}

// initBinary builds the init as a static, trimmed binary. When this run
// collects coverage the binary counts its own, so refusal runs keep main's.
func initBinary(t *testing.T) (path, coverdir string) {
	t.Helper()
	goTool, err := exec.LookPath("go")
	if err != nil {
		t.Fatal(err)
	}
	path = filepath.Join(t.TempDir(), "itos-container-init")
	if runtime.GOOS == "windows" {
		path += ".exe"
	}
	args := []string{"build", "-trimpath", "-o", path}
	coverdir = flag.Lookup("test.gocoverdir").Value.String()
	if coverdir != "" {
		args = append(args, "-cover")
	}
	command := exec.Command(goTool, append(args, ".")...)
	command.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := command.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	return path, coverdir
}

// invocation is one run of the init binary.
type invocation struct {
	args []string
	env  []string
	dir  string
}

// runInit runs the init with exactly the invocation's environment and
// returns its exit code and what it wrote.
func runInit(t *testing.T, binary string, in invocation) (code int, stdout, stderr string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, binary, in.args...)
	command.Dir, command.Env = in.dir, in.env
	var out, errOut bytes.Buffer
	command.Stdout, command.Stderr = &out, &errOut
	err := command.Run()
	var exit *exec.ExitError
	switch {
	case err == nil:
		return 0, out.String(), errOut.String()
	case errors.As(err, &exit):
		return exit.ExitCode(), out.String(), errOut.String()
	}
	t.Fatalf("running %s: %v", binary, err)
	return 0, "", ""
}

func absent(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the workload ran: %s exists (%v)", path, err)
	}
}

func readJSON(t *testing.T, path string, into any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the workload left no report: %v", err)
	}
	if err := json.Unmarshal(data, into); err != nil {
		t.Fatal(err)
	}
}

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

func TestInitBinary(t *testing.T) {
	binary, coverdir := initBinary(t)
	checkStatic(t, binary)

	abi := measureABI()
	enforcing := runtime.GOOS == "linux" && abi >= 9
	branch := "refusal"
	if enforcing {
		branch = "enforcement"
	}
	t.Logf("workerinit kernel branch: landlock ABI %d, %s", abi, branch)

	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	withCover := func(env []string) []string {
		if coverdir != "" {
			return append(env, "GOCOVERDIR="+coverdir)
		}
		return env
	}
	marker := func(name string) string { return filepath.Join(root, name+".marker") }
	workload := []string{self, "-test.run=^TestInitWorkload$"}

	// The usage refusals hold on every host and never reach the workload.
	for i, args := range [][]string{
		nil,
		{"--"},
		append([]string{}, workload...),
		append([]string{"-h", "--"}, workload...),
		append([]string{"--help"}, workload...),
		append([]string{"--version"}, workload...),
		append([]string{"x", "--"}, workload...),
	} {
		m := marker(fmt.Sprintf("usage-%d", i))
		code, stdout, stderr := runInit(t, binary, invocation{args: args, env: withCover([]string{modeEnv + "=report", markerEnv + "=" + m}), dir: root})
		if code != 125 || stdout != "" || stderr != "itos-container-init: usage\n" {
			t.Errorf("init %q = %d, %q, %q; want 125 and the usage line", args, code, stdout, stderr)
		}
		absent(t, m)
	}

	if !enforcing {
		m := marker("refusal")
		args := append([]string{"--"}, workload...)
		code, stdout, stderr := runInit(t, binary, invocation{args: args, env: withCover([]string{modeEnv + "=report", markerEnv + "=" + m}), dir: root})
		if code != 125 || stdout != "" || stderr != "itos-container-init: landlock-unavailable\n" {
			t.Errorf("init without Landlock ABI 9 = %d, %q, %q; want 125, landlock-unavailable", code, stdout, stderr)
		}
		absent(t, m)
		return
	}
	enforcement(t, binary, root, withCover)
}

// socketDir makes a private directory short enough for a socket path: TMPDIR
// can be longer than the 108 bytes a sockaddr_un holds.
func socketDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "t65")
	if err == nil && len(dir) > 60 {
		_ = os.Remove(dir)
		dir, err = os.MkdirTemp("/tmp", "t65")
	}
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

// TestMainRefusesInProcess runs main itself, so its own lines are measured:
// a usage refusal never reaches Landlock or the workload.
func TestMainRefusesInProcess(t *testing.T) {
	stderr, err := os.CreateTemp(t.TempDir(), "stderr")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stderr.Close() })

	oldArgs, oldStderr, oldExit := os.Args, os.Stderr, exit
	t.Cleanup(func() { os.Args, os.Stderr, exit = oldArgs, oldStderr, oldExit })
	code := -1
	os.Args, os.Stderr, exit = []string{"itos-container-init"}, stderr, func(c int) { code = c }

	main()
	// main locks this goroutine to its thread; release it for later tests.
	runtime.UnlockOSThread()

	if code != 125 {
		t.Errorf("main exited %d, want 125", code)
	}
	got, err := os.ReadFile(stderr.Name())
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "itos-container-init: usage\n" {
		t.Errorf("main wrote %q, want the usage line", got)
	}
}
