package features

import (
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

func (s *isolationWorld) reports(kind string, count int) ([]isolationReport, error) {
	if s.controlError != nil {
		return nil, fmt.Errorf("required positive control or observation failed: %w", s.controlError)
	}
	if len(s.runs) != count {
		return nil, fmt.Errorf("expected %d native invocations, observed %d", count, len(s.runs))
	}
	var result []isolationReport
	for _, run := range s.runs {
		if run.err != nil {
			return nil, fmt.Errorf("workload observation failed (controller exit %d): %w", run.exit, run.err)
		}
		found := 0
		for _, r := range run.reports {
			if r.Kind == kind {
				if r.Nonce != run.config.Nonce {
					return nil, errors.New("probe nonce differs from this invocation")
				}
				found++
				result = append(result, r)
			}
		}
		if found != 1 {
			return nil, fmt.Errorf("expected one complete %s report, got %d", kind, found)
		}
	}
	return result, nil
}

func (w *world) isolationCheck(check string) error {
	s := w.offline.isolation
	switch check {
	case "identity", "capabilities", "elevation", "pids":
		return s.identityCheck(check)
	case "root", "writes", "persistent", "mounts", "nested", "ephemeral":
		return s.filesystemCheck(check)
	case "host-files", "host-sockets", "namespaces":
		return s.hostCheck(check)
	case "route-controls", "route-denials", "receipts", "loopback", "network-inspection":
		return s.networkCheck(check)
	case "descendant":
		if _, err := s.reports("trailing", 1); err != nil {
			return err
		}
		r := s.runs[0]
		if r.descendantPID == 0 || r.descendantStart == "" || isolationProcessStart(r.descendantPID) == r.descendantStart {
			return errors.New("the independently observed descendant still runs or was never observed")
		}
		return nil
	case "native-absence":
		return s.completeAll()
	case "retained":
		return s.retained()
	case "safe-diagnostic":
		return s.safeDiagnostic()
	case "preserved":
		return s.preserved()
	case "complete":
		if err := s.completeAll(); err != nil {
			return err
		}
		return s.preserved()
	}
	return fmt.Errorf("unknown isolation assertion %q", check)
}

func (s *isolationWorld) identityCheck(check string) error {
	reports, err := s.reports("identity", 1)
	if err != nil {
		return err
	}
	r := reports[0].Values
	switch check {
	case "identity":
		for _, prefix := range []string{"", "child-"} {
			if r[prefix+"uid"] != s.w.approval.uid || r[prefix+"gid"] != s.w.approval.gid {
				return errors.New("process/child effective IDs differ from the real host IDs")
			}
			for key, want := range map[string]string{"Uid": s.w.approval.uid, "Gid": s.w.approval.gid} {
				f := strings.Fields(r[prefix+key])
				if len(f) != 4 {
					return errors.New("missing real/effective/saved/filesystem IDs")
				}
				for _, v := range f {
					if v != want {
						return errors.New("process/child identity is not the host non-root identity")
					}
				}
			}
			groups, present := r[prefix+"Groups"]
			if !present {
				return errors.New("missing supplementary group observation")
			}
			for _, g := range strings.Fields(groups) {
				if g != s.w.approval.gid {
					return errors.New("worker inherited a host supplementary group")
				}
			}
		}
		if r["child-error"] != "<nil>" || s.w.approval.uid == "0" || len(s.groups) == 0 {
			return errors.New("identity control did not execute under a non-root host with supplementary groups")
		}
	case "capabilities":
		for _, prefix := range []string{"", "child-"} {
			for _, key := range []string{"CapEff", "CapPrm", "CapInh", "CapAmb", "CapBnd"} {
				v := r[prefix+key]
				n, e := strconv.ParseUint(v, 16, 64)
				if e != nil || n != 0 {
					return fmt.Errorf("%s%s is missing or grants capabilities", prefix, key)
				}
			}
			if r[prefix+"NoNewPrivs"] != "1" {
				return errors.New("NoNewPrivs is not one")
			}
		}
	case "elevation":
		if s.setuidBefore != "0" || s.setuidAfter != "0" {
			return errors.New("same-file setuid controls did not elevate before and after")
		}
		if r["setuid-error"] == "" || r["setuid-error"] == "<nil>" || r["after-setuid"] != s.w.approval.uid || r["helper-error"] != "<nil>" || r["helper-uid"] != s.w.approval.uid || r["mount-error"] == "" || r["mount-error"] == "<nil>" {
			return errors.New("setuid or privileged mount attempt elevated or did not execute")
		}
	case "pids":
		if r["pids.max"] != "4096" {
			return errors.New("actual cgroup pids.max is not 4096")
		}
	}
	return nil
}

func (s *isolationWorld) filesystemCheck(check string) error {
	reports, err := s.reports("filesystem", 2)
	if err != nil {
		return err
	}
	for _, report := range reports {
		r := report.Values
		switch check {
		case "root":
			if !strings.Contains(","+r["root-options"]+",", ",ro,") || r["root-write"] == "" || r["root-write"] == "<nil>" {
				return errors.New("image write was not denied by an observed read-only root")
			}
		case "writes":
			if r["HOME"] != "/tmp" || r["TMPDIR"] != "/tmp" || r["tmp-type"] != "tmpfs" || r["tmp-mode"] != "1777" {
				return errors.New("HOME/TMPDIR do not use the observed tmpfs")
			}
			for _, flag := range []string{"rw", "nosuid", "nodev"} {
				if !strings.Contains(","+r["tmp-options"]+",", ","+flag+",") {
					return fmt.Errorf("tmpfs lacks %s", flag)
				}
			}
			for _, path := range []string{"checkout", "home", "temp"} {
				if r[path+"-write"] != "<nil>" || r[path+"-owner"] != s.w.approval.uid+":"+s.w.approval.gid {
					return fmt.Errorf("%s is not writable as the host UID/GID", path)
				}
			}
		case "nested":
			if !s.nestedBefore || !s.nestedAfter || r["nested-attempt"] != "true" || r["nested-read"] != "false" || r["nested-match"] != "false" {
				return errors.New("working nested host mount was visible or not probed")
			}
		}
	}
	switch check {
	case "persistent":
		r := reports[1].Values
		if r["previous-checkout-read"] != "true" || r["previous-checkout-match"] != "true" || r["previous-home-read"] != "false" || r["previous-temp-read"] != "false" {
			return errors.New("second run lost the checkout edit, retained temporary data, or omitted prior-file reads")
		}
	case "mounts":
		for _, run := range s.runs {
			if err := s.mountBoundary(run); err != nil {
				return err
			}
		}
	case "ephemeral":
		if err := s.completeAll(); err != nil {
			return err
		}
		return s.preserved()
	}
	return nil
}

func (s *isolationWorld) mountBoundary(run *isolationRun) error {
	found := false
	for _, r := range run.requests {
		if r.Method != "POST" || r.Path != "/containers/"+run.id+"/start" {
			continue
		}
		found = true
		c := r.Observation
		h := object(c["HostConfig"])
		if h["ReadonlyRootfs"] != true {
			return errors.New("native root is not read-only")
		}
		binds, temps := 0, 0
		mounts, _ := c["Mounts"].([]any)
		for _, raw := range mounts {
			m := object(raw)
			switch m["Type"] {
			case "bind":
				if m["Source"] != s.w.doctor.checkout || m["Destination"] != "/workspace" || m["RW"] != true {
					return errors.New("native worker has an unapproved host mount")
				}
				binds++
			case "tmpfs":
				if m["Destination"] != "/tmp" {
					return errors.New("unexpected requested temporary mount")
				}
				temps++
			default:
				return errors.New("native worker has a persistent volume or unapproved mount")
			}
		}
		// Docker reports HostConfig.Tmpfs separately on API versions that omit
		// those entries from Mounts; it is still independently inspected here.
		if temps == 0 && object(h["Tmpfs"])["/tmp"] != nil {
			temps = 1
		}
		if binds != 1 || temps != 1 {
			return errors.New("native mount inventory is incomplete")
		}
	}
	if !found {
		return errors.New("no independently inspected native mount boundary before start")
	}
	return nil
}

func (s *isolationWorld) hostCheck(check string) error {
	reports, err := s.reports("host", 1)
	if err != nil {
		return err
	}
	r := reports[0].Values
	if len(s.hostBefore) == 0 || !reflect.DeepEqual(s.hostBefore, s.hostAfter) {
		return errors.New("host positive controls are missing or changed")
	}
	if r["local-file"] != "true" || r["local-socket"] != "true" {
		return errors.New("worker-local file/socket operations did not succeed")
	}
	switch check {
	case "host-files":
		for name := range s.config.Files {
			if r["file-attempt-"+name] != "true" || r["file-read-"+name] != "false" {
				return fmt.Errorf("host file %s was accessible or unprobed", name)
			}
		}
	case "host-sockets":
		for name := range s.config.Sockets {
			if r["socket-attempt-"+name] != "true" || r["socket-connect-"+name] != "false" {
				return fmt.Errorf("host socket %s was reachable or unprobed", name)
			}
		}
		if r["file-read-home"] != "false" {
			return errors.New("host home was accessible")
		}
	case "namespaces":
		for _, ns := range []string{"pid", "ipc", "uts", "mnt", "net"} {
			if r["ns-"+ns] == "" || r["ns-"+ns] == s.hostBefore["ns-"+ns] {
				return fmt.Errorf("%s namespace is missing or shared with the host", ns)
			}
		}
		if r["sentinel-attempt"] != "true" || r["sentinel-match"] != "false" {
			return errors.New("host process sentinel is visible or unprobed")
		}
	}
	return nil
}

func isolationRouteReports(reports []isolationReport, c isolationConfig, nonce string, control bool) error {
	if len(c.Routes) == 0 {
		return errors.New("empty planned route inventory")
	}
	phases := map[string]bool{}
	for _, r := range reports {
		if r.Kind != c.Mode {
			continue
		}
		if r.Nonce != nonce || (r.Phase != "first" && r.Phase != "running") || phases[r.Phase] {
			return errors.New("missing, duplicate or foreign probe phase")
		}
		phases[r.Phase] = true
		if c.Mode == "dns" {
			var targets []string
			for _, line := range strings.Split(r.Values["resolv.conf"], "\n") {
				f := strings.Fields(line)
				if len(f) >= 2 && f[0] == "nameserver" {
					targets = append(targets, f[1])
				}
			}
			if len(targets) == 0 || !reflect.DeepEqual(targets, c.Resolvers) {
				return errors.New("system resolver targets differ from the independently controlled inventory")
			}
		}
		if len(r.Attempts) != len(c.Routes) {
			return errors.New("incomplete route attempt inventory")
		}
		seen := map[string]bool{}
		for _, want := range c.Routes {
			found := false
			for _, a := range r.Attempts {
				if a.Name != want.Name {
					continue
				}
				if seen[a.Name] {
					return errors.New("duplicate route report")
				}
				seen[a.Name] = true
				found = true
				if !a.Attempted || a.Protocol != want.Protocol || a.Address != want.Address || a.Nonce != nonce+"-"+r.Phase+"-"+want.Name {
					return errors.New("route was not attempted with the expected endpoint and nonce")
				}
				if control {
					expected := want.Answer
					if c.Mode == "network" {
						expected += ":" + a.Nonce
					}
					if a.Error != "" || a.Result != expected {
						return fmt.Errorf("positive route control %s/%s failed", r.Phase, want.Name)
					}
				} else if a.Result != "" || a.Error == "" {
					return fmt.Errorf("worker route %s/%s succeeded or supplied no operation outcome", r.Phase, want.Name)
				}
			}
			if !found {
				return fmt.Errorf("route %s has no attempt report", want.Name)
			}
		}
	}
	if len(phases) != 2 {
		return errors.New("both execution phases must be reported")
	}
	return nil
}

func (s *isolationWorld) networkCheck(check string) error {
	if s.controlError != nil {
		return s.controlError
	}
	if len(s.runs) != 1 || s.runs[0].err != nil {
		return errors.New("network/DNS probe did not execute completely")
	}
	r := s.runs[0]
	switch check {
	case "route-controls":
		if err := isolationRouteReports(s.controlsBefore, s.config, s.config.Nonce+"b", true); err != nil {
			return err
		}
		if err := isolationRouteReports(s.controlsAfter, s.config, s.config.Nonce+"a", true); err != nil {
			return err
		}
		return s.routeReceipts(true)
	case "route-denials":
		return isolationRouteReports(r.reports, s.config, s.config.Nonce, false)
	case "receipts":
		if err := isolationRouteReports(r.reports, s.config, s.config.Nonce, false); err != nil {
			return err
		}
		return s.routeReceipts(false)
	case "loopback":
		for _, v := range r.reports {
			if v.Kind == "loopback" && v.Nonce == s.config.Nonce && v.Values["reply"] == "worker:"+s.config.Nonce && v.Values["error"] == "<nil>" {
				return s.routeReceipts(false)
			}
		}
		return errors.New("worker did not reach its own loopback listener at the host port")
	case "network-inspection":
		if err := isolationNetworkBoundary(r.running); err != nil {
			return err
		}
		found := false
		for _, req := range r.requests {
			if req.Method == "POST" && req.Path == "/containers/"+r.id+"/start" {
				found = true
				if err := isolationNetworkBoundary(req.Observation); err != nil {
					return err
				}
			}
		}
		if !found {
			return errors.New("missing before-start native network observation")
		}
	}
	return nil
}

func isolationNetworkBoundary(c map[string]any) error {
	h, n := object(c["HostConfig"]), object(c["NetworkSettings"])
	if h["NetworkMode"] != "none" || h["PublishAllPorts"] == true || !offlineEmpty(h["PortBindings"]) || !offlineEmpty(n["Ports"]) {
		return errors.New("native worker has network access or published ports")
	}
	networks := object(n["Networks"])
	if len(networks) != 1 || networks["none"] == nil {
		return errors.New("native worker has absent or extra network attachments")
	}
	return nil
}

func (s *isolationWorld) routeReceipts(controls bool) error {
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
	for _, r := range s.receipts {
		seen[r] = true
	}
	s.mu.Unlock()
	for _, phase := range []string{"first", "running"} {
		for _, r := range s.config.Routes {
			if controls {
				for _, suffix := range []string{"b", "a"} {
					if !seen[s.config.Nonce+suffix+"-"+phase+"-"+r.Name] {
						return fmt.Errorf("no listener receipt for control %s/%s", phase, r.Name)
					}
				}
			} else if seen[s.config.Nonce+"-"+phase+"-"+r.Name] {
				return fmt.Errorf("listener received worker traffic on %s/%s", phase, r.Name)
			}
		}
	}
	return nil
}
