package features

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"
)

type anchorRecoveryEntry struct {
	recoveryEntry
	Anchor string
}

func (w *world) anchorsEntry(p *recoveryProcess, code int) (anchorRecoveryEntry, error) {
	var entry anchorRecoveryEntry
	if p == nil {
		return entry, errors.New("fresh recovery process did not run")
	}
	p.wait()
	if p.err != nil {
		return entry, p.err
	}
	if p.exit != code {
		return entry, fmt.Errorf("anchor recovery exit %d, expected %d\n%s", p.exit, code, w.anchorRecoveryDiagnostics())
	}
	var report struct {
		Schema   int
		OK       bool
		Problems []recoveryProblem
		Sessions []json.RawMessage
	}
	if json.Unmarshal(p.out.bytes(), &report) != nil || report.Schema != 1 || report.OK != (code == 0) || len(report.Sessions) != 1 {
		return entry, errors.New("missing one original session in schema-1 anchor report")
	}
	if json.Unmarshal(report.Sessions[0], &entry) != nil {
		return entry, errors.New("invalid anchor session result")
	}
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(report.Sessions[0], &fields)
	if !recoverySessionFields(fields, "id", "project", "fingerprint", "state", "resource", "anchor", "controller", "cleanup", "workload_exit", "stop_candidate", "retry") {
		return entry, errors.New("status/stop did not account for the original anchor in its public result")
	}
	if entry.Anchor != "present" && entry.Anchor != "absent" && entry.Anchor != "unknown" {
		return entry, errors.New("invalid anchor observation")
	}
	if entry.ID != w.recovery.id || entry.Project != w.approval.project || entry.Fingerprint != w.approval.fingerprint {
		return entry, errors.New("anchor recovery lost original session identity")
	}
	if code == 0 && len(report.Problems) != 0 {
		return entry, errors.New("successful recovery retained problems")
	}
	return entry, nil
}

// anchorsCause is the world's recorded cause with the diagnostics of that
// world: the cause first, so that a truncated annotation keeps it, and still
// found by errors.Is.
func (w *world) anchorsCause() error {
	cause := w.recovery.err
	if cause == nil {
		return nil
	}
	return &lifecycleEvidenceError{text: cause.Error() + "\n" + w.anchorRecoveryDiagnostics(), causes: []error{cause}}
}

func (w *world) anchorsLostStatus() error {
	if err := w.anchorsCause(); err != nil {
		return err
	}
	s, err := w.anchorsEntry(w.recovery.status, 0)
	if err != nil {
		return err
	}
	if s.State != "running" || s.Resource != "running" || s.Anchor != "present" || s.Cleanup != "pending" || s.Controller != "absent" || s.WorkloadExit != nil || !s.StopCandidate || !reflect.DeepEqual(s.Retry, []string{"itos", "container", "stop", w.recovery.id}) {
		return errors.New("status did not retain running anchored session after controller death with unknown exit and exact retry")
	}
	return w.anchorsStatusUnchanged()
}

func (w *world) anchorsPendingStatus() error {
	if err := w.anchorsCause(); err != nil {
		return err
	}
	s, err := w.anchorsEntry(w.recovery.status, 0)
	if err != nil {
		return err
	}
	if s.Anchor != "present" || s.Cleanup != "pending" || s.Controller != "absent" || s.WorkloadExit != nil || !reflect.DeepEqual(s.Retry, []string{"itos", "container", "stop", w.recovery.id}) {
		return errors.New("uncertain startup status lost retained anchor or invented workload success")
	}
	return w.anchorsStatusUnchanged()
}

func (w *world) anchorsContinuity() error {
	s := w.anchors
	if len(w.anchor.records) == 0 {
		return errors.New("missing actual protected anchor read targets")
	}
	if s.later.Sequence <= s.initial.Sequence {
		return errors.New("worker made no progress after controller loss")
	}
	// The first report may precede the target-path handshake. Both phases must
	// report the original inode and working local controls; the later report
	// must attempt every actual protected path supplied by the harness.
	if s.initial.Identity != w.anchor.originalID || s.initial.Nonce != w.anchor.nonce || !s.initial.Controls["before"] || !s.initial.Controls["after"] {
		return errors.New("initial worker identity/control handshake failed")
	}
	if err := w.anchorsProbeCheck(s.later); err != nil {
		return err
	}
	edit, err := os.ReadFile(filepath.Join(w.anchor.original, "subdir", "slice-17-recovery-edit"))
	if err != nil || !strings.HasSuffix(string(edit), ":"+w.anchor.nonce) {
		return errors.New("original-tree edit missing after controller loss")
	}
	if _, err = os.Stat(filepath.Join(w.anchor.replacement, "subdir", "slice-17-recovery-edit")); !os.IsNotExist(err) {
		return errors.New("replacement tree was edited")
	}
	return nil
}

