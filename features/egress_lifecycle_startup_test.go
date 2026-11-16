package features

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/moby/moby/client"
)

// A barrier is claimed only after a successful real daemon reply. Neither a
// rejected create nor a failed transport can be evidence of an accepted create.
type egressLifeBarrier struct {
	mu             sync.Mutex
	kind           string
	ready, release chan struct{}
	once           sync.Once
	claimed        bool
	request        egressLifeRequest
	err            error
	act            func(context.Context, string) error
}

func newEgressLifeBarrier(kind string) *egressLifeBarrier {
	return &egressLifeBarrier{kind: kind, ready: make(chan struct{}), release: make(chan struct{})}
}

func (b *egressLifeBarrier) open() { b.once.Do(func() { close(b.release) }) }

func egressLifeCreateKind(e *egressLifeRequest) string {
	if e.method != "POST" || (e.path != "/containers/create" && e.path != "/networks/create") {
		return ""
	}
	var body map[string]any
	if json.Unmarshal(e.body, &body) != nil {
		return ""
	}
	kind, _ := object(body["Labels"])["io.itos-container.kind"].(string)
	if kind == "egress-proxy" {
		return "proxy"
	}
	return map[string]string{"internal-network": "internal", "egress-network": "egress"}[kind]
}

func (b *egressLifeBarrier) matches(e *egressLifeRequest) bool {
	switch b.kind {
	case "ready-held":
		query, _ := url.ParseQuery(e.query)
		return e.method == "GET" && strings.HasSuffix(e.path, "/logs") && (query.Get("follow") == "1" || query.Get("follow") == "true")
	case "ready-killed":
		return e.method == "POST" && strings.HasSuffix(e.path, "/start")
	default:
		return b.kind == "create-"+egressLifeCreateKind(e)
	}
}

func (b *egressLifeBarrier) after(r *http.Request, e *egressLifeRequest, response *http.Response) error {
	if response.StatusCode < 200 || response.StatusCode >= 300 || !b.matches(e) {
		return nil
	}
	b.mu.Lock()
	if b.claimed {
		b.mu.Unlock()
		return nil
	}
	b.claimed = true
	b.mu.Unlock()
	// A create reply must be read to observe the full native ID before holding
	// it back. The readiness stream stays unread, bounded by its caller's call.
	var data []byte
	var err error
	if strings.HasPrefix(b.kind, "create-") {
		data, err = io.ReadAll(io.LimitReader(response.Body, 1<<20+1))
		err = errors.Join(err, response.Body.Close())
		if len(data) > 1<<20 {
			err = errors.Join(err, errors.New("create response exceeds observation bound"))
		}
		response.Body = io.NopCloser(bytes.NewReader(data))
	}
	if b.act != nil {
		id := strings.TrimSuffix(strings.TrimPrefix(e.path, "/containers/"), "/start")
		err = errors.Join(err, b.act(r.Context(), id))
	}
	b.mu.Lock()
	b.request, b.err = *e, err
	b.request.body, b.request.response = bytes.Clone(e.body), bytes.Clone(data)
	b.mu.Unlock()
	close(b.ready)
	if err != nil {
		return err
	}
	if b.kind == "ready-killed" {
		return nil
	}
	select {
	case <-b.release:
		return nil
	case <-r.Context().Done():
		return r.Context().Err()
	}
}

type egressLifeStartup struct {
	resource            string
	barrier             *egressLifeBarrier
	recorder            *egressLifeRecorder
	run, control        *egressNativeRun
	controlID, id, path string
	controlStatus       egressResult
	journal             []egressLifeRequest
	started, ended      time.Time
	reached             bool
	original            *egressObserved
}

func (w *world) egressLifeStartupSteps(sc lifecycleStepRegistrar) {
	sc.Step(`^a CI egress startup fixture with readiness fault "([^"]*)" and a separate healthy control$`, w.egressLifeStartupFixture)
	sc.Step(`^run executes the healthy control and then the faulted invocation$`, w.egressLifeStartupRun)
	sc.Step(`^the control completes a verified TLS exchange and fresh status reports its stored log$`, w.egressLifeStartupControl)
	sc.Step(`^the faulted run reaches the named proxy barrier and reports "([^"]*)" with exit (\d+) within the existing budgets$`, w.egressLifeStartupResult)
	sc.Step(`^no worker is created or started and no fallback route proxy restart or duplicate resource is requested$`, w.egressLifeStartupNoWorker)
	sc.Step(`^only created owned resources are removed in reverse order with a retained log report visible to fresh status$`, w.egressLifeStartupCleanup)
	sc.Step(`^a CI egress startup fixture that withholds the accepted "([^"]*)" create reply$`, w.egressLifeCreateFixture)
	sc.Step(`^run reaches that native create barrier the controller is killed and a fresh stop reconciles the original session$`, w.egressLifeCreateRun)
	sc.Step(`^the durable intent precedes the real create and recovery saves the matching full ID before any removal$`, w.egressLifeCreateAuthority)
	sc.Step(`^no worker instruction proxy restart or replacement resource is started by recovery$`, w.egressLifeCreateNoStart)
	sc.Step(`^fresh status reports cleanup complete with unknown workload exit and refusal evidence "([^"]*)"$`, w.egressLifeCreateStatus)
	sc.Step(`^all created original resources are confirmed absent in reverse order without changing unrelated resources$`, w.egressLifeCreateCleanup)
}

