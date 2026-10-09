// An itos can be linked as git before the real one on the PATH (itos git-shim
// install), so a git a scenario's PATH would find may run an itos's own
// policy where the scenario meant only itos-container. The rule is itos's own
// (internal/git's IsItos, its bug 45 and T-104), copied since a module cannot
// import another's internal packages.
package features

import (
	"debug/buildinfo"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// isItos is whether the program at p is an itos, so never the real git: a
// symbolic link whose target, at any link of the chain, is a file named itos
// (itos.exe), as git-shim install makes one; or a Go binary built from itos's
// cmd/itos, any major version, as its build information records it, which a
// hard link or a copy of any itos keeps and costs a read of the file, never a
// run of it.
func isItos(p string) bool {
	return linksNamedItos(p) || builtAsItos(p)
}

// linksNamedItos is whether p is a symbolic link whose target, or the target
// of any link after it, is named itos.
func linksNamedItos(p string) bool {
	for range 40 {
		target, err := os.Readlink(p)
		if err != nil {
			return false
		}
		if strings.TrimSuffix(strings.ToLower(filepath.Base(target)), ".exe") == "itos" {
			return true
		}
		if !filepath.IsAbs(target) {
			target = filepath.Join(filepath.Dir(p), target)
		}
		p = target
	}
	return false
}

// itosMain is the main package of an itos build, at any major version of its
// module.
var itosMain = regexp.MustCompile(`^github\.com/donvargax/itos(/v[0-9]+)?/cmd/itos$`)

// builtAsItos is whether the program at p is a Go binary built from itos's
// cmd/itos.
func builtAsItos(p string) bool {
	info, err := buildinfo.ReadFile(p)
	return err == nil && itosMain.MatchString(info.Path)
}
