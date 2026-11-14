package features

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/cucumber/godog"
	"github.com/moby/moby/client"
)

// egressIsolation is the independent control of ID-EGRESS-15 and -16. It is
// slice-14's trusted listener and resolver control, unchanged: a container of
// its own on an internal fixture network, holding the private, metadata and
// loopback destinations in its own namespace, never as host routes. Controls
// run inside that namespace; the worker's attempts are made by the same client
// from inside the worker, started by the slice-27 probe.
type egressIsolation struct {
	s             *isolationWorld
	rows          []string
	before, after []isolationReport
	controlErr    error
	// The slice-27 probe's own operations, run inside the control's namespace
	// with nonces no worker is given.
	planBefore, planAfter egressProbeReport
	resolvBefore          string // the host's resolver file, which nothing here may change
	resolvAfter           string
}

// The routes and resolver paths each table row stands for. A row the harness
// has no attempt for is refused in When, never passed over.
var egressRouteRows = map[string][]string{
	"direct IPv4 TCP 443 to the allowed endpoint":                     nil, // the probe's own direct attempts
	"direct IPv6 TCP 443 to a controlled endpoint":                    {"endpoint6-tcp"},
	"direct TCP on another port":                                      {"literal4-tcp", "literal6-tcp"},
	"UDP including QUIC-shaped traffic":                               {"literal4-udp", "literal6-udp", "quic4-udp", "quic6-udp"},
	"the host gateway and a host service bound to all IPv4 addresses": {"gateway4-tcp", "gateway4-udp", "gateway6-tcp", "gateway6-udp", "host-all4-tcp", "host-all4-udp"},
	"private IPv4 and IPv6 destinations":                              {"private4-tcp", "private4-udp", "private6-tcp", "private6-udp"},
	"synthetic metadata IPv4 and IPv6 destinations":                   {"metadata4-tcp", "metadata4-udp", "metadata6-tcp", "metadata6-udp"},
	"host loopback and alternate loopback addresses":                  {"host-loopback4-tcp", "host-loopback4-udp", "host-loopback6-tcp", "host-loopback6-udp", "alternate-loopback4-tcp", "alternate-loopback4-udp"},
	"forwarding via the proxy's internal address without CONNECT":     nil, // the probe's forward and raw attempts
}

var egressResolverRows = map[string][]string{
	"the worker's system resolver for A and AAAA":             {"system-a", "system-aaaa"},
	"Docker 127.0.0.11 over UDP and TCP for an outside alias": {"docker-a", "docker-aaaa"}, // UDP; TCP is the probe's own lookup
	"controlled external DNS over UDP and TCP":                {"direct4-udp-a", "direct4-udp-aaaa", "direct4-tcp-a", "direct4-tcp-aaaa", "direct6-udp-a", "direct6-udp-aaaa", "direct6-tcp-a", "direct6-tcp-aaaa"},
	"alternate loopback resolvers":                            {"loopback-udp-a", "loopback-udp-aaaa", "loopback-tcp-a", "loopback-tcp-aaaa"},
	"direct HTTPS DNS":                                        {"https-a"},
	"CONNECT to an unlisted HTTPS DNS name":                   nil, // the probe's raw CONNECT
}

const (
	egressIsolationPlan = "/workspace/slice-27-isolation.json"
	egressUnlistedDNS   = "doh.example"
	egressControl4      = "172.30.14.2"
	egressControl6      = "fd14:14::2"
)

func (w *world) egressIsolationSteps(sc lifecycleStepRegistrar) {
	sc.Step(`^a CI egress checkout and matching protected approval with independent route controls$`, func() error { return w.egressIsolationFixture("network") })
	sc.Step(`^a CI egress checkout and matching protected approval with independent DNS controls$`, func() error { return w.egressIsolationFixture("dns") })
	sc.Step(`^its first-instruction and running probes ignore proxy variables and attempt these routes:$`, func(table *godog.Table) error {
		return w.egressIsolationProbes(table, egressRouteRows)
	})
	sc.Step(`^its first-instruction and running probes query unique outside names by these paths:$`, func(table *godog.Table) error {
		return w.egressIsolationProbes(table, egressResolverRows)
	})
	w.egressIsolationAssertions(sc)
}