func (w *world) egressLifeStartupBase() error {
	if err := w.egressNativeCheckout([]string{"api.example.com"}, []string{"api.example.com"}); err != nil {
		return err
	}
	recorder, err := w.egressLifeRecord()
	if err != nil {
		return err
	}
	l := &egressLifeStartup{recorder: recorder}
	w.egress.lifeStartup = l
	recorder.before = func(e *egressLifeRequest) {
		if e.method != "POST" && e.method != "DELETE" {
			return
		}
		// Only the original project's private intents are read. The fixture
		// never writes a product intent or supplies a successful engine reply.
		intent, err := w.egressLifeCurrentIntent()
		recorder.mu.Lock()
		e.intent, e.intentErr = intent, err
		recorder.mu.Unlock()
	}
	return w.egressEndpoints(func(*egressNative) [][]string { return [][]string{{"startup", "public", "api.example.com"}} })
}

func (w *world) egressLifeCurrentIntent() (map[string]any, error) {
	root := w.egressStatePath("intents", w.approval.project)
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var found map[string]any
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(root, entry.Name()))
		if err != nil {
			return nil, err
		}
		var record map[string]any
		if err := json.Unmarshal(data, &record); err != nil {
			return nil, err
		}
		if object(record["anchor"])["stage"] == "absent" {
			continue
		}
		if found != nil {
			return nil, errors.New("multiple live fixture intents")
		}
		found = record
	}
	if found == nil {
		return nil, errors.New("no live fixture intent")
	}
	return found, nil
}

