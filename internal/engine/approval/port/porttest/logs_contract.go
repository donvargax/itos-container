package porttest

import (
	"bytes"
	"context"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/donvargax/itos-container/internal/engine/approval/port"
	budget "github.com/donvargax/itos-container/internal/engine/budget/port"
)

// SessionLogsFixture binds one registry's storage to its staged logs and to what
// a test can observe of a session's log without going through either port.
type SessionLogsFixture struct {
	Storage   port.RegistryStorage
	Logs      port.SessionLogs
	Candidate port.Candidate
	// Read returns a session's published log, nil when no file exists and an
	// empty slice when an empty one does.
	Read func(id string) []byte
	// Staged counts a session's staged temporaries that are not yet released.
	Staged func(id string) int
	// Fault makes one step fail from now on, by the memory fake's names: write,
	// file fsync, atomic publication or parent fsync. An empty name clears it.
	Fault func(string)
}

type sessionLogs struct {
	t     Reporter
	f     SessionLogsFixture
	id    string
	phase context.Context
}

// stage opens a handle under a call context that has ended by the time it is
// used, while the phase it was acquired in is still alive.
func (s sessionLogs) stage(step string) port.StagedLog {
	s.t.Helper()
	call, end := context.WithTimeout(s.phase, time.Second)
	staged, failure := s.f.Logs.Stage(budget.WithLifetime(call, s.phase), s.f.Candidate, s.id)
	end()
	if failure != "" || staged == nil {
		s.t.Errorf("%s: stage = %v, %s", step, staged, failure)
		return nil
	}
	return staged
}

// holds checks the visible log, nil for none, and the count of temporaries.
func (s sessionLogs) holds(step string, want []byte, staged int) {
	s.t.Helper()
	got := s.f.Read(s.id)
	if (got == nil) != (want == nil) || !bytes.Equal(got, want) || s.f.Staged(s.id) != staged {
		s.t.Errorf("%s: log is %d bytes (present %t) with %d staged, want %d bytes (present %t) with %d staged",
			step, len(got), got != nil, s.f.Staged(s.id), len(want), want != nil, staged)
	}
}

// publish stages, writes and publishes one clean log.
func (s sessionLogs) publish(step string, data []byte) {
	s.t.Helper()
	staged := s.stage(step)
	if staged == nil {
		return
	}
	if failure := staged.Write(data); failure != "" {
		s.t.Errorf("%s: write = %s", step, failure)
	}
	if failure := staged.Publish(); failure != "" {
		s.t.Errorf("%s: publish = %s", step, failure)
	}
	s.holds(step, data, 0)
}

type sessionListing struct {
	IDs      []string
	Sessions []port.RegisteredSession
	Failures [3]port.Failure
}

func (s sessionLogs) listing() sessionListing {
	var l sessionListing
	ctx := context.Background()
	l.IDs, l.Failures[0] = s.f.Storage.List(ctx, s.f.Candidate)
	sort.Strings(l.IDs)
	admission, failure := s.f.Storage.Admission(ctx, s.f.Candidate)
	l.Failures[1] = failure
	if admission != nil {
		l.Sessions, l.Failures[2] = admission.Sessions()
		admission.Close()
	}
	return l
}

