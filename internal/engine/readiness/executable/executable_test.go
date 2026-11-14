package executable

import (
	"bytes"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/donvargax/itos-container/internal/engine/readiness/port"
	"github.com/donvargax/itos-container/internal/engine/readiness/port/porttest"
)

const (
	amd64, arm64, riscv = 62, 183, 243
	systemV, linuxABI   = 0, 3
	freeBSD             = 9
)

// synthetic is the smallest ELF image carrying exactly the named properties.
// It is data for the inspector: no loader could run it and nothing tries.
type synthetic struct {
	bits                 int
	abi                  byte
	machine              uint16
	interpreter, segment bool
	// section is the type of the one named section of a section table, or
	// zero for an image with no section table at all.
	section   uint32
	bigEndian bool
}

const dynamicSection, programSection = 6, 1

func static() synthetic { return synthetic{bits: 64, abi: systemV, machine: amd64} }

func (s synthetic) bytes() []byte {
	const load, dynamic, interp = 1, 2, 3
	var order binary.ByteOrder = binary.LittleEndian
	if s.bigEndian {
		order = binary.BigEndian
	}
	type segment struct {
		kind uint32
		data []byte
	}
	var segments []segment
	if s.interpreter {
		segments = append(segments, segment{interp, []byte("/lib64/ld-linux-x86-64.so.2\x00")})
	}
	if s.segment {
		segments = append(segments, segment{dynamic, make([]byte, 16)}) // DT_NULL alone
	}
	headerSize, entrySize := 64, 56
	if s.bits == 32 {
		headerSize, entrySize = 52, 32
	}
	count := 1 + len(segments)
	offset := headerSize + entrySize*count
	total := offset
	for _, part := range segments {
		total += len(part.data)
	}
	ident := []byte{0x7f, 'E', 'L', 'F', 2, 1, 1, s.abi, 0, 0, 0, 0, 0, 0, 0, 0}
	if s.bits == 32 {
		ident[4] = 1
	}
	if s.bigEndian {
		ident[5] = 2
	}
	var body bytes.Buffer
	body.Write(ident)
	put := func(values ...any) {
		for _, value := range values {
			_ = binary.Write(&body, order, value)
		}
	}
	// A section table is written only for the 64-bit image that asks for one:
	// a null section, the one section asked for and the section name table.
	names := []byte("\x00.section\x00.shstrtab\x00")
	sections, sectionOffset := 0, 0
	if s.section != 0 {
		sections, sectionOffset = 3, total+16+len(names)
	}
	const base = 0x400000
	if s.bits == 64 {
		put(uint16(2), s.machine, uint32(1), uint64(base+offset), uint64(headerSize), uint64(sectionOffset), uint32(0), uint16(headerSize), uint16(entrySize), uint16(count), uint16(64), uint16(sections), uint16(max(sections-1, 0)))
		put(uint32(load), uint32(5), uint64(0), uint64(base), uint64(base), uint64(total), uint64(total), uint64(0x1000))
		at := offset
		for _, part := range segments {
			put(part.kind, uint32(4), uint64(at), uint64(base+at), uint64(base+at), uint64(len(part.data)), uint64(len(part.data)), uint64(1))
			at += len(part.data)
		}
	} else {
		put(uint16(2), s.machine, uint32(1), uint32(base+offset), uint32(headerSize), uint32(0), uint32(0), uint16(headerSize), uint16(entrySize), uint16(count), uint16(40), uint16(0), uint16(0))
		put(uint32(load), uint32(0), uint32(base), uint32(base), uint32(total), uint32(total), uint32(5), uint32(0x1000))
		at := offset
		for _, part := range segments {
			put(part.kind, uint32(at), uint32(base+at), uint32(base+at), uint32(len(part.data)), uint32(len(part.data)), uint32(4), uint32(1))
			at += len(part.data)
		}
	}
	for _, part := range segments {
		body.Write(part.data)
	}
	if s.section != 0 {
		body.Write(make([]byte, 16)) // the section's content: one empty entry
		body.Write(names)
		put(uint32(0), uint32(0), uint64(0), uint64(0), uint64(0), uint64(0), uint32(0), uint32(0), uint64(0), uint64(0))
		put(uint32(1), s.section, uint64(0), uint64(0), uint64(total), uint64(16), uint32(2), uint32(0), uint64(8), uint64(16))
		put(uint32(10), uint32(3), uint64(0), uint64(0), uint64(total+16), uint64(len(names)), uint32(0), uint32(0), uint64(1), uint64(0))
	}
	return body.Bytes()
}

func write(t *testing.T, dir, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, 0700); err != nil {
		t.Fatal(err)
	}
	return path
}

