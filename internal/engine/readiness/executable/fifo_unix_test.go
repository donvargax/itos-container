//go:build unix

package executable

import (
	"net"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/donvargax/itos-container/internal/engine/readiness/port"
)

// A pipe where the program was must neither hold the observation nor be read.
func TestProgramDoesNotWaitOnAPipe(t *testing.T) {
	path := filepath.Join(t.TempDir(), "itos-container")
	if err := syscall.Mkfifo(path, 0600); err != nil {
		t.Skipf("no named pipes here: %v", err)
	}
	type outcome struct {
		observed port.Executable
		failure  port.Failure
	}
	done := make(chan outcome, 1)
	go func() {
		observed, failure := at(path).Observe()
		done <- outcome{observed, failure}
	}()
	select {
	case got := <-done:
		if want := (port.Executable{Path: real(t, path)}); got.failure != port.None || got.observed != want {
			t.Errorf("observed %+v (failure %v), want %+v", got.observed, got.failure, want)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the observation waited for a writer")
	}
}

// A socket has a name that resolves and cannot be opened, whoever asks.
func TestProgramFailsOnANameItCannotOpen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s")
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Skipf("no Unix sockets here: %v", err)
	}
	defer func() { _ = listener.Close() }()
	if got, failure := at(path).Observe(); failure != port.EgressExecutable || got != (port.Executable{}) {
		t.Errorf("observed %+v with failure %v", got, failure)
	}
}
