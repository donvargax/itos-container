package features

import (
	"bytes"
	"debug/elf"
	"os"
	"path/filepath"
	"testing"
)

// The synthetic executables are read here by the standard library alone, so a
// scenario's refusal is never the fixture's own malformed input.
func TestEgressSyntheticELFCarriesExactlyItsProperty(t *testing.T) {
	const amd64, arm64 = 62, 183
	for _, tc := range []struct {
		name                 string
		data                 []byte
		class                elf.Class
		machine              elf.Machine
		interpreter, dynamic bool
	}{
		{"static control", egressELF(64, amd64, false, false), elf.ELFCLASS64, elf.EM_X86_64, false, false},
		{"interpreter", egressELF(64, amd64, true, true), elf.ELFCLASS64, elf.EM_X86_64, true, true},
		{"dynamic dependency", egressELF(64, amd64, false, true), elf.ELFCLASS64, elf.EM_X86_64, false, true},
		{"another machine", egressELF(64, arm64, false, false), elf.ELFCLASS64, elf.EM_AARCH64, false, false},
		{"wrong class", egressELF(32, amd64, false, false), elf.ELFCLASS32, elf.EM_X86_64, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			file, err := elf.NewFile(bytes.NewReader(tc.data))
			if err != nil {
				t.Fatal(err)
			}
			interpreter, dynamic := false, false
			for _, program := range file.Progs {
				interpreter = interpreter || program.Type == elf.PT_INTERP
				dynamic = dynamic || program.Type == elf.PT_DYNAMIC
			}
			if file.Class != tc.class || file.Machine != tc.machine || file.Type != elf.ET_EXEC || file.OSABI != elf.ELFOSABI_NONE || interpreter != tc.interpreter || dynamic != tc.dynamic {
				t.Errorf("class %v machine %v type %v interpreter %v dynamic %v", file.Class, file.Machine, file.Type, interpreter, dynamic)
			}
		})
	}
	if _, err := elf.NewFile(bytes.NewReader(egressELF(64, amd64, false, false)[:40])); err == nil {
		t.Error("the truncated input still parses")
	}
}

// The harness's own reading of a product executable, which EGRESS-04 holds
// doctor's "ready" against: static and outside passes, every defect fails.
func TestEgressStaticOutsideRefusesEachDefect(t *testing.T) {
	const amd64 = 62
	root := filepath.Join(t.TempDir(), "checkout")
	outside := t.TempDir()
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	write := func(dir, name string, data []byte) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, data, 0700); err != nil {
			t.Fatal(err)
		}
		return path
	}
	static := egressELF(64, amd64, false, false)
	if err := egressStaticOutside(write(outside, "control", static), root); err != nil {
		t.Errorf("static control outside the checkout refused: %v", err)
	}
	link := filepath.Join(outside, "link")
	if err := os.Symlink(write(root, "inside", static), link); err != nil {
		t.Fatal(err)
	}
	for name, path := range map[string]string{
		"inside the checkout": filepath.Join(root, "inside"), "symlink into the checkout": link,
		"interpreter": write(outside, "interp", egressELF(64, amd64, true, true)), "dynamic": write(outside, "dynamic", egressELF(64, amd64, false, true)),
		"another machine": write(outside, "arm64", egressELF(64, 183, false, false)), "wrong class": write(outside, "x32", egressELF(32, amd64, false, false)),
		"truncated": write(outside, "short", static[:40]), "absent": filepath.Join(outside, "absent"),
	} {
		if egressStaticOutside(path, root) == nil {
			t.Errorf("%s was read as a static executable outside the checkout", name)
		}
	}
}
