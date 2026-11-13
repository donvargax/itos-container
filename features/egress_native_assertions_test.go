package features

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/cucumber/godog"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/client"
)

func (w *world) egressNativeAssertions(sc *godog.ScenarioContext) {
	last := func(check func(*egressNative, *egressNativeRun) error) func() error {
		return func() error {
			n, err := w.egressNativeWorld()
			if err != nil {
				return err
			}
			run, err := n.last()
			if err != nil {
				return err
			}
			if err = check(n, run); err != nil {
				return fmt.Errorf("%w\n%s", err, run.describe())
			}
			return nil
		}
	}
	controls := func(key string) func() error {
		return last(func(n *egressNative, _ *egressNativeRun) error { return n.endpoints[key].controlled() })
	}
	sc.Step(`^each canonical name completes TLS with the upstream's own certificate and exchanges its nonce$`, last(func(n *egressNative, run *egressNativeRun) error {
		return n.exchanged(run, 2*len(n.names))
	}))
	sc.Step(`^native observations show exactly the specified worker and proxy restrictions and two product-owned networks$`, last(func(n *egressNative, run *egressNativeRun) error { return n.boundary(run) }))
	sc.Step(`^the proxy argv contains only the frozen canonical list and its image is the inspected pinned content$`, last(func(n *egressNative, run *egressNativeRun) error {
		return n.frozenProxy(run.observed["attach"], n.original.names, n.original.fingerprint)
	}))
	sc.Step(`^the worker's loopback service works through the specified NO_PROXY values$`, last(func(_ *egressNative, run *egressNativeRun) error {
		results := run.results("loopback")
		if len(results) != 1 || !results[0].OK {
			return errors.New("the worker's own loopback service was not reached")
		}
		for _, key := range []string{"NO_PROXY", "no_proxy"} {
			if run.report.Env[key] != "localhost,127.0.0.1,::1" {
				return fmt.Errorf("worker %s is %q", key, run.report.Env[key])
			}
		}
		return nil
	}))
	sc.Step(`^the product returns the observed workload exit and confirms all its owned resources absent$`, last(func(_ *egressNative, run *egressNativeRun) error { return run.cleaned() }))
	sc.Step(`^every (?:surrounding canonical control completes its TLS nonce exchange|valid control verifies the upstream certificate and exchanges its nonce)$`, last(func(n *egressNative, run *egressNativeRun) error {
		return n.exchanged(run, len(n.rows)+1)
	}))
	sc.Step(`^every raw request receives 403 with an empty body and closure without an upstream dial for that request$`, last(func(n *egressNative, run *egressNativeRun) error {
		results := run.results("raw")
		if len(results) != len(n.rows) {
			return fmt.Errorf("%d of %d raw requests were answered", len(results), len(n.rows))
		}
		for i, r := range results {
			if r.Method != n.rows[i]["method"] || r.Target != n.rows[i]["target"] || !r.OK || r.Status != 403 || r.Body != 0 || !r.Closed {
				return fmt.Errorf("%s %s was answered %d with %d body bytes (closed %v, %s)", r.Method, r.Target, r.Status, r.Body, r.Closed, r.Error)
			}
		}
		return n.endpoints["shared"].onlyExchanges(false)
	}))
	sc.Step(`^every invalid input closes after 200 with zero upstream bytes and no proxy-generated TLS bytes returned$`, last(func(n *egressNative, run *egressNativeRun) error {
		results := run.results("hello")
		if len(results) != len(n.rows) {
			return fmt.Errorf("%d of %d inputs were sent", len(results), len(n.rows))
		}
		for i, r := range results {
			if r.Input != n.rows[i]["input"] || !r.OK || r.Status != 200 || !r.Closed || r.Received != 0 {
				return fmt.Errorf("%q: tunnel status %d, closed %v, %d bytes returned (%s)", r.Input, r.Status, r.Closed, r.Received, r.Error)
			}
		}
		// A refused hello may have opened the upstream connection, never used it.
		return n.endpoints["recorded"].onlyExchanges(true)
	}))
	sc.Step(`^the worker completes every table row and the product confirms owned cleanup$`, last(func(n *egressNative, run *egressNativeRun) error {
		if got := len(run.results("raw")) + len(run.results("hello")); got != len(n.rows) || len(n.rows) == 0 {
			return fmt.Errorf("the worker completed %d of %d rows", got, len(n.rows))
		}
		return run.cleaned()
	}))
	sc.Step(`^public\.example completes the verified TLS nonce exchange$`, last(func(n *egressNative, run *egressNativeRun) error { return n.exchanged(run, 1) }))
	sc.Step(`^the private endpoint's independent control succeeds before and after the worker attempt$`, controls("private.example"))
	sc.Step(`^the unlisted endpoint's independent control succeeds before and after the worker attempt$`, controls("unlisted.example"))
	sc.Step(`^private\.example receives 403 with no connection to its private endpoint$`, last(func(n *egressNative, run *egressNativeRun) error {
		results := run.results("raw")
		if len(results) != 1 || results[0].Status != 403 || results[0].Body != 0 {
			return errors.New("the privately resolving name was not refused with an empty 403")
		}
		return n.endpoints["private.example"].onlyControls()
	}))
	sc.Step(`^the product confirms owned cleanup without changing the fixture endpoints$`, last(func(n *egressNative, run *egressNativeRun) error {
		return errors.Join(run.cleaned(), n.endpointsIntact())
	}))
	sc.Step(`^the first verified TLS request receives the expected redirect$`, last(func(n *egressNative, run *egressNativeRun) error {
		download := n.endpoints["download.example"]
		results := run.results("redirect")
		if len(results) != 1 || results[0].Status != 302 || results[0].Location != download.redirect || results[0].Peer != download.leaf {
			return errors.New("the allowed endpoint's redirect was not received over its own verified certificate")
		}
		receipts, err := download.receipts()
		if err != nil {
			return err
		}
		if !slices.ContainsFunc(receipts, func(r egressReceipt) bool {
			return r.Kind == "redirect" && r.Name == "download.example" && r.Nonce == "worker-"+n.nonce
		}) {
			return errors.New("the allowed endpoint holds no receipt of the worker's request")
		}
		return nil
	}))
	sc.Step(`^the redirected CONNECT receives 403 without an upstream connection to the unlisted endpoint$`, last(func(n *egressNative, run *egressNativeRun) error {
		results := run.results("redirect")
		if len(results) != 1 || results[0].OK || !strings.Contains(results[0].Error, "Forbidden") {
			return errors.New("following the redirect was not refused by the proxy")
		}
		return n.endpoints["unlisted.example"].onlyControls()
	}))
	sc.Step(`^the frozen allowlist stays unchanged and the product confirms owned cleanup$`, last(func(n *egressNative, run *egressNativeRun) error {
		if err := n.frozenProxy(run.observed["attach"], n.original.names, n.original.fingerprint); err != nil {
			return err
		}
		return errors.Join(w.runValidateSnapshot(run.snapshot, "1"), run.cleaned())
	}))
	sc.Step(`^the original worker reaches first\.example in every phase and later\.example is refused in every phase$`, func() error {
		n, err := w.egressNativeWorld()
		if err != nil || len(n.runs) == 0 {
			return errors.Join(err, errors.New("the product was never run"))
		}
		run := n.runs[0]
		reached, refused := run.results("tls"), run.results("raw")
		if len(reached) != 3 || len(refused) != 3 {
			return fmt.Errorf("the worker attempted %d and %d of three phases\n%s", len(reached), len(refused), run.describe())
		}
		first := n.endpoints["first.example"]
		for i := range 3 {
			if !reached[i].OK || reached[i].Peer != first.leaf || reached[i].Reply != first.nonce || refused[i].Status != 403 {
				return fmt.Errorf("phase %d: first.example ok %v, later.example status %d\n%s", i+1, reached[i].OK, refused[i].Status, run.describe())
			}
		}
		return n.endpoints["later.example"].onlyControls()
	})
	sc.Step(`^the proxy argv original session fingerprint and protected snapshot remain unchanged$`, func() error {
		n, err := w.egressNativeWorld()
		if err != nil || len(n.runs) == 0 {
			return errors.Join(err, errors.New("the product was never run"))
		}
		run := n.runs[0]
		id := ""
		for _, phase := range []string{"attach", "edit", "approve"} {
			o := run.observed[phase]
			if err := n.frozenProxy(o, n.original.names, n.original.fingerprint); err != nil {
				return fmt.Errorf("%s: %w", phase, err)
			}
			if id != "" && o.proxy.ID != id {
				return fmt.Errorf("%s: the proxy was replaced", phase)
			}
			id = o.proxy.ID
		}
		if strings.Count(n.consent, "y/N") != 1 {
			return errors.New("the wider request was not reviewed at exactly one prompt")
		}
		now, err := os.ReadFile(n.original.snapshotPath)
		if err != nil || len(n.original.snapshot) == 0 || string(now) != string(n.original.snapshot) {
			return errors.New("the original session's protected snapshot changed")
		}
		return nil
	})
	sc.Step(`^the fresh worker reaches both names through a newly owned proxy using the new fingerprint$`, last(func(n *egressNative, run *egressNativeRun) error {
		if len(n.runs) != 2 {
			return errors.New("no fresh run followed the original")
		}
		if err := n.exchanged(run, 2); err != nil {
			return err
		}
		if w.approval.fingerprint == n.original.fingerprint {
			return errors.New("the wider approval has the original fingerprint")
		}
		if err := n.frozenProxy(run.observed["attach"], n.names, w.approval.fingerprint); err != nil {
			return err
		}
		if before := n.runs[0].observed["attach"]; before == nil || before.proxy == nil || before.proxy.ID == run.observed["attach"].proxy.ID {
			return errors.New("the fresh run did not own a new proxy")
		}
		return nil
	}))
	sc.Step(`^neither run requests consent and both confirm their owned cleanup$`, func() error {
		n, err := w.egressNativeWorld()
		if err != nil || len(n.runs) != 2 {
			return errors.Join(err, errors.New("two runs are required"))
		}
		for i, run := range n.runs {
			if strings.Contains(string(run.stdout.bytes())+string(run.stderr.bytes()), "y/N") {
				return fmt.Errorf("run %d asked for consent", i+1)
			}
			if err := run.cleaned(); err != nil {
				return fmt.Errorf("run %d: %w\n%s", i+1, err, run.describe())
			}
		}
		return nil
	})
}

