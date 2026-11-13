package features

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/moby/moby/client"
)

func (w *world) egressIsolationAssertions(sc lifecycleStepRegistrar) {
	check := func(body func(*egressNative, *egressIsolation, *egressNativeRun) error) func() error {
		return func() error {
			n, err := w.egressNativeWorld()
			if err != nil {
				return err
			}
			if n.isolation == nil {
				return errors.New("missing route or resolver control")
			}
			run, err := n.last()
			if err != nil {
				return err
			}
			if err = body(n, n.isolation, run); err != nil {
				return fmt.Errorf("%w\n%s", err, run.describe())
			}
			return nil
		}
	}
	executed := check(func(n *egressNative, iso *egressIsolation, run *egressNativeRun) error {
		if err := iso.phased(run); err != nil {
			return err
		}
		return n.exchanged(run, 1)
	})
	sc.Step(`^the worker has executed both probe phases and a canonical TLS control succeeds through the proxy$`, executed)
	sc.Step(`^the worker has executed both DNS phases and its allowed proxy-resolved TLS control succeeds$`, executed)
	controlled := check(func(n *egressNative, iso *egressIsolation, _ *egressNativeRun) error { return iso.controlled(n) })
	sc.Step(`^every route's independent before and after controls exchange their distinct nonces$`, controlled)
	sc.Step(`^each DNS control returns its nonce-bearing answer before and after the worker attempt$`, controlled)
	denied := check(func(n *egressNative, iso *egressIsolation, run *egressNativeRun) error { return iso.denied(n, run) })
	sc.Step(`^every worker bypass attempt is denied with no matching destination receipt$`, denied)
	sc.Step(`^no worker probe obtains an outside answer or delivers its query to a controlled outside resolver$`, denied)
	sc.Step(`^native inspection shows no worker default route outside its isolated network and proxy IPv4 forwarding disabled$`, check(func(_ *egressNative, _ *egressIsolation, run *egressNativeRun) error {
		return egressNoRouteOut(run)
	}))
	sc.Step(`^the product confirms owned cleanup without changing the unrelated control resources$`, check(func(n *egressNative, iso *egressIsolation, run *egressNativeRun) error {
		return errors.Join(run.cleaned(), iso.intact(n))
	}))
	sc.Step(`^the product confirms owned cleanup without changing host DNS or unrelated controls$`, check(func(n *egressNative, iso *egressIsolation, run *egressNativeRun) error {
		if iso.resolvBefore == "" || iso.resolvAfter != iso.resolvBefore {
			return errors.New("the host's resolver file changed")
		}
		return errors.Join(run.cleaned(), iso.intact(n))
	}))
}

// workerReports are slice-14's framed reports as the worker's two route
// clients wrote them, in plan order.
func (iso *egressIsolation) workerReports(run *egressNativeRun) ([]isolationReport, error) {
	results := run.results("isolation")
	if len(results) != 2 {
		return nil, fmt.Errorf("the worker ran %d of its two probe phases", len(results))
	}
	var reports []isolationReport
	for _, r := range results {
		if !r.OK {
			return nil, fmt.Errorf("the %s phase's route client failed: %s", r.Name, r.Error)
		}
		phase, err := isolationReports([]byte(r.Reply))
		if err != nil {
			return nil, fmt.Errorf("the %s phase: %w", r.Name, err)
		}
		reports = append(reports, phase...)
	}
	return reports, nil
}

// phased requires the first phase before the worker's barrier and the running
// phase after it, around a worker that finished its whole plan.
func (iso *egressIsolation) phased(run *egressNativeRun) error {
	if run.err != nil {
		return run.err
	}
	if !run.reached["attach"] || !run.report.Done || len(run.report.Results) != len(run.steps) {
		return errors.New("the worker did not pass its barrier and finish its plan")
	}
	reports, err := iso.workerReports(run)
	if err != nil {
		return err
	}
	if len(reports) != 2 || reports[0].Phase != "first" || reports[1].Phase != "running" {
		return errors.New("the worker did not report a first-instruction and a running phase")
	}
	position := func(op, name string) int {
		return slices.IndexFunc(run.report.Results, func(r egressProbeResult) bool { return r.Op == op && r.Name == name })
	}
	first, barrier, running := position("isolation", "first"), position("barrier", "attach"), position("isolation", "running")
	if first < 0 || first > barrier || barrier > running {
		return errors.New("the probe phases do not surround the worker's barrier")
	}
	return nil
}

