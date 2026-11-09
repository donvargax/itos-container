package features

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// A native recovery assertion needs observed successful fsync calls, not just
// visible JSON. This observer traces only its child and that child's threads.
// The standalone helper uses the already pinned x/sys package, never Docker or
// product packages. PTRACE_O_EXITKILL prevents an observer failure stranding its
// controller. The engine fixture separately owns bounded worker cleanup.
type recoverySync struct {
	Path, Data, Inode string
	Access            string
	At                time.Time
	Directory         bool
	Files             map[string]string
}

func recoveryBuildHelper(root, output, source string) error {
	path := output + ".go"
	if err := os.WriteFile(path, []byte(source), 0600); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "build", "-trimpath", "-o", output, path)
	cmd.Dir, cmd.Env = root, sessionChildEnvironment(os.Environ())
	for key, value := range map[string]string{"CGO_ENABLED": "0", "GOOS": "linux", "GOARCH": "amd64", "GOFLAGS": "", "GOWORK": "off"} {
		cmd.Env = setEnv(cmd.Env, key, value)
	}
	data, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("build recovery observer: %w\n%s", err, data)
	}
	return nil
}

func (w *world) recoveryTraceBinary() (string, error) {
	path := filepath.Join(w.dir, "slice-11-trace")
	if filepath.IsAbs(w.bin) {
		path = w.bin + "-slice-11-trace"
	}
	return featureBinary(path, func(target string) error {
		return recoveryBuildHelper(w.root, target, recoveryTraceSource)
	})
}

func recoveryReadSyncs(path string) ([]recoverySync, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 4096), 1<<20)
	var events []recoverySync
	for scanner.Scan() {
		var event recoverySync
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, scanner.Err()
}

