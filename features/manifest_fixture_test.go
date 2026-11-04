package features

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
)

func (w *world) manifestCheckout() error {
	m := &manifestWorld{checkout: filepath.Join(w.dir, "checkout"), host: filepath.Join(w.dir, "host"), recorder: filepath.Join(w.dir, "recorder"), env: map[string]string{}}
	w.manifest = m
	for _, dir := range []string{m.checkout, m.host, m.recorder} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
	}
	for key, name := range map[string]string{"HOME": "home", "USERPROFILE": "home", "XDG_CONFIG_HOME": "config", "XDG_STATE_HOME": "state", "XDG_CACHE_HOME": "cache", "DOCKER_CONFIG": "docker"} {
		dir := filepath.Join(m.host, name)
		if err := writeManifestFixture(filepath.Join(dir, "sentinel"), []byte("preserve host state"), 0o600); err != nil {
			return err
		}
		m.env[key] = dir
	}
	if err := w.selectManifest(".devcontainer/devcontainer.json"); err != nil {
		return err
	}
	return w.manifestText(manifestBytes)
}

func (w *world) manifestEnv(env []string) []string {
	if w.manifest == nil {
		return env
	}
	prefixes := []string{"DOCKER_", "CONTAINER_", "XDG_", "SSH_", "AWS_", "AZURE_", "GOOGLE_", "GCLOUD_", "CLAUDE_", "ANTHROPIC_", "OPENAI_", "OPENCODE_", "SLICE_2_"}
	env = slices.DeleteFunc(env, func(kv string) bool {
		key, _, _ := strings.Cut(kv, "=")
		for _, prefix := range prefixes {
			if strings.HasPrefix(strings.ToUpper(key), prefix) {
				return true
			}
		}
		return false
	})
	for key, value := range w.manifest.env {
		env = setEnv(env, key, value)
	}
	return env
}

func (w *world) manifestLayout(layout string) error {
	path := w.manifestPath()
	if err := os.Remove(path); err != nil {
		return err
	}
	switch layout {
	case "directory at the selected candidate":
		return os.Mkdir(path, 0o700)
	case "FIFO at the selected candidate":
		return manifestFIFO(path)
	case ".devcontainer directory symlink", "dangling .devcontainer directory symlink":
		path = filepath.Dir(path)
		if err := os.Remove(path); err != nil {
			return err
		}
		target := filepath.Join(w.manifest.checkout, "linked-manifest-directory")
		if layout == ".devcontainer directory symlink" {
			if err := writeManifestFixture(filepath.Join(target, "devcontainer.json"), []byte(manifestBytes), 0o600); err != nil {
				return err
			}
		}
		return os.Symlink(target, path)
	case "file symlink to an in-checkout file", "file symlink outside the checkout", "dangling file symlink":
		target := filepath.Join(w.manifest.checkout, "linked-manifest.json")
		if layout == "file symlink outside the checkout" {
			target = filepath.Join(w.manifest.host, "linked-manifest.json")
		}
		if layout != "dangling file symlink" {
			if err := writeManifestFixture(target, []byte(manifestBytes), 0o600); err != nil {
				return err
			}
		}
		return os.Symlink(target, path)
	default:
		return fmt.Errorf("unknown manifest layout %q", layout)
	}
}

func (w *world) workspaceLayout(entry, layout string) error {
	path := filepath.Join(w.manifest.checkout, entry)
	if layout == "regular file" {
		return writeManifestFixture(path, []byte("not a directory"), 0o600)
	}
	first, rest, intermediate := strings.Cut(entry, "/")
	target := filepath.Join(w.manifest.checkout, "workspace-target")
	if strings.Contains(layout, "outside the checkout") {
		target = filepath.Join(w.manifest.host, "workspace-target")
	}
	if layout != "dangling symlink" {
		if err := os.MkdirAll(filepath.Join(target, rest), 0o700); err != nil {
			return err
		}
	}
	if intermediate {
		path = filepath.Join(w.manifest.checkout, first)
	}
	return os.Symlink(target, path)
}

