package disk

import (
	"errors"
	"io"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/donvargax/itos-container/internal/engine/readiness/port"
)

type credentialFileHandle struct{ file, directory *os.File }

// credentialOperations supplies raw OS observations for adapter-local contract fixtures.
type credentialOperations struct {
	lstat     func(string) (os.FileInfo, error)
	open      func(int, string) (int, error)
	metadata  func(*os.File, os.FileInfo) (port.CredentialNode, bool)
	statx     func(int) (uint64, uint32, error)
	read      func(string, int64) ([]byte, bool)
	readlink  func(string, []byte) (int, error)
	entries   func(*os.File, int) ([]os.DirEntry, error)
	directory func(string) (*os.File, error)
	now       func() time.Time
}

// CredentialFiles owns metadata-only native descriptors. It neither reads a
// token nor enumerates protected state. Each observation gets a fresh instance.
type CredentialFiles struct {
	ops     *credentialOperations
	handles map[port.CredentialHandle]*credentialFileHandle
	next    port.CredentialHandle
}

func (f *CredentialFiles) operations() credentialOperations {
	if f.ops != nil {
		return *f.ops
	}
	return credentialOperations{lstat: os.Lstat, open: credentialOpen, metadata: func(_ *os.File, info os.FileInfo) (port.CredentialNode, bool) { return credentialMetadata(info) }, statx: credentialStatx, read: credentialReadBounded, readlink: credentialReadlink, entries: (*os.File).ReadDir, directory: os.Open, now: time.Now}
}

func (f *CredentialFiles) Now() time.Time { return f.operations().now() }

func (f *CredentialFiles) Open(parent port.CredentialHandle, name string) (port.CredentialHandle, port.CredentialNode, port.CredentialRead) {
	var zero port.CredentialNode
	parentFD := -100
	path := name
	if parent != 0 {
		p := f.handles[parent]
		if p == nil {
			return 0, zero, port.CredentialReadUnavailable
		}
		parentFD = int(p.file.Fd())
		path = "/proc/self/fd/" + strconv.Itoa(parentFD) + "/" + name
	}
	before, err := f.operations().lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return 0, zero, port.CredentialReadMissing
	}
	if err != nil {
		return 0, zero, port.CredentialReadUnavailable
	}
	fd, err := f.operations().open(parentFD, name)
	if err != nil {
		return 0, zero, port.CredentialReadUnavailable
	}
	file := os.NewFile(uintptr(fd), name)
	after, err := file.Stat()
	if err != nil || !os.SameFile(before, after) {
		_ = file.Close()
		return 0, zero, port.CredentialReadUnavailable
	}
	node, ok := f.operations().metadata(file, after)
	if !ok {
		_ = file.Close()
		return 0, zero, port.CredentialReadUnavailable
	}
	if f.handles == nil {
		f.handles = map[port.CredentialHandle]*credentialFileHandle{}
	}
	f.next++
	f.handles[f.next] = &credentialFileHandle{file: file}
	return f.next, node, port.CredentialReadOK
}

func (f *CredentialFiles) Stat(h port.CredentialHandle) (port.CredentialNode, bool) {
	if f.handles[h] == nil {
		return port.CredentialNode{}, false
	}
	info, err := f.handles[h].file.Stat()
	if err != nil {
		return port.CredentialNode{}, false
	}
	return f.operations().metadata(f.handles[h].file, info)
}

func credentialMetadata(info os.FileInfo) (port.CredentialNode, bool) {
	raw := reflect.Indirect(reflect.ValueOf(info.Sys()))
	if !raw.IsValid() || raw.Kind() != reflect.Struct {
		return port.CredentialNode{}, false
	}
	var values []uint64
	for _, name := range []string{"Dev", "Ino", "Nlink", "Uid", "Mode"} {
		v := raw.FieldByName(name)
		if !v.IsValid() || !v.CanUint() {
			return port.CredentialNode{}, false
		}
		values = append(values, v.Uint())
	}
	uid, mode := values[3], values[4]
	if uid > 1<<32-1 || mode > 1<<32-1 {
		return port.CredentialNode{}, false
	}
	return port.CredentialNode{Device: values[0], Inode: values[1], Links: values[2], UID: uint32(uid), Mode: uint32(mode), Size: info.Size()}, true
}

