package disk

import (
	"bytes"
	"strconv"
	"strings"

	"github.com/donvargax/itos-container/internal/engine/readiness/port"
)

// ParseCredentialMounts validates complete bounded kernel evidence around the
// pinned library parser. It deliberately uses no early-stopping filter. Library
// numeric errors are not propagated by mountinfo, so validate those fields here.
func ParseCredentialMounts(data []byte) ([]port.CredentialMount, bool) {
	if len(data) == 0 || len(data) > 8*1024*1024 || data[len(data)-1] != '\n' {
		return nil, false
	}
	lines := strings.Split(string(data[:len(data)-1]), "\n")
	if len(lines) > 16384 {
		return nil, false
	}
	for _, line := range lines {
		if len(line) > 65535 {
			return nil, false
		}
		fields := strings.Split(line, " ")
		if len(fields) < 10 {
			return nil, false
		}
		for _, field := range fields {
			if field == "" {
				return nil, false
			}
		}
		device := strings.Split(fields[2], ":")
		if len(device) != 2 {
			return nil, false
		}
		for _, number := range append(fields[:2:2], device...) {
			if number == "" || strings.Trim(number, "0123456789") != "" {
				return nil, false
			}
			if _, err := strconv.ParseUint(number, 10, 32); err != nil {
				return nil, false
			}
		}
	}
	parsed, err := credentialParseMounts(bytes.NewReader(data))
	if err != nil || len(parsed) != len(lines) {
		return nil, false
	}
	seen := map[int]bool{}
	result := make([]port.CredentialMount, 0, len(parsed))
	for _, row := range parsed {
		if row.ID <= 0 || seen[row.ID] || !credentialMountPath(row.Root) || !credentialMountPath(row.Mountpoint) {
			return nil, false
		}
		seen[row.ID] = true
		// The prefix was validated above; parse the widths explicitly rather
		// than narrowing the library's platform-sized integer fields.
		deviceText := strings.Split(strings.Split(lines[len(result)], " ")[2], ":")
		major, _ := strconv.ParseUint(deviceText[0], 10, 32)
		minor, _ := strconv.ParseUint(deviceText[1], 10, 32)
		device := (minor & 0xff) | (major&0xfff)<<8 | (minor&^uint64(0xff))<<12 | (major&^uint64(0xfff))<<32
		result = append(result, port.CredentialMount{ID: row.ID, Device: device, Root: row.Root, Point: row.Mountpoint})
	}
	return result, true
}

func credentialMountPath(path string) bool {
	if path == "/" {
		return true
	}
	if len(path) > 4096 || !strings.HasPrefix(path, "/") || strings.ContainsRune(path, 0) {
		return false
	}
	for _, part := range strings.Split(path[1:], "/") {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	return true
}
