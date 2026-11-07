package features

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

func (w *world) overlapSteps(sc *godog.ScenarioContext) {
	w.overlapProgressSteps(sc)
	w.overlapAbandonmentSteps(sc)
	sc.Step(`^an overlap fixture with descendant "([^"]*)" and "([^"]*)" local engine fixtures$`, func(child, engines string) error { return w.overlapSetup(child, engines, false) })
	sc.Step(`^an overlap fixture with a pre-create barrier for "([^"]*)" after durable registration and intent publication$`, func(first string) error {
		if err := w.overlapSetup("child", "shared daemon", false); err != nil {
			return err
		}
		w.overlap.firstName, w.overlap.boundary = first, "before create"
		return nil
	})
	sc.Step(`^an overlap fixture with "([^"]*)" local engine fixtures and a shared preflight rendezvous$`, func(engines string) error {
		if err := w.overlapSetup("child", engines, false); err != nil {
			return err
		}
		w.overlap.configureRace()
		return nil
	})
	sc.Step(`^an overlap fixture with an ancestor run configured to leave "([^"]*)"$`, func(condition string) error {
		if err := w.overlapSetup("child", "shared daemon", false); err != nil {
			return err
		}
		f := w.overlap
		f.condition = condition
		a := f.engines["ancestor"]
		a.faultRoot = f.worlds["ancestor"].doctor.checkout
		// Observation loss is armed now and activated after controller loss;
		// prospective preflight reads remain available throughout.
		if condition != "controller absent with resource observation unavailable" {
			a.condition = condition
		}
		return nil
	})
	sc.Step(`^an overlap fixture with a post-create pre-start barrier for "([^"]*)" outside its session operation lock$`, func(first string) error {
		if err := w.overlapSetup("child", "shared daemon", false); err != nil {
			return err
		}
		w.overlap.firstName, w.overlap.boundary = first, "created before start"
		return nil
	})
	sc.Step(`^an overlap fixture with an existing registered "([^"]*)" worker and no admission-specific metadata$`, func(first string) error {
		if err := w.overlapSetup("child", "shared daemon", false); err != nil {
			return err
		}
		w.overlap.firstName = first
		return w.overlap.seedLegacy()
	})
	sc.Step(`^an overlap fixture with two independently approved invocations of the ancestor checkout$`, func() error { return w.overlapSetup("child", "shared daemon", false) })
	sc.Step(`^a CI overlap fixture with nested checkouts the cached image and distinct heartbeat workloads$`, func() error { return w.overlapSetup("child", "shared daemon", true) })
	sc.Step(`^a CI overlap fixture with nested checkouts the cached image and a shared preflight rendezvous$`, func() error {
		if err := w.overlapSetup("child", "shared daemon", true); err != nil {
			return err
		}
		w.overlap.configureRace()
		return nil
	})
	sc.Step(`^run starts "([^"]*)" and a fresh run attempts the other nested checkout while the first workload remains active$`, func(first string) { w.overlap.firstName = first; w.overlap.sequential() })
	sc.Step(`^the product runs "([^"]*)" to its first heartbeat and attempts the other nested checkout$`, func(first string) { w.overlap.firstName = first; w.overlap.sequential() })
	sc.Step(`^the first run pauses at the pre-create barrier and a fresh run attempts the other nested checkout$`, func() { w.overlap.pausedAttempt() })
	sc.Step(`^the first run pauses after durable ownership and a fresh run attempts the other nested checkout$`, func() { w.overlap.pausedAttempt() })
	sc.Step(`^independent ancestor and descendant run processes leave the rendezvous together and their create replies are observed before release$`, func() { w.overlap.race() })
	sc.Step(`^independent native ancestor and descendant run processes leave the rendezvous together$`, func() { w.overlap.race() })
	sc.Step(`^a fresh run attempts the other nested checkout before recovery of the existing worker$`, func() { w.overlap.attempt() })
	sc.Step(`^run reaches that condition and a fresh descendant run is attempted before product recovery$`, func() { w.overlap.faultAttempt() })
	sc.Step(`^both ancestor runs start and the descendant run is attempted before and after product stop completes only the first ancestor session$`, func() {
		f := w.overlap
		f.first = f.start("ancestor", "", "")
		f.observe(f.first, false)
		f.checkpoint(f.first)
		f.second = f.start("ancestor", "", "")
		f.observe(f.second, false)
		f.checkpoint(f.second)
		f.attempt()
		f.stop(f.first)
		f.attempt()
	})
	sc.Step(`^runs start the textual-prefix and equal-basename disjoint controls while the first workload remains active$`, func() { w.overlap.startControls("prefix", "basename") })
	sc.Step(`^a disjoint run starts before the first barrier is released and the first run then resumes$`, func() {
		f := w.overlap
		f.startControls("prefix")
		f.remember(f.first.probe.resume())
		f.first.paused = false
		f.observe(f.first, false)
	})
	sc.Step(`^a disjoint run starts while the winning nested workload remains active$`, func() { w.overlap.startControls("prefix") })
	sc.Step(`^the fixture clears the fault and a fresh product stop completes the ancestor session before another descendant run$`, func() {
		f := w.overlap
		a := f.engines["ancestor"]
		a.mu.Lock()
		a.condition = ""
		a.mu.Unlock()
		f.recoverAndStart()
	})
	sc.Step(`^product stop completes the existing session and a fresh contender run is attempted$`, func() { w.overlap.recoverAndStart() })
	sc.Step(`^product stop completes the second ancestor session and a fresh descendant run is attempted$`, func() {
		f := w.overlap
		f.stop(f.second)
		f.contender = f.start("descendant", "", "")
		f.observe(f.contender, false)
	})
	sc.Step(`^product stop completes the paused session and a fresh run targets the other nested checkout before the original controller is released$`, func() {
		f := w.overlap
		f.recoverAndStart()
		f.checkpoint(f.contender)
		f.first.beforeResume = len(f.first.engine.journal())
	})
	sc.Step(`^the original controller resumes while the newly admitted nested worker remains active$`, func() {
		f := w.overlap
		f.remember(f.first.probe.resume())
		f.first.paused = false
		f.remember(overlapAwait("stale controller exits", 35*time.Second, f.first.ended))
		_ = f.first.result()
	})
	sc.Step(`^product run starts a disjoint heartbeat control and product stop completes the original nested session before a fresh contender run$`, func() { f := w.overlap; f.startControls("prefix"); f.recoverAndStart() })
	sc.Step(`^product stop completes the winning session and a fresh run targets the losing checkout$`, func() { w.overlap.recoverAndStart() })

	checks := map[string]func(*overlapFixture) error{
		`^the nested contender reports run.checkout-overlap with exit 1 and original-checkout recovery guidance before any resource creation$`: func(f *overlapFixture) error { return f.refused(f.contender) },
		`^the native nested contender reports run.checkout-overlap with exit 1 and creates no worker or workload marker$`:                      func(f *overlapFixture) error { return f.refused(f.contender) },
		`^the first workload remains active with its original full ID and unchanged ownership$`: func(f *overlapFixture) error {
			if err := f.live(f.first); err != nil {
				return err
			}
			return f.first.unchanged()
		},
		`^the original native worker keeps the same full ID mount and advancing heartbeat through the refusal$`: func(f *overlapFixture) error {
			if err := f.live(f.first); err != nil {
				return err
			}
			return f.first.unchanged()
		},
		`^both disjoint workloads start and remain independently observable without changing either nested checkout or its approval$`: func(f *overlapFixture) error {
			if len(f.controls) != 2 {
				return errors.New("missing disjoint controls")
			}
			return f.controlsLive()
		},
		`^neither nested invocation has created or started a worker while the first barrier is held$`: func(f *overlapFixture) error {
			if err := f.prepared(); err != nil {
				return err
			}
			if f.first.worker != "" || f.contender.worker != "" {
				return errors.New("pre-create barrier did not exclude worker creation")
			}
			return nil
		},
		`^the disjoint workload starts without waiting for the barrier and the first nested run starts exactly one owned worker after release$`: func(f *overlapFixture) error {
			if err := f.live(f.first); err != nil {
				return err
			}
			if f.creates(f.first) != 1 {
				return errors.New("resumed run did not create exactly one worker")
			}
			return f.controlsLive()
		},
		`^exactly one nested invocation reaches create and the other reports run.checkout-overlap with exit 1 before any engine mutation$`: func(f *overlapFixture) error { return f.raceAssertion() },
		`^after reply release exactly one owned nested worker starts and the losing invocation has no workload output or marker$`: func(f *overlapFixture) error {
			if f.gate != nil {
				close(f.gate)
				f.gate = nil
			}
			f.observe(f.first, false)
			f.checkpoint(f.first)
			if err := f.live(f.first); err != nil {
				return err
			}
			return f.refused(f.contender)
		},
		`^both admitted workloads remain independently active and the losing invocation has not changed their ownership or approvals$`: func(f *overlapFixture) error {
			if err := f.live(f.first); err != nil {
				return err
			}
			if err := f.first.unchanged(); err != nil {
				return err
			}
			return f.controlsLive()
		},
		`^exactly one native nested run produces an owned worker and heartbeat while the other reports run.checkout-overlap with exit 1 and no created resource$`: func(f *overlapFixture) error {
			if err := f.raceAssertion(); err != nil {
				return err
			}
			f.observe(f.first, false)
			f.checkpoint(f.first)
			return f.live(f.first)
		},
		`^independent native inspection finds a checkout mount only for the winning nested invocation and the losing workload marker is absent$`: func(f *overlapFixture) error {
			if err := f.live(f.first); err != nil {
				return err
			}
			return f.refused(f.contender)
		},
		`^the original registration intent snapshot and unresolved ownership remain available without contender-triggered cleanup or adoption$`:             func(f *overlapFixture) error { return f.first.unchanged() },
		`^the descendant run starts only after confirmed original full-ID absence and durable cleanup while original edits approvals and snapshots remain$`: func(f *overlapFixture) error { return f.replacementAssertion() },
		`^the paused run has exactly one durably owned unstarted worker and no workload output$`:                                                            func(f *overlapFixture) error { return f.pausedOwned() },
		`^stop confirms original full-ID absence and durable cleanup and the fresh contender starts while the original controller remains paused$`: func(f *overlapFixture) error {
			if !f.first.paused || f.first.ended() {
				return errors.New("replacement waited for controller exit")
			}
			return f.replacementAssertion()
		},
		`^the resumed controller exits nonzero with empty workload output and no create or start request after completed cleanup$`: func(f *overlapFixture) error {
			if f.err != nil {
				return f.err
			}
			r := f.first
			got := r.result()
			if !r.ended() || got.exit <= 0 || got.stdout != "" {
				return errors.New("stale controller reported success or ran a workload")
			}
			for _, q := range r.engine.journal()[r.beforeResume:] {
				if q.pid == r.pid() && q.method == "POST" && (q.path == "/containers/create" || strings.HasSuffix(q.path, "/start")) {
					return errors.New("stale controller launched after completion")
				}
			}
			return nil
		},
		`^fresh original-checkout status reports stopped absent complete and null workload exit while the newly admitted worker keeps its full ID ownership and activity$`: func(f *overlapFixture) error { return f.staleStatus() },
		`^both nested checkouts approvals and snapshots remain unchanged by the stale continuation$`:                                                                       func(f *overlapFixture) error { return f.preserved() },
		`^the existing worker and its original protected ownership remain unchanged without migration or adoption$`: func(f *overlapFixture) error {
			if err := f.live(f.first); err != nil {
				return err
			}
			return f.first.unchanged()
		},
		`^the contender starts after confirmed original full-ID absence and durable cleanup while the original completed records remain discoverable$`: func(f *overlapFixture) error { return f.replacementAssertion() },
		`^both descendant attempts report run.checkout-overlap with exit 1 before resource creation while the second ancestor worker stays active$`: func(f *overlapFixture) error {
			if len(f.refusals) != 2 {
				return errors.New("missing descendant attempts")
			}
			if err := f.allRefused(); err != nil {
				return err
			}
			return f.live(f.second)
		},
		`^the first ancestor session alone has confirmed durable cleanup and the second retains its own original ownership$`: func(f *overlapFixture) error {
			if err := f.complete(f.first); err != nil {
				return err
			}
			return f.second.unchanged()
		},
		`^the descendant run starts despite retained completed ancestor records and preserves both ancestors' workload outcomes edits approvals and snapshots$`: func(f *overlapFixture) error {
			if err := f.live(f.contender); err != nil {
				return err
			}
			return f.sameOutcomes()
		},
		`^the formerly refused checkout starts its own native heartbeat only after confirmed original full-ID absence and durable cleanup$`:     func(f *overlapFixture) error { return f.replacementAssertion() },
		`^the disjoint native heartbeat continues while checkout edits approvals snapshots and unrelated resources survive$`:                    func(f *overlapFixture) error { return f.controlsLive() },
		`^the losing checkout starts a new owned native worker after durable winner cleanup without changing approvals or unrelated resources$`: func(f *overlapFixture) error { return f.replacementAssertion() },
	}
	for pattern, check := range checks {
		sc.Step(pattern, func() error { return check(w.overlap) })
	}
}

