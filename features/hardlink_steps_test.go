package features

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

func (w *world) hardlinkSteps(sc *godog.ScenarioContext) {
	sc.Step(`^the hard-link fixture links "([^"]*)" to an "([^"]*)" name with mode "([^"]*)"$`, w.hardlinkLink)
	sc.Step(`^an independent hard-link fixture file at "([^"]*)" with mode "([^"]*)"$`, w.hardlinkFile)
	sc.Step(`^the hard-link fixture files are checkpointed$`, w.hardlinkCheckpoint)
	sc.Step(`^the hard-link fixture hides a linked file below an unreadable subtree$`, w.hardlinkHidden)
	sc.Step(`^the hard-link fixture invokes "([^"]*)" with valid arguments$`, w.hardlinkInvoke)
	sc.Step(`^the hard-link command reports "([^"]*)" with exit (\d+)$`, w.hardlinkProblem)
	sc.Step(`^the hard-link diagnostic names the checkout and explains independent file inodes$`, w.hardlinkDiagnostic)
	sc.Step(`^the hard-link observations prove the two names shared an inode before invocation$`, w.hardlinkShared)
	sc.Step(`^the hard-link fixture trees and link relationships are unchanged$`, w.hardlinkUnchanged)
	sc.Step(`^the hard-link unreadable subtree and outside alias were not repaired or changed$`, w.hardlinkHiddenUnchanged)
	sc.Step(`^the hard-link fixture host state is unchanged$`, w.hardlinkHostUnchanged)
	sc.Step(`^doctor inspects the hard-link checkout, the fixture replaces its linked entry with an independent copy, and doctor inspects again$`, w.hardlinkTwoDoctors)
	sc.Step(`^the first hard-link inspection reports "([^"]*)" with exit (\d+)$`, w.hardlinkFirstProblem)
	sc.Step(`^the second hard-link inspection is ready with the original matching approval and fingerprint$`, func() error {
		if err := w.hardlinkResult(1); err != nil {
			return err
		}
		return errors.Join(w.doctorReady(), w.approvalCandidate(), w.approvalStatus("matching"))
	})
	sc.Step(`^only the second hard-link inspection contacted the engine and used inspection requests only$`, func() error {
		h := w.hardlinks
		if len(h.connections) != 2 || h.connections[0] != 0 || h.connections[1] == 0 || len(h.journals[0]) != 0 {
			return fmt.Errorf("wrong inspection connections: %v", h.connections)
		}
		return w.runJournal(h.journals[1], "complete", true)
	})
	sc.Step(`^the hard-link copy has identical initial bytes and mode but a different inode and one link$`, w.hardlinkCopyIndependent)
	sc.Step(`^the hard-link outside file and approval are unchanged apart from the fixture's removed alias$`, w.hardlinkOutsideAfterCopy)
	sc.Step(`^terminal approve shows its review, the hard-link fixture links "([^"]*)" outside, and then answers yes$`, w.hardlinkReview)
	sc.Step(`^the hard-link action reached the review before adding the alias$`, w.hardlinkReached)
	sc.Step(`^"([^"]*)" runs and the hard-link fixture performs "([^"]*)" before releasing its image response$`, w.hardlinkImageRace)
	sc.Step(`^the hard-link action reached the image barrier before installing the shared inode$`, w.hardlinkReached)
	sc.Step(`^the hard-link observations retain the original checkout and Git directory identities$`, w.hardlinkIdentity)
	sc.Step(`^run reaches owned attachment and the hard-link fixture links "([^"]*)" outside before releasing attachment$`, w.hardlinkAttachRace)
	sc.Step(`^run refuses with "([^"]*)" and exit (\d+) without any worker start request$`, w.hardlinkNoStart)
	sc.Step(`^the hard-link action reached attachment after durable full-ID ownership$`, func() error { return errors.Join(w.hardlinkReached(), w.hardlinks.ownedErr) })
	sc.Step(`^the unstarted full-ID worker is removed with confirmed absence and completed protected cleanup$`, w.hardlinkCleanup)
	sc.Step(`^the hard-link checkout and outside file are unchanged apart from the fixture's added alias$`, w.hardlinkCheckoutUnchanged)
	sc.Step(`^the original approval and unrelated resources are unchanged$`, func() error { return errors.Join(w.approvalRecordUnchanged(), w.doctorNoMarker()) })
	w.hardlinkNativeSteps(sc)
}

