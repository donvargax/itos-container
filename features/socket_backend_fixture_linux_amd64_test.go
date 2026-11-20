package features

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

const socketBackendSupported = true

func runSocketBackendFixture(t *testing.T) {
	t.Run("kernel_answer", landlockKernelAnswer)
	t.Run("unpinned", landlockUnpinned)
	t.Run("umask_composed", landlockUmaskComposed)
	t.Run("other_call_refused", landlockOtherCallRefused)
	t.Run("arrival_action", landlockArrivalAction)
	t.Run("no_query", landlockNoQuery)
	t.Run("exec_unchanged", landlockExecUnchanged)
	t.Run("no_feature_binaries_left", landlockNoFeatureBinariesLeft)
	t.Run("cleanup_ends_supervision", landlockCleanupEndsSupervision)
	t.Run("helper_failure", landlockHelperFailure)
	t.Run("refused_answers", landlockRefusedAnswers)
	t.Run("runtimes", landlockRuntimes)
}

// landlockTestWorld is a world with a scratch folder whose supervision ends with the test.
func landlockTestWorld(t *testing.T) *world {
	t.Helper()
	w := &world{dir: t.TempDir(), deadline: time.Minute}
	t.Cleanup(func() {
		if err := w.closeLandlock(); err != nil {
			t.Errorf("ending supervision: %v", err)
		}
	})
	return w
}

func landlockProgram(t *testing.T) string {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return exe
}

// landlockProbe runs the probe through runEnv and returns the lines it printed.
func landlockProbe(t *testing.T, w *world, actions ...string) []landlockProbeLine {
	t.Helper()
	args := append([]string{landlockProbeMode}, actions...)
	if err := w.runEnv(w.dir, w.env(), landlockProgram(t), args...); err != nil {
		t.Fatal(err)
	}
	return landlockLines(t, w.stdout)
}

func landlockLines(t *testing.T, output string) []landlockProbeLine {
	t.Helper()
	var lines []landlockProbeLine
	for _, text := range strings.Split(output, "\n") {
		if text == "" {
			continue
		}
		var line landlockProbeLine
		if err := json.Unmarshal([]byte(text), &line); err != nil {
			t.Fatalf("probe line %q: %v", text, err)
		}
		lines = append(lines, line)
	}
	return lines
}

func landlockEvents(lines []landlockProbeLine, event string) []landlockProbeLine {
	var found []landlockProbeLine
	for _, line := range lines {
		if line.Event == event {
			found = append(found, line)
		}
	}
	return found
}

// landlockReplies is each reply of the lines as "<call> <ret> <errno>".
func landlockReplies(lines []landlockProbeLine) []string {
	var replies []string
	for _, line := range landlockEvents(lines, "reply") {
		text := line.Call + " " + strconv.Itoa(line.Ret)
		if line.Errno != "" {
			text += " " + line.Errno
		}
		replies = append(replies, text)
	}
	return replies
}

// landlockRecordText is the harness's record, each invocation's calls as
// "<call> <answer>" joined by commas.
func landlockRecordText(w *world) []string {
	var record []string
	for _, calls := range w.landlockRecord() {
		var texts []string
		for _, call := range calls {
			kind := "other"
			if call.Version {
				kind = "version"
			}
			texts = append(texts, kind+" "+call.Answer)
		}
		record = append(record, strings.Join(texts, ","))
	}
	return record
}

func requireLandlockRecord(t *testing.T, w *world, want ...string) {
	t.Helper()
	if got := landlockRecordText(w); !slices.Equal(got, want) {
		t.Fatalf("the harness's record is %q, want %q", got, want)
	}
}

func requireLandlockReplies(t *testing.T, lines []landlockProbeLine, want ...string) {
	t.Helper()
	if got := landlockReplies(lines); !slices.Equal(got, want) {
		t.Fatalf("the program's replies are %q, want %q", got, want)
	}
}

