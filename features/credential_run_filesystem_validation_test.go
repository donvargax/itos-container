package features

import (
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestCredentialRunMountInfo(t *testing.T) {
	input := "7 1 8:1 /sub\\040volume /mount\\134path rw - ext4 /dev/test rw\n"
	mounts, err := readCredentialRunMounts(strings.NewReader(input))
	if want := []credentialRunMount{{id: 7, root: "/sub volume", path: `/mount\path`, kind: "ext4"}}; err != nil || !slices.Equal(mounts, want) {
		t.Fatalf("mountinfo = %v, %v, want %v", mounts, err, want)
	}
	for _, row := range []string{"missing separator", "invalid 1 8:1 / / rw - ext4 /dev/test rw"} {
		if _, err := readCredentialRunMounts(strings.NewReader(row)); err == nil {
			t.Errorf("accepted malformed mountinfo row %q", row)
		}
	}
}

func TestCredentialRunFilesystemVerification(t *testing.T) {
	cwd, err := filepath.Abs(".")
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.VolumeName(cwd) + string(filepath.Separator)
	parent := filepath.Join(root, "fixture")
	checkout := filepath.Join(parent, "checkout")
	fixtureMount := fmt.Sprintf("7 1 8:1 %s %s rw - ext4 /dev/test rw\n", root, root)
	foreignMount := fmt.Sprintf("8 7 0:1 %s %s rw - tmpfs tmpfs rw\n", root, filepath.Join(checkout, "nested"))
	tests := []struct {
		name      string
		shmMode   uint32
		change    func(*credentialRunFileStat)
		extra     string
		errorPath string
	}{
		{name: "verified", shmMode: credentialRunDirectory | 0o1777},
		{name: "non-sticky shared memory", shmMode: credentialRunDirectory | 0o777, errorPath: "/dev/shm"},
		{name: "untrusted ancestor owner", change: func(stat *credentialRunFileStat) { stat.uid = 1001 }, errorPath: parent},
		{name: "writable ancestor", change: func(stat *credentialRunFileStat) { stat.mode |= 0o020 }, errorPath: parent},
		{name: "symlink ancestor", change: func(stat *credentialRunFileStat) { stat.mode = credentialRunSymlink }, errorPath: parent},
		{name: "absent mount ID", change: func(stat *credentialRunFileStat) { stat.mask = 0 }, errorPath: parent},
		{name: "foreign nested mount", extra: foreignMount, errorPath: checkout},
		{name: "duplicate mount ID", extra: fixtureMount, errorPath: checkout},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shmMode := tt.shmMode
			if shmMode == 0 {
				shmMode = credentialRunDirectory | 0o1777
			}
			var observed []string
			filesystem := credentialRunFilesystemIO{
				openMounts: func() (io.ReadCloser, error) {
					return io.NopCloser(strings.NewReader(fixtureMount + tt.extra)), nil
				},
				statfs: func(path string) (uint32, error) {
					if path == "/dev/shm" {
						return credentialRunTmpfs, nil
					}
					return 0xEF53, nil
				},
				lstat: func(string) (credentialRunFileStat, error) {
					return credentialRunFileStat{mode: shmMode}, nil
				},
				statx: func(path string) (credentialRunFileStat, error) {
					observed = append(observed, path)
					stat := credentialRunFileStat{uid: 1000, mode: credentialRunDirectory | 0o700, mask: credentialRunMountID, mountID: 7}
					if path == parent && tt.change != nil {
						tt.change(&stat)
					}
					return stat, nil
				},
			}
			err := checkCredentialRunFilesystem(filesystem, 1000, []string{checkout})
			if tt.errorPath != "" {
				if err == nil || !strings.HasPrefix(err.Error(), tt.errorPath+" type ") {
					t.Fatalf("verification error = %v, want error for %s", err, tt.errorPath)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if want := []string{checkout, parent, root}; !slices.Equal(observed, want) {
				t.Fatalf("checked %v, want %v", observed, want)
			}
		})
	}
}