func (f *overlapFixture) attempt() {
	f.contender = f.start(f.other(), "", "")
	f.observe(f.contender, true)
	f.refusals = append(f.refusals, f.contender)
}
func (f *overlapFixture) sequential() {
	f.first = f.start(f.firstName, "", "")
	f.observe(f.first, false)
	f.checkpoint(f.first)
	f.attempt()
}
func (f *overlapFixture) pausedAttempt() {
	f.first = f.start(f.firstName, f.boundary, "")
	if f.first.probe == nil {
		return
	}
	f.first.paused = f.first.probe.await()
	f.first.discover()
	f.checkpoint(f.first)
	f.attempt()
}
func (f *overlapFixture) startControls(names ...string) {
	for _, name := range names {
		r := f.start(name, "", "")
		f.controls = append(f.controls, r)
		f.observe(r, false)
		f.checkpoint(r)
	}
}
func (f *overlapFixture) recoverAndStart() {
	f.stop(f.first)
	f.contender = f.start(f.other(), "", "")
	f.observe(f.contender, false)
}
func (f *overlapFixture) replacementAssertion() error {
	if err := f.complete(f.first); err != nil {
		return err
	}
	if err := f.live(f.contender); err != nil {
		return err
	}
	return f.preserved()
}

func (f *overlapFixture) configureRace() {
	f.rendezvous = newOverlapRendezvous()
	if !f.native {
		f.gate = make(chan struct{})
		for _, a := range f.engines {
			a.preflight = f.rendezvous
			a.createGate = f.gate
		}
		return
	}
	for i, name := range []string{"ancestor", "descendant"} {
		p := f.worlds[name]
		handler := p.offline.api.server.Handler
		p.offline.api.server.Handler = http.HandlerFunc(func(out http.ResponseWriter, r *http.Request) {
			if r.Method == "GET" && doctorRequestPath(r.URL.Path) == "/images/"+p.doctor.image+"/json" {
				response := httptest.NewRecorder()
				handler.ServeHTTP(response, r)
				f.rendezvous.wait(r.Context(), i+1)
				for k, values := range response.Header() {
					out.Header()[k] = values
				}
				out.WriteHeader(response.Code)
				_, _ = out.Write(response.Body.Bytes())
				return
			}
			handler.ServeHTTP(out, r)
		})
	}
}

