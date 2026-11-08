package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	"golang.org/x/sys/unix"
)

func emit(r report) {
	outputMu.Lock()
	defer outputMu.Unlock()
	if err := json.NewEncoder(os.Stdout).Encode(r); err != nil {
		panic(err)
	}
}

func frame(c config, process, kind, phase string) report {
	return report{Kind: kind, Nonce: c.Nonce, Phase: phase, Process: process,
		PID: os.Getpid(), PPID: os.Getppid(), UID: os.Geteuid(), GID: os.Getegid()}
}

func closeListeners(closers []func()) {
	var wg sync.WaitGroup
	for _, closeListener := range closers {
		wg.Go(closeListener)
	}
	wg.Wait()
}

func serve(c config) error {
	var closers []func()
	defer func() { closeListeners(closers) }()
	for _, t := range c.Targets {
		closeListener, err := listen(t, filepath.Join(c.Root, t.Path)+c.Suffix, func(value string) {
			r := frame(c, "listener", "receipt", "")
			r.Nonce, r.Target = value, t.Name
			emit(r)
		})
		if err != nil {
			return err
		}
		closers = append(closers, closeListener)
	}
	emit(frame(c, "listener", "ready", ""))
	input := json.NewDecoder(os.Stdin)
	for {
		var request struct{ Nonce, Phase string }
		if err := input.Decode(&request); err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		if request.Nonce != c.Nonce {
			return errors.New("foreign listener checkpoint")
		}
		emit(frame(c, "listener", "checkpoint", request.Phase))
	}
}

func control(c config, phase string) {
	r := frame(c, "host", "host-control", phase)
	for _, t := range c.Targets {
		r.Attempts = append(r.Attempts, operate(t, filepath.Join(c.Root, t.Path), c.Nonce+"/host/"+phase+"/"+t.Name))
	}
	emit(r)
}

func private(c config, process, phase string) error {
	dir, err := os.MkdirTemp("/tmp", "slice-19-private-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	r := frame(c, process, "private", phase)
	var stat unix.Statfs_t
	if err = unix.Statfs(dir, &stat); err != nil {
		return err
	}
	r.Tmpfs = stat.Type == unix.TMPFS_MAGIC
	var mu sync.Mutex
	var closers []func()
	defer func() { closeListeners(closers) }()
	seen := map[string]bool{}
	for _, t := range c.Targets {
		if seen[t.Op] {
			continue
		}
		seen[t.Op] = true
		path := filepath.Join(dir, t.Op)
		closeListener, e := listen(t, path, func(value string) {
			mu.Lock()
			r.Receipts = append(r.Receipts, value)
			mu.Unlock()
		})
		if e != nil {
			return e
		}
		closers = append(closers, closeListener)
		r.Attempts = append(r.Attempts, operate(t, path, c.Nonce+"/private/"+phase+"/"+t.Op))
	}
	mu.Lock()
	emit(r)
	mu.Unlock()
	return nil
}

func prepareAliases(c config, process, dir string) error {
	r := frame(c, process, "prepared", "live")
	for _, t := range c.Targets {
		path, alias := filepath.Join(c.Root, t.Path), filepath.Join(dir, t.Name)
		b, err := os.ReadFile(path)
		read := fileRead{Target: t.Name, Path: path, Data: string(b), Alias: alias}
		if err == nil {
			err = os.Symlink(path, alias)
		}
		if err != nil {
			read.Error = err.Error()
		}
		r.Reads = append(r.Reads, read)
	}
	emit(r)
	return nil
}

func phase(c config, process, name, aliasDir string) error {
	r := frame(c, process, "attempts", name)
	spellings := []string{"absolute"}
	if c.Mode == "replace" {
		spellings = append(spellings, "relative", "symlink")
	}
	for _, spelling := range spellings {
		for _, t := range c.Targets {
			path := filepath.Join(c.Root, t.Path)
			switch spelling {
			case "relative":
				path = t.Path
			case "symlink":
				path = filepath.Join(aliasDir, t.Name)
			}
			r.Attempts = append(r.Attempts, operate(t, path, c.Nonce+"/worker/"+name+"/"+spelling+"/"+t.Name))
		}
	}
	emit(r)
	return private(c, process, name)
}

func barrier(c config, process, name string, input *json.Decoder) error {
	emit(frame(c, process, "barrier", name))
	var reply struct{ Nonce, Phase string }
	if err := input.Decode(&reply); err != nil {
		return err
	}
	if reply.Nonce != c.Nonce || reply.Phase != name {
		return errors.New("foreign barrier release")
	}
	return nil
}

type child struct {
	cmd    *exec.Cmd
	input  io.WriteCloser
	output *json.Decoder
}

func startChild(c config) (*child, error) {
	self, err := os.Executable()
	if err != nil {
		return nil, err
	}
	b, err := json.Marshal(c)
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(self, "child", string(b))
	cmd.Stderr = os.Stderr
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		_ = in.Close()
		return nil, err
	}
	if err = cmd.Start(); err != nil {
		_ = in.Close()
		return nil, err
	}
	return &child{cmd: cmd, input: in, output: json.NewDecoder(out)}, nil
}

