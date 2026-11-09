package features

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/cucumber/godog"
)

type recoveryRegistration struct {
	record sessionRecord
	path   string
	intent sessionRecord
	syncs  []recoverySync
	token  string
}

func (w *world) recoveryRunSteps(sc *godog.ScenarioContext) {
	sc.Step(`^a recovery run fixture with one approved snapshot and two independent invocations$`, func() error { return w.recoveryRunFixture() })
	sc.Step(`^both runs reach the create barrier through each local backend and status observes them after release$`, func() error {
		w.recoveryLocal.boundary = "before create"
		for range 2 {
			p, err := w.recoveryProbeStart("run", "probe")
			if err != nil {
				return err
			}
			w.recoveryLocal.probes = append(w.recoveryLocal.probes, p)
			if !p.await() {
				return w.recoveryProbeWait(p)
			}
		}
		w.recoveryLocal.barrierReached = true
		for _, p := range w.recoveryLocal.probes {
			if err := p.resume(); err != nil {
				return err
			}
			if err := w.recoveryProbeWait(p); err != nil {
				return err
			}
		}
		return w.recoveryInvocation("status", "--json")
	})
	sc.Step(`^each create follows durable public registration and intent publication with distinct random session IDs and distinct operation tokens$`, func() error {
		l := w.recoveryLocal
		if !l.barrierReached || len(l.registrations) != 2 {
			return fmt.Errorf("two runs did not publish registration and intent before create")
		}
		ids, tokens := map[string]bool{}, map[string]bool{}
		for _, r := range l.registrations {
			record, err := sessionDecodeRecord(r.record.Bytes)
			if err != nil {
				return err
			}
			id, _ := record["id"].(string)
			if !offlineHex(id) || !offlineHex(r.token) || id == r.token || ids[id] || tokens[r.token] {
				return fmt.Errorf("public IDs and operation tokens were not independent")
			}
			ids[id], tokens[r.token] = true, true
			if !recoverySynced(r.record, r.path, r.syncs) || !recoverySynced(r.intent, filepath.Join(w.doctor.env["XDG_STATE_HOME"], "itos-container", fmt.Sprint(record["intent"])), r.syncs) {
				return fmt.Errorf("create preceded durable public registration or intent")
			}
		}
		return nil
	})
	sc.Step(`^each status entry resolves exactly its own original intent snapshot and full worker ID$`, func() error {
		r, err := recoveryLocalReport(w.recoveryLocal.results[len(w.recoveryLocal.results)-1], 0)
		if err != nil {
			return err
		}
		if len(r.Sessions) != 2 {
			return fmt.Errorf("status did not retain both sessions")
		}
		for _, e := range r.Sessions {
			if e.Project != w.approval.project || e.Fingerprint != w.approval.fingerprint || e.Cleanup != "complete" {
				return fmt.Errorf("registration lost original binding")
			}
			data, err := os.ReadFile(filepath.Join(filepath.Dir(filepath.Dir(w.recoveryRecordPath())), e.ID, "record.json"))
			if err != nil {
				return err
			}
			var p map[string]any
			_ = json.Unmarshal(data, &p)
			intent, err := os.ReadFile(filepath.Join(w.doctor.env["XDG_STATE_HOME"], "itos-container", fmt.Sprint(p["intent"])))
			if err != nil {
				return err
			}
			var i map[string]any
			_ = json.Unmarshal(intent, &i)
			if i["snapshot"] != "snapshots/"+w.approval.project+"/1.json" || !offlineHex(fmt.Sprint(i["container_id"])) {
				return fmt.Errorf("entry did not resolve original full ID")
			}
		}
		return nil
	})
	sc.Step(`^registration and lock objects have the specified protected paths modes and strict schema without workload data$`, func() error {
		for _, r := range w.recoveryLocal.registrations {
			p, err := sessionDecodeRecord(r.record.Bytes)
			if err != nil {
				return err
			}
			if len(p) != 7 || p["schema"] != float64(1) || p["state"] != "preparing" || p["workload_exit"] != nil || p["last_problem"] != nil || r.record.Mode != 0600 {
				return fmt.Errorf("invalid initial public schema or mode")
			}
			for _, name := range []string{"controller.lock", "operation.lock"} {
				info, err := os.Stat(filepath.Join(filepath.Dir(r.path), name))
				if err != nil || info.Mode().Perm() != 0600 || info.Size() != 0 {
					return fmt.Errorf("unprotected lock")
				}
			}
			info, err := os.Stat(filepath.Dir(r.path))
			if err != nil || info.Mode().Perm() != 0700 {
				return fmt.Errorf("unprotected public directory")
			}
		}
		return nil
	})
	sc.Step(`^a recovery run fixture with registration fault "([^"]*)"$`, func(fault string) error {
		if err := w.recoveryRunFixture(); err != nil {
			return err
		}
		w.recoveryLocal.fault = fault
		if fault == "occupied public ID" {
			w.recoveryLocal.id = strings.Repeat("b", 64)
			dir := filepath.Dir(w.recoveryRecordPath())
			if err := os.MkdirAll(dir, 0700); err != nil {
				return err
			}
			if err := os.WriteFile(filepath.Join(dir, "record.json"), []byte("existing-public-registration"), 0600); err != nil {
				return err
			}
		}
		return w.recoveryCapture()
	})
	sc.Step(`^run attempts public registration through each local backend$`, func() error { return w.recoveryProbeInvoke("run", "probe") })
	sc.Step(`^run refuses registration with cause "([^"]*)" before any create or start and without an executable retry for an unpublished identity$`, func(cause string) error {
		r := w.recoveryLocal.results[0]
		code := map[string]int{"ownership": 1, "internal": 70, "temporary": 75}[cause]
		if r.exit != code || !strings.Contains(r.stderr, "run.session-registration-"+cause) || strings.Contains(r.stderr, "itos container stop ") {
			return fmt.Errorf("registration refusal lost its cause or advertised an unusable retry: %+v", r)
		}
		return w.recoveryNoLaunch()
	})
	sc.Step(`^existing registration intent and snapshot bytes remain intact with no replacement ID draw$`, func() error {
		for path, before := range w.recoveryLocal.before {
			if strings.HasSuffix(path, ".json") {
				now, err := doctorTree(filepath.Dir(path))
				if err != nil || now[path] != before {
					return fmt.Errorf("registration failure overwrote prior protected data")
				}
			}
		}
		return w.recoveryNoLaunch()
	})
	sc.Step(`^a recovery run fixture with observed workload exit (\d+) and transient removal failure$`, func(exit int) error {
		if err := w.recoveryRunFixture(); err != nil {
			return err
		}
		w.recoveryLocal.runExit = exit
		w.recoveryLocal.exit = strconv.Itoa(exit)
		w.recoveryLocal.failRemove = true
		return nil
	})
	sc.Step(`^run finishes, the removal fault is cleared and a fresh process executes its printed stop argv in the same project$`, func() error {
		if err := w.recoveryTracedInvocation("run", "--", "probe"); err != nil {
			return err
		}
		w.recoveryLocalDiscover()
		w.session.api.mu.Lock()
		w.recoveryLocal.failRemove = false
		w.session.api.mu.Unlock()
		return w.recoveryPrintedRetry()
	})
	sc.Step(`^run returns (\d+) with run.cleanup-temporary and a registered session ID whose printed retry succeeds with stop exit zero$`, func(code int) error {
		l := w.recoveryLocal
		if len(l.results) < 2 || l.results[0].exit != code || !strings.Contains(l.results[0].stderr, "run.cleanup-temporary") || !offlineHex(l.id) {
			return fmt.Errorf("run did not retain the observed result and registered retry")
		}
		if _, err := w.recoveryLocalEntry(l.results[1], "stopped", "absent", "complete", l.exit); err != nil {
			return err
		}
		return w.recoveryLocalDurable()
	})
	sc.Step(`^later status reports that session stopped with workload exit (\d+) and no pending retry$`, func(exit int) error {
		if err := w.recoveryInvocation("status", "--json"); err != nil {
			return err
		}
		_, err := w.recoveryLocalEntry(w.recoveryLocal.results[len(w.recoveryLocal.results)-1], "stopped", "absent", "complete", strconv.Itoa(exit))
		return err
	})
	sc.Step(`^recovery preserves run's original result and all unrelated state without creating or starting another worker$`, func() error {
		if len(w.session.api.created) != 1 {
			return fmt.Errorf("retry created another worker")
		}
		return w.recoveryImmutable()
	})
	sc.Step(`^a recovery run fixture with a transition barrier at "([^"]*)"$`, func(boundary string) error {
		if err := w.recoveryRunFixture(); err != nil {
			return err
		}
		w.recoveryLocal.boundary = boundary
		w.recoveryLocal.runExit = 7
		w.recoveryLocal.keepRunning = boundary == "foreground wait"
		w.recoveryLocal.resource = "created"
		if boundary == "foreground wait" {
			w.recoveryLocal.resource = "running"
		}
		return nil
	})
	sc.Step(`^run pauses outside the session operation lock at that barrier and an independent stop completes before run resumes$`, w.recoveryStopPausedRun)
	sc.Step(`^stop returns zero with confirmed full-ID absence and durable cleanup and no run transition starts or recreates the session afterward$`, func() error {
		l := w.recoveryLocal
		if !l.barrierReached || len(l.results) < 2 {
			return fmt.Errorf("run did not reach requested transition")
		}
		if _, err := recoveryLocalReport(l.results[0], 0); err != nil {
			return err
		}
		if w.session.api.local[w.session.seedID] != nil {
			return fmt.Errorf("run recreated a stopped worker")
		}
		for _, q := range w.session.api.requests[l.beforeRequests:] {
			if q.Path == "/containers/create" || strings.HasSuffix(q.Path, "/start") {
				return fmt.Errorf("late run launch after stop")
			}
		}
		return nil
	})
	sc.Step(`^run preserves any observed workload exit without overwriting completion or inventing success$`, func() error {
		l := w.recoveryLocal
		r := l.results[1]
		want := "unknown"
		if l.boundary != "created before start" {
			want = "7"
			if r.exit != 7 {
				return fmt.Errorf("run lost observed workload exit: %+v", r)
			}
		} else if r.exit == 0 {
			return fmt.Errorf("unstarted workload became successful")
		}
		if err := w.recoveryInvocation("status", "--json"); err != nil {
			return err
		}
		_, err := w.recoveryLocalEntry(l.results[len(l.results)-1], "stopped", "absent", "complete", want)
		return err
	})
	sc.Step(`^every engine mutation uses the same session lock and freshly validated ownership$`, func() error {
		if len(w.session.api.created) != 1 {
			return fmt.Errorf("run/stop did not share one owned resource")
		}
		return w.recoveryLocalSignals()
	})
	sc.Step(`^a recovery run fixture whose session reopen deadline expires after create$`, func() error {
		if err := w.recoveryRunFixture(); err != nil {
			return err
		}
		w.recoveryLocal.fault = "session reopen deadline"
		return nil
	})
	sc.Step(`^run attempts the launch and fresh JSON status follows its exit$`, func() error {
		if err := w.recoveryProbeInvoke("run", "probe"); err != nil {
			return err
		}
		w.recoveryLocalDiscover()
		return w.recoveryInvocation("status", "--json")
	})
	sc.Step(`^run removes the created worker and durably completes cleanup despite the reopen failure$`, func() error {
		if len(w.session.api.created) != 1 {
			return fmt.Errorf("run did not create the worker before the reopen failure")
		}
		return w.recoveryLocalDurable()
	})
	sc.Step(`^run returns temporary failure without starting the worker or inventing a workload exit$`, func() error {
		r := w.recoveryLocal.results[0]
		if r.exit != 75 || !strings.Contains(r.stderr, "run.cleanup-incomplete") || strings.Contains(r.stderr, "run.cleanup-temporary") {
			return fmt.Errorf("run lost the launch failure or reported incomplete cleanup: %+v", r)
		}
		for _, q := range w.session.api.requests {
			if strings.HasSuffix(q.Path, "/start") || strings.HasSuffix(q.Path, "/kill") {
				return fmt.Errorf("unstarted worker was started or signalled")
			}
		}
		return nil
	})
	sc.Step(`^status reports the registered session stopped with unknown workload exit and no pending retry$`, func() error {
		_, err := w.recoveryLocalEntry(w.recoveryLocal.results[1], "stopped", "absent", "complete", "unknown")
		return err
	})
	sc.Step(`^a recovery run fixture with a barrier after durable registration and intent publication before any create request$`, func() error {
		if err := w.recoveryRunFixture(); err != nil {
			return err
		}
		w.recoveryLocal.boundary = "before create"
		return nil
	})
	sc.Step(`^the recovery engine fixture is configured with a missing intent name and no concrete resource ID$`, func() error { return nil })
	sc.Step(`^run reaches the barrier, an independent JSON stop finishes before run is released and fresh JSON status follows run's exit$`, func() error {
		if err := w.recoveryStopPausedRun(); err != nil {
			return err
		}
		return w.recoveryInvocation("status", "--json")
	})
	sc.Step(`^stop and the later status report exit zero state stopped resource absent cleanup complete workload exit null and no retry$`, func() error {
		l := w.recoveryLocal
		if !l.barrierReached || len(l.results) != 3 {
			return fmt.Errorf("run did not expose durable pre-create identity")
		}
		for _, i := range []int{0, 2} {
			if _, err := w.recoveryLocalEntry(l.results[i], "stopped", "absent", "complete", "unknown"); err != nil {
				return err
			}
		}
		return nil
	})
	sc.Step(`^stop durably records cancellation with an empty concrete ID and stopped progress before releasing the session operation lock$`, func() error {
		data, err := os.ReadFile(w.recoveryRecordPath())
		if err != nil {
			return err
		}
		var record map[string]any
		_ = json.Unmarshal(data, &record)
		if record["state"] != "stopped" || record["last_problem"] != nil || record["workload_exit"] != nil {
			return fmt.Errorf("stop did not retain completed cancellation")
		}
		if err = recoveryDurableFile(w.recoveryRecordPath(), record, w.recoveryLocal.processes[:1]); err != nil {
			return err
		}
		intent := w.sessionExpectedRecord("cancelled", "")
		return recoveryDurableFile(w.sessionRecordPath(), intent, w.recoveryLocal.processes[:1])
	})
	sc.Step(`^resumed run returns a nonzero exit with unknown workload exit without overwriting cancellation$`, func() error {
		r := w.recoveryLocal.results[1]
		if r.exit == 0 {
			return fmt.Errorf("cancelled run invented workload success: %+v", r)
		}
		data, err := os.ReadFile(w.sessionRecordPath())
		if err != nil {
			return err
		}
		if !recoveryJSONEqual(string(data), w.sessionExpectedRecord("cancelled", "")) {
			return fmt.Errorf("resumed run overwrote cancellation")
		}
		return nil
	})
	sc.Step(`^no create start stop kill or remove request or engine absence lookup occurs after the pre-create barrier$`, func() error {
		if err := w.recoveryNoLaunch(); err != nil {
			return err
		}
		if err := w.recoveryNoMutation(); err != nil {
			return err
		}
		for _, q := range w.session.api.requests {
			if strings.HasPrefix(q.Path, "/containers/") {
				return fmt.Errorf("pre-create cancellation inferred absence from the engine")
			}
		}
		return nil
	})
	sc.Step(`^the cancelled registration and original snapshot remain discoverable with complete cleanup$`, func() error {
		if _, err := os.Stat(w.recoveryRecordPath()); err != nil {
			return err
		}
		return w.recoveryImmutable()
	})
	sc.Step(`^a recovery run fixture with distinct synthetic argv environment stream and engine-body markers and a cleanup fault$`, func() error {
		if err := w.recoveryRunFixture(); err != nil {
			return err
		}
		w.recoveryLocal.failRemove = true
		w.recoveryLocal.markers = []string{"slice-11-argv-marker", "slice-11-env-marker", "slice-11-stream-marker", "slice-11-engine-body-marker"}
		w.doctor.env["RECOVERY_SYNTHETIC_SECRET"] = "slice-11-env-marker"
		return nil
	})
	sc.Step(`^run streams its probe output, JSON status inspects the retained session and JSON stop is retried after clearing the fault$`, func() error {
		if err := w.recoveryTracedInvocation("run", "--", "slice-11-argv-marker"); err != nil {
			return err
		}
		w.recoveryLocalDiscover()
		if err := w.recoveryInvocation("status", "--json"); err != nil {
			return err
		}
		w.recoveryLocal.failRemove = false
		return w.recoveryTracedInvocation("stop", "--json", w.recoveryLocal.id)
	})
	sc.Step(`^status reports the real session ID original fingerprint cleanup pending and exact retry argv without any synthetic marker$`, func() error {
		if _, err := w.recoveryLocalEntry(w.recoveryLocal.results[1], "failed", "exited", "pending", "0"); err != nil {
			return err
		}
		return w.recoveryNoMarkers(w.recoveryLocal.results[1].stdout + w.recoveryLocal.results[1].stderr)
	})
	sc.Step(`^stop returns zero with that session ID original fingerprint cleanup complete and retry null without any synthetic marker$`, func() error {
		if _, err := w.recoveryLocalEntry(w.recoveryLocal.results[2], "stopped", "absent", "complete", "0"); err != nil {
			return err
		}
		return w.recoveryNoMarkers(w.recoveryLocal.results[2].stdout + w.recoveryLocal.results[2].stderr)
	})
	sc.Step(`^controller diagnostics public recovery records intents and lock files contain no raw argv environment response bodies or copied stream bytes$`, func() error {
		for _, r := range w.recoveryLocal.results {
			if err := w.recoveryNoMarkers(r.stderr); err != nil {
				return err
			}
		}
		for _, path := range []string{w.recoveryRecordPath(), w.sessionRecordPath(), filepath.Join(filepath.Dir(w.recoveryRecordPath()), "controller.lock"), filepath.Join(filepath.Dir(w.recoveryRecordPath()), "operation.lock")} {
			b, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if err = w.recoveryNoMarkers(string(b)); err != nil {
				return err
			}
		}
		return nil
	})
	sc.Step(`^the foreground stream still contains its deliberate output marker and the original approved snapshot is preserved$`, func() error {
		if !strings.Contains(w.recoveryLocal.results[0].stdout, "slice-11-stream-marker") {
			return fmt.Errorf("foreground stream was hidden")
		}
		return w.recoveryImmutable()
	})
}

