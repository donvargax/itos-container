package features

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"time"
)

// Wire values belong to the trusted probe protocol, not a product API.
type socketTarget struct{ Name, Op, Path string }
type socketAttempt struct {
	Target, Operation, Path, Nonce          string
	Invoked, Connected, Sent                bool
	Errno                                   int
	Error, SetupError, ExchangeError, Reply string
}
type socketRead struct{ Target, Path, Data, Alias, Error string }
type socketReport struct {
	Kind, Nonce, Phase, Process, Target, Error string
	PID, PPID, UID, GID                        int
	Attempts                                   []socketAttempt
	Reads                                      []socketRead
	Receipts                                   []string
	Tmpfs                                      bool
}
type socketConfig struct {
	Nonce, Mode, Root, Suffix string
	Targets                   []socketTarget
}
type socketIdentity struct {
	Inode, UID, GID string
	Mode            os.FileMode
}
type socketWorld struct {
	w              *world
	config         socketConfig
	probe          string
	listener       *socketProcess
	listenerCancel context.CancelFunc
	identities     map[string]socketIdentity
	original       map[string]sessionRecord
	publications   map[string]socketIdentity
	controls       []socketReport
	reports        []socketReport
	barriers       []string
	id             string
	snapshot       sessionRecord
	edit           sessionRecord
	editIdentity   socketIdentity
	fixtureErr     error
	runErr         error
}

var socketOperations = []string{"stream-connect", "seqpacket-connect", "dgram-connect", "dgram-sendto", "dgram-sendmsg"}

func socketTargets(mode string) []socketTarget {
	dirs := []string{"", "nested/old"}
	if mode == "late" {
		dirs = []string{"existing", "new/nested"}
	}
	var result []socketTarget
	for d, dir := range dirs {
		for i, op := range socketOperations {
			name := fmt.Sprintf("d%d-s%d", d, i)
			result = append(result, socketTarget{Name: name, Op: op, Path: filepath.ToSlash(filepath.Join(dir, name))})
		}
	}
	return result
}

func buildSocketProbe(root, output string) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	base := filepath.Join(root, "features", "testdata", "slice-19")
	cmd := exec.CommandContext(ctx, "go", "build", "-trimpath", "-o", output, filepath.Join(base, "probe.go"), filepath.Join(base, "socket.go"))
	cmd.Dir, cmd.Env = root, sessionChildEnvironment(os.Environ())
	for k, v := range map[string]string{"CGO_ENABLED": "0", "GOOS": "linux", "GOARCH": "amd64", "GOFLAGS": "", "GOWORK": "off"} {
		cmd.Env = setEnv(cmd.Env, k, v)
	}
	b, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("compile trusted slice-19 probe: %w\n%s", err, b)
	}
	return nil
}

func (w *world) socketFixture(mode string) error {
	return w.socketFixtureTargets(mode, socketTargets(mode))
}

func (w *world) socketFixtureTargets(mode string, targets []socketTarget) error {
	if err := doctorRealGuard(); err != nil {
		return err
	}
	// sockaddr_un has a small pathname limit. Use a short disposable root, not
	// a symlink alias or permissions that could manufacture socket denial.
	if err := os.Remove(w.dir); err != nil {
		return err
	}
	var err error
	w.dir, err = os.MkdirTemp("/tmp", "slice-19-")
	if err != nil {
		return err
	}
	if err = w.offlineFixture("sockets"); err != nil {
		return err
	}
	s := &socketWorld{w: w, config: socketConfig{Nonce: isolationNonce(), Mode: mode, Root: "/workspace", Targets: targets},
		original: map[string]sessionRecord{}, publications: map[string]socketIdentity{}}
	w.sockets = s
	s.probe = filepath.Join(w.dir, "slice-19-probe")
	if err = buildSocketProbe(w.root, s.probe); err != nil {
		return err
	}
	data, err := os.ReadFile(s.probe)
	if err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(w.doctor.checkout, "slice-19-probe"), data, 0700); err != nil {
		return err
	}
	for _, target := range s.config.Targets {
		path := filepath.Join(w.doctor.checkout, target.Path)
		if len(path)+2 >= 108 {
			return errors.New("socket fixture path exceeds sockaddr_un")
		}
		if mode != "late" || strings.HasPrefix(target.Path, "existing/") {
			if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				return err
			}
		}
		if mode == "replace" {
			if err = os.WriteFile(path, []byte(s.config.Nonce+"/original/"+target.Name), 0600); err != nil {
				return err
			}
			s.original[target.Name], err = sessionReadRecord(path)
			if err != nil {
				return err
			}
		}
	}
	if mode == "existing" {
		if err = s.publish(context.Background()); err != nil {
			return err
		}
	}
	w.offline.before, err = s.protected()
	return err
}

func socketStat(path string) (socketIdentity, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return socketIdentity{}, err
	}
	stat := reflect.Indirect(reflect.ValueOf(info.Sys()))
	return socketIdentity{Inode: fmt.Sprintf("%v:%v", stat.FieldByName("Dev"), stat.FieldByName("Ino")),
		UID: fmt.Sprint(stat.FieldByName("Uid")), GID: fmt.Sprint(stat.FieldByName("Gid")), Mode: info.Mode()}, nil
}

