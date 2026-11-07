package features

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

func (w *world) controlLockSteps(sc *godog.ScenarioContext) {
	var target string
	var elapsed time.Duration
	sc.Step(`^a readable account home and valid inputs for locked-home "([^"]*)"$`, func(command string) error {
		f := w.control
		target = f.home
		if err := os.Chmod(f.home, 0755); err != nil {
			return err
		}
		if command == "run" {
			return f.p.approvalSeed("1", false)
		}
		return nil
	})
	sc.Step(`^valid "([^"]*)" inputs and a private coordination lock held by another process$`, func(command string) error {
		f := w.control
		if command == "run" {
			if err := f.p.approvalSeed("1", false); err != nil {
				return err
			}
		}
		if _, err := f.seedBinding(f.fixedRoot()); err != nil {
			return err
		}
		target = filepath.Join(f.coordination(), "control.lock")
		return os.WriteFile(target, nil, 0600)
	})
	run := func(command string) {
		f := w.control
		release, err := f.holdControlLock(target)
		if err != nil {
			f.err = err
			return
		}
		defer release()
		f.before, f.err = f.inventory()
		started := time.Now()
		if command == "run" && target == f.home {
			f.workers.first = f.workers.start("P", "", "")
			f.workers.observe(f.workers.first, false)
			f.err = errors.Join(f.err, f.workers.live(f.workers.first))
		} else if command == "run" {
			f.err = errors.Join(f.err, f.p.runDoctor(f.p.env(), "run", "--", "slice-21-workload"))
		} else {
			f.inspect(command, "JSON")
		}
		elapsed = time.Since(started)
	}
	sc.Step(`^P invokes "([^"]*)" while an independent read-only descriptor holds the home lock$`, run)
	sc.Step(`^P invokes "([^"]*)" while that private lock remains held$`, run)
	sc.Step(`^the command makes its expected progress before the home lock is released$`, func() error {
		f := w.control
		if f.err != nil {
			return f.err
		}
		if elapsed >= 5*time.Second {
			return fmt.Errorf("home lock delayed command for %s", elapsed)
		}
		if f.workers.first != nil {
			return f.workers.live(f.workers.first)
		}
		return f.p.exitsWith(0)
	})
	sc.Step(`^P reports "([^"]*)" with exit 75 within the ten-second call budget without writes or worker creation$`, func(rule string) error {
		f := w.control
		if err := errors.Join(f.err, f.p.exitsWith(75)); err != nil {
			return err
		}
		if !strings.Contains(f.p.stdout+f.p.stderr, rule) || elapsed < 9*time.Second || elapsed > 12*time.Second {
			return fmt.Errorf("missing typed call-budget refusal after %s: %s %s", elapsed, f.p.stdout, f.p.stderr)
		}
		return errors.Join(f.unchanged(), f.inspectionOnly())
	})
}

// The helper receives only read access, exactly the capability another account
// has on a readable home. No UID-changing or privileged helper is required.
func (f *controlFixture) holdControlLock(path string) (func(), error) {
	helper := f.p.bin + "-slice-21-lock"
	if _, err := os.Stat(helper); err != nil {
		if err = recoveryBuildHelper(f.p.root, helper, controlLockSource); err != nil {
			return nil, err
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	cmd := exec.CommandContext(ctx, helper, path)
	in, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		_ = in.Close()
		cancel()
		return nil, err
	}
	if err = cmd.Start(); err != nil {
		_ = in.Close()
		cancel()
		return nil, err
	}
	release := func() { _ = in.Close(); _ = cmd.Wait(); cancel() }
	ready, err := bufio.NewReader(out).ReadString('\n')
	if err != nil || ready != "locked\n" {
		release()
		return nil, fmt.Errorf("home-lock helper did not acquire its descriptor: %q %v", ready, err)
	}
	return release, nil
}

const controlLockSource = `package main
import ("os"; "fmt"; "io"; "golang.org/x/sys/unix")
func main() {
 f, err := os.Open(os.Args[1]); if err != nil { panic(err) }; defer f.Close()
 if err = unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil { panic(err) }
 fmt.Println("locked")
 io.Copy(io.Discard, os.Stdin)
}`
