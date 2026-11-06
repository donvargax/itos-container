package features

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

// These controls execute only the trusted socket fixture on the test host.
// They never run the product, use an engine, spoof CI, or supply product red
// evidence. Reachable sockets are deliberately required in these controls.
func TestSocketProbeControls(t *testing.T) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Skip("trusted probe uses Linux amd64 Unix socket syscalls")
	}
	root, err := moduleRoot()
	if err != nil {
		t.Fatal(err)
	}
	probe := filepath.Join(t.TempDir(), "slice-19-probe")
	if err = buildSocketProbe(root, probe); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(removeCallerPath)
	for _, mode := range []string{"existing", "late", "replace"} {
		t.Run(mode, func(t *testing.T) {
			s := socketLocalFixture(t, root, probe, mode)
			socketLocalRun(t, s)
			if err := socketInventory(s.config, s.reports, s.w.approval.uid, s.w.approval.gid); err != nil {
				t.Fatal(err)
			}
			if err := s.hostControls(); err != nil {
				t.Fatal(err)
			}
			if err := socketPrivate(s.config, s.reports, false); err != nil {
				t.Fatal(err)
			}
			listener, err := s.listener.output.snapshot()
			if err != nil {
				t.Fatal(err)
			}
			if err = socketDenied(s.config, s.reports, listener); err == nil || !strings.Contains(err.Error(), "checkout host sockets were reachable") {
				t.Fatalf("reachable controls did not fail the denial assertion: %v", err)
			}
			for _, r := range s.reports {
				if r.Kind == "attempts" {
					for _, a := range r.Attempts {
						if !socketPositive(a) {
							t.Fatalf("trusted same-path control failed: %+v", a)
						}
					}
				}
			}
			socketRejectIncomplete(t, s)
		})
	}
}

func socketLocalFixture(t *testing.T, root, probe, mode string) *socketWorld {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "slice-19-control-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	checkout := filepath.Join(dir, "c")
	w := &world{root: root, dir: dir, doctor: &doctorWorld{checkout: checkout},
		approval: &approvalWorld{uid: strconv.Itoa(os.Geteuid()), gid: strconv.Itoa(os.Getegid())}}
	s := &socketWorld{w: w, probe: probe, config: socketConfig{Nonce: isolationNonce(), Mode: mode, Root: checkout, Targets: socketTargets(mode)},
		original: map[string]sessionRecord{}, publications: map[string]socketIdentity{}}
	w.sockets = s
	t.Cleanup(func() {
		if err := w.closeSockets(); err != nil {
			t.Error(err)
		}
	})
	for _, target := range s.config.Targets {
		path := filepath.Join(checkout, target.Path)
		if mode != "late" || strings.HasPrefix(target.Path, "existing/") {
			if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				t.Fatal(err)
			}
		}
		if mode == "replace" {
			if err = os.WriteFile(path, []byte(s.config.Nonce+"/original/"+target.Name), 0600); err != nil {
				t.Fatal(err)
			}
			s.original[target.Name], err = sessionReadRecord(path)
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	return s
}

func socketLocalRun(t *testing.T, s *socketWorld) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	control := func(phase string) {
		t.Helper()
		if err := s.hostControl(ctx, phase); err != nil {
			t.Fatal(err)
		}
	}
	if s.config.Mode == "existing" {
		if err := s.publish(ctx); err != nil {
			t.Fatal(err)
		}
		control("first-before")
	}
	b, err := json.Marshal(s.config)
	if err != nil {
		t.Fatal(err)
	}
	p, err := startSocketProcess(ctx, s.probe, []string{"worker", string(b)}, s.config.Root, s.w.env())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.input.Close(); cancel(); <-p.done }()
	wait := func(phase string) {
		t.Helper()
		if _, err := p.await(ctx, "barrier", phase, s.config.Nonce); err != nil {
			t.Fatal(err)
		}
	}
	release := func(phase string) {
		t.Helper()
		if err := p.release(s.config.Nonce, phase); err != nil {
			t.Fatal(err)
		}
	}
	if s.config.Mode == "existing" {
		wait("first")
		control("first-after")
		control("running-before")
		release("first")
	} else {
		wait("live")
		if err := s.publish(ctx); err != nil {
			t.Fatal(err)
		}
		control("parent-before")
		release("live")
		wait("parent")
		control("parent-after")
		control("child-before")
		release("parent")
	}
	wait("exit")
	last := "child-after"
	if s.config.Mode == "existing" {
		last = "running-after"
	}
	control(last)
	edit, err := os.ReadFile(filepath.Join(s.config.Root, "slice-19-edit"))
	if err != nil || string(edit) != s.config.Nonce {
		t.Fatalf("probe failed its pre-exit edit: %q %v", edit, err)
	}
	select {
	case <-p.done:
		t.Fatal("probe exited before its final release")
	default:
	}
	release("exit")
	_ = p.input.Close()
	<-p.done
	if p.err != nil || p.exit != 0 || len(p.stderr.bytes()) != 0 {
		t.Fatalf("trusted probe failed: %v exit %d stderr %s", p.err, p.exit, p.stderr.bytes())
	}
	s.reports, err = p.output.snapshot()
	if err != nil {
		t.Fatal(err)
	}
}

