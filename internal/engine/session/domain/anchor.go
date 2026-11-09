package domain

import (
	"context"

	budget "github.com/donvargax/itos-container/internal/engine/budget/domain"

	"github.com/donvargax/itos-container/internal/engine/session/port"
)

func prepareAnchor(ctx context.Context, i port.Intent, c port.RunConnection, workspace port.Workspace, session string) port.Outcome {
	w, ok := workspace.(port.AnchorWorkspace)
	if !ok {
		return port.MountUnavailable
	}
	source, failure := w.Source()
	if failure != "" {
		b := i.Binding()
		recordMountDiagnostic(ctx, port.MountDiagnostic{Check: "checkout-descriptor", Reason: "source observation failed", ExpectedDevice: b.Checkout.Device, ExpectedInode: b.Checkout.Inode, System: mountSystemDetail(w)})
		return failure
	}
	b := i.Binding()
	if source.Device != b.Checkout.Device || source.Inode != b.Checkout.Inode || !port.DescriptorPath(source.Path) {
		recordMountDiagnostic(ctx, port.MountDiagnostic{Check: "checkout-source", Reason: "descriptor or identity mismatch", ExpectedDevice: b.Checkout.Device, ExpectedInode: b.Checkout.Inode, ObservedDevice: source.Device, ObservedInode: source.Inode})
		return port.MountRefused
	}
	r := i.Record()
	if r.Schema != 1 || r.Stage != "reserved" || r.ContainerID != "" {
		return anchorRefusal(ctx, r, "anchor-intent", "intent is not reserved before create")
	}
	r.Schema, r.Anchor = 2, port.Anchor{Session: session, Name: "itos-container-anchor-" + r.NameToken(), Source: source.Path, Device: source.Device, Inode: source.Inode, Stage: "intent"}
	if i.Save(r) != "" {
		return port.MountWrite
	}
	if failure = c.Check(ctx); failure != "" {
		return failure
	}
	if failure = c.AllocateAnchor(ctx, r); failure != "" {
		return failure
	}
	o, status := c.InspectAnchor(ctx, r.Anchor.Name)
	if status != port.Observed {
		return port.MountUncertain
	}
	if !r.OwnsAnchor(o) {
		return anchorRefusal(ctx, r, "anchor-ownership", "volume options or ownership differ")
	}
	r.Anchor.Created, r.Anchor.Stage = o.Created, "owned"
	if i.Save(r) != "" {
		return port.MountWrite
	}
	return verifyAnchor(ctx, i.Record(), c)
}

func verifyAnchor(ctx context.Context, r port.Record, c port.RunConnection) port.Outcome {
	if r.Schema != 2 || r.Anchor.Stage != "owned" {
		return anchorRefusal(ctx, r, "anchor-record", "owned anchor record is unavailable")
	}
	o, status := c.InspectAnchor(ctx, r.Anchor.Name)
	if status == port.Changed || status == port.NotFound {
		return anchorRefusal(ctx, r, "anchor-inspect", "volume is absent or changed")
	}
	if status != port.Observed {
		return port.MountUncertain
	}
	if !r.OwnsAnchor(o) {
		return anchorRefusal(ctx, r, "anchor-ownership", "volume options or ownership differ")
	}
	return ""
}

func detectAnchor(ctx context.Context, i port.Intent, c port.RunConnection, workspace port.Workspace) (string, port.Outcome) {
	r := i.Record()
	o, failure := inspectDetector(ctx, r, c)
	if failure != "" {
		return port.DetectorRefused, failure
	}
	if detectorExited(o) {
		return port.DetectorExited, ""
	}
	d := port.MountDiagnostic{ExpectedDevice: r.Anchor.Device, ExpectedInode: r.Anchor.Inode, PID: o.PID}
	refuse := func(check, reason string) (string, port.Outcome) {
		d.Check, d.Reason = check, reason
		recordMountDiagnostic(ctx, d)
		return port.DetectorRefused, port.MountRefused
	}
	w, ok := workspace.(port.AnchorWorkspace)
	pw, processOK := workspace.(port.ProcessWorkspace)
	if !ok || !processOK {
		return refuse("worker-observation-port", "mount or process observation unavailable")
	}
	if !o.Running || o.PID <= 0 {
		return refuse("worker-init-pid", "inspection has no live init PID")
	}
	p, failure := pw.HoldProcess(o.PID)
	if p != nil {
		defer p.Close()
	}
	// A failed pidfd open may mean the worker just exited. Only a fresh owned
	// full-ID observation can establish that; an arbitrary syscall error cannot.
	fresh, inspected := inspectDetector(ctx, r, c)
	if inspected == "" && detectorExited(fresh) {
		return port.DetectorExited, ""
	}
	if failure == port.ProcessAbsent && inspected == "" && fresh.Running && fresh.PID == o.PID {
		d.Check, d.Reason, d.System = "pidfd-open", "inspected process is absent", mountSystemDetail(workspace)
		return detectorAfterExit(ctx, r, c, d, true)
	}
	if failure != "" || p == nil {
		d.System = mountSystemDetail(workspace)
		return refuse("pidfd-open", "process reference unavailable; worker exit not confirmed")
	}
	if inspected != "" {
		return port.DetectorRefused, inspected
	}
	if !fresh.Running || fresh.PID != o.PID {
		d.ObservedPID = fresh.PID
		return refuse("worker-pid-recheck", "fresh inspection changed the live init PID")
	}
	if exited, f := p.Exited(); f != "" || exited {
		d.Check, d.Reason, d.System = "pidfd-before-stat", "held process is exited or unavailable", mountSystemDetail(p)
		return detectorAfterExit(ctx, r, c, d, exited && f == "")
	}
	mounted, failure := w.Mounted(o.PID)
	// Save the stat diagnostic before any later observation can replace it.
	if failure != "" {
		d.Check, d.Reason, d.System = "mounted-stat", "workspace stat or directory identity unavailable", mountSystemDetail(workspace)
	}
	// Check the held process again: a numeric PID reused during stat must not
	// supply either a verified mount or a mismatch attributed to this worker.
	exited, processFailure := p.Exited()
	// A live worker may change its root or namespaces before this detector.
	// The descriptor-backed volume is the protection; lack of a visible root
	// cannot verify the mount, but must not discard the workload's real result.
	if failure == port.MountRootUnavailable && !exited && processFailure == "" {
		return port.DetectorRootChanged, ""
	}
	if failure != "" || exited || processFailure != "" {
		if failure == "" {
			d.Check, d.Reason, d.System = "pidfd-after-stat", "held process is exited or unavailable", mountSystemDetail(p)
		}
		return detectorAfterExit(ctx, r, c, d, exited && processFailure == "")
	}
	if mounted.Device != r.Anchor.Device || mounted.Inode != r.Anchor.Inode {
		d.ObservedDevice, d.ObservedInode = mounted.Device, mounted.Inode
		return refuse("mounted-identity", "mounted checkout identity differs")
	}
	return port.DetectorVerified, ""
}

