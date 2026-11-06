package features

import (
	"errors"
	"fmt"
	"os"

	"github.com/cucumber/godog"
)

func (w *world) overlapProgressSteps(sc *godog.ScenarioContext) {
	sc.Step(`^an overlap fixture with concurrent public progress publication$`, func() error {
		return w.overlapSetup("child", "shared daemon", false)
	})
	sc.Step(`^an ancestor worker remains active and a disjoint run scans during its progress publication$`, func() {
		f := w.overlap
		f.first = f.start("ancestor", "", "")
		f.observe(f.first, false)
		f.checkpoint(f.first)
		f.worlds["prefix"].recoveryLocal.id = f.first.id
		f.contender = f.start("prefix", "", "public progress replacement")
		f.observe(f.contender, false)
	})
	sc.Step(`^the disjoint workload starts without a registration refusal while the original worker retains its full ID and ownership$`, func() error {
		f := w.overlap
		if err := f.live(f.contender); err != nil {
			got := f.contender.result()
			return fmt.Errorf("disjoint run refused concurrent progress: exit %d, stderr %q: %w", got.exit, got.stderr, err)
		}
		if err := f.live(f.first); err != nil {
			return err
		}
		marker, err := os.ReadFile(f.contender.probe.ready)
		if err != nil || string(marker) != "replaced" {
			return errors.New("progress publication did not overlap admission's record read")
		}
		current, err := sessionReadRecord(f.first.record)
		if err != nil || current.Bytes != f.first.public.Bytes || current.Inode == f.first.public.Inode {
			return errors.New("progress did not atomically replace the original valid public record")
		}
		// The replacement inode is intentional; all ownership bytes remain fixed.
		f.first.public = current
		return f.first.unchanged()
	})
	sc.Step(`^both checkouts approvals and snapshots remain unchanged through progress publication$`, func() error {
		return w.overlap.preserved()
	})
}