func (f *CredentialFiles) Descriptor(h port.CredentialHandle) (port.CredentialDescriptor, bool) {
	var zero port.CredentialDescriptor
	p := f.handles[h]
	if p == nil {
		return zero, false
	}
	mountID, mask, err := f.operations().statx(int(p.file.Fd()))
	// STATX_MNT_ID is the reusable ID in mountinfo, not STATX_MNT_ID_UNIQUE.
	// Validate the returned mask: older kernels can succeed without this field.
	if err != nil || mask&0x1000 == 0 || mountID == 0 || mountID > 1<<32-1 {
		return zero, true
	}
	fd := strconv.FormatUint(uint64(p.file.Fd()), 10)
	data, ok := f.operations().read("/proc/self/fdinfo/"+fd, 64*1024)
	if !ok {
		return zero, false
	}
	id, ok := ParseCredentialMountID(data)
	if !ok || id != int(mountID) {
		return zero, false
	}
	// The kernel names a descriptor in at most 4095 bytes and fails the read
	// beyond that. One byte of room tells a longer or cut result from that
	// name, should a kernel ever return one.
	buf := make([]byte, 4096)
	n, err := f.operations().readlink("/proc/self/fd/"+fd, buf)
	if err != nil || n > 4095 {
		return zero, false
	}
	return port.CredentialDescriptor{Path: string(buf[:n]), MountID: int(mountID)}, true
}

// ParseCredentialMountID accepts complete bounded fdinfo evidence. The reader
// must establish EOF; this parser requires exactly one positive numeric mount ID.
func ParseCredentialMountID(data []byte) (int, bool) {
	if len(data) > 64*1024 {
		return 0, false
	}
	id := 0
	for _, line := range strings.Split(string(data), "\n") {
		key, value, found := strings.Cut(line, ":")
		if !found || key != "mnt_id" {
			continue
		}
		if id != 0 {
			return 0, false
		}
		value = strings.TrimSpace(value)
		if value == "" || strings.Trim(value, "0123456789") != "" {
			return 0, false
		}
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed <= 0 {
			return 0, false
		}
		id = parsed
	}
	return id, id > 0
}

func (f *CredentialFiles) Entries(h port.CredentialHandle) ([]string, bool, bool) {
	p := f.handles[h]
	if p == nil {
		return nil, false, false
	}
	if p.directory == nil {
		before, err := p.file.Stat()
		if err != nil || !before.IsDir() {
			return nil, false, false
		}
		dir, err := f.operations().directory("/proc/self/fd/" + strconv.FormatUint(uint64(p.file.Fd()), 10))
		if err != nil {
			return nil, false, false
		}
		p.directory = dir
		a, e1 := dir.Stat()
		b, e2 := p.file.Stat()
		if e1 != nil || e2 != nil || !a.IsDir() || !os.SameFile(a, b) {
			return nil, false, false
		}
	}
	entries, err := f.operations().entries(p.directory, checkoutBatchSize)
	if err != nil && err != io.EOF {
		return nil, false, false
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names, err == io.EOF, true
}

func (f *CredentialFiles) Mounts() ([]port.CredentialMount, bool) {
	data, ok := f.operations().read("/proc/self/mountinfo", 8*1024*1024)
	if !ok {
		return nil, false
	}
	return ParseCredentialMounts(data)
}

func credentialReadBounded(path string, limit int64) ([]byte, bool) {
	f, err := os.OpenInRoot("/", strings.TrimPrefix(path, "/"))
	if err != nil {
		return nil, false
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	return data, err == nil && int64(len(data)) <= limit
}

func (f *CredentialFiles) Close(h port.CredentialHandle) {
	if p := f.handles[h]; p != nil {
		if p.directory != nil {
			_ = p.directory.Close()
		}
		_ = p.file.Close()
		delete(f.handles, h)
	}
}
