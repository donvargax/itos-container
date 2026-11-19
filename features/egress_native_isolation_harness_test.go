package features

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cucumber/godog"
)

// egressDefinitions collects step expressions without running anything. A
// definition may take the step's table after its captured arguments.
type egressDefinitions struct {
	t        *testing.T
	patterns []*regexp.Regexp
}

func (d *egressDefinitions) Step(expression, function any) {
	d.t.Helper()
	pattern := regexp.MustCompile(expression.(string))
	fn := reflect.TypeOf(function)
	if fn.Kind() != reflect.Func {
		d.t.Fatalf("step %s has no function", expression)
	}
	arguments := fn.NumIn()
	if arguments > 0 && fn.In(arguments-1) == reflect.TypeOf(&godog.Table{}) {
		arguments--
	}
	if arguments != pattern.NumSubexp() {
		d.t.Fatalf("step %s has an incompatible definition", expression)
	}
	d.patterns = append(d.patterns, pattern)
}

// Parse and register only: no Given, When or native setup executes. An
// undefined step in a CI red run is a wasted dispatch, not red evidence.
func TestEgressNativeStepsDefined(t *testing.T) {
	definitions := &egressDefinitions{t: t}
	(&world{}).egressNativeSteps(definitions)
	suite := godog.TestSuite{
		Name:    "slice-27-native-definitions",
		Options: &godog.Options{Paths: []string{"egress.feature"}, Tags: "@slice-27 && @engine", Output: io.Discard},
	}
	features, err := suite.RetrieveFeatures()
	if err != nil {
		t.Fatal(err)
	}
	cases := 0
	for _, feature := range features {
		for _, scenario := range feature.Pickles {
			cases++
			for _, step := range scenario.Steps {
				matches := 0
				for _, expression := range definitions.patterns {
					if expression.MatchString(step.Text) {
						matches++
					}
				}
				if matches != 1 {
					t.Errorf("%s: %q matches %d egress definitions, expected exactly one", scenario.Name, step.Text, matches)
				}
			}
		}
	}
	if cases != 8 {
		t.Fatalf("native egress scenarios = %d, expected 8", cases)
	}
}

// Every row of the two scenarios' tables must be one the harness has attempts
// for, and each table must ask for all of them: When refuses anything else.
func TestEgressIsolationRowsMatchTheScenarios(t *testing.T) {
	suite := godog.TestSuite{
		Name:    "slice-27-isolation-rows",
		Options: &godog.Options{Paths: []string{"egress.feature"}, Tags: "@ID-EGRESS-15,@ID-EGRESS-16", Output: io.Discard},
	}
	features, err := suite.RetrieveFeatures()
	if err != nil {
		t.Fatal(err)
	}
	tables := 0
	for _, feature := range features {
		for _, scenario := range feature.Pickles {
			for _, step := range scenario.Steps {
				if step.Argument == nil || step.Argument.DataTable == nil {
					continue
				}
				tables++
				known := egressResolverRows
				if strings.Contains(step.Text, "attempt these routes") {
					known = egressRouteRows
				}
				rows := step.Argument.DataTable.Rows[1:]
				if len(rows) != len(known) {
					t.Errorf("%q asks for %d rows, the harness defines %d", step.Text, len(rows), len(known))
				}
				for _, row := range rows {
					if _, ok := known[row.Cells[0].Value]; !ok {
						t.Errorf("%q: no attempt is defined for %q", step.Text, row.Cells[0].Value)
					}
				}
			}
		}
	}
	if tables != 2 {
		t.Fatalf("route and resolver tables = %d, expected 2", tables)
	}
}

func TestEgressIsolationGuard(t *testing.T) {
	t.Setenv("ITOS_CONTAINER_ENGINE_TESTS", "0")
	for _, mode := range []string{"network", "dns"} {
		w := &world{}
		if err := w.egressIsolationFixture(mode); err == nil || w.egress != nil || w.doctor != nil {
			t.Fatalf("the %s control fixture initialized outside CI", mode)
		}
	}
}

