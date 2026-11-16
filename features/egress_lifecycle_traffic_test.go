package features

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/cucumber/godog"
	"github.com/moby/moby/api/pkg/stdcopy"
)

type egressLifeTraffic struct {
	kind                          string
	recorder                      *egressLifeRecorder
	run                           *egressNativeRun
	original                      *egressObserved
	id, path                      string
	rows                          []map[string]string
	steps                         []egressStep
	fault                         *egressLifeRemoveFault
	journal                       []egressLifeRequest
	saved, frames, stdout         []byte
	observedErr                   error
	dnsControls                   []map[string]bool
	resolverBefore                []byte
	trace, traceJournal, tracePID string
	measured                      egressLifeMeasurement
	beforeStatus                  map[string]string
}

func (w *world) egressLifeTrafficSteps(sc lifecycleStepRegistrar) {
	sc.Step(`^independent allowed TLS DNS-failure and connect-failure controls with request receipts$`, w.egressLifeMetadataFixture)
	sc.Step(`^the product's worker sends these requests between verified allowed TLS controls and exits zero:$`, w.egressLifeMetadataRun)
	sc.Step(`^every control and table row has an independent completion receipt with the specified empty response and closure$`, w.egressLifeMetadataReceipts)
	sc.Step(`^the protected log contains exactly one safe record per row with the original method target port and reason$`, w.egressLifeTrafficRecords)
	sc.Step(`^its schema times and consecutive sequences match the proxy stdout captured before removal except for ready$`, w.egressLifeTrafficCopies)
	sc.Step(`^fresh plain and JSON status name that log with six stored records and the lost-tail limitation$`, func() error { return w.egressLifeTrafficStatus(6) })
	sc.Step(`^run returns zero only after the product confirms owned cleanup without changing the fixture endpoints$`, w.egressLifeTrafficClean)
	sc.Step(`^an independent TLS endpoint and fresh synthetic payload markers for these surfaces and expected log reasons:$`, w.egressLifePayloadFixture)
	sc.Step(`^the product runs every marker probe and fresh stop retries the one removal fault before status in both formats$`, func() error { return w.egressLifeTrafficRun() })
	sc.Step(`^independent receipts prove every marker probe and its expected acceptance or refusal actually occurred$`, w.egressLifePayloadReceipts)
	sc.Step(`^both persisted proxy log copies contain only the permitted metadata with the expected refusal reasons$`, func() error { return errors.Join(w.egressLifeTrafficRecords(), w.egressLifeTrafficCopies()) })
	sc.Step(`^fresh status names the retained log with seven independently counted records after complete owned cleanup$`, func() error { return w.egressLifeTrafficStatus(7) })
	sc.Step(`^a CI egress cap probe with an allowed TLS control and a worker that exits zero on SIGTERM$`, w.egressLifeCapFixture)
	sc.Step(`^run starts it and the worker completes a control then sends 20000 refused CONNECT attempts$`, w.egressLifeTrafficRun)
	sc.Step(`^the actual proxy exits nonzero at the refusal cap and run reports run.proxy-exited with process exit 1$`, w.egressLifeCapExit)
	sc.Step(`^the published log contains 20000 request records followed by one log-full record with original consecutive sequences$`, w.egressLifeCapRecords)
	sc.Step(`^the earliest and final request metadata remain without rotation truncation or ready in the published file$`, w.egressLifeCapReceipts)
	sc.Step(`^CI records the full log byte count copy and commit durations and remaining call and teardown budgets$`, w.egressLifeCapTiming)
	sc.Step(`^fresh status reports 20001 stored records and the lost-tail limitation after complete owned cleanup$`, func() error { return w.egressLifeTrafficStatus(20001) })
}

func (w *world) egressLifeTrafficFixture(kind string) error {
	recorder, err := w.egressLifeRecord()
	if err != nil {
		return err
	}
	w.egress.lifeTraffic = &egressLifeTraffic{kind: kind, recorder: recorder}
	return nil
}

