package porttest

import (
	"context"
	"github.com/donvargax/itos-container/internal/engine/session/port"
	"reflect"
	"sync"
)

// RefusalLogFixture lets a writer derive reports while the memory fake scripts them.
// Fault changes only publication mechanics, never the expected bytes or report.
type RefusalLogFixture struct {
	Log    port.RefusalLog
	Read   func(string) []byte
	Fault  func(string)
	Report func(port.RefusalReport)
}

func RefusalLogContract(t Reporter, fixture RefusalLogFixture, input, stored, replacementInput, replacementStored []byte, report port.RefusalReport) {
	t.Helper()
	if string(input) == string(replacementInput) || string(stored) == string(replacementStored) {
		t.Errorf("refusal log fixture needs distinguishable input and stored copies")
		return
	}
	ctx := context.Background()
	const session = "session"
	fixture.Report(report)
	copy, f := fixture.Log.Begin(ctx, session)
	if f != "" || copy == nil {
		t.Errorf("begin: %s", f)
		return
	}
	if n, e := copy.Write(input); n != len(input) || e != nil {
		t.Errorf("write: %d %v", n, e)
	}
	got, f := copy.Commit()
	copy.Close()
	if f != "" || !reflect.DeepEqual(got, report) || string(fixture.Read(session)) != string(stored) {
		t.Errorf("publication: %#v %s %q", got, f, fixture.Read(session))
	}
	if _, e := copy.Write(input); e == nil {
		t.Errorf("write after commit accepted")
	}
	copy, f = fixture.Log.Begin(ctx, session)
	if f != "" {
		t.Errorf("second begin: %s", f)
		return
	}
	_, _ = copy.Write([]byte("discard"))
	copy.Close()
	if string(fixture.Read(session)) != string(stored) {
		t.Errorf("uncommitted copy replaced bytes")
	}
	if _, e := copy.Write(input); e == nil {
		t.Errorf("write after close accepted")
	}
	for _, fault := range []string{"write", "commit"} {
		fixture.Fault(fault)
		copy, f = fixture.Log.Begin(ctx, session)
		if f != "" {
			t.Errorf("begin with %s fault: %s", fault, f)
			return
		}
		_, err := copy.Write(replacementInput)
		got, f = copy.Commit()
		copy.Close()
		if fault == "write" && err == nil {
			t.Errorf("write fault ignored")
		}
		if f == "" || !reflect.DeepEqual(got, port.RefusalReport{}) || string(fixture.Read(session)) != string(stored) {
			t.Errorf("%s fault published or lost earlier bytes: %#v %s", fault, got, f)
		}
	}
	fixture.Fault("")
	copy, f = fixture.Log.Begin(ctx, session)
	if f != "" {
		t.Errorf("replacement begin: %s", f)
		return
	}
	if n, err := copy.Write(replacementInput); n != len(replacementInput) || err != nil {
		t.Errorf("replacement write: %d %v", n, err)
	}
	got, f = copy.Commit()
	copy.Close()
	if f != "" || !reflect.DeepEqual(got, report) || string(fixture.Read(session)) != string(replacementStored) {
		t.Errorf("replacement lost bytes: %s", f)
	}
	copy, f = fixture.Log.Begin(ctx, session)
	if f != "" {
		t.Errorf("concurrent begin: %s", f)
		return
	}
	// The committed result must describe the bytes staged before Commit starts,
	// even when Close races it after a caller's timeout. Keep the competing Write
	// empty so both publication outcomes have an exact, adapter-independent value.
	if n, err := copy.Write(input); n != len(input) || err != nil {
		t.Errorf("concurrent prewrite: %d %v", n, err)
	}
	var wg sync.WaitGroup
	for _, fn := range []func(){func() { _, _ = copy.Write(nil) }, func() { got, f = copy.Commit() }, copy.Close} {
		wg.Go(fn)
	}
	wg.Wait()
	copy.Close()
	if f == "" {
		if !reflect.DeepEqual(got, report) || string(fixture.Read(session)) != string(stored) {
			t.Errorf("successful concurrent commit lost report or staged bytes: %#v %q", got, fixture.Read(session))
		}
	} else if !reflect.DeepEqual(got, port.RefusalReport{}) || string(fixture.Read(session)) != string(replacementStored) {
		t.Errorf("failed concurrent commit published a report or changed earlier bytes: %#v %s %q", got, f, fixture.Read(session))
	}
	if _, e := copy.Write(input); e == nil {
		t.Errorf("closed concurrent copy writable")
	}
}
