package disk

import (
	"slices"
	"strconv"
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

// path is the protected destination this fixture's staged log publishes to.
func (f refusalFixture) path() string {
	return "sessions/" + f.key + "/" + f.id + "/egress-refusals.jsonl"
}

// copyAll writes each part, commits and closes, expecting a publication whose
// derived report names this destination and, at most, a real sequence gap.
func (f refusalFixture) copyAll(t *testing.T, parts ...string) sp.RefusalReport {
	t.Helper()
	copy := f.begin(t)
	defer copy.Close()
	for _, part := range parts {
		if n, err := copy.Write([]byte(part)); n != len(part) || err != nil {
			t.Fatalf("write of %d bytes = %d, %v", len(part), n, err)
		}
	}
	report, failure := copy.Commit()
	if failure != "" || (report.Outcome != "" && report.Outcome != sp.RefusalLogGap) || report.Path != f.path() {
		t.Fatalf("commit = %#v, %s", report, failure)
	}
	return report
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
	derived := sp.RefusalReport{Records: 1, Path: f.path()}
	sf.RefusalLogContract(t, sf.RefusalLogFixture{
		SessionID: f.id,
		Log:       f.log,
		Read:      func(id string) []byte { data, _ := f.files.SessionLog(f.key, id); return data },
		Fault:     func(name string) { f.files.Fault = fault[name] },
		Report:    func(sp.RefusalReport) {},
	}, []byte(sf.ProxyStdout), []byte(lines[1]), []byte(lines[0]+replacement), []byte(replacement), derived)
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
	for _, tc := range sf.ReadyRecords() {
		t.Run(tc.Name, func(t *testing.T) {
			if !strings.HasSuffix(tc.Line, "\n") {
				f := refusalLogFixture()
				f.copyAll(t, refusal+tc.Line)
				if got := string(f.stored()); got != refusal+tc.Line {
					t.Fatalf("unterminated evidence changed: %q", got)
				}
				return
			}
			// Each row stands between two refusals, so a dropped line loses nothing beside it.
			want := refusal + tc.Line + refusal
			if tc.Accepted {
				want = refusal + refusal
			}
			f := refusalLogFixture()
			f.copyAll(t, refusal+tc.Line+refusal)
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
			report, failure := copy.Commit()
			if failure != "" || (report.Outcome != "" && report.Outcome != sp.RefusalLogGap) || (report.Outcome == sp.RefusalLogGap) != (len(report.Gaps) > 0) || report.Path != f.path() {
				t.Fatalf("commit = %#v, %s", report, failure)
			}
			last := written[strings.LastIndexByte(written, '\n')+1:]
			want := stagedAfter(written+"\n", tc.ready...)
			want = strings.TrimSuffix(want, "\n")
			if report.Records != expectedRecords(want) {
				t.Fatalf("commit counted %d records in %d stored bytes", report.Records, len(want))
			}
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

// expectedRecords counts the complete lines a copy stores, plus a held
// unterminated one Commit publishes as it is. A line already flushed past the
// ready bound is stored as it arrives; only its completing newline counts it.
func expectedRecords(stored string) uint64 {
	records := uint64(strings.Count(stored, "\n"))
	if tail := stored[strings.LastIndexByte(stored, '\n')+1:]; tail != "" && len(tail) <= 2048 {
		records++
	}
	return records
}

// TestRefusalLogDerivesGaps exercises the sequence judgement over the whole
// stream, ready included: a missing initial event is a gap in the report even
// though the published file never holds readiness, log-full counts as a record,
// and an empty valid copy reports a clean zero.
func TestRefusalLogDerivesGaps(t *testing.T) {
	ready := strings.SplitAfter(sf.ProxyStdout, "\n")[0]
	refusal := strings.SplitAfter(sf.ProxyStdout, "\n")[1]
	at := func(seq int) string {
		return strings.Replace(refusal, `"seq":2`, `"seq":`+strconv.Itoa(seq), 1)
	}
	full := `{"schema":1,"seq":3,"time":"2026-11-16T00:00:00Z","event":"log-full"}` + "\n"
	for _, tc := range []struct {
		name    string
		input   string
		stored  string
		gaps    []sp.RefusalGap
		records uint64
	}{
		{"clean consecutive", ready + refusal + at(3), refusal + at(3), nil, 2},
		{"initial ready missing", refusal + at(3), refusal + at(3), []sp.RefusalGap{{First: 1, Last: 1}}, 2},
		{"middle gap", ready + at(2) + at(5) + at(6), at(2) + at(5) + at(6), []sp.RefusalGap{{First: 3, Last: 4}}, 3},
		{"two gaps", at(2) + at(5) + at(7), at(2) + at(5) + at(7), []sp.RefusalGap{{First: 1, Last: 1}, {First: 3, Last: 4}, {First: 6, Last: 6}}, 3},
		{"log-full counts", ready + refusal + full, refusal + full, nil, 2},
		{"empty copy", "", "", nil, 0},
		{"unparsed lines keep their bytes", "not JSON\n" + at(2), "not JSON\n" + at(2), []sp.RefusalGap{{First: 1, Last: 1}}, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := refusalLogFixture()
			report := f.copyAll(t, tc.input)
			if got := string(f.stored()); got != tc.stored {
				t.Fatalf("stored %q, want %q", got, tc.stored)
			}
			want := sp.RefusalReport{Records: tc.records, Path: f.path(), Gaps: tc.gaps}
			if len(want.Gaps) > 0 {
				want.Outcome = sp.RefusalLogGap
			}
			if report.Outcome != want.Outcome || report.Records != want.Records || report.Path != want.Path || !slices.Equal(report.Gaps, want.Gaps) {
				t.Fatalf("report = %#v, want %#v", report, want)
			}
		})
	}
}

// TestRefusalLogReadDerivesEvidence reads a session's committed evidence back
// through the same port: pending before any copy, stored or gapped from the
// published pair, unknown when the report cannot confirm the log beside it.
func TestRefusalLogReadDerivesEvidence(t *testing.T) {
	ready := strings.SplitAfter(sf.ProxyStdout, "\n")[0]
	refusal := strings.SplitAfter(sf.ProxyStdout, "\n")[1]
	f := refusalLogFixture()
	pending, failure := f.log.Read(t.Context(), f.id)
	if failure != "" || pending.State != "pending" || pending.Records != nil || pending.Path != f.path() || !slices.Equal(pending.Missing, []sp.RefusalGap{}) {
		t.Fatalf("unread evidence = %#v, %s", pending, failure)
	}
	f.copyAll(t, ready+refusal)
	evidence, failure := f.log.Read(t.Context(), f.id)
	records := uint64(1)
	if failure != "" || evidence.State != "stored" || evidence.Path != f.path() || evidence.Records == nil || *evidence.Records != records || len(evidence.Missing) != 0 {
		t.Fatalf("stored evidence = %#v, %s", evidence, failure)
	}
	gapped := f.copyAll(t, strings.Replace(refusal, `"seq":2`, `"seq":5`, 1))
	if gapped.Outcome != sp.RefusalLogGap || len(gapped.Gaps) != 1 || gapped.Gaps[0] != (sp.RefusalGap{First: 1, Last: 4}) {
		t.Fatalf("gapped copy = %#v", gapped)
	}
	evidence, failure = f.log.Read(t.Context(), f.id)
	records = 1
	if failure != "" || evidence.State != "gapped" || evidence.Records == nil || *evidence.Records != records || !slices.Equal(evidence.Missing, gapped.Gaps) {
		t.Fatalf("gapped evidence = %#v, %s", evidence, failure)
	}
	// A log without its report claims nothing: an interrupted publication is
	// unknown, never a stored zero.
	log, report := f.files.SessionReport(f.key, f.id)
	f.files.Sessions[f.key+"/"+f.id].Report = nil
	evidence, failure = f.log.Read(t.Context(), f.id)
	if failure != "" || evidence.State != "unknown" || evidence.Records != nil {
		t.Fatalf("unconfirmed log = %#v, %s", evidence, failure)
	}
	f.files.Sessions[f.key+"/"+f.id].Report = report
	f.files.Sessions[f.key+"/"+f.id].Log = append(log, []byte("changed\n")...)
	evidence, failure = f.log.Read(t.Context(), f.id)
	if failure != "" || evidence.State != "unknown" || evidence.Records != nil {
		t.Fatalf("stale report = %#v, %s", evidence, failure)
	}
	// A retained sidecar without its committed log is required evidence gone
	// missing: unknown, and a failed read, never a successful cleanup claim.
	f.files.Sessions[f.key+"/"+f.id].Log = nil
	evidence, failure = f.log.Read(t.Context(), f.id)
	if failure != sp.Incomplete || evidence.Path != f.path() || evidence.State != "unknown" || evidence.Records != nil || evidence.Missing == nil || len(evidence.Missing) != 0 {
		t.Fatalf("missing committed log = %#v, %s", evidence, failure)
	}
}

func TestRefusalLogReadInvalidMetadataAndUnavailableEvidence(t *testing.T) {
	f := refusalLogFixture()
	read := func(id string, want sp.Outcome) {
		t.Helper()
		report, failure := f.log.Read(t.Context(), id)
		if failure != want || report.State != "unknown" || report.Records != nil || report.Missing == nil || len(report.Missing) != 0 {
			t.Fatalf("read = %#v, %s; want unknown, %s", report, failure, want)
		}
	}
	read("invalid", sp.Invalid)
	f.log.Store.Input.Root = "relative"
	read(f.id, sp.Refused)
	f = refusalLogFixture()
	f.files.Failure = "state.unavailable"
	read(f.id, sp.Incomplete)
	f.files.Failure = ""
	f.copyAll(t, `{"seq":0}`+"\n"+`{"seq":-1}`+"\n"+`{"seq":1.5}`+"\n")
	entry := f.files.Sessions[f.key+"/"+f.id]
	good := string(entry.Report)
	digestAt := strings.Index(good, `"sha256":"`) + len(`"sha256":"`)
	for _, bad := range []string{
		"{", strings.Replace(good, `"schema":1`, `"schema":2`, 1),
		strings.Replace(good, `"records":3`, `"records":null`, 1),
		good[:digestAt] + good[digestAt+1:], good[:digestAt] + "x" + good[digestAt+1:],
	} {
		entry.Report = []byte(bad)
		read(f.id, "")
	}
}