// The resolver rows must name exactly the routes slice-14's control plans, so
// no planned path goes unclaimed by a row and no row claims an unserved one.
func TestEgressResolverRowsCoverThePlannedInventory(t *testing.T) {
	s := &isolationWorld{config: isolationConfig{Resolvers: []string{"192.0.2.53"}}}
	if err := s.dnsRoutes(); err != nil {
		t.Fatal(err)
	}
	claimed := map[string]bool{}
	for _, routes := range egressResolverRows {
		for _, route := range routes {
			claimed[route] = true
		}
	}
	for _, route := range s.config.Routes {
		if !claimed[route.Name] {
			t.Errorf("no row claims the planned route %s", route.Name)
		}
		delete(claimed, route.Name)
	}
	for route := range claimed {
		t.Errorf("a row claims %s, which the control does not plan", route)
	}
}

func TestEgressDeniedRequiresCompleteUnansweredAttempts(t *testing.T) {
	for _, mode := range []string{"network", "dns"} {
		c := isolationConfig{Mode: mode, Nonce: "nonce", Routes: []isolationRoute{{Name: "v4", Protocol: "tcp4", Address: "192.0.2.1:1", Answer: "control"}, {Name: "docker-a", Protocol: "docker", Address: "127.0.0.11:53", Answer: "172.30.14.2"}}}
		reports := func() []isolationReport {
			var all []isolationReport
			for _, phase := range []string{"first", "running"} {
				r := isolationReport{Kind: mode, Nonce: c.Nonce, Phase: phase, Values: map[string]string{"resolv.conf": "nameserver 127.0.0.11\noptions ndots:0\n"}}
				for _, route := range c.Routes {
					r.Attempts = append(r.Attempts, isolationAttempt{Name: route.Name, Protocol: route.Protocol, Address: route.Address, Nonce: c.Nonce + "-" + phase + "-" + route.Name, Attempted: true, Error: "network is unreachable"})
				}
				all = append(all, r)
			}
			return all
		}
		if err := egressDenied(reports(), c, c.Nonce); err != nil {
			t.Fatal(mode, err)
		}
		// Docker's own resolver may refuse a name with an answer that carries
		// none: no transport error, and still no result.
		refused := reports()
		refused[0].Attempts[1].Error = ""
		if err := egressDenied(refused, c, c.Nonce); err != nil {
			t.Fatal(mode, err)
		}
		changes := []func(*[]isolationReport){
			func(r *[]isolationReport) { *r = nil }, func(r *[]isolationReport) { *r = (*r)[:1] },
			func(r *[]isolationReport) { (*r)[1].Phase = "first" }, func(r *[]isolationReport) { (*r)[0].Kind = "other" },
			func(r *[]isolationReport) { (*r)[0].Nonce = "stale" }, func(r *[]isolationReport) { (*r)[0].Attempts = (*r)[0].Attempts[:1] },
			func(r *[]isolationReport) { (*r)[0].Attempts[1] = (*r)[0].Attempts[0] },
			func(r *[]isolationReport) { (*r)[0].Attempts[0].Attempted = false },
			func(r *[]isolationReport) { (*r)[0].Attempts[0].Address = "different" },
			func(r *[]isolationReport) { (*r)[0].Attempts[0].Nonce = "stale" },
			func(r *[]isolationReport) { (*r)[1].Attempts[0].Result = "control:answer" },
			func(r *[]isolationReport) { (*r)[1].Attempts[1].Result = "172.30.14.2" },
			func(r *[]isolationReport) { (*r)[0].Attempts[0].Error = "" },
		}
		if mode == "dns" {
			changes = append(changes,
				func(r *[]isolationReport) { (*r)[0].Values["resolv.conf"] = "nameserver 192.0.2.53\n" },
				func(r *[]isolationReport) {
					(*r)[1].Values["resolv.conf"] = "nameserver 127.0.0.11\nnameserver 192.0.2.53\n"
				},
				func(r *[]isolationReport) { (*r)[1].Values["resolv.conf"] = "" })
		}
		for i, change := range changes {
			r := reports()
			change(&r)
			if egressDenied(r, c, c.Nonce) == nil {
				t.Errorf("%s: missing or contradicted evidence %d passed", mode, i)
			}
		}
	}
}

