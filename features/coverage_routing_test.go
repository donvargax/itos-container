package features

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/cucumber/godog"
)

func productCoverageRequested() bool {
	return os.Getenv("GOCOVERDIR") != "" || os.Getenv("ITOS_CC_TEST_COVERDIR") != ""
}

var coverageID = regexp.MustCompile(`^@ID-[A-Z]+-[0-9]+$`)

// Capture the destination before fixture setup can launch product code. Never
// change the process environment here: other worlds may be running concurrently
// or nested inside the native quiet-worker scheduler.
func (w *world) initializeCoverage(scenario *godog.Scenario) error {
	dir := os.Getenv("GOCOVERDIR")
	if root := os.Getenv("ITOS_CC_TEST_COVERDIR"); root != "" {
		var ids []string
		for _, tag := range scenario.Tags {
			if strings.HasPrefix(tag.Name, "@ID-") {
				if !coverageID.MatchString(tag.Name) {
					return fmt.Errorf("scenario coverage: invalid ID tag %q", tag.Name)
				}
				ids = append(ids, strings.TrimPrefix(tag.Name, "@"))
			}
		}
		if len(ids) != 1 {
			return fmt.Errorf("scenario coverage: expected exactly one ID tag, got %d", len(ids))
		}
		dir = filepath.Join(root, ids[0])
	}
	if dir == "" {
		return nil
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("scenario coverage destination %q: %w", dir, err)
	}
	if err := os.MkdirAll(abs, 0700); err != nil {
		return fmt.Errorf("create scenario coverage destination %q: %w", abs, err)
	}
	w.coverdir = abs
	return nil
}

func (w *world) coverageDir() string {
	if w.coverdir != "" {
		return w.coverdir
	}
	// Standalone harness contracts also construct worlds directly. They use a
	// private aggregate directory; scenario worlds always capture theirs above.
	return os.Getenv("GOCOVERDIR")
}

// Apply at environment construction, before any invocation-specific override.
// In particular, the restrictive-umask wrappers must retain their staging paths.
func (w *world) coverageEnv(env []string) []string {
	if dir := w.coverageDir(); dir != "" {
		return setEnv(env, "GOCOVERDIR", dir)
	}
	return env
}

// Independent harness regressions must neither require synthetic scenario IDs
// nor contaminate the external collector's inventory. Setenv restores both inputs.
func isolateProductCoverage(t *testing.T) {
	t.Helper()
	t.Setenv("ITOS_CC_TEST_COVERDIR", "")
	t.Setenv("GOCOVERDIR", t.TempDir())
}
