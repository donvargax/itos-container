package architecture

import (
	"context"
	"fmt"
	"strings"
)

// Watched identifies every input that can change the architecture matrix's verdict.
func Watched(path string) bool {
	switch path {
	case ".golangci.yml", "tools/bin/architecture-check", "tools/bin/domain-coverage", "tools/hooks/pre-commit", "tools/bin/pinned", "go.mod", "go.sum", "itos.yaml":
		return true
	}
	return strings.HasPrefix(path, "tools/architecture/") || strings.HasPrefix(path, "tools/testdata/architecture/")
}

// Selected reads staged changes or a validated commit range. --no-renames presents
// both sides of a rename as delete/add; NUL records preserve arbitrary path names.
func Selected(ctx context.Context, dir string, staged bool, base, head string) (bool, error) {
	args := []string{"diff", "--name-only", "-z", "--no-renames"}
	if staged {
		args = append(args, "--cached")
	} else {
		for _, ref := range []string{base, head} {
			if ref == "" {
				return false, fmt.Errorf("missing fixture selection range")
			}
			if _, err := Run(ctx, dir, "git", "rev-parse", "--verify", "--end-of-options", ref+"^{commit}"); err != nil {
				return false, err
			}
		}
		args = append(args, base, head)
	}
	args = append(args, "--")
	b, err := Run(ctx, dir, "git", args...)
	if err != nil {
		return false, err
	}
	for _, path := range strings.Split(string(b), "\x00") {
		if Watched(path) {
			return true, nil
		}
	}
	return false, nil
}
