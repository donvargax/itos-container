package features

import (
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"
)

func (w *world) offlineLaunchObserved() error {
	s := w.session
	if s.fixtureError != nil {
		return fmt.Errorf("offline observation failed: %w; %s", s.fixtureError, s.transcript)
	}
	if len(s.create.Trace) == 0 {
		return fmt.Errorf("runner produced no port observations: %s", s.create.Diagnostic)
	}
	return nil
}

func (w *world) offlineLaunchControl() error {
	if err := w.offlineLaunchObserved(); err != nil {
		return err
	}
	control := w.session.offlineControl
	if control == nil {
		return fmt.Errorf("missing fault-free control")
	}
	if err := control.offlineLaunchOrder(); err != nil {
		return fmt.Errorf("positive control: %w", err)
	}
	if err := control.offlineLaunchSuccess(); err != nil {
		return fmt.Errorf("positive control: %w", err)
	}
	return control.offlineFrozenBoundary()
}

func (w *world) offlineLaunchOrder() error {
	if err := w.offlineLaunchObserved(); err != nil {
		return err
	}
	s := w.session
	if len(s.api.created) != 1 {
		return fmt.Errorf("launch needs one actual create, got %d", len(s.api.created))
	}
	id := s.api.created[0]
	// Ordered real port observations join filesystem durability and SDK calls.
	sequence := []string{"intent", "open", "validate", "check", "create", "inspect", "save", "verify", "attach", "validate", "check", "verify", "start", "wait"}
	next, starts := 0, 0
	for _, event := range s.create.Trace {
		if event.Operation == "start" {
			starts++
		}
		if next == len(sequence) || event.Operation != sequence[next] {
			continue
		}
		if event.Failure != "" {
			return fmt.Errorf("launch %s failed: %s", event.Operation, event.Failure)
		}
		switch event.Operation {
		case "create":
			if event.ID != id || !reflect.DeepEqual(event.Record, w.sessionExpectedRecord("intent", "")) {
				return fmt.Errorf("create lacks exact original intent")
			}
			if err := w.sessionDurability(event.Writes, "intent", ""); err != nil {
				return err
			}
		case "inspect":
			o := event.Observation
			if event.ID != id || event.Status != 0 || o.ID != id || o.Token != s.token || o.Project != w.approval.project || o.Fingerprint != w.approval.fingerprint || o.Name != "itos-container-"+s.token || o.ImageID != w.doctor.imageID || !o.StatePresent || o.Started || o.Running || o.Status != "created" {
				return fmt.Errorf("independent ownership inspection did not precede publication")
			}
		case "save", "verify", "attach", "start":
			if !reflect.DeepEqual(event.Record, w.sessionExpectedRecord("owned", id)) {
				return fmt.Errorf("%s lacks exact concrete ownership", event.Operation)
			}
			if err := w.offlineOwnedDurability(event.Writes, id); err != nil {
				return err
			}
			if event.Operation != "save" && event.ID != id {
				return fmt.Errorf("%s targets another worker", event.Operation)
			}
		}
		next++
	}
	if next != len(sequence) || starts != 1 || s.create.ID != id {
		return fmt.Errorf("missing ordered launch evidence at %d/%d; starts=%d outcome=%s", next, len(sequence), starts, s.create.Outcome)
	}
	if s.backend == "disk" {
		inspections, attached, started := 0, false, false
		for _, q := range s.api.requests {
			if q.Path == "/containers/"+id+"/json" && q.Status == 200 && !started {
				inspections++
			}
			if q.Path == "/containers/"+id+"/attach" {
				attached = q.Status == 101 && inspections >= 2
			}
			if q.Path == "/containers/"+id+"/start" {
				if !attached || inspections < 3 || started || q.Status != 204 {
					return fmt.Errorf("SDK start preceded inspections/attach or repeated")
				}
				if err := w.sessionExactRecord(q.Record, "owned", id); err != nil {
					return err
				}
				if err := w.offlineOwnedDurability(q.Events, id); err != nil {
					return err
				}
				started = true
			}
		}
		if !started {
			return fmt.Errorf("SDK never actually started the inspected worker")
		}
	}
	return nil
}

