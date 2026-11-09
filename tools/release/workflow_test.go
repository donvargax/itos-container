// Package release_test checks release wiring as parsed data. These assertions
// preserve the gate; only a real main-push Actions run proves its execution.
package release_test

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"go.yaml.in/yaml/v4"
)

type step struct {
	Name, ID, Uses, Run, If, Shell string
	With, Env                      map[string]string
	ContinueOnError                bool `yaml:"continue-on-error"`
}

type job struct {
	Needs       []string
	If, Uses    string
	Permissions map[string]string
	Concurrency map[string]any
	Steps       []step
}

type workflow struct {
	On          map[string]any
	Permissions map[string]string
	Concurrency map[string]any
	Jobs        map[string]job
}

func readYAML(t *testing.T, path string, into any) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", path))
	if err != nil {
		t.Fatal(err)
	}
	if err := yaml.Unmarshal(data, into); err != nil {
		t.Fatal(err)
	}
}

func equal(t *testing.T, name string, got, want any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s = %#v; want %#v", name, got, want)
	}
}

func TestReleaseWorkflow(t *testing.T) {
	var ci, reusable workflow
	readYAML(t, ".github/workflows/ci.yml", &ci)
	caller, ok := ci.Jobs["release"]
	if !ok {
		t.Fatal("ci.yml has no active release caller")
	}
	readYAML(t, ".github/workflows/release.yml", &reusable)

	slices.Sort(caller.Needs)
	equal(t, "release prerequisites", caller.Needs, []string{"ci", "engine", "platform"})
	equal(t, "release predicate", caller.If, "${{ success() && github.event_name == 'push' && github.ref == 'refs/heads/main' }}")
	equal(t, "release callee", caller.Uses, "./.github/workflows/release.yml")
	write := map[string]string{"contents": "write", "id-token": "write", "attestations": "write"}
	equal(t, "caller permissions", caller.Permissions, write)
	equal(t, "top-level permissions", ci.Permissions, map[string]string{"contents": "read", "actions": "read"})
	for name, j := range ci.Jobs {
		if name != "release" && len(j.Permissions) != 0 {
			t.Errorf("%s unexpectedly overrides read permissions: %v", name, j.Permissions)
		}
	}
	equal(t, "ci concurrency", ci.Concurrency, map[string]any{
		"group": "ci-${{ github.ref }}", "cancel-in-progress": "${{ github.event_name == 'pull_request' }}",
	})
	for _, name := range []string{"platform-macos", "platform-windows"} {
		equal(t, name+" remains manual", ci.Jobs[name].If, "${{ github.event_name == 'workflow_dispatch' }}")
	}
	equal(t, "engine prerequisite", ci.Jobs["ci"].Needs, []string{"engine"})
	ciSteps := ci.Jobs["ci"].Steps
	if len(ciSteps) == 0 {
		t.Fatal("ci job has no engine gate")
	}
	equal(t, "engine gate", ciSteps[0].Run, "test \"$ENGINE_RESULT\" = success")
	equal(t, "engine result", ciSteps[0].Env["ENGINE_RESULT"], "${{ needs.engine.result }}")
	for _, command := range []string{`itos verify "$FROM" "$TO"`, `itos ci run "$FROM" "$TO"`} {
		if !slices.ContainsFunc(ciSteps, func(s step) bool { return s.Run == command }) {
			t.Errorf("ci lacks %s", command)
		}
	}
	if !slices.ContainsFunc(ci.Jobs["platform"].Steps, func(s step) bool { return s.Run == "go test ./... -count=1" }) {
		t.Error("automatic Linux test suite is missing")
	}

	equal(t, "reusable triggers", reusable.On, map[string]any{"workflow_call": nil})
	equal(t, "reusable job count", len(reusable.Jobs), 1)
	release := reusable.Jobs["release"]
	equal(t, "reusable permissions", release.Permissions, write)
	equal(t, "release concurrency", release.Concurrency, map[string]any{"group": "release", "cancel-in-progress": false})
	steps := release.Steps
	if len(steps) != 10 {
		t.Fatalf("release has %d steps; want checkout, Go, version, itos, CLI, tag, draft, attestation, notes, publish", len(steps))
	}
	if !strings.HasPrefix(steps[0].Uses, "actions/checkout@") || !strings.HasPrefix(steps[1].Uses, "actions/setup-go@") {
		t.Fatal("checkout and Go setup must precede Version")
	}
	equal(t, "checkout SHA", steps[0].With["ref"], "${{ github.sha }}")
	equal(t, "checkout history", steps[0].With["fetch-depth"], "0")
	equal(t, "Go version", steps[1].With["go-version-file"], "go.mod")
	equal(t, "Version id", steps[2].ID, "version")
	equal(t, "Version command and output protocol", steps[2].Run,
		"go run ./tools/bin/release-version > \"$RUNNER_TEMP/version.txt\"\n"+
			"cat \"$RUNNER_TEMP/version.txt\" >> \"$GITHUB_OUTPUT\"\ncat \"$RUNNER_TEMP/version.txt\"\n")
	for i, s := range steps {
		if s.ContinueOnError || (s.Shell != "" && s.Shell != "bash") {
			t.Errorf("step %d could suppress failure: %+v", i, s)
		}
		if i > 2 {
			equal(t, "nonempty next for "+s.Name+s.Uses, s.If, "steps.version.outputs.next != ''")
		} else {
			equal(t, "unconditional preparation", s.If, "")
		}
	}
	equal(t, "ordered release steps", []string{steps[5].Name, steps[6].Name, steps[7].Name, steps[8].Name, steps[9].Name},
		[]string{"Tag", "GoReleaser", "Attest", "Notes", "Publish"})
	equal(t, "local immutable tag", steps[5].Run, `git tag "$TAG" "$GITHUB_SHA"`)
	equal(t, "tag version", steps[5].Env["TAG"], "v${{ steps.version.outputs.next }}")
	equal(t, "pinned draft builder", steps[6].Run, "tools/bin/pinned goreleaser release --clean")
	equal(t, "builder token", steps[6].Env["GITHUB_TOKEN"], "${{ github.token }}")
	if !strings.HasPrefix(steps[7].Uses, "actions/attest-build-provenance@") {
		t.Error("archive attestation is missing")
	}
	equal(t, "attestation subjects", steps[7].With["subject-checksums"], "dist/goreleaser/checksums.txt")
	equal(t, "notes inputs", steps[8].Env, map[string]string{
		"LAST": "${{ steps.version.outputs.last }}", "NEXT": "${{ steps.version.outputs.next }}",
	})
	equal(t, "notes command", steps[8].Run,
		"go run ./tools/bin/release-notes -version \"$NEXT\" -from \"$LAST\" > \"$RUNNER_TEMP/notes.md\"\n"+
			"cat \"$RUNNER_TEMP/notes.md\" >> \"$GITHUB_STEP_SUMMARY\"\n")
	equal(t, "publication", steps[9].Run, `gh release edit "$TAG" --notes-file "$RUNNER_TEMP/notes.md" --draft=false --latest`)
	equal(t, "publication environment", steps[9].Env, map[string]string{
		"GH_TOKEN": "${{ github.token }}", "GH_REPO": "${{ github.repository }}", "TAG": "v${{ steps.version.outputs.next }}",
	})
	t.Run("archives", testArchives)
}

