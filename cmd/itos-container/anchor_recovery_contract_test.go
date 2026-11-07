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

	"github.com/donvargax/itos-container/internal/engine/session/domain"
	sp "github.com/donvargax/itos-container/internal/engine/session/port"
	sf "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
	"github.com/donvargax/itos-container/internal/engine/session/presentation"
	status "github.com/donvargax/itos-container/internal/slices/status/command"
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
	m := publicMemoryFixture(t)
	id, worker := m.seed("a")
	token := strings.Repeat("a", 63) + "1"
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
	if o.Status == "running" {
		o.Running, o.Started, o.PID = true, true, 123
	}
	if o.Status == "exited" {
		o.Started = true
	}
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
	sp.RunEngine
	volumes     *sf.AnchorResources
	inspections int
}

func (e *anchorRecoveryMemoryEngine) OpenRun(ctx context.Context, b sp.Binding) (sp.RunConnection, sp.Outcome) {
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
	return c.engine.volumes.InspectAnchor(ctx, name)
}
func (c anchorRecoveryMemoryConnection) RemoveAnchor(ctx context.Context, name string) {
	c.engine.volumes.RemoveAnchor(ctx, name)
}