// cleaned is the product's own account of its end: the workload's exit code,
// every planned step done, and nothing the project still owns on the engine.
func (run *egressNativeRun) cleaned() error {
	if run.err != nil {
		return run.err
	}
	if run.timedOut || run.exit != run.wantExit {
		return fmt.Errorf("the product returned %d, not the workload's %d", run.exit, run.wantExit)
	}
	if !run.report.Done {
		return errors.New("the worker did not finish its plan")
	}
	if len(run.remaining) != 0 {
		return fmt.Errorf("the product left %v", run.remaining)
	}
	return nil
}

// exchanged requires exactly want verified tunnels in this run: the upstream's
// own certificate, its nonce, and its receipt of the worker's nonce.
func (n *egressNative) exchanged(run *egressNativeRun, want int) error {
	results := run.results("tls")
	if len(results) != want || want == 0 {
		return fmt.Errorf("%d of %d TLS exchanges were attempted", len(results), want)
	}
	sent := map[string][]string{}
	for _, step := range run.steps {
		if step.Op == "tls" {
			sent[step.Name] = append(sent[step.Name], step.Nonce)
		}
	}
	for _, r := range results {
		e := n.endpointFor(r.Name)
		if e == nil {
			return fmt.Errorf("no fixture endpoint serves %s", r.Name)
		}
		if !r.OK || r.Status != 200 || r.Peer != e.leaf || r.Reply != e.nonce {
			return fmt.Errorf("%s (%s): status %d, verified %v, nonce %v: %s", r.Name, r.Mode, r.Status, r.Peer == e.leaf, r.Reply == e.nonce, r.Error)
		}
	}
	for name, nonces := range sent {
		receipts, err := n.endpointFor(name).receipts()
		if err != nil {
			return err
		}
		for _, nonce := range nonces {
			if !slices.ContainsFunc(receipts, func(r egressReceipt) bool { return r.Kind == "nonce" && r.Name == name && r.Nonce == nonce }) {
				return fmt.Errorf("%s holds no receipt of the worker's nonce", name)
			}
		}
	}
	return nil
}

