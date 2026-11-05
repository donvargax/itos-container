package features

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
)

type approvalWorld struct {
	settings                                 map[string]any
	prior                                    map[string]any
	project, fingerprint, record, generation string
	before                                   map[string]string
	recordBefore                             []byte
	recordInfo                               os.FileInfo
	results                                  []doctorResult
	beforeReview                             string
	mode, condition, uid, gid, location      string
	contract                                 map[string]any
	secondRoot, firstRecord                  string
	firstBytes                               []byte
	literal                                  string
}

func approvalJSON(value any) []byte { b, _ := json.Marshal(value); return b }
func approvalHash(value any) string {
	sum := sha256.Sum256(approvalJSON(value))
	return hex.EncodeToString(sum[:])
}
func approvalClone(value map[string]any) map[string]any {
	var out map[string]any
	_ = json.Unmarshal(approvalJSON(value), &out)
	return out
}

func (w *world) approvalCheckout() error {
	if err := w.doctorCheckout(); err != nil {
		return err
	}
	if err := w.doctorUsers(strconv.Itoa(os.Geteuid()), strconv.Itoa(os.Geteuid())); err != nil {
		return err
	}
	if err := w.doctorLocalAPI(false); err != nil {
		return err
	}
	w.approval = &approvalWorld{uid: strconv.Itoa(os.Geteuid()), gid: strconv.Itoa(os.Getegid())}
	for _, key := range []string{"USER", "LOGNAME", "UID", "GID", "SUDO_UID", "SUDO_GID"} {
		w.doctor.env[key] = ""
	}
	return w.approvalOracle()
}

// This oracle uses only fixture inputs and independent filesystem observations.
// It never calls the product's canonicalizer or requires a successful doctor.
func (w *world) approvalOracle() error {
	a, d := w.approval, w.doctor
	checkout, err := doctorCheckoutIdentity(d.checkout)
	if err != nil {
		return err
	}
	name := d.manifest["name"]
	workspace, ok := d.manifest["workspaceFolder"]
	if !ok {
		workspace = "/workspace"
	}
	env, ok := d.manifest["containerEnv"]
	if !ok {
		env = map[string]any{}
	}
	path := ".devcontainer/devcontainer.json"
	data, err := os.ReadFile(filepath.Join(d.checkout, "itos-container.yaml"))
	if err != nil {
		return err
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "manifest:") {
			path = strings.TrimSpace(strings.TrimPrefix(line, "manifest:"))
		}
	}
	d.engine.mu.Lock()
	id := d.engine.info["ID"]
	imageID := d.engine.image["Id"]
	d.engine.mu.Unlock()
	endpoint := d.env["DOCKER_HOST"]
	a.settings = map[string]any{"schema": 1, "profile": "offline-v1", "host": map[string]any{"uid": a.uid, "gid": a.gid}, "checkout": checkout,
		"engine":   map[string]any{"endpoint": endpoint, "id": id, "os": "linux", "architecture": "amd64"},
		"policy":   map[string]any{"version": 1, "manifest": path, "network": map[string]any{"mode": "offline"}},
		"manifest": map[string]any{"name": name, "image": d.manifest["image"], "workspaceFolder": workspace, "containerEnv": env, "containerUser": a.uid, "remoteUser": a.uid},
		"image":    map[string]any{"reference": d.manifest["image"], "id": imageID, "os": "linux", "architecture": "amd64"}}
	a.fingerprint = "sha256:" + approvalHash(a.settings)
	a.project = approvalHash(map[string]any{"schema": 1, "uid": a.uid, "checkout": checkout})
	state := d.env["XDG_STATE_HOME"]
	if state == "" {
		state = filepath.Join(d.env["HOME"], ".local/state")
	}
	a.record = filepath.Join(state, "itos-container", "approvals", a.project, "current.json")
	return nil
}

func (w *world) approvalSeed(generation string, changed bool) error {
	if err := w.approvalOracle(); err != nil {
		return err
	}
	a := w.approval
	settings := approvalClone(a.settings)
	if changed {
		m := settings["manifest"].(map[string]any)
		m["containerEnv"] = map[string]any{"APP": "old"}
		m["workspaceFolder"] = "/workspace/old"
	}
	record := map[string]any{"schema": 1, "project": a.project, "generation": generation, "fingerprint": "sha256:" + approvalHash(settings), "settings": settings}
	if err := writeManifestFixture(a.record, approvalJSON(record), 0600); err != nil {
		return err
	}
	a.prior = settings
	a.generation = generation
	return w.approvalCapture()
}

