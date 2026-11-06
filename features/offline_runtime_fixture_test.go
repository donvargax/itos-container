package features

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"time"
)

type offlineRuntime struct {
	api                    *offlineAPI
	mode                   string
	arguments, argv        []string
	before, after          map[string]string
	resourcesBefore        map[string]string
	resourcesAfter         map[string]string
	stdout, stderr         []byte
	exit                   int
	handshake, inputClosed bool
	err                    error
}

const offlineAPP = "$(touch env-marker);$HOST_ONLY"

var (
	offlineReadyOut = []byte("slice-10:stdout-ready\x00\xff\n")
	offlineReadyErr = []byte("slice-10:stderr-ready\x00\xfe\n")
	offlineInput    = []byte("slice-10:input\x00\xfd\n")
	offlineTailOut  = []byte("slice-10:stdout-tail\x00\xfc\n")
	offlineTailErr  = []byte("slice-10:stderr-tail\x00\xfb\n")
)

func (w *world) offlineFixture(mode string) error {
	// Guard before even allocating a fixture or reading native-engine inputs.
	if err := doctorRealGuard(); err != nil {
		return err
	}
	if err := w.sessionFixture(true, "0022"); err != nil {
		return err
	}
	w.offline = &offlineRuntime{mode: mode, exit: -1}
	if err := w.startOfflineAPI(); err != nil {
		return err
	}
	if mode == "shell" {
		w.doctor.manifest["workspaceFolder"] = "/workspace/subdir"
		w.doctor.manifest["containerEnv"] = map[string]any{"APP": offlineAPP}
		if err := os.Mkdir(filepath.Join(w.doctor.checkout, "subdir"), 0700); err != nil {
			return err
		}
	}
	if err := w.approvalManifest(); err != nil {
		return err
	}
	if err := w.buildOfflineProbe(); err != nil {
		return err
	}
	// Shell glob candidates and marker names make interpretation observable.
	for _, name := range []string{"a", "bb", "pattern-a"} {
		if err := os.WriteFile(filepath.Join(w.doctor.checkout, name), []byte("fixture"), 0600); err != nil {
			return err
		}
	}
	// Independent canonical data after every setup edit and endpoint selection;
	// neither approve nor run is invoked to manufacture expected settings.
	if err := w.approvalSeed("1", false); err != nil {
		return err
	}
	return w.offlineCheckpoint()
}

func (w *world) offlineCheckpoint() error {
	var err error
	w.offline.before, err = w.offlineProtected()
	if err == nil {
		w.offline.resourcesBefore, err = w.doctor.real.snapshotResources()
	}
	return err
}

func (w *world) offlineProtected() (map[string]string, error) {
	result := map[string]string{}
	state := filepath.Join(w.doctor.env["XDG_STATE_HOME"], "itos-container")
	for _, root := range []string{w.doctor.checkout, w.doctor.host} {
		tree, err := doctorTree(root)
		if err != nil {
			return nil, err
		}
		for path, value := range tree {
			// These are authorized product additions, checked separately. Keep
			// every pre-existing sibling, approval and image reference in the audit.
			if path == filepath.Join(w.doctor.checkout, "subdir", "edit.txt") ||
				path == filepath.Join(state, "intents") || strings.HasPrefix(path, filepath.Join(state, "intents", w.approval.project)+string(os.PathSeparator)) || path == filepath.Join(state, "intents", w.approval.project) ||
				path == filepath.Join(state, "snapshots") || strings.HasPrefix(path, filepath.Join(state, "snapshots", w.approval.project)+string(os.PathSeparator)) || path == filepath.Join(state, "snapshots", w.approval.project) {
				continue
			}
			result[path] = value
		}
	}
	return result, nil
}

// Buffers are observed while exec's independent stdout/stderr drains are live.
// Readiness on just one stream cannot release stdin.
type offlineBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (b *offlineBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Write(p)
}
func (b *offlineBuffer) bytes() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return bytes.Clone(b.b.Bytes())
}

func (w *world) offlineExecute(argv []string, handshake bool) error {
	o := w.offline
	o.argv = append([]string{}, argv...)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, w.bin, append([]string{"run", "--"}, argv...)...)
	cmd.Dir, cmd.Env = w.doctor.checkout, w.env()
	for key, value := range w.doctor.env {
		cmd.Env = setEnv(cmd.Env, key, value)
	}
	cmd.Env = setEnv(cmd.Env, "ITOS_ROOT", w.doctor.checkout)
	var out, errout offlineBuffer
	cmd.Stdout, cmd.Stderr = &out, &errout
	stdin, err := cmd.StdinPipe()
	if err != nil {
		o.err = err
		return nil
	}
	defer func() { _ = stdin.Close() }()
	if err = cmd.Start(); err != nil {
		o.err = err
		return nil
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	if handshake {
		ticker := time.NewTicker(5 * time.Millisecond)
		defer ticker.Stop()
	waiting:
		for {
			select {
			case err = <-done:
				done <- err // early refusal is data, never a missing-barrier setup failure
				break waiting
			case <-ctx.Done():
				break waiting
			case <-ticker.C:
				if bytes.Contains(out.bytes(), offlineReadyOut) && bytes.Contains(errout.bytes(), offlineReadyErr) {
					// Independent native state, not kill(pid, 0), proves that
					// the workload is still running when both frames arrive.
					o.handshake = o.api.running()
					if o.handshake {
						_, err = stdin.Write(offlineInput)
						o.inputClosed = err == nil && stdin.Close() == nil
					}
					break waiting
				}
			}
		}
	} else {
		_ = stdin.Close()
	}
	err = <-done
	o.stdout, o.stderr = out.bytes(), errout.bytes()
	o.exit = 0
	if err != nil {
		var status *exec.ExitError
		if errors.As(err, &status) {
			o.exit = status.ExitCode()
		} else {
			o.err = err
		}
	}
	if ctx.Err() != nil {
		o.err = fmt.Errorf("offline product invocation exceeded its fixture deadline")
	}
	// Record absence before any harness cleanup; closeOfflineRuntime never
	// supplies evidence to these observations.
	o.err = errors.Join(o.err, o.api.observeAfter())
	o.after, err = w.offlineProtected()
	o.err = errors.Join(o.err, err)
	o.resourcesAfter, err = w.doctor.real.snapshotResources()
	o.err = errors.Join(o.err, err)
	return nil
}

func (w *world) closeOfflineRuntime() error {
	if w.offline == nil || w.offline.api == nil {
		return nil
	}
	a := w.offline.api
	// End hijacked transports before removing any surviving, independently
	// identified worker. This evidence is deliberately not added to the journal.
	err := a.server.Close()
	a.closeConnections()
	return errors.Join(err, a.cleanup())
}

func (w *world) offlineUnrelated() error {
	o := w.offline
	if o.before == nil || o.after == nil || !reflect.DeepEqual(o.before, o.after) {
		return fmt.Errorf("offline run changed unrelated checkout or host fixture files")
	}
	if o.resourcesBefore == nil || o.resourcesAfter == nil || !reflect.DeepEqual(o.resourcesBefore, o.resourcesAfter) {
		return fmt.Errorf("offline run changed unrelated engine resources or image references")
	}
	return nil
}
