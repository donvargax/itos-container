package features

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	engineCallBudget    = time.Second
	engineStartupBudget = 3 * time.Second
	engineGraceBudget   = time.Second
	engineQuietInterval = max(engineCallBudget, engineStartupBudget) + time.Second
)

// Only the harness builds this fixture, outside every writable checkout. The
// product has no environment, flag, init hook or runtime setting for test limits.
// Go's overlay changes only the fixed durations, preserving the real code paths
// and clocks. The normal build and ordinary contract tests use the source file.
func engineBudgetOverlay(root, dir string) (string, error) {
	source := fmt.Sprintf(`package domain
import "time"
const (
 CallDuration = time.Duration(%d)
 StartupDuration = time.Duration(%d)
 GraceDuration = time.Duration(%d)
)
`, engineCallBudget, engineStartupBudget, engineGraceBudget)
	replacement := filepath.Join(dir, "T-33-engine-limits.go")
	if err := os.WriteFile(replacement, []byte(source), 0600); err != nil {
		return "", err
	}
	data, err := json.Marshal(struct{ Replace map[string]string }{map[string]string{
		filepath.Join(root, "internal", "engine", "budget", "domain", "limits.go"): replacement,
	}})
	if err != nil {
		return "", err
	}
	overlay := filepath.Join(dir, "T-33-engine-overlay.json")
	if err := os.WriteFile(overlay, data, 0600); err != nil {
		return "", err
	}
	return overlay, nil
}