func (w *world) recoveryRunFixture() error {
	if err := w.sessionFixture(false, "0022"); err != nil {
		return err
	}
	if err := w.sessionSeedSnapshot(); err != nil {
		return err
	}
	w.recoveryLocal = &recoveryLocal{runFixture: true, exit: "unknown", workerExit: make(chan struct{})}
	w.recovery = &recoveryNative{mode: "local"}
	w.sessionReplacement()
	return w.recoveryCapture()
}
func (w *world) recoveryNoLaunch() error {
	for _, q := range w.session.api.requests {
		if q.Path == "/containers/create" || strings.HasSuffix(q.Path, "/start") {
			return fmt.Errorf("refusal launched a worker")
		}
	}
	return nil
}
func (w *world) recoveryLocalDiscover() {
	w.session.api.mu.Lock()
	defer w.session.api.mu.Unlock()
	w.recoveryLocalDiscoverLocked()
}

func (w *world) recoveryLocalDiscoverLocked() {
	root := filepath.Join(w.doctor.env["XDG_STATE_HOME"], "itos-container", "sessions", w.recoveryIndexKey())
	dirs, _ := os.ReadDir(root)
	for _, dir := range dirs {
		data, err := os.ReadFile(filepath.Join(root, dir.Name(), "record.json"))
		if err != nil {
			continue
		}
		var r map[string]any
		if json.Unmarshal(data, &r) != nil {
			continue
		}
		ref, _ := r["intent"].(string)
		if strings.HasSuffix(ref, "/"+w.session.token+".json") || w.session.seedID == "" {
			w.recoveryLocal.id = dir.Name()
			w.session.reference = ref
			w.session.token = strings.TrimSuffix(filepath.Base(ref), ".json")
			w.recovery.id = dir.Name()
			w.recovery.worker = w.session.seedID
			return
		}
	}
}
func (w *world) recoveryPrintedRetry() error {
	m := regexp.MustCompile(`itos container stop ([0-9a-f]{64})(?:\s|$)`).FindStringSubmatch(w.recoveryLocal.results[0].stderr)
	if len(m) != 2 {
		w.recoveryLocal.results = append(w.recoveryLocal.results, doctorResult{exit: -1})
		return nil
	}
	return w.recoveryTracedInvocation("stop", "--json", m[1])
}
func (w *world) recoveryStopPausedRun() error {
	p, err := w.recoveryProbeStart("run", "probe")
	if err != nil {
		return err
	}
	w.recoveryLocal.probes = append(w.recoveryLocal.probes, p)
	w.recoveryLocal.barrierReached = p.await()
	w.recoveryLocalDiscover()
	if w.recoveryLocal.barrierReached {
		if err = w.recoveryTracedInvocation("stop", "--json", w.recoveryLocal.id); err != nil {
			return err
		}
		w.recoveryLocal.beforeRequests = len(w.session.api.requests)
		if err = p.resume(); err != nil {
			return err
		}
	}
	return w.recoveryProbeWait(p)
}
func (w *world) recoveryNoMarkers(text string) error {
	for _, marker := range w.recoveryLocal.markers {
		if strings.Contains(text, marker) {
			return fmt.Errorf("recovery disclosed synthetic workload or engine text")
		}
	}
	return nil
}

