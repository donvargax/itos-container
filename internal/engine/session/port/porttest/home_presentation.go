package porttest

import "github.com/donvargax/itos-container/internal/engine/session/port"

// HomePresentation retains output in memory for command-boundary tests.
type HomePresentation struct {
	Result port.HomeResult
	JSON   bool
}

func (p *HomePresentation) PrintHomes(r port.HomeResult, json bool) int {
	p.Result, p.JSON = r, json
	exit := 0
	for _, f := range r.Failures {
		code := 70
		switch f.Outcome {
		case port.ConsentRequired, port.HomeInUse, port.Refused, port.Unsafe:
			code = 1
		case port.Usage, port.Invalid:
			code = 2
		case port.Environment, port.EnvironmentMissing:
			code = 3
		case port.HomeUnavailable, port.Uncertain, port.Incomplete, port.IntentWrite, port.OwnershipWrite, port.CompletionWrite, port.ExecutionFailed:
			code = 75
		}
		exit = max(exit, code)
	}
	return exit
}

func HomePresentationContract(t Reporter, p port.HomePresentation) {
	t.Helper()
	for _, json := range []bool{false, true} {
		if got := p.PrintHomes(port.HomeResult{}, json); got != 0 {
			t.Errorf("successful home result exit = %d", got)
		}
		for _, tc := range []struct {
			outcomes []port.Outcome
			exit     int
		}{
			{[]port.Outcome{port.ConsentRequired, port.HomeInUse, port.Refused, port.Unsafe}, 1},
			{[]port.Outcome{port.Usage, port.Invalid}, 2},
			{[]port.Outcome{port.Environment, port.EnvironmentMissing}, 3},
			{[]port.Outcome{port.HomeUnavailable, port.Uncertain, port.Incomplete, port.IntentWrite, port.OwnershipWrite, port.CompletionWrite, port.ExecutionFailed}, 75},
			{[]port.Outcome{"unknown"}, 70},
		} {
			for _, outcome := range tc.outcomes {
				r := port.HomeResult{Failures: []port.RecoveryFailure{{Outcome: outcome}}}
				if got := p.PrintHomes(r, json); got != tc.exit {
					t.Errorf("%s home result exit = %d, want %d", outcome, got, tc.exit)
				}
			}
		}
		r := port.HomeResult{Failures: []port.RecoveryFailure{{Outcome: port.Incomplete}, {Outcome: port.Usage}}}
		if got := p.PrintHomes(r, json); got != 75 {
			t.Errorf("mixed home result exit = %d", got)
		}
	}
}