func (w *world) anchorsOwned() (*anchorVolume, error) {
	if err := w.anchorsCause(); err != nil {
		return nil, err
	}
	if len(w.anchor.volumes) != 1 || len(w.anchor.records) == 0 || !offlineHex(w.recovery.worker) {
		return nil, errors.New("missing original worker and independently owned anchor recovery evidence")
	}
	for _, v := range w.anchor.volumes {
		if !v.beforeWorker || !anchorVolumeMatches(v, v.observed) || !anchorVolumeMatches(v, v.retained) || !reflect.DeepEqual(v.observed, v.retained) {
			return nil, errors.New("original owned volume did not survive controller loss unchanged")
		}
		mounts, _ := w.anchors.workerBefore["Mounts"].([]any)
		found := 0
		for _, value := range mounts {
			m := object(value)
			if m["Destination"] == "/tmp" && m["Type"] == "tmpfs" {
				continue
			}
			if m["Type"] != "volume" || m["Name"] != v.name || m["Destination"] != "/workspace" || m["RW"] != true {
				return nil, errors.New("worker does not retain exactly its original anchor mount")
			}
			found++
		}
		if found != 1 || !w.anchors.boundary {
			return nil, errors.New("retained worker boundary differs from the approved offline boundary")
		}
		for _, record := range w.anchor.records {
			if record.Mode != 0600 || record.UID != w.approval.uid || !strings.Contains(record.Bytes, w.approval.fingerprint) || !strings.Contains(record.Bytes, v.name) {
				return nil, errors.New("anchor lost protected original ownership")
			}
		}
		return v, nil
	}
	return nil, errors.New("missing anchor")
}

func (w *world) anchorsRelease() error {
	// A recorded cause explains a missed barrier; the bare precondition does not.
	if err := w.anchorsCause(); err != nil {
		return err
	}
	if (w.anchors.mode == "create" || w.anchors.mode == "start") && !w.anchors.reached {
		return errors.New("uncertain startup never reached its successful native response barrier")
	}
	v, err := w.anchorsOwned()
	if err != nil {
		return err
	}
	a, r := w.offline.api, w.recovery
	a.mu.Lock()
	requests := append([]offlineRequest(nil), a.requests...)
	a.mu.Unlock()
	workerAbsent, volumeObserved, deleted, volumeAbsent := false, false, false, false
	deletes := 0
	for _, q := range requests {
		if q.Method == "GET" && q.Path == "/containers/"+r.worker+"/json" && q.Status == 404 {
			if !workerAbsent {
				volumeObserved = false
			}
			workerAbsent = true
		}
		if q.Method == "GET" && q.Path == "/volumes/"+v.name && q.Status == 200 && anchorVolumeMatches(v, q.Observation) {
			volumeObserved = true
		}
		if q.Method == "DELETE" && strings.HasPrefix(q.Path, "/volumes/") {
			if !workerAbsent || !volumeObserved || q.Path != "/volumes/"+v.name || (q.Query != "" && q.Query != "force=false") {
				return errors.New("anchor deletion preceded full-ID absence or fresh owned volume inspection")
			}
			if q.Status == 204 {
				deleted = true
				deletes++
			}
		}
		if deleted && q.Method == "GET" && q.Path == "/volumes/"+v.name && q.Status == 404 {
			volumeAbsent = true
		}
	}
	if !workerAbsent || !volumeAbsent || deletes != 1 {
		return errors.New("missing confirmed worker absence followed by exactly one owned anchor deletion and confirmed volume absence")
	}
	if w.doctor.real.read("/containers/"+r.worker+"/json", nil, 404) != nil || w.doctor.real.read("/volumes/"+v.name, nil, 404) != nil {
		return errors.New("independent native observer still sees owned resources")
	}
	return w.anchorsNoExpiredSource()
}

func (w *world) anchorsNoExpiredSource() error {
	for _, p := range w.recovery.processes {
		if p == w.recovery.run {
			continue
		}
		events, err := recoveryReadSyncs(p.journal)
		if err != nil {
			return err
		}
		for _, event := range events {
			if event.Access == "" {
				continue
			}
			for _, v := range w.anchor.volumes {
				source, _ := v.options["device"].(string)
				if source != "" && (event.Path == source || strings.HasPrefix(event.Path, source+"/")) {
					return errors.New("recovery reopened or resolved the expired controller descriptor source")
				}
			}
		}
	}
	return nil
}

