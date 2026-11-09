package features

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

func (w *world) anchorRecoveryMemory() error {
	bin, err := w.sessionProbeBinary()
	if err != nil {
		return err
	}
	a := w.anchorLocalRecovery
	request := map[string]string{"mode": a.mode, "fault": a.fault, "phase": a.phase, "worker": a.worker, "volume": a.volume, "completion": a.completion, "exit": a.exit}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	args := []string{"-test.run=^TestAnchorRecoveryMemory$", "-test.count=1", "-test.v"}
	if cover := w.coverageDir(); cover != "" {
		args = append(args, "-test.gocoverdir="+cover)
	}
	c := exec.CommandContext(ctx, bin, args...)
	c.Env = append(w.env(), "SLICE_17_ANCHOR_RECOVERY="+string(approvalJSON(request)))
	data, err := c.CombinedOutput()
	a.memory = doctorResult{stdout: string(data)}
	if err != nil {
		a.memory.exit = 1
	}
	return nil
}
func (w *world) anchorRecoveryMemoryResult() error {
	r := w.anchorLocalRecovery.memory
	if r.exit != 0 || !strings.Contains(r.stdout, "--- PASS: TestAnchorRecoveryMemory ") {
		return fmt.Errorf("memory anchor recovery behavior: %s", r.stdout)
	}
	return nil
}
