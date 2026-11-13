package features

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/cucumber/godog"
)

// One invocation of a step that runs several commands: its own output, exit and
// Engine API journal, so every Then applies to each and not only to the last.
type egressResult struct {
	command string
	runObservation
}

type egressWorld struct {
	approved, requested []string // the approved list and the edited request, for value-free refusals
	results             []egressResult
	copyBefore          map[string]string
	contract            map[string]any
	contractBefore      map[string]string
	contractDir         string
	status              *egressStatus
}

func (w *world) egressFixture() *egressWorld {
	if w.egress == nil {
		w.egress = &egressWorld{}
	}
	return w.egress
}

func (w *world) egressSteps(sc *godog.ScenarioContext) {
	sc.Step(`^an egress-capable local Engine API fixture$`, w.localEgressFixture)
	sc.Step(`^the egress request is edited to "([^"]*)"$`, w.egressEdit)
	sc.Step(`^the egress request has equivalent spelling "([^"]*)"$`, w.egressSpelling)
	sc.Step(`^the egress refusal says exactly "([^"]*)" without either list's values$`, w.egressChangedMessage)
	sc.Step(`^the egress-only prerequisites are unavailable while offline readiness is supported$`, w.egressOfflineOnly)
	sc.Step(`^only the pinned proxy image is absent from the egress fixture$`, func() error {
		return w.egressScript(func(a *localEgressAPI) error { a.proxyStatus = 404; return nil })
	})
	sc.Step(`^only the proxy image observation has defect "([^"]*)"$`, func(defect string) error {
		return w.egressScript(func(a *localEgressAPI) error { return egressImageDefect(a.proxyImage, defect) })
	})
	sc.Step(`^the egress fixture advertises Docker "([^"]*)" with negotiated API "([^"]*)"$`, w.egressDaemon)
	sc.Step(`^the egress fixture returns boundary defect "([^"]*)"$`, func(defect string) error {
		return w.egressScript(func(a *localEgressAPI) error {
			for _, known := range egressBoundaryDefects {
				if known == defect {
					a.defect = defect
					return nil
				}
			}
			return fmt.Errorf("unknown boundary defect %q", defect)
		})
	})
	sc.Step(`^a copy of the built product executable inside that temporary checkout$`, w.egressCopyController)
	sc.Step(`^the active approval record is snapshotted$`, func() error {
		// Only reads: the stored record's bytes, inode and mtime become the
		// baseline the second invocation must leave untouched.
		if err := w.approvalCapture(); err != nil {
			return err
		}
		if w.approval.recordInfo == nil || len(w.approval.recordBefore) == 0 {
			return fmt.Errorf("no stored approval record to snapshot")
		}
		return nil
	})

	sc.Step(`^ordinary doctor inspects the egress request in plain and JSON modes$`, func() error {
		return w.egressInvoke([]string{"doctor"}, []string{"doctor", "--json"})
	})
	sc.Step(`^doctor approve and run inspect that egress request$`, func() error {
		return w.egressInvoke([]string{"doctor", "--json"}, []string{"approve", "--json"}, []string{"run", "--", "fixture-workload"})
	})
	sc.Step(`^that copied executable runs with "([^"]*)"$`, func(args string) error {
		words, err := commandWords(args)
		if err != nil {
			return err
		}
		return w.egressInvoke(words)
	})

	sc.Step(`^the local egress run completes the owned worker with the exact literal argv and exit zero$`, w.egressRunCompleted)
	sc.Step(`^the proxy create uses exactly the frozen canonical list and independently inspected proxy config ID$`, w.egressProxyCreate)
	sc.Step(`^the original proxy config ID survives reopening the protected session binding$`, w.egressEvidenceSurvives)
	sc.Step(`^the local egress journal confirms ordered ownership verification readiness and complete owned cleanup$`, w.egressJournal)
	sc.Step(`^the worker uses network none without proxy variables or egress resources$`, w.egressOfflineWorker)
	sc.Step(`^no proxy image or executable inspection was attempted$`, w.egressNoProxyInspection)
	sc.Step(`^doctor reports ready only after inspecting both pinned images and the installed static executable$`, w.egressDoctorReady)
	sc.Step(`^plain doctor names the exact list and proxy image with CONNECT on 443 and an isolated internal network$`, w.egressDoctorPlain)
	sc.Step(`^both doctor outputs report the same independently calculated allowlist candidate with approval missing$`, w.egressDoctorCandidate)
	sc.Step(`^the egress prerequisite reports "([^"]*)" with exit (\d+) and the pinned proxy reference as subject$`, func(rule string, exit int) error {
		return w.egressEach(func(r egressResult) error { return egressProblem(r, rule, exit, "image", busyboxFixturePin) })
	})
	sc.Step(`^its fix names exactly "([^"]*)"$`, func(fix string) error {
		return w.egressEach(func(r egressResult) error { return egressExactFix(r, fix) })
	})
	sc.Step(`^each reports "([^"]*)" with exit (\d+) and guidance naming Docker 28\.0 and API 1\.48$`, func(rule string, exit int) error {
		return w.egressEach(func(r egressResult) error {
			if err := egressProblem(r, rule, exit, "field", "DOCKER_HOST"); err != nil {
				return err
			}
			return egressGuidance(r, rule, "Docker", "28.0", "API", "1.48")
		})
	})
	sc.Step(`^each reports "([^"]*)" with exit (\d+) and the pinned proxy reference as subject$`, func(rule string, exit int) error {
		return w.egressEach(func(r egressResult) error { return egressProblem(r, rule, exit, "image", busyboxFixturePin) })
	})
	sc.Step(`^the egress prerequisite reports "([^"]*)" with exit (\d+) and guidance to use the installed release outside the checkout$`, func(rule string, exit int) error {
		return w.egressEach(func(r egressResult) error {
			if err := egressProblem(r, rule, exit, "", ""); err != nil {
				return err
			}
			return egressGuidance(r, rule, "installed", "release", "outside")
		})
	})
	sc.Step(`^the copied executable checkout and original protected records are unchanged$`, w.egressCopyUnchanged)
	sc.Step(`^run reports the observed egress boundary mismatch as "([^"]*)" with exit (\d+)$`, w.egressBoundaryRefusal)
	sc.Step(`^no worker instruction is started and no fallback network or proxy restart is requested$`, w.egressNoFallback)
	sc.Step(`^every created owned resource is recorded before its confirmed cleanup$`, w.egressRecordedCleanup)
	sc.Step(`^the terminal review describes allowlist-v1 without a temporary run refusal before exactly one y/N prompt$`, w.egressRunnableReview)
	w.egressExecutableSteps(sc)
	w.egressStatusSteps(sc)
}