func landlockKernelAnswer(t *testing.T) {
	w := landlockTestWorld(t)
	var want []string
	for _, row := range []struct {
		name, pin, record, reply string
	}{
		{"ABI 9", "ABI 9", "version ABI 9", "version 9"},
		{"ABI 8", "ABI 8", "version ABI 8", "version 8"},
		{"ABI 0", "ABI 0", "version ABI 0", "version 0"},
		{"ENOSYS", "ENOSYS", "version ENOSYS", "version -1 ENOSYS"},
		{"EOPNOTSUPP", "EOPNOTSUPP", "version EOPNOTSUPP", "version -1 EOPNOTSUPP"},
		{"EPERM", "EPERM", "version EPERM", "version -1 EPERM"},
	} {
		t.Run(row.name, func(t *testing.T) {
			if err := w.pinKernelAnswer(row.pin); err != nil {
				t.Fatal(err)
			}
			lines := landlockProbe(t, w, "version", "report")
			want = append(want, row.record)
			requireLandlockRecord(t, w, want...)
			requireLandlockReplies(t, lines, row.reply)
		})
	}
}

func landlockUnpinned(t *testing.T) {
	w := landlockTestWorld(t)
	lines := landlockProbe(t, w, "version", "report")
	ret, errno := landlockQuery(unix.LANDLOCK_CREATE_RULESET_VERSION)
	want := "version " + strconv.Itoa(ret)
	if errno != 0 {
		want += " " + unix.ErrnoName(errno)
	}
	requireLandlockReplies(t, lines, want)
	status, err := landlockThreadStatus()
	if err != nil {
		t.Fatal(err)
	}
	reports := landlockEvents(lines, "report")
	if len(reports) != 1 {
		t.Fatalf("the probe printed %d reports", len(reports))
	}
	if got := reports[0].NoNewPrivs; got != status["NoNewPrivs"] {
		t.Errorf("NoNewPrivs is %q, the test process's is %q", got, status["NoNewPrivs"])
	}
	if got := reports[0].SeccompFilters; got != status["Seccomp_filters"] {
		t.Errorf("Seccomp_filters is %q, the test process's is %q", got, status["Seccomp_filters"])
	}
	requireLandlockRecord(t, w)
}

func landlockUmaskComposed(t *testing.T) {
	w := landlockTestWorld(t)
	w.approval = &approvalWorld{location: "relative XDG_STATE_HOME"}
	if err := w.pinKernelAnswer("ABI 8"); err != nil {
		t.Fatal(err)
	}
	lines := landlockProbe(t, w, "version", "report")
	requireLandlockRecord(t, w, "version ABI 8")
	requireLandlockReplies(t, lines, "version 8")
	reports := landlockEvents(lines, "report")
	if len(reports) != 1 {
		t.Fatalf("the probe printed %d reports", len(reports))
	}
	if reports[0].Umask != "0000" {
		t.Fatalf("the probe's umask is %q, want 0000", reports[0].Umask)
	}
	requireFixtureError(t, w.runEnv(w.dir, w.env(), "/nonexistent/T-66/program"))
}

// requireFixtureError wants the step to fail, naming the fixture.
func requireFixtureError(t *testing.T, err error) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), "socket backend fixture") {
		t.Fatalf("a program that cannot be executed gave %v, want an error naming the socket backend fixture", err)
	}
}

func landlockOtherCallRefused(t *testing.T) {
	w := landlockTestWorld(t)
	if err := w.pinKernelAnswer("ABI 9"); err != nil {
		t.Fatal(err)
	}
	lines := landlockProbe(t, w, "other", "version", "report")
	requireLandlockRecord(t, w, "other EPERM,version ABI 9")
	requireLandlockReplies(t, lines, "other -1 EPERM", "version 9")
}

