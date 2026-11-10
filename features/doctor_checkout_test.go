package features

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"
)

func (w *world) doctorCheckout() error {
	return w.doctorCheckoutForHost(runtime.GOOS, os.Geteuid())
}

func (w *world) doctorCheckoutForHost(hostOS string, uid int) error {
	if hostOS == "windows" {
		// The fixture must pass manifest validation before the real product can
		// diagnose an unsupported host. Windows' -1 is not a container UID.
		uid = 1000
	}
	d := &doctorWorld{checkout: filepath.Join(w.dir, "checkout"), host: filepath.Join(w.dir, "host"), recorder: filepath.Join(w.dir, "recorder"),
		image: doctorImage, imageID: doctorImageID, engineID: "engine-A", api: "1.56", env: map[string]string{}}
	w.doctor = d
	// The real product reads the account home from /etc/passwd. The probe
	// assembly honors the isolated one the fixture names, so every scenario that
	// builds a checkout runs it instead, compiled once for the suite.
	if !isProbeBinary(w.bin) {
		binary, err := w.productProbeBinary()
		if err != nil {
			return err
		}
		w.bin = binary
	}
	d.cwd = d.checkout
	d.manifest = map[string]any{"image": d.image, "containerUser": strconv.Itoa(uid), "remoteUser": strconv.Itoa(uid)}
	for key, name := range map[string]string{"HOME": "home", "USERPROFILE": "home", "XDG_CONFIG_HOME": "config", "XDG_STATE_HOME": "state", "XDG_CACHE_HOME": "cache", "XDG_DATA_HOME": "data", "DOCKER_CONFIG": "docker", "PATH": "bin", "SLICE_21_ACCOUNT_HOME": "account-home"} {
		dir := filepath.Join(d.host, name)
		if err := os.MkdirAll(dir, 0700); err != nil {
			return err
		}
		d.env[key] = dir
	}
	if err := os.MkdirAll(d.recorder, 0700); err != nil {
		return err
	}
	return w.writeDoctorCheckout(d.checkout)
}

func (w *world) writeDoctorCheckout(root string) error {
	for _, dir := range []string{".git/objects/info", ".git/refs/heads"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0700); err != nil {
			return err
		}
	}
	for path, data := range map[string]string{
		".git/HEAD":           "ref: refs/heads/main\n",
		".git/config":         "[core]\nrepositoryformatversion = 0\nbare = false\n",
		"itos-container.yaml": "version: 1\nmanifest: .devcontainer/devcontainer.json\nnetwork:\n  mode: offline\n",
	} {
		if err := writeManifestFixture(filepath.Join(root, path), []byte(data), 0600); err != nil {
			return err
		}
	}
	data, err := json.Marshal(w.doctor.manifest)
	if err != nil {
		return err
	}
	return writeManifestFixture(filepath.Join(root, ".devcontainer/devcontainer.json"), data, 0600)
}

func (w *world) doctorImageName(name string) error {
	w.doctor.image = name + "@sha256:" + strings.Repeat("a", 64)
	w.doctor.manifest["image"] = w.doctor.image
	data, err := json.Marshal(w.doctor.manifest)
	if err != nil {
		return err
	}
	return writeManifestFixture(filepath.Join(w.doctor.checkout, ".devcontainer/devcontainer.json"), data, 0600)
}

func (w *world) doctorUsers(container, remote string) error {
	for key, value := range map[string]string{"containerUser": container, "remoteUser": remote} {
		if value == "host UID" {
			value = strconv.Itoa(os.Geteuid())
		}
		if value == "other UID" {
			value = strconv.Itoa(os.Geteuid() + 1)
		}
		if value == "omitted" {
			delete(w.doctor.manifest, key)
		} else {
			w.doctor.manifest[key] = value
		}
	}
	data, err := json.Marshal(w.doctor.manifest)
	if err != nil {
		return err
	}
	return writeManifestFixture(filepath.Join(w.doctor.checkout, ".devcontainer/devcontainer.json"), data, 0600)
}

