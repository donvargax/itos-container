package disk

import (
	"context"
	"errors"
	readiness "github.com/donvargax/itos-container/internal/engine/readiness/port"
	"github.com/donvargax/itos-container/internal/engine/session/port"
	"io"
	"io/fs"
	"os"
	"reflect"
	"strconv"
	"strings"
)

// Credentials has no production alternative to /dev/shm. Private hooks let
// unit tests observe ordinary temporary files without claiming mount evidence.
type Credentials struct {
	OS     string
	root   string
	tmpfs  func(string) bool
	before func(string)
	read   func(*os.File, []byte) (int, error)
	write  func(*os.File, []byte) (int, error)
	unlink func(*os.Root, string) error
}

const credentialNoFollow = 0x20000
const credentialDirectory = 0x10000

func (s Credentials) base() (string, bool) {
	root := s.root
	if root == "" {
		root = "/dev/shm"
	}
	check := s.tmpfs
	if check == nil {
		check = credentialTmpfs
	}
	return root, s.OS == "linux" && check(root)
}
func credentialNode(info fs.FileInfo) (readiness.CredentialNode, bool) {
	if info == nil {
		return readiness.CredentialNode{}, false
	}
	raw := reflect.Indirect(reflect.ValueOf(info.Sys()))
	if !raw.IsValid() || raw.Kind() != reflect.Struct {
		return readiness.CredentialNode{}, false
	}
	values := map[string]uint64{}
	for _, key := range []string{"Dev", "Ino", "Nlink", "Uid", "Mode"} {
		v := raw.FieldByName(key)
		if !v.IsValid() || !v.CanUint() {
			return readiness.CredentialNode{}, false
		}
		values[key] = v.Uint()
	}
	uid, mode := values["Uid"], values["Mode"]
	if uid > 1<<32-1 || mode > 1<<32-1 {
		return readiness.CredentialNode{}, false
	}
	return readiness.CredentialNode{Device: values["Dev"], Inode: values["Ino"], Links: values["Nlink"], UID: uint32(uid), Mode: uint32(mode), Size: info.Size()}, true
}
func credentialIdentity(n readiness.CredentialNode) string {
	return strconv.FormatUint(n.Device, 10) + ":" + strconv.FormatUint(n.Inode, 10)
}
func credentialOwner(n readiness.CredentialNode, uid string, mode uint32) bool {
	return strconv.FormatUint(uint64(n.UID), 10) == uid && n.Mode&0177777 == mode
}
func (s Credentials) event(name string) {
	if s.before != nil {
		s.before(name)
	}
}
func (s Credentials) Stage(ctx context.Context, r port.Record, sources port.CredentialSources, publish func(port.CredentialStaging) port.Outcome) port.Outcome {
	if r.Credentials == (port.CredentialStaging{}) && sources == nil {
		return ""
	}
	if !r.Credentials.Valid() || r.Credentials.Stage != "intent" || sources == nil || sources.Kinds() != r.Credentials.Kinds {
		return port.CredentialRefused
	}
	if f := sources.Revalidate(ctx); f != "" {
		return f
	}
	checked := sources.Checked()
	if len(checked) != len(r.Credentials.Kinds.List()) {
		return port.CredentialRefused
	}
	seen := map[string]bool{}
	selected := map[string]bool{}
	for _, kind := range r.Credentials.Kinds.List() {
		selected[kind] = true
	}
	for _, source := range checked {
		if !selected[source.Kind] || port.CredentialIndex(source.Kind) < 0 || !strings.HasPrefix(source.ReadPath, "/proc/self/fd/") || seen[source.Kind] {
			return port.CredentialRefused
		}
		seen[source.Kind] = true
		number := strings.TrimPrefix(source.ReadPath, "/proc/self/fd/")
		if n, err := strconv.ParseUint(number, 10, 64); err != nil || n == 0 || strconv.FormatUint(n, 10) != number {
			return port.CredentialRefused
		}
	}
	root, ok := s.base()
	if !ok {
		return port.CredentialUnavailable
	}
	if ctx.Err() != nil {
		return port.CredentialUnavailable
	}
	base, err := os.OpenRoot(root)
	if err != nil {
		return port.CredentialUnavailable
	}
	defer func() { _ = base.Close() }()
	name := strings.TrimPrefix(r.Credentials.Directory, "/dev/shm/")
	s.event("mkdir")
	if err = base.Mkdir(name, 0700); err != nil {
		return port.CredentialRefused
	}
	file, err := base.OpenFile(name, os.O_RDONLY|credentialDirectory|credentialNoFollow, 0)
	if err != nil {
		return port.CredentialRefused
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	n, ok := credentialNode(info)
	if err != nil || !ok || !credentialOwner(n, r.UID, 0040700) {
		return port.CredentialRefused
	}
	staging := r.Credentials
	staging.Device, staging.Inode = strconv.FormatUint(n.Device, 10), strconv.FormatUint(n.Inode, 10)
	staging.Stage = "owned"
	defer func() {
		if ctx.Err() != nil {
			late := r
			late.Credentials = staging
			_ = s.Remove(context.WithoutCancel(ctx), late)
		}
	}()
	// Identity publication precedes token file creation. A late reply cannot
	// publish after cancellation; the caller still retains serialization.
	if ctx.Err() != nil {
		return port.CredentialUnavailable
	}
	if f := publish(staging); f != "" {
		// Recovery still has only the intent. Retain the open descriptor while
		// removing by its validated identity; cleanup cannot replace the save failure.
		unpublished := r
		unpublished.Credentials = staging
		_ = s.Remove(context.WithoutCancel(ctx), unpublished)
		return f
	}
	dir, err := os.OpenRoot("/proc/self/fd/" + strconv.FormatUint(uint64(file.Fd()), 10))
	if err != nil {
		return port.CredentialUnavailable
	}
	defer func() { _ = dir.Close() }()
	buffer := make([]byte, 65537)
	defer clear(buffer)
	for _, source := range checked {
		if ctx.Err() != nil {
			return port.CredentialUnavailable
		}
		if f := s.copy(ctx, dir, r.UID, source, buffer, &staging, publish); f != "" {
			return f
		}
	}
	return ""
}
func (s Credentials) copy(ctx context.Context, dir *os.Root, uid string, source port.CheckedCredential, buffer []byte, staging *port.CredentialStaging, publish func(port.CredentialStaging) port.Outcome) port.Outcome {
	s.event("read")
	input, err := os.Open(source.ReadPath)
	if err != nil {
		return port.CredentialRefused
	}
	defer func() { _ = input.Close() }()
	info, err := input.Stat()
	node, ok := credentialNode(info)
	if err != nil || !ok || node != source.Node || node.Size < 1 || node.Size > 65536 || node.Mode&0170000 != 0100000 || node.Mode&0077 != 0 || node.Links != 1 || strconv.FormatUint(uint64(node.UID), 10) != uid {
		return port.CredentialRefused
	}
	read := s.read
	if read == nil {
		read = (*os.File).Read
	}
	count, err := read(input, buffer[:int(node.Size)+1])
	if err != nil || int64(count) != node.Size {
		return port.CredentialRefused
	}
	after, err := input.Stat()
	final, ok := credentialNode(after)
	if err != nil || !ok || final != node || ctx.Err() != nil {
		return port.CredentialRefused
	}
	s.event("file")
	output, err := dir.OpenFile(source.Kind, os.O_WRONLY|os.O_CREATE|os.O_EXCL|credentialNoFollow, 0400)
	if err != nil {
		return port.CredentialRefused
	}
	defer func() { _ = output.Close() }()
	info, err = output.Stat()
	created, ok := credentialNode(info)
	if err != nil || !ok || !credentialOwner(created, uid, 0100400) || created.Links != 1 {
		return port.CredentialRefused
	}
	staging.Files[port.CredentialIndex(source.Kind)] = credentialIdentity(created)
	if ctx.Err() != nil {
		return port.CredentialUnavailable
	}
	if f := publish(*staging); f != "" {
		// Recovery has no authority over this unpublished identity. Keep the
		// descriptor open while checking its name, and never unlink a replacement.
		if named, err := dir.Lstat(source.Kind); err == nil && os.SameFile(info, named) {
			_ = dir.Remove(source.Kind)
		}
		return f
	}
	s.event("copy")
	if ctx.Err() != nil {
		return port.CredentialUnavailable
	}
	write := s.write
	if write == nil {
		write = (*os.File).Write
	}
	written, err := write(output, buffer[:count])
	clear(buffer)
	if err != nil || written != count {
		return port.CredentialUnavailable
	}
	if ctx.Err() != nil {
		return port.CredentialUnavailable
	}
	return ""
}

// heldDirectory validates all entries before any unlink. Missing expected files
// are allowed after partial cleanup; extra entries and substitutions retain proof.
func (s Credentials) heldDirectory(ctx context.Context, r port.Record) (*os.Root, *os.File, *os.Root, string, port.Outcome) {
	if !r.Credentials.Valid() {
		return nil, nil, nil, "unknown", port.CredentialRefused
	}
	root, ok := s.base()
	if !ok || ctx.Err() != nil {
		return nil, nil, nil, "unknown", port.CredentialUnavailable
	}
	base, err := os.OpenRoot(root)
	if err != nil {
		return nil, nil, nil, "unknown", port.CredentialUnavailable
	}
	name := strings.TrimPrefix(r.Credentials.Directory, "/dev/shm/")
	file, err := base.OpenFile(name, os.O_RDONLY|credentialDirectory|credentialNoFollow, 0)
	if errors.Is(err, os.ErrNotExist) {
		_ = base.Close()
		return nil, nil, nil, "absent", ""
	}
	fail := func() (*os.Root, *os.File, *os.Root, string, port.Outcome) {
		if file != nil {
			_ = file.Close()
		}
		_ = base.Close()
		return nil, nil, nil, "unknown", port.CredentialRefused
	}
	if err != nil {
		return fail()
	}
	info, err := file.Stat()
	node, ok := credentialNode(info)
	if err != nil || !ok || !credentialOwner(node, r.UID, 0040700) || strconv.FormatUint(node.Device, 10) != r.Credentials.Device || strconv.FormatUint(node.Inode, 10) != r.Credentials.Inode || r.Credentials.Stage == "absent" {
		return fail()
	}
	dir, err := os.OpenRoot("/proc/self/fd/" + strconv.FormatUint(uint64(file.Fd()), 10))
	if err != nil {
		return fail()
	}
	names, err := file.Readdirnames(3)
	if err != nil {
		if !errors.Is(err, io.EOF) {
			_ = dir.Close()
			return fail()
		}
	}
	if len(names) > len(r.Credentials.Kinds.List()) {
		_ = dir.Close()
		return fail()
	}
	for _, kind := range names {
		index := port.CredentialIndex(kind)
		if index < 0 || r.Credentials.Files[index] == "" {
			_ = dir.Close()
			return fail()
		}
		info, e := dir.Lstat(kind)
		n, valid := credentialNode(info)
		if e != nil || !valid || !credentialOwner(n, r.UID, 0100400) || n.Links != 1 || credentialIdentity(n) != r.Credentials.Files[index] {
			_ = dir.Close()
			return fail()
		}
	}
	return base, file, dir, "present", ""
}
func (s Credentials) Observe(ctx context.Context, r port.Record) (string, port.Outcome) {
	base, file, dir, presence, f := s.heldDirectory(ctx, r)
	if dir != nil {
		_ = dir.Close()
	}
	if file != nil {
		_ = file.Close()
	}
	if base != nil {
		_ = base.Close()
	}
	return presence, f
}
func (s Credentials) Remove(ctx context.Context, r port.Record) port.Outcome {
	base, file, dir, presence, f := s.heldDirectory(ctx, r)
	if f != "" || presence == "absent" {
		return f
	}
	defer func() { _ = dir.Close(); _ = file.Close(); _ = base.Close() }()
	var failure port.Outcome
	unlink := s.unlink
	if unlink == nil {
		unlink = (*os.Root).Remove
	}
	for _, kind := range r.Credentials.Kinds.List() {
		s.event("unlink")
		if ctx.Err() != nil {
			return port.CredentialUnavailable
		}
		if err := unlink(dir, kind); err != nil && !errors.Is(err, os.ErrNotExist) {
			failure = port.Incomplete
		}
	}
	if failure != "" {
		return failure
	}
	// Compare the parent name with the descriptor again before removing it.
	info, err := base.Lstat(strings.TrimPrefix(r.Credentials.Directory, "/dev/shm/"))
	held, e := file.Stat()
	if err != nil || e != nil || !os.SameFile(info, held) {
		return port.CredentialRefused
	}
	if err = base.Remove(strings.TrimPrefix(r.Credentials.Directory, "/dev/shm/")); err != nil {
		return port.Incomplete
	}
	return ""
}

func credentialTmpfs(path string) bool {
	magic, err := credentialFilesystem(path)
	return err == nil && magic == 0x01021994
}