func (w *world) offlineFrozenBoundary() error {
	s, a, d := w.session, w.approval, w.doctor
	argv := []string{"probe", "literal;$HOME", "slice-13-private-argv"}
	checks := 0
	for _, event := range s.create.Trace {
		if event.Operation != "create" && event.Operation != "verify" {
			continue
		}
		checks++
		b := event.Binding
		var env any
		if json.Unmarshal([]byte(fmt.Sprint(b["Environment"])), &env) != nil {
			return fmt.Errorf("binding lost environment")
		}
		if b["UID"] != a.uid || b["GID"] != a.gid || b["ImageID"] != d.imageID || b["ImageReference"] != d.image || b["Workspace"] != object(a.settings["manifest"])["workspaceFolder"] || object(b["Checkout"])["Root"] != d.checkout || !reflect.DeepEqual(env, object(a.settings["manifest"])["containerEnv"]) || !slices.Equal(event.Argv, argv) {
			return fmt.Errorf("%s used different frozen runtime values", event.Operation)
		}
	}
	if checks != 3 {
		return fmt.Errorf("expected frozen create and two verifications, got %d", checks)
	}
	if s.backend == "memory" {
		return nil
	} // Memory proves domain bindings, not a synthesized SDK request.
	inspected := 0
	for _, q := range s.api.requests {
		if q.Method == "POST" && q.Path == "/containers/create" {
			if err := w.offlineExactConfiguration(q.Body, object(q.Body["HostConfig"])); err != nil {
				return fmt.Errorf("create: %w", err)
			}
		}
		if q.Method == "GET" && strings.HasPrefix(q.Path, "/containers/") && q.Status == 200 && object(q.Response["State"])["Status"] == "created" {
			inspected++
			if err := w.offlineExactConfiguration(object(q.Response["Config"]), object(q.Response["HostConfig"])); err != nil {
				return fmt.Errorf("inspection: %w", err)
			}
			mounts := q.Response["Mounts"]
			want := []any{map[string]any{"Type": "bind", "Source": d.checkout, "Destination": "/workspace", "RW": true, "Propagation": "rprivate"}}
			if !reflect.DeepEqual(mounts, want) || len(object(object(q.Response["NetworkSettings"])["Networks"])) != 1 || object(object(q.Response["NetworkSettings"])["Networks"])["none"] == nil {
				return fmt.Errorf("inspection has extra mounts or networks")
			}
		}
	}
	if inspected != 3 {
		return fmt.Errorf("expected three actual unstarted inspections, got %d", inspected)
	}
	return nil
}

func (w *world) offlineExactConfiguration(config, host map[string]any) error {
	a, d := w.approval, w.doctor
	want := map[string]any{
		"Image": d.imageID, "User": a.uid + ":" + a.gid, "WorkingDir": object(a.settings["manifest"])["workspaceFolder"],
		"Cmd": []any{"probe", "literal;$HOME", "slice-13-private-argv"}, "Healthcheck": map[string]any{"Test": []any{"NONE"}},
		"Tty": false, "OpenStdin": true, "StdinOnce": true, "AttachStdin": true, "AttachStdout": true, "AttachStderr": true,
	}
	for key, value := range want {
		if !reflect.DeepEqual(config[key], value) {
			return fmt.Errorf("wrong configuration %s: %v", key, config[key])
		}
	}
	if len(anySlice(config["Entrypoint"])) != 0 || len(object(config["Volumes"])) != 0 {
		return fmt.Errorf("image entrypoint or volume inherited")
	}
	env := map[string]string{}
	for _, entry := range anySlice(config["Env"]) {
		key, value, _ := strings.Cut(fmt.Sprint(entry), "=")
		if _, exists := env[key]; exists {
			return fmt.Errorf("duplicate environment key")
		}
		env[key] = value
	}
	expected := map[string]string{"IMAGE_ONLY": "retained", "HOME": "/tmp", "TMPDIR": "/tmp"}
	for key, value := range object(object(a.settings["manifest"])["containerEnv"]) {
		expected[key] = fmt.Sprint(value)
	}
	if !reflect.DeepEqual(env, expected) {
		return fmt.Errorf("environment differs from independent image defaults and approved overlay")
	}
	wantHost := map[string]any{
		"NetworkMode": "none", "ReadonlyRootfs": true, "CapDrop": []any{"ALL"}, "SecurityOpt": []any{"no-new-privileges"}, "PidsLimit": float64(4096),
		"IpcMode": "private", "CgroupnsMode": "private", "Init": false, "LogConfig": map[string]any{"Type": "none", "Config": nil},
		"Tmpfs": map[string]any{"/tmp": "rw,nosuid,nodev,mode=1777"},
	}
	for key, value := range wantHost {
		if !reflect.DeepEqual(host[key], value) {
			return fmt.Errorf("wrong hardened field %s: %v", key, host[key])
		}
	}
	if object(host["RestartPolicy"])["Name"] != "no" {
		return fmt.Errorf("restart policy enabled")
	}
	for _, key := range []string{"Privileged", "AutoRemove", "PublishAllPorts"} {
		if host[key] == true {
			return fmt.Errorf("enabled %s", key)
		}
	}
	for _, key := range []string{"PidMode", "UTSMode", "UsernsMode"} {
		if host[key] != "" && host[key] != nil {
			return fmt.Errorf("nonprivate %s", key)
		}
	}
	for _, key := range []string{"CapAdd", "GroupAdd", "Devices", "DeviceRequests", "DeviceCgroupRules", "Binds", "VolumesFrom", "Links", "ExtraHosts"} {
		if len(anySlice(host[key])) != 0 {
			return fmt.Errorf("extra privilege/source %s", key)
		}
	}
	if len(object(host["PortBindings"])) != 0 || len(object(host["Sysctls"])) != 0 {
		return fmt.Errorf("extra port or sysctl")
	}
	mounts := anySlice(host["Mounts"])
	if len(mounts) != 1 {
		return fmt.Errorf("checkout is not the only host source")
	}
	m := object(mounts[0])
	options := object(m["BindOptions"])
	if m["Type"] != "bind" || m["Source"] != d.checkout || m["Target"] != "/workspace" || m["ReadOnly"] == true || options["Propagation"] != "rprivate" || options["NonRecursive"] != true || options["CreateMountpoint"] == true {
		return fmt.Errorf("checkout bind differs from frozen nonrecursive private writable mount")
	}
	return nil
}

