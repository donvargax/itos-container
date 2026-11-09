package features

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
)

func offlineFrames(values []string) []byte {
	var result bytes.Buffer
	_ = binary.Write(&result, binary.BigEndian, uint32(len(values)))
	for _, value := range values {
		_ = binary.Write(&result, binary.BigEndian, uint32(len(value)))
		_, _ = result.WriteString(value)
	}
	return result.Bytes()
}

func (w *world) offlineObserved() error {
	if w.offline.err != nil {
		return w.offline.err
	}
	return nil
}

func (w *world) offlineArguments() error {
	if err := w.offlineObserved(); err != nil {
		return err
	}
	expected := append([]byte("slice-10:argv\x00"), offlineFrames(w.offline.arguments)...)
	if !bytes.Equal(w.offline.stdout, expected) {
		return fmt.Errorf("worker did not report the exact literal argv bytes (product exit %d)", w.offline.exit)
	}
	return nil
}

func (w *world) offlineNoShellMarkers() error {
	for _, root := range []string{w.doctor.checkout, w.doctor.host} {
		if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			switch entry.Name() {
			case "shell-marker", "backtick-marker", "semicolon-marker", "redirect-marker", "env-marker", "host-helper-marker":
				return fmt.Errorf("offline execution created a shell or host helper marker at %s", path)
			}
			return nil
		}); err != nil {
			return err
		}
	}
	return nil
}

func (w *world) offlineHandshake() error {
	if err := w.offlineObserved(); err != nil {
		return err
	}
	if !w.offline.handshake || !w.offline.inputClosed {
		return fmt.Errorf("both live readiness streams were not observed before the stdin reply (product exit %d)", w.offline.exit)
	}
	return nil
}

func (w *world) offlineStreamBytes(inputOnly bool) error {
	if err := w.offlineHandshake(); err != nil {
		return err
	}
	ack := append([]byte("slice-10:stdin-and-eof\x00"), offlineFrames([]string{string(offlineInput)})...)
	if !bytes.Contains(w.offline.stdout, ack) {
		return fmt.Errorf("worker did not acknowledge exact stdin bytes and EOF")
	}
	if inputOnly {
		return nil
	}
	expectedOut := append(bytes.Clone(offlineReadyOut), ack...)
	expectedOut = append(expectedOut, offlineTailOut...)
	expectedErr := append(bytes.Clone(offlineReadyErr), offlineTailErr...)
	if !bytes.Equal(w.offline.stdout, expectedOut) || !bytes.Equal(w.offline.stderr, expectedErr) {
		return fmt.Errorf("separate foreground streams lost bytes, gained controller bytes, or were mixed")
	}
	return nil
}

func (w *world) offlineShellWorkspace() error {
	if err := w.offlineObserved(); err != nil {
		return err
	}
	if !bytes.HasPrefix(w.offline.stdout, []byte("/workspace/subdir\n")) {
		return fmt.Errorf("explicit shell did not report the frozen worker directory (product exit %d)", w.offline.exit)
	}
	data, err := os.ReadFile(filepath.Join(w.doctor.checkout, "subdir", "edit.txt"))
	if err != nil || string(data) != "worker" {
		return fmt.Errorf("explicit shell did not leave its edit in the approved subdirectory")
	}
	// The native observation must place the command inside the worker as well;
	// the file and output alone could be manufactured by a host-side execution.
	return w.offlineLaunch()
}

func (w *world) offlineLiteralEnvironment() error {
	if !bytes.HasPrefix(w.offline.stdout, []byte("/workspace/subdir\n"+offlineAPP+"\n")) {
		return fmt.Errorf("worker APP was expanded or changed")
	}
	return w.offlineNoShellMarkers()
}

func (w *world) offlineEnvironment() error {
	prefix := []byte("/workspace/subdir\n" + offlineAPP + "\n/tmp\n/tmp\n")
	if !bytes.HasPrefix(w.offline.stdout, prefix) {
		return fmt.Errorf("worker HOME and TMPDIR are not /tmp")
	}
	var report struct {
		Present, Writable map[string]bool
	}
	if err := json.Unmarshal(bytes.TrimPrefix(w.offline.stdout, prefix), &report); err != nil {
		return fmt.Errorf("worker environment probe did not complete")
	}
	expected := map[string]bool{"HOST_ONLY": false, "GITHUB_TOKEN": false, "AWS_SECRET_ACCESS_KEY": false, "ITOS_ROOT_MARKER": false, "host-path": false}
	if !reflect.DeepEqual(report.Present, expected) || !reflect.DeepEqual(report.Writable, map[string]bool{"HOME": true, "TMPDIR": true}) {
		return fmt.Errorf("worker inherited host markers or could not write its temporary home")
	}
	return nil
}

