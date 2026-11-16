package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/alecthomas/kong"
	"github.com/donvargax/itos-container/internal/cli"
	ap "github.com/donvargax/itos-container/internal/engine/approval/port"
	af "github.com/donvargax/itos-container/internal/engine/approval/port/porttest"
	sd "github.com/donvargax/itos-container/internal/engine/session/domain"
	sp "github.com/donvargax/itos-container/internal/engine/session/port"
	sf "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
	tp "github.com/donvargax/itos-container/internal/engine/terminal/port"
	tf "github.com/donvargax/itos-container/internal/engine/terminal/port/porttest"
	runcommand "github.com/donvargax/itos-container/internal/slices/run/command"
	rf "github.com/donvargax/itos-container/internal/slices/run/port/porttest"
)

type terminalContractReport struct {
	Status                                                           sp.RecoveryResult
	RecoveryRestarted                                                bool
	Backend, Diagnostic, Output, Error                               string
	Code                                                             int
	Ready, Fired, TTY, InputRestored, OutputRestored, ObserversEnded bool
	Initial                                                          tp.Dimensions
	Result                                                           sp.RunResult
	Elapsed                                                          time.Duration
	Events                                                           []terminalContractEvent
	Record                                                           sp.PublicRecord
	Recovery                                                         sp.RecoveryResult
	Remaining                                                        int
	AcquiredInput                                                    bool
	Retry                                                            *terminalContractReport
}
type terminalContractEvent struct {
	Name, Target string
	After        time.Duration
	Remaining    time.Duration
}

type terminalContractNative struct {
	Provider      tp.Provider
	Output        sp.Writer
	BindingError  error
	Restored      func() (bool, bool)
	ReadOutput    func() (string, error)
	AcquiredInput func() bool
	Clear         func()
}

var newTerminalContractNative func(*testing.T, string) *terminalContractNative

func TestTerminalRunContract(t *testing.T) {
	kind := os.Getenv("SLICE_37_CONTRACT")
	if kind == "" {
		t.Skip("selected by terminal-run scenarios")
	}
	fault, child := os.Getenv("SLICE_37_FAULT"), os.Getenv("SLICE_37_CHILD")
	var reports []terminalContractReport
	for _, backend := range []string{"ports", "protected records"} {
		synctest.Test(t, func(t *testing.T) {
			reports = append(reports, terminalContractCase(t, backend, kind, fault, child))
			reports = append(reports, terminalContractCase(t, backend, "healthy", "", "0"))
		})
	}
	if kind == "acquisition" && newTerminalContractNative != nil {
		reports = append(reports, terminalContractCase(t, "native terminals", kind, fault, child))
		reports = append(reports, terminalContractCase(t, "native terminals", "healthy", "", "0"))
	}
	if err := json.NewEncoder(os.Stdout).Encode(reports); err != nil {
		t.Fatal(err)
	}
	os.Exit(0)
}

type terminalContractFixture struct {
	l                  *lifecycleLocal
	pair               *tf.Pair
	in, out            *tf.Fake
	fault, child, kind string
	mu                 sync.Mutex
	ready, fired       bool
	size               tp.Dimensions
	result             sp.RunResult
	streams            chan sp.Outcome
}

