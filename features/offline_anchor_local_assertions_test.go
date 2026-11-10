package features

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

func anchorDurable(events []sessionEvent, stage string) bool {
	var selected string
	seen := map[string]bool{}
	for _, e := range events {
		var record map[string]any
		if json.Unmarshal([]byte(e.Bytes), &record) != nil || object(record["anchor"])["stage"] != stage {
			continue
		}
		if e.Bytes != selected {
			selected, seen = e.Bytes, map[string]bool{}
		}
		if e.Failed {
			return false
		}
		seen[e.Operation] = true
	}
	return seen["write"] && seen["file fsync"] && seen["atomic publication"] && seen["parent fsync"]
}

func anchorLocalIntent(w *world) error {
	i := anchorEventIndex(w, "anchor allocate")
	if i < 0 {
		return fmt.Errorf("missing anchor allocation")
	}
	e := w.session.create.Trace[i]
	a := object(e.Record["anchor"])
	if a["stage"] != "intent" || !anchorDurable(e.Writes, "intent") || a["session"] != w.session.create.SessionID || a["device"] != object(e.Binding["Checkout"])["Device"] || a["inode"] != object(e.Binding["Checkout"])["Inode"] {
		return fmt.Errorf("anchor allocated without durable original identity and session intent")
	}
	return nil
}

func anchorLocalOrder(w *world) error {
	if err := anchorLocalIntent(w); err != nil {
		return err
	}
	allocated, inspected, created := anchorEventIndex(w, "anchor allocate"), anchorEventIndex(w, "anchor inspect"), anchorEventIndex(w, "create")
	if inspected <= allocated || created <= inspected {
		return fmt.Errorf("anchor was not independently inspected before worker create")
	}
	e := w.session.create.Trace[created]
	a := object(e.Record["anchor"])
	if a["stage"] != "owned" || a["created"] == "" || !anchorDurable(e.Writes, "owned") {
		return fmt.Errorf("worker consumed an anchor without durable concrete ownership")
	}
	return nil
}

func anchorLocalMount(w *world) error {
	i := anchorEventIndex(w, "anchor inspect")
	if i < 0 {
		return fmt.Errorf("missing anchor observation")
	}
	e := w.session.create.Trace[i]
	o, a := e.Anchor, object(e.Record["anchor"])
	if o["Name"] != a["name"] || o["Driver"] != "local" || o["Scope"] != "local" || o["Type"] != "none" || o["Options"] != "bind" || o["ExactOptions"] != true || o["ExactLabels"] != true || o["Source"] != a["source"] || !regexp.MustCompile(`^/proc/[1-9][0-9]*/fd/[0-9]+$`).MatchString(fmt.Sprint(o["Source"])) {
		return fmt.Errorf("volume does not have the exact owned descriptor-backed bind options")
	}
	if w.session.backend == "disk" {
		for _, q := range w.session.api.requests {
			if q.Path != "/containers/create" {
				continue
			}
			h := object(q.Body["HostConfig"])
			mounts, _ := h["Mounts"].([]any)
			if len(mounts) != 1 {
				return fmt.Errorf("worker has unexpected requested mounts")
			}
			m := object(mounts[0])
			if m["Type"] != "volume" || m["Source"] != o["Name"] || m["Target"] != "/workspace" || m["ReadOnly"] == true || object(m["VolumeOptions"])["NoCopy"] != true {
				return fmt.Errorf("worker did not request only its no-copy anchor volume")
			}
			return nil
		}
		return fmt.Errorf("missing SDK worker create")
	}
	return nil
}

func anchorLocalBoundary(w *world) error {
	if err := anchorLocalMount(w); err != nil {
		return err
	}
	i := anchorEventIndex(w, "mounted stat")
	if i < 0 {
		return fmt.Errorf("missing mounted identity")
	}
	e := w.session.create.Trace[i]
	b := object(e.Binding["Checkout"])
	if e.Source["Device"] != b["Device"] || e.Source["Inode"] != b["Inode"] {
		return fmt.Errorf("worker mount differs from validated identity")
	}
	return w.offlineFrozenBoundary()
}

func anchorLocalDetector(w *world) error {
	t := w.session.create.Trace
	start, stat, closed := anchorEventIndex(w, "start"), anchorEventIndex(w, "mounted stat"), anchorEventIndex(w, "descriptor close")
	if start < 0 || stat <= start || closed <= start || t[start].Failure != "" {
		return fmt.Errorf("missing descriptor lifetime or post-start detector")
	}
	if stat == 0 || t[stat-1].Operation != "inspect" || t[stat-1].Status != 0 || t[stat-1].ID == "" || t[stat-1].Observation.PID != t[stat].Exit || t[stat].Exit <= 0 {
		return fmt.Errorf("mounted stat did not use the freshly inspected init PID")
	}
	for _, e := range t[start+1 : stat] {
		if e.Operation == "wait" {
			return fmt.Errorf("workload wait preceded the detector")
		}
	}
	return nil
}

