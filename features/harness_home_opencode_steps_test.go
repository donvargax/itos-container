package features

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/cucumber/godog"
)

// The opencode-v1 profile, written here from the specification and never read
// back from the product. The three values are the worker environment it binds.
var openCodeEnvironment = [][2]string{
	{"OPENCODE_CONFIG_CONTENT", `{"share":"disabled"}`},
	{"OPENCODE_DISABLE_AUTOUPDATE", "true"},
	{"OPENCODE_DISABLE_MODELS_FETCH", "true"},
}

const openCodeSentinel = "synthetic host OpenCode sentinel"

// openCodeHome is the approved home object, with or without the harness.
func openCodeHome(harness bool) map[string]any {
	home := map[string]any{"storage": "volume", "target": "/home/itos", "lifetime": "project", "mode": "0700"}
	if harness {
		home["harnesses"] = []any{map[string]any{"name": "opencode", "profile": "opencode-v1"}}
	}
	return home
}

func openCodePolicy(harness bool) string {
	if harness {
		return "version: 1\nmanifest: .devcontainer/devcontainer.json\nhome: {storage: volume, harnesses: [opencode]}\n"
	}
	return "version: 1\nmanifest: .devcontainer/devcontainer.json\nhome: {storage: volume}\n"
}

// openCodeSentinels writes host-home OpenCode configuration and login files
// that no worker may ever see.
func openCodeSentinels(home string) error {
	for _, path := range []string{".config/opencode/opencode.json", ".local/share/opencode/auth.json", ".local/state/opencode/state.json"} {
		full := filepath.Join(home, path)
		if err := os.MkdirAll(filepath.Dir(full), 0700); err != nil {
			return err
		}
		if err := os.WriteFile(full, []byte(openCodeSentinel), 0600); err != nil {
			return err
		}
	}
	return nil
}

// openCodeMismatch is one checkout of ID-HOME-27: its raw request, its
// independently encoded approval and what the product's run did with them.
type openCodeMismatch struct {
	name           string
	requested      bool // the raw policy asks for the harness
	approved       bool // the independently encoded record carries it
	dir            string
	doctor         *doctorWorld
	approval       *approvalWorld
	stdout, stderr string
	exit           int
	connections    int64
	journal        []doctorRequest
}

