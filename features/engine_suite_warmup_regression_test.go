package features

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cucumber/godog"
)

func testWarmupSelection(t *testing.T) {
	var suite engineSuiteWarmup
	var output bytes.Buffer
	settings := engineWarmupSettings{guard: func() error { t.Error("disabled selection reached native guard"); return errors.New("guard") }, fixture: func(*world) error { t.Error("disabled selection prepared fixture"); return nil }}
	for _, selection := range []scenarioSelection{{count: 1}, {engine: true}, {engine: true, count: 1, skip: "empty"}} {
		if err := suite.before(selection, "missing root", &output, settings); err != nil {
			t.Fatal(err)
		}
	}
	if output.Len() != 0 {
		t.Fatal("disabled selection attempted warm-up", output.String())
	}
	// Neither a synthetically selected engine row nor a bogus fixture input may
	// bypass the independently guarded native factory in an ordinary process.
	t.Setenv("ITOS_CONTAINER_ENGINE_TESTS", "")
	t.Setenv("ITOS_CONTAINER_ENGINE_FIXTURE_IMAGE", "must not be read")
	err := suite.before(scenarioSelection{engine: true, count: 1}, "missing root", &output, nativeEngineWarmup())
	if err == nil || !strings.Contains(err.Error(), "guard failed") || strings.Contains(err.Error(), "build failed") {
		t.Fatal("native factory reached preparation before guard", err)
	}
	first := output.String()
	if next := suite.before(scenarioSelection{engine: true, count: 1}, "missing root", &output, settings); next != err || output.String() != first {
		t.Fatal("failed attempt was repeated or replaced", next)
	}
}

func testWarmupIntervals(t *testing.T) {
	origin := time.Now()
	create := offlineRequest{Method: "POST", Path: "/containers/create", At: origin.Add(7 * time.Nanosecond), Forwarded: origin.Add(11 * time.Nanosecond), Answered: origin.Add(29 * time.Nanosecond), Status: 201}
	got, err := warmupInterval(origin, []offlineRequest{{Method: "GET", Path: "/version"}, create})
	if err != nil || got != "arrival_ns=7 forwarded_ns=11 answer_ns=29 forward_to_answer_ns=18" {
		t.Fatal("timing differs from create exchange", got, err)
	}
	for _, kind := range []string{"absent", "duplicate", "no origin", "no arrival", "no forwarded", "no answer", "arrival before origin", "forward before arrival", "answer before forward", "not created", "cancelled"} {
		t.Run(kind, func(t *testing.T) {
			r, start := create, origin
			switch kind {
			case "no origin":
				start = time.Time{}
			case "no arrival":
				r.At = time.Time{}
			case "no forwarded":
				r.Forwarded = time.Time{}
			case "no answer":
				r.Answered = time.Time{}
			case "arrival before origin":
				r.At = origin.Add(-time.Nanosecond)
			case "forward before arrival":
				r.Forwarded = r.At.Add(-time.Nanosecond)
			case "answer before forward":
				r.Answered = r.Forwarded.Add(-time.Nanosecond)
			case "not created":
				r.Status = 500
			case "cancelled":
				r.Cancelled = true
			}
			rows := []offlineRequest{r}
			if kind == "absent" {
				rows = nil
			}
			if kind == "duplicate" {
				rows = append(rows, r)
			}
			got, err := warmupInterval(start, rows)
			if err == nil {
				t.Fatal("invalid create timing passed", got)
			}
			if strings.HasPrefix(kind, "no ") || kind == "absent" || kind == "duplicate" {
				if !strings.Contains(got, "missing") {
					t.Fatal("missing timing was fabricated", got)
				}
			}
		})
	}
}

func assertWarmupTiming(t *testing.T, text string, requests []offlineRequest) {
	t.Helper()
	var arrival, forwarded, answer, interval int64
	var outcome string
	n, err := fmt.Sscanf(text, "engine suite warm-up timing: arrival_ns=%d forwarded_ns=%d answer_ns=%d forward_to_answer_ns=%d outcome=%s", &arrival, &forwarded, &answer, &interval, &outcome)
	if n != 5 || err != nil || outcome != "passed" || arrival < 0 || forwarded < arrival || answer < forwarded || interval != answer-forwarded || strings.Count(text, "\n") != 1 {
		t.Fatalf("invalid timing line %q: %v", text, err)
	}
	var creates []offlineRequest
	for _, r := range requests {
		if r.Method == "POST" && r.Path == "/containers/create" {
			creates = append(creates, r)
		}
	}
	if len(creates) != 1 {
		t.Fatal("expected exactly one journal create", len(creates))
	}
	r := creates[0]
	if forwarded-arrival != r.Forwarded.Sub(r.At).Nanoseconds() || interval != r.Answered.Sub(r.Forwarded).Nanoseconds() || r.Status != 201 || r.Cancelled {
		t.Fatal("printed intervals differ from recorder journal")
	}
}

