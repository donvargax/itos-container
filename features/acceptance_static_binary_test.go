package features

import (
	"debug/buildinfo"
	"debug/elf"
	"path/filepath"
	"testing"
)

func TestAcceptanceStaticBinary(t *testing.T) {
	root, err := moduleRoot()
	if err != nil {
		t.Fatal(err)
	}
	// Force the inherited setting even on hosts whose default already disables cgo.
	t.Setenv("CGO_ENABLED", "1")
	engineDirs := map[bool]string{false: t.TempDir(), true: t.TempDir()}
	for _, builder := range []string{"cli", "engine-cli", "session-probe", "engine-probe"} {
		for _, covered := range []bool{false, true} {
			mode := "plain"
			if covered {
				mode = "covered"
			}
			t.Run(builder+"/"+mode, func(t *testing.T) {
				t.Setenv("GOCOVERDIR", "")
				t.Setenv("ITOS_CC_TEST_COVERDIR", "")
				coverdir := ""
				if covered {
					coverdir = t.TempDir()
					switch builder {
					case "cli":
						t.Setenv("GOCOVERDIR", coverdir)
					case "engine-cli", "engine-probe":
						t.Setenv("ITOS_CC_TEST_COVERDIR", coverdir)
					}
				}
				var binary string
				var err error
				switch builder {
				case "cli":
					binary, err = buildProduct(root, t.TempDir(), false)
				case "engine-cli":
					binary, err = buildProduct(root, engineDirs[covered], true)
				case "session-probe":
					dir := t.TempDir()
					w := &world{root: root, dir: dir, bin: filepath.Join(dir, "product"), coverdir: coverdir}
					binary, err = w.sessionProbeBinary()
				case "engine-probe":
					dir := engineDirs[covered]
					w := &world{root: root, dir: dir, bin: filepath.Join(dir, "product")}
					// Reuse the engine CLI's tree, but also support selecting this row alone.
					if productSource(root, w.bin) == root {
						if _, err := engineBudgetTree(root, dir); err != nil {
							t.Fatal(err)
						}
					}
					binary, err = w.productProbeBinary()
				}
				if err != nil {
					t.Fatal(err)
				}
				assertAcceptanceStaticBinary(t, binary, covered)
			})
		}
	}
}

func assertAcceptanceStaticBinary(t *testing.T, binary string, covered bool) {
	t.Helper()
	info, err := buildinfo.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	settings := make(map[string]string, len(info.Settings))
	for _, setting := range info.Settings {
		settings[setting.Key] = setting.Value
	}
	if got := settings["CGO_ENABLED"]; got != "0" {
		t.Errorf("CGO_ENABLED build setting = %q, want explicit 0", got)
	}
	switch got := settings["-cover"]; {
	case covered && got != "true":
		t.Errorf("-cover build setting = %q, want true", got)
	case !covered && got != "" && got != "false":
		t.Errorf("-cover build setting = %q, want absent or false", got)
	}
	goos := settings["GOOS"]
	if goos == "" {
		t.Fatal("missing GOOS build setting")
	}
	if goos != "linux" {
		return
	}
	artifact, err := elf.Open(binary)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = artifact.Close() }()
	for _, program := range artifact.Progs {
		if program.Type == elf.PT_INTERP {
			t.Error("Linux artifact has a PT_INTERP segment")
		}
	}
	libraries, err := artifact.DynString(elf.DT_NEEDED)
	if err != nil {
		t.Fatal(err)
	}
	if len(libraries) != 0 {
		t.Errorf("Linux artifact needs dynamic libraries: %v", libraries)
	}
}
