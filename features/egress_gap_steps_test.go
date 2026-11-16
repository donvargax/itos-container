package features

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/cucumber/godog"
)

type egressLife struct {
	operation, id, source, stored, missing string
	sequences                              []int
	beforeRepeat                           map[string]string
}

func (w *world) egressGapSteps(sc *godog.ScenarioContext) {
	sc.Step(`^a local egress lifecycle fixture for "([^"]*)" with observed workload exit "([^"]*)"$`, w.egressGapFixture)
	sc.Step(`^its copied proxy stdout has ready sequence "([^"]*)" and request sequences "([^"]*)"$`, w.egressGapSource)
	sc.Step(`^the product completes that egress operation and fresh status and stop follow$`, w.egressGapRun)
	sc.Step(`^the operation reports "([^"]*)" with missing ranges "([^"]*)" and process exit (\d+)$`, w.egressGapResult)
	sc.Step(`^the committed log holds exactly request sequences "([^"]*)" without ready or renumbering$`, w.egressGapBytes)
	sc.Step(`^fresh status and repeated stop report recovery.refusal-log-gap with exit 1 and the same gapped three-record report$`, w.egressGapReopened)
	sc.Step(`^cleanup is complete with no retry and every owned resource is confirmed absent$`, w.egressGapCleanup)
	sc.Step(`^repeated stop changes neither the committed log nor completed records or engine resources$`, w.egressGapUnchanged)
}

func (w *world) egressGapFixture(operation, workload string) error {
	if operation != "run" && operation != "stop" {
		return fmt.Errorf("unknown egress operation %q", operation)
	}
	life := &egressLife{operation: operation}
	if operation == "stop" {
		if workload != "unknown" {
			return fmt.Errorf("stop fixture requires an unknown workload exit")
		}
		if err := w.egressSeedSessions("running with controller absent"); err != nil {
			return err
		}
		s := w.egress.status.egress
		life.id = s.id
		if err := w.approvalNetworkRequest(2, s.allow); err != nil {
			return err
		}
		a := w.doctor.engine.egress
		a.mu.Lock()
		for role, id := range map[string]string{"worker": s.worker, "proxy": strings.Repeat("9", 64), "internal": strings.Repeat("7", 64), "egress": strings.Repeat("8", 64)} {
			a.ids[role], a.roles[id] = id, role
			if c := a.containers[id]; c != nil {
				a.bodies[role], a.starts[role], a.exited[id] = object(c["Config"]), 1, make(chan struct{})
			} else {
				a.bodies[role] = a.networks[id]
			}
		}
		a.mu.Unlock()
	} else {
		if err := w.approvalCheckout(); err != nil {
			return err
		}
		if err := w.localEgressFixture(); err != nil {
			return err
		}
		if err := w.approvalNetworkRequest(2, []string{"api.example.com"}); err != nil {
			return err
		}
		if err := w.approvalSeed("1", false); err != nil {
			return err
		}
		code, err := strconv.Atoi(workload)
		if err != nil || (code != 0 && code != 17) {
			return fmt.Errorf("invalid workload fixture %q", workload)
		}
		w.doctor.engine.egress.workerExit = code
	}
	w.egress.life = life
	return w.seedControlLocations()
}

func egressRefusalLine(seq int) string {
	return fmt.Sprintf("{\"schema\":1,\"seq\":%d,\"time\":\"2026-11-09T12:00:00Z\",\"event\":\"refused\",\"method\":\"CONNECT\",\"target\":\"unlisted.example\",\"port\":\"443\",\"reason\":\"not-listed\"}\n", seq)
}

func (w *world) egressGapSource(ready, requests string) error {
	life := w.egress.life
	if ready != "1" && ready != "none" {
		return fmt.Errorf("unknown ready sequence %q", ready)
	}
	if ready == "1" {
		life.source = egressProxyReady + "\n"
	}
	for _, value := range strings.Fields(requests) {
		seq, err := strconv.Atoi(value)
		if err != nil || seq < 2 {
			return fmt.Errorf("invalid request sequence")
		}
		life.sequences = append(life.sequences, seq)
		life.stored += egressRefusalLine(seq)
	}
	life.source += life.stored
	a := w.doctor.engine.egress
	a.mu.Lock()
	a.copyStdout = &life.source
	a.mu.Unlock()
	return nil
}

func (w *world) egressGapRun() error {
	life := w.egress.life
	command := []string{"run", "--", "fixture-workload"}
	if life.operation == "stop" {
		command = []string{"stop", life.id}
	}
	if err := w.egressInvoke(command); err != nil {
		return err
	}
	if life.id == "" {
		entries, err := os.ReadDir(w.egressStatePath("sessions", w.recoveryIndexKey()))
		if err == nil && len(entries) == 1 {
			life.id = entries[0].Name()
		}
	}
	// Missing identity is an observation of premature product refusal, checked
	// by Then; never manufacture a session or call stop with a replacement ID.
	if life.id == "" {
		return nil
	}
	var err error
	life.beforeRepeat, err = doctorTree(w.egressStatePath())
	if err != nil {
		return err
	}
	return w.egressInvoke([]string{"status", "--json"}, []string{"stop", life.id, "--json"})
}

func (w *world) egressGapPath() string {
	return w.egressStatePath("sessions", w.recoveryIndexKey(), w.egress.life.id, "egress-refusals.jsonl")
}