// SessionLogsContract runs unchanged against the memory fake and the protected
// disk registry. Every refusal has a healthy control beside it, and no outcome
// leaves a staged temporary behind.
func SessionLogsContract(t Reporter, fixture SessionLogsFixture) {
	t.Helper()
	ctx := context.Background()
	storage, logs, c := fixture.Storage, fixture.Logs, fixture.Candidate
	id, absent := strings.Repeat("a", 64), strings.Repeat("b", 64)
	phase, end := context.WithCancel(ctx)
	defer end()
	s := sessionLogs{t: t, f: fixture, id: id, phase: phase}
	if staged, failure := logs.Stage(ctx, c, id); staged != nil || failure != "" {
		t.Errorf("no registration at all: stage = %v, %s", staged, failure)
	}
	lease, failure := storage.Register(ctx, c, id, []byte("record"))
	if failure != "" {
		t.Errorf("registration: %s", failure)
		return
	}
	defer lease.Close()
	if staged, failure := logs.Stage(ctx, c, "session"); staged != nil || failure != "state.invalid" {
		t.Errorf("malformed ID: stage = %v, %s", staged, failure)
	}
	ended, cancel := context.WithCancel(ctx)
	cancel()
	if staged, failure := logs.Stage(ended, c, id); staged != nil || failure != "state.unavailable" {
		t.Errorf("ended context: stage = %v, %s", staged, failure)
	}
	if staged, failure := logs.Stage(ctx, c, absent); staged != nil || failure != "" {
		t.Errorf("unregistered session: stage = %v, %s", staged, failure)
	}
	s.holds("refused stages", nil, 0)

	// The session stays open for writing throughout, as its controller holds it.
	held, failure := storage.Open(ctx, c, id, false)
	if failure != "" || held == nil {
		t.Errorf("open for writing: %s", failure)
		return
	}
	defer held.Close()
	before := s.listing()
	if before.Failures != [3]port.Failure{} || !reflect.DeepEqual(before.IDs, []string{id}) || len(before.Sessions) != 1 {
		t.Errorf("listing before any log: %#v", before)
	}

	first := s.stage("first")
	if first == nil {
		return
	}
	s.holds("staged, unwritten", nil, 1)
	body := append(bytes.Repeat([]byte("0123456789abcde\n"), 65536), "tail\n"...)
	for _, part := range [][]byte{body[:400000], body[400000 : len(body)-5], body[len(body)-5:]} {
		if failure := first.Write(part); failure != "" {
			t.Errorf("write of %d bytes: %s", len(part), failure)
		}
	}
	s.holds("written, unpublished", nil, 1)
	if failure := first.Publish(); failure != "" {
		t.Errorf("publish over 1 MiB: %s", failure)
	}
	s.holds("published over 1 MiB", body, 0)
	if failure := first.Write([]byte("late")); failure != "state.write" {
		t.Errorf("write after publish: %s", failure)
	}
	if failure := first.Publish(); failure != "state.conflict" {
		t.Errorf("publish after publish: %s", failure)
	}
	first.Close()
	s.holds("close after publish", body, 0)

	if after := s.listing(); !reflect.DeepEqual(after, before) {
		t.Errorf("listing changed beside a log: %#v, was %#v", after, before)
	}
	if data, failure := held.Read("record.json"); failure != "" || string(data) != "record" || !held.Unchanged() {
		t.Errorf("held session beside a log: %q %s, unchanged %t", data, failure, held.Unchanged())
	}
	if failure := held.Publish("record.json", []byte("record"), []byte("second")); failure != "" {
		t.Errorf("record publication beside a log: %s", failure)
	}
	if data, failure := held.Read("record.json"); failure != "" || string(data) != "second" {
		t.Errorf("record after publication: %q %s", data, failure)
	}

	// Both handles are staged before either publishes: the last rename wins whole.
	earlier, later := s.stage("earlier of two"), s.stage("later of two")
	if earlier == nil || later == nil {
		return
	}
	s.holds("two staged", body, 2)
	if a, b := earlier.Write([]byte("earlier\n")), later.Write([]byte("later handle\n")); a != "" || b != "" {
		t.Errorf("two writes: %s %s", a, b)
	}
	if failure := later.Publish(); failure != "" {
		t.Errorf("later publish: %s", failure)
	}
	s.holds("later published first", []byte("later handle\n"), 1)
	if failure := earlier.Publish(); failure != "" {
		t.Errorf("earlier publish: %s", failure)
	}
	s.holds("earlier published last", []byte("earlier\n"), 0)

	closed := s.stage("closed")
	if closed == nil {
		return
	}
	if failure := closed.Write([]byte("discarded\n")); failure != "" {
		t.Errorf("write before close: %s", failure)
	}
	closed.Close()
	s.holds("closed unpublished", []byte("earlier\n"), 0)
	if failure := closed.Write([]byte("late")); failure != "state.write" {
		t.Errorf("write after close: %s", failure)
	}
	if failure := closed.Publish(); failure != "state.conflict" {
		t.Errorf("publish after close: %s", failure)
	}
	closed.Close()
	s.holds("second close", []byte("earlier\n"), 0)

	s.publish("empty", []byte{})
	visible := []byte("kept\n")
	s.publish("kept", visible)

	for _, fault := range []string{"write", "file fsync", "atomic publication", "parent fsync"} {
		data := []byte("under " + fault + "\n")
		fixture.Fault(fault)
		staged := s.stage(fault)
		if staged == nil {
			fixture.Fault("")
			return
		}
		wrote := staged.Write(data)
		if fault == "write" {
			if second := staged.Write(data); wrote != "state.write" || second != "state.write" {
				t.Errorf("write fault: writes = %s, %s", wrote, second)
			}
		} else if wrote != "" {
			t.Errorf("%s: write = %s", fault, wrote)
		}
		if failure := staged.Publish(); failure != "state.write" {
			t.Errorf("%s: publish = %s", fault, failure)
		}
		if fault == "parent fsync" {
			// The rename happened: the new bytes are visible, without a durability claim.
			visible = data
		}
		s.holds(fault, visible, 0)
		if fault == "write" {
			unpublished := s.stage("second handle under the write fault")
			if unpublished == nil {
				fixture.Fault("")
				return
			}
			if failure := unpublished.Write(data); failure != "state.write" {
				t.Errorf("second handle's write under the fault: %s", failure)
			}
			unpublished.Close()
			s.holds("second handle closed under the write fault", visible, 0)
		}
		fixture.Fault("")
		visible = []byte("retry after " + fault + "\n")
		s.publish("retry after "+fault, visible)
	}

	// A handle outlives the call that staged it, never the phase it belongs to.
	late := s.stage("late")
	if late == nil {
		return
	}
	if failure := late.Write([]byte("after the phase\n")); failure != "" {
		t.Errorf("write before the phase ended: %s", failure)
	}
	end()
	if failure := late.Publish(); failure != "state.path" {
		t.Errorf("publish after the phase ended: %s", failure)
	}
	s.holds("phase ended", visible, 0)
}
