package features

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/cucumber/godog"
)

// Definition coverage is engine-free; it is not runtime or red evidence.
func TestOfflineRuntimeStepDefinitions(t *testing.T) {
	var output strings.Builder
	suite := godog.TestSuite{Options: &godog.Options{Paths: []string{"offline.feature"}, Format: "progress", Output: &output, ShowStepDefinitions: true}, ScenarioInitializer: func(sc *godog.ScenarioContext) { initializeScenario(sc, "unused", "unused") }}
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
	checked := 0
	for _, feature := range features {
		for _, scenario := range feature.Pickles {
			selected := false
			for _, tag := range scenario.Tags {
				selected = selected || tag.Name == "@ID-OFFLINE-06" || tag.Name == "@ID-OFFLINE-07" || tag.Name == "@ID-OFFLINE-13" || tag.Name == "@ID-OFFLINE-29" || tag.Name == "@ID-OFFLINE-30" || tag.Name == "@slice-14"
			}
			if !selected {
				continue
			}
			checked++
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
	if checked != 24 {
		t.Fatalf("checked %d offline runtime cases", checked)
	}
}

func TestOfflineRuntimeGuard(t *testing.T) {
	t.Setenv("ITOS_CONTAINER_ENGINE_TESTS", "0")
	w := &world{}
	if err := w.offlineFixture("argv"); err == nil || w.offline != nil || w.doctor != nil {
		t.Fatal("offline fixture initialized outside the guarded CI tier")
	}
}

func TestOfflineTrustedProbeCompilesWithoutAnEngine(t *testing.T) {
	root, err := moduleRoot()
	if err != nil {
		t.Fatal(err)
	}
	w := &world{root: root, dir: t.TempDir(), doctor: &doctorWorld{checkout: t.TempDir()}}
	if err := w.buildOfflineProbe(); err != nil {
		t.Fatal(err)
	}
	// Compilation is the only local operation: the trusted probe itself must
	// execute inside the worker, never as a stand-in for runtime evidence.
}

func TestOfflineArgumentOracleRejectsAbsentOrChangedWorkload(t *testing.T) {
	w := &world{offline: &offlineRuntime{arguments: []string{"", "two words", "\t\n", "雪", "*", "--", "--json"}}}
	correct := append([]byte("slice-10:argv\x00"), offlineFrames(w.offline.arguments)...)
	w.offline.stdout = correct
	if err := w.offlineArguments(); err != nil {
		t.Fatal(err)
	}
	for _, report := range [][]byte{nil, []byte("approval matched\n"), correct[:len(correct)-1], append([]byte("controller\n"), correct...), append(bytes.Clone(correct), '\n'), append([]byte("slice-10:argv\x00"), offlineFrames([]string{"two words", "", "\t\n", "雪", "*", "--", "--json"})...)} {
		w.offline.stdout = report
		if w.offlineArguments() == nil {
			t.Fatal("absent, changed, reordered or contaminated argv report passed")
		}
	}
}

func TestOfflineStreamOracleRequiresLiveHandshakeAndCompleteBytes(t *testing.T) {
	ack := append([]byte("slice-10:stdin-and-eof\x00"), offlineFrames([]string{string(offlineInput)})...)
	out := append(append(bytes.Clone(offlineReadyOut), ack...), offlineTailOut...)
	errout := append(bytes.Clone(offlineReadyErr), offlineTailErr...)
	w := &world{offline: &offlineRuntime{stdout: out, stderr: errout, handshake: true, inputClosed: true}}
	if err := w.offlineStreamBytes(false); err != nil {
		t.Fatal(err)
	}
	w.offline.handshake = false
	if w.offlineStreamBytes(false) == nil {
		t.Fatal("post-exit output satisfied the live handshake")
	}
	w.offline.handshake = true
	w.offline.inputClosed = false
	if w.offlineStreamBytes(false) == nil {
		t.Fatal("missing EOF satisfied the handshake")
	}
	w.offline.inputClosed = true
	for _, pair := range [][2][]byte{{nil, nil}, {out[:len(out)-1], errout}, {out, errout[:len(errout)-1]}, {append(bytes.Clone(out), errout...), nil}, {append([]byte("controller\n"), out...), errout}} {
		w.offline.stdout, w.offline.stderr = pair[0], pair[1]
		if w.offlineStreamBytes(false) == nil {
			t.Fatal("missing, mixed or truncated stream passed")
		}
	}
}

func TestOfflineEnvironmentOracleRequiresCompleteReport(t *testing.T) {
	prefix := []byte("/workspace/subdir\n" + offlineAPP + "\n/tmp\n/tmp\n")
	report := map[string]any{"Present": map[string]bool{"HOST_ONLY": false, "GITHUB_TOKEN": false, "AWS_SECRET_ACCESS_KEY": false, "ITOS_ROOT_MARKER": false, "host-path": false}, "Writable": map[string]bool{"HOME": true, "TMPDIR": true}}
	w := &world{offline: &offlineRuntime{stdout: append(bytes.Clone(prefix), approvalJSON(report)...)}}
	if err := w.offlineEnvironment(); err != nil {
		t.Fatal(err)
	}
	for _, broken := range []map[string]any{{}, {"Present": map[string]bool{}, "Writable": report["Writable"]}, {"Present": report["Present"], "Writable": map[string]bool{"HOME": true}}} {
		w.offline.stdout = append(bytes.Clone(prefix), approvalJSON(broken)...)
		if w.offlineEnvironment() == nil {
			t.Fatal("missing environment observations passed")
		}
	}
	report["Present"].(map[string]bool)["host-path"] = true
	w.offline.stdout = append(bytes.Clone(prefix), approvalJSON(report)...)
	if w.offlineEnvironment() == nil {
		t.Fatal("host PATH forwarding passed")
	}
}

func offlineRecorderWorld(t *testing.T, native http.Handler) *world {
	t.Helper()
	w := newSessionHarnessWorld(t)
	w.offline = &offlineRuntime{}
	server := httptest.NewServer(native)
	t.Cleanup(server.Close)
	transport := &http.Transport{DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
	}}
	t.Cleanup(transport.CloseIdleConnections)
	w.doctor.real = &doctorReal{transport: transport, client: &http.Client{Transport: transport, Timeout: time.Second}}
	if err := w.startOfflineAPI(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = w.offline.api.server.Close()
		w.offline.api.closeConnections()
		// The old session recorder never created a native resource.
		w.doctor.real = nil
	})
	return w
}

