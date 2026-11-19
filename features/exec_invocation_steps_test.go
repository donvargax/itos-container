package features

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

// The invocation rows exercise the real parser in a disposable project. The
// observer is armed only during the product process, so fixture reads cannot
// supply or obscure evidence about validation touching protected state.
func (w *world) execInvocationSteps(sc *godog.ScenarioContext) {
	var before map[string]runEntry
	var accesses []string
	var connections int64
	sc.Step(`^an exec invocation fixture with isolated state and access recorders$`, func() error {
		if err := w.approvalCheckout(); err != nil {
			return err
		}
		if err := w.approvalSeed("1", false); err != nil {
			return err
		}
		if err := w.doctorSentinels(); err != nil {
			return err
		}
		var err error
		before, err = runTree(w.dir)
		return err
	})
	sc.Step(`^the exec CLI receives "([^"]*)" with ITOS_JSON "([^"]*)"$`, func(arguments, inherited string) error {
		args, err := commandWords(arguments)
		if err != nil {
			return err
		}
		for i, arg := range args {
			if arg == "SESSION" {
				args[i] = strings.Repeat("a", 64)
			}
		}
		watch, err := execInvocationWatch(before, w.doctor.env["XDG_STATE_HOME"])
		if err != nil {
			return err
		}
		connections = w.doctor.engine.listener.connections.Load()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, w.bin, args...)
		cmd.Dir = w.doctor.cwd
		cmd.Env = setEnv(w.env(), "ITOS_JSON", inherited)
		var stdout, stderr strings.Builder
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		cmd.WaitDelay = time.Second
		runErr := cmd.Run()
		accesses, err = watch.finish()
		w.stdout, w.stderr = stdout.String(), stderr.String()
		w.exit = -1
		if cmd.ProcessState != nil {
			w.exit = cmd.ProcessState.ExitCode()
		}
		var exit *exec.ExitError
		if runErr != nil && !errors.As(runErr, &exit) {
			return errors.Join(err, runErr)
		}
		return err
	})
	sc.Step(`^exec invocation validation accessed no session state or engine and changed no files$`, func() error {
		if len(accesses) != 0 || w.doctor.engine.listener.connections.Load() != connections {
			return fmt.Errorf("exec validation accessed protected state or the engine")
		}
		after, err := runTree(w.dir)
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(before, after) {
			return fmt.Errorf("exec invocation validation changed fixture files")
		}
		return nil
	})
	sc.Step(`^exec reports exactly one JSON refusal "([^"]*)" with exit (\d+) and removal guidance$`, func(rule string, exit int) error {
		var report struct {
			Schema   int
			OK       *bool
			Problems []struct {
				Rule, Message, Fix string
				Exit               int
			}
		}
		if err := json.Unmarshal([]byte(w.stdout), &report); err != nil {
			return fmt.Errorf("exec did not return one JSON refusal: %w\n%s", err, w.report())
		}
		if report.Schema != 1 || report.OK == nil || *report.OK || len(report.Problems) != 1 || w.exit != exit {
			return fmt.Errorf("exec returned the wrong refusal envelope\n%s", w.report())
		}
		p := report.Problems[0]
		if p.Rule != rule || p.Exit != exit || p.Message == "" ||
			!strings.Contains(strings.ToLower(p.Fix), "remove") ||
			!strings.Contains(p.Fix, "--json") || !strings.Contains(p.Fix, "ITOS_JSON=1") ||
			!strings.Contains(w.stderr, rule) || !strings.Contains(w.stderr, "JSON") {
			return fmt.Errorf("exec refusal omitted its rule, explanation or removal guidance\n%s", w.report())
		}
		return nil
	})
}

func execInvocationWatch(tree map[string]runEntry, state string) (*sessionReadWatch, error) {
	fd, err := sessionInotifyInit()
	if err != nil {
		return nil, err
	}
	watch := &sessionReadWatch{file: os.NewFile(uintptr(fd), "slice-38-state-watch"), paths: map[int32]string{}}
	for path := range tree {
		if path != state && !strings.HasPrefix(path, state+string(filepath.Separator)) {
			continue
		}
		id, err := sessionInotifyAdd(fd, path)
		if err != nil {
			_ = watch.file.Close()
			return nil, err
		}
		watch.paths[int32(id)] = path
	}
	if len(watch.paths) == 0 {
		_ = watch.file.Close()
		return nil, fmt.Errorf("exec fixture has no observed state")
	}
	return watch, nil
}