var egressBoundaryDefects = []string{
	"internal bridge ignores isolated mode", "internal bridge has an IPAM gateway", "internal bridge enables IPv6",
	"proxy has IPv4 forwarding enabled", "proxy executable mount is writable",
	"worker has an additional network", "worker has a published port",
}

// The four limitations approve, status and docs/security.md share, in order.
var egressLimitations = []string{
	"No TLS interception: the proxy verifies the CONNECT name and the first ClientHello's SNI.",
	"It cannot see the HTTP Host inside TLS or anything after that hello, a second hello after a HelloRetryRequest included, so fronting remains a limit, as does a hello the upstream reads differently, and it cannot stop data sent to an allowed service.",
	"Clients that omit SNI or send ECH, and SSH over 443, are refused.",
	"The proxy logs each refusal, but a stored log is no proof that every refusal was kept: a log that ends early reads as a complete one.",
}

func (w *world) egressScript(apply func(*localEgressAPI) error) error {
	if w.doctor == nil || w.doctor.engine == nil || w.doctor.engine.egress == nil {
		return fmt.Errorf("missing egress-capable fixture")
	}
	a := w.doctor.engine.egress
	a.mu.Lock()
	err := apply(a)
	a.mu.Unlock()
	if err != nil {
		return err
	}
	// Seeding an approval for a run also seeds the control catalog. Later
	// "unchanged" assertions compare against the state as the command found it.
	return w.approvalCapture()
}

