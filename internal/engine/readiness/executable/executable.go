// Package executable observes the controller's own program without running it.
package executable

import (
	"debug/elf"
	"os"
	"path/filepath"

	"github.com/donvargax/itos-container/internal/engine/readiness/port"
)

// Program reads the file Path names; nil is the process's own executable, as
// the operating system reports it, never PATH or a name the checkout supplies.
// It opens the file read-only and reads its ELF headers. It runs nothing.
type Program struct {
	Path func() (string, error)
}

var _ port.Executables = Program{}

const magic = "\x7fELF"

// Observe resolves every symbolic link, then reports what the opened file is.
// A file that cannot be located by an absolute name, resolved, opened or, being
// ELF, parsed is a failure with no reading beside it. A file that is not
// regular or not ELF is observed as such, with no ELF reading.
func (p Program) Observe() (port.Executable, port.Failure) {
	locate := p.Path
	if locate == nil {
		locate = os.Executable
	}
	named, err := locate()
	if err != nil || !filepath.IsAbs(named) {
		return unobserved()
	}
	resolved, err := filepath.EvalSymlinks(named)
	if err != nil {
		return unobserved()
	}
	file, err := os.OpenFile(resolved, openFlags, 0) // #nosec G304 -- the process's own executable, read-only.
	if err != nil {
		return unobserved()
	}
	defer func() { _ = file.Close() }()
	// The opened file is the one judged: the name may be replaced afterwards.
	observed := port.Executable{Path: resolved}
	if info, err := file.Stat(); err != nil || !info.Mode().IsRegular() {
		return observed, port.None
	}
	observed.Regular = true
	start := make([]byte, len(magic))
	if n, _ := file.ReadAt(start, 0); string(start[:n]) != magic {
		return observed, port.None
	}
	image, err := elf.NewFile(file)
	if err != nil {
		return unobserved()
	}
	observed.OS, observed.Architecture = platform(image.FileHeader)
	for _, program := range image.Progs {
		switch program.Type {
		case elf.PT_INTERP:
			observed.Interpreter = true
		case elf.PT_DYNAMIC:
			observed.Dynamic = true
		}
	}
	for _, section := range image.Sections {
		if section.Type == elf.SHT_DYNAMIC {
			observed.Dynamic = true
		}
	}
	return observed, port.None
}

func unobserved() (port.Executable, port.Failure) {
	return port.Executable{}, port.EgressExecutable
}

// platform names what the header says the image is for. Only a little-endian
// 64-bit x86-64 image is amd64; the same machine in the 32-bit class is not.
// Linux images carry the System V or the Linux ABI.
func platform(header elf.FileHeader) (system, architecture string) {
	if header.OSABI == elf.ELFOSABI_NONE || header.OSABI == elf.ELFOSABI_LINUX {
		system = "linux"
	}
	little := header.Data == elf.ELFDATA2LSB
	switch {
	case header.Machine == elf.EM_X86_64 && header.Class == elf.ELFCLASS64 && little:
		architecture = "amd64"
	case header.Machine == elf.EM_X86_64 && header.Class == elf.ELFCLASS32 && little:
		architecture = "amd64p32"
	case header.Machine == elf.EM_AARCH64 && header.Class == elf.ELFCLASS64 && little:
		architecture = "arm64"
	default:
		architecture = "unsupported"
	}
	return system, architecture
}
