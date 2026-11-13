package features

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
)

type egressStep struct {
	Op, Name, Mode, Method, Target, Input, Nonce, URL string
}

type egressProbeResult struct {
	Op, Name, Mode, Method, Target, Input string
	OK, Closed                            bool
	Status, Body, Received                int
	Peer, Reply, Location, Error          string
}

type egressProbeReport struct {
	Env     map[string]string
	Results []egressProbeResult
	Done    bool
}

// egressObserved is what the engine itself said about the product's resources
// while the worker waited at a barrier. The harness asked; the product did not.
type egressObserved struct {
	worker, proxy *container.InspectResponse
	networks      map[string]network.Inspect // by the product's kind label
	err           error
}

type egressNativeRun struct {
	n              *egressNative
	steps          []egressStep
	wantExit       int
	cmd            *exec.Cmd
	cancel         context.CancelFunc
	done           chan struct{}
	stdout, stderr offlineBuffer
	exit           int
	started        bool
	timedOut       bool
	reached        map[string]bool
	observed       map[string]*egressObserved
	attached       []string // the harness's own extra attachments, recorded apart
	report         egressProbeReport
	remaining      []string
	snapshotPath   string
	snapshot       []byte
	err            error
}

func (n *egressNative) newRun(steps []egressStep, exit int) *egressNativeRun {
	run := &egressNativeRun{n: n, steps: append([]egressStep{{Op: "barrier", Name: "attach"}}, steps...), wantExit: exit, exit: -1, reached: map[string]bool{}, observed: map[string]*egressObserved{}, done: make(chan struct{})}
	n.runs = append(n.runs, run)
	return run
}

// start launches the product. A refusal, a missing behaviour or a timeout is
// captured for Then; only a harness fault is an error of the step itself.
func (run *egressNativeRun) start() error {
	w := run.n.w
	checkout := w.doctor.checkout
	stale, _ := filepath.Glob(filepath.Join(checkout, "slice-27-[br][ae][rl][re][ia]*"))
	for _, path := range append(stale, filepath.Join(checkout, "slice-27-report.json")) {
		_ = os.Remove(path)
	}
	if err := os.WriteFile(filepath.Join(checkout, "slice-27-plan.json"), approvalJSON(map[string]any{"Steps": run.steps, "Exit": run.wantExit}), 0600); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	run.cancel = cancel
	run.cmd = exec.CommandContext(ctx, w.bin, "run", "--", "/workspace/slice-27-probe", "/workspace/slice-27-plan.json")
	run.cmd.Dir, run.cmd.Env = checkout, setEnv(w.env(), "ITOS_ROOT", checkout)
	run.cmd.Stdout, run.cmd.Stderr = &run.stdout, &run.stderr
	run.cmd.WaitDelay = 5 * time.Second
	if err := run.cmd.Start(); err != nil {
		cancel()
		return err
	}
	run.started, run.snapshotPath = true, w.runSnapshotPath()
	go func() {
		_ = run.cmd.Wait()
		close(run.done)
	}()
	return nil
}

// await is whether the worker reached the named barrier before the product
// exited or the limit passed.
func (run *egressNativeRun) await(name string, limit time.Duration) bool {
	path := filepath.Join(run.n.w.doctor.checkout, "slice-27-barrier-"+name)
	deadline := time.After(limit)
	for {
		if _, err := os.Stat(path); err == nil {
			run.reached[name] = true
			return true
		}
		select {
		case <-run.done:
			return false
		case <-deadline:
			return false
		case <-time.After(25 * time.Millisecond):
		}
	}
}

func (run *egressNativeRun) release(name string) error {
	return os.WriteFile(filepath.Join(run.n.w.doctor.checkout, "slice-27-release-"+name), nil, 0600)
}

// observe inspects the product's containers and networks by its own labels.
func (run *egressNativeRun) observe(name string) {
	n := run.n
	o := &egressObserved{networks: map[string]network.Inspect{}}
	run.observed[name] = o
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	filter := make(client.Filters).Add("label", "io.itos-container.project="+n.w.approval.project)
	containers, err := n.cli.ContainerList(ctx, client.ContainerListOptions{All: true, Filters: filter})
	if err != nil {
		o.err = err
		return
	}
	for _, item := range containers.Items {
		inspected, err := n.cli.ContainerInspect(ctx, item.ID, client.ContainerInspectOptions{})
		if err != nil || inspected.Container.Config == nil {
			o.err = errors.Join(o.err, err, errors.New("product container could not be inspected"))
			continue
		}
		c := inspected.Container
		switch kind := c.Config.Labels["io.itos-container.kind"]; {
		case kind == "egress-proxy" && o.proxy == nil:
			o.proxy = &c
		case kind == "" && o.worker == nil:
			o.worker = &c
		default:
			o.err = errors.Join(o.err, fmt.Errorf("unexpected product container of kind %q", kind))
		}
	}
	networks, err := n.cli.NetworkList(ctx, client.NetworkListOptions{Filters: filter})
	if err != nil {
		o.err = errors.Join(o.err, err)
		return
	}
	for _, item := range networks.Items {
		inspected, err := n.cli.NetworkInspect(ctx, item.ID, client.NetworkInspectOptions{})
		if err != nil {
			o.err = errors.Join(o.err, err)
			continue
		}
		kind := inspected.Network.Labels["io.itos-container.kind"]
		if _, twice := o.networks[kind]; twice {
			o.err = errors.Join(o.err, fmt.Errorf("two product networks of kind %q", kind))
		}
		o.networks[kind] = inspected.Network
	}
}