const recoveryTraceSource = `package main

import (
    "encoding/json"
    "errors"
    "fmt"
    "io"
    "os"
    "os/exec"
    "path/filepath"
    "runtime"
    "strconv"
    "strings"
    "syscall"
    "time"

    "golang.org/x/sys/unix"
)

type event struct {
    Path, Data, Inode string
    Access string
    At time.Time
    Directory bool
    Files map[string]string
}
type call struct { entering bool; sync *event }
// The outer signal tracer owns the observer, while this observer exclusively
// owns its child. CLONE_UNTRACED avoids competing ptracers without changing
// either process's group, session or terminal. Child signal syscalls are logged
// here as well so that this separation cannot hide a controller's sender.
var unixSysProcAttr = syscall.SysProcAttr{Ptrace:true, Cloneflags:unix.CLONE_UNTRACED}

func signalCall(log *os.File, tid int, regs unix.PtraceRegs) {
    if log == nil { return }
    switch regs.Orig_rax {
    case unix.SYS_KILL:
        fmt.Fprintf(log, "%d kill(%d, %d)\n", tid, int32(regs.Rdi), regs.Rsi)
    case unix.SYS_TKILL:
        fmt.Fprintf(log, "%d tkill(%d, %d)\n", tid, int32(regs.Rdi), regs.Rsi)
    case unix.SYS_TGKILL:
        fmt.Fprintf(log, "%d tgkill(%d, %d, %d)\n", tid, int32(regs.Rdi), int32(regs.Rsi), regs.Rdx)
    case unix.SYS_PIDFD_SEND_SIGNAL:
        target, _ := os.ReadFile(fmt.Sprintf("/proc/%d/fdinfo/%d", tid, regs.Rdi))
        fmt.Fprintf(log, "%d pidfd_send_signal(%d, %d) fdinfo=%q\n", tid, regs.Rdi, regs.Rsi, target)
    case unix.SYS_SETSID:
        fmt.Fprintf(log, "%d setsid()\n", tid)
    case unix.SYS_SETPGID:
        fmt.Fprintf(log, "%d setpgid(%d, %d)\n", tid, int32(regs.Rdi), int32(regs.Rsi))
    }
}

func inode(path string) string {
    var stat unix.Stat_t
    if unix.Stat(path, &stat) != nil { return "" }
    return fmt.Sprintf("%d:%d", stat.Dev, stat.Ino)
}
func observe(pid int, fd uint64, root string) *event {
    descriptor := fmt.Sprintf("/proc/%d/fd/%d", pid, fd)
    path, err := os.Readlink(descriptor)
    if err != nil || (path != root && !strings.HasPrefix(path, root+"/")) { return nil }
    info, err := os.Stat(descriptor)
    if err != nil { return nil }
    e := &event{Path:path, Inode:inode(descriptor), Directory:info.IsDir()}
    if info.Mode().IsRegular() && info.Size() <= 65536 {
        f, err := os.Open(descriptor)
        if err == nil {
            data, readErr := io.ReadAll(io.LimitReader(f, 65537))
            f.Close()
            if readErr == nil && len(data) <= 65536 { e.Data = string(data) }
        }
    }
    return e
}
func completed(e *event) {
    e.At = time.Now()
    if !e.Directory { return }
    entries, err := os.ReadDir(e.Path)
    if err != nil { return }
    e.Files = map[string]string{}
    for _, entry := range entries {
        // Only concrete files in the just-synced directory are correlated;
        // no recursive scan or observation of state outside the fixture root.
        if entry.Type().IsRegular() { e.Files[entry.Name()] = inode(filepath.Join(e.Path, entry.Name())) }
    }
}
// Record attempted proc-path reads, including failures. The target process is
// stopped at entry; relative dirfd paths are resolved by the independent tracer.
func access(pid int, regs unix.PtraceRegs) *event {
    var pointer uint64
    fd := int32(-100)
    switch regs.Orig_rax {
    case unix.SYS_OPEN, unix.SYS_STAT, unix.SYS_LSTAT, unix.SYS_READLINK:
        pointer = regs.Rdi
    case unix.SYS_OPENAT, unix.SYS_OPENAT2, unix.SYS_NEWFSTATAT, unix.SYS_STATX, unix.SYS_READLINKAT:
        pointer, fd = regs.Rsi, int32(regs.Rdi)
    default: return nil
    }
    var data []byte
    for len(data)<4096 {
        var b [8]byte
        if _,err:=unix.PtracePeekData(pid,uintptr(pointer)+uintptr(len(data)),b[:]);err!=nil{return nil}
        end:=false
        for _,value:=range b{if value==0{end=true;break};data=append(data,value)}
        if end{break}
    }
    path:=string(data)
    if !filepath.IsAbs(path) {
        target:=fmt.Sprintf("/proc/%d/cwd",pid)
        if fd != -100 {target=fmt.Sprintf("/proc/%d/fd/%d",pid,fd)}
        parent,err:=os.Readlink(target);if err!=nil{return nil}
        path=filepath.Join(parent,path)
    }
    if !strings.HasPrefix(path,"/proc/"){return nil}
    return &event{Path:path,Access:"proc-read",At:time.Now()}
}
func main() {
    if len(os.Args) < 6 { os.Exit(96) }
    runtime.LockOSThread()
    var signals *os.File
    if path := os.Getenv("SLICE_11_SIGNAL_TRACE"); path != "" {
        var err error
        signals, err = os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0600)
        if err != nil { fmt.Fprintln(os.Stderr, "cannot open controller signal trace:", err); os.Exit(96) }
        defer signals.Close()
    }
    journal, err := os.OpenFile(os.Args[1], os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
    if err != nil { os.Exit(96) }
    defer journal.Close()
    root, err := filepath.Abs(os.Args[3])
    if err != nil { os.Exit(96) }
    cmd := exec.Command(os.Args[4], os.Args[5:]...)
    cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
    cmd.SysProcAttr = &unixSysProcAttr
    if err = cmd.Start(); err != nil { os.Exit(96) }
    pid := cmd.Process.Pid
    fail := func() { cmd.Process.Kill(); os.Exit(96) }
    var status unix.WaitStatus
    if _, err = unix.Wait4(pid, &status, 0, nil); err != nil || !status.Stopped() { fail() }
    options := unix.PTRACE_O_TRACECLONE | unix.PTRACE_O_TRACEFORK | unix.PTRACE_O_TRACEVFORK | unix.PTRACE_O_TRACESYSGOOD | unix.PTRACE_O_EXITKILL
    if unix.PtraceSetOptions(pid, options) != nil { fail() }
    if os.WriteFile(os.Args[2], []byte(strconv.Itoa(pid)), 0600) != nil { fail() }
    calls := map[int]*call{pid:{}}
    traceAccess := os.Getenv("SLICE_17_ACCESS_TRACE") == "1"
    if unix.PtraceSyscall(pid, 0) != nil { fail() }
    exit := 96
    for len(calls) != 0 {
        tid, err := unix.Wait4(-1, &status, unix.WALL, nil)
        if errors.Is(err, unix.EINTR) { continue }
        if err != nil { fail() }
        c := calls[tid]
        if c == nil { c = &call{}; calls[tid] = c }
        if status.Exited() || status.Signaled() {
            if tid == pid {
                if status.Exited() { exit = status.ExitStatus() } else { exit = 128+int(status.Signal()) }
            }
            delete(calls, tid)
            continue
        }
        if !status.Stopped() { continue }
        signal := int(status.StopSignal())
        if signal == int(unix.SIGTRAP)|0x80 {
            var regs unix.PtraceRegs
            if err := unix.PtraceGetRegs(tid, &regs); err != nil {
                // exit_group in a sibling can retire this thread between its
                // syscall stop and register read. Wait still reaps its exit.
                if errors.Is(err, unix.ESRCH) { continue }
                fail()
            }
            // Linux reports -ENOSYS on syscall entry. A new clone's first
            // observed stop may be syscall exit, so a simple toggle is unsafe.
            entry := int64(regs.Rax) == -int64(unix.ENOSYS)
            if entry {
                signalCall(signals, tid, regs)
                if traceAccess {if e:=access(tid,regs);e!=nil{if json.NewEncoder(journal).Encode(e)!=nil{fail()}}}
                c.sync = nil
                if regs.Orig_rax == unix.SYS_FSYNC { c.sync = observe(tid, regs.Rdi, root) }
            } else if c.entering && c.sync != nil {
                if regs.Rax == 0 {
                    completed(c.sync)
                    if json.NewEncoder(journal).Encode(c.sync) != nil { fail() }
                }
                c.sync = nil
            }
            c.entering = entry
            signal = 0
        } else if signal == int(unix.SIGTRAP) {
            switch status.TrapCause() {
            case unix.PTRACE_EVENT_CLONE, unix.PTRACE_EVENT_FORK, unix.PTRACE_EVENT_VFORK:
                child, err := unix.PtraceGetEventMsg(tid)
                if err != nil { fail() }
                if calls[int(child)] == nil { calls[int(child)] = &call{} }
                signal = 0
            }
        } else if signal == int(unix.SIGSTOP) {
            signal = 0 // initial stop of a newly traced thread
        }
        if signals != nil && signal != 0 && signal != int(unix.SIGURG) {
            fmt.Fprintf(signals, "%d signal-forward(%d, %d)\n", os.Getpid(), tid, signal)
        }
        if err := unix.PtraceSyscall(tid, signal); err != nil && !errors.Is(err, unix.ESRCH) { fail() }
    }
    os.Exit(exit)
}
`
