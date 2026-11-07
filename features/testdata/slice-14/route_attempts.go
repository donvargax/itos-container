package main

import "sync"

type route struct{ Name, Protocol, Address, Answer string }
type attempt struct {
	Name, Protocol, Address, Nonce, Result, Error string
	Attempted                                     bool
}

func routeAttempts(routes []route, nonce, phase string, call func(route, string) (string, error)) []attempt {
	results := make([]attempt, len(routes))
	var pending sync.WaitGroup
	// Each route has its own client, deadline and nonce. Overlap independent
	// waits while retaining every result in inventory order. No round or phase
	// can publish a report or begin its after-controls until all calls finish.
	for i, p := range routes {
		pending.Go(func() {
			n := nonce + "-" + phase + "-" + p.Name
			a := attempt{Name: p.Name, Protocol: p.Protocol, Address: p.Address, Nonce: n, Attempted: true}
			var err error
			a.Result, err = call(p, n)
			if err != nil {
				a.Error = err.Error()
			}
			results[i] = a
		})
	}
	pending.Wait()
	return results
}