func (w *world) egressLifeMetadataFixture() error {
	if err := w.egressLifeTrafficFixture("metadata"); err != nil {
		return err
	}
	n := w.egress.native
	// The closed-port target is independently reachable on 444, with its own
	// verified TLS nonce; nothing listens on its 443. DNS never leaves fixtures.
	closed, err := n.endpointConfigured("closed", "public", []string{"connect.example"}, "", "444", nil)
	if err != nil {
		return err
	}
	_, err = n.endpointConfigured("allowed", "public", []string{"api.example.com"}, "", "443", map[string]string{"api.example.com": "11.22.33.11", "connect.example": closed.address.String(), "dns.example": ""})
	if err != nil {
		return err
	}
	n.controlAll()
	w.egress.lifeTraffic.resolverBefore, err = os.ReadFile("/etc/resolv.conf")
	return err
}

func egressLifeTable(table *godog.Table) []map[string]string {
	var rows []map[string]string
	for _, row := range table.Rows[1:] {
		cells := map[string]string{}
		for i, cell := range row.Cells {
			cells[table.Rows[0].Cells[i].Value] = cell.Value
		}
		rows = append(rows, cells)
	}
	return rows
}

func (w *world) egressLifeMetadataRun(table *godog.Table) error {
	l, n := w.egress.lifeTraffic, w.egress.native
	l.rows = egressLifeTable(table)
	l.steps = []egressStep{n.canonicalControl()}
	for _, row := range l.rows {
		l.steps = append(l.steps, egressStep{Op: "raw", Method: row["method"], Target: row["target"]})
	}
	l.steps = append(l.steps, n.canonicalControl(), egressStep{Op: "barrier", Name: "evidence"})
	return w.egressLifeTrafficRun()
}

// Change DNS only in the independently inspected, disposable proxy's network
// namespace, after readiness. The worker and host rules/resolver stay untouched.
// The trusted nsenter commands are native CI fixture actions, not product calls.
func (w *world) egressLifeDNSInstall() error {
	l, n := w.egress.lifeTraffic, w.egress.native
	if l.original == nil || l.original.proxy == nil || l.original.proxy.State == nil || !l.original.proxy.State.Running {
		return errors.New("no running original proxy for DNS control")
	}
	pid := strconv.Itoa(l.original.proxy.State.Pid)
	// Docker's resolver address is loopback. DNAT to a bridge endpoint also
	// needs loopback routing and a routable return address, confined to this
	// disposable proxy namespace. Neither setting touches the host or worker.
	commands := [][]string{{"sysctl", "-w", "net.ipv4.conf.all.route_localnet=1"}}
	address := n.endpoints["allowed"].address.String()
	for _, protocol := range []string{"udp", "tcp"} {
		commands = append(commands,
			[]string{"iptables", "-t", "nat", "-I", "POSTROUTING", "1", "-d", address, "-p", protocol, "--dport", "53", "-j", "MASQUERADE"},
			[]string{"iptables", "-t", "nat", "-I", "OUTPUT", "1", "-d", "127.0.0.11", "-p", protocol, "--dport", "53", "-j", "DNAT", "--to-destination", address + ":53"},
		)
	}
	for _, command := range commands {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		args := append([]string{"-n", "nsenter", "--target", pid, "--net", "--"}, command...)
		cmd := exec.CommandContext(ctx, "sudo", args...)
		out, err := cmd.CombinedOutput()
		cancel()
		if err != nil {
			return fmt.Errorf("proxy-namespace DNS control: %w (%s)", err, out)
		}
	}
	return w.egressLifeDNSControl()
}