func (a *sessionAPI) recoveryCreate(r *http.Request, q sessionRequest, index int) {
	w, l := a.w, a.w.recoveryLocal
	id := approvalHash(map[string]any{"fixture-created": w.session.token})
	w.session.seedID = id
	config := approvalClone(q.Body)
	host := config["HostConfig"]
	delete(config, "HostConfig")
	c := map[string]any{"Id": id, "Name": "/" + r.URL.Query().Get("name"), "Image": config["Image"], "Config": config, "HostConfig": host, "State": map[string]any{"Status": "created", "Running": false, "Pid": 0, "StartedAt": "0001-01-01T00:00:00Z"}, "NetworkSettings": map[string]any{"Networks": map[string]any{"none": map[string]any{}}}, "Mounts": []any{map[string]any{"Type": "bind", "Source": w.doctor.checkout, "Destination": "/workspace", "RW": true, "Propagation": "rprivate"}}}
	c["Mounts"] = localAnchorMount(object(host))
	a.local[id], a.observed[id] = c, approvalClone(c)
	a.created = append(a.created, id)
	w.recoveryLocalDiscoverLocked()
	e := recoveryRegistration{path: w.recoveryRecordPath(), token: w.session.token, intent: q.Record}
	e.record, _ = sessionReadRecord(e.path)
	for _, p := range w.recovery.processes {
		events, _ := recoveryReadSyncs(p.journal)
		e.syncs = append(e.syncs, events...)
	}
	l.registrations = append(l.registrations, e)
	a.requests[index].Record = q.Record
}
func (a *sessionAPI) recoveryExit(c map[string]any) {
	l := a.w.recoveryLocal
	c["State"] = map[string]any{"Status": "exited", "Running": false, "Pid": 0, "StartedAt": "2026-01-01T00:00:00Z"}
	if l.workerExit != nil {
		select {
		case <-l.workerExit:
		default:
			close(l.workerExit)
		}
	}
	if a.attached != nil {
		if len(l.markers) > 0 {
			data := []byte("slice-11-stream-marker\n")
			frame := make([]byte, 8)
			frame[0] = 1
			binary.BigEndian.PutUint32(frame[4:], uint32(len(data)))
			_, _ = a.attached.Write(append(frame, data...))
		}
		_ = a.attached.Close()
		a.attached = nil
	}
}
func recoverySynced(record sessionRecord, path string, events []recoverySync) bool {
	file := false
	for _, event := range events {
		if !event.Directory && event.Inode == record.Inode && event.Data == record.Bytes {
			file = true
		}
		if file && event.Directory && event.Path == filepath.Dir(path) && event.Files[filepath.Base(path)] == record.Inode {
			return true
		}
	}
	return false
}