func TestOfflineRecorderIncludesRefusedRequests(t *testing.T) {
	w := offlineRecorderWorld(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Error("forbidden request reached native transport") }))
	transport := doctorTransport(strings.TrimPrefix(w.doctor.env["DOCKER_HOST"], "unix://"))
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: time.Second}
	for _, path := range []string{"/images/create", "/containers/foreign/start", "/networks/create", "/volumes/prune"} {
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
			t.Fatalf("unsafe request returned %d", response.StatusCode)
		}
	}
	w.offline.api.mu.Lock()
	defer w.offline.api.mu.Unlock()
	if len(w.offline.api.requests) != 4 {
		t.Fatal("refused requests disappeared from journal")
	}
}

func TestOfflineRecorderRefusesUnsafeCreateBeforeNativeMutation(t *testing.T) {
	w := newSessionHarnessWorld(t)
	a := &offlineAPI{w: w}
	body := map[string]any{
		"Image": w.doctor.imageID, "User": w.approval.uid + ":" + w.approval.gid,
		"Labels": w.sessionLabels(), "Tty": false,
		"HostConfig": map[string]any{
			"NetworkMode": "none", "ReadonlyRootfs": true,
			"CapDrop": []any{"ALL"}, "SecurityOpt": []any{"no-new-privileges"},
			"Mounts": []any{
				map[string]any{"Type": "bind", "Source": w.doctor.checkout, "Target": "/workspace", "BindOptions": map[string]any{"NonRecursive": true, "Propagation": "rprivate"}},
				map[string]any{"Type": "tmpfs", "Target": "/tmp"},
			},
		},
	}
	if !a.safeCreate(body) {
		t.Fatal("recorder refused its bounded positive create control")
	}
	for _, mutate := range []func(map[string]any){
		func(b map[string]any) { b["User"] = "0:0" },
		func(b map[string]any) { b["Image"] = "foreign" },
		func(b map[string]any) { b["Labels"] = map[string]any{} },
		func(b map[string]any) { object(b["HostConfig"])["Privileged"] = true },
		func(b map[string]any) { object(b["HostConfig"])["NetworkMode"] = "host" },
		func(b map[string]any) { object(b["HostConfig"])["ReadonlyRootfs"] = false },
		func(b map[string]any) { object(b["HostConfig"])["PidMode"] = "host" },
		func(b map[string]any) { object(b["HostConfig"])["CapDrop"] = []any{} },
		func(b map[string]any) { object(b["HostConfig"])["SecurityOpt"] = []any{"seccomp=unconfined"} },
		func(b map[string]any) { object(b["HostConfig"])["Binds"] = []any{"/:/host"} },
		func(b map[string]any) { object(b["HostConfig"])["Tmpfs"] = map[string]any{"/additional": "rw"} },
		func(b map[string]any) { object(object(b["HostConfig"])["Mounts"].([]any)[0])["Source"] = "/outside" },
		func(b map[string]any) {
			object(object(object(b["HostConfig"])["Mounts"].([]any)[0])["BindOptions"])["NonRecursive"] = false
		},
	} {
		candidate := approvalClone(body)
		mutate(candidate)
		if a.safeCreate(candidate) {
			t.Fatal("recorder allowed an unsafe native create")
		}
	}
}