func (w *world) hardlinkInvoke(command string) error {
	h := w.hardlinks
	h.command = command
	args := []string{command, "--json"}
	switch command {
	case "doctor":
	case "approve":
		args = append(args, "--fingerprint", w.approval.fingerprint)
	case "run":
		args = []string{"run", "--", "/bin/true"}
	default:
		return fmt.Errorf("unknown hard-link command %q", command)
	}
	return w.runPreflightProcess(w.env(), args...)
}

func (w *world) hardlinkProblem(rule string, code int) error {
	if w.hardlinks.command == "run" {
		return w.runProblem(rule, code, "checkout path")
	}
	if w.hardlinks.command == "approve" {
		return w.approvalProblem(rule, code, "")
	}
	return w.doctorProblem(rule, code)
}

func (w *world) hardlinkDiagnostic() error {
	text := w.stderr
	if w.stdout != "" {
		obj, err := w.doctorJSON()
		if err != nil {
			return err
		}
		problems, _ := obj["problems"].([]any)
		if len(problems) != 1 {
			return fmt.Errorf("expected one hard-link problem")
		}
		p := object(problems[0])
		if p["path"] != w.doctor.checkout {
			return fmt.Errorf("diagnostic lacks safe root subject")
		}
		text = fmt.Sprint(p["message"], " ", p["fix"], " ", p["path"])
	}
	for _, word := range []string{w.doctor.checkout, "hard link", "independent", "inode"} {
		if !strings.Contains(text, word) {
			return fmt.Errorf("hard-link diagnostic lacks %q: %s", word, text)
		}
	}
	if strings.Contains(text, "chmod") || strings.Contains(text, "itos container approve") {
		return fmt.Errorf("diagnostic offers ineffective repair: %s", text)
	}
	return nil
}

func (w *world) hardlinkTwoDoctors() error {
	h := w.hardlinks
	for i := 0; i < 2; i++ {
		if i == 1 {
			if err := w.hardlinkCopy(); err != nil {
				return err
			}
		}
		count, n := w.doctor.engine.listener.connections.Load(), len(w.doctor.engine.requests())
		if err := w.runDoctor(w.env(), "doctor", "--json"); err != nil {
			return err
		}
		h.results = append(h.results, doctorResult{stdout: w.stdout, stderr: w.stderr, exit: w.exit})
		h.connections = append(h.connections, w.doctor.engine.listener.connections.Load()-count)
		h.journals = append(h.journals, w.doctor.engine.requests()[n:])
	}
	return nil
}

func (w *world) hardlinkResult(index int) error {
	if len(w.hardlinks.results) <= index {
		return fmt.Errorf("missing inspection %d", index)
	}
	r := w.hardlinks.results[index]
	w.stdout, w.stderr, w.exit = r.stdout, r.stderr, r.exit
	return nil
}
func (w *world) hardlinkFirstProblem(rule string, code int) error {
	if err := w.hardlinkResult(0); err != nil {
		return err
	}
	return w.doctorProblem(rule, code)
}
func (w *world) hardlinkReached() error {
	if !w.hardlinks.reached {
		return fmt.Errorf("command did not reach the fixture barrier")
	}
	return w.hardlinkShared()
}

func (w *world) hardlinkReview(path string) error {
	w.approval.mode = "plain"
	p, err := w.startApproval("plain", "", "yes")
	if err != nil {
		return err
	}
	if strings.Contains(p.review, "y/N") {
		w.hardlinks.reached = true
		if w.hardlinks.path != filepath.Join(w.doctor.checkout, path) {
			p.cancel()
			w.finishApproval(p)
			return fmt.Errorf("wrong reviewed fixture path")
		}
		err = w.hardlinkAlias("outside")
		if err == nil {
			err = w.hardlinkCheckpoint()
		}
	}
	if err != nil {
		p.cancel()
	} else {
		p.answer("yes")
	}
	w.finishApproval(p)
	return err
}

func (w *world) hardlinkImageRace(command, edit string) error {
	h := w.hardlinks
	h.command = command
	w.doctor.engine.paused = true
	args := []string{"doctor", "--json"}
	if command == "run" {
		args = []string{"run", "--", "/bin/true"}
	}
	p, err := w.startRun(w.env(), args...)
	if err != nil {
		return err
	}
	select {
	case <-p.done:
	case <-w.doctor.engine.reached:
		h.reached = true
		if edit == "replace with linked inode" {
			// The original independent inode remains allocated until replacement, so
			// this edit cannot silently reuse its inode number.
			replacement := h.path + ".slice-20-new"
			err = writeManifestFixture(replacement, []byte(h.initial.Data), h.initial.Mode.Perm())
			if err == nil {
				err = os.Rename(replacement, h.path)
			}
		} else if edit != "add alias" {
			err = fmt.Errorf("unknown link edit %q", edit)
		}
		if err == nil {
			err = w.hardlinkAlias("outside")
		}
		if err == nil {
			err = w.hardlinkCheckpoint()
		}
	case <-time.After(6 * time.Second):
	}
	w.doctor.engine.unpause()
	return errors.Join(err, w.hardlinkFinish(p))
}
func (w *world) hardlinkFinish(p *runProcess) error {
	r, err := w.finishRun(p)
	w.run.results = append(w.run.results, r)
	w.stdout, w.stderr, w.exit = r.result.stdout, r.result.stderr, r.result.exit
	return err
}

