package features

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"time"
)

type egressLifeRemoval struct {
	logInfo                                          os.FileInfo
	resource                                         string
	fault                                            *egressLifeRemoveFault
	second                                           *world
	other                                            *egressNativeRun
	otherBefore                                      *egressObserved
	otherEdit                                        string
	retry                                            []string
	pending                                          egressResult
	firstStop, secondStop, firstStatus, secondStatus egressResult
	remaining                                        []string
	kept                                             []byte
	beforeRetry, afterRetry                          map[string]string
	repeatJournal                                    []egressLifeRequest
}

type egressLifeRemoveFault struct {
	mu   sync.Mutex
	path string
	hits int
	body string
}

func (f *egressLifeRemoveFault) refuse(r *egressLifeRequest) *http.Response {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.path == "" || r.method != "DELETE" || r.path != f.path || f.hits != 0 {
		return nil
	}
	f.hits++
	return &http.Response{StatusCode: http.StatusInternalServerError, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(f.body))}
}

func (w *world) egressLifeRemovalSteps(sc lifecycleStepRegistrar) {
	sc.Step(`^a CI egress cleanup probe with one refusal and a one-shot removal fault on "([^"]*)"$`, w.egressLifeRemovalFixture)
	sc.Step(`^another project's live worker proxy and networks with independent progress receipts$`, w.egressLifeOtherFixture)
	sc.Step(`^run exits zero and encounters that removal fault$`, w.egressLifeRemovalRun)
	sc.Step(`^run reports run.cleanup-temporary with exit 75 and exactly the original public session's stop retry$`, w.egressLifeRemovalRetry)
	sc.Step(`^fresh status names the committed one-record log and pending cleanup with the original resource identities$`, w.egressLifeRemovalPending)
	sc.Step(`^removal has stopped at the fault without deleting later dependencies or the other project's resources$`, w.egressLifeRemovalStopped)
	sc.Step(`^a fresh process executes that exact retry and then repeats it$`, w.egressLifeRemovalRecover)
	sc.Step(`^the first stop confirms every original owned resource absent and the second performs no mutation$`, w.egressLifeRemovalComplete)
	sc.Step(`^both statuses retain the same log count and path without rewriting its bytes$`, w.egressLifeRemovalStored)
	sc.Step(`^both projects' edits approvals and the other project's progress are preserved$`, w.egressLifeOtherPreserved)
}

func (w *world) egressLifeRemovalFixture(resource string) error {
	if resource != "proxy" && resource != "egress network" && resource != "internal network" {
		return errors.New("unknown removal resource")
	}
	if err := w.egressLifeRuntimeFixture("exits zero on SIGTERM"); err != nil {
		return err
	}
	l := &egressLifeRemoval{resource: resource, fault: &egressLifeRemoveFault{body: `{"message":"synthetic one-shot removal fault"}`}, otherEdit: "other-edit-" + egressNonce()}
	w.egress.lifeRemoval = l
	w.egress.lifeRuntime.recorder.refuse = l.fault.refuse
	return nil
}

// Given provisions inputs only. Both real product invocations happen in When;
// a missing worker or progress receipt remains a failed assertion there.
func (w *world) egressLifeOtherFixture() error {
	l := w.egress.lifeRemoval
	child := &world{root: w.root, bin: w.bin, coverdir: w.coverdir, dir: filepath.Join(w.dir, "slice-28-other-project")}
	l.second = child
	if err := os.Mkdir(child.dir, 0700); err != nil {
		return err
	}
	w.egress.native.cleanup = append(w.egress.native.cleanup, child.closeEgressNative)
	if err := child.egressNativeCheckout([]string{"other.example"}, []string{"other.example"}); err != nil {
		return err
	}
	return child.approvalCapture()
}

