package features

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"time"
)

type anchorRecoveryWorld struct {
	mode                                       string
	ready, release                             chan struct{}
	held, reached, failDelete, parkLost        bool // recorder fields protected by offlineAPI.mu
	pidPath                                    string
	initial, later                             anchorRecoveryProbe
	workerBefore                               map[string]any
	boundary                                   bool
	protected, tree, statusBefore, statusAfter map[string]string
	completed                                  map[string]sessionRecord
	recoveryIndex, repeatedIndex, foreignIndex int
	foreign                                    *anchorVolume
	foreignStatus, foreignStop, cross          *recoveryProcess
	child                                      *int
}

type anchorRecoveryProbe struct {
	Sequence         int
	Identity, Nonce  string
	Denied, Controls map[string]bool
	Error            string
}

func (w *world) anchorsFixture(mode string) error {
	if err := doctorRealGuard(); err != nil {
		return err
	}
	if err := w.anchorsPrepare(mode); err != nil {
		return err
	}
	if mode != "foreign" && mode != "pair" {
		return nil
	}
	other := &world{root: w.root, bin: w.bin, coverdir: w.coverdir, dir: filepath.Join(w.dir, "slice-17-recovery-b")}
	w.recovery.second = other
	if err := os.Mkdir(other.dir, 0700); err != nil {
		return err
	}
	if err := other.anchorsPrepare("control"); err != nil {
		return err
	}
	if filepath.Base(w.doctor.checkout) != filepath.Base(other.doctor.checkout) || w.doctor.checkout == other.doctor.checkout {
		return errors.New("anchor fixtures are not disjoint equal-basename checkouts")
	}
	return nil
}

func (w *world) anchorsPrepare(mode string) error {
	if err := w.prepareAnchor(""); err != nil {
		return err
	}
	w.anchors = &anchorRecoveryWorld{mode: mode, pidPath: filepath.Join(w.dir, "slice-11-pid-1"), ready: make(chan struct{}), release: make(chan struct{}), failDelete: mode == "foreign"}
	w.recovery = &recoveryNative{mode: mode}
	// Recovery always executes outside the replaceable tree with ITOS_ROOT set
	// to the original effective root by the existing traced command runner.
	w.recoveryLocal = &recoveryLocal{invocationDir: w.dir}
	a := w.offline.api
	a.mu.Lock()
	a.recovery, a.failRemove = true, mode == "retry"
	a.mu.Unlock()
	if err := recoveryBuildHelper(w.root, filepath.Join(w.anchor.original, "slice-17-recovery-probe"), anchorRecoveryProbeSource); err != nil {
		return err
	}
	if _, err := w.recoveryTraceBinary(); err != nil {
		return err
	}
	var err error
	w.anchors.tree, err = anchorTree(w.anchor.original)
	return err
}

func (w *world) anchorsStart() {
	w.recovery.run = w.recoveryStart("run", "--", "/workspace/slice-17-recovery-probe")
}

func (w *world) anchorsAwait(after int) anchorRecoveryProbe {
	deadline, tick := time.NewTimer(35*time.Second), time.NewTicker(20*time.Millisecond)
	defer deadline.Stop()
	defer tick.Stop()
	for {
		select {
		case <-deadline.C:
			w.recovery.err = errors.Join(w.recovery.err, errors.New("anchor identity probe missed its bounded barrier"))
			return anchorRecoveryProbe{}
		case <-tick.C:
			var report anchorRecoveryProbe
			data, err := os.ReadFile(filepath.Join(w.anchor.original, "subdir", "slice-17-recovery-report"))
			if err == nil && json.Unmarshal(data, &report) == nil && report.Sequence > after {
				return report
			}
			// After controller loss the worker is expected to outlive this process.
			if after == 0 {
				select {
				case <-w.recovery.run.done:
					return report
				default:
				}
			}
		}
	}
}

func (w *world) anchorsLive() {
	w.anchors.initial = w.anchorsAwait(0)
	w.anchorsCapture()
	paths := make([]string, 0, len(w.anchor.records))
	for path := range w.anchor.records {
		paths = append(paths, path)
	}
	// This is a fixture handshake inside the one existing checkout mount.
	w.recovery.err = errors.Join(w.recovery.err, os.WriteFile(filepath.Join(w.anchor.original, "subdir", "slice-17-recovery-targets"), approvalJSON(paths), 0600))
}

