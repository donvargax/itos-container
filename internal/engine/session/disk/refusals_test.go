package disk

import (
	"context"
	"slices"
	"strings"
	"testing"

	ap "github.com/donvargax/itos-container/internal/engine/approval/port"
	af "github.com/donvargax/itos-container/internal/engine/approval/port/porttest"
	rp "github.com/donvargax/itos-container/internal/engine/readiness/port"
	sp "github.com/donvargax/itos-container/internal/engine/session/port"
	sf "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
)

// refusalFixture is a RefusalLog over the approval fake, with one registered
// session under the key the registry's own lookup gives.
type refusalFixture struct {
	log   RefusalLog
	files *af.Registry
	host  *af.Host
	key   string
	id    string
}

func refusalLogFixture() refusalFixture {
	b, _, _ := sf.Fixture()
	id, key := strings.Repeat("b", 64), registryKey(b.Checkout.Root, b.UID)
	files := &af.Registry{Sessions: map[string]*af.SessionFiles{key + "/" + id: {Data: []byte("record")}}}
	host := &af.Host{UID: b.UID, GID: b.GID}
	store := Store{Host: host, Input: rp.Input{Root: b.Checkout.Root, OS: "linux", Architecture: "amd64"}}
	return refusalFixture{log: RefusalLog{Store: store, Storage: files}, files: files, host: host, key: key, id: id}
}

// stored is the session's published log, nil when there is none.
func (f refusalFixture) stored() []byte { data, _ := f.files.SessionLog(f.key, f.id); return data }

// staged counts the session's handles not yet released by Publish or Close.
func (f refusalFixture) staged() int { _, staged := f.files.SessionLog(f.key, f.id); return staged }

// handed is what the staged log was given by the Writes the fake accepted.
func (f refusalFixture) handed() string {
	var data []byte
	for _, event := range f.files.Events {
		if event.Operation == "write" && !event.Failed {
			data = append(data, event.Data...)
		}
	}
	return string(data)
}

// operations names the fake's events in order.
func (f refusalFixture) operations() []string {
	names := []string{}
	for _, event := range f.files.Events {
		names = append(names, event.Operation)
	}
	return names
}

func (f refusalFixture) begin(t *testing.T) sp.RefusalCopy {
	t.Helper()
	copy, failure := f.log.Begin(t.Context(), f.id)
	if failure != "" || copy == nil {
		t.Fatalf("begin = %v, %s", copy, failure)
	}
	return copy
}

// copyAll writes each part, commits and closes, expecting a clean publication.
func (f refusalFixture) copyAll(t *testing.T, parts ...string) {
	t.Helper()
	copy := f.begin(t)
	defer copy.Close()
	for _, part := range parts {
		if n, err := copy.Write([]byte(part)); n != len(part) || err != nil {
			t.Fatalf("write of %d bytes = %d, %v", len(part), n, err)
		}
	}
	if report, failure := copy.Commit(); failure != "" || report.Outcome != "" || report.Gaps != nil {
		t.Fatalf("commit = %#v, %s", report, failure)
	}
}

// sessionNamed hands Begin a registered session in place of the name the
// shared contract passes, which no registry holds.
type sessionNamed struct {
	log sp.RefusalLog
	id  string
}

func (l sessionNamed) Begin(ctx context.Context, session string) (sp.RefusalCopy, sp.Outcome) {
	if session == "session" {
		session = l.id
	}
	return l.log.Begin(ctx, session)
}

