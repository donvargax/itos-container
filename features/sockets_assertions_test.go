package features

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
)

func socketPhases(mode string) []string {
	if mode == "existing" {
		return []string{"first", "running"}
	}
	return []string{"parent", "child"}
}

func socketSequence(mode string) []string {
	if mode == "existing" {
		return []string{"attempts/first/parent", "private/first/parent", "barrier/first/parent", "attempts/running/parent", "private/running/parent", "barrier/exit/parent"}
	}
	result := []string{}
	if mode == "replace" {
		result = append(result, "prepared/live/parent", "prepared/live/child", "barrier/child-ready/child")
	}
	return append(result, "barrier/live/parent", "attempts/parent/parent", "private/parent/parent", "barrier/parent/parent", "attempts/child/child", "private/child/child", "barrier/exit/parent")
}

// Require complete, ordered reports BEFORE evaluating denials. All processes
// identify themselves; a parent-only, skipped, duplicate or partial run fails.
func socketInventory(c socketConfig, reports []socketReport, uid, gid string) error {
	sequence := socketSequence(c.Mode)
	if len(reports) != len(sequence) {
		return fmt.Errorf("incomplete socket execution: %d frames, want %d", len(reports), len(sequence))
	}
	pids := map[string]int{}
	aliases := map[string]map[string]string{}
	for i, r := range reports {
		key := r.Kind + "/" + r.Phase + "/" + r.Process
		if key != sequence[i] || r.Nonce != c.Nonce || r.Error != "" || r.PID < 1 || strconv.Itoa(r.UID) != uid || strconv.Itoa(r.GID) != gid {
			return fmt.Errorf("socket frame %d has an unexpected phase, process, nonce or identity: %s", i, key)
		}
		if prior := pids[r.Process]; prior != 0 && prior != r.PID {
			return errors.New("socket process restarted between preparation and attempt")
		}
		pids[r.Process] = r.PID
		if r.Process == "child" && (r.PID == pids["parent"] || r.PPID != pids["parent"]) {
			return errors.New("child socket report lacks a distinct child of the same parent")
		}
		if r.Kind == "prepared" {
			if len(r.Reads) != len(c.Targets) {
				return errors.New("missing original file reads and pre-existing aliases")
			}
			aliases[r.Process] = map[string]string{}
			for j, t := range c.Targets {
				read := r.Reads[j]
				if read.Target != t.Name || read.Path != filepath.Join(c.Root, t.Path) || read.Data != c.Nonce+"/original/"+t.Name || read.Error != "" ||
					!strings.HasPrefix(read.Alias, "/tmp/slice-19-alias-") || filepath.Clean(read.Alias) != read.Alias || filepath.Base(read.Alias) != t.Name {
					return errors.New("original read or pre-publication tmpfs alias is missing or changed")
				}
				aliases[r.Process][t.Name] = read.Alias
			}
		}
		if r.Kind != "attempts" {
			continue
		}
		spellings := []string{"absolute"}
		if c.Mode == "replace" {
			spellings = append(spellings, "relative", "symlink")
		}
		if len(r.Attempts) != len(c.Targets)*len(spellings) {
			return errors.New("missing or extra socket operations/aliases")
		}
		for j, spelling := range spellings {
			for k, t := range c.Targets {
				a := r.Attempts[j*len(c.Targets)+k]
				path := filepath.Join(c.Root, t.Path)
				switch spelling {
				case "relative":
					path = t.Path
				case "symlink":
					path = aliases[r.Process][t.Name]
				}
				if a.Target != t.Name || a.Operation != t.Op || a.Path != path || a.Nonce != c.Nonce+"/worker/"+r.Phase+"/"+spelling+"/"+t.Name ||
					!a.Invoked || a.SetupError != "" || (a.Errno == 0) != (a.Error == "") {
					return fmt.Errorf("incomplete syscall observation for %s/%s/%s", r.Phase, spelling, t.Name)
				}
				// Native Linux errno values: missing path, wrong path type, timeout,
				// nonblocking/in-progress or absent listener are not boundary proof.
				if slices.Contains([]int{2, 11, 20, 88, 110, 111, 115}, a.Errno) {
					return fmt.Errorf("inconclusive socket operation %s/%s: %s", r.Phase, t.Name, a.Error)
				}
			}
		}
	}
	return nil
}

func socketPositive(a socketAttempt) bool {
	connect := strings.HasSuffix(a.Operation, "-connect")
	return a.Invoked && a.SetupError == "" && a.Errno == 0 && a.Error == "" && a.ExchangeError == "" &&
		a.Connected == connect && a.Sent && a.Reply == a.Nonce
}

func (s *socketWorld) completeReports() (err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("%w\nstartup diagnostics (not product red evidence):\n%s", err, s.startupDiagnostics())
		}
	}()
	if s.fixtureErr != nil {
		return fmt.Errorf("socket fixture/control failure (not product red evidence): %w", s.fixtureErr)
	}
	if s.runErr != nil {
		return fmt.Errorf("incomplete socket workload (product exit %d): %w", s.w.offline.exit, s.runErr)
	}
	if err := socketInventory(s.config, s.reports, s.w.approval.uid, s.w.approval.gid); err != nil {
		return err
	}
	want := []string{"live", "parent", "exit"}
	if s.config.Mode == "existing" {
		want = []string{"first", "exit"}
	}
	if !reflect.DeepEqual(s.barriers, want) || !offlineHex(s.id) {
		return errors.New("missing independent same-worker running observations")
	}
	return nil
}

