package main

import (
	"bytes"
	"context"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	sp "github.com/donvargax/itos-container/internal/engine/session/port"
)

func lifecycleDiagnosticBudget(t *testing.T, backend string, parts []string) {
	t.Helper()
	child, err := strconv.Atoi(parts[1])
	if err != nil {
		t.Fatal(err)
	}
	control := lifecycleLocalFixture(t, backend)
	control.m.e.Exit = child
	code, diagnostic, _ := control.invoke(context.Background())
	if code != child || diagnostic != "" || len(control.m.e.Resources) != 1 || control.count("start") != 1 {
		t.Fatal("healthy control failed", code, diagnostic)
	}
	l := lifecycleLocalFixture(t, backend)
	l.m.e.Exit, l.m.e.DrainUntilCancel = child, true
	w := &lifecycleDiagnostic{}
	release := make(chan struct{})
	switch parts[2] {
	case "blocked":
		w.release = release
	case "before limit":
		w.delay = 10*time.Second - time.Nanosecond
	case "after limit":
		w.delay = 10*time.Second + time.Nanosecond
	default:
		t.Fatal("unknown diagnostic sink", parts[2])
	}
	var out bytes.Buffer
	start := time.Now()
	code = l.invokeWithStreams(context.Background(), &out, w)
	elapsed, atReturn := time.Since(start), w.String()
	id := strings.Repeat("a", 64)
	entry, failure := l.registry.Open(context.Background(), id, true)
	if failure != "" {
		close(release)
		t.Fatal("diagnostic timeout lost the session", failure)
	}
	record := entry.Record()
	intent := entry.Intent().Record()
	entry.Close()
	// Release only after command return and recovery observations. A caller
	// unblocking its writer cannot supply timely output or change the result.
	close(release)
	time.Sleep(w.delay)
	synctest.Wait()
	want := child
	if child == 0 {
		want = 75
	}
	if elapsed > 20*time.Second || code != want {
		t.Errorf("diagnostic call exceeded 10 s after the 10 s drain: elapsed %s, exit %d, want %d", elapsed, code, want)
	}
	if parts[2] == "before limit" {
		if elapsed != 10*time.Second+w.delay || !strings.Contains(atReturn, "run.stream-timeout") {
			t.Error("readable diagnostic was not delivered within its call budget", elapsed, atReturn)
		}
	} else if atReturn != "" || elapsed != 20*time.Second {
		t.Error("blocked or late diagnostic extended its call budget", elapsed, atReturn)
	}
	if record.State != "stopped" || intent.Stage != "absent" || record.WorkloadExit == nil || *record.WorkloadExit != child ||
		record.LastProblem == nil || *record.LastProblem != sp.StreamTimeout || len(l.m.e.Resources) != 1 || l.count("start") != 1 {
		t.Error("diagnostic wait changed workload outcome or owned cleanup", record, intent)
	}
	entry, failure = l.registry.Open(context.Background(), id, true)
	if failure != "" {
		t.Fatal("late writer lost recovery", failure)
	}
	defer entry.Close()
	if !reflect.DeepEqual(record, entry.Record()) || strings.Contains(w.String(), "synthetic-sensitive-argv") {
		t.Error("late diagnostic changed recovery or exposed workload input")
	}
}
