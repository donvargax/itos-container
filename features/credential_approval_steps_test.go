package features

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"

	"github.com/cucumber/godog"
)

type credentialApprovalWorld struct {
	observedInputs []string
	request        string
	grants         []credentialGrant
	accessError    error
	runResult      doctorResult
	runBefore      map[string]string
	frozen         map[string]any
	admission      *credentialAdmissionFixture
}

func (w *world) credentialApprovalSteps(sc *godog.ScenarioContext) {
	w.credentialApprovalHelpSteps(sc)
	w.credentialApprovalNativeSteps(sc)
	w.credentialApprovalHistorySteps(sc)
	w.credentialAdmissionSteps(sc)
	w.credentialFreezeSteps(sc)
	sc.Step(`^a credential approval checkout with private synthetic sources$`, w.credentialApprovalCheckout)
	sc.Step(`^the credential approval request is "([^"]*)" on "([^"]*)"$`, w.credentialApprovalRequest)
	sc.Step(`^doctor reports the independently calculated credential fingerprint and complete effective settings$`, w.approvalCandidate)
	sc.Step(`^its credential profile is "([^"]*)" with exactly the sorted grants and fixed targets$`, w.credentialApprovalProfile)
	sc.Step(`^credential inspection is read-only with no publication or source data access$`, w.credentialApprovalReadOnly)
}

func (w *world) credentialApprovalCheckout() error {
	// Native observations must use a named filesystem. This is real checkout
	// storage, not a substituted mount table or a production bypass.
	dir, err := os.MkdirTemp(w.root, "slice-30-")
	if err != nil {
		return err
	}
	if err := os.Remove(w.dir); err != nil {
		_ = os.RemoveAll(dir)
		return err
	}
	w.dir = dir
	if err := w.approvalCheckout(); err != nil {
		return err
	}
	return w.credentialApprovalSources()
}

func (w *world) credentialApprovalSources() error {
	w.credentialApproval = &credentialApprovalWorld{}
	w.doctor.engine.mu.Lock()
	w.doctor.engine.credentialBodies = true
	w.doctor.engine.mu.Unlock()
	c := &credentialWorld{paths: map[string]string{}, sources: filepath.Join(w.doctor.host, "sources"), state: filepath.Join(w.doctor.env["XDG_STATE_HOME"], "itos-container")}
	w.credentials = c
	if err := os.WriteFile(filepath.Join(w.doctor.host, "slice-30-read-control"), []byte("independent observer control"), 0600); err != nil {
		return err
	}
	if err := os.MkdirAll(c.sources, 0700); err != nil {
		return err
	}
	for _, name := range []string{"claude-a", "claude-b", "github-a"} {
		// Sixteen random bytes are represented by exactly 32 lowercase hex digits.
		var random [16]byte
		if _, err := rand.Read(random[:]); err != nil {
			return err
		}
		marker := fmt.Sprintf("itos-synthetic-credential-%x", random)
		c.markers = append(c.markers, marker)
		c.paths[name] = filepath.Join(c.sources, name)
		if err := os.WriteFile(c.paths[name], []byte(marker), 0600); err != nil {
			return err
		}
	}
	return nil
}

func (w *world) credentialApprovalRequest(request, network string) error {
	c := w.credentials
	var grants []credentialGrant
	add := func(kind, name string) { grants = append(grants, credentialGrant{Kind: kind, Source: c.paths[name]}) }
	switch request {
	case "none":
	case "claude":
		add("claude-token", "claude-a")
	case "github":
		add("github-token", "github-a")
	case "both":
		add("github-token", "github-a")
		add("claude-token", "claude-a")
	case "repointed claude":
		add("claude-token", "claude-b")
	case "swapped sources":
		add("github-token", "claude-a")
		add("claude-token", "github-a")
	default:
		return fmt.Errorf("unknown credential request %q", request)
	}
	w.credentialApproval.request, w.credentialApproval.grants = request, grants
	w.approval.policyVersion = 3
	w.approval.allow = nil
	if network == "allowlist" {
		w.approval.allow = []string{"api.example.com", "registry.example.org"}
		if w.doctor.engine.egress == nil {
			if err := w.localEgressFixture(); err != nil {
				return err
			}
		}
	} else if network != "offline" {
		return fmt.Errorf("unknown network %q", network)
	}
	if err := w.credentialApprovalPolicy(); err != nil {
		return err
	}
	if err := w.approvalOracle(); err != nil {
		return err
	}
	return w.approvalCapture()
}

