// Package presentation renders recovery facts without raw storage or SDK errors.
package presentation

import (
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strconv"

	"github.com/donvargax/itos-container/internal/engine/session/port"
)

type UI struct{ Out, Err io.Writer }
type Problem struct {
	Rule    string `json:"rule"`
	Exit    int    `json:"exit"`
	Message string `json:"message"`
	Fix     string `json:"fix"`
	Session string `json:"session,omitempty"`
}

func problem(f port.RecoveryFailure) Problem {
	p := Problem{Rule: "recovery.internal", Exit: 70, Message: "Recovery could not complete an internal operation.", Fix: "Have the host user inspect this failure before retrying."}
	if port.HexID(f.Session) {
		p.Session = f.Session
	}
	switch f.Outcome {
	case port.Usage:
		p.Rule, p.Exit, p.Message, p.Fix = "recovery.usage", 2, "Invalid recovery invocation.", "Use status with no positional arguments, or stop with exactly one complete lowercase 64-digit session ID."
	case port.Refused:
		p.Rule, p.Exit, p.Message, p.Fix = "recovery.not-owned", 1, "The original session ownership could not be verified.", "Use the same effective project and host user; inspect the protected evidence without adopting other resources."
	case port.Unsafe:
		p.Rule, p.Exit, p.Message, p.Fix = "recovery.unsafe-state", 1, "Protected recovery state is unsafe.", "Have the host user inspect ownership, modes and links outside the checkout."
	case port.Invalid:
		p.Rule, p.Exit, p.Message, p.Fix = "recovery.invalid-state", 2, "Protected recovery state is invalid or incomplete.", "Have the host user inspect the retained original record, intent and snapshot."
	case port.Environment, port.EnvironmentMissing:
		p.Rule, p.Exit, p.Message, p.Fix = "recovery.environment", 3, "The supported local engine environment is unavailable.", "Restore the original Linux amd64 native Docker environment."
	case port.Uncertain, port.Incomplete, port.IntentWrite, port.OwnershipWrite, port.CompletionWrite, port.ExecutionFailed:
		p.Rule, p.Exit, p.Message, p.Fix = "recovery.temporary", 75, "Recovery observation or durable completion is uncertain.", "Retry inspection or stop in the same effective project when the original engine and protected state are available."
	}
	return p
}

func (u UI) Print(r port.RecoveryResult, withJSON bool) int {
	problems := make([]Problem, 0, len(r.Failures))
	code := 0
	for _, f := range r.Failures {
		p := problem(f)
		problems = append(problems, p)
		code = max(code, p.Exit)
	}
	slices.SortFunc(problems, func(a, b Problem) int {
		if a.Session != b.Session {
			if a.Session < b.Session {
				return -1
			}
			return 1
		}
		if a.Rule < b.Rule {
			return -1
		}
		if a.Rule > b.Rule {
			return 1
		}
		return 0
	})
	if r.Entries == nil {
		r.Entries = []port.RecoveryEntry{}
	}
	slices.SortFunc(r.Entries, func(a, b port.RecoveryEntry) int {
		if a.ID < b.ID {
			return -1
		}
		if a.ID > b.ID {
			return 1
		}
		return 0
	})
	if withJSON {
		_ = json.NewEncoder(u.Out).Encode(struct {
			Schema   int                  `json:"schema"`
			OK       bool                 `json:"ok"`
			Problems []Problem            `json:"problems"`
			Sessions []port.RecoveryEntry `json:"sessions"`
		}{1, code == 0, problems, r.Entries})
	} else if len(r.Entries) == 0 && code == 0 {
		_, _ = fmt.Fprintln(u.Out, "No owned sessions in this project.")
	} else {
		for _, e := range r.Entries {
			exit := "unknown"
			if e.WorkloadExit != nil {
				exit = strconv.Itoa(*e.WorkloadExit)
			}
			_, _ = fmt.Fprintf(u.Out, "session %s project %s fingerprint %s state %s resource %s controller %s cleanup %s workload_exit %s stop_candidate %t\n", e.ID, e.Project, e.Fingerprint, e.State, e.Resource, e.Controller, e.Cleanup, exit, e.StopCandidate)
			if e.Anchor != "" {
				_, _ = fmt.Fprintf(u.Out, "worker resource=%s anchor=%s cleanup=%s\n", e.Resource, e.Anchor, e.Cleanup)
			}
			if len(e.Retry) != 0 {
				_, _ = fmt.Fprintf(u.Out, "Use the same effective project: itos container stop %s\n", e.ID)
			}
		}
	}
	for _, p := range problems {
		_, _ = fmt.Fprintf(u.Err, "%s: %s %s\n", p.Rule, p.Message, p.Fix)
	}
	return code
}
