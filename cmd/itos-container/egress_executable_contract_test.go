package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	readiness "github.com/donvargax/itos-container/internal/engine/readiness/domain"
	"github.com/donvargax/itos-container/internal/engine/readiness/executable"
	rp "github.com/donvargax/itos-container/internal/engine/readiness/port"
	rf "github.com/donvargax/itos-container/internal/engine/readiness/port/porttest"
)

type executableContractInput struct{ Root, Control, Defect, Kind, Output string }

// One judgement of one input by one backend, as the shared diagnostic reads it.
type executableContractResult struct {
	Admitted     bool
	Rule         string
	Exit         int
	Message, Fix string
	Calls        int
	Contract     []string
}

type executableContractReporter struct{ failures []string }

func (*executableContractReporter) Helper() {}
func (r *executableContractReporter) Errorf(format string, args ...any) {
	r.failures = append(r.failures, fmt.Sprintf(format, args...))
}
func (r *executableContractReporter) Fatalf(format string, args ...any) { r.Errorf(format, args...) }

func executableContractJudge(name string, backend rp.Executables, path, root string) executableContractResult {
	reporter := &executableContractReporter{}
	rf.ExecutableCase(reporter, name, backend, path)
	observed, failure := backend.Observe()
	result := executableContractResult{Contract: reporter.failures}
	if fake, ok := backend.(*rf.Executables); ok {
		result.Calls = fake.Calls
	}
	refused := readiness.ProxyExecutable(observed, failure, root)
	if refused == rp.None {
		result.Admitted = true
		return result
	}
	p := rp.Diagnostic(rp.Issue{Kind: refused, Subject: "executable"})
	result.Rule, result.Exit, result.Message, result.Fix = p.Rule, p.Exit, p.Message, p.Fix
	return result
}

// Only a go test subprocess can enter this assembly, never a product flag. The
// scenario wrote the ELF inputs; the real adapter reads them, the fake is given
// what the shared contract says they hold, and one readiness rule judges both.
func TestEgressExecutableContractProbe(t *testing.T) {
	path := os.Getenv("SLICE_27_PROBE_INPUT")
	if path == "" {
		t.Skip("feature contract subprocess only")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var input executableContractInput
	if err = json.Unmarshal(data, &input); err != nil {
		t.Fatal(err)
	}
	out := map[string]map[string]executableContractResult{"fake": {}, "real": {}}
	for name, file := range map[string]string{rf.ExecutableControl: input.Control, input.Kind: input.Defect} {
		resolved, err := filepath.EvalSymlinks(file)
		if err != nil {
			resolved = file
		}
		key := "defect"
		if name == rf.ExecutableControl {
			key = "control"
		}
		scripted, failure := rf.ExpectedExecutable(name, resolved)
		out["fake"][key] = executableContractJudge(name, &rf.Executables{Observed: scripted, Failure: failure}, resolved, input.Root)
		out["real"][key] = executableContractJudge(name, executable.Program{Path: func() (string, error) { return file, nil }}, resolved, input.Root)
	}
	data, err = json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(input.Output, data, 0600); err != nil {
		t.Fatal(err)
	}
}
