package features

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"reflect"

	"github.com/cucumber/godog"
)

// Independently encode the specified protected lookup key, never derive it
// from current .git contents or the product's own output.
func (w *world) recoveryIndexKey() string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte("itos-container/session-index/v1\x00"+w.approval.uid+"\x00"+w.doctor.checkout)))
}

func (w *world) recoveryCheckoutSteps(sc *godog.ScenarioContext) {
	sc.Step(`^the worker Git directory is "([^"]*)" and fresh status and stop run$`, func(change string) error {
		git := filepath.Join(w.doctor.checkout, ".git")
		if err := os.Rename(git, git+".original"); err != nil {
			return err
		}
		var err error
		switch change {
		case "replaced":
			err = os.Mkdir(git, 0700)
		case "missing":
		case "symlinked":
			err = os.Symlink(".git.original", git)
		case "a file":
			err = os.WriteFile(git, []byte("hostile checkout data"), 0600)
		default:
			return fmt.Errorf("unknown Git replacement %q", change)
		}
		if err != nil {
			return err
		}
		if err := w.recoveryCapture(); err != nil {
			return err
		}
		return w.recoveryStatusStop()
	})
	sc.Step(`^status finds the original running session without a controller and stop completes its owned cleanup$`, func() error {
		entry, err := w.recoveryLocalEntry(w.recoveryLocal.results[0], "running", "running", "pending", "unknown")
		if err != nil {
			return err
		}
		if entry.Controller != "absent" || !entry.StopCandidate || !reflect.DeepEqual(entry.Retry, []string{"itos", "container", "stop", w.recoveryLocal.id}) {
			return fmt.Errorf("lost controller did not retain an executable recovery candidate")
		}
		if _, err := w.recoveryLocalEntry(w.recoveryLocal.results[1], "stopped", "absent", "complete", "unknown"); err != nil {
			return err
		}
		return w.recoveryLocalDurable()
	})
}
