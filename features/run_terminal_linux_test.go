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
	"syscall"
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
	local             bool
	outputMaster      *os.File
	outputSlave       *os.File
	outputSaved       *unix.Termios
	attempts          []terminalAttempt
	action            func(*exec.Cmd)
	payload           []byte
	contract          *terminalContract
	alias             string
}

type terminalAttempt struct {
	exit            int
	stdout, stderr  string
	connections     int64
	statesRestored  bool
	stateError      error
	settingsChanged bool
	requests        []doctorRequest
}

func (w *world) runTerminalSteps(sc *godog.ScenarioContext) {
	var fixture *terminalInvocation
	w.terminalLocalSteps(sc, &fixture)
	w.terminalNativeSteps(sc, &fixture)
	w.terminalContractSteps(sc, func(contract *terminalContract) { fixture = &terminalInvocation{w: w, contract: contract} })
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
		if fixture.timedOut || w.exit != exit {
			return fmt.Errorf("expected JSON refusal exit %d, timed out=%t: %s", exit, fixture.timedOut, w.report())
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
	if err := f.bindDescriptors(input, output); err != nil {
		return nil, err
	}
	f.connections = w.doctor.engine.listener.connections.Load()
	return f, w.runCheckpoint()
}

func (f *terminalInvocation) bindDescriptors(input, output string) error {
	keep := func(file *os.File) { f.w.doctor.cleanup = append(f.w.doctor.cleanup, func() { _ = file.Close() }) }
	if input == "terminal" || output == "terminal" {
		var err error
		f.master, f.slave, err = pty.Open()
		if err != nil {
			return err
		}
		keep(f.master)
		keep(f.slave)
		if err := pty.Setsize(f.slave, &pty.Winsize{Cols: 113, Rows: 37}); err != nil {
			return err
		}
		state, err := unix.IoctlGetTermios(int(f.slave.Fd()), unix.TCGETS)
		if err != nil {
			return err
		}
		// Deliberately nondefault state, including output translation. The
		// controller, not the fixture, must decide whether to acquire raw mode.
		state.Cc[unix.VEOF] = 7
		state.Oflag = unix.OPOST | unix.ONLCR
		if err := unix.IoctlSetTermios(int(f.slave.Fd()), unix.TCSETS, state); err != nil {
			return err
		}
		f.saved = state
	}
	if input == "terminal" {
		f.input = f.slave
	} else {
		read, write, err := os.Pipe()
		if err != nil {
			return err
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
			return err
		}
		keep(read)
		keep(write)
		f.output, f.outputRead = write, read
	}
	return nil
}

func (f *terminalInvocation) invoke(args, inherited string) error {
	w := f.w
	words, err := commandWords(args)
	if err != nil {
		return err
	}
	bin := w.bin
	env := setEnv(w.env(), "ITOS_JSON", inherited)
	if f.local {
		bin, err = w.sessionProbeBinary()
		if err != nil {
			return err
		}
		words = append([]string{"-test.run=^TestLocalRunCommand$", "-test.count=1", "--"}, words...)
		env = append(env, "SLICE_17_LOCAL_RUN=1", "SLICE_17_LOCAL_MOUNT_ROOT="+w.doctor.checkout)
	}
	guard := 5 * time.Second
	if terminal := w.doctor.engine.offline.terminal; terminal != nil {
		// A delay the fixture injects into /start is harness bookkeeping, not a
		// looser bound: no product deadline or other scenario's guard changes.
		guard += terminal.startDelay
	}
	if terminal := w.doctor.engine.offline.terminal; terminal != nil && terminal.fault == "start accepted but its reply times out" {
		// The ordinary SDK keeps its real ten-second finite-call deadline.
		// A harness kill must not replace that failure or the 30 s cleanup total.
		guard = 45 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), guard)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, words...)
	cmd.Dir, cmd.Env = w.doctor.cwd, env
	if f.alias != "" {
		cmd.Env = setEnv(cmd.Env, "SLICE_37_TERMINAL_ALIAS", f.alias)
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
	}
	for key, value := range map[string]string{"COLUMNS": "7", "LINES": "9", "TERM": "slice-37-misleading"} {
		cmd.Env = setEnv(cmd.Env, key, value)
	}
	var stderr strings.Builder
	cmd.Stdin, cmd.Stdout, cmd.Stderr = f.input, f.output, &stderr
	cmd.WaitDelay = time.Second
	connections := w.doctor.engine.listener.connections.Load()
	f.payload, f.changed = nil, false
	a := w.doctor.engine.offline
	a.mu.Lock()
	if err := cmd.Start(); err != nil {
		a.mu.Unlock()
		return err
	}
	for i, arg := range words {
		if arg == "--" && (!f.local || i > 2) {
			a.expected[cmd.Process.Pid] = append([]string{}, words[i+1:]...)
			break
		}
	}
	a.mu.Unlock()
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
			if err := f.readAvailable(); err != nil {
				cancel()
				<-done
				return err
			}
			if f.action != nil {
				f.action(cmd)
			}
			if f.slave != nil {
				state, err := unix.IoctlGetTermios(int(f.slave.Fd()), unix.TCGETS)
				f.changed = f.changed || err != nil || !reflect.DeepEqual(state, f.saved)
			}
		}
	}
	f.timedOut = ctx.Err() != nil
	if terminal := a.terminal; terminal != nil && terminal.fault == "host SIGTERM during preflight" {
		w.doctor.engine.unpause()
	}
	w.exit, w.stderr = cmd.ProcessState.ExitCode(), stderr.String()
	if f.outputRead != nil {
		_ = f.output.Close()
		data, err := io.ReadAll(f.outputRead)
		w.stdout = string(data)
		if err != nil {
			return err
		}
	} else {
		if err := f.readAvailable(); err != nil {
			return err
		}
		w.stdout = string(f.payload)
	}
	observed := terminalAttempt{exit: w.exit, stdout: w.stdout, stderr: w.stderr, connections: w.doctor.engine.listener.connections.Load() - connections, statesRestored: f.restored() == nil, settingsChanged: f.changed, stateError: w.runSameTree(false)}
	for _, request := range w.doctor.engine.requests() {
		if request.PID == cmd.Process.Pid {
			observed.requests = append(observed.requests, request)
		}
	}
	f.attempts = append(f.attempts, observed)
	return nil
}

