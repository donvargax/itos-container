package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/donvargax/itos-container/internal/engine/session/domain"
	sp "github.com/donvargax/itos-container/internal/engine/session/port"
	sf "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
	"github.com/donvargax/itos-container/internal/engine/session/presentation"
	status "github.com/donvargax/itos-container/internal/slices/status/command"
	stop "github.com/donvargax/itos-container/internal/slices/stop/command"
)

// This executable seam only assembles real handlers and codecs with I/O-free
// ports. The enclosing scenario passes values and collects the test verdict.
func TestAnchorRecoveryMemory(t *testing.T) {
	input := os.Getenv("SLICE_17_ANCHOR_RECOVERY")
	if input == "" {
		t.Skip("slice-17 scenario subprocess only")
	}
	var request map[string]string
	if err := json.Unmarshal([]byte(input), &request); err != nil {
		t.Fatal(err)
	}
	synctest.Test(t, func(t *testing.T) { anchorRecoveryMemoryCase(t, request) })
}
func anchorRecoveryMemoryCase(t *testing.T, request map[string]string) {
	m := publicMemoryFixtureAt(t, request["root"])
	letter := request["letter"]
	if letter == "" {
		letter = "a"
	}
	id, worker := m.seed(letter)
	token := strings.Repeat(letter, 63) + "1"
	r := m.b.Intent(token)
	r.Schema, r.Stage, r.ContainerID = 2, "owned", worker
	r.Anchor = sp.Anchor{Session: id, Name: "itos-container-anchor-" + token, Source: "/proc/2147483647/fd/17", Device: m.b.Checkout.Device, Inode: m.b.Checkout.Inode, Created: "2026-11-07T17:00:00Z", Stage: "owned"}
	if request["completion"] == "durable" {
		r.Stage, r.Anchor.Stage = "absent", "absent"
	}
	m.storage.Directories["intents/"+m.c.Project].Files[token+".json"] = m.registry.Store.Codec.Encode(r)
	files := m.files.Sessions[m.index().Project+"/"+id]
	p, _ := m.registry.Codec.DecodePublic(files.Data)
	p.State = request["phase"]
	if request["exit"] != "unknown" {
		n, _ := strconv.Atoi(request["exit"])
		p.WorkloadExit = &n
	}
	files.Data = m.registry.Codec.EncodePublic(p)
	o := m.e.Resources[worker]
	o.Status = request["worker"]
	if o.Status == "running ignoring SIGTERM" {
		o.Status = "running"
		m.e.IgnoreSignal = true
	}
	if o.Status == "running" {
		o.Running, o.Started, o.PID = true, true, 123
	}
	if o.Status == "exited" {
		o.Started = true
	}
	m.e.Resources[worker] = o
	var observed map[string]any
	encoded, _ := json.Marshal(o)
	_ = json.Unmarshal(encoded, &observed)
	observed["AnchorName"], observed["AnchorExact"] = r.Anchor.Name, true
	encoded, _ = json.Marshal(observed)
	_ = json.Unmarshal(encoded, &o)
	m.e.Resources[worker] = o
	if request["worker"] == "absent" {
		delete(m.e.Resources, worker)
	}
	engine := &anchorRecoveryMemoryEngine{RunEngine: m.e, volumes: &sf.AnchorResources{Created: r.Anchor.Created}}
	if request["volume"] == "present" {
		if f := engine.volumes.AllocateAnchor(context.Background(), r); f != "" {
			t.Fatal(f)
		}
	}
	legacy, legacyWorker := m.seed("c")
	delete(m.e.Resources, legacyWorker)
	l := m.files.Sessions[m.index().Project+"/"+legacy]
	lp, _ := m.registry.Codec.DecodePublic(l.Data)
	lp.State = "stopped"
	l.Data = m.registry.Codec.EncodePublic(lp)
	li := m.b.Intent(strings.Repeat("c", 63) + "1")
	li.Stage, li.ContainerID = "absent", legacyWorker
	m.storage.Directories["intents/"+m.c.Project].Files[li.NameToken()+".json"] = m.registry.Store.Codec.Encode(li)
	before := append([]byte(nil), files.Data...)
	beforeLegacy := append([]byte(nil), l.Data...)
	service := domain.Recovery{Registry: m.registry, Engine: engine}
	if request["mode"] == "stop" {
		code, report := anchorMemoryInvoke(service, "status", id)
		if code != 0 || len(report.Sessions) != 2 {
			t.Fatal("control status failed", code, report)
		}
		if !bytes.Equal(before, files.Data) || !bytes.Equal(beforeLegacy, l.Data) {
			t.Fatal("control status wrote state")
		}
		anchorRecoveryMemoryStop(t, m, engine, service, id, worker, r, p)
		return
	}
	if request["mode"] != "" {
		anchorRecoveryMemoryFaultCase(t, request, m, engine, service, id, worker, r, p)
		return
	}
	var out, errout bytes.Buffer
	code := (status.CLI{Service: service, Presentation: presentation.UI{Out: &out, Err: &errout}, JSON: true}).Run()
	var report struct{ Sessions []map[string]any }
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Fatalf("anchor status failed: %d %s %s", code, out.String(), errout.String())
	}
	found := false
	for _, e := range report.Sessions {
		if e["id"] == legacy {
			if _, ok := e["anchor"]; ok {
				t.Fatal("legacy acquired anchor")
			}
			continue
		}
		if e["id"] != id {
			t.Fatal("unrelated session", e)
		}
		found = true
		cleanup := "pending"
		if request["completion"] == "durable" {
			cleanup = "complete"
		}
		for key, want := range map[string]string{"state": request["phase"], "resource": request["worker"], "anchor": request["volume"], "cleanup": cleanup, "fingerprint": m.c.Fingerprint, "controller": "absent"} {
			if e[key] != want {
				t.Errorf("%s = %v, want %s", key, e[key], want)
			}
		}
		if p.WorkloadExit == nil && e["workload_exit"] != nil {
			t.Error("invented workload exit")
		}
		if p.WorkloadExit != nil && e["workload_exit"] != float64(*p.WorkloadExit) {
			t.Error("lost workload exit")
		}
		if cleanup == "pending" && !reflect.DeepEqual(e["retry"], []any{"itos", "container", "stop", id}) {
			t.Error("lost retry")
		}
		if cleanup == "complete" && e["retry"] != nil {
			t.Error("completed retry")
		}
	}
	if !found {
		t.Fatal("anchor session missing")
	}
	out.Reset()
	errout.Reset()
	if code := (status.CLI{Service: service, Presentation: presentation.UI{Out: &out, Err: &errout}}).Run(); code != 0 || !strings.Contains(out.String(), "anchor="+request["volume"]) {
		t.Error("plain status omitted anchor", out.String(), errout.String())
	}
	if !bytes.Equal(before, files.Data) || !bytes.Equal(beforeLegacy, l.Data) || len(m.files.Events) != 0 {
		t.Error("status mutated protected state")
	}
	if engine.inspections != 2 {
		t.Error("anchor observation missing or unrelated anchor inspected", engine.inspections)
	}
}