func TestOfflineRecorderRelaysUpgradeAndStdinHalfClose(t *testing.T) {
	id := strings.Repeat("a", 64)
	var w *world
	input := make(chan []byte, 1)
	native := http.HandlerFunc(func(out http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/json") {
			_ = json.NewEncoder(out).Encode(map[string]any{"Id": id, "Name": "/itos-container-" + w.session.token, "Image": w.doctor.imageID, "Config": map[string]any{"Labels": w.sessionLabels()}, "State": map[string]any{"Running": true}})
			return
		}
		if !strings.HasSuffix(r.URL.Path, "/attach") {
			http.Error(out, "unexpected fixture request", 400)
			return
		}
		conn, rw, err := out.(http.Hijacker).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		defer func() { _ = conn.Close() }()
		_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
		_, _ = rw.WriteString("HTTP/1.1 101 UPGRADED\r\nConnection: Upgrade\r\nUpgrade: tcp\r\n\r\n")
		_, _ = rw.Write(offlineReadyOut)
		_ = rw.Flush()
		data, err := io.ReadAll(rw)
		if err != nil {
			t.Error("native fixture did not receive stdin EOF", err)
			return
		}
		input <- data
		_, _ = conn.Write(offlineTailOut)
	})
	w = offlineRecorderWorld(t, native)
	w.offline.api.created = []string{id}
	path := strings.TrimPrefix(w.doctor.env["DOCKER_HOST"], "unix://")
	conn, err := net.DialUnix("unix", nil, &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(4 * time.Second))
	_, err = fmt.Fprintf(conn, "POST /containers/%s/attach?stream=1&stdin=1&stdout=1&stderr=1 HTTP/1.1\r\nHost: engine\r\nConnection: Upgrade\r\nUpgrade: tcp\r\nContent-Length: 0\r\n\r\n", id)
	if err != nil {
		t.Fatal(err)
	}
	reader := bufio.NewReader(conn)
	response, err := http.ReadResponse(reader, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != 101 {
		t.Fatalf("attach returned %d", response.StatusCode)
	}
	ready := make([]byte, len(offlineReadyOut))
	if _, err := io.ReadFull(reader, ready); err != nil || !bytes.Equal(ready, offlineReadyOut) {
		t.Fatal("upgrade lost live output", err)
	}
	if _, err := conn.Write(offlineInput); err != nil {
		t.Fatal(err)
	}
	if err := conn.CloseWrite(); err != nil {
		t.Fatal(err)
	}
	tail, err := io.ReadAll(reader)
	if err != nil || !bytes.Equal(tail, offlineTailOut) {
		t.Fatal("stdin EOF closed output before the trailing bytes", err)
	}
	select {
	case got := <-input:
		if !bytes.Equal(got, offlineInput) {
			t.Fatal("upgraded stdin bytes changed")
		}
	default:
		t.Fatal("native fixture never observed input and EOF")
	}
}
