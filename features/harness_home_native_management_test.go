package features

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/creack/pty"
	"github.com/cucumber/godog"
)

type homeNativeManagement struct {
	action, id, successor, obsolete                     string
	live, retired, catalog, retry, reset, deletionRetry homeNativeResult
	liveWorker, liveHome                                map[string]any
	before                                              map[string]runEntry
	initial, later, reread                              homeNativeRun
	err                                                 error
}

func (w *world) homeNativeManagementSteps(sc *godog.ScenarioContext) {
	sc.Step(`^two CI home checkouts with retained markers one live worker and a disposable terminal for action "([^"]*)"$`, func(action string) error {
		if err := w.homeNativePairFixture(); err != nil {
			return err
		}
		// Givens allocate isolated inputs only. The When starts the marker
		// workloads so an early product refusal reaches a behavior assertion.
		w.home.native.management = &homeNativeManagement{action: action}
		master, slave, err := pty.Open()
		if err == nil {
			err = errors.Join(master.Close(), slave.Close())
		}
		return err
	})
	sc.Step(`^home management is attempted while the worker lives then ordinary stop completes and the chosen action receives fresh consent$`, w.homeNativeManage)
	sc.Step(`^the live attempt reports recovery.home-in-use with its worker and home intact and the consented attempt durably retires only the selected home$`, w.homeNativeRetired)
	sc.Step(`^native inspection confirms the original volume absent and the other checkout still reads its retained marker before harness teardown$`, w.homeNativeOtherRetained)
	sc.Step(`^a later independently approved run and repeated management of the old ID are attempted$`, w.homeNativeAfterRetirement)
	sc.Step(`^reset permits one fresh empty home while delete refuses allocation until a separate consented reset and old-ID retries never remove successor data$`, w.homeNativeSuccessor)
}

func (m *homeNativeManagement) flag() string {
	if m.action == "delete obsolete home" {
		return "--delete"
	}
	return "--reset"
}

func (w *world) homeNativeManage() {
	f, peer := w.home.native, w.home.native.second
	m := f.management
	peer.homeNativeCompleted("write", "beta", "alpha")
	p := w.homeNativeStart("write-hold", "alpha", "beta")
	f.ended = p
	if p.id == "" || p.run.home == nil {
		p.cancel()
		m.initial = p.finish()
		return
	}
	m.id = strings.TrimPrefix(fmt.Sprint(p.run.home["Name"]), "itos-container-home-")
	m.live = w.homeNativeConsent(m.id, m.flag())
	m.err = w.doctor.real.read("/containers/"+fmt.Sprint(p.run.worker["Id"])+"/json", &m.liveWorker, 200)
	m.err = errors.Join(m.err, w.doctor.real.read("/volumes/"+fmt.Sprint(p.run.home["Name"]), &m.liveHome, 200))
	f.stopped = w.homeNativeInvoke("stop", p.id, "--json")
	m.initial = p.finish()
	f.runs = append(f.runs, m.initial)
	entry, err := homeNativeSession(f.stopped, p.id)
	if err != nil {
		m.err = errors.Join(m.err, err)
		return
	}
	if err := homeNativeCompletedSession(entry, p.id, w.approval.project, w.approval.fingerprint, fmt.Sprint(p.run.home["Name"])); err != nil {
		m.err = errors.Join(m.err, err)
		return
	}
	// Confirm product cleanup before the fixture simulates external storage loss.
	if err := w.homeNativeEphemeralAbsent(); err != nil {
		m.err = errors.Join(m.err, err)
		return
	}
	if m.action == "reset externally missing home" {
		var current map[string]any
		name := fmt.Sprint(p.run.home["Name"])
		if err := w.doctor.real.read("/volumes/"+name, &current, 200); err != nil || !homeNativeSameVolume(current, p.run.home) {
			m.err = errors.Join(err, errors.New("external-loss fixture refused an untracked home"))
			return
		}
		if err := w.homeNativeRemove("/volumes/" + name); err != nil {
			m.err = err
			return
		}
	}
	if m.action == "delete obsolete home" {
		m.obsolete = filepath.Join(w.dir, "slice-34-original-git")
		if err := os.Rename(filepath.Join(w.doctor.checkout, ".git"), m.obsolete); err != nil {
			m.err = err
			return
		}
		// A different Git inode makes the selected home obsolete. Management
		// must still use the original protected identity, never today's checkout.
		if err := os.Mkdir(filepath.Join(w.doctor.checkout, ".git"), 0700); err != nil {
			m.err = err
			return
		}
	}
	m.before, m.err = runTree(w.doctor.env["SLICE_21_ACCOUNT_HOME"])
	if m.err != nil {
		return
	}
	m.retired = w.homeNativeConsent(m.id, m.flag())
	m.catalog = w.homeNativeInvoke("status", "--homes", "--json")
}