type anchorRecoveryMemoryEngine struct {
	delay         time.Duration
	inspectAnchor func(context.Context, string, sp.AnchorObservation, sp.Inspection) (sp.AnchorObservation, sp.Inspection)
	removeAnchor  func(context.Context, string) bool
	removeWorker  func(context.Context, string) bool
	sp.RunEngine
	volumes      *sf.AnchorResources
	inspections  int
	deletions    int
	beforeDelete func()
}

func (e *anchorRecoveryMemoryEngine) OpenRun(ctx context.Context, b sp.Binding) (sp.RunConnection, sp.Outcome) {
	if e.delay > 0 {
		select {
		case <-time.After(e.delay):
		case <-ctx.Done():
			return nil, sp.Incomplete
		}
	}
	c, f := e.RunEngine.OpenRun(ctx, b)
	if f != "" {
		return nil, f
	}
	return anchorRecoveryMemoryConnection{RunConnection: c, engine: e}, ""
}

type anchorRecoveryMemoryConnection struct {
	sp.RunConnection
	engine *anchorRecoveryMemoryEngine
}

func (c anchorRecoveryMemoryConnection) AllocateAnchor(ctx context.Context, r sp.Record) sp.Outcome {
	return c.engine.volumes.AllocateAnchor(ctx, r)
}
func (c anchorRecoveryMemoryConnection) InspectAnchor(ctx context.Context, name string) (sp.AnchorObservation, sp.Inspection) {
	c.engine.inspections++
	o, s := c.engine.volumes.InspectAnchor(ctx, name)
	if c.engine.inspectAnchor != nil {
		return c.engine.inspectAnchor(ctx, name, o, s)
	}
	return o, s
}
func (c anchorRecoveryMemoryConnection) RemoveAnchor(ctx context.Context, name string) {
	if c.engine.beforeDelete != nil {
		c.engine.beforeDelete()
	}
	c.engine.deletions++
	if c.engine.removeAnchor != nil && !c.engine.removeAnchor(ctx, name) {
		return
	}
	c.engine.volumes.RemoveAnchor(ctx, name)
}

