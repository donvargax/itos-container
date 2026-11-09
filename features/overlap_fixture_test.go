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

type overlapFixture struct {
	worlds                         map[string]*world
	engines                        map[string]*overlapEngine
	native                         bool
	firstName, condition, boundary string
	runs                           []*overlapRun
	first, second, contender       *overlapRun
	refusals, controls             []*overlapRun
	err                            error
	before                         map[string]string
	identities                     map[string]map[string]any
	resources                      map[string]string
	rendezvous                     *overlapRendezvous
	gate                           chan struct{}
	drainBound                     time.Duration // zero means overlapDrainBound
}

// How long a covered fixture waits for its controllers to drain after the
// assertions. A healthy drain returns when its last controller joins, so this
// only lengthens the wait on a hung one. Once waitRun returns, the product's
// teardown takes 30 s (internal/engine/session/domain/registered_run.go), then
// the controller exits and Go exports its counters; this adds five seconds for
// that, which is also the 'stale controller exits' wait's 35 s. A timeout means
// a stuck controller: diagnose it, never raise this again.
const overlapDrainBound = 35 * time.Second

type overlapRun struct {
	w                                         *world
	engine                                    *overlapEngine
	process                                   *recoveryProcess
	probe                                     *recoveryProbe
	collected                                 bool
	marker, id, token, worker, record, intent string
	owned, public, snapshot                   sessionRecord
	paused                                    bool
	heartbeat                                 int
	stop                                      *recoveryProcess
	stopEntry                                 recoveryEntry
	beforeResume                              int
}

func (w *world) overlapSetup(child, engines string, native bool) error {
	if native {
		if err := doctorRealGuard(); err != nil {
			return err
		}
	}
	f := &overlapFixture{worlds: map[string]*world{}, engines: map[string]*overlapEngine{}, native: native, firstName: "ancestor", identities: map[string]map[string]any{}}
	w.overlap = f
	base := filepath.Join(w.dir, "slice-16")
	state := filepath.Join(base, "control", "state")
	if err := os.MkdirAll(state, 0700); err != nil {
		return err
	}
	paths := map[string]string{"ancestor": filepath.Join(base, "projects", "work"), "descendant": filepath.Join(base, "projects", "work", child), "prefix": filepath.Join(base, "projects", "work-more"), "basename": filepath.Join(base, "other", "work")}
	for _, name := range []string{"ancestor", "descendant", "prefix", "basename"} {
		p := &world{root: w.root, bin: w.bin, coverdir: w.coverdir, dir: filepath.Join(base, "control", name)}
		f.worlds[name] = p
		if err := os.MkdirAll(p.dir, 0700); err != nil {
			return err
		}
		if native {
			if err := p.sessionFixture(true, "0022"); err != nil {
				return err
			}
			p.offline = &offlineRuntime{mode: "recovery"}
			if err := p.startOfflineAPI(); err != nil {
				return err
			}
			p.offline.api.recovery = true
		} else {
			if err := p.approvalCheckout(); err != nil {
				return err
			}
			p.session = &sessionWorld{}
		}
		if err := os.MkdirAll(filepath.Dir(paths[name]), 0700); err != nil {
			return err
		}
		if err := os.Rename(p.doctor.checkout, paths[name]); err != nil {
			return err
		}
		p.doctor.checkout, p.doctor.cwd = paths[name], paths[name]
		p.doctor.env["XDG_STATE_HOME"], p.doctor.env["ITOS_ROOT"] = state, paths[name]
		p.recovery = &recoveryNative{mode: "overlap"}
		p.recoveryLocal = &recoveryLocal{}
		if !native {
			a := f.engines["ancestor"]
			if a == nil || (name == "descendant" && engines == "distinct daemons") {
				var err error
				a, err = newOverlapEngine(p, "slice-16-daemon-"+name)
				if err != nil {
					return err
				}
			}
			f.engines[name] = a
			p.doctor.env["DOCKER_HOST"] = a.endpoint
			p.doctor.engine.info["ID"] = a.metadata.info["ID"]
			p.doctor.engineID = fmt.Sprint(a.metadata.info["ID"])
		}
		if err := p.approvalSeed("1", false); err != nil {
			return err
		}
		if err := p.sessionSeedSnapshot(); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(p.doctor.checkout, "slice-16-edit"), []byte("retain original edit"), 0600); err != nil {
			return err
		}
	}
	if native {
		helper := filepath.Join(base, "control", "slice-16-probe")
		if err := recoveryBuildHelper(w.root, helper, overlapWorkerSource); err != nil {
			return err
		}
		data, err := os.ReadFile(helper)
		if err != nil {
			return err
		}
		for _, p := range f.worlds {
			if err = os.WriteFile(filepath.Join(p.doctor.checkout, "slice-16-probe"), data, 0700); err != nil {
				return err
			}
		}
		f.resources, err = f.worlds["ancestor"].doctor.real.snapshotResources()
		if err != nil {
			return err
		}
	}
	for name, p := range f.worlds {
		identity, err := doctorCheckoutIdentity(p.doctor.checkout)
		if err != nil {
			return err
		}
		f.identities[name] = identity
	}
	var err error
	f.before, err = f.protected()
	return err
}