func (w *world) approvalCapture() error {
	a := w.approval
	var err error
	a.before, err = doctorTree(w.dir)
	if err != nil {
		return err
	}
	a.recordBefore, _ = os.ReadFile(a.record)
	a.recordInfo, _ = os.Stat(a.record)
	return nil
}
func (w *world) approvalManifest() error {
	return os.WriteFile(filepath.Join(w.doctor.checkout, ".devcontainer/devcontainer.json"), approvalJSON(w.doctor.manifest), 0600)
}
func (w *world) approvalState(state string) error {
	switch state {
	case "missing":
		return nil
	case "matching":
		return w.approvalSeed("1", false)
	case "changed", "historical A with active B":
		return w.approvalSeed("2", true)
	case "invalid":
		return w.approvalDefect("truncated JSON")
	case "matching only in the checkout":
		if err := w.approvalSeed("1", false); err != nil {
			return err
		}
		if err := os.Rename(filepath.Dir(filepath.Dir(filepath.Dir(w.approval.record))), filepath.Join(w.doctor.checkout, "itos-container")); err != nil {
			return err
		}
		return w.approvalCapture()
	default:
		return fmt.Errorf("unknown state %q", state)
	}
}
func (w *world) approvalDefect(defect string) error {
	if err := w.approvalSeed("1", false); err != nil {
		return err
	}
	a := w.approval
	data := a.recordBefore
	var obj map[string]any
	_ = json.Unmarshal(data, &obj)
	switch defect {
	case "truncated JSON":
		data = []byte(`{"schema":`)
	case "duplicate decoded member":
		data = append([]byte(`{"sch\u0065ma":1,`), data[1:]...)
	case "unknown schema or member":
		obj["extra"] = true
		data = approvalJSON(obj)
	case "noncanonical JSON bytes":
		data = append(data, '\n')
	case "invalid generation":
		obj["generation"] = "01"
		data = approvalJSON(obj)
	case "fingerprint does not hash settings":
		obj["fingerprint"] = "sha256:" + strings.Repeat("0", 64)
		data = approvalJSON(obj)
	case "project does not hash host and checkout":
		obj["project"] = strings.Repeat("0", 64)
		data = approvalJSON(obj)
	case "settings belong to another effective UID":
		s := obj["settings"].(map[string]any)
		s["host"].(map[string]any)["uid"] = "999999"
		obj["fingerprint"] = "sha256:" + approvalHash(s)
		data = approvalJSON(obj)
	case "record larger than 1 MiB":
		data = []byte(strings.Repeat(" ", 1024*1024+1))
	case "record deeper than 32 levels":
		data = []byte(strings.Repeat("[", 33) + "0" + strings.Repeat("]", 33))
	default:
		return fmt.Errorf("unknown defect %q", defect)
	}
	if err := os.WriteFile(a.record, data, 0600); err != nil {
		return err
	}
	return w.approvalCapture()
}

