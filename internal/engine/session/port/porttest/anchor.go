package porttest

import (
	"context"
	"strconv"
	"strings"

	"github.com/donvargax/itos-container/internal/engine/session/port"
)

// AnchorResources models engine volume records, not native mounts. Ownership
// decisions and worker-before-volume teardown belong to the session domain.
// Tests change Resources only between calls, as with the worker engine fake.
type AnchorResources struct {
	Resources map[string]port.AnchorObservation
	Created   string
}

func (a *AnchorResources) AllocateAnchor(ctx context.Context, r port.Record) port.Outcome {
	if ctx.Err() != nil {
		return port.MountUncertain
	}
	if a.Resources == nil {
		a.Resources = map[string]port.AnchorObservation{}
	}
	// Like a named local volume, an existing entry is not replaced by create.
	// Callers must inspect it rather than infer ownership from this response.
	if _, exists := a.Resources[r.Anchor.Name]; !exists {
		a.Resources[r.Anchor.Name] = port.AnchorObservation{
			Name: r.Anchor.Name, Created: a.Created, Driver: "local", Scope: "local",
			Source: r.Anchor.Source, Type: "none", Options: "bind",
			Token: r.NameToken(), Project: r.Project, Fingerprint: r.Fingerprint,
			Session: r.Anchor.Session, Device: r.Anchor.Device, Inode: r.Anchor.Inode,
			ExactOptions: true, ExactLabels: true,
		}
	}
	return ""
}

func (a *AnchorResources) InspectAnchor(ctx context.Context, name string) (port.AnchorObservation, port.Inspection) {
	if ctx.Err() != nil {
		return port.AnchorObservation{}, port.Unavailable
	}
	o, exists := a.Resources[name]
	if !exists {
		return port.AnchorObservation{}, port.NotFound
	}
	return o, port.Observed
}

func (a *AnchorResources) RemoveAnchor(ctx context.Context, name string) {
	if ctx.Err() == nil {
		delete(a.Resources, name)
	}
}

// AnchoredWorkspace keeps the pinned identity independent of mutable pathname
// observations and of the identities explicitly supplied for worker init PIDs.
// All paths and process observations here are values; this fake performs no I/O.
type AnchoredWorkspace struct {
	Workspace
	Descriptor string
	Mounts     map[int]port.MountSource
	binding    port.Binding
	pinned     bool
}

func (w *AnchoredWorkspace) Pin(b port.Binding) port.Outcome {
	w.pinned = false
	if failure := w.Workspace.Pin(b); failure != "" {
		return failure
	}
	w.binding, w.pinned = b, true
	return ""
}

func (w *AnchoredWorkspace) Source() (port.MountSource, port.Outcome) {
	if !w.pinned || w.Closed {
		return port.MountSource{}, port.MountRefused
	}
	return port.MountSource{Path: w.Descriptor, Device: w.binding.Checkout.Device, Inode: w.binding.Checkout.Inode}, ""
}

func (w *AnchoredWorkspace) Mounted(pid int) (port.MountSource, port.Outcome) {
	mounted, exists := w.Mounts[pid]
	if pid <= 0 || !exists {
		return port.MountSource{}, port.MountRefused
	}
	return mounted, ""
}

// AnchorConnectionContract is ready to run unchanged against the SDK's local
// HTTP fixture. It does not prove kernel resolution, durable ownership or safe
// release ordering; the slice-17 scenarios supply those separate oracles.
func AnchorConnectionContract(t Reporter, c port.AnchorConnection, r port.Record) {
	t.Helper()
	ctx := context.Background()
	if o, status := c.InspectAnchor(ctx, r.Anchor.Name); status != port.NotFound || o != (port.AnchorObservation{}) {
		t.Errorf("missing anchor observation: %#v %v", o, status)
	}
	if failure := c.AllocateAnchor(ctx, r); failure != "" {
		t.Errorf("anchor allocation: %s", failure)
		return
	}
	o, status := c.InspectAnchor(ctx, r.Anchor.Name)
	want := port.AnchorObservation{
		Name: r.Anchor.Name, Created: o.Created, Driver: "local", Scope: "local",
		Source: r.Anchor.Source, Type: "none", Options: "bind",
		Token: r.NameToken(), Project: r.Project, Fingerprint: r.Fingerprint,
		Session: r.Anchor.Session, Device: r.Anchor.Device, Inode: r.Anchor.Inode,
		ExactOptions: true, ExactLabels: true,
	}
	if status != port.Observed || o.Created == "" || o != want {
		t.Errorf("allocated anchor lacks exact observed authority: %#v %v", o, status)
	}
	// Same-name allocation is not proof that the supplied tuple was installed.
	other := r
	other.Anchor.Source = "/proc/999/fd/8"
	if failure := c.AllocateAnchor(ctx, other); failure != "" && failure != port.MountRefused {
		t.Errorf("existing anchor allocation: %s", failure)
	}
	if again, status := c.InspectAnchor(ctx, r.Anchor.Name); status != port.Observed || again != o {
		t.Errorf("same-name allocation replaced the original anchor: %#v %v", again, status)
	}
	c.RemoveAnchor(ctx, r.Anchor.Name)
	if o, status := c.InspectAnchor(ctx, r.Anchor.Name); status != port.NotFound || o != (port.AnchorObservation{}) {
		t.Errorf("removed anchor lacks confirmed absence: %#v %v", o, status)
	}
}

// AnchorWorkspaceContract requires fixture-controlled substitution and a worker
// PID whose independently supplied mount observation has the validated identity.
func AnchorWorkspaceContract(t Reporter, w port.AnchorWorkspace, b port.Binding, pid int, substitute func()) {
	t.Helper()
	defer w.Close()
	if failure := w.Pin(b); failure != "" {
		t.Errorf("anchor workspace pin: %s", failure)
		return
	}
	source, failure := w.Source()
	if failure != "" || !descriptorSource(source.Path) || source.Device != b.Checkout.Device || source.Inode != b.Checkout.Inode {
		t.Errorf("source is not the pinned descriptor identity: %#v %s", source, failure)
	}
	for _, invalid := range []int{0, -1} {
		if mounted, failure := w.Mounted(invalid); failure != port.MountRefused || mounted != (port.MountSource{}) {
			t.Errorf("invalid init PID %d gave mount authority: %#v %s", invalid, mounted, failure)
		}
	}
	mounted, failure := w.Mounted(pid)
	if failure != "" || mounted.Device != b.Checkout.Device || mounted.Inode != b.Checkout.Inode {
		t.Errorf("worker mount identity: %#v %s", mounted, failure)
	}
	substitute()
	if failure := w.Validate(); failure != port.Refused {
		t.Errorf("pathname substitution escaped validation: %s", failure)
	}
	if again, failure := w.Source(); failure != "" || again != source {
		t.Errorf("pathname substitution changed the pinned source: %#v %s", again, failure)
	}
	w.Close()
	if source, failure := w.Source(); failure != port.MountRefused || source != (port.MountSource{}) {
		t.Errorf("closed descriptor supplied mount authority: %#v %s", source, failure)
	}
}

func descriptorSource(path string) bool {
	parts := strings.Split(path, "/")
	if len(parts) != 5 || parts[0] != "" || parts[1] != "proc" || parts[3] != "fd" {
		return false
	}
	pid, pidError := strconv.ParseUint(parts[2], 10, 64)
	fd, fdError := strconv.ParseUint(parts[4], 10, 64)
	return pidError == nil && fdError == nil && pid > 0 &&
		strconv.FormatUint(pid, 10) == parts[2] && strconv.FormatUint(fd, 10) == parts[4]
}
