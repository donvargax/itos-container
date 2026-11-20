package features

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"slices"
	"strconv"
	"sync"
)

// The socket-backend fixture (T-66). Doctor, approve and run choose the backend
// the host offers, and the product asks the kernel itself: a native probe
// issues landlock_create_ruleset(NULL, 0, LANDLOCK_CREATE_RULESET_VERSION). So
// a world pins the answer at the system call, and names the runtime inventory
// the fixture daemon reports. The slices' steps call the helpers here; the step
// texts, and the product output they judge, stay with the slices.
//
// A world that pins an answer starts each later invocation, on every route that
// applies the wrapper (runEnv does), through a helper mode of this test binary.
// The helper installs a seccomp filter whose only notified call is
// landlock_create_ruleset, hands the listener to the harness and execs the
// program in the same process. The harness answers from the world's answer as
// it stands when a query arrives, until its route ends the wrapper. A world that
// pins nothing starts its program directly and sees the host's kernel.

// landlockNoUmask asks the wrapper to leave the program's umask as it is.
const landlockNoUmask = -1

// landlockCall is one landlock_create_ruleset call a supervised program made,
// recorded once its answer was delivered.
type landlockCall struct {
	Version bool   // the call was the version query
	Answer  string // what was delivered: "ABI <n>", "ENOSYS", "EOPNOTSUPP" or "EPERM"
}

// landlockInvocation is the record of one supervised invocation.
type landlockInvocation struct{ calls []landlockCall }

// landlockWorld is a world's pinned answer and what the harness recorded.
type landlockWorld struct {
	mu          sync.Mutex
	answer      string
	action      func() error
	invocations []*landlockInvocation
	runs        []*landlockRun // supervisions not yet ended
}

var landlockABIText = regexp.MustCompile(`^ABI (0|[1-9][0-9]*)$`)

// parseLandlockAnswer reads a pinned text: "ABI <n>" is the version n, a
// decimal from 0 to 2147483647 with no sign or leading zeros since the kernel's
// reply is a C int, and an errno's name is a failure with that errno. Any
// other text is refused.
func parseLandlockAnswer(text string) (abi int, errno string, err error) {
	switch text {
	case "ENOSYS", "EOPNOTSUPP", "EPERM":
		return -1, text, nil
	}
	if match := landlockABIText.FindStringSubmatch(text); match != nil {
		if abi, err := strconv.ParseInt(match[1], 10, 64); err == nil && abi <= math.MaxInt32 {
			return int(abi), "", nil
		}
	}
	return 0, "", fmt.Errorf("socket backend fixture: cannot pin %q: use ABI <n> with n from 0 to 2147483647, ENOSYS, EOPNOTSUPP or EPERM", text)
}

// pinKernelAnswer makes the kernel answer the Landlock version query with text
// from now on. It is safe while invocations run: each query reads the answer as
// it stands when it arrives. A host that cannot supervise refuses every pin: a
// pinned world there would start its program unfiltered, so the product would
// read the host kernel's answer as the pinned one, and runEnv would skip its
// /bin/sh umask wrapper for want of a helper to set the umask.
func (w *world) pinKernelAnswer(text string) error {
	if !socketBackendSupported {
		return errors.New("socket backend fixture: this host cannot pin a kernel answer, which is chosen at the system call of a Linux amd64 process")
	}
	if _, _, err := parseLandlockAnswer(text); err != nil {
		return err
	}
	if w.landlock == nil {
		w.landlock = &landlockWorld{}
	}
	w.landlock.mu.Lock()
	defer w.landlock.mu.Unlock()
	w.landlock.answer = text
	return nil
}

// onNextLandlockQuery arranges one action to run when the next version query
// arrives, before its answer is read. An action's error fails the invocation's
// step; the query still gets the world's answer as it then stands.
func (w *world) onNextLandlockQuery(action func() error) error {
	if w.landlock == nil {
		return errors.New("socket backend fixture: no kernel answer is pinned, so no query will arrive")
	}
	w.landlock.mu.Lock()
	defer w.landlock.mu.Unlock()
	if w.landlock.action != nil {
		return errors.New("socket backend fixture: an action is already arranged for the next query")
	}
	w.landlock.action = action
	return nil
}

