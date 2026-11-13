// The steps. itos-container is a black box here: each scenario gets a
// scratch folder in a temporary directory, runs the binary TestFeatures
// built from this tree in it, and reads its exit code and its output.
// Nothing here imports or reads itos-container's code, so the steps judge
// it by its command line alone. Harvested from itos's (github.com/donvargax/
// itos, features/steps_test.go) and the owner's itos-template's, its domain's
// steps left behind.
package features

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cucumber/godog"
)

// One scenario's state.
type world struct {
	root     string // the itos-container checkout: where go.mod is
	bin      string // the itos-container binary under test
	dir      string // the scratch folder
	coverdir string // product coverage, outside the scenario's disposable state

	exit           int
	stdout, stderr string
	deadline       time.Duration
	timedOut       bool

	stdin               string // the text the run reads from its standard input, "" for none
	policy              *policyWorld
	manifest            *manifestWorld
	doctor              *doctorWorld
	approval            *approvalWorld
	run                 *runWorld
	session             *sessionWorld
	offline             *offlineRuntime
	sockets             *socketWorld
	anchor              *anchorWorld
	anchors             *anchorRecoveryWorld
	anchorLocalRecovery *anchorRecoveryLocal
	recovery            *recoveryNative
	recoveryLocal       *recoveryLocal
	lifecycle           *lifecycleNative
	loss                *lifecycleLoss
	overlap             *overlapFixture
	hardlinks           *hardlinkWorld
	control             *controlFixture
	egress              *egressWorld
	duringQuiet         func() // Selected independent cases inside a stable quiet-worker lifetime.
}

