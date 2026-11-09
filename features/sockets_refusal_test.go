package features

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"
)

func (w *world) socketStartupRun() error {
	s := w.sockets
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	s.fixtureErr = s.hostControl(ctx, "startup-before")
	if s.fixtureErr != nil {
		return nil // Then reports controls separately from product red evidence.
	}
	if err := w.offlineExecute([]string{"/bin/true"}, false); err != nil {
		return err
	}
	s.fixtureErr = s.hostControl(ctx, "startup-after")
	var err error
	w.offline.after, err = s.protected()
	s.fixtureErr = errors.Join(s.fixtureErr, err)
	return nil
}

// Check the historical request journal as well as the created-ID inventory:
// an unsuccessful create or a worker removed before observation still fails.
func socketRefusal(exit int, stderr []byte, requests []offlineRequest, created []string) error {
	creates := 0
	for _, request := range requests {
		if request.Method == "POST" && request.Path == "/containers/create" {
			creates++
		}
	}
	if creates != 0 || len(created) != 0 {
		return fmt.Errorf("socket checkout reached worker creation: exit=%d create requests=%d worker IDs=%d", exit, creates, len(created))
	}
	if exit != 1 || !strings.Contains(string(stderr), "run.isolation-unverified ") {
		return fmt.Errorf("missing typed socket isolation refusal: exit=%d; unrelated startup failure is not product red evidence", exit)
	}
	return nil
}

func (w *world) socketStartupRefused() (err error) {
	s, o := w.sockets, w.offline
	defer func() {
		if err != nil {
			err = fmt.Errorf("%w\nstartup diagnostics:\n%s", err, s.startupDiagnostics())
		}
	}()
	if err = s.hostControlsFor([]string{"startup"}); err != nil {
		return err
	}
	if o.err != nil {
		return fmt.Errorf("socket fixture/observation failure (not product red evidence): %w", o.err)
	}
	if o.api == nil {
		return errors.New("missing engine recorder (not product red evidence)")
	}
	a := o.api
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.problem != nil {
		return fmt.Errorf("socket recorder failure (not product red evidence): %w", a.problem)
	}
	return socketRefusal(o.exit, o.stderr, a.requests, a.created)
}

func (w *world) socketStartupPreserved() error {
	o := w.offline
	if o.before == nil || o.after == nil {
		return errors.New("missing socket refusal preservation observations")
	}
	for path, before := range o.before {
		if after, exists := o.after[path]; !exists || after != before {
			return fmt.Errorf("socket refusal changed an existing checkout or protected host path: %s", path)
		}
	}
	if o.resourcesBefore == nil || o.resourcesAfter == nil || !reflect.DeepEqual(o.resourcesBefore, o.resourcesAfter) {
		return errors.New("socket refusal changed unrelated engine resources or image references")
	}
	return nil
}