func (f *overlapFixture) other() string {
	if f.firstName == "ancestor" {
		return "descendant"
	}
	return "ancestor"
}
func (f *overlapFixture) remember(err error) { f.err = errors.Join(f.err, err) }

func (f *overlapFixture) start(name, boundary, fault string) *overlapRun {
	w := f.worlds[name]
	r := &overlapRun{w: w, engine: f.engines[name], marker: fmt.Sprintf("slice-16-marker-%d", len(f.runs)+1)}
	f.runs = append(f.runs, r)
	args := []string{"slice-16-workload", r.marker}
	if f.native {
		args = []string{"/workspace/slice-16-probe", r.marker}
	}
	if boundary != "" || fault != "" {
		w.recoveryLocal.boundary, w.recoveryLocal.fault = boundary, fault
		var err error
		r.probe, err = w.recoveryProbeStart(append([]string{"run"}, args...)...)
		if err != nil {
			f.remember(err)
			return r
		}
		r.process = r.probe.process
	} else {
		r.process = w.recoveryStart(append([]string{"run", "--"}, args...)...)
	}
	return r
}

func (r *overlapRun) ended() bool {
	if r.process == nil {
		return true
	}
	select {
	case <-r.process.done:
		return true
	default:
		return false
	}
}

func (r *overlapRun) result() doctorResult {
	if r.process == nil {
		return doctorResult{exit: -1}
	}
	if r.ended() && r.probe != nil && !r.collected {
		// The test assembly publishes the command's exit and streams separately
		// from Go's test-runner status. Preserve both failures for assertions.
		if err := r.w.recoveryProbeWait(r.probe); err != nil {
			r.process.err = err
		}
		r.collected = true
	}
	code := -1
	if r.ended() {
		code = r.process.exit
	}
	if r.probe != nil && !r.collected {
		out, _ := os.ReadFile(r.probe.stdout)
		stderr, _ := os.ReadFile(r.probe.stderr)
		return doctorResult{exit: code, stdout: string(out), stderr: string(stderr)}
	}
	return doctorResult{exit: code, stdout: string(r.process.out.bytes()), stderr: string(r.process.errout.bytes())}
}

