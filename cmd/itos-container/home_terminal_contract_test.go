package main

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/creack/pty"
	sp "github.com/donvargax/itos-container/internal/engine/session/port"
	"github.com/donvargax/itos-container/internal/slices/stop/terminal"
)

type homeTerminalAnswer struct {
	t        *testing.T
	response string
	want     sp.HomeEntry
}

func (c homeTerminalAnswer) Review(ctx context.Context, entry sp.HomeEntry, action string) bool {
	c.t.Helper()
	if entry.ID != c.want.ID || entry.Project != c.want.Project || entry.UID != c.want.UID || entry.GID != c.want.GID || entry.Engine != c.want.Engine || entry.Name != c.want.Name {
		c.t.Fatal("review changed original binding", entry, c.want)
	}
	if c.response == "redirected yes" {
		return (terminal.Consent{In: strings.NewReader("y\n"), Err: io.Discard}).Review(ctx, entry, action)
	}
	master, slave, err := pty.Open()
	if errors.Is(err, pty.ErrUnsupported) {
		c.t.Skip("terminal unavailable")
	}
	if err != nil {
		c.t.Fatal(err)
	}
	defer func() { _ = master.Close() }()
	defer func() { _ = slave.Close() }()
	review, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan bool, 1)
	go func() { done <- (terminal.Consent{In: slave, Err: slave}).Review(review, entry, action) }()
	prompt := make(chan string, 1)
	go func() {
		var output strings.Builder
		b := make([]byte, 1)
		for {
			n, e := master.Read(b)
			if n > 0 {
				output.Write(b[:n])
			}
			if e != nil || strings.Contains(output.String(), "y/N: ") {
				prompt <- output.String()
				return
			}
		}
	}()
	select {
	case text := <-prompt:
		for _, want := range []string{action, entry.ID, entry.Project, entry.Scope, entry.UID, entry.GID, entry.Engine.ID, entry.Engine.Endpoint, entry.Name, entry.State, "Permanent loss of ALL harness contents", "cannot be undone"} {
			if !strings.Contains(text, want) {
				c.t.Fatal("terminal review omitted", want, text)
			}
		}
	case <-time.After(5 * time.Second):
		c.t.Fatal("review did not precede consent")
	}
	answer := ""
	switch c.response {
	case "terminal yes":
		answer = "y\n"
	case "terminal no":
		answer = "n\n"
	case "terminal empty":
		answer = "\n"
	case "terminal EOF":
		answer = "\x04"
	case "terminal cancelled":
		cancel()
	default:
		c.t.Fatal(c.response)
	}
	if answer != "" {
		if _, err = io.WriteString(master, answer); err != nil {
			c.t.Fatal(err)
		}
	}
	select {
	case yes := <-done:
		return yes
	case <-time.After(5 * time.Second):
		c.t.Fatal("terminal did not finish")
	}
	return false
}

func homeConsentFor(t *testing.T, f *homeLocalFixture, g sp.HomeGeneration, response string) sp.HomeConsent {
	s := f.scope()
	return homeTerminalAnswer{t: t, response: response, want: sp.HomeEntry{ID: g.ID, Name: g.Name, Project: s.Project, UID: s.UID, GID: s.GID, Engine: s.Engine}}
}
