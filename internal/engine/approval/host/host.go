// Package host observes effective process and socket identities without NSS.
package host

import (
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
)

// Process contains no environment-based identity fallback.
type Process struct{}

// Identity returns decimal effective IDs; unavailable platforms return empty IDs.
func (Process) Identity() (string, string) {
	uid, gid := os.Geteuid(), os.Getegid()
	if uid < 0 || gid < 0 {
		return "", ""
	}
	return strconv.Itoa(uid), strconv.Itoa(gid)
}

// Socket supplies an ephemeral review guard, deliberately excluded from hashes.
func (Process) Socket(endpoint string) string {
	path, ok := strings.CutPrefix(endpoint, "unix://")
	if !ok {
		return ""
	}
	info, err := os.Lstat(path)
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