func initializeScenario(sc *godog.ScenarioContext, root, bin string) *world {
	w := &world{root: root, bin: bin}
	sc.Before(func(ctx context.Context, scenario *godog.Scenario) (context.Context, error) {
		if err := w.initializeCoverage(scenario); err != nil {
			return ctx, err
		}
		return ctx, w.setUp()
	})
	sc.StepContext().After(w.reportMountDiagnosticFailure)
	// The scenario's error is godog's already: returned here, it would be
	// reported twice, as the hook's and as the step's.
	sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		// Before the world is reset, whatever the row's outcome: a failing row is the
		// one whose offsets matter, and godog skips the steps after a failure.
		ctx = withSignalOffsets(ctx, w.lifecycle.signalOffsets())
		sessionErr := errors.Join(w.closeLifecycleNative(), w.closeOverlap(), w.closeRecoveryNative(), w.closeOfflineRuntime(), w.closeAnchor(), w.closeSockets(), w.closeSession(), w.closeEgressNative())
		w.closeDoctor()
		w.closePolicy()
		_ = os.RemoveAll(w.dir)
		// godog retains matched handlers (and therefore w) in its results.
		// Assertions have already materialized their diagnostics, and owned
		// cleanup must finish before releasing snapshots, output and fixtures.
		// Keep the small receiver, but none of the completed scenario's state.
		*w = world{}
		return ctx, sessionErr
	})

	// Split before anything else is expanded, so a word with a space in it
	// stays one argument. Quotes also preserve empty words for completion
	// requests.
	sc.Step(`^itos-container runs with "([^"]*)"$`, func(args string) error {
		return w.runWith(w.env(), args)
	})
	sc.Step(`^itos-container runs with "([^"]*)" and the environment "([^"]*)"$`, func(args, vars string) error {
		return w.runWith(w.withEnv(vars), args)
	})

	sc.Step(`^it exits with code (\d+)$`, w.exitsWith)
	sc.Step(`^its standard output lists "([^"]*)"$`, func(text string) error {
		if !slices.Contains(outputLines(w.stdout), text) {
			return fmt.Errorf("standard output does not list %q\n%s", text, w.report())
		}
		return nil
	})
	sc.Step(`^its standard output does not list "([^"]*)"$`, func(text string) error {
		if slices.Contains(outputLines(w.stdout), text) {
			return fmt.Errorf("standard output lists %q\n%s", text, w.report())
		}
		return nil
	})
	sc.Step(`^the last line of its standard output is "([^"]*)"$`, func(text string) error {
		lines := outputLines(w.stdout)
		last := strings.TrimSuffix(lines[len(lines)-1], "\r")
		if last != text {
			return fmt.Errorf("the last line of standard output is %q, not %q\n%s", last, text, w.report())
		}
		return nil
	})
	sc.Step(`^its standard output says "([^"]*)"$`, func(text string) error {
		if !strings.Contains(w.stdout, text) {
			return fmt.Errorf("standard output does not say %q\n%s", text, w.report())
		}
		return nil
	})
	sc.Step(`^its standard output does not say "([^"]*)"$`, func(text string) error {
		if strings.Contains(w.stdout, text) {
			return fmt.Errorf("standard output says %q\n%s", text, w.report())
		}
		return nil
	})
	sc.Step(`^its error output says "([^"]*)"$`, func(text string) error {
		if !strings.Contains(w.stderr, text) {
			return fmt.Errorf("error output does not say %q\n%s", text, w.report())
		}
		return nil
	})
	sc.Step(`^its error output does not say "([^"]*)"$`, func(text string) error {
		if strings.Contains(w.stderr, text) {
			return fmt.Errorf("error output says %q\n%s", text, w.report())
		}
		return nil
	})
	sc.Step(`^the first line of its standard output is "([^"]*)" and the stamped version$`, func(name string) error {
		return w.firstLineIs(name + " " + stampedVersion)
	})
	sc.Step(`^the second line of its standard output is "([^"]*)" and the stamped commit$`, func(word string) error {
		return w.secondLineIs(word + " " + stampedCommit)
	})

	// The --json object, as a script reads it.
	sc.Step(`^its JSON output names the problem "([^"]*)"$`, w.jsonNamesProblem)
	sc.Step(`^its JSON output gives file "([^"]*)", (\d+) lines and (\d+) words$`, w.jsonGivesCounts)
	sc.Step(`^its JSON output gives each problem a rule, a message and a fix$`, w.jsonProblemsComplete)

	// The fixtures a scenario asks for. godog takes one parameter per
	// capturing group and none variadic, so the second text is its own
	// group, empty where the scenario names one.
	sc.Step(`^the file "([^"]*)" contains "([^"]*)"(?:, "([^"]*)")?$`, func(name, first, second string) error {
		texts := []string{first}
		if second != "" {
			texts = append(texts, second)
		}
		return w.fileContains(name, texts...)
	})
	sc.Step(`^the file "([^"]*)" holds a line of (\d+) characters$`, w.fileHoldsLine)
	sc.Step(`^standard input holds "([^"]*)"$`, func(text string) error {
		w.stdin = text
		return nil
	})
	sc.Step(`^an empty folder "([^"]*)"$`, w.emptyFolder)
	w.policySteps(sc)
	w.manifestSteps(sc)
	w.doctorSteps(sc)
	w.runSteps(sc)
	w.approvalSteps(sc)
	w.sessionSteps(sc)
	w.offlineRuntimeSteps(sc)
	w.recoveryNativeSteps(sc)
	w.recoveryLocalSteps(sc)
	w.offlineIsolationSteps(sc)
	w.anchorSteps(sc)
	w.anchorRecoverySteps(sc)
	w.socketSteps(sc)
	w.lifecycleNativeSteps(sc)
	w.lifecycleLossSteps(sc)
	w.lifecycleLocalSteps(sc)
	w.overlapSteps(sc)
	w.hardlinkSteps(sc)
	w.controlStateSteps(sc)
	w.runExitSteps(sc)
	w.egressSteps(sc)
	return w
}

// setUp makes the scenario's scratch folder.
func (w *world) setUp() error {
	var err error
	if w.dir, err = os.MkdirTemp("", "itos-container-features-"); err != nil {
		return err
	}
	return nil
}

// The folder holding go.mod, above the working directory go test gives.
func moduleRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("no go.mod above the working directory")
		}
		dir = parent
	}
}

// The environment every command runs in: the caller's, less what would make
// git, itos or itos-container read anything but the scratch folder (a
// hook's GIT_DIR, the caller's ITOS_CONTAINER_ settings, CI's), with no
// global or system git config and a fixed identity, and the caller's PATH
// without its claude, its itos, its itos extensions or a git that is an itos
// (callerPath).
func (w *world) env() []string {
	var env []string
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(name, "GIT_") || strings.HasPrefix(name, "ITOS_") ||
			strings.HasPrefix(name, "ITOS_CONTAINER_") || strings.HasPrefix(name, "GITHUB_") ||
			strings.HasPrefix(name, "GH_") || name == "CI" || strings.EqualFold(name, "PATH") {
			continue
		}
		env = append(env, kv)
	}
	env = append(env,
		"GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=itos-container features",
		"GIT_AUTHOR_EMAIL=features@localhost",
		"GIT_COMMITTER_NAME=itos-container features",
		"GIT_COMMITTER_EMAIL=features@localhost",
		"PATH="+callerPath(),
	)
	return w.coverageEnv(w.doctorEnv(w.manifestEnv(w.policyEnv(env))))
}

