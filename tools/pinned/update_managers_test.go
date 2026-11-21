package pinned_test

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/tailscale/hujson"
)

// The unames the launcher maps to platforms, with the platform each yields.
var unamePlatforms = []struct{ uname, platform string }{
	{"Linux-x86_64", "linux-amd64"},
	{"Linux-aarch64", "linux-arm64"},
	{"Darwin-x86_64", "darwin-amd64"},
	{"Darwin-arm64", "darwin-arm64"},
	{"MINGW64_NT-10.0-x86_64", "windows-amd64"},
	{"MINGW64_NT-10.0-aarch64", "windows-arm64"},
}

// The platforms each tool pins today; the supported sets must not change.
var supportedPlatforms = map[string][]string{
	"golangci-lint": {"linux-amd64", "linux-arm64", "darwin-amd64", "darwin-arm64", "windows-amd64", "windows-arm64"},
	"goreleaser":    {"linux-amd64", "linux-arm64", "darwin-amd64", "darwin-arm64"},
	"git-cliff":     {"linux-amd64", "linux-arm64", "darwin-amd64", "darwin-arm64"},
	"itos-cc":       {"linux-amd64", "linux-arm64", "darwin-amd64", "darwin-arm64", "windows-amd64"},
}

const wantConfigPreserved = `{
	"$schema": "https://docs.renovatebot.com/renovate-schema.json",
	"extends": [
		"github>donvargax/renovate-config#v1.3.0",
		"github>donvargax/renovate-config:go#v1.3.0"
	],
	"ignoreDeps": ["donvargax/itos"],
	"automergeType": "branch",
	"automergeSchedule": ["at any time"],
	"semanticCommits": "enabled",
	"commitBody": "Renovate's update, landed on main once CI passed on its branch.\n\n{{#each upgrades}}{{#unless @first}}\n{{/unless}}- {{{depName}}} {{#if (equals updateType 'digest')}}{{{currentDigestShort}}} to {{{newDigestShort}}}{{else}}{{{currentVersion}}} to {{{newVersion}}}{{#if newDigestShort}} ({{{newDigestShort}}}){{/if}}{{/if}}{{/each}}",
	"packageRules": [
		{"matchPackageNames": ["*"], "semanticCommitType": "build", "semanticCommitScope": null},
		{"matchManagers": ["github-actions", "renovate-config"], "semanticCommitType": "ci", "semanticCommitScope": null},
		{"matchManagers": ["github-actions"], "pinDigests": true}
	]
}`

const wantCustomManagers = `[{
	"customType": "regex",
	"managerFilePatterns": ["/^tools/bin/pinned$/"],
	"matchStrings": ["# renovate: depName=(?<depName>\\S+)\\n\\t+version=(?<currentValue>\\S+)\\n"],
	"datasourceTemplate": "github-releases",
	"extractVersionTemplate": "^v(?<version>.+)$"
}]`

