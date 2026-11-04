package architecture

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// Package contains go list's import graph, including internal/external test arrows.
type Package struct {
	ImportPath, Name, Dir              string
	Imports, TestImports, XTestImports []string
	IgnoredGoFiles                     []string
	Module                             *struct {
		Path string
		Main bool
	}
}

// Run runs an inspection command in dir; a load/process failure stays an error.
func Run(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
	c := exec.CommandContext(ctx, name, args...) //nolint:gosec // internal callers select go or git and pass argument boundaries without a shell
	c.Dir = dir
	var stderr bytes.Buffer
	c.Stderr = &stderr
	b, err := c.Output()
	if err != nil {
		return nil, fmt.Errorf("%s %v: %w: %s", name, args, err, stderr.String())
	}
	return b, nil
}

// Load returns the current module identity and its packages, never an empty success.
func Load(ctx context.Context, dir string) (string, []Package, error) {
	pkgs, err := List(ctx, dir, "./...")
	if err != nil {
		return "", nil, err
	}
	// In workspace mode, go list -m reports every workspace module. Package
	// metadata identifies the module actually inspected by ./... in dir.
	mod := pkgs[0].Module
	if mod == nil || !mod.Main || mod.Path == "" {
		return "", nil, fmt.Errorf("no current module")
	}
	// Ordinary go list exposes TestImports but does not resolve them. Validate
	// the test graph as well, without treating synthetic test binaries as roles.
	if _, err = Run(ctx, dir, "go", "list", "-test", "-deps", "-buildvcs=false", "./..."); err != nil {
		return "", nil, err
	}
	for _, p := range pkgs {
		if p.Module == nil || !p.Module.Main || p.Module.Path != mod.Path {
			return "", nil, fmt.Errorf("package %s is not in current module %s", p.ImportPath, mod.Path)
		}
	}
	return mod.Path, pkgs, nil
}

// List decodes go list packages and propagates inspection errors.
func List(ctx context.Context, dir string, args ...string) ([]Package, error) {
	b, err := Run(ctx, dir, "go", append([]string{"list", "-json", "-buildvcs=false"}, args...)...)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(b))
	var pkgs []Package
	for {
		var p Package
		if err = decoder.Decode(&p); err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if p.ImportPath == "" || p.Name == "" || p.Dir == "" {
			return nil, fmt.Errorf("malformed package input")
		}
		pkgs = append(pkgs, p)
	}
	if len(pkgs) == 0 {
		return nil, fmt.Errorf("empty package target list")
	}
	return pkgs, nil
}

type role struct{ kind, owner, root string }

func under(path, root string) bool { return path == root || strings.HasPrefix(path, root+"/") }
func classify(path string) role {
	for _, name := range []string{"cli", "version", "release"} {
		if under(path, "internal/"+name) {
			if under(path, "internal/release/releasetest") {
				return role{kind: "support", owner: "release", root: "internal/release/releasetest"}
			}
			if support(path) {
				return role{}
			}
			return role{kind: name, owner: name, root: "internal/" + name}
		}
	}
	parts := strings.Split(path, "/")
	if len(parts) >= 4 && parts[0] == "internal" && (parts[1] == "slices" || parts[1] == "engine") {
		owner := strings.Join(parts[:3], "/")
		layer := parts[3]
		if layer == "adapters" || layer == "fakes" || layer == "porttest" || (parts[1] == "engine" && layer == "command") {
			return role{}
		}
		if layer == "port" && len(parts) >= 5 && parts[4] == "porttest" {
			return role{"support", owner, owner + "/port/porttest"}
		}
		if support(strings.Join(parts[4:], "/")) {
			return role{}
		}
		kind := layer
		if kind != "domain" && kind != "port" && kind != "command" {
			kind = "adapter"
		}
		return role{kind, owner, owner + "/" + layer}
	}
	if under(path, "cmd") {
		return role{kind: "cmd"}
	}
	if under(path, "features") || under(path, "tools") {
		return role{kind: "outside"}
	}
	return role{}
}

// ignoredImport names paths that go list ./... does not inspect as packages.
func ignoredImport(path string) bool {
	for _, part := range strings.Split(path, "/") {
		if part == "testdata" || strings.HasPrefix(part, "_") || strings.HasPrefix(part, ".") {
			return true
		}
	}
	return false
}

func isSlice(r role) bool  { return strings.HasPrefix(r.owner, "internal/slices/") }
func isEngine(r role) bool { return strings.HasPrefix(r.owner, "internal/engine/") }
func support(path string) bool {
	for _, part := range strings.Split(path, "/") {
		if part == "porttest" || part == "fakes" || part == "releasetest" {
			return true
		}
	}
	return false
}

// Domains returns actual slice/engine domain packages, including nested ones.
func Domains(pkgs []Package, module string) []string {
	var paths []string
	for _, p := range pkgs {
		path := strings.TrimPrefix(p.ImportPath, module+"/")
		if classify(path).kind == "domain" && !support(path) {
			paths = append(paths, "./"+path)
		}
	}
	return paths
}
