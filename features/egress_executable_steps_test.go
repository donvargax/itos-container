package features

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

func (w *world) egressExecutableSteps(sc *godog.ScenarioContext) {
	sc.Step(`^an egress executable contract with a static Linux amd64 control and defect "([^"]*)"$`, w.egressContractFixture)
	sc.Step(`^the shared egress readiness contract inspects both inputs through the fake and real observation adapter$`, w.egressContractProbe)
	sc.Step(`^each backend admits the outside static control and refuses the defect as "([^"]*)" with exit (\d+)$`, w.egressContractJudgement)
	sc.Step(`^the defect fix names the installed static Linux amd64 release outside the checkout$`, func() error {
		return w.egressContractEach(func(backend string, _, defect map[string]any) error {
			fix := strings.ToLower(fmt.Sprint(defect["Fix"]))
			for _, word := range []string{"installed", "static", "linux amd64", "release", "outside", "checkout"} {
				if !strings.Contains(fix, word) {
					return fmt.Errorf("%s: fix %q does not name %q", backend, defect["Fix"], word)
				}
			}
			return nil
		})
	})
	sc.Step(`^neither inspection executes a helper or changes a file$`, w.egressContractUntouched)
}

// egressELF writes the smallest executable image that carries one property: a
// loadable header table and, where asked, an interpreter or a dynamic segment.
// It is data for an inspector. No loader could run it and no step tries.
func egressELF(bits int, machine uint16, interpreter, dynamic bool) []byte {
	const load, dynamicType, interpType = 1, 2, 3
	order := binary.LittleEndian
	type segment struct {
		kind uint32
		data []byte
	}
	var segments []segment
	if interpreter {
		segments = append(segments, segment{interpType, []byte("/lib64/ld-linux-x86-64.so.2\x00")})
	}
	if dynamic {
		// DT_NEEDED (1) naming offset 1 of the string table, then DT_NULL.
		table := make([]byte, 32)
		order.PutUint64(table[0:], 1)
		order.PutUint64(table[8:], 1)
		segments = append(segments, segment{dynamicType, append(table, []byte("\x00libc.so.6\x00")...)})
	}
	headerSize, entrySize := 64, 56
	if bits == 32 {
		headerSize, entrySize = 52, 32
	}
	count := 1 + len(segments)
	offset := headerSize + entrySize*count
	var body bytes.Buffer
	ident := []byte{0x7f, 'E', 'L', 'F', 2, 1, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	if bits == 32 {
		ident[4] = 1
	}
	body.Write(ident)
	put := func(values ...any) {
		for _, value := range values {
			_ = binary.Write(&body, order, value)
		}
	}
	total := offset
	for _, s := range segments {
		total += len(s.data)
	}
	const base = 0x400000
	if bits == 64 {
		put(uint16(2), machine, uint32(1), uint64(base+offset), uint64(headerSize), uint64(0), uint32(0), uint16(headerSize), uint16(entrySize), uint16(count), uint16(64), uint16(0), uint16(0))
		put(uint32(load), uint32(5), uint64(0), uint64(base), uint64(base), uint64(total), uint64(total), uint64(0x1000))
		at := offset
		for _, s := range segments {
			put(s.kind, uint32(4), uint64(at), uint64(base+at), uint64(base+at), uint64(len(s.data)), uint64(len(s.data)), uint64(1))
			at += len(s.data)
		}
	} else {
		put(uint16(2), machine, uint32(1), uint32(base+offset), uint32(headerSize), uint32(0), uint32(0), uint16(headerSize), uint16(entrySize), uint16(count), uint16(40), uint16(0), uint16(0))
		put(uint32(load), uint32(0), uint32(base), uint32(base), uint32(total), uint32(total), uint32(5), uint32(0x1000))
		at := offset
		for _, s := range segments {
			put(s.kind, uint32(at), uint32(base+at), uint32(base+at), uint32(len(s.data)), uint32(len(s.data)), uint32(4), uint32(1))
			at += len(s.data)
		}
	}
	for _, s := range segments {
		body.Write(s.data)
	}
	return body.Bytes()
}

func (w *world) egressContractFixture(defect string) error {
	const amd64, arm64 = 62, 183
	e := w.egressFixture()
	e.contractDir = filepath.Join(w.dir, "slice-27-contract")
	checkout, outside := filepath.Join(e.contractDir, "checkout"), filepath.Join(e.contractDir, "installed")
	tools, recorder := filepath.Join(e.contractDir, "bin"), filepath.Join(e.contractDir, "recorder")
	for _, dir := range []string{checkout, outside, tools, recorder} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return err
		}
	}
	// Any helper an inspector might be tempted to run records that it ran.
	marker := "#!/bin/sh\nprintf executed > '" + filepath.Join(recorder, "executed") + "'\n"
	for _, name := range []string{"ldd", "readelf", "objdump", "file", "nm", "sh", "itos-container"} {
		if err := os.WriteFile(filepath.Join(tools, name), []byte(marker), 0700); err != nil {
			return err
		}
	}
	static := egressELF(64, amd64, false, false)
	control := filepath.Join(outside, "itos-container")
	if err := os.WriteFile(control, static, 0700); err != nil {
		return err
	}
	input := filepath.Join(outside, "slice-27-defect")
	var err error
	switch defect {
	case "a resolved executable in the checkout":
		input = filepath.Join(checkout, "itos-container")
		err = os.WriteFile(input, static, 0700)
	case "a symlink resolving into the checkout":
		if err = os.WriteFile(filepath.Join(checkout, "itos-container"), static, 0700); err == nil {
			err = os.Symlink(filepath.Join(checkout, "itos-container"), input)
		}
	case "ELF with PT_INTERP":
		err = os.WriteFile(input, egressELF(64, amd64, true, true), 0700)
	case "ELF with a dynamic dependency":
		err = os.WriteFile(input, egressELF(64, amd64, false, true), 0700)
	case "ELF for another machine":
		err = os.WriteFile(input, egressELF(64, arm64, false, false), 0700)
	case "ELF with the wrong class":
		err = os.WriteFile(input, egressELF(32, amd64, false, false), 0700)
	case "truncated ELF":
		err = os.WriteFile(input, static[:40], 0700)
	case "unavailable executable observation":
		// The path names nothing: no observation can be made of it.
	default:
		return fmt.Errorf("unknown executable defect %q", defect)
	}
	if err != nil {
		return err
	}
	request := map[string]string{"Root": checkout, "Control": control, "Defect": input, "Kind": defect, "Output": filepath.Join(w.dir, "slice-27-contract-result.json")}
	if err = os.WriteFile(filepath.Join(w.dir, "slice-27-contract-input.json"), approvalJSON(request), 0600); err != nil {
		return err
	}
	e.contractBefore, err = egressTree(e.contractDir)
	return err
}