func (w *world) hardlinkAttachRace(path string) error {
	h, a := w.hardlinks, w.doctor.engine.offline
	reached, release := make(chan struct{}), make(chan struct{})
	a.attachBarrier = func(token, id string) {
		h.ownedErr = a.record(token, "owned", id)
		if !offlineHex(id) {
			h.ownedErr = fmt.Errorf("attachment lacks full ID")
		}
		close(reached)
		<-release
	}
	p, err := w.startRun(w.env(), "run", "--", "/bin/true")
	if err != nil {
		close(release)
		return err
	}
	select {
	case <-p.done:
	case <-reached:
		h.reached = true
		if h.path != filepath.Join(w.doctor.checkout, path) {
			err = fmt.Errorf("wrong attachment fixture path")
		} else {
			err = w.hardlinkAlias("outside")
		}
		if err == nil {
			err = w.hardlinkCheckpoint()
		}
	case <-time.After(6 * time.Second):
	}
	close(release)
	return errors.Join(err, w.hardlinkFinish(p))
}

func (w *world) hardlinkNoStart(rule string, code int) error {
	if w.exit != code || w.stdout != "" || !strings.Contains(w.stderr, rule) {
		return fmt.Errorf("expected late refusal %s: %s", rule, w.report())
	}
	for _, r := range w.doctor.engine.requests() {
		if r.Method == "POST" && strings.HasSuffix(doctorRequestPath(r.Path), "/start") {
			return fmt.Errorf("worker start attempted")
		}
	}
	return nil
}

func (w *world) hardlinkCleanup() error {
	a := w.doctor.engine.offline
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.problem != nil {
		return a.problem
	}
	if len(a.created) != 1 || len(a.started) != 0 || len(a.resources) != 0 {
		return fmt.Errorf("unstarted owned worker not cleaned")
	}
	for id, r := range a.created {
		if !offlineHex(id) || !a.removed[id] {
			return fmt.Errorf("missing full-ID removal")
		}
		token, _ := object(object(r["Config"])["Labels"])["io.itos-container.intent"].(string)
		if err := a.record(token, "absent", id); err != nil {
			return err
		}
		removed, confirmed := false, false
		for _, request := range w.doctor.engine.requests() {
			path := doctorRequestPath(request.Path)
			if request.Method == "DELETE" && path == "/containers/"+id {
				removed = true
			}
			if removed && request.Method == "GET" && path == "/containers/"+id+"/json" {
				confirmed = true
			}
		}
		if !confirmed {
			return fmt.Errorf("worker absence not confirmed after removal")
		}
	}
	tree, err := runTree(w.dir)
	if err != nil {
		return err
	}
	root := filepath.Join(w.doctor.env["XDG_STATE_HOME"], "itos-container", "sessions", w.recoveryIndexKey())
	count := 0
	for path, e := range tree {
		if filepath.Base(path) != "record.json" || filepath.Dir(filepath.Dir(path)) != root {
			continue
		}
		count++
		var record map[string]any
		if err := json.Unmarshal([]byte(e.Data), &record); err != nil {
			return err
		}
		if record["state"] != "stopped" || record["workload_exit"] != nil || record["last_problem"] != nil || record["project"] != w.approval.project || !offlineHex(fmt.Sprint(record["id"])) {
			return fmt.Errorf("late refusal left incomplete protected registration: %v", record)
		}
	}
	if count != 1 {
		return fmt.Errorf("missing completed public registration")
	}
	return nil
}

func (w *world) hardlinkCheckoutUnchanged() error {
	for _, root := range []string{w.doctor.checkout, filepath.Dir(w.hardlinks.alias)} {
		got, err := hardlinkTree(root, "")
		if err != nil {
			return err
		}
		want := map[string]hardlinkEntry{}
		for path, e := range w.hardlinks.before {
			if path == root || strings.HasPrefix(path, root+string(os.PathSeparator)) {
				want[path] = e
			}
		}
		if err := hardlinkEqual(want, got); err != nil {
			return err
		}
	}
	return nil
}