func (w *world) egressLifeStartupFixture(fault string) error {
	kind, ok := map[string]string{"proxy killed after start before ready delivery": "ready-killed", "ready response held until its call expires": "ready-held"}[fault]
	if !ok {
		return errors.New("unknown readiness fault")
	}
	if err := w.egressLifeStartupBase(); err != nil {
		return err
	}
	l := w.egress.lifeStartup
	l.barrier = newEgressLifeBarrier(kind)
	if kind == "ready-killed" {
		l.barrier.act = func(ctx context.Context, id string) error {
			c, err := w.egress.native.cli.ContainerInspect(ctx, id, client.ContainerInspectOptions{})
			if err != nil {
				return err
			}
			if c.Container.Config == nil || c.Container.Config.Labels["io.itos-container.kind"] != "egress-proxy" || c.Container.Config.Labels["io.itos-container.project"] != w.approval.project {
				return errors.New("refuse to kill anything but the original owned proxy")
			}
			_, err = w.egress.native.cli.ContainerKill(ctx, id, client.ContainerKillOptions{Signal: "SIGKILL"})
			if err != nil {
				return err
			}
			// Wait independently before releasing the start reply, so the
			// failure is proxy death, never the scheduler's race with a kill.
			wait := w.egress.native.cli.ContainerWait(ctx, id, client.ContainerWaitOptions{Condition: "not-running"})
			select {
			case <-wait.Result:
				return nil
			case err := <-wait.Error:
				return err
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
	w.egress.native.cleanup = append(w.egress.native.cleanup, func() error { l.barrier.open(); return nil })
	return nil
}

func egressLifeRunID(run *egressNativeRun) string {
	if o := run.observed["attach"]; o != nil && o.proxy != nil && o.proxy.Config != nil {
		return o.proxy.Config.Labels["io.itos-container.session"]
	}
	return ""
}

func (w *world) egressLifeStartupRun() error {
	n, l := w.egress.native, w.egress.lifeStartup
	var err error
	l.control, err = n.drive([]egressStep{n.canonicalControl()}, 0)
	if err != nil {
		return err
	}
	l.controlID = egressLifeRunID(l.control)
	if err := w.egressInvoke([]string{"status", "--json"}); err != nil {
		return err
	}
	l.controlStatus = w.egress.results[len(w.egress.results)-1]
	// Install the one-shot fault after the separate healthy invocation ended.
	l.recorder.mu.Lock()
	l.recorder.requests, l.recorder.after = nil, l.barrier.after
	l.recorder.mu.Unlock()
	l.run = n.newRun([]egressStep{n.canonicalControl()}, 0)
	l.started = time.Now()
	if err := l.run.start(); err != nil {
		return err
	}
	select {
	case <-l.barrier.ready:
		l.reached = true
	case <-l.run.done:
	case <-time.After(150 * time.Second):
	}
	l.run.finish(45 * time.Second)
	l.ended = time.Now()
	l.barrier.open()
	l.journal = l.recorder.snapshot()
	l.id = egressLifeJournalSession(l.journal)
	l.path = w.egressStatePath("sessions", w.recoveryIndexKey(), l.id, "egress-refusals.jsonl")
	n.controlAll()
	return w.egressInvoke([]string{"status", "--json"})
}

func egressLifeJournalSession(journal []egressLifeRequest) string {
	for _, e := range journal {
		if egressLifeCreateKind(&e) != "" {
			var body map[string]any
			if json.Unmarshal(e.body, &body) == nil {
				if id, ok := object(body["Labels"])["io.itos-container.session"].(string); ok {
					return id
				}
			}
		}
	}
	return ""
}

func (w *world) egressLifeStartupControl() error {
	l, n := w.egress.lifeStartup, w.egress.native
	if l.control == nil {
		return errors.New("no healthy control invocation")
	}
	if err := errors.Join(l.control.cleaned(), n.exchanged(l.control, 1)); err != nil {
		return fmt.Errorf("%w\n%s", err, l.control.describe())
	}
	path := w.egressStatePath("sessions", w.recoveryIndexKey(), l.controlID, "egress-refusals.jsonl")
	return egressLifeStoredStatus(l.controlStatus, l.controlID, path, "stored", 0)
}

func egressLifeStoredStatus(result egressResult, id, path, state string, records any) error {
	entry, problems, err := egressLifeEntry(result, id)
	if err != nil {
		return err
	}
	want := map[string]any{"path": path, "state": state, "records": records, "missing": []any{}}
	if result.result.exit != 0 || len(problems) != 0 || !sameJSON(object(entry["egress"])["refusal_log"], want) || entry["cleanup"] != "complete" || len(anyList(entry["retry"])) != 0 {
		return fmt.Errorf("fresh status did not retain %s refusal evidence independently of completed cleanup", state)
	}
	if !sameJSON(object(entry["egress"])["limitations"], offlineStrings(egressLimitations)) {
		return errors.New("fresh status lost frozen egress limitations")
	}
	return nil
}

func (w *world) egressLifeStartupResult(rule string, exit int) error {
	l := w.egress.lifeStartup
	if !l.reached {
		return fmt.Errorf("product missed the native proxy barrier\n%s", l.run.describe())
	}
	l.barrier.mu.Lock()
	err, event := l.barrier.err, l.barrier.request
	l.barrier.mu.Unlock()
	if err != nil {
		return fmt.Errorf("native proxy barrier: %w", err)
	}
	if l.run.err != nil || l.run.timedOut || l.run.exit != exit || !strings.Contains(string(l.run.stderr.bytes()), rule) {
		return fmt.Errorf("wrong proxy startup failure\n%s", l.run.describe())
	}
	if l.ended.Sub(l.started) > engineReadinessAllowance+32*time.Second || l.ended.Sub(event.at) > engineCallBudget+32*time.Second {
		return errors.New("startup refusal exceeded the existing harness readiness/call/teardown budgets with 2s observation allowance")
	}
	if l.barrier.kind == "ready-held" && l.ended.Sub(event.at) < engineCallBudget {
		return errors.New("held readiness failed before its product call budget expired")
	}
	return nil
}

func (w *world) egressLifeStartupNoWorker() error {
	l := w.egress.lifeStartup
	creates, starts := map[string]int{}, 0
	for _, e := range l.journal {
		if e.method == "POST" && strings.HasSuffix(e.path, "/create") {
			kind := egressLifeCreateKind(&e)
			if e.path == "/containers/create" && kind != "proxy" {
				return errors.New("startup failure created a worker")
			}
			creates[e.path+kind]++
		}
		if e.method == "POST" && strings.HasSuffix(e.path, "/start") {
			starts++
		}
	}
	for _, count := range creates {
		if count != 1 {
			return errors.New("startup failure requested duplicate resources")
		}
	}
	if len(creates) != 4 || creates["/volumes/create"] != 1 || creates["/containers/createproxy"] != 1 || creates["/networks/createinternal"] != 1 || creates["/networks/createegress"] != 1 || starts != 1 {
		return errors.New("startup did not reach the original proxy exactly once")
	}
	if err := egressLifeStartupRoutes(l.journal); err != nil {
		return err
	}
	if data, err := os.ReadFile(filepath.Join(w.doctor.checkout, "slice-27-report.json")); err == nil && len(data) != 0 {
		return errors.New("startup refusal executed worker instructions")
	}
	return nil
}

// Only the initial attachment of the original proxy to its owned networks is
// permitted. A new attachment, update, restart or exec is not startup cleanup.
func egressLifeStartupRoutes(journal []egressLifeRequest) error {
	proxy := ""
	networks, attached := map[string]bool{}, map[string]bool{}
	for _, e := range journal {
		if e.status >= 200 && e.status < 300 {
			var reply struct {
				ID string `json:"Id"`
			}
			_ = json.Unmarshal(e.response, &reply)
			switch egressLifeCreateKind(&e) {
			case "proxy":
				proxy = reply.ID
			case "internal", "egress":
				networks[reply.ID] = true
			}
		}
		if e.method == "GET" || e.method == "HEAD" || e.method == "DELETE" {
			continue
		}
		if e.method != "POST" {
			return errors.New("startup requested an unexpected mutation")
		}
		if strings.HasSuffix(e.path, "/create") {
			continue
		}
		if proxy != "" && (e.path == "/containers/"+proxy+"/start" || e.path == "/containers/"+proxy+"/wait" || e.path == "/containers/"+proxy+"/kill") {
			continue
		}
		id := strings.TrimSuffix(strings.TrimPrefix(e.path, "/networks/"), "/connect")
		var connect struct{ Container string }
		if strings.HasPrefix(e.path, "/networks/") && strings.HasSuffix(e.path, "/connect") && networks[id] && !attached[id] && proxy != "" && json.Unmarshal(e.body, &connect) == nil && connect.Container == proxy {
			attached[id] = true
			continue
		}
		return errors.New("startup requested a fallback attachment, restart or unexpected mutation")
	}
	return nil
}

func (w *world) egressLifeStartupCleanup() error {
	l := w.egress.lifeStartup
	if len(l.run.remaining) != 0 {
		return errors.New("startup failure left owned resources")
	}
	if err := egressLifeReverseCreated(l.journal); err != nil {
		return err
	}
	if err := egressLifeStoredStatus(w.egress.results[len(w.egress.results)-1], l.id, l.path, "stored", 0); err != nil {
		return err
	}
	return errors.Join(w.approvalRecordUnchanged(), w.egress.native.endpointsIntact())
}

// The oracle derives the removal inventory from successful native create replies,
// not from whatever the product later happens to delete.
func egressLifeReverseCreated(journal []egressLifeRequest) error {
	var created, removed []string
	for _, e := range journal {
		if e.method == "POST" && e.status >= 200 && e.status < 300 {
			var reply map[string]any
			_ = json.Unmarshal(e.response, &reply)
			switch e.path {
			case "/volumes/create":
				name, _ := reply["Name"].(string)
				if name == "" {
					return errors.New("native anchor create has no name")
				}
				created = append(created, "/volumes/"+name)
			case "/containers/create", "/networks/create":
				id, _ := reply["Id"].(string)
				if id == "" {
					return errors.New("native create has no full ID")
				}
				created = append(created, strings.TrimSuffix(e.path, "/create")+"/"+id)
			}
		}
		if e.method == "DELETE" {
			if e.status != 204 {
				return errors.New("owned removal was not accepted")
			}
			removed = append(removed, e.path)
		}
	}
	slices.Reverse(created)
	if len(created) == 0 || !slices.Equal(created, removed) {
		return errors.New("native removal inventory/order differs from accepted creates")
	}
	for i, path := range removed {
		deleteAt, absentAt, nextAt := -1, -1, len(journal)
		inspect := path
		if strings.HasPrefix(path, "/containers/") {
			inspect += "/json"
		}
		for j, e := range journal {
			if e.method == "DELETE" && e.path == path {
				deleteAt = j
			}
			if deleteAt >= 0 && j > deleteAt && e.method == "GET" && e.path == inspect && e.status == 404 && absentAt < 0 {
				absentAt = j
			}
			if i+1 < len(removed) && e.method == "DELETE" && e.path == removed[i+1] {
				nextAt = j
			}
		}
		if deleteAt < 0 || absentAt <= deleteAt || absentAt >= nextAt {
			return errors.New("removal lacks an independent absence reply before the next dependency")
		}
	}
	return nil
}
