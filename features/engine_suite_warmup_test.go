package features

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// The factory substitutes only fixture preparation. The launch, recorder and
// teardown are shared with TestFeatures; ordinary tests never enable Docker.
type engineWarmupSettings struct {
	guard   func() error
	fixture func(*world) error
	limit   time.Duration
}

type engineSuiteWarmup struct {
	once sync.Once
	err  error
}

var processEngineWarmup engineSuiteWarmup

func nativeEngineWarmup() engineWarmupSettings {
	return engineWarmupSettings{guard: doctorRealGuard, fixture: func(w *world) error {
		return w.sessionFixture(true, "0022")
	}, limit: 45 * time.Second}
}

func (s *engineSuiteWarmup) before(selection scenarioSelection, root string, output io.Writer, settings engineWarmupSettings) error {
	if !selection.engine || selection.count == 0 || selection.skip != "" {
		return nil
	}
	s.once.Do(func() { s.err = performEngineWarmup(root, output, settings) })
	return s.err
}

// Own every allocation from its first successful creation, including partial
// preparation. No scenario world or coverage destination is reused here.
func performEngineWarmup(root string, output io.Writer, settings engineWarmupSettings) (result error) {
	var w *world
	var origin time.Time
	var requests []offlineRequest
	defer func() {
		if w != nil {
			if w.offline != nil && w.offline.api != nil {
				a := w.offline.api
				result = errors.Join(result, warmupStage("recorder drain", a.join()))
				result = errors.Join(result, warmupStage("cleanup", a.cleanup()))
				a.mu.Lock()
				requests = append(requests, a.requests...)
				result = errors.Join(result, warmupStage("recorder", a.problem))
				a.mu.Unlock()
			}
			result = errors.Join(result, warmupStage("fixture cleanup", w.closeSession()))
			w.closeDoctor()
			result = errors.Join(result, warmupStage("directory cleanup", os.RemoveAll(filepath.Dir(w.dir))))
		}
		interval, err := warmupInterval(origin, requests)
		result = errors.Join(result, warmupStage("timing", err))
		line := fmt.Sprintf("engine suite warm-up timing: %s outcome=%s\n", interval, timingOutcome(result != nil))
		n, err := io.WriteString(output, line)
		if err == nil && n != len(line) {
			err = io.ErrShortWrite
		}
		result = errors.Join(result, warmupStage("timing output", err))
	}()
	if settings.guard != nil {
		if err := settings.guard(); err != nil {
			return warmupStage("guard", err)
		}
	}
	dir, err := os.MkdirTemp("/tmp", "T-57-engine-suite-")
	if err != nil {
		return warmupStage("preparation", err)
	}
	w = &world{root: root, dir: filepath.Join(dir, "world"), socketRoot: dir}
	if err = os.Mkdir(w.dir, 0700); err != nil {
		return warmupStage("preparation", err)
	}
	if productCoverageRequested() {
		w.coverdir = filepath.Join(dir, "coverage")
		if err = os.Mkdir(w.coverdir, 0700); err != nil {
			return warmupStage("coverage preparation", err)
		}
	}
	// This folder has no engineBudgetTree sibling: the account-home assembly
	// must also compile the original checkout's production limits.
	w.bin, err = buildProduct(root, dir, false)
	if err != nil {
		return warmupStage("build", err)
	}
	if err = settings.fixture(w); err != nil {
		return warmupStage("preparation", err)
	}
	w.offline = &offlineRuntime{argv: []string{"/bin/true"}}
	if err = w.startOfflineAPI(); err != nil {
		return warmupStage("recorder preparation", err)
	}
	// Re-seed after selecting this recorder's endpoint. This is independent
	// synthetic approval data, never an invocation of approve.
	if err = w.approvalSeed("1", false); err != nil {
		return warmupStage("approval preparation", err)
	}
	origin, err = invokeEngineWarmup(w, settings.limit)
	return warmupStage("launch", err)
}

// Keep diagnostics stage-specific and data-free: subprocess transcripts and
// helper errors can contain host paths or arbitrary engine response bodies.
func warmupStage(stage string, err error) error {
	if err == nil {
		return nil
	}
	return warmupFailure{stage: stage, cause: err}
}

type warmupFailure struct {
	stage string
	cause error
}

func (e warmupFailure) Error() string { return "engine suite warm-up: " + e.stage + " failed" }
func (e warmupFailure) Unwrap() error { return e.cause }

func invokeEngineWarmup(w *world, limit time.Duration) (time.Time, error) {
	ctx, cancel := context.WithTimeout(context.Background(), limit)
	defer cancel()
	cmd := exec.CommandContext(ctx, w.bin, "run", "--", "/bin/true")
	cmd.Dir, cmd.Env = w.doctor.checkout, w.env()
	for key, value := range w.doctor.env {
		cmd.Env = setEnv(cmd.Env, key, value)
	}
	cmd.Env = setEnv(cmd.Env, "ITOS_ROOT", w.doctor.checkout)
	cmd.Env = setEnv(cmd.Env, "ITOS_CC_TEST_COVERDIR", "")
	cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
	cmd.WaitDelay = time.Second
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return time.Time{}, err
	}
	defer func() { _ = stdin.Close() }()
	origin := time.Now()
	// Run joins even a killed child. Stdin stays open through Wait: EOF must
	// never be the event that cancels this successful /bin/true workload.
	err = cmd.Run()
	return origin, errors.Join(err, ctx.Err())
}

