package features

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/cucumber/godog"
	"github.com/moby/moby/client"
)

// The synthetic stand-in is built from this reviewed source by the harness and
// only the sandbox runs it. It is not OpenCode and claims no compatibility: it
// reports what the worker gave it, as one JSON event per stdout line. With the
// egress plan beside it, it then lets the egress probe make the plan's
// requests, after the harness's barrier.
const openCodeStandInSource = `package main
import (
    "encoding/json"
    "os"
    "os/exec"
    "path/filepath"
    "strconv"
    "strings"
    "syscall"
)
type report struct {
    Type string
    Argv []string
    Env map[string]string
    Model string
    UID, GID, RootUID, RootGID int
    Mode string
    Dirs map[string]string
    Writable, ConfigFound, AuthFound bool
    Marker string
    Errors []string
}
func main() {
    r := report{Type: "report", Argv: os.Args, Env: map[string]string{}, Dirs: map[string]string{}, Errors: []string{}}
    check := func(err error) { if err != nil { r.Errors = append(r.Errors, err.Error()) } }
    for _, entry := range os.Environ() {
        key, value, _ := strings.Cut(entry, "=")
        if strings.HasPrefix(key, "OPENCODE_") || strings.HasPrefix(key, "XDG_") || key == "HOME" || key == "TMPDIR" || strings.HasSuffix(key, "_PROXY") || strings.HasSuffix(key, "_proxy") {
            r.Env[key] = value
        }
    }
    prompt := ""
    for i := 1; i < len(os.Args); i++ {
        if os.Args[i] == "--model" && i+1 < len(os.Args) {
            r.Model = os.Args[i+1]
            i++
        } else if !strings.HasPrefix(os.Args[i], "--") && os.Args[i] != "run" {
            prompt = os.Args[i]
        }
    }
    r.UID, r.GID = os.Geteuid(), os.Getegid()
    home := os.Getenv("HOME")
    if info, err := os.Lstat(home); err == nil {
        r.Mode = info.Mode().String()
        if st, ok := info.Sys().(*syscall.Stat_t); ok { r.RootUID, r.RootGID = int(st.Uid), int(st.Gid) }
    } else { check(err) }
    config := filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "opencode")
    data := filepath.Join(os.Getenv("XDG_DATA_HOME"), "opencode")
    state := filepath.Join(os.Getenv("XDG_STATE_HOME"), "opencode")
    r.Dirs = map[string]string{"config": config, "data": data, "state": state}
    _, err := os.Stat(filepath.Join(config, "opencode.json"))
    r.ConfigFound = err == nil
    _, err = os.Stat(filepath.Join(data, "auth.json"))
    r.AuthFound = err == nil
    marker := filepath.Join(data, "slice-41-state-marker")
    if previous, err := os.ReadFile(marker); err == nil { r.Marker = string(previous) }
    for _, dir := range []string{config, data, state} { check(os.MkdirAll(dir, 0700)) }
    check(os.WriteFile(marker, []byte("written by the first run"), 0600))
    err = os.WriteFile(filepath.Join(home, "slice-41-write-probe"), []byte("x"), 0600)
    r.Writable = err == nil
    _ = os.Remove(filepath.Join(home, "slice-41-write-probe"))
    encoder := json.NewEncoder(os.Stdout)
    check(encoder.Encode(r))
    // The report precedes the barrier and every request the plan makes.
    if encoded, err := json.Marshal(r); err == nil { _ = os.WriteFile("/workspace/slice-41-report.json", encoded, 0600) }
    exit := 0
    if _, err := os.Stat("/workspace/slice-27-plan.json"); err == nil {
        child := exec.Command("/workspace/slice-27-probe", "/workspace/slice-27-plan.json")
        child.Stderr = os.Stderr
        if err := child.Run(); err != nil {
            if status, ok := err.(*exec.ExitError); ok { exit = status.ExitCode() } else { exit = 99 }
        }
    } else if strings.Contains(prompt, "seven") {
        exit = 7
    }
    _ = encoder.Encode(map[string]any{"type": "text", "text": "done", "exit": strconv.Itoa(exit)})
    os.Exit(exit)
}
`

