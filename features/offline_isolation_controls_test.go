package features

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/moby/moby/api/types/container"
)

func (s *isolationWorld) prepare() error {
	w := s.w
	switch s.mode {
	case "identity":
		data, err := os.ReadFile(filepath.Join(w.doctor.checkout, "slice-14-probe"))
		if err != nil {
			return err
		}
		if err = os.WriteFile(filepath.Join(w.doctor.checkout, "slice-14-setuid"), data, 0755); err != nil {
			return err
		}
		c, err := s.controlCreate(w.doctor.checkout, []string{"/workspace/slice-14-probe", "prepare-setuid"}, "0:0", "none", nil, []string{"CHOWN", "FOWNER", "FSETID"}, false)
		if err != nil {
			return err
		}
		if _, err = s.controlWait(c); err != nil {
			return err
		}
		s.control, err = s.controlCreate(w.doctor.checkout, []string{"/workspace/slice-14-probe", "child", s.config.Nonce}, w.approval.uid+":"+w.approval.gid, "none", nil, nil, false)
		if err != nil {
			return err
		}
		s.setuidBefore, err = s.setuidControl()
		if err != nil {
			return err
		}
		// Only this trusted control grants mount authority in its own namespace.
		s.privilegedControl, err = s.controlCreate(w.doctor.checkout, []string{"/workspace/slice-14-probe", "child", s.config.Nonce}, "0:0", "none", nil, []string{"SYS_ADMIN", "SETUID"}, true, func(h *container.HostConfig) {
			h.SecurityOpt = append(h.SecurityOpt, "apparmor=unconfined")
		})
		if err != nil {
			return err
		}
		s.privilegedBefore, err = s.privilegeControl()
		return err
	case "filesystem":
		source := filepath.Join(w.doctor.host, "slice-14-nested-source")
		target := filepath.Join(w.doctor.checkout, "slice-14-nested")
		if err := os.Mkdir(source, 0700); err != nil {
			return err
		}
		if err := os.Mkdir(target, 0700); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(source, "marker"), []byte(s.config.Nonce), 0600); err != nil {
			return err
		}
		if err := isolationSudo("mount", "--bind", source, target); err != nil {
			return err
		}
		s.cleanup = append(s.cleanup, func() error { return isolationSudo("umount", target) })
		if err := isolationSudo("mount", "-o", "remount,bind,ro", target); err != nil {
			return err
		}
		s.config.Files["nested"] = s.config.Nonce
		s.nestedBefore = s.nestedControl()
		if !s.nestedBefore {
			return errors.New("nested read-only host mount control is unavailable")
		}
		var err error
		s.control, err = s.controlCreate(w.doctor.checkout, []string{"/workspace/slice-14-probe", "child", s.config.Nonce}, "0:0", "none", nil, nil, true, func(h *container.HostConfig) {
			h.ReadonlyRootfs = false
		})
		if err != nil {
			return err
		}
		s.rootBefore, err = s.rootWriteControl()
		return err
	case "host":
		return s.prepareHost()
	case "primary", "exit":
		if s.mode == "exit" {
			w.doctor.manifest["containerEnv"] = map[string]any{"APP": "SLICE_14_SYNTHETIC_ENV"}
			if err := w.approvalManifest(); err != nil {
				return err
			}
			if err := w.approvalSeed("1", false); err != nil {
				return err
			}
		}
		var err error
		s.control, err = s.controlCreate(w.doctor.checkout, []string{"/workspace/slice-14-probe", "child", s.config.Nonce}, w.approval.uid+":"+w.approval.gid, "none", nil, nil, true)
		if err != nil {
			return err
		}
		s.controlBeatsBefore, err = s.liveControl()
		return err
	case "network", "dns":
		return s.prepareNetwork()
	}
	return fmt.Errorf("unknown isolation fixture %q", s.mode)
}

func isolationSudo(argv ...string) error {
	if err := doctorRealGuard(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "sudo", append([]string{"-n", "--"}, argv...)...)
	// Only fixed host tools and disposable paths supplied by this fixture. No
	// checkout program, shell, Docker CLI or credential-bearing environment.
	cmd.Env = []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin", "LANG=C"}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("required native fixture operation %s failed: %w", argv[0], err)
	}
	return nil
}

