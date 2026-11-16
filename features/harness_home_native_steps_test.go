package features

import (
	"fmt"
	"reflect"

	"github.com/cucumber/godog"
)

func (w *world) homeNativeSteps(sc *godog.ScenarioContext) {
	sc.Step(`^a CI home checkout with a cached approved image for the actual non-root controller UID GID and isolated host-home sentinels$`, w.homeNativeFixture)
	sc.Step(`^two fresh product runs write then read a home marker and directly execute a copied ELF probe from /tmp$`, w.homeNativeRuns)
	sc.Step(`^both probes report the approved IDs and a writable private /home/itos and the second reads the first home marker$`, w.homeNativePersistence)
	sc.Step(`^each temporary ELF executes successfully while the second run cannot read the first temporary marker$`, func() error {
		for i, run := range w.home.native.runs {
			if !run.report.TmpExec || (i == 1 && run.report.TmpBefore) {
				return fmt.Errorf("run %d did not execute its temporary ELF or reused temporary state", i+1)
			}
		}
		return nil
	})
	sc.Step(`^independent inspection finds only the approved home checkout and tmpfs mounts with no host-home or image-home copy$`, w.homeNativeMounts)
	sc.Step(`^host-home sentinels and checkout ownership stay unchanged and both workers and anchors are absent while the home remains$`, func() error {
		f := w.home.native
		after, err := runTree(w.doctor.checkout)
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(after, f.checkoutBefore) {
			return fmt.Errorf("home runs changed checkout contents, ownership or inode identity")
		}
		host, err := runTree(f.sentinels)
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(host, f.hostBefore) {
			return fmt.Errorf("home runs changed host-home sentinels")
		}
		return w.homeNativeResources()
	})
}
