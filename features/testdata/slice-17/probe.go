// The harness builds this trusted reporting workload for Linux amd64. It is
// executed only in the CI worker, never as checkout code on the host.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

type report struct {
	Phase, Identity, Nonce, CWD string
	Denied                      map[string]bool
	Controls                    map[string]bool
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(91)
	}
}

func run() error {
	var st syscall.Stat_t
	if err := syscall.Stat("/workspace", &st); err != nil {
		return err
	}
	nonce, err := os.ReadFile("/workspace/slice-17-nonce")
	if err != nil {
		return err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	out, in := json.NewEncoder(os.Stdout), json.NewDecoder(os.Stdin)
	r := report{Phase: "ready", Identity: fmt.Sprintf("%d:%d", st.Dev, st.Ino), Nonce: string(nonce), CWD: cwd}
	if err = out.Encode(r); err != nil {
		return err
	}
	var targets []string
	if err = in.Decode(&targets); err != nil {
		return err
	}
	r.Phase, r.Denied, r.Controls = "checked", map[string]bool{}, map[string]bool{}
	for _, path := range append([]string{"/workspace/slice-17-replacement", "/workspace/slice-17-nested/marker"}, targets...) {
		_, err = os.ReadFile(path)
		r.Denied[path+":read"] = err != nil
	}
	for i, path := range targets {
		for _, operation := range []string{"append", "rename", "unlink"} {
			for _, phase := range []string{"before", "after"} {
				control := filepath.Join("/tmp", fmt.Sprintf("slice-17-%d-%s-%s", i, operation, phase))
				if err = os.WriteFile(control, []byte("control"), 0600); err != nil {
					return err
				}
				r.Controls[path+":"+operation+":"+phase] = mutate(control, operation) == nil
				if phase == "before" {
					r.Denied[path+":"+operation] = mutate(path, operation) != nil
				}
			}
		}
	}
	if err = os.WriteFile("slice-17-edit", []byte(r.Nonce), 0600); err != nil {
		return err
	}
	if err = out.Encode(r); err != nil {
		return err
	}
	// Keep the workload alive until the independent record/mount observers finish.
	var release bool
	return in.Decode(&release)
}

func mutate(path, operation string) error {
	switch operation {
	case "append":
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0)
		if err != nil {
			return err
		}
		_, err = f.WriteString("slice-17-worker-mutation")
		closeErr := f.Close()
		if err != nil {
			return err
		}
		return closeErr
	case "rename":
		return os.Rename(path, path+".slice-17-moved")
	default:
		return os.Remove(path)
	}
}