func anySlice(value any) []any { result, _ := value.([]any); return result }

func (w *world) offlineLaunchSuccess() error {
	if err := w.offlineLaunchObserved(); err != nil {
		return err
	}
	r := w.session.create
	exit := w.session.launch.child
	if r.Code != exit || r.Workload == nil || *r.Workload != exit || r.Outcome != "" || r.Cleanup != "cleanup-complete" || r.Stdout != "slice-13-workload-out\n" || r.Stderr != "slice-13-workload-err\n" {
		return fmt.Errorf("workload/cleanup/stream result differs: code %d outcome %q cleanup %q stdout %q stderr %q", r.Code, r.Outcome, r.Cleanup, r.Stdout, r.Stderr)
	}
	sequence := []string{"wait", "drain", "reopen", "open", "validate", "check", "inspect", "save", "validate", "check", "remove", "inspect", "save"}
	next := 0
	for _, e := range r.Trace {
		if next == len(sequence) || e.Operation != sequence[next] {
			continue
		}
		if e.Failure != "" {
			return fmt.Errorf("cleanup %s failed", e.Operation)
		}
		if next == 0 && e.Exit != exit {
			return fmt.Errorf("lost observed wait result")
		}
		if next == 11 && e.Status != 1 {
			return fmt.Errorf("cleanup did not observe absence")
		}
		next++
	}
	if next != len(sequence) {
		return fmt.Errorf("cleanup ordering incomplete at %d", next)
	}
	return w.offlineLocalCleanup()
}

func (w *world) offlineEffects() error {
	if err := w.sessionUnrelated(); err != nil {
		return err
	}
	s := w.session
	foreign := strings.Repeat("e", 64)
	if memoryObservation(s.api.local[foreign]) != memoryObservation(mapFromJSON(s.resourcesBefore[foreign])) {
		return fmt.Errorf("unrelated worker changed")
	}
	for _, q := range s.api.requests {
		if q.Method == "GET" || q.Method == "HEAD" {
			if doctorAllowedRequest(doctorRequest{Method: q.Method, Path: q.Path, Query: q.Query}, w.doctor.image, true) || q.Path == "/images/"+w.doctor.imageID+"/json" {
				continue
			}
			if strings.HasPrefix(q.Path, "/containers/") && strings.HasSuffix(q.Path, "/json") {
				continue
			}
		}
		if q.Method == "POST" && q.Path == "/containers/create" {
			continue
		}
		allowed := false
		for _, id := range s.api.created {
			if q.Method == "DELETE" && q.Path == "/containers/"+id {
				allowed = true
			}
			for _, operation := range []string{"attach", "start", "wait"} {
				if q.Method == "POST" && q.Path == "/containers/"+id+"/"+operation {
					allowed = true
				}
			}
		}
		if !allowed {
			return fmt.Errorf("unexpected engine operation %s %s", q.Method, q.Path)
		}
	}
	return nil
}

func mapFromJSON(data string) map[string]any {
	var value map[string]any
	_ = json.Unmarshal([]byte(data), &value)
	return value
}

func (w *world) offlineIncomplete() error {
	s, l := w.session, w.session.launch
	r := s.create
	if !l.reached || r.Workload == nil || *r.Workload != l.child || r.Code == 0 || r.Cleanup == "cleanup-complete" || r.Reference != s.reference || !strings.Contains(r.Diagnostic, s.reference) || !strings.Contains(r.Diagnostic, "cleanup is not confirmed") {
		return fmt.Errorf("incomplete cleanup lost workload or recovery reference")
	}
	if err := w.offlineLaunchOrder(); err != nil {
		return err
	}
	if len(s.api.created) != 1 || s.api.local[s.api.created[0]] == nil || s.api.absent[s.api.created[0]] {
		return fmt.Errorf("failed cleanup fabricated absence")
	}
	if l.cause == "configuration" {
		if s.final.Bytes != `{"invalid":` {
			return fmt.Errorf("invalid recovery record overwritten")
		}
	} else if err := w.sessionExactRecord(s.final, "owned", s.api.created[0]); err != nil {
		return err
	}
	removals := 0
	for _, q := range s.api.requests {
		if q.Method == "DELETE" {
			removals++
			if l.cause != "temporary" || (s.backend == "disk" && q.Status != 503) {
				return fmt.Errorf("unsafe or unexpectedly successful removal after cleanup fault")
			}
		}
	}
	if l.cause == "temporary" && removals != 1 {
		return fmt.Errorf("transient fault never exercised owned removal")
	}
	for _, e := range r.Events {
		if e.Stage == "absent" {
			return fmt.Errorf("failed cleanup published completion")
		}
	}
	return w.offlineEffects()
}