type openCodeReport struct {
	Type                       string
	Argv                       []string
	Env                        map[string]string
	Model                      string
	UID, GID, RootUID, RootGID int
	Mode                       string
	Dirs                       map[string]string
	Writable                   bool
	ConfigFound, AuthFound     bool
	Marker                     string
	Errors                     []string
}

type openCodeRunResult struct {
	exit           int
	stdout, stderr []byte
	err            error
	events         []map[string]any
	report         openCodeReport
	prompt         string
}

type openCodeNative struct {
	runs []openCodeRunResult
}

// openCodeStandIn builds the stand-in into the checkout, named by its explicit path.
func (w *world) installOpenCodeStandIn() error {
	return w.installNativeProbe("opencode", func(path string) error { return recoveryBuildHelper(w.root, path, openCodeStandInSource) })
}

// openCodeEvents splits stdout into JSON event lines and refuses any other byte.
func openCodeEvents(stdout []byte) ([]map[string]any, error) {
	if len(stdout) == 0 || stdout[len(stdout)-1] != '\n' {
		return nil, fmt.Errorf("stdout is not whole event lines: %q", stdout)
	}
	var events []map[string]any
	for _, line := range bytes.Split(bytes.TrimSuffix(stdout, []byte("\n")), []byte("\n")) {
		var event map[string]any
		if err := json.Unmarshal(line, &event); err != nil || event["Type"] == nil && event["type"] == nil {
			return nil, fmt.Errorf("stdout holds a line that is no event: %q", line)
		}
		events = append(events, event)
	}
	return events, nil
}

// openCodeCheckReport requires the literal argv, the opencode-v1 environment and
// the isolated directories inside a private, writable /home/itos.
func (w *world) openCodeCheckReport(r openCodeReport, argv []string, model string) error {
	if !slices.Equal(r.Argv, argv) || r.Model != model {
		return fmt.Errorf("the stand-in reports argv %q and model %q; want %q and %q", r.Argv, r.Model, argv, model)
	}
	for _, pair := range openCodeEnvironment {
		if got, ok := r.Env[pair[0]]; !ok || got != pair[1] {
			return fmt.Errorf("the stand-in reports %s=%q, want %q", pair[0], got, pair[1])
		}
	}
	for name, want := range map[string]string{"HOME": "/home/itos", "TMPDIR": "/tmp", "XDG_CONFIG_HOME": "/home/itos/.config", "XDG_DATA_HOME": "/home/itos/.local/share", "XDG_STATE_HOME": "/home/itos/.local/state", "XDG_CACHE_HOME": "/home/itos/.cache"} {
		if r.Env[name] != want {
			return fmt.Errorf("the stand-in reports %s=%q, want %q", name, r.Env[name], want)
		}
	}
	for name, dir := range r.Dirs {
		if !strings.HasPrefix(dir, "/home/itos/") || !strings.HasSuffix(dir, "/opencode") {
			return fmt.Errorf("OpenCode %s directory %q is not inside /home/itos", name, dir)
		}
	}
	if len(r.Dirs) != 3 || !r.Writable || r.Mode != "drwx------" || strconv.Itoa(r.UID) != w.approval.uid || strconv.Itoa(r.GID) != w.approval.gid || len(r.Errors) != 0 {
		return fmt.Errorf("the stand-in did not find a writable private /home/itos: %+v", r)
	}
	return nil
}

