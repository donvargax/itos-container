package release

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// Build is what a binary is built from, as the release cut compares it
// (decision 23): each module it links, by path, at its version (a
// replacement's path and version after " => "), and the Go toolchain that
// builds it. The main module is not one of the modules: its own code changes
// with every commit, and a commit's type says whether that is a release.
type Build struct {
	Modules   map[string]string
	Toolchain string
}

// Moved lists what differs from old to new, in a line each for the release
// cut's reason: every linked module whose version moved, that came or that
// went, by path, then the toolchain. None means the binary is built from the
// same modules with the same toolchain, whatever else the range changed.
func Moved(old, new Build) []string {
	var paths []string
	for p := range old.Modules {
		paths = append(paths, p)
	}
	for p := range new.Modules {
		if _, ok := old.Modules[p]; !ok {
			paths = append(paths, p)
		}
	}
	sort.Strings(paths)
	var moved []string
	for _, p := range paths {
		was, linked := old.Modules[p]
		is, links := new.Modules[p]
		switch {
		case !linked:
			moved = append(moved, fmt.Sprintf("%s %s, newly linked", p, is))
		case !links:
			moved = append(moved, fmt.Sprintf("%s %s, no longer linked", p, was))
		case was != is:
			moved = append(moved, fmt.Sprintf("%s %s to %s", p, was, is))
		}
	}
	if old.Toolchain != new.Toolchain {
		moved = append(moved, fmt.Sprintf("the toolchain %s to %s", old.Toolchain, new.Toolchain))
	}
	return moved
}

// binary is the package the release builds (.goreleaser.yaml's main), and
// systems the GOOS it builds it for: a module one of them alone links
// counts.
const binary = "./cmd/itos-container"

var systems = []string{"linux", "darwin", "windows"}

// BinaryMoved lists what moved beneath the binary from one commit of the
// repository in the current folder to the other, by Moved: none when it is
// built from the same. tools/bin/release-version cuts a patch on it, and
// tools/bin/release-notes says why from it, so both read each side here,
// once. go list needs the modules of both commits: the module cache, or the
// network to the module proxy.
func BinaryMoved(from, to string) ([]string, error) {
	old, err := BuiltAt(from)
	if err != nil {
		return nil, err
	}
	now, err := BuiltAt(to)
	if err != nil {
		return nil, err
	}
	return Moved(old, now), nil
}

// BuiltAt is what the binary is built from at rev, read from a copy of rev's
// tree in a temporary folder (git archive, the working tree never touched):
// the modules are what go list -deps lists for ./cmd/itos-template on each
// system the release builds for, each package's module path and version, a
// replacement's after it, the main module left out; the toolchain is
// go.mod's toolchain line, else its go line, as go mod edit -json reads them.
func BuiltAt(rev string) (Build, error) {
	dir, err := os.MkdirTemp("", "release-built-")
	if err != nil {
		return Build{}, err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	if err := extract(rev, dir); err != nil {
		return Build{}, fmt.Errorf("cannot copy %s's tree: %v", rev, err)
	}
	b := Build{Modules: map[string]string{}}
	for _, goos := range systems {
		out, err := goIn(dir, goos, "list", "-deps", "-json=Module", binary)
		if err != nil {
			return Build{}, fmt.Errorf("at %s: %v", rev, err)
		}
		for decoder := json.NewDecoder(strings.NewReader(out)); ; {
			var pkg struct{ Module *module }
			if err := decoder.Decode(&pkg); errors.Is(err, io.EOF) {
				break
			} else if err != nil {
				return Build{}, fmt.Errorf("at %s, go list's output: %v", rev, err)
			}
			if m := pkg.Module; m != nil && !m.Main {
				b.Modules[m.Path] = m.version()
			}
		}
	}
	out, err := goIn(dir, "", "mod", "edit", "-json")
	if err != nil {
		return Build{}, fmt.Errorf("at %s: %v", rev, err)
	}
	var mod struct{ Go, Toolchain string }
	if err := json.Unmarshal([]byte(out), &mod); err != nil {
		return Build{}, fmt.Errorf("at %s, go mod edit's output: %v", rev, err)
	}
	b.Toolchain = mod.Toolchain
	if b.Toolchain == "" {
		b.Toolchain = "go" + mod.Go
	}
	return b, nil
}

// module is what go list says of a package's module.
type module struct {
	Path, Version string
	Main          bool
	Replace       *module
}

// version is the module's version, and its replacement's path and version
// after " => " when it has one.
func (m module) version() string {
	v := m.Version
	if r := m.Replace; r != nil {
		v += " => " + strings.TrimSpace(r.Path+" "+r.Version)
	}
	return v
}

// goIn runs go in dir, for goos when it is not "", as the release builds
// (CGO_ENABLED=0), and outside any go.work.
func goIn(dir, goos string, args ...string) (string, error) {
	cmd := exec.Command("go", args...) //nolint:gosec // the release tools name every argument here
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "CGO_ENABLED=0")
	if goos != "" {
		cmd.Env = append(cmd.Env, "GOOS="+goos, "GOARCH=amd64")
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("go %s: %v\n%s", strings.Join(args, " "), err, stderr.String())
	}
	return string(out), nil
}

// extract writes rev's tree into dir, from git archive, which leaves the
// repository and its working tree as they are.
func extract(rev, dir string) error {
	cmd := exec.Command("git", "archive", "--format=tar", rev) //nolint:gosec // a revision is the only variable, git reads it
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	unpacked := untar(pipe, dir)
	if unpacked != nil {
		_, _ = io.Copy(io.Discard, pipe)
	}
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("git archive %s: %v\n%s", rev, err, stderr.String())
	}
	return unpacked
}

// untar writes a tar stream's folders, files and links into dir, refusing a
// name that would leave it.
func untar(r io.Reader, dir string) error {
	archive := tar.NewReader(r)
	for {
		h, err := archive.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if !filepath.IsLocal(filepath.FromSlash(h.Name)) {
			return fmt.Errorf("the archive names %q, outside its folder", h.Name)
		}
		path := filepath.Join(dir, filepath.FromSlash(h.Name))
		switch h.Typeflag {
		case tar.TypeDir:
			err = os.MkdirAll(path, 0o750)
		case tar.TypeReg:
			err = write(path, archive, h.FileInfo().Mode().Perm())
		case tar.TypeSymlink:
			err = os.Symlink(h.Linkname, path)
		}
		if err != nil {
			return err
		}
	}
}

func write(path string, r io.Reader, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm) //nolint:gosec // untar refused any name leaving dir
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, r); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}