func socketCloneReports(t *testing.T, reports []socketReport) []socketReport {
	t.Helper()
	b, err := json.Marshal(reports)
	if err != nil {
		t.Fatal(err)
	}
	var result []socketReport
	if err = json.Unmarshal(b, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func socketRejectIncomplete(t *testing.T, s *socketWorld) {
	t.Helper()
	for name, change := range map[string]func([]socketReport) []socketReport{
		"missing reports": func([]socketReport) []socketReport { return nil },
		"missing phase":   func(r []socketReport) []socketReport { return r[1:] },
		"foreign nonce":   func(r []socketReport) []socketReport { r[0].Nonce = "foreign"; return r },
		"missing operation": func(r []socketReport) []socketReport {
			for i := range r {
				if r[i].Kind == "attempts" {
					r[i].Attempts = r[i].Attempts[1:]
					break
				}
			}
			return r
		},
		"duplicate operation": func(r []socketReport) []socketReport {
			for i := range r {
				if r[i].Kind == "attempts" {
					r[i].Attempts[1] = r[i].Attempts[0]
					break
				}
			}
			return r
		},
		"missing path is not denial": func(r []socketReport) []socketReport {
			for i := range r {
				if r[i].Kind == "attempts" {
					r[i].Attempts[0].Errno, r[i].Attempts[0].Error = 2, "missing"
					break
				}
			}
			return r
		},
		"timeout is not denial": func(r []socketReport) []socketReport {
			for i := range r {
				if r[i].Kind == "attempts" {
					r[i].Attempts[0].Errno, r[i].Attempts[0].Error = 110, "timeout"
					break
				}
			}
			return r
		},
		"no invocation": func(r []socketReport) []socketReport {
			for i := range r {
				if r[i].Kind == "attempts" {
					r[i].Attempts[0].Invoked = false
					break
				}
			}
			return r
		},
		"process restart": func(r []socketReport) []socketReport { r[len(r)-1].PID++; return r },
	} {
		t.Run(name, func(t *testing.T) {
			r := change(socketCloneReports(t, s.reports))
			if err := socketInventory(s.config, r, s.w.approval.uid, s.w.approval.gid); err == nil {
				t.Fatal("incomplete evidence was accepted")
			}
		})
	}
	t.Run("receipt defeats reported denial", func(t *testing.T) {
		r := socketCloneReports(t, s.reports)
		for i := range r {
			for j := range r[i].Attempts {
				a := &r[i].Attempts[j]
				a.Connected, a.Sent, a.Errno, a.Error = false, false, 13, "denied"
			}
		}
		listener := []socketReport{{Kind: "receipt", Nonce: s.config.Nonce + "/worker/first/absolute/d0-s0"}}
		if err := socketDenied(s.config, r, listener); err == nil {
			t.Fatal("listener receipt was hidden by a reported error")
		}
	})
	t.Run("connect without echo is failure", func(t *testing.T) {
		r := []socketReport{{Kind: "attempts", Attempts: []socketAttempt{{Connected: true, Error: "exchange failed", Errno: 13}}}}
		if err := socketDenied(s.config, r, nil); err == nil {
			t.Fatal("successful connect without a reply was accepted")
		}
	})
	t.Run("private controls need receipt", func(t *testing.T) {
		r := socketCloneReports(t, s.reports)
		for i := range r {
			if r[i].Kind == "private" {
				r[i].Receipts = nil
				break
			}
		}
		if err := socketPrivate(s.config, r, false); err == nil {
			t.Fatal("datagram send success alone was accepted")
		}
	})
	t.Run("host controls need receipt", func(t *testing.T) {
		s.listener.output.mu.Lock()
		original := s.listener.output.frames
		s.listener.output.frames = nil
		s.listener.output.mu.Unlock()
		err := s.hostControls()
		s.listener.output.mu.Lock()
		s.listener.output.frames = original
		s.listener.output.mu.Unlock()
		if err == nil {
			t.Fatal("empty listener receipt inventory was accepted")
		}
	})
}
