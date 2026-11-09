package domain

import (
	"context"
	"github.com/donvargax/itos-container/internal/engine/session/port"
)

type egressRecoveryKey struct{}
type egressRecoveryState struct {
	log    port.RefusalLog
	report *port.RefusalReport
}

func egressResource(r *port.Record, index int) *port.EgressResource {
	switch index {
	case 0:
		return &r.Egress.Internal
	case 1:
		return &r.Egress.Network
	default:
		return &r.Egress.Proxy
	}
}
func egressKind(index int) port.NetworkKind {
	if index == 0 {
		return port.InternalNetwork
	}
	return port.EgressNetwork
}

// Inspection never adopts an intent. Only the mutating recovery path may save
// its independently observed full ID before releasing it.
func inspectEgressResource(ctx context.Context, i port.Intent, c port.RunConnection, index int) (id string, present, running bool, failure port.Outcome) {
	r := i.Record()
	v := *egressResource(&r, index)
	if v.Stage == "" {
		return "", false, false, ""
	}
	if f := anchorAuthority(ctx, i, c); f != "" {
		return "", false, false, f
	}
	target := v.ID
	if v.Stage == "intent" {
		target = v.Name
	}
	var status port.Inspection
	var owned bool
	if index == 2 {
		o, s := c.Inspect(ctx, target)
		status = s
		id = o.ID
		running = o.Running
		owned = r.OwnsProxy(o, i.Binding()) && o.StatePresent && ((o.Status == "running" && o.Running) || (o.Status == "created" && !o.Running && !o.Started && o.PID == 0) || (o.Status == "exited" && !o.Running && o.PID == 0))
	} else {
		o, s := c.InspectNetwork(ctx, target)
		status = s
		id = o.ID
		owned = r.OwnsNetwork(o, egressKind(index))
		running = o.Endpoints != 0
	}
	if status == port.Changed {
		return "", false, false, port.Refused
	}
	if status == port.NotFound {
		if v.Stage == "intent" {
			return "", false, false, port.Incomplete
		}
		return v.ID, false, false, ""
	}
	if status != port.Observed {
		return "", false, false, port.Incomplete
	}
	if v.Stage == "absent" || !owned {
		return "", false, false, port.Refused
	}
	return id, true, running, ""
}
func inspectEgress(ctx context.Context, i port.Intent, c port.RunConnection) port.Outcome {
	if i.Record().Schema != 3 {
		return ""
	}
	for index := 2; index >= 0; index-- {
		if _, _, _, f := inspectEgressResource(ctx, i, c, index); f != "" {
			return f
		}
	}
	return ""
}
func releaseEgress(ctx context.Context, i port.Intent, c port.RunConnection) port.Outcome {
	r := i.Record()
	if r.Schema != 3 {
		return ""
	}
	if r.Stage != "reserved" && r.Stage != "cancelled" && r.Stage != "absent" {
		return port.Refused
	}
	for index := 2; index >= 0; index-- {
		if f := releaseEgressResource(ctx, i, c, index); f != "" {
			return f
		}
	}
	return ""
}
func releaseEgressResource(ctx context.Context, i port.Intent, c port.RunConnection, index int) port.Outcome {
	r := i.Record()
	v := egressResource(&r, index)
	if v.Stage == "" {
		return ""
	}
	id, present, running, f := inspectEgressResource(ctx, i, c, index)
	if f != "" {
		return f
	}
	if v.Stage == "absent" {
		return ""
	}
	if v.Stage == "intent" {
		v.ID, v.Stage = id, "owned"
		if f := anchorAuthority(ctx, i, c); f != "" {
			return f
		}
		if i.Save(r) != "" {
			return port.OwnershipWrite
		}
		_, present, running, f = inspectEgressResource(ctx, i, c, index)
		if f != "" {
			return f
		}
	}
	if present {
		if index == 2 {
			if f := stopAndCopyProxy(ctx, i, c, id, running); f != "" {
				return f
			}
		} else if running {
			return port.Incomplete
		}
		if f := anchorAuthority(ctx, i, c); f != "" {
			return f
		}
		_, present, running, f = inspectEgressResource(ctx, i, c, index)
		if f != "" {
			return f
		}
		if running {
			return port.Incomplete
		}
		if present {
			if index == 2 {
				c.RemoveRun(ctx, id)
			} else {
				c.RemoveNetwork(ctx, id)
			}
		}
		_, present, _, f = inspectEgressResource(ctx, i, c, index)
		if f != "" {
			return f
		}
		if present {
			return port.Incomplete
		}
	}
	if f := anchorAuthority(ctx, i, c); f != "" {
		return f
	}
	r = i.Record()
	v = egressResource(&r, index)
	v.Stage = "absent"
	if i.Save(r) != "" {
		return port.CompletionWrite
	}
	return ""
}
func releaseSessionResources(ctx context.Context, i port.Intent, c port.RunConnection) port.Outcome {
	if f := releaseEgress(ctx, i, c); f != "" {
		return f
	}
	return releaseAnchor(ctx, i, c)
}
func recoveryCleanup(r port.RecoveryResult, report *port.RefusalReport) port.Outcome {
	outcome := port.Complete
	for _, f := range r.Failures {
		if f.Outcome == port.RefusalLogGap {
			*report = port.RefusalReport{Outcome: f.Outcome, Gaps: append([]port.RefusalGap(nil), f.Gaps...)}
		} else if outcome == port.Complete {
			outcome = f.Outcome
		}
	}
	return outcome
}
