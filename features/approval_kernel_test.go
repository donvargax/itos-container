package features

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const approvalKernelHelper = "slice-4-without-fchmodat2"

func (w *world) approvalLegacyKernel(umask string) (result error) {
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
	env := w.env()
	coverdir := os.Getenv("GOCOVERDIR")
	var collected string
	if coverdir != "" {
		// Keep this child's coverage separate: umask 0777 also applies to Go's
		// metadata/counter files, which are not part of the approval state.
		collected, err = os.MkdirTemp(coverdir, "slice-4-kernel-")
		if err != nil {
			return err
		}
		defer func() {
			result = errors.Join(result, os.RemoveAll(collected))
		}()
		env = setEnv(env, "GOCOVERDIR", collected)
	}
	if err := w.runEnv(w.doctor.cwd, env, helper,
		"-test.run=^TestApprovalKernelHelper$", "--", approvalKernelHelper, umask,
		w.bin, "approve", "--json", "--fingerprint", w.approval.fingerprint); err != nil {
		return err
	}
	if collected != "" {
		return exportApprovalKernelCoverage(collected, coverdir)
	}
	return nil
}

// Restore owner access only to this exited child's generated coverage files.
// Moving them within the caller's filesystem preserves every measured byte and
// leaves approval-state permissions and the product's umask untouched.
func exportApprovalKernelCoverage(source, destination string) error {
	entries, err := os.ReadDir(source)
	if err != nil {
		return err
	}
	var metadata, counters bool
	for _, entry := range entries {
		meta := strings.HasPrefix(entry.Name(), "covmeta.")
		counter := strings.HasPrefix(entry.Name(), "covcounters.")
		if !entry.Type().IsRegular() || (!meta && !counter) {
			return fmt.Errorf("unexpected older-kernel coverage entry %q", entry.Name())
		}
		path := filepath.Join(source, entry.Name())
		if err := os.Chmod(path, 0600); err != nil {
			return err
		}
		if err := os.Rename(path, filepath.Join(destination, entry.Name())); err != nil {
			return err
		}
		metadata = metadata || meta
		counters = counters || counter
	}
	if !metadata || !counters {
		return fmt.Errorf("older-kernel child did not export both coverage metadata and counters")
	}
	return nil
}
