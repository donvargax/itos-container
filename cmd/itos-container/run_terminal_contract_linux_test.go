package main

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/creack/pty"
	terminalnative "github.com/donvargax/itos-container/internal/engine/terminal/native"
	"golang.org/x/sys/unix"
	"golang.org/x/term"
)

func init() { newTerminalContractNative = makeTerminalContractNative }

func makeTerminalContractNative(t *testing.T, fault string) *terminalContractNative {
	t.Helper()
	open := func(columns, rows uint16, eof uint8) (*os.File, *os.File, *term.State) {
		t.Helper()
		master, slave, err := pty.Open()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = master.Close(); _ = slave.Close() })
		if err := pty.Setsize(slave, &pty.Winsize{Cols: columns, Rows: rows}); err != nil {
			t.Fatal(err)
		}
		state, err := unix.IoctlGetTermios(int(slave.Fd()), unix.TCGETS)
		if err != nil {
			t.Fatal(err)
		}
		state.Cc[unix.VEOF], state.Oflag = eof, unix.OPOST|unix.ONLCR
		if err := unix.IoctlSetTermios(int(slave.Fd()), unix.TCSETS, state); err != nil {
			t.Fatal(err)
		}
		saved, err := term.GetState(int(slave.Fd()))
		if err != nil {
			t.Fatal(err)
		}
		return master, slave, saved
	}
	_, in, inBefore := open(113, 37, 7)
	outMaster, out, outBefore := open(0, 0, 11)
	provider := terminalnative.New(in, "linux")
	n := &terminalContractNative{Provider: provider, Output: out}
	set := func(name string, value any) {
		field := reflect.ValueOf(provider).Elem().FieldByName(name)
		if !field.IsValid() || !field.CanSet() || !reflect.TypeOf(value).AssignableTo(field.Type()) {
			n.BindingError = errors.Join(n.BindingError, fmt.Errorf("native terminal test seam %s unavailable", name))
			return
		}
		field.Set(reflect.ValueOf(value))
	}
	set("Output", out)
	sawRaw := false
	observe := func() {
		state, err := unix.IoctlGetTermios(int(in.Fd()), unix.TCGETS)
		sawRaw = err == nil && state.Lflag&(unix.ICANON|unix.ECHO|unix.ISIG) == 0
	}
	read := func(fd int) (*term.State, error) {
		observe()
		if strings.HasPrefix(fault, "reading distinct") {
			return nil, errors.New("fixture output state read failed")
		}
		return term.GetState(fd)
	}
	raw := func(fd int) (*term.State, error) {
		observe()
		if strings.HasPrefix(fault, "acquiring distinct") {
			return nil, errors.New("fixture output raw acquisition failed")
		}
		return term.MakeRaw(fd)
	}
	set("ReadOutputState", read)
	set("MakeOutputRaw", raw)
	n.AcquiredInput = func() bool { return sawRaw }
	n.Clear = func() { fault = "" }
	n.Restored = func() (bool, bool) {
		a, errA := term.GetState(int(in.Fd()))
		b, errB := term.GetState(int(out.Fd()))
		_, inErr := in.Stat()
		_, outErr := out.Stat()
		return errA == nil && inErr == nil && reflect.DeepEqual(a, inBefore), errB == nil && outErr == nil && reflect.DeepEqual(b, outBefore)
	}
	n.ReadOutput = func() (string, error) {
		var data []byte
		for {
			fds := []unix.PollFd{{Fd: int32(outMaster.Fd()), Events: unix.POLLIN}}
			_, err := unix.Poll(fds, 0)
			if errors.Is(err, unix.EINTR) {
				continue
			}
			if err != nil {
				return "", err
			}
			if fds[0].Revents&unix.POLLIN == 0 {
				return string(data), nil
			}
			var chunk [4096]byte
			n, err := outMaster.Read(chunk[:])
			if err != nil {
				return "", err
			}
			data = append(data, chunk[:n]...)
		}
	}
	return n
}