func (w *world) homeOpenCodeSteps(sc *godog.ScenarioContext) {
	var cases []*openCodeMismatch
	var base string
	var oldFingerprint, newFingerprint string
	var newSettings map[string]any
	var plainDoctor, jsonDoctor doctorResult
	var checkout, host map[string]runEntry

	// ID-HOME-25
	sc.Step(`^an OpenCode harness policy fixture using version (\d+) and no credential grants$`, func(version int) error {
		w.home = &homeWorld{version: version}
		return w.isolatePolicy()
	})
	sc.Step(`^file-only doctor inspects a volume home without harnesses, a volume home with the OpenCode harness and the malformed harness requests$`, func() error {
		return w.homePolicyRun([]homePolicyObservation{
			{input: "home: {storage: volume}\n"},
			{input: "home: {storage: volume, harnesses: [opencode]}\n"},
			{input: "home: {storage: volume, harnesses: null}\n", rule: "policy.type", subject: "/home/harnesses"},
			{input: "home: {storage: volume, harnesses: opencode}\n", rule: "policy.type", subject: "/home/harnesses"},
			{input: "home: {storage: volume, harnesses: {}}\n", rule: "policy.type", subject: "/home/harnesses"},
			{input: "home: {storage: volume, harnesses: []}\n", rule: "policy.home_harness", subject: "/home/harnesses"},
			{input: "home: {storage: volume, harnesses: [codex]}\n", rule: "policy.home_harness", subject: "/home/harnesses/0"},
			{input: "home: {storage: volume, harnesses: [OpenCode]}\n", rule: "policy.home_harness", subject: "/home/harnesses/0"},
			{input: "home: {storage: volume, harnesses: [opencode, opencode]}\n", rule: "policy.home_harness", subject: "/home/harnesses/1"},
			{input: "home: {storage: volume, harnesses: [7]}\n", rule: "policy.type", subject: "/home/harnesses/0"},
			{input: "home: {harnesses: [opencode]}\n", rule: "policy.required", subject: "/home/storage"},
		})
	})
	sc.Step(`^the OpenCode request succeeds with exactly the normalized home storage and harness and the request without harnesses keeps its slice-34 output$`, func() error {
		for i, observation := range w.home.policy[:2] {
			home := map[string]any{"storage": "volume"}
			if i == 1 {
				home["harnesses"] = []any{"opencode"}
			}
			expected := map[string]any{"version": float64(w.home.version), "manifest": "devcontainer.json", "network": map[string]any{"mode": "offline"}, "home": home}
			out := observation.output
			problems, ok := out["problems"].([]any)
			if observation.code != 0 || out["schema"] != float64(1) || out["ok"] != true || out["scope"] != "policy" || !ok || len(problems) != 0 || !reflect.DeepEqual(out["policy"], expected) {
				return fmt.Errorf("home input %q: exit %d, output %s; expected normalized policy %s", observation.input, observation.code, approvalJSON(out), approvalJSON(expected))
			}
		}
		return nil
	})
	sc.Step(`^malformed harness requests have the specified rules and subjects without state or engine access$`, w.homePolicyRefused)

	// ID-HOME-26
	sc.Step(`^an OpenCode approval fixture with an independently seeded approval of a volume home without harnesses and a requested OpenCode harness$`, func() error {
		if err := w.approvalCheckout(); err != nil {
			return err
		}
		a, d := w.approval, w.doctor
		if err := os.WriteFile(filepath.Join(d.checkout, "itos-container.yaml"), []byte(openCodePolicy(false)), 0600); err != nil {
			return err
		}
		a.home = openCodeHome(false)
		if err := w.approvalSeed("1", false); err != nil {
			return err
		}
		oldFingerprint = a.fingerprint
		a.home = openCodeHome(true)
		if err := w.approvalOracle(); err != nil {
			return err
		}
		newFingerprint, newSettings = a.fingerprint, approvalClone(a.settings)
		if newFingerprint == oldFingerprint {
			return fmt.Errorf("the requested harness did not change the independently calculated fingerprint")
		}
		if err := os.WriteFile(filepath.Join(d.checkout, "itos-container.yaml"), []byte(openCodePolicy(true)), 0600); err != nil {
			return err
		}
		if err := openCodeSentinels(d.env["HOME"]); err != nil {
			return err
		}
		var err error
		checkout, err = runTree(d.checkout)
		if err == nil {
			host, err = runTree(d.env["HOME"])
		}
		return err
	})
	sc.Step(`^JSON doctor and plain doctor inspect the requested harness and bare approve runs at a terminal and receives "y" after the review$`, func() error {
		if err := w.runDoctor(w.env(), "doctor"); err != nil {
			return err
		}
		plainDoctor = doctorResult{stdout: w.stdout, stderr: w.stderr, exit: w.exit}
		if err := w.runDoctor(w.env(), "doctor", "--json"); err != nil {
			return err
		}
		jsonDoctor = doctorResult{stdout: w.stdout, stderr: w.stderr, exit: w.exit}
		return w.approvalTerminal("plain", "y", "", "")
	})
	// beforePrompt is the review's text before its one y/N prompt.
	beforePrompt := func() (string, error) {
		// A terminal turns each line end into CR LF.
		review := strings.ReplaceAll(w.approval.beforeReview, "\r\n", "\n")
		if strings.Count(review, "y/N") != 1 {
			return "", fmt.Errorf("the review does not hold exactly one y/N prompt: %s", review)
		}
		return review[:strings.Index(review, "y/N")], nil
	}
	sc.Step(`^JSON doctor reports the independently expected home object and the /home/harnesses difference from null to the OpenCode harness$`, func() error {
		var report map[string]any
		if jsonDoctor.exit != 0 || json.Unmarshal([]byte(jsonDoctor.stdout), &report) != nil {
			return fmt.Errorf("doctor did not return JSON: exit %d, %s", jsonDoctor.exit, jsonDoctor.stdout)
		}
		approval := object(report["approval"])
		effective := object(approval["effective"])
		if !reflect.DeepEqual(effective["home"], newSettings["home"]) {
			return fmt.Errorf("doctor's effective home is not the expected %s: %s", approvalJSON(newSettings["home"]), jsonDoctor.stdout)
		}
		harnesses := newSettings["home"].(map[string]any)["harnesses"]
		differences, _ := approval["differences"].([]any)
		count := 0
		for _, entry := range differences {
			change := object(entry)
			if change["path"] == "/home/harnesses" {
				count++
				if value, present := change["before"]; !present || value != nil || !reflect.DeepEqual(change["after"], harnesses) {
					return fmt.Errorf("the /home/harnesses difference is not null to the OpenCode harness: %s", jsonDoctor.stdout)
				}
			}
		}
		if count != 1 {
			return fmt.Errorf("doctor reports %d /home/harnesses differences: %s", count, jsonDoctor.stdout)
		}
		return nil
	})
	sc.Step(`^plain doctor and the transcript before exactly one y/N prompt both show the OpenCode harness profile and its exact environment lines, and the transcript shows the /home/harnesses difference with old and new values$`, func() error {
		before, err := beforePrompt()
		if err != nil {
			return err
		}
		wants := []string{"OpenCode harness: profile opencode-v1."}
		for _, pair := range openCodeEnvironment {
			wants = append(wants, pair[0]+"="+pair[1])
		}
		for name, text := range map[string]string{"plain doctor": plainDoctor.stdout, "review": before} {
			last := -1
			for _, want := range wants {
				at := strings.Index(text, want+"\n")
				if at < 0 {
					return fmt.Errorf("%s omits the line %q: %s", name, want, text)
				}
				if at < last {
					return fmt.Errorf("%s lists %q out of order: %s", name, want, text)
				}
				last = at
			}
		}
		if strings.Contains(plainDoctor.stdout, "/home/harnesses:") {
			return fmt.Errorf("plain doctor prints a difference line: %s", plainDoctor.stdout)
		}
		difference := `/home/harnesses: null -> [{"name":"opencode","profile":"opencode-v1"}]`
		if !strings.Contains(before, difference) {
			return fmt.Errorf("the review omits the difference %q: %s", difference, before)
		}
		return nil
	})
	sc.Step(`^plain doctor and the review before its prompt state where OpenCode's global configuration and state live, that the checkout's own OpenCode configuration applies, that managed configuration in the approved image overrides the profile, that the image's other OPENCODE defaults can redirect or turn off configuration layers, who chooses provider and model, that no provider is reachable offline and that no login is delivered$`, func() error {
		before, err := beforePrompt()
		if err != nil {
			return err
		}
		// The sentences' wording is the implementer's; the facts are checked by
		// their fixed words.
		facts := []string{"/home/itos", "opencode.json", "/etc/opencode", "OPENCODE_CONFIG_DIR", "OPENCODE_DISABLE_PROJECT_CONFIG", "provider", "model", "no provider is reachable", "login"}
		for name, text := range map[string]string{"plain doctor": plainDoctor.stdout, "review": before} {
			lower := strings.ToLower(text)
			for _, fact := range facts {
				if !strings.Contains(lower, strings.ToLower(fact)) {
					return fmt.Errorf("%s omits the fact worded by %q: %s", name, fact, text)
				}
			}
		}
		return nil
	})
	sc.Step(`^approval exits 0 and records the independently expected home object at the next generation$`, func() error {
		if w.exit != 0 {
			return fmt.Errorf("approval exited %d: %s %s", w.exit, w.stdout, w.stderr)
		}
		data, err := os.ReadFile(w.approval.record)
		if err != nil {
			return err
		}
		want := approvalJSON(map[string]any{"schema": 1, "project": w.approval.project, "generation": "2", "fingerprint": newFingerprint, "settings": newSettings})
		if string(data) != string(want) {
			return fmt.Errorf("the approval record differs: %s", data)
		}
		return nil
	})
	sc.Step(`^doctor and the persisted approval both carry the independently calculated new fingerprint, which differs from the seeded approval's fingerprint$`, func() error {
		var report map[string]any
		if json.Unmarshal([]byte(jsonDoctor.stdout), &report) != nil || object(report["approval"])["fingerprint"] != newFingerprint || newFingerprint == oldFingerprint {
			return fmt.Errorf("doctor does not carry the new fingerprint %s: %s", newFingerprint, jsonDoctor.stdout)
		}
		data, err := os.ReadFile(w.approval.record)
		if err != nil {
			return err
		}
		var record map[string]any
		if json.Unmarshal(data, &record) != nil || record["fingerprint"] != newFingerprint {
			return fmt.Errorf("the persisted approval does not carry the new fingerprint: %s", data)
		}
		return nil
	})
	sc.Step(`^no home volume or worker is created and checkout and host-home OpenCode sentinels are unchanged$`, func() error {
		for _, req := range w.doctor.engine.requests() {
			if req.Method != "GET" && req.Method != "HEAD" {
				return fmt.Errorf("review mutated the engine: %+v", req)
			}
		}
		after, err := runTree(w.doctor.checkout)
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(after, checkout) {
			return fmt.Errorf("review changed the checkout")
		}
		after, err = runTree(w.doctor.env["HOME"])
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(after, host) {
			return fmt.Errorf("review changed the host-home OpenCode sentinels")
		}
		return nil
	})

	// ID-HOME-27
	sc.Step(`^OpenCode harness approval mismatch checkouts with raw policies and independently encoded approval records for adding and removing the harness and matching controls$`, func() error {
		base, cases = w.dir, nil
		for _, c := range []*openCodeMismatch{
			{name: "adding", requested: true, approved: false},
			{name: "removing", requested: false, approved: true},
			{name: "matching-with", requested: true, approved: true},
			{name: "matching-without", requested: false, approved: false},
		} {
			c.dir = filepath.Join(base, "slice-41-"+c.name)
			if err := os.MkdirAll(c.dir, 0700); err != nil {
				return err
			}
			w.dir = c.dir
			if err := w.approvalCheckout(); err != nil {
				return err
			}
			if err := os.WriteFile(filepath.Join(w.doctor.checkout, "itos-container.yaml"), []byte(openCodePolicy(c.requested)), 0600); err != nil {
				return err
			}
			w.approval.home = openCodeHome(c.approved)
			if err := w.approvalSeed("1", false); err != nil {
				return err
			}
			if err := w.seedControlLocations(); err != nil {
				return err
			}
			c.doctor, c.approval = w.doctor, w.approval
			cases = append(cases, c)
		}
		w.dir = base
		return nil
	})
	sc.Step(`^the itos-container binary runs each changed request and then each matching control through its production approval preflight$`, func() error {
		for i, c := range cases {
			w.dir, w.doctor, w.approval = c.dir, c.doctor, c.approval
			if err := w.runWith(w.env(), "run -- fixture-workload"); err != nil {
				return err
			}
			c.stdout, c.stderr, c.exit = w.stdout, w.stderr, w.exit
			c.journal = c.doctor.engine.requests()
			c.connections = c.doctor.engine.listener.connections.Load()
			if i < len(cases)-1 {
				w.closeDoctor()
			}
		}
		w.dir = base
		return nil
	})
	sc.Step(`^each mismatch reports changed approval at /home/harnesses with exit 1 and the Engine API fixture saw no connection and journalled no request$`, func() error {
		for _, c := range cases[:2] {
			want := "itos-container: run.approval-changed: run approval changed at /home/harnesses; use itos container approve"
			if c.exit != 1 || c.stdout != "" || !strings.Contains(c.stderr, want+"\n") {
				return fmt.Errorf("%s: expected exit 1 and %q: exit %d, stdout %q, stderr %q", c.name, want, c.exit, c.stdout, c.stderr)
			}
			if c.connections != 0 || len(c.journal) != 0 {
				return fmt.Errorf("%s: the engine saw %d connections and %+v", c.name, c.connections, c.journal)
			}
		}
		return nil
	})
	sc.Step(`^each matching control reports no approval change and its Engine API fixture journal holds the readiness inspection of version, info and the approved image$`, func() error {
		for _, c := range cases[2:] {
			if strings.Contains(c.stderr, "run.approval-changed") || strings.Contains(c.stderr, "approval changed") {
				return fmt.Errorf("%s: the matching control reports changed approval: %s", c.name, c.stderr)
			}
			seen := map[string]bool{}
			for _, request := range c.journal {
				path := doctorRequestPath(request.Path)
				if request.Method == "GET" && (path == "/version" || path == "/info" || path == "/images/"+c.doctor.image+"/json") {
					seen[path] = true
				}
			}
			if len(seen) != 3 {
				return fmt.Errorf("%s: the readiness inspection is incomplete (%v): %+v; stderr %s", c.name, seen, c.journal, c.stderr)
			}
		}
		return nil
	})
}
