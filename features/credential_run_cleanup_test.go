package features

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"
)

// Failure cleanup happens only after the scenario's observations. The fresh
// stop uses the protected project and session identity, including when a run
// exited before the probe and the harness never observed a worker ID.
func (f *credentialRunFixture) cleanupProject(p *credentialRunProject) error {
	if len(p.runs) == 0 {
		return nil
	}
	for _, run := range p.runs {
		if run.clean {
			continue
		}
		present, err := f.cleanupNeeded(p, run)
		if err != nil {
			return err
		}
		if !present {
			continue
		}
		id := run.session
		if id == "" {
			id, err = p.cleanupSession()
			if err != nil {
				return err
			}
		}
		result := p.w.homeNativeInvoke("stop", id)
		if result.err != nil || result.exit != 0 {
			return errors.New("fixture could not clean its protected credential run")
		}
		present, err = f.cleanupNeeded(p, run)
		if err != nil || present {
			return errors.New("credential run fixture still owns engine or staging resources")
		}
	}
	return nil
}

func (f *credentialRunFixture) cleanupNeeded(p *credentialRunProject, run *credentialRunProcess) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	owned, err := p.engine(f.cli).owned(ctx)
	if err != nil || len(owned) != 0 {
		return len(owned) != 0, err
	}
	if credentialStageFile(run.staging) {
		_, err := os.Lstat(filepath.Dir(run.staging))
		if !os.IsNotExist(err) {
			return err == nil, err
		}
	}
	return false, nil
}

// With no live report, resolve one protected stop candidate from this
// checkout. A missing or ambiguous identity leaves the fixture for diagnosis.
func (p *credentialRunProject) cleanupSession() (string, error) {
	status := p.w.homeNativeInvoke("status", "--json")
	entries, _ := credentialRunObject(status.out)["sessions"].([]any)
	id := ""
	for _, entry := range entries {
		session := object(entry)
		if session["project"] != p.w.approval.project || session["stop_candidate"] != true {
			continue
		}
		candidate, _ := session["id"].(string)
		if id != "" || !offlineHex(candidate) {
			return "", errors.New("fixture cleanup has no unique protected session")
		}
		id = candidate
	}
	if status.err != nil || status.exit != 0 || id == "" {
		return "", errors.New("fixture cleanup could not locate its protected session")
	}
	return id, nil
}
