package features

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestOfflineIsolationGuard(t *testing.T) {
	t.Setenv("ITOS_CONTAINER_ENGINE_TESTS", "0")
	for _, mode := range []string{"identity", "filesystem", "host", "network", "dns", "primary", "exit"} {
		t.Run(mode, func(t *testing.T) {
			w := &world{}
			if err := w.isolationFixture(mode); err == nil || w.offline != nil || w.doctor != nil {
				t.Fatal("native fixture initialized outside CI")
			}
		})
	}
}

func TestOfflineIsolationProbeCompilesWithoutEngine(t *testing.T) {
	root, err := moduleRoot()
	if err != nil {
		t.Fatal(err)
	}
	w := &world{root: root, dir: t.TempDir(), doctor: &doctorWorld{checkout: t.TempDir()}}
	if err = w.buildIsolationProbe(); err != nil {
		t.Fatal(err)
	}
	// Never execute this helper on the host, including for a harness test.
}

func TestOfflineIsolationRoutesRequireCompleteAttemptsAndWorkingControls(t *testing.T) {
	c := isolationConfig{Mode: "network", Nonce: "nonce", Routes: []isolationRoute{{Name: "v4", Protocol: "tcp4", Address: "192.0.2.1:1", Answer: "control"}, {Name: "v6", Protocol: "udp6", Address: "[2001:db8::1]:1", Answer: "control"}}}
	makeReports := func(control bool) []isolationReport {
		var reports []isolationReport
		for _, phase := range []string{"first", "running"} {
			r := isolationReport{Kind: c.Mode, Nonce: c.Nonce, Phase: phase}
			for _, route := range c.Routes {
				a := isolationAttempt{Name: route.Name, Protocol: route.Protocol, Address: route.Address, Nonce: c.Nonce + "-" + phase + "-" + route.Name, Attempted: true, Error: "network is unreachable"}
				if control {
					a.Error = ""
					a.Result = "control:" + a.Nonce
				}
				r.Attempts = append(r.Attempts, a)
			}
			reports = append(reports, r)
		}
		return reports
	}
	for _, control := range []bool{false, true} {
		t.Run(fmt.Sprint(control), func(t *testing.T) {
			if err := isolationRouteReports(makeReports(control), c, c.Nonce, control); err != nil {
				t.Fatal(err)
			}
			for _, change := range []func(*[]isolationReport){
				func(r *[]isolationReport) { *r = nil }, func(r *[]isolationReport) { *r = (*r)[:1] },
				func(r *[]isolationReport) { (*r)[1].Phase = "first" }, func(r *[]isolationReport) { (*r)[0].Attempts = nil },
				func(r *[]isolationReport) { (*r)[0].Attempts[1] = (*r)[0].Attempts[0] },
				func(r *[]isolationReport) { (*r)[0].Attempts[0].Attempted = false },
				func(r *[]isolationReport) { (*r)[0].Attempts[0].Address = "different" },
				func(r *[]isolationReport) { (*r)[0].Attempts[0].Nonce = "stale" },
				func(r *[]isolationReport) { (*r)[0].Attempts[0].Result = "unrelated answer" },
				func(r *[]isolationReport) { (*r)[0].Attempts[0].Error = ""; (*r)[0].Attempts[0].Result = "" },
			} {
				r := makeReports(control)
				change(&r)
				if isolationRouteReports(r, c, c.Nonce, control) == nil {
					t.Fatal("missing or contradicted route evidence passed")
				}
			}
		})
	}
}

func TestOfflineIsolationRejectsMissingOrStaleReports(t *testing.T) {
	s := &isolationWorld{runs: []*isolationRun{{config: isolationConfig{Nonce: "expected"}}}}
	for _, r := range [][]isolationReport{nil, {{Kind: "identity", Nonce: "stale"}}, {{Kind: "identity", Nonce: "expected"}, {Kind: "identity", Nonce: "expected"}}} {
		s.runs[0].reports = r
		if _, err := s.reports("identity", 1); err == nil {
			t.Fatal("absent, stale or duplicate identity report passed")
		}
	}
	for _, b := range [][]byte{nil, []byte("{}\ntruncated"), []byte(`{"Kind":"error","Error":"probe failed"}`)} {
		if _, err := isolationReports(b); err == nil {
			t.Fatal("missing, malformed or failed probe passed")
		}
	}
}

