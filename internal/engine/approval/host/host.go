// Package host observes effective process and socket identities and the effective account home.
package host

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"

	"github.com/donvargax/itos-container/internal/engine/approval/port"
	"reflect"
	"strconv"
	"strings"
)

// Process contains no environment-based identity fallback. Its zero value uses
// the OS; tests can supply unavailable observations without changing host IDs.
type Process struct{ ops *operations }

type operations struct {
	geteuid, getegid func() int
	lstat            func(string) (os.FileInfo, error)
	lookup           func(string) (*user.User, error)
}

func (p Process) operations() operations {
	if p.ops != nil {
		return *p.ops
	}
	return operations{geteuid: os.Geteuid, getegid: os.Getegid, lstat: os.Lstat}
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

// AccountHome resolves the effective UID through the host account database. HOME,
// USER and LOGNAME cannot select the common coordination anchor.
func (p Process) AccountHome() (string, port.Failure) {
	uid, _ := p.Identity()
	lookup := p.operations().lookup
	if lookup == nil {
		lookup = user.LookupId
	}
	account, err := lookup(uid)
	if err != nil || account == nil || account.Uid != uid || !filepath.IsAbs(account.HomeDir) {
		return "", "state.unavailable"
	}
	return filepath.Clean(account.HomeDir), ""
}