func egressImageDefect(image map[string]any, defect string) error {
	switch defect {
	case "another attested manifest digest":
		image["RepoDigests"] = []any{"busybox@sha256:" + strings.Repeat("c", 64)}
	case "missing image config":
		delete(image, "Config")
	case "invalid image config ID":
		image["Id"] = "invalid"
	case "linux arm64":
		image["Architecture"] = "arm64"
	case "a declared image volume":
		object(image["Config"])["Volumes"] = map[string]any{"/data": map[string]any{}}
	default:
		return fmt.Errorf("unknown proxy image defect %q", defect)
	}
	return nil
}

// egressEdit changes only the policy file. The approval oracle keeps the settings
// the seeded record holds, which a changed request must be compared against.
func (w *world) egressEdit(request string) error {
	a, e := w.approval, w.egressFixture()
	e.approved = append([]string(nil), a.allow...)
	names := append([]string(nil), a.allow...)
	replace := func(old, next string) {
		for i, name := range names {
			if name == old {
				names[i] = next
			}
		}
	}
	switch {
	case request == "version 2 offline":
		e.requested = nil
		return w.egressPolicy("version: 2\nmanifest: .devcontainer/devcontainer.json\n")
	case strings.HasPrefix(request, "add "):
		names = append(names, strings.TrimPrefix(request, "add "))
	case strings.HasPrefix(request, "remove "):
		remove := strings.TrimPrefix(request, "remove ")
		names = nil
		for _, name := range a.allow {
			if name != remove {
				names = append(names, name)
			}
		}
	case strings.HasPrefix(request, "replace "):
		old, next, ok := strings.Cut(strings.TrimPrefix(request, "replace "), " with ")
		if !ok {
			return fmt.Errorf("bad edit %q", request)
		}
		replace(old, next)
	default:
		return fmt.Errorf("unknown egress edit %q", request)
	}
	sort.Strings(names)
	e.requested = names
	return w.egressPolicy(egressAllowlistPolicy(names, ""))
}

func egressAllowlistPolicy(names []string, comment string) string {
	data := "version: 2\nmanifest: .devcontainer/devcontainer.json\nnetwork:\n  mode: allowlist\n  allow:\n"
	for _, name := range names {
		data += "    - " + name + comment + "\n"
	}
	return data
}

func (w *world) egressPolicy(data string) error {
	if err := os.WriteFile(filepath.Join(w.doctor.checkout, "itos-container.yaml"), []byte(data), 0600); err != nil {
		return err
	}
	return w.approvalCapture()
}

// Each spelling names the approved set again: the record, its fingerprint and
// its generation must be honoured as they were written, with no new consent.
func (w *world) egressSpelling(spelling string) error {
	names := append([]string(nil), w.approval.allow...)
	switch spelling {
	case "unchanged from the approval written before run support":
		return w.approvalCapture()
	case "reversed list order and comments":
		for i, j := 0, len(names)-1; i < j; i, j = i+1, j-1 {
			names[i], names[j] = names[j], names[i]
		}
		return w.egressPolicy("# the same request, reordered\n" + egressAllowlistPolicy(names, " # unchanged"))
	case "API.Example.COM. and BÜCHER.example":
		if len(names) != 2 || names[0] != "api.example.com" || names[1] != "xn--bcher-kva.example" {
			return fmt.Errorf("the respelling names another approved list: %v", names)
		}
		return w.egressPolicy(egressAllowlistPolicy([]string{"API.Example.COM.", "BÜCHER.example"}, ""))
	}
	return fmt.Errorf("unknown spelling %q", spelling)
}

func (w *world) egressChangedMessage(text string) error {
	e := w.egressFixture()
	const prefix = "itos-container: run.approval-changed: "
	found := false
	for _, line := range strings.Split(w.stderr, "\n") {
		if message, ok := strings.CutPrefix(line, prefix); ok {
			if message != text {
				return fmt.Errorf("changed-approval message is %q, want exactly %q", message, text)
			}
			found = true
		}
	}
	if !found {
		return fmt.Errorf("no changed-approval message\n%s", w.report())
	}
	for _, name := range append(append([]string(nil), e.approved...), e.requested...) {
		if strings.Contains(w.stdout+w.stderr, name) {
			return fmt.Errorf("refusal echoes the listed value %q\n%s", name, w.report())
		}
	}
	return nil
}