func TestOfflineIsolationIdentityOracleRejectsWrongRuntime(t *testing.T) {
	r := map[string]string{"uid": "1001", "gid": "1002", "child-uid": "1001", "child-gid": "1002", "Uid": "1001 1001 1001 1001", "Gid": "1002 1002 1002 1002", "child-Uid": "1001 1001 1001 1001", "child-Gid": "1002 1002 1002 1002", "Groups": "", "child-Groups": "1002", "child-error": "<nil>", "setuid-error": "operation not permitted", "after-setuid": "1001", "helper-error": "<nil>", "helper-uid": "1001", "mount-error": "operation not permitted", "pids.max": "4096"}
	for _, prefix := range []string{"", "child-"} {
		r[prefix+"NoNewPrivs"] = "1"
		for _, k := range []string{"CapEff", "CapPrm", "CapInh", "CapAmb", "CapBnd"} {
			r[prefix+k] = "0000000000000000"
		}
	}
	s := &isolationWorld{w: &world{approval: &approvalWorld{uid: "1001", gid: "1002"}}, groups: []int{1002, 1003}, setuidBefore: "0", setuidAfter: "0", privilegedBefore: true, privilegedAfter: true, runs: []*isolationRun{{config: isolationConfig{Nonce: "n"}, reports: []isolationReport{{Kind: "identity", Nonce: "n", Values: r}}}}}
	for _, check := range []string{"identity", "capabilities", "elevation", "pids"} {
		if err := s.identityCheck(check); err != nil {
			t.Fatal(err)
		}
	}
	for _, test := range []struct{ check, key, wrong string }{{"identity", "uid", "0"}, {"identity", "child-Groups", "1003"}, {"capabilities", "CapEff", "1"}, {"capabilities", "child-NoNewPrivs", "0"}, {"elevation", "helper-uid", "0"}, {"elevation", "mount-error", "<nil>"}, {"pids", "pids.max", "max"}} {
		old := r[test.key]
		r[test.key] = test.wrong
		if s.identityCheck(test.check) == nil {
			t.Errorf("wrong %s passed", test.key)
		}
		r[test.key] = old
	}
	for _, control := range []*bool{&s.privilegedBefore, &s.privilegedAfter} {
		*control = false
		if s.identityCheck("elevation") == nil {
			t.Error("missing root/mount control passed")
		}
		*control = true
	}
}

func TestOfflineIsolationRootRequiresWritableControlAndReadOnlyObservation(t *testing.T) {
	r := map[string]string{"root-options": "ro,relatime", "root-write": "permission denied"}
	s := &isolationWorld{rootBefore: true, rootAfter: true, runs: []*isolationRun{
		{config: isolationConfig{Nonce: "first"}, reports: []isolationReport{{Kind: "filesystem", Nonce: "first", Values: r}}},
		{config: isolationConfig{Nonce: "second"}, reports: []isolationReport{{Kind: "filesystem", Nonce: "second", Values: r}}},
	}}
	if err := s.filesystemCheck("root"); err != nil {
		t.Fatal(err)
	}
	for _, control := range []*bool{&s.rootBefore, &s.rootAfter} {
		*control = false
		if s.filesystemCheck("root") == nil {
			t.Error("missing writable-root control passed")
		}
		*control = true
	}
	r["root-options"] = "rw,relatime"
	if s.filesystemCheck("root") == nil {
		t.Error("permission denial alone proved a read-only root")
	}
	r["root-options"], r["root-write"] = "ro,relatime", "<nil>"
	if s.filesystemCheck("root") == nil {
		t.Error("successful image-root write passed")
	}
}

