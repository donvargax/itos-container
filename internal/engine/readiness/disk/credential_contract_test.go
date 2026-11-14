package disk

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	policy "github.com/donvargax/itos-container/internal/engine/policy/port"
	"github.com/donvargax/itos-container/internal/engine/readiness/domain"
	"github.com/donvargax/itos-container/internal/engine/readiness/port"
	"github.com/donvargax/itos-container/internal/engine/readiness/port/porttest"
)

func TestCredentialObservationContract(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("native descriptor calls require Linux")
	}
	for _, adapter := range []bool{false, true} {
		t.Run(fmt.Sprintf("disk=%v", adapter), func(t *testing.T) {
			porttest.CredentialContract(t, func(raw *porttest.CredentialFiles, grants []policy.Credential, input port.CredentialInput) port.CredentialResult {
				raw.Parser = ParseCredentialMounts
				raw.FDParser = ParseCredentialMountID
				var files port.CredentialFS = raw
				if adapter {
					files = credentialNativeFixture(t, raw)
				}
				return (domain.Credentials{FS: files, Input: input}).Inspect(grants)
			})
		})
	}
}

// Only raw OS observations are substituted. The disk adapter still acquires
// native O_PATH handles, compares pre-open identities, parses kernel text and
// performs its descriptor/readdir mapping. No mount or privileged chown occurs.
func credentialNativeFixture(t *testing.T, raw *porttest.CredentialFiles) port.CredentialFS {
	t.Helper()
	root := t.TempDir()
	byFD := map[int]port.CredentialHandle{}
	// Open supplies the initial metadata; only subsequent Stat calls advance
	// the shared final-stat schedule, just as they do for the in-memory fake.
	opening := map[int]port.CredentialNode{}
	type descriptorRead struct {
		value port.CredentialDescriptor
		ok    bool
	}
	descriptors := map[int]descriptorRead{}
	backing := func(n port.CredentialNode) string {
		key := n.Inode
		if key >= 10000 {
			key = 10000
		}
		path := filepath.Join(root, fmt.Sprintf("%d-%d-%d", n.Device, key, n.Mode&0170000))
		if n.Mode&0170000 == 0040000 {
			if err := os.MkdirAll(path, 0700); err != nil {
				t.Fatal(err)
			}
		} else if _, err := os.Stat(path); os.IsNotExist(err) {
			if err := os.WriteFile(path, []byte("synthetic"), 0600); err != nil {
				t.Fatal(err)
			}
		}
		return path
	}
	openRaw := func(parent int, name string) (port.CredentialHandle, port.CredentialNode, error) {
		p := port.CredentialHandle(0)
		if parent != -100 {
			p = byFD[parent]
		}
		h, n, status := raw.Open(p, name)
		if status == port.CredentialReadMissing {
			return 0, n, os.ErrNotExist
		}
		if status != port.CredentialReadOK {
			return 0, n, os.ErrPermission
		}
		return h, n, nil
	}
	fdFrom := func(path, prefix string) int {
		text := strings.TrimPrefix(path, prefix)
		text, _, _ = strings.Cut(text, "/")
		n, err := strconv.Atoi(text)
		if err != nil {
			t.Fatalf("invalid fixture descriptor %q", path)
		}
		return n
	}
	files := &CredentialFiles{}
	ops := files.operations()
	ops.now = raw.Now
	ops.lstat = func(path string) (os.FileInfo, error) {
		parent, name := -100, path
		if strings.HasPrefix(path, "/proc/self/fd/") {
			parent = fdFrom(path, "/proc/self/fd/")
			_, name, _ = strings.Cut(strings.TrimPrefix(path, "/proc/self/fd/"), "/")
		}
		h, n, err := openRaw(parent, name)
		if err != nil {
			return nil, err
		}
		defer raw.Close(h)
		return os.Lstat(backing(n))
	}
	ops.open = func(parent int, name string) (int, error) {
		h, n, err := openRaw(parent, name)
		if err != nil {
			return -1, err
		}
		fd, err := credentialOpen(-100, backing(n))
		if err == nil {
			byFD[fd] = h
			opening[fd] = n
		}
		return fd, err
	}
	ops.metadata = func(file *os.File, _ os.FileInfo) (port.CredentialNode, bool) {
		fd := int(file.Fd())
		if n, ok := opening[fd]; ok {
			delete(opening, fd)
			return n, true
		}
		return raw.Stat(byFD[fd])
	}
	ops.statx = func(fd int) (uint64, uint32, error) {
		d, ok := raw.Descriptor(byFD[fd])
		descriptors[fd] = descriptorRead{d, ok}
		// Descriptor failures still reach the independent fdinfo/readlink seams.
		// Only the explicit missing-statx fixture omits its returned field mask.
		if ok && d.MountID == 0 {
			return 0, 0, nil
		}
		id := d.MountID
		if id == 0 {
			id = 1
		}
		return uint64(id), 0x1000, nil
	}
	ops.readlink = func(path string, buf []byte) (int, error) {
		observation := descriptors[fdFrom(path, "/proc/self/fd/")]
		if !observation.ok {
			return 0, os.ErrPermission
		}
		return copy(buf, observation.value.Path), nil
	}
	activityDone := false
	ops.read = func(path string, limit int64) ([]byte, bool) {
		if path == "/proc/self/mountinfo" {
			if !activityDone && raw.Activity != "" {
				// These are genuine unrelated state writes between acquisition of
				// the first observation and its return, never precomputed outcomes.
				state := filepath.Join(root, "unrelated-state")
				if err := os.Mkdir(state, 0700); err != nil {
					t.Fatal(err)
				}
				temp, record := filepath.Join(state, "temp"), filepath.Join(state, "record")
				if err := os.WriteFile(temp, []byte("synthetic unrelated record"), 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Rename(temp, record); err != nil {
					t.Fatal(err)
				}
				if strings.Contains(raw.Activity, "cleanup") || strings.Contains(raw.Activity, "disappearing") {
					if err := os.Remove(record); err != nil {
						t.Fatal(err)
					}
				}
				activityDone = true
			}
			data, ok := raw.RawMounts()
			return data, ok && int64(len(data)) <= limit
		}
		observation := descriptors[fdFrom(path, "/proc/self/fdinfo/")]
		d, ok := observation.value, observation.ok
		if data, exists := raw.FDInfo[d.Path]; exists {
			return []byte(data), ok && int64(len(data)) <= limit
		}
		return []byte(fmt.Sprintf("mnt_id:\t%d\n", d.MountID)), ok
	}
	ops.entries = func(file *os.File, _ int) ([]os.DirEntry, error) {
		h := byFD[fdFrom(file.Name(), "/proc/self/fd/")]
		names, done, ok := raw.Entries(h)
		if !ok {
			return nil, os.ErrPermission
		}
		entries := make([]os.DirEntry, 0, len(names))
		for _, name := range names {
			entries = append(entries, credentialEntryName(name))
		}
		if done {
			return entries, io.EOF
		}
		return entries, nil
	}
	files.ops = &ops
	return credentialContractFiles{CredentialFS: files, closeRaw: func(h port.CredentialHandle) {
		if p := files.handles[h]; p != nil {
			fd := int(p.file.Fd())
			raw.Close(byFD[fd])
			delete(byFD, fd)
			delete(descriptors, fd)
		}
	}}
}

// Keep the raw observation handles' lifetimes aligned with the native handles
// they describe, including early closes during pathname revalidation.
type credentialContractFiles struct {
	port.CredentialFS
	closeRaw func(port.CredentialHandle)
}

func (f credentialContractFiles) Close(h port.CredentialHandle) {
	f.closeRaw(h)
	f.CredentialFS.Close(h)
}

type credentialEntryName string

func (n credentialEntryName) Name() string             { return string(n) }
func (credentialEntryName) IsDir() bool                { return false }
func (credentialEntryName) Type() fs.FileMode          { return 0 }
func (credentialEntryName) Info() (fs.FileInfo, error) { return nil, os.ErrInvalid }