func decodeJSON(t *testing.T, data string) any {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(data), &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func loadConfig(t *testing.T) map[string]any {
	t.Helper()
	source, err := os.ReadFile("../../.github/renovate.json5")
	if err != nil {
		t.Fatal(err)
	}
	standard, err := hujson.Standardize(source)
	if err != nil {
		t.Fatalf("renovate.json5 is not HuJSON: %v", err)
	}
	var config map[string]any
	if err := json.Unmarshal(standard, &config); err != nil {
		t.Fatal(err)
	}
	return config
}

func readLauncher(t *testing.T) string {
	t.Helper()
	source, err := os.ReadFile("../bin/pinned")
	if err != nil {
		t.Fatal(err)
	}
	return string(source)
}

// launcherArm is one tool's case arm in the launcher.
type launcherArm struct {
	tool       string
	start, end int
}

func launcherArms(t *testing.T, script string) []launcherArm {
	t.Helper()
	open := strings.Index(script, "case \"$tool\" in\n")
	if open < 0 {
		t.Fatal("cannot locate the launcher's tool case")
	}
	open += len("case \"$tool\" in\n")
	closing := strings.Index(script[open:], "\nesac\n")
	if closing < 0 {
		t.Fatal("cannot locate the end of the launcher's tool case")
	}
	body := script[open : open+closing+1]
	headers := regexp.MustCompile(`(?m)^(\S+)\)`).FindAllStringSubmatchIndex(body, -1)
	var arms []launcherArm
	for i, h := range headers {
		end := len(body)
		if i+1 < len(headers) {
			end = headers[i+1][0]
		}
		if tool := body[h[2]:h[3]]; tool != "*" {
			arms = append(arms, launcherArm{tool, open + h[0], open + end})
		}
	}
	return arms
}

func managerMatch(t *testing.T) *regexp.Regexp {
	t.Helper()
	managers, ok := loadConfig(t)["customManagers"].([]any)
	if !ok || len(managers) != 1 {
		t.Fatal("renovate.json5 has no single custom manager")
	}
	strs, _ := managers[0].(map[string]any)["matchStrings"].([]any)
	if len(strs) != 1 {
		t.Fatal("the custom manager has no single match string")
	}
	pattern, _ := strs[0].(string)
	re, err := regexp.Compile(pattern)
	if err != nil {
		t.Fatal(err)
	}
	return re
}

// metadataScript cuts the launcher before its cache lookup, as TestGitCliffPin
// does, so it resolves a tool's pins and stops.
func metadataScript(t *testing.T, script string) string {
	t.Helper()
	end := strings.Index(script, "\ndir=$root/.tools/$tool-$version")
	if end < 0 {
		t.Fatal("cannot locate launcher metadata boundary")
	}
	script = strings.Replace(script[:end], `"$(uname -s)-$(uname -m)"`, `"$T70_PLATFORM"`, 1)
	return script + "\nprintf '%s\\n' \"$version\" \"$for_version\" \"$sum\" \"$asset_id\" \"$download_url\"\n"
}

func runMetadata(t *testing.T, script, tool, uname string) (stdout, stderr string, code int) {
	t.Helper()
	cmd := exec.Command("sh", "-s", "--", tool)
	cmd.Stdin = strings.NewReader(script)
	cmd.Env = append(os.Environ(), "T70_PLATFORM="+uname)
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	err := cmd.Run()
	if err != nil {
		exitErr, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatal(err)
		}
		code = exitErr.ExitCode()
	}
	return out.String(), errOut.String(), code
}