func (s *socketWorld) publish(ctx context.Context) error {
	root := s.w.doctor.checkout
	if s.config.Mode == "late" {
		if _, err := os.Lstat(filepath.Join(root, "new")); !os.IsNotExist(err) {
			return errors.New("planned new directory already exists before host publication")
		}
	}
	for _, t := range s.config.Targets {
		path := filepath.Join(root, t.Path)
		if s.config.Mode == "replace" {
			current, err := sessionReadRecord(path)
			if err != nil || current != s.original[t.Name] {
				return fmt.Errorf("original file changed before planned replacement: %s", t.Name)
			}
		} else if _, err := os.Lstat(path); !os.IsNotExist(err) {
			return fmt.Errorf("planned socket already exists: %s", t.Name)
		}
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return err
		}
	}
	c := s.config
	c.Root = root
	if c.Mode == "replace" {
		c.Suffix = ".n"
	}
	data, err := json.Marshal(c)
	if err != nil {
		return err
	}
	serverCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	s.listenerCancel = cancel
	s.listener, err = startSocketProcess(serverCtx, s.probe, []string{"serve", string(data)}, s.w.dir, s.w.env())
	if err != nil {
		return err
	}
	readyCtx, readyCancel := context.WithTimeout(ctx, 10*time.Second)
	defer readyCancel()
	if _, err = s.listener.await(readyCtx, "ready", "", c.Nonce); err != nil {
		return fmt.Errorf("fixture listener startup: %w", err)
	}
	s.identities = map[string]socketIdentity{}
	for _, t := range c.Targets {
		path := filepath.Join(root, t.Path)
		id, err := socketStat(path + c.Suffix)
		if err != nil || id.Mode&os.ModeSocket == 0 || id.Mode.Perm() != 0600 || id.UID != s.w.approval.uid || id.GID != s.w.approval.gid {
			return fmt.Errorf("listener %s lacks a same-user writable socket identity: %v", t.Name, err)
		}
		if c.Mode == "replace" {
			if id.Inode == s.original[t.Name].Inode {
				return errors.New("replacement did not obtain a new socket inode")
			}
			if err = os.Rename(path+c.Suffix, path); err != nil {
				return err
			}
		}
		s.identities[t.Name] = id
		s.publications[path] = id
		for dir := filepath.Dir(path); dir != root; dir = filepath.Dir(dir) {
			s.publications[dir], err = socketStat(dir)
			if err != nil {
				return err
			}
		}
	}
	return s.listenersUnchanged()
}

func (s *socketWorld) listenersUnchanged() error {
	if s.listener == nil || len(s.identities) != len(s.config.Targets) {
		return errors.New("missing complete fixture listener inventory")
	}
	select {
	case <-s.listener.done:
		return errors.New("host fixture listener exited prematurely")
	default:
	}
	for path, original := range s.publications {
		current, err := socketStat(path)
		if err != nil || current != original {
			return fmt.Errorf("fixture publication inode, owner or mode changed at %s", path)
		}
	}
	return nil
}

func (s *socketWorld) hostControl(ctx context.Context, phase string) error {
	if err := s.listenersUnchanged(); err != nil {
		return err
	}
	c := s.config
	c.Root = s.w.doctor.checkout
	b, err := json.Marshal(c)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, s.probe, "control", string(b), phase)
	cmd.Dir, cmd.Env = s.w.dir, s.w.env()
	cmd.WaitDelay = time.Second
	var out, errout bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errout
	err = cmd.Run()
	if err != nil || errout.Len() != 0 {
		return fmt.Errorf("host socket control process failed: %v %s", err, errout.Bytes())
	}
	var r socketReport
	if err = json.Unmarshal(out.Bytes(), &r); err != nil {
		return err
	}
	s.controls = append(s.controls, r)
	// Receipt frames precede this checkpoint on the same listener stdout.
	if err = s.listener.release(c.Nonce, phase); err != nil {
		return err
	}
	if _, err = s.listener.await(ctx, "checkpoint", phase, c.Nonce); err != nil {
		return err
	}
	return s.listenersUnchanged()
}

// Planned publications and the single authorized edit are checked by their own
// identity/content observations. Every other path stays in the preservation map.
func (s *socketWorld) protected() (map[string]string, error) {
	tree, err := s.w.offlineProtected()
	if err != nil {
		return nil, err
	}
	root := s.w.doctor.checkout
	delete(tree, filepath.Join(root, "slice-19-edit"))
	for _, t := range s.config.Targets {
		path := filepath.Join(root, t.Path)
		delete(tree, path)
		if s.config.Mode == "late" && strings.HasPrefix(t.Path, "new/") {
			for dir := filepath.Dir(path); dir != root; dir = filepath.Dir(dir) {
				delete(tree, dir)
			}
		}
	}
	for path, value := range tree {
		id, err := socketStat(path)
		if err != nil {
			return nil, err
		}
		tree[path] = value + fmt.Sprintf(":%s:%s", id.Inode, id.GID)
	}
	return tree, nil
}

func (w *world) closeSockets() error {
	if w.sockets == nil || w.sockets.listenerCancel == nil {
		return nil
	}
	s := w.sockets
	defer s.listenerCancel()
	if s.listener == nil {
		return nil
	}
	_ = s.listener.input.Close()
	select {
	case <-s.listener.done:
		return s.listener.err
	case <-time.After(3 * time.Second):
		s.listenerCancel()
		<-s.listener.done
		return errors.New("host socket fixture did not stop within its cleanup budget")
	}
}
