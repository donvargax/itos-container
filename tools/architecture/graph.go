package architecture

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
)

// Checks selects structural rules independently; all four are the default.
type Checks struct{ Siblings, Outside, Linked, Toplevel bool }

// Inspect returns named violations, or an inspection error. A load failure never passes.
func Inspect(ctx context.Context, dir string, checks Checks) ([]string, error) {
	if _, err := Rules(filepath.Join(dir, ".golangci.yml")); err != nil {
		return nil, err
	}
	module, pkgs, err := Load(ctx, dir)
	if err != nil {
		return nil, err
	}
	var violations []string
	for _, p := range pkgs {
		path := strings.TrimPrefix(p.ImportPath, module+"/")
		from := classify(path)
		if checks.Toplevel && from.kind == "" {
			violations = append(violations, fmt.Sprintf("toplevel: %s: unclassified package", p.ImportPath))
		}
		if checks.Toplevel && (from.kind == "domain" || from.kind == "port") {
			for _, file := range p.IgnoredGoFiles {
				violations = append(violations, fmt.Sprintf("toplevel: %s: ignored Go file %s; domains and ports must be inspected on every build", p.ImportPath, file))
			}
		}
		groups := [][]string{p.Imports, p.TestImports, p.XTestImports}
		for n, imports := range groups {
			for _, imp := range imports {
				test := n > 0
				if test && imp == p.ImportPath {
					continue
				} // external test's package under test
				local := strings.HasPrefix(imp, module+"/")
				target := strings.TrimPrefix(imp, module+"/")
				to := classify(target)
				arrow := fmt.Sprintf("%s imports %s", p.ImportPath, imp)
				if test {
					arrow += " from tests"
				}
				if checks.Toplevel && ignoredImport(imp) {
					violations = append(violations, "toplevel: "+arrow+"; ignored import path element")
				}
				if checks.Toplevel && !test && from.kind != "support" && support(imp) {
					violations = append(violations, "toplevel: "+arrow+"; production imports test support")
				}
				if !local {
					continue
				}
				if checks.Outside && isSlice(to) && !isSlice(from) && from.kind != "cmd" {
					violations = append(violations, "outside: "+arrow+"; only cmd may import a slice from outside slices")
				}
				if checks.Toplevel && under(target, "internal") && from.kind == "outside" && !releaseHelperEdge(path, target, test) {
					violations = append(violations, "toplevel: "+arrow+"; outside packages cannot import internal")
				}
				if checks.Siblings && !allowed(from, to, target, test) {
					violations = append(violations, "siblings: "+arrow+"; forbidden owner or layer arrow")
				}
			}
		}
	}
	if checks.Toplevel {
		excluded, err := excludedPureFiles(dir, module, pkgs)
		if err != nil {
			return nil, err
		}
		violations = append(violations, excluded...)
		refusals, err := Suppressions(dir)
		if err != nil {
			return nil, err
		}
		violations = append(violations, refusals...)
	}
	if checks.Linked {
		mains := 0
		for _, p := range pkgs {
			path := strings.TrimPrefix(p.ImportPath, module+"/")
			if p.Name != "main" || !under(path, "cmd") {
				continue
			}
			mains++
			deps, err := List(ctx, dir, "-deps", p.ImportPath)
			if err != nil {
				return nil, err
			}
			for _, dep := range deps {
				if support(dep.ImportPath) {
					violations = append(violations, fmt.Sprintf("linked: %s links %s; binary links test support", p.ImportPath, dep.ImportPath))
				}
			}
		}
		if mains == 0 {
			return nil, fmt.Errorf("empty production main target list under cmd")
		}
	}
	return violations, nil
}

// releaseHelperEdge is the exact q-44 exception for T-25's two build tools.
// No role or prefix is widened; test support remains unavailable to production.
func releaseHelperEdge(from, target string, test bool) bool {
	if from != "tools/bin/release-version" && from != "tools/bin/release-notes" {
		return false
	}
	return target == "internal/release" || (test && target == "internal/release/releasetest")
}

func allowed(from, to role, target string, test bool) bool {
	if from.kind == "cmd" || from.kind == "outside" || from.kind == "" {
		return true
	}
	if to.kind == "support" {
		if !test && from.kind != "support" {
			return false
		}
		return from.owner == to.owner || (isEngine(to) && (isSlice(from) || isEngine(from)))
	}
	switch from.kind {
	case "cli", "version":
		return false
	case "release":
		return to.kind == "release"
	}
	if !isSlice(from) && !isEngine(from) {
		return from.kind == "support" && from.owner == to.owner
	}
	if isSlice(to) && from.owner != to.owner {
		return false
	}
	// Engine domains and ports are the only public project APIs across owners.
	if isEngine(to) && from.owner != to.owner {
		if from.kind == "port" || from.kind == "support" {
			return to.kind == "port"
		}
		return to.kind == "domain" || to.kind == "port"
	}
	if from.kind == "command" && to.kind == "cli" {
		return true
	}
	if from.owner != to.owner {
		return false
	}
	switch from.kind {
	case "domain":
		return to.kind == "domain" || to.kind == "port"
	case "port", "support":
		return to.kind == "port"
	case "command":
		return to.kind == "domain" || to.kind == "port" || under(target, from.root)
	case "adapter":
		return to.kind == "domain" || to.kind == "port" || under(target, from.root)
	}
	return false
}
