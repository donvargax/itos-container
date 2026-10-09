// Package version is the version itos-container says it is, and the commit
// it was built from. Harvested from the owner's itos-template's internal/
// version, which took it from itos's.
//
// The version is the release's tag, written nowhere in the tree: a release's
// build stamps it into the binary with -ldflags
// "-X github.com/donvargax/itos-container/internal/version.stamp=<v>", as
// the scenario harness stamps the binary it builds. A binary built without
// the stamp says the module version Go records: the release's for
// `go install github.com/donvargax/itos-container/cmd/itos-container@<x>`,
// a pseudo-version from git (0.0.0-<time>-<commit>, +dirty with changes) for
// a go build in a checkout. One that records none (go run, go test, a build
// with -buildvcs=false) says Dev.
//
// The commit is stamped beside the version: a release's build sets it with
// "-X github.com/donvargax/itos-container/internal/version.commit=<id>",
// GoReleaser's full commit, and the scenario harness sets a known one. A
// binary built without the stamp says the vcs.revision Go records for a build
// in a checkout; one that records none (go install of a module, go run, go
// test, a build with -buildvcs=false) knows no commit, and says none.
package version

import (
	"runtime/debug"
	"strings"
)

// stamp is the version, and commit the full id of the commit the binary was
// built from, each set at link time (-ldflags -X).
var stamp, commit string

// Dev is what a binary says it is when it was built with neither the stamp
// nor a module version: Go's own word for it.
const Dev = "(devel)"

// Version is the version this binary says it is: the stamp, else the module
// version Go recorded, without its leading v, else Dev.
func Version() string {
	if stamp != "" {
		return stamp
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		return fromModule(info.Main.Version)
	}
	return Dev
}

// fromModule reads a module version as Go records it ("v0.6.0", or "(devel)"
// when it knows none).
func fromModule(v string) string {
	if v == "" || v == Dev {
		return Dev
	}
	return strings.TrimPrefix(v, "v")
}

// Commit is the full id of the commit this binary was built from: the
// stamp, else the vcs.revision Go recorded, else "", when it knows none.
func Commit() string {
	info, _ := debug.ReadBuildInfo()
	return commitOf(commit, info)
}

// commitOf is the commit a binary stamped with stamped and built as info
// says was built from; info is nil when Go recorded no build info.
func commitOf(stamped string, info *debug.BuildInfo) string {
	if stamped != "" {
		return stamped
	}
	if info == nil {
		return ""
	}
	for _, setting := range info.Settings {
		if setting.Key == "vcs.revision" {
			return setting.Value
		}
	}
	return ""
}

// Text is what program --version and program version print: "<program>
// <version>", then "commit <id>" on a second line when the build knows its
// commit.
func Text(program string) string {
	return text(program, Version(), Commit())
}

func text(program, version, commit string) string {
	if commit == "" {
		return program + " " + version
	}
	return program + " " + version + "\ncommit " + commit
}
