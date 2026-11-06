package features

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/cucumber/godog"
)

// Check every expanded row without invoking any fixture or engine.
func TestSessionStepDefinitions(t *testing.T) {
	var output strings.Builder
	suite := godog.TestSuite{Options: &godog.Options{Paths: []string{"session.feature"}, Format: "progress", Output: &output, ShowStepDefinitions: true}, ScenarioInitializer: func(sc *godog.ScenarioContext) { initializeScenario(sc, "unused", "unused") }}
	if code := suite.Run(); code != 2 {
		t.Fatalf("definition listing exited %d", code)
	}
	ansi := regexp.MustCompile("\x1b\\[[0-9;]*m")
	var definitions []*regexp.Regexp
	for _, line := range strings.Split(ansi.ReplaceAllString(output.String(), ""), "\n") {
		expression, _, found := strings.Cut(line, " # ")
		if found {
			definitions = append(definitions, regexp.MustCompile(strings.TrimSpace(expression)))
		}
	}
	features, err := suite.RetrieveFeatures()
	if err != nil {
		t.Fatal(err)
	}
	rows := 0
	for _, feature := range features {
		for _, scenario := range feature.Pickles {
			rows++
			for _, step := range scenario.Steps {
				matches := 0
				for _, definition := range definitions {
					if definition.MatchString(step.Text) {
						matches++
					}
				}
				if matches != 1 {
					t.Errorf("step %q has %d definitions", step.Text, matches)
				}
			}
		}
	}
	if rows < 70 {
		t.Fatalf("checked only %d session rows", rows)
	}
}

func newSessionHarnessWorld(t *testing.T) *world {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("Linux Unix-socket fixture")
	}
	root, err := moduleRoot()
	if err != nil {
		t.Fatal(err)
	}
	w := &world{root: root, dir: t.TempDir()}
	t.Cleanup(func() {
		if err := w.closeSession(); err != nil {
			t.Error(err)
		}
		w.closeDoctor()
	})
	if err := w.sessionFixture(false, "0022"); err != nil {
		t.Fatal(err)
	}
	return w
}

// A local HTTP-only control exercises the same subprocess/barrier/observer path.
// It is harness evidence, not red evidence for either CI-only scenario.
func TestSessionLocalControl(t *testing.T) {
	w := newSessionHarnessWorld(t)
	if err := w.sessionRun(false); err != nil {
		t.Fatal(err)
	}
	if w.session.fixtureError != nil {
		t.Fatal(w.session.fixtureError)
	}

	if err := w.sessionOwned(); err != nil {
		t.Fatal(err)
	}
	if err := w.sessionIntentBeforeCreate(); err != nil {
		t.Fatal(err)
	}
	if err := w.sessionDurableAbsent(); err != nil {
		t.Fatal(err)
	}
	if err := w.sessionUnrelated(); err != nil {
		t.Fatal(err)
	}
	if err := w.sessionNoForbidden(); err != nil {
		t.Fatal(err)
	}
}

func TestSessionRecorderCountsForbiddenRequests(t *testing.T) {
	w := newSessionHarnessWorld(t)
	path := strings.TrimPrefix(w.doctor.env["DOCKER_HOST"], "unix://")
	transport := doctorTransport(path)
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: time.Second}
	for _, path := range []string{"/containers/arbitrary/start", "/images/create", "/volumes/create", "/containers/arbitrary?force=true&v=true"} {
		request, err := http.NewRequestWithContext(context.Background(), "POST", "http://engine"+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		_ = response.Body.Close()
		if response.StatusCode != 403 {
			t.Fatalf("forbidden fixture request returned %d", response.StatusCode)
		}
	}
	w.session.api.mu.Lock()
	defer w.session.api.mu.Unlock()
	if len(w.session.api.requests) != 4 || w.sessionNoForbidden() == nil {
		t.Fatal("refused requests disappeared from the session audit")
	}
}

