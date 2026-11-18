package features

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (w *world) egressLifeCapFixture() error {
	if err := w.egressNativeCheckout([]string{"api.example.com"}, []string{"api.example.com"}); err != nil {
		return err
	}
	if err := w.egressLifeTrafficFixture("cap"); err != nil {
		return err
	}
	n, l := w.egress.native, w.egress.lifeTraffic
	if _, err := n.endpoint("cap", "public", []string{"api.example.com"}, ""); err != nil {
		return err
	}
	n.controlAll()
	l.trace = filepath.Join(w.dir, "slice-28-log-trace")
	l.traceJournal = filepath.Join(w.dir, "slice-28-log-trace.jsonl")
	l.tracePID = filepath.Join(w.dir, "slice-28-log-trace.pid")
	if err := recoveryBuildHelper(w.root, l.trace, egressLifeTraceSource()); err != nil {
		return err
	}
	l.steps = []egressStep{n.canonicalControl(), {Op: "cap"}, {Op: "wait-signal", Name: "cap-exit"}}
	return nil
}

func (w *world) egressLifeCapExit() error {
	l := w.egress.lifeTraffic
	if err := w.egressLifeTrafficHealthy(); err != nil {
		return err
	}
	if l.run.exit != 1 || !strings.Contains(string(l.run.stderr.bytes()), "run.proxy-exited") {
		return errors.New("cap did not cause run.proxy-exited with exit 1")
	}
	if err := w.egress.native.exchanged(l.run, 1); err != nil {
		return err
	}
	cap := l.run.results("cap")
	if len(cap) != 1 || !cap[0].OK || cap[0].Count != 20000 {
		return errors.New("worker did not complete all cap attempts")
	}
	proxyExit := false
	for _, r := range l.journal {
		if r.method == "POST" && r.path == "/containers/"+l.original.proxy.ID+"/wait" && r.status == 200 && !r.finished.IsZero() {
			var v struct{ StatusCode *int }
			if json.Unmarshal(r.response, &v) == nil && v.StatusCode != nil && *v.StatusCode != 0 {
				proxyExit = true
			}
		}
	}
	if !proxyExit || !egressLifeWaitExit(l.journal, l.original.worker.ID, 0) {
		return errors.New("missing actual proxy failure or cooperative worker zero-exit wait")
	}
	signal, err := os.ReadFile(filepath.Join(w.doctor.checkout, "slice-28-signal"))
	if err != nil || string(signal) != "SIGTERM" {
		return errors.New("cap worker did not acknowledge SIGTERM")
	}
	return errors.Join(l.run.cleanedWithExit(1), egressLifeReverseCreated(l.journal))
}

func egressLifeCapTarget(index int) string {
	return fmt.Sprintf("%06d", index) + strings.Repeat("a", 57) + "." + strings.Repeat("b", 63) + "." + strings.Repeat("c", 63) + "." + strings.Repeat("d", 63)
}

func egressLifeCheckCap(data []byte) error {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 2049), 2049)
	count := 0
	for scanner.Scan() {
		count++
		records, err := egressLifeLogRecords(append(bytes.Clone(scanner.Bytes()), '\n'))
		if err != nil {
			return err
		}
		r := records[0]
		if r["seq"] != float64(count+1) {
			return errors.New("full log lost original consecutive sequence")
		}
		if count == 20001 {
			if len(r) != 4 || r["event"] != "log-full" {
				return errors.New("missing final log-full record")
			}
		} else if count > 20000 || len(r) != 8 || r["event"] != "refused" || r["reason"] != "not-listed" || r["method"] != "CONNECT" || r["port"] != "443" || r["target"] != egressLifeCapTarget(count) {
			return fmt.Errorf("full log request %d lost its original metadata", count)
		}
	}
	if scanner.Err() != nil || count != 20001 || len(data) == 0 || data[len(data)-1] != '\n' {
		return errors.New("full log was rotated truncated or incomplete")
	}
	return nil
}

func (w *world) egressLifeCapRecords() error { return egressLifeCheckCap(w.egress.lifeTraffic.saved) }

func (w *world) egressLifeCapReceipts() error {
	l := w.egress.lifeTraffic
	file, err := os.Open(filepath.Join(w.doctor.checkout, "slice-28-attempts.jsonl"))
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	decoder := json.NewDecoder(file)
	for i := 1; i <= 20000; i++ {
		var r egressProbeResult
		if decoder.Decode(&r) != nil || r.Target != egressLifeCapTarget(i)+":443" {
			return errors.New("missing distinct cap attempt receipt")
		}
		request := fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", r.Target, r.Target)
		if r.Sent != len(request) || r.Digest != egressLifeDigest([]byte(request)) {
			return errors.New("cap attempt send receipt differs")
		}
		// The last connection may close before an HTTP response. Earlier rows must
		// have acknowledged empty refusals; an unexecuted final attempt fails above.
		if i < 20000 && (r.Status != 403 || r.Body != 0 || !r.Closed || !r.EmptyLength || !r.CloseHeader) {
			return errors.New("early cap refusal was not acknowledged")
		}
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return errors.New("unexpected extra cap attempt")
	}
	if err := egressLifePublishedBeforeRemoval(l.journal, l.original.proxy.ID, l.saved); err != nil {
		return err
	}
	return egressLifeCheckCap(l.saved)
}