func (w *world) doctorCheckouts(first, second string) error {
	if err := w.doctorCheckout(); err != nil {
		return err
	}
	for _, name := range []string{first, second} {
		if err := w.writeDoctorCheckout(filepath.Join(w.dir, name)); err != nil {
			return err
		}
	}
	return nil
}

func (w *world) doctorRoot(kind string) error {
	path := filepath.Join(w.dir, "missing-root")
	switch kind {
	case "a relative path":
		path = "checkout"
	case "an absent absolute path":
	case "an unreadable absolute directory":
		path = filepath.Join(w.dir, "unreadable")
		if err := os.Mkdir(path, 0000); err != nil {
			return err
		}
		w.doctor.cleanup = append(w.doctor.cleanup, func() { _ = os.Chmod(path, 0700) })
	default:
		return fmt.Errorf("unknown root fixture %q", kind)
	}
	w.doctor.env["ITOS_ROOT"] = path
	return nil
}

func (w *world) doctorLayout(layout string) error {
	if err := w.doctorCheckout(); err != nil {
		return err
	}
	root := w.doctor.checkout
	git := filepath.Join(root, ".git")
	switch layout {
	case "ordinary directory without .git":
		return os.RemoveAll(git)
	case "bare repository":
		if err := os.Rename(git, filepath.Join(w.dir, "bare")); err != nil {
			return err
		}
		w.doctor.checkout, w.doctor.cwd = filepath.Join(w.dir, "bare"), filepath.Join(w.dir, "bare")
		// Keep valid policy/manifest fixtures: only Git storage differs.
		for _, name := range []string{"itos-container.yaml", ".devcontainer"} {
			if err := os.Rename(filepath.Join(root, name), filepath.Join(w.doctor.checkout, name)); err != nil {
				return err
			}
		}
		return os.WriteFile(filepath.Join(w.doctor.checkout, "config"), []byte("[core]\nbare = true\n"), 0600)
	case "linked worktree with a .git file", "submodule worktree with a .git file":
		target := filepath.Join(w.dir, "external-git")
		if err := os.Rename(git, target); err != nil {
			return err
		}
		return os.WriteFile(git, []byte("gitdir: "+target+"\n"), 0600)
	case ".git directory with a commondir file":
		return os.WriteFile(filepath.Join(git, "commondir"), []byte("../../outside\n"), 0600)
	case ".git/objects/info/alternates file", ".git/objects/info/http-alternates file":
		name := strings.TrimSuffix(layout, " file")
		return os.WriteFile(filepath.Join(root, name), []byte("../../outside\n"), 0600)
	case "nested working directory below the root":
		nested := filepath.Join(root, "nested")
		if err := os.Mkdir(nested, 0700); err != nil {
			return err
		}
		for _, name := range []string{"itos-container.yaml", ".devcontainer"} {
			if err := os.Rename(filepath.Join(root, name), filepath.Join(nested, name)); err != nil {
				return err
			}
		}
		w.doctor.cwd = nested
		return nil
	case "missing .git/HEAD":
		return os.Remove(filepath.Join(git, "HEAD"))
	default:
		return fmt.Errorf("unknown checkout layout %q", layout)
	}
}

func (w *world) doctorSymlink(entry string) error {
	if err := w.doctorCheckout(); err != nil {
		return err
	}
	d := w.doctor
	if entry == "ITOS_ROOT" {
		alias := filepath.Join(w.dir, "root-alias")
		d.env["ITOS_ROOT"] = alias
		return os.Symlink(d.checkout, alias)
	}
	if entry == "an ITOS_ROOT ancestor" {
		outside := filepath.Join(w.dir, "outside")
		if err := os.Mkdir(outside, 0700); err != nil {
			return err
		}
		if err := os.Rename(d.checkout, filepath.Join(outside, "checkout")); err != nil {
			return err
		}
		alias := filepath.Join(w.dir, "ancestor-alias")
		if err := os.Symlink(outside, alias); err != nil {
			return err
		}
		d.checkout = filepath.Join(outside, "checkout")
		d.cwd = d.checkout
		d.env["ITOS_ROOT"] = filepath.Join(alias, "checkout")
		return nil
	}
	path := filepath.Join(d.checkout, entry)
	outside := filepath.Join(w.dir, "outside-entry")
	if err := os.Rename(path, outside); err != nil {
		return err
	}
	return os.Symlink(outside, path)
}

