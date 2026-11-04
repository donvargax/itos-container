// This executable exercises the real pinned depguard and coverage script in
// disposable modules. testdata keeps intentionally forbidden imports out of ./....
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type probe struct {
	Name    string
	File    string
	Package string
	Imports []string
	Denied  string
	Add     []string
	Module  string
	Source  string
}

type matrix struct {
	Packages []string
	Probes   []probe
}

type issue struct {
	FromLinter string
	Text       string
	Pos        struct{ Filename string }
}

type runner struct {
	root, temp, linter, generator string
	failures                      int
}

func main() {
	if len(os.Args) != 3 {
		fatal("usage: go run ./tools/testdata/slice-layout <root> <generator|original>")
	}
	root, err := filepath.Abs(os.Args[1])
	check(err)
	tmp, err := os.MkdirTemp("", "T-10-slice-fixtures-")
	check(err)
	// Return through run so even infrastructure failures remove the fixture tree.
	code := run(root, tmp, os.Args[2])
	check(os.RemoveAll(tmp))
	os.Exit(code)
}

func run(root, tmp, generator string) (code int) {
	defer func() {
		if problem := recover(); problem != nil {
			fmt.Fprintln(os.Stderr, "slice fixtures: infrastructure failure:", problem)
			code = 2
		}
	}()
	r := runner{root: root, temp: tmp, generator: generator}
	out, err := command(root, filepath.Join(root, "tools/bin/pinned"), "--path", "golangci-lint")
	checkOutput(err, out)
	r.linter = strings.TrimSpace(out)
	var cases matrix
	check(json.Unmarshal(read(filepath.Join(root, "tools/testdata/slice-layout/imports.json")), &cases))
	for _, p := range cases.Probes {
		r.lint(cases.Packages, p)
	}
	r.coverage("coverage-low", false, false, false)
	r.coverage("coverage-each", true, false, false)
	r.coverage("coverage-empty", false, true, false)
	r.coverage("coverage-no-statements", true, false, true)
	if r.failures != 0 {
		fmt.Fprintf(os.Stderr, "slice fixtures: %d assertions failed\n", r.failures)
		return 1
	}
	fmt.Printf("slice fixtures: %d import probes and 4 coverage probes passed\n", len(cases.Probes))
	return 0
}

func (r *runner) module(name, module string) string {
	dir := filepath.Join(r.temp, name)
	mod := string(read(filepath.Join(r.root, "go.mod")))
	first := strings.IndexByte(mod, '\n')
	write(filepath.Join(dir, "go.mod"), "module "+module+mod[first:])
	write(filepath.Join(dir, "go.sum"), string(read(filepath.Join(r.root, "go.sum"))))
	write(filepath.Join(dir, ".golangci.yml"), string(read(filepath.Join(r.root, ".golangci.yml"))))
	return dir
}

func (r *runner) lint(packages []string, p probe) {
	module := p.Module
	if module == "" {
		module = "github.com/donvargax/itos-container"
	}
	dir := r.module(p.Name, module)
	for _, pkg := range append(append([]string{}, packages...), p.Add...) {
		write(filepath.Join(dir, pkg, "base.go"), "package "+filepath.Base(pkg)+"\n")
	}
	source := "package " + p.Package + "\nimport (\n"
	for _, imp := range p.Imports {
		source += fmt.Sprintf("_ %q\n", strings.ReplaceAll(imp, "$module", module))
	}
	source += ")\n"
	if p.Source != "" {
		source = p.Source
	}
	write(filepath.Join(dir, p.File), source)
	// Depguard can use syntax-only loading. Prove that every probe compiles
	// independently so a type error or import cycle can never satisfy a denial.
	out, err := command(dir, "go", "test", "-run=^$", "./...")
	checkOutput(err, out)
	config := filepath.Join(dir, ".golangci.yml")
	if r.generator != "original" {
		config = filepath.Join(r.temp, p.Name+".yml")
		out, err = command(dir, r.generator, "config", dir)
		checkOutput(err, out)
		write(config, out)
		before := out
		out, err = command(dir, r.generator, "config", dir)
		checkOutput(err, out)
		if out != before {
			panic("configuration generation is not deterministic")
		}
	}
	report := filepath.Join(r.temp, p.Name+".json")
	out, err = command(dir, r.linter, "run", "--config", config, "--enable-only", "depguard",
		"--issues-exit-code", "7", "--max-issues-per-linter", "0", "--max-same-issues", "0",
		"--output.json.path", report, "--output.text.path", os.DevNull, "--show-stats=false", "./...")
	if err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) || exit.ExitCode() != 7 {
			checkOutput(err, out)
		}
	}
	var result struct{ Issues []issue }
	check(json.Unmarshal(read(report), &result))
	denied := strings.ReplaceAll(p.Denied, "$module", module)
	found := false
	for _, i := range result.Issues {
		file := filepath.ToSlash(i.Pos.Filename)
		if i.FromLinter != "depguard" || !(file == p.File || strings.HasSuffix(file, "/"+p.File)) {
			panic(fmt.Sprintf("%s: unexpected diagnostic %+v", p.Name, i))
		}
		if denied == "" || !strings.Contains(i.Text, "import '"+denied+"' is not allowed") {
			r.fail("%s: unexpected depguard refusal in %s: %s", p.Name, file, i.Text)
			continue
		}
		found = true
		fmt.Printf("PASS %s: %s: %s (depguard)\n", p.Name, p.File, i.Text)
	}
	if denied != "" && !found {
		r.fail("%s: %s: expected depguard refusal for %s; got none", p.Name, p.File, denied)
	} else if denied == "" && len(result.Issues) == 0 {
		fmt.Printf("PASS %s: %s accepts %s\n", p.Name, p.File, strings.Join(p.Imports, ", "))
	}
}