// Extend only the existing child syscall observer. The product, budgets, return
// values and filesystem calls are untouched. Both entry and completion of writes
// and fsyncs are captured, including Go threads and the durable rename's parent.
func egressLifeTraceSource() string {
	source := recoveryTraceSource
	for _, change := range [][2]string{
		{`type event struct {`, `type event struct { Op string; Started time.Time; Bytes int64; Observer int64;`},
		{`exit := 96`, `exit := 96
    var observer time.Duration`},
		{`signal := int(status.StopSignal())`, `stopObserved := time.Now()
        signal := int(status.StopSignal())`},
		{`if regs.Orig_rax == unix.SYS_FSYNC { c.sync = observe(tid, regs.Rdi, root) }`, `if regs.Orig_rax == unix.SYS_FSYNC || regs.Orig_rax == unix.SYS_WRITE || regs.Orig_rax == unix.SYS_WRITEV {
                    started := time.Now()
                    c.sync = observe(tid, regs.Rdi, root)
                    if c.sync != nil {
                        c.sync.Started = started
                        c.sync.Op = "write"
                        if regs.Orig_rax == unix.SYS_FSYNC { c.sync.Op = "fsync" }
                    }
                }`},
		{`if regs.Rax == 0 {
                    completed(c.sync)`, `if int64(regs.Rax) >= 0 {
                    c.sync.Bytes = int64(regs.Rax)
                    completed(c.sync)`},
		{`if err := unix.PtraceSyscall(tid, signal);`, `observer += time.Since(stopObserved)
        if err := unix.PtraceSyscall(tid, signal);`},
		{`os.Exit(exit)`, `if json.NewEncoder(journal).Encode(event{Op:"observer", Observer:int64(observer)}) != nil { fail() }
    os.Exit(exit)`},
	} {
		if strings.Count(source, change[0]) != 1 {
			panic("slice-28 timing observer no longer matches its reviewed base")
		}
		source = strings.Replace(source, change[0], change[1], 1)
	}
	return source
}

type egressLifeTimingEvent struct {
	recoverySync
	Op              string
	Started         time.Time
	Bytes, Observer int64
}

type egressLifeMeasurement struct {
	Bytes                                    int64 `json:"bytes"`
	CopyLower, CopyUpper                     time.Duration
	CommitLower, CommitUpper                 time.Duration
	TeardownUpper                            time.Duration
	ObserverHandling                         time.Duration
	CopyMargin, CommitMargin, TeardownMargin time.Duration
	Error                                    string `json:"error,omitempty"`
	// Trace is a failure diagnostic, never a bound: the event counts the
	// observer recorded around the published log, so a failed run names the
	// durability phase the real engine did not deliver.
	Trace string `json:"trace,omitempty"`
}