func (w *world) offlineLaunch() error {
	o, a := w.offline, w.offline.api
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.problem != nil {
		return a.problem
	}
	if len(a.created) != 1 {
		return fmt.Errorf("expected one native worker, observed %d", len(a.created))
	}
	id := a.created[0]
	creates, starts, attaches := 0, 0, 0
	inspected := false
	for _, r := range a.requests {
		if r.Status >= 400 && r.Status != 404 {
			return fmt.Errorf("offline request was refused or failed: %s %s HTTP %d", r.Method, r.Path, r.Status)
		}
		if r.Method == "POST" && r.Path == "/containers/create" {
			creates++
			if err := w.sessionExactRecord(r.Record, "intent", ""); err != nil {
				return err
			}
			v := a.anchorVolumes()["itos-container-anchor-"+w.session.token]
			if v == nil || !anchorVolumeMatches(v, r.Anchor) {
				return fmt.Errorf("create lacks its independently observed original anchor")
			}
			if !a.safeCreate(r.Body) || r.Body["WorkingDir"] != object(w.approval.settings["manifest"])["workspaceFolder"] ||
				!reflect.DeepEqual(r.Body["Cmd"], offlineStrings(o.argv)) || !reflect.DeepEqual(r.Body["Entrypoint"], []any{}) {
				return fmt.Errorf("create did not preserve the frozen command and runtime settings")
			}
		}
		if r.Method == "GET" && r.Path == "/containers/"+id+"/json" && r.Status == 200 {
			if err := a.identity(r.Observation, id); err != nil {
				return err
			}
			inspected = true
		}
		if r.Method == "POST" && r.Path == "/containers/"+id+"/attach" && r.Status == 101 {
			attaches++
		}
		if r.Method == "POST" && r.Path == "/containers/"+id+"/start" {
			starts++
			if !inspected || attaches != 1 || r.Status != 204 || !a.safeObserved(r.Observation) ||
				object(r.Observation["State"])["Status"] != "created" {
				return fmt.Errorf("start preceded inspection/attach or had an unsafe observed boundary")
			}
			if err := w.sessionExactRecord(r.Record, "owned", id); err != nil {
				return err
			}
			v := a.anchorVolumes()["itos-container-anchor-"+w.session.token]
			if v == nil || !anchorVolumeMatches(v, r.Anchor) {
				return fmt.Errorf("start lacks its independently observed original anchor")
			}
		}
	}
	if creates != 1 || starts != 1 || attaches != 1 {
		return fmt.Errorf("expected one create/attach/start, observed %d/%d/%d", creates, attaches, starts)
	}
	return nil
}

func offlineStrings(values []string) []any {
	result := make([]any, len(values))
	for i, value := range values {
		result[i] = value
	}
	return result
}

func (w *world) offlineSuccess() error {
	if err := w.offlineObserved(); err != nil {
		return err
	}
	if w.offline.exit != 0 {
		return fmt.Errorf("offline product exit = %d, expected 0", w.offline.exit)
	}
	if err := w.offlineLaunch(); err != nil {
		return err
	}
	a := w.offline.api
	a.mu.Lock()
	defer a.mu.Unlock()
	id := a.created[0]
	removed, confirmed, waited := false, false, false
	for _, r := range a.requests {
		if r.Method == "POST" && r.Path == "/containers/"+id+"/wait" && r.Status == 200 && r.Observation["StatusCode"] == float64(0) {
			waited = true
		}
		if r.Method == "DELETE" && r.Path == "/containers/"+id && r.Status == 204 {
			if !waited {
				return fmt.Errorf("product removal preceded observed primary exit")
			}
			removed = true
		}
		if removed && r.Method == "GET" && r.Path == "/containers/"+id+"/json" && r.Status == 404 {
			confirmed = true
		}
	}
	if !removed || !confirmed || !a.absent[id] {
		return fmt.Errorf("zero exit preceded product removal and independently confirmed full-ID absence")
	}
	if err := w.sessionExactRecord(a.final, "absent", id); err != nil {
		return err
	}
	// Presence is only a content check here. Fsync ordering remains the local
	// session contract's job; these runtime cases make no new durability claim.
	path := filepath.Join(w.doctor.env["XDG_STATE_HOME"], "itos-container", "snapshots", w.approval.project, "1.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var frozen map[string]any
	if json.Unmarshal(data, &frozen) != nil || !reflect.DeepEqual(frozen, approvalClone(map[string]any{"schema": 1, "project": w.approval.project, "generation": "1", "fingerprint": w.approval.fingerprint, "settings": w.approval.settings})) {
		return fmt.Errorf("run lost or changed its frozen settings snapshot")
	}
	if strings.Contains(string(w.offline.stderr), "SLICE_10_SYNTHETIC_") {
		return fmt.Errorf("controller copied a synthetic host-only marker into diagnostics")
	}
	return w.offlineUnrelated()
}
