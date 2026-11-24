package features

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
)

// codexLogicalLines joins `\` continuations, cuts ` # ` comments and drops comment-only lines,
// so a comment cannot supply a call or a guard.
func codexLogicalLines(body string) []string {
	var lines []string
	for _, line := range strings.Split(strings.ReplaceAll(body, "\\\n", ""), "\n") {
		line = strings.TrimSpace(strings.SplitN(line, " # ", 2)[0])
		if line != "" && !strings.HasPrefix(line, "#") {
			lines = append(lines, line)
		}
	}
	return lines
}

func codexFind(lines []string, prefix string) (first, count int) {
	first = -1
	for i, line := range lines {
		if strings.HasPrefix(line, prefix) {
			if first < 0 {
				first = i
			}
			count++
		}
	}
	return first, count
}

var codexGuarded = regexp.MustCompile(`\|\| fail `)

func codexFixtureGuards() map[string][]string {
	return map[string][]string{
		"prepare": {`codex_fixture "$port" "$fixture_uid" "$fixture_gid"`, `codex_handoff "$fixture_uid" "$fixture_gid"`},
		"codex_fixture": {
			`mkdir -- `, `curl --fail --location `,
			`codex_archive_verify "$codex_download/codex-x86_64-unknown-linux-musl.tar.gz" "$workspace/features/testdata/codex-fixture/SHA256SUMS" "$codex_context"`,
			`printf '%s\n' "$codex_repository" >"$state/codex.repository"`, `printf '%s\n' "$codex_tag" >"$state/codex.tag"`,
			`d image build `, `codex_built=$(cat "$state/codex.build.iid")`, `codex_base=$(cat "$state/base.iid")`,
			`codex_registry=$(cat "$state/registry.iid")`, `digest_valid "$codex_built"`, `d image push "$codex_tag"`,
			`d image save `, `tar -xOf `, `codex_config_path=$(daemon_config_path `, `codex_config=$(daemon_config_digest `,
			`curl --fail --silent `, `codex_digest=$(validate_manifest `, `printf '%s\n' "$codex_digest" >"$state/codex.digest"`,
			`remove_codex_ref tag`, `remove_codex_ref digest`, `d image pull `, `d image inspect `, `validate_cached `,
		},
		"codex_handoff": {
			`codex_repository=$(cat `, `codex_digest=$(cat `, `codex_built=$(cat `, `d image inspect `, `validate_cached `,
			`printf '%s\n' "$codex_reference" >"$state/codex.reference"`, `printf '%s\n' "$codex_built" >"$state/codex.iid"`,
		},
	}
}

// codexFixtureOrderIssues reads the driver's executable lines only. It requires the preparation,
// handoff and cleanup order the T-81 protocol depends on, and a guard on every step.
func codexFixtureOrderIssues(driver string) []string {
	var issues []string
	add := func(format string, args ...any) { issues = append(issues, fmt.Sprintf(format, args...)) }
	functions := map[string][]string{}
	for _, name := range []string{"prepare", "cleanup", "codex_fixture", "codex_handoff"} {
		functions[name] = codexLogicalLines(fixtureFunction(driver, name))
		if len(functions[name]) == 0 {
			add("missing function %s", name)
		}
	}
	for name, forms := range codexFixtureGuards() {
		for _, form := range forms {
			first, _ := codexFind(functions[name], form)
			if first < 0 {
				add("missing executable %s: %s", name, form)
			}
			for _, line := range functions[name] {
				if strings.HasPrefix(line, form) && !codexGuarded.MatchString(line) {
					add("unguarded %s: %s", name, form)
				}
			}
		}
	}
	// before requires both steps, the first occurrence of a before the first of b.
	before := func(name, a, b string) {
		x, _ := codexFind(functions[name], a)
		y, _ := codexFind(functions[name], b)
		if x < 0 || y < 0 || x > y {
			add("%s: %s must come before %s", name, a, b)
		}
	}
	const (
		verify = `codex_archive_verify "$codex_download/`
		build  = `d image build `
		pull   = `d image pull `
		write  = `printf '%s\n' "$codex_`
	)
	before("codex_fixture", `curl --fail --location `, verify)
	before("codex_fixture", verify, write+`repository" >"$state/codex.repository"`)
	before("codex_fixture", write+`repository" >"$state/codex.repository"`, build)
	before("codex_fixture", write+`tag" >"$state/codex.tag"`, build)
	before("codex_fixture", verify, build)
	if first, _ := codexFind(functions["codex_fixture"], build); first < 0 ||
		!strings.Contains(functions["codex_fixture"][first], `--iidfile "$state/codex.build.iid"`) ||
		!strings.Contains(functions["codex_fixture"][first], `"$codex_context"`) {
		add("codex_fixture: the build must use --iidfile \"$state/codex.build.iid\" and \"$codex_context\"")
	}
	digestWrite := write + `digest" >"$state/codex.digest"`
	before("codex_fixture", `codex_digest=$(validate_manifest `, digestWrite)
	before("codex_fixture", digestWrite, `remove_codex_ref tag`)
	before("codex_fixture", digestWrite, `remove_codex_ref digest`)
	before("codex_fixture", digestWrite, pull)
	before("codex_fixture", pull, `validate_cached "$state/codex-image.json"`)
	// Preparation: codex_fixture between the two removals, codex_handoff after the product warm-up.
	call, handoffCall := `codex_fixture "$port" "$fixture_uid" "$fixture_gid"`, `codex_handoff "$fixture_uid" "$fixture_gid"`
	for _, form := range []string{call, handoffCall} {
		if _, count := codexFind(functions["prepare"], form); count != 1 {
			add("prepare: %s must be called exactly once, not %d times", form, count)
		}
	}
	before("prepare", `remove_container smoke || fail`, call)
	before("prepare", call, `remove_container registry || fail`)
	before("prepare", `engine_warmup_product_run `, handoffCall)
	before("prepare", handoffCall, `printf 'image=%s`)
	// Handoff: validated first, written once and nowhere else.
	reference, iid := write+`reference" >"$state/codex.reference"`, write+`built" >"$state/codex.iid"`
	before("codex_handoff", `validate_cached `, reference)
	before("codex_handoff", reference, iid)
	for _, handoff := range []string{`>"$state/codex.reference"`, `>"$state/codex.iid"`} {
		if strings.Count(driver, handoff) != 1 || strings.Count(fixtureFunction(driver, "codex_handoff"), handoff) != 1 {
			add("the driver must hold exactly one handoff write %s, in codex_handoff", handoff)
		}
	}
	before("cleanup", `(remove_image_ref reference) || failed=1`, `(remove_codex_ref tag) || failed=1`)
	before("cleanup", `(remove_codex_ref tag) || failed=1`, `(remove_codex_ref digest) || failed=1`)
	return issues
}

