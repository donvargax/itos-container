package features

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"time"

	"github.com/creack/pty"
	"github.com/cucumber/godog"
	"golang.org/x/sys/unix"
)

// These invocation fixtures own disposable descriptors, never the operator's
// terminal. The approval is seeded independently; no Given runs the product.
type terminalInvocation struct {
	w                 *world
	input, output     *os.File
	master, slave     *os.File
	outputRead        *os.File
	saved             *unix.Termios
	connections       int64
	timedOut, changed bool
}

func (w *world) runTerminalSteps(sc *godog.ScenarioContext) {
	var fixture *terminalInvocation
	sc.Step(`^a local terminal-run fixture with (terminal|pipe) stdin and (terminal|pipe) stdout$`, func(input, output string) error {
		var err error
		fixture, err = w.terminalInvocationFixture(input, output)
		return err
	})
	sc.Step(`^the terminal fixture invokes "([^"]*)"$`, func(args string) error { return fixture.invoke(args, "0") })
	sc.Step(`^the terminal fixture invokes "([^"]*)" with inherited ITOS_JSON "([^"]*)"$`, func(args, inherited string) error { return fixture.invoke(args, inherited) })
	sc.Step(`^terminal run reports "([^"]*)" with exit (\d+) and terminal-or-omit-tty guidance$`, func(rule string, exit int) error {
		if err := fixture.refusal(rule, exit); err != nil {
			return err
		}
		if !strings.Contains(w.stderr, "terminal") || !strings.Contains(w.stderr, "--tty") || !strings.Contains(strings.ToLower(w.stderr), "omit") {
			return fmt.Errorf("missing terminal-or-omit-tty guidance: %s", w.report())
		}
		return nil
	})
	sc.Step(`^terminal run reports "([^"]*)" with exit (\d+) and explicit-command guidance$`, func(rule string, exit int) error {
		if err := fixture.refusal(rule, exit); err != nil {
			return err
		}
		if !strings.Contains(w.stderr, "nonempty") || !strings.Contains(w.stderr, "after --") {
			return fmt.Errorf("missing explicit-command guidance: %s", w.report())
		}
		return nil
	})
	sc.Step(`^terminal run emits exactly one schema-1 JSON refusal for "([^"]*)" and exits (\d+)$`, func(rule string, exit int) error {
		if err := fixture.refusal(rule, exit); err != nil {
			return err
		}
		return w.runJSONRefusal(rule, exit)
	})
	sc.Step(`^terminal run explains removal of wrapper JSON and ITOS_JSON on its separate diagnostic channel$`, func() error {
		if !strings.Contains(w.stderr, "JSON") || !strings.Contains(w.stderr, "--json") || !strings.Contains(w.stderr, "ITOS_JSON=1") || !strings.Contains(strings.ToLower(w.stderr), "remove") {
			return fmt.Errorf("missing separate JSON refusal guidance: %s", w.report())
		}
		return nil
	})
	sc.Step(`^terminal run emits no stdout and changes no terminal settings checkout or host state$`, func() error {
		if w.stdout != "" {
			return fmt.Errorf("refusal emitted stdout: %s", w.report())
		}
		return fixture.unchanged()
	})
	sc.Step(`^terminal run changes no terminal settings checkout or host state$`, func() error { return fixture.unchanged() })
	sc.Step(`^terminal run makes no engine connection or helper execution$`, func() error {
		if w.doctor.engine.listener.connections.Load() != fixture.connections || len(w.doctor.engine.requests()) != 0 {
			return fmt.Errorf("invocation refusal reached the engine")
		}
		return w.doctorNoMarker()
	})
	sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		fixture = nil
		return ctx, nil
	})
}