func (w *world) egressLifeRemovalRun() error {
	l, r := w.egress.lifeRemoval, w.egress.lifeRuntime
	other := l.second.egress.native
	l.other = other.newRun([]egressStep{{Op: "edit", Nonce: l.otherEdit}, {Op: "barrier", Name: "progress-before"}, {Op: "edit", Nonce: l.otherEdit + "-after"}, {Op: "barrier", Name: "progress-after"}}, 0)
	if err := l.other.start(); err != nil {
		return err
	}
	if l.other.await("attach", 150*time.Second) {
		if err := l.other.release("attach"); err != nil {
			return err
		}
		if l.other.await("progress-before", 30*time.Second) {
			l.other.observe("progress-before")
			l.otherBefore = l.other.observed["progress-before"]
		}
	}
	n := w.egress.native
	steps := []egressStep{n.canonicalControl(), {Op: "raw", Method: "CONNECT", Target: "before-loss.example:443"}, {Op: "barrier", Name: "loss"}}
	if err := w.egressLifeRuntimeStart(steps, 0); err != nil {
		return err
	}
	if r.original != nil && r.original.proxy != nil {
		path := "/containers/" + r.original.proxy.ID
		switch l.resource {
		case "egress network":
			path = "/networks/" + r.original.networks["egress-network"].ID
		case "internal network":
			path = "/networks/" + r.original.networks["internal-network"].ID
		}
		l.fault.mu.Lock()
		l.fault.path = path
		l.fault.mu.Unlock()
		if err := r.run.release("loss"); err != nil {
			return err
		}
	}
	r.run.finish(45 * time.Second)
	r.journal = r.recorder.snapshot()
	l.remaining = append([]string(nil), r.run.remaining...)
	l.kept, _ = os.ReadFile(r.path)
	l.logInfo, _ = os.Lstat(r.path)
	n.controlAll()
	if err := w.egressInvoke([]string{"status", "--json"}); err != nil {
		return err
	}
	l.pending = w.egress.results[len(w.egress.results)-1]
	return nil
}

func (w *world) egressLifeRemovalRetry() error {
	l, r := w.egress.lifeRemoval, w.egress.lifeRuntime
	if err := errors.Join(egressLifeRunningPair(r.original), egressLifeRunningPair(l.otherBefore), r.run.err); err != nil {
		return err
	}
	if r.run.timedOut || r.run.exit != 75 || !strings.Contains(string(r.run.stderr.bytes()), "run.cleanup-temporary") {
		return fmt.Errorf("removal failure was not recoverable\n%s", r.run.describe())
	}
	if !egressLifeWaitExit(r.journal, r.original.worker.ID, 0) {
		return errors.New("primary zero exit lacks native evidence")
	}
	if err := w.egress.native.exchanged(r.run, 1); err != nil {
		return err
	}
	refused := r.run.results("raw")
	if len(refused) != 1 || !refused[0].OK || refused[0].Status != 403 || refused[0].Body != 0 || !refused[0].Closed {
		return errors.New("refusal was not acknowledged")
	}
	want := "itos container stop " + r.id
	diagnostic := string(r.run.stderr.bytes())
	if strings.Count(diagnostic, want) != 1 {
		return errors.New("run did not print exactly the original public retry")
	}
	l.retry = []string{"itos", "container", "stop", r.id}
	return nil
}

func (w *world) egressLifeRemovalPending() error {
	l, r := w.egress.lifeRemoval, w.egress.lifeRuntime
	entry, problems, err := egressLifeEntry(l.pending, r.id)
	if err != nil {
		return err
	}
	want := map[string]any{"path": r.path, "state": "stored", "records": 1, "missing": []any{}}
	if l.pending.result.exit != 0 || len(problems) != 0 || entry["cleanup"] != "pending" || entry["stop_candidate"] != true || !sameJSON(entry["retry"], l.retry) || !sameJSON(object(entry["egress"])["refusal_log"], want) {
		return errors.New("fresh status lost stored evidence or pending recovery")
	}
	// The stored intent supplies the exact original resource IDs. Compare
	// these to pre-fault daemon observations, independently of status prose.
	token := r.original.proxy.Config.Labels["io.itos-container.intent"]
	data, err := os.ReadFile(w.egressStatePath("intents", w.approval.project, token+".json"))
	var intent map[string]any
	if err != nil || json.Unmarshal(data, &intent) != nil {
		return errors.New("original recovery intent was lost")
	}
	expected := map[string]string{"proxy": r.original.proxy.ID, "network": r.original.networks["egress-network"].ID, "internal": r.original.networks["internal-network"].ID}
	for key, id := range expected {
		if object(object(intent["egress"])[key])["id"] != id {
			return errors.New("recovery identity differs from the original full ID")
		}
	}
	l.beforeRetry, err = doctorTree(w.egressStatePath())
	return err
}