// landlockRecord is each supervised invocation's calls, in the order they were
// made, the invocations in the order they started.
func (w *world) landlockRecord() [][]landlockCall {
	l := w.landlock
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	var record [][]landlockCall
	for _, invocation := range l.invocations {
		record = append(record, slices.Clone(invocation.calls))
	}
	return record
}

// closeLandlock is the scenario's cleanup: it ends any supervision still
// running, closing its listener as a route's end does.
func (w *world) closeLandlock() error {
	l := w.landlock
	if l == nil {
		return nil
	}
	l.mu.Lock()
	runs := slices.Clone(l.runs)
	l.mu.Unlock()
	var errs []error
	for _, run := range runs {
		errs = append(errs, run.end(true))
	}
	return errors.Join(errs...)
}

// takeAction hands the arranged action to the query that has arrived, once.
func (l *landlockWorld) takeAction() func() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	action := l.action
	l.action = nil
	return action
}

// current is the answer a query gets now.
func (l *landlockWorld) current() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.answer
}

// record notes a call whose answer was delivered.
func (l *landlockWorld) record(invocation *landlockInvocation, call landlockCall) {
	l.mu.Lock()
	defer l.mu.Unlock()
	invocation.calls = append(invocation.calls, call)
}

// begin records a supervised invocation that is starting.
func (l *landlockWorld) begin(run *landlockRun) *landlockInvocation {
	l.mu.Lock()
	defer l.mu.Unlock()
	invocation := &landlockInvocation{}
	l.invocations = append(l.invocations, invocation)
	l.runs = append(l.runs, run)
	return invocation
}

// forget drops a supervision that has ended.
func (l *landlockWorld) forget(run *landlockRun) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.runs = slices.DeleteFunc(l.runs, func(other *landlockRun) bool { return other == run })
}

// The runtime inventories a fixture daemon reports in /info. T-64's docker
// tests judge these shapes: Moby reports a shim registration as an empty
// object, and an inventory may be missing altogether.
func runtimeInventory(name string) (runtimes map[string]any, ok bool) {
	runc := map[string]any{"path": "runc"}
	switch name {
	case "runc only":
		return map[string]any{"runc": runc}, true
	case "runc and runsc":
		return map[string]any{"runc": runc, "runsc": map[string]any{"path": "/usr/local/bin/runsc"}}, true
	case "runsc permitting host sockets":
		return map[string]any{"runc": runc, "runsc": map[string]any{"path": "/usr/local/bin/runsc", "runtimeArgs": []string{"--host-uds=open"}}}, true
	case "unidentified runsc":
		return map[string]any{"runc": runc, "runsc": map[string]any{}}, true
	case "missing inventory":
		return nil, true
	}
	return nil, false
}

// chooseRuntimes selects what the daemon's /info reports from now on. Each
// request reads the choice afresh. An unknown name is refused and changes
// nothing.
func (a *doctorAPI) chooseRuntimes(name string) error {
	runtimes, ok := runtimeInventory(name)
	if !ok {
		return fmt.Errorf("socket backend fixture: unknown runtime inventory %q", name)
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if runtimes == nil {
		delete(a.info, "Runtimes")
	} else {
		a.info["Runtimes"] = runtimes
	}
	a.info["DefaultRuntime"] = "runc"
	return nil
}

// chooseRuntimeInventory names the selected fixture daemon's runtime inventory.
func (w *world) chooseRuntimeInventory(name string) error {
	if w.doctor == nil || w.doctor.engine == nil {
		return errors.New("socket backend fixture: the world has no fixture daemon")
	}
	return w.doctor.engine.chooseRuntimes(name)
}
