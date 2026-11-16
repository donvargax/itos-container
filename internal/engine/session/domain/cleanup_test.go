package domain

import (
	"fmt"
	"slices"
	"testing"
	"testing/synctest"
	"time"

	budget "github.com/donvargax/itos-container/internal/engine/budget/domain"
	"github.com/donvargax/itos-container/internal/engine/session/port"
	f "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
)

func cleanupFixture(t *testing.T, schema int, stage string) (Runner, *f.Store, *f.RunEngine, string, port.Record) {
	t.Helper()
	runner, registry, engine, id, r := seedEgressSession(t, 8)
	store := registry.Store
	ref := registry.Records[id].Intent
	if schema < 3 {
		store.Codec.(*f.Codec).Value.Egress = port.EgressGrant{}
		delete(engine.Resources, r.Egress.Proxy.ID)
		engine.Networks = nil
		r.Egress = port.EgressRecord{}
	}
	if schema == 1 {
		engine.Anchors = nil
		r.Anchor = port.Anchor{}
	}
	if stage != "owned" {
		delete(engine.Resources, r.ContainerID)
		if stage != "absent" {
			r.ContainerID = ""
		}
	}
	r.Schema, r.Stage = schema, stage
	store.Records[ref] = store.Codec.Encode(r)
	store.Events = nil
	return runner, store, engine, ref, r
}

func TestCleanupOpenFailuresPreserveResources(t *testing.T) {
	for _, tc := range []struct {
		name string
		want port.Outcome
	}{
		{"store", port.Unsafe},
		{"engine", port.ExecutionFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runner, store, engine, ref, before := cleanupFixture(t, 3, "reserved")
			if tc.name == "store" {
				store.UnsafeRecord = true
			} else {
				engine.OpenFailure = tc.want
			}
			result := runner.Cleanup(t.Context(), ref)
			if result.Reference != ref || result.Outcome != tc.want || result.RefusalLog.Outcome != "" {
				t.Fatalf("cleanup = %#v, want %s for %s", result, tc.want, ref)
			}
			after, failure := store.Codec.Decode(store.Records[ref])
			if failure != "" || after != before || store.Locked || len(store.Events) != 0 || len(engine.Events) != 0 || len(engine.Engine.Events) != 0 {
				t.Fatal("failed acquisition advanced cleanup or retained its lock", after, store.Events, engine.Events)
			}
			if len(engine.Resources) != 1 || len(engine.Networks) != 2 || len(engine.Anchors) != 1 {
				t.Fatal("failed acquisition removed resources")
			}
		})
	}
}

func TestCleanupUncreatedSessions(t *testing.T) {
	for _, schema := range []int{2, 3} {
		for _, stage := range []string{"reserved", "cancelled"} {
			for _, fault := range []string{"none", "resource ownership", "cancellation publication"} {
				t.Run(fmt.Sprintf("schema%d/%s/%s", schema, stage, fault), func(t *testing.T) {
					runner, store, engine, ref, before := cleanupFixture(t, schema, stage)
					want := port.Complete
					switch fault {
					case "resource ownership":
						want = port.Refused
						if schema == 3 {
							o := engine.Resources[before.Egress.Proxy.ID]
							o.Project = "foreign"
							engine.Resources[o.ID] = o
						} else {
							o := engine.Anchors[before.Anchor.Name]
							o.Project = "foreign"
							engine.Anchors[o.Name] = o
						}
					case "cancellation publication":
						want = port.IntentWrite
						// Fail only the final cancellation save, after the anchor's
						// absence was durably recorded, including on a cancelled retry.
						store.AfterSave = func() {
							r, _ := store.Codec.Decode(store.Records[ref])
							if r.Anchor.Stage == "absent" {
								store.Fault = "cancelled-record file fsync"
							}
						}
					}
					result := runner.Cleanup(t.Context(), ref)
					if result.Reference != ref || result.Outcome != want || store.Locked {
						t.Fatalf("cleanup = %#v, want %s with released lock", result, want)
					}
					expected := before
					if fault != "resource ownership" {
						expected.Anchor.Stage = "absent"
						if schema == 3 {
							expected.Egress.Proxy.Stage, expected.Egress.Network.Stage, expected.Egress.Internal.Stage = "absent", "absent", "absent"
							log := runner.Refusals.(*f.RefusalLog)
							if string(log.Copies[before.Anchor.Session]) != f.ProxyStdout {
								t.Error("released proxy without its complete log")
							}
						}
						if fault == "none" {
							expected.Stage = "cancelled"
						}
						if len(engine.Resources) != 0 || len(engine.Networks) != 0 || len(engine.Anchors) != 0 {
							t.Error("cleanup left owned resources", engine.Events)
						}
					} else if len(store.Events) != 0 || len(engine.Anchors) != 1 || (schema == 3 && (len(engine.Resources) != 1 || len(engine.Networks) != 2)) {
						t.Error("release failure advanced cancellation or removed resources", store.Events, engine.Events)
					}
					after, failure := store.Codec.Decode(store.Records[ref])
					if failure != "" || after != expected {
						t.Fatalf("cleanup record = %#v, want %#v (decode %s)", after, expected, failure)
					}
					if fault == "cancellation publication" && !slices.ContainsFunc(store.Events, func(e f.WriteEvent) bool {
						return e.Stage == "cancelled" && e.Operation == "file fsync" && e.Failed
					}) {
						t.Error("cleanup never attempted the cancellation publication")
					}
					for _, event := range engine.Engine.Events {
						if event.Method == "INSPECT" && event.Target == before.Name {
							t.Error("uncreated cleanup fell through to worker lookup")
						}
					}
				})
			}
		}
	}
}

