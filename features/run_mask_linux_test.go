package features

import (
	"flag"
	"golang.org/x/sys/unix"
	"os"
	"runtime"
	"strconv"
	"testing"
)

func TestRunMaskHelper(t *testing.T) {
	args := flag.Args()
	if len(args) == 0 || args[0] != "slice-8-mask" {
		return
	}
	if len(args) < 3 {
		t.Fatal("mask helper arguments")
	}
	mask, err := strconv.ParseUint(args[1], 8, 9)
	if err != nil {
		t.Fatal(err)
	}
	runtime.LockOSThread()
	unix.Umask(int(mask))
	if err := unix.Exec(args[2], args[2:], os.Environ()); err != nil {
		t.Fatal(err)
	}
}
