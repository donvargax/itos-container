// Command check runs the repository's architecture inspections and explicit fixtures.
package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/donvargax/itos-container/tools/architecture"
)

func main() { os.Exit(run(os.Args[1:])) }
func run(args []string) int {
	ctx := context.Background()
	dir, err := os.Getwd()
	if err != nil {
		return inspection(err)
	}
	var checks architecture.Checks
	fixtures, domains, staged := false, false, false
	base, head := "", ""
	selection := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--siblings":
			checks.Siblings = true
		case "--outside":
			checks.Outside = true
		case "--linked":
			checks.Linked = true
		case "--toplevel":
			checks.Toplevel = true
		case "--domains":
			domains = true
		case "--fixtures":
			fixtures = true
		case "--fixtures-if-staged":
			staged = true
			selection = true
		case "--fixtures-if-changed":
			if i+2 >= len(args) {
				return inspection(fmt.Errorf("usage: --fixtures-if-changed <base> <head>"))
			}
			base, head = args[i+1], args[i+2]
			selection = true
			i += 2
		default:
			return inspection(fmt.Errorf("usage: unknown argument %q", args[i]))
		}
	}
	if len(args) == 0 {
		checks = architecture.Checks{Siblings: true, Outside: true, Linked: true, Toplevel: true}
	}
	if selection {
		selected, err := architecture.Selected(ctx, dir, staged, base, head)
		if err != nil {
			return inspection(err)
		}
		fixtures = fixtures || selected
	}
	if domains {
		if _, err := architecture.Rules(filepath.Join(dir, ".golangci.yml")); err != nil {
			return inspection(err)
		}
		module, pkgs, err := architecture.Load(ctx, dir)
		if err != nil {
			return inspection(err)
		}
		paths := architecture.Domains(pkgs, module)
		if len(paths) == 0 {
			return inspection(fmt.Errorf("no domain packages"))
		}
		fmt.Println(strings.Join(paths, "\n"))
	}
	if checks != (architecture.Checks{}) {
		violations, err := architecture.Inspect(ctx, dir, checks)
		if err != nil {
			return inspection(err)
		}
		for _, v := range violations {
			fmt.Fprintln(os.Stderr, "architecture-check:", v)
		}
		if len(violations) > 0 {
			return 1
		}
	}
	if fixtures {
		executable, err := os.Executable()
		if err != nil {
			return inspection(err)
		}
		c := exec.CommandContext(ctx, "go", "test", "./tools/testdata/architecture", "-count=1", "-v")
		// Fixtures create independent repositories; hook-local Git variables
		// must never direct their graph/selection probes at the caller's index.
		for _, env := range os.Environ() {
			name, _, _ := strings.Cut(env, "=")
			if !strings.HasPrefix(name, "GIT_") {
				c.Env = append(c.Env, env)
			}
		}
		c.Env = append(c.Env, "ARCHITECTURE_CHECKER="+executable)
		c.Stdout, c.Stderr = os.Stdout, os.Stderr
		if err := c.Run(); err != nil {
			return 1
		}
	}
	return 0
}
func inspection(err error) int {
	fmt.Fprintln(os.Stderr, "architecture-check: inspection:", err)
	return 2
}