func (w *world) doctorSentinels() error {
	d := w.doctor
	// These cases run only on Linux. Absolute /bin/sh and its builtin printf do
	// not depend on any caller tool or interpreter selected by the checkout.
	code := "#!/bin/sh\nprintf executed > '" + filepath.Join(d.recorder, "executed") + "'\n"
	for _, name := range []string{"git", "docker", "devcontainer", "ssh", "sh", "bash", "npm", "make", "docker-credential-slice-3"} {
		if err := writeManifestFixture(filepath.Join(d.env["PATH"], name), []byte(code), 0700); err != nil {
			return err
		}
	}
	for _, name := range []string{"slice-3-record-execution", ".git/hooks/pre-commit", ".git/hooks/post-checkout", ".git/hooks/post-index-change"} {
		if err := writeManifestFixture(filepath.Join(d.checkout, name), []byte(code), 0700); err != nil {
			return err
		}
	}
	config := "[core]\nrepositoryformatversion = 0\nbare = false\nfsmonitor = ./slice-3-record-execution\n[credential]\nhelper = ./slice-3-record-execution\n"
	if err := os.WriteFile(filepath.Join(d.checkout, ".git/config"), []byte(config), 0600); err != nil {
		return err
	}
	for name, data := range map[string]string{
		"Makefile":     "all:\n\t./slice-3-record-execution\n",
		"Dockerfile":   "FROM scratch\nRUN ./slice-3-record-execution\n",
		"package.json": `{"scripts":{"prepare":"./slice-3-record-execution","build":"./slice-3-record-execution"}}`,
	} {
		if err := writeManifestFixture(filepath.Join(d.checkout, name), []byte(data), 0600); err != nil {
			return err
		}
	}
	return nil
}

func (w *world) doctorCredentialSentinels() error {
	if err := w.doctorSentinels(); err != nil {
		return err
	}
	for name, data := range map[string]string{
		"config.json":                        `{"currentContext":"remote","credsStore":"slice-3"}`,
		"contexts/meta/slice-3/meta.json":    `{"Name":"remote","Endpoints":{"docker":{"Host":"ssh://127.0.0.1"}}}`,
		"contexts/tls/slice-3/docker/ca.pem": "synthetic fixture, not a certificate",
	} {
		if err := writeManifestFixture(filepath.Join(w.doctor.env["DOCKER_CONFIG"], name), []byte(data), 0600); err != nil {
			return err
		}
	}
	return nil
}

