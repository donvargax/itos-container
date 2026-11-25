package features

import (
	"net/http"
	"reflect"
	"strings"
	"testing"
)

func TestTerminalCompletionIndependentlyObservesWorkerAbsence(t *testing.T) {
	for _, status := range []int{http.StatusNotFound, http.StatusOK, http.StatusServiceUnavailable} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			w, engine := createRecorderWorld(t, "plain")
			w.session.reference = "intents/fixture.json"
			id := strings.Repeat("a", 64)
			w.offline.api.created = []string{id}
			engine.override = func(_, _ string) (int, any) {
				return status, map[string]any{"Id": id}
			}
			n := &terminalNative{f: &terminalInvocation{w: w}, ready: true}
			err := n.complete()
			if got := engine.journal(); !reflect.DeepEqual(got, []string{"GET /containers/" + id + "/json"}) {
				t.Fatalf("completion omitted independent inspection: %v", got)
			}
			if w.offline.api.absent[id] != (status == http.StatusNotFound) {
				t.Fatal("absence requires an explicit independent 404")
			}
			// Inspection alone is insufficient: this fixture has no observed exit or
			// product removal journal, so completion must still refuse success.
			if err == nil {
				t.Fatal("claimed completion without workload and removal evidence")
			}
		})
	}
}