func (n *egressNative) endpointFor(name string) *egressEndpoint {
	for _, key := range n.order {
		if slices.Contains(n.endpoints[key].names, name) {
			return n.endpoints[key]
		}
	}
	return nil
}

// controlled requires an independent exchange before and after the worker's
// attempt, each successful. An unavailable control fails; it never skips.
func (e *egressEndpoint) controlled() error {
	if len(e.controls) < 2 {
		return fmt.Errorf("%s has %d independent controls, not one before and one after", e.key, len(e.controls))
	}
	return errors.Join(e.controls...)
}

// onlyExchanges requires every connection the endpoint accepted to be a
// completed, receipted exchange. With unused permitted, a connection may also
// have been opened and closed without a single byte.
func (e *egressEndpoint) onlyExchanges(unused bool) error {
	receipts, err := e.receipts()
	if err != nil {
		return err
	}
	completed, exchanges := 0, 0
	for _, r := range receipts {
		switch {
		case r.Kind == "nonce" || r.Kind == "redirect":
			exchanges++
		case r.Kind == "conn" && r.Handshake:
			completed++
		case r.Kind == "conn" && r.Bytes == 0 && unused:
		default:
			return fmt.Errorf("%s accepted a connection carrying %d bytes without an exchange", e.key, r.Bytes)
		}
	}
	if completed != exchanges || exchanges == 0 {
		return fmt.Errorf("%s completed %d connections for %d exchanges", e.key, completed, exchanges)
	}
	return nil
}