// Record no-follow entries, including symlink targets and special-file modes.
// In particular, a snapshot never opens the FIFO whose refusal it will check.
func (w *world) manifestFiles() (map[string]string, error) {
	files := map[string]string{}
	for _, root := range []string{w.manifest.checkout, w.manifest.host} {
		err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			data := ""
			if info.Mode().IsRegular() {
				bytes, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				data = string(bytes)
			} else if info.Mode()&os.ModeSymlink != 0 {
				data, err = os.Readlink(path)
				if err != nil {
					return err
				}
			}
			files[path] = info.Mode().String() + ":" + data
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return files, nil
}

func (w *world) manifestSentinels() error {
	// Compile trusted test material once per feature run, outside snapshotted
	// directories. The copied programs record only; they never delegate.
	target := w.bin + ".slice-2-sentinel.exe"
	data, err := os.ReadFile(target)
	if os.IsNotExist(err) {
		source := w.bin + ".slice-2-sentinel.go"
		code := `package main
import ("os"; "path/filepath")
func main() {
  if err := os.WriteFile(filepath.Join(os.Getenv("SLICE_2_RECORDER"), filepath.Base(os.Args[0])), []byte("executed"), 0600); err != nil { os.Exit(1) }
}`
		if err := os.WriteFile(source, []byte(code), 0o600); err != nil {
			return err
		}
		cmd := exec.Command("go", "build", "-buildvcs=false", "-o", target, source)
		cmd.Dir = w.root
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("build manifest sentinel: %w: %s", err, output)
		}
		data, err = os.ReadFile(target)
	}
	if err != nil {
		return err
	}
	bin := filepath.Join(w.manifest.host, "bin")
	for _, name := range []string{"docker", "devcontainer", "git", "sh", "bash", "npm"} {
		for _, suffix := range []string{"", ".exe"} {
			if err := writeManifestFixture(filepath.Join(bin, name+suffix), data, 0o700); err != nil {
				return err
			}
		}
	}
	for _, name := range []string{"slice-2-record-execution", "slice-2-record-execution.exe", ".git/hooks/post-checkout", ".git/hooks/pre-commit"} {
		if err := writeManifestFixture(filepath.Join(w.manifest.checkout, name), data, 0o700); err != nil {
			return err
		}
	}
	if err := writeManifestFixture(filepath.Join(w.manifest.checkout, ".vscode/tasks.json"), []byte(`{"tasks":[{"label":"sentinel","type":"shell","command":"./slice-2-record-execution"}]}`), 0o600); err != nil {
		return err
	}
	w.manifest.env["PATH"], w.manifest.env["SLICE_2_RECORDER"] = bin, w.manifest.recorder
	w.manifest.snapshot, err = w.manifestFiles()
	return err
}

func (w *world) manifestUnchanged() error {
	files, err := w.manifestFiles()
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(files, w.manifest.snapshot) {
		return fmt.Errorf("manifest inspection changed checkout or host state")
	}
	entries, err := os.ReadDir(w.manifest.recorder)
	if err != nil {
		return err
	}
	if len(entries) != 0 {
		return fmt.Errorf("manifest inspection executed a helper: %v", entries)
	}
	return nil
}

func (w *world) runManifestContract(mode string) error {
	w.manifest.metadata = true
	bin := w.bin + ".slice-2-contract.exe"
	if _, err := os.Stat(bin); os.IsNotExist(err) {
		cmd := exec.Command("go", "test", "-c", "-o", bin, "./cmd/itos-container")
		cmd.Dir = w.root
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("build manifest contract assembly: %w: %s", err, output)
		}
	} else if err != nil {
		return err
	}
	manifest, err := os.ReadFile(w.manifestPath())
	if err != nil {
		return err
	}
	data, err := json.Marshal(struct {
		Path     string
		Manifest string
		Label    *string
		JSON     bool
	}{w.manifest.selected, string(manifest), w.manifest.label, mode == "JSON"})
	if err != nil {
		return err
	}
	w.stdin = string(data)
	defer func() { w.stdin = "" }()
	return w.runEnv(w.manifest.checkout, setEnv(w.env(), "SLICE_2_CONTRACT", "1"), bin, "-test.run=^TestManifestContractProcess$")
}