func (w *world) terminalInvocationFixture(input, output string) (*terminalInvocation, error) {
	if err := w.approvalCheckout(); err != nil {
		return nil, err
	}
	w.localOfflineFixture()
	if err := w.approvalSeed("1", false); err != nil {
		return nil, err
	}
	f := &terminalInvocation{w: w}
	keep := func(file *os.File) { w.doctor.cleanup = append(w.doctor.cleanup, func() { _ = file.Close() }) }
	if input == "terminal" || output == "terminal" {
		var err error
		f.master, f.slave, err = pty.Open()
		if err != nil {
			return nil, err
		}
		keep(f.master)
		keep(f.slave)
		if err := pty.Setsize(f.slave, &pty.Winsize{Cols: 113, Rows: 37}); err != nil {
			return nil, err
		}
		state, err := unix.IoctlGetTermios(int(f.slave.Fd()), unix.TCGETS)
		if err != nil {
			return nil, err
		}
		// Deliberately nondefault state, including output translation. The
		// controller, not the fixture, must decide whether to acquire raw mode.
		state.Cc[unix.VEOF] = 7
		state.Oflag = unix.OPOST | unix.ONLCR
		if err := unix.IoctlSetTermios(int(f.slave.Fd()), unix.TCSETS, state); err != nil {
			return nil, err
		}
		f.saved = state
	}
	if input == "terminal" {
		f.input = f.slave
	} else {
		read, write, err := os.Pipe()
		if err != nil {
			return nil, err
		}
		keep(read)
		keep(write)
		f.input = read
	}
	if output == "terminal" {
		f.output = f.slave
	} else {
		read, write, err := os.Pipe()
		if err != nil {
			return nil, err
		}
		keep(read)
		keep(write)
		f.output, f.outputRead = write, read
	}
	f.connections = w.doctor.engine.listener.connections.Load()
	return f, w.runCheckpoint()
}

func (f *terminalInvocation) invoke(args, inherited string) error {
	w := f.w
	words, err := commandWords(args)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, w.bin, words...)
	cmd.Dir, cmd.Env = w.doctor.cwd, setEnv(w.env(), "ITOS_JSON", inherited)
	for key, value := range map[string]string{"COLUMNS": "7", "LINES": "9", "TERM": "slice-37-misleading"} {
		cmd.Env = setEnv(cmd.Env, key, value)
	}
	var stderr strings.Builder
	cmd.Stdin, cmd.Stdout, cmd.Stderr = f.input, f.output, &stderr
	cmd.WaitDelay = time.Second
	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
waiting:
	for {
		select {
		case <-done:
			break waiting
		case <-ticker.C:
			if f.slave != nil {
				state, err := unix.IoctlGetTermios(int(f.slave.Fd()), unix.TCGETS)
				f.changed = f.changed || err != nil || !reflect.DeepEqual(state, f.saved)
			}
		}
	}
	f.timedOut = ctx.Err() != nil
	w.exit, w.stderr = cmd.ProcessState.ExitCode(), stderr.String()
	if f.outputRead != nil {
		_ = f.output.Close()
		data, err := io.ReadAll(f.outputRead)
		w.stdout = string(data)
		return err
	}
	// Keep the slave open for exact-state inspection. After child exit, poll
	// and read the remaining small refusal without waiting for slave EOF.
	var data []byte
	for {
		poll := []unix.PollFd{{Fd: int32(f.master.Fd()), Events: unix.POLLIN}}
		_, err := unix.Poll(poll, 0)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			return err
		}
		if poll[0].Revents&unix.POLLIN == 0 {
			break
		}
		var chunk [4096]byte
		n, err := f.master.Read(chunk[:])
		if err != nil {
			return err
		}
		data = append(data, chunk[:n]...)
	}
	w.stdout = string(data)
	return nil
}

func (f *terminalInvocation) refusal(rule string, exit int) error {
	if f.timedOut || f.w.exit != exit || !strings.Contains(f.w.stderr, rule) {
		return fmt.Errorf("expected %s with exit %d, timed out=%t: %s", rule, exit, f.timedOut, f.w.report())
	}
	return nil
}

func (f *terminalInvocation) unchanged() error {
	if f.slave != nil {
		state, err := unix.IoctlGetTermios(int(f.slave.Fd()), unix.TCGETS)
		if err != nil || !reflect.DeepEqual(state, f.saved) || f.changed {
			return fmt.Errorf("invocation refusal changed the saved terminal state: %v", err)
		}
	}
	return f.w.runSameTree(false)
}