// moveCodexLine moves the one physical line holding moved to just before the line holding anchor,
// or after the whole continued statement that starts at the anchor's line.
func moveCodexLine(t *testing.T, driver, moved, anchor string, after bool) string {
	t.Helper()
	lines := strings.Split(driver, "\n")
	at := -1
	for i, line := range lines {
		if strings.Contains(line, moved) {
			requireFixture(t, at < 0, "moved text appears twice: %s", moved)
			at = i
		}
	}
	requireFixture(t, at >= 0, "moved text is absent: %s", moved)
	line := lines[at]
	lines = append(lines[:at:at], lines[at+1:]...)
	target := -1
	for i, candidate := range lines {
		if strings.Contains(candidate, anchor) {
			target = i
			break
		}
	}
	requireFixture(t, target >= 0, "anchor is absent: %s", anchor)
	if after {
		for strings.HasSuffix(lines[target], `\`) {
			target++
		}
		target++
	}
	lines = append(lines[:target:target], append([]string{line}, lines[target:]...)...)
	return strings.Join(lines, "\n")
}

func testCodexOrderGuard(t *testing.T) {
	data, err := os.ReadFile("../tools/bin/engine-fixture")
	requireFixture(t, err == nil, "cannot read driver: %v", err)
	driver := string(data)
	requireFixture(t, len(codexFixtureOrderIssues(driver)) == 0, "unedited driver: %v", codexFixtureOrderIssues(driver))
	iid := `--iidfile "$state/codex.build.iid"`
	pull := `d image pull --platform linux/amd64 "$codex_reference"`
	edits := map[string]func() string{
		"repository_after_build": func() string { return moveCodexLine(t, driver, `>"$state/codex.repository"`, iid, true) },
		"tag_after_build":        func() string { return moveCodexLine(t, driver, `>"$state/codex.tag"`, iid, true) },
		"digest_after_tag_removal": func() string {
			return moveCodexLine(t, driver, `>"$state/codex.digest"`, `remove_codex_ref tag || fail`, true)
		},
		"digest_after_cold_pull": func() string { return moveCodexLine(t, driver, `>"$state/codex.digest"`, pull, true) },
		"iidfile_renamed":        func() string { return strings.Replace(driver, iid, `--iidfile "$state/codex.build.id"`, 1) },
		"reference_before_pull":  func() string { return moveCodexLine(t, driver, `>"$state/codex.reference"`, pull, false) },
		"iid_before_validation": func() string {
			return moveCodexLine(t, driver, `>"$state/codex.iid"`, `validate_cached "$state/codex-final-image.json"`, false)
		},
		"handoff_before_fixture": func() string {
			return moveCodexLine(t, driver, `codex_handoff "$fixture_uid"`, `codex_fixture "$port"`, false)
		},
		"verify_after_build":     func() string { return moveCodexLine(t, driver, `codex_archive_verify "$codex_download/`, iid, true) },
		"verify_guard_commented": func() string { return commentCodexGuard(t, driver, `SHA256SUMS" "$codex_context" || fail`) },
		"push_guard_commented":   func() string { return commentCodexGuard(t, driver, `d image push "$codex_tag" || fail`) },
		"iid_guard_commented":    func() string { return commentCodexGuard(t, driver, `>"$state/codex.iid" || fail`) },
		"fixture_guard_commented": func() string {
			return commentCodexGuard(t, driver, `codex_fixture "$port" "$fixture_uid" "$fixture_gid" || fail`)
		},
	}
	for name, edit := range edits {
		t.Run(name, func(t *testing.T) {
			edited := edit()
			requireFixture(t, edited != driver, "the edit did not change the driver")
			requireFixture(t, len(codexFixtureOrderIssues(edited)) > 0, "the edited driver was accepted")
		})
	}
}

// commentCodexGuard comments out `|| fail` in the one place the left text occurs.
func commentCodexGuard(t *testing.T, driver, left string) string {
	t.Helper()
	requireFixture(t, strings.Count(driver, left) == 1, "the driver must contain %s once", left)
	return strings.Replace(driver, left, strings.TrimSuffix(left, " || fail")+" # || fail", 1)
}