func anchorLocalImmediate(w *world) error {
	t := w.session.create.Trace
	start, killed, removed := anchorEventIndex(w, "start"), anchorEventIndex(w, "SIGKILL"), anchorEventIndex(w, "remove")
	if start < 0 || killed <= start || removed <= killed {
		return fmt.Errorf("missing immediate owned kill/removal after detector failure")
	}
	check := -1
	for i := start + 1; i < killed; i++ {
		e := t[i]
		if e.Operation == "inspect" && e.ID == t[start].ID {
			check = i
		}
		if e.Operation == "wait" || e.Operation == "SIGTERM" {
			return fmt.Errorf("detector refusal waited gracefully before killing")
		}
	}
	if check < 0 {
		return fmt.Errorf("detector or fresh ownership inspection missing")
	}
	if !strings.Contains(w.session.anchorFault, "inspection") && anchorEventIndex(w, "mounted stat") <= start {
		return fmt.Errorf("missing attempted mounted stat")
	}
	if t[killed].ID != t[start].ID || t[removed].ID != t[start].ID {
		return fmt.Errorf("detector teardown mutated another worker")
	}
	return anchorLocalSafeRelease(w)
}

func anchorLocalSafeRelease(w *world) error {
	t := w.session.create.Trace
	released := anchorEventIndex(w, "anchor remove")
	if released < 0 {
		return nil
	}
	if w.session.anchorFault == "anchor identity is now foreign" || w.session.anchorFault == "protected source replaced after ownership publication" {
		return fmt.Errorf("foreign anchor was released")
	}
	if anchorEventIndex(w, "create") < 0 {
		return nil
	}
	for _, e := range t[:released] {
		if e.Operation == "inspect" && e.Status == 1 && offlineHex(e.ID) {
			return nil
		}
	}
	return fmt.Errorf("anchor released without full-ID worker absence")
}

func anchorLocalRelease(w *world) error {
	if err := anchorLocalSafeRelease(w); err != nil {
		return err
	}
	i := anchorEventIndex(w, "anchor remove")
	if i < 0 {
		return fmt.Errorf("anchor was never released")
	}
	absent := false
	for _, e := range w.session.create.Trace[i+1:] {
		if e.Operation == "anchor inspect" && e.Status == 1 {
			absent = true
		}
	}
	if !absent || anchorRecord(w)["stage"] != "absent" || !anchorDurable(w.session.create.Events, "absent") || w.session.create.Cleanup != "cleanup-complete" {
		return fmt.Errorf("missing confirmed anchor absence and durable completion")
	}
	return nil
}

func anchorLocalSuccess(w *world) error {
	if w.session.create.Code != 0 || w.session.create.Workload == nil || *w.session.create.Workload != 0 || w.session.create.Cleanup != "cleanup-complete" {
		return fmt.Errorf("healthy anchor did not execute and clean: %d %q", w.session.create.Code, w.session.create.Diagnostic)
	}
	return w.offlineEffects()
}

func anchorLocalRetained(w *world) error {
	a := anchorRecord(w)
	r := w.session.create
	if a["name"] == nil || a["source"] == nil || a["session"] != r.SessionID || r.Cleanup == "cleanup-complete" || r.Code == 0 {
		return fmt.Errorf("missing retained protected anchor authority and incomplete cleanup")
	}
	if w.session.final.Mode != 0600 || w.session.final.UID != w.approval.uid {
		return fmt.Errorf("retained anchor authority is unprotected")
	}
	public, err := w.offlinePublicRecord()
	if err != nil {
		return err
	}
	var p map[string]any
	if json.Unmarshal(public, &p) != nil || p["state"] == "stopped" {
		return fmt.Errorf("incomplete anchor cleanup declared stopped")
	}
	return anchorLocalSafeRelease(w)
}

func anchorLocalAccounted(w *world) error {
	if anchorEventIndex(w, "anchor allocate") < 0 {
		return nil
	}
	if w.session.create.Cleanup == "cleanup-complete" {
		return anchorLocalRelease(w)
	}
	return anchorLocalRetained(w)
}

func anchorLocalNoRetry(w *world) error {
	for _, op := range []string{"create", "start", "anchor allocate"} {
		n := 0
		for _, e := range w.session.create.Trace {
			if e.Operation == op {
				n++
			}
		}
		if n > 1 {
			return fmt.Errorf("retried uncertain %s", op)
		}
	}
	for _, q := range w.session.api.requests {
		if q.Path == "/containers/create" {
			mounts, _ := object(q.Body["HostConfig"])["Mounts"].([]any)
			for _, m := range mounts {
				if object(m)["Source"] == w.doctor.checkout {
					return fmt.Errorf("fell back to checkout pathname")
				}
			}
		}
	}
	return anchorLocalSafeRelease(w)
}
