package features

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
)

func (w *world) anchorOriginal() error {
	s, o := w.anchor, w.offline
	if s.err != nil {
		return fmt.Errorf("native anchor observations unavailable: %w", s.err)
	}
	if !s.reached || !s.held {
		return fmt.Errorf("run did not reach the submitted-start barrier (exit %d, stderr %q)", o.exit, o.stderr)
	}
	if len(s.reports) == 0 {
		// Q-27's detector runs after start. A detector refusal is evidence that
		// the anchor failed, not green proof even if teardown beat probe output.
		if o.api != nil {
			for _, q := range o.api.requests {
				if q.Method == "POST" && strings.HasSuffix(q.Path, "/start") && q.Status >= 200 && q.Status < 300 {
					return errors.New("anchor detector refused after successful native start")
				}
			}
		}
		if o.exit != 1 || len(o.stdout) != 0 || !strings.Contains(string(o.stderr), "run.mount-refused") {
			return fmt.Errorf("missing original-inode execution or explicit mount refusal (exit %d, stderr %q)", o.exit, o.stderr)
		}
		for _, root := range []string{s.original, s.replacement} {
			if _, err := os.Stat(filepath.Join(root, "subdir", "slice-17-edit")); !os.IsNotExist(err) {
				return errors.New("refused race wrote a workload edit")
			}
		}
		return nil
	}
	return w.anchorExecutedOriginal()
}

func (w *world) anchorExecutedOriginal() error {
	s := w.anchor
	if len(s.reports) != 2 || s.reports[0].Phase != "ready" || s.reports[1].Phase != "checked" {
		return fmt.Errorf("expected two complete native probe frames, observed %d", len(s.reports))
	}
	for _, report := range s.reports {
		if report.Identity != s.originalID || report.Nonce != s.nonce || report.CWD != "/workspace/subdir" {
			return fmt.Errorf("worker observed identity %s nonce %q cwd %q; validated original is %s nonce %q (Docker %s)", report.Identity, report.Nonce, report.CWD, s.originalID, s.nonce, w.doctor.real.serverVersion)
		}
	}
	edit, err := os.ReadFile(filepath.Join(s.original, "subdir", "slice-17-edit"))
	if err != nil || string(edit) != s.nonce {
		return errors.New("workload edit is absent from the moved original")
	}
	if _, err = os.Stat(filepath.Join(s.replacement, "subdir", "slice-17-edit")); !os.IsNotExist(err) {
		return errors.New("workload changed the substituted checkout")
	}
	return nil
}

func (w *world) anchorCheck(check string) error {
	if err := w.anchorOneCheck(check); err != nil {
		return fmt.Errorf("substitution run: %w", err)
	}
	if err := w.anchor.control.anchorOneCheck(check); err != nil {
		return fmt.Errorf("no-substitution control: %w", err)
	}
	return nil
}

func (w *world) anchorOneCheck(check string) error {
	s := w.anchor
	if s.err != nil {
		return s.err
	}
	switch check {
	case "source":
		if len(s.volumes) != 1 || len(s.records) == 0 {
			return errors.New("missing independently observed anchor volume and protected intent/ownership records")
		}
		for _, v := range s.volumes {
			if !v.beforeWorker || !anchorVolumeMatches(v, v.observed) {
				return errors.New("anchor volume was not independently owned before worker create")
			}
			mounts, _ := s.before["Mounts"].([]any)
			workspace := 0
			for _, item := range mounts {
				m := object(item)
				if m["Type"] == "tmpfs" && m["Destination"] == "/tmp" {
					continue
				}
				if m["Type"] != "volume" || m["Name"] != v.name || m["Destination"] != "/workspace" || m["RW"] != true {
					return errors.New("worker has an unexpected host-source mount")
				}
				workspace++
			}
			if workspace != 1 {
				return errors.New("worker does not mount exactly its owned anchor at /workspace")
			}
		}
		for _, record := range s.records {
			if record.Mode != 0600 || record.UID != w.approval.uid || record.Inode == "" || !strings.Contains(record.Bytes, w.approval.fingerprint) {
				return errors.New("anchor ownership record is not protected or bound to frozen approval")
			}
		}
	case "reads", "mutations":
		// Refusal releases no workload; the independently successful control
		// must still attempt every denied read/mutation and its positive controls.
		if len(s.reports) == 0 && s.reached {
			return nil
		}
		if len(s.reports) != 2 || len(s.records) == 0 || !s.nestedOK || object(s.running["State"])["Running"] != true {
			return errors.New("missing live probe, actual protected record targets or nested-mount controls")
		}
		r := s.reports[1]
		if check == "reads" {
			for _, path := range []string{"/workspace/slice-17-replacement", "/workspace/slice-17-nested/marker"} {
				if !r.Denied[path+":read"] {
					return fmt.Errorf("missing denied read of %s", path)
				}
			}
			for path := range s.records {
				if !r.Denied[path+":read"] {
					return errors.New("protected anchor record read did not fail")
				}
			}
			return nil
		}
		if !reflect.DeepEqual(s.records, s.checkedRecords) {
			return errors.New("anchor control records changed during the workload mutation attempts")
		}
		for path := range s.records {
			for _, op := range []string{"append", "rename", "unlink"} {
				if !r.Denied[path+":"+op] || !r.Controls[path+":"+op+":before"] || !r.Controls[path+":"+op+":after"] {
					return fmt.Errorf("incomplete denial or positive controls for anchor-record %s", op)
				}
			}
		}
	case "cleanup":
		if len(w.offline.api.created) != 1 || len(s.volumes) != 1 {
			return errors.New("missing concrete worker or anchor identity")
		}
		for _, id := range w.offline.api.created {
			if !w.offline.api.absent[id] {
				return errors.New("product did not confirm worker absence before harness cleanup")
			}
		}
		for _, v := range s.volumes {
			if !v.absent {
				return errors.New("product did not remove its anchor volume")
			}
		}
		if w.offline.resourcesBefore == nil || !reflect.DeepEqual(w.offline.resourcesBefore, w.offline.resourcesAfter) {
			return errors.New("native anchor run changed unrelated engine resources or image references")
		}
		if len(s.reports) != 0 && w.offline.exit != s.child {
			return fmt.Errorf("completed anchored workload returned %d: %s", w.offline.exit, w.offline.stderr)
		}
		after, err := anchorTree(s.replacement)
		if err != nil || !reflect.DeepEqual(s.replacementBefore, after) {
			return errors.New("substituted checkout changed")
		}
	default:
		return fmt.Errorf("unknown anchor assertion %q", check)
	}
	return nil
}

func (w *world) anchorControl() error {
	control := w.anchor.control
	if control.anchor.err != nil {
		return control.anchor.err
	}
	if err := control.anchorExecutedOriginal(); err != nil {
		return err
	}
	for _, check := range []string{"source", "reads", "mutations", "cleanup"} {
		if err := control.anchorOneCheck(check); err != nil {
			return err
		}
	}
	return nil
}

func anchorTree(root string) (map[string]string, error) {
	tree, err := doctorTree(root)
	result := map[string]string{}
	for path, value := range tree {
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return nil, relErr
		}
		result[rel] = value
	}
	return result, err
}