func (w *world) egressLifeRemovalStopped() error {
	l, r := w.egress.lifeRemoval, w.egress.lifeRuntime
	l.fault.mu.Lock()
	path, hits := l.fault.path, l.fault.hits
	l.fault.mu.Unlock()
	if hits != 1 {
		return errors.New("named native removal fault was not reached exactly once")
	}
	if err := egressLifeRemovalBoundary(r.journal, path); err != nil {
		return err
	}
	original := r.original
	anchor := ""
	for _, m := range original.worker.Mounts {
		if m.Destination == "/workspace" {
			anchor = m.Name
		}
	}
	want := []string{"volume " + anchor, "network " + original.networks["internal-network"].ID}
	if l.resource != "internal network" {
		want = append(want, "network "+original.networks["egress-network"].ID)
	}
	if l.resource == "proxy" {
		want = append(want, "container "+original.proxy.ID)
	}
	slices.Sort(want)
	got := append([]string(nil), l.remaining...)
	slices.Sort(got)
	if anchor == "" || !slices.Equal(got, want) {
		return errors.New("later dependencies were removed or unexpected resources retained")
	}
	return w.egressLifeOtherUntouched()
}

func egressLifeRemovalBoundary(journal []egressLifeRequest, path string) error {
	failed := false
	for _, e := range journal {
		if e.method != "DELETE" {
			continue
		}
		if failed {
			return errors.New("removal continued after the injected failure")
		}
		if e.path == path {
			if e.status != 500 {
				return errors.New("named removal did not fail")
			}
			failed = true
		} else if e.status != 204 {
			return errors.New("an earlier removal failed instead")
		}
	}
	if !failed {
		return errors.New("named removal was never attempted")
	}
	return nil
}

func (w *world) egressLifeRemovalRecover() error {
	l, r := w.egress.lifeRemoval, w.egress.lifeRuntime
	if len(l.retry) != 4 {
		return errors.New("no observed retry command")
	}

	if err := w.egressInvoke(append(append([]string{}, l.retry[2:]...), "--json"), []string{"status", "--json"}); err != nil {
		return err
	}
	results := w.egress.results
	l.firstStop, l.firstStatus = results[len(results)-2], results[len(results)-1]

	var err error
	l.afterRetry, err = doctorTree(w.egressStatePath())
	if err != nil {
		return err
	}
	at := len(r.recorder.snapshot())
	if err := w.egressInvoke(append(append([]string{}, l.retry[2:]...), "--json"), []string{"status", "--json"}); err != nil {
		return err
	}
	results = w.egress.results
	l.secondStop, l.secondStatus = results[len(results)-2], results[len(results)-1]
	l.repeatJournal = r.recorder.snapshot()[at:]
	r.journal = r.recorder.snapshot()
	if err := l.other.release("progress-before"); err != nil {
		return err
	}
	if l.other.await("progress-after", 30*time.Second) {
		l.other.observe("progress-after")
	}
	return nil
}