// withEnv is the scenarios' environment with each NAME=value of vars, a
// semicolon-separated list, set as given.
func (w *world) withEnv(vars string) []string {
	env := w.env()
	for _, one := range strings.Split(vars, ";") {
		if name, value, ok := strings.Cut(one, "="); ok {
			env = setEnv(env, name, value)
		}
	}
	return env
}

// The programs no scenario reaches on the caller's PATH: its claude, its itos
// and its itos extensions (itos-*). An itos installed on the machine would
// run its own policy where a scenario meant only itos-container (itos's
// T-087), and itos runs and lists every extension it finds, so one installed
// on the machine would change what a scenario sees (itos's T-080).
var hiddenAlways = []string{"claude", "itos", "itos-*"}

var (
	hiddenPathsMu sync.Mutex
	hiddenPaths   = map[string]string{}
	callerPathDir string
)

// callerPath is the caller's PATH with none of hiddenAlways on it, so no
// scenario reaches the Claude Code or the itos of the machine it runs on:
// each folder holding one is replaced by a folder of links to everything else
// in it, or left out where links cannot be made (windows without the right to
// make them).
func callerPath() string { return pathHiding(hiddenAlways...) }

// pathHiding is the caller's PATH with none of the programs named on it, nor
// a git that is an itos, made once a run for each set of names. A git shim of
// the caller's (itos git-shim install) would run the global itos for a
// scenario's git command; it is told from the real git by the rule itos
// itself goes by (isItos, git_test.go), so every git a scenario's commands
// start is the real one, as the steps' own are (gitBin, itos's T-104).
func pathHiding(names ...string) string {
	hiddenPathsMu.Lock()
	defer hiddenPathsMu.Unlock()
	key := strings.Join(names, " ")
	if text, ok := hiddenPaths[key]; ok {
		return text
	}
	var folders []string
	for i, folder := range filepath.SplitList(os.Getenv("PATH")) {
		hidden := func(e os.DirEntry) bool {
			return isOneOf(e, names) || isOneOf(e, []string{"git"}) && isItos(filepath.Join(folder, e.Name()))
		}
		entries, err := os.ReadDir(folder)
		if err != nil || !slices.ContainsFunc(entries, hidden) {
			folders = append(folders, folder)
			continue
		}
		if callerPathDir == "" {
			if callerPathDir, err = os.MkdirTemp("", "itos-container-features-path-"); err != nil {
				continue
			}
		}
		links := filepath.Join(callerPathDir, strconv.Itoa(len(hiddenPaths)), strconv.Itoa(i))
		if linkAllBut(folder, links, entries, hidden) == nil {
			folders = append(folders, links)
		}
	}
	text := strings.Join(folders, string(os.PathListSeparator))
	hiddenPaths[key] = text
	return text
}

// removeCallerPath removes the folders callerPath and pathHiding made.
func removeCallerPath() {
	if callerPathDir != "" {
		_ = os.RemoveAll(callerPathDir)
	}
}

// isOneOf is whether a folder's entry is a program of the names the PATH
// would find: the name, or on windows the name with any extension, in any
// case. A name ending in * is a prefix, so itos-* is every itos extension.
func isOneOf(e os.DirEntry, names []string) bool {
	name := e.Name()
	same := func(a, b string) bool { return a == b }
	starts := strings.HasPrefix
	if runtime.GOOS == "windows" {
		name = strings.TrimSuffix(name, filepath.Ext(name))
		same = strings.EqualFold
		starts = func(s, prefix string) bool {
			return len(s) >= len(prefix) && strings.EqualFold(s[:len(prefix)], prefix)
		}
	}
	return slices.ContainsFunc(names, func(n string) bool {
		if prefix, ok := strings.CutSuffix(n, "*"); ok {
			return starts(name, prefix)
		}
		return same(name, n)
	})
}

// linkAllBut makes links a folder of links to every entry of folder but the
// hidden ones.
func linkAllBut(folder, links string, entries []os.DirEntry, hidden func(os.DirEntry) bool) error {
	if err := os.MkdirAll(links, 0o755); err != nil {
		return err
	}
	for _, e := range entries {
		if hidden(e) {
			continue
		}
		if err := os.Symlink(filepath.Join(folder, e.Name()), filepath.Join(links, e.Name())); err != nil {
			return err
		}
	}
	return nil
}

// When steps.

