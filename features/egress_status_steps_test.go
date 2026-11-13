package features

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"

	"github.com/cucumber/godog"
)

// egressSeeded is one original session written by the fixture alone: its
// snapshot, protected intent, public record and, where it ran with a grant, the
// names it was given. No product approval or launch produced any of it.
type egressSeeded struct {
	id, token, generation, fingerprint string
	worker                             string
	allow                              []string
}

type egressStatus struct {
	phase           string
	egress, offline egressSeeded
	before          map[string]string
}

func (w *world) egressStatusSteps(sc *godog.ScenarioContext) {
	sc.Step(`^original protected egress and offline sessions with phase "([^"]*)"$`, w.egressSeedSessions)
	sc.Step(`^the current policy and approval no longer describe either original session$`, w.egressCurrentDiffers)
	sc.Step(`^status inspects those sessions in plain and JSON modes$`, func() error {
		return w.egressInvoke([]string{"status"}, []string{"status", "--json"})
	})
	sc.Step(`^the egress entry names only its original profile and allowlist with all four specified limitations in order$`, w.egressStatusPlain)
	sc.Step(`^JSON carries the same facts in that entry's egress object while the offline entry omits it$`, w.egressStatusJSON)
	sc.Step(`^status neither claims TLS interception nor that all refusals were stored$`, w.egressStatusNoClaim)
	sc.Step(`^status writes no state requests no consent and changes no engine resource$`, w.egressStatusReadOnly)
}

func (w *world) egressStatePath(parts ...string) string {
	return filepath.Join(append([]string{w.doctor.env["XDG_STATE_HOME"], "itos-container"}, parts...)...)
}

func (w *world) egressSeedSessions(phase string) error {
	if phase != "running with controller absent" && phase != "stopped with cleanup complete" {
		return fmt.Errorf("unknown session phase %q", phase)
	}
	if err := w.approvalCheckout(); err != nil {
		return err
	}
	if err := w.localEgressFixture(); err != nil {
		return err
	}
	s := &egressStatus{phase: phase}
	w.egressFixture().status = s
	s.egress = egressSeeded{id: strings.Repeat("a", 64), token: strings.Repeat("1", 63) + "a", generation: "1", worker: strings.Repeat("5", 64), allow: []string{"api.example.com", "registry.example.org"}}
	s.offline = egressSeeded{id: strings.Repeat("b", 64), token: strings.Repeat("2", 63) + "b", generation: "2", worker: strings.Repeat("6", 64)}
	if err := w.approvalNetworkRequest(2, s.egress.allow); err != nil {
		return err
	}
	if err := w.egressSeedSession(&s.egress); err != nil {
		return err
	}
	if err := w.approvalNetworkRequest(1, nil); err != nil {
		return err
	}
	return w.egressSeedSession(&s.offline)
}