// egressIsolationFixture provisions only trusted controls: the checkout and
// synthetic approval, the allowed TLS endpoint, and slice-14's control with
// its own network. It asks the product for nothing.
func (w *world) egressIsolationFixture(mode string) error {
	allowed := []string{"api.example.com"}
	if err := w.egressNativeCheckout(allowed, allowed); err != nil {
		return err
	}
	n := w.egress.native
	if _, err := n.endpoint("api.example.com", "public", allowed, ""); err != nil {
		return err
	}
	if mode == "dns" {
		if _, err := n.endpoint(egressUnlistedDNS, "public", []string{egressUnlistedDNS}, ""); err != nil {
			return err
		}
	}
	if err := w.buildIsolationProbe(); err != nil {
		return err
	}
	// Its own client: the control's close also closes it.
	cli, err := client.New(client.WithHost("unix:///var/run/docker.sock"))
	if err != nil {
		return err
	}
	s := &isolationWorld{w: w, cli: cli, mode: mode, config: isolationConfig{Nonce: isolationNonce(), Mode: mode, Files: map[string]string{}, Sockets: map[string]string{}}}
	iso := &egressIsolation{s: s}
	n.isolation = iso // before anything is created, so a partial control is still removed
	if mode == "network" {
		s.extraRoutes = iso.extraRoutes
	}
	resolv, err := os.ReadFile("/etc/resolv.conf")
	if err != nil {
		return err
	}
	iso.resolvBefore = string(resolv)
	if err = s.prepareNetwork(); err != nil {
		return err
	}
	probe, err := os.ReadFile(filepath.Join(w.doctor.checkout, "slice-27-probe"))
	if err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(s.controlDir, "slice-27-probe"), probe, 0700); err != nil {
		return err
	}
	// The worker's copy of the plan carries the control's public certificate
	// at most, never its key.
	if s.config.Key != "" || s.config.Cert != "" {
		return errors.New("the worker's route plan would carry control key material")
	}
	if err = os.WriteFile(filepath.Join(w.doctor.checkout, filepath.Base(egressIsolationPlan)), approvalJSON(s.config), 0600); err != nil {
		return err
	}
	n.controlAll()
	return nil
}

// extraRoutes adds what the egress boundary has to refuse beyond slice-14's
// inventory. Each is one more echo listener in the control's namespace, or on
// the host, with the same nonce exchange and receipt as the rest.
func (iso *egressIsolation) extraRoutes() error {
	s := iso.s
	// A host service bound to every IPv4 address, reached by the control
	// through the fixture bridge's own host address.
	port, err := s.hostEcho("0.0.0.0")
	if err != nil {
		return err
	}
	all := "172.30.14.1:" + strconv.Itoa(port)
	s.config.Routes = append(s.config.Routes,
		isolationRoute{Name: "host-all4-tcp", Protocol: "tcp4", Address: all, Answer: "host"},
		isolationRoute{Name: "host-all4-udp", Protocol: "udp4", Address: all, Answer: "host"},
		isolationRoute{Name: "endpoint6-tcp", Protocol: "tcp6", Address: "[" + egressControl6 + "]:443", Answer: "control"},
		isolationRoute{Name: "quic4-udp", Protocol: "udp4", Address: egressControl4 + ":443", Answer: "control"},
		isolationRoute{Name: "quic6-udp", Protocol: "udp6", Address: "[" + egressControl6 + "]:443", Answer: "control"},
		isolationRoute{Name: "alternate-loopback4-tcp", Protocol: "tcp4", Address: "127.0.0.2:32114", Answer: "control"},
		isolationRoute{Name: "alternate-loopback4-udp", Protocol: "udp4", Address: "127.0.0.2:32114", Answer: "control"},
	)
	return nil
}

// token is the nonce of one of the slice-27 probe's own attempts. The worker's
// carry no suffix; a control's carries "b" or "a", as slice-14's do.
func (iso *egressIsolation) token(kind, phase, suffix string) string {
	return kind + "-" + phase + "-" + iso.s.config.Nonce + suffix
}

// alias is a name Docker's embedded resolver answers on the control's network
// only: the alias slice-14 already gives its control for each nonce and phase.
func (iso *egressIsolation) alias(phase, suffix, kind string) string {
	return iso.s.config.Nonce + suffix + "-" + phase + "-docker-" + kind
}

// ownSteps are the slice-27 probe's own attempts for one phase: the routes and
// resolver paths slice-14's client has no operation for.
func (iso *egressIsolation) ownSteps(phase, suffix string) []egressStep {
	if iso.s.mode == "dns" {
		return []egressStep{
			{Op: "lookup", Mode: "docker-tcp", Target: "127.0.0.11:53", Input: "ip4", Name: iso.alias(phase, suffix, "a")},
			{Op: "lookup", Mode: "docker-tcp", Target: "127.0.0.11:53", Input: "ip6", Name: iso.alias(phase, suffix, "aaaa")},
		}
	}
	return []egressStep{{Op: "datagram", Method: "udp4", Target: egressControl4 + ":443", Nonce: iso.token("quic", phase, suffix)}}
}

// workerSteps are everything the worker attempts in one phase, ignoring its
// proxy variables or using the proxy as anything but a CONNECT proxy.
func (iso *egressIsolation) workerSteps(n *egressNative, phase string) []egressStep {
	steps := []egressStep{{Op: "isolation", Name: phase, Nonce: iso.s.config.Nonce, Target: egressIsolationPlan}}
	steps = append(steps, iso.ownSteps(phase, "")...)
	if iso.s.mode == "dns" {
		return append(steps, egressStep{Op: "raw", Method: "CONNECT", Target: egressUnlistedDNS + ":443"})
	}
	allowed := n.endpoints["api.example.com"]
	return append(steps,
		egressStep{Op: "direct", Method: "tcp4", Target: allowed.address.String() + ":443", Name: "api.example.com", Nonce: iso.token("direct", phase, "")},
		egressStep{Op: "forward", Method: "tcp4", Target: "32114", Nonce: iso.token("forward-tcp", phase, "")},
		egressStep{Op: "forward", Method: "udp4", Target: "32114", Nonce: iso.token("forward-udp", phase, "")},
		egressStep{Op: "forward", Method: "tcp4", Target: "443", Nonce: iso.token("forward-tls", phase, "")},
		egressStep{Op: "raw", Method: "GET", Target: "http://" + egressControl4 + ":32114/" + iso.token("forward-get", phase, "")},
		egressStep{Op: "raw", Method: "CONNECT", Target: egressControl4 + ":32114"},
	)
}

