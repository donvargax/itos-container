package features

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync/atomic"

	"github.com/cucumber/godog"
)

type policyWorld struct {
	host, recorder, cwd string
	env                 map[string]string
	listener            net.Listener
	done                chan struct{}
	connections         atomic.Int64
	snapshot            map[string]string
}

func (w *world) policySteps(sc *godog.ScenarioContext) {
	sc.Step("^policy inspection has isolated host state and a recording engine endpoint$", w.isolatePolicy)
	sc.Step("^the policy file contains:$", func(doc *godog.DocString) error {
		return os.WriteFile(filepath.Join(w.dir, "itos-container.yaml"), []byte(doc.Content), 0o600)
	})
	sc.Step("^the policy file is empty$", func() error {
		return os.WriteFile(filepath.Join(w.dir, "itos-container.yaml"), nil, 0o600)
	})
	sc.Step(`^the core configuration environment names "([^"]*)"$`, func(name string) {
		w.policy.env["ITOS_CONFIG"] = filepath.Join(w.dir, name)
	})
	sc.Step(`^the command working directory is "([^"]*)"$`, func(name string) {
		w.policy.cwd = filepath.Join(w.dir, name)
	})
	sc.Step("^its policy JSON contains this effective policy:$", w.effectivePolicy)
	sc.Step("^its policy JSON reports exactly these problems:$", func(table *godog.Table) error {
		return w.policyProblems(table, true)
	})
	sc.Step("^its doctor JSON reports exactly these policy problems:$", func(table *godog.Table) error {
		return w.policyProblems(table, false)
	})
	sc.Step("^the engine recorder has no connections$", func() error {
		// Closing joins the accept loop so an already queued connection is observed.
		if err := w.policy.listener.Close(); err != nil {
			return err
		}
		<-w.policy.done
		if w.policy.connections.Load() != 0 {
			return fmt.Errorf("inspection contacted the engine recorder")
		}
		return nil
	})
	sc.Step("^inspection helper sentinels are installed$", w.installPolicySentinels)
	sc.Step("^no inspection helper sentinel was executed$", func() error {
		entries, err := os.ReadDir(w.policy.recorder)
		if err != nil {
			return err
		}
		if len(entries) != 0 {
			return fmt.Errorf("inspection executed a helper: %v", entries)
		}
		return nil
	})
	sc.Step("^the policy inspection files are snapshotted$", func() error {
		var err error
		w.policy.snapshot, err = w.policyFiles()
		return err
	})
	sc.Step("^the policy inspection files are unchanged$", func() error {
		actual, err := w.policyFiles()
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(actual, w.policy.snapshot) {
			return fmt.Errorf("inspection changed checkout or host-state files")
		}
		return nil
	})
}

func (w *world) isolatePolicy() error {
	p := &policyWorld{cwd: w.dir, env: map[string]string{}, done: make(chan struct{})}
	w.policy = p
	var err error
	p.host, err = os.MkdirTemp("", "slice-1-host-")
	if err != nil {
		return err
	}
	p.recorder, err = os.MkdirTemp("", "slice-1-recorder-")
	if err != nil {
		return err
	}
	for key, name := range map[string]string{
		"HOME": "home", "USERPROFILE": "home", "XDG_CONFIG_HOME": "config",
		"XDG_STATE_HOME": "state", "XDG_CACHE_HOME": "cache", "DOCKER_CONFIG": "docker",
	} {
		dir := filepath.Join(p.host, name)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, "sentinel"), []byte("preserve host state"), 0o600); err != nil {
			return err
		}
		p.env[key] = dir
	}
	p.listener, err = net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	p.env["DOCKER_HOST"] = "tcp://" + p.listener.Addr().String()
	go func() {
		defer close(p.done)
		for {
			conn, err := p.listener.Accept()
			if err != nil {
				return
			}
			p.connections.Add(1)
			_ = conn.Close()
		}
	}()
	return nil
}

func (w *world) closePolicy() {
	if w.policy == nil {
		return
	}
	if w.policy.listener != nil {
		_ = w.policy.listener.Close()
		<-w.policy.done
	}
	_ = os.RemoveAll(w.policy.host)
	_ = os.RemoveAll(w.policy.recorder)
	w.policy = nil
}

func (w *world) policyEnv(env []string) []string {
	if w.policy == nil {
		return env
	}
	env = slices.DeleteFunc(env, func(kv string) bool {
		key, _, _ := strings.Cut(kv, "=")
		return strings.HasPrefix(key, "DOCKER_") || strings.HasPrefix(key, "CONTAINER_")
	})
	for key, value := range w.policy.env {
		env = setEnv(env, key, value)
	}
	return env
}