// Sample after the event under test, then require another atomic report. A
// report merely newer than initial startup might still predate controller loss.
func (w *world) anchorsProgress() anchorRecoveryProbe {
	var current anchorRecoveryProbe
	data, err := os.ReadFile(filepath.Join(w.anchor.original, "subdir", "slice-17-recovery-report"))
	if err != nil || json.Unmarshal(data, &current) != nil {
		w.recovery.err = errors.Join(w.recovery.err, errors.New("missing post-event probe checkpoint"))
		return anchorRecoveryProbe{}
	}
	return w.anchorsAwait(current.Sequence)
}

func (w *world) anchorsCapture() {
	r, s, a := w.recovery, w.anchor, w.offline.api
	w.recoveryDiscover()
	var err error
	s.records, err = w.anchorRecords()
	r.err = errors.Join(r.err, err)
	w.anchors.protected, err = w.anchorSelectedRecords()
	r.err = errors.Join(r.err, err)
	if r.worker != "" {
		w.anchors.workerBefore, err = a.inspect(r.worker)
		r.err = errors.Join(r.err, err)
		a.mu.Lock()
		w.anchors.boundary = err == nil && a.safeObserved(w.anchors.workerBefore)
		a.mu.Unlock()
	}
	for _, v := range s.volumes {
		v.retained, err = a.anchorInspect(v)
		r.err = errors.Join(r.err, err)
	}
}

func (w *world) anchorsKill() {
	r := w.recovery
	r.err = errors.Join(r.err, r.run.killController())
	r.run.wait()
	w.anchorsCapture()
}

func (w *world) anchorsStatus() {
	r := w.recovery
	var err error
	w.anchors.statusBefore, err = doctorTree(filepath.Join(w.doctor.env["XDG_STATE_HOME"], "itos-container"))
	r.err = errors.Join(r.err, err)
	a := w.offline.api
	a.mu.Lock()
	w.anchors.recoveryIndex = len(a.requests)
	a.mu.Unlock()
	r.status = w.recoveryStart("status", "--json")
	r.status.wait()
	w.anchors.statusAfter, err = doctorTree(filepath.Join(w.doctor.env["XDG_STATE_HOME"], "itos-container"))
	r.err = errors.Join(r.err, err)
}

func (w *world) anchorsStopTwice(args ...string) {
	r := w.recovery
	if !offlineHex(r.id) {
		return
	}
	if len(args) == 0 {
		args = []string{"stop", r.id, "--json"}
	}
	first := w.recoveryStart(args...)
	first.wait()
	r.stops = append(r.stops, first)
	w.anchorsFinishStops(args)
}

func (w *world) anchorsFinishStops(args []string) {
	r, a := w.recovery, w.offline.api
	var err error
	w.anchors.completed, err = w.anchorsCompletionRecords()
	r.err = errors.Join(r.err, err)
	a.mu.Lock()
	w.anchors.repeatedIndex = len(a.requests)
	a.mu.Unlock()
	second := w.recoveryStart(args...)
	second.wait()
	r.stops = append(r.stops, second)
	r.after = w.recoveryStart("status", "--json")
	r.after.wait()
}

func (w *world) anchorsLoss() {
	w.anchorsStart()
	w.anchorsLive()
	if w.anchors.initial.Sequence > 0 {
		w.recovery.err = errors.Join(w.recovery.err, w.anchorSubstitute("checkout root replaced by real directory"))
		w.anchorsKill()
		w.anchors.later = w.anchorsProgress()
	}
	w.anchorsStatus()
	w.anchorsStopTwice()
}

func (w *world) anchorsRetry(child int) {
	w.anchors.child = &child
	w.anchorsStart()
	w.anchorsLive()
	if w.anchors.initial.Sequence > 0 {
		w.anchors.later = w.anchorsProgress()
		// The worker polls this path. Publish a complete request so it cannot
		// parse the empty file between creation and WriteFile's write as exit 96.
		path := filepath.Join(w.anchor.original, "subdir", "slice-17-recovery-exit")
		err := os.WriteFile(path+".next", []byte(strconv.Itoa(child)), 0600)
		if err == nil {
			err = os.Rename(path+".next", path)
		}
		w.recovery.err = errors.Join(w.recovery.err, err)
	}
	r := w.recovery
	r.run.wait()
	w.anchorsCapture()
	matches := recoveryRetryPattern.FindAllSubmatch(r.run.errout.bytes(), -1)
	if len(matches) == 1 {
		r.retry = []string{"stop", string(matches[0][1])}
	}
	a := w.offline.api
	a.mu.Lock()
	a.failRemove = false
	a.mu.Unlock()
	w.anchorsStatus()
	if len(r.retry) > 0 {
		w.anchorsStopTwice(r.retry...)
	}
}