func (w *world) credentialApprovalPolicy() error {
	p := map[string]any{"version": w.approval.policyVersion, "manifest": ".devcontainer/devcontainer.json", "network": map[string]any{"mode": "offline"}}
	if w.approval.allow != nil {
		p["network"] = map[string]any{"mode": "allowlist", "allow": w.approval.allow}
	}
	if len(w.credentialApproval.grants) > 0 {
		p["credentials"] = w.credentialApproval.grants
	}
	return os.WriteFile(filepath.Join(w.doctor.checkout, "itos-container.yaml"), []byte(credentialApprovalEscape(string(approvalJSON(p)))), 0600)
}

func (w *world) credentialApprovalSettings() {
	grants := slices.Clone(w.credentialApproval.grants)
	if len(grants) == 0 {
		return
	}
	slices.SortFunc(grants, func(a, b credentialGrant) int { return strings.Compare(a.Kind, b.Kind) })
	for i := range grants {
		grants[i].Target = "/run/itos-container/credentials/" + grants[i].Kind
	}
	w.approval.settings["credentials"] = grants
	w.approval.settings["profile"] = strings.TrimSuffix(w.approval.settings["profile"].(string), "-v1") + "-credentials-v1"
}

func (w *world) credentialApprovalProfile(profile string) error {
	view, err := w.approvalView()
	if err != nil {
		return err
	}
	settings := object(view["effective"])
	if settings["profile"] != profile {
		return fmt.Errorf("credential profile differs")
	}
	var expected any
	_ = json.Unmarshal(approvalJSON(w.approval.settings["credentials"]), &expected)
	if !reflect.DeepEqual(settings["credentials"], expected) {
		return fmt.Errorf("credential grant set differs")
	}
	return nil
}

func (w *world) credentialApprovalReadOnly() error {
	if w.credentialApproval.accessError != nil {
		return w.credentialApproval.accessError
	}
	if err := w.approvalSameTree(); err != nil {
		return err
	}
	if err := w.approvalRecordUnchanged(); err != nil {
		return err
	}
	return w.doctorAllowedJournal(true)
}

func (w *world) credentialApprovalWatch() (func(), error) {
	if w.credentialApproval == nil {
		return func() {}, nil
	}
	fd, err := credentialWatchOpen()
	if err != nil {
		return nil, err
	}
	paths := slices.Clone(w.credentialApproval.observedInputs)
	for _, path := range w.credentials.paths {
		paths = append(paths, path)
	}
	for _, path := range paths {
		info, err := os.Lstat(path)
		if os.IsNotExist(err) || err == nil && !info.Mode().IsRegular() {
			continue
		}
		if err != nil {
			credentialWatchClose(fd)
			return nil, err
		}
		mode := info.Mode().Perm()
		if mode&0400 == 0 {
			if err := os.Chmod(path, mode|0400); err != nil {
				credentialWatchClose(fd)
				return nil, err
			}
		}
		_, watchErr := credentialWatchAdd(fd, path)
		if mode&0400 == 0 {
			watchErr = errors.Join(watchErr, os.Chmod(path, mode))
		}
		if watchErr != nil {
			credentialWatchClose(fd)
			return nil, watchErr
		}
	}
	control := filepath.Join(w.doctor.host, "slice-30-read-control")
	wd, err := credentialWatchAdd(fd, control)
	if err != nil {
		credentialWatchClose(fd)
		return nil, err
	}
	if _, err := os.ReadFile(control); err != nil {
		credentialWatchClose(fd)
		return nil, err
	}
	return func() {
		data, err := credentialWatchRead(fd)
		credentialWatchClose(fd)
		controlSeen := false
		for len(data) >= 16 {
			id := int(int32(binary.NativeEndian.Uint32(data)))
			n := 16 + int(binary.NativeEndian.Uint32(data[12:]))
			if n > len(data) {
				err = fmt.Errorf("truncated source observer event")
				break
			}
			if id != wd {
				err = fmt.Errorf("product opened or read credential source data")
			} else {
				controlSeen = true
			}
			data = data[n:]
		}
		if !controlSeen && err == nil {
			err = fmt.Errorf("source observer missed independent read control")
		}
		if err != nil {
			w.credentialApproval.accessError = err
		}
	}, nil
}
