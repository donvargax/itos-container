package features

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

// The factory substitutes only fixture preparation. The launch, recorder and
// teardown are shared with TestFeatures; ordinary tests never enable Docker.
type engineWarmupSettings struct {
	fixture func(*world) error
	limit   time.Duration
}

type engineSuiteWarmup struct{}

func (s *engineSuiteWarmup) before(selection scenarioSelection, root string, output io.Writer, settings engineWarmupSettings) error {
	return nil // T-57 red seam: the suite has not launched its throwaway product.
}

func TestEngineSuiteWarmup(t *testing.T) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Skip("Linux amd64 product and Unix-socket fixture")
	}
	t.Setenv("GOCOVERDIR", "")
	t.Setenv("ITOS_CC_TEST_COVERDIR", "")
	root, err := moduleRoot()
	if err != nil {
		t.Fatal(err)
	}
	t.Run("built product through recorder", func(t *testing.T) {
		var suite engineSuiteWarmup
		var output bytes.Buffer
		var w *world
		var engine *sessionAPI
		settings := engineWarmupSettings{limit: 45 * time.Second, fixture: func(candidate *world) error {
			w = candidate
			var err error
			engine, err = warmupLocalFixture(w)
			return err
		}}
		selection := scenarioSelection{engine: true, count: 1}
		if err := suite.before(selection, root, &output, settings); err != nil {
			t.Fatal(err)
		}
		if w == nil || engine == nil {
			t.Fatal("missing throwaway product launch before engine rows")
		}
		engine.mu.Lock()
		defer engine.mu.Unlock()
		if len(engine.created) != 1 || len(engine.local) != 0 || len(engine.volumes) != 0 {
			t.Fatalf("launch did not create and remove its worker and anchor: created=%d workers=%d anchors=%d", len(engine.created), len(engine.local), len(engine.volumes))
		}
		var create map[string]any
		for _, request := range engine.requests {
			if request.Method == "POST" && request.Path == "/containers/create" {
				create = request.Body
			}
		}
		if !reflect.DeepEqual(create["Cmd"], []any{"/bin/true"}) || create["Image"] != w.doctor.imageID || create["User"] != w.approval.uid+":"+w.approval.gid {
			t.Fatal("warm-up did not launch the literal argv with the fixture identity", create)
		}
		if strings.Count(output.String(), "engine suite warm-up timing:") != 1 || !strings.Contains(output.String(), "outcome=passed\n") {
			t.Fatal("missing warm-up create interval", output.String())
		}
		if _, err := os.Stat(w.dir); !os.IsNotExist(err) {
			t.Fatal("warm-up retained its disposable world", err)
		}
	})
}

// Reuse the session fixture's stateful protocol engine behind the *actual*
// offline recorder. Its separate doctor value prevents recursive forwarding.
func warmupLocalFixture(w *world) (*sessionAPI, error) {
	if err := w.sessionFixture(false, "0022"); err != nil {
		return nil, err
	}
	d := *w.doctor
	s := *w.session
	s.offline = true
	upstream := &world{doctor: &d, session: &s, approval: w.approval}
	a := &sessionAPI{w: upstream, observed: map[string]map[string]any{}, local: map[string]map[string]any{}, absent: map[string]bool{}}
	server := httptest.NewServer(http.HandlerFunc(func(out http.ResponseWriter, request *http.Request) {
		// The real product chooses its own random intent token.
		if doctorRequestPath(request.URL.Path) == "/containers/create" {
			a.mu.Lock()
			s.token = strings.TrimPrefix(request.URL.Query().Get("name"), "itos-container-")
			s.reference = "intents/" + w.approval.project + "/" + s.token + ".json"
			a.mu.Unlock()
		}
		a.serve(out, request)
	}))
	transport := &http.Transport{DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
	}}
	w.doctor.real = &doctorReal{transport: transport, client: &http.Client{Transport: transport, Timeout: time.Second}}
	w.doctor.cleanup = append(w.doctor.cleanup, func() {
		a.mu.Lock()
		if a.attached != nil {
			_ = a.attached.Close()
		}
		a.mu.Unlock()
		server.Close()
		transport.CloseIdleConnections()
	})
	return a, nil
}
