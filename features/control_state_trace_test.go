package features

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type controlEvent struct {
	recoverySync
	Op, Phase string
	Success   bool
	Flags     uint64
}

// Extend the existing child-only syscall observer, preserving its thread and
// signal handling. Faults replace only the named filesystem call with EIO.
// Neither command outcomes nor state records are supplied by this observer.
func controlTraceSource() string {
	source := recoveryTraceSource
	for _, change := range [][2]string{
		{`"syscall"`, `"syscall"; "time"`},
		{`type event struct {`, `type event struct { Op, Phase string; Success bool; Flags uint64;`},
		{`type call struct { entering bool; sync *event }`, `type call struct { entering bool; sync *event; fault bool }`},
		{`entry := int64(regs.Rax) == -int64(unix.ENOSYS)`, `entry := int64(regs.Rax) == -int64(unix.ENOSYS) && !c.fault`},
		{`if regs.Orig_rax == unix.SYS_FSYNC { c.sync = observe(tid, regs.Rdi, root) }`, `
                c.sync = controlObserve(tid, regs, root)
                if c.sync != nil {
                    c.sync.Phase = "begin"
                    if json.NewEncoder(journal).Encode(c.sync) != nil { fail() }
                    controlBarrier(c.sync)
                    c.fault = controlFault(c.sync)
                    if c.fault {
                        regs.Orig_rax = ^uint64(0)
                        if unix.PtraceSetRegs(tid, &regs) != nil { fail() }
                    }
                }`},
		{`if regs.Rax == 0 {
                    completed(c.sync)
                    if json.NewEncoder(journal).Encode(c.sync) != nil { fail() }
                }`, `
                if c.fault {
                    regs.Rax = ^uint64(unix.EIO-1)
                    if c.sync.Op == "account-read" { regs.Rax = 0 }
                    if unix.PtraceSetRegs(tid, &regs) != nil { fail() }
                    c.fault = false
                }
                c.sync.Phase = "end"
                c.sync.Success = int64(regs.Rax) >= 0
                if c.sync.Success { completed(c.sync) }
                if json.NewEncoder(journal).Encode(c.sync) != nil { fail() }`},
	} {
		if strings.Count(source, change[0]) != 1 {
			panic("slice-21 syscall observer extension no longer matches its base")
		}
		source = strings.Replace(source, change[0], change[1], 1)
	}
	return source + controlTraceExtension
}

func (f *controlFixture) traceCommand(args ...string) *recoveryProcess {
	index := filepath.Base(f.p.dir) + "-" + strconv.Itoa(len(f.p.recovery.processes))
	p := &recoveryProcess{exit: -1, done: make(chan struct{}), journal: filepath.Join(f.parent, "slice-21-trace-"+index), pid: filepath.Join(f.parent, "slice-21-pid-"+index)}
	f.p.recovery.processes = append(f.p.recovery.processes, p)
	trace := f.p.bin + "-slice-21-trace"
	if _, err := os.Stat(trace); err != nil {
		if err = recoveryBuildHelper(f.p.root, trace, controlTraceSource()); err != nil {
			p.err = err
			close(p.done)
			return p
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	p.cancel = cancel
	p.cmd = exec.CommandContext(ctx, trace, append([]string{p.journal, p.pid, f.parent, f.p.bin}, args...)...)
	p.cmd.Dir, p.cmd.Env = f.p.doctor.cwd, f.p.env()
	p.cmd.Stdout, p.cmd.Stderr = &p.out, &p.errout
	if err := p.cmd.Start(); err != nil {
		p.err = err
		cancel()
		close(p.done)
		return p
	}
	go func() {
		defer close(p.done)
		defer cancel()
		err := p.cmd.Wait()
		p.exit = 0
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			p.exit = exit.ExitCode()
		} else {
			p.err = err
		}
		if ctx.Err() != nil {
			p.err = fmt.Errorf("slice-21 traced command exceeded its deadline: %w", ctx.Err())
		}
	}()
	return p
}

func controlEvents(path string) ([]controlEvent, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 4096), 1<<20)
	var events []controlEvent
	for scanner.Scan() {
		var event controlEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, scanner.Err()
}