func testArchives(t *testing.T) {
	var config struct {
		ProjectName string `yaml:"project_name"`
		Builds      []struct {
			ID, Main, Binary string
			Goos, Goarch     []string
			Ldflags          []string
			Ignore           []struct{ Goos, Goarch string }
		}
		Archives []struct {
			IDs             []string
			Formats         []string
			NameTemplate    string `yaml:"name_template"`
			FormatOverrides []struct {
				Goos    string
				Formats []string
			} `yaml:"format_overrides"`
			Files []struct{ Src string }
		}
		Checksum struct {
			NameTemplate string `yaml:"name_template"`
			Algorithm    string
		}
		Release struct {
			Github               map[string]string
			Draft                bool
			ReplaceExistingDraft bool   `yaml:"replace_existing_draft"`
			TargetCommitish      string `yaml:"target_commitish"`
		}
	}
	readYAML(t, ".goreleaser.yaml", &config)
	equal(t, "project", config.ProjectName, "itos-container")
	if len(config.Builds) != 1 || len(config.Archives) != 1 {
		t.Fatal("want one binary and its archive definition")
	}
	b := config.Builds[0]
	equal(t, "binary ID", b.ID, "itos-container")
	equal(t, "binary", b.Binary, "itos-container")
	equal(t, "entry point", b.Main, "./cmd/itos-container")
	var targets []string
	for _, os := range b.Goos {
		for _, arch := range b.Goarch {
			if !slices.ContainsFunc(b.Ignore, func(ignore struct{ Goos, Goarch string }) bool { return ignore.Goos == os && ignore.Goarch == arch }) {
				targets = append(targets, os+"/"+arch)
			}
		}
	}
	slices.Sort(targets)
	equal(t, "five archive targets", targets, []string{"darwin/amd64", "darwin/arm64", "linux/amd64", "linux/arm64", "windows/amd64"})
	for _, stamp := range []string{"-X github.com/donvargax/itos-container/internal/version.stamp={{ .Version }}", "-X github.com/donvargax/itos-container/internal/version.commit={{ .FullCommit }}"} {
		if !strings.Contains(strings.Join(b.Ldflags, " "), stamp) {
			t.Errorf("missing binary stamp %s", stamp)
		}
	}
	a := config.Archives[0]
	equal(t, "archive binary", a.IDs, []string{"itos-container"})
	equal(t, "archive names", a.NameTemplate, "itos-container-{{ .Version }}-{{ .Os }}-{{ .Arch }}")
	equal(t, "archive format", a.Formats, []string{"tar.gz"})
	if len(a.FormatOverrides) != 1 {
		t.Fatal("want exactly one Windows zip override")
	}
	equal(t, "zip target", a.FormatOverrides[0].Goos, "windows")
	equal(t, "zip format", a.FormatOverrides[0].Formats, []string{"zip"})
	var files []string
	for _, file := range a.Files {
		files = append(files, file.Src)
	}
	equal(t, "archive notices", files, []string{"LICENSE", "README.md", "THIRD_PARTY_NOTICES.md"})
	equal(t, "checksums file", config.Checksum.NameTemplate, "checksums.txt")
	equal(t, "checksum algorithm", config.Checksum.Algorithm, "sha256")
	equal(t, "release repository", config.Release.Github, map[string]string{"owner": "donvargax", "name": "itos-container"})
	equal(t, "draft", config.Release.Draft, true)
	equal(t, "draft replacement", config.Release.ReplaceExistingDraft, true)
	equal(t, "draft target", config.Release.TargetCommitish, "{{ .FullCommit }}")
}
