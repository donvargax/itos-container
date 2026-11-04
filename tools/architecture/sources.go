package architecture

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
)

// excludedPureFiles catches whole packages omitted by go list ./... when every
// file is build-constrained. Imports still come exclusively from the Go graph.
func excludedPureFiles(dir, module string, pkgs []Package) ([]string, error) {
	listed := make(map[string]bool, len(pkgs))
	for _, p := range pkgs {
		listed[filepath.Clean(p.Dir)] = true
	}
	var violations []string
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if path != dir && (ignoredImport(entry.Name()) || entry.Name() == "vendor") {
				return filepath.SkipDir
			}
			return nil
		}
		folder := filepath.Dir(path)
		if !strings.HasSuffix(path, ".go") || listed[folder] {
			return nil
		}
		rel, err := filepath.Rel(dir, folder)
		if err != nil {
			return err
		}
		r := classify(filepath.ToSlash(rel))
		if r.kind == "domain" || r.layer == "port" {
			violations = append(violations, fmt.Sprintf("toplevel: %s/%s: ignored Go file %s; domain or port package excluded from go list", module, filepath.ToSlash(rel), entry.Name()))
		}
		return nil
	})
	return violations, err
}
