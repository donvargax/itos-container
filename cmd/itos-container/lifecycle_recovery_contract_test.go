package main

import (
	"context"
	"os"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/donvargax/itos-container/internal/engine/session/domain"
	sp "github.com/donvargax/itos-container/internal/engine/session/port"
	sf "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
)

func TestLifecycleRecoveryContract(t *testing.T) {
	operation := os.Getenv("SLICE_12_RECOVERY")
	if operation == "" {
		t.Skip("selected by lifecycle scenarios")
	}
	for _, backend := range []string{"ports", "protected records"} {
		t.Run(backend, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				l := lifecycleLocalFixture(t, backend)
				id, worker := l.m.seed("a")
				if registry, ok := l.registry.(*sf.Registry); ok {
					ref := sp.Reference(l.m.c.Project, strings.Repeat("a", 63)+"1")
					registry.Records = map[string]sp.PublicRecord{id: {Schema: 1, ID: id, Project: l.m.c.Project, Intent: ref, State: "failed"}}
					registry.Store.Records = map[string][]byte{ref: l.m.storage.Directories["intents/"+l.m.c.Project].Files[strings.Repeat("a", 63)+"1.json"]}
				}
				command := os.Getenv("SLICE_12_COMMAND")
				limit := 30 * time.Second
				if command == "status" {
					limit = 10 * time.Second
				}
				if operation == "signal delivery" {
					o := l.m.e.Resources[worker]
					o.Running, o.Started, o.Status, o.PID = true, true, "running", 123
					l.m.e.Resources[worker] = o
					l.m.e.WaitUntilCancel, l.m.e.IgnoreSignal = true, true
				}
				removed, fired := false, false
				l.hook = func(ctx context.Context, event string) {
					match := operation == "engine inspection" && event == "inspect" || operation == "session lock acquisition" && event == "session-open" || operation == "signal delivery" && event == "SIGTERM" || operation == "removal" && event == "remove" || operation == "absence confirmation" && removed && event == "inspect"
					if event == "remove" {
						removed = true
					}
					if match {
						fired = true
						<-ctx.Done()
					}
					if operation == "aggregate record reconciliation" && (event == "open" || event == "check" || event == "inspect") || operation == "aggregate cleanup retries" && (event == "check" || event == "inspect") {
						fired = true
						select {
						case <-time.After(6 * time.Second):
						case <-ctx.Done():
						}
					}
				}
				service := domain.Recovery{Registry: lifecycleRegistry{l.registry, l}, Engine: lifecycleEngine{l}}
				start := time.Now()
				first := publicMemoryCommand(t, service, command, id, 75)
				elapsed := time.Since(start)
				if !fired || elapsed > limit || len(first.Problems) == 0 || first.Problems[0].Rule != "recovery.temporary" {
					t.Error("recovery deadline/result", elapsed, first)
				}
				for _, event := range l.events {
					if event.remaining > 10*time.Second {
						t.Error("finite recovery call exceeds limit", event.name, event.remaining)
					}
				}
				for _, entry := range first.Sessions {
					if entry.ID != id || entry.WorkloadExit != nil || entry.Cleanup == "complete" {
						t.Error("timeout fabricated completion or lost session", entry)
					}
				}
				l.hook = nil
				l.m.e.WaitUntilCancel, l.m.e.IgnoreSignal = false, false
				before := len(l.events)
				fresh := publicMemoryCommand(t, service, command, id, 0)
				if len(fresh.Sessions) != 1 || fresh.Sessions[0].ID != id || fresh.Sessions[0].Project != l.m.c.Project || fresh.Sessions[0].Fingerprint != l.m.c.Fingerprint {
					t.Error("fresh recovery lost binding", fresh)
				}
				if len(l.events) == before || l.events[before].remaining <= 0 {
					t.Error("fresh recovery reused expired budget")
				}
				if command == "stop" && (fresh.Sessions[0].Cleanup != "complete" || len(l.m.e.Resources) != 1) {
					t.Error("fresh stop did not complete", fresh)
				}
				if command == "status" && len(l.m.e.Resources) != 2 {
					t.Error("status mutated worker")
				}
				if l.count("create") != 0 || l.count("start") != 0 || l.m.e.Resources[strings.Repeat("f", 64)].Name != "unrelated" {
					t.Error("recovery adopted/launched/unrelated mutation")
				}
			})
		})
	}
}