// Doctor must leave host control state alone. Unrelated synthetic files outside
// every managed tree prove it ignores them; the records it can read or change
// are valid ones in the selected tree: this checkout's approval, the account
// catalog that binds the fixed and selected trees, and a public session with
// its intent. Without the catalog, session evidence refuses inspection.
func (w *world) doctorHostState() error {
	for _, name := range []string{"home/.local/state/itos-container/approvals/existing.json", "state/unrelated-sessions/existing.json", "config/itos-container/approvals.json"} {
		if err := writeManifestFixture(filepath.Join(w.doctor.host, name), []byte(`{"synthetic":"preserve"}`), 0600); err != nil {
			return err
		}
	}
	w.approval = &approvalWorld{uid: strconv.Itoa(os.Geteuid()), gid: strconv.Itoa(os.Getegid())}
	if err := w.approvalSeed("1", false); err != nil {
		return err
	}
	token, id, worker := strings.Repeat("a", 64), strings.Repeat("b", 64), strings.Repeat("c", 64)
	w.session = &sessionWorld{token: token, reference: "intents/" + w.approval.project + "/" + token + ".json"}
	if err := w.sessionSeedSnapshot(); err != nil {
		return err
	}
	if err := writeManifestFixture(w.sessionRecordPath(), approvalJSON(w.sessionExpectedRecord("owned", worker)), 0600); err != nil {
		return err
	}
	record := filepath.Join(w.doctor.env["XDG_STATE_HOME"], "itos-container", "sessions", w.recoveryIndexKey(), id)
	public := map[string]any{"schema": 1, "id": id, "project": w.approval.project, "intent": w.session.reference, "state": "running", "workload_exit": nil, "last_problem": nil}
	if err := writeManifestFixture(filepath.Join(record, "record.json"), approvalJSON(public), 0600); err != nil {
		return err
	}
	for _, name := range []string{"controller.lock", "operation.lock"} {
		if err := os.WriteFile(filepath.Join(record, name), nil, 0600); err != nil {
			return err
		}
	}
	return nil
}

func (w *world) doctorEnv(env []string) []string {
	if w.doctor == nil {
		return env
	}
	prefixes := []string{"DOCKER_", "CONTAINER_", "XDG_", "SSH_", "AWS_", "AZURE_", "GOOGLE_", "GCLOUD_", "CLAUDE_", "ANTHROPIC_", "OPENAI_", "OPENCODE_", "SLICE_"}
	env = slices.DeleteFunc(env, func(kv string) bool {
		key, _, _ := strings.Cut(kv, "=")
		key = strings.ToUpper(key)
		if strings.HasSuffix(key, "_PROXY") || key == "ALL_PROXY" {
			return true
		}
		for _, prefix := range prefixes {
			if strings.HasPrefix(key, prefix) {
				return true
			}
		}
		return false
	})
	for key, value := range w.doctor.env {
		env = setEnv(env, key, value)
	}
	return env
}

// Include every entry created during invocation, modes and link targets. Never
// follow a symlink or open a socket, FIFO or deliberately unreadable directory.
func doctorTree(root string) (map[string]string, error) {
	return doctorTreeWithWalk(root, nil, filepath.WalkDir)
}

// A per-call walker lets observer controls deliver entries across a rename.
// Native observations always use filepath.WalkDir.
type doctorTreeWalk func(string, fs.WalkDirFunc) error

func doctorTreeWithWalk(root string, excluded map[string]bool, walk doctorTreeWalk) (map[string]string, error) {
	files := map[string]string{}
	err := walk(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if excluded[path] {
			// Omit only this entry before inspecting it. A directory's children
			// remain audited, so this must not return SkipDir or SkipAll.
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		data := ""
		if info.Mode().IsRegular() {
			b, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			data = string(b)
		} else if info.Mode()&os.ModeSymlink != 0 {
			data, err = os.Readlink(path)
			if err != nil {
				return err
			}
		}
		owner := "unavailable"
		stat := reflect.Indirect(reflect.ValueOf(info.Sys()))
		if stat.IsValid() && stat.Kind() == reflect.Struct {
			if uid := stat.FieldByName("Uid"); uid.IsValid() {
				owner = fmt.Sprint(uid.Interface())
			}
		}
		files[path] = fmt.Sprintf("%v:%s:%s", info.Mode(), owner, data)
		if info.IsDir() && info.Mode().Perm()&0500 == 0 {
			return filepath.SkipDir
		}
		return nil
	})
	return files, err
}

func doctorCheckoutIdentity(root string) (map[string]any, error) {
	canonical, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, err
	}
	result := map[string]any{"root": canonical}
	for name, path := range map[string]string{"": root, "git_": filepath.Join(root, ".git")} {
		info, err := os.Stat(path)
		if err != nil {
			return nil, err
		}
		stat := reflect.Indirect(reflect.ValueOf(info.Sys()))
		for key, field := range map[string]string{"device": "Dev", "inode": "Ino"} {
			value := stat.FieldByName(field)
			if !value.IsValid() {
				return nil, fmt.Errorf("filesystem identity unavailable for %s", path)
			}
			result[name+key] = fmt.Sprint(value.Interface())
		}
	}
	return result, nil
}

