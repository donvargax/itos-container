// This fixture runs only as literal argv inside a CI worker.
package main

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const prefix = "/workspace/slice-38-"

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "exec fixture:", err)
		os.Exit(98)
	}
}

func read(name string) string {
	b, err := os.ReadFile(prefix + name)
	must(err)
	return string(b)
}

func publish(name, value string) {
	must(os.WriteFile(prefix+name+".next", []byte(value), 0600))
	must(os.Rename(prefix+name+".next", prefix+name))
}

func released(name string) bool {
	_, err := os.Stat(prefix + name)
	if errors.Is(err, os.ErrNotExist) {
		return false
	}
	must(err)
	return true
}

func report(nonce string) []byte {
	cwd, err := os.Getwd()
	must(err)
	mnt, err := os.Readlink("/proc/self/ns/mnt")
	must(err)
	netns, err := os.Readlink("/proc/self/ns/net")
	must(err)
	var root syscall.Statfs_t
	must(syscall.Statfs("/", &root))
	var workspace syscall.Stat_t
	must(syscall.Stat("/workspace", &workspace))
	status, err := os.ReadFile("/proc/self/status")
	must(err)
	fields := map[string]string{}
	for _, line := range strings.Split(string(status), "\n") {
		key, value, ok := strings.Cut(line, ":")
		if ok && (strings.HasPrefix(key, "Cap") || key == "NoNewPrivs") {
			fields[key] = strings.TrimSpace(value)
		}
	}
	// Make the write attempt independently of the mount flags. A non-root
	// user can also get EACCES, so the host checks the read-only flag too.
	f, err := os.CreateTemp("/", "slice-38-write-")
	denied := errors.Is(err, syscall.EROFS) || errors.Is(err, syscall.EACCES)
	if err == nil {
		must(f.Close())
		must(os.Remove(f.Name()))
	}
	env := os.Environ()
	slices.Sort(env)
	body, err := json.Marshal(map[string]any{
		"nonce": nonce, "argv": os.Args, "uid": os.Geteuid(), "gid": os.Getegid(),
		"cwd": cwd, "app": os.Getenv("APP"), "home": os.Getenv("HOME"), "tmp": os.Getenv("TMPDIR"),
		"envHash": fmt.Sprintf("%x", sha256.Sum256([]byte(strings.Join(env, "\x00")))),
		"mountNS": mnt, "netNS": netns, "workspaceInode": workspace.Ino, "workspaceDevice": workspace.Dev,
		"readOnly": root.Flags&1 != 0, "denied": denied, "status": fields,
	})
	must(err)
	return body
}

func main() {
	if len(os.Args) < 2 {
		os.Exit(96)
	}
	nonce := read("nonce")
	signal.Ignore(syscall.SIGINT, syscall.SIGTERM, syscall.SIGPIPE)
	switch os.Args[1] {
	case "primary":
		publish("edit", nonce)
		publish("primary", string(report(nonce)))
		fmt.Println("primary", nonce)
		for i := 1; !released("end-primary"); i++ {
			publish("primary-tick", strconv.Itoa(i))
			time.Sleep(50 * time.Millisecond)
		}
		os.Exit(17)
	case "once":
		if len(os.Args) != 3 {
			os.Exit(96)
		}
		publish("once", os.Args[2])
		fmt.Println("once", nonce, os.Args[2])
		os.Exit(23)
	case "tick":
		name := os.Args[2]
		for i := 1; ; i++ {
			publish(name+"-child", strconv.Itoa(i))
			time.Sleep(50 * time.Millisecond)
		}
	case "watch":
		name, mode := os.Args[2], os.Args[3]
		child := exec.Command(os.Args[0], "tick", name)
		must(child.Start())
		for !released(name + "-child") {
			time.Sleep(10 * time.Millisecond)
		}
		fmt.Fprintln(os.Stdout, "ready", nonce, name)
		fmt.Fprintln(os.Stderr, "live", nonce, name)
		for !released(name + "-write") {
			time.Sleep(10 * time.Millisecond)
		}
		block := strings.Repeat("x", 32768)
		for i := 1; ; i++ {
			publish(name+"-attempt", strconv.Itoa(i))
			switch mode {
			case "stderr":
				fmt.Fprintln(os.Stderr, "pulse")
			case "blocked":
				fmt.Fprint(os.Stdout, block)
			default:
				fmt.Fprintln(os.Stdout, "pulse")
			}
			time.Sleep(20 * time.Millisecond)
		}
	default:
		// The reporting row supplies the whole argv literally, including an
		// empty argv[1]. All control data lives outside the argument array.
		fmt.Fprintln(os.Stdout, string(report(nonce)))
		fmt.Fprintln(os.Stderr, "live", nonce, "report")
		input, err := io.ReadAll(os.Stdin)
		must(err)
		publish("eof", string(input))
		for !released("finish-report") {
			time.Sleep(10 * time.Millisecond)
		}
		fmt.Fprintln(os.Stdout, "stdout tail", nonce)
		fmt.Fprintln(os.Stderr, "stderr tail", nonce)
		exit, err := strconv.Atoi(read("exit"))
		must(err)
		os.Exit(exit)
	}
}