func (s *isolationWorld) setuidControl() (string, error) {
	r, err := s.controlExec(s.control, s.w.approval.uid+":"+s.w.approval.gid, "/workspace/slice-14-setuid", "ids")
	if err != nil {
		return "", err
	}
	if len(r) != 1 || r[0].Kind != "ids" || r[0].Values["uid"] != "0" {
		return "", errors.New("same-file setuid control did not elevate without NNP")
	}
	return r[0].Values["uid"], nil
}

func (s *isolationWorld) privilegeControl() (bool, error) {
	r, err := s.controlExec(s.privilegedControl, "0:0", "/workspace/slice-14-probe", "privileged-control")
	if err != nil {
		return false, err
	}
	if len(r) != 1 || r[0].Kind != "privileged-control" || r[0].Values["uid"] != "0" || r[0].Values["setuid-error"] != "<nil>" || r[0].Values["mount-error"] != "<nil>" {
		return false, errors.New("root and mount positive controls did not succeed")
	}
	return true, nil
}

func (s *isolationWorld) rootWriteControl() (bool, error) {
	r, err := s.controlExec(s.control, "0:0", "/workspace/slice-14-probe", "root-write-control", isolationNonce())
	if err != nil {
		return false, err
	}
	if len(r) != 1 || r[0].Kind != "root-write-control" || r[0].Values["root-write"] != "<nil>" || r[0].Values["root-match"] != "true" || !strings.Contains(","+r[0].Values["root-options"]+",", ",rw,") {
		return false, errors.New("same-path writable image root control did not succeed")
	}
	return true, nil
}

func (w *world) isolationIdentityInputs() error {
	s := w.offline.isolation
	groups, err := os.Getgroups()
	if err != nil {
		return err
	}
	s.groups = groups
	found := false
	for _, g := range groups {
		found = found || g != os.Getegid()
	}
	if !found {
		return errors.New("required distinct host supplementary group is unavailable")
	}
	for _, k := range []string{"UID", "GID", "USER", "LOGNAME", "SUDO_UID", "SUDO_GID"} {
		w.doctor.env[k] = "31414"
	}
	return nil
}

func (s *isolationWorld) nestedControl() bool {
	target := filepath.Join(s.w.doctor.checkout, "slice-14-nested")
	b, err := os.ReadFile(filepath.Join(target, "marker"))
	if err != nil || string(b) != s.config.Files["nested"] {
		return false
	}
	b, err = os.ReadFile("/proc/self/mountinfo")
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Fields(line)
		if len(f) > 5 && f[4] == target && strings.Contains(","+f[5]+",", ",ro,") {
			return true
		}
	}
	return false
}

func (s *isolationWorld) prepareHost() error {
	w := s.w
	for name, dir := range map[string]string{"outside": w.doctor.host, "home": w.doctor.env["HOME"]} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return err
		}
		path := filepath.Join(dir, "slice-14-"+name)
		if err := os.WriteFile(path, []byte(s.config.Nonce), 0600); err != nil {
			return err
		}
		s.config.Files[name] = path
	}
	s.config.Files["approval"] = w.approval.record
	s.config.Files["ownership"] = filepath.Join(w.doctor.env["XDG_STATE_HOME"], "itos-container", "intents", strings.Repeat("e", 64), strings.Repeat("d", 64)+".json")
	link := filepath.Join(w.doctor.checkout, "slice-14-outside-link")
	if err := os.Symlink(s.config.Files["outside"], link); err != nil {
		return err
	}
	s.config.Files["symlink"] = "/workspace/slice-14-outside-link"
	for _, name := range []string{"host", "ssh"} {
		// A short /tmp path also keeps Unix sockaddr limits independent of the
		// feature scratch path. This is never a real SSH agent.
		dir, err := os.MkdirTemp("/tmp", "slice-14-socket-")
		if err != nil {
			return err
		}
		path := filepath.Join(dir, "listener.sock")
		l, err := net.Listen("unix", path)
		if err != nil {
			return err
		}
		s.cleanup = append(s.cleanup, func() error { err := l.Close(); return errors.Join(err, os.RemoveAll(dir)) })
		go func() {
			for {
				c, err := l.Accept()
				if err != nil {
					return
				}
				_ = c.Close()
			}
		}()
		s.config.Sockets[name] = path
	}
	s.config.Sockets["engine"] = "/var/run/docker.sock"
	w.doctor.env["SSH_AUTH_SOCK"] = s.config.Sockets["ssh"]
	p := exec.Command("/bin/sleep", "240")
	p.Env = []string{"SLICE_14_SENTINEL=" + s.config.Nonce}
	if err := p.Start(); err != nil {
		return err
	}
	s.cleanup = append(s.cleanup, func() error { err := p.Process.Kill(); _ = p.Wait(); return err })
	s.config.HostPID = p.Process.Pid
	s.config.HostStart = isolationProcessStart(p.Process.Pid)
	if s.config.HostStart == "" {
		return errors.New("host sentinel start identity unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := isolationAwaitHostProcess(ctx, s.config); err != nil {
		return err
	}
	var err error
	s.hostBefore, err = s.hostControls()
	return err
}

