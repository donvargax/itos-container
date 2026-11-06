package porttest

import (
	"context"
	"strings"
	"sync"

	approval "github.com/donvargax/itos-container/internal/engine/approval/port"
	"github.com/donvargax/itos-container/internal/engine/session/port"
)

// Admissions shares reservations between independent in-memory registries.
// Controller leases are deliberately unrelated to admission lifetime.
type Admissions struct {
	mu      sync.Mutex
	records map[string]port.AdmissionRecord
	Failure port.Outcome
}

func (a *Admissions) reserve(root, id, intent string) port.Outcome {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.Failure != "" {
		return a.Failure
	}
	for _, r := range a.records {
		if r.Root != root && !r.Complete && (strings.HasPrefix(root, strings.TrimRight(r.Root, "/")+"/") || strings.HasPrefix(r.Root, strings.TrimRight(root, "/")+"/")) {
			return port.CheckoutOverlap
		}
	}
	if a.records == nil {
		a.records = map[string]port.AdmissionRecord{}
	}
	key := root + "\x00" + id
	if _, ok := a.records[key]; ok {
		return port.Refused
	}
	a.records[key] = port.AdmissionRecord{Schema: 1, Root: root, ID: id, Intent: intent}
	return ""
}
func (a *Admissions) complete(root, id string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	key := root + "\x00" + id
	r := a.records[key]
	r.Complete = true
	a.records[key] = r
}

// AdmissionFixture contains nested roots and a textual-prefix disjoint control.
// The adapter factory also supplies a durable-write fault, never an engine.
type AdmissionFixture struct {
	Registries   [3]port.Registry
	Stores       [3]port.Store
	Frozen       [3]approval.Durable
	WriteFailure func(bool)
}

func AdmissionContract(t Reporter, open func() AdmissionFixture) {
	t.Helper()
	for _, first := range []int{0, 1} {
		f := open()
		ctx := context.Background()
		id, token := strings.Repeat("a", 64), strings.Repeat("1", 64)
		lease, failure := f.Registries[first].Register(ctx, f.Frozen[first], id, token)
		if failure != "" {
			t.Errorf("first reservation: %s", failure)
			return
		}
		lease.Close()
		other := 1 - first
		if lease, failure := f.Registries[other].Register(ctx, f.Frozen[other], strings.Repeat("b", 64), token); failure != port.CheckoutOverlap || lease != nil {
			t.Errorf("preparing reservation lost after controller release: %s", failure)
			return
		}
		// Equal roots remain independently admissible.
		second, failure := f.Registries[first].Register(ctx, f.Frozen[first], strings.Repeat("c", 64), strings.Repeat("2", 64))
		if failure != "" {
			t.Errorf("same-root reservation: %s", failure)
			return
		}
		defer second.Close()
		control, failure := f.Registries[2].Register(ctx, f.Frozen[2], id, token)
		if failure != "" {
			t.Errorf("disjoint reservation: %s", failure)
			return
		}
		defer control.Close()
		for n, sessionID := range []string{id, strings.Repeat("c", 64)} {
			sessionToken := token
			if n == 1 {
				sessionToken = strings.Repeat("2", 64)
			}
			i, failure := f.Stores[first].New(ctx, f.Frozen[first], sessionToken)
			if failure != "" {
				t.Errorf("intent: %s", failure)
				return
			}
			i.Close()
			s, failure := f.Registries[first].Open(ctx, sessionID, false)
			if failure != "" {
				t.Errorf("open: %s", failure)
				return
			}
			r := s.Intent().Record()
			r.ContainerID, r.Stage = strings.Repeat("e", 64), "absent"
			if failure = s.Intent().Save(r); failure != "" {
				t.Errorf("completion intent: %s", failure)
				s.Close()
				return
			}
			p := s.Record()
			p.State = "stopped"
			if failure = s.Save(p); failure != "" {
				t.Errorf("completion: %s", failure)
				s.Close()
				return
			}
			s.Close()
			if n == 0 {
				if _, failure = f.Registries[other].Register(ctx, f.Frozen[other], strings.Repeat("b", 64), token); failure != port.CheckoutOverlap {
					t.Errorf("released root with another session: %s", failure)
				}
			}
		}
		admitted, failure := f.Registries[other].Register(ctx, f.Frozen[other], strings.Repeat("b", 64), token)
		if failure != "" {
			t.Errorf("durable cleanup did not release: %s", failure)
			return
		}
		admitted.Close()
	}
	f := open()
	f.WriteFailure(true)
	if lease, failure := f.Registries[0].Register(context.Background(), f.Frozen[0], strings.Repeat("a", 64), strings.Repeat("1", 64)); failure == "" || lease != nil {
		t.Errorf("failed durable reservation granted a lease")
	}
	f.WriteFailure(false)
	lease, failure := f.Registries[1].Register(context.Background(), f.Frozen[1], strings.Repeat("b", 64), strings.Repeat("1", 64))
	if failure != "" {
		t.Errorf("failed unpublished reservation blocked retry: %s", failure)
		return
	}
	lease.Close()
}
