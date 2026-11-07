package features

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
)

func (w *world) lossFreshBoundary() error {
	l, s := w.loss, w.offline.isolation
	if l.err != nil {
		return l.err
	}
	if len(l.rounds) != 3 || l.dead.IsZero() || len(w.recovery.stops) != 1 {
		return errors.New("missing baseline, post-loss or pre-stop round")
	}
	first := l.rounds[0]
	wantSnapshot := map[string]any{"schema": 1, "project": w.approval.project, "generation": "1", "fingerprint": w.approval.fingerprint, "settings": w.approval.settings}
	if !recoveryJSONEqual(w.recovery.snapshot.Bytes, wantSnapshot) {
		return errors.New("frozen snapshot differs from the independently approved settings")
	}
	for i, r := range l.rounds {
		if r.err != nil {
			return r.err
		}
		if err := lossReportInventory(r.reports, r.nonce, r.start); err != nil {
			return err
		}
		if r.pid != first.pid || r.start != first.start || r.snapshot != w.recovery.snapshot ||
			lossRecord(r.snapshot)["fingerprint"] != w.approval.fingerprint || r.released.IsZero() || !r.collected.After(r.released) {
			return errors.New("worker identity, frozen input or fresh challenge correlation changed")
		}
		if (i == 0 && !r.collected.Before(l.dead)) || (i > 0 && (!r.released.After(l.dead) || r.nonce == first.nonce)) ||
			(i == 1 && !r.collected.Before(w.recovery.status.started)) ||
			(i == 2 && (!r.released.After(w.recovery.status.finished) || !r.collected.Before(w.recovery.stops[0].started) || r.nonce == l.rounds[1].nonce)) {
			return errors.New("rounds do not bracket confirmed loss, fresh status and explicit stop")
		}
		if err := isolationNetworkBoundary(r.boundary); err != nil {
			return err
		}
		for _, key := range []string{"Id", "Image", "Config", "HostConfig", "Mounts"} {
			if !reflect.DeepEqual(r.boundary[key], first.boundary[key]) {
				return fmt.Errorf("native worker %s changed after controller loss", key)
			}
		}
		// Reuse the exact slice-14 mount oracle for every independent native
		// observation, as well as the recorder's actual pre-start observation.
		view := &isolationRun{id: w.recovery.worker, requests: []offlineRequest{{Method: "POST", Path: "/containers/" + w.recovery.worker + "/start", Observation: r.boundary}}}
		if err := s.mountBoundary(view); err != nil {
			return err
		}
	}
	a := w.offline.api
	a.mu.Lock()
	requests := append([]offlineRequest{}, a.requests...)
	a.mu.Unlock()
	if err := s.mountBoundary(&isolationRun{id: w.recovery.worker, requests: requests}); err != nil {
		return err
	}
	for _, r := range requests {
		if r.Method == "POST" && r.Path == "/containers/"+w.recovery.worker+"/start" {
			if err := isolationNetworkBoundary(r.Observation); err != nil {
				return err
			}
		}
	}
	return nil
}

func (w *world) lossNetwork() error {
	s := w.offline.isolation
	if err := w.lossFreshBoundary(); err != nil {
		return err
	}
	for _, r := range w.loss.rounds {
		for _, plan := range []*isolationConfig{s.config.Network, s.config.DNS} {
			before, after := r.before.network, r.after.network
			if plan.Mode == "dns" {
				before, after = r.before.dns, r.after.dns
			}
			if err := isolationRouteReports(before, *plan, r.nonce+"b", true); err != nil {
				return err
			}
			if err := isolationRouteReports(after, *plan, r.nonce+"a", true); err != nil {
				return err
			}
			if err := isolationRouteReports(r.reports, *plan, r.nonce, false); err != nil {
				return err
			}
			for _, report := range r.reports {
				if report.Kind != plan.Mode {
					continue
				}
				for _, attempt := range report.Attempts {
					// A broken TLS verifier or an external DNS answer is never
					// evidence of the offline transport boundary.
					lower := strings.ToLower(attempt.Error)
					if strings.Contains(lower, "certificate") || strings.Contains(lower, "no such host") || strings.Contains(lower, "tls:") {
						return errors.New("certificate failure or NXDOMAIN was counted as network enforcement")
					}
				}
			}
		}
		found := false
		for _, report := range r.reports {
			if report.Kind == "loopback" {
				found = report.Values["reply"] == "worker:"+r.nonce && report.Values["error"] == "<nil>"
			}
		}
		if !found {
			return errors.New("worker-local loopback control failed")
		}
	}
	// Read after confirmed product removal; listeners remained alive throughout
	// every round and stop. Late worker traffic cannot escape this final audit.
	if err := w.recoveryCompleted(); err != nil {
		return err
	}
	logs, err := s.controlLogs(s.control)
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, r := range logs {
		if r.Kind == "receipt" {
			seen[r.Nonce] = true
		}
	}
	s.mu.Lock()
	for _, nonce := range s.receipts {
		seen[nonce] = true
	}
	s.mu.Unlock()
	for _, round := range w.loss.rounds {
		if err := lossReceiptInventory(seen, round.nonce, s.config.Routes); err != nil {
			return err
		}
	}
	return nil
}