func (w *world) egressContractProbe() error {
	e := w.egressFixture()
	bin, err := w.sessionProbeBinary()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	args := []string{"-test.run=^TestEgressExecutableContractProbe$", "-test.count=1"}
	if cover := w.coverageDir(); cover != "" {
		args = append(args, "-test.gocoverdir="+cover)
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = w.dir
	cmd.Env = w.coverageEnv(sessionChildEnvironment(os.Environ()))
	// The only programs on the child's PATH are the fixture's recording helpers.
	cmd.Env = setEnv(cmd.Env, "PATH", filepath.Join(e.contractDir, "bin"))
	cmd.Env = setEnv(cmd.Env, "SLICE_27_PROBE_INPUT", filepath.Join(w.dir, "slice-27-contract-input.json"))
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("executable contract fixture: %w\n%s", err, out)
	}
	data, err := os.ReadFile(filepath.Join(w.dir, "slice-27-contract-result.json"))
	if err != nil {
		return fmt.Errorf("executable contract fixture wrote no observations: %w", err)
	}
	return json.Unmarshal(data, &e.contract)
}

func (w *world) egressContractEach(check func(backend string, control, defect map[string]any) error) error {
	e := w.egressFixture()
	for _, backend := range []string{"fake", "real"} {
		observed := object(e.contract[backend])
		control, defect := object(observed["control"]), object(observed["defect"])
		if control == nil || defect == nil {
			return fmt.Errorf("missing %s observations", backend)
		}
		if err := check(backend, control, defect); err != nil {
			return err
		}
	}
	return nil
}

func (w *world) egressContractJudgement(rule string, exit int) error {
	return w.egressContractEach(func(backend string, control, defect map[string]any) error {
		if control["Admitted"] != true || control["Rule"] != "" {
			return fmt.Errorf("%s: the outside static control was refused as %v with exit %v", backend, control["Rule"], control["Exit"])
		}
		if defect["Admitted"] != false || defect["Rule"] != rule || defect["Exit"] != float64(exit) {
			return fmt.Errorf("%s: the defect was judged %v/%v/%v, want a refusal as %s with exit %d", backend, defect["Admitted"], defect["Rule"], defect["Exit"], rule, exit)
		}
		if strings.TrimSpace(fmt.Sprint(defect["Message"])) == "" || defect["Message"] == nil {
			return fmt.Errorf("%s: the refusal explains nothing", backend)
		}
		// Each backend must have observed what the shared contract says the
		// input holds; a refusal for another reason proves nothing about it.
		for _, input := range []map[string]any{control, defect} {
			if failures := anyList(input["Contract"]); len(failures) != 0 {
				return fmt.Errorf("%s: observation differs from the shared contract: %v", backend, failures)
			}
		}
		return nil
	})
}

func (w *world) egressContractUntouched() error {
	e := w.egressFixture()
	after, err := egressTree(e.contractDir)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(after, e.contractBefore) {
		return fmt.Errorf("an inspection changed its inputs or ran a recording helper")
	}
	entries, err := os.ReadDir(filepath.Join(e.contractDir, "recorder"))
	if err != nil {
		return err
	}
	if len(entries) != 0 {
		return fmt.Errorf("an inspection executed a helper")
	}
	return w.egressContractEach(func(backend string, control, defect map[string]any) error {
		// The fake holds no file: it can only have been asked.
		if backend == "fake" && (control["Calls"] == float64(0) || defect["Calls"] == float64(0)) {
			return fmt.Errorf("the fake was never consulted")
		}
		return nil
	})
}