func landlockArrivalAction(t *testing.T) {
	w := landlockTestWorld(t)
	if err := w.pinKernelAnswer("ABI 8"); err != nil {
		t.Fatal(err)
	}
	ran := 0
	if err := w.onNextLandlockQuery(func() error {
		ran++
		return w.pinKernelAnswer("ABI 9")
	}); err != nil {
		t.Fatal(err)
	}
	lines := landlockProbe(t, w, "version", "version", "report")
	requireLandlockRecord(t, w, "version ABI 9,version ABI 9")
	requireLandlockReplies(t, lines, "version 9", "version 9")
	if ran != 1 {
		t.Fatalf("the action ran %d times, want once", ran)
	}

	failure := errors.New("take over the daemon")
	if err := w.onNextLandlockQuery(func() error { return failure }); err != nil {
		t.Fatal(err)
	}
	args := []string{landlockProbeMode, "version", "report"}
	err := w.runEnv(w.dir, w.env(), landlockProgram(t), args...)
	if err == nil || !strings.Contains(err.Error(), failure.Error()) {
		t.Fatalf("an action's error gave %v, want %v", err, failure)
	}
	requireLandlockRecord(t, w, "version ABI 9,version ABI 9", "version ABI 9")
	requireLandlockReplies(t, landlockLines(t, w.stdout), "version 9")
}

func landlockNoQuery(t *testing.T) {
	w := landlockTestWorld(t)
	if err := w.pinKernelAnswer("ABI 8"); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	lines := landlockProbe(t, w, "exit=7")
	elapsed := time.Since(start)
	requireLandlockRecord(t, w, "")
	if w.exit != 7 || len(lines) != 0 {
		t.Fatalf("the step reports exit %d and lines %v, want exit 7 and none", w.exit, lines)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("the invocation took %v, want under two seconds", elapsed)
	}
}

// landlockStarted is what one program printed, by the route that started it.
type landlockStarted struct {
	lines          []landlockProbeLine
	pid            int
	stdout, stderr string
}