func TestRefusalLogContract(t *testing.T) {
	f := refusalLogFixture()
	held, failure := f.files.Open(t.Context(), ap.Candidate{Project: f.key}, f.id, false)
	if failure != "" || held == nil {
		t.Fatal("the session must be held open for writing throughout", failure)
	}
	defer held.Close()
	if copy, failure := f.log.Begin(t.Context(), "session"); copy != nil || failure != sp.Invalid {
		t.Fatal("the adapter accepted the contract's own session name", failure)
	}
	lines := strings.SplitAfter(sf.ProxyStdout, "\n")
	replacement := strings.Replace(lines[1], "not-listed", "port", 1)
	fault := map[string]string{"write": "write", "commit": "file fsync", "": ""}
	sf.RefusalLogContract(t, sf.RefusalLogFixture{
		Log:    sessionNamed{f.log, f.id},
		Read:   func(string) []byte { return f.stored() },
		Fault:  func(name string) { f.files.Fault = fault[name] },
		Report: func(sp.RefusalReport) {},
	}, []byte(sf.ProxyStdout), []byte(lines[1]), []byte(lines[0]+replacement), []byte(replacement), sp.RefusalReport{})
	if f.staged() != 0 {
		t.Fatal("the contract left a staged handle", f.staged())
	}
	if data, failure := held.Read("record.json"); failure != "" || string(data) != "record" || !held.Unchanged() {
		t.Fatal("the held session changed beside its log", failure)
	}
}

func TestRefusalLogDropsOnlyReady(t *testing.T) {
	ready := strings.SplitAfter(sf.ProxyStdout, "\n")[0]
	refusal := strings.SplitAfter(sf.ProxyStdout, "\n")[1]
	body := strings.TrimSuffix(ready, "\n")
	for _, tc := range []struct {
		name, line string
		dropped    bool
	}{
		{"ready", ready, true},
		{"reordered keys", `{"event":"ready","time":"2026-11-09T12:00:00Z","seq":1,"schema":1}` + "\n", true},
		{"fractional seconds", strings.Replace(ready, "12:00:00Z", "12:00:00.123456789Z", 1), true},
		{"spaces inside", `{ "schema" : 1 , "seq" : 1 , "time" : "2026-11-09T12:00:00Z" , "event" : "ready" }` + "\n", true},
		{"leading spaces to 2,048 bytes", strings.Repeat(" ", 2048-len(body)) + ready, true},
		{"leading spaces to 2,049 bytes", strings.Repeat(" ", 2049-len(body)) + ready, false},
		{"stderr looking stdout", "failure\n", false},
		{"extra key", `{"schema":1,"seq":1,"time":"2026-11-09T12:00:00Z","event":"ready","extra":0}` + "\n", false},
		{"wrong seq", strings.Replace(ready, `"seq":1`, `"seq":2`, 1), false},
		{"wrong event", strings.Replace(ready, "ready", "refused", 1), false},
		{"wrong time", strings.Replace(ready, "Z", "+00:00", 1), false},
		{"long", strings.Repeat(" ", 2049) + "\n", false},
		{"trailing space", body + " \n", false},
		{"trailing carriage return", body + "\r\n", false},
		{"trailing object", body + "{}\n", false},
		{"repeated key", `{"schema":1,"schema":1,"seq":1,"time":"2026-11-09T12:00:00Z","event":"ready"}` + "\n", false},
		{"repeated key in place of one", `{"schema":1,"schema":1,"time":"2026-11-09T12:00:00Z","event":"ready"}` + "\n", false},
		{"schema 1.0", strings.Replace(ready, `"schema":1`, `"schema":1.0`, 1), false},
		{"schema 2", strings.Replace(ready, `"schema":1`, `"schema":2`, 1), false},
		{"schema as text", strings.Replace(ready, `"schema":1`, `"schema":"1"`, 1), false},
		{"seq as text", strings.Replace(ready, `"seq":1`, `"seq":"1"`, 1), false},
		{"event as number", strings.Replace(ready, `"ready"`, `1`, 1), false},
		{"time as number", strings.Replace(ready, `"2026-11-09T12:00:00Z"`, `1`, 1), false},
		{"time that is no date", strings.Replace(ready, "2026-11-09T12:00:00Z", "soon, Z", 1), false},
		{"no event", `{"schema":1,"seq":1,"time":"2026-11-09T12:00:00Z"}` + "\n", false},
		{"another key in place of schema", `{"other":1,"seq":1,"time":"2026-11-09T12:00:00Z","event":"ready"}` + "\n", false},
		{"array", `[1,1,"2026-11-09T12:00:00Z","ready"]` + "\n", false},
		{"unclosed object", strings.TrimSuffix(body, "}") + "\n", false},
		{"missing value", `{"schema":1,"seq":1,"time":"2026-11-09T12:00:00Z","event":}` + "\n", false},
		{"key without value", `{"schema":1,"seq":1,"time":"2026-11-09T12:00:00Z","event"}` + "\n", false},
		{"unquoted key", `{schema:1,"seq":1,"time":"2026-11-09T12:00:00Z","event":"ready"}` + "\n", false},
		{"empty line", "\n", false},
		{"empty object", "{}\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if strings.Count(tc.line, "\n") != 1 || !strings.HasSuffix(tc.line, "\n") {
				t.Fatal("each row is one terminated line")
			}
			// Each row stands between two refusals, so a dropped line loses nothing beside it.
			want := refusal + tc.line + refusal
			if tc.dropped {
				want = refusal + refusal
			}
			f := refusalLogFixture()
			f.copyAll(t, refusal+tc.line+refusal)
			if got := string(f.stored()); got != want {
				t.Fatalf("stored %d bytes, want %d: %q", len(got), len(want), got)
			}
		})
	}
	t.Run("unterminated", func(t *testing.T) {
		for _, last := range []string{"{", body} {
			f := refusalLogFixture()
			f.copyAll(t, ready+refusal+last)
			if got := string(f.stored()); got != refusal+last {
				t.Fatalf("a last line with no newline was not stored as it is: %q", got)
			}
		}
	})
}