// onlyControls requires that nothing but the harness's own controls reached it.
func (e *egressEndpoint) onlyControls() error {
	if err := e.controlled(); err != nil {
		return err
	}
	receipts, err := e.receipts()
	if err != nil {
		return err
	}
	connections := 0
	for _, r := range receipts {
		if r.Kind == "conn" {
			connections++
		} else if !slices.Contains(e.sent, r.Nonce) {
			return fmt.Errorf("%s received a request that no control made", e.key)
		}
	}
	if connections != len(e.sent) {
		return fmt.Errorf("%s accepted %d connections for %d controls", e.key, connections, len(e.sent))
	}
	return nil
}

func (n *egressNative) endpointsIntact() error {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	for _, key := range n.order {
		e := n.endpoints[key]
		inspected, err := n.cli.ContainerInspect(ctx, e.id, client.ContainerInspectOptions{})
		if err != nil || inspected.Container.State == nil || !inspected.Container.State.Running {
			return fmt.Errorf("the %s endpoint is no longer running", key)
		}
		if err = e.controlled(); err != nil {
			return err
		}
	}
	return nil
}

// frozenProxy holds one observation of the proxy to the names and fingerprint
// its session started with, and to the pinned image the harness inspected.
func (n *egressNative) frozenProxy(o *egressObserved, names []string, fingerprint string) error {
	if o == nil || o.err != nil || o.proxy == nil || o.worker == nil {
		return errors.New("the product's proxy and worker were not observed")
	}
	if !slices.Equal(o.proxy.Config.Cmd, append([]string{"/itos-container", "__egress-proxy", "--"}, names...)) || len(o.proxy.Config.Entrypoint) != 0 {
		return fmt.Errorf("proxy argv is %v", o.proxy.Config.Cmd)
	}
	if o.proxy.Image != n.proxy {
		return errors.New("the proxy does not run the inspected pinned image")
	}
	for _, labels := range []map[string]string{o.proxy.Config.Labels, o.worker.Config.Labels} {
		if labels["io.itos-container.fingerprint"] != fingerprint || labels["io.itos-container.project"] != n.w.approval.project {
			return errors.New("the session's resources carry another fingerprint or project")
		}
	}
	return nil
}