func (f *terminalInvocation) readAvailable() error {
	master := f.master
	if f.outputMaster != nil {
		master = f.outputMaster
	}
	if master == nil || f.outputRead != nil {
		return nil
	}
	// Keep the slave open for exact-state inspection. After child exit, poll
	// and read the remaining small refusal without waiting for slave EOF.
	for {
		poll := []unix.PollFd{{Fd: int32(master.Fd()), Events: unix.POLLIN}}
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
		n, err := master.Read(chunk[:])
		if err != nil {
			return err
		}
		f.payload = append(f.payload, chunk[:n]...)
	}
	return nil
}

func (f *terminalInvocation) restored() error {
	for file, want := range map[*os.File]*unix.Termios{f.slave: f.saved, f.outputSlave: f.outputSaved} {
		if file == nil {
			continue
		}
		state, err := unix.IoctlGetTermios(int(file.Fd()), unix.TCGETS)
		if err != nil || !reflect.DeepEqual(state, want) {
			return fmt.Errorf("terminal %s was not restored: %v", file.Name(), err)
		}
		if _, err := file.Stat(); err != nil {
			return err
		}
	}
	return nil
}

func terminalResize(cmd *exec.Cmd, file *os.File, columns, rows uint16) error {
	if err := pty.Setsize(file, &pty.Winsize{Cols: columns, Rows: rows}); err != nil {
		return err
	}
	return cmd.Process.Signal(syscall.SIGWINCH)
}

func (f *terminalInvocation) refusal(rule string, exit int) error {
	message := rule
	if rule == "run.command-required" {
		// The existing plain refusal exposes its message, while its rule ID
		// belongs to structured output (which run itself refuses).
		message = "run requires a nonempty command after --; use itos container run -- <command>"
	}
	if f.timedOut || f.w.exit != exit || !strings.Contains(f.w.stderr, message) {
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