func (r *overlapRun) discover() {
	if r.engine != nil {
		for _, q := range r.engine.journal() {
			if q.method == "POST" && q.path == "/containers/create" && q.pid == r.pid() {
				r.token = q.token
				r.worker = approvalHash(map[string]any{"slice-16-worker": r.token})
			}
		}
	} else if r.w.offline != nil {
		a := r.w.offline.api
		a.mu.Lock()
		for _, q := range a.requests {
			if q.Method == "POST" && q.Path == "/containers/create" {
				cmd, _ := q.Body["Cmd"].([]any)
				if len(cmd) == 2 && cmd[1] == r.marker {
					r.token, _ = object(q.Body["Labels"])["io.itos-container.intent"].(string)
				}
			}
		}
		for id, identity := range a.identities {
			if identity["Name"] == "/itos-container-"+r.token {
				r.worker = id
			}
		}
		a.mu.Unlock()
	}
	root := filepath.Join(r.w.doctor.env["XDG_STATE_HOME"], "itos-container", "sessions", r.w.recoveryIndexKey())
	entries, _ := os.ReadDir(root)
	for _, e := range entries {
		path := filepath.Join(root, e.Name(), "record.json")
		data, _ := os.ReadFile(path)
		var record map[string]any
		if json.Unmarshal(data, &record) != nil {
			continue
		}
		ref, _ := record["intent"].(string)
		if (r.token != "" && strings.HasSuffix(ref, "/"+r.token+".json")) || (r.token == "" && r.paused && len(entries) == 1) {
			r.id, r.record, r.intent = e.Name(), path, filepath.Join(r.w.doctor.env["XDG_STATE_HOME"], "itos-container", ref)
			if r.token == "" {
				r.token = strings.TrimSuffix(filepath.Base(ref), ".json")
			}
		}
	}
}

func (r *overlapRun) pid() int {
	if r.process == nil {
		return -1
	}
	data, _ := os.ReadFile(r.process.pid)
	n, _ := strconv.Atoi(string(data))
	return n
}

func (f *overlapFixture) observe(r *overlapRun, createOnly bool) {
	f.remember(overlapAwait("run reaches create/activity or returns", 20*time.Second, func() bool {
		r.discover()
		return r.ended() || (createOnly && r.worker != "") || r.active()
	}))
	r.discover()
}

func (r *overlapRun) active() bool {
	if r.worker == "" {
		return false
	}
	if r.engine != nil {
		r.engine.mu.Lock()
		defer r.engine.mu.Unlock()
		c := r.engine.workers[r.worker]
		return c != nil && !c.removed && object(c.value["State"])["Running"] == true
	}
	var observed map[string]any
	if r.w.doctor.real.read("/containers/"+r.worker+"/json", &observed, 200) != nil {
		return false
	}
	if r.w.offline.api.identity(observed, r.worker) != nil || object(observed["State"])["Running"] != true {
		return false
	}
	a := r.w.offline.api
	a.mu.Lock()
	anchor, err := a.observeMountAnchor(observed)
	a.mu.Unlock()
	record, recordErr := sessionReadRecord(r.intent)
	view := &isolationRun{id: r.worker, sessionID: r.id, requests: []offlineRequest{{Method: "POST", Path: "/containers/" + r.worker + "/start", Observation: observed, Anchor: anchor, Record: record}}}
	if err != nil || recordErr != nil || (&isolationWorld{w: r.w}).mountBoundary(view) != nil {
		return false
	}
	data, err := os.ReadFile(filepath.Join(r.w.doctor.checkout, r.marker))
	n, parseErr := strconv.Atoi(string(data))
	return err == nil && parseErr == nil && n > 0
}

func (r *overlapRun) capture() error {
	r.discover()
	var err error
	r.public, err = sessionReadRecord(r.record)
	if err != nil {
		return err
	}
	r.owned, err = sessionReadRecord(r.intent)
	if err != nil {
		return err
	}
	r.snapshot, err = sessionReadRecord(r.w.sessionSnapshotPath())
	return err
}

func (f *overlapFixture) stop(r *overlapRun) {
	if r == nil || !offlineHex(r.id) {
		f.remember(errors.New("no registered session available to stop"))
		return
	}
	r.stop = r.w.recoveryStart("stop", "--json", r.id)
	r.stop.wait()
	report, err := recoveryReadReport(r.stop)
	f.remember(err)
	if err == nil {
		for _, e := range report.Sessions {
			if e.ID == r.id {
				r.stopEntry = e
			}
		}
	}
}