func (w *world) effectivePolicy(doc *godog.DocString) error {
	object, err := w.json()
	if err != nil {
		return err
	}
	var expected any
	if err := json.Unmarshal([]byte(doc.Content), &expected); err != nil {
		return err
	}
	problems, array := object["problems"].([]any)
	if object["schema"] != float64(1) || object["ok"] != true || object["scope"] != "policy" ||
		!array || len(problems) != 0 || !reflect.DeepEqual(object["policy"], expected) {
		return fmt.Errorf("incorrect effective policy envelope\n%s", w.report())
	}
	return nil
}

func (w *world) policyProblems(table *godog.Table, policyOnly bool) error {
	object, err := w.json()
	if err != nil {
		return err
	}
	_, partial := object["policy"]
	if object["schema"] != float64(1) || object["ok"] != false || partial ||
		(policyOnly && object["scope"] != "policy") {
		return fmt.Errorf("incorrect policy failure envelope\n%s", w.report())
	}
	problems, ok := object["problems"].([]any)
	if !ok || len(problems) != len(table.Rows)-1 {
		return fmt.Errorf("incorrect number of policy problems\n%s", w.report())
	}
	var actual, expected []string
	for _, value := range problems {
		problem, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("problem is not an object")
		}
		for _, key := range []string{"rule", "message", "fix"} {
			if text, _ := problem[key].(string); strings.TrimSpace(text) == "" {
				return fmt.Errorf("problem lacks %s", key)
			}
		}
		path, pathOK := problem["path"].(string)
		exit, exitOK := problem["exit"].(float64)
		if problem["file"] != "itos-container.yaml" || !pathOK || !exitOK {
			return fmt.Errorf("problem lacks a flat location or exit")
		}
		actual = append(actual, fmt.Sprintf("%s|%s|%g", problem["rule"], path, exit))
	}
	for _, row := range table.Rows[1:] {
		expected = append(expected, row.Cells[0].Value+"|"+row.Cells[1].Value+"|"+row.Cells[2].Value)
	}
	slices.Sort(actual)
	slices.Sort(expected)
	if !reflect.DeepEqual(actual, expected) {
		return fmt.Errorf("policy problems %v, expected %v\n%s", actual, expected, w.report())
	}
	return nil
}

func (w *world) policyFiles() (map[string]string, error) {
	files := map[string]string{}
	for _, root := range []string{w.dir, w.policy.host} {
		err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			files[path] = info.Mode().String()
			if !entry.IsDir() {
				data, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				files[path] += string(data)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return files, nil
}

// Build a harmless executable for the test host, including Windows. A copied
// helper records its name; it never delegates to a host command or engine.
func (w *world) installPolicySentinels() error {
	bin := filepath.Join(w.policy.host, "bin")
	if err := os.Mkdir(bin, 0o700); err != nil {
		return err
	}
	source := filepath.Join(bin, "sentinel.go")
	code := `package main
import ("os"; "path/filepath")
func main() {
  path := filepath.Join(os.Getenv("SLICE_1_RECORDER"), filepath.Base(os.Args[0]))
  if err := os.WriteFile(path, []byte("executed"), 0600); err != nil { os.Exit(1) }
}
`
	if err := os.WriteFile(source, []byte(code), 0o600); err != nil {
		return err
	}
	target := filepath.Join(bin, "helper.exe")
	build := exec.Command("go", "build", "-o", target, source)
	if output, err := build.CombinedOutput(); err != nil {
		return fmt.Errorf("building inspection sentinel: %w: %s", err, output)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		return err
	}
	for _, name := range []string{"git", "docker", "devcontainer", "sh", "bash", "npm"} {
		for _, suffix := range []string{"", ".exe"} {
			if err := os.WriteFile(filepath.Join(bin, name+suffix), data, 0o700); err != nil {
				return err
			}
		}
	}
	w.policy.env["PATH"] = bin
	w.policy.env["SLICE_1_RECORDER"] = w.policy.recorder
	if err := os.WriteFile(filepath.Join(w.dir, "generate"), data, 0o700); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(w.dir, ".devcontainer"), 0o700); err != nil {
		return err
	}
	manifest := `{"initializeCommand":["sh","-c","generate"]}`
	return os.WriteFile(filepath.Join(w.dir, ".devcontainer", "devcontainer.json"), []byte(manifest), 0o600)
}
