package features

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

// egressOriginal is the first run's approval as the fixture wrote it, kept so a
// later, wider approval cannot stand in for it.
type egressOriginal struct {
	fingerprint, snapshotPath string
	snapshot                  []byte
	names                     []string
}

// The unlisted aliases of ID-EGRESS-13 resolve to the allowed endpoint's own
// address, so a proxy that matched by suffix or by address would reach it.
var egressUnlistedAliases = []string{"example.com", "api.example.com.other.example", "other.example", "xn--pi-6kc.example.com"}

func (w *world) egressNativeSteps(sc lifecycleStepRegistrar) {
	sc.Step(`^a CI egress checkout with policy spellings "([^"]*)" and matching protected approval$`, func(spellings string) error {
		if spellings != "API.Example.COM. BÜCHER.example" {
			return fmt.Errorf("no independent canonical form recorded for %q", spellings)
		}
		return w.egressNativeCheckout(strings.Fields(spellings), []string{"api.example.com", "xn--bcher-kva.example"})
	})
	sc.Step(`^a CI egress checkout allowing "([^"]*)" and matching protected approval$`, func(names string) error {
		return w.egressNativeCheckout(strings.Fields(names), strings.Fields(names))
	})
	sc.Step(`^independent TLS endpoints for the canonical names with nonce receipts$`, func() error {
		return w.egressEndpoints(func(n *egressNative) [][]string {
			var each [][]string
			for _, name := range n.names {
				each = append(each, []string{name, "public", name})
			}
			return each
		})
	})
	sc.Step(`^independent allowed and unlisted TLS aliases sharing a fixture address$`, func() error {
		return w.egressEndpoints(func(n *egressNative) [][]string {
			return [][]string{append([]string{"shared", "public"}, append(append([]string{}, n.names...), egressUnlistedAliases...)...)}
		})
	})
	sc.Step(`^an independent TLS endpoint recording every byte before handshake$`, func() error {
		return w.egressEndpoints(func(n *egressNative) [][]string {
			return [][]string{{"recorded", "public", n.names[0], "other.example"}}
		})
	})
	sc.Step(`^proxy-visible public-range and private-range fixture aliases with independent TLS controls$`, func() error {
		return w.egressEndpoints(func(*egressNative) [][]string {
			return [][]string{{"public.example", "public", "public.example"}, {"private.example", "private", "private.example"}}
		})
	})
	sc.Step(`^independent TLS endpoints for first\.example and later\.example$`, func() error {
		return w.egressEndpoints(func(*egressNative) [][]string {
			return [][]string{{"first.example", "public", "first.example"}, {"later.example", "public", "later.example"}}
		})
	})
	sc.Step(`^download\.example returns a TLS redirect to an unlisted independently reachable fixture endpoint$`, func() error {
		n, err := w.egressNativeWorld()
		if err != nil {
			return err
		}
		if _, err = n.endpoint("unlisted.example", "public", []string{"unlisted.example"}, ""); err != nil {
			return err
		}
		if _, err = n.endpoint("download.example", "public", []string{"download.example"}, "https://unlisted.example/nonce?n=redirected-"+n.nonce); err != nil {
			return err
		}
		n.controlAll()
		return nil
	})

	sc.Step(`^the product runs the egress probe through its proxy variables and explicit CONNECT connections$`, func() error {
		return w.egressNativeDrive(23, func(n *egressNative) []egressStep {
			var steps []egressStep
			for _, name := range n.names {
				steps = append(steps, egressStep{Op: "tls", Mode: "env", Name: name, Nonce: "env-" + egressNonce()}, egressStep{Op: "tls", Mode: "connect", Name: name, Nonce: "connect-" + egressNonce()})
			}
			return append(steps, egressStep{Op: "loopback", Nonce: "loopback-" + egressNonce()})
		})
	})
	sc.Step(`^the product's worker sends these raw proxy requests between successful canonical controls:$`, func(table *godog.Table) error {
		return w.egressNativeDrive(0, func(n *egressNative) []egressStep {
			shared := n.endpoints["shared"]
			steps := []egressStep{n.canonicalControl()}
			for _, row := range table.Rows[1:] {
				target := strings.NewReplacer("<endpoint-ipv4>", shared.address.String(), "<endpoint-ipv6>", shared.address6.String()).Replace(row.Cells[1].Value)
				n.rows = append(n.rows, map[string]string{"method": row.Cells[0].Value, "target": target})
				steps = append(steps, egressStep{Op: "raw", Method: row.Cells[0].Value, Target: target}, n.canonicalControl())
			}
			return steps
		})
	})
	sc.Step(`^the product's worker follows successful CONNECT responses with these inputs between valid TLS controls:$`, func(table *godog.Table) error {
		return w.egressNativeDrive(0, func(n *egressNative) []egressStep {
			steps := []egressStep{n.canonicalControl()}
			for _, row := range table.Rows[1:] {
				n.rows = append(n.rows, map[string]string{"input": row.Cells[0].Value})
				steps = append(steps, egressStep{Op: "hello", Name: n.names[0], Input: row.Cells[0].Value}, n.canonicalControl())
			}
			return steps
		})
	})
	sc.Step(`^the product's worker CONNECTs to each listed name on 443$`, func() error {
		return w.egressNativeDrive(0, func(*egressNative) []egressStep {
			return []egressStep{{Op: "tls", Mode: "connect", Name: "public.example", Nonce: "worker-" + egressNonce()}, {Op: "raw", Method: "CONNECT", Target: "private.example:443"}}
		})
	})
	sc.Step(`^the product's worker follows that redirect using the owned proxy$`, func() error {
		return w.egressNativeDrive(0, func(n *egressNative) []egressStep {
			return []egressStep{{Op: "redirect", URL: "https://download.example/redirect?n=worker-" + n.nonce}}
		})
	})
	sc.Step(`^the product starts the worker and it attempts both names before an edit after a wider request and after terminal approval of that request$`, w.egressWidenDuringRun)
	sc.Step(`^the original run exits and the product starts a fresh run with the newly approved list$`, func() error {
		n, err := w.egressNativeWorld()
		if err != nil {
			return err
		}
		first, err := n.last()
		if err != nil {
			return err
		}
		first.finish(3 * time.Minute)
		_, err = n.drive([]egressStep{
			{Op: "tls", Mode: "connect", Name: "first.example", Nonce: "fresh-" + egressNonce()},
			{Op: "tls", Mode: "connect", Name: "later.example", Nonce: "fresh-" + egressNonce()},
		}, 0)
		n.controlAll()
		return err
	})
	w.egressIsolationSteps(sc)
	w.egressNativeAssertions(sc)
}

