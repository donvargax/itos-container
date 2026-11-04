// Compile-first mechanics adapted from itos-container T-10, f6f339c (AGPL-3.0).
// Ownership/linked graph probes follow itos-cc's architecture-check design.
package architecture_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const module = "github.com/donvargax/itos-container"

type probe struct {
	Name, File, Import, Rule, Kind, Source string
	Files                                  map[string]string
	Flags                                  []string
	CompileFiles                           []string
}
type issue struct {
	FromLinter, Text string
	Pos              struct{ Filename string }
}

func root(t *testing.T) string {
	t.Helper()
	r, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func read(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
func write(t *testing.T, dir, name, data string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
}
func command(t *testing.T, dir, program string, args ...string) (string, int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	c := exec.CommandContext(ctx, program, args...)
	c.Dir = dir
	for _, env := range os.Environ() {
		name, _, _ := strings.Cut(env, "=")
		if strings.HasPrefix(name, "GIT_") || name == "GOWORK" || name == "GOFLAGS" {
			continue
		}
		c.Env = append(c.Env, env)
	}
	c.Env = append(c.Env, "GOWORK=off", "GOFLAGS=-mod=readonly")
	b, err := c.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("infrastructure: command timed out: %s %v\n%s", program, args, b)
	}
	if err == nil {
		return string(b), 0
	}
	var exit *exec.ExitError
	if !errors.As(err, &exit) {
		t.Fatalf("infrastructure: %v", err)
	}
	return string(b), exit.ExitCode()
}
func must(t *testing.T, dir, program string, args ...string) string {
	t.Helper()
	out, code := command(t, dir, program, args...)
	if code != 0 {
		t.Fatalf("infrastructure: %s %v exit %d\n%s", program, args, code, out)
	}
	return out
}
func fixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	r := root(t)
	for _, name := range []string{"go.mod", "go.sum", ".golangci.yml"} {
		write(t, dir, name, read(t, filepath.Join(r, name)))
	}
	write(t, dir, "cmd/control/main.go", "package main\nfunc main() {}\n")
	return dir
}
func source(file, imp string) string {
	pkg := filepath.Base(filepath.Dir(file))
	if strings.HasSuffix(file, "external_test.go") {
		pkg += "_test"
	}
	text := "package " + pkg + "\n"
	if imp != "" {
		text += fmt.Sprintf("import _ %q\n", imp)
	}
	return text
}
func TestPolicy(t *testing.T) {
	r := root(t)
	linter := strings.TrimSpace(must(t, r, filepath.Join(r, "tools/bin/pinned"), "--path", "golangci-lint"))
	var probes []probe
	for _, file := range []string{"cases.json", "review-cases.json"} {
		var cases []probe
		if err := json.Unmarshal([]byte(read(t, filepath.Join(r, "tools/testdata/architecture", file))), &cases); err != nil {
			t.Fatal(err)
		}
		probes = append(probes, cases...)
	}
	baseline := os.Getenv("T14_BASELINE") == "1"
	checker := os.Getenv("ARCHITECTURE_CHECKER")
	if !baseline && checker == "" {
		t.Fatal("infrastructure: ARCHITECTURE_CHECKER is required (use --fixtures)")
	}
	for _, p := range probes {
		t.Run(p.Name, func(t *testing.T) {
			dir := fixture(t)
			for file, text := range p.Files {
				write(t, dir, file, text)
			}
			imp := strings.ReplaceAll(p.Import, "$module", module)
			text := p.Source
			if text == "" {
				text = source(p.File, imp)
			}
			write(t, dir, p.File, strings.ReplaceAll(text, "$module", module))
			if strings.HasSuffix(p.File, "external_test.go") {
				base := filepath.ToSlash(filepath.Join(filepath.Dir(p.File), "base.go"))
				write(t, dir, base, source(base, ""))
			}
			if strings.HasPrefix(imp, module+"/") {
				folder := strings.TrimPrefix(imp, module+"/")
				if folder != filepath.ToSlash(filepath.Dir(p.File)) {
					write(t, dir, folder+"/base.go", source(folder+"/base.go", ""))
				}
			}
			// A type error (including an external test's imports) is never a refusal.
			must(t, dir, "go", "test", "-run=^$", "./...")
			// Explicit filenames also compile probes excluded by build constraints.
			for _, file := range p.CompileFiles {
				must(t, dir, "go", "test", "-run=^$", file)
			}
			report := filepath.Join(dir, "lint.json")
			out, code := command(t, dir, linter, "run", "--config", filepath.Join(dir, ".golangci.yml"), "--enable-only", "depguard", "--issues-exit-code", "7", "--max-issues-per-linter", "0", "--max-same-issues", "0", "--uniq-by-line=false", "--output.json.path", report, "--output.text.path", os.DevNull, "--show-stats=false", "./...")
			if code != 0 && code != 7 {
				t.Fatalf("infrastructure: lint exit %d\n%s", code, out)
			}
			var result struct{ Issues []issue }
			if err := json.Unmarshal([]byte(read(t, report)), &result); err != nil {
				t.Fatal(err)
			}
			if p.Kind == "lint" {
				expected := strings.Split(p.Rule, ",")
				found := map[string]bool{}
				for _, i := range result.Issues {
					if i.FromLinter != "depguard" || !strings.HasSuffix(filepath.ToSlash(i.Pos.Filename), p.File) || !strings.Contains(i.Text, "import '"+imp+"' is not allowed") {
						t.Fatalf("infrastructure: unexpected diagnostic %+v", i)
					}
					recognized := false
					for _, rule := range expected {
						if strings.Contains(i.Text, "list '"+rule+"'") {
							found[rule] = true
							recognized = true
						}
					}
					if !recognized {
						t.Fatalf("infrastructure: unexpected rule diagnostic %+v", i)
					}
				}
				for _, rule := range expected {
					if !found[rule] {
						t.Errorf("expected refusal: %s imports %s; rule %s; got none", p.File, imp, rule)
					}
				}
				return
			}
			if p.Kind == "allow" && len(result.Issues) != 0 {
				t.Fatalf("allowed arrow refused: %+v", result.Issues)
			}
			if baseline {
				if p.Kind != "allow" {
					t.Errorf("expected refusal: %s imports %s; rule %s; current linter produced %+v", p.File, imp, p.Rule, result.Issues)
				}
				return
			}
			flags := p.Flags
			if len(flags) == 0 {
				flags = []string{"--siblings", "--outside", "--linked", "--toplevel"}
			}
			out, code = command(t, dir, checker, flags...)
			if p.Kind == "allow" {
				if code != 0 {
					t.Fatalf("allowed arrow refused: exit %d\n%s", code, out)
				}
				return
			}
			if code == 2 {
				t.Fatalf("infrastructure: inspection failure\n%s", out)
			}
			importer := module + "/" + filepath.ToSlash(filepath.Dir(p.File))
			if p.Kind == "suppression" {
				importer = p.File
			}
			if code != 1 || !strings.Contains(out, importer) || !strings.Contains(out, p.Rule) || (imp != "" && !strings.Contains(out, imp)) {
				t.Errorf("expected refusal: %s imports %s; rule %s; exit %d\n%s", importer, imp, p.Rule, code, out)
			}
		})
	}
}
