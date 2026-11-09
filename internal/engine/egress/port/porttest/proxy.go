package porttest

import (
	"context"
	"encoding/json"
	"slices"
	"sync"
	"time"

	"github.com/donvargax/itos-container/internal/engine/egress/port"
)

// ProxyFixture is what one proxy under the contract is given. An adapter consumes
// Names and Log. The fake validates nothing, because a port-side package may not
// import the domain, so it takes the failure it should return for the names.
type ProxyFixture struct {
	Names   []string
	Log     port.Log
	Failure port.Failure
}

// Proxy is the in-memory port.Proxy for the handler's tests. It records the names
// it was given and writes the ready record, then serves until ctx ends.
type Proxy struct {
	fixture ProxyFixture
	mu      sync.Mutex
	names   []string
}

// NewProxy returns a fake that fails with fixture.Failure when it is a list
// failure, and otherwise writes the ready record to fixture.Log.
func NewProxy(fixture ProxyFixture) *Proxy { return &Proxy{fixture: fixture} }

// Names returns the names the last Serve was given.
func (p *Proxy) Names() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.names)
}

// Serve records the names, writes the ready record and waits for ctx.
func (p *Proxy) Serve(ctx context.Context, names []string) error {
	p.mu.Lock()
	p.names = slices.Clone(names)
	p.mu.Unlock()
	if p.fixture.Failure == port.ListInvalid {
		return port.ListInvalid
	}
	ready := port.Record{Schema: 1, Seq: 1, Time: time.Now().UTC().Format(time.RFC3339), Event: port.Ready}
	line, err := json.Marshal(ready)
	if err == nil {
		_, err = p.fixture.Log.Write(append(line, '\n'))
	}
	if err != nil {
		return port.LogFailed
	}
	<-ctx.Done()
	return nil
}