// egressEndpoints starts each endpoint as {key, network, names...} and makes
// the independent control that precedes every worker attempt.
func (w *world) egressEndpoints(plan func(*egressNative) [][]string) error {
	n, err := w.egressNativeWorld()
	if err != nil {
		return err
	}
	for _, e := range plan(n) {
		if _, err = n.endpoint(e[0], e[1], e[2:], ""); err != nil {
			return err
		}
	}
	n.controlAll()
	return nil
}

// controlAll makes one more independent exchange with every endpoint. A failed
// control is kept for Then: an unavailable control fails there, never skips.
func (n *egressNative) controlAll() {
	for _, key := range n.order {
		e := n.endpoints[key]
		n.control(e, e.names[0])
	}
}

func (n *egressNative) canonicalControl() egressStep {
	return egressStep{Op: "tls", Mode: "connect", Name: n.names[0], Nonce: "control-" + egressNonce()}
}

// egressNativeDrive runs the product once with the plan and repeats every
// independent control afterwards.
func (w *world) egressNativeDrive(exit int, plan func(*egressNative) []egressStep) error {
	n, err := w.egressNativeWorld()
	if err != nil {
		return err
	}
	n.original = egressOriginal{fingerprint: w.approval.fingerprint, names: append([]string(nil), n.names...)}
	_, err = n.drive(plan(n), exit)
	n.controlAll()
	return err
}

func egressBothNames() []egressStep {
	return []egressStep{{Op: "tls", Mode: "connect", Name: "first.example", Nonce: "worker-" + egressNonce()}, {Op: "raw", Method: "CONNECT", Target: "later.example:443"}}
}

// egressWidenDuringRun holds one worker across three phases. Between them the
// harness edits the request and then approves it at a terminal, exactly as a
// person would; the running session must see neither.
func (w *world) egressWidenDuringRun() error {
	n, err := w.egressNativeWorld()
	if err != nil {
		return err
	}
	n.original = egressOriginal{fingerprint: w.approval.fingerprint, names: append([]string(nil), n.names...)}
	steps := egressBothNames()
	steps = append(steps, egressStep{Op: "barrier", Name: "edit"})
	steps = append(steps, egressBothNames()...)
	steps = append(steps, egressStep{Op: "barrier", Name: "approve"})
	steps = append(steps, egressBothNames()...)
	run := n.newRun(steps, 0)
	if err = run.start(); err != nil {
		return err
	}
	// A product that never brings the worker to a barrier has returned or is
	// stuck: either way its result is read now and left for Then.
	if !run.await("attach", 150*time.Second) {
		run.finish(10 * time.Second)
		return nil
	}
	run.observe("attach")
	run.attach(run.observed["attach"])
	n.original.snapshotPath = w.runSnapshotPath()
	n.original.snapshot, _ = os.ReadFile(n.original.snapshotPath)
	if err = run.release("attach"); err != nil {
		return err
	}
	if !run.await("edit", 2*time.Minute) {
		run.finish(10 * time.Second)
		return nil
	}
	wider := []string{"first.example", "later.example"}
	if err = os.WriteFile(w.doctor.checkout+"/itos-container.yaml", []byte(egressAllowlistPolicy(wider, "")), 0600); err != nil {
		return err
	}
	run.observe("edit")
	if err = run.release("edit"); err != nil {
		return err
	}
	if !run.await("approve", 2*time.Minute) {
		run.finish(10 * time.Second)
		return nil
	}
	// The oracle now describes the wider request; the review is answered yes.
	w.approval.allow, n.names = wider, wider
	if err = w.approvalOracle(); err != nil {
		return err
	}
	w.approval.generation = "2"
	approveErr := w.approvalTerminal("plain", "y", "", "")
	n.consent = w.stdout + w.stderr
	if w.exit != 0 {
		approveErr = errors.Join(approveErr, fmt.Errorf("terminal approval of the wider request exited %d\n%s", w.exit, w.report()))
	}
	run.observe("approve")
	return errors.Join(approveErr, run.release("approve"))
}
