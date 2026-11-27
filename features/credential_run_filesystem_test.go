package features

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type credentialRunMount struct {
	id               uint64
	root, path, kind string
}

// These values are part of the Linux stat and statx records returned by the
// fixture backend. Keep their interpretation here with the verification rules.
const (
	credentialRunTmpfs     = 0x01021994
	credentialRunMountID   = 0x1000
	credentialRunFileType  = 0o170000
	credentialRunDirectory = 0o040000
	credentialRunSymlink   = 0o120000
	credentialRunSticky    = 0o001000
)

type credentialRunFileStat struct {
	uid, mode, mask uint32
	mountID         uint64
}

type credentialRunFilesystemIO struct {
	openMounts func() (io.ReadCloser, error)
	statfs     func(string) (uint32, error)
	lstat      func(string) (credentialRunFileStat, error)
	statx      func(string) (credentialRunFileStat, error)
}

func readCredentialRunMounts(file io.Reader) ([]credentialRunMount, error) {
	decode := strings.NewReplacer(`\040`, " ", `\011`, "\t", `\012`, "\n", `\134`, "\\")
	var mounts []credentialRunMount
	lines := bufio.NewScanner(file)
	for lines.Scan() {
		left, right, ok := strings.Cut(lines.Text(), " - ")
		fields, fs := strings.Fields(left), strings.Fields(right)
		if !ok || len(fields) < 6 || len(fs) < 3 {
			return nil, errors.New("invalid fixture mountinfo row")
		}
		id, err := strconv.ParseUint(fields[0], 10, 64)
		if err != nil {
			return nil, errors.New("invalid fixture mountinfo ID")
		}
		mounts = append(mounts, credentialRunMount{id, decode.Replace(fields[3]), decode.Replace(fields[4]), fs[0]})
	}
	return mounts, lines.Err()
}

func verifyCredentialRunFilesystem(paths []string) error {
	return checkCredentialRunFilesystem(credentialRunFilesystemSystem, uint32(os.Geteuid()), paths)
}

func checkCredentialRunFilesystem(filesystem credentialRunFilesystemIO, euid uint32, paths []string) error {
	file, err := filesystem.openMounts()
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	mounts, err := readCredentialRunMounts(file)
	if err != nil {
		return err
	}
	allowed := map[uint32]string{0xEF53: "ext4", 0x9123683E: "btrfs", 0x58465342: "xfs"}
	const shm = "/dev/shm"
	shmType, fsErr := filesystem.statfs(shm)
	shmStat, statErr := filesystem.lstat(shm)
	if fsErr != nil || statErr != nil || shmType != credentialRunTmpfs || shmStat.uid != 0 ||
		shmStat.mode&credentialRunSticky == 0 || shmStat.mode&credentialRunFileType != credentialRunDirectory {
		return fmt.Errorf("%s type %#x uid %d", shm, shmType, shmStat.uid)
	}
	for _, root := range paths {
		for path := root; ; path = filepath.Dir(path) {
			fsType, fsErr := filesystem.statfs(path)
			stat, statErr := filesystem.statx(path)
			kind := allowed[fsType]
			bad := func() error { return fmt.Errorf("%s type %#x uid %d", path, fsType, stat.uid) }
			if fsErr != nil || statErr != nil || kind == "" || stat.mask&credentialRunMountID == 0 ||
				(stat.uid != euid && stat.uid != 0) ||
				(stat.mode&0022 != 0 && (stat.uid != 0 || stat.mode&credentialRunSticky == 0)) ||
				stat.mode&credentialRunFileType == credentialRunSymlink {
				return bad()
			}
			named := 0
			for _, m := range mounts {
				if m.id == stat.mountID {
					named++
					if !filepath.IsAbs(m.root) || !credentialRunUnderOrEqual(path, m.path) || m.kind != kind {
						return bad()
					}
				}
				if (credentialRunUnderOrEqual(root, m.path) || credentialRunUnderOrEqual(m.path, root)) &&
					m.kind != "ext4" && m.kind != "btrfs" && m.kind != "xfs" {
					return fmt.Errorf("%s type %s uid %d", path, m.kind, stat.uid)
				}
			}
			if named != 1 {
				return bad()
			}
			if path == filepath.Dir(path) {
				break
			}
		}
	}
	return nil
}