func TestEgressDefaultRouteReadsTheKernelTable(t *testing.T) {
	const header = "Iface\tDestination\tGateway \tFlags\tRefCnt\tUse\tMetric\tMask\t\tMTU\tWindow\tIRTT\n"
	connected := "eth0\t000012AC\t00000000\t0001\t0\t0\t0\t0000FFFF\t0\t0\t0\n"
	if routed, err := egressDefaultRoute(header + connected); err != nil || routed {
		t.Fatalf("a connected route alone read as %v, %v", routed, err)
	}
	if routed, err := egressDefaultRoute(header + "eth0\t00000000\t010012AC\t0003\t0\t0\t0\t00000000\t0\t0\t0\n" + connected); err != nil || !routed {
		t.Fatalf("a default route read as %v, %v", routed, err)
	}
	for _, table := range []string{"", header, connected, header + "eth0\t00000000\n"} {
		if _, err := egressDefaultRoute(table); err == nil {
			t.Errorf("%q read as a routing table", table)
		}
	}
}

func TestEgressPhasesMustSurroundTheBarrier(t *testing.T) {
	iso := &egressIsolation{s: &isolationWorld{mode: "network", config: isolationConfig{Mode: "network", Nonce: "nonce"}}}
	frame := func(phase string) string {
		data, _ := json.Marshal(isolationReport{Kind: "network", Nonce: "nonce", Phase: phase})
		return string(data) + "\n"
	}
	build := func() *egressNativeRun {
		run := &egressNativeRun{reached: map[string]bool{"attach": true}, steps: make([]egressStep, 3)}
		run.report = egressProbeReport{Done: true, Results: []egressProbeResult{
			{Op: "isolation", Name: "first", OK: true, Reply: frame("first")}, {Op: "barrier", Name: "attach", OK: true}, {Op: "isolation", Name: "running", OK: true, Reply: frame("running")}}}
		return run
	}
	if err := iso.phased(build()); err != nil {
		t.Fatal(err)
	}
	for i, change := range []func(*egressNativeRun){
		func(run *egressNativeRun) { run.reached = map[string]bool{} },
		func(run *egressNativeRun) { run.report.Done = false },
		func(run *egressNativeRun) { run.err = errors.New("harness") },
		func(run *egressNativeRun) { run.report.Results[0].OK = false },
		func(run *egressNativeRun) { run.report.Results[2].Reply = "" },
		func(run *egressNativeRun) { run.report.Results[2].Reply = frame("first") },
		func(run *egressNativeRun) { run.report.Results[0].Reply = `{"Kind":"error","Error":"probe failed"}` },
		func(run *egressNativeRun) {
			r := run.report.Results
			r[0], r[1] = r[1], r[0]
		},
		func(run *egressNativeRun) { run.report.Results = run.report.Results[:2]; run.steps = run.steps[:2] },
	} {
		run := build()
		change(run)
		if iso.phased(run) == nil {
			t.Errorf("incomplete or misordered phase evidence %d passed", i)
		}
	}
}

