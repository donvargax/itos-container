// Package command runs the private proxy entry point; cmd supplies its adapter.
package command

import (
	"context"
	"errors"
	"fmt"

	"github.com/donvargax/itos-container/internal/engine/egress/port"
)

type CLI struct {
	Args []string `arg:"" optional:"" passthrough:"partial" sep:"none"`
}

func (c CLI) Run(ctx context.Context, proxy port.Proxy, stderr port.Log) int {
	if len(c.Args) < 2 || c.Args[0] != "--" {
		_, _ = fmt.Fprintln(stderr, port.ListInvalid)
		return 1
	}
	if err := proxy.Serve(ctx, c.Args[1:]); err != nil {
		failure := port.AcceptFailed
		var typed port.Failure
		if errors.As(err, &typed) {
			switch typed {
			case port.ListInvalid, port.ListenFailed, port.AcceptFailed, port.LogFailed, port.LogExhausted:
				failure = typed
			}
		}
		_, _ = fmt.Fprintln(stderr, failure)
		return 1
	}
	return 0
}
