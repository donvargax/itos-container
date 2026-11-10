// Package host observes effective process and socket identities and the effective account home.
package host

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"

	"github.com/donvargax/itos-container/internal/engine/approval/port"
)

// Process contains no environment-based identity fallback. Its zero value uses
// the OS; tests can supply unavailable observations without changing host IDs.
type Process struct{ ops *operations }

type operations struct {
	geteuid, getegid func() int
	lstat            func(string) (os.FileInfo, error)
	openAccount      func() (io.ReadCloser, error)
}

func (p Process) operations() operations {
	if p.ops != nil {
		return *p.ops
	}
	return operations{
		geteuid: os.Geteuid, getegid: os.Getegid, lstat: os.Lstat,
		openAccount: func() (io.ReadCloser, error) { return os.Open("/etc/passwd") },
	}
}

// Identity returns decimal effective IDs; unavailable platforms return empty IDs.
func (p Process) Identity() (string, string) {
	ops := p.operations()
	uid, gid := ops.geteuid(), ops.getegid()
	if uid < 0 || gid < 0 {
		return "", ""
	}
	return strconv.Itoa(uid), strconv.Itoa(gid)
}

// Socket supplies an ephemeral review guard, deliberately excluded from hashes.
func (p Process) Socket(endpoint string) string {
	path, ok := strings.CutPrefix(endpoint, "unix://")
	if !ok {
		return ""
	}
	info, err := p.operations().lstat(path)
	if err != nil || info.Mode()&os.ModeSocket == 0 {
		return ""
	}
	stat := reflect.Indirect(reflect.ValueOf(info.Sys()))
	if !stat.IsValid() || stat.Kind() != reflect.Struct {
		return ""
	}
	dev, ino := stat.FieldByName("Dev"), stat.FieldByName("Ino")
	if !dev.IsValid() || !ino.IsValid() {
		return ""
	}
	return fmt.Sprint(dev.Interface(), ":", ino.Interface())
}

// AccountHome requires a local account record for the effective UID. os/user's
// pure-Go Current/LookupId can synthesize a record from USER and HOME, so it is
// not evidence for the common coordination anchor. NSS-only accounts fail closed.
func (p Process) AccountHome() (string, port.Failure) {
	uid, _ := p.Identity()
	if uid == "" {
		return "", "state.unavailable"
	}
	file, err := p.operations().openAccount()
	if err != nil {
		return "", "state.unavailable"
	}
	defer func() { _ = file.Close() }()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Split(strings.TrimSpace(scanner.Text()), ":")
		if len(fields) != 7 || fields[0] == "" || strings.ContainsAny(fields[0][:1], "#+-") || fields[2] != uid {
			continue
		}
		if _, err := strconv.ParseUint(fields[3], 10, 32); err != nil || !filepath.IsAbs(fields[5]) || strings.ContainsRune(fields[5], 0) {
			return "", "state.unavailable"
		}
		return filepath.Clean(fields[5]), ""
	}
	// EOF without a record and all read/scanner failures lack account evidence.
	return "", "state.unavailable"
}
