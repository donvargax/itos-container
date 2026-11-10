package domain

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/donvargax/itos-container/internal/engine/session/port"
	f "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
)

func TestEgressInspectionDistinguishesEveryResourceStage(t *testing.T) {
	for index, kind := range []string{"internal-network", "egress-network", "proxy"} {
		for _, stage := range []string{"", "intent", "owned", "absent"} {
			for _, present := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/present=%v", kind, stage, present), func(t *testing.T) {
					seed := index * 2
					switch stage {
					case "intent":
						seed++
					case "owned":
						seed += 2
					case "absent":
						seed = 12 - index
					}
					_, registry, engine, id, r := seedEgressSession(t, seed)
					i, c := openEgressTestSession(t, registry, engine, id)
					v := *egressResource(&r, index)
					resourceID := v.ID
					if resourceID == "" {
						resourceID = strings.Repeat("f", 64)
					}
					if index == 2 {
						o := f.EgressProxy(r, i.Binding())
						o.ID = resourceID
						if present {
							engine.Resources[resourceID] = o
						} else {
							delete(engine.Resources, resourceID)
						}
					} else {
						networkKind := port.InternalNetwork
						if index == 1 {
							networkKind = port.EgressNetwork
						}
						o := f.EgressNetwork(r, networkKind)
						o.ID = resourceID
						if present {
							engine.Networks[resourceID] = o
						} else {
							delete(engine.Networks, resourceID)
						}
					}
					engine.Events, engine.Engine.Events = nil, nil
					before := slices.Clone(registry.Store.Records[registry.Records[id].Intent])
					gotID, gotPresent, running, failure := inspectEgressResource(t.Context(), i, c, index)
					wantID, wantPresent, wantFailure := resourceID, present, port.Outcome("")
					switch {
					case stage == "":
						wantID, wantPresent = "", false
						if len(engine.Events) != 0 || len(engine.Engine.Events) != 0 {
							t.Error("never-requested resource was inspected")
						}
					case stage == "intent" && !present:
						wantID, wantPresent, wantFailure = "", false, port.Incomplete
					case stage == "absent" && present:
						wantID, wantPresent, wantFailure = "", false, port.Refused
					}
					if gotID != wantID || gotPresent != wantPresent || running || failure != wantFailure {
						t.Fatalf("inspection = %q/%v/%v/%s, want %q/%v/false/%s", gotID, gotPresent, running, failure, wantID, wantPresent, wantFailure)
					}
					if !slices.Equal(before, registry.Store.Records[registry.Records[id].Intent]) {
						t.Fatal("inspection adopted or rewrote the resource")
					}
				})
			}
		}
	}
}

func TestEgressNetworkReleaseEndpointBoundary(t *testing.T) {
	for index, kind := range []port.NetworkKind{port.InternalNetwork, port.EgressNetwork} {
		for _, endpoints := range []int{0, 1, 2} {
			t.Run(fmt.Sprintf("%s/%d", kind, endpoints), func(t *testing.T) {
				_, registry, engine, id, r := seedEgressSession(t, 2+index*2)
				i, c := openEgressTestSession(t, registry, engine, id)
				v := *egressResource(&r, index)
				o := engine.Networks[v.ID]
				o.Endpoints = endpoints
				engine.Networks[v.ID] = o
				want := port.Incomplete
				if endpoints == 0 {
					want = ""
				}
				if got := releaseEgressResource(t.Context(), i, c, index); got != want {
					t.Fatalf("release = %s, want %s", got, want)
				}
				saved := i.Record()
				if endpoints == 0 {
					if egressResource(&saved, index).Stage != "absent" || !slices.Contains(engine.Events, string(kind)+" remove") {
						t.Fatal("empty network was not durably released", saved, engine.Events)
					}
				} else if saved != r || slices.Contains(engine.Events, string(kind)+" remove") {
					t.Fatal("occupied network received a destructive request", saved, engine.Events)
				}
				// Removing the foreign endpoint permits a fresh retry. A second
				// retry of the absent record must inspect but never remove again.
				o.Endpoints = 0
				if endpoints != 0 {
					engine.Networks[v.ID] = o
				}
				for range 2 {
					if got := releaseEgressResource(t.Context(), i, c, index); got != "" {
						t.Fatal("empty/absent retry failed", got)
					}
				}
				removals := 0
				for _, event := range engine.Events {
					if event == string(kind)+" remove" {
						removals++
					}
				}
				if removals != 1 {
					t.Fatal("network was removed more than once", engine.Events)
				}
			})
		}
	}
}

func TestEgressRecoveryKeepsFirstFailureAndCopiesGap(t *testing.T) {
	gap := port.RecoveryFailure{Outcome: port.RefusalLogGap, Gaps: []port.RefusalGap{{First: 2, Last: 3}, {First: 5, Last: 5}}}
	for _, tc := range []struct {
		name     string
		failures []port.RecoveryFailure
		want     port.Outcome
		gap      bool
	}{
		{"clean", nil, port.Complete, false},
		{"gap only", []port.RecoveryFailure{gap}, port.Complete, true},
		{"failure then gap", []port.RecoveryFailure{{Outcome: port.Refused}, gap, {Outcome: port.Incomplete}}, port.Refused, true},
		{"gap then failures", []port.RecoveryFailure{gap, {Outcome: port.Incomplete}, {Outcome: port.Refused}}, port.Incomplete, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var report port.RefusalReport
			got := recoveryCleanup(port.RecoveryResult{Failures: tc.failures}, &report)
			wantReport := port.RefusalReport{}
			if tc.gap {
				wantReport = port.RefusalReport{Outcome: gap.Outcome, Gaps: slices.Clone(gap.Gaps)}
			}
			if got != tc.want || !reflect.DeepEqual(report, wantReport) {
				t.Fatalf("cleanup = %s/%#v, want %s/%#v", got, report, tc.want, wantReport)
			}
			if tc.gap {
				report.Gaps[0].First = 99
				if gap.Gaps[0].First != 2 {
					t.Fatal("returned gap aliases recovery storage")
				}
			}
		})
	}
}