func inspectDetector(ctx context.Context, r port.Record, c port.RunConnection) (port.Observation, port.Outcome) {
	o, status := c.Inspect(ctx, r.ContainerID)
	if status != port.Observed {
		return port.Observation{}, anchorRefusal(ctx, r, "worker-inspect", "full-ID inspection unavailable")
	}
	if o.ID != r.ContainerID || !owns(r, o) {
		return port.Observation{}, anchorRefusal(ctx, r, "worker-ownership", "full-ID inspection differs from owned worker")
	}
	if !o.StatePresent {
		return port.Observation{}, anchorRefusal(ctx, r, "worker-state", "inspection has no process state")
	}
	return o, ""
}

func detectorExited(o port.Observation) bool {
	return !o.Running && o.Started && o.Status == "exited"
}

func detectorAfterExit(ctx context.Context, r port.Record, c port.RunConnection, d port.MountDiagnostic, processExited bool) (string, port.Outcome) {
	// Kernel process absence can precede Docker's state publication. Bound the
	// entire reconciliation, including its fresh owned inspections, to one call.
	ctx, cancel := context.WithTimeout(ctx, budget.CallDuration)
	defer cancel()
	o, failure := inspectDetector(ctx, r, c)
	if failure == "" && detectorExited(o) {
		return port.DetectorExited, ""
	}
	if failure == "" && processExited && o.Running && o.PID == d.PID {
		exit, waited := c.Wait(ctx, r.ContainerID)
		if waited == "" && budget.Cause(ctx) == nil {
			o, failure = inspectDetector(ctx, r, c)
			if failure == "" && detectorExited(o) && budget.Cause(ctx) == nil {
				if state, ok := ctx.Value(startupKey{}).(*startupState); ok {
					state.detectorExit = &exit
				}
				return port.DetectorExited, ""
			}
		}
	}
	d.Reason += "; worker exit not confirmed"
	recordMountDiagnostic(ctx, d)
	return port.DetectorRefused, port.MountRefused
}

// A deadline may end the controller's wait before the engine's actual start
// returns. Keep the descriptor pinned behind that connection's operation gate.
func closeAnchorWorkspace(c port.RunConnection, w port.Workspace) {
	if finite, ok := c.(finiteConnection); ok {
		select {
		case <-finite.gate:
			w.Close()
			finite.gate <- struct{}{}
		default:
			go func() { <-finite.gate; w.Close(); finite.gate <- struct{}{} }()
		}
		return
	}
	w.Close()
}

// releaseAnchor is reachable only after exact worker absence or a reserved
// intent which proves no worker create was issued. It never resolves Source.
func releaseAnchor(ctx context.Context, i port.Intent, c port.RunConnection) port.Outcome {
	r := i.Record()
	if r.Schema != 2 || (r.Stage != "reserved" && r.Stage != "absent" && r.Stage != "cancelled") {
		return port.Refused
	}
	o, failure := inspectRecoveryAnchor(ctx, i, c)
	if failure != "" {
		return failure
	}
	if o.Name == "" {
		if r.Anchor.Created == "" {
			return port.Incomplete
		}
	} else {
		if r.Anchor.Stage == "intent" {
			r.Anchor.Stage, r.Anchor.Created = "owned", o.Created
			if failure = anchorAuthority(ctx, i, c); failure != "" {
				return failure
			}
			if failure = i.Save(r); failure != "" {
				return port.OwnershipWrite
			}
		}
		// Revalidate every protected record and the daemon, then independently
		// inspect the complete volume tuple immediately before its deletion.
		if failure = anchorAuthority(ctx, i, c); failure != "" {
			return failure
		}
		o, failure = inspectRecoveryAnchor(ctx, i, c)
		if failure != "" {
			return failure
		}
		if o.Name == "" {
			return port.Incomplete
		}
		if failure = anchorAuthority(ctx, i, c); failure != "" {
			return failure
		}
		c.RemoveAnchor(ctx, r.Anchor.Name)
		o, failure = inspectRecoveryAnchor(ctx, i, c)
		if failure != "" {
			return failure
		}
		if o.Name != "" {
			return port.Incomplete
		}
	}
	if failure = anchorAuthority(ctx, i, c); failure != "" {
		return failure
	}
	r.Anchor.Stage = "absent"
	if i.Save(r) != "" {
		return port.CompletionWrite
	}
	return ""
}