// landlockStartWith starts the probe as a route would: with an extra descriptor
// at 3, standard input, its own argv[0], working directory and environment, and
// the wrapper when wrapped.
func landlockStartWith(t *testing.T, w *world, wrapped bool, dir string, env []string) landlockStarted {
	t.Helper()
	r, pw, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	defer func() { _ = pw.Close() }()
	var stdout, stderr bytes.Buffer
	cmd := &exec.Cmd{
		Path:       landlockProgram(t),
		Args:       []string{"itos-argv0", landlockProbeMode, "version", "stdin", "report"},
		Dir:        dir,
		Env:        env,
		Stdin:      strings.NewReader("standard input of the route\n"),
		Stdout:     &stdout,
		Stderr:     &stderr,
		ExtraFiles: []*os.File{r},
	}
	var run *landlockRun
	if wrapped {
		if run, err = w.superviseLandlock(cmd, landlockNoUmask); err != nil {
			t.Fatal(err)
		}
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	waitErr := cmd.Wait()
	if err := run.end(true); err != nil {
		t.Fatal(err)
	}
	if waitErr != nil {
		t.Fatalf("the probe: %v\n%s", waitErr, stderr.String())
	}
	return landlockStarted{lines: landlockLines(t, stdout.String()), pid: cmd.Process.Pid, stdout: stdout.String(), stderr: stderr.String()}
}

func landlockExecUnchanged(t *testing.T) {
	w := landlockTestWorld(t)
	if err := w.pinKernelAnswer("ABI 8"); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	env := append(w.env(), "T66_FIXTURE_PROBE=exec unchanged", "T66_EMPTY=")
	direct := landlockStartWith(t, w, false, dir, env)
	requireLandlockRecord(t, w)
	wrapped := landlockStartWith(t, w, true, dir, env)
	requireLandlockRecord(t, w, "version ABI 8")
	requireLandlockReplies(t, wrapped.lines, "version 8")

	var reports [2]landlockProbeLine
	for i, started := range []landlockStarted{direct, wrapped} {
		found := landlockEvents(started.lines, "report")
		if len(found) != 1 {
			t.Fatalf("run %d printed %d reports", i, len(found))
		}
		reports[i] = found[0]
		if reports[i].Pid != started.pid {
			t.Errorf("run %d reports pid %d, its started process is %d", i, reports[i].Pid, started.pid)
		}
		if !strings.Contains(started.stderr, "probe stderr") || started.stdout == "" {
			t.Errorf("run %d did not pass its output through: stdout %q, stderr %q", i, started.stdout, started.stderr)
		}
	}
	if reports[0].Pid == reports[1].Pid {
		t.Errorf("both runs report pid %d", reports[0].Pid)
	}
	for _, field := range []string{"Argv", "Dir", "Stdin", "Umask", "SigBlk", "SigIgn", "Fds"} {
		a, b := reflect.ValueOf(reports[0]).FieldByName(field).Interface(), reflect.ValueOf(reports[1]).FieldByName(field).Interface()
		if !reflect.DeepEqual(a, b) {
			t.Errorf("%s differs: unwrapped %v, wrapped %v", field, a, b)
		}
	}
	// Name the variables that differ, never their values: an environment may hold secrets.
	envs := [2][]string{slices.Sorted(slices.Values(reports[0].Env)), slices.Sorted(slices.Values(reports[1].Env))}
	if !slices.Equal(envs[0], envs[1]) {
		var names []string
		for _, kv := range append(slices.Clone(envs[0]), envs[1]...) {
			if slices.Contains(envs[0], kv) != slices.Contains(envs[1], kv) {
				name, _, _ := strings.Cut(kv, "=")
				names = append(names, name)
			}
		}
		t.Errorf("the environments differ in %q", slices.Compact(slices.Sorted(slices.Values(names))))
	}
	if want := []string{"itos-argv0", landlockProbeMode, "version", "stdin", "report"}; !slices.Equal(reports[1].Argv, want) {
		t.Errorf("argv is %q, want %q", reports[1].Argv, want)
	}
	if !slices.Contains(reports[1].Fds, "3:pipe") {
		t.Errorf("the route's descriptor 3 is missing from %q", reports[1].Fds)
	}
}

func landlockNoFeatureBinariesLeft(t *testing.T) {
	w := landlockTestWorld(t)
	if err := w.pinKernelAnswer("ABI 8"); err != nil {
		t.Fatal(err)
	}
	private := t.TempDir()
	exe := landlockProgram(t)
	env := setEnv(w.env(), "TMPDIR", private)
	if err := w.runEnv(w.dir, env, exe, landlockProbeMode, "version", "report"); err != nil {
		t.Fatal(err)
	}
	requireLandlockRecord(t, w, "version ABI 8")
	requireLandlockReplies(t, landlockLines(t, w.stdout), "version 8")
	if err := w.runEnv(w.dir, env, exe, landlockProbeMode, "exit=7"); err != nil {
		t.Fatal(err)
	}
	requireLandlockRecord(t, w, "version ABI 8", "")
	if w.exit != 7 {
		t.Fatalf("the probe exited %d, want 7", w.exit)
	}
	entries, err := os.ReadDir(private)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "itos-container-feature-binaries-") {
			t.Errorf("a helper or probe left %s", entry.Name())
		}
	}
}

func landlockCleanupEndsSupervision(t *testing.T) {
	w := landlockTestWorld(t)
	if err := w.pinKernelAnswer("ABI 9"); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(landlockProgram(t), landlockProbeMode, "version", "stdin", "version", "report")
	cmd.Dir, cmd.Env = w.dir, w.env()
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	// A route would end the wrapper after Wait. This row never does.
	if _, err := w.superviseLandlock(cmd, landlockNoUmask); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill() }()
	reader := bufio.NewReader(stdout)
	first, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("the first reply: %v", err)
	}
	requireLandlockReplies(t, landlockLines(t, first), "version 9")

	if err := w.closeLandlock(); err != nil {
		t.Fatal(err)
	}
	requireLandlockRecord(t, w, "version ABI 9")
	if err := stdin.Close(); err != nil {
		t.Fatal(err)
	}
	rest := make(chan string, 1)
	go func() {
		data, _ := io.ReadAll(reader)
		rest <- string(data)
	}()
	select {
	case text := <-rest:
		requireLandlockReplies(t, landlockLines(t, text), "version -1 ENOSYS")
	case <-time.After(2 * time.Second):
		t.Fatal("the second query did not fail within two seconds")
	}
	_ = cmd.Wait()
	requireLandlockRecord(t, w, "version ABI 9")
}

