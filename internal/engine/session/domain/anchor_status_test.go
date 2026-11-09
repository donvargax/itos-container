package domain_test

import (
	"bytes"
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/donvargax/itos-container/internal/engine/session/port"
	f "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
)

func TestAnchoredStatusKeepsNoCreateProofAndInspectsAnchor(t *testing.T) {
	for _, stage := range []string{"reserved", "cancelled"} {
		for _, fault := range []string{"present", "absent", "foreign", "unavailable", "engine unavailable", "unsafe session"} {
			t.Run(stage+"/"+fault, func(t *testing.T) {
				s, registry, engine, id := publicRecoveryFixture(t)
				p := registry.Records[id]
				p.State = "preparing"
				i, failure := registry.Store.Open(t.Context(), p.Intent)
				if failure != "" {
					t.Fatal(failure)
				}
				r := f.RunIntent(i.Binding())
				i.Close()
				r.Stage, r.ContainerID, r.Anchor.Session = stage, "", id
				if stage == "cancelled" && fault == "absent" {
					p.State, r.Anchor.Stage = "stopped", "absent"
				}
				registry.Records[id] = p
				data := registry.Store.Codec.Encode(r)
				registry.Store.Records[p.Intent] = bytes.Clone(data)
				delete(engine.Resources, strings.Repeat("2", 64))
				anchors := f.AnchorResources{Created: r.Anchor.Created}
				if failure := anchors.AllocateAnchor(t.Context(), r); failure != "" {
					t.Fatal(failure)
				}
				engine.Anchors = anchors.Resources
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				wantResource, wantAnchor, wantCleanup := "absent", fault, "pending"
				var wantFailure port.Outcome
				switch fault {
				case "absent":
					delete(engine.Anchors, r.Anchor.Name)
					if stage == "cancelled" {
						wantCleanup = "complete"
					}
				case "foreign":
					o := engine.Anchors[r.Anchor.Name]
					o.Session = strings.Repeat("f", 64)
					engine.Anchors[r.Anchor.Name] = o
					wantFailure = port.Refused
				case "unavailable":
					engine.After = func(event string) {
						if event == "anchor inspect" {
							cancel()
						}
					}
					wantFailure = port.Incomplete
				case "engine unavailable":
					engine.Unavailable = true
					wantFailure = port.Incomplete
				case "unsafe session":
					registry.AfterOpen = func(string) { registry.Store.UnsafeRecord = true }
					wantResource, wantFailure = "unknown", port.Unsafe
				}
				if wantFailure != "" {
					wantAnchor, wantCleanup = "unknown", "unknown"
				}
				before := len(registry.Store.Events)
				result := s.Status(ctx)
				if len(result.Entries) != 1 {
					t.Fatal("lost no-create session", result)
				}
				e := result.Entries[0]
				if e.Resource != wantResource || e.Anchor != wantAnchor || e.Cleanup != wantCleanup || e.WorkloadExit != nil {
					t.Fatal("worker proof and anchor observation were conflated", result)
				}
				if wantFailure == "" && len(result.Failures) != 0 || wantFailure != "" && !slices.Equal(result.Failures, []port.RecoveryFailure{{Session: id, Outcome: wantFailure}}) {
					t.Fatal("lost independent failure", result)
				}
				if (e.Retry == nil) != (wantCleanup == "complete") || e.StopCandidate != (wantCleanup != "complete") {
					t.Fatal("incorrect completion guidance", e)
				}
				for _, event := range engine.Engine.Events {
					if event.Method != "INFO" {
						t.Fatal("no-create status looked up or changed a worker", engine.Engine.Events)
					}
				}
				if fault != "unsafe session" && fault != "engine unavailable" && !slices.Contains(engine.Events, "anchor inspect") {
					t.Fatal("status skipped independent anchor inspection", engine.Events)
				}
				if registry.Records[id] != p || !bytes.Equal(data, registry.Store.Records[p.Intent]) || registry.Writes != 0 || registry.Syncs != 0 || len(registry.Store.Events) != before {
					t.Fatal("status changed protected evidence")
				}
			})
		}
	}
}
