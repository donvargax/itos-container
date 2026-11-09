package porttest

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/donvargax/itos-container/internal/engine/session/port"
)

func anchorFixture() (port.Binding, port.Record) {
	b, _, _ := Fixture()
	b.Checkout.Device, b.Checkout.Inode, b.Workspace = "2049", "1701", "/workspace"
	r := b.Intent(strings.Repeat("1", 64))
	r.Schema, r.Stage = 2, "reserved"
	r.Anchor = port.Anchor{Session: strings.Repeat("2", 64), Name: "itos-container-anchor-" + r.NameToken(),
		Source: "/proc/123/fd/9", Device: b.Checkout.Device, Inode: b.Checkout.Inode, Stage: "intent"}
	return b, r
}

func anchorWorkspaceFixture(b port.Binding) *AnchoredWorkspace {
	return &AnchoredWorkspace{Descriptor: "/proc/123/fd/9", Mounts: map[int]port.MountSource{
		456: {Device: b.Checkout.Device, Inode: b.Checkout.Inode},
	}}
}

func TestAnchorMemoryContracts(t *testing.T) {
	b, r := anchorFixture()
	t.Run("engine records", func(t *testing.T) {
		AnchorConnectionContract(t, &AnchorResources{Created: "2026-11-07T17:00:00Z"}, r)
	})
	t.Run("pinned workspace", func(t *testing.T) {
		w := anchorWorkspaceFixture(b)
		AnchorWorkspaceContract(t, w, b, 456, func() { w.Changed = true })
	})
}

func TestAnchorResourcesRetainUnrelatedAndForeignRecords(t *testing.T) {
	_, r := anchorFixture()
	foreign := port.AnchorObservation{Name: r.Anchor.Name, Token: "foreign", Source: "/foreign"}
	unrelated := port.AnchorObservation{Name: "unrelated", Token: "another session"}
	a := &AnchorResources{Resources: map[string]port.AnchorObservation{foreign.Name: foreign, unrelated.Name: unrelated}}
	if failure := a.AllocateAnchor(context.Background(), r); failure != "" {
		t.Fatal(failure)
	}
	if o, status := a.InspectAnchor(context.Background(), r.Anchor.Name); status != port.Observed || o != foreign {
		t.Fatalf("allocation overwrote foreign evidence: %#v %v", o, status)
	}
	r.Anchor.Name = "independent-owned-anchor"
	if failure := a.AllocateAnchor(context.Background(), r); failure != "" {
		t.Fatal(failure)
	}
	a.RemoveAnchor(context.Background(), r.Anchor.Name)
	if o, status := a.InspectAnchor(context.Background(), foreign.Name); status != port.Observed || o != foreign {
		t.Fatalf("removal touched the foreign volume: %#v %v", o, status)
	}
	if o, status := a.InspectAnchor(context.Background(), unrelated.Name); status != port.Observed || o != unrelated {
		t.Fatalf("removal touched another volume: %#v %v", o, status)
	}
}

func TestAnchorResourcesCancelledOperations(t *testing.T) {
	_, r := anchorFixture()
	a := &AnchorResources{Created: "2026-11-07T17:00:00Z"}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if failure := a.AllocateAnchor(ctx, r); failure != port.MountUncertain || len(a.Resources) != 0 {
		t.Fatalf("cancelled allocation changed state: %s %#v", failure, a.Resources)
	}
	if failure := a.AllocateAnchor(context.Background(), r); failure != "" {
		t.Fatal(failure)
	}
	if o, status := a.InspectAnchor(ctx, r.Anchor.Name); status != port.Unavailable || o != (port.AnchorObservation{}) {
		t.Fatalf("cancelled inspection supplied authority: %#v %v", o, status)
	}
	a.RemoveAnchor(ctx, r.Anchor.Name)
	if _, status := a.InspectAnchor(context.Background(), r.Anchor.Name); status != port.Observed {
		t.Fatal("cancelled removal discarded anchor")
	}
}

