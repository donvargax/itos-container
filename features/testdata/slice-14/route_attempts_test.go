package main

import (
	"errors"
	"reflect"
	"testing"
	"testing/synctest"
	"time"
)

func TestRouteAttemptsOverlapAndRetainEveryResult(t *testing.T) {
	// Only memory and virtual time: no native probe, resolver or socket runs.
	// Complete out of order, retaining both successful and failed observations.
	synctest.Test(t, func(t *testing.T) {
		routes := []route{
			{Name: "slow", Protocol: "system6", Address: "resolver"},
			{Name: "fast", Protocol: "tcp4", Address: "listener"},
			{Name: "partial", Protocol: "udp6", Address: "other-listener"},
		}
		calls := make(chan string, len(routes))
		start := time.Now()
		got := routeAttempts(routes, "fresh", "running", func(p route, nonce string) (string, error) {
			calls <- nonce
			switch p.Name {
			case "slow":
				time.Sleep(3 * time.Second)
				return "", errors.New("deadline")
			case "fast":
				time.Sleep(time.Second)
				return "control:" + nonce, nil
			default:
				time.Sleep(2 * time.Second)
				return "partial reply", errors.New("incomplete")
			}
		})
		if elapsed := time.Since(start); elapsed != 3*time.Second {
			t.Errorf("independent route waits accumulated: got %s, want longest call of 3s", elapsed)
		}
		want := []attempt{
			{Name: "slow", Protocol: "system6", Address: "resolver", Nonce: "fresh-running-slow", Error: "deadline", Attempted: true},
			{Name: "fast", Protocol: "tcp4", Address: "listener", Nonce: "fresh-running-fast", Result: "control:fresh-running-fast", Attempted: true},
			{Name: "partial", Protocol: "udp6", Address: "other-listener", Nonce: "fresh-running-partial", Result: "partial reply", Error: "incomplete", Attempted: true},
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("missing or misattributed route observations: got %+v, want %+v", got, want)
		}
		close(calls)
		seen := map[string]int{}
		for nonce := range calls {
			seen[nonce]++
		}
		if !reflect.DeepEqual(seen, map[string]int{"fresh-running-slow": 1, "fresh-running-fast": 1, "fresh-running-partial": 1}) {
			t.Fatalf("each route must execute exactly once: %v", seen)
		}
	})
}

func TestRouteAttemptsEmpty(t *testing.T) {
	got := routeAttempts(nil, "fresh", "first", func(route, string) (string, error) {
		t.Fatal("an empty route inventory made a call")
		return "", nil
	})
	if len(got) != 0 {
		t.Fatalf("empty inventory fabricated attempts: %+v", got)
	}
}