func TestOfflineIsolationCleanupRequiresObservedExitAndAbsence(t *testing.T) {
	id := strings.Repeat("a", 64)
	expected := map[string]any{"uid": "1001", "stage": "absent", "container_id": id}
	b, _ := json.Marshal(expected)
	r := &isolationRun{id: id, exit: 3, absent: true, expected: expected, record: sessionRecord{Bytes: string(b), UID: "1001", Mode: 0600}}
	r.requests = []offlineRequest{{Method: "POST", Path: "/containers/create", Status: 201}, {Method: "POST", Path: "/containers/" + id + "/attach", Status: 101}, {Method: "POST", Path: "/containers/" + id + "/start", Status: 204}, {Method: "POST", Path: "/containers/" + id + "/wait", Status: 200, Observation: map[string]any{"StatusCode": float64(3)}}, {Method: "DELETE", Path: "/containers/" + id, Status: 204}, {Method: "GET", Path: "/containers/" + id + "/json", Status: 404}}
	s := &isolationWorld{}
	if err := s.completed(r, 3); err != nil {
		t.Fatal(err)
	}
	for i := range r.requests {
		saved := r.requests[i]
		r.requests[i] = offlineRequest{}
		if s.completed(r, 3) == nil {
			t.Errorf("missing request %d passed", i)
		}
		r.requests[i] = saved
	}
	r.absent = false
	if s.completed(r, 3) == nil {
		t.Fatal("unconfirmed absence passed")
	}
	r.absent = true
	r.requests[3].Observation = map[string]any{"StatusCode": float64(0)}
	if s.completed(r, 3) == nil {
		t.Fatal("wrapper code was accepted as observed workload status")
	}
}

func TestOfflineIsolationRemovalFaultRequiresRealWaitForSameID(t *testing.T) {
	s := &isolationWorld{removeFault: true}
	a := &offlineAPI{w: &world{offline: &offlineRuntime{isolation: s}}}
	remove := offlineRequest{Method: "DELETE", Path: "/containers/owned"}
	if a.removalFault(remove) {
		t.Fatal("fault activated before a workload exit")
	}
	for _, wait := range []offlineRequest{
		{Method: "POST", Path: "/containers/foreign/wait", Status: 200, Observation: map[string]any{"StatusCode": float64(0)}},
		{Method: "POST", Path: "/containers/owned/wait", Status: 503, Observation: map[string]any{"StatusCode": float64(0)}},
		{Method: "POST", Path: "/containers/owned/wait", Status: 200},
	} {
		a.requests = []offlineRequest{wait}
		if a.removalFault(remove) {
			t.Fatal("unavailable or unrelated wait activated the fault")
		}
	}
	for _, status := range []int{0, 7, 75} {
		a.requests = []offlineRequest{{Method: "POST", Path: "/containers/owned/wait", Status: 200, Observation: map[string]any{"StatusCode": float64(status)}}}
		if !a.removalFault(remove) {
			t.Fatal("actual workload exit did not activate removal-only fault")
		}
	}
	if a.removalFault(offlineRequest{Method: "GET", Path: remove.Path}) {
		t.Fatal("fault affected inspection")
	}
	s.removeFault = false
	if a.removalFault(remove) {
		t.Fatal("fault affected the fault-free control")
	}
}

func TestOfflineIsolationRecorderRetainsOriginalIdentityAcrossRuns(t *testing.T) {
	w := newSessionHarnessWorld(t)
	id := strings.Repeat("a", 64)
	name, labels := "/itos-container-"+w.session.token, w.sessionLabels()
	a := &offlineAPI{w: w, identities: map[string]map[string]any{id: {"Name": name, "Labels": labels}}}
	w.session.token = strings.Repeat("b", 64)
	observed := map[string]any{"Id": id, "Name": name, "Image": w.doctor.imageID, "Config": map[string]any{"Labels": labels}}
	if err := a.identity(observed, id); err != nil {
		t.Fatal(err)
	}
	object(observed["Config"])["Labels"] = w.sessionLabels()
	if a.identity(observed, id) == nil {
		t.Fatal("a later run authorized mutation of the earlier identity")
	}
}
