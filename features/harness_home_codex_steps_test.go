package features

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"

	"github.com/cucumber/godog"
)

// The Codex harness object and its disclosure, written here from the
// specification and never read back from the product.
const (
	codexTarget  = "/home/itos/.codex"
	codexGateMsg = `itos-container: policy.home_harness "itos-container.yaml/home/harness": Home harness is not accepted by this command yet; omit home.harness to run, or review what it requests with doctor and approve`
)

var codexDisclosure = []string{
	"Codex harness: seed codex-v1 at /home/itos/.codex.",
	"Each run sets CODEX_HOME=/home/itos/.codex and rewrites only config.toml there from the fixed codex-v1 seed.",
	"The seed turns off Codex update checks, analytics and plugins, and Codex's own sandbox; this container is the sandbox.",
	"Codex sessions, history and other files under /home/itos/.codex are hostile worker data kept for the next run; run does not read, reset or remove them.",
	"CODEX_HOME is reserved: the manifest's containerEnv cannot set it.",
	"No Codex login is delivered by this setting. The seed is no network or credential grant.",
}

func codexObject() map[string]any {
	return map[string]any{"kind": "codex", "seed": "codex-v1", "target": codexTarget}
}

// codexHome is the approved home object, with or without the Codex harness.
func codexHome(harness bool) map[string]any {
	home := map[string]any{"storage": "volume", "target": "/home/itos", "lifetime": "project", "mode": "0700"}
	if harness {
		home["harness"] = codexObject()
	}
	return home
}

func codexPolicy(harness bool) string {
	if harness {
		return "version: 1\nmanifest: .devcontainer/devcontainer.json\nhome: {storage: volume, harness: codex}\n"
	}
	return "version: 1\nmanifest: .devcontainer/devcontainer.json\nhome: {storage: volume}\n"
}

// codexSentinels writes host-home Codex files that nothing may read or change.
func codexSentinels(home string) error {
	for _, path := range []string{".codex/config.toml", ".codex/auth.json", ".codex/sessions/sentinel.jsonl"} {
		full := filepath.Join(home, path)
		if err := os.MkdirAll(filepath.Dir(full), 0700); err != nil {
			return err
		}
		if err := os.WriteFile(full, []byte("synthetic host Codex sentinel"), 0600); err != nil {
			return err
		}
	}
	return nil
}

// codexManifestPair is one doctor --manifest-only run of ID-CODEXHOME-01.
type codexManifestPair struct {
	selection, spelling string
	entries             map[string]string
	code                int
	output              map[string]any
}