// receipts are everything a control or host listener recorded receiving.
func (iso *egressIsolation) receipts() ([]string, error) {
	s := iso.s
	logs, err := s.controlLogs(s.control)
	if err != nil {
		return nil, err
	}
	var all []string
	for _, r := range logs {
		if r.Kind == "receipt" {
			all = append(all, r.Nonce)
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return append(all, s.receipts...), nil
}

// controlled requires every independent control, before and after the worker's
// attempts, each with its own nonce and its listener's or resolver's receipt.
// An unavailable control fails here; it never skips.
func (iso *egressIsolation) controlled(n *egressNative) error {
	s := iso.s
	if iso.controlErr != nil {
		return iso.controlErr
	}
	if err := isolationRouteReports(iso.before, s.config, s.config.Nonce+"b", true); err != nil {
		return fmt.Errorf("before: %w", err)
	}
	if err := isolationRouteReports(iso.after, s.config, s.config.Nonce+"a", true); err != nil {
		return fmt.Errorf("after: %w", err)
	}
	if err := s.routeReceipts(true); err != nil {
		return err
	}
	received, err := iso.receipts()
	if err != nil {
		return err
	}
	for suffix, report := range map[string]egressProbeReport{"b": iso.planBefore, "a": iso.planAfter} {
		steps := append(iso.ownSteps("first", suffix), iso.ownSteps("running", suffix)...)
		if len(report.Results) != len(steps) {
			return errors.New("the probe's own operations have no complete control")
		}
		for i, step := range steps {
			r := report.Results[i]
			want := "control:" + step.Nonce
			if step.Op == "lookup" {
				want = egressControl4
				if step.Input == "ip6" {
					want = egressControl6
				}
			}
			if r.Op != step.Op || !r.OK || r.Reply != want {
				return fmt.Errorf("control %s %s%s answered %q, not %q (%s)", step.Op, step.Name, step.Nonce, r.Reply, want, r.Error)
			}
			if step.Op == "datagram" && !slices.ContainsFunc(received, func(receipt string) bool { return strings.Contains(receipt, step.Nonce) }) {
				return fmt.Errorf("no listener receipt for control %s", step.Nonce)
			}
		}
	}
	for _, key := range n.order {
		if err := n.endpoints[key].controlled(); err != nil {
			return err
		}
	}
	return nil
}

// denied requires every worker attempt, in both phases, to have been made and
// to have reached nothing: no answer, no receipt at any listener or resolver,
// and no connection at a TLS endpoint beyond the verified exchanges.
func (iso *egressIsolation) denied(n *egressNative, run *egressNativeRun) error {
	s := iso.s
	reports, err := iso.workerReports(run)
	if err != nil {
		return err
	}
	if err = egressDenied(reports, s.config, s.config.Nonce); err != nil {
		return err
	}
	if err = s.routeReceipts(false); err != nil {
		return err
	}
	want := map[string]int{"lookup": 4, "raw": 2}
	if s.mode == "network" {
		want = map[string]int{"datagram": 2, "direct": 2, "forward": 6, "raw": 4}
	}
	var tokens []string
	for op, count := range want {
		results := run.results(op)
		if len(results) != count {
			return fmt.Errorf("the worker made %d of %d %s attempts", len(results), count, op)
		}
		for _, r := range results {
			if op == "raw" {
				if !r.OK || r.Status != 403 || r.Body != 0 || !r.Closed {
					return fmt.Errorf("%s %s was answered %d with %d body bytes (%s)", r.Method, r.Target, r.Status, r.Body, r.Error)
				}
				continue
			}
			if r.OK || r.Error == "" || r.Reply != "" || r.Peer != "" {
				return fmt.Errorf("the worker's %s attempt %s%s was answered %q", op, r.Name, r.Target, r.Reply)
			}
		}
	}
	for _, step := range run.steps {
		switch step.Op {
		case "datagram", "direct", "forward":
			tokens = append(tokens, step.Nonce)
		case "raw":
			if _, token, found := strings.Cut(step.Target, ":32114/"); found {
				tokens = append(tokens, token)
			}
		}
	}
	received, err := iso.receipts()
	if err != nil {
		return err
	}
	for _, token := range tokens {
		if slices.ContainsFunc(received, func(receipt string) bool { return strings.Contains(receipt, token) }) {
			return fmt.Errorf("a listener received the worker's %s", token)
		}
	}
	allowed := n.endpoints["api.example.com"]
	seen, err := allowed.receipts()
	if err != nil {
		return err
	}
	if slices.ContainsFunc(seen, func(r egressReceipt) bool { return slices.Contains(tokens, r.Nonce) }) {
		return errors.New("the allowed endpoint received a request the worker sent around the proxy")
	}
	if err = allowed.onlyExchanges(false); err != nil {
		return err
	}
	if unlisted := n.endpoints[egressUnlistedDNS]; unlisted != nil {
		return unlisted.onlyControls()
	}
	return nil
}

// egressDenied holds the worker's reports to the planned inventory: both
// phases once, every route attempted with its own nonce at its planned
// address, and no answer to any. A resolver the worker may reach, Docker's own,
// can refuse a name without a transport error; every other route must fail.
func egressDenied(reports []isolationReport, c isolationConfig, nonce string) error {
	if len(c.Routes) == 0 {
		return errors.New("empty planned route inventory")
	}
	phases := map[string]bool{}
	for _, r := range reports {
		if r.Kind != c.Mode || r.Nonce != nonce || (r.Phase != "first" && r.Phase != "running") || phases[r.Phase] {
			return errors.New("missing, duplicate or foreign probe phase")
		}
		phases[r.Phase] = true
		if c.Mode == "dns" {
			var targets []string
			for _, line := range strings.Split(r.Values["resolv.conf"], "\n") {
				if f := strings.Fields(line); len(f) >= 2 && f[0] == "nameserver" {
					targets = append(targets, f[1])
				}
			}
			if !reflect.DeepEqual(targets, []string{"127.0.0.11"}) {
				return fmt.Errorf("the worker's system resolver targets are %v, not Docker's embedded resolver alone", targets)
			}
		}
		if len(r.Attempts) != len(c.Routes) {
			return errors.New("incomplete route attempt inventory")
		}
		for i, want := range c.Routes {
			a := r.Attempts[i]
			if a.Name != want.Name || !a.Attempted || a.Protocol != want.Protocol || a.Address != want.Address || a.Nonce != nonce+"-"+r.Phase+"-"+want.Name {
				return fmt.Errorf("route %s was not attempted with its planned endpoint and nonce", want.Name)
			}
			if a.Result != "" {
				return fmt.Errorf("worker route %s/%s was answered %q", r.Phase, want.Name, a.Result)
			}
			if a.Error == "" && want.Protocol != "docker" {
				return fmt.Errorf("worker route %s/%s supplied no operation outcome", r.Phase, want.Name)
			}
		}
	}
	if len(phases) != 2 {
		return errors.New("both execution phases must be reported")
	}
	return nil
}

// egressDefaultRoute reads a kernel IPv4 routing table for a default route.
func egressDefaultRoute(table string) (bool, error) {
	lines := strings.Split(strings.TrimSpace(table), "\n")
	if len(lines) < 2 || !strings.HasPrefix(lines[0], "Iface") {
		return false, errors.New("no routing table with at least its connected route")
	}
	for _, line := range lines[1:] {
		f := strings.Fields(line)
		if len(f) < 8 {
			return false, errors.New("malformed routing table row")
		}
		if f[1] == "00000000" && f[7] == "00000000" {
			return true, nil
		}
	}
	return false, nil
}

// egressNoRouteOut reads the engine's own inspection, taken at the worker's
// barrier before the harness attached anything, and the worker's own table.
func egressNoRouteOut(run *egressNativeRun) error {
	o := run.observed["attach"]
	if o == nil || o.err != nil || o.proxy == nil || o.worker == nil || o.proxy.HostConfig == nil || o.worker.NetworkSettings == nil {
		return errors.New("the product's proxy and worker were not observed")
	}
	internal, ok := o.networks["internal-network"]
	if !ok || !internal.Internal || internal.EnableIPv6 || internal.Options["com.docker.network.bridge.gateway_mode_ipv4"] != "isolated" {
		return errors.New("the worker's network is not an isolated internal IPv4 bridge")
	}
	for _, c := range internal.IPAM.Config {
		if c.Gateway.IsValid() {
			return errors.New("the internal network has a gateway")
		}
	}
	endpoint := o.worker.NetworkSettings.Networks[internal.Name]
	if len(o.worker.NetworkSettings.Networks) != 1 || endpoint == nil {
		return errors.New("the worker is not on the internal network alone")
	}
	for _, gateway := range []interface {
		IsValid() bool
		IsUnspecified() bool
	}{endpoint.Gateway, endpoint.IPv6Gateway} {
		if gateway.IsValid() && !gateway.IsUnspecified() {
			return errors.New("the worker's network endpoint has a gateway")
		}
	}
	if o.proxy.HostConfig.Sysctls["net.ipv4.ip_forward"] != "0" {
		return errors.New("the proxy's IPv4 forwarding is not disabled")
	}
	tables := run.results("routes")
	if len(tables) != 1 || !tables[0].OK {
		return errors.New("the worker did not report its routing table")
	}
	routed, err := egressDefaultRoute(tables[0].Reply)
	if err != nil {
		return err
	}
	if routed {
		return errors.New("the worker has a default route")
	}
	return nil
}

// intact requires the control, its network and every endpoint to be as the
// fixture made them, with nothing of the product's left on the network.
func (iso *egressIsolation) intact(n *egressNative) error {
	s := iso.s
	if iso.controlErr != nil {
		return iso.controlErr
	}
	observed, err := s.controlInspect(s.control)
	if err != nil {
		return err
	}
	if observed.State == nil || !observed.State.Running {
		return errors.New("the route and resolver control is no longer running")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	network, err := s.cli.NetworkInspect(ctx, s.networkID, client.NetworkInspectOptions{})
	if err != nil {
		return err
	}
	if network.Network.Name != s.networkName || !reflect.DeepEqual(network.Network.Labels, s.networkLabels) || len(network.Network.Containers) != 1 {
		return errors.New("the control network changed or still carries a product container")
	}
	if _, only := network.Network.Containers[s.control.id]; !only {
		return errors.New("the control network no longer carries its control alone")
	}
	return n.endpointsIntact()
}