// controlPlan runs the slice-27 probe's own operations inside the control's
// namespace, as the unprivileged fixture user, and returns its report.
func (iso *egressIsolation) controlPlan(suffix string) (egressProbeReport, error) {
	s := iso.s
	var report egressProbeReport
	if s.control == nil {
		return report, errors.New("no control to run the probe's own operations in")
	}
	if _, err := s.controlInspect(s.control); err != nil {
		return report, err
	}
	steps := append(iso.ownSteps("first", suffix), iso.ownSteps("running", suffix)...)
	plan, result := filepath.Join(s.controlDir, "slice-27-plan.json"), filepath.Join(s.controlDir, "slice-27-report.json")
	_ = os.Remove(result)
	if err := os.WriteFile(plan, approvalJSON(map[string]any{"Steps": steps, "Exit": 0}), 0600); err != nil {
		return report, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	created, err := s.cli.ExecCreate(ctx, s.control.id, client.ExecCreateOptions{User: s.w.approval.uid + ":" + s.w.approval.gid, Cmd: []string{"/workspace/slice-27-probe", "/workspace/slice-27-plan.json"}, AttachStdout: true, AttachStderr: true})
	if err != nil {
		return report, err
	}
	attached, err := s.cli.ExecAttach(ctx, created.ID, client.ExecAttachOptions{})
	if err != nil {
		return report, err
	}
	defer attached.Close()
	_ = attached.Conn.SetDeadline(time.Now().Add(55 * time.Second))
	// The stream ends when the probe exits; its findings are in its report.
	if _, err = io.Copy(io.Discard, attached.Reader); err != nil {
		return report, err
	}
	observed, err := s.cli.ExecInspect(ctx, created.ID, client.ExecInspectOptions{})
	if err != nil {
		return report, err
	}
	if observed.Running || observed.ExitCode != 0 {
		return report, fmt.Errorf("the probe's own control operations exited %d", observed.ExitCode)
	}
	data, err := os.ReadFile(result)
	if err != nil {
		return report, err
	}
	if err = json.Unmarshal(data, &report); err != nil {
		return report, err
	}
	if !report.Done || len(report.Results) != len(steps) {
		return report, errors.New("the probe's own control operations did not all report")
	}
	return report, nil
}

// egressIsolationProbes makes every independent control, runs the product
// once with both probe phases around its barrier, and makes every control
// again. A refusal or an early product exit is left for Then.
func (w *world) egressIsolationProbes(table *godog.Table, known map[string][]string) error {
	n, err := w.egressNativeWorld()
	if err != nil {
		return err
	}
	iso := n.isolation
	if iso == nil {
		return errors.New("missing route or resolver control")
	}
	for _, row := range table.Rows[1:] {
		name := row.Cells[0].Value
		routes, ok := known[name]
		if !ok {
			return fmt.Errorf("no attempt is defined for %q", name)
		}
		for _, route := range routes {
			found := false
			for _, planned := range iso.s.config.Routes {
				found = found || planned.Name == route
			}
			if !found {
				return fmt.Errorf("%q: the control serves no route %s", name, route)
			}
		}
		iso.rows = append(iso.rows, name)
	}
	if len(iso.rows) != len(known) {
		return fmt.Errorf("%d of the %d defined rows were asked for", len(iso.rows), len(known))
	}
	// An unavailable control is kept for Then: it fails there, never skips.
	iso.before, iso.controlErr = iso.s.routeControls(false)
	iso.planBefore, err = iso.controlPlan("b")
	iso.controlErr = errors.Join(iso.controlErr, err)
	n.original = egressOriginal{fingerprint: w.approval.fingerprint, names: append([]string(nil), n.names...)}
	running := append([]egressStep{n.canonicalControl()}, iso.workerSteps(n, "running")...)
	if iso.s.mode == "network" {
		running = append(running, egressStep{Op: "routes"}, egressStep{Op: "loopback", Nonce: "loopback-" + egressNonce()})
	}
	if _, err = n.driveRun(n.newPhasedRun(iso.workerSteps(n, "first"), running, 0)); err != nil {
		return err
	}
	iso.after, err = iso.s.routeControls(true)
	iso.controlErr = errors.Join(iso.controlErr, err)
	iso.planAfter, err = iso.controlPlan("a")
	iso.controlErr = errors.Join(iso.controlErr, err)
	n.controlAll()
	resolv, err := os.ReadFile("/etc/resolv.conf")
	iso.controlErr = errors.Join(iso.controlErr, err)
	iso.resolvAfter = string(resolv)
	return nil
}