func (s *socketWorld) hostControls() error {
	if s.fixtureErr != nil {
		return fmt.Errorf("socket fixture/control failure (not product red evidence): %w", s.fixtureErr)
	}
	if err := s.listenersUnchanged(); err != nil {
		return err
	}
	frames, err := s.listener.output.snapshot()
	if err != nil {
		return err
	}
	receipts := map[string]string{}
	for _, r := range frames {
		if r.Kind == "receipt" {
			if _, duplicate := receipts[r.Nonce]; duplicate {
				return errors.New("duplicate listener receipt")
			}
			receipts[r.Nonce] = r.Target
		}
	}
	if len(s.controls) != 4 || len(receipts) == 0 {
		return errors.New("missing before/after host controls or listener receipts")
	}
	i := 0
	for _, phase := range socketPhases(s.config.Mode) {
		for _, edge := range []string{"before", "after"} {
			name := phase + "-" + edge
			r := s.controls[i]
			i++
			if r.Kind != "host-control" || r.Process != "host" || r.Phase != name || r.Nonce != s.config.Nonce ||
				strconv.Itoa(r.UID) != s.w.approval.uid || strconv.Itoa(r.GID) != s.w.approval.gid || len(r.Attempts) != len(s.config.Targets) {
				return fmt.Errorf("incomplete host control %s", name)
			}
			for j, t := range s.config.Targets {
				a := r.Attempts[j]
				nonce := s.config.Nonce + "/host/" + name + "/" + t.Name
				if a.Nonce != nonce || a.Target != t.Name || a.Operation != t.Op || a.Path != filepath.Join(s.w.doctor.checkout, t.Path) || !socketPositive(a) || receipts[nonce] != t.Name {
					return fmt.Errorf("fixture/control failure (not product red evidence): %s/%s did not exchange and record its nonce: %+v", name, t.Name, a)
				}
			}
		}
	}
	return nil
}

func socketPrivate(c socketConfig, reports []socketReport, requireTmpfs bool) error {
	found := 0
	for _, r := range reports {
		if r.Kind != "private" {
			continue
		}
		found++
		if (requireTmpfs && !r.Tmpfs) || len(r.Attempts) != len(socketOperations) || len(r.Receipts) != len(socketOperations) {
			return errors.New("private IPC controls lack tmpfs, operations or receipts")
		}
		for j, op := range socketOperations {
			a := r.Attempts[j]
			nonce := c.Nonce + "/private/" + r.Phase + "/" + op
			if a.Operation != op || a.Nonce != nonce || !strings.HasPrefix(a.Path, "/tmp/slice-19-private-") || filepath.Clean(a.Path) != a.Path ||
				filepath.Base(a.Path) != op || !socketPositive(a) || !slices.Contains(r.Receipts, nonce) {
				return fmt.Errorf("worker-private control failed for %s/%s: %+v", r.Phase, op, a)
			}
		}
	}
	if found != 2 {
		return errors.New("missing worker-private control phases")
	}
	return nil
}

func socketDenied(c socketConfig, reports, listener []socketReport) error {
	var failures []string
	for _, r := range reports {
		if r.Kind != "attempts" {
			continue
		}
		for _, a := range r.Attempts {
			if a.Connected || a.Sent || a.Errno == 0 || a.Error == "" {
				failures = append(failures, fmt.Sprintf("%s/%s %s at %s connected=%t sent=%t errno=%d", r.Phase, r.Process, a.Operation, a.Path, a.Connected, a.Sent, a.Errno))
			}
		}
	}
	for _, r := range listener {
		if r.Kind == "receipt" && strings.HasPrefix(r.Nonce, c.Nonce+"/worker/") {
			failures = append(failures, "host listener received "+r.Nonce)
		}
	}
	if len(failures) != 0 {
		return fmt.Errorf("checkout host sockets were reachable:\n%s", strings.Join(failures, "\n"))
	}
	return nil
}

func (w *world) socketDenials() error {
	s := w.sockets
	// A refusal, undefined/missing report, failed positive control or timeout
	// never becomes the expected connectivity red assertion.
	if err := s.completeReports(); err != nil {
		return err
	}
	if err := s.hostControls(); err != nil {
		return err
	}
	if err := socketPrivate(s.config, s.reports, true); err != nil {
		return err
	}
	listener, err := s.listener.output.snapshot()
	if err != nil {
		return err
	}
	return socketDenied(s.config, s.reports, listener)
}

func (w *world) socketEdit() error {
	s := w.sockets
	if err := s.completeReports(); err != nil {
		return err
	}
	if s.edit.Bytes != s.config.Nonce || s.edit.UID != w.approval.uid || s.edit.Mode != 0600 || s.editIdentity.GID != w.approval.gid {
		return errors.New("regular nonce edit was not visible on the host before worker exit")
	}
	path := filepath.Join(w.doctor.checkout, "slice-19-edit")
	after, err := sessionReadRecord(path)
	if err != nil || after != s.edit {
		return errors.New("checkout edit changed or disappeared during owned cleanup")
	}
	id, err := socketStat(path)
	if err != nil || id != s.editIdentity {
		return errors.New("checkout edit lacks the host group")
	}
	return w.offlineSuccess()
}

func (w *world) socketPreserved() error {
	s := w.sockets
	if err := s.listenersUnchanged(); err != nil {
		return err
	}
	after, err := sessionReadRecord(s.snapshotPath())
	if err != nil || after != s.snapshot || after.Bytes == "" {
		return errors.New("cleanup changed protected snapshot bytes or identity")
	}
	for _, path := range []string{filepath.Dir(s.snapshotPath()), filepath.Dir(w.sessionRecordPath())} {
		entries, err := os.ReadDir(path)
		if err != nil || len(entries) != 1 {
			return errors.New("unexpected snapshot or completion-record inventory")
		}
	}
	return w.offlineSuccess()
}
