package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	ap "github.com/donvargax/itos-container/internal/engine/approval/port"
	"github.com/moby/moby/api/types/container"
)

// The opencode-v1 profile as the specification states it, written out here and
// never read back from the product.
var openCodeProfile = map[string]string{
	"OPENCODE_CONFIG_CONTENT":       `{"share":"disabled"}`,
	"OPENCODE_DISABLE_AUTOUPDATE":   "true",
	"OPENCODE_DISABLE_MODELS_FETCH": "true",
}

// The synthetic image's own conflicting defaults. Only the three profile names
// may be overridden; the rest belong to the approved, digest-pinned image.
var openCodeImageDefaults = []string{
	`OPENCODE_CONFIG_CONTENT={"share":"manual"}`,
	"OPENCODE_DISABLE_AUTOUPDATE=false",
	"OPENCODE_DISABLE_MODELS_FETCH=false",
	"OPENCODE_CONFIG=/image/opencode.json",
	"XDG_RUNTIME_DIR=/image-runtime",
}

var openCodeArgv = []string{"opencode", "--standalone", "run", "--format", "json", "--model", "fake/fake-model", "say-ok"}

const openCodeHostSentinel = "slice-41-host-sentinel"

// enableOpenCode freezes the harness in the approved settings. The snapshot
// bytes are encoded here, never through the approval codec.
func (f *homeLocalFixture) enableOpenCode(t *testing.T) {
	t.Helper()
	var v map[string]any
	if err := json.Unmarshal([]byte(f.m.c.Canonical), &v); err != nil {
		t.Fatal(err)
	}
	v["home"].(map[string]any)["harnesses"] = []any{map[string]any{"name": "opencode", "profile": "opencode-v1"}}
	data, _ := json.Marshal(v)
	f.m.c.Canonical = string(data)
	f.m.c.Fingerprint = "sha256:" + fmt.Sprintf("%x", sha256.Sum256(data))
	n, _ := strconv.Atoi(f.m.b.Frozen.Generation)
	generation := strconv.Itoa(n + 1)
	f.m.b.Frozen = ap.Frozen{Project: f.m.c.Project, Generation: generation, Fingerprint: f.m.c.Fingerprint, Settings: f.m.c.Canonical}
	record, _ := json.Marshal(map[string]any{"schema": 1, "project": f.m.c.Project, "generation": generation, "fingerprint": f.m.c.Fingerprint, "settings": v})
	f.m.storage.Directories["snapshots/"+f.m.c.Project].Files[generation+".json"] = record
}

// openCodeInspectionDefect alters only an inspection reply, as the row names it.
func openCodeInspectionDefect(v *container.InspectResponse, defect string) {
	set := func(name, value string) {
		v.Config.Env = slices.DeleteFunc(v.Config.Env, func(e string) bool { return strings.HasPrefix(e, name+"=") })
		if value != "" {
			v.Config.Env = append(v.Config.Env, name+"="+value)
		}
	}
	switch defect {
	case "OPENCODE_CONFIG_CONTENT left at the image's conflicting value":
		set("OPENCODE_CONFIG_CONTENT", `{"share":"manual"}`)
	case "OPENCODE_CONFIG_CONTENT missing":
		set("OPENCODE_CONFIG_CONTENT", "")
	case `OPENCODE_CONFIG_CONTENT set to {"share":"auto"}`:
		set("OPENCODE_CONFIG_CONTENT", `{"share":"auto"}`)
	case "OPENCODE_DISABLE_MODELS_FETCH missing":
		set("OPENCODE_DISABLE_MODELS_FETCH", "")
	case "OPENCODE_DISABLE_MODELS_FETCH left at the image's false":
		set("OPENCODE_DISABLE_MODELS_FETCH", "false")
	case "OPENCODE_DISABLE_AUTOUPDATE set to false":
		set("OPENCODE_DISABLE_AUTOUPDATE", "false")
	case "OPENCODE_DISABLE_AUTOUPDATE missing":
		set("OPENCODE_DISABLE_AUTOUPDATE", "")
	}
}

