package features

import (
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"testing/synctest"
	"time"
)

func testEngineBudgetFixture(t *testing.T) {
	t.Helper()
	root, err := moduleRoot()
	if err != nil {
		t.Fatal(err)
	}
	tree, err := engineBudgetTree(root, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	// The harness's CI product build always requests coverage, so check the
	// fixture under the same instrumentation: it is what dropped an overlay.
	cmd := exec.Command("go", "test", "-cover", "-coverpkg=./...", "./cmd/itos-container", "-count=1", "-v", "-run", "^TestEngineFixtureBudgetContract$")
	cmd.Dir = tree
	output, err := cmd.CombinedOutput()
	want := fmt.Sprintf("compiled budgets: call=%s startup=%s grace=%s", engineCallBudget, engineStartupBudget, engineGraceBudget)
	if err != nil || !strings.Contains(string(output), want) || !strings.Contains(string(output), "--- PASS: TestEngineFixtureBudgetContract") {
		t.Fatalf("compiled fixture did not enforce its budgets: %v\n%s", err, output)
	}
}

func TestEngineFixtureQuietBudget(t *testing.T) {
	ready := time.Now()
	w := &world{lifecycle: &lifecycleNative{condition: "quiet workload and stdin", ready: ready, signalled: ready.Add(4 * time.Second)}}
	if err := w.lifecycleQuietVerified(); err != nil {
		t.Fatalf("quiet observation beyond the 3 s fixture startup budget was refused: %v", err)
	}
	w.lifecycle.signalled = ready.Add(3 * time.Second)
	if err := w.lifecycleQuietVerified(); err == nil {
		t.Fatal("quiet observation at the startup boundary was accepted")
	}
}

func TestEngineFixtureGraceBudget(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		start := time.Now()
		p := &recoveryProcess{done: make(chan struct{})}
		go func() { time.Sleep(time.Second); close(p.done) }()
		var times []time.Duration
		lifecycleRepeatedSignals(p, start, func(string) { times = append(times, time.Since(start)) }, func() bool { return true })
		<-p.done
		if len(times) != 2 || times[0] != 200*time.Millisecond || times[1] != 800*time.Millisecond {
			t.Fatalf("repetitions must remain inside the 1 s fixture grace: %v", times)
		}
	})
}