func testWarmupEntryWiring(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "features_test.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	// The dynamic scheduling tests below exercise this call. Keep the public
	// entry point attached to it; an unused working helper is not a suite phase.
	var entry *ast.FuncDecl
	for _, decl := range file.Decls {
		if f, ok := decl.(*ast.FuncDecl); ok && f.Name.Name == "TestFeatures" {
			entry = f
		}
	}
	if entry == nil {
		t.Fatal("missing TestFeatures")
	}
	calls := map[string]int{}
	ast.Inspect(entry, func(n ast.Node) bool {
		if c, ok := n.(*ast.CallExpr); ok {
			if name, ok := c.Fun.(*ast.Ident); ok {
				calls[name.Name]++
			}
		}
		return true
	})
	if calls["runPreparedFeatures"] != 1 || calls["nativeEngineWarmup"] != 1 || calls["runFeatureScenarios"] != 0 {
		t.Fatal("TestFeatures bypasses guarded preparation", calls)
	}
}

func testWarmupScheduling(t *testing.T, root string) {
	dir := writeFeatureFixture(t, engineScheduleFixture)
	selection, err := selectScenarios("", dir, featureEnvironment{engine: "1", ci: "true", actions: "true", os: "linux", arch: "amd64"})
	if err != nil {
		t.Fatal(err)
	}
	selection.started = time.Now()
	var suite engineSuiteWarmup
	var output bytes.Buffer
	var warmed *world
	var engine *sessionAPI
	launches, setups, rows := 0, 0, 0
	settings := engineWarmupSettings{limit: 45 * time.Second, fixture: func(w *world) error {
		launches++
		warmed = w
		var err error
		engine, err = warmupLocalFixture(w)
		return err
	}}
	initialize := func(sc *godog.ScenarioContext, duringQuiet func()) {
		sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
			setups++
			if warmed == nil {
				return ctx, errors.New("row setup preceded warm-up")
			}
			if _, err := os.Stat(warmed.dir); !os.IsNotExist(err) {
				return ctx, errors.New("row setup preceded warm-up teardown")
			}
			engine.mu.Lock()
			defer engine.mu.Unlock()
			if len(engine.created) != 1 || len(engine.local) != 0 || len(engine.volumes) != 0 {
				return ctx, errors.New("warm-up resources entered a row snapshot")
			}
			return ctx, suite.before(selection, root, &output, settings)
		})
		sc.Step(`^observe escalation (\d+)$`, func(row int) error {
			rows++
			if row == 2 {
				return errors.New("intentional synthetic row failure")
			}
			return nil
		})
		sc.Step(`^observe quiet interval$`, func() { rows++; duringQuiet(); duringQuiet() })
		sc.Step(`^observe other case$`, func() { rows++ })
	}
	for range 2 {
		err = runPreparedFeatures(selection, root, godog.Options{Paths: []string{dir}, Format: "progress", Output: &output}, initialize, &suite, settings)
		if err == nil || err.Error() != "a scenario failed" {
			t.Fatal("synthetic row failure lost", err)
		}
	}
	if launches != 1 || setups != 2*selection.count || rows != setups || strings.Count(output.String(), "engine suite warm-up timing:") != 1 {
		t.Fatalf("attempt or scenario counts changed: launches=%d setups=%d rows=%d\n%s", launches, setups, rows, output.String())
	}
	want := fmt.Sprintf(`"selected":%d,"completed":%d`, selection.count, selection.count)
	if strings.Count(output.String(), want) != 2 || !strings.Contains(output.String(), "intentional synthetic row failure") {
		t.Fatal("suite timing or original row failure lost", output.String())
	}
	var failed engineSuiteWarmup
	settings.fixture = func(*world) error { return errors.New("preparation control") }
	before := setups
	if err := runPreparedFeatures(selection, root, godog.Options{Paths: []string{dir}, Output: &output}, initialize, &failed, settings); err == nil || !strings.Contains(err.Error(), "preparation failed") || setups != before {
		t.Fatal("failed warm-up entered scenario runner", err)
	}
}

type warmupBrokenOutput struct{}

func (warmupBrokenOutput) Write([]byte) (int, error) {
	return 0, errors.New("unwritable timing output")
}

