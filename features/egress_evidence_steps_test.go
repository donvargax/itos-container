package features

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"

	"github.com/cucumber/godog"
)

// The fixture writes original evidence independently of run/status/stop. The
// sidecar binds the ready-inclusive judgement to the ready-filtered bytes.
// A destination without a sidecar is not a committed clean copy.
func (w *world) egressEvidenceSeed(fixture string) error {
	phase := "stopped with cleanup complete"
	if fixture == "running proxy without a published copy" {
		phase = "running with controller absent"
	}
	if err := w.egressSeedSessions(phase); err != nil {
		return err
	}
	if phase == "running with controller absent" {
		return nil
	}
	var data []byte
	switch fixture {
	case "completed copy after a ready-only stream":
	case "completed copy with three refusal records":
		for seq := 2; seq <= 4; seq++ {
			data = append(data, []byte(fmt.Sprintf("{\"schema\":1,\"seq\":%d,\"time\":\"2026-11-09T12:00:00Z\",\"event\":\"refused\",\"method\":\"CONNECT\",\"target\":\"unlisted.example\",\"port\":\"443\",\"reason\":\"not-listed\"}\n", seq))...)
		}
	default:
		return fmt.Errorf("unknown refusal evidence fixture %q", fixture)
	}
	return w.egressEvidenceWrite(data, []any{})
}

func (w *world) egressEvidenceWrite(data []byte, missing []any) error {
	path := w.egressEvidencePath()
	if err := os.WriteFile(path, data, 0600); err != nil {
		return err
	}
	digest := sha256.Sum256(data)
	metadata := map[string]any{"schema": 1, "sha256": hex.EncodeToString(digest[:]), "records": strings.Count(string(data), "\n"), "missing": missing}
	return os.WriteFile(filepath.Join(filepath.Dir(path), "egress-refusals.json"), approvalJSON(metadata), 0600)
}

func (w *world) egressEvidencePath() string {
	return w.egressStatePath("sessions", w.recoveryIndexKey(), w.egress.status.egress.id, "egress-refusals.jsonl")
}

func (w *world) egressEvidenceSteps(sc *godog.ScenarioContext) {
	sc.Step(`^an original protected egress session with evidence fixture "([^"]*)" and a separate offline session$`, w.egressEvidenceSeed)
	sc.Step(`^the current policy and approval differ from both original sessions$`, w.egressCurrentDiffers)
	sc.Step(`^fresh status processes inspect those sessions in plain and JSON modes$`, func() error {
		return w.egressInvoke([]string{"status"}, []string{"status", "--json"})
	})
	sc.Step(`^the egress refusal log report has state "([^"]*)" records "([^"]*)" and missing ranges "([^"]*)"$`, w.egressEvidenceReport)
	sc.Step(`^the report names that session's protected log destination and all frozen egress limitations$`, w.egressEvidenceDestination)
	sc.Step(`^the offline entry omits egress and no status prints request records or claims a complete history$`, w.egressEvidenceNoClaims)
	sc.Step(`^both status invocations return zero without writing state or mutating resources$`, func() error {
		plain, structured, err := w.egressStatusResults()
		if err != nil {
			return err
		}
		if plain.result.exit != 0 || structured.result.exit != 0 {
			return fmt.Errorf("status exits = %d/%d, want 0/0", plain.result.exit, structured.result.exit)
		}
		return w.egressStatusReadOnly()
	})
}

func (w *world) egressEvidenceEntries() (map[string]any, map[string]any, error) {
	_, structured, err := w.egressStatusResults()
	if err != nil {
		return nil, nil, err
	}
	var report struct {
		Schema   int
		Sessions []map[string]any
	}
	if err := json.Unmarshal([]byte(structured.result.stdout), &report); err != nil || report.Schema != 1 || len(report.Sessions) != 2 {
		return nil, nil, fmt.Errorf("status must return two schema-1 session entries: %v (exit %d)\n%s%s", err, structured.result.exit, structured.result.stdout, structured.result.stderr)
	}
	var egress, offline map[string]any
	for _, entry := range report.Sessions {
		switch entry["id"] {
		case w.egress.status.egress.id:
			egress = entry
		case w.egress.status.offline.id:
			offline = entry
		default:
			return nil, nil, fmt.Errorf("status returned an unexpected session")
		}
	}
	if egress == nil || offline == nil {
		return nil, nil, fmt.Errorf("status omitted an original session")
	}
	return egress, offline, nil
}

func (w *world) egressEvidenceReport(state, records, missing string) error {
	entry, _, err := w.egressEvidenceEntries()
	if err != nil {
		return err
	}
	report, present := object(entry["egress"])["refusal_log"]
	if !present {
		return fmt.Errorf("egress status has no refusal_log report; want %s with records=%s", state, records)
	}
	var count any
	if records != "null" {
		n, err := strconv.Atoi(records)
		if err != nil {
			return err
		}
		count = float64(n)
	}
	if missing != "none" {
		return fmt.Errorf("unsupported evidence range fixture %q", missing)
	}
	want := map[string]any{"path": w.egressEvidencePath(), "state": state, "records": count, "missing": []any{}}
	if !reflect.DeepEqual(object(report), want) {
		return fmt.Errorf("refusal_log = %s, want %s", approvalJSON(report), approvalJSON(want))
	}
	return nil
}

func (w *world) egressEvidenceDestination() error {
	if err := w.egressStatusPlain(); err != nil {
		return err
	}
	plain, _, err := w.egressStatusResults()
	if err != nil {
		return err
	}
	block := egressStatusBlocks(plain.result.stdout)[w.egress.status.egress.id]
	if !strings.Contains(block, w.egressEvidencePath()) {
		return fmt.Errorf("plain status omits the original protected refusal log path")
	}
	entry, _, err := w.egressEvidenceEntries()
	if err != nil {
		return err
	}
	egress := object(entry["egress"])
	if !sameJSON(egress["limitations"], offlineStrings(egressLimitations)) || !sameJSON(egress["allow"], offlineStrings(w.egress.status.egress.allow)) || egress["profile"] != "allowlist-v1" {
		return fmt.Errorf("JSON status lost the original egress grant or limitations")
	}
	return nil
}

func (w *world) egressEvidenceNoClaims() error {
	_, offline, err := w.egressEvidenceEntries()
	if err != nil {
		return err
	}
	if _, present := offline["egress"]; present {
		return fmt.Errorf("offline status contains egress")
	}
	plain, structured, err := w.egressStatusResults()
	if err != nil {
		return err
	}
	for _, result := range []egressResult{plain, structured} {
		text := result.result.stdout + result.result.stderr
		for _, forbidden := range []string{"unlisted.example", "not-listed", "total attempts", "complete history", "every refusal was stored"} {
			if strings.Contains(text, forbidden) {
				return fmt.Errorf("status dumps request metadata or claims completeness (%s)", forbidden)
			}
		}
	}
	return nil
}
