package features

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

type credentialRunMount struct {
	id               uint64
	root, path, kind string
}

func readCredentialRunMounts() ([]credentialRunMount, error) {
	file, err := os.Open("/proc/self/mountinfo")
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
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
	mounts, err := readCredentialRunMounts()
	if err != nil {
		return err
	}
	allowed := map[int64]string{0xEF53: "ext4", 0x9123683E: "btrfs", 0x58465342: "xfs"}
	euid := uint32(os.Geteuid())
	shm := "/dev/shm"
	var fs unix.Statfs_t
	var st unix.Stat_t
	if unix.Statfs(shm, &fs) != nil || unix.Lstat(shm, &st) != nil || fs.Type != unix.TMPFS_MAGIC || st.Uid != 0 || st.Mode&unix.S_ISVTX == 0 || st.Mode&unix.S_IFMT != unix.S_IFDIR {
		return fmt.Errorf("%s type %#x uid %d", shm, fs.Type, st.Uid)
	}
	for _, root := range paths {
		for path := root; ; path = filepath.Dir(path) {
			var fs unix.Statfs_t
			var stat unix.Statx_t
			fsErr := unix.Statfs(path, &fs)
			statErr := unix.Statx(unix.AT_FDCWD, path, unix.AT_SYMLINK_NOFOLLOW, unix.STATX_BASIC_STATS|unix.STATX_MNT_ID, &stat)
			kind := allowed[int64(fs.Type)]
			bad := func() error { return fmt.Errorf("%s type %#x uid %d", path, fs.Type, stat.Uid) }
			if fsErr != nil || statErr != nil || kind == "" || stat.Mask&unix.STATX_MNT_ID == 0 ||
				(stat.Uid != euid && stat.Uid != 0) ||
				(stat.Mode&0022 != 0 && (stat.Uid != 0 || stat.Mode&unix.S_ISVTX == 0)) ||
				stat.Mode&unix.S_IFMT == unix.S_IFLNK {
				return bad()
			}
			named := 0
			for _, m := range mounts {
				if m.id == stat.Mnt_id {
					named++
					if !filepath.IsAbs(m.root) || !credentialRunUnderOrEqual(path, m.path) || m.kind != kind {
						return bad()
					}
				}
				if (credentialRunUnderOrEqual(root, m.path) || credentialRunUnderOrEqual(m.path, root)) &&
					m.kind != "ext4" && m.kind != "btrfs" && m.kind != "xfs" {
					return fmt.Errorf("%s type %s uid %d", path, m.kind, stat.Uid)
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
