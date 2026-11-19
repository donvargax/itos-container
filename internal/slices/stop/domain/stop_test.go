package domain_test

import (
	"context"
	"strings"
	"testing"

	session "github.com/donvargax/itos-container/internal/engine/session/domain"
	"github.com/donvargax/itos-container/internal/engine/session/port"
	fake "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
	"github.com/donvargax/itos-container/internal/slices/stop/domain"
)

// retirementService records the exact request the validated home path forwards.
type retirementService struct {
	id, action string
	consent    port.HomeConsent
	result     port.HomeResult
}

func (s *retirementService) ListHomes(context.Context) port.HomeResult { return s.result }
func (s *retirementService) Retire(_ context.Context, id, action string, consent port.HomeConsent) port.HomeResult {
	s.id, s.action, s.consent = id, action, consent
	return s.result
}

func TestStopValidatesCompleteIDBeforeOpeningRecovery(t *testing.T) {
	id := strings.Repeat("a", 64)
	for _, args := range [][]string{nil, {""}, {"../outside"}, {id[:63]}, {strings.ToUpper(id)}, {id, "extra"}} {
		r := domain.Stop(context.Background(), nil, args)
		if len(r.Failures) != 1 || r.Failures[0].Outcome != port.Usage {
			t.Fatal(args, r)
		}
	}
	r := domain.Stop(context.Background(), session.Recovery{Registry: &fake.Registry{}}, []string{id})
	if len(r.Failures) != 1 || r.Failures[0].Outcome != port.Refused || r.Failures[0].Session != id {
		t.Fatal(r)
	}
}

func TestHomeRetiresOnlyOneConsentedExactAction(t *testing.T) {
	id := strings.Repeat("a", 64)
	consent := fake.HomeConsent{Yes: true}
	for _, request := range []struct {
		name          string
		id            string
		args          []string
		reset, remove bool
	}{
		{"incomplete id", id[:63], nil, true, false},
		{"uppercase id", strings.ToUpper(id), nil, false, true},
		{"extra arguments", id, []string{"outside"}, true, false},
		{"no chosen action", id, nil, false, false},
		{"two chosen actions", id, nil, true, true},
	} {
		t.Run(request.name, func(t *testing.T) {
			r := domain.Home(context.Background(), nil, request.id, request.args, request.reset, request.remove, consent)
			if len(r.Failures) != 1 || r.Failures[0].Outcome != port.Usage || len(r.Homes) != 0 {
				t.Fatal(request.name, r)
			}
		})
	}
	for _, request := range []struct {
		name          string
		reset, remove bool
		action        string
	}{
		{"reset", true, false, "reset"},
		{"delete", false, true, "delete"},
	} {
		t.Run(request.name, func(t *testing.T) {
			service := &retirementService{result: port.HomeResult{Homes: []port.HomeEntry{{ID: id}}}}
			r := domain.Home(context.Background(), service, id, nil, request.reset, request.remove, consent)
			if len(r.Homes) != 1 || r.Homes[0].ID != id || len(r.Failures) != 0 {
				t.Fatal(request.name, r)
			}
			if service.id != id || service.action != request.action {
				t.Fatal(request.name, service.id, service.action)
			}
			if recorded, ok := service.consent.(fake.HomeConsent); !ok || !recorded.Yes {
				t.Fatal(request.name, "retirement lost the terminal consent")
			}
		})
	}
}
