package features

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// T-81 cleanup runs the real driver against a recording fake docker. The child sees only the
// private PATH below, so no real engine, curl or tar is reachable. Linux only: the driver needs
// GNU coreutils (readlink -e, timeout, stat -c), which the manual macOS and Windows jobs lack.

const (
	codexRepository = "127.0.0.1:5000/itos-fixture-4242-1-codex"
	codexTag        = codexRepository + ":fixture"
	codexDecoy      = codexRepository + ":decoy"
)

var (
	codexBaseID  = "sha256:" + strings.Repeat("a", 64)
	codexRegID   = "sha256:" + strings.Repeat("b", 64)
	codexBuiltID = "sha256:" + strings.Repeat("c", 64)
	codexOtherID = "sha256:" + strings.Repeat("d", 64)
	codexDigest  = "sha256:" + strings.Repeat("e", 64)
)

type codexWorld struct {
	bash, driver, bin, runner, workspace, state string
}

func newCodexWorld(t *testing.T, machine string) *codexWorld {
	t.Helper()
	bash, err := exec.LookPath("bash")
	requireFixture(t, err == nil, "host tool missing: bash")
	driver, err := filepath.Abs("../tools/bin/engine-fixture")
	requireFixture(t, err == nil, "driver path: %v", err)
	w := &codexWorld{bash: bash, driver: driver, bin: t.TempDir(), runner: t.TempDir(), workspace: t.TempDir()}
	for _, tool := range []string{"bash", "env", "timeout", "jq", "sha256sum", "readlink", "id", "mkdir", "cat", "stat"} {
		path, err := exec.LookPath(tool)
		requireFixture(t, err == nil, "host tool missing: %s", tool)
		requireFixture(t, os.Symlink(path, filepath.Join(w.bin, tool)) == nil, "cannot link host tool %s", tool)
	}
	w.stub("uname", "case \"${1:-}\" in\n-s) printf 'Linux\\n' ;;\n-m) printf '"+machine+"\\n' ;;\n*) exit 99 ;;\nesac\n")
	for _, name := range []string{"curl", "tar"} {
		w.stub(name, "printf '%s\\n' \"$name $*\" >>\""+filepath.Join(w.bin, "log")+"\"\nexit 99\n", "name="+name)
	}
	w.stub("docker", strings.Join([]string{
		`dir=` + w.bin,
		`while [[ ${1:-} == --config || ${1:-} == --host ]]; do shift 2; done`,
		`printf '%s\n' "$*" >>"$dir/log"`,
		`if [[ ${1:-} == image && ${2:-} == inspect && $# == 3 ]]; then`,
		`  while IFS=$'\t' read -r ref id; do`,
		`    if [[ $ref == "$3" ]]; then printf '[{"Id":"%s"}]\n' "$id"; exit 0; fi`,
		`  done <"$dir/store"`,
		`  printf 'Error response from daemon: No such image: %s\n' "$3" >&2`,
		`  exit 1`,
		`fi`,
		`if [[ ${1:-} == image && ${2:-} == rm && $# == 3 ]]; then`,
		`  lines=(); kept=''`,
		`  while IFS= read -r line; do lines+=("$line"); done <"$dir/store"`,
		`  for line in "${lines[@]}"; do [[ ${line%%$'\t'*} == "$3" ]] || kept+="$line"$'\n'; done`,
		`  printf '%s' "$kept" >"$dir/store"`,
		`  printf 'Untagged: %s\n' "$3"`,
		`  exit 0`,
		`fi`,
		`exit 99`,
	}, "\n")+"\n")
	temp, err := filepath.EvalSymlinks(w.runner)
	requireFixture(t, err == nil, "runner temp: %v", err)
	w.state = filepath.Join(temp, "T-20-4242-1-engine-fixture")
	return w
}

// stub writes an executable bash script that needs nothing from PATH.
func (w *codexWorld) stub(name, body string, prelude ...string) {
	script := "#!" + w.bash + "\n" + strings.Join(prelude, "\n") + "\n" + body
	if err := os.WriteFile(filepath.Join(w.bin, name), []byte(script), 0o700); err != nil {
		panic(err)
	}
}

func (w *codexWorld) records(t *testing.T, records map[string]string, store ...[2]string) {
	t.Helper()
	requireFixture(t, os.Mkdir(w.state, 0o700) == nil, "cannot create state directory")
	for name, data := range records {
		requireFixture(t, os.WriteFile(filepath.Join(w.state, name), []byte(data+"\n"), 0o600) == nil, "cannot write record %s", name)
	}
	var lines strings.Builder
	for _, entry := range store {
		lines.WriteString(entry[0] + "\t" + entry[1] + "\n")
	}
	requireFixture(t, os.WriteFile(filepath.Join(w.bin, "store"), []byte(lines.String()), 0o600) == nil, "cannot write image store")
}

func (w *codexWorld) baseRecords() map[string]string {
	return map[string]string{
		"owner": "T-20-4242-1", "base.iid": codexBaseID, "registry.iid": codexRegID,
		"codex.repository": codexRepository, "codex.tag": codexTag, "codex.build.iid": codexBuiltID,
	}
}

