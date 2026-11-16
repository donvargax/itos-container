package features

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestEgressLifecycleCaptureWithoutLocalListener(t *testing.T) {
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	w := &world{bin: binary, dir: dir, doctor: &doctorWorld{cwd: dir, engine: newDoctorAPI(doctorImage), env: map[string]string{"SLICE_28_CAPTURE_CHILD": "1"}}, approval: &approvalWorld{record: filepath.Join(dir, "approvals", "project", "approval.json"), project: "project", generation: "1"}, egress: &egressWorld{native: &egressNative{}}}
	if err := os.MkdirAll(filepath.Dir(w.approval.record), 0700); err != nil {
		t.Fatal(err)
	}
	if err := w.egressInvoke([]string{"-test.run=^TestEgressLifecycleCaptureChild$"}); err != nil {
		t.Fatal(err)
	}
	if len(w.egress.results) != 1 {
		t.Fatal("no invocation observation")
	}
	r := w.egress.results[0]
	if r.result.exit != 17 || r.result.stdout != "native capture\n" || r.result.stderr != "native diagnostic\n" || r.connections != 0 || r.approvalErr != nil {
		t.Fatalf("native capture lost its process outcome: %+v", r)
	}
}

func TestEgressLifecycleCaptureChild(t *testing.T) {
	if os.Getenv("SLICE_28_CAPTURE_CHILD") != "1" {
		return
	}
	_, _ = fmt.Fprintln(os.Stdout, "native capture")
	_, _ = fmt.Fprintln(os.Stderr, "native diagnostic")
	os.Exit(17)
}

func TestEgressLifecycleCapOracleRejectsMissingAndChangedRows(t *testing.T) {
	var data bytes.Buffer
	for i := 1; i <= 20000; i++ {
		data.Write(approvalJSON(map[string]any{"schema": 1, "seq": i + 1, "time": "2026-11-16T00:00:00Z", "event": "refused", "reason": "not-listed", "method": "CONNECT", "target": egressLifeCapTarget(i), "port": "443"}))
		data.WriteByte('\n')
	}
	data.WriteString(`{"schema":1,"seq":20002,"time":"2026-11-16T00:00:00Z","event":"log-full"}` + "\n")
	complete := data.Bytes()
	if err := egressLifeCheckCap(complete); err != nil {
		t.Fatal(err)
	}
	first := bytes.IndexByte(complete, '\n') + 1
	for _, bad := range [][]byte{complete[first:], complete[:len(complete)-1], bytes.Replace(complete, []byte(egressLifeCapTarget(20000)), []byte(egressLifeCapTarget(1)), 1), bytes.Replace(complete, []byte(`"seq":2,`), []byte(`"seq":3,`), 1), bytes.Replace(complete, []byte(`"log-full"`), []byte(`"ready"`), 1)} {
		if err := egressLifeCheckCap(bad); err == nil {
			t.Fatal("damaged full-log evidence was accepted")
		}
	}
}

func TestEgressLifecycleTimingObservesRealStorageCalls(t *testing.T) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Skip("child syscall observation targets Linux amd64")
	}
	root, err := moduleRoot()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	tracer, child := filepath.Join(dir, "trace"), filepath.Join(dir, "child")
	for path, source := range map[string]string{tracer: egressLifeTraceSource(), child: egressLifeTimingChild} {
		if err := recoveryBuildHelper(root, path, source); err != nil {
			t.Fatal(err)
		}
	}
	for _, mode := range []string{"complete", "no-file-sync", "no-parent-sync"} {
		t.Run(mode, func(t *testing.T) {
			state := t.TempDir()
			journal, pid := filepath.Join(t.TempDir(), "trace.jsonl"), filepath.Join(t.TempDir(), "pid")
			ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
			defer cancel()
			command := exec.CommandContext(ctx, tracer, journal, pid, state, child, state, mode)
			start := time.Now()
			if output, err := command.CombinedOutput(); err != nil {
				t.Fatalf("trace failed: %v %s", err, output)
			}
			end := time.Now()
			path := filepath.Join(state, "egress-refusals.jsonl")
			saved, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			requests := []egressLifeRequest{{method: "POST", path: "/containers/worker/wait", status: 200, finished: start}, {method: "GET", path: "/containers/proxy/logs", at: start, finished: end}, {method: "DELETE", path: "/containers/proxy", status: 204, at: end, logBefore: saved}, {method: "GET", path: "/volumes/anchor", status: 404, finished: end.Add(time.Millisecond)}}
			m := egressLifeMeasure(journal, requests, path)
			if mode != "complete" {
				if m.Error == "" {
					t.Fatal("missing durability phase accepted")
				}
				return
			}
			if m.Error != "" || m.Bytes != 2<<20 || m.CopyUpper < m.CopyLower || m.CommitUpper < m.CommitLower || m.ObserverHandling <= 0 {
				t.Fatalf("incomplete measurement: %+v", m)
			}
			data, err := os.ReadFile(journal)
			if err != nil {
				t.Fatal(err)
			}
			var dropped []byte
			for _, line := range bytes.Split(bytes.TrimSpace(data), []byte{'\n'}) {
				var e egressLifeTimingEvent
				if json.Unmarshal(line, &e) != nil {
					t.Fatal("bad trace")
				}
				if e.Op != "write" {
					dropped = append(dropped, append(bytes.Clone(line), '\n')...)
				}
			}
			if err := os.WriteFile(journal, dropped, 0600); err != nil {
				t.Fatal(err)
			}
			if m := egressLifeMeasure(journal, requests, path); m.Error == "" {
				t.Fatal("a header timestamp without writes passed as complete copy")
			}
		})
	}
}

