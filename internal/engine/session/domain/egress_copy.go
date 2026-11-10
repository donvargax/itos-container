package domain

import (
	"context"
	budget "github.com/donvargax/itos-container/internal/engine/budget/domain"
	"github.com/donvargax/itos-container/internal/engine/session/port"
)

func stopAndCopyProxy(ctx context.Context, i port.Intent, c port.RunConnection, id string, running bool) port.Outcome {
	if running {
		if f := anchorAuthority(ctx, i, c); f != "" {
			return f
		}
		if f := c.Signal(ctx, id, "SIGKILL"); f != "" {
			return f
		}
		_, f, err := budget.Call2(ctx, "proxy stop wait", func(call context.Context) (int, port.Outcome) { return c.Wait(call, id) })
		if err != nil || f != "" {
			return port.Incomplete
		}
		_, present, running, f := inspectEgressResource(ctx, i, c, 2)
		if f != "" {
			return f
		}
		if !present {
			return ""
		}
		if running {
			return port.Incomplete
		}
	}
	state, ok := ctx.Value(egressRecoveryKey{}).(egressRecoveryState)
	if !ok || state.log == nil {
		return port.Internal
	}
	type opened struct {
		copy port.RefusalCopy
		f    port.Outcome
	}
	staged, err := budget.Acquire(ctx, "refusal copy begin", func(call context.Context) opened {
		copy, f := state.log.Begin(call, i.Record().Anchor.Session)
		return opened{copy, f}
	}, func(v opened) {
		if v.copy != nil {
			v.copy.Close()
		}
	})
	if staged.copy != nil {
		defer staged.copy.Close()
	}
	if err != nil || staged.f != "" || staged.copy == nil {
		return port.Incomplete
	}
	f, err := budget.Call(ctx, "proxy log copy", func(call context.Context) port.Outcome { return c.CopyProxyLog(call, id, staged.copy) })
	if err != nil || f != "" {
		return port.Incomplete
	}
	report, f, err := budget.Call2(ctx, "refusal copy commit", func(context.Context) (port.RefusalReport, port.Outcome) { return staged.copy.Commit() })
	if err != nil || f != "" {
		return port.Incomplete
	}
	if report.Outcome == port.RefusalLogGap {
		*state.report = port.RefusalReport{Outcome: report.Outcome, Gaps: append([]port.RefusalGap(nil), report.Gaps...)}
	}
	return ""
}
