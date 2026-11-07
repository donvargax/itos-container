// Trusted CI probe. Built from these reviewed files, never from the disposable
// consumer checkout. It executes only inside the subject or a fixture control.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type config struct {
	Nonce, Previous, Mode string
	HostNonce             string
	Network, DNS          *config
	Status                int
	Files, Sockets        map[string]string
	HostPID               int
	HostStart             string
	Routes                []route
	Addresses, Resolvers  []string
	CA, Cert, Key         string
	Port                  int
}

type report struct {
	Kind, Nonce, Phase string
	Values             map[string]string
	Attempts           []attempt
	Error              string
}

var reportOutput io.Writer = os.Stdout

func emit(r report) { _ = json.NewEncoder(reportOutput).Encode(r) }
func must(err error) {
	if err != nil {
		emit(report{Kind: "error", Error: err.Error()})
		os.Exit(98)
	}
}
func text(path string) string {
	b, e := os.ReadFile(path)
	if e != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}
func status() map[string]string {
	r := map[string]string{}
	for _, line := range strings.Split(text("/proc/self/status"), "\n") {
		k, v, ok := strings.Cut(line, ":")
		if ok {
			r[k] = strings.TrimSpace(v)
		}
	}
	return r
}
func start(pid int) string {
	s := text(fmt.Sprintf("/proc/%d/stat", pid))
	i := strings.LastIndex(s, ") ")
	if i < 0 {
		return ""
	}
	f := strings.Fields(s[i+2:])
	if len(f) < 20 {
		return ""
	}
	return f[19]
}
func ids() map[string]string {
	r := status()
	r["uid"], r["gid"] = strconv.Itoa(os.Geteuid()), strconv.Itoa(os.Getegid())
	r["pid"], r["start"] = strconv.Itoa(os.Getpid()), start(os.Getpid())
	return r
}
func main() {
	if len(os.Args) < 2 {
		os.Exit(99)
	}
	if os.Args[1] == "ids" {
		emit(report{Kind: "ids", Values: ids()})
		return
	}
	if os.Args[1] == "prepare-setuid" {
		must(os.Chown("/workspace/slice-14-setuid", 0, 0))
		must(os.Chmod("/workspace/slice-14-setuid", 0755|os.ModeSetuid))
		emit(report{Kind: "prepared"})
		return
	}
	if os.Args[1] == "child" {
		child(os.Args[2])
		return
	}
	if os.Args[1] == "resolver-plan" {
		emit(report{Kind: "resolvers", Values: map[string]string{"resolv.conf": text("/etc/resolv.conf")}})
		return
	}
	if os.Args[1] == "privileged-control" {
		r := ids()
		r["setuid-error"] = fmt.Sprint(syscall.Setuid(0))
		r["mount-error"] = fmt.Sprint(mountAttempt())
		emit(report{Kind: "privileged-control", Values: r})
		return
	}
	if os.Args[1] == "root-write-control" {
		nonce := os.Args[2]
		err := rootWrite(nonce)
		_, matched := fileAttempt("/slice-14-root-marker", nonce)
		options, _ := mountInfo("/")
		emit(report{Kind: "root-write-control", Values: map[string]string{"root-write": fmt.Sprint(err), "root-match": strconv.FormatBool(matched), "root-options": options}})
		return
	}
	var c config
	f, e := os.Open(os.Args[2])
	must(e)
	must(json.NewDecoder(f).Decode(&c))
	must(f.Close())
	switch os.Args[1] {
	case "loss":
		loss(os.Args[2])
	case "identity":
		identity(c)
	case "filesystem":
		filesystem(c)
	case "host":
		host(c)
	case "primary":
		primary(c)
	case "exit":
		must(os.WriteFile("/workspace/slice-14-edit-"+c.Nonce, []byte(c.Nonce), 0600))
		emit(report{Kind: "exit", Nonce: c.Nonce, Values: map[string]string{"status": strconv.Itoa(c.Status)}})
		os.Exit(c.Status)
	case "network", "dns":
		// The route attempts are the first application operations, before a
		// readiness frame, local listener or input handshake is established.
		r := attempts(c, "first")
		emit(r)
		if _, e := bufio.NewReader(os.Stdin).ReadString('\n'); e != nil {
			os.Exit(97)
		}
		emit(attempts(c, "running"))
		loopback(c)
	case "control":
		if c.Mode == "loss" {
			must(controlAddressReady("fd14:14::2"))
			// Docker can regenerate the control's resolver file on reconnect.
			// Restore only this isolated control's independently captured plan.
			controlResolvers(c)
			if os.Args[5] == "network" {
				c = *c.Network
			} else {
				c = *c.DNS
			}
		}
		c.Nonce = os.Args[3]
		emit(attempts(c, os.Args[4]))
	case "serve":
		serve(c)
	default:
		os.Exit(96)
	}
}

