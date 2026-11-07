package docker

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	budget "github.com/donvargax/itos-container/internal/engine/budget/port"
	"github.com/donvargax/itos-container/internal/engine/readiness/port"
	"github.com/donvargax/itos-container/internal/engine/readiness/port/porttest"
)

func TestContextEngineContract(t *testing.T) {
	porttest.ContextEngineContract(t, func(data *porttest.Session) (port.ContextEngine, string, string) {
		return Engine{}, socketServer(t, fixtureHandler(t, data)), t.TempDir()
	})
	porttest.ContextEngineContract(t, func(data *porttest.Session) (port.ContextEngine, string, string) {
		return &porttest.Engine{Session: data}, data.Address, "/checkout"
	})
}
func TestContextRequestCancelsActualSocket(t *testing.T) {
	data := porttest.HealthySession()
	base := fixtureHandler(t, data)
	arrived, ended := make(chan struct{}), make(chan struct{})
	endpoint := socketServer(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/info") {
			close(arrived)
			<-r.Context().Done()
			close(ended)
			return
		}
		base(w, r)
	})
	phase, end := context.WithTimeout(context.Background(), 2*time.Second)
	defer end()
	call, cancel := context.WithTimeout(phase, time.Second)
	s, f := (Engine{}).OpenContext(budget.WithLifetime(call, phase), endpoint, t.TempDir())
	cancel()
	if f != port.None {
		t.Fatal(f)
	}
	defer s.Close()
	request, stop := context.WithCancel(phase)
	defer stop()
	done := make(chan port.Failure, 1)
	go func() { _, f := s.(port.ContextSession).ForContext(request).Info(); done <- f }()
	select {
	case <-arrived:
	case <-phase.Done():
		t.Fatal("request did not reach socket fixture")
	}
	stop()
	select {
	case f := <-done:
		if f == port.None {
			t.Fatal("cancelled request invented observation")
		}
	case <-phase.Done():
		t.Fatal("request ignored cancellation")
	}
	select {
	case <-ended:
	case <-phase.Done():
		t.Fatal("HTTP request remained pending")
	}
}
