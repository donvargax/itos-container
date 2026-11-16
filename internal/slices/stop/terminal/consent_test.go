package terminal

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/creack/pty"
	"github.com/donvargax/itos-container/internal/engine/session/port"
)

func TestHomeConsentRequiresReviewedTerminal(t *testing.T) {
	for _, tc := range []struct {
		answer       string
		want, cancel bool
	}{
		{"y\n", true, false}, {"yes\n", true, false}, {"Y\n", true, false}, {"n\n", false, false}, {"\n", false, false}, {"\x04", false, false}, {"y", false, true},
	} {
		t.Run(tc.answer, func(t *testing.T) {
			master, slave, err := pty.Open()
			if errors.Is(err, pty.ErrUnsupported) {
				t.Skip("terminal unavailable")
			}
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = master.Close() }()
			defer func() { _ = slave.Close() }()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			entry := port.HomeEntry{ID: strings.Repeat("a", 64), Name: "itos-container-home-" + strings.Repeat("a", 64), UID: "1000", GID: "3001", Project: strings.Repeat("b", 64), Scope: strings.Repeat("c", 64), Engine: port.HomeEngineIdentity{Endpoint: "unix:///fixture.sock", ID: "original-daemon"}, References: []string{strings.Repeat("d", 64)}, State: "present", Disposition: "retained"}
			result := make(chan bool, 1)
			go func() { result <- (Consent{In: slave, Err: slave}).Review(ctx, entry, "reset") }()
			prompt := make(chan string, 1)
			go func() {
				var output strings.Builder
				b := make([]byte, 1)
				for {
					n, err := master.Read(b)
					if n > 0 {
						output.Write(b[:n])
					}
					if strings.Contains(output.String(), "y/N: ") || err != nil {
						prompt <- output.String()
						return
					}
				}
			}()
			select {
			case text := <-prompt:
				for _, expected := range []string{"reset", entry.ID, entry.Project, entry.Scope, entry.Name, entry.UID, entry.GID, entry.Engine.Endpoint, entry.Engine.ID, entry.References[0], "Permanent loss of ALL harness contents", "cannot be undone"} {
					if !strings.Contains(text, expected) {
						t.Fatalf("review omitted %q: %s", expected, text)
					}
				}
			case <-time.After(3 * time.Second):
				t.Fatal("review did not precede consent")
			}
			if tc.cancel {
				cancel()
			} else {
				_, _ = io.WriteString(master, tc.answer)
			}
			select {
			case got := <-result:
				if got != tc.want {
					t.Fatal("consent", got, "want", tc.want)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("terminal answer did not complete")
			}
		})
	}
}
func TestHomeConsentCannotBeRedirected(t *testing.T) {
	var output bytes.Buffer
	for _, input := range []string{"y\n", "yes\n", "Y\n", ""} {
		if (Consent{In: strings.NewReader(input), Err: &output}).Review(context.Background(), port.HomeEntry{}, "delete") {
			t.Fatal("redirected input consented")
		}
	}
	if output.Len() != 0 {
		t.Fatal("nonterminal prompt")
	}
	master, slave, err := pty.Open()
	if errors.Is(err, pty.ErrUnsupported) {
		t.Skip("terminal unavailable")
	}
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = master.Close() }()
	defer func() { _ = slave.Close() }()
	if (Consent{In: slave, Err: &output}).Review(context.Background(), port.HomeEntry{}, "delete") {
		t.Fatal("redirected review consented")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if (Consent{In: slave, Err: slave}).Review(ctx, port.HomeEntry{}, "delete") {
		t.Fatal("cancelled request consented")
	}
}