const controlTraceExtension = `
func controlString(tid int, address uint64) string {
    var data []byte
    for len(data) < 4096 {
        var buf [8]byte
        n, err := unix.PtracePeekData(tid, uintptr(address)+uintptr(len(data)), buf[:])
        if err != nil || n == 0 { return "" }
        for _, b := range buf[:n] {
            if b == 0 { return string(data) }
            data = append(data, b)
        }
    }
    return ""
}
func controlPath(tid int, fd, address uint64) string {
    name := controlString(tid, address)
    if filepath.IsAbs(name) { return name }
    descriptor := fmt.Sprintf("/proc/%d/fd/%d", tid, fd)
    if int32(fd) == unix.AT_FDCWD { descriptor = fmt.Sprintf("/proc/%d/cwd", tid) }
    base, err := os.Readlink(descriptor)
    if err != nil || name == "" { return "" }
    return filepath.Join(base, name)
}
func controlObserve(tid int, r unix.PtraceRegs, root string) *event {
    e := &event{}
    if os.Getenv("SLICE_21_ACCOUNT_PROBE") == "1" {
        if r.Orig_rax == unix.SYS_OPENAT && controlPath(tid, r.Rdi, r.Rsi) == "/etc/passwd" {
            return &event{Path:"/etc/passwd", Op:"account-open"}
        }
        if r.Orig_rax == unix.SYS_READ {
            path, _ := os.Readlink(fmt.Sprintf("/proc/%d/fd/%d", tid, r.Rdi))
            if path == "/etc/passwd" { return &event{Path:path, Op:"account-read"} }
        }
    }
    switch r.Orig_rax {
    case unix.SYS_FSYNC, unix.SYS_FLOCK, unix.SYS_WRITE, unix.SYS_READ:
        e = observe(tid, r.Rdi, root)
        if e == nil { return nil }
        switch r.Orig_rax {
        case unix.SYS_FSYNC: e.Op = "fsync"
        case unix.SYS_FLOCK: e.Op = "flock"; e.Flags = r.Rsi
        case unix.SYS_WRITE: e.Op = "write"
        case unix.SYS_READ: e.Op = "read"
        }
    case unix.SYS_MKDIRAT:
        e.Path, e.Op = controlPath(tid, r.Rdi, r.Rsi), "mkdir"
    case unix.SYS_OPENAT:
        if r.Rdx & unix.O_CREAT == 0 { return nil }
        e.Path, e.Op, e.Flags = controlPath(tid, r.Rdi, r.Rsi), "create", r.Rdx
    case unix.SYS_RENAMEAT, unix.SYS_RENAMEAT2:
        e.Path, e.Op = controlPath(tid, r.Rdx, r.R10), "rename"
        from := controlPath(tid, r.Rdi, r.Rsi)
        e.Inode = inode(from)
        data, _ := os.ReadFile(from); e.Data = string(data)
    default: return nil
    }
    if e.Path != root && !strings.HasPrefix(e.Path, root+"/") { return nil }
    return e
}
func controlFault(e *event) bool {
    fault := os.Getenv("SLICE_21_FAULT")
    coord := filepath.Join(os.Getenv("SLICE_21_ACCOUNT_HOME"), ".local/state/itos-container/coordination")
    inside := strings.HasPrefix(e.Path, coord+"/") && filepath.Base(e.Path) != "control.lock"
    switch fault {
    case "account database empty": return e.Op == "account-read"
    case "account database unreadable": return e.Op == "account-open"
    case "coordination directory create": return e.Op == "mkdir" && e.Path == coord
    case "coordination directory fsync": return e.Op == "fsync" && e.Directory && e.Path == coord
    case "coordination parent fsync":
        _, err := os.Stat(coord)
        return err == nil && e.Op == "fsync" && e.Directory && e.Path == filepath.Dir(coord)
    case "self-binding record write", "location record write": return e.Op == "write" && inside
    case "self-binding file fsync": return e.Op == "fsync" && !e.Directory && inside
    case "location record file fsync": return e.Op == "fsync" && !e.Directory && inside && filepath.Base(e.Path) != "self.json"
    case "self-binding rename", "location publication rename": return e.Op == "rename" && inside
    case "self-binding parent fsync", "location parent fsync":
        if e.Op != "fsync" || !e.Directory || e.Path != coord { return false }
        entries, _ := os.ReadDir(coord)
        for _, entry := range entries { if strings.HasSuffix(entry.Name(), ".json") && (fault == "self-binding parent fsync" || entry.Name() != "self.json") { return true } }
    case "cataloged reservation cannot be read":
        return e.Op == "read" && strings.Contains(e.Path, "/admission/")
    }
    return false
}
var controlPaused bool
func controlBarrier(e *event) {
    ready := os.Getenv("SLICE_21_TRACE_READY")
    if ready == "" || controlPaused || e.Op != "create" || !strings.HasPrefix(filepath.Base(e.Path), ".approval-") { return }
    controlPaused = true
    os.WriteFile(ready, []byte("ready"), 0600)
    timer := time.NewTimer(15*time.Second); defer timer.Stop()
    tick := time.NewTicker(5*time.Millisecond); defer tick.Stop()
    for {
        data, _ := os.ReadFile(os.Getenv("SLICE_21_TRACE_RELEASE"))
        if string(data) == "continue" { return }
        select { case <-timer.C: return; case <-tick.C: }
    }
}
`