func (w *world) anchorsCompleted() error {
	if err := w.anchorsRelease(); err != nil {
		return err
	}
	r, s := w.recovery, w.anchors
	if len(r.stops) < 2 {
		return errors.New("fresh repeated stop did not run")
	}
	for _, p := range r.stops {
		if p.err != nil || p.exit != 0 {
			return errors.New("fresh stop did not confirm cleanup with exit zero")
		}
	}
	entry, err := w.anchorsEntry(r.after, 0)
	if err != nil {
		return err
	}
	if entry.State != "stopped" || entry.Resource != "absent" || entry.Anchor != "absent" || entry.Cleanup != "complete" || entry.Retry != nil || entry.StopCandidate {
		return errors.New("status lacks durable completion of both resources")
	}
	if s.child != nil {
		if entry.WorkloadExit == nil || *entry.WorkloadExit != *s.child {
			return errors.New("recovery lost the observed workload result")
		}
	} else if entry.WorkloadExit != nil && *entry.WorkloadExit == 0 {
		return errors.New("cleanup invented workload success")
	}
	current, err := w.anchorsCompletionRecords()
	if err != nil {
		return err
	}
	if len(current) < 2 || !reflect.DeepEqual(current, s.completed) {
		return errors.New("repeated stop rewrote completed ownership records")
	}
	anchorAbsent := false
	for path, record := range current {
		var value map[string]any
		if json.Unmarshal([]byte(record.Bytes), &value) != nil {
			return errors.New("invalid completed recovery record")
		}
		if object(value["anchor"])["stage"] == "absent" {
			anchorAbsent = true
		}
		if err := recoveryDurableFile(path, value, r.stops[:1]); err != nil {
			return err
		}
		if err := recoveryDurableFile(path, value, r.stops[len(r.stops)-1:]); err != nil {
			return err
		}
	}
	if !anchorAbsent {
		return errors.New("visible completion lacks durable original anchor absence")
	}
	a := w.offline.api
	a.mu.Lock()
	defer a.mu.Unlock()
	if s.repeatedIndex <= 0 || s.repeatedIndex > len(a.requests) {
		return errors.New("missing repeated-stop request checkpoint")
	}
	for _, q := range a.requests[s.repeatedIndex:] {
		if anchorsMutates(q) {
			return errors.New("repeated stop issued another engine mutation")
		}
	}
	return nil
}

func anchorsMutates(q offlineRequest) bool {
	return q.Method == "DELETE" || (q.Method == "POST" && !strings.HasSuffix(q.Path, "/wait"))
}

func (w *world) anchorsNoRelaunch() error {
	a, r := w.offline.api, w.recovery
	a.mu.Lock()
	requests := append([]offlineRequest(nil), a.requests...)
	a.mu.Unlock()
	if w.anchors.recoveryIndex <= 0 {
		return errors.New("missing fresh recovery checkpoint")
	}
	for _, q := range requests[w.anchors.recoveryIndex:] {
		if q.Method == "POST" && !strings.HasSuffix(q.Path, "/kill") && !strings.HasSuffix(q.Path, "/stop") && !strings.HasSuffix(q.Path, "/wait") {
			return errors.New("recovery allocated or relaunched work")
		}
		if anchorsMutates(q) && strings.HasPrefix(q.Path, "/containers/") {
			if q.Record.Mode != 0600 || !strings.Contains(q.Record.Bytes, r.worker) {
				return errors.New("recovery mutated a worker without concrete protected ownership")
			}
			var value map[string]any
			if json.Unmarshal([]byte(q.Record.Bytes), &value) != nil {
				return errors.New("invalid concrete worker record")
			}
			// Check the exact inode visible at mutation against successful fsync
			// observations, including run's acknowledged-create publication.
			if err := anchorsSyncedRecord(q.Record, w.sessionRecordPath(), q.At, r.processes); err != nil {
				return err
			}
		}
	}
	return w.anchorsStatusUnchanged()
}

