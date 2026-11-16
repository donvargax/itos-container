package features

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"time"
)

type homeNativeReport struct {
	UID, GID, RootUID, RootGID                             int
	Home, Tmp, Mode                                        string
	Writable, HomeBefore, TmpBefore, TmpExec, HostReadable bool
	Initial, Errors                                        []string
}

type homeNativeRun struct {
	exit        int
	out, errout []byte
	err         error
	report      homeNativeReport
	worker      map[string]any
	home        map[string]any
}

type homeNative struct {
	sentinels, sentinel, scope string
	hostBefore, checkoutBefore map[string]runEntry
	resourcesBefore            map[string]string
	runs                       []homeNativeRun
	workers, volumes, anchors  map[string]map[string]any
}

func (w *world) homeNativeFixture() error {
	// Guard before fixture allocation, engine input lookup or transport creation.
	if err := doctorRealGuard(); err != nil {
		return err
	}
	if err := w.sessionFixture(true, "0022"); err != nil {
		return err
	}
	if os.Geteuid() == 0 {
		return errors.New("HOME-13 requires a non-root controller")
	}
	f := &homeNative{workers: map[string]map[string]any{}, volumes: map[string]map[string]any{}, anchors: map[string]map[string]any{}}
	w.home = &homeWorld{native: f}
	w.doctor.env["DOCKER_HOST"] = "unix:///var/run/docker.sock"
	if err := w.installNativeProbe("slice-34-probe", func(path string) error {
		return recoveryBuildHelper(w.root, path, homeNativeProbeSource)
	}); err != nil {
		return err
	}
	// doctorCheckout selected the test assembly, whose account-home port is
	// isolated independently from the HOME and XDG selections below.
	f.sentinels = filepath.Join(w.doctor.host, "slice-34-host-home")
	f.sentinel = filepath.Join(f.sentinels, "private-session")
	if err := os.MkdirAll(f.sentinels, 0700); err != nil {
		return err
	}
	if err := os.WriteFile(f.sentinel, []byte("synthetic host-home sentinel"), 0600); err != nil {
		return err
	}
	w.doctor.env["HOME"] = f.sentinels
	if err := os.WriteFile(filepath.Join(w.doctor.checkout, "itos-container.yaml"), []byte("version: 1\nmanifest: .devcontainer/devcontainer.json\nhome: {storage: volume}\n"), 0600); err != nil {
		return err
	}
	if err := w.approvalManifest(); err != nil {
		return err
	}
	if err := w.approvalSeed("1", false); err != nil {
		return err
	}
	a := w.approval
	a.settings["profile"] = "offline-home-v1"
	a.settings["home"] = map[string]any{"storage": "volume", "target": "/home/itos", "lifetime": "project", "mode": "0700"}
	a.fingerprint = "sha256:" + approvalHash(a.settings)
	if err := os.WriteFile(a.record, approvalJSON(map[string]any{"schema": 1, "project": a.project, "generation": "1", "fingerprint": a.fingerprint, "settings": a.settings}), 0600); err != nil {
		return err
	}
	if err := w.seedControlLocations(); err != nil {
		return err
	}
	f.scope = approvalHash(map[string]any{"schema": 1, "project": a.project, "uid": a.uid, "gid": a.gid, "engine": map[string]any{"endpoint": w.doctor.env["DOCKER_HOST"], "id": w.doctor.engineID}})
	var err error
	f.hostBefore, err = runTree(f.sentinels)
	if err == nil {
		f.checkoutBefore, err = runTree(w.doctor.checkout)
	}
	if err == nil {
		f.resourcesBefore, err = w.doctor.real.snapshotResources()
	}
	return err
}

func (w *world) homeNativeRuns() error {
	for _, mode := range []string{"write", "read"} {
		run := w.homeNativeRun(mode)
		w.home.native.runs = append(w.home.native.runs, run)
	}
	return nil // early refusal and a missing report are observations for Then
}