type warmupShortOutput struct{}

func (warmupShortOutput) Write(p []byte) (int, error) { return len(p) - 1, nil }

func testWarmupFailures(t *testing.T, root string) {
	for _, kind := range []string{"build", "partial preparation", "nonzero", "hanging child", "late create", "recorder", "cleanup", "cleanup inspection", "nonzero and cleanup", "timing output", "short timing output"} {
		t.Run(kind, func(t *testing.T) {
			var suite engineSuiteWarmup
			var output bytes.Buffer
			var w *world
			var engine *sessionAPI
			var reached atomic.Bool
			settings := engineWarmupSettings{limit: 45 * time.Second, fixture: func(candidate *world) error {
				w = candidate
				var err error
				engine, err = warmupLocalFixture(w, func(a *sessionAPI, out http.ResponseWriter, r *http.Request) bool {
					path := doctorRequestPath(r.URL.Path)
					if kind == "cleanup inspection" && r.Method == "GET" && strings.HasPrefix(path, "/containers/") {
						select {
						case <-w.offline.api.stopped:
							http.Error(out, "inspection unavailable", http.StatusServiceUnavailable)
							return true
						default:
						}
					}
					if kind == "hanging child" && strings.HasSuffix(path, "/wait") {
						reached.Store(true)
						out.Header().Set("Content-Type", "application/json")
						out.WriteHeader(200)
						out.(http.Flusher).Flush()
						<-r.Context().Done()
						return true
					}
					if path == "/containers/create" && kind == "late create" {
						reached.Store(true)
						// The recorder announces the controller's joined exit. Only
						// then does the engine answer its detached forwarded create.
						select {
						case <-w.offline.api.stopped:
						case <-r.Context().Done():
							return true
						}
					}
					if path == "/containers/create" && kind == "recorder" {
						w.offline.api.mu.Lock()
						w.offline.api.problem = errors.New("synthetic recorder failure")
						w.offline.api.mu.Unlock()
					}
					return false
				})
				if err != nil {
					return err
				}
				engine.w.session.launch = &offlineLaunchWorld{}
				if strings.Contains(kind, "nonzero") {
					engine.w.session.launch.child = 7
				}
				if kind == "cleanup" || kind == "nonzero and cleanup" {
					engine.w.session.anchorFault = "worker removal fails and full ID remains"
				}
				// An unrelated object must survive both successful and failed
				// cleanup. No fixture cleanup may prune this endpoint.
				engine.local["unrelated"] = map[string]any{"Name": "/unrelated"}
				if kind == "partial preparation" {
					return errors.New("partial preparation control")
				}
				return nil
			}}
			if kind == "hanging child" || kind == "late create" {
				settings.limit = 2 * time.Second
			}
			source := root
			if kind == "build" {
				source = t.TempDir()
			}
			var writer io.Writer = &output
			if kind == "timing output" {
				writer = warmupBrokenOutput{}
			}
			if kind == "short timing output" {
				writer = warmupShortOutput{}
			}
			err := suite.before(scenarioSelection{engine: true, count: 1}, source, writer, settings)
			if err == nil || !strings.HasPrefix(err.Error(), "engine suite warm-up:") {
				t.Fatal("failure control passed", err)
			}
			stage := "launch"
			switch kind {
			case "build":
				stage = "build"
			case "partial preparation":
				stage = "preparation"
			case "recorder":
				stage = "recorder"
			case "cleanup", "cleanup inspection":
				stage = "cleanup"
			case "timing output", "short timing output":
				stage = "timing output"
			}
			if !strings.Contains(err.Error(), stage+" failed") {
				t.Fatal("lost failed stage", err)
			}
			if kind == "nonzero and cleanup" && !strings.Contains(err.Error(), "cleanup failed") {
				t.Fatal("cleanup overwrote launch failure", err)
			}
			if kind == "cleanup inspection" && strings.Contains(err.Error(), "launch failed") {
				t.Fatal("cleanup control did not follow a successful product launch", err)
			}
			if kind == "short timing output" && !errors.Is(err, io.ErrShortWrite) {
				t.Fatal("short write cause lost", err)
			}
			if !strings.Contains(kind, "timing output") && (strings.Count(output.String(), "\n") != 1 || !strings.HasSuffix(output.String(), "outcome=failed\n")) {
				t.Fatal("failed attempt lost its one timing line", output.String())
			}
			if kind == "hanging child" || kind == "late create" {
				if !reached.Load() {
					t.Fatal("watchdog fired before the intended blocking operation")
				}
			}
			if w != nil {
				if _, err := os.Stat(w.dir); !os.IsNotExist(err) {
					t.Fatal("failed attempt retained its world", err)
				}
				engine.mu.Lock()
				defer engine.mu.Unlock()
				wantWorkers := 1
				if kind == "cleanup" || kind == "nonzero and cleanup" {
					wantWorkers++
				}
				if engine.local["unrelated"] == nil || len(engine.local) != wantWorkers {
					t.Fatal("cleanup lost an unrelated object or leaked an owned worker", len(engine.local))
				}
				if !strings.Contains(kind, "cleanup") && len(engine.volumes) != 0 {
					t.Fatal("cleanup leaked its anchor")
				}
				if kind == "late create" {
					if len(engine.created) != 1 {
						t.Fatal("late create never completed")
					}
					q := createEntry(t, w.offline.api)
					if !q.Cancelled || q.Answered.IsZero() {
						t.Fatal("late answer was lost")
					}
				}
			}
			prior := output.String()
			if next := suite.before(scenarioSelection{engine: true, count: 1}, root, &output, nativeEngineWarmup()); next != err || output.String() != prior {
				t.Fatal("failed attempt retried", next)
			}
		})
	}
}