// egressTestResolver answers one name per record type and refuses the rest,
// so the probe's lookup is exercised over both transports without Docker.
func egressTestResolver(t *testing.T) string {
	t.Helper()
	answer := func(query []byte) []byte {
		if len(query) < 17 {
			return nil
		}
		end, name := 12, ""
		for end < len(query) && query[end] != 0 {
			name += string(query[end+1:end+1+int(query[end])]) + "."
			end += 1 + int(query[end])
		}
		if end+5 > len(query) {
			return nil
		}
		kind := binary.BigEndian.Uint16(query[end+1:])
		reply := append([]byte{}, query[:end+5]...)
		binary.BigEndian.PutUint16(reply[6:], 0)
		binary.BigEndian.PutUint16(reply[8:], 0)
		binary.BigEndian.PutUint16(reply[10:], 0)
		address := net.ParseIP(egressControl4).To4()
		if kind == 28 {
			address = net.ParseIP(egressControl6).To16()
		}
		if name != "outside-alias." || (kind != 1 && kind != 28) {
			binary.BigEndian.PutUint16(reply[2:], 0x8183) // no such name
			return reply
		}
		binary.BigEndian.PutUint16(reply[2:], 0x8180)
		binary.BigEndian.PutUint16(reply[6:], 1)
		reply = append(reply, 0xc0, 0x0c, byte(kind>>8), byte(kind), 0, 1, 0, 0, 0, 0, 0, byte(len(address)))
		return append(reply, address...)
	}
	stream, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	packets, err := net.ListenPacket("udp4", stream.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stream.Close(); _ = packets.Close() })
	go func() {
		buffer := make([]byte, 4096)
		for {
			n, from, err := packets.ReadFrom(buffer)
			if err != nil {
				return
			}
			if reply := answer(buffer[:n]); reply != nil {
				_, _ = packets.WriteTo(reply, from)
			}
		}
	}()
	go func() {
		for {
			conn, err := stream.Accept()
			if err != nil {
				return
			}
			go func() {
				defer func() { _ = conn.Close() }()
				_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
				var size [2]byte
				for {
					if _, err := io.ReadFull(conn, size[:]); err != nil {
						return
					}
					query := make([]byte, binary.BigEndian.Uint16(size[:]))
					if _, err := io.ReadFull(conn, query); err != nil {
						return
					}
					reply := answer(query)
					if reply == nil {
						return
					}
					binary.BigEndian.PutUint16(size[:], uint16(len(reply)))
					if _, err := conn.Write(append(size[:], reply...)); err != nil {
						return
					}
				}
			}()
		}
	}()
	return stream.Addr().String()
}

// egressTestEcho is the nonce echo slice-14's control runs, here on loopback:
// the same reply shape, with everything it received kept.
func egressTestEcho(t *testing.T) (address string, received func() []string) {
	t.Helper()
	stream, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	packets, err := net.ListenPacket("udp4", stream.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stream.Close(); _ = packets.Close() })
	var mu sync.Mutex
	var all []string
	keep := func(value string) {
		mu.Lock()
		defer mu.Unlock()
		all = append(all, value)
	}
	go func() {
		buffer := make([]byte, 2048)
		for {
			n, from, err := packets.ReadFrom(buffer)
			if err != nil {
				return
			}
			value := strings.TrimSpace(string(buffer[:n]))
			keep(value)
			_, _ = packets.WriteTo([]byte("control:"+value), from)
		}
	}()
	go func() {
		for {
			conn, err := stream.Accept()
			if err != nil {
				return
			}
			go func() {
				defer func() { _ = conn.Close() }()
				_ = conn.SetDeadline(time.Now().Add(time.Second))
				buffer := make([]byte, 1024)
				n, _ := conn.Read(buffer)
				value := strings.TrimSpace(string(buffer[:n]))
				keep(value)
				_, _ = io.WriteString(conn, "control:"+value+"\n")
			}()
		}
	}()
	return stream.Addr().String(), func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), all...)
	}
}