func egressLifeMeasure(journal string, requests []egressLifeRequest, path string) egressLifeMeasurement {
	m := egressLifeMeasurement{}
	bad := func(message string) egressLifeMeasurement { m.Error = message; return m }
	info, err := os.Stat(path)
	if err != nil {
		return bad("published log missing")
	}
	m.Bytes = info.Size()
	file, err := os.Open(journal)
	if err != nil {
		return bad("storage trace missing")
	}
	defer func() { _ = file.Close() }()
	var events []egressLifeTimingEvent
	decoder := json.NewDecoder(file)
	for {
		var e egressLifeTimingEvent
		err := decoder.Decode(&e)
		if err == io.EOF {
			break
		}
		if err != nil {
			return bad("storage trace unreadable")
		}
		events = append(events, e)
		if e.Op == "observer" {
			m.ObserverHandling = time.Duration(e.Observer)
		}
	}
	var parent, fsync, lastWrite egressLifeTimingEvent
	var dirSyncs, stagedSyncs, stagedWrites int64
	for _, e := range events {
		if e.Op == "fsync" && e.Directory && e.Path == filepath.Dir(path) {
			dirSyncs++
			if e.Files[filepath.Base(path)] != "" && parent.At.IsZero() {
				parent = e
			}
		}
	}
	inode := parent.Files[filepath.Base(path)]
	var written int64
	for _, e := range events {
		if inode != "" && e.Inode == inode {
			if e.Op == "write" {
				written += e.Bytes
				stagedWrites++
				if e.At.After(lastWrite.At) {
					lastWrite = e
				}
			}
			if e.Op == "fsync" && !e.Directory {
				fsync = e
				stagedSyncs++
			}
		}
	}
	m.Trace = fmt.Sprintf("events=%d session-dir-fsyncs=%d parent-fsync-with-log=%t log-inode-writes=%d log-inode-fsyncs=%d written=%d bytes=%d observer=%s", len(events), dirSyncs, inode != "", stagedWrites, stagedSyncs, written, m.Bytes, m.ObserverHandling)
	// The six durability observations are individually named so a failed run
	// reports which phase the real engine did not deliver; together they are
	// exactly the one reviewed "incomplete staged-write/file-fsync/rename/
	// parent-fsync timing" refusal.
	switch {
	case inode == "":
		return bad("no parent fsync observed the renamed log in its session directory")
	case lastWrite.At.IsZero():
		return bad("no staged write observed for the published log's inode")
	case fsync.At.IsZero():
		return bad("no file fsync observed for the published log's inode")
	case fsync.Started.Before(lastWrite.At):
		return bad("the log's file fsync began before its last staged write")
	case parent.At.Before(fsync.At):
		return bad("the session directory fsync completed before the log's file fsync")
	case written != m.Bytes:
		return bad(fmt.Sprintf("staged writes observed %d of the file's %d bytes", written, m.Bytes))
	case m.ObserverHandling == 0:
		return bad("the observer reported no handling time")
	}
	saved, err := os.ReadFile(path)
	if err != nil {
		return bad("published log unreadable")
	}
	var copyStart, copyEOF, removeStart, teardownStart, teardownEnd time.Time
	for _, r := range requests {
		if r.method == "GET" && strings.HasSuffix(r.path, "/logs") && !strings.Contains(r.query, "follow=1") && !strings.Contains(r.query, "follow=true") {
			copyStart, copyEOF = r.at, r.finished
		}
		if r.method == "DELETE" && bytes.Equal(r.logBefore, saved) && r.logErr == nil && r.status == 204 && r.at.After(parent.At) && removeStart.IsZero() {
			removeStart = r.at
		}
		if r.method == "POST" && strings.HasSuffix(r.path, "/wait") && r.status == 200 && !r.finished.IsZero() && (teardownStart.IsZero() || r.finished.Before(teardownStart)) {
			teardownStart = r.finished
		}
		if r.finished.After(teardownEnd) {
			teardownEnd = r.finished
		}
	}
	if copyStart.IsZero() || copyEOF.IsZero() || removeStart.IsZero() || teardownStart.IsZero() || teardownEnd.Before(removeStart) {
		return bad("complete API copy/removal/teardown intervals missing")
	}
	// Each upper bound includes adjacent controller work and observation overhead.
	// The newline-terminated cap leaves no held partial line for Commit. Including
	// the last write in its upper bound still covers that boundary conservatively.
	m.CopyLower = lastWrite.At.Sub(copyStart)
	m.CopyUpper = fsync.Started.Sub(copyStart)
	m.CommitLower = parent.At.Sub(fsync.Started)
	m.CommitUpper = removeStart.Sub(lastWrite.Started)
	m.TeardownUpper = teardownEnd.Sub(teardownStart)
	m.CopyMargin = 10*time.Second - m.CopyUpper
	m.CommitMargin = 10*time.Second - m.CommitUpper
	m.TeardownMargin = 30*time.Second - m.TeardownUpper
	if m.CopyLower < 0 || m.CopyUpper < m.CopyLower || m.CommitLower < 0 || m.CommitUpper < m.CommitLower || m.TeardownUpper < 0 {
		return bad("timing intervals are not ordered")
	}
	return m
}

func (m egressLifeMeasurement) emit() {
	data, _ := json.Marshal(m)
	fmt.Printf("::notice title=slice-28 full-log timing::runner=%s/%s/%s durations=nanoseconds upper-bounds-include-observer-and-adjacent-work; observer-handling-excludes-scheduler-wait; no-overhead-subtracted %s\n", os.Getenv("RUNNER_NAME"), os.Getenv("RUNNER_OS"), os.Getenv("RUNNER_ARCH"), data)
}

func (w *world) egressLifeCapTiming() error {
	m := w.egress.lifeTraffic.measured
	if m.Error != "" {
		return errors.New(m.Error)
	}
	if m.Bytes <= 1<<20 || m.CopyMargin <= 0 || m.CommitMargin <= 0 || m.TeardownMargin <= 0 {
		return errors.New("full-log measurement has no proven budget margin")
	}
	return nil
}
