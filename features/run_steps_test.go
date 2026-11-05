package features

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/creack/pty"
	"github.com/cucumber/godog"
	"golang.org/x/term"
)

func (w *world) runSteps(sc *godog.ScenarioContext) {
	sc.Step(`^its standard output is empty$`, func() error {
		if w.stdout != "" {
			return fmt.Errorf("expected zero stdout bytes\n%s", w.report())
		}
		return nil
	})
	sc.Step(`^its run JSON reports exactly one refusal with rule "([^"]*)" and exit (\d+)$`, w.runJSONRefusal)
	sc.Step(`^itos-container runs at a terminal with "([^"]*)" and the environment "([^"]*)"$`, func(args, vars string) error {
		return w.runAtTerminal(w.withEnv(vars), args)
	})
}

func (w *world) runJSONRefusal(rule string, code int) error {
	// Unmarshal consumes the entire stream, rejecting prose and second documents.
	var object map[string]any
	if err := json.Unmarshal([]byte(w.stdout), &object); err != nil {
		return fmt.Errorf("expected exactly one run refusal object: %w\n%s", err, w.report())
	}
	problems, ok := object["problems"].([]any)
	if object["schema"] != float64(1) || object["ok"] != false || !ok || len(problems) != 1 {
		return fmt.Errorf("incorrect run refusal envelope\n%s", w.report())
	}
	problem, ok := problems[0].(map[string]any)
	if !ok || problem["rule"] != rule || problem["exit"] != float64(code) {
		return fmt.Errorf("incorrect run refusal problem\n%s", w.report())
	}
	message, _ := problem["message"].(string)
	fix, _ := problem["fix"].(string)
	if !strings.Contains(message, "run") || !strings.Contains(message, "JSON") ||
		!strings.Contains(strings.ToLower(message), "not support") ||
		!strings.Contains(strings.ToLower(fix), "remove") ||
		!strings.Contains(fix, "--json") || !strings.Contains(fix, "ITOS_JSON=1") {
		return fmt.Errorf("run refusal needs JSON explanation and removal guidance\n%s", w.report())
	}
	return nil
}

// Both child descriptors are real terminal files, verified before launch. Stderr
// has a separate pipe; terminal line endings are accepted as JSON whitespace.
func (w *world) runAtTerminal(env []string, args string) error {
	fields, err := commandWords(args)
	if err != nil {
		return err
	}
	master, terminal, err := pty.Open()
	if err != nil {
		return fmt.Errorf("allocate terminal: %w", err)
	}
	defer func() { _ = master.Close() }()
	defer func() { _ = terminal.Close() }()
	if !term.IsTerminal(int(terminal.Fd())) {
		return errors.New("PTY child descriptors are not a terminal")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, w.bin, fields...)
	cmd.Dir = w.dir
	if w.policy != nil {
		cmd.Dir = w.policy.cwd
	}
	cmd.Env = env
	cmd.Stdin, cmd.Stdout = terminal, terminal
	var stderr strings.Builder
	cmd.Stderr = &stderr
	cmd.WaitDelay = time.Second
	if err := cmd.Start(); err != nil {
		return err
	}
	// Only the child keeps the slave open; its exit ends the output drain.
	_ = terminal.Close()
	type output struct {
		data []byte
		err  error
	}
	drained := make(chan output, 1)
	go func() {
		data, err := io.ReadAll(master)
		drained <- output{data, err}
	}()
	err = cmd.Wait() // CommandContext kills and reaps a child that exceeds the budget.
	w.stderr = stderr.String()
	w.exit = cmd.ProcessState.ExitCode()
	var exit *exec.ExitError
	if err != nil && !errors.As(err, &exit) {
		return err
	}
	select {
	case out := <-drained:
		w.stdout = string(out.data)
		// Linux signals the final slave close with EIO rather than EOF.
		if out.err != nil && !errors.Is(out.err, syscall.EIO) {
			return out.err
		}
	case <-ctx.Done():
		return errors.New("terminal output did not drain before the harness deadline")
	}
	return ctx.Err()
}