// egressSeedSession writes one session from the approval oracle's current
// settings, which become that session's original snapshot.
func (w *world) egressSeedSession(s *egressSeeded) error {
	a, d := w.approval, w.doctor
	running := w.egress.status.phase == "running with controller absent"
	s.fingerprint = a.fingerprint
	snapshot := approvalJSON(map[string]any{"schema": 1, "project": a.project, "generation": s.generation, "fingerprint": a.fingerprint, "settings": a.settings})
	if err := writeManifestFixture(w.egressStatePath("snapshots", a.project, s.generation+".json"), snapshot, 0600); err != nil {
		return err
	}
	stage := "absent"
	if running {
		stage = "owned"
	}
	checkout := object(approvalClone(a.settings)["checkout"])
	anchor := map[string]any{"session": s.id, "name": "itos-container-anchor-" + s.token, "source": "/proc/2147483647/fd/17", "device": checkout["device"], "inode": checkout["inode"], "created": "2026-11-09T12:00:00Z", "stage": stage}
	record := map[string]any{"schema": 2, "project": a.project, "generation": s.generation, "fingerprint": a.fingerprint,
		"snapshot": "snapshots/" + a.project + "/" + s.generation + ".json", "uid": a.uid, "gid": a.gid,
		"engine_endpoint": d.env["DOCKER_HOST"], "engine_id": d.engineID, "name": "itos-container-" + s.token,
		"image_reference": d.image, "image_id": d.imageID, "container_id": s.worker, "stage": stage, "anchor": anchor}
	api := d.engine.egress
	labels := func(kind string) map[string]any {
		l := map[string]any{"io.itos-container.intent": s.token, "io.itos-container.project": a.project, "io.itos-container.fingerprint": a.fingerprint}
		if kind != "" {
			l["io.itos-container.session"], l["io.itos-container.kind"] = s.id, kind
		}
		return l
	}
	internal, network, proxy := strings.Repeat("7", 64), strings.Repeat("8", 64), strings.Repeat("9", 64)
	if s.allow != nil {
		record["schema"] = 3
		record["egress"] = map[string]any{
			"internal": map[string]any{"name": "itos-container-internal-" + s.token, "id": internal, "stage": stage},
			"network":  map[string]any{"name": "itos-container-egress-" + s.token, "id": network, "stage": stage},
			"proxy":    map[string]any{"name": "itos-container-proxy-" + s.token, "id": proxy, "stage": stage},
		}
		// The original runtime evidence: the proxy image's config ID as this
		// session inspected it, beside the session's protected intent. The
		// implementing change records this durable mapping in its decision.
		evidence := approvalJSON(map[string]any{"schema": 1, "proxy_image_id": egressProxyConfigID})
		if err := writeManifestFixture(w.egressStatePath("intents", a.project, s.token+".egress.json"), evidence, 0600); err != nil {
			return err
		}
	}
	if err := writeManifestFixture(w.egressStatePath("intents", a.project, s.token+".json"), approvalJSON(record), 0600); err != nil {
		return err
	}
	public := map[string]any{"schema": 1, "id": s.id, "project": a.project, "intent": "intents/" + a.project + "/" + s.token + ".json", "state": "running", "workload_exit": nil, "last_problem": nil}
	if !running {
		public["state"], public["workload_exit"] = "stopped", 0
	}
	directory := w.egressStatePath("sessions", w.recoveryIndexKey(), s.id)
	if err := writeManifestFixture(filepath.Join(directory, "record.json"), approvalJSON(public), 0600); err != nil {
		return err
	}
	for _, name := range []string{"controller.lock", "operation.lock"} {
		if err := os.WriteFile(filepath.Join(directory, name), nil, 0600); err != nil {
			return err
		}
	}
	if !running {
		claim := map[string]any{"schema": 1, "root": d.checkout, "id": s.id, "intent": public["intent"], "complete": true}
		return writeManifestFixture(w.admissionPath(s.id), approvalJSON(claim), 0600)
	}
	// The engine still holds what the absent controller owned.
	api.mu.Lock()
	defer api.mu.Unlock()
	api.volumes[fmt.Sprint(anchor["name"])] = map[string]any{"Name": anchor["name"], "Driver": "local", "Scope": "local", "CreatedAt": anchor["created"],
		"Labels": func() map[string]any {
			l := labels("checkout-anchor")
			l["io.itos-container.device"], l["io.itos-container.inode"] = anchor["device"], anchor["inode"]
			return l
		}(), "Options": map[string]any{"type": "none", "o": "bind", "device": anchor["source"]}}
	networks := map[string]any{"none": map[string]any{}}
	mode := "none"
	if s.allow != nil {
		mode = "itos-container-internal-" + s.token
		networks = map[string]any{mode: map[string]any{"NetworkID": internal, "IPAddress": "172.28.0.3"}}
		api.networks[internal] = map[string]any{"Id": internal, "Name": mode, "Driver": "bridge", "Scope": "local", "Internal": true, "EnableIPv6": false,
			"Options": map[string]any{"com.docker.network.bridge.gateway_mode_ipv4": "isolated"}, "Labels": labels("internal-network"),
			"IPAM":       map[string]any{"Driver": "default", "Config": []any{map[string]any{"Subnet": "172.28.0.0/16"}}},
			"Containers": map[string]any{proxy: map[string]any{"Name": "itos-container-proxy-" + s.token}, s.worker: map[string]any{"Name": "itos-container-" + s.token}}}
		api.networks[network] = map[string]any{"Id": network, "Name": "itos-container-egress-" + s.token, "Driver": "bridge", "Scope": "local", "Internal": false, "EnableIPv6": false,
			"Options": map[string]any{}, "Labels": labels("egress-network"),
			"IPAM":       map[string]any{"Driver": "default", "Config": []any{map[string]any{"Subnet": "172.29.0.0/16", "Gateway": "172.29.0.1"}}},
			"Containers": map[string]any{proxy: map[string]any{"Name": "itos-container-proxy-" + s.token}}}
		api.containers[proxy] = map[string]any{"Id": proxy, "Name": "/itos-container-proxy-" + s.token, "Image": egressProxyConfigID,
			"Config":     map[string]any{"Image": egressProxyConfigID, "Labels": labels("egress-proxy"), "Cmd": append([]any{"/itos-container", "__egress-proxy", "--"}, offlineStrings(s.allow)...)},
			"HostConfig": map[string]any{"NetworkMode": "itos-container-egress-" + s.token},
			"State":      map[string]any{"Status": "running", "Running": true, "Pid": 122, "StartedAt": "2026-11-09T12:00:00Z"},
			"NetworkSettings": map[string]any{"Networks": map[string]any{
				mode:                               map[string]any{"NetworkID": internal, "IPAddress": egressInternalAddress},
				"itos-container-egress-" + s.token: map[string]any{"NetworkID": network, "IPAddress": "172.29.0.2"}}}}
	}
	api.containers[s.worker] = map[string]any{"Id": s.worker, "Name": "/itos-container-" + s.token, "Image": d.imageID,
		"Config":          map[string]any{"Image": d.imageID, "Labels": labels("")},
		"HostConfig":      map[string]any{"NetworkMode": mode, "Mounts": []any{map[string]any{"Type": "volume", "Source": anchor["name"], "Target": "/workspace", "VolumeOptions": map[string]any{"NoCopy": true}}}},
		"Mounts":          []any{map[string]any{"Type": "volume", "Name": anchor["name"], "Driver": "local", "Destination": "/workspace", "RW": true}},
		"State":           map[string]any{"Status": "running", "Running": true, "Pid": 123, "StartedAt": "2026-11-09T12:00:00Z"},
		"NetworkSettings": map[string]any{"Networks": networks}}
	return nil
}