func (w *world) anchorsUncertain() {
	w.anchorsStart()
	s, r := w.anchors, w.recovery
	timer := time.NewTimer(35 * time.Second)
	select {
	case <-s.ready:
		s.reached = true
		w.anchorsKill()
	case <-r.run.done:
	case <-timer.C:
		r.err = errors.Join(r.err, errors.New("successful startup response barrier not reached"))
	}
	timer.Stop()
	close(s.release)
	if s.mode == "start" && s.reached {
		w.anchorsLive()
		s.later = w.anchorsProgress()
	}
	w.anchorsCapture()
	w.anchorsStatus()
	w.anchorsStopTwice()
}

// A complete state tree comparison is used around status: creating a lock or
// repairing a record is visible even if its bytes happen to be unchanged.
func (w *world) anchorsStatusUnchanged() error {
	if w.anchors.statusBefore == nil || !reflect.DeepEqual(w.anchors.statusBefore, w.anchors.statusAfter) {
		return errors.New("anchor status mutated protected state or locks")
	}
	return nil
}

const anchorRecoveryProbeSource = `package main
import("encoding/json";"fmt";"os";"strconv";"syscall";"time")
type report struct {Sequence int; Identity,Nonce string; Denied,Controls map[string]bool; Error string}
func main(){
 for n:=1;;n++ {
  r:=report{Sequence:n,Denied:map[string]bool{},Controls:map[string]bool{}}
  var st syscall.Stat_t
  if err:=syscall.Stat("/workspace",&st);err!=nil{r.Error=err.Error()}else{r.Identity=fmt.Sprintf("%d:%d",st.Dev,st.Ino)}
  nonce,err:=os.ReadFile("/workspace/slice-17-nonce");if err!=nil{r.Error=err.Error()};r.Nonce=string(nonce)
  var targets []string
  data,_:=os.ReadFile("slice-17-recovery-targets");_ = json.Unmarshal(data,&targets)
  for _,phase:=range []string{"before","after"}{
   control:="/tmp/slice-17-recovery-control-"+phase
   err:=os.WriteFile(control,nonce,0600);got,readErr:=os.ReadFile(control)
   r.Controls[phase]=err==nil&&readErr==nil&&string(got)==r.Nonce
   if phase=="before" { for _,path:=range append([]string{"/workspace/slice-17-replacement","/workspace/slice-17-nested/marker"},targets...){_,err:=os.ReadFile(path);r.Denied[path]=err!=nil} }
  }
  if n==1 { if err:=os.WriteFile("slice-17-recovery-edit",[]byte(strconv.Itoa(n)+":"+r.Nonce),0600);err!=nil{r.Error=err.Error()} }
  data,_=json.Marshal(r)
  if os.WriteFile("slice-17-recovery-report.next",data,0600)!=nil{os.Exit(96)}
  if os.Rename("slice-17-recovery-report.next","slice-17-recovery-report")!=nil{os.Exit(96)}
  if data,err:=os.ReadFile("slice-17-recovery-exit");err==nil{code,err:=strconv.Atoi(string(data));if err!=nil{os.Exit(96)};os.Exit(code)}
  time.Sleep(50*time.Millisecond)
 }
}`

func (w *world) anchorsCompletionRecords() (map[string]sessionRecord, error) {
	result, err := w.anchorRecords()
	if err != nil {
		return nil, err
	}
	for _, path := range []string{w.sessionRecordPath(), filepath.Join(w.doctor.env["XDG_STATE_HOME"], "itos-container", "sessions", w.recoveryIndexKey(), w.recovery.id, "record.json")} {
		record, readErr := sessionReadRecord(path)
		if readErr != nil {
			return nil, readErr
		}
		result[path] = record
	}
	return result, nil
}

func (w *world) anchorsProbeCheck(p anchorRecoveryProbe) error {
	if p.Sequence <= 0 || p.Error != "" || p.Identity != w.anchor.originalID || p.Nonce != w.anchor.nonce {
		return fmt.Errorf("missing original-inode live probe: sequence %d, identity %s, error %q", p.Sequence, p.Identity, p.Error)
	}
	if !p.Controls["before"] || !p.Controls["after"] {
		return errors.New("worker-local read/write controls failed")
	}
	for _, path := range []string{"/workspace/slice-17-replacement", "/workspace/slice-17-nested/marker"} {
		if !p.Denied[path] {
			return errors.New("replacement or nested host source readable")
		}
	}
	for path := range w.anchor.records {
		if !p.Denied[path] {
			return errors.New("anchor control record read was not denied")
		}
	}
	return nil
}

func anchorRecoveryOperation(q offlineRequest, operation string) bool {
	return q.Method == "POST" && ((operation == "create" && q.Path == "/containers/create") || (operation == "start" && strings.HasSuffix(q.Path, "/start")))
}