func (c anchorRecoveryMemoryConnection) Remove(ctx context.Context, id string) {
	if c.engine.removeWorker != nil && !c.engine.removeWorker(ctx, id) {
		return
	}
	c.RunConnection.Remove(ctx, id)
}

func anchorRecoveryMemoryStop(t *testing.T, m publicMemory, engine *anchorRecoveryMemoryEngine, service domain.Recovery, id, worker string, r sp.Record, p sp.PublicRecord) {
	t.Helper()
	engine.beforeDelete = func() {
		if _, present := m.e.Resources[worker]; present {
			t.Error("anchor removed before worker absence")
		}
	}
	var previous []byte
	deletions := 0
	for attempt := 0; attempt < 2; attempt++ {
		before := len(m.e.Events)
		var out, errout bytes.Buffer
		code := (stop.CLI{Service: service, Presentation: presentation.UI{Out: &out, Err: &errout}, JSON: true, Args: []string{id}}).Run()
		var report struct{ Sessions []map[string]any }
		if code != 0 || json.Unmarshal(out.Bytes(), &report) != nil {
			t.Fatalf("anchor stop failed: %d %s %s", code, out.String(), errout.String())
		}
		if len(report.Sessions) != 1 {
			t.Fatal("missing session", report)
		}
		e := report.Sessions[0]
		for key, want := range map[string]string{"state": "stopped", "resource": "absent", "anchor": "absent", "cleanup": "complete"} {
			if e[key] != want {
				t.Errorf("%s=%v want %s", key, e[key], want)
			}
		}
		if e["retry"] != nil || e["stop_candidate"] != false {
			t.Error("completion retains retry")
		}
		if p.WorkloadExit == nil && e["workload_exit"] != nil {
			t.Error("invented workload exit")
		}
		if p.WorkloadExit != nil && e["workload_exit"] != float64(*p.WorkloadExit) {
			t.Error("lost workload exit")
		}
		if _, present := m.e.Resources[worker]; present {
			t.Error("worker remains")
		}
		if _, present := engine.volumes.Resources[r.Anchor.Name]; present {
			t.Error("anchor remains")
		}
		files := m.files.Sessions[m.index().Project+"/"+id]
		if attempt == 0 {
			deletions = engine.deletions
			previous = append([]byte(nil), files.Data...)
		} else {
			if engine.deletions != deletions {
				t.Error("repeat deleted anchor")
			}
			if !bytes.Equal(previous, files.Data) {
				t.Error("repeat rewrote completion")
			}
			for _, event := range m.e.Events[before:] {
				if event == "remove" || event == "SIGTERM" || event == "SIGKILL" || event == "create" || event == "start" {
					t.Error("repeat mutated worker", event)
				}
			}
		}
	}
	for _, event := range m.e.Events {
		if event == "create" || event == "start" {
			t.Error("recovery launched work")
		}
	}
}
