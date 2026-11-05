package porttest

import (
	"github.com/donvargax/itos-container/internal/engine/approval/port"
	"sync"
)

// SnapshotContract is shared unchanged by the memory and disk publisher.
func SnapshotContract(t Reporter, open func() (port.Snapshots, port.Frozen, port.Frozen)) {
	t.Helper()
	store, a, b := open()
	first, failure := store.Publish(a)
	if failure != "" || first.Frozen != a || first.Reference == "" {
		t.Errorf("first durable snapshot = %#v %s", first, failure)
		return
	}
	var wg sync.WaitGroup
	results := make(chan port.Durable, 2)
	failures := make(chan port.Failure, 2)
	for range 2 {
		wg.Go(func() { value, failure := store.Publish(a); results <- value; failures <- failure })
	}
	wg.Wait()
	close(results)
	close(failures)
	for value := range results {
		if value != first {
			t.Errorf("reuse changed handoff: %#v", value)
		}
	}
	for failure := range failures {
		if failure != "" {
			t.Errorf("reuse refused: %s", failure)
		}
	}
	conflict := b
	conflict.Generation = a.Generation
	if handoff, failure := store.Publish(conflict); failure != "snapshot.invalid" || handoff.Frozen != (port.Frozen{}) {
		t.Errorf("conflicting record accepted: %#v %s", handoff, failure)
	}
	next, failure := store.Publish(b)
	if failure != "" || next.Frozen != b || next.Reference == first.Reference {
		t.Errorf("new generation = %#v %s", next, failure)
	}
	bytes := next.Frozen.Bytes()
	if len(bytes) > 0 {
		bytes[0] = 'X'
	}
	if again, failure := store.Publish(a); failure != "" || again != first {
		t.Errorf("later generation changed A: %#v %s", again, failure)
	}
}