func testWarmupCoverage(t *testing.T, root string) {
	for _, mode := range []string{"plain", "aggregate", "per ID", "both"} {
		t.Run(mode, func(t *testing.T) {
			aggregate, perID := t.TempDir(), t.TempDir()
			t.Setenv("GOCOVERDIR", "")
			t.Setenv("ITOS_CC_TEST_COVERDIR", "")
			if mode == "aggregate" || mode == "both" {
				t.Setenv("GOCOVERDIR", aggregate)
			}
			if mode == "per ID" || mode == "both" {
				t.Setenv("ITOS_CC_TEST_COVERDIR", perID)
			}
			var suite engineSuiteWarmup
			observed := false
			settings := engineWarmupSettings{limit: 45 * time.Second, fixture: func(w *world) error {
				_, err := warmupLocalFixture(w)
				if err != nil {
					return err
				}
				// Exercise the actual compiled warm-up assembly, not its source.
				ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
				defer cancel()
				cmd := exec.CommandContext(ctx, w.bin, "-test.run=^TestEngineFixtureBudgetContract$", "-test.v")
				cmd.Env = setEnv(sessionChildEnvironment(os.Environ()), "GOCOVERDIR", w.coverdir)
				if w.coverdir != "" {
					cmd.Args = append(cmd.Args, "-test.gocoverdir="+w.coverdir)
				}
				data, err := cmd.CombinedOutput()
				if err != nil || !strings.Contains(string(data), "compiled budgets: call=10s startup=2m0s grace=10s") || !strings.Contains(string(data), "--- PASS: TestEngineFixtureBudgetContract") {
					t.Errorf("warm-up changed production budgets: %v\n%s", err, data)
				}
				prior, _ := filepath.Glob(filepath.Join(w.coverdir, "covcounters.*"))
				w.doctor.cleanup = append(w.doctor.cleanup, func() {
					observed = true
					if mode == "plain" {
						if w.coverdir != "" || strings.Contains(w.bin, "-cover") {
							t.Error("plain launch became covered")
						}
						return
					}
					if w.coverdir == "" || strings.HasPrefix(w.coverdir, aggregate) || strings.HasPrefix(w.coverdir, perID) || !strings.Contains(w.bin, "-cover") {
						t.Error("coverage was not privately instrumented")
					}
					files, err := filepath.Glob(filepath.Join(w.coverdir, "covcounters.*"))
					if err != nil || len(files) <= len(prior) {
						t.Error("warm-up produced no private execution counters", err)
					}
				})
				return nil
			}}
			if err := suite.before(scenarioSelection{engine: true, count: 1}, root, io.Discard, settings); err != nil {
				t.Fatal(err)
			}
			if !observed {
				t.Fatal("coverage observer did not run")
			}
			for _, dir := range []string{aggregate, perID} {
				files, err := os.ReadDir(dir)
				if err != nil || len(files) != 0 {
					t.Fatal("warm-up credited collected coverage", files, err)
				}
			}
			following := &world{}
			if following.coverageDir() != os.Getenv("GOCOVERDIR") {
				t.Fatal("warm-up changed a following world's coverage destination")
			}
			if (mode == "per ID" || mode == "both") && os.Getenv("ITOS_CC_TEST_COVERDIR") != perID {
				t.Fatal("warm-up changed the per-ID input")
			}
		})
	}
}
