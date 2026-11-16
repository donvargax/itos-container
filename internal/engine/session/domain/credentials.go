package domain

import (
	"context"
	budget "github.com/donvargax/itos-container/internal/engine/budget/domain"
	"github.com/donvargax/itos-container/internal/engine/session/port"
	"strings"
)

type credentialRecoveryKey struct{}
type credentialRecoveryState struct{ stager port.CredentialStager }

// credentialCall shares the intent gate with publication and Close. A late
// syscall retains both source descriptors and serialization until it returns.
// Only the caller goroutine adopts a returned record, never a late reply.
func credentialHeld[T any](ctx context.Context, i port.Intent, name string, fn func(context.Context, port.Intent) T) (T, error) {
	raw := i
	var finite *finiteIntent
	if f, ok := i.(*finiteIntent); ok {
		finite = f
		raw = f.Intent
	}
	return budget.Call(ctx, name, func(call context.Context) T {
		if finite != nil {
			select {
			case <-finite.gate:
				defer func() { finite.gate <- struct{}{} }()
			case <-call.Done():
				var zero T
				return zero
			}
		}
		if budget.Cause(call) != nil {
			var zero T
			return zero
		}
		return fn(call, raw)
	})
}
func credentialCall(ctx context.Context, i port.Intent, name string, fn func(context.Context, port.Intent) port.Outcome) port.Outcome {
	type reply struct {
		record  port.Record
		failure port.Outcome
	}
	r, err := credentialHeld(ctx, i, name, func(call context.Context, raw port.Intent) reply { f := fn(call, raw); return reply{raw.Record(), f} })
	if err != nil {
		if state, ok := ctx.Value(startupKey{}).(*startupState); ok {
			state.operation = name
			state.failure = startupResult(ctx, ctx, err, name).Outcome
			return state.failure
		}
		return port.Incomplete
	}
	if finite, ok := i.(*finiteIntent); ok {
		finite.record = r.record
	}
	return r.failure
}
func (s Runner) prepareCredentials(ctx context.Context, i port.Intent) port.Outcome {
	kinds := i.Binding().StagingKinds
	if kinds == (port.CredentialKinds{}) {
		if s.Sources != nil {
			return port.CredentialRefused
		}
		return ""
	}
	sourceOwner := s.sourceOwner
	if sourceOwner == nil {
		sourceOwner = make(chan struct{}, 1)
		sourceOwner <- struct{}{}
	}
	defer func() {
		select {
		case <-sourceOwner:
			if s.Sources != nil {
				s.Sources.Close()
			}
		default:
		}
	}()
	if s.Credentials == nil || s.Sources == nil || s.StagingEntropy == nil || s.Sources.Kinds() != kinds {
		return port.CredentialRefused
	}
	token, f, err := budget.Call2(ctx, "credential staging identity", func(context.Context) (string, port.Outcome) { return s.StagingEntropy.StagingToken() })
	if err != nil {
		return port.IdentityFailed
	}
	if f != "" || len(token) != 32 || strings.Trim(token, "0123456789abcdef") != "" {
		return port.IdentityFailed
	}
	r := i.Record()
	r.Schema = 4
	r.Credentials = port.CredentialStaging{Directory: "/dev/shm/itos-container-" + token, Stage: "intent", Kinds: kinds}
	if f := i.Save(r); f != "" {
		return f
	}
	return credentialCall(ctx, i, "credential staging", func(call context.Context, raw port.Intent) port.Outcome {
		select {
		case <-sourceOwner:
			defer s.Sources.Close()
		default:
			return port.Incomplete
		}
		return s.Credentials.Stage(call, r, s.Sources, func(staging port.CredentialStaging) port.Outcome {
			if call.Err() != nil {
				return port.Incomplete
			}
			next := raw.Record()
			next.Credentials = staging
			return raw.Save(next)
		})
	})
}
func cleanupCredentials(ctx context.Context, i port.Intent, stager port.CredentialStager) port.Outcome {
	if i.Record().Schema != 4 {
		return ""
	}
	if stager == nil {
		return port.Internal
	}
	return credentialCall(ctx, i, "credential staging cleanup", func(call context.Context, raw port.Intent) port.Outcome {
		if f := raw.Validate(); f != "" {
			return f
		}
		r := raw.Record()
		if f := stager.Remove(call, r); f != "" {
			return f
		}
		presence, f := stager.Observe(call, r)
		if f != "" {
			return f
		}
		if presence != "absent" {
			return port.Incomplete
		}
		if call.Err() != nil {
			return port.Incomplete
		}
		r.Credentials.Stage = "absent"
		return raw.Save(r)
	})
}
func releaseCredentials(ctx context.Context, i port.Intent) port.Outcome {
	state, _ := ctx.Value(credentialRecoveryKey{}).(credentialRecoveryState)
	return cleanupCredentials(ctx, i, state.stager)
}
func (s Recovery) credentialPresence(ctx context.Context, entry port.Session, e *port.RecoveryEntry) port.Outcome {
	if entry.Intent().Record().Schema != 4 {
		return ""
	}
	e.Staging = "unknown"
	if s.Credentials == nil {
		return port.Internal
	}
	type reply struct {
		presence string
		failure  port.Outcome
	}
	r, err := credentialHeld(ctx, entry.Intent(), "credential staging observation", func(call context.Context, raw port.Intent) reply {
		if f := raw.Validate(); f != "" {
			return reply{"unknown", f}
		}
		p, f := s.Credentials.Observe(call, raw.Record())
		return reply{p, f}
	})
	if err != nil {
		return port.Incomplete
	}
	e.Staging = r.presence
	return r.failure
}
