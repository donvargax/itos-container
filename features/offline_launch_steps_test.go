package features

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/cucumber/godog"
)

func (w *world) offlineLaunchSteps(sc *godog.ScenarioContext) {
	sc.Step(`^an offline run contract fixture with approved settings and successful engine observations$`, w.offlineLaunchFixture)
	sc.Step(`^the offline contract executes an argv probe through both fake and disk SDK adapters$`, w.offlineLaunchAction)
	sc.Step(`^each offline run starts exactly one inspected owned worker after durable intent, owned ID, boundary verification and attach$`, func() error { return w.sessionEach((*world).offlineLaunchOrder) })
	sc.Step(`^each create and inspection has the exact frozen image, UID GID, workspace, argv, environment and hardened runtime configuration$`, func() error { return w.sessionEach((*world).offlineFrozenBoundary) })
	sc.Step(`^each offline run observes exit zero and confirms owned removal before returning zero$`, func() error { return w.sessionEach((*world).offlineLaunchSuccess) })
	sc.Step(`^no offline operation acquires images, executes host helpers or changes unrelated resources$`, func() error { return w.sessionEach((*world).offlineEffects) })

	sc.Step(`^an offline run contract fixture with independently seeded approved settings A and B$`, w.offlineFrozenFixture)
	sc.Step(`^run freezes A and the fixture replaces manifest environment, workspace request and active approval with B before create and a fresh run follows$`, w.offlineFrozenAction)
	sc.Step(`^the first offline worker actually starts with exactly A's image, UID GID, mounts, working directory and literal environment$`, func() error {
		return w.sessionEach(func(w *world) error {
			if err := w.offlineLaunchObserved(); err != nil {
				return err
			}
			first := w.session.launch.first
			if first == nil {
				return fmt.Errorf("first frozen run never completed")
			}
			if err := first.offlineLaunchOrder(); err != nil {
				return err
			}
			return first.offlineFrozenBoundary()
		})
	})
	sc.Step(`^the fresh offline worker starts with exactly B while A's snapshot remains unchanged$`, func() error {
		return w.sessionEach(func(w *world) error {
			if err := w.offlineLaunchOrder(); err != nil {
				return err
			}
			if err := w.offlineFrozenBoundary(); err != nil {
				return err
			}
			l := w.session.launch
			actual, err := sessionReadRecord(l.first.sessionSnapshotPath())
			if err != nil {
				return err
			}
			if actual.Bytes == "" || actual != l.snapshotA {
				return fmt.Errorf("A's selected snapshot was rewritten")
			}
			return nil
		})
	})
	sc.Step(`^neither invocation rereads policy or manifest values after its frozen selection$`, func() error {
		return w.sessionEach(func(w *world) error {
			l := w.session.launch
			if !l.watch || len(w.session.selectedReads) != 0 || len(l.first.session.selectedReads) != 0 {
				return fmt.Errorf("post-selection reads of mutable request/approval values: %v / %v", l.first.session.selectedReads, w.session.selectedReads)
			}
			return nil
		})
	})
	sc.Step(`^both offline runs preserve ownership, isolation and confirmed cleanup$`, func() error {
		return w.sessionEach(func(w *world) error {
			for _, run := range []*world{w.session.launch.first, w} {
				if err := run.offlineLaunchSuccess(); err != nil {
					return err
				}
				if err := run.offlineEffects(); err != nil {
					return err
				}
			}
			return nil
		})
	})

	sc.Step(`^an offline run contract fixture with a launch barrier at "([^"]*)"$`, w.offlineSubstitutionFixture)
	sc.Step(`^the offline run reaches that barrier and the fixture replaces "([^"]*)" with an outside symlink before release$`, func(path string) error {
		if err := w.sessionEach(func(w *world) error { w.session.launch.replaced = path; return nil }); err != nil {
			return err
		}
		return w.offlineLaunchAction()
	})
	sc.Step(`^the offline run refuses the changed checkout identity with exit 1 and no start$`, func() error {
		return w.sessionEach(func(w *world) error {
			if err := w.offlineLaunchControl(); err != nil {
				return err
			}
			s := w.session
			if !s.launch.reached || s.create.Code != 1 || s.create.Workload != nil || s.create.ID != "" || !strings.Contains(s.create.Diagnostic, "run.ownership-refused") {
				return fmt.Errorf("changed checkout allowed launch or lost identity refusal: exit %d outcome %q", s.create.Code, s.create.Outcome)
			}
			for _, event := range s.create.Trace {
				if event.Operation == "start" {
					return fmt.Errorf("changed checkout reached start")
				}
			}
			for _, q := range s.api.requests {
				if strings.HasSuffix(q.Path, "/start") {
					return fmt.Errorf("changed checkout sent start")
				}
			}
			return nil
		})
	})
	sc.Step(`^the outside target is neither mounted, modified nor used as the working directory$`, func() error { return w.sessionEach((*world).offlineOutsideUntouched) })
	sc.Step(`^any already created owned worker is cleaned without touching the substituted source$`, func() error {
		return w.sessionEach(func(w *world) error {
			s := w.session
			if s.launch.barrier == "before create" {
				if len(s.api.created) != 0 {
					return fmt.Errorf("changed checkout reached create")
				}
				for _, q := range s.api.requests {
					if q.Method == "POST" || q.Method == "DELETE" {
						return fmt.Errorf("changed checkout caused mutation %s", q.Path)
					}
				}
				return w.offlineEffects()
			}
			if err := w.offlineLocalCleanup(); err != nil {
				return err
			}
			return w.offlineEffects()
		})
	})

	sc.Step(`^an offline run contract fixture with observed workload exit (\d+) and cleanup cause "([^"]*)"$`, w.offlineCleanupFixture)
	sc.Step(`^the offline contract finishes through both fake and disk SDK adapters$`, w.offlineLaunchAction)
	sc.Step(`^each offline result returns (\d+) and separately reports "([^"]*)" with cleanup classification (\d+)$`, func(code int, rule string, cleanup int) error {
		return w.sessionEach(func(w *world) error {
			if err := w.offlineLaunchControl(); err != nil {
				return err
			}
			r := w.session.create
			if r.Code != code || r.CleanupCode != cleanup || !strings.Contains(r.Diagnostic, rule+" ") {
				return fmt.Errorf("cleanup result got exit %d classification %d diagnostic %q; want %d / %d / %s", r.Code, r.CleanupCode, r.Diagnostic, code, cleanup, rule)
			}
			return nil
		})
	})
	sc.Step(`^each offline result preserves the observed workload exit and reports incomplete cleanup with its safe protected intent reference$`, func() error { return w.sessionEach((*world).offlineIncomplete) })
	sc.Step(`^no offline result claims stopped resources or advertises an unimplemented stop retry$`, func() error {
		return w.sessionEach(func(w *world) error {
			for _, text := range []string{"cleanup-complete", "stopped", "itos container stop"} {
				if strings.Contains(w.session.create.Diagnostic, text) {
					return fmt.Errorf("incomplete cleanup advertises %q", text)
				}
			}
			return nil
		})
	})
	sc.Step(`^retained offline ownership and diagnostics contain no raw argv environment response body or copied workload output$`, func() error {
		return w.sessionEach(func(w *world) error {
			r := w.session.create
			if r.Stdout != "slice-13-workload-out\n" || !strings.HasPrefix(r.Stderr, "slice-13-workload-err\n") {
				return fmt.Errorf("lost or merged workload streams")
			}
			diagnostics := strings.TrimPrefix(r.Stderr, "slice-13-workload-err\n")
			for _, marker := range []string{"literal;$HOME", "slice-13-private-argv", "slice-13-private-env", "slice-13-private-response", "slice-13-workload-out", "slice-13-workload-err", "slice-13-private-adapter-failure", doctorCanary} {
				if strings.Contains(diagnostics+w.session.final.Bytes+w.session.transcript, marker) {
					return fmt.Errorf("ownership or diagnostic copied private fixture input")
				}
			}
			return nil
		})
	})
}

func (w *world) offlineOutsideUntouched() error {
	l := w.session.launch
	actual, err := doctorTree(l.outside)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(actual, l.outsideBefore) {
		return fmt.Errorf("outside bytes or modes changed")
	}
	for _, q := range w.session.api.requests {
		if strings.Contains(string(approvalJSON(q.Body)), l.outside) {
			return fmt.Errorf("outside path reached an engine request")
		}
	}
	return nil
}