func (f *overlapFixture) race() {
	a, b := f.start("ancestor", "", ""), f.start("descendant", "", "")
	f.remember(overlapAwait("both preflight inspections", 10*time.Second, func() bool { return f.rendezvous.count() == 2 || a.ended() || b.ended() }))
	if f.rendezvous.count() != 2 {
		f.remember(errors.New("both processes did not reach successful preflight"))
	}
	close(f.rendezvous.release)
	f.remember(overlapAwait("each contender creates or refuses", 8*time.Second, func() bool {
		a.discover()
		b.discover()
		return (a.worker != "" || a.ended()) && (b.worker != "" || b.ended())
	}))
	f.first, f.contender, f.firstName = a, b, "ancestor"
	if a.worker == "" && b.worker != "" {
		f.first, f.contender, f.firstName = b, a, "descendant"
	}
}

func (f *overlapFixture) creates(r *overlapRun) int {
	count := 0
	if r.engine != nil {
		for _, q := range r.engine.journal() {
			if q.pid == r.pid() && q.method == "POST" && q.path == "/containers/create" {
				count++
			}
		}
		return count
	}
	a := r.w.offline.api
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.created)
}
func (f *overlapFixture) raceAssertion() error {
	if f.err != nil {
		return f.err
	}
	if f.creates(f.first) != 1 || f.creates(f.contender) != 0 {
		return fmt.Errorf("concurrent nested runs must admit exactly one create; got %d and %d", f.creates(f.first), f.creates(f.contender))
	}
	return f.refused(f.contender)
}

