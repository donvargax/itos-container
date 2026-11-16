package features

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"unicode"

	"github.com/cucumber/godog"
)

type egressLifeRecovery struct {
	defect, conflicting string
	log                 []byte
	before              map[string]string
	foreign             map[string]any
	intent              map[string]any
}

func (w *world) egressLifeRecoverySteps(sc *godog.ScenarioContext) {
	sc.Step(`^protected egress recovery records with a committed two-record log and ownership defect "([^"]*)"$`, w.egressOwnershipFixture)
	sc.Step(`^independently recorded foreign containers networks and another project's live session$`, w.egressOwnershipControls)
	sc.Step(`^fresh status and stop inspect the original session$`, func() error {
		return w.egressInvoke([]string{"status", "--json"}, []string{"stop", w.egress.life.id, "--json"})
	})
	sc.Step(`^both report recovery.not-owned with exit 1 while preserving the stored two-record log report$`, w.egressOwnershipResult)
	sc.Step(`^neither operation signals removes adopts or rewrites the conflicting resource$`, w.egressOwnershipNoMutation)
	sc.Step(`^original recovery authority and the exact retry remain without a cleanup-complete claim$`, w.egressOwnershipAuthority)
	sc.Step(`^every foreign resource and the other project's progress remain unchanged$`, w.egressOwnershipPreserved)
	sc.Step(`^an original protected egress session with a gapped log under a state-root name containing "([^"]*)"$`, w.egressPathFixture)
	sc.Step(`^fresh status processes inspect that session in plain and JSON modes$`, func() error {
		return w.egressInvoke([]string{"status"}, []string{"status", "--json"})
	})
	sc.Step(`^plain status and its recovery.refusal-log-gap diagnostic escape the complete log path without terminal controls$`, w.egressPathPlain)
	sc.Step(`^the JSON refusal log path decodes to the original filesystem path with the same stored count and gaps$`, w.egressPathJSON)
	sc.Step(`^the log and every protected record remain byte-for-byte unchanged$`, w.egressPathUnchanged)
}

func (w *world) egressOwnershipFixture(defect string) error {
	if err := w.egressGapFixture("stop", "unknown"); err != nil {
		return err
	}
	l := &egressLifeRecovery{defect: defect, log: []byte(egressRefusalLine(2) + egressRefusalLine(3))}
	w.egress.lifeRecovery = l
	if err := w.egressEvidenceWrite(l.log, []any{}); err != nil {
		return err
	}
	a := w.doctor.engine.egress
	a.mu.Lock()
	defer a.mu.Unlock()
	proxy, egress, internal := a.ids["proxy"], a.ids["egress"], a.ids["internal"]
	l.conflicting = proxy
	switch defect {
	case "proxy full ID differs from its observation":
		a.inspectIDMismatch = proxy
	case "proxy image differs from the frozen ID":
		a.containers[proxy]["Image"] = "sha256:" + strings.Repeat("c", 64)
	case "proxy ownership label differs":
		object(object(a.containers[proxy]["Config"])["Labels"])["io.itos-container.session"] = strings.Repeat("c", 64)
	case "egress network full ID differs":
		l.conflicting = egress
		a.networks[egress]["Id"] = strings.Repeat("c", 64)
	case "egress network ownership label differs":
		l.conflicting = egress
		object(a.networks[egress]["Labels"])["io.itos-container.session"] = strings.Repeat("c", 64)
	case "internal network ownership label differs":
		l.conflicting = internal
		object(a.networks[internal]["Labels"])["io.itos-container.session"] = strings.Repeat("c", 64)
	case "name-only candidate conflicts with full ID":
		// The first name lookup finds the candidate; its subsequent full-ID
		// inspection disagrees. No successful adoption is scripted by the test.
		path := w.egressStatePath("intents", w.approval.project, w.egress.status.egress.token+".json")
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var record map[string]any
		if err := json.Unmarshal(data, &record); err != nil {
			return err
		}
		r := object(object(record["egress"])["proxy"])
		r["id"], r["stage"] = "", "intent"
		record["stage"], record["container_id"] = "reserved", ""
		if err := os.WriteFile(path, approvalJSON(record), 0600); err != nil {
			return err
		}
		// This unresolved create predates worker authority. Preserve a valid
		// public seed and leave only the candidate proxy for reconciliation.
		publicPath := w.egressStatePath("sessions", w.recoveryIndexKey(), w.egress.life.id, "record.json")
		publicData, err := os.ReadFile(publicPath)
		if err != nil {
			return err
		}
		var public map[string]any
		if err := json.Unmarshal(publicData, &public); err != nil {
			return err
		}
		public["state"] = "preparing"
		if err := os.WriteFile(publicPath, approvalJSON(public), 0600); err != nil {
			return err
		}
		delete(a.containers, a.ids["worker"])
		a.detach(a.ids["worker"])
		a.inspectIDMismatch = proxy
	default:
		return fmt.Errorf("unknown ownership defect %q", defect)
	}
	var err error
	l.intent, err = a.intent(a.token())
	return err
}