// attach joins the product's proxy to every fixture endpoint network, as the
// egress record allows a harness to. It never touches the worker or either of
// the product's own networks.
func (run *egressNativeRun) attach(o *egressObserved) {
	if o.proxy == nil {
		run.err = errors.Join(run.err, errors.New("no proxy to attach to the fixture endpoints"))
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	for key, f := range run.n.networks {
		if _, err := run.n.cli.NetworkConnect(ctx, f.id, client.NetworkConnectOptions{Container: o.proxy.ID}); err != nil {
			run.err = errors.Join(run.err, fmt.Errorf("attach the proxy to the %s fixture network: %w", key, err))
			continue
		}
		run.attached = append(run.attached, f.name)
	}
	// The route and resolver control's network is one more fixture network: a
	// proxy that forwarded would now have a leg on it.
	if iso := run.n.isolation; iso != nil && iso.s.networkID != "" {
		if _, err := run.n.cli.NetworkConnect(ctx, iso.s.networkID, client.NetworkConnectOptions{Container: o.proxy.ID}); err != nil {
			run.err = errors.Join(run.err, fmt.Errorf("attach the proxy to the control network: %w", err))
			return
		}
		run.attached = append(run.attached, iso.s.networkName)
	}
}

// finish waits for the product to return, reads the probe's report and counts
// what the project still owns before any harness cleanup can run.
func (run *egressNativeRun) finish(limit time.Duration) {
	if !run.started {
		return
	}
	select {
	case <-run.done:
	case <-time.After(limit):
		run.timedOut = true
		run.stop()
	}
	run.cancel()
	if state := run.cmd.ProcessState; state != nil {
		run.exit = state.ExitCode()
	}
	w := run.n.w
	if data, err := os.ReadFile(filepath.Join(w.doctor.checkout, "slice-27-report.json")); err == nil {
		run.err = errors.Join(run.err, json.Unmarshal(data, &run.report))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	var err error
	run.remaining, err = run.n.owned(ctx)
	run.err = errors.Join(run.err, err)
	run.snapshot, _ = os.ReadFile(run.snapshotPath)
}

func (run *egressNativeRun) stop() {
	if !run.started {
		return
	}
	select {
	case <-run.done:
	default:
		run.cancel()
		<-run.done
	}
}

// newPhasedRun also gives the worker steps to take before its first barrier:
// its earliest instructions, while the proxy has no fixture network at all.
func (n *egressNative) newPhasedRun(first, running []egressStep, exit int) *egressNativeRun {
	run := n.newRun(running, exit)
	run.steps = append(append([]egressStep{}, first...), run.steps...)
	return run
}

// drive is the common run: wait for the worker's first barrier, observe, attach
// the proxy to the fixture endpoints, release, and wait for the product.
func (n *egressNative) drive(steps []egressStep, exit int) (*egressNativeRun, error) {
	return n.driveRun(n.newRun(steps, exit))
}

func (n *egressNative) driveRun(run *egressNativeRun) (*egressNativeRun, error) {
	if err := run.start(); err != nil {
		return run, err
	}
	if run.await("attach", 150*time.Second) {
		run.observe("attach")
		run.attach(run.observed["attach"])
		if err := run.release("attach"); err != nil {
			return run, err
		}
	}
	run.finish(3 * time.Minute)
	return run, nil
}

func (n *egressNative) last() (*egressNativeRun, error) {
	if len(n.runs) == 0 {
		return nil, errors.New("the product was never run")
	}
	return n.runs[len(n.runs)-1], nil
}

// results are the probe's observations of one kind, in plan order.
func (run *egressNativeRun) results(op string) []egressProbeResult {
	var out []egressProbeResult
	for _, r := range run.report.Results {
		if r.Op == op {
			out = append(out, r)
		}
	}
	return out
}

func (run *egressNativeRun) describe() string {
	return fmt.Sprintf("product exit %d (timed out %v)\n--- stdout\n%s--- stderr\n%s--- probe\n%s", run.exit, run.timedOut, run.stdout.bytes(), run.stderr.bytes(), approvalJSON(run.report))
}