func (f *overlapFixture) faultAttempt() {
	fault := ""
	if f.condition == "resource removed with session completion fsync failed" {
		fault = "session completion parent fsync"
	}
	f.first = f.start("ancestor", "", fault)
	if strings.HasPrefix(f.condition, "controller absent") {
		f.observe(f.first, false)
		if f.first.active() {
			f.remember(f.first.process.killController())
			f.first.process.wait()
		}
		if f.condition == "controller absent with resource observation unavailable" {
			a := f.engines["ancestor"]
			a.mu.Lock()
			a.condition = f.condition
			a.mu.Unlock()
		}
	} else {
		f.remember(overlapAwait("faulted run returns", 40*time.Second, f.first.ended))
		_ = f.first.result()
		f.first.discover()
	}
	f.checkpoint(f.first)
	f.attempt()
}

func (f *overlapFixture) seedLegacy() error {
	w := f.worlds[f.firstName]
	r := &overlapRun{w: w, engine: f.engines[f.firstName], id: strings.Repeat("b", 64), token: strings.Repeat("a", 64), worker: strings.Repeat("c", 64)}
	f.first = r
	f.runs = append(f.runs, r)
	w.session.token = r.token
	w.session.reference = "intents/" + w.approval.project + "/" + r.token + ".json"
	r.intent = w.sessionRecordPath()
	r.record = filepath.Join(w.doctor.env["XDG_STATE_HOME"], "itos-container", "sessions", w.recoveryIndexKey(), r.id, "record.json")
	if err := writeManifestFixture(r.intent, approvalJSON(w.sessionExpectedRecord("owned", r.worker)), 0600); err != nil {
		return err
	}
	public := map[string]any{"schema": 1, "id": r.id, "project": w.approval.project, "intent": w.session.reference, "state": "running", "workload_exit": nil, "last_problem": nil}
	if err := writeManifestFixture(r.record, approvalJSON(public), 0600); err != nil {
		return err
	}
	for _, name := range []string{"controller.lock", "operation.lock"} {
		if err := os.WriteFile(filepath.Join(filepath.Dir(r.record), name), nil, 0600); err != nil {
			return err
		}
	}
	r.engine.workers[r.worker] = &overlapWorker{root: w.doctor.checkout, token: r.token, exited: make(chan struct{}), value: map[string]any{
		"Id": r.worker, "Name": "/itos-container-" + r.token, "Image": w.doctor.imageID, "Config": map[string]any{"Labels": w.sessionLabels()}, "State": map[string]any{"Status": "running", "Running": true, "Pid": 123, "StartedAt": "2026-01-01T00:00:00Z"},
	}}
	return r.capture()
}
