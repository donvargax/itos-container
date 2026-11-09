package features

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
	dir := t.TempDir()
	tree, err := engineBudgetTree(root, dir)
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
	// Scenario worlds do not run the engine product itself: the fixture that
	// builds a checkout swaps in the assembly that honors its isolated account
	// home. Built through a world beside the engine product, that assembly must
	// carry the same shortened budgets, or every native wait runs at full length.
	t.Setenv("ITOS_CC_TEST_COVERDIR", t.TempDir())
	w := &world{root: root, dir: t.TempDir(), bin: filepath.Join(dir, "itos-container")}
	if err := w.doctorCheckout(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(w.closeDoctor)
	if !isProbeBinary(w.bin) || filepath.Dir(w.bin) != dir {
		t.Fatalf("the world's product is %q, want the probe assembly beside the engine product", w.bin)
	}
	if again, err := w.sessionProbeBinary(); err != nil || again != w.bin {
		t.Fatalf("a world already on the probe compiled another assembly: %q (%v), want %q", again, err, w.bin)
	}
	probe := exec.Command(w.bin, "-test.run", "^TestEngineFixtureBudgetContract$", "-test.v")
	probe.Dir = filepath.Join(tree, "cmd", "itos-container")
	output, err = probe.CombinedOutput()
	if err != nil || !strings.Contains(string(output), want) || !strings.Contains(string(output), "--- PASS: TestEngineFixtureBudgetContract") {
		t.Fatalf("the world's assembly did not enforce the engine budgets: %v\n%s", err, output)
	}
}

func TestProductSourceFollowsTheEngineFixture(t *testing.T) {
	root := t.TempDir()
	engine, other := t.TempDir(), t.TempDir()
	tree := filepath.Join(engine, engineTreeName)
	if err := os.MkdirAll(tree, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tree, "go.mod"), []byte("module m\n"), 0600); err != nil {
		t.Fatal(err)
	}
	// A fixture folder without its module is not a product tree.
	if err := os.MkdirAll(filepath.Join(other, engineTreeName), 0700); err != nil {
		t.Fatal(err)
	}
	for name, tc := range map[string]struct{ bin, want string }{
		"engine tier product":        {filepath.Join(engine, "itos-container"), tree},
		"its probe":                  {filepath.Join(engine, "itos-container-session-probe-cover.exe"), tree},
		"checkout product":           {filepath.Join(other, "itos-container"), root},
		"no module in the folder":    {filepath.Join(other, "product"), root},
		"relative path":              {"itos-container", root},
		"empty path":                 {"", root},
		"engine folder, other child": {filepath.Join(engine, "nested", "itos-container"), root},
	} {
		if got := productSource(root, tc.bin); got != tc.want {
			t.Errorf("%s: compiled %q, want %q", name, got, tc.want)
		}
	}
}

func TestProbeProductIsStable(t *testing.T) {
	for bin, want := range map[string]string{
		"/x/itos-container":                                 "/x/itos-container",
		"/x/itos-container-session-probe.exe":               "/x/itos-container",
		"/x/itos-container-session-probe-cover.exe":         "/x/itos-container",
		"/x/itos-container.exe-session-probe-cover.exe":     "/x/itos-container.exe",
		"/x-session-probe/itos-container":                   "/x-session-probe/itos-container",
		"/x/itos-container-session-probe.exe-session-probe": "/x/itos-container",
	} {
		if got := probeProduct(bin); got != want {
			t.Errorf("probeProduct(%q) = %q, want %q", bin, got, want)
		}
		if got := probeProduct(want + "-session-probe-cover.exe"); got != want {
			t.Errorf("a probe of %q names %q", want, got)
		}
	}
	if isProbeBinary("/x-session-probe/itos-container") || !isProbeBinary("/x/itos-container-session-probe.exe") {
		t.Error("only the file name marks a probe")
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
