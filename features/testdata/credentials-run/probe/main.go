// A cooperative, hash-only worker used by the native credential run scenarios.
package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

type plan struct {
	Nonce, Target string
	Forbidden     []string
	Child         json.RawMessage
}

type access struct {
	Kind, Hash, Mode, Errno string
	UID                     uint32
}

type report struct {
	Nonce, Phase                  string
	UID, GID                      int
	Target                        access
	Forbidden                     map[string]access
	Writes                        map[string]string
	CredentialEntries, ShmEntries []string
	CredentialListing, ShmListing string
	TokenEnv, MarkerInArgsOrEnv   bool
	Child                         json.RawMessage
	ChildError                    string
}

func main() {
	if len(os.Args) != 2 {
		os.Exit(2)
	}
	data, err := os.ReadFile(os.Args[1])
	var p plan
	if err != nil || json.Unmarshal(data, &p) != nil || p.Nonce == "" || p.Target != "/run/itos-container/credentials/claude-token" {
		os.Exit(2)
	}
	input := bufio.NewScanner(os.Stdin)
	if len(p.Child) != 0 && (!input.Scan() || input.Text() != "network-ready") {
		os.Exit(2)
	}
	child, childErr := runChild(p.Child)
	out := json.NewEncoder(os.Stdout)
	if out.Encode(observe(p, "first", child, childErr)) != nil {
		os.Exit(2)
	}
	for input.Scan() {
		if input.Text() == "read" {
			if out.Encode(observe(p, "read", child, childErr)) != nil {
				os.Exit(2)
			}
			continue
		}
		if code, ok := strings.CutPrefix(input.Text(), "exit "); ok {
			n, err := strconv.Atoi(code)
			if err == nil && n >= 0 && n <= 255 && out.Encode(observe(p, "final", child, childErr)) == nil {
				os.Exit(n)
			}
		}
		os.Exit(2)
	}
	os.Exit(2)
}

func observe(p plan, phase string, child json.RawMessage, childErr string) report {
	target, data := read(p.Target)
	r := report{Nonce: p.Nonce, Phase: phase, UID: os.Geteuid(), GID: os.Getegid(), Target: target, Forbidden: map[string]access{},
		Writes: map[string]string{}, Child: child, ChildError: childErr}
	_, r.TokenEnv = os.LookupEnv("CLAUDE_CODE_OAUTH_TOKEN")
	marker := bytes.TrimRight(data, "\n")
	if len(marker) != 0 {
		for _, value := range append(os.Environ(), os.Args...) {
			r.MarkerInArgsOrEnv = r.MarkerInArgsOrEnv || bytes.Contains([]byte(value), marker)
		}
	}
	for _, path := range p.Forbidden {
		result, _ := read(path)
		r.Forbidden[path] = result
	}
	dir := filepath.Dir(p.Target)
	r.CredentialEntries, r.CredentialListing = names(dir)
	r.ShmEntries, r.ShmListing = names("/dev/shm")
	r.Writes["open"] = openWrite(p.Target, os.O_WRONLY|os.O_APPEND)
	r.Writes["create"] = openWrite(filepath.Join(dir, "probe-write-"+p.Nonce), os.O_WRONLY|os.O_CREATE|os.O_EXCL)
	r.Writes["unlink"] = errno(os.Remove(p.Target))
	r.Writes["rename"] = errno(os.Rename(p.Target, filepath.Join(dir, "probe-rename-"+p.Nonce)))
	return r
}

func read(path string) (access, []byte) {
	info, err := os.Lstat(path)
	if errors.Is(err, syscall.ENOENT) || errors.Is(err, syscall.ENOTDIR) {
		return access{Kind: "absent", Errno: errno(err)}, nil
	}
	if err != nil {
		// A parent directory can deny traversal before an open is possible.
		return readError(err), nil
	}
	f, err := os.Open(path)
	if err != nil {
		return readError(err), nil
	}
	_ = f.Close()
	if !info.Mode().IsRegular() {
		return access{Kind: "error", Errno: "not-regular"}, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return readError(err), nil
	}
	return access{Kind: "file", Hash: fmt.Sprintf("%x", sha256.Sum256(data)), Mode: fmt.Sprintf("%04o", info.Mode().Perm()), UID: info.Sys().(*syscall.Stat_t).Uid}, data
}

func readError(err error) access {
	kind := "error"
	if errors.Is(err, syscall.EACCES) || errors.Is(err, syscall.EPERM) {
		kind = "denied"
	}
	return access{Kind: kind, Errno: errno(err)}
}

func names(path string) ([]string, string) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, errno(err)
	}
	result := make([]string, 0, len(entries))
	for _, entry := range entries {
		result = append(result, entry.Name())
	}
	return result, ""
}

func openWrite(path string, flags int) string {
	f, err := os.OpenFile(path, flags, 0600)
	if err != nil {
		return errno(err)
	}
	_ = f.Close()
	return "succeeded"
}

func errno(err error) string {
	if err == nil {
		return "succeeded"
	}
	for value, name := range map[syscall.Errno]string{
		syscall.EACCES: "EACCES", syscall.EPERM: "EPERM", syscall.ENOENT: "ENOENT",
		syscall.ENOTDIR: "ENOTDIR", syscall.EROFS: "EROFS", syscall.EBUSY: "EBUSY",
		syscall.EINVAL: "EINVAL", syscall.EEXIST: "EEXIST", syscall.EXDEV: "EXDEV",
	} {
		if errors.Is(err, value) {
			return name
		}
	}
	var number syscall.Errno
	if errors.As(err, &number) {
		return fmt.Sprintf("errno-%d", number)
	}
	return "unknown"
}

func runChild(plan json.RawMessage) (json.RawMessage, string) {
	if len(plan) == 0 {
		return nil, ""
	}
	dir, err := os.MkdirTemp("/tmp", "credrun-egress-")
	if err != nil {
		return nil, errno(err)
	}
	ca, err := os.ReadFile("/workspace/slice-27-ca.pem")
	if err != nil {
		return nil, errno(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "slice-27-ca.pem"), ca, 0600); err != nil {
		return nil, errno(err)
	}
	path := filepath.Join(dir, "slice-27-plan.json")
	if err := os.WriteFile(path, plan, 0600); err != nil {
		return nil, errno(err)
	}
	cmd := exec.Command("/workspace/slice-27-probe", path)
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		return nil, errno(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "slice-27-report.json"))
	if err != nil {
		return nil, errno(err)
	}
	if !json.Valid(data) {
		return nil, "invalid-report"
	}
	// Return only result fields. The child report's environment is deliberately
	// excluded so this cooperative probe never reports arbitrary environment values.
	var summary struct {
		Done    bool
		Results []json.RawMessage
	}
	if json.Unmarshal(data, &summary) != nil {
		return nil, "invalid-report"
	}
	result, err := json.Marshal(summary)
	if err != nil {
		return nil, "invalid-report"
	}
	return result, ""
}