func terminalContractCase(t *testing.T, backend, kind, fault, child string) terminalContractReport {
	l := lifecycleLocalFixture(t, backend)
	f := &terminalContractFixture{l: l, fault: fault, child: child, kind: kind, streams: make(chan sp.Outcome, 1)}
	f.in = &tf.Fake{Size: tp.Dimensions{Columns: 113, Rows: 37}, State: 3}
	f.out = f.in
	if strings.Contains(fault, "distinct") || kind == "acquisition" {
		f.out = &tf.Fake{OutputOnly: true, State: 7}
	}
	f.pair = &tf.Pair{Input: f.in, Output: f.out}
	acquiredInput := false
	f.pair.AfterInput = func() { acquiredInput = f.in.Raw() }
	var provider tp.Provider = f.pair
	var native *terminalContractNative
	if backend == "native terminals" {
		native = newTerminalContractNative(t, fault)
		provider = native.Provider
	}
	wait := func() {
		if native == nil {
			synctest.Wait()
		}
	}
	if strings.Contains(fault, "restoration fails") {
		target := f.in
		if strings.Contains(fault, "stdout") {
			target = f.out
		}
		target.RestoreError = errors.New("fixture restore operation failed")
	}
	if kind == "acquisition" {
		if strings.Contains(fault, "reading") {
			f.pair.OutputStateError = errors.New("fixture stdout state read failed")
		} else {
			f.pair.OutputRawError = errors.New("fixture stdout raw acquisition failed")
		}
	}
	inBefore, outBefore := f.in.Snapshot(), f.out.Snapshot()
	l.m.e.Exit, _ = strconv.Atoi(child)
	l.m.e.Output = []byte("slice-37 contract raw\n\r\x01\x00payload")
	l.m.e.StdinFailures = f.streams
	l.m.e.WaitUntilCancel = kind != "healthy" && kind != "acquisition" && !strings.Contains(fault, "restoration fails") && fault != "terminal output blocks during exit"
	if fault == "terminal output blocks during exit" {
		l.m.e.DrainUntilCancel = true
	}
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	var out, diagnostic bytes.Buffer
	var c commandLine
	parser, err := kong.New(&c, append(cli.Flags(), kong.Name("itos-container"), kong.Vars{"version": "fixture"}, kong.Writers(&out, &diagnostic))...)
	if err != nil {
		t.Fatal(err)
	}
	ui := &cli.UI{Stdout: &out, Stderr: &diagnostic, Classify: runcommand.Classify}
	if native != nil {
		ui.Stdout = native.Output
	}
	_, err = parser.Parse([]string{"run", "--tty", "--", "fixture-workload"})
	report := terminalContractReport{Backend: backend}
	if err != nil {
		report.Code = ui.Usage("run", err, false)
	} else {
		// The red commit compiles against the pre-TTY command. Injection is a
		// typed port assignment once that command exposes its terminal binding;
		// it does not replace the entry point or provide an expected result.
		field := reflect.ValueOf(&c.Run).Elem().FieldByName("Terminal")
		if native != nil && native.BindingError != nil {
			report.Error = native.BindingError.Error()
		} else if !field.IsValid() || !field.CanSet() || !reflect.TypeOf(provider).AssignableTo(field.Type()) {
			report.Error = "run terminal provider binding is unavailable"
		} else {
			field.Set(reflect.ValueOf(provider))
			approval := &rf.Approval{View: ap.View{Inspection: af.Ready(), Candidate: l.m.c, Record: ap.Record{Candidate: l.m.c, Generation: "1"}, Status: "matching"}}
			c.Run.Approval = lifecycleApproval{approval, l, ctx}
			c.Run.Snapshots = &af.Snapshots{}
			c.Run.Context = ctx
			c.Run.Runner = terminalContractRunner{f, sd.Runner{Store: l.store, Registry: lifecycleRegistry{l.registry, l}, Engine: terminalContractEngine{f}, Workspace: sf.MountWorkspace(l.m.b), Entropy: &publicMemoryEntropy{}}}
			report.Code = c.Run.Run(ui)
		}
	}
	wait()
	report.Diagnostic, report.Output = diagnostic.String(), out.String()
	report.Elapsed = time.Since(l.start)
	report.Ready, report.Fired, report.Initial = f.ready, f.fired, f.size
	report.TTY = f.size == (tp.Dimensions{Columns: 113, Rows: 37})
	report.InputRestored, report.OutputRestored = reflect.DeepEqual(inBefore, f.in.Snapshot()), reflect.DeepEqual(outBefore, f.out.Snapshot())
	if native != nil {
		report.InputRestored, report.OutputRestored = native.Restored()
		acquiredInput = native.AcquiredInput()
		if output, err := native.ReadOutput(); err != nil {
			report.Error = err.Error()
		} else {
			report.Output = output
		}
	}
	report.AcquiredInput = acquiredInput
	// Reacquisition proves ownership and observation ended without closing any
	// caller resource. Ignore a retained simulated hangup, not a Busy refusal.
	lease, acquireErr := provider.Acquire(context.Background())
	var failure *tp.Failure
	report.ObserversEnded = !errors.As(acquireErr, &failure) || failure.Kind != tp.Busy
	if lease != nil {
		lease.Close()
	}
	report.Result = f.result
	for _, e := range l.events {
		report.Events = append(report.Events, terminalContractEvent{e.name, e.target, e.at.Sub(l.start), e.remaining})
	}
	if f.result.SessionID != "" {
		if entry, outcome := l.registry.Open(context.Background(), f.result.SessionID, true); outcome == "" {
			report.Record = entry.Record()
			entry.Close()
		}
	}
	if kind == "disconnect" && f.ready {
		l.hidden = false
		l.m.e.Unavailable = false
		l.m.e.Following = ""
		f.mu.Lock()
		f.kind = "recovery"
		f.mu.Unlock()
		recovery := sd.Recovery{Registry: l.registry, Engine: terminalContractEngine{f}}
		creates, starts := l.count("create"), l.count("start")
		report.Status = recovery.Status(context.Background())
		report.Recovery = recovery.Stop(context.Background(), f.result.SessionID)
		report.RecoveryRestarted = creates != l.count("create") || starts != l.count("start")
	}
	report.Remaining = len(l.m.e.Resources)
	if kind == "acquisition" && report.Error == "" && report.Code != 2 {
		f.pair.OutputStateError, f.pair.OutputRawError = nil, nil
		if native != nil {
			native.Clear()
		}
		f.kind, f.child, l.m.e.Exit = "healthy", "0", 0
		out.Reset()
		diagnostic.Reset()
		code := c.Run.Run(ui)
		wait()
		report.Retry = &terminalContractReport{Backend: backend, Code: code, Ready: f.ready,
			TTY: f.size == (tp.Dimensions{Columns: 113, Rows: 37}), Initial: f.size,
			Diagnostic: diagnostic.String(), Output: out.String(), Result: f.result,
			InputRestored: reflect.DeepEqual(inBefore, f.in.Snapshot()), OutputRestored: reflect.DeepEqual(outBefore, f.out.Snapshot()),
			Remaining: len(l.m.e.Resources)}
		if native != nil {
			report.Retry.InputRestored, report.Retry.OutputRestored = native.Restored()
			if output, err := native.ReadOutput(); err != nil {
				report.Retry.Error = err.Error()
			} else {
				report.Retry.Output = output
			}
		}
	}
	return report
}