// Forward the child's own frames, retaining its actual PID/PPID. The parent
// never supplies a child attempt or substitutes its own socket results.
func (p *child) forward(until string) error {
	for {
		var r report
		if err := p.output.Decode(&r); err != nil {
			if err == io.EOF && until == "" {
				return p.cmd.Wait()
			}
			return err
		}
		emit(r)
		if r.Kind == "barrier" && r.Phase == until {
			return nil
		}
	}
}

func worker(c config, process string) error {
	input := json.NewDecoder(os.Stdin)
	// Existing sockets are the first application operations: no private control,
	// filesystem edit, child, or blocking handshake precedes these attempts.
	if c.Mode == "existing" {
		if err := phase(c, process, "first", ""); err != nil {
			return err
		}
		if err := barrier(c, process, "first", input); err != nil {
			return err
		}
		if err := phase(c, process, "running", ""); err != nil {
			return err
		}
	} else {
		aliasDir := ""
		if c.Mode == "replace" {
			var err error
			aliasDir, err = os.MkdirTemp("/tmp", "slice-19-alias-")
			if err != nil {
				return err
			}
			defer os.RemoveAll(aliasDir)
			if err = prepareAliases(c, process, aliasDir); err != nil {
				return err
			}
		}
		if process == "child" {
			if c.Mode == "replace" {
				if err := barrier(c, process, "child-ready", input); err != nil {
					return err
				}
			}
			return phase(c, process, "child", aliasDir)
		}
		var child *child
		var err error
		if c.Mode == "replace" {
			child, err = startChild(c)
			if err != nil {
				return err
			}
			defer child.input.Close()
			if err = child.forward("child-ready"); err != nil {
				return err
			}
		}
		if err = barrier(c, process, "live", input); err != nil {
			return err
		}
		if err = phase(c, process, "parent", aliasDir); err != nil {
			return err
		}
		if err = barrier(c, process, "parent", input); err != nil {
			return err
		}
		if c.Mode == "late" {
			child, err = startChild(c)
			if err != nil {
				return err
			}
			defer child.input.Close()
		} else if err = json.NewEncoder(child.input).Encode(map[string]string{"Nonce": c.Nonce, "Phase": "child-ready"}); err != nil {
			return err
		}
		if err = child.forward(""); err != nil {
			return err
		}
	}
	if err := os.WriteFile(filepath.Join(c.Root, "slice-19-edit"), []byte(c.Nonce), 0600); err != nil {
		return err
	}
	return barrier(c, process, "exit", input)
}

func main() {
	if len(os.Args) < 3 {
		panic("expected trusted probe mode and config")
	}
	var c config
	if err := json.Unmarshal([]byte(os.Args[2]), &c); err != nil {
		panic(err)
	}
	var err error
	switch os.Args[1] {
	case "serve":
		err = serve(c)
	case "control":
		if len(os.Args) != 4 {
			panic("missing host phase")
		}
		control(c, os.Args[3])
	case "worker":
		err = worker(c, "parent")
	case "child":
		err = worker(c, "child")
	default:
		err = fmt.Errorf("unknown trusted mode %q", os.Args[1])
	}
	if err != nil {
		r := frame(c, os.Args[1], "error", "")
		r.Error = err.Error()
		emit(r)
		os.Exit(1)
	}
}