func (w *world) homeOpenCodeNativeSteps(sc *godog.ScenarioContext) {
	var native *openCodeNative
	var sentinels map[string]runEntry
	prompts := []string{"say-ok", "fail-seven"}

	// ID-HOME-30
	sc.Step(`^a CI OpenCode checkout with a cached approved image for the actual non-root controller UID GID, an approved offline volume home with the OpenCode harness, host-home OpenCode configuration and login sentinels and a synthetic opencode stand-in installed at "/workspace/opencode"$`, func() error {
		if err := w.homeNativeFixture(); err != nil {
			return err
		}
		a, d, f := w.approval, w.doctor, w.home.native
		if err := os.WriteFile(filepath.Join(d.checkout, "itos-container.yaml"), []byte(openCodePolicy(true)), 0600); err != nil {
			return err
		}
		a.home = openCodeHome(true)
		if err := w.approvalOracle(); err != nil {
			return err
		}
		if err := os.WriteFile(a.record, approvalJSON(map[string]any{"schema": 1, "project": a.project, "generation": "1", "fingerprint": a.fingerprint, "settings": a.settings}), 0600); err != nil {
			return err
		}
		if err := openCodeSentinels(f.sentinels); err != nil {
			return err
		}
		if err := w.installOpenCodeStandIn(); err != nil {
			return err
		}
		var err error
		if f.hostBefore, err = runTree(f.sentinels); err != nil {
			return err
		}
		sentinels = f.hostBefore
		f.checkoutBefore, err = runTree(d.checkout)
		native = &openCodeNative{}
		return err
	})
	sc.Step(`^two fresh product runs invoke the stand-in as "/workspace/opencode --standalone run --format json" with a prompt, the first exiting zero and the second exiting seven$`, func() error {
		for _, prompt := range prompts {
			native.runs = append(native.runs, w.openCodeProductRun([]string{"/workspace/opencode", "--standalone", "run", "--format", "json", prompt}, prompt))
		}
		return nil
	})
	sc.Step(`^in both runs the stand-in reports its literal argv, the opencode-v1 environment and its configuration data and state directories inside a writable private /home/itos$`, func() error {
		if len(native.runs) != 2 {
			return errors.New("both runs were not made")
		}
		for i, run := range native.runs {
			if run.err != nil {
				return run.err
			}
			argv := []string{"/workspace/opencode", "--standalone", "run", "--format", "json", prompts[i]}
			if err := w.openCodeCheckReport(run.report, argv, ""); err != nil {
				return fmt.Errorf("run %d: %w\nstderr: %s", i+1, err, run.stderr)
			}
		}
		return nil
	})
	sc.Step(`^it finds no host OpenCode configuration or login and the second run reads the state marker the first wrote under its OpenCode data directory$`, func() error {
		for i, run := range native.runs {
			if run.report.ConfigFound || run.report.AuthFound {
				return fmt.Errorf("run %d found host OpenCode configuration or login", i+1)
			}
			want := ""
			if i == 1 {
				want = "written by the first run"
			}
			if run.report.Marker != want {
				return fmt.Errorf("run %d read state marker %q, want %q", i+1, run.report.Marker, want)
			}
		}
		return nil
	})
	sc.Step(`^run's stdout is exactly the stand-in's JSON event lines with exit codes zero and seven and product diagnostics only on stderr$`, func() error {
		for i, run := range native.runs {
			if run.exit != []int{0, 7}[i] {
				return fmt.Errorf("run %d exited %d\nstderr: %s", i+1, run.exit, run.stderr)
			}
			if len(run.events) != 2 || run.events[0]["Type"] != "report" || run.events[1]["type"] != "text" {
				return fmt.Errorf("run %d stdout is not the stand-in's two events: %s", i+1, run.stdout)
			}
			if bytes.Contains(run.stderr, []byte(`"Type":"report"`)) || bytes.Contains(run.stderr, []byte(`"type":"text"`)) {
				return fmt.Errorf("run %d moved stand-in events to stderr", i+1)
			}
		}
		return nil
	})
	sc.Step(`^after each run its worker and anchor are absent while the home remains and the host-home sentinels and checkout ownership are unchanged$`, func() error {
		f := w.home.native
		after, err := runTree(w.doctor.checkout)
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(after, f.checkoutBefore) {
			return errors.New("the runs changed the checkout contents, ownership or inode identity")
		}
		host, err := runTree(f.sentinels)
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(host, sentinels) {
			return errors.New("the runs changed the host-home sentinels")
		}
		if len(f.volumes) != 1 {
			return fmt.Errorf("expected exactly one retained home, observed %d", len(f.volumes))
		}
		return w.homeNativeResources()
	})

	// ID-HOME-31
	sc.Step(`^a CI OpenCode checkout allowing "provider\.example" with an approved volume home with the OpenCode harness, matching protected approval and a synthetic opencode stand-in installed at "/workspace/opencode"$`, func() error {
		if err := w.egressNativeCheckoutHome([]string{"provider.example"}, []string{"provider.example"}, true); err != nil {
			return err
		}
		n, err := w.egressNativeWorld()
		if err != nil {
			return err
		}
		a := w.approval
		scope := approvalHash(map[string]any{"schema": 1, "project": a.project, "uid": a.uid, "gid": a.gid, "engine": map[string]any{"endpoint": w.doctor.env["DOCKER_HOST"], "id": w.doctor.engineID}})
		n.cleanup = append(n.cleanup, func() error { return n.removeHomes(scope) })
		return w.installOpenCodeStandIn()
	})
	sc.Step(`^independent fixture TLS aliases with nonce receipts for "provider\.example models\.opencode\.ai" on a fixture network$`, func() error {
		return w.egressEndpoints(func(*egressNative) [][]string {
			return [][]string{{"provider.example", "public", "provider.example"}, {"models.opencode.ai", "public", "models.opencode.ai"}}
		})
	})
	sc.Step(`^the product runs "/workspace/opencode --standalone run --format json --model fake/fake-model say-ok" and at the stand-in's checkout barrier the harness attaches only the proxy to the fixture network$`, func() error {
		n, err := w.egressNativeWorld()
		if err != nil {
			return err
		}
		run := n.newRun([]egressStep{
			{Op: "tls", Mode: "env", Name: "provider.example", Nonce: "model-" + egressNonce()},
			{Op: "raw", Method: "CONNECT", Target: "models.opencode.ai:443"},
		}, 0)
		run.argv = []string{"/workspace/opencode", "--standalone", "run", "--format", "json", "--model", "fake/fake-model", "say-ok"}
		_, err = n.driveRun(run)
		n.controlAll()
		return err
	})
	last := func(check func(*egressNative, *egressNativeRun) error) func() error {
		return func() error {
			n, err := w.egressNativeWorld()
			if err != nil {
				return err
			}
			run, err := n.last()
			if err != nil {
				return err
			}
			if err = check(n, run); err != nil {
				return fmt.Errorf("%w\n%s", err, run.describe())
			}
			return nil
		}
	}
	sc.Step(`^the run is not refused before its worker starts and the stand-in reports its literal argv, the opencode-v1 environment and model "fake/fake-model" before it sends its model request$`, last(func(_ *egressNative, run *egressNativeRun) error {
		if !run.started || !run.reached["attach"] {
			return errors.New("the run was refused before its worker reached the checkout barrier")
		}
		data, err := os.ReadFile(filepath.Join(w.doctor.checkout, "slice-41-report.json"))
		if err != nil {
			return fmt.Errorf("the stand-in made no report before its requests: %w", err)
		}
		var report openCodeReport
		if err = json.Unmarshal(data, &report); err != nil {
			return err
		}
		return w.openCodeCheckReportEgress(report)
	}))
	sc.Step(`^the fake provider receives exactly the stand-in's nonce request through the proxy variables and the stand-in prints its final JSON event on run's stdout$`, last(func(n *egressNative, run *egressNativeRun) error {
		if err := n.exchanged(run, 1); err != nil {
			return err
		}
		if results := run.results("tls"); results[0].Mode != "env" || results[0].Name != "provider.example" {
			return errors.New("the model request did not use the proxy variables")
		}
		if err := n.endpoints["provider.example"].onlyExchanges(false); err != nil {
			return err
		}
		events, err := openCodeEvents(run.stdout.bytes())
		if err != nil {
			return err
		}
		if len(events) != 2 || events[0]["Type"] != "report" || events[1]["type"] != "text" {
			return fmt.Errorf("stdout is not the stand-in's report and final event: %s", run.stdout.bytes())
		}
		return nil
	}))
	sc.Step(`^each startup-service name receives 403 without an upstream dial or a fixture receipt while its independent control succeeds$`, last(func(n *egressNative, run *egressNativeRun) error {
		results := run.results("raw")
		if len(results) != 1 || results[0].Target != "models.opencode.ai:443" || !results[0].OK || results[0].Status != 403 || results[0].Body != 0 || !results[0].Closed {
			return fmt.Errorf("the startup-service name was not refused with an empty 403: %+v", results)
		}
		return n.endpoints["models.opencode.ai"].onlyControls()
	}))
	sc.Step(`^the product returns the observed workload exit and confirms its worker, proxy and both networks absent while the home remains$`, last(func(n *egressNative, run *egressNativeRun) error {
		if err := run.cleaned(); err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		homes, err := n.homes(ctx)
		if err != nil {
			return err
		}
		if len(homes) != 1 {
			return fmt.Errorf("expected the home to remain, found %v", homes)
		}
		return nil
	}))
}