func landlockHelperFailure(t *testing.T) {
	w := landlockTestWorld(t)
	if err := w.pinKernelAnswer("ABI 8"); err != nil {
		t.Fatal(err)
	}
	// The helper starts inside a supervised invocation, where the kernel
	// refuses a second listener with EBUSY.
	lines := landlockProbe(t, w, "nested", "report")
	nested := landlockEvents(lines, "nested")
	if len(nested) != 1 {
		t.Fatalf("the probe printed %d nested results", len(nested))
	}
	if got := nested[0].Error; !strings.Contains(got, "socket backend fixture") || !strings.Contains(got, "busy") {
		t.Fatalf("the nested start gave the error %q, want a failure of the socket backend fixture naming the busy kernel", got)
	}
	if nested[0].Inner != "" {
		t.Errorf("the program ran and printed %d bytes", len(nested[0].Inner))
	}
	requireFixtureError(t, w.runEnv(w.dir, w.env(), "/nonexistent/T-66/program"))
}

func landlockRefusedAnswers(t *testing.T) {
	w := landlockTestWorld(t)
	for _, text := range []string{"ABI -1", "ABI +9", "ABI 09", "ABI 2147483648", "ABI x", "abi 9", "ABI 9 ", "EINVAL", ""} {
		if err := w.pinKernelAnswer(text); err == nil {
			t.Errorf("pinning %q was accepted", text)
		}
	}
	// Nothing was pinned, so the program is started directly.
	landlockProbe(t, w, "version")
	requireLandlockRecord(t, w)

	if err := w.pinKernelAnswer("ABI 2147483647"); err != nil {
		t.Fatal(err)
	}
	lines := landlockProbe(t, w, "version", "report")
	requireLandlockRecord(t, w, "version ABI 2147483647")
	requireLandlockReplies(t, lines, "version 2147483647")
}

func landlockRuntimes(t *testing.T) {
	api := newDoctorAPI("registry.example/feature@sha256:" + strings.Repeat("a", 64))
	server := httptest.NewServer(http.HandlerFunc(api.serve))
	t.Cleanup(server.Close)
	w := &world{doctor: &doctorWorld{engine: api}}
	read := func(t *testing.T, wantRuntimes string) {
		t.Helper()
		response, err := http.Get(server.URL + "/v1.56/info")
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = response.Body.Close() }()
		var info map[string]any
		if err := json.NewDecoder(response.Body).Decode(&info); err != nil {
			t.Fatal(err)
		}
		if info["DefaultRuntime"] != "runc" {
			t.Errorf("DefaultRuntime is %v, want runc", info["DefaultRuntime"])
		}
		got, present := info["Runtimes"]
		if wantRuntimes == "" {
			if present {
				t.Errorf("Runtimes is %v, want none", got)
			}
			return
		}
		var want any
		if err := json.Unmarshal([]byte(wantRuntimes), &want); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("Runtimes is %v, want %v", got, want)
		}
	}
	const runc = `"runc":{"path":"runc"}`
	t.Run("default", func(t *testing.T) { read(t, `{`+runc+`}`) })
	for _, row := range []struct{ name, runtimes string }{
		{"runc only", `{` + runc + `}`},
		{"runc and runsc", `{` + runc + `,"runsc":{"path":"/usr/local/bin/runsc"}}`},
		{"runsc permitting host sockets", `{` + runc + `,"runsc":{"path":"/usr/local/bin/runsc","runtimeArgs":["--host-uds=open"]}}`},
		{"unidentified runsc", `{` + runc + `,"runsc":{}}`},
		{"missing inventory", ""},
	} {
		t.Run(row.name, func(t *testing.T) {
			if err := w.chooseRuntimeInventory(row.name); err != nil {
				t.Fatal(err)
			}
			read(t, row.runtimes)
		})
	}
	t.Run("refused inventory", func(t *testing.T) {
		if err := w.chooseRuntimeInventory("runsc only"); err == nil {
			t.Error("an unknown inventory was accepted")
		}
		read(t, "") // the choice before it stands
	})
}