func (w *world) approvalEdit(edit string) error {
	d := w.doctor
	switch edit {
	case "reorder YAML and JSON keys and add comments and whitespace":
		if err := os.WriteFile(filepath.Join(d.checkout, "itos-container.yaml"), []byte("# request\nnetwork: {mode: offline}\nmanifest: .devcontainer/devcontainer.json\nversion: 1\n"), 0600); err != nil {
			return err
		}
		data := approvalJSON(d.manifest)
		return os.WriteFile(filepath.Join(d.checkout, ".devcontainer/devcontainer.json"), append([]byte("// comment\n"), data...), 0600)
	case "spell default workspace and empty environment explicitly":
		d.manifest["workspaceFolder"] = "/workspace"
		d.manifest["containerEnv"] = map[string]string{}
	case "omit both manifest users that equal the effective host UID":
		delete(d.manifest, "containerUser")
		delete(d.manifest, "remoteUser")
	case "change JSON string escaping without changing decoded values":
		data := strings.ReplaceAll(string(approvalJSON(d.manifest)), "worker", `\u0077orker`)
		return os.WriteFile(filepath.Join(d.checkout, ".devcontainer/devcontainer.json"), []byte(data), 0600)
	case "change unused editor and accepted image-label display data":
		d.manifest["customizations"] = map[string]any{"vscode": map[string]any{}}
		d.engine.config()["Labels"] = map[string]string{"devcontainer.metadata": `[{"name":"display"}]`}
	case "change Git HEAD, origin and an ordinary source file":
		for name, data := range map[string]string{".git/HEAD": "ref: refs/heads/other\n", ".git/config": "[core]\nbare=false\n[remote \"origin\"]\nurl=example.test/repo\n", "source.go": "package fixture\n"} {
			if err := os.WriteFile(filepath.Join(d.checkout, name), []byte(data), 0600); err != nil {
				return err
			}
		}
		return nil
	case "negotiate another supported Engine API version":
		d.engine.version["ApiVersion"] = "1.55"
	case "change the literal application environment", "the manifest environment":
		d.manifest["containerEnv"] = map[string]string{"APP": "new value"}
	case "select an existing nested workspace":
		d.manifest["workspaceFolder"] = "/workspace/nested"
		if err := os.Mkdir(filepath.Join(d.checkout, "nested"), 0700); err != nil {
			return err
		}
	case "change the manifest display name":
		d.manifest["name"] = "quoted \\\" <>&\u2028"
	case "select an identical manifest at another path", "the policy manifest path":
		if err := writeManifestFixture(filepath.Join(d.checkout, "other.json"), approvalJSON(d.manifest), 0600); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(d.checkout, "itos-container.yaml"), []byte("version: 1\nmanifest: other.json\nnetwork: {mode: offline}\n"), 0600)
	case "request and inspect another pinned image digest":
		d.image = "worker.test/agent@sha256:" + strings.Repeat("c", 64)
		d.manifest["image"] = d.image
		d.engine.expectedImage = d.image
		d.engine.image["RepoDigests"] = []string{d.image}
	case "observe another config digest for the image", "the selected image config ID":
		d.engine.image["Id"] = "sha256:" + strings.Repeat("c", 64)
	case "observe another daemon at the same endpoint", "daemon ID":
		d.engine.info["ID"] = "engine-B"
	case "select another outside socket for the same daemon":
		old := d.engine
		if err := w.doctorLocalAPI(false); err != nil {
			return err
		}
		d.cleanup = append(d.cleanup, old.close)
	case "socket device/inode":
		return d.engine.replaceSocket()
	case "checkout root":
		old := d.checkout + "-old"
		if err := os.Rename(d.checkout, old); err != nil {
			return err
		}
		if err := w.writeDoctorCheckout(d.checkout); err != nil {
			return err
		}
		d.env["ITOS_ROOT"] = d.checkout
		d.replacement = d.checkout
		var err error
		d.replacementSnapshot, err = doctorTree(d.checkout)
		return err
	case "embedded .git":
		if err := os.Rename(filepath.Join(d.checkout, ".git"), filepath.Join(w.dir, "git-old")); err != nil {
			return err
		}
		if err := w.writeDoctorCheckout(d.checkout); err != nil {
			return err
		}
		d.replacement = d.checkout
		var err error
		d.replacementSnapshot, err = doctorTree(d.checkout)
		return err
	default:
		return fmt.Errorf("unknown edit %q", edit)
	}
	return w.approvalManifest()
}

func (w *world) approvalUnsafe(layout string) error {
	if err := w.approvalSeed("1", false); err != nil {
		return err
	}
	a, d := w.approval, w.doctor
	managed := filepath.Join(d.env["XDG_STATE_HOME"], "itos-container")
	target := filepath.Join(w.dir, "outside-target")
	switch layout {
	case "XDG_STATE_HOME beneath the checkout":
		d.env["XDG_STATE_HOME"] = filepath.Join(d.checkout, "state")
	case "a symlinked state ancestor":
		if err := os.Rename(d.env["XDG_STATE_HOME"], target); err != nil {
			return err
		}
		if err := os.Symlink(target, d.env["XDG_STATE_HOME"]); err != nil {
			return err
		}
	case "itos-container is a symlink", "approvals project directory is a symlink", "current.json is a symlink":
		path := managed
		if layout == "approvals project directory is a symlink" {
			path = filepath.Dir(a.record)
		}
		if layout == "current.json is a symlink" {
			path = a.record
		}
		if err := os.Rename(path, target); err != nil {
			return err
		}
		if err := os.Symlink(target, path); err != nil {
			return err
		}
	case "current.json is a FIFO":
		if err := os.Remove(a.record); err != nil {
			return err
		}
		if err := approvalFIFO(a.record); err != nil {
			return err
		}
	case "current.json has a second hard link":
		if err := os.Link(a.record, target); err != nil {
			return err
		}
	case "managed directory mode is 0777":
		if err := os.Chmod(managed, 0777); err != nil {
			return err
		}
	case "current.json mode is 0644":
		if err := os.Chmod(a.record, 0644); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown unsafe fixture %q", layout)
	}
	return w.approvalCapture()
}

func (w *world) approvalSameTree() error {
	actual, err := doctorTree(w.dir)
	if err != nil {
		return err
	}
	expected := w.approval.before
	if expected == nil {
		expected = w.doctor.snapshot
	}
	if !reflect.DeepEqual(actual, expected) {
		return fmt.Errorf("approval changed checkout/host entries")
	}
	return nil
}