func (w *world) runDoctor(env []string, args ...string) error {
	d := w.doctor
	if d.snapshot == nil {
		var err error
		d.snapshot, err = doctorTree(w.dir)
		if err != nil {
			return err
		}
	}
	// Capture expected identities independently before launch; unsupported layouts
	// have no success assertion and need not expose a stat structure on Windows.
	var expected map[string]any
	if strings.Contains(strings.Join(args, " "), "doctor") && len(args) > 0 {
		expected, _ = doctorCheckoutIdentityPortable(d.checkout)
	}
	w.deadline = 8 * time.Second
	start := time.Now()
	err := w.runEnv(d.cwd, env, w.bin, args...)
	result := doctorResult{stdout: w.stdout, stderr: w.stderr, exit: w.exit, elapsed: time.Since(start), checkout: expected, daemon: d.engineID}
	if d.engine != nil {
		result.endpoint = "unix://" + d.engine.socket
	}
	d.results = append(d.results, result)
	return err
}

func doctorCheckoutIdentityPortable(root string) (map[string]any, error) {
	// Reflection permits this test binary to compile on Windows, whose unsupported
	// host case never asserts Unix filesystem identities.
	info, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	stat := reflect.Indirect(reflect.ValueOf(info.Sys()))
	if !stat.IsValid() || stat.Kind() != reflect.Struct || !stat.FieldByName("Dev").IsValid() {
		return nil, nil
	}
	return doctorCheckoutIdentity(root)
}

func (w *world) closeDoctor() {
	if w.doctor == nil {
		return
	}
	if w.doctor.engine != nil {
		w.doctor.engine.close()
	}
	for i := len(w.doctor.cleanup) - 1; i >= 0; i-- {
		w.doctor.cleanup[i]()
	}
}

func (w *world) doctorThreeRuns(first, second, third string) error {
	for _, name := range []string{first, second, third} {
		w.doctor.checkout = filepath.Join(w.dir, name)
		w.doctor.cwd = w.doctor.checkout
		if err := w.runDoctor(w.env(), "doctor", "--json"); err != nil {
			return err
		}
	}
	return nil
}

func (w *world) doctorTwoRuns(id string) error {
	if err := w.runDoctor(w.env(), "doctor", "--json"); err != nil {
		return err
	}
	d := w.doctor
	d.engine.mu.Lock()
	d.engine.info["ID"] = id
	d.engine.mu.Unlock()
	d.engineID = id
	return w.runDoctor(w.env(), "doctor", "--json")
}

func (w *world) doctorRace(entry string, engine bool) error {
	d := w.doctor
	done := make(chan error, 1)
	go func() { done <- w.runDoctor(w.env(), "doctor", "--json") }()
	select {
	case err := <-done:
		return err // Missing inspection still reaches the Then assertion.
	case <-d.engine.reached:
	}
	var err error
	if engine {
		if entry == "daemon ID" {
			d.engine.mu.Lock()
			d.engine.info["ID"] = "engine-B"
			d.engine.mu.Unlock()
		} else {
			err = d.engine.replaceSocket()
		}
	} else {
		path := d.checkout
		if entry == ".git" {
			path = filepath.Join(path, ".git")
		}
		err = os.Rename(path, path+".before")
		if err == nil {
			if entry == ".git" {
				err = os.MkdirAll(filepath.Join(path, "objects"), 0700)
			} else {
				err = w.writeDoctorCheckout(path)
			}
		}
		if err == nil {
			d.replacement = path
			d.replacementSnapshot, err = doctorTree(path)
		}
	}
	d.engine.unpause()
	runErr := <-done
	if err != nil {
		return err
	}
	return runErr
}