func (w *world) egressOwnershipControls() error {
	a, l := w.doctor.engine.egress, w.egress.lifeRecovery
	a.mu.Lock()
	defer a.mu.Unlock()
	foreign := strings.Repeat("e", 64)
	a.containers[foreign] = map[string]any{"Id": foreign, "Name": "/slice-28-other-project", "Config": map[string]any{"Labels": map[string]any{"io.itos-container.project": strings.Repeat("f", 64)}}, "State": map[string]any{"Running": true, "Pid": 321}, "Progress": "independent-before"}
	a.networks[strings.Repeat("f", 64)] = map[string]any{"Id": strings.Repeat("f", 64), "Name": "slice-28-foreign", "Containers": map[string]any{foreign: map[string]any{"Name": "slice-28-other-project"}}}
	l.foreign = map[string]any{"worker": approvalClone(a.containers[foreign]), "network": approvalClone(a.networks[strings.Repeat("f", 64)]), "offline": approvalClone(a.containers[w.egress.status.offline.worker])}
	var err error
	l.before, err = doctorTree(w.egressStatePath())
	return err
}

func (w *world) egressOwnershipResult() error {
	want := map[string]any{"path": w.egressEvidencePath(), "state": "stored", "records": 2, "missing": []any{}}
	for _, r := range w.egress.results {
		e, problems, err := egressLifeEntry(r, w.egress.life.id)
		if err != nil {
			return fmt.Errorf("%w\n%s%s", err, r.result.stdout, r.result.stderr)
		}
		if r.result.exit != 1 || len(problems) != 1 || problems[0]["rule"] != "recovery.not-owned" {
			return fmt.Errorf("%s did not refuse conflicting ownership as recovery.not-owned\n%s%s", r.command, r.result.stdout, r.result.stderr)
		}
		if !sameJSON(object(e["egress"])["refusal_log"], want) {
			return errors.New("ownership refusal lost the original stored two-record log report")
		}
	}
	return nil
}

func (w *world) egressOwnershipNoMutation() error {
	l := w.egress.lifeRecovery
	for _, r := range w.egress.results {
		for _, request := range r.journal {
			if request.Method != "GET" && request.Method != "HEAD" && (strings.Contains(request.Path, l.conflicting) || strings.Contains(request.Path, "create") || strings.Contains(request.Path, "start")) {
				return errors.New("conflicting ownership caused a mutation or replacement")
			}
		}
	}
	data, err := os.ReadFile(w.egressEvidencePath())
	if err != nil || !bytes.Equal(data, l.log) {
		return errors.New("ownership refusal rewrote the committed log")
	}
	return nil
}

func (w *world) egressOwnershipAuthority() error {
	a, l := w.doctor.engine.egress, w.egress.lifeRecovery
	a.mu.Lock()
	after, err := a.intent(a.token())
	a.mu.Unlock()
	if err != nil {
		return err
	}
	for _, field := range []string{"schema", "project", "generation", "fingerprint", "snapshot", "uid", "gid", "engine_endpoint", "engine_id", "name", "image_reference", "image_id"} {
		if !reflect.DeepEqual(l.intent[field], after[field]) {
			return errors.New("recovery rewrote original authority")
		}
	}
	for _, r := range w.egress.results {
		e, _, err := egressLifeEntry(r, w.egress.life.id)
		if err != nil {
			return err
		}
		if e["cleanup"] == "complete" || !sameJSON(e["retry"], []string{"itos", "container", "stop", w.egress.life.id}) {
			return errors.New("conflicting recovery lost its retry or claimed completion")
		}
	}
	return nil
}