// runWith runs itos-container in the scratch folder with args, split as a
// scenario wrote them, in the environment env.
func (w *world) runWith(env []string, args string) error {
	fields, err := commandWords(args)
	if err != nil {
		return err
	}
	if w.run != nil && w.doctor != nil {
		return w.runPreflightProcess(env, fields...)
	}
	if w.doctor != nil {
		return w.runDoctor(env, fields...)
	}
	dir := w.dir
	if w.policy != nil {
		dir = w.policy.cwd
	}
	if w.manifest != nil {
		dir = w.manifest.checkout
	}
	return w.runEnv(dir, env, w.bin, fields...)
}

// commandWords splits a scenario's command line without involving a shell.
// Quotes group words and preserve an explicitly empty argument, as the
// completion protocol needs for a new token. Backslashes remain literal, so
// Windows paths survive the harness.
func commandWords(line string) ([]string, error) {
	var words []string
	var word strings.Builder
	var quote rune
	started := false
	for _, r := range line {
		if quote != 0 {
			if r == quote {
				quote = 0
				continue
			}
			word.WriteRune(r)
			started = true
			continue
		}
		switch r {
		case '\'', '"':
			quote = r
			started = true
		case ' ', '\t', '\n', '\r':
			if started {
				words = append(words, word.String())
				word.Reset()
				started = false
			}
		default:
			word.WriteRune(r)
			started = true
		}
	}
	if quote != 0 {
		return nil, fmt.Errorf("unclosed quote in scenario command %q", line)
	}
	if started {
		words = append(words, word.String())
	}
	return words, nil
}

// setEnv is env with the variable name, in any case, set to value.
func setEnv(env []string, name, value string) []string {
	var out []string
	for _, kv := range env {
		if n, _, _ := strings.Cut(kv, "="); !strings.EqualFold(n, name) {
			out = append(out, kv)
		}
	}
	return append(out, name+"="+value)
}

// runEnv runs program in the folder dir in the environment env, its exit
// code and output what the Then steps read.
func (w *world) runEnv(dir string, env []string, program string, args ...string) error {
	ctx := context.Background()
	if w.deadline > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, w.deadline)
		defer cancel()
	}
	cmd := exec.CommandContext(ctx, program, args...)
	if w.approval != nil && w.approval.location != "" {
		// This trusted test wrapper changes only the child umask. Positional arguments
		// remain literal, and no checkout shell or helper participates.
		words := append([]string{"-c", `umask 0000; exec "$@"`, "slice-4-umask", program}, args...)
		cmd = exec.CommandContext(ctx, "/bin/sh", words...)
	}

	cmd.Dir = dir
	cmd.Env = env
	if w.stdin != "" {
		cmd.Stdin = strings.NewReader(w.stdin)
	}
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := w.startLocalCommand(cmd)
	if err == nil {
		err = cmd.Wait()
	}
	unlock := w.lockLocalAPI()
	defer unlock()
	w.timedOut = errors.Is(ctx.Err(), context.DeadlineExceeded)
	w.stdout, w.stderr = stdout.String(), stderr.String()
	var exit *exec.ExitError
	switch {
	case err == nil:
		w.exit = 0
	case errors.As(err, &exit):
		w.exit = exit.ExitCode()
	default:
		return fmt.Errorf("running %s: %w", program, err)
	}
	return nil
}

// A subprocess/HTTP exchange is not Go memory synchronization. Guard child
// startup and result publication with the same locks as the fixture handlers.
// Session handlers may call doctor handlers, so acquire their locks in that order.
func (w *world) lockLocalAPI() func() {
	if w.session != nil && w.session.api != nil {
		w.session.api.mu.Lock()
	}
	if w.doctor != nil && w.doctor.engine != nil {
		w.doctor.engine.mu.Lock()
	}
	return func() {
		if w.doctor != nil && w.doctor.engine != nil {
			w.doctor.engine.mu.Unlock()
		}
		if w.session != nil && w.session.api != nil {
			w.session.api.mu.Unlock()
		}
	}
}

func (w *world) startLocalCommand(cmd *exec.Cmd) error {
	unlock := w.lockLocalAPI()
	defer unlock()
	// Release before waiting: the running child must be able to contact the API.
	return cmd.Start()
}

// Fixture steps.

// fileContains writes name in the scratch folder, one line per text the
// scenario named, each ending in the newline a text file's last line has.
func (w *world) fileContains(name string, texts ...string) error {
	if len(texts) == 0 || texts[0] == "" {
		return fmt.Errorf("the file %q is given no text to contain", name)
	}
	body := strings.Join(texts, "\n") + "\n"
	return os.WriteFile(filepath.Join(w.dir, name), []byte(body), 0o600)
}