func TestAnchoredWorkspaceDoesNotInventObservation(t *testing.T) {
	b, _ := anchorFixture()
	w := anchorWorkspaceFixture(b)
	if source, failure := w.Source(); failure != port.MountRefused || source != (port.MountSource{}) {
		t.Fatalf("unpinned source: %#v %s", source, failure)
	}
	if failure := w.Pin(b); failure != "" {
		t.Fatal(failure)
	}
	if mounted, failure := w.Mounted(789); failure != port.MountRefused || mounted != (port.MountSource{}) {
		t.Fatalf("unknown worker PID gained identity: %#v %s", mounted, failure)
	}
	// A detector must receive the actual fixture observation, even a mismatch.
	wrong := port.MountSource{Device: "99", Inode: "100"}
	w.Mounts[456] = wrong
	if mounted, failure := w.Mounted(456); failure != "" || mounted != wrong {
		t.Fatalf("mount observation was replaced by the validated identity: %#v %s", mounted, failure)
	}
	w.Close()
	w.Failure = port.Refused
	if failure := w.Pin(b); failure != port.Refused {
		t.Fatal("failed pin reported success")
	}
	if source, failure := w.Source(); failure != port.MountRefused || source != (port.MountSource{}) {
		t.Fatalf("failed repin retained an old source: %#v %s", source, failure)
	}
	w.Failure = ""
	b.Checkout.Inode = "1900"
	if failure := w.Pin(b); failure != "" {
		t.Fatal(failure)
	}
	if source, failure := w.Source(); failure != "" || source.Inode != "1900" {
		t.Fatalf("repin did not select the new identity: %#v %s", source, failure)
	}
	w.Close()
}

type anchorReporter struct{ problems []string }

func (*anchorReporter) Helper() {}
func (r *anchorReporter) Errorf(format string, args ...any) {
	r.problems = append(r.problems, fmt.Sprintf(format, args...))
}

func TestAnchorWorkspaceOracleRejectsPathnames(t *testing.T) {
	b, _ := anchorFixture()
	for _, path := range []string{"/checkout", "/proc/self/fd/9", "/proc/123/root/9", "/proc/0/fd/9", "/proc/0123/fd/9", "/proc/123/fd/-1", "/proc/123/fd/09", "/proc/123/fd/18446744073709551616"} {
		t.Run(path, func(t *testing.T) {
			w := anchorWorkspaceFixture(b)
			w.Descriptor = path
			reporter := &anchorReporter{}
			AnchorWorkspaceContract(reporter, w, b, 456, func() { w.Changed = true })
			if len(reporter.problems) != 1 || !strings.Contains(reporter.problems[0], "pinned descriptor identity") {
				t.Fatalf("pathname passed the source oracle: %v", reporter.problems)
			}
		})
	}
	if !descriptorSource("/proc/123/fd/0") {
		t.Fatal("file descriptor zero is a valid descriptor")
	}
}

func TestAnchorWorkspaceOracleRejectsMissingControls(t *testing.T) {
	b, _ := anchorFixture()
	for _, fault := range []string{"pin", "mount", "substitution"} {
		t.Run(fault, func(t *testing.T) {
			w := anchorWorkspaceFixture(b)
			switch fault {
			case "pin":
				w.Failure = port.Refused
			case "mount":
				delete(w.Mounts, 456)
			}
			reporter := &anchorReporter{}
			AnchorWorkspaceContract(reporter, w, b, 456, func() { w.Changed = fault != "substitution" })
			if len(reporter.problems) != 1 {
				t.Fatalf("missing control passed or masked another problem: %v", reporter.problems)
			}
		})
	}
}

func TestAnchorConnectionOracleRejectsMissingConcreteIdentity(t *testing.T) {
	_, r := anchorFixture()
	reporter := &anchorReporter{}
	AnchorConnectionContract(reporter, &AnchorResources{}, r)
	if len(reporter.problems) != 1 || !strings.Contains(reporter.problems[0], "exact observed authority") {
		t.Fatalf("name and labels passed without concrete identity: %v", reporter.problems)
	}
}