func TestCleanupWorkerRecords(t *testing.T) {
	for _, schema := range []int{1, 2, 3} {
		for _, stage := range []string{"intent", "owned", "absent"} {
			t.Run(fmt.Sprintf("schema%d/%s", schema, stage), func(t *testing.T) {
				runner, store, engine, ref, before := cleanupFixture(t, schema, stage)
				want := port.Complete
				expected := before
				if stage == "intent" {
					want = port.Uncertain
				} else {
					expected.Stage = "absent"
					if schema >= 2 {
						expected.Anchor.Stage = "absent"
					}
					if schema == 3 {
						expected.Egress.Proxy.Stage, expected.Egress.Network.Stage, expected.Egress.Internal.Stage = "absent", "absent", "absent"
					}
				}
				result := runner.Cleanup(t.Context(), ref)
				if result.Reference != ref || result.Outcome != want || store.Locked {
					t.Fatalf("cleanup = %#v, want %s with released lock", result, want)
				}
				after, failure := store.Codec.Decode(store.Records[ref])
				if failure != "" || after != expected {
					t.Fatalf("cleanup record = %#v, want %#v (decode %s)", after, expected, failure)
				}
				if stage == "intent" {
					if len(store.Events) != 0 || slices.Contains(engine.Events, "anchor remove") || slices.Contains(engine.Events, "proxy remove") {
						t.Fatal("uncertain worker released recovery resources", engine.Events)
					}
				} else if !after.Completed() || len(engine.Resources) != 0 || len(engine.Networks) != 0 || len(engine.Anchors) != 0 {
					t.Fatal("worker cleanup did not finish resource release", after, engine.Events)
				}
			})
		}
	}
}

func TestCleanupLegacyUncreatedRecordsRemainUncertain(t *testing.T) {
	for _, stage := range []string{"reserved", "cancelled"} {
		t.Run(stage, func(t *testing.T) {
			runner, store, engine, ref, before := cleanupFixture(t, 1, stage)
			result := runner.Cleanup(t.Context(), ref)
			if result.Reference != ref || result.Outcome != port.Uncertain || store.Locked {
				t.Fatalf("legacy cleanup = %#v, want uncertain with released lock", result)
			}
			after, failure := store.Codec.Decode(store.Records[ref])
			if failure != "" || after != before || len(store.Events) != 0 || !slices.ContainsFunc(engine.Engine.Events, func(e f.EngineEvent) bool {
				return e.Method == "INSPECT" && e.Target == before.Name
			}) {
				t.Fatal("legacy cleanup bypassed worker reconciliation", after, engine.Engine.Events)
			}
		})
	}
}

func TestCleanupConnectionBudgetBySchema(t *testing.T) {
	for _, tc := range []struct {
		schema   int
		want     port.Outcome
		duration time.Duration
	}{
		{1, port.Uncertain, budget.CallDuration + time.Second},
		{2, port.Complete, budget.CallDuration + time.Second},
		{3, port.Incomplete, budget.CallDuration},
	} {
		t.Run(fmt.Sprintf("schema%d", tc.schema), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				// Let an acquisition that outlives its call budget return and close
				// before the fake-clock scope exits, even if an assertion fails.
				defer func() { time.Sleep(2 * time.Second); synctest.Wait() }()
				runner, store, engine, ref, before := cleanupFixture(t, tc.schema, "reserved")
				engine.AfterOpen = func() { time.Sleep(budget.CallDuration + time.Second) }
				start := time.Now()
				result := runner.Cleanup(t.Context(), ref)
				if result.Reference != ref || result.Outcome != tc.want || time.Since(start) != tc.duration || store.Locked {
					t.Fatalf("slow connection cleanup = %#v after %s, want %s after %s", result, time.Since(start), tc.want, tc.duration)
				}
				if tc.schema == 3 {
					after, failure := store.Codec.Decode(store.Records[ref])
					if failure != "" || after != before || len(store.Events) != 0 || len(engine.Resources) != 1 || len(engine.Networks) != 2 || len(engine.Anchors) != 1 {
						t.Fatal("connection timeout advanced cleanup", after)
					}
				}
			})
		})
	}
}