func (w *world) homeNativeRun(mode string) homeNativeRun {
	run := homeNativeRun{exit: -1}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, w.bin, "run", "--", "/workspace/slice-34-probe", mode, w.home.native.sentinel)
	cmd.Dir, cmd.Env = w.doctor.checkout, w.env()
	var out, errout offlineBuffer
	cmd.Stdout, cmd.Stderr = &out, &errout
	stdin, err := cmd.StdinPipe()
	if err != nil {
		run.err = err
		return run
	}
	defer func() { _ = stdin.Close() }()
	if err := cmd.Start(); err != nil {
		run.err = err
		return run
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	tick := time.NewTicker(5 * time.Millisecond)
	defer tick.Stop()
	finished := false
waiting:
	for {
		select {
		case err = <-done:
			finished = true
			break waiting
		case <-ctx.Done():
			break waiting
		case <-tick.C:
			if json.Unmarshal(out.bytes(), &run.report) == nil {
				// The probe waits for EOF, so this observation precedes exit.
				run.worker, run.home, run.err = w.homeNativeObserve()
				break waiting
			}
		}
	}
	_ = stdin.Close()
	if !finished {
		err = <-done
	}
	run.out, run.errout = out.bytes(), errout.bytes()
	run.exit = cmd.ProcessState.ExitCode()
	var exited *exec.ExitError
	if err != nil && !errors.As(err, &exited) {
		run.err = errors.Join(run.err, err)
	}
	if ctx.Err() != nil {
		run.err = errors.Join(run.err, errors.New("home product run exceeded its fixture deadline"))
	}
	// Track retained volumes even if a failure preceded the worker's report.
	_, _, observationErr := w.homeNativeObserve()
	run.err = errors.Join(run.err, observationErr)
	return run
}

func (w *world) homeNativeObserve() (map[string]any, map[string]any, error) {
	f, real := w.home.native, w.doctor.real
	var workers []map[string]any
	if err := real.read("/containers/json?all=1", &workers, 200); err != nil {
		return nil, nil, err
	}
	var selected map[string]any
	for _, worker := range workers {
		labels := object(worker["Labels"])
		if labels["io.itos-container.project"] != w.approval.project || labels["io.itos-container.fingerprint"] != w.approval.fingerprint {
			continue
		}
		id, _ := worker["Id"].(string)
		if !offlineHex(id) {
			return nil, nil, errors.New("native home worker has no full ID")
		}
		var observed map[string]any
		if err := real.read("/containers/"+id+"/json", &observed, 200); err != nil {
			return nil, nil, err
		}
		f.workers[id] = observed
		if selected != nil {
			return nil, nil, errors.New("more than one home worker exists")
		}
		selected = observed
	}
	var catalog struct{ Volumes []map[string]any }
	if err := real.read("/volumes", &catalog, 200); err != nil {
		return selected, nil, err
	}
	var home map[string]any
	for _, volume := range catalog.Volumes {
		labels := object(volume["Labels"])
		if labels["io.itos-container.project"] == w.approval.project && labels["io.itos-container.fingerprint"] == w.approval.fingerprint && labels["io.itos-container.kind"] == "checkout-anchor" {
			name, _ := volume["Name"].(string)
			if token := strings.TrimPrefix(name, "itos-container-anchor-"); offlineHex(token) && labels["io.itos-container.intent"] == token {
				var observed map[string]any
				if err := real.read("/volumes/"+name, &observed, 200); err != nil {
					return selected, nil, err
				}
				f.anchors[name] = observed
			}
		}
		if labels["io.itos-container.scope"] != f.scope || labels["io.itos-container.kind"] != "harness-home" {
			continue
		}
		name, _ := volume["Name"].(string)
		token := strings.TrimPrefix(name, "itos-container-home-")
		if !offlineHex(token) || name != "itos-container-home-"+token || labels["io.itos-container.token"] != token || len(labels) != 3 {
			return selected, nil, errors.New("native home volume has unexpected identity labels")
		}
		var observed map[string]any
		if err := real.read("/volumes/"+name, &observed, 200); err != nil {
			return selected, nil, err
		}
		if prior := f.volumes[name]; prior != nil && !homeNativeSameVolume(prior, observed) {
			return selected, nil, errors.New("native home volume was substituted")
		}
		f.volumes[name] = observed
		home = observed
	}
	return selected, home, nil
}