func (f *overlapFixture) protected() (map[string]string, error) {
	result := map[string]string{}
	for _, w := range f.worlds {
		tree, err := doctorTree(w.doctor.checkout)
		if err != nil {
			return nil, err
		}
		for path, value := range tree {
			if strings.HasPrefix(filepath.Base(path), "slice-16-marker-") {
				continue
			}
			result[path] = value
		}
		for _, path := range []string{w.approval.record, w.sessionSnapshotPath()} {
			r, err := sessionReadRecord(path)
			if err != nil {
				return nil, err
			}
			result[path] = fmt.Sprint(r)
		}
	}
	return result, nil
}

func (w *world) closeOverlap() error {
	f := w.overlap
	if f == nil {
		return nil
	}
	if f.rendezvous != nil {
		select {
		case <-f.rendezvous.release:
		default:
			close(f.rendezvous.release)
		}
	}
	if f.gate != nil {
		select {
		case <-f.gate:
		default:
			close(f.gate)
		}
	}
	var err error
	if !f.native && w.coverageDir() != "" {
		err = f.finishLocalWorkloads()
	}
	for _, p := range f.worlds {
		err = errors.Join(err, p.closeRecoveryNative(), p.closeOfflineRuntime(), p.closeSession())
		p.closeDoctor()
	}
	seen := map[*overlapEngine]bool{}
	for _, a := range f.engines {
		if !seen[a] {
			seen[a] = true
			a.close()
		}
	}
	return err
}

// Called only after a covered scenario's assertions. Complete the existing
// simulated workloads so their controllers can export measured counters on
// normal exit. Plain worlds retain cancellation and joining without waiting for
// product teardown that has no counters to export. Forced cleanup
// still joins every process if the bounded coverage drain fails.
func (f *overlapFixture) finishLocalWorkloads() error {
	var draining []*overlapRun
	for _, r := range f.runs {
		if r.ended() {
			continue
		}
		r.engine.mu.Lock()
		worker := r.engine.workers[r.worker]
		if worker != nil && !worker.removed && object(worker.value["State"])["Running"] == true {
			worker.exit()
			draining = append(draining, r)
		}
		r.engine.mu.Unlock()
	}
	bound := f.drainBound
	if bound == 0 {
		bound = overlapDrainBound
	}
	return overlapAwait("local controllers drain after assertions", bound, func() bool {
		for _, r := range draining {
			if !r.ended() {
				return false
			}
		}
		return true
	})
}

func (f *overlapFixture) preserved() error {
	if f.err != nil {
		return f.err
	}
	now, err := f.protected()
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(now, f.before) {
		return errors.New("overlap operation changed checkout edits, approvals or snapshots")
	}
	for name, w := range f.worlds {
		id, err := doctorCheckoutIdentity(w.doctor.checkout)
		if err != nil || !reflect.DeepEqual(id, f.identities[name]) {
			return errors.New("overlap operation changed root or .git identity")
		}
	}
	if f.native {
		resources, err := f.worlds["ancestor"].doctor.real.snapshotResources()
		if err != nil {
			return err
		}
		var owned []string
		for _, r := range f.runs {
			owned = append(owned, r.worker)
		}
		for _, w := range f.worlds {
			resources, err = w.recoveryWithoutRetainedAnchor(resources)
			if err != nil {
				return err
			}
		}
		if !reflect.DeepEqual(recoveryWithoutWorkers(resources, owned), recoveryWithoutWorkers(f.resources, owned)) {
			return errors.New("overlap operation changed unrelated native resources")
		}
	}
	return nil
}

// Built on the host, executed only by a guarded native product worker.
const overlapWorkerSource = `package main
import ("os"; "strconv"; "time")
func main() {
 if len(os.Args)!=2 {os.Exit(96)}
 path:="/workspace/"+os.Args[1]
 for n:=1;;n++ {
  if os.WriteFile(path+".next",[]byte(strconv.Itoa(n)),0600)!=nil {os.Exit(97)}
  if os.Rename(path+".next",path)!=nil {os.Exit(97)}
  time.Sleep(50*time.Millisecond)
 }
}
`