// openCodeCheckReportEgress is the same report check for the egress checkout,
// where the stand-in's home is the approved volume.
func (w *world) openCodeCheckReportEgress(r openCodeReport) error {
	argv := []string{"/workspace/opencode", "--standalone", "run", "--format", "json", "--model", "fake/fake-model", "say-ok"}
	return w.openCodeCheckReport(r, argv, "fake/fake-model")
}

// homes lists this checkout's retained harness-home volumes by the scope label.
func (n *egressNative) homes(ctx context.Context) ([]string, error) {
	a := n.w.approval
	scope := approvalHash(map[string]any{"schema": 1, "project": a.project, "uid": a.uid, "gid": a.gid, "engine": map[string]any{"endpoint": n.w.doctor.env["DOCKER_HOST"], "id": n.w.doctor.engineID}})
	filter := make(client.Filters).Add("label", "io.itos-container.scope="+scope).Add("label", "io.itos-container.kind=harness-home")
	volumes, err := n.cli.VolumeList(ctx, client.VolumeListOptions{Filters: filter})
	if err != nil {
		return nil, err
	}
	var names []string
	for _, v := range volumes.Items {
		names = append(names, v.Name)
	}
	return names, nil
}

// removeHomes is the harness's own teardown of the retained home it observed.
func (n *egressNative) removeHomes(scope string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	filter := make(client.Filters).Add("label", "io.itos-container.scope="+scope).Add("label", "io.itos-container.kind=harness-home")
	volumes, err := n.cli.VolumeList(ctx, client.VolumeListOptions{Filters: filter})
	if err != nil {
		return err
	}
	var result error
	for _, v := range volumes.Items {
		_, err = n.cli.VolumeRemove(ctx, v.Name, client.VolumeRemoveOptions{})
		result = errors.Join(result, err)
	}
	return result
}

