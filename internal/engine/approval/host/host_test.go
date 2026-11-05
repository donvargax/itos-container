package host

import (
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
)

func TestEffectiveIdentityAndSocket(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux host identity target")
	}
	host := Process{}
	uid, gid := host.Identity()
	if uid != strconv.Itoa(os.Geteuid()) || gid != strconv.Itoa(os.Getegid()) {
		t.Fatal(uid, gid)
	}
	for _, endpoint := range []string{"", "tcp://host", "unix:///no/such/fixture"} {
		if host.Socket(endpoint) != "" {
			t.Fatal(endpoint)
		}
	}
	dir, err := os.MkdirTemp("/tmp", "slice-4-host-")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(dir) }()
	path := filepath.Join(dir, "socket")
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()
	token := host.Socket("unix://" + path)
	if token == "" || host.Socket("unix://"+path) != token {
		t.Fatal("unstable socket token")
	}
	alias := path + "-link"
	if err := os.Symlink(path, alias); err != nil {
		t.Fatal(err)
	}
	if host.Socket("unix://"+alias) != "" {
		t.Fatal("followed socket alias")
	}
}

func TestIdentityRequiresBothEffectiveIDs(t *testing.T) {
	for _, tc := range []struct {
		name             string
		uid, gid         int
		wantUID, wantGID string
	}{
		{"unavailable UID", -1, 3000, "", ""},
		{"unavailable GID", 2000, -1, "", ""},
		{"both unavailable", -1, -1, "", ""},
		{"effective IDs", 2000, 3000, "2000", "3000"},
		{"root observed for domain refusal", 0, 0, "0", "0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := Process{ops: &operations{
				geteuid: func() int { return tc.uid },
				getegid: func() int { return tc.gid },
			}}
			uid, gid := p.Identity()
			if uid != tc.wantUID || gid != tc.wantGID {
				t.Fatalf("identity = %q:%q; want %q:%q", uid, gid, tc.wantUID, tc.wantGID)
			}
		})
	}
}

type socketInfo struct {
	os.FileInfo
	system any
}

func (socketInfo) Mode() os.FileMode { return os.ModeSocket | 0600 }
func (i socketInfo) Sys() any        { return i.system }

func TestSocketRequiresDeviceAndInode(t *testing.T) {
	for _, tc := range []struct {
		name   string
		system any
		want   string
	}{
		{"absent stat", nil, ""},
		{"non-struct stat", 1, ""},
		{"missing fields", struct{}{}, ""},
		{"missing device", struct{ Ino uint64 }{2}, ""},
		{"missing inode", struct{ Dev uint64 }{1}, ""},
		{"stat value", struct{ Dev, Ino uint64 }{1, 2}, "1:2"},
		{"stat pointer", &struct{ Dev, Ino uint64 }{3, 4}, "3:4"},
		{"large identities", &struct{ Dev, Ino uint64 }{1 << 63, 1<<64 - 1}, "9223372036854775808:18446744073709551615"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := Process{ops: &operations{lstat: func(string) (os.FileInfo, error) {
				return socketInfo{system: tc.system}, nil
			}}}
			if got := p.Socket("unix:///fixture.sock"); got != tc.want {
				t.Fatalf("socket identity = %q; want %q", got, tc.want)
			}
		})
	}
}