func homeNativeHomeEntry(result homeNativeResult, id string, code int, rule string) (map[string]any, error) {
	if result.err != nil {
		return nil, result.err
	}
	var report map[string]any
	if result.exit != code || json.Unmarshal(result.out, &report) != nil || report["schema"] != float64(1) || report["ok"] != (code == 0) {
		return nil, fmt.Errorf("home management returned exit %d, stdout %s, diagnostic %s; expected %d %s", result.exit, result.out, result.errout, code, rule)
	}
	problems, ok := report["problems"].([]any)
	if !ok || (rule == "" && len(problems) != 0) || (rule != "" && (len(problems) != 1 || object(problems[0])["rule"] != rule || object(problems[0])["exit"] != float64(code))) {
		return nil, errors.New("home management omitted its required typed outcome")
	}
	entries, ok := report["homes"].([]any)
	if !ok {
		return nil, errors.New("home management omitted its home catalog")
	}
	var selected map[string]any
	for _, value := range entries {
		entry := object(value)
		if entry["id"] == id {
			if selected != nil {
				return nil, errors.New("home management duplicated the selected generation")
			}
			selected = entry
		}
	}
	if selected == nil {
		return nil, errors.New("home management lost the selected generation")
	}
	return selected, nil
}

func (w *world) homeNativeRetired() error {
	f, a := w.home.native, w.approval
	m := f.management
	if err := homeNativeProbeValid(m.initial, a.uid, a.gid, m.initial.exit); err != nil {
		return err
	}
	if m.err != nil {
		return m.err
	}
	if !m.initial.report.OwnReadable || !m.initial.report.TmpExec || m.initial.report.HomeBefore || len(m.initial.report.Initial) != 0 {
		return errors.New("management fixture did not observe a fresh home marker from the product worker")
	}
	live, err := homeNativeHomeEntry(m.live, m.id, 1, "recovery.home-in-use")
	if err != nil {
		return err
	}
	if object(m.liveWorker["State"])["Running"] != true || m.liveWorker["Id"] != m.initial.worker["Id"] || !homeNativeSameVolume(m.initial.home, m.liveHome) ||
		!reflect.DeepEqual(live["references"], []any{f.ended.id}) {
		return errors.New("home-in-use attempt changed its live worker, home or session claim")
	}
	want := "reset"
	if m.flag() == "--delete" {
		want = "deleted"
	}
	for _, result := range []homeNativeResult{m.retired, m.catalog} {
		entry, err := homeNativeHomeEntry(result, m.id, 0, "")
		if err != nil {
			return err
		}
		if err := homeNativeRetirementEntry(entry, m.id, f.scope, a.project, fmt.Sprint(m.initial.home["Name"]), want); err != nil {
			return err
		}
		engine := object(entry["engine"])
		if fmt.Sprint(entry["uid"]) != a.uid || fmt.Sprint(entry["gid"]) != a.gid || engine["endpoint"] != w.doctor.env["DOCKER_HOST"] || engine["id"] != w.doctor.engineID {
			return errors.New("retirement changed the original home owner or daemon")
		}
		if entry["successor"] != nil {
			successor, ok := entry["successor"].(string)
			if !ok || !offlineHex(successor) || successor == m.id || (m.successor != "" && m.successor != successor) {
				return errors.New("reset did not reserve one distinct stable successor")
			}
			m.successor = successor
		}
	}
	if !strings.Contains(m.retired.review, "y/N") || !strings.Contains(m.retired.review, m.id) || !strings.Contains(m.retired.review, a.project) {
		return errors.New("home retirement lacked fresh terminal review of its original identity")
	}
	return w.homeNativeDurable(m.retired, m.before)
}

func homeNativeRetirementEntry(entry map[string]any, id, scope, project, name, disposition string) error {
	refs, ok := entry["references"].([]any)
	if entry["id"] != id || entry["scope"] != scope || entry["project"] != project || entry["name"] != name || entry["state"] != "absent" ||
		entry["disposition"] != disposition || !ok || len(refs) != 0 || entry["retry"] != nil ||
		(disposition == "reset" && entry["successor"] == nil) || (disposition == "deleted" && entry["successor"] != nil) {
		return errors.New("retirement did not retain the exact original generation, release its claims and publish the requested disposition")
	}
	return nil
}

func (w *world) homeNativeEphemeralAbsent() error {
	for id := range w.home.native.workers {
		if err := w.doctor.real.read("/containers/"+id+"/json", nil, 404); err != nil {
			return err
		}
	}
	for name := range w.home.native.anchors {
		if err := w.doctor.real.read("/volumes/"+name, nil, 404); err != nil {
			return err
		}
	}
	return nil
}

func (w *world) homeNativeOtherRetained() error {
	m, peer := w.home.native.management, w.home.native.second
	if err := w.doctor.real.read("/volumes/"+fmt.Sprint(m.initial.home["Name"]), nil, 404); err != nil {
		return err
	}
	// A reset reserves an unallocated token; management must not create it.
	if m.successor != "" {
		if err := w.doctor.real.read("/volumes/itos-container-home-"+m.successor, nil, 404); err != nil {
			return err
		}
	}
	peer.homeNativeCompleted("read", "beta", "alpha")
	if err := homeNativeRetained(peer.home.native.runs, "beta", peer.approval.uid, peer.approval.gid); err != nil {
		return err
	}
	if err := peer.homeNativeMounts(); err != nil {
		return err
	}
	return w.homeNativeEphemeralAbsent()
}