func (w *world) homeCodexSteps(sc *godog.ScenarioContext) {
	var manifest *manifestWorld
	var plain []doctorResult
	var pairs []codexManifestPair
	var oldFingerprint, newFingerprint string
	var newSettings map[string]any
	var plainDoctor, jsonDoctor, stale, terminal doctorResult
	var staleRecord []byte
	var checkout, host map[string]runEntry
	var cases []*openCodeMismatch
	var base string

	// ID-CODEXHOME-01
	sc.Step(`^a Codex harness policy fixture using version (\d+) and no credential grants$`, func(version int) error {
		w.home = &homeWorld{version: version}
		if err := w.isolatePolicy(); err != nil {
			return err
		}
		if err := w.manifestCheckout(); err != nil {
			return err
		}
		// File-only runs use the policy world's folder; the manifest world takes
		// over for the manifest-only runs.
		manifest, w.manifest = w.manifest, nil
		return nil
	})
	sc.Step(`^file-only doctor inspects the accepted Codex harness requests and the malformed Codex harness requests in JSON, and plain file-only doctor inspects the Codex request$`, func() error {
		if err := w.homePolicyRun([]homePolicyObservation{
			{input: "home: {storage: volume, harness: codex}\n"},
			{input: "home: {storage: volume, harness: codex, harnesses: [opencode]}\n"},
			{input: "home: {storage: volume, harness: null}\n", rule: "policy.type", subject: "/home/harness"},
			{input: "home: {storage: volume, harness: 7}\n", rule: "policy.type", subject: "/home/harness"},
			{input: "home: {storage: volume, harness: true}\n", rule: "policy.type", subject: "/home/harness"},
			{input: "home: {storage: volume, harness: [codex]}\n", rule: "policy.type", subject: "/home/harness"},
			{input: "home: {storage: volume, harness: {kind: codex}}\n", rule: "policy.type", subject: "/home/harness"},
			{input: "home: {storage: volume, harness: \"\"}\n", rule: "policy.home_harness", subject: "/home/harness"},
			{input: "home: {storage: volume, harness: Codex}\n", rule: "policy.home_harness", subject: "/home/harness"},
			{input: "home: {storage: volume, harness: CODEX}\n", rule: "policy.home_harness", subject: "/home/harness"},
			{input: "home: {storage: volume, harness: \" codex\"}\n", rule: "policy.home_harness", subject: "/home/harness"},
			{input: "home: {storage: volume, harness: opencode}\n", rule: "policy.home_harness", subject: "/home/harness"},
			{input: "home: {storage: volume, harness: codex-v1}\n", rule: "policy.home_harness", subject: "/home/harness"},
			{input: "home: {harness: codex}\n", rule: "policy.required", subject: "/home/storage"},
			{input: "home: {storage: tmpfs, harness: codex}\n", rule: "policy.home_storage", subject: "/home/storage"},
			{input: "home: {storage: volume, harness: codex, harness: codex}\n", rule: "policy.duplicate_key", subject: "/home/harness"},
		}); err != nil {
			return err
		}
		plain = nil
		for _, home := range []string{"home: {storage: volume, harness: codex}\n", "home: {storage: volume, harness: codex, harnesses: [opencode]}\n"} {
			data := fmt.Sprintf("version: %d\nmanifest: devcontainer.json\n%s", w.home.version, home)
			if err := os.WriteFile(filepath.Join(w.dir, "itos-container.yaml"), []byte(data), 0600); err != nil {
				return err
			}
			if err := w.runWith(w.env(), "doctor --policy-only"); err != nil {
				return err
			}
			plain = append(plain, doctorResult{stdout: w.stdout, stderr: w.stderr, exit: w.exit})
		}
		return nil
	})
	sc.Step(`^manifest-only doctor inspects each CODEX_HOME spelling and the near names under the Codex, plain volume, OpenCode and omitted home selections$`, func() error {
		w.manifest = manifest
		pairs = nil
		selections := []struct{ name, home string }{
			{"codex", "home: {storage: volume, harness: codex}\n"},
			{"volume", "home: {storage: volume}\n"},
			{"opencode", "home: {storage: volume, harnesses: [opencode]}\n"},
			{"omitted", ""},
		}
		manifests := []struct {
			spelling string
			entries  map[string]string
		}{
			{"CODEX_HOME", map[string]string{"CODEX_HOME": "/x"}},
			{"codex_home", map[string]string{"codex_home": "/x"}},
			{"Codex_Home", map[string]string{"Codex_Home": "/x"}},
			{"near names", map[string]string{"CODEX_HOME_DIR": "/x", "CODEX_SANDBOX": "seatbelt", "CODEXHOME": "/x", "MY_CODEX_HOME": "/x"}},
		}
		for _, selection := range selections {
			for _, m := range manifests {
				policy := fmt.Sprintf("version: %d\nmanifest: .devcontainer/devcontainer.json\n%s", w.home.version, selection.home)
				if err := os.WriteFile(filepath.Join(w.manifest.checkout, "itos-container.yaml"), []byte(policy), 0600); err != nil {
					return err
				}
				text, err := json.Marshal(map[string]any{"image": manifestImage, "containerEnv": m.entries})
				if err != nil {
					return err
				}
				if err := w.manifestText(string(text)); err != nil {
					return err
				}
				if err := w.runWith(w.env(), "doctor --manifest-only --json"); err != nil {
					return err
				}
				pair := codexManifestPair{selection: selection.name, spelling: m.spelling, entries: m.entries, code: w.exit}
				pair.output, err = w.json()
				if err != nil {
					return err
				}
				pairs = append(pairs, pair)
			}
		}
		return nil
	})
	sc.Step(`^each accepted Codex request succeeds with exactly its normalized home storage, harness and harnesses, and plain doctor prints home.harness codex after home.storage$`, func() error {
		for i, observation := range w.home.policy[:2] {
			home := map[string]any{"storage": "volume", "harness": "codex"}
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
		for i, result := range plain {
			lines := strings.Split(result.stdout, "\n")
			storage := -1
			for at, line := range lines {
				if line == "home.storage: volume" {
					storage = at
				}
			}
			if result.exit != 0 || storage < 0 || storage+1 >= len(lines) || lines[storage+1] != "home.harness: codex" {
				return fmt.Errorf("plain doctor does not print home.harness: codex right after home.storage: exit %d, %q", result.exit, result.stdout)
			}
			if i == 1 && (storage+2 >= len(lines) || lines[storage+2] != "home.harnesses: opencode") {
				return fmt.Errorf("plain doctor does not print home.harnesses after home.harness: %q", result.stdout)
			}
		}
		return nil
	})
	sc.Step(`^each malformed Codex harness request has exactly its specified rule, subject and fix without state or engine access$`, func() error {
		if err := w.homePolicyRefused(); err != nil {
			return err
		}
		for _, observation := range w.home.policy[2:] {
			problem := object(observation.output["problems"].([]any)[0])
			switch {
			case observation.rule == "policy.home_harness" && observation.subject == "/home/harness":
				if problem["message"] != "Home harness is unsupported" || problem["fix"] != "use home.harness codex, or omit harness" {
					return fmt.Errorf("home input %q: wrong message or fix: %s", observation.input, approvalJSON(problem))
				}
			case observation.rule == "policy.type":
				if problem["message"] != "Value has the wrong type" || problem["fix"] != "use the field type defined by policy version "+strconv.Itoa(w.home.version) {
					return fmt.Errorf("home input %q: wrong message or fix: %s", observation.input, approvalJSON(problem))
				}
			}
		}
		return nil
	})
	manifestOf := func(pair codexManifestPair) (map[string]any, error) {
		out := pair.output
		report := object(out["manifest"])
		if out["schema"] != float64(1) || out["scope"] != "manifest" || report == nil || report["path"] != ".devcontainer/devcontainer.json" {
			return nil, fmt.Errorf("%s under %s: not a manifest report: %s", pair.spelling, pair.selection, approvalJSON(out))
		}
		return report, nil
	}
	sc.Step(`^under the Codex selection each CODEX_HOME spelling is manifest.reserved at its own containerEnv path with exit 2 and no effective manifest$`, func() error {
		count := 0
		for _, pair := range pairs {
			if pair.selection != "codex" || pair.spelling == "near names" {
				continue
			}
			count++
			report, err := manifestOf(pair)
			if err != nil {
				return err
			}
			problems, _ := pair.output["problems"].([]any)
			if pair.code != 2 || pair.output["ok"] != false || report["valid"] != false || report["effective"] != nil || len(problems) != 1 {
				return fmt.Errorf("%s under the Codex selection: exit %d, %s", pair.spelling, pair.code, approvalJSON(pair.output))
			}
			problem := object(problems[0])
			if problem["rule"] != "manifest.reserved" || problem["path"] != "/containerEnv/"+pair.spelling || problem["file"] != ".devcontainer/devcontainer.json" || problem["exit"] != float64(2) ||
				problem["message"] != "Environment name is reserved" {
				return fmt.Errorf("%s under the Codex selection: wrong problem %s", pair.spelling, approvalJSON(problem))
			}
		}
		if count != 3 {
			return fmt.Errorf("the fixture ran %d reserved pairs, expected 3", count)
		}
		return nil
	})
	sc.Step(`^under every other selection each CODEX_HOME spelling stays in the effective containerEnv with its value, and the near names stay under every selection, the Codex one included$`, func() error {
		count := 0
		for _, pair := range pairs {
			if pair.selection == "codex" && pair.spelling != "near names" {
				continue
			}
			count++
			report, err := manifestOf(pair)
			if err != nil {
				return err
			}
			problems, _ := pair.output["problems"].([]any)
			effective := object(report["effective"])
			env := map[string]string{}
			for name, value := range object(effective["containerEnv"]) {
				env[name], _ = value.(string)
			}
			if pair.code != 0 || pair.output["ok"] != true || report["valid"] != true || len(problems) != 0 || effective == nil || !reflect.DeepEqual(env, pair.entries) {
				return fmt.Errorf("%s under %s: exit %d, %s", pair.spelling, pair.selection, pair.code, approvalJSON(pair.output))
			}
		}
		if count != 13 {
			return fmt.Errorf("the fixture ran %d ordinary pairs, expected 13", count)
		}
		return nil
	})

	// ID-CODEXHOME-02
	sc.Step(`^a Codex harness approval fixture whose checkout requests the Codex harness over an independently seeded approval of a volume home without it, with host-home Codex sentinels$`, func() error {
		if err := w.approvalCheckout(); err != nil {
			return err
		}
		a, d := w.approval, w.doctor
		if err := os.WriteFile(filepath.Join(d.checkout, "itos-container.yaml"), []byte(codexPolicy(false)), 0600); err != nil {
			return err
		}
		a.home = codexHome(false)
		if err := w.approvalSeed("1", false); err != nil {
			return err
		}
		oldFingerprint = a.fingerprint
		a.home = codexHome(true)
		if err := w.approvalOracle(); err != nil {
			return err
		}
		newFingerprint, newSettings = a.fingerprint, approvalClone(a.settings)
		if newFingerprint == oldFingerprint {
			return fmt.Errorf("the requested harness did not change the independently calculated fingerprint")
		}
		if err := os.WriteFile(filepath.Join(d.checkout, "itos-container.yaml"), []byte(codexPolicy(true)), 0600); err != nil {
			return err
		}
		if err := codexSentinels(d.env["HOME"]); err != nil {
			return err
		}
		var err error
		checkout, err = runTree(d.checkout)
		if err == nil {
			host, err = runTree(d.env["HOME"])
		}
		return err
	})
	sc.Step(`^Codex harness run checkouts with raw policies and independently encoded approval records for removing, adding and matching the harness and a no-harness control$`, func() error {
		mainDir, mainDoctor, mainApproval := w.dir, w.doctor, w.approval
		base, cases = w.dir, nil
		for _, c := range []*openCodeMismatch{
			{name: "removing", requested: false, approved: true},
			{name: "adding", requested: true, approved: false},
			{name: "matching", requested: true, approved: true},
			{name: "control", requested: false, approved: false},
		} {
			c.dir = filepath.Join(base, "slice-54-"+c.name)
			if err := os.MkdirAll(c.dir, 0700); err != nil {
				return err
			}
			w.dir = c.dir
			if err := w.approvalCheckout(); err != nil {
				return err
			}
			if err := os.WriteFile(filepath.Join(w.doctor.checkout, "itos-container.yaml"), []byte(codexPolicy(c.requested)), 0600); err != nil {
				return err
			}
			w.approval.home = codexHome(c.approved)
			if err := w.approvalSeed("1", false); err != nil {
				return err
			}
			if err := w.seedControlLocations(); err != nil {
				return err
			}
			c.doctor, c.approval = w.doctor, w.approval
			cases = append(cases, c)
		}
		w.dir, w.doctor, w.approval = mainDir, mainDoctor, mainApproval
		return nil
	})
	sc.Step(`^JSON doctor and plain doctor inspect the requested Codex harness, approve runs without a terminal as JSON with the seeded approval's fingerprint, and then bare approve runs at a terminal and receives "y" after the review$`, func() error {
		if err := w.runDoctor(w.env(), "doctor", "--json"); err != nil {
			return err
		}
		jsonDoctor = doctorResult{stdout: w.stdout, stderr: w.stderr, exit: w.exit}
		if err := w.runDoctor(w.env(), "doctor"); err != nil {
			return err
		}
		plainDoctor = doctorResult{stdout: w.stdout, stderr: w.stderr, exit: w.exit}
		if err := w.runDoctor(w.env(), "approve", "--json", "--fingerprint", oldFingerprint); err != nil {
			return err
		}
		stale = doctorResult{stdout: w.stdout, stderr: w.stderr, exit: w.exit}
		var err error
		if staleRecord, err = os.ReadFile(w.approval.record); err != nil {
			return err
		}
		if err = w.approvalTerminal("plain", "y", "", ""); err != nil {
			return err
		}
		// The run checkouts overwrite the last result; keep this one.
		terminal = doctorResult{stdout: w.stdout, stderr: w.stderr, exit: w.exit}
		return nil
	})
	sc.Step(`^the itos-container binary runs each Codex harness run checkout through its production approval preflight$`, func() error {
		mainDir, mainDoctor, mainApproval := w.dir, w.doctor, w.approval
		for _, c := range cases {
			w.dir, w.doctor, w.approval = c.dir, c.doctor, c.approval
			if err := w.runWith(w.env(), "run -- fixture-workload"); err != nil {
				return err
			}
			c.stdout, c.stderr, c.exit = w.stdout, w.stderr, w.exit
			c.journal = c.doctor.engine.requests()
			c.connections = c.doctor.engine.listener.connections.Load()
			w.closeDoctor()
		}
		w.dir, w.doctor, w.approval = mainDir, mainDoctor, mainApproval
		return nil
	})
	beforePrompt := func() (string, error) {
		// A terminal turns each line end into CR LF.
		review := strings.ReplaceAll(w.approval.beforeReview, "\r\n", "\n")
		if strings.Count(review, "y/N") != 1 {
			return "", fmt.Errorf("the review does not hold exactly one y/N prompt: %s", review)
		}
		return review[:strings.Index(review, "y/N")], nil
	}
	sc.Step(`^JSON doctor reports the independently expected home object with the Codex harness, the single /home/harness difference from null to it and the independently calculated new fingerprint$`, func() error {
		var report map[string]any
		if jsonDoctor.exit != 0 || json.Unmarshal([]byte(jsonDoctor.stdout), &report) != nil {
			return fmt.Errorf("doctor did not return JSON: exit %d, %s", jsonDoctor.exit, jsonDoctor.stdout)
		}
		approval := object(report["approval"])
		if !reflect.DeepEqual(object(approval["effective"])["home"], newSettings["home"]) {
			return fmt.Errorf("doctor's effective home is not the expected %s: %s", approvalJSON(newSettings["home"]), jsonDoctor.stdout)
		}
		differences, _ := approval["differences"].([]any)
		if len(differences) != 1 {
			return fmt.Errorf("doctor reports %d differences, expected only /home/harness: %s", len(differences), jsonDoctor.stdout)
		}
		change := object(differences[0])
		if value, present := change["before"]; change["path"] != "/home/harness" || !present || value != nil || !reflect.DeepEqual(change["after"], codexObject()) {
			return fmt.Errorf("the difference is not /home/harness from null to the Codex harness: %s", jsonDoctor.stdout)
		}
		if approval["fingerprint"] != newFingerprint || newFingerprint == oldFingerprint {
			return fmt.Errorf("doctor does not carry the new fingerprint %s: %s", newFingerprint, jsonDoctor.stdout)
		}
		return nil
	})
	sc.Step(`^the stale fingerprint is refused with approve.fingerprint and exit 1 without a prompt and the seeded approval record is unchanged$`, func() error {
		var report map[string]any
		if json.Unmarshal([]byte(stale.stdout), &report) != nil {
			return fmt.Errorf("the stale approval did not return JSON: exit %d, %s %s", stale.exit, stale.stdout, stale.stderr)
		}
		problems, _ := report["problems"].([]any)
		if stale.exit != 1 || report["ok"] != false || len(problems) != 1 || object(problems[0])["rule"] != "approve.fingerprint" || strings.Contains(stale.stdout+stale.stderr, "y/N") {
			return fmt.Errorf("the stale fingerprint is not refused with approve.fingerprint: exit %d, %s %s", stale.exit, stale.stdout, stale.stderr)
		}
		if !bytes.Equal(staleRecord, w.approval.recordBefore) {
			return fmt.Errorf("the stale fingerprint changed the seeded approval record: %s", staleRecord)
		}
		return nil
	})
	sc.Step(`^plain doctor and the review before exactly one y/N prompt both show the six Codex harness disclosure lines in order after the home disclosure, and only the review shows the /home/harness difference line$`, func() error {
		before, err := beforePrompt()
		if err != nil {
			return err
		}
		for name, text := range map[string]string{"plain doctor": plainDoctor.stdout, "review": before} {
			lines := strings.Split(text, "\n")
			home, first := -1, -1
			for at, line := range lines {
				if strings.HasPrefix(line, "Harness home: private volume at /home/itos") {
					home = at
				}
				if line == codexDisclosure[0] && first < 0 {
					first = at
				}
			}
			// The six lines are one run of whole lines. Wherever the existing
			// three-line home disclosure is printed they follow it at once; the
			// review always prints it.
			if first < 0 || first+len(codexDisclosure) > len(lines) || !reflect.DeepEqual(lines[first:first+len(codexDisclosure)], codexDisclosure) {
				return fmt.Errorf("%s does not show the six Codex disclosure lines in order: %s", name, text)
			}
			if (home >= 0 || name == "review") && first != home+3 {
				return fmt.Errorf("%s does not show the six Codex disclosure lines right after the home disclosure: %s", name, text)
			}
			if opencode := strings.Index(text, "OpenCode harness:"); opencode >= 0 && opencode < strings.Index(text, codexDisclosure[0]) {
				return fmt.Errorf("%s shows an OpenCode line before the Codex lines: %s", name, text)
			}
		}
		difference := `/home/harness: null -> {"kind":"codex","seed":"codex-v1","target":"/home/itos/.codex"}`
		if strings.Contains(plainDoctor.stdout, "/home/harness:") {
			return fmt.Errorf("plain doctor prints a difference line: %s", plainDoctor.stdout)
		}
		if !strings.Contains(before, "\n"+difference+"\n") {
			return fmt.Errorf("the review omits the difference %q: %s", difference, before)
		}
		return nil
	})
	sc.Step(`^terminal approval exits 0 and records the independently expected settings and new fingerprint at generation 2$`, func() error {
		if terminal.exit != 0 {
			return fmt.Errorf("approval exited %d: %s %s", terminal.exit, terminal.stdout, terminal.stderr)
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
	sc.Step(`^no home volume or worker is created and checkout and host-home Codex sentinels are unchanged$`, func() error {
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
			return fmt.Errorf("review changed the host-home Codex sentinels")
		}
		return nil
	})
	sc.Step(`^the removing checkout reports changed approval at /home/harness with exit 1, the adding and matching checkouts report the Codex harness run gate with exit 2, and none of the three Engine API fixtures saw a connection$`, func() error {
		for _, c := range cases[:3] {
			want, code := "itos-container: run.approval-changed: run approval changed at /home/harness; use itos container approve", 1
			if c.name != "removing" {
				want, code = codexGateMsg, 2
			}
			if c.exit != code || c.stdout != "" || !strings.Contains(c.stderr, want+"\n") {
				return fmt.Errorf("%s: expected exit %d and %q: exit %d, stdout %q, stderr %q", c.name, code, want, c.exit, c.stdout, c.stderr)
			}
			if c.name != "removing" && c.stderr != want+"\n" {
				return fmt.Errorf("%s: stderr holds more than the gate: %q", c.name, c.stderr)
			}
			if c.connections != 0 || len(c.journal) != 0 {
				return fmt.Errorf("%s: the engine saw %d connections and %+v", c.name, c.connections, c.journal)
			}
		}
		return nil
	})
	sc.Step(`^the no-harness control reports no approval change and its Engine API fixture journal holds the readiness inspection of version, info and the approved image$`, func() error {
		c := cases[3]
		if strings.Contains(c.stderr, "run.approval-changed") || strings.Contains(c.stderr, "approval changed") || strings.Contains(c.stderr, "policy.home_harness") {
			return fmt.Errorf("the control is refused: %s", c.stderr)
		}
		seen := map[string]bool{}
		for _, request := range c.journal {
			path := doctorRequestPath(request.Path)
			if request.Method == "GET" && (path == "/version" || path == "/info" || path == "/images/"+c.doctor.image+"/json") {
				seen[path] = true
			}
		}
		if len(seen) != 3 {
			return fmt.Errorf("the readiness inspection is incomplete (%v): %+v; stderr %s", seen, c.journal, c.stderr)
		}
		return nil
	})
}