func identity(c config) {
	r := ids()
	child, e := exec.Command("/workspace/slice-14-probe", "ids").Output()
	r["child-error"] = fmt.Sprint(e)
	var cr report
	if json.Unmarshal(child, &cr) == nil {
		for k, v := range cr.Values {
			r["child-"+k] = v
		}
	}
	r["setuid-error"] = fmt.Sprint(syscall.Setuid(0))
	r["after-setuid"] = strconv.Itoa(os.Geteuid())
	child, e = exec.Command("/workspace/slice-14-setuid", "ids").Output()
	r["helper-error"] = fmt.Sprint(e)
	if json.Unmarshal(child, &cr) == nil {
		r["helper-uid"] = cr.Values["uid"]
	}
	r["mount-error"] = fmt.Sprint(mountAttempt())
	r["pids.max"] = text("/sys/fs/cgroup/pids.max")
	if r["pids.max"] == "" { // cgroup v1 hosts expose the controller separately.
		r["pids.max"] = text("/sys/fs/cgroup/pids/pids.max")
	}
	emit(report{Kind: "identity", Nonce: c.Nonce, Values: r})
}

func mountAttempt() error {
	path, err := os.MkdirTemp("/tmp", "slice-14-mount-")
	must(err)
	defer os.Remove(path)
	err = syscall.Mount("none", path, "tmpfs", 0, "")
	if err == nil {
		must(syscall.Unmount(path, 0))
	}
	return err
}

func rootWrite(nonce string) error {
	return os.WriteFile("/slice-14-root-marker", []byte(nonce), 0600)
}

func mountInfo(path string) (string, string) {
	for _, line := range strings.Split(text("/proc/self/mountinfo"), "\n") {
		left, right, ok := strings.Cut(line, " - ")
		f := strings.Fields(left)
		g := strings.Fields(right)
		if ok && len(f) > 5 && len(g) > 0 && f[4] == path {
			return f[5], g[0]
		}
	}
	return "", ""
}
func fileAttempt(path, expected string) (bool, bool) {
	b, e := os.ReadFile(path)
	return e == nil, e == nil && string(b) == expected
}
func filesystem(c config) {
	r := map[string]string{}
	r["root-options"], r["root-type"] = mountInfo("/")
	r["tmp-options"], r["tmp-type"] = mountInfo("/tmp")
	i, e := os.Stat("/tmp")
	must(e)
	r["tmp-mode"] = fmt.Sprintf("%o", i.Sys().(*syscall.Stat_t).Mode&07777)
	r["root-write"] = fmt.Sprint(rootWrite(c.Nonce))
	r["HOME"], r["TMPDIR"] = os.Getenv("HOME"), os.Getenv("TMPDIR")
	for key, path := range map[string]string{"checkout": "/workspace/slice-14-edit-", "home": filepath.Join(os.Getenv("HOME"), "slice-14-home-"), "temp": filepath.Join(os.Getenv("TMPDIR"), "slice-14-temp-")} {
		if c.Previous != "" {
			readable, matched := fileAttempt(path+c.Previous, c.Previous)
			r["previous-"+key+"-read"] = strconv.FormatBool(readable)
			r["previous-"+key+"-match"] = strconv.FormatBool(matched)
		}
		e := os.WriteFile(path+c.Nonce, []byte(c.Nonce), 0600)
		r[key+"-write"] = fmt.Sprint(e)
		if e == nil {
			i, e := os.Stat(path + c.Nonce)
			must(e)
			s := i.Sys().(*syscall.Stat_t)
			r[key+"-owner"] = fmt.Sprintf("%d:%d", s.Uid, s.Gid)
		}
	}
	r["nested-attempt"] = "true"
	readable, matched := fileAttempt("/workspace/slice-14-nested/marker", c.Files["nested"])
	r["nested-read"], r["nested-match"] = strconv.FormatBool(readable), strconv.FormatBool(matched)
	emit(report{Kind: "filesystem", Nonce: c.Nonce, Values: r})
}