func warmupInterval(origin time.Time, requests []offlineRequest) (string, error) {
	var creates []offlineRequest
	for _, r := range requests {
		if r.Method == http.MethodPost && r.Path == "/containers/create" {
			creates = append(creates, r)
		}
	}
	values := []string{"missing", "missing", "missing", "missing"}
	var err error
	if len(creates) != 1 {
		err = errors.New("expected one create")
	} else {
		r := creates[0]
		for i, stamp := range []time.Time{r.At, r.Forwarded, r.Answered} {
			if !origin.IsZero() && !stamp.IsZero() {
				values[i] = fmt.Sprint(stamp.Sub(origin).Nanoseconds())
			}
		}
		if !r.Forwarded.IsZero() && !r.Answered.IsZero() {
			values[3] = fmt.Sprint(r.Answered.Sub(r.Forwarded).Nanoseconds())
		}
		if origin.IsZero() || r.At.Before(origin) || r.Forwarded.Before(r.At) || r.Answered.Before(r.Forwarded) || r.Status != http.StatusCreated || r.Cancelled {
			err = errors.New("incomplete or unordered create")
		}
	}
	return fmt.Sprintf("arrival_ns=%s forwarded_ns=%s answer_ns=%s forward_to_answer_ns=%s", values[0], values[1], values[2], values[3]), err
}

func TestEngineSuiteWarmup(t *testing.T) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Skip("Linux amd64 product and Unix-socket fixture")
	}
	t.Setenv("GOCOVERDIR", "")
	t.Setenv("ITOS_CC_TEST_COVERDIR", "")
	root, err := moduleRoot()
	if err != nil {
		t.Fatal(err)
	}
	t.Run("built product through recorder", func(t *testing.T) {
		var suite engineSuiteWarmup
		var output bytes.Buffer
		var w *world
		var engine *sessionAPI
		settings := engineWarmupSettings{limit: 45 * time.Second, fixture: func(candidate *world) error {
			w = candidate
			var err error
			engine, err = warmupLocalFixture(w)
			return err
		}}
		selection := scenarioSelection{engine: true, count: 1}
		if err := suite.before(selection, root, &output, settings); err != nil {
			t.Fatal(err)
		}
		if w == nil || engine == nil {
			t.Fatal("missing throwaway product launch before engine rows")
		}
		engine.mu.Lock()
		defer engine.mu.Unlock()
		if len(engine.created) != 1 || len(engine.local) != 0 || len(engine.volumes) != 0 {
			t.Fatalf("launch did not create and remove its worker and anchor: created=%d workers=%d anchors=%d", len(engine.created), len(engine.local), len(engine.volumes))
		}
		var create map[string]any
		for _, request := range engine.requests {
			if request.Method == "POST" && request.Path == "/containers/create" {
				create = request.Body
			}
		}
		if !reflect.DeepEqual(create["Cmd"], []any{"/bin/true"}) || create["Image"] != w.doctor.imageID || create["User"] != w.approval.uid+":"+w.approval.gid {
			t.Fatal("warm-up did not launch the literal argv with the fixture identity", create)
		}
		if strings.Count(output.String(), "engine suite warm-up timing:") != 1 || !strings.Contains(output.String(), "outcome=passed\n") {
			t.Fatal("missing warm-up create interval", output.String())
		}
		assertWarmupTiming(t, output.String(), w.offline.api.requests)
		for _, operation := range []string{"attach", "start", "wait"} {
			found := false
			for _, r := range engine.requests {
				found = found || r.Method == "POST" && strings.HasSuffix(r.Path, "/"+operation) && r.Status < 300
			}
			if !found {
				t.Errorf("launch did not reach %s", operation)
			}
		}
		if _, err := os.Stat(w.dir); !os.IsNotExist(err) {
			t.Fatal("warm-up retained its disposable world", err)
		}
	})
	t.Run("selection and native guard", testWarmupSelection)
	t.Run("timing failures", testWarmupIntervals)
	t.Run("entry point wiring", testWarmupEntryWiring)
	t.Run("scheduling", func(t *testing.T) { testWarmupScheduling(t, root) })
	t.Run("failure controls", func(t *testing.T) { testWarmupFailures(t, root) })
	t.Run("coverage isolation", func(t *testing.T) { testWarmupCoverage(t, root) })
}

// Reuse the session fixture's stateful protocol engine behind the *actual*
// offline recorder. Its separate doctor value prevents recursive forwarding.
func warmupLocalFixture(w *world, controls ...func(*sessionAPI, http.ResponseWriter, *http.Request) bool) (*sessionAPI, error) {
	if err := w.sessionFixture(false, "0022"); err != nil {
		return nil, err
	}
	d := *w.doctor
	s := *w.session
	s.offline = true
	upstream := &world{doctor: &d, session: &s, approval: w.approval}
	a := &sessionAPI{w: upstream, observed: map[string]map[string]any{}, local: map[string]map[string]any{}, absent: map[string]bool{}}
	server := httptest.NewServer(http.HandlerFunc(func(out http.ResponseWriter, request *http.Request) {
		for _, control := range controls {
			if control(a, out, request) {
				return
			}
		}
		// The real product chooses its own random intent token.
		if doctorRequestPath(request.URL.Path) == "/containers/create" {
			a.mu.Lock()
			s.token = strings.TrimPrefix(request.URL.Query().Get("name"), "itos-container-")
			s.reference = "intents/" + w.approval.project + "/" + s.token + ".json"
			a.mu.Unlock()
		}
		a.serve(out, request)
	}))
	transport := &http.Transport{DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
	}}
	w.doctor.real = &doctorReal{transport: transport, client: &http.Client{Transport: transport, Timeout: time.Second}}
	w.doctor.cleanup = append(w.doctor.cleanup, func() {
		a.mu.Lock()
		if a.attached != nil {
			_ = a.attached.Close()
		}
		a.mu.Unlock()
		server.Close()
		transport.CloseIdleConnections()
	})
	return a, nil
}
