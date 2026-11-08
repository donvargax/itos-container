package features

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/cucumber/godog"
)

type hardlinkNativeResult struct {
	result           doctorResult
	outside          hardlinkEntry
	outsideErr       error
	requests         []offlineRequest
	live             map[string]any
	id               string
	absent, released bool
	err              error
	tree             map[string]hardlinkEntry
}

func (w *world) hardlinkNativeSteps(sc *godog.ScenarioContext) {
	sc.Step(`^a CI-only hard-link checkout with a cached native worker and isolated seeded approval$`, func() error { return w.offlineFixture("hardlink") })
	sc.Step(`^the native hard-link write probe and independent file observations are prepared$`, w.hardlinkNativePrepare)
	sc.Step(`^run attempts the hard-link write probe, the fixture replaces the checkout link with an independent copy, and run retries the probe$`, w.hardlinkNativeTwice)
	sc.Step(`^the first native hard-link run reports "([^"]*)" with exit (\d+) before any worker creation or start$`, w.hardlinkNativeRefused)
	sc.Step(`^the outside file's bytes mode owner and inode stayed unchanged after each run$`, w.hardlinkNativeOutside)
	sc.Step(`^the second native hard-link run executes the probe as the approved UID and GID with only the checkout host mount$`, w.hardlinkNativeBoundary)
	sc.Step(`^the second probe writes through its in-checkout hard link and exits with code 0$`, w.hardlinkNativeEdits)
	sc.Step(`^the second run preserves those checkout edits and confirms full-ID worker absence despite the new in-checkout hard link$`, w.hardlinkNativeAbsent)
	sc.Step(`^neither run changes approval or unrelated resources or acquires an image$`, w.hardlinkNativePreserved)
}

func (w *world) hardlinkNativePrepare() error {
	if err := doctorRealGuard(); err != nil {
		return err
	}
	source := filepath.Join(w.dir, "slice-20-probe.go")
	if err := os.WriteFile(source, []byte(hardlinkProbe), 0600); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "build", "-trimpath", "-o", filepath.Join(w.doctor.checkout, "slice-20-probe"), source)
	cmd.Dir, cmd.Env = w.root, sessionChildEnvironment(os.Environ())
	for key, value := range map[string]string{"CGO_ENABLED": "0", "GOOS": "linux", "GOARCH": "amd64", "GOFLAGS": "", "GOWORK": "off"} {
		cmd.Env = setEnv(cmd.Env, key, value)
	}
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("compile trusted hard-link probe: %w\n%s", err, output)
	}
	if err := w.hardlinkCheckpoint(); err != nil {
		return err
	}
	var err error
	w.hardlinks.resources, err = w.doctor.real.snapshotResources()
	return err
}

// The probe overwrites through the opened inode, never by rename. It executes
// only inside the CI worker and leaves a second, wholly internal linked pair.
const hardlinkProbe = `package main
import ("fmt"; "io"; "os")
func must(err error) { if err != nil { fmt.Fprintln(os.Stderr, err); os.Exit(98) } }
func main() {
 if len(os.Args) != 2 { os.Exit(99) }
 path := os.Args[1]
 must(os.Chmod(path, 0600))
 f, err := os.OpenFile(path, os.O_WRONLY|os.O_TRUNC, 0); must(err)
 _, err = io.WriteString(f, "slice-20 overwritten through original inode\n"); must(err); must(f.Close())
 must(os.WriteFile("/workspace/slice-20-edit", []byte("initial"), 0600))
 must(os.Link("/workspace/slice-20-edit", "/workspace/slice-20-edit-alias"))
 f, err = os.OpenFile("/workspace/slice-20-edit-alias", os.O_WRONLY|os.O_TRUNC, 0); must(err)
 _, err = io.WriteString(f, "slice-20 linked edit\n"); must(err); must(f.Close())
 fmt.Printf("slice-20-ready %d %d\n", os.Geteuid(), os.Getegid())
 data, err := io.ReadAll(os.Stdin); must(err)
 if string(data) != "slice-20-release\n" { os.Exit(97) }
}
`

type hardlinkOutput struct {
	offlineBuffer
	once  sync.Once
	ready chan struct{}
}

func (b *hardlinkOutput) Write(p []byte) (int, error) {
	n, err := b.offlineBuffer.Write(p)
	if bytes.Contains(b.bytes(), []byte("slice-20-ready ")) {
		b.once.Do(func() { close(b.ready) })
	}
	return n, err
}

