package features

import (
	"os"
	"path/filepath"
	"time"
)

const approvalKernelHelper = "slice-4-without-fchmodat2"

func (w *world) approvalLegacyKernel(umask string) error {
	if err := w.approvalOracle(); err != nil {
		return err
	}
	if err := w.approvalCapture(); err != nil {
		return err
	}
	helper, err := os.Executable()
	if err != nil {
		return err
	}
	// A failed initialization under 0777 may leave an unsearchable directory.
	// Restore fixture access only during cleanup, after the behavior assertions.
	w.doctor.cleanup = append(w.doctor.cleanup, func() {
		base := filepath.Join(w.doctor.env["XDG_STATE_HOME"], "itos-container")
		for _, path := range []string{base, filepath.Join(base, "approvals"), filepath.Dir(w.approval.record)} {
			_ = os.Chmod(path, 0700)
		}
	})
	w.deadline = 8 * time.Second
	w.approval.mode = "--json"
	return w.runEnv(w.doctor.cwd, w.env(), helper,
		"-test.run=^TestApprovalKernelHelper$", "--", approvalKernelHelper, umask,
		w.bin, "approve", "--json", "--fingerprint", w.approval.fingerprint)
}