// A denial means something only if the same operation reports a reachable
// destination as reached. Each of the probe's bypass operations is run here
// against a local listener that answers and against one that does not exist,
// with no engine and no product. The route client itself is slice-14's and
// never runs on the host: a stand-in shows only that its report is carried.
func TestEgressProbeReportsAReachableRouteAsReached(t *testing.T) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Skip("the trusted programs are built for Linux amd64")
	}
	root, err := moduleRoot()
	if err != nil {
		t.Fatal(err)
	}
	programs, control, checkout := t.TempDir(), t.TempDir(), t.TempDir()
	for _, name := range []string{"endpoint", "probe"} {
		if err := egressBuild(root, filepath.Join("features", "testdata", "slice-27", name), filepath.Join(programs, name)); err != nil {
			t.Fatal(err)
		}
	}
	config := map[string]any{"Names": []string{"api.example.com"}, "Nonce": "endpoint-nonce", "Listen": "127.0.0.1:0"}
	if err := os.WriteFile(filepath.Join(control, "config.json"), approvalJSON(config), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	endpoint := exec.CommandContext(ctx, filepath.Join(programs, "endpoint"), "serve", control)
	if err := endpoint.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cancel(); _ = endpoint.Wait() })
	var ready struct{ Address, Leaf string }
	for ready.Leaf == "" {
		if data, err := os.ReadFile(filepath.Join(control, "ready.json")); err == nil {
			_ = json.Unmarshal(data, &ready)
		}
		if ctx.Err() != nil {
			t.Fatal("the endpoint never published its certificate")
		}
		time.Sleep(20 * time.Millisecond)
	}
	authority, err := os.ReadFile(filepath.Join(control, "ca.pem"))
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(checkout, "slice-27-ca.pem"), authority, 0600); err != nil {
		t.Fatal(err)
	}
	standIn := "#!/bin/sh\nprintf '{\"Kind\":\"%s\",\"Nonce\":\"%s\",\"Phase\":\"%s\",\"Values\":{\"plan\":\"%s\"}}\\n' \"$1\" \"$3\" \"$4\" \"$2\"\n"
	if err = os.WriteFile(filepath.Join(checkout, "slice-14-probe"), []byte(standIn), 0700); err != nil {
		t.Fatal(err)
	}
	echo, received := egressTestEcho(t)
	_, echoPort, _ := net.SplitHostPort(echo)
	resolver := egressTestResolver(t)
	// A port nothing listens on: reserved, then released.
	closed, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	absent := closed.Addr().String()
	_, absentPort, _ := net.SplitHostPort(absent)
	_ = closed.Close()
	iso := &egressIsolation{s: &isolationWorld{mode: "network", config: isolationConfig{Mode: "network", Nonce: "nonce"}}}
	reached := []egressStep{
		{Op: "isolation", Name: "first", Nonce: "nonce", Target: egressIsolationPlan},
		{Op: "direct", Method: "tcp4", Target: ready.Address, Name: "api.example.com", Nonce: iso.token("direct", "first", "b")},
		{Op: "forward", Method: "tcp4", Target: echoPort, Nonce: iso.token("forward-tcp", "first", "b")},
		{Op: "forward", Method: "udp4", Target: echoPort, Nonce: iso.token("forward-udp", "first", "b")},
		{Op: "datagram", Method: "udp4", Target: echo, Nonce: iso.token("quic", "first", "b")},
		{Op: "lookup", Mode: "docker-udp", Target: resolver, Input: "ip4", Name: "outside-alias"},
		{Op: "lookup", Mode: "docker-tcp", Target: resolver, Input: "ip4", Name: "outside-alias"},
		{Op: "lookup", Mode: "docker-tcp", Target: resolver, Input: "ip6", Name: "outside-alias"},
		{Op: "routes"},
	}
	denied := []egressStep{
		{Op: "direct", Method: "tcp4", Target: absent, Name: "api.example.com", Nonce: iso.token("direct", "first", "")},
		{Op: "direct", Method: "tcp4", Target: echo, Name: "api.example.com", Nonce: iso.token("direct", "running", "")},
		{Op: "forward", Method: "tcp4", Target: absentPort, Nonce: iso.token("forward-tcp", "first", "")},
		{Op: "forward", Method: "udp4", Target: absentPort, Nonce: iso.token("forward-udp", "first", "")},
		{Op: "datagram", Method: "udp4", Target: absent, Nonce: iso.token("quic", "first", "")},
		{Op: "lookup", Mode: "docker-udp", Target: resolver, Input: "ip4", Name: "inside-only"},
		{Op: "lookup", Mode: "docker-tcp", Target: resolver, Input: "ip4", Name: "inside-only"},
		{Op: "lookup", Mode: "docker-tcp", Target: absent, Input: "ip4", Name: "outside-alias"},
	}
	steps := append(append([]egressStep{}, reached...), denied...)
	if err = os.WriteFile(filepath.Join(checkout, "slice-27-plan.json"), approvalJSON(map[string]any{"Steps": steps, "Exit": 0}), 0600); err != nil {
		t.Fatal(err)
	}
	probe := exec.CommandContext(ctx, filepath.Join(programs, "probe"), filepath.Join(checkout, "slice-27-plan.json"))
	// The stand-in proxy address: forward reaches other ports of this host.
	probe.Env = []string{"HTTPS_PROXY=http://127.0.0.1:3128", "PATH=" + os.Getenv("PATH")}
	if out, err := probe.CombinedOutput(); err != nil {
		t.Fatalf("the probe failed: %v\n%s", err, out)
	}
	data, err := os.ReadFile(filepath.Join(checkout, "slice-27-report.json"))
	if err != nil {
		t.Fatal(err)
	}
	var report egressProbeReport
	if err = json.Unmarshal(data, &report); err != nil || !report.Done || len(report.Results) != len(steps) {
		t.Fatalf("incomplete report: %v\n%s", err, data)
	}
	want := []string{"", "endpoint-nonce", "control:" + steps[2].Nonce, "control:" + steps[3].Nonce, "control:" + steps[4].Nonce, egressControl4, egressControl4, egressControl6, ""}
	for i, r := range report.Results[:len(reached)] {
		if !r.OK || (want[i] != "" && r.Reply != want[i]) {
			t.Errorf("reachable %s %s%s: ok %v, reply %q, error %q", r.Op, r.Mode, r.Target, r.OK, r.Reply, r.Error)
		}
	}
	if r := report.Results[1]; r.Peer != ready.Leaf {
		t.Errorf("the direct exchange did not verify the endpoint's own certificate")
	}
	carried, err := isolationReports([]byte(report.Results[0].Reply))
	if err != nil || len(carried) != 1 || carried[0].Kind != "control" || carried[0].Nonce != "nonce" || carried[0].Phase != "first" || carried[0].Values["plan"] != egressIsolationPlan {
		t.Errorf("the route client's report was not carried whole: %v %+v", err, carried)
	}
	// This host's table is whatever it is, perhaps empty in a sandbox with no
	// network: only that the kernel's own table was carried is held here.
	if table := report.Results[len(reached)-1].Reply; !strings.HasPrefix(table, "Iface") {
		t.Errorf("the worker's routing table was not carried: %q", table)
	}
	for _, r := range report.Results[len(reached):] {
		if r.OK || r.Error == "" || r.Reply != "" || r.Peer != "" {
			t.Errorf("unreachable %s %s%s: ok %v, reply %q, peer %q", r.Op, r.Mode, r.Target, r.OK, r.Reply, r.Peer)
		}
	}
	// The QUIC-shaped datagram arrives as a client's first flight would: a long
	// header with version 1, 1200 bytes, the nonce in the clear.
	shaped := false
	for _, value := range received() {
		if strings.Contains(value, steps[4].Nonce) {
			shaped = len(value) == 1200 && value[0] == 0xc3 && value[1:5] == "\x00\x00\x00\x01"
		}
	}
	if !shaped {
		t.Error("the datagram did not arrive shaped as a 1200-byte QUIC version 1 Initial")
	}
	if port, _ := strconv.Atoi(echoPort); port == 0 {
		t.Fatal("no echo port")
	}
}