// openCodeExpected is the image's defaults with the home mappings over them and
// the opencode-v1 values over those.
func openCodeExpected() []string {
	env := map[string]string{}
	for _, entry := range append([]string{"HOME=/image-home", "TMPDIR=/image-temp", "XDG_CONFIG_HOME=/image-config", "XDG_DATA_HOME=/image-data", "XDG_STATE_HOME=/image-state", "XDG_CACHE_HOME=/image-cache"}, openCodeImageDefaults...) {
		key, value, _ := strings.Cut(entry, "=")
		env[key] = value
	}
	maps.Copy(env, map[string]string{"HOME": "/home/itos", "TMPDIR": "/tmp", "XDG_CONFIG_HOME": "/home/itos/.config", "XDG_DATA_HOME": "/home/itos/.local/share", "XDG_STATE_HOME": "/home/itos/.local/state", "XDG_CACHE_HOME": "/home/itos/.cache"})
	maps.Copy(env, openCodeProfile)
	var out []string
	for _, key := range slices.Sorted(maps.Keys(env)) {
		out = append(out, key+"="+env[key])
	}
	return out
}

// openCodeProfileOnly requires each approved profile name exactly once, with
// its approved bytes.
func openCodeProfileOnly(t *testing.T, where string, env []string) {
	t.Helper()
	for name, want := range openCodeProfile {
		count := 0
		for _, entry := range env {
			if key, value, _ := strings.Cut(entry, "="); key == name {
				count++
				if value != want {
					t.Fatalf("%s holds %s=%q, want %q", where, name, value, want)
				}
			}
		}
		if count != 1 {
			t.Fatalf("%s holds %d entries for %s, want exactly one", where, count, name)
		}
	}
}