func (w *world) egressLifeDNSControl() error {
	l := w.egress.lifeTraffic
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "sudo", "-n", "nsenter", "--target", strconv.Itoa(l.original.proxy.State.Pid), "--net", "--", filepath.Join(w.doctor.checkout, "slice-27-probe"), "dns-control")
	out, err := cmd.Output()
	var receipt map[string]bool
	if decode := json.Unmarshal(out, &receipt); decode != nil {
		return errors.New("proxy DNS control produced no receipt")
	}
	l.dnsControls = append(l.dnsControls, receipt)
	return err
}

func (w *world) egressLifeTrafficRun() error {
	l, n := w.egress.lifeTraffic, w.egress.native
	l.run = n.newRun(l.steps, 0)
	if l.trace != "" {
		l.run.wrapper = []string{l.trace, l.traceJournal, l.tracePID, w.egressStatePath()}
	}
	if err := l.run.start(); err != nil {
		return err
	}
	if l.kind == "cap" {
		// Emit the available bounds even if a later capture or status invocation
		// fails. A missing phase remains an explicit failed measurement.
		defer func() {
			l.measured = egressLifeMeasure(l.traceJournal, l.recorder.snapshot(), l.path)
			l.measured.emit()
		}()
	}
	if l.run.await("attach", 150*time.Second) {
		l.run.observe("attach")
		l.original = l.run.observed["attach"]
		l.run.attach(l.original)
		if l.original != nil && l.original.proxy != nil {
			l.id = l.original.proxy.Config.Labels["io.itos-container.session"]
			l.path = w.egressStatePath("sessions", w.recoveryIndexKey(), l.id, "egress-refusals.jsonl")
			l.recorder.mu.Lock()
			l.recorder.logPath = l.path
			l.recorder.mu.Unlock()
			if l.fault != nil {
				l.fault.mu.Lock()
				l.fault.path = "/containers/" + l.original.proxy.ID
				l.fault.mu.Unlock()
			}
			if l.kind == "metadata" {
				l.observedErr = errors.Join(l.observedErr, w.egressLifeDNSInstall())
			}
		}
		if err := l.run.release("attach"); err != nil {
			return err
		}
		if l.kind != "cap" && l.run.await("evidence", 90*time.Second) {
			if l.kind == "metadata" {
				l.observedErr = errors.Join(l.observedErr, w.egressLifeDNSControl())
			}
			if err := l.run.release("evidence"); err != nil {
				return err
			}
		}
	}
	l.run.finish(3 * time.Minute)
	l.journal = l.recorder.snapshot()
	l.saved, _ = os.ReadFile(l.path)
	// The raw daemon frames below are the actual stopped log read, not worker
	// output or constructed JSON. The request completes before proxy deletion.
	if l.original != nil && l.original.proxy != nil && l.kind != "cap" {
		for _, r := range l.journal {
			if r.method == "GET" && r.path == "/containers/"+l.original.proxy.ID+"/logs" && !strings.Contains(r.query, "follow=1") && !strings.Contains(r.query, "follow=true") && !r.finished.IsZero() {
				l.frames = bytes.Clone(r.response)
			}
		}
		var stdout, stderr bytes.Buffer
		_, err := stdcopy.StdCopy(&stdout, &stderr, bytes.NewReader(l.frames))
		l.stdout = stdout.Bytes()
		if err != nil || stderr.Len() != 0 {
			l.observedErr = errors.Join(l.observedErr, errors.New("invalid or non-stdout final log stream"))
		}
	}
	n.controlAll()
	if l.kind == "payload" && l.id != "" {
		if err := w.egressInvoke([]string{"stop", l.id, "--json"}); err != nil {
			return err
		}
		l.journal = l.recorder.snapshot()
	}
	var err error
	l.beforeStatus, err = doctorTree(w.egressStatePath())
	if err != nil {
		l.observedErr = errors.Join(l.observedErr, err)
	}
	if err := w.egressInvoke([]string{"status"}, []string{"status", "--json"}); err != nil {
		return err
	}
	if l.kind == "payload" {
		w.egressLifeMarkerOutputs()
	}
	return nil
}