func (w *world) egressLifeRemovalComplete() error {
	l, r := w.egress.lifeRemoval, w.egress.lifeRuntime
	for _, result := range []egressResult{l.firstStop, l.secondStop} {
		if err := egressLifeStoredStatus(result, r.id, r.path, "stored", 1); err != nil {
			return err
		}
	}
	// Failed removal remains in the journal; omit only that known one-shot
	// failure when judging successful full-ID reverse removal and absence.
	var succeeded []egressLifeRequest
	for _, e := range r.journal {
		if e.method == "DELETE" && e.path == l.fault.path && e.status == 500 {
			continue
		}
		succeeded = append(succeeded, e)
	}
	if err := egressLifeReverseCreated(succeeded); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	remaining, err := w.egress.native.owned(ctx)
	if err != nil || len(remaining) != 0 {
		return errors.New("original native resources remain")
	}
	for _, e := range l.repeatJournal {
		if e.method != "GET" && e.method != "HEAD" {
			return errors.New("repeated stop mutated the daemon")
		}
	}
	after, err := doctorTree(w.egressStatePath())
	if err != nil || !reflect.DeepEqual(after, l.afterRetry) {
		return errors.New("repeated stop rewrote protected records")
	}
	return nil
}

func (w *world) egressLifeRemovalStored() error {
	l, r := w.egress.lifeRemoval, w.egress.lifeRuntime
	for _, result := range []egressResult{l.firstStatus, l.secondStatus} {
		if err := egressLifeStoredStatus(result, r.id, r.path, "stored", 1); err != nil {
			return err
		}
	}
	data, err := os.ReadFile(r.path)
	if err != nil || len(l.kept) == 0 || !bytes.Equal(data, l.kept) || bytes.Count(data, []byte{'\n'}) != 1 {
		return errors.New("retry rewrote or duplicated committed refusal evidence")
	}
	info, statErr := os.Lstat(r.path)
	if statErr != nil || l.logInfo == nil || !os.SameFile(info, l.logInfo) || !info.ModTime().Equal(l.logInfo.ModTime()) || l.beforeRetry[r.path] == "" || l.beforeRetry[r.path] != l.afterRetry[r.path] {
		return errors.New("retry replaced the stored log inode or metadata")
	}
	return egressLifePublishedBeforeRemoval(r.journal, r.original.proxy.ID, data)
}

func (w *world) egressLifeOtherUntouched() error {
	l, r := w.egress.lifeRemoval, w.egress.lifeRuntime
	if err := egressLifeRunningPair(l.otherBefore); err != nil {
		return err
	}
	ids := []string{l.otherBefore.worker.ID, l.otherBefore.proxy.ID}
	for _, network := range l.otherBefore.networks {
		ids = append(ids, network.ID)
	}
	for _, e := range r.journal {
		if e.method == "GET" || e.method == "HEAD" {
			continue
		}
		for _, id := range ids {
			if strings.Contains(e.path, id) || bytes.Contains(e.body, []byte(id)) {
				return errors.New("product mutated the other project's resource")
			}
		}
	}
	return nil
}

func (w *world) egressLifeOtherPreserved() error {
	l, r := w.egress.lifeRemoval, w.egress.lifeRuntime
	if err := w.egressLifeOtherUntouched(); err != nil {
		return err
	}
	after := l.other.observed["progress-after"]
	if err := egressLifeRunningPair(after); err != nil {
		return err
	}
	if after.worker.ID != l.otherBefore.worker.ID || after.proxy.ID != l.otherBefore.proxy.ID || after.worker.State.StartedAt != l.otherBefore.worker.State.StartedAt || after.proxy.State.StartedAt != l.otherBefore.proxy.State.StartedAt || after.worker.RestartCount != l.otherBefore.worker.RestartCount || after.proxy.RestartCount != l.otherBefore.proxy.RestartCount {
		return errors.New("other project's resources were replaced or restarted")
	}
	for project, want := range map[*world]string{w: r.edit, l.second: l.otherEdit + "-after"} {
		data, err := os.ReadFile(filepath.Join(project.doctor.checkout, "slice-28-edit"))
		if err != nil || string(data) != want {
			return errors.New("checkout edit or independent post-recovery progress was lost")
		}
		if err := project.approvalRecordUnchanged(); err != nil {
			return err
		}
	}
	return nil
}
