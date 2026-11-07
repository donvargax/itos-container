package features

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestControlStateTrace(t *testing.T) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Skip("Linux amd64 syscall observer")
	}
	root, err := moduleRoot()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	trace, child := filepath.Join(dir, "slice-21-trace"), filepath.Join(dir, "slice-21-child")
	for path, source := range map[string]string{trace: controlTraceSource(), child: controlTraceChild} {
		if err := recoveryBuildHelper(root, path, source); err != nil {
			t.Fatal(err)
		}
	}
	for _, fault := range []string{"", "coordination directory create", "coordination directory fsync", "coordination parent fsync", "self-binding record write", "self-binding file fsync", "self-binding rename", "self-binding parent fsync"} {
		t.Run(fault, func(t *testing.T) {
			home := t.TempDir()
			log, pid := filepath.Join(dir, t.Name()[len("TestControlStateTrace/"):]+".log"), filepath.Join(dir, "pid")
			ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, trace, log, pid, home, child, home)
			cmd.Env = append(sessionChildEnvironment(os.Environ()), "SLICE_21_ACCOUNT_HOME="+home, "SLICE_21_FAULT="+fault)
			out, err := cmd.CombinedOutput()
			var exit *exec.ExitError
			if fault == "" && err != nil {
				t.Fatalf("positive observer: %v %s", err, out)
			}
			if fault != "" && (!errors.As(err, &exit) || exit.ExitCode() != 7) {
				t.Fatalf("fault was not delivered to its syscall: %v %s", err, out)
			}
			events, err := controlEvents(log)
			if err != nil {
				t.Fatal(err)
			}
			lock, failed := false, false
			for _, e := range events {
				if e.Op == "flock" && e.Path == home && e.Success {
					lock = true
				}
				if e.Phase == "end" && !e.Success {
					failed = true
				}
			}
			if !lock || failed != (fault != "") {
				t.Fatalf("incorrect lock/failure observations: %v", events)
			}
		})
	}
}

const controlTraceChild = `package main
import ("os"; "path/filepath"; "golang.org/x/sys/unix")
func must(err error) { if err != nil { os.Exit(7) } }
func sync(path string) { f,e:=os.Open(path); must(e); must(f.Sync()); must(f.Close()) }
func main() {
 home:=os.Args[1]; h,e:=os.Open(home); must(e); defer h.Close(); must(unix.Flock(int(h.Fd()),unix.LOCK_EX))
 root:=filepath.Join(home,".local/state/itos-container"); must(os.MkdirAll(root,0700))
 coord:=filepath.Join(root,"coordination"); must(os.Mkdir(coord,0700)); sync(root); sync(coord)
 tmp:=filepath.Join(coord,".self.tmp"); f,e:=os.OpenFile(tmp,os.O_CREATE|os.O_EXCL|os.O_WRONLY,0600); must(e)
 _,e=f.WriteString("{}"); must(e); must(f.Sync()); must(f.Close()); must(os.Rename(tmp,filepath.Join(coord,"self.json"))); sync(coord)
 must(unix.Flock(int(h.Fd()),unix.LOCK_UN))
}
`