// Neither original session is what the checkout asks for or the person last
// approved. Status must read each session's own snapshot, never these.
func (w *world) egressCurrentDiffers() error {
	if err := w.approvalNetworkRequest(2, []string{"current.example"}); err != nil {
		return err
	}
	if err := w.approvalSeed("3", false); err != nil {
		return err
	}
	if err := w.seedControlLocations(); err != nil {
		return err
	}
	var err error
	w.egress.status.before, err = doctorTree(w.dir)
	return err
}

func (w *world) egressStatusResults() (plain, structured egressResult, err error) {
	results := w.egressFixture().results
	if len(results) != 2 || results[0].command != "status" || results[1].command != "status --json" {
		return plain, structured, fmt.Errorf("missing plain and JSON status results")
	}
	return results[0], results[1], nil
}

// egressStatusBlocks splits plain status at each session line, by session ID.
func egressStatusBlocks(stdout string) map[string]string {
	blocks := map[string]string{}
	id := ""
	for _, line := range strings.Split(strings.ReplaceAll(stdout, "\r\n", "\n"), "\n") {
		if rest, ok := strings.CutPrefix(line, "session "); ok {
			id, _, _ = strings.Cut(rest, " ")
		}
		if id != "" {
			blocks[id] += line + "\n"
		}
	}
	return blocks
}

func (w *world) egressStatusPlain() error {
	plain, _, err := w.egressStatusResults()
	if err != nil {
		return err
	}
	s := w.egress.status
	if plain.result.exit != 0 {
		return fmt.Errorf("plain status failed\n%s%s", plain.result.stdout, plain.result.stderr)
	}
	blocks := egressStatusBlocks(plain.result.stdout)
	block, ok := blocks[s.egress.id]
	if !ok {
		return fmt.Errorf("plain status lacks the egress session\n%s", plain.result.stdout)
	}
	if !strings.Contains(block, "fingerprint "+s.egress.fingerprint) || !strings.Contains(block, "allowlist-v1") {
		return fmt.Errorf("egress entry lacks its original fingerprint or profile\n%s", block)
	}
	var names []string
	for _, line := range strings.Split(block, "\n") {
		if name, ok := strings.CutPrefix(line, "network.allow: "); ok {
			names = append(names, name)
		}
	}
	if !reflect.DeepEqual(names, s.egress.allow) {
		return fmt.Errorf("egress entry lists %v, want the original %v", names, s.egress.allow)
	}
	rest := block
	for _, limitation := range egressLimitations {
		_, after, found := strings.Cut(rest, limitation)
		if !found {
			return fmt.Errorf("egress entry lacks, in order, %q\n%s", limitation, block)
		}
		rest = after
	}
	other, ok := blocks[s.offline.id]
	if !ok || !strings.Contains(other, "fingerprint "+s.offline.fingerprint) {
		return fmt.Errorf("plain status lacks the offline session\n%s", plain.result.stdout)
	}
	for _, text := range append([]string{"allowlist-v1", "network.allow"}, egressLimitations...) {
		if strings.Contains(other, text) {
			return fmt.Errorf("offline entry carries egress text %q", text)
		}
	}
	return egressNoCurrent(plain.result.stdout + plain.result.stderr)
}