func openCodeHostSentinels(t *testing.T) (string, map[string]string) {
	t.Helper()
	host := t.TempDir()
	files := map[string]string{}
	for _, path := range []string{".config/opencode/opencode.json", ".local/share/opencode/auth.json", ".local/state/opencode/state.json"} {
		full := filepath.Join(host, path)
		if err := os.MkdirAll(filepath.Dir(full), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(openCodeHostSentinel), 0600); err != nil {
			t.Fatal(err)
		}
		files[full] = openCodeHostSentinel
	}
	t.Setenv("HOME", host)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(host, ".config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(host, ".local/share"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(host, ".local/state"))
	t.Setenv("OPENCODE_CONFIG_CONTENT", openCodeHostSentinel)
	t.Setenv("OPENCODE_CONFIG", filepath.Join(host, ".config/opencode/opencode.json"))
	return host, files
}

func openCodeRun(t *testing.T, f *homeLocalFixture, want int, rule string) {
	t.Helper()
	code, _, diagnostic := f.command(t, append([]string{"run", "--"}, openCodeArgv...)...)
	if code != want || rule != "" && !strings.Contains(diagnostic, rule) {
		t.Fatalf("OpenCode run: exit %d, %s; want %d %s", code, diagnostic, want, rule)
	}
}

// openCodeLaunch is ID-HOME-28. Only the disk backend judges the worker
// environment: the memory fake records argv but no environment.
func openCodeLaunch(t *testing.T) {
	f := newHomeLocalFixture(t, "1000", "1000")
	host, sentinels := openCodeHostSentinels(t)
	if f.sdk != nil {
		f.sdk.imageEnv = openCodeImageDefaults
	}
	f.enableOpenCode(t)
	openCodeRun(t, f, 0, "")
	if f.sdk == nil && !slices.Equal(f.engine.Argv, openCodeArgv) {
		t.Fatal("the first worker's command is not the supplied argv", f.engine.Argv)
	}
	scope := f.scope()
	if len(scope.Generations) != 1 || len(f.engine.Homes) != 1 {
		t.Fatal("the first run did not allocate exactly one home", scope)
	}
	g := scope.Generations[0]
	root := f.engine.Roots[g.Name]
	root.Marker = "retained worker data"
	f.engine.Roots[g.Name] = root
	openCodeRun(t, f, 0, "")
	if f.sdk == nil && !slices.Equal(f.engine.Argv, openCodeArgv) {
		t.Fatal("the second worker's command is not the supplied argv", f.engine.Argv)
	}
	counts := map[string]int{}
	for _, event := range f.engine.HomeEvents {
		counts[event]++
	}
	if f.engine.Roots[g.Name].Marker != root.Marker || len(f.engine.Homes) != 1 || counts["allocate"] != 1 || counts["initialize"] != 1 || counts["remove"] != 0 {
		t.Fatal("the second run did not reuse the home without writing into it", counts)
	}
	for path, content := range sentinels {
		if data, err := os.ReadFile(path); err != nil || string(data) != content {
			t.Fatal("host-home sentinel changed", path, err)
		}
	}
	if entries, _ := os.ReadDir(host); len(entries) != 2 {
		t.Fatal("host home gained or lost entries", entries)
	}
	if f.sdk == nil {
		return
	}
	f.sdk.mu.Lock()
	defer f.sdk.mu.Unlock()
	if len(f.sdk.created) != 2 {
		t.Fatal("expected two created workers", len(f.sdk.created))
	}
	expected := openCodeExpected()
	for i, worker := range f.sdk.created {
		if !slices.Equal(worker.Config.Cmd, openCodeArgv) {
			t.Fatal("worker command is not the supplied argv", worker.Config.Cmd)
		}
		env := slices.Sorted(slices.Values(worker.Config.Env))
		if !slices.Equal(env, expected) {
			t.Fatalf("worker %d created environment %v, want %v", i, env, expected)
		}
		for _, entry := range worker.Config.Env {
			if strings.Contains(entry, openCodeHostSentinel) || strings.Contains(entry, host) {
				t.Fatal("a host value reached the worker environment", entry)
			}
		}
		if len(worker.HostConfig.Mounts) != 2 {
			t.Fatal("worker mounts are not the anchor and the home", worker.HostConfig.Mounts)
		}
		for _, m := range worker.HostConfig.Mounts {
			if m.Type != "volume" || strings.Contains(m.Source, host) || strings.Contains(m.Source, "opencode") {
				t.Fatal("unexpected worker mount", m)
			}
		}
	}
	if len(f.sdk.inspected) == 0 {
		t.Fatal("no worker inspection was recorded")
	}
	for i, worker := range f.sdk.inspected {
		if worker.Config == nil || !strings.HasPrefix(worker.Name, "/itos-container-") {
			continue
		}
		if env := slices.Sorted(slices.Values(worker.Config.Env)); !slices.Equal(env, expected) {
			t.Fatalf("inspection %d environment %v, want %v", i, env, expected)
		}
	}
}

// openCodeBoundary is ID-HOME-29: the defective launch is refused and the
// control, with the same request and an unaltered reply, succeeds. Only the
// disk backend's recording endpoint judges the worker environment.
func openCodeBoundary(t *testing.T, defect string) {
	if os.Getenv("SLICE_34_HOME_BACKEND") != "disk" {
		t.Skip("only the disk backend's recording endpoint judges the worker environment")
	}
	f := newHomeLocalFixture(t, "1000", "1000")
	f.sdk.imageEnv = openCodeImageDefaults
	f.enableOpenCode(t)
	f.boundary = defect
	openCodeRun(t, f, 1, "run.isolation-unverified")
	if len(f.engine.Homes) != 1 || len(f.engine.Resources) != 0 {
		t.Fatal("the defective launch left a worker or lost the home", f.engine.Homes, f.engine.Resources)
	}
	for _, event := range f.engine.Events {
		if event == "start" {
			t.Fatal("the defective launch started")
		}
	}
	for _, event := range f.engine.HomeEvents {
		if event == "remove" {
			t.Fatal("the defective launch removed the home")
		}
	}
	f.sdk.mu.Lock()
	if len(f.sdk.created) != 1 {
		f.sdk.mu.Unlock()
		t.Fatal("the defective launch created", len(f.sdk.created))
	}
	openCodeProfileOnly(t, "the defective launch's create body", f.sdk.created[0].Config.Env)
	f.sdk.mu.Unlock()
	f.boundary = ""
	openCodeRun(t, f, 0, "")
	f.sdk.mu.Lock()
	defer f.sdk.mu.Unlock()
	if len(f.sdk.created) != 2 {
		t.Fatal("the control did not create its worker", len(f.sdk.created))
	}
	openCodeProfileOnly(t, "the control's create body", f.sdk.created[1].Config.Env)
	last := f.sdk.inspected[len(f.sdk.inspected)-1]
	if last.Config == nil {
		t.Fatal("the control was not inspected")
	}
	openCodeProfileOnly(t, "the control's inspection reply", last.Config.Env)
}