func (w *codexWorld) pinned() [][2]string {
	return [][2]string{{busyboxFixturePin, codexBaseID}, {registryFixturePin, codexRegID}, {codexDecoy, codexBuiltID}}
}

func (w *codexWorld) run(t *testing.T) (string, string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, w.bash, w.driver, "cleanup")
	cmd.Dir = w.workspace
	cmd.Env = []string{
		"PATH=" + w.bin, "HOME=" + t.TempDir(), "RUNNER_TEMP=" + w.runner, "GITHUB_WORKSPACE=" + w.workspace,
		"CI=true", "GITHUB_ACTIONS=true", "GITHUB_RUN_ID=4242", "GITHUB_RUN_ATTEMPT=1", "LC_ALL=C",
	}
	var stdout, stderr strings.Builder
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	requireFixture(t, ctx.Err() == nil, "driver exceeded forty seconds")
	return stdout.String(), stderr.String(), err
}

func (w *codexWorld) log(t *testing.T) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(w.bin, "log"))
	if os.IsNotExist(err) {
		return nil
	}
	requireFixture(t, err == nil, "cannot read call log: %v", err)
	return strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
}

func (w *codexWorld) store(t *testing.T) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(w.bin, "store"))
	requireFixture(t, err == nil, "cannot read image store: %v", err)
	return strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
}

// requireOwnedRemovals holds the recorded-references-only invariant: the log holds only inspections
// and removals of the recorded references, and each removal sits between inspections of itself.
func requireOwnedRemovals(t *testing.T, log []string, recorded []string, removed ...string) {
	t.Helper()
	var got []string
	for i, line := range log {
		kind, ref, _ := strings.Cut(strings.TrimPrefix(line, "image "), " ")
		requireFixture(t, strings.HasPrefix(line, "image ") && (kind == "inspect" || kind == "rm") && slices.Contains(recorded, ref), "unexpected call: %q", line)
		if kind != "rm" {
			continue
		}
		got = append(got, ref)
		requireFixture(t, i > 0 && i+1 < len(log) && log[i-1] == "image inspect "+ref && log[i+1] == "image inspect "+ref,
			"removal of %s is not between inspections of it: %q", ref, log)
	}
	requireFixture(t, slices.Equal(got, removed), "removals: expected %q; observed %q", removed, got)
}

func TestEngineFixtureCodexCleanup(t *testing.T) {
	t.Run("host_guard_still_refuses", func(t *testing.T) {
		w := newCodexWorld(t, "aarch64")
		w.records(t, w.baseRecords(), w.pinned()...)
		_, stderr, err := w.run(t)
		requireFixture(t, err != nil && strings.Contains(stderr, "requires an actual Linux amd64 host"), "guard did not refuse: %v stderr=%s", err, stderr)
		requireFixture(t, len(w.log(t)) == 0, "docker or curl ran under the refused guard: %q", w.log(t))
	})
	t.Run("failed_after_tag", func(t *testing.T) {
		w := newCodexWorld(t, "x86_64")
		w.records(t, w.baseRecords(), append(w.pinned(), [2]string{codexTag, codexBuiltID})...)
		stdout, stderr, err := w.run(t)
		requireFixture(t, err == nil && strings.Contains(stdout, "engine-fixture cleanup: PASS"), "cleanup failed: %v stdout=%s stderr=%s", err, stdout, stderr)
		requireOwnedRemovals(t, w.log(t), []string{codexTag}, codexTag)
		requireFixture(t, slices.Equal(w.store(t), []string{busyboxFixturePin + "\t" + codexBaseID, registryFixturePin + "\t" + codexRegID, codexDecoy + "\t" + codexBuiltID}), "store: %q", w.store(t))
	})
	t.Run("failed_after_cold_pull", func(t *testing.T) {
		w := newCodexWorld(t, "x86_64")
		records := w.baseRecords()
		records["codex.digest"] = codexDigest
		digestRef := codexRepository + "@" + codexDigest
		w.records(t, records, append(w.pinned(), [2]string{digestRef, codexBuiltID})...)
		stdout, stderr, err := w.run(t)
		requireFixture(t, err == nil && strings.Contains(stdout, "engine-fixture cleanup: PASS"), "cleanup failed: %v stdout=%s stderr=%s", err, stdout, stderr)
		requireOwnedRemovals(t, w.log(t), []string{codexTag, digestRef}, digestRef)
		requireFixture(t, slices.Equal(w.store(t), []string{busyboxFixturePin + "\t" + codexBaseID, registryFixturePin + "\t" + codexRegID, codexDecoy + "\t" + codexBuiltID}), "store: %q", w.store(t))
	})
	t.Run("mismatched_id", func(t *testing.T) {
		w := newCodexWorld(t, "x86_64")
		w.records(t, w.baseRecords(), append(w.pinned(), [2]string{codexTag, codexOtherID})...)
		_, stderr, err := w.run(t)
		requireFixture(t, err != nil && strings.Contains(stderr, "codex reference identity"), "mismatch accepted or unnamed: %v stderr=%s", err, stderr)
		for _, line := range w.log(t) {
			requireFixture(t, !strings.HasPrefix(line, "image rm"), "removed despite mismatch: %q", w.log(t))
		}
	})
	t.Run("archive_verify", testCodexArchiveVerify)
	t.Run("order_guard", testCodexOrderGuard)
}
