package features

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"

	"github.com/cucumber/godog"
)

type homeNativeSessionEntry struct {
	recoveryEntry
	Home struct {
		Storage, State, Name string
	} `json:"home"`
}

func (w *world) homeNativeEndingSteps(sc *godog.ScenarioContext) {
	sc.Step(`^a CI home checkout with a cached image and a probe that writes a home marker before "([^"]*)"$`, func(ending string) error {
		if err := w.homeNativeFixture(); err != nil {
			return err
		}
		w.home.native.ending = ending
		return nil
	})
	sc.Step(`^the product session ends that way and fresh product status discovers the independently recorded session ID then fresh product stop recovers it and status inspects it again$`, w.homeNativeEnd)
	sc.Step(`^fresh status identifies the original retained home and confirms worker and anchor absence without inventing a workload exit$`, w.homeNativeRecovered)
	sc.Step(`^a later independently approved product run reads the home and temporary markers$`, func() error {
		if err := w.homeNativeReapprove("2"); err != nil {
			return err
		}
		w.homeNativeCompleted("read", "alpha", "beta")
		return nil
	})
	sc.Step(`^the later worker reads the original home marker and cannot read the old temporary marker$`, func() error {
		f := w.home.native
		if len(f.runs) != 2 {
			return errors.New("missing later approved home run")
		}
		run := f.runs[1]
		if err := homeNativeProbeValid(run, w.approval.uid, w.approval.gid, 0); err != nil {
			return err
		}
		if !run.report.HomeBefore || run.report.Marker != "alpha" || !run.report.OwnReadable || run.report.TmpBefore || !run.report.TmpExec ||
			!homeNativeSameVolume(f.runs[0].home, run.home) {
			return errors.New("session ending lost retained state, reused temporary state or changed the home generation")
		}
		return nil
	})
	sc.Step(`^no product operation deletes a home or changes unrelated resources and the later worker is also cleaned$`, func() error {
		if err := w.homeNativeCheckoutPreserved("alpha"); err != nil {
			return err
		}
		if err := w.homeNativeMounts(); err != nil {
			return err
		}
		return w.homeNativeResources()
	})
}

func (w *world) homeNativeReapprove(generation string) error {
	a := w.approval
	a.generation = generation
	return os.WriteFile(a.record, approvalJSON(map[string]any{"schema": 1, "project": a.project, "generation": generation, "fingerprint": a.fingerprint, "settings": a.settings}), 0600)
}

func (w *world) homeNativeEnd() {
	f := w.home.native
	mode := "write"
	if f.ending == "observed exit seven" {
		mode = "write-seven"
	}
	if f.ending == "controller cancellation" || f.ending == "controller death and stop" {
		mode = "write-hold"
	}
	p := w.homeNativeStart(mode, "alpha", "beta")
	f.ended = p
	if p.id != "" {
		switch f.ending {
		case "controller cancellation":
			p.run.err = errors.Join(p.run.err, p.cmd.Process.Signal(os.Interrupt))
		case "controller death and stop":
			p.run.err = errors.Join(p.run.err, p.cmd.Process.Kill())
		}
	}
	f.runs = append(f.runs, p.finish())
	f.status = w.homeNativeInvoke("status", "--json")
	if p.id != "" {
		f.stopped = w.homeNativeInvoke("stop", p.id, "--json")
	}
	f.after = w.homeNativeInvoke("status", "--json")
}

func homeNativeSession(result homeNativeResult, id string) (homeNativeSessionEntry, error) {
	var report struct {
		Schema   int
		OK       bool
		Problems []recoveryProblem
		Sessions []homeNativeSessionEntry
	}
	if result.err != nil {
		return homeNativeSessionEntry{}, result.err
	}
	if result.exit != 0 || json.Unmarshal(result.out, &report) != nil || report.Schema != 1 || !report.OK || report.Problems == nil || len(report.Problems) != 0 {
		return homeNativeSessionEntry{}, fmt.Errorf("home recovery did not return a successful schema-1 result: exit %d, stdout %s, diagnostic %s", result.exit, result.out, result.errout)
	}
	var found []homeNativeSessionEntry
	for _, entry := range report.Sessions {
		if entry.ID == id {
			found = append(found, entry)
		}
	}
	if len(found) != 1 || !offlineHex(id) {
		return homeNativeSessionEntry{}, errors.New("fresh recovery did not identify the independently registered session exactly once")
	}
	return found[0], nil
}

func homeNativeCompletedSession(entry homeNativeSessionEntry, id, project, fingerprint, name string) error {
	if entry.ID != id || entry.Project != project || entry.Fingerprint != fingerprint || entry.State != "stopped" ||
		entry.Resource != "absent" || entry.Anchor != "absent" || entry.Cleanup != "complete" || entry.StopCandidate || entry.Retry != nil ||
		entry.Home.Storage != "volume" || entry.Home.State != "present" || entry.Home.Name != name {
		return fmt.Errorf("recovered session lacks original retained home and confirmed ephemeral cleanup: %+v", entry)
	}
	return nil
}

func (w *world) homeNativeRecovered() error {
	f, a := w.home.native, w.approval
	if len(f.runs) != 1 {
		return errors.New("missing original home run")
	}
	run := f.runs[0]
	code := 0
	if f.ending == "observed exit seven" {
		code = 7
	}
	if f.ending == "controller cancellation" || f.ending == "controller death and stop" {
		code = run.exit // the session assertions below retain only observed workload knowledge
	}
	if err := homeNativeProbeValid(run, a.uid, a.gid, code); err != nil {
		return err
	}
	if run.home == nil || !run.report.OwnReadable || run.report.HomeBefore || len(run.report.Initial) != 0 || !run.report.TmpExec {
		return errors.New("initial native worker did not establish a fresh private home marker")
	}
	before, err := homeNativeSession(f.status, f.ended.id)
	if err != nil {
		return err
	}
	name, _ := run.home["Name"].(string)
	if before.Home.Storage != "volume" || before.Home.State != "present" || before.Home.Name != name {
		return errors.New("fresh status lost the original retained home")
	}
	switch f.ending {
	case "controller death and stop":
		if before.Controller != "absent" || before.Resource != "running" || before.WorkloadExit != nil || !before.StopCandidate ||
			!reflect.DeepEqual(before.Retry, []string{"itos", "container", "stop", f.ended.id}) {
			return errors.New("controller death lost the live session or invented workload knowledge")
		}
	case "normal exit zero", "observed exit seven":
		if before.WorkloadExit == nil || *before.WorkloadExit != code {
			return errors.New("session ending lost its observed workload exit")
		}
	}
	for _, result := range []homeNativeResult{f.stopped, f.after} {
		entry, err := homeNativeSession(result, f.ended.id)
		if err != nil {
			return err
		}
		if err := homeNativeCompletedSession(entry, f.ended.id, a.project, a.fingerprint, name); err != nil {
			return err
		}
		if !reflect.DeepEqual(entry.WorkloadExit, before.WorkloadExit) {
			return errors.New("stop or later status changed observed workload knowledge")
		}
	}
	if len(f.workers) != 1 || len(f.anchors) != 1 || len(f.volumes) != 1 {
		return errors.New("session recovery created replacement resources")
	}
	return w.homeNativeResources()
}