func unixAttempt(path string) bool {
	c, e := net.DialTimeout("unix", path, 300*time.Millisecond)
	if e != nil {
		return false
	}
	_ = c.Close()
	return true
}
func host(c config) {
	r := map[string]string{}
	must(os.WriteFile("/tmp/slice-14-local", []byte(c.Nonce), 0600))
	a, b := fileAttempt("/tmp/slice-14-local", c.Nonce)
	r["local-file"] = strconv.FormatBool(a && b)
	l, e := net.Listen("unix", "/tmp/slice-14-local.sock")
	must(e)
	defer l.Close()
	r["local-socket"] = strconv.FormatBool(unixAttempt(l.Addr().String()))
	for name, path := range c.Files {
		a, _ := fileAttempt(path, c.Nonce)
		r["file-attempt-"+name] = "true"
		r["file-read-"+name] = strconv.FormatBool(a)
	}
	for name, path := range c.Sockets {
		r["socket-attempt-"+name] = "true"
		r["socket-connect-"+name] = strconv.FormatBool(unixAttempt(path))
	}
	for _, ns := range []string{"pid", "ipc", "uts", "mnt", "net"} {
		i, e := os.Stat("/proc/self/ns/" + ns)
		must(e)
		s := i.Sys().(*syscall.Stat_t)
		r["ns-"+ns] = fmt.Sprintf("%d:%d", s.Dev, s.Ino)
	}
	r["sentinel-attempt"] = "true"
	env := text(fmt.Sprintf("/proc/%d/environ", c.HostPID))
	nonce := c.HostNonce
	if nonce == "" {
		nonce = c.Nonce
	}
	r["sentinel-match"] = strconv.FormatBool(start(c.HostPID) == c.HostStart && strings.Contains(env, "SLICE_14_SENTINEL="+nonce))
	emit(report{Kind: "host", Nonce: c.Nonce, Values: r})
}

func child(nonce string) {
	for n := 1; ; n++ {
		r := ids()
		r["beat"] = strconv.Itoa(n)
		emit(report{Kind: "heartbeat", Nonce: nonce, Values: r})
		time.Sleep(40 * time.Millisecond)
	}
}
func primary(c config) {
	must(os.WriteFile("/workspace/slice-14-edit-"+c.Nonce, []byte(c.Nonce), 0600))
	p := exec.Command("/workspace/slice-14-probe", "child", c.Nonce)
	pipe, e := p.StdoutPipe()
	must(e)
	p.Stderr = os.Stderr
	must(p.Start())
	d := json.NewDecoder(pipe)
	var r report
	for n := 0; n < 2; n++ {
		must(d.Decode(&r))
	}
	r.Kind = "descendant"
	emit(r)
	// The harness independently binds this child's host PID/start time to the
	// inspected full container ID before releasing the primary.
	_, e = bufio.NewReader(os.Stdin).ReadString('\n')
	must(e)
	emit(report{Kind: "trailing", Nonce: c.Nonce, Values: map[string]string{"status": strconv.Itoa(c.Status)}})
	os.Exit(c.Status)
}