// boundary reads T-40's launch off the engine's own inspection.
func (n *egressNative) boundary(run *egressNativeRun) error {
	o := run.observed["attach"]
	if err := n.frozenProxy(o, n.original.names, n.original.fingerprint); err != nil {
		return err
	}
	internal, okInternal := o.networks["internal-network"]
	egress, okEgress := o.networks["egress-network"]
	if len(o.networks) != 2 || !okInternal || !okEgress {
		return fmt.Errorf("the product owns %d networks, not one internal and one egress", len(o.networks))
	}
	if !internal.Internal || internal.EnableIPv6 || internal.Options["com.docker.network.bridge.gateway_mode_ipv4"] != "isolated" || egress.Internal || egress.EnableIPv6 {
		return errors.New("the product's networks are not an isolated internal IPv4 bridge and a plain IPv4 bridge")
	}
	for _, c := range internal.IPAM.Config {
		if c.Gateway.IsValid() {
			return errors.New("the internal network has a gateway")
		}
	}
	// Observed before the harness attached the proxy anywhere else.
	if len(internal.Containers) != 2 || len(egress.Containers) != 1 || len(o.proxy.NetworkSettings.Networks) != 2 || len(o.worker.NetworkSettings.Networks) != 1 {
		return errors.New("the proxy is not on both networks alone, or the worker not on the internal one alone")
	}
	if _, joined := egress.Containers[o.worker.ID]; joined || o.worker.NetworkSettings.Networks[internal.Name] == nil {
		return errors.New("the worker has a network besides the internal one")
	}
	owner := n.w.approval.uid + ":" + n.w.approval.gid
	for role, c := range map[string]*struct {
		user string
		caps []string
		ro   bool
		opts []string
		pb   int
		pub  bool
	}{
		"proxy":  {o.proxy.Config.User, o.proxy.HostConfig.CapDrop, o.proxy.HostConfig.ReadonlyRootfs, o.proxy.HostConfig.SecurityOpt, len(o.proxy.HostConfig.PortBindings), o.proxy.HostConfig.PublishAllPorts},
		"worker": {o.worker.Config.User, o.worker.HostConfig.CapDrop, o.worker.HostConfig.ReadonlyRootfs, o.worker.HostConfig.SecurityOpt, len(o.worker.HostConfig.PortBindings), o.worker.HostConfig.PublishAllPorts},
	} {
		if c.user != owner || !slices.Equal(c.caps, []string{"ALL"}) || !c.ro || len(c.opts) != 1 || !strings.HasPrefix(c.opts[0], "no-new-privileges") || c.pb != 0 || c.pub {
			return fmt.Errorf("the %s lost a specified restriction", role)
		}
	}
	if published(o.proxy.NetworkSettings.Ports)+published(o.worker.NetworkSettings.Ports) != 0 {
		return errors.New("a product container publishes a port")
	}
	h := o.proxy.HostConfig
	if h.Sysctls["net.ipv4.ip_forward"] != "0" || h.RestartPolicy.Name != "no" || len(o.proxy.Mounts) != 1 {
		return errors.New("the proxy can forward, restart or reach another mount")
	}
	m := o.proxy.Mounts[0]
	if m.Type != mount.TypeBind || m.Destination != "/itos-container" || m.RW || strings.HasPrefix(m.Source, n.w.doctor.checkout) {
		return errors.New("the proxy's executable is not one read-only bind from outside the checkout")
	}
	address := o.proxy.NetworkSettings.Networks[internal.Name].IPAddress
	if !address.Is4() {
		return errors.New("the proxy has no internal IPv4 address")
	}
	want := "http://" + address.String() + ":3128"
	for _, key := range []string{"HTTP_PROXY", "HTTPS_PROXY", "http_proxy", "https_proxy"} {
		if run.report.Env[key] != want || !slices.Contains(o.worker.Config.Env, key+"="+want) {
			return fmt.Errorf("worker %s does not name the proxy's internal address", key)
		}
	}
	if len(run.attached) != len(n.networks) {
		return errors.New("the harness could not attach the proxy to its fixture endpoints")
	}
	return nil
}

// published counts the ports with at least one host binding.
func published[K comparable, V any](ports map[K][]V) int {
	count := 0
	for _, bindings := range ports {
		if len(bindings) != 0 {
			count++
		}
	}
	return count
}
