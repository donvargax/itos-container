package features

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cucumber/godog"
)

type recoveryJSONExpectation struct {
	label string
	json  bool
	exit  int
}

func (w *world) recoveryJSONSteps(sc *godog.ScenarioContext) {
	sc.Step(`^recovery "(status|stop)" successes and usage refusals run with inherited and explicit JSON preferences$`, w.recoveryJSONInvocations)
	sc.Step(`^recovery inherits JSON only for the literal value 1 and explicit switches select the same format on every path$`, w.recoveryJSONFormats)
}

func (w *world) recoveryJSONInvocations(command string) error {
	for _, preference := range []struct {
		env, flag string
		json      bool
	}{
		{"true", "", false}, {"TRUE", "", false}, {"True", "", false},
		{"t", "", false}, {"T", "", false}, {"not-a-bool", "", false},
		{"2", "", false}, {"01", "", false}, {" 1 ", "", false},
		{"", "", false}, {"0", "", false}, {"false", "", false},
		{"1", "", true},
		{"1", "--no-json", false}, {"0", "--json", true},
		{"true", "--json", true}, {"true", "--no-json", false},
		{"not-a-bool", "--json", true}, {"not-a-bool", "--no-json", false},
	} {
		w.doctor.env["ITOS_JSON"] = preference.env
		for _, path := range []string{"success", "parser refusal", "argument refusal"} {
			args := []string{command}
			if preference.flag != "" {
				args = append(args, preference.flag)
			}
			if command == "stop" {
				args = append(args, w.recoveryLocal.id)
			}
			wantExit := 0
			switch path {
			case "parser refusal":
				args = append(args, "--slice-11-invalid-option")
				wantExit = 2
			case "argument refusal":
				args = append(args, "extra-argument")
				wantExit = 2
			}
			if err := w.recoveryInvocation(args...); err != nil {
				return err
			}
			w.recoveryLocal.jsonExpectations = append(w.recoveryLocal.jsonExpectations, recoveryJSONExpectation{
				label: fmt.Sprintf("%s %s with ITOS_JSON=%q and flag %q", command, path, preference.env, preference.flag),
				json:  preference.json, exit: wantExit,
			})
		}
	}
	return nil
}

func (w *world) recoveryJSONFormats() error {
	for i, want := range w.recoveryLocal.jsonExpectations {
		r := w.recoveryLocal.results[i]
		if r.exit != want.exit || (want.exit == 0 && r.stderr != "") || (want.exit == 2 && !strings.Contains(r.stderr, "recovery.usage")) {
			return fmt.Errorf("%s: exit = %d, want %d; stderr = %q", want.label, r.exit, want.exit, r.stderr)
		}
		if json.Valid([]byte(r.stdout)) != want.json {
			return fmt.Errorf("%s: JSON output = %t, want %t", want.label, json.Valid([]byte(r.stdout)), want.json)
		}
		if want.json {
			if want.exit == 0 {
				if _, err := w.recoveryLocalEntry(r, "stopped", "absent", "complete", "7"); err != nil {
					return fmt.Errorf("%s: %w", want.label, err)
				}
			} else {
				report, err := recoveryLocalReport(r, 2)
				if err != nil || len(report.Problems) != 1 || report.Problems[0].Rule != "recovery.usage" || len(report.Sessions) != 0 {
					return fmt.Errorf("%s: missing schema-1 recovery usage refusal: %q (%v)", want.label, r.stdout, err)
				}
			}
		} else if want.exit == 0 {
			for _, fact := range []string{w.recoveryLocal.id, w.approval.project, w.approval.fingerprint, "state stopped", "resource absent", "cleanup complete", "workload_exit 7"} {
				if !strings.Contains(r.stdout, fact) {
					return fmt.Errorf("%s: plain output omitted %q", want.label, fact)
				}
			}
		} else if r.stdout != "" {
			return fmt.Errorf("%s: plain usage wrote stdout: %q", want.label, r.stdout)
		}
	}
	return nil
}