func (w *world) egressLifeTrafficHealthy() error {
	l, n := w.egress.lifeTraffic, w.egress.native
	if err := errors.Join(l.observedErr, l.run.err, egressLifeRunningPair(l.original)); err != nil {
		return err
	}
	if l.run.timedOut || !l.run.report.Done {
		return errors.New("traffic probe did not complete")
	}
	for _, e := range n.endpoints {
		if err := e.controlled(); err != nil {
			return err
		}
	}
	return n.endpointsIntact()
}

func egressLifeEmptyResponse(r egressProbeResult, status int) bool {
	return r.OK && r.Sent > 0 && r.Status == status && r.Body == 0 && r.Closed && r.EmptyLength && r.CloseHeader
}

func (w *world) egressLifeMetadataReceipts() error {
	l, n := w.egress.lifeTraffic, w.egress.native
	if err := w.egressLifeTrafficHealthy(); err != nil {
		return err
	}
	if err := n.exchanged(l.run, 2); err != nil {
		return err
	}
	results := l.run.results("raw")
	if len(results) != len(l.rows) || len(l.rows) != 6 {
		return errors.New("missing table request receipt")
	}
	for i, r := range results {
		code, err := strconv.Atoi(l.rows[i]["response"])
		if err != nil || !egressLifeEmptyResponse(r, code) || r.Method != l.rows[i]["method"] || r.Target != l.rows[i]["target"] {
			return fmt.Errorf("table row %d lacks its exact empty response and closure", i+1)
		}
	}
	if len(l.dnsControls) != 2 {
		return errors.New("missing pre/post DNS fault controls")
	}
	for _, r := range l.dnsControls {
		if len(r) != 3 || !r["api.example.com"] || !r["dns.example"] || !r["connect.example"] {
			return errors.New("DNS or closed-port control failed")
		}
	}
	receipts, err := n.endpoints["allowed"].receipts()
	if err != nil {
		return err
	}
	seen := map[string]int{}
	for _, r := range receipts {
		if r.Kind == "dns" {
			seen[r.Name]++
			if !slices.Contains([]string{"api.example.com", "dns.example", "connect.example"}, r.Name) {
				return errors.New("proxy resolved a policy-refused name")
			}
		}
	}
	for _, name := range []string{"api.example.com", "dns.example", "connect.example"} {
		if seen[name] < 3 {
			return errors.New("missing DNS control and product query receipts")
		}
	}
	after, err := os.ReadFile("/etc/resolv.conf")
	if err != nil || !bytes.Equal(after, l.resolverBefore) {
		return errors.New("host resolver changed")
	}
	return nil
}

func egressLifeLogRecords(data []byte) ([]map[string]any, error) {
	if len(data) == 0 || data[len(data)-1] != '\n' {
		return nil, errors.New("missing complete refusal log")
	}
	var records []map[string]any
	for _, line := range bytes.Split(data[:len(data)-1], []byte{'\n'}) {
		var r map[string]any
		if len(line) > 2048 || json.Unmarshal(line, &r) != nil || r["schema"] != float64(1) {
			return nil, errors.New("invalid bounded schema-1 record")
		}
		stamp, ok := r["time"].(string)
		if !ok || !strings.HasSuffix(stamp, "Z") {
			return nil, errors.New("record has no UTC timestamp")
		}
		if _, err := time.Parse(time.RFC3339Nano, stamp); err != nil {
			return nil, err
		}
		for key := range r {
			if !slices.Contains([]string{"schema", "seq", "time", "event", "reason", "method", "target", "port"}, key) {
				return nil, errors.New("record contains a forbidden field")
			}
		}
		records = append(records, r)
	}
	return records, nil
}

