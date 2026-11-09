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

func RefusalLogContract(t Reporter, fixture RefusalLogFixture, input, stored []byte, report port.RefusalReport) {
	t.Helper()
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
		_, err := copy.Write(input)
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
	_, _ = copy.Write(input)
	_, f = copy.Commit()
	copy.Close()
	if f != "" || string(fixture.Read(session)) != string(stored) {
		t.Errorf("replacement lost bytes: %s", f)
	}
	copy, f = fixture.Log.Begin(ctx, session)
	if f != "" {
		t.Errorf("concurrent begin: %s", f)
		return
	}
	var wg sync.WaitGroup
	for _, fn := range []func(){func() { _, _ = copy.Write(input) }, func() { _, _ = copy.Commit() }, copy.Close} {
		wg.Go(fn)
	}
	wg.Wait()
	copy.Close()
	if _, e := copy.Write(input); e == nil {
		t.Errorf("closed concurrent copy writable")
	}
}
