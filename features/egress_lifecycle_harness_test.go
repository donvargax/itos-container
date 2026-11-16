package features

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/cucumber/godog"
)

func TestEgressLifecycleNativeStepsDefined(t *testing.T) {
	definitions := &egressDefinitions{t: t}
	(&world{}).egressNativeSteps(definitions)
	(&world{}).syntheticMarkerSteps(definitions)
	suite := godog.TestSuite{Name: "slice-28-native-definitions", Options: &godog.Options{Paths: []string{"egress_lifecycle.feature"}, Tags: "@ID-EGRESSLIFE-06,@ID-EGRESSLIFE-07,@ID-EGRESSLIFE-11,@ID-EGRESSLIFE-08,@ID-EGRESSLIFE-09,@ID-EGRESSLIFE-10,@ID-EGRESSLIFE-12,@ID-EGRESSLIFE-13,@ID-EGRESSLIFE-14", Output: io.Discard}}
	features, err := suite.RetrieveFeatures()
	if err != nil {
		t.Fatal(err)
	}
	cases := 0
	for _, feature := range features {
		for _, scenario := range feature.Pickles {
			cases++
			for _, step := range scenario.Steps {
				matches := 0
				for _, expression := range definitions.patterns {
					if expression.MatchString(step.Text) {
						matches++
					}
				}
				if matches != 1 {
					t.Errorf("%q has %d step definitions", step.Text, matches)
				}
			}
		}
	}
	if cases != 19 {
		t.Fatalf("implemented native outlines have %d rows, want 19", cases)
	}
}

func TestEgressLifecycleRecorderForwardsRealResponses(t *testing.T) {
	path := filepath.Join(t.TempDir(), "egress-refusals.jsonl")
	if err := os.WriteFile(path, []byte("published refusal\n"), 0600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(out http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" || r.URL.Path != "/v1.48/containers/original" {
			t.Errorf("forwarded another request: %s %s", r.Method, r.URL.Path)
		}
		data, _ := io.ReadAll(r.Body)
		if string(data) != "original body" {
			t.Error("changed request body")
		}
		out.WriteHeader(http.StatusConflict)
		_, _ = out.Write([]byte("actual daemon refusal"))
	}))
	defer server.Close()
	a := &egressLifeRecorder{transport: http.DefaultTransport.(*http.Transport).Clone(), logPath: path}
	defer a.transport.CloseIdleConnections()
	request, err := http.NewRequestWithContext(t.Context(), "DELETE", server.URL+"/v1.48/containers/original?force=0", strings.NewReader("original body"))
	if err != nil {
		t.Fatal(err)
	}
	response, err := a.RoundTrip(request)
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil || response.StatusCode != http.StatusConflict || string(data) != "actual daemon refusal" {
		t.Fatalf("forwarder manufactured a response: %v", err)
	}
	journal := a.snapshot()
	if len(journal) != 1 || journal[0].status != http.StatusConflict || string(journal[0].response) != string(data) || journal[0].finished.Before(journal[0].at) || string(journal[0].logBefore) != "published refusal\n" || journal[0].logMode != info.Mode() {
		t.Fatal("recorder lost response/publication evidence")
	}
	journal[0].response[0] = 0
	if a.snapshot()[0].response[0] == 0 {
		t.Fatal("journal snapshot aliases live data")
	}
}

type egressUpgradeControl struct{ bytes.Buffer }

func (*egressUpgradeControl) Close() error { return nil }

func TestEgressLifecycleRecorderKeepsUpgradeWrites(t *testing.T) {
	stream := &egressUpgradeControl{}
	body := &egressLifeBody{ReadCloser: stream, recorder: &egressLifeRecorder{}, request: &egressLifeRequest{}}
	if n, err := body.Write([]byte("stdin")); err != nil || n != 5 || stream.String() != "stdin" {
		t.Fatal("upgraded stream lost worker input", n, err)
	}
	ordinary := &egressLifeBody{ReadCloser: io.NopCloser(strings.NewReader("ordinary"))}
	if _, err := ordinary.Write(nil); err == nil {
		t.Fatal("non-upgraded body became writable")
	}
	for _, response := range []string{"{}", "null", `{"StatusCode":17}`} {
		if egressLifeWaitExit([]egressLifeRequest{{method: "POST", path: "/containers/id/wait", status: 200, response: []byte(response), finished: time.Now()}}, "id", 0) {
			t.Fatal("unobserved zero exit invented")
		}
	}
}

func TestEgressLifecycleProbeRecordsSignalsWithoutAnEngine(t *testing.T) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Skip("the native probe targets Linux amd64")
	}
	root, err := moduleRoot()
	if err != nil {
		t.Fatal(err)
	}
	program := filepath.Join(t.TempDir(), "probe")
	if err := egressBuild(root, "features/testdata/slice-27/probe", program); err != nil {
		t.Fatal(err)
	}
	for _, signal := range []os.Signal{os.Interrupt, syscall.SIGTERM} {
		dir := t.TempDir()
		plan := filepath.Join(dir, "plan.json")
		if err := os.WriteFile(plan, approvalJSON(map[string]any{"Steps": []egressStep{{Op: "edit", Nonce: "retained"}, {Op: "wait-signal", Name: "ending"}}, "Exit": 17}), 0600); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
		cmd := exec.CommandContext(ctx, program, plan)
		if err := cmd.Start(); err != nil {
			cancel()
			t.Fatal(err)
		}
		for {
			if _, err := os.Stat(filepath.Join(dir, "slice-27-barrier-ending")); err == nil {
				break
			}
			if ctx.Err() != nil {
				_ = cmd.Wait()
				cancel()
				t.Fatal("probe missed its signal barrier")
			}
			time.Sleep(5 * time.Millisecond)
		}
		if err := cmd.Process.Signal(signal); err != nil {
			cancel()
			_ = cmd.Wait()
			t.Fatal(err)
		}
		err := cmd.Wait()
		cancel()
		var exit *exec.ExitError
		if !errors.As(err, &exit) || exit.ExitCode() != 17 {
			t.Fatalf("probe lost planned nonzero signal exit: %v", err)
		}
		receipt, err := os.ReadFile(filepath.Join(dir, "slice-28-signal"))
		want := "SIGTERM"
		if signal == os.Interrupt {
			want = "SIGINT"
		}
		if err != nil || string(receipt) != want {
			t.Fatalf("signal receipt = %q, %v", receipt, err)
		}
		edit, err := os.ReadFile(filepath.Join(dir, "slice-28-edit"))
		if err != nil || string(edit) != "retained" {
			t.Fatal("probe did not make its independent checkout edit")
		}
		data, err := os.ReadFile(filepath.Join(dir, "slice-27-report.json"))
		var report egressProbeReport
		if err != nil || json.Unmarshal(data, &report) != nil || !report.Done || len(report.Results) != 2 || !report.Results[1].OK || report.Results[1].Reply != want {
			t.Fatal("probe signal completion was not reported")
		}
	}
}