// A daemon older than the isolated bridge, no cached proxy image and a
// controller inside the checkout: each would refuse an allowlist run, and none
// may be consulted for an offline one.
func (w *world) egressOfflineOnly() error {
	d := w.doctor
	if d.engine.offline == nil || d.engine.egress != nil {
		return fmt.Errorf("the offline control needs the plain local foreground fixture")
	}
	d.engine.mu.Lock()
	d.engine.version["Version"], d.engine.version["ApiVersion"], d.engine.version["MinAPIVersion"] = "27.5.1", "1.47", "1.24"
	d.engine.version["Components"] = []any{map[string]any{"Name": "Engine", "Version": "27.5.1"}}
	d.engine.info["ServerVersion"] = "27.5.1"
	d.engine.mu.Unlock()
	d.api = "1.47"
	return w.egressCopyController()
}

// egressCopyController copies, never links, the suite's built assembly into the
// scenario's checkout and runs every later command from that copy. Nothing else
// in the checkout is executed.
func (w *world) egressCopyController() error {
	source, err := w.sessionProbeBinary()
	if err != nil {
		return err
	}
	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	target := filepath.Join(w.doctor.checkout, "slice-27-controller")
	if err = os.WriteFile(target, data, 0700); err != nil {
		return err
	}
	w.runFixture().controller = target
	if err = w.seedControlLocations(); err != nil {
		return err
	}
	if w.egressFixture().copyBefore, err = egressTree(w.dir); err != nil {
		return err
	}
	return w.egressCaptureRecord()
}

// egressController is the executable a scenario's commands start: the suite's
// own, or the copy a scenario placed elsewhere, such as inside its checkout.
func (w *world) egressController(suite string) string {
	if w.run != nil && w.run.controller != "" {
		return w.run.controller
	}
	return suite
}

// egressCaptureRecord refreshes only the stored approval's bytes and identity.
// The whole-tree text snapshot is skipped: it would hold the copied binary.
func (w *world) egressCaptureRecord() error {
	a := w.approval
	a.recordBefore = nil
	a.recordInfo, _ = os.Lstat(a.record)
	if a.recordInfo != nil && a.recordInfo.Mode().IsRegular() {
		var err error
		if a.recordBefore, err = os.ReadFile(a.record); err != nil {
			return err
		}
	}
	return nil
}

func (w *world) egressDaemon(version, api string) error {
	e := w.doctor.engine
	if e.egress == nil {
		return fmt.Errorf("missing egress-capable fixture")
	}
	if err := w.approvalCapture(); err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.version["ApiVersion"] = api
	if version == "unknown" {
		delete(e.version, "Version")
		e.version["Components"] = []any{}
		e.info["ServerVersion"] = ""
	} else {
		e.version["Version"] = version
		e.version["Components"] = []any{map[string]any{"Name": "Engine", "Version": version}}
		e.info["ServerVersion"] = version
	}
	w.doctor.api = api
	return nil
}

// egressInvoke runs each command in turn through the scenario's product and
// keeps every result. A refusal or a missing behaviour is evidence for Then.
func (w *world) egressInvoke(commands ...[]string) error {
	e := w.egressFixture()
	w.runFixture()
	for _, command := range commands {
		if err := w.runPreflightProcess(w.env(), command...); err != nil {
			return err
		}
		e.results = append(e.results, egressResult{strings.Join(command, " "), w.run.results[len(w.run.results)-1]})
	}
	return nil
}

// egressEach applies check to every captured invocation, or to the single
// command a generic step ran.
func (w *world) egressEach(check func(egressResult) error) error {
	results := w.egressFixture().results
	if len(results) == 0 {
		if w.run == nil || len(w.run.results) == 0 {
			return fmt.Errorf("no captured invocation")
		}
		results = []egressResult{{"the scenario's command", w.run.results[len(w.run.results)-1]}}
	}
	for _, r := range results {
		if err := check(r); err != nil {
			return fmt.Errorf("%s: %w\nexit %d\n--- stdout\n%s--- stderr\n%s", r.command, err, r.result.exit, r.result.stdout, r.result.stderr)
		}
	}
	return nil
}