// openCodeProductRun runs the product with the literal argv and captures
// whatever happens for the Then steps.
func (w *world) openCodeProductRun(argv []string, prompt string) openCodeRunResult {
	run := openCodeRunResult{exit: -1, prompt: prompt}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, w.bin, append([]string{"run", "--"}, argv...)...)
	cmd.Dir, cmd.Env = w.doctor.checkout, w.env()
	var out, errout offlineBuffer
	cmd.Stdout, cmd.Stderr = &out, &errout
	err := cmd.Run()
	run.stdout, run.stderr = out.bytes(), errout.bytes()
	if cmd.ProcessState != nil {
		run.exit = cmd.ProcessState.ExitCode()
	}
	var exited *exec.ExitError
	if err != nil && !errors.As(err, &exited) {
		run.err = err
	}
	// The home persists; track it so the harness can remove it afterwards.
	if _, _, observed := w.homeNativeObserve(); observed != nil {
		run.err = errors.Join(run.err, observed)
	}
	run.events, err = openCodeEvents(run.stdout)
	if err != nil {
		run.err = errors.Join(run.err, fmt.Errorf("%w; exit %d, stderr %s", err, run.exit, run.stderr))
		return run
	}
	if data, marshalErr := json.Marshal(run.events[0]); marshalErr == nil {
		run.err = errors.Join(run.err, json.Unmarshal(data, &run.report))
	}
	return run
}
