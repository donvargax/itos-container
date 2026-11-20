package features

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

// The probe is a program of the features binary. Each argument after the mode
// is an action, run in order:
//
//	version   the Landlock version query, reported as a reply line
//	other     landlock_create_ruleset with flags 0, reported as a reply line
//	stdin     read standard input to its end
//	nested    start the fixture's helper from inside this process
//	report    one report line of what the program was started with
//	exit=N    exit N at once
//
// Every line it prints is one JSON object, so a reply reaches the reader before
// the program does anything else. It runs before TestMain makes its directory,
// so an invocation that exits at once leaves none behind.
const landlockProbeMode = "itos-container-landlock-probe"

// socketBackendChild runs the mode the features binary was started in, if any.
// TestMain calls it first: neither the helper, which execs, nor a probe that
// exits at once returns to remove the directory TestMain makes.
func socketBackendChild() {
	if len(os.Args) > 1 && os.Args[1] == landlockProbeMode {
		landlockProbeMain(os.Args[2:])
		os.Exit(0)
	}
}

// landlockProbeLine is one line of the probe's output: a reply, a report or the
// result of the nested start.
type landlockProbeLine struct {
	Event string // "reply", "report" or "nested"

	Call  string // reply: "version" or "other"
	Ret   int    // reply: the return value, -1 when the call failed
	Errno string // reply: the errno's name when it failed

	Argv           []string // report
	Dir            string
	Env            []string
	Stdin          string
	Pid            int
	Fds            []string // "<number>:<kind>", the probe's own listing descriptor left out
	Umask          string
	SigBlk         string
	SigIgn         string
	NoNewPrivs     string
	SeccompFilters string

	Error string // nested: what the fixture reported, if anything
	Inner string // nested: what the inner program printed
}

func landlockProbeMain(actions []string) {
	// A blocked set and the filter count are read per thread.
	runtime.LockOSThread()
	out := json.NewEncoder(os.Stdout)
	emit := func(line landlockProbeLine) {
		if err := out.Encode(line); err != nil {
			fmt.Fprintln(os.Stderr, "probe:", err)
			os.Exit(2)
		}
	}
	var stdin []byte
	for _, action := range actions {
		switch {
		case action == "version" || action == "other":
			flags := uintptr(0)
			if action == "version" {
				flags = unix.LANDLOCK_CREATE_RULESET_VERSION
			}
			line := landlockProbeLine{Event: "reply", Call: action}
			ret, errno := landlockQuery(flags)
			line.Ret = ret
			if errno != 0 {
				line.Errno = unix.ErrnoName(errno)
			}
			emit(line)
		case action == "stdin":
			data, err := io.ReadAll(os.Stdin)
			if err != nil {
				fmt.Fprintln(os.Stderr, "probe:", err)
				os.Exit(2)
			}
			stdin = data
		case action == "nested":
			emit(landlockNested())
		case action == "report":
			emit(landlockReport(stdin))
			fmt.Fprintln(os.Stderr, "probe stderr")
		case strings.HasPrefix(action, "exit="):
			code, err := strconv.Atoi(strings.TrimPrefix(action, "exit="))
			if err != nil {
				fmt.Fprintln(os.Stderr, "probe:", err)
				os.Exit(2)
			}
			os.Exit(code)
		default:
			fmt.Fprintf(os.Stderr, "probe: unknown action %q\n", action)
			os.Exit(2)
		}
	}
}

// landlockQuery is landlock_create_ruleset(NULL, 0, flags). The kernel's reply
// is a C int.
func landlockQuery(flags uintptr) (int, unix.Errno) {
	r, _, errno := unix.Syscall(unix.SYS_LANDLOCK_CREATE_RULESET, 0, 0, flags)
	if errno != 0 {
		return -1, errno
	}
	return int(int32(r)), 0
}

func landlockReport(stdin []byte) landlockProbeLine {
	line := landlockProbeLine{Event: "report", Argv: os.Args, Env: os.Environ(), Stdin: string(stdin), Pid: os.Getpid()}
	var err error
	if line.Dir, err = os.Getwd(); err != nil {
		line.Dir = "error: " + err.Error()
	}
	if line.Fds, err = landlockDescriptors(); err != nil {
		line.Fds = []string{"error: " + err.Error()}
	}
	status, err := landlockThreadStatus()
	if err != nil {
		status = map[string]string{"Umask": "error: " + err.Error()}
	}
	line.Umask, line.SigBlk, line.SigIgn = status["Umask"], status["SigBlk"], status["SigIgn"]
	line.NoNewPrivs, line.SeccompFilters = status["NoNewPrivs"], status["Seccomp_filters"]
	return line
}

// landlockThreadStatus is /proc/thread-self/status of a locked thread.
func landlockThreadStatus() (map[string]string, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	data, err := os.ReadFile("/proc/thread-self/status")
	if err != nil {
		return nil, err
	}
	status := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		if key, value, ok := strings.Cut(line, ":"); ok {
			status[key] = strings.TrimSpace(value)
		}
	}
	return status, nil
}

var landlockInode = regexp.MustCompile(`:\[[0-9]+\]$`)

// landlockDescriptors lists the open descriptors by number and kind: what each
// is, without the inode a pipe or socket carries.
func landlockDescriptors() ([]string, error) {
	dir, err := os.Open("/proc/self/fd")
	if err != nil {
		return nil, err
	}
	own := int(dir.Fd())
	names, err := dir.Readdirnames(-1)
	if closeErr := dir.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return nil, err
	}
	var numbers []int
	for _, name := range names {
		number, err := strconv.Atoi(name)
		if err != nil {
			return nil, err
		}
		if number != own {
			numbers = append(numbers, number)
		}
	}
	sort.Ints(numbers)
	var fds []string
	for _, number := range numbers {
		target, err := os.Readlink("/proc/self/fd/" + strconv.Itoa(number))
		if err != nil {
			return nil, err
		}
		fds = append(fds, strconv.Itoa(number)+":"+landlockInode.ReplaceAllString(target, ""))
	}
	return fds, nil
}

// landlockNested starts the fixture's helper from inside a supervised process.
// The kernel refuses a second listener there with EBUSY.
func landlockNested() landlockProbeLine {
	line := landlockProbeLine{Event: "nested"}
	self, err := os.Executable()
	w := &world{}
	if err == nil {
		err = w.pinKernelAnswer("ABI 8")
	}
	var inner bytes.Buffer
	if err == nil {
		cmd := exec.Command(self, landlockProbeMode, "report")
		cmd.Stdout = &inner
		var run *landlockRun
		if run, err = w.superviseLandlock(cmd, landlockNoUmask); err == nil {
			startErr := cmd.Start()
			if startErr == nil {
				// The exit status is the helper's; the fixture reports its failure.
				_ = cmd.Wait()
			}
			err = errors.Join(startErr, run.end(startErr == nil))
		}
	}
	if err != nil {
		line.Error = err.Error()
	}
	line.Inner = inner.String()
	return line
}
