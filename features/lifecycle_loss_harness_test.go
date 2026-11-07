package features

import (
	"io"
	"reflect"
	"testing"

	"github.com/cucumber/godog"
)

func TestLifecycleLossGuard(t *testing.T) {
	t.Setenv("ITOS_CONTAINER_ENGINE_TESTS", "0")
	w := &world{}
	if err := w.lifecycleLossFixture(); err == nil || w.loss != nil || w.offline != nil || w.doctor != nil {
		t.Fatal("loss fixture initialized outside the guarded CI tier")
	}
}

func TestLifecycleLossPreservationAccountsOnlyForItsControlReconnect(t *testing.T) {
	resource := func(endpoint, address string) map[string]string {
		return map[string]string{"/images/json?all=1": "unchanged image", "/networks": string(approvalJSON(map[string]any{
			"Id": "fixture-network", "Labels": map[string]any{"fixture": "same"}, "Containers": map[string]any{
				"fixture-control": map[string]any{"Name": "control", "IPv4Address": address, "IPv6Address": "fd14:14::2/64", "EndpointID": endpoint, "MacAddress": endpoint},
			},
		}))}
	}
	view := func(r map[string]string) (map[string]string, error) {
		return lossResourceView(r, "fixture-network", "fixture-control", "control")
	}
	before, err := view(resource("before", "172.30.14.2/24"))
	if err != nil {
		t.Fatal(err)
	}
	after, err := view(resource("after", "172.30.14.2/24"))
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("recorded fixture reconnect was not accounted for", err)
	}
	if _, err := view(resource("after", "172.30.14.9/24")); err == nil {
		t.Fatal("changed control address passed")
	}
	r := resource("after", "172.30.14.2/24")
	r["/images/json?all=1"] = "changed image"
	after, err = view(r)
	if err != nil || reflect.DeepEqual(before, after) {
		t.Fatal("unrelated change was hidden", err)
	}
}

func TestLifecycleLossStepsDefined(t *testing.T) {
	definitions := &lifecycleDefinitions{t: t}
	(&world{}).lifecycleLossSteps(definitions)
	(&world{}).lifecycleNativeSteps(definitions)
	(&world{}).offlineIsolationSteps(definitions)
	suite := godog.TestSuite{Name: "slice-15-definitions", Options: &godog.Options{Paths: []string{"lifecycle.feature"}, Tags: "@slice-15", Output: io.Discard}}
	features, err := suite.RetrieveFeatures()
	if err != nil {
		t.Fatal(err)
	}
	cases, steps := 0, 0
	for _, feature := range features {
		for _, scenario := range feature.Pickles {
			cases++
			for _, step := range scenario.Steps {
				steps++
				matches := 0
				for _, expression := range definitions.patterns {
					if expression.MatchString(step.Text) {
						matches++
					}
				}
				if matches != 1 {
					t.Errorf("%q has %d definitions", step.Text, matches)
				}
			}
		}
	}
	if cases != 1 || steps != 8 {
		t.Fatalf("transferred case/step totals = %d/%d, want 1/8", cases, steps)
	}
}

func TestLifecycleLossFreshReportInventory(t *testing.T) {
	makeReports := func() []isolationReport {
		var reports []isolationReport
		for _, kind := range []string{"round", "identity", "filesystem", "host", "network", "network", "dns", "dns", "loopback"} {
			reports = append(reports, isolationReport{Kind: kind, Nonce: "fresh", Values: map[string]string{"pid": "1", "start": "123"}})
		}
		return reports
	}
	if err := lossReportInventory(makeReports(), "fresh", "123"); err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func([]isolationReport) []isolationReport{
		"empty":       func([]isolationReport) []isolationReport { return nil },
		"missing DNS": func(r []isolationReport) []isolationReport { return r[:6] },
		"duplicate":   func(r []isolationReport) []isolationReport { r[3] = r[0]; return r },
		"stale after kill": func(r []isolationReport) []isolationReport {
			for i := range r {
				r[i].Nonce = "baseline"
			}
			return r
		},
		"mixed nonce":          func(r []isolationReport) []isolationReport { r[5].Nonce = "baseline"; return r },
		"replacement primary":  func(r []isolationReport) []isolationReport { r[0].Values["start"] = "456"; return r },
		"replacement identity": func(r []isolationReport) []isolationReport { r[1].Values["start"] = "456"; return r },
		"descendant":           func(r []isolationReport) []isolationReport { r[0].Values["pid"] = "2"; return r },
		"probe error":          func(r []isolationReport) []isolationReport { r[2].Error = "failed"; return r },
	} {
		t.Run(name, func(t *testing.T) {
			if lossReportInventory(change(makeReports()), "fresh", "123") == nil {
				t.Fatal("invalid loss evidence passed")
			}
		})
	}
}

func TestLifecycleLossReceiptsRequireBothControlsAndNoWorkerTraffic(t *testing.T) {
	routes := []isolationRoute{{Name: "v4"}, {Name: "v6"}, {Name: "docker-aaaa"}, {Name: "https-a"}}
	seen := map[string]bool{}
	for _, route := range routes {
		for _, phase := range []string{"first", "running"} {
			for _, suffix := range []string{"b", "a"} {
				seen["round"+suffix+"-"+phase+"-"+route.Name] = true
			}
		}
	}
	if err := lossReceiptInventory(seen, "round", routes); err != nil {
		t.Fatal(err)
	}
	for key := range seen {
		seen[key] = false
		if lossReceiptInventory(seen, "round", routes) == nil {
			t.Fatalf("missing receipt %s passed", key)
		}
		seen[key] = true
	}
	for _, route := range routes {
		for _, phase := range []string{"first", "running"} {
			key := "round-" + phase + "-" + route.Name
			seen[key] = true
			if lossReceiptInventory(seen, "round", routes) == nil {
				t.Fatalf("worker receipt %s passed", key)
			}
			delete(seen, key)
		}
	}
	if lossReceiptInventory(seen, "round", nil) == nil {
		t.Fatal("empty route plan passed")
	}
}