func (w *world) egressLifeTrafficRecords() error {
	l := w.egress.lifeTraffic
	records, err := egressLifeLogRecords(l.saved)
	if err != nil {
		return err
	}
	var rows []map[string]string
	for _, r := range l.rows {
		if r["reason"] != "no record" {
			rows = append(rows, r)
		}
	}
	if len(records) != len(rows) {
		return errors.New("refusal record count differs from attempts")
	}
	for i, r := range records {
		row := rows[i]
		if r["seq"] != float64(i+2) || r["reason"] != row["reason"] {
			return fmt.Errorf("record %d lost reason or sequence", i+1)
		}
		if l.kind == "metadata" {
			host, port, _ := strings.Cut(row["target"], ":")
			if len(r) != 8 || r["event"] != row["event"] || r["method"] != row["method"] || r["target"] != host || r["port"] != port {
				return fmt.Errorf("record %d lost original safe metadata", i+1)
			}
		} else {
			method, target, port, fields := "CONNECT", "", "", 6
			if i == 3 {
				method = "GET"
			}
			if i >= 4 {
				fields = 8
				target = "api.example.com"
				port = "443"
				if i == 4 {
					target = "unlisted.example"
				}
			}
			if r["event"] != "refused" || r["method"] != method || len(r) != fields {
				return errors.New("payload record contains unexpected metadata")
			}
			if fields == 6 {
				if _, ok := r["target"]; ok {
					return errors.New("malformed target entered log")
				}
				if _, ok := r["port"]; ok {
					return errors.New("malformed port entered log")
				}
			} else if r["target"] != target || r["port"] != port {
				return errors.New("payload record lost permitted target or port")
			}
		}
	}
	return nil
}

func (w *world) egressLifeTrafficCopies() error {
	l := w.egress.lifeTraffic
	records, err := egressLifeLogRecords(l.stdout)
	if err != nil {
		return err
	}
	if records[0]["event"] != "ready" || len(records[0]) != 4 {
		return errors.New("daemon stdout does not begin with readiness")
	}
	for i, r := range records {
		if r["seq"] != float64(i+1) {
			return errors.New("stdout sequence is not consecutive")
		}
	}
	_, tail, _ := bytes.Cut(l.stdout, []byte{'\n'})
	if !bytes.Equal(tail, l.saved) {
		return errors.New("protected log differs from daemon stdout after ready")
	}
	if err := egressLifePublishedBeforeRemoval(l.journal, l.original.proxy.ID, l.saved); err != nil {
		return err
	}
	info, err := os.Lstat(l.path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 {
		return errors.New("published log is not private")
	}
	return nil
}

func (w *world) egressLifeTrafficStatus(count int) error {
	l := w.egress.lifeTraffic
	plain, structured, err := w.egressStatusResults()
	if err != nil {
		return err
	}
	if err := egressLifeStoredStatus(structured, l.id, l.path, "stored", count); err != nil {
		return err
	}
	if plain.result.exit != 0 || !strings.Contains(plain.result.stdout, l.path) || !strings.Contains(plain.result.stdout, "stored") || !strings.Contains(plain.result.stdout, strconv.Itoa(count)) {
		return errors.New("plain status lost stored evidence")
	}
	for _, result := range []egressResult{plain, structured} {
		if !strings.Contains(result.result.stdout, egressLimitations[3]) {
			return errors.New("status lost the lost-tail limitation")
		}
	}
	after, err := doctorTree(w.egressStatePath())
	if err != nil || !reflect.DeepEqual(after, l.beforeStatus) {
		return errors.New("status changed protected state")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	remaining, err := w.egress.native.owned(ctx)
	if err != nil || len(remaining) != 0 {
		return errors.New("native owned cleanup is incomplete")
	}
	return nil
}

func (w *world) egressLifeTrafficClean() error {
	l := w.egress.lifeTraffic
	if l.run.exit != 0 {
		return fmt.Errorf("traffic run exited %d", l.run.exit)
	}
	return errors.Join(l.run.cleaned(), egressLifeReverseCreated(l.journal), w.egress.native.endpointsIntact())
}

func egressLifeDigest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
