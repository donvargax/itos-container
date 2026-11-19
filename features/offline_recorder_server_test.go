package features

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

func (w *world) startOfflineAPI() error {
	dir, err := os.MkdirTemp(w.fixtureSocketRoot(), "slice-10-api-")
	if err != nil {
		return err
	}
	w.doctor.cleanup = append(w.doctor.cleanup, func() { _ = os.RemoveAll(dir) })
	path := filepath.Join(dir, "engine.sock")
	listener, err := net.Listen("unix", path)
	if err != nil {
		return err
	}
	a := &offlineAPI{w: w, absent: map[string]bool{}, connections: map[net.Conn]bool{}, identities: map[string]map[string]any{}}
	a.serveDone = make(chan error, 1)
	a.stopped = make(chan struct{})
	a.server = &http.Server{ConnContext: func(ctx context.Context, c net.Conn) context.Context {
		return context.WithValue(ctx, runPeerKey{}, runPeerPID(c))
	}, Handler: http.HandlerFunc(func(out http.ResponseWriter, request *http.Request) {
		a.mu.Lock()
		if a.stopping {
			a.mu.Unlock()
			return
		}
		a.handlers.Add(1)
		a.mu.Unlock()
		defer a.handlers.Done()
		a.serve(out, request)
	}), ReadHeaderTimeout: time.Second, ConnState: func(c net.Conn, state http.ConnState) {
		a.mu.Lock()
		defer a.mu.Unlock()
		switch state {
		case http.StateHijacked:
			a.connections[c] = true
		case http.StateClosed:
			delete(a.connections, c)
		}
	}}
	w.offline.api = a
	w.doctor.env["DOCKER_HOST"] = "unix://" + path
	go func() { a.serveDone <- a.server.Serve(listener) }()
	return nil
}

// Stop admission before waiting; no handler can register after the join begins.
// Forwarded creates retain their existing independent ten-second allowance.
func (a *offlineAPI) join() error {
	a.mu.Lock()
	a.stopping, a.closed = true, true
	close(a.stopped)
	a.mu.Unlock()
	err := a.server.Close()
	a.closeConnections()
	deadline := time.NewTimer(engineCreateOutcomeAllowance)
	defer deadline.Stop()
	select {
	case serveErr := <-a.serveDone:
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			err = errors.Join(err, serveErr)
		}
	case <-deadline.C:
		return errors.Join(err, errors.New("recorder listener did not finish"))
	}
	a.awaitCreates()
	done := make(chan struct{})
	go func() { a.handlers.Wait(); close(done) }()
	select {
	case <-done:
	case <-deadline.C:
		err = errors.Join(err, errors.New("recorder handlers did not finish"))
	}
	return err
}
