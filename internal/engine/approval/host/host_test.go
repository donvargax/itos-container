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