func (r *runner) coverage(name string, covered, empty, noStatements bool) {
	dir := r.module(name, "example.org/coverage")
	write(filepath.Join(dir, "tools/bin/domain-coverage"), string(read(filepath.Join(r.root, "tools/bin/domain-coverage"))))
	// Copy the real discovery helper when present, never reimplement its list
	// or the coverage script's statement arithmetic in this harness.
	files, err := filepath.Glob(filepath.Join(r.root, "tools/slice-layout/*.go"))
	check(err)
	for _, file := range files {
		write(filepath.Join(dir, "tools/slice-layout", filepath.Base(file)), string(read(file)))
	}
	if !empty {
		for _, pkg := range []string{"count/domain", "inspect/domain", "inspect/domain/nested"} {
			folder := filepath.Join(dir, "internal", pkg)
			write(filepath.Join(folder, "value.go"), "package "+filepath.Base(pkg)+"\n"+string(read(filepath.Join(r.root, "tools/testdata/slice-layout/value.go.txt"))))
			test := "full_test.go.txt"
			if !covered && pkg == "inspect/domain" {
				test = "partial_test.go.txt"
			}
			write(filepath.Join(folder, "value_test.go"), "package "+filepath.Base(pkg)+"\n"+string(read(filepath.Join(r.root, "tools/testdata/slice-layout/"+test))))
		}
		// These untested statements must not enter any domain's denominator.
		for _, pkg := range []string{"count/disk", "count/port/porttest", "count/command", "cli", "release", "count/domain/testdata/ignored", "count/domain/.tools/ignored"} {
			write(filepath.Join(dir, "internal", pkg, "value.go"), "package "+filepath.Base(pkg)+"\n"+string(read(filepath.Join(r.root, "tools/testdata/slice-layout/value.go.txt"))))
		}
		if noStatements {
			write(filepath.Join(dir, "internal/empty/domain/empty.go"), "package domain\nconst Value = 1\n")
		}
	}
	out, err := command(dir, "sh", "tools/bin/domain-coverage")
	want := []string{"./internal/count/domain 100.0%", "./internal/inspect/domain 100.0%", "./internal/inspect/domain/nested 100.0%"}
	if empty {
		want = []string{"no domain packages"}
	} else if noStatements {
		want = append(want, "./internal/empty/domain has no statements to measure")
	} else if !covered {
		want[1] = "./internal/inspect/domain 66.6%"
		want = append(want, "./internal/inspect/domain is at 66.6%, under the floor of 80%")
	}
	ok := (err == nil) == (covered && !empty && !noStatements)
	for _, fragment := range want {
		ok = ok && strings.Contains(out, fragment)
	}
	// A failing process alone is never evidence; require every expected figure
	// or the specific empty/missing-statements diagnostic from the real script.
	if !ok {
		r.fail("%s: expected %q; got error %v and output:\n%s", name, want, err, out)
		return
	}
	fmt.Printf("PASS %s:\n%s", name, out)
}

func (r *runner) fail(format string, args ...any) {
	r.failures++
	fmt.Fprintf(os.Stderr, "FAIL "+format+"\n", args...)
}

func command(dir, program string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, program, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=-mod=readonly")
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		panic(fmt.Sprintf("%s timed out: %s", program, out))
	}
	return string(out), err
}

func read(path string) []byte {
	data, err := os.ReadFile(path)
	check(err)
	return data
}

func write(path, data string) {
	check(os.MkdirAll(filepath.Dir(path), 0o755))
	check(os.WriteFile(path, []byte(data), 0o600))
}

func check(err error) {
	if err != nil {
		panic(err)
	}
}

func checkOutput(err error, out string) {
	if err != nil {
		panic(fmt.Sprintf("%v\n%s", err, out))
	}
}

func fatal(message string) {
	fmt.Fprintln(os.Stderr, message)
	os.Exit(2)
}