func isolationAwaitHostProcess(ctx context.Context, c isolationConfig) error {
	// Start can return while /proc still exposes an empty exec environment.
	// Establish the nonce control before probing the worker; subsequent host
	// controls remain immediate so a lost sentinel cannot pass through retries.
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("host process nonce control is unavailable: %w", err)
		}
		if isolationProcessStart(c.HostPID) != c.HostStart {
			return errors.New("host process sentinel disappeared")
		}
		matched, err := isolationProcessNonce(c.HostPID, c.Nonce)
		if err != nil {
			return fmt.Errorf("read host process nonce control: %w", err)
		}
		if matched {
			return nil
		}
		select {
		case <-ctx.Done():
		case <-ticker.C:
		}
	}
}

func isolationProcessNonce(pid int, nonce string) (bool, error) {
	env, err := os.ReadFile(fmt.Sprintf("/proc/%d/environ", pid))
	return slices.Contains(strings.Split(string(env), "\x00"), "SLICE_14_SENTINEL="+nonce), err
}

func isolationProcessStart(pid int) string {
	b, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return ""
	}
	i := strings.LastIndex(string(b), ") ")
	if i < 0 {
		return ""
	}
	f := strings.Fields(string(b[i+2:]))
	if len(f) < 20 || f[0] == "Z" || f[0] == "X" {
		return ""
	}
	return f[19]
}
func (s *isolationWorld) hostControls() (map[string]string, error) {
	r := map[string]string{}
	for name, path := range s.config.Files {
		if name == "symlink" {
			path = filepath.Join(s.w.doctor.checkout, "slice-14-outside-link")
		}
		b, err := os.ReadFile(path)
		if err != nil || len(b) == 0 {
			return nil, fmt.Errorf("host file control %s is unavailable", name)
		}
		r["file-"+name] = approvalHash(string(b))
	}
	for name, path := range s.config.Sockets {
		c, err := net.DialTimeout("unix", path, time.Second)
		if err != nil {
			return nil, fmt.Errorf("host socket control %s is unavailable", name)
		}
		_ = c.Close()
		r["socket-"+name] = "connected"
	}
	for _, ns := range []string{"pid", "ipc", "uts", "mnt", "net"} {
		info, err := os.Stat("/proc/self/ns/" + ns)
		if err != nil {
			return nil, err
		}
		v := reflect.Indirect(reflect.ValueOf(info.Sys()))
		r["ns-"+ns] = fmt.Sprintf("%v:%v", v.FieldByName("Dev"), v.FieldByName("Ino"))
	}
	if isolationProcessStart(s.config.HostPID) != s.config.HostStart {
		return nil, errors.New("host process sentinel disappeared")
	}
	matched, err := isolationProcessNonce(s.config.HostPID, s.config.Nonce)
	if err != nil || !matched {
		return nil, errors.New("host process nonce control is unavailable")
	}
	r["sentinel"] = s.config.HostStart
	return r, nil
}

func (s *isolationWorld) liveControl() (int, error) {
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		v, err := s.controlInspect(s.control)
		if err != nil {
			return 0, err
		}
		if v.State == nil || !v.State.Running {
			return 0, errors.New("unrelated control is not running")
		}
		r, err := s.controlLogs(s.control)
		if err == nil {
			last := r[len(r)-1]
			n, _ := strconv.Atoi(last.Values["beat"])
			if last.Kind == "heartbeat" && last.Nonce == s.config.Nonce && n > s.controlBeatsBefore {
				return n, nil
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	return 0, errors.New("unrelated control did not produce a fresh heartbeat")
}
