package architecture

import (
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"regexp"
	"strings"
)

// A trailing comma does not undo a listed suppression.
var nolint = regexp.MustCompile(`(?i)^nolint(?:\s*:\s*([a-z0-9_-]+(?:\s*,\s*[a-z0-9_-]+)*)(?:\s*,)?)?(?:\s|//|$)`)

// Suppressions examines Go comments (never string literals), including generated files.
func Suppressions(dir string) ([]string, error) {
	var violations []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", ".tools", ".metrics", "vendor":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			return err
		}
		for _, group := range file.Comments {
			for _, comment := range group.List {
				text := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(comment.Text, "//"), "/*"), "*/"))
				m := nolint.FindStringSubmatch(text)
				if m == nil {
					continue
				}
				denied := m[1] == ""
				for _, name := range strings.Split(m[1], ",") {
					name = strings.TrimSpace(name)
					if strings.EqualFold(name, "depguard") || strings.EqualFold(name, "all") {
						denied = true
					}
				}
				if denied {
					rel, err := filepath.Rel(dir, path)
					if err != nil {
						return err
					}
					violations = append(violations, fmt.Sprintf("toplevel: %s:%d suppresses depguard", filepath.ToSlash(rel), fset.Position(comment.Pos()).Line))
				}
			}
		}
		return nil
	})
	return violations, err
}