// The current request and approval name only this host.
func egressNoCurrent(output string) error {
	if strings.Contains(output, "current.example") {
		return fmt.Errorf("status reports the current request instead of the session's own grant")
	}
	return nil
}

func (w *world) egressStatusJSON() error {
	_, structured, err := w.egressStatusResults()
	if err != nil {
		return err
	}
	s := w.egress.status
	var report struct {
		Schema   int
		OK       bool
		Problems []any
		Sessions []map[string]any
	}
	if json.Unmarshal([]byte(structured.result.stdout), &report) != nil || report.Schema != 1 || !report.OK || len(report.Problems) != 0 || structured.result.exit != 0 {
		return fmt.Errorf("JSON status is not one successful schema-1 report\n%s%s", structured.result.stdout, structured.result.stderr)
	}
	if len(report.Sessions) != 2 {
		return fmt.Errorf("JSON status lists %d sessions", len(report.Sessions))
	}
	state, cleanup := "running", "pending"
	if s.phase == "stopped with cleanup complete" {
		state, cleanup = "stopped", "complete"
	}
	for _, entry := range report.Sessions {
		seeded := s.offline
		if entry["id"] == s.egress.id {
			seeded = s.egress
		} else if entry["id"] != s.offline.id {
			return fmt.Errorf("unknown session %v", entry["id"])
		}
		if entry["fingerprint"] != seeded.fingerprint || entry["state"] != state || entry["cleanup"] != cleanup {
			return fmt.Errorf("session %v reports %v/%v/%v", entry["id"], entry["fingerprint"], entry["state"], entry["cleanup"])
		}
		egress, present := entry["egress"]
		if seeded.allow == nil {
			if present {
				return fmt.Errorf("offline entry carries an egress object")
			}
			continue
		}
		want := map[string]any{"profile": "allowlist-v1", "allow": offlineStrings(seeded.allow), "limitations": offlineStrings(egressLimitations)}
		if !sameJSON(egress, want) || len(object(egress)) != 3 {
			return fmt.Errorf("egress object is %s, want %s", approvalJSON(egress), approvalJSON(want))
		}
	}
	return egressNoCurrent(structured.result.stdout + structured.result.stderr)
}

var egressStatusClaim = regexp.MustCompile(`(?i)intercept|refusal|\blog(s|ged)?\b`)

func (w *world) egressStatusNoClaim() error {
	plain, structured, err := w.egressStatusResults()
	if err != nil {
		return err
	}
	for _, r := range []egressResult{plain, structured} {
		text := r.result.stdout + r.result.stderr
		if !strings.Contains(text, egressLimitations[0]) || !strings.Contains(text, egressLimitations[3]) {
			return fmt.Errorf("%s omits the interception or stored-log limitation", r.command)
		}
		for _, limitation := range egressLimitations {
			text = strings.ReplaceAll(text, limitation, "")
		}
		// Outside the four disclosed limits nothing may speak of interception,
		// of a refusal log, of its path or of a count of stored refusals.
		if claim := egressStatusClaim.FindString(text); claim != "" {
			return fmt.Errorf("%s says %q outside the specified limitations", r.command, claim)
		}
	}
	return nil
}

func (w *world) egressStatusReadOnly() error {
	plain, structured, err := w.egressStatusResults()
	if err != nil {
		return err
	}
	for _, r := range []egressResult{plain, structured} {
		text := r.result.stdout + r.result.stderr
		if strings.Contains(text, "y/N") || strings.Contains(text, "itos container approve") {
			return fmt.Errorf("%s asks for consent", r.command)
		}
		for _, request := range r.journal {
			if request.Method != "GET" && request.Method != "HEAD" {
				return fmt.Errorf("%s changed an engine resource: %+v", r.command, request)
			}
		}
	}
	after, err := doctorTree(w.dir)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(after, w.egress.status.before) {
		return fmt.Errorf("status wrote or changed state")
	}
	api := w.doctor.engine.egress
	api.mu.Lock()
	defer api.mu.Unlock()
	if api.problem != nil {
		return api.problem
	}
	want := 0
	if w.egress.status.phase == "running with controller absent" {
		want = 3
	}
	if len(api.containers) != want {
		return fmt.Errorf("status changed the engine's containers")
	}
	return nil
}
