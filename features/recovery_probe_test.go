package features

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

type recoveryProbe struct {
	process                                *recoveryProcess
	output, stdout, stderr, ready, release string
}

func (w *world) recoveryProbeStart(args ...string) (*recoveryProbe, error) {
	bin, err := w.sessionProbeBinary()
	if err != nil {
		return nil, err
	}
	prefix := filepath.Join(w.dir, "slice-11-probe-"+strconv.Itoa(len(w.recovery.processes)))
	p := &recoveryProbe{output: prefix + "-code", stdout: prefix + "-stdout", stderr: prefix + "-stderr", ready: prefix + "-ready", release: prefix + "-release"}
	request := map[string]any{"Args": args, "Fault": w.recoveryLocal.fault, "Boundary": w.recoveryLocal.boundary, "ID": w.recoveryLocal.id, "Ready": p.ready, "Release": p.release, "Output": p.output, "Stdout": p.stdout, "Stderr": p.stderr}
	if w.doctor.real == nil {
		request["LocalMount"] = w.doctor.checkout
	}
	input := prefix + "-input.json"
	if err = os.WriteFile(input, approvalJSON(request), 0600); err != nil {
		return nil, err
	}
	arguments := []string{"-test.run=^TestPublicRecoveryCommand$", "-test.count=1"}
	env := map[string]string{"SLICE_11_REQUEST": input}
	cover, collected := w.coverageDir(), ""
	if cover != "" {
		// testing.Main rereads every counter in its report directory. Keep
		// this traced invocation independent of sibling/outline counters;
		// closeRecoveryNative exports these bytes after joining the child.
		collected, err = os.MkdirTemp(cover, "recovery-probe-")
		if err != nil {
			return nil, err
		}
		env["GOCOVERDIR"] = collected
		arguments = append(arguments, "-test.gocoverdir="+collected)
	}
	p.process = w.recoveryStartConfigured(120*time.Second, bin, env, arguments...)
	p.process.coverageSource, p.process.coverageDestination = collected, cover
	return p, nil
}
func (w *world) recoveryProbeWait(p *recoveryProbe) error {
	p.process.wait()
	if err := p.process.exportCoverage(); err != nil {
		return err
	}
	if p.process.err != nil {
		return p.process.err
	}
	data, err := os.ReadFile(p.output)
	if err != nil {
		// A build without the command probe is an early refusal captured as
		// data. The positive behavior assertion, not fixture setup, must fail.
		w.recoveryLocal.results = append(w.recoveryLocal.results, doctorResult{exit: p.process.exit, stdout: string(p.process.out.bytes()), stderr: string(p.process.errout.bytes())})
		return nil
	}
	var code int
	if err = json.Unmarshal(data, &code); err != nil {
		return err
	}
	stdout, err := os.ReadFile(p.stdout)
	if err != nil {
		return err
	}
	stderr, err := os.ReadFile(p.stderr)
	if err != nil {
		return err
	}
	p.process.exit = code
	p.process.out, p.process.errout = offlineBuffer{}, offlineBuffer{}
	_, _ = p.process.out.Write(stdout)
	_, _ = p.process.errout.Write(stderr)
	w.recoveryLocal.processes = append(w.recoveryLocal.processes, p.process)
	w.recoveryLocal.results = append(w.recoveryLocal.results, doctorResult{exit: code, stdout: string(stdout), stderr: string(stderr)})
	return nil
}
func (w *world) recoveryProbeInvoke(args ...string) error {
	p, err := w.recoveryProbeStart(args...)
	if err != nil {
		return err
	}
	return w.recoveryProbeWait(p)
}
func (p *recoveryProbe) await() bool {
	timer := time.NewTimer(15 * time.Second)
	defer timer.Stop()
	tick := time.NewTicker(5 * time.Millisecond)
	defer tick.Stop()
	for {
		if data, err := os.ReadFile(p.ready); err == nil && string(data) == "ready" {
			return true
		}
		select {
		case <-p.process.done:
			return false
		case <-timer.C:
			return false
		case <-tick.C:
		}
	}
}
func (p *recoveryProbe) resume() error { return os.WriteFile(p.release, []byte("continue"), 0600) }