// stagedAfter is what the staged log must hold once input was written: every
// complete line that is not ready, and an unfinished one only once it is too
// long to be ready, since the copy holds at most 2,048 bytes of a line.
func stagedAfter(input string, ready ...string) string {
	lines := strings.SplitAfter(input, "\n")
	var out strings.Builder
	for _, line := range lines[:len(lines)-1] {
		if !slices.Contains(ready, line) {
			out.WriteString(line)
		}
	}
	if last := lines[len(lines)-1]; len(last) > 2048 {
		out.WriteString(last)
	}
	return out.String()
}

func TestRefusalLogStreamsLines(t *testing.T) {
	ready := strings.SplitAfter(sf.ProxyStdout, "\n")[0]
	refusal := strings.SplitAfter(sf.ProxyStdout, "\n")[1]
	padded := strings.Repeat(" ", 2049-len(ready)) + ready
	long := strings.Repeat(" ", 2049) + ready
	input := ready + refusal + "not JSON\n" + strings.Replace(refusal, `"seq":2`, `"seq":4`, 1)
	bytewise := func(s string) []string { return strings.Split(s, "") }
	for _, tc := range []struct {
		name  string
		parts []string
		ready []string
	}{
		{"whole log at once", []string{input}, []string{ready}},
		{"one byte to a Write", bytewise(input), []string{ready}},
		{"split inside the ready line", []string{input[:20], input[20:]}, []string{ready}},
		{"split at a newline", []string{ready, input[len(ready):]}, []string{ready}},
		{"split before a newline", []string{ready[:len(ready)-1], input[len(ready)-1:]}, []string{ready}},
		{"empty Writes between", []string{"", ready, "", refusal, ""}, []string{ready}},
		{"unterminated last line", []string{input, "{\"schema\":1"}, []string{ready}},
		{"unterminated ready record", []string{refusal, strings.TrimSuffix(ready, "\n")}, nil},
		{"ready at 2,048 bytes, newline apart", []string{refusal, strings.TrimSuffix(padded, "\n"), "\n", refusal}, []string{padded}},
		{"ready at 2,048 bytes, bytewise", bytewise(refusal + padded + refusal), []string{padded}},
		{"ready behind 2,049 spaces, split at the record", []string{strings.Repeat(" ", 2049), ready, refusal}, nil},
		{"ready behind 2,049 spaces, bytewise", bytewise(long + refusal), nil},
		{"long line in pieces", []string{strings.Repeat("x", 2048), "y", strings.Repeat("z", 3000), "tail\n", ready, refusal}, []string{ready}},
		{"long unterminated line", []string{refusal, strings.Repeat("x", 5000)}, nil},
		{"empty copy", nil, nil},
		{"ready only", []string{ready}, []string{ready}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := refusalLogFixture()
			copy := f.begin(t)
			defer copy.Close()
			written := ""
			for _, part := range tc.parts {
				calls := len(f.files.Events)
				if n, err := copy.Write([]byte(part)); n != len(part) || err != nil {
					t.Fatalf("write of %d bytes = %d, %v", len(part), n, err)
				}
				before := stagedAfter(written, tc.ready...)
				written += part
				want := stagedAfter(written, tc.ready...)
				if got := f.handed(); got != want {
					t.Fatalf("after %d bytes the staged log holds %d bytes, want %d", len(written), len(got), len(want))
				}
				// A Write with nothing to store makes no call; one with something makes one.
				if made, owed := len(f.files.Events)-calls, min(len(want)-len(before), 1); made != owed {
					t.Fatalf("after %d bytes the Write made %d calls to the staged log, want %d", len(written), made, owed)
				}
				if held := len(copy.(*refusalCopy).held); held > 2048 || held != len(written)-len(want)-dropped(written, tc.ready) {
					t.Fatalf("after %d bytes the copy holds %d", len(written), held)
				}
			}
			if f.stored() != nil {
				t.Fatal("a log is visible before Commit")
			}
			calls := f.operations()
			if report, failure := copy.Commit(); failure != "" || report.Outcome != "" || report.Gaps != nil {
				t.Fatalf("commit = %#v, %s", report, failure)
			}
			last := written[strings.LastIndexByte(written, '\n')+1:]
			want := stagedAfter(written+"\n", tc.ready...)
			want = strings.TrimSuffix(want, "\n")
			if slices.Contains(tc.ready, last+"\n") {
				t.Fatal("fixture: an unterminated line is never a ready record")
			}
			if got := f.stored(); got == nil || string(got) != want {
				t.Fatalf("stored %d bytes (present %t), want %d", len(got), got != nil, len(want))
			}
			// Commit stores a held line, when there is one, and then publishes once.
			published := []string{"file fsync", "atomic publication", "parent fsync"}
			if len(last) > 0 && len(last) <= 2048 {
				published = slices.Insert(published, 0, "write")
			}
			if got := f.operations()[len(calls):]; !slices.Equal(got, published) {
				t.Fatalf("commit made the calls %v, want %v", got, published)
			}
			if f.staged() != 0 {
				t.Fatal("staged handle left after Commit")
			}
		})
	}
	t.Run("more than 1 MiB of records", func(t *testing.T) {
		var log strings.Builder
		log.WriteString(ready)
		for seq := 2; log.Len() <= 1024*1024+len(ready); seq++ {
			log.WriteString(strings.Replace(refusal, `"seq":2`, `"seq":`+strings.Repeat("7", seq%9+1), 1))
		}
		all := log.String()
		var parts []string
		for rest := all; rest != ""; {
			n := min(len(rest), 4093)
			parts, rest = append(parts, rest[:n]), rest[n:]
		}
		f := refusalLogFixture()
		f.copyAll(t, parts...)
		if got := string(f.stored()); len(got) <= 1024*1024 || got != strings.TrimPrefix(all, ready) {
			t.Fatalf("stored %d of %d bytes", len(got), len(all)-len(ready))
		}
	})
}

// dropped counts the bytes of the ready lines that input completes.
func dropped(input string, ready []string) int {
	n := 0
	for _, line := range strings.SplitAfter(input, "\n") {
		if slices.Contains(ready, line) {
			n += len(line)
		}
	}
	return n
}