const egressLifeTimingChild = `package main
import("bytes";"os";"path/filepath")
func must(err error) { if err!=nil { os.Exit(94) } }
func main(){
 root,mode:=os.Args[1],os.Args[2]
 file,err:=os.OpenFile(filepath.Join(root,"staged"),os.O_CREATE|os.O_EXCL|os.O_WRONLY,0600);must(err)
 for i:=0;i<64;i++ { _,err=file.Write(bytes.Repeat([]byte("x"),32768));must(err) }
 if mode!="no-file-sync" {must(file.Sync())};must(file.Close())
 must(os.Rename(filepath.Join(root,"staged"),filepath.Join(root,"egress-refusals.jsonl")))
 parent,err:=os.Open(root);must(err)
 if mode!="no-parent-sync" {must(parent.Sync())};must(parent.Close())
}
`

func TestEgressLifecycleCapProbeStreamsEveryDistinctAttempt(t *testing.T) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Skip("the worker probe targets Linux amd64")
	}
	root, err := moduleRoot()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	program := filepath.Join(dir, "probe")
	if err := egressBuild(root, "features/testdata/slice-27/probe", program); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()
	done := make(chan error, 1)
	go func() {
		for i := 1; i <= 20000; i++ {
			conn, err := listener.Accept()
			if err != nil {
				done <- err
				return
			}
			_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
			reader := bufio.NewReader(conn)
			var head strings.Builder
			for {
				line, err := reader.ReadString('\n')
				if err != nil {
					_ = conn.Close()
					done <- err
					return
				}
				head.WriteString(line)
				if line == "\r\n" {
					break
				}
			}
			target := egressLifeCapTarget(i) + ":443"
			want := fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", target, target)
			if head.String() != want {
				_ = conn.Close()
				done <- errors.New("probe changed attempt metadata")
				return
			}
			if i < 20000 {
				_, err = io.WriteString(conn, "HTTP/1.1 403 Forbidden\r\nContent-Length: 0\r\nConnection: close\r\n\r\n")
			}
			_ = conn.Close()
			if err != nil {
				done <- err
				return
			}
		}
		done <- nil
	}()
	plan := filepath.Join(dir, "plan.json")
	if err := os.WriteFile(plan, approvalJSON(map[string]any{"Steps": []egressStep{{Op: "cap"}}, "Exit": 0}), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, program, plan)
	command.Env = setEnv(sessionChildEnvironment(os.Environ()), "HTTPS_PROXY", "http://"+listener.Addr().String())
	if output, err := command.CombinedOutput(); err != nil {
		_ = listener.Close()
		t.Fatalf("probe failed: %v %s", err, output)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "slice-27-report.json"))
	var report egressProbeReport
	if err != nil || json.Unmarshal(data, &report) != nil || !report.Done || len(report.Results) != 1 || !report.Results[0].OK || report.Results[0].Count != 20000 {
		t.Fatal("cap probe omitted its completed aggregate receipt")
	}
	file, err := os.Open(filepath.Join(dir, "slice-28-attempts.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	decoder := json.NewDecoder(file)
	for i := 1; i <= 20000; i++ {
		var r egressProbeResult
		if decoder.Decode(&r) != nil || r.Target != egressLifeCapTarget(i)+":443" || r.Sent == 0 || r.Digest == "" {
			t.Fatal("missing streamed attempt receipt")
		}
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		t.Fatal("unexpected attempt receipt")
	}
}