func homeNativeSameVolume(a, b map[string]any) bool {
	if a == nil || b == nil {
		return false
	}
	for _, field := range []string{"Name", "Driver", "Scope", "CreatedAt", "Options", "Labels"} {
		if !reflect.DeepEqual(a[field], b[field]) {
			return false
		}
	}
	return true
}

func (w *world) homeNativePersistence() error {
	f := w.home.native
	if len(f.runs) != 2 {
		return errors.New("both home runs were not observed")
	}
	for i, run := range f.runs {
		if run.err != nil {
			return run.err
		}
		r := run.report
		if run.exit != 0 || r.Home != "/home/itos" || r.Tmp != "/tmp" || !r.Writable || r.Mode != "drwx------" ||
			strconv.Itoa(r.UID) != w.approval.uid || strconv.Itoa(r.GID) != w.approval.gid || r.RootUID != r.UID || r.RootGID != r.GID ||
			(i == 0 && (r.HomeBefore || len(r.Initial) != 0)) || (i == 1 && !r.HomeBefore) || len(r.Errors) != 0 {
			return fmt.Errorf("run %d did not provide the approved persistent private home: exit %d, report %s, diagnostic %s", i+1, run.exit, approvalJSON(r), run.errout)
		}
	}
	return nil
}

func (w *world) closeHomeNative() error {
	if w.home == nil || w.home.native == nil || w.doctor == nil || w.doctor.real == nil {
		return nil
	}
	if err := doctorRealGuard(); err != nil {
		return err
	}
	// Only independently observed full IDs/tuples are eligible for harness
	// teardown. Assertions run first; this contributes no product cleanup proof.
	var result error
	for id, prior := range w.home.native.workers {
		var current map[string]any
		if err := w.doctor.real.read("/containers/"+id+"/json", &current, 200); err != nil {
			if absent := w.doctor.real.read("/containers/"+id+"/json", nil, 404); absent != nil {
				result = errors.Join(result, err)
			}
			continue
		}
		if current["Id"] != id || !reflect.DeepEqual(object(current["Config"])["Labels"], object(prior["Config"])["Labels"]) {
			result = errors.Join(result, errors.New("harness refuses substituted home worker"))
			continue
		}
		result = errors.Join(result, w.homeNativeRemove("/containers/"+id+"?force=true"))
	}
	for name, prior := range w.home.native.volumes {
		var current map[string]any
		if err := w.doctor.real.read("/volumes/"+name, &current, 200); err != nil {
			result = errors.Join(result, err)
			continue
		}
		if !homeNativeSameVolume(prior, current) {
			result = errors.Join(result, errors.New("harness refuses substituted home volume"))
			continue
		}
		result = errors.Join(result, w.homeNativeRemove("/volumes/"+name))
	}
	for name, prior := range w.home.native.anchors {
		var current map[string]any
		if err := w.doctor.real.read("/volumes/"+name, &current, 200); err != nil {
			if absent := w.doctor.real.read("/volumes/"+name, nil, 404); absent != nil {
				result = errors.Join(result, err)
			}
			continue
		}
		if !homeNativeSameVolume(prior, current) {
			result = errors.Join(result, errors.New("harness refuses substituted home checkout anchor"))
			continue
		}
		result = errors.Join(result, w.homeNativeRemove("/volumes/"+name))
	}
	return result
}

func (w *world) homeNativeRemove(path string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	r, err := http.NewRequestWithContext(ctx, http.MethodDelete, "http://engine"+path, nil)
	if err != nil {
		return err
	}
	response, err := w.doctor.real.client.Do(r)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 65536))
	if response.StatusCode != 204 {
		return fmt.Errorf("home harness teardown returned HTTP %d", response.StatusCode)
	}
	return nil
}