func anchorsSyncedRecord(record sessionRecord, path string, before time.Time, processes []*recoveryProcess) error {
	for _, p := range processes {
		events, err := recoveryReadSyncs(p.journal)
		if err != nil {
			return err
		}
		synced := false
		for _, event := range events {
			if event.At.IsZero() || !event.At.Before(before) {
				continue
			}
			if !event.Directory && event.Inode == record.Inode && event.Data == record.Bytes {
				synced = true
			}
			if synced && event.Directory && event.Path == filepath.Dir(path) && event.Files[filepath.Base(path)] == record.Inode {
				return nil
			}
		}
	}
	return errors.New("worker ownership at mutation has no durable file/publication evidence")
}

func (w *world) anchorsRunResult(child, result int) error {
	r := w.recovery
	if r.run == nil || r.run.err != nil || r.run.exit != result || !strings.Contains(string(r.run.errout.bytes()), "run.cleanup-temporary") || !reflect.DeepEqual(r.retry, []string{"stop", r.id}) {
		return errors.New("foreground result lost workload precedence or exact executable cleanup retry")
	}
	a := w.offline.api
	a.mu.Lock()
	defer a.mu.Unlock()
	waited, failed := false, false
	for _, q := range a.requests {
		if q.Method == "POST" && q.Path == "/containers/"+r.worker+"/wait" && q.Status == 200 && q.Observation["StatusCode"] == float64(child) {
			waited = true
		}
		if waited && q.Method == "DELETE" && q.Path == "/containers/"+r.worker && q.Status == 503 {
			failed = true
		}
	}
	if !failed {
		return errors.New("worker-only removal fault did not follow genuine workload exit")
	}
	return nil
}

func (w *world) anchorsExit(child int) error {
	s, err := w.anchorsEntry(w.recovery.after, 0)
	if err != nil {
		return err
	}
	if s.WorkloadExit == nil || *s.WorkloadExit != child {
		return errors.New("fresh status lost observed workload exit")
	}
	return w.anchorsCompleted()
}

func (w *world) anchorsPreserved() error {
	if err := w.anchorsPreservedOne(); err != nil {
		return err
	}
	if other := w.recovery.second; other != nil {
		if err := other.anchorsPreservedOne(); err != nil {
			return err
		}
	}
	resources, err := w.doctor.real.snapshotResources()
	if err != nil {
		return err
	}
	before := w.offline.resourcesBefore
	// Only independently identified workers/anchors of the fixture pair are
	// excluded. B's exact identities and liveness are checked separately.
	for _, p := range []*world{w, w.recovery.second} {
		if p == nil {
			continue
		}
		resources = recoveryWithoutWorkers(resources, []string{p.recovery.worker})
		before = recoveryWithoutWorkers(before, []string{p.recovery.worker})
		for name := range p.anchor.volumes {
			resources = anchorsWithoutVolume(resources, name)
			before = anchorsWithoutVolume(before, name)
		}
	}
	if before == nil || !reflect.DeepEqual(before, resources) {
		return errors.New("anchor recovery changed unrelated native resources")
	}
	return nil
}

func anchorsWithoutVolume(resources map[string]string, name string) map[string]string {
	result := make(map[string]string, len(resources))
	for path, value := range resources {
		if path != "/volumes" {
			result[path] = value
			continue
		}
		var kept []string
		for _, line := range strings.Split(value, "\n") {
			var v map[string]any
			_ = json.Unmarshal([]byte(line), &v)
			if v["Name"] != name {
				kept = append(kept, line)
			}
		}
		result[path] = strings.Join(kept, "\n")
	}
	return result
}

func (w *world) anchorsPreservedOne() error {
	protected, err := w.anchorSelectedRecords()
	if err != nil {
		return err
	}
	if w.anchors.protected == nil || !reflect.DeepEqual(protected, w.anchors.protected) {
		return errors.New("recovery changed original approvals or snapshots")
	}
	tree, err := anchorTree(w.anchor.original)
	if err != nil {
		return err
	}
	for _, name := range []string{"edit", "report", "report.next", "targets", "exit"} {
		delete(tree, filepath.Join("subdir", "slice-17-recovery-"+name))
	}
	if !reflect.DeepEqual(tree, w.anchors.tree) {
		return errors.New("original checkout changed beyond named probe edits and fixture handshake")
	}
	replacement, err := anchorTree(w.anchor.replacement)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(replacement, w.anchor.replacementBefore) {
		return errors.New("replacement checkout changed")
	}
	if w.anchors.initial.Sequence > 0 {
		edit, readErr := os.ReadFile(filepath.Join(w.anchor.original, "subdir", "slice-17-recovery-edit"))
		if readErr != nil || !strings.HasSuffix(string(edit), ":"+w.anchor.nonce) {
			return errors.New("original worker edit was lost")
		}
	}
	return nil
}