func TestSessionLostReplyHappensAfterObservedCreation(t *testing.T) {
	w := newSessionHarnessWorld(t)
	w.session.fault = "lost create response"
	body := map[string]any{"Image": w.doctor.imageID, "User": w.approval.uid + ":" + w.approval.gid,
		"Cmd": []any{"/bin/true"}, "Entrypoint": []any{}, "Labels": w.sessionLabels(),
		"HostConfig": map[string]any{"NetworkMode": "none", "ReadonlyRootfs": true, "PidsLimit": float64(4096),
			"CapDrop": []any{"ALL"}, "SecurityOpt": []any{"no-new-privileges"}, "RestartPolicy": map[string]any{"Name": "no"}}}
	if err := w.sessionRequestProblem(body); err != nil {
		t.Fatal(err)
	}
	transport := doctorTransport(strings.TrimPrefix(w.doctor.env["DOCKER_HOST"], "unix://"))
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: time.Second}
	request, err := http.NewRequestWithContext(context.Background(), "POST", "http://engine/containers/create?name=itos-container-"+w.session.token, bytes.NewReader(approvalJSON(body)))
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Do(request)
	if response != nil {
		_ = response.Body.Close()
	}
	if err == nil {
		t.Fatal("lost-create fixture delivered the success reply")
	}
	a := w.session.api
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.problem != nil || len(a.created) != 1 || len(a.requests) != 1 || a.requests[0].Status != 201 {
		t.Fatal("reply was lost without successful server-side creation")
	}
	if err := w.sessionIdentityProblem(a.observed[a.created[0]], a.created[0]); err != nil {
		t.Fatal(err)
	}
	// Deliberately omit ownership/durability: an observed container alone must
	// not satisfy the scenario's intent-before-create assertion.
	if w.sessionIntentBeforeCreate() == nil {
		t.Fatal("created resource without a durable intent satisfied the contract")
	}
}

func TestSessionObserverRejectsWrongIdentityAndState(t *testing.T) {
	w := newSessionHarnessWorld(t)
	id := strings.Repeat("c", 64)
	correct := map[string]any{"Id": id, "Name": "/itos-container-" + w.session.token, "Image": w.doctor.imageID,
		"Config": map[string]any{"Labels": w.sessionLabels()}, "State": map[string]any{"Status": "created", "Running": false, "Pid": float64(0), "StartedAt": "0001-01-01T00:00:00Z"}}
	if err := w.sessionIdentityProblem(correct, id); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(map[string]any){
		func(c map[string]any) { sessionMutateInspection(c, "mounted volume") },
		func(c map[string]any) { sessionMutateInspection(c, "configured volume") },
		func(c map[string]any) { delete(c, "State") },
		func(c map[string]any) { object(c["State"])["Status"] = "exited" },
		func(c map[string]any) { object(c["State"])["Running"] = true },
		func(c map[string]any) { object(c["State"])["Pid"] = float64(1) },
		func(c map[string]any) { object(c["State"])["StartedAt"] = "2026-01-01T00:00:00Z" },
		func(c map[string]any) { c["Id"] = strings.Repeat("d", 64) },
		func(c map[string]any) { c["Image"] = "foreign" },
		func(c map[string]any) { c["Name"] = "/foreign" },
		func(c map[string]any) { object(object(c["Config"])["Labels"])["io.itos-container.intent"] = "foreign" },
	} {
		candidate := approvalClone(correct)
		mutate(candidate)
		if w.sessionIdentityProblem(candidate, id) == nil {
			t.Fatal("wrong identity or previously-started state escaped the independent observer")
		}
	}
}

func TestSessionChildDropsEngineAndRedInputs(t *testing.T) {
	got := sessionChildEnvironment([]string{"PATH=/tools", "GOCOVERDIR=/coverage", "ITOS_CONTAINER_ENGINE_TESTS=1", "ITOS_CONTAINER_ENGINE_FIXTURE_IMAGE=private", "ITOS_CONTAINER_RED_SCENARIOS=@ID-SESSION-11", "DOCKER_HOST=ambient"})
	if strings.Join(got, "\n") != "PATH=/tools\nGOCOVERDIR=/coverage" {
		t.Fatal("test assembly inherited ambient engine or dispatch input")
	}
}

func TestSessionNoEngineGuard(t *testing.T) {
	t.Setenv("ITOS_CONTAINER_ENGINE_TESTS", "0")
	w := &world{}
	if err := w.sessionFixture(true, "0022"); err == nil || w.doctor != nil {
		t.Fatal("session real fixture initialized outside the engine tier")
	}
}

func TestSessionDurabilityCannotBeInferredFromVisibility(t *testing.T) {
	w := newSessionHarnessWorld(t)
	data, err := json.Marshal(w.sessionExpectedRecord("intent", ""))
	if err != nil {
		t.Fatal(err)
	}
	events := []sessionEvent{}
	for _, operation := range []string{"write", "file fsync", "atomic publication", "parent fsync"} {
		events = append(events, sessionEvent{Operation: operation, Stage: "intent", Bytes: string(data)})
	}
	if err := w.sessionDurability(events, "intent", ""); err != nil {
		t.Fatal(err)
	}
	for i := range events {
		broken := append([]sessionEvent{}, events...)
		broken[i].Failed = true
		if w.sessionDurability(broken, "intent", "") == nil {
			t.Fatal("failed durability operation accepted as durable publication")
		}
	}
	if err := os.WriteFile(w.session.journal, []byte("{partial"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := sessionReadEvents(w.session.journal); err == nil {
		t.Fatal("partial observation journal accepted")
	}
}