func (w *world) homeNativeAfterRetirement() error {
	m := w.home.native.management
	if m.obsolete != "" {
		if err := os.Remove(filepath.Join(w.doctor.checkout, ".git")); err != nil {
			return err
		}
		if err := os.Rename(m.obsolete, filepath.Join(w.doctor.checkout, ".git")); err != nil {
			return err
		}
		m.obsolete = ""
	}
	if err := w.homeNativeReapprove("2"); err != nil {
		return err
	}
	m.later = w.homeNativeCompleted("write", "successor", "alpha")
	if m.flag() == "--delete" {
		m.deletionRetry = w.homeNativeConsent(m.id, "--delete")
		before, err := runTree(w.doctor.env["SLICE_21_ACCOUNT_HOME"])
		if err != nil {
			return err
		}
		m.reset = w.homeNativeConsent(m.id, "--reset")
		m.err = w.homeNativeDurable(m.reset, before)
		if entry, err := homeNativeHomeEntry(m.reset, m.id, 0, ""); err == nil {
			m.successor, _ = entry["successor"].(string)
		}
		w.homeNativeCompleted("write", "successor", "alpha")
	}
	// Repeating the old reset after successor data exists may never retarget it.
	m.retry = w.homeNativeConsent(m.id, "--reset")
	m.reread = w.homeNativeCompleted("read", "successor", "alpha")
	return nil
}

func (w *world) homeNativeSuccessor() error {
	f, a := w.home.native, w.approval
	m := f.management
	created := m.later
	if m.flag() == "--delete" {
		if m.later.err != nil || m.later.exit != 1 || !strings.Contains(string(m.later.errout), "run.home-refused") || m.later.home != nil || m.later.worker != nil {
			return errors.New("deletion did not refuse allocation after independent reapproval")
		}
		if !offlineHex(m.successor) || !strings.Contains(m.reset.review, "y/N") {
			return errors.New("deleted scope did not require separate terminal-consented reset")
		}
		if m.err != nil {
			return m.err
		}
		deleted, err := homeNativeHomeEntry(m.deletionRetry, m.id, 0, "")
		if err != nil {
			return err
		}
		if err := homeNativeRetirementEntry(deleted, m.id, f.scope, a.project, fmt.Sprint(m.initial.home["Name"]), "deleted"); err != nil {
			return err
		}
		if len(f.runs) != 4 {
			return errors.New("missing run after consented reset of deleted scope")
		}
		created = f.runs[2]
	}
	if err := homeNativeProbeValid(created, a.uid, a.gid, 0); err != nil {
		return err
	}
	if created.report.HomeBefore || len(created.report.Initial) != 0 || created.report.OtherReadable || !created.report.OwnReadable || created.home["Name"] != "itos-container-home-"+m.successor {
		return errors.New("consented reset did not allocate exactly its reserved fresh empty home")
	}
	if err := homeNativeProbeValid(m.reread, a.uid, a.gid, 0); err != nil {
		return err
	}
	if !m.reread.report.HomeBefore || m.reread.report.Marker != "successor" || m.reread.report.OtherReadable || !homeNativeSameVolume(created.home, m.reread.home) {
		return errors.New("old-ID retry removed successor data or selected another volume")
	}
	entry, err := homeNativeHomeEntry(m.retry, m.id, 0, "")
	if err != nil || entry["successor"] != m.successor || entry["disposition"] != "reset" || len(f.volumes) != 2 {
		return errors.Join(err, errors.New("old-ID retry lost its original successor or allocated another generation"))
	}
	if err := w.homeNativeEphemeralAbsent(); err != nil {
		return err
	}
	if err := w.doctor.real.read("/volumes/"+fmt.Sprint(m.initial.home["Name"]), nil, 404); err != nil {
		return err
	}
	if err := w.homeNativeCheckoutPreserved("successor"); err != nil {
		return err
	}
	// Retired homes must be absent; keep their tuples for safe harness teardown,
	// but the final unrelated-resource comparison excludes only retained homes.
	return w.homeNativeManagementResources()
}

func (w *world) homeNativeManagementResources() error {
	f := w.home.native
	homes := map[string]map[string]any{}
	for _, project := range w.homeNativeProjects() {
		for name, volume := range project.home.native.volumes {
			homes[name] = volume
		}
	}
	// The primary's retired volume is independently proved absent above.
	old := fmt.Sprint(f.management.initial.home["Name"])
	retired := f.volumes[old]
	delete(f.volumes, old)
	defer func() { f.volumes[old] = retired }()
	if err := w.homeNativeResourcesWith(homes); err != nil {
		return err
	}
	peer := f.second
	if err := peer.homeNativeCheckoutPreserved("beta"); err != nil {
		return err
	}
	return peer.homeNativeResourcesWith(homes)
}