func lossReceiptInventory(seen map[string]bool, nonce string, routes []isolationRoute) error {
	if nonce == "" || len(routes) == 0 {
		return errors.New("missing listener inventory")
	}
	for _, route := range routes {
		for _, phase := range []string{"first", "running"} {
			suffix := "-" + phase + "-" + route.Name
			if seen[nonce+suffix] {
				return fmt.Errorf("listener received worker attempt %s", route.Name)
			}
			if !seen[nonce+"b"+suffix] || !seen[nonce+"a"+suffix] {
				return fmt.Errorf("listener missed a before/after control for %s", route.Name)
			}
		}
	}
	return nil
}

func (w *world) lossHost() error {
	s := w.offline.isolation
	if err := w.lossFreshBoundary(); err != nil {
		return err
	}
	var filesystems []*isolationRun
	for _, r := range w.loss.rounds {
		run := &isolationRun{config: w.lossConfig(r.nonce), reports: r.reports, err: r.err}
		view := &isolationWorld{w: w, config: run.config, runs: []*isolationRun{run}, groups: s.groups,
			hostBefore: r.before.host, hostAfter: r.after.host, setuidBefore: r.before.setuid, setuidAfter: r.after.setuid,
			privilegedBefore: r.before.privileged, privilegedAfter: r.after.privileged,
			controlError: errors.Join(r.before.err, r.after.err)}
		for _, check := range []string{"identity", "capabilities", "elevation", "pids"} {
			if err := view.identityCheck(check); err != nil {
				return err
			}
		}
		for _, check := range []string{"host-files", "host-sockets", "namespaces"} {
			if err := view.hostCheck(check); err != nil {
				return err
			}
		}
		if !r.before.root || !r.after.root {
			return errors.New("writable-root control failed around a loss round")
		}
		filesystems = append(filesystems, run)
	}
	// Slice-14's filesystem oracle compares two invocations. Here adjacent
	// fresh rounds reuse it, covering all three reports without duplicating one.
	for i := 1; i < len(filesystems); i++ {
		view := &isolationWorld{w: w, runs: filesystems[i-1 : i+1], rootBefore: true, rootAfter: true}
		for _, check := range []string{"root", "writes"} {
			if err := view.filesystemCheck(check); err != nil {
				return err
			}
		}
	}
	return nil
}

func (w *world) lossDurableStop() error {
	if w.loss.err != nil {
		return w.loss.err
	}
	if err := w.recoveryCompleted(); err != nil {
		return err
	}
	a := w.offline.api
	a.mu.Lock()
	entries := append([]offlineRequest{}, a.requests[w.loss.stopBegin:]...)
	a.mu.Unlock()
	if err := w.lifecycleRevalidated(entries); err != nil {
		return err
	}
	entry, err := w.recoveryEntry(w.recovery.after, "stopped", "absent", "complete")
	if err != nil {
		return err
	}
	if entry.WorkloadExit == nil {
		return nil
	}
	for _, e := range entries {
		if e.Path == "/containers/"+w.recovery.worker+"/wait" && e.Status == 200 && !e.Cancelled &&
			!e.Replied.IsZero() && e.Observation["StatusCode"] == float64(*entry.WorkloadExit) && *entry.WorkloadExit != 0 {
			return nil
		}
	}
	return errors.New("removal alone rewrote unknown workload history")
}

func (w *world) lossPreserved() error {
	if err := w.lossFreshBoundary(); err != nil {
		return err
	}
	current, err := w.lossProtected()
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(current, w.loss.before) {
		return errors.New("loss or recovery changed unrelated files, owners, modes or approval")
	}
	for _, round := range w.loss.rounds {
		path := filepath.Join(w.doctor.checkout, "slice-14-edit-"+round.nonce)
		b, err := os.ReadFile(path)
		if err != nil || string(b) != round.nonce {
			return errors.New("fresh worker checkout edit was lost")
		}
	}
	snapshot, err := sessionReadRecord(w.offline.isolation.config.Files["snapshot"])
	if err != nil || snapshot != w.recovery.snapshot {
		return errors.New("recovery changed the original frozen snapshot")
	}
	if _, err = w.offlineRegistryObjects(w.recovery.id); err != nil {
		return err
	}
	resources, err := w.doctor.real.snapshotResources()
	if err != nil {
		return err
	}
	s := w.offline.isolation
	before, err := lossResourceView(s.resourcesBefore, s.networkID, s.control.id, s.control.name)
	if err != nil {
		return err
	}
	after, err := lossResourceView(resources, s.networkID, s.control.id, s.control.name)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(after, before) {
		return errors.New("loss or stop changed unrelated native resources")
	}
	return nil
}

func lossResourceView(resources map[string]string, networkID, controlID, controlName string) (map[string]string, error) {
	result := maps.Clone(resources)
	var rows []string
	for _, line := range strings.Split(resources["/networks"], "\n") {
		var n map[string]any
		if err := json.Unmarshal([]byte(line), &n); err != nil {
			return nil, err
		}
		if n["Id"] == networkID {
			if endpoint, ok := object(n["Containers"])[controlID]; ok {
				e := object(endpoint)
				if e["Name"] != controlName || e["IPv4Address"] != "172.30.14.2/24" || e["IPv6Address"] != "fd14:14::2/64" {
					return nil, errors.New("nonce-alias reconnect changed the trusted control identity or addresses")
				}
				// Only these two generated fields belong to our recorded control
				// reconnect. Every other resource field is compared unchanged.
				delete(e, "EndpointID")
				delete(e, "MacAddress")
			}
		}
		rows = append(rows, string(approvalJSON(n)))
	}
	slices.Sort(rows)
	result["/networks"] = strings.Join(rows, "\n")
	return result, nil
}