func (w *world) hardlinkNativeRun() (hardlinkNativeResult, error) {
	if err := doctorRealGuard(); err != nil {
		return hardlinkNativeResult{}, err
	}
	h, a := w.hardlinks, w.offline.api
	relative, err := filepath.Rel(w.doctor.checkout, h.path)
	if err != nil {
		return hardlinkNativeResult{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, w.bin, "run", "--", "/workspace/slice-20-probe", "/workspace/"+filepath.ToSlash(relative))
	cmd.Dir, cmd.Env = w.doctor.checkout, w.env()
	out := &hardlinkOutput{ready: make(chan struct{})}
	var stderr offlineBuffer
	cmd.Stdout, cmd.Stderr = out, &stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return hardlinkNativeResult{}, err
	}
	defer func() { _ = stdin.Close() }()
	a.mu.Lock()
	start := len(a.requests)
	a.mu.Unlock()
	if err := cmd.Start(); err != nil {
		return hardlinkNativeResult{}, err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	r := hardlinkNativeResult{}
	select {
	case err = <-done:
		done <- err
	case <-ctx.Done():
	case <-out.ready:
		a.mu.Lock()
		if len(a.created) > 0 {
			r.id = a.created[len(a.created)-1]
			r.live, r.err = a.inspect(r.id)
		}
		a.mu.Unlock()
		_, writeErr := io.WriteString(stdin, "slice-20-release\n")
		closeErr := stdin.Close()
		r.released = writeErr == nil && closeErr == nil
	}
	err = <-done
	r.result = doctorResult{stdout: string(out.bytes()), stderr: string(stderr.bytes()), exit: cmd.ProcessState.ExitCode()}
	var exit *exec.ExitError
	if err != nil && !errors.As(err, &exit) {
		r.err = errors.Join(r.err, err)
	}
	if ctx.Err() != nil {
		r.err = errors.Join(r.err, ctx.Err())
	}
	r.outside, r.outsideErr = hardlinkObserve(h.alias)
	r.tree, err = hardlinkTree(w.dir, "")
	r.err = errors.Join(r.err, err)
	a.mu.Lock()
	r.requests = append([]offlineRequest{}, a.requests[start:]...)
	if r.id != "" {
		r.absent = w.doctor.real.read("/containers/"+r.id+"/json", nil, 404) == nil
	}
	r.err = errors.Join(r.err, a.problem)
	a.mu.Unlock()
	return r, nil
}

func (w *world) hardlinkNativeTwice() error {
	for i := 0; i < 2; i++ {
		if i == 1 {
			if err := w.hardlinkCopy(); err != nil {
				return err
			}
		}
		r, err := w.hardlinkNativeRun()
		if err != nil {
			return err
		}
		w.hardlinks.native = append(w.hardlinks.native, r)
	}
	return nil
}
func (w *world) hardlinkNativeRefused(rule string, code int) error {
	if len(w.hardlinks.native) != 2 {
		return fmt.Errorf("missing native attempts")
	}
	r := w.hardlinks.native[0]
	if r.result.exit != code || r.result.stdout != "" || !strings.Contains(r.result.stderr, rule) {
		return fmt.Errorf("first native refusal missing: %+v", r.result)
	}
	if r.err != nil {
		return r.err
	}
	if len(r.requests) != 0 {
		return fmt.Errorf("shared checkout reached native engine: %v", r.requests)
	}
	return hardlinkEqual(w.hardlinks.before, r.tree)
}
func (w *world) hardlinkNativeOutside() error {
	for i, r := range w.hardlinks.native {
		if r.outsideErr != nil {
			return r.outsideErr
		}
		want := w.hardlinks.outside
		if i == 1 {
			want.Links = "1"
		}
		if r.outside != want {
			return fmt.Errorf("outside inode changed during native attempt %d", i+1)
		}
	}
	return nil
}
func (w *world) hardlinkNativeBoundary() error {
	r := w.hardlinks.native[1]
	if r.err != nil {
		return r.err
	}
	if r.live == nil || !r.released || r.id == "" || object(r.live["State"])["Running"] != true {
		return fmt.Errorf("independent retry did not reach live probe")
	}
	if err := w.offline.api.identity(r.live, r.id); err != nil {
		return err
	}
	if object(r.live["Config"])["User"] != w.approval.uid+":"+w.approval.gid || r.result.stdout != "slice-20-ready "+w.approval.uid+" "+w.approval.gid+"\n" {
		return fmt.Errorf("probe did not execute with approved effective identity")
	}
	mounts, ok := r.live["Mounts"].([]any)
	if !ok {
		return fmt.Errorf("missing native mount observations")
	}
	anchors := 0
	for _, entry := range mounts {
		m := object(entry)
		if m["Type"] == "volume" {
			anchors++
			name, _ := m["Name"].(string)
			if w.offline.api.anchorVolumes()[name] == nil || m["Driver"] != "local" || m["Destination"] != "/workspace" || m["RW"] != true {
				return fmt.Errorf("wrong checkout mount")
			}
		} else if m["Type"] != "tmpfs" {
			return fmt.Errorf("unexpected native mount")
		}
	}
	if anchors != 1 {
		return fmt.Errorf("worker lacks sole checkout host mount")
	}
	return w.hardlinkIdentity()
}
func (w *world) hardlinkNativeEdits() error {
	r := w.hardlinks.native[1]
	if r.result.exit != 0 || r.result.stderr != "" {
		return fmt.Errorf("probe failed: %+v", r.result)
	}
	target, err := hardlinkObserve(w.hardlinks.path)
	if err != nil {
		return err
	}
	if target.Data != "slice-20 overwritten through original inode\n" || target.Mode.Perm() != 0600 || target.Inode == w.hardlinks.outside.Inode || target.Inode != w.hardlinks.copy.Inode || target.Device != w.hardlinks.copy.Device {
		return fmt.Errorf("probe did not overwrite independent target")
	}
	first, err := hardlinkObserve(filepath.Join(w.doctor.checkout, "slice-20-edit"))
	if err != nil {
		return err
	}
	second, err := hardlinkObserve(filepath.Join(w.doctor.checkout, "slice-20-edit-alias"))
	if err != nil {
		return err
	}
	if first.Data != "slice-20 linked edit\n" || first != second || first.Links != "2" {
		return fmt.Errorf("probe did not write through retained internal alias")
	}
	return nil
}
func (w *world) hardlinkNativeAbsent() error {
	if err := w.hardlinkNativeEdits(); err != nil {
		return err
	}
	r := w.hardlinks.native[1]
	if !offlineHex(r.id) || !r.absent {
		return fmt.Errorf("full-ID native worker not independently absent")
	}
	record, err := sessionReadRecord(w.sessionRecordPath())
	if err != nil {
		return err
	}
	if !recoveryJSONEqual(record.Bytes, w.sessionExpectedRecord("absent", r.id)) {
		return fmt.Errorf("protected native cleanup not complete")
	}
	_, err = w.offlineCompletedRegistryPaths()
	return err
}
func (w *world) hardlinkNativePreserved() error {
	if err := w.approvalRecordUnchanged(); err != nil {
		return err
	}
	resources, err := w.doctor.real.snapshotResources()
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(resources, w.hardlinks.resources) {
		return fmt.Errorf("unrelated native resources changed")
	}
	for _, r := range w.hardlinks.native {
		for _, req := range r.requests {
			if strings.HasPrefix(req.Path, "/images/") && req.Method != "GET" || strings.HasPrefix(req.Path, "/build") {
				return fmt.Errorf("native image acquisition attempted")
			}
		}
	}
	return w.hardlinkNativeTreePreserved()
}

func (w *world) hardlinkNativeTreePreserved() error {
	h := w.hardlinks
	allowed, err := w.offlineCompletedRegistryPaths()
	if err != nil {
		return err
	}
	state := filepath.Join(w.doctor.env["XDG_STATE_HOME"], "itos-container")
	intent := filepath.Join(state, w.session.reference)
	for _, path := range []string{intent, filepath.Join(filepath.Dir(intent), ".lock"), w.sessionSnapshotPath(), filepath.Join(w.doctor.checkout, "slice-20-edit"), filepath.Join(w.doctor.checkout, "slice-20-edit-alias")} {
		allowed[path] = true
	}
	// Only ancestors of the exact validated additions can change directory
	// timestamps/link counts. Existing records still have to match byte for byte.
	parents := map[string]bool{filepath.Dir(h.path): true}
	for path := range allowed {
		for parent := filepath.Dir(path); parent != w.dir && strings.HasPrefix(parent, w.dir+string(os.PathSeparator)); parent = filepath.Dir(parent) {
			parents[parent], allowed[parent] = true, true
		}
	}
	got := h.native[1].tree
	for path, want := range h.before {
		actual, exists := got[path]
		if !exists {
			return fmt.Errorf("native run removed unrelated entry %s", path)
		}
		if path == h.path {
			want = h.copy
			want.Data, want.Mode, want.Modified = "slice-20 overwritten through original inode\n", 0600, actual.Modified
		}
		if path == h.alias {
			want.Links = "1"
		}
		if want.Mode.IsDir() && parents[path] {
			actual.Modified, actual.Links = want.Modified, want.Links
		}
		if actual != want {
			return fmt.Errorf("native run changed unrelated entry %s", path)
		}
	}
	for path := range got {
		if _, existed := h.before[path]; !existed && !allowed[path] {
			return fmt.Errorf("unexpected native addition %s", path)
		}
	}
	return nil
}