func (w *world) egressGapResult(rule, missing string, exit int) error {
	life := w.egress.life
	life.missing = missing
	results := w.egress.results
	if len(results) == 0 {
		return fmt.Errorf("egress operation was not invoked")
	}
	r := results[0]
	if r.result.exit != exit || !strings.Contains(r.result.stderr, rule) {
		return fmt.Errorf("operation exit %d, want %d with %s\n%s%s", r.result.exit, exit, rule, r.result.stdout, r.result.stderr)
	}
	for _, part := range append([]string{life.id, w.egressGapPath()}, strings.Split(missing, ",")...) {
		if part == "" || !strings.Contains(r.result.stderr, part) {
			return fmt.Errorf("gap diagnostic omits its session, destination or inclusive range")
		}
	}
	if !strings.Contains(r.result.stderr, "cannot") || !strings.Contains(r.result.stderr, "recover") {
		return fmt.Errorf("gap diagnostic omits irrecoverable-loss guidance")
	}
	return nil
}

func (w *world) egressGapBytes(requests string) error {
	data, err := os.ReadFile(w.egressGapPath())
	if err != nil {
		return err
	}
	if string(data) != w.egress.life.stored {
		return fmt.Errorf("copied evidence changed or lost request bytes")
	}
	var actual []string
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var record struct {
			Seq   int
			Event string
		}
		if json.Unmarshal([]byte(line), &record) != nil || record.Event != "refused" {
			return fmt.Errorf("stored log contains a malformed or non-refusal record")
		}
		actual = append(actual, strconv.Itoa(record.Seq))
	}
	if strings.Join(actual, " ") != requests {
		return fmt.Errorf("stored sequences were changed")
	}
	info, err := os.Lstat(w.egressGapPath())
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 {
		return fmt.Errorf("stored log is not a private regular file")
	}
	return nil
}

func (w *world) egressGapReports() ([]map[string]any, error) {
	if len(w.egress.results) != 3 {
		return nil, fmt.Errorf("operation did not supply a session to fresh status and stop")
	}
	var entries []map[string]any
	for _, r := range w.egress.results[1:] {
		var report struct {
			Sessions []map[string]any
			Problems []map[string]any
		}
		if json.Unmarshal([]byte(r.result.stdout), &report) != nil || r.result.exit != 1 {
			return nil, fmt.Errorf("%s did not return a problem report with exit 1\n%s%s", r.command, r.result.stdout, r.result.stderr)
		}
		if len(report.Problems) != 1 || report.Problems[0]["rule"] != "recovery.refusal-log-gap" {
			return nil, fmt.Errorf("%s lost or conflated the gap diagnosis", r.command)
		}
		found := false
		for _, entry := range report.Sessions {
			if entry["id"] == w.egress.life.id {
				entries = append(entries, entry)
				found = true
			}
		}
		if !found {
			return nil, fmt.Errorf("%s lost the original session", r.command)
		}
	}
	return entries, nil
}

func (w *world) egressGapReopened() error {
	entries, err := w.egressGapReports()
	if err != nil {
		return err
	}
	missing := []any{}
	for _, interval := range strings.Split(w.egress.life.missing, ",") {
		first, last, ok := strings.Cut(interval, "-")
		if !ok {
			return fmt.Errorf("bad expected gap range")
		}
		lo, err1 := strconv.Atoi(first)
		hi, err2 := strconv.Atoi(last)
		if err1 != nil || err2 != nil {
			return fmt.Errorf("bad expected gap range")
		}
		missing = append(missing, map[string]any{"first": lo, "last": hi})
	}
	want := map[string]any{"path": w.egressGapPath(), "state": "gapped", "records": 3, "missing": missing}
	for _, entry := range entries {
		if !sameJSON(object(entry["egress"])["refusal_log"], want) {
			return fmt.Errorf("fresh process lost the committed count, path or ranges")
		}
	}
	return nil
}

func (w *world) egressGapCleanup() error {
	entries, err := w.egressGapReports()
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry["cleanup"] != "complete" || entry["resource"] != "absent" || entry["anchor"] != "absent" || len(anyList(entry["retry"])) != 0 {
			return fmt.Errorf("gap was mistaken for remaining cleanup")
		}
	}
	a := w.doctor.engine.egress
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.problem != nil {
		return a.problem
	}
	for role, id := range a.ids {
		if a.containers[id] != nil || a.networks[id] != nil {
			return fmt.Errorf("owned %s remains", role)
		}
	}
	if !slices.Equal(a.removed, []string{"worker", "proxy", "egress", "internal", "anchor"}) {
		return fmt.Errorf("removal order = %v", a.removed)
	}
	return nil
}

func (w *world) egressGapUnchanged() error {
	after, err := doctorTree(w.egressStatePath())
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(after, w.egress.life.beforeRepeat) {
		return fmt.Errorf("fresh status or repeated stop changed committed state")
	}
	for _, r := range w.egress.results[1:] {
		for _, request := range r.journal {
			if request.Method != "GET" && request.Method != "HEAD" {
				return fmt.Errorf("completed session caused an engine mutation")
			}
		}
	}
	return w.egressGapBytes(strings.TrimSpace(strings.Join(func() []string {
		var values []string
		for _, seq := range w.egress.life.sequences {
			values = append(values, strconv.Itoa(seq))
		}
		return values
	}(), " ")))
}