type terminalContractRunner struct {
	f      *terminalContractFixture
	runner sd.Runner
}

func (r terminalContractRunner) Run(ctx context.Context, frozen ap.Durable, argv []string, streams sp.Streams) sp.RunResult {
	r.f.result = r.runner.Run(ctx, frozen, argv, streams)
	return r.f.result
}

type terminalContractEngine struct{ f *terminalContractFixture }

func (e terminalContractEngine) OpenRun(ctx context.Context, b sp.Binding) (sp.RunConnection, sp.Outcome) {
	c, outcome := (lifecycleEngine{e.f.l}).OpenRun(ctx, b)
	if c == nil {
		return nil, outcome
	}
	return terminalContractConnection{c, e.f}, outcome
}

type terminalContractConnection struct {
	sp.RunConnection
	f *terminalContractFixture
}

func (c terminalContractConnection) SetTerminal(size tp.Dimensions) sp.Outcome {
	c.f.size = size
	if bound, ok := c.RunConnection.(interface {
		SetTerminal(tp.Dimensions) sp.Outcome
	}); ok {
		return bound.SetTerminal(size)
	}
	return ""
}
func (c terminalContractConnection) Resize(ctx context.Context, id string, size tp.Dimensions) sp.Outcome {
	c.f.l.event(ctx, "resize", id)
	if c.f.fault == "running worker resize times out" {
		<-ctx.Done()
		return sp.ExecutionFailed
	}
	if c.f.fault == "running worker resize fails" {
		return sp.ExecutionFailed
	}
	if bound, ok := c.RunConnection.(interface {
		Resize(context.Context, string, tp.Dimensions) sp.Outcome
	}); ok {
		return bound.Resize(ctx, id, size)
	}
	return ""
}
func (c terminalContractConnection) Start(ctx context.Context, id string) sp.Outcome {
	failure := c.RunConnection.Start(ctx, id)
	if failure == "" {
		c.f.ready = true
	}
	return failure
}
func (c terminalContractConnection) Wait(ctx context.Context, id string) (int, sp.Outcome) {
	f := c.f
	f.mu.Lock()
	first := !f.fired && f.kind != "healthy" && f.kind != "acquisition" && !strings.Contains(f.fault, "restoration fails") && f.fault != "terminal output blocks during exit"
	if first {
		f.fired = true
	}
	kind, child := f.kind, f.child
	f.mu.Unlock()
	if first {
		switch {
		case kind == "disconnect":
			switch f.fault {
			case "owned removal fails":
				f.l.m.e.Following = "still present"
			case "engine becomes unreachable":
				f.l.hidden, f.l.unknownExit = true, true
			case "inspected ownership changes":
				o := f.l.m.e.Resources[id]
				o.Name = "foreign-replacement"
				f.l.m.e.Resources[id] = o
			}
			f.in.Hangup(false)
		case f.fault == "terminal observation fails":
			f.in.Break()
		case f.fault == "resized terminal has zero rows":
			f.in.Resize(tp.Dimensions{Columns: 113})
		case strings.HasPrefix(f.fault, "running worker resize"):
			f.in.Resize(tp.Dimensions{Columns: 167, Rows: 53})
		case f.fault == "terminal input read fails":
			f.streams <- sp.ExecutionFailed
		default:
			panic(fmt.Sprintf("unknown terminal fault %q", f.fault))
		}
		<-ctx.Done()
		return 0, sp.ExecutionFailed
	}
	if child == "unknown" && kind != "healthy" {
		return 0, sp.ExecutionFailed
	}
	return c.RunConnection.Wait(ctx, id)
}
