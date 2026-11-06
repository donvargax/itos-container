package features

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"
)

// This local observer contract has no engine connection. Positive and negative
// controls prevent a broken tracer, mere file visibility or failed fsync from
// masquerading as native recovery durability evidence.
func TestRecoveryTrace(t *testing.T) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Skip("the native recovery syscall observer targets Linux amd64")
	}
	root, err := moduleRoot()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	trace, child := filepath.Join(dir, "slice-11-trace"), filepath.Join(dir, "slice-11-child")
	for path, source := range map[string]string{trace: recoveryTraceSource, child: recoveryTraceControl} {
		if err := recoveryBuildHelper(root, path, source); err != nil {
			t.Fatal(err)
		}
	}
	for _, mode := range []string{"success", "unsynced", "parent-before-file", "failed-file-sync"} {
		t.Run(mode, func(t *testing.T) {
			state := filepath.Join(t.TempDir(), "state")
			if err := os.Mkdir(state, 0700); err != nil {
				t.Fatal(err)
			}
			journal := filepath.Join(t.TempDir(), "slice-11-sync.jsonl")
			pid := filepath.Join(t.TempDir(), "slice-11-pid")
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, trace, journal, pid, state, child, mode, state)
			cmd.Env = sessionChildEnvironment(os.Environ())
			output, err := cmd.CombinedOutput()
			var exit *exec.ExitError
			if !errors.As(err, &exit) || exit.ExitCode() != 7 || string(output) != "slice-11 trace control\n" {
				t.Fatalf("observer did not preserve child result: %v (%s)", err, output)
			}
			want := map[string]any{"schema": 1, "state": "stopped"}
			err = recoveryDurableFile(filepath.Join(state, "record.json"), want, []*recoveryProcess{{journal: journal}})
			if (err == nil) != (mode == "success") {
				t.Fatalf("durability control %s: %v", mode, err)
			}
		})
	}
	t.Run("controller-death", func(t *testing.T) {
		state := t.TempDir()
		pidFile, journal := filepath.Join(state, "slice-11-pid"), filepath.Join(state, "slice-11-journal")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, trace, journal, pidFile, state, child, "wait", state)
		cmd.Env = sessionChildEnvironment(os.Environ())
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		ticker := time.NewTicker(5 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case err := <-done:
				t.Fatalf("observer exited before the controller-loss barrier: %v", err)
			case <-ctx.Done():
				<-done
				t.Fatal("observer did not publish its child's PID")
			case <-ticker.C:
				data, err := os.ReadFile(pidFile)
				if err != nil {
					continue
				}
				pid, err := strconv.Atoi(string(data))
				if err != nil {
					continue
				}
				process, err := os.FindProcess(pid)
				if err != nil {
					t.Fatal(err)
				}
				err = process.Kill()
				_ = process.Release()
				if err != nil {
					t.Fatal(err)
				}
				var exit *exec.ExitError
				if err = <-done; !errors.As(err, &exit) || exit.ExitCode() != 137 {
					t.Fatalf("observer did not retain uncatchable controller loss: %v", err)
				}
				return
			}
		}
	})
}

const recoveryTraceControl = `package main
import ("fmt"; "os"; "path/filepath"; "runtime"; "time"; "golang.org/x/sys/unix")
func must(err error) { if err != nil { fmt.Fprintln(os.Stderr, err); os.Exit(95) } }
func main() {
    mode, root := os.Args[1], os.Args[2]
    if mode == "wait" { for { time.Sleep(time.Second) } }
    done := make(chan struct{})
    go func() {
        // Exercise a traced runtime thread as well as the original PID.
        runtime.LockOSThread()
        defer close(done)
        temp := filepath.Join(root, "private-record")
        file, err := os.OpenFile(temp, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0600); must(err)
        _, err = file.WriteString("{\"schema\":1,\"state\":\"stopped\"}"); must(err)
        parent, err := os.Open(root); must(err)
        switch mode {
        case "success": must(file.Sync())
        case "failed-file-sync":
            fd, err := unix.Open(temp, unix.O_PATH|unix.O_CLOEXEC, 0); must(err)
            if unix.Fsync(fd) == nil { os.Exit(94) }
            must(unix.Close(fd))
        }
        must(os.Rename(temp, filepath.Join(root,"record.json")))
        if mode != "unsynced" { must(parent.Sync()) }
        if mode == "parent-before-file" { must(file.Sync()) }
        must(file.Close()); must(parent.Close())
    }()
    <-done
    fmt.Println("slice-11 trace control")
    os.Exit(7)
}
`
