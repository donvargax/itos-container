package features

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func (w *world) sessionProbe(action, reference string, checkpoint func() error) (sessionResult, error) {
	var result sessionResult
	binary, err := w.sessionProbeBinary()
	if err != nil {
		return result, err
	}
	s := w.session
	var watch *sessionReadWatch
	if (action == "cleanup" || action == "run-cleanup") && s.change != "" {
		watch, err = w.sessionWatchInputs()
		if err != nil {
			return result, err
		}
	}
	prefix := filepath.Join(w.dir, "slice-9-"+action)
	output, selected := prefix+"-result.json", prefix+"-selected.json"
	for _, path := range []string{output, selected} {
		if err := os.WriteFile(path, nil, 0600); err != nil {
			return result, err
		}
	}
	input := prefix + "-input.json"
	request := map[string]string{"Action": action, "Backend": s.backend, "Reference": reference, "Token": s.token, "Fault": s.fault, "Umask": s.umask, "Output": output, "Checkpoint": selected, "Journal": s.journal}
	request["Boundary"] = s.boundary
	request["AnchorFault"], request["AnchorFixture"] = s.anchorFault, strconv.FormatBool(s.anchorFixture)
	if s.launch != nil {
		request["LaunchBarrier"], request["ReplacedPath"] = s.launch.barrier, s.launch.replaced
		request["CleanupCause"], request["ChildExit"] = s.launch.cause, strconv.Itoa(s.launch.child)
		request["Registered"] = strconv.FormatBool(s.launch.registered)
		if err := os.WriteFile(selected+"-barrier", nil, 0600); err != nil {
			return result, err
		}
	}
	request["MemoryFile"] = prefix + "-memory.json"
	if s.backend == "memory" && (action == "cleanup" || action == "run-cleanup") {
		if err := w.writeMemorySeed(request["MemoryFile"]); err != nil {
			return result, err
		}
	}
	if action == "cleanup" || action == "run-cleanup" {
		delete(request, "Token")
	}
	if err := os.WriteFile(input, approvalJSON(request), 0600); err != nil {
		return result, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()
	args := []string{"-test.run=^TestSessionContractProbe$", "-test.count=1"}
	if cover := w.coverageDir(); cover != "" {
		args = append(args, "-test.gocoverdir="+cover)
	}
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Dir = w.doctor.checkout
	cmd.Env = w.coverageEnv(sessionChildEnvironment(os.Environ()))
	for key, value := range w.doctor.env {
		cmd.Env = setEnv(cmd.Env, key, value)
	}
	cmd.Env = setEnv(cmd.Env, "ITOS_ROOT", w.doctor.checkout)
	cmd.Env = setEnv(cmd.Env, "SLICE_9_PROBE_INPUT", input)
	cmd.Env = setEnv(cmd.Env, "SLICE_9_SYNTHETIC_PRIVATE", doctorCanary)
	var transcript bytes.Buffer
	cmd.Stdout, cmd.Stderr = &transcript, &transcript
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return result, err
	}
	defer func() { _ = stdin.Close() }()
	if err := w.startLocalCommand(cmd); err != nil {
		return result, err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	if action == "create" || action == "run" {
		// Wait for either the observed checkpoint OR process exit. An unavailable
		// or refused product cannot turn a missing call into a hung fixture.
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		waiting := true
		for waiting {
			select {
			case err = <-done:
				done <- err
				waiting = false
			case <-ctx.Done():
				err = <-done
				done <- err
				waiting = false
			case <-ticker.C:
				data, readErr := os.ReadFile(selected)
				if readErr != nil || !json.Valid(data) {
					continue
				}
				if checkpoint != nil {
					if err = checkpoint(); err != nil {
						cancel()
						<-done
						return result, err
					}
				}
				if s.backend == "memory" {
					if seedErr := w.writeMemorySeed(request["MemoryFile"]); seedErr != nil {
						cancel()
						<-done
						return result, seedErr
					}
				}
				if s.change != "" || (s.launch != nil && s.launch.watch) {
					watch, err = w.sessionWatchInputs()
					if err != nil {
						cancel()
						<-done
						return result, err
					}
				}
				if err = json.NewEncoder(stdin).Encode("continue"); err != nil {
					cancel()
					<-done
					return result, err
				}
				waiting = false
			}
		}
	}
	if s.launch == nil {
		err = <-done
	} else {
		err = w.offlineAwaitBarrier(ctx, cancel, done, selected+"-barrier", stdin)
	}
	if watch != nil {
		reads, watchErr := watch.finish()
		s.selectedReads = append(s.selectedReads, reads...)
		if watchErr != nil {
			return result, watchErr
		}
	}
	// Synchronize with the recorder after the controller has exited, including
	// the final handler whose response let that controller finish.
	s.api.mu.Lock()
	observerErr := s.api.problem
	s.api.mu.Unlock()
	s.transcript += transcript.String()
	if observerErr != nil {
		return result, observerErr
	}
	if err != nil {
		return result, fmt.Errorf("session contract subprocess failed: %w\n%s", err, transcript.String())
	}
	data, err := os.ReadFile(output)
	if err != nil {
		return result, err
	}
	err = json.Unmarshal(data, &result)
	if err == nil && s.backend == "memory" {
		err = w.observeMemory(result.Memory)
		if err != nil && s.offline {
			err = fmt.Errorf("%w (exit %d; %s; %s)", err, result.Code, result.Diagnostic, result.PreflightFailure)
		}
	}
	return result, err
}

// sessionProbeBinary is the session contract assembly. It always compiles the
// checkout, so it keeps the production limits even in the engine tier, which the
// engine rows of the session contract rely on (see features/README.md).
func (w *world) sessionProbeBinary() (string, error) {
	return w.probeAssembly(w.root, sessionProbeSuffix)
}

// productProbeBinary is the assembly a scenario world runs as its product, with
// the account home its fixture isolates. In the engine tier it is compiled from
// the fixture tree beside the suite's product, like that product, so the world's
// native waits are the shortened ones; elsewhere it is the session assembly.
func (w *world) productProbeBinary() (string, error) {
	if source := productSource(w.root, w.bin); source != w.root {
		return w.probeAssembly(source, engineProbeSuffix)
	}
	return w.sessionProbeBinary()
}

func (w *world) probeAssembly(source, suffix string) (string, error) {
	path := filepath.Join(w.dir, "slice-9-contract-probe")
	// The suite binary lives outside every scenario's writable checkout and
	// survives their teardown. Reuse only the executable; requests, processes,
	// state, journals and coverage destinations remain scenario-specific.
	if filepath.IsAbs(w.bin) {
		path = probeProduct(w.bin) + suffix
	}
	covered := productCoverageRequested() || w.coverdir != ""
	if covered {
		path += "-cover"
	}
	// Portable manifest/approval fixtures also use this assembly. Their suite
	// path need not have an extension, while Windows executable lookup needs one.
	path += ".exe"
	return featureBinary(path, func(target string) error {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "go", probeBuildArgs(target, covered)...)
		cmd.Dir, cmd.Env = source, sessionChildEnvironment(os.Environ())
		data, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("build session test assembly: %w\n%s", err, data)
		}
		return nil
	})
}

// probeBuildArgs compiles the test assembly the way the suite's product is
// built. -trimpath keeps the source folder out of every compiled package's cache
// key, so the assembly reuses the packages that product build (and the other
// assembly) already compiled instead of compiling the whole module again. Without
// it each cold covered assembly costs about as much as the product build itself;
// the engine tier compiles two of them inside its measured step.
func probeBuildArgs(target string, covered bool) []string {
	args := []string{"test", "-c", "-trimpath", "./cmd/itos-container", "-o", target}
	if covered {
		args = append(args, "-cover", "-covermode=atomic", "-coverpkg=./...")
	}
	return args
}

func sessionChildEnvironment(env []string) []string {
	out := make([]string, 0, len(env))
	for _, value := range env {
		if strings.HasPrefix(value, "ITOS_CONTAINER_RED_SCENARIOS=") || strings.HasPrefix(value, "ITOS_CONTAINER_ENGINE_") || strings.HasPrefix(value, "DOCKER_") {
			continue
		}
		out = append(out, value)
	}
	return out
}