func (w *world) egressOwnershipPreserved() error {
	a, l := w.doctor.engine.egress, w.egress.lifeRecovery
	a.mu.Lock()
	defer a.mu.Unlock()
	actual := map[string]any{"worker": a.containers[strings.Repeat("e", 64)], "network": a.networks[strings.Repeat("f", 64)], "offline": a.containers[w.egress.status.offline.worker]}
	if !sameJSON(actual, l.foreign) || a.problem != nil {
		return errors.New("foreign resources or independent progress changed")
	}
	return nil
}

func (w *world) egressPathFixture(text string) error {
	component, ok := map[string]string{"newline and ANSI escape": "slice-28-\n\x1b[31m-state", "C1 U+0085": "slice-28-\u0085-state", "bidi override U+202E": "slice-28-\u202e-state"}[text]
	if !ok {
		return errors.New("unknown path fixture")
	}
	if err := w.egressEvidenceSeed("completed copy with three refusal records"); err != nil {
		return err
	}
	previous := w.doctor.env["XDG_STATE_HOME"]
	next := filepath.Join(filepath.Dir(previous), component)
	if err := os.Rename(previous, next); err != nil {
		return err
	}
	w.doctor.env["XDG_STATE_HOME"] = next
	w.approval.record = filepath.Join(next, strings.TrimPrefix(w.approval.record, previous+string(os.PathSeparator)))
	data := []byte(egressRefusalLine(2) + egressRefusalLine(5) + egressRefusalLine(7))
	if err := w.egressEvidenceWrite(data, []any{map[string]int{"first": 3, "last": 4}, map[string]int{"first": 6, "last": 6}}); err != nil {
		return err
	}
	l := &egressLifeRecovery{log: data}
	w.egress.lifeRecovery = l
	var err error
	l.before, err = doctorTree(w.egressStatePath())
	return err
}

func (w *world) egressPathPlain() error {
	r := w.egress.results[0].result
	if r.exit != 1 || !strings.Contains(r.stderr, "recovery.refusal-log-gap") {
		return fmt.Errorf("plain status has no durable gap diagnostic with exit 1\n%s%s", r.stdout, r.stderr)
	}
	quoted := strconv.Quote(w.egressEvidencePath())
	for _, output := range []string{r.stdout, r.stderr} {
		if !strings.Contains(output, quoted) {
			return errors.New("plain output lost the complete quoted log path")
		}
		for _, char := range output {
			if (unicode.IsControl(char) && char != '\n' && char != '\t') || unicode.Is(unicode.Cf, char) {
				return errors.New("plain path emitted a terminal control")
			}
		}
	}
	return nil
}

func (w *world) egressPathJSON() error {
	r := w.egress.results[1]
	e, problems, err := egressLifeEntry(r, w.egress.status.egress.id)
	if err != nil {
		return err
	}
	want := map[string]any{"path": w.egressEvidencePath(), "state": "gapped", "records": 3, "missing": []any{map[string]int{"first": 3, "last": 4}, map[string]int{"first": 6, "last": 6}}}
	if r.result.exit != 1 || len(problems) != 1 || problems[0]["rule"] != "recovery.refusal-log-gap" || !sameJSON(object(e["egress"])["refusal_log"], want) {
		return errors.New("JSON changed the path identity or lost committed gap evidence")
	}
	return nil
}

func (w *world) egressPathUnchanged() error {
	after, err := doctorTree(w.egressStatePath())
	if err != nil || !reflect.DeepEqual(after, w.egress.lifeRecovery.before) {
		return errors.New("status changed the protected state under its hostile path")
	}
	data, err := os.ReadFile(w.egressEvidencePath())
	if err != nil || !bytes.Equal(data, w.egress.lifeRecovery.log) {
		return errors.New("status changed the committed refusal bytes")
	}
	for _, r := range w.egress.results {
		for _, request := range r.journal {
			if request.Method != "GET" && request.Method != "HEAD" {
				return errors.New("status mutated a resource")
			}
		}
	}
	return nil
}
