package port_test

import (
	"strings"
	"testing"

	"github.com/donvargax/itos-container/internal/engine/session/port"
)

// The default branch of ValidTransition holds authority that has published an
// intent or owned a worker: it may repeat itself, acquire the worker identity
// or complete to absent, and nothing else. Both conjuncts of the guard are
// load-bearing on their own: a hex container identity must not excuse rolling
// the stage back to intent, and an owned or absent stage without the complete
// container identity must never be accepted.
func TestPublishedAuthorityOnlyRepeatsAcquiresOrCompletes(t *testing.T) {
	intent := port.Record{Schema: 1, Stage: "intent"}
	worker := strings.Repeat("c", 64)
	owned := intent
	owned.Stage, owned.ContainerID = "owned", worker
	absent := owned
	absent.Stage = "absent"
	if !intent.ValidTransition(owned) {
		t.Fatal("valid acquisition refused")
	}
	if !owned.ValidTransition(owned) {
		t.Fatal("valid owned repeat refused")
	}
	if !owned.ValidTransition(absent) {
		t.Fatal("valid completion refused")
	}
	rollback := owned
	rollback.Stage = "intent"
	if owned.ValidTransition(rollback) {
		t.Fatal("ownership rolled back to intent")
	}
	unnamed := owned
	unnamed.ContainerID = ""
	if intent.ValidTransition(unnamed) {
		t.Fatal("owned accepted without container identity")
	}
	completion := absent
	completion.ContainerID = "short"
	if owned.ValidTransition(completion) {
		t.Fatal("completion accepted without container identity")
	}
}
