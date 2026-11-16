package features

import (
	"encoding/base64"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

func (w *world) credentialApprovalNoAccess() error {
	if w.credentialApproval.accessError != nil {
		return w.credentialApproval.accessError
	}
	return w.credentialApprovalScan()
}

func (w *world) credentialApprovalNeedles() []string {
	var needles []string
	for _, marker := range w.credentials.markers {
		needles = append(needles, marker)
		for i := 0; i < 3; i++ {
			text := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("x", i) + marker))
			needles = append(needles, text[4:len(text)-4])
		}
	}
	return needles
}

func credentialApprovalLeak(data string, needles []string) bool {
	for _, needle := range needles {
		if strings.Contains(data, needle) {
			return true
		}
	}
	return false
}

func (w *world) credentialApprovalMarkerControl() error {
	needles := w.credentialApprovalNeedles()
	if len(needles) == 0 {
		return fmt.Errorf("no marker control")
	}
	var controls []string
	for _, marker := range w.credentials.markers {
		controls = append(controls, "independent raw control: "+marker)
		for offset := 0; offset < 3; offset++ {
			controls = append(controls, base64.StdEncoding.EncodeToString([]byte(strings.Repeat("x", offset)+marker+"suffix")))
		}
	}
	path := filepath.Join(w.doctor.host, "slice-30-marker-control")
	if err := os.WriteFile(path, []byte(strings.Join(controls, "\n")), 0600); err != nil {
		return err
	}
	bytes, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	for _, control := range strings.Split(string(bytes), "\n") {
		if !credentialApprovalLeak(control, needles) {
			return fmt.Errorf("marker scanner missed a raw or base64-offset positive control")
		}
	}
	return w.approvalCapture()
}

func (w *world) credentialApprovalScan() error {
	needles := w.credentialApprovalNeedles()
	check := func(data string) error {
		if credentialApprovalLeak(data, needles) {
			return fmt.Errorf("synthetic credential escaped")
		}
		return nil
	}
	if err := check(w.stdout + w.stderr); err != nil {
		return err
	}
	for _, r := range w.approval.results {
		if err := check(r.stdout + r.stderr); err != nil {
			return err
		}
	}
	w.doctor.engine.mu.Lock()
	bodies := slices.Clone(w.doctor.engine.bodyJournal)
	bodyError := w.doctor.engine.bodyError
	w.doctor.engine.mu.Unlock()
	if bodyError != nil {
		return bodyError
	}
	for _, body := range bodies {
		if err := check(body); err != nil {
			return err
		}
	}
	journal := string(approvalJSON(w.doctor.engine.requests()))
	if err := check(journal); err != nil {
		return err
	}
	for _, root := range []string{w.doctor.checkout, filepath.Join(w.doctor.env["XDG_STATE_HOME"], "itos-container")} {
		if err := filepath.WalkDir(root, func(path string, e fs.DirEntry, err error) error {
			if os.IsNotExist(err) {
				return nil
			}
			if err != nil {
				return err
			}
			if !e.Type().IsRegular() {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			return check(string(data))
		}); err != nil {
			return err
		}
	}
	return nil
}

func (w *world) credentialApprovalSafeText(text string) string {
	if w.credentialApproval == nil || w.credentials == nil {
		return text
	}
	for _, needle := range w.credentialApprovalNeedles() {
		text = strings.ReplaceAll(text, needle, "<synthetic marker redacted>")
	}
	return text
}
