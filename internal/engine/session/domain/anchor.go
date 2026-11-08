package domain

import (
	"context"

	"github.com/donvargax/itos-container/internal/engine/session/port"
)

func prepareAnchor(ctx context.Context, i port.Intent, c port.RunConnection, workspace port.Workspace, session string) port.Outcome {
	w, ok := workspace.(port.AnchorWorkspace)
	if !ok {
		return port.MountUnavailable
	}
	source, failure := w.Source()
	if failure != "" {
		return failure
	}
	b := i.Binding()
	if source.Device != b.Checkout.Device || source.Inode != b.Checkout.Inode || !port.DescriptorPath(source.Path) {
		return port.MountRefused
	}
	r := i.Record()
	if r.Schema != 1 || r.Stage != "reserved" || r.ContainerID != "" {
		return port.MountRefused
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
		return port.MountRefused
	}
	r.Anchor.Created, r.Anchor.Stage = o.Created, "owned"
	if i.Save(r) != "" {
		return port.MountWrite
	}
	return verifyAnchor(ctx, i.Record(), c)
}

func verifyAnchor(ctx context.Context, r port.Record, c port.RunConnection) port.Outcome {
	if r.Schema != 2 || r.Anchor.Stage != "owned" {
		return port.MountRefused
	}
	o, status := c.InspectAnchor(ctx, r.Anchor.Name)
	if status == port.Changed || status == port.NotFound {
		return port.MountRefused
	}
	if status != port.Observed {
		return port.MountUncertain
	}
	if !r.OwnsAnchor(o) {
		return port.MountRefused
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
	w, ok := workspace.(port.AnchorWorkspace)
	pw, processOK := workspace.(port.ProcessWorkspace)
	if !ok || !processOK || !o.Running || o.PID <= 0 {
		return port.DetectorRefused, port.MountRefused
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
	if failure != "" || p == nil || inspected != "" || !fresh.Running || fresh.PID != o.PID {
		return port.DetectorRefused, port.MountRefused
	}
	if exited, f := p.Exited(); f != "" || exited {
		return detectorAfterExit(ctx, r, c)
	}
	mounted, failure := w.Mounted(o.PID)
	// Check the held process again: a numeric PID reused during stat must not
	// supply either a verified mount or a mismatch attributed to this worker.
	exited, processFailure := p.Exited()
	if failure != "" || exited || processFailure != "" {
		return detectorAfterExit(ctx, r, c)
	}
	if mounted.Device != r.Anchor.Device || mounted.Inode != r.Anchor.Inode {
		return port.DetectorRefused, port.MountRefused
	}
	return port.DetectorVerified, ""
}

func inspectDetector(ctx context.Context, r port.Record, c port.RunConnection) (port.Observation, port.Outcome) {
	o, status := c.Inspect(ctx, r.ContainerID)
	if status != port.Observed || o.ID != r.ContainerID || !owns(r, o) || !o.StatePresent {
		return port.Observation{}, port.MountRefused
	}
	return o, ""
}

func detectorExited(o port.Observation) bool {
	return !o.Running && o.Started && o.Status == "exited"
}

func detectorAfterExit(ctx context.Context, r port.Record, c port.RunConnection) (string, port.Outcome) {
	o, failure := inspectDetector(ctx, r, c)
	if failure == "" && detectorExited(o) {
		return port.DetectorExited, ""
	}
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
