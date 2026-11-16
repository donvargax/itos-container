package features

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	engineCallBudget    = time.Second
	engineStartupBudget = 3 * time.Second
	engineGraceBudget   = time.Second
	engineQuietInterval = max(engineCallBudget, engineStartupBudget) + time.Second
	// One observation allowance from controller start, including stream propagation.
	engineReadinessAllowance = 35 * time.Second
	// SIGKILL may leave this long after the grace expires. Absolute, as the 30 s
	// teardown assertion's 2 s is: scheduling on a hosted runner under -cover does
	// not shrink with the fixture's grace. It is no license for a timer that the
	// last repeat restarted in an immediate row, or that the first signal's
	// acknowledgement restarted in a delayed row: lifecycleSignals refuses those from
	// the instant they could fire (escalationWindow says when a cut hold lets one
	// through). A timer the first repeat restarted passes: a known gap, held by
	// p2-grace-origin-fake-clock-row.
	engineEscalationTolerance = 2 * time.Second
	// How long the native recorder keeps the engine side of a create it forwarded,
	// counted from forwarding, once nothing on the controller side can end it: the
	// production call limit, so the engine gets as long as a production controller
	// would have waited. Harness bookkeeping, not a product or scenario bound: the
	// fixture's product still gives up at engineCallBudget.
	engineCreateOutcomeAllowance = 10 * time.Second
)

// engineTreeName is the fixture's folder, beside the engine tier's product.
const engineTreeName = "T-33-engine-tree"

// The two kinds of test assembly that stand in for the product where a fixture
// isolates the account home. The session contract assembly always compiles the
// checkout; a world's product assembly compiles the engine tier's fixture tree.
const (
	sessionProbeSuffix = "-session-probe"
	engineProbeSuffix  = "-engine-probe"
)

// productSource is the module a world's product is compiled from: the engine
// tier's shortened-budget copy when one sits beside bin, which only that tier's
// buildProduct creates, else the checkout. A product compiled from the checkout
// carries the production limits, so a native scenario that waits out a call,
// startup or grace budget would wait for the full length.
func productSource(root, bin string) string {
	if filepath.IsAbs(bin) {
		tree := filepath.Join(filepath.Dir(bin), engineTreeName)
		if info, err := os.Stat(filepath.Join(tree, "go.mod")); err == nil && info.Mode().IsRegular() {
			return tree
		}
	}
	return root
}

func isProbeBinary(bin string) bool {
	name := filepath.Base(bin)
	return strings.Contains(name, sessionProbeSuffix) || strings.Contains(name, engineProbeSuffix)
}

// probeProduct names the product that bin, or the assembly built from it, stands
// for. A world already running an assembly derives the same paths again instead
// of compiling a second assembly of the same sources.
func probeProduct(bin string) string {
	dir, name := filepath.Split(bin)
	for _, suffix := range []string{sessionProbeSuffix, engineProbeSuffix} {
		if i := strings.Index(name, suffix); i >= 0 {
			name = name[:i]
		}
	}
	return dir + name
}

// Only the harness builds this fixture, from a disposable copy of the product
// sources outside every writable checkout. The product has no environment, flag,
// init hook or runtime setting for test limits. The copy differs from the checkout
// in one file, the fixed durations, so the real code paths and clocks are kept.
// The normal build and ordinary contract tests use the checkout's source file.
//
// A copy rather than go build -overlay: with -cover, which the CI engine tier
// always sets, cmd/go hands the cover tool each package file's path on disk and
// ignores the overlay, so the product kept its 10/120/10 s limits and every wait
// stayed long.
func engineBudgetTree(root, dir string) (string, error) {
	tree := filepath.Join(dir, engineTreeName)
	for _, name := range []string{"cmd", "internal"} {
		if err := os.CopyFS(filepath.Join(tree, name), os.DirFS(filepath.Join(root, name))); err != nil {
			return "", fmt.Errorf("copy %s for the engine budget fixture: %w", name, err)
		}
	}
	for _, name := range []string{"go.mod", "go.sum"} {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			return "", err
		}
		if err := os.WriteFile(filepath.Join(tree, name), data, 0600); err != nil {
			return "", err
		}
	}
	limits := filepath.Join(tree, "internal", "engine", "budget", "domain", "limits.go")
	// Replace, never create: a moved limits file must fail here, not leave the
	// production values in the harness's binary.
	if _, err := os.Stat(limits); err != nil {
		return "", fmt.Errorf("engine budget fixture cannot find the product limits: %w", err)
	}
	// The header also keeps this file's content, and so the build cache's key for
	// the package, distinct from the overlay fixture's: a cover build under that
	// overlay compiled the production limits and stored them under the key of the
	// fixture's text, and a cache restored from such a run would serve them here.
	source := fmt.Sprintf(`// Engine harness fixture limits: built only into the disposable engine product.
package domain
import "time"
const (
 CallDuration = time.Duration(%d)
 StartupDuration = time.Duration(%d)
 GraceDuration = time.Duration(%d)
)
`, engineCallBudget, engineStartupBudget, engineGraceBudget)
	if err := os.WriteFile(limits, []byte(source), 0600); err != nil {
		return "", err
	}
	return tree, nil
}