// real is where a name resolves, as the operating system follows its links: a
// temporary directory may itself sit behind one.
func real(t *testing.T, path string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

func at(path string) Program { return Program{Path: func() (string, error) { return path, nil }} }

func link(t *testing.T, target, name string) {
	t.Helper()
	if err := os.Symlink(target, name); err != nil {
		t.Skipf("no symbolic links here: %v", err)
	}
}

func TestProgramHoldsTheSharedContractOverSyntheticImages(t *testing.T) {
	root := t.TempDir()
	checkout, installed := filepath.Join(root, "checkout"), filepath.Join(root, "installed")
	for _, dir := range []string{checkout, installed} {
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	images := map[string]synthetic{
		porttest.ExecutableControl:              static(),
		"a resolved executable in the checkout": static(),
		"ELF with PT_INTERP":                    {bits: 64, machine: amd64, interpreter: true, segment: true},
		"ELF with a dynamic dependency":         {bits: 64, machine: amd64, segment: true},
		"ELF for another machine":               {bits: 64, machine: arm64},
		"ELF with the wrong class":              {bits: 32, machine: amd64},
	}
	porttest.ExecutableContract(t, func(name string) (port.Executables, string) {
		switch name {
		case "a resolved executable in the checkout":
			path := write(t, checkout, "itos-container", images[name].bytes())
			return at(path), real(t, path)
		case "a symlink resolving into the checkout":
			target := write(t, checkout, "linked", static().bytes())
			named := filepath.Join(installed, "link")
			link(t, target, named)
			return at(named), real(t, target)
		case "truncated ELF":
			path := write(t, installed, "truncated", static().bytes()[:40])
			return at(path), real(t, path)
		case "unavailable executable observation":
			path := filepath.Join(installed, "absent")
			return at(path), path
		}
		image, known := images[name]
		if !known {
			t.Fatalf("no synthetic image for %q", name)
		}
		path := write(t, installed, "image-"+filepath.Base(t.Name())+"-"+name[len(name)-5:], image.bytes())
		return at(path), real(t, path)
	})
}

func TestProgramReadsEachPropertyOfAnImage(t *testing.T) {
	dir := t.TempDir()
	for name, tc := range map[string]struct {
		image synthetic
		want  port.Executable
	}{
		"static System V amd64":        {static(), port.Executable{Regular: true, OS: "linux", Architecture: "amd64"}},
		"static Linux ABI amd64":       {synthetic{bits: 64, abi: linuxABI, machine: amd64}, port.Executable{Regular: true, OS: "linux", Architecture: "amd64"}},
		"another operating system":     {synthetic{bits: 64, abi: freeBSD, machine: amd64}, port.Executable{Regular: true, Architecture: "amd64"}},
		"arm64":                        {synthetic{bits: 64, machine: arm64}, port.Executable{Regular: true, OS: "linux", Architecture: "arm64"}},
		"a machine with no name here":  {synthetic{bits: 64, machine: riscv}, port.Executable{Regular: true, OS: "linux", Architecture: "unsupported"}},
		"x86-64 in the 32-bit class":   {synthetic{bits: 32, machine: amd64}, port.Executable{Regular: true, OS: "linux", Architecture: "amd64p32"}},
		"arm64 in the 32-bit class":    {synthetic{bits: 32, machine: arm64}, port.Executable{Regular: true, OS: "linux", Architecture: "unsupported"}},
		"an interpreter alone":         {synthetic{bits: 64, machine: amd64, interpreter: true}, port.Executable{Regular: true, OS: "linux", Architecture: "amd64", Interpreter: true}},
		"a dynamic segment alone":      {synthetic{bits: 64, machine: amd64, segment: true}, port.Executable{Regular: true, OS: "linux", Architecture: "amd64", Dynamic: true}},
		"a dynamic section alone":      {synthetic{bits: 64, machine: amd64, section: dynamicSection}, port.Executable{Regular: true, OS: "linux", Architecture: "amd64", Dynamic: true}},
		"sections, none dynamic":       {synthetic{bits: 64, machine: amd64, section: programSection}, port.Executable{Regular: true, OS: "linux", Architecture: "amd64"}},
		"x86-64 in big-endian order":   {synthetic{bits: 64, machine: amd64, bigEndian: true}, port.Executable{Regular: true, OS: "linux", Architecture: "unsupported"}},
		"arm64 in big-endian order":    {synthetic{bits: 64, machine: arm64, bigEndian: true}, port.Executable{Regular: true, OS: "linux", Architecture: "unsupported"}},
		"32-bit x86-64, big-endian":    {synthetic{bits: 32, machine: amd64, bigEndian: true}, port.Executable{Regular: true, OS: "linux", Architecture: "unsupported"}},
		"an interpreter and a segment": {synthetic{bits: 64, machine: amd64, interpreter: true, segment: true}, port.Executable{Regular: true, OS: "linux", Architecture: "amd64", Interpreter: true, Dynamic: true}},
	} {
		path := write(t, dir, "image", tc.image.bytes())
		tc.want.Path = real(t, path)
		if got, failure := at(path).Observe(); failure != port.None || got != tc.want {
			t.Errorf("%s: observed %+v (failure %v), want %+v", name, got, failure, tc.want)
		}
	}
}

func TestProgramObservesAFileThatIsNotAnImageWithoutAnELFReading(t *testing.T) {
	dir := t.TempDir()
	for name, data := range map[string][]byte{
		"empty": nil, "shorter than the magic": []byte("\x7fEL"), "a script": []byte("#!/bin/sh\nexit 0\n"),
		"the magic displaced": append([]byte{0}, static().bytes()...), "another format": []byte("\xcf\xfa\xed\xfe\x07\x00\x00\x01"),
	} {
		path := write(t, dir, "file", data)
		want := port.Executable{Path: real(t, path), Regular: true}
		if got, failure := at(path).Observe(); failure != port.None || got != want {
			t.Errorf("%s: observed %+v (failure %v), want %+v", name, got, failure, want)
		}
	}
	// A directory is opened and seen for what it is, never read as a file.
	want := port.Executable{Path: real(t, dir)}
	if got, failure := at(dir).Observe(); failure != port.None || got != want {
		t.Errorf("a directory: observed %+v (failure %v), want %+v", got, failure, want)
	}
}

func TestProgramFailsWhereNoObservationCanBeMade(t *testing.T) {
	dir := t.TempDir()
	image := write(t, dir, "itos-container", static().bytes())
	for name, program := range map[string]Program{
		"the system names no program":         {Path: func() (string, error) { return "", nil }},
		"the system fails to name it":         {Path: func() (string, error) { return image, errors.New("unavailable") }},
		"a name relative to somewhere":        at("itos-container"),
		"a name with no file":                 at(filepath.Join(dir, "absent")),
		"a name below a file":                 at(filepath.Join(image, "below")),
		"an image cut short":                  at(write(t, dir, "short", static().bytes()[:40])),
		"an image with only its identifier":   at(write(t, dir, "ident", static().bytes()[:16])),
		"an image whose order contradicts it": at(write(t, dir, "big", static().orderFlipped())),
		"an image whose tables lie past it":   at(write(t, dir, "sections", static().sectionsBeyond())),
	} {
		if got, failure := program.Observe(); failure != port.EgressExecutable || got != (port.Executable{}) {
			t.Errorf("%s: observed %+v with failure %v", name, got, failure)
		}
	}
}

// orderFlipped is the static image with its byte order byte changed alone, so
// every field after the identifier reads as nonsense.
func (s synthetic) orderFlipped() []byte {
	data := s.bytes()
	data[5] = 2
	return data
}

// sectionsBeyond is the static image claiming a section table past its end.
func (s synthetic) sectionsBeyond() []byte {
	data := s.bytes()
	binary.LittleEndian.PutUint64(data[40:], uint64(len(data)+4096)) // e_shoff
	binary.LittleEndian.PutUint16(data[60:], 3)                      // e_shnum
	binary.LittleEndian.PutUint16(data[62:], 2)                      // e_shstrndx
	return data
}

func TestProgramResolvesEveryLinkBeforeItReads(t *testing.T) {
	dir := t.TempDir()
	target := write(t, dir, "itos-container", static().bytes())
	first, second := filepath.Join(dir, "first"), filepath.Join(dir, "second")
	link(t, target, first)
	link(t, first, second)
	want := port.Executable{Path: real(t, target), Regular: true, OS: "linux", Architecture: "amd64"}
	if got, failure := at(second).Observe(); failure != port.None || got != want {
		t.Errorf("observed %+v (failure %v), want %+v", got, failure, want)
	}
	dangling := filepath.Join(dir, "dangling")
	link(t, filepath.Join(dir, "absent"), dangling)
	if got, failure := at(dangling).Observe(); failure != port.EgressExecutable || got != (port.Executable{}) {
		t.Errorf("a dangling link: observed %+v with failure %v", got, failure)
	}
}

func TestProgramFailsOnAnImageItMayNotOpen(t *testing.T) {
	path := write(t, t.TempDir(), "itos-container", static().bytes())
	if err := os.Chmod(path, 0); err != nil {
		t.Fatal(err)
	}
	if file, err := os.Open(path); err == nil {
		_ = file.Close()
		t.Skip("this user and system open a file whatever its mode")
	}
	if got, failure := at(path).Observe(); failure != port.EgressExecutable || got != (port.Executable{}) {
		t.Errorf("observed %+v with failure %v", got, failure)
	}
}

// With no Path the program observed is this test's own: whatever its format
// here, a regular file at the place the system reports, read the same twice.
func TestProgramObservesTheProcessItself(t *testing.T) {
	own, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	first, failure := Program{}.Observe()
	if failure != port.None || first.Path != real(t, own) || !first.Regular {
		t.Fatalf("observed %+v with failure %v", first, failure)
	}
	if runtime.GOOS == "linux" && (first.OS != "linux" || (first.Architecture == "amd64") != (runtime.GOARCH == "amd64")) {
		t.Errorf("a Linux %s test program was read as %s %s", runtime.GOARCH, first.OS, first.Architecture)
	}
	if runtime.GOOS != "linux" && runtime.GOOS != "freebsd" && first.OS == "linux" && first.Architecture == "amd64" && !first.Dynamic && !first.Interpreter {
		t.Errorf("a %s test program was read as a static Linux amd64 image", runtime.GOOS)
	}
	if second, failure := (Program{}).Observe(); failure != port.None || second != first {
		t.Errorf("a second observation differs: %+v (failure %v)", second, failure)
	}
}