func TestPinnedUpdateManagers(t *testing.T) {
	t.Run("config", func(t *testing.T) {
		config := loadConfig(t)
		want := decodeJSON(t, wantConfigPreserved).(map[string]any)
		want["enabledManagers"] = []any{"gomod", "github-actions", "renovate-config", "custom.regex"}
		want["commitTrailers"] = []any{"Task: T-70"}
		want["customManagers"] = decodeJSON(t, wantCustomManagers)
		if len(config) != len(want) {
			t.Errorf("renovate.json5 has %d top-level keys, want %d", len(config), len(want))
		}
		for key, wantValue := range want {
			if got, ok := config[key]; !ok || !reflect.DeepEqual(got, wantValue) {
				t.Errorf("%s = %#v, want %#v", key, got, wantValue)
			}
		}
		for key := range config {
			if _, ok := want[key]; !ok {
				t.Errorf("unexpected top-level key %q", key)
			}
		}
	})

	script := readLauncher(t)
	arms := launcherArms(t, script)
	cut := metadataScript(t, script)

	t.Run("tracked", func(t *testing.T) {
		if len(arms) != len(supportedPlatforms) {
			t.Fatalf("launcher has %d tool arms, want %d", len(arms), len(supportedPlatforms))
		}
		re := managerMatch(t)
		matches := re.FindAllStringSubmatchIndex(script, -1)
		if len(matches) != len(arms) {
			t.Fatalf("the match string finds %d matches in the launcher, want one per tool arm (%d)", len(matches), len(arms))
		}
		depName, currentValue := re.SubexpIndex("depName"), re.SubexpIndex("currentValue")
		if depName < 0 || currentValue < 0 {
			t.Fatal("the match string has no depName or currentValue group")
		}
		repo := regexp.MustCompile(`url=https://api\.github\.com/repos/(\S+?)/releases/`)
		resolved := regexp.MustCompile(`(?m)^\tversion=(\S+)$`)
		for _, arm := range arms {
			t.Run(arm.tool, func(t *testing.T) {
				text := script[arm.start:arm.end]
				var inArm [][]int
				for _, m := range matches {
					if m[0] >= arm.start && m[1] <= arm.end {
						inArm = append(inArm, m)
					}
				}
				if len(inArm) != 1 {
					t.Fatalf("%d matches inside the %s arm, want 1", len(inArm), arm.tool)
				}
				m := inArm[0]
				gotRepo := repo.FindStringSubmatch(text)
				version := resolved.FindStringSubmatch(text)
				if gotRepo == nil || version == nil {
					t.Fatalf("cannot read the %s arm's url or version", arm.tool)
				}
				if name := script[m[2*depName]:m[2*depName+1]]; name != gotRepo[1] {
					t.Errorf("depName = %q, want the url's repository %q", name, gotRepo[1])
				}
				current := script[m[2*currentValue]:m[2*currentValue+1]]
				if current != version[1] {
					t.Errorf("currentValue = %q, want the arm's version %q", current, version[1])
				}
				stdout, stderr, code := runMetadata(t, cut, arm.tool, "Linux-x86_64")
				if code != 0 {
					t.Fatalf("launcher exited %d: %s", code, stderr)
				}
				lines := strings.Split(stdout, "\n")
				if len(lines) < 5 || !strings.Contains(lines[4], "/download/v"+current+"/") {
					t.Errorf("download_url %q does not hold /download/v%s/", lines[len(lines)-2], current)
				}
			})
		}
	})

	hex64 := regexp.MustCompile(`^[0-9a-f]{64}$`)
	numeric := regexp.MustCompile(`^[0-9]+$`)
	for tool, supported := range supportedPlatforms {
		for _, up := range unamePlatforms {
			t.Run("verified/"+tool+"/"+up.uname, func(t *testing.T) {
				stdout, stderr, code := runMetadata(t, cut, tool, up.uname)
				if !contains(supported, up.platform) {
					if code != 2 || !strings.Contains(stderr, "no pinned "+tool+" for "+up.platform) {
						t.Fatalf("unsupported platform: exit %d, stderr %q", code, stderr)
					}
					return
				}
				if code != 0 {
					t.Fatalf("exit %d, stderr %q", code, stderr)
				}
				lines := strings.Split(stdout, "\n")
				if len(lines) < 4 || lines[1] != lines[0] || lines[0] == "" || !hex64.MatchString(lines[2]) || !numeric.MatchString(lines[3]) {
					t.Fatalf("version, for_version, sum and asset_id are not verified: %q", stdout)
				}
			})
		}
	}

	for tool, supported := range supportedPlatforms {
		t.Run("stale/"+tool, func(t *testing.T) {
			re := managerMatch(t)
			currentValue := re.SubexpIndex("currentValue")
			var arm launcherArm
			for _, a := range arms {
				if a.tool == tool {
					arm = a
				}
			}
			var span []int
			for _, m := range re.FindAllStringSubmatchIndex(script, -1) {
				if m[0] >= arm.start && m[1] <= arm.end {
					span = m
				}
			}
			if span == nil {
				t.Fatalf("no match in the %s arm to move", tool)
			}
			old := script[span[2*currentValue]:span[2*currentValue+1]]
			moved := script[:span[2*currentValue]] + "999.0.0" + script[span[2*currentValue+1]:]
			movedCut := metadataScript(t, moved)
			for _, up := range unamePlatforms {
				if !contains(supported, up.platform) {
					continue
				}
				t.Run(up.uname, func(t *testing.T) {
					stdout, stderr, code := runMetadata(t, movedCut, tool, up.uname)
					want := "pinned: " + tool + " 999.0.0 has no verified pin for " + up.platform + ": its SHA-256 and asset ID are for " + old
					if code != 2 || stdout != "" || !strings.Contains(stderr, want) {
						t.Fatalf("a moved version was not refused: exit %d, stdout %q, stderr %q, want %q", code, stdout, stderr, want)
					}
				})
			}
		})
	}
}

func contains(list []string, item string) bool {
	for _, s := range list {
		if s == item {
			return true
		}
	}
	return false
}
