package version

import (
	"runtime/debug"
	"testing"
)

// With nothing stamped, a build of a checkout, as go test's is, says the dev
// version; the scenario harness always stamps, so this is where that is
// shown.
func TestVersionUnstamped(t *testing.T) {
	saved := stamp
	t.Cleanup(func() { stamp = saved })
	stamp = ""
	if got := Version(); got != Dev {
		t.Errorf("Version() with nothing stamped = %q, want %q", got, Dev)
	}
}

// The stamp, when there is one, is the version.
func TestVersionStamped(t *testing.T) {
	saved := stamp
	t.Cleanup(func() { stamp = saved })
	stamp = "1.4.0"
	if got := Version(); got != "1.4.0" {
		t.Errorf("Version() stamped 1.4.0 = %q", got)
	}
}

// A module version go install records is read without its v; none is Dev.
func TestFromModule(t *testing.T) {
	for in, want := range map[string]string{"v0.6.0": "0.6.0", "(devel)": Dev, "": Dev} {
		if got := fromModule(in); got != want {
			t.Errorf("fromModule(%q) = %q, want %q", in, got, want)
		}
	}
}

// The commit stamped wins over the one Go recorded; without a stamp, Go's
// vcs.revision is the commit; a build that recorded none, or no build info
// at all, knows no commit.
func TestCommitOf(t *testing.T) {
	recorded := &debug.BuildInfo{Settings: []debug.BuildSetting{
		{Key: "vcs", Value: "git"},
		{Key: "vcs.revision", Value: "feedface"},
		{Key: "vcs.modified", Value: "true"},
	}}
	unrecorded := &debug.BuildInfo{Settings: []debug.BuildSetting{{Key: "-trimpath", Value: "true"}}}
	for _, c := range []struct {
		name    string
		stamped string
		info    *debug.BuildInfo
		want    string
	}{
		{"stamped, Go's recorded too", "c0ffee", recorded, "c0ffee"},
		{"stamped, no build info", "c0ffee", nil, "c0ffee"},
		{"Go's vcs.revision", "", recorded, "feedface"},
		{"no vcs.revision", "", unrecorded, ""},
		{"no build info", "", nil, ""},
	} {
		if got := commitOf(c.stamped, c.info); got != c.want {
			t.Errorf("%s: commitOf(%q, …) = %q, want %q", c.name, c.stamped, got, c.want)
		}
	}
}

// go test records no vcs.revision, so with nothing stamped the binary knows
// no commit; stamped, it is the stamp.
func TestCommit(t *testing.T) {
	saved := commit
	t.Cleanup(func() { commit = saved })
	commit = ""
	if got := Commit(); got != "" {
		t.Errorf("Commit() with nothing stamped under go test = %q, want none", got)
	}
	commit = "c0ffee"
	if got := Commit(); got != "c0ffee" {
		t.Errorf("Commit() stamped c0ffee = %q", got)
	}
}

// The text names the program and its version, and the commit on a second
// line only when there is one.
func TestText(t *testing.T) {
	if got, want := text("prog", "1.4.0", "c0ffee"), "prog 1.4.0\ncommit c0ffee"; got != want {
		t.Errorf("text with a commit = %q, want %q", got, want)
	}
	if got, want := text("prog", "1.4.0", ""), "prog 1.4.0"; got != want {
		t.Errorf("text with no commit = %q, want %q", got, want)
	}
	savedStamp, savedCommit := stamp, commit
	t.Cleanup(func() { stamp, commit = savedStamp, savedCommit })
	stamp, commit = "1.4.0", "c0ffee"
	if got, want := Text("prog"), "prog 1.4.0\ncommit c0ffee"; got != want {
		t.Errorf("Text stamped = %q, want %q", got, want)
	}
}