// fileHoldsLine writes name in the scratch folder holding one line of count
// characters, long enough to beat a token limit a reader might have, and no
// other line.
func (w *world) fileHoldsLine(name string, count int) error {
	body := strings.Repeat("x", count) + "\n"
	return os.WriteFile(filepath.Join(w.dir, name), []byte(body), 0o600)
}

// emptyFolder makes a folder in the scratch directory.
func (w *world) emptyFolder(name string) error {
	return os.Mkdir(filepath.Join(w.dir, name), 0o755)
}

// Then steps.

func (w *world) report() string {
	return fmt.Sprintf("exit %d\n--- stdout\n%s--- stderr\n%s", w.exit, w.stdout, w.stderr)
}

func outputLines(output string) []string {
	return strings.Split(strings.TrimSuffix(output, "\n"), "\n")
}

func (w *world) exitsWith(code int) error {
	if w.exit != code {
		return fmt.Errorf("itos-container exited %d, not %d\n%s", w.exit, code, w.report())
	}
	return nil
}

// secondLineIs is whether standard output's second line is text, a missing
// line read as an empty one and a line ending in \r\n as one in \n.
func (w *world) secondLineIs(text string) error {
	_, rest, _ := strings.Cut(w.stdout, "\n")
	second, _, _ := strings.Cut(rest, "\n")
	if second = strings.TrimSuffix(second, "\r"); second != text {
		return fmt.Errorf("the second line of standard output is %q, not %q\n%s", second, text, w.report())
	}
	return nil
}

// firstLineIs is whether standard output's first line is text, a line ending
// in \r\n read as one in \n.
func (w *world) firstLineIs(text string) error {
	first, _, _ := strings.Cut(w.stdout, "\n")
	if first = strings.TrimSuffix(first, "\r"); first != text {
		return fmt.Errorf("the first line of standard output is %q, not %q\n%s", first, text, w.report())
	}
	return nil
}

// The JSON steps read standard output as the one object --json prints.

// json is the object standard output holds, one line of JSON as --json
// prints it.
func (w *world) json() (map[string]any, error) {
	var object map[string]any
	if err := json.Unmarshal([]byte(w.stdout), &object); err != nil {
		return nil, fmt.Errorf("standard output is not one JSON object: %v\n%s", err, w.report())
	}
	return object, nil
}

// jsonNamesProblem is whether the object's problems list one with that rule.
func (w *world) jsonNamesProblem(rule string) error {
	object, err := w.json()
	if err != nil {
		return err
	}
	problems, _ := object["problems"].([]any)
	for _, one := range problems {
		problem, _ := one.(map[string]any)
		if problem["rule"] == rule {
			return nil
		}
	}
	return fmt.Errorf("no problem of the JSON output has the rule %q\n%s", rule, w.report())
}

// jsonGivesCounts is whether the success object's file is file, its lines
// the count and its words the count.
func (w *world) jsonGivesCounts(file string, lines, words int) error {
	object, err := w.json()
	if err != nil {
		return err
	}
	if object["ok"] != true {
		return fmt.Errorf("the JSON output's ok is %v, not true\n%s", object["ok"], w.report())
	}
	for _, want := range []struct {
		key  string
		got  any
		want any
	}{
		{"file", object["file"], file},
		{"lines", numberOf(object["lines"]), lines},
		{"words", numberOf(object["words"]), words},
	} {
		if want.got != want.want {
			return fmt.Errorf("the JSON output's %s is %v, not %v\n%s", want.key, want.got, want.want, w.report())
		}
	}
	return nil
}

// jsonProblemsComplete is whether the object's problems all carry a rule, a
// message and a fix, and there is one.
func (w *world) jsonProblemsComplete() error {
	object, err := w.json()
	if err != nil {
		return err
	}
	problems, _ := object["problems"].([]any)
	if len(problems) == 0 {
		return fmt.Errorf("the JSON output lists no problem\n%s", w.report())
	}
	for i, one := range problems {
		problem, _ := one.(map[string]any)
		for _, key := range []string{"rule", "message", "fix"} {
			if text, _ := problem[key].(string); text == "" {
				return fmt.Errorf("problem %d of the JSON output has no %s\n%s", i+1, key, w.report())
			}
		}
	}
	return nil
}

// numberOf is the number a decoded JSON value holds, a float64 as encoding/
// json decodes every number into.
func numberOf(value any) int {
	number, _ := value.(float64)
	return int(number)
}
