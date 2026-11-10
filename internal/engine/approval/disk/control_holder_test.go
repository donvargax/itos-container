package disk

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/donvargax/itos-container/internal/engine/approval/port/porttest"
)

// A command killed while it holds the account's control lock must not leave the
// account locked: a fresh status, stop or run that follows the loss may not wait
// on a lock nobody holds. The lock is a flock on the private lock file, so the
// kernel releases it with the process; nothing in the protected tree records an
// owner that a later command would have to outlive.
func TestControlLockDoesNotOutliveAKilledHolder(t *testing.T) {
	control, c, _, home, selected := controlFixture(t)
	for _, write := range []bool{true, false} {
		t.Run(map[bool]string{true: "writer", false: "reader"}[write], func(t *testing.T) {
			// The holder is a copy of this test binary: it takes the transaction
			// through the same adapter over the same folders, then waits to be killed.
			child := exec.Command(os.Args[0], "-test.run=^TestControlHolderChild$", "-test.count=1")
			child.Env = append(os.Environ(),
				"SLICE_21_HOLDER_HOME="+home, "SLICE_21_HOLDER_STATE="+filepath.Dir(selected),
				"SLICE_21_HOLDER_WRITE="+strconv.FormatBool(write), "SLICE_21_HOLDER_ROOT="+c.Root)
			stdout, err := child.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			if err = child.Start(); err != nil {
				t.Fatal(err)
			}
			killed := false
			t.Cleanup(func() {
				if !killed {
					_ = child.Process.Kill()
					_ = child.Wait()
				}
			})
			line, err := bufio.NewReader(stdout).ReadString('\n')
			if err != nil || line != "held\n" {
				t.Fatalf("the holder did not take its transaction: %q, %v", line, err)
			}
			// The holder really excludes this process. A writer is excluded by a
			// reader as well; a reader is excluded only by a writer.
			blocked, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
			tx, failure := control.Begin(blocked, c, true)
			cancel()
			if tx != nil {
				tx.Close()
			}
			if tx != nil || failure != "state.busy" {
				t.Fatalf("a writer began beside a live holder: %v, %q; want state.busy", tx, failure)
			}
			if err = child.Process.Kill(); err != nil {
				t.Fatal(err)
			}
			killed = true
			_ = child.Wait()
			// The next command starts at once, for the whole budget of a call.
			start := time.Now()
			after, failure := control.Begin(t.Context(), c, write)
			if failure != "" {
				t.Fatalf("a command after the holder was killed: %q", failure)
			}
			after.Close()
			if elapsed := time.Since(start); elapsed > 5*time.Second {
				t.Fatalf("a command after the holder was killed waited %v", elapsed)
			}
		})
	}
}

// TestControlHolderChild is the killed process of the test above, and does nothing
// when run as an ordinary test.
func TestControlHolderChild(t *testing.T) {
	home := os.Getenv("SLICE_21_HOLDER_HOME")
	if home == "" {
		t.Skip("run only as the holder of TestControlLockDoesNotOutliveAKilledHolder")
	}
	store, c, _ := diskFixture(t)
	store.StateHome = os.Getenv("SLICE_21_HOLDER_STATE")
	c.Root = os.Getenv("SLICE_21_HOLDER_ROOT")
	control := Control{Account: &porttest.Account{Home: home}, Store: store, Reservations: &porttest.Reservations{RootsByLocation: map[string][]string{}}}
	write, _ := strconv.ParseBool(os.Getenv("SLICE_21_HOLDER_WRITE"))
	tx, failure := control.Begin(t.Context(), c, write)
	if failure != "" {
		t.Fatal(failure)
	}
	defer tx.Close()
	_, _ = os.Stdout.WriteString("held\n")
	// Wait to be killed. A sleep, not a bare select, which the runtime would end
	// as a deadlock and release the lock the parent means to see held.
	time.Sleep(time.Minute)
}
