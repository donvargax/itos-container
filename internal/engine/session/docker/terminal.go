package docker

import (
	"context"
	"github.com/donvargax/itos-container/internal/engine/session/port"
	terminal "github.com/donvargax/itos-container/internal/engine/terminal/port"
	"github.com/moby/moby/client"
)

func (c *runConnection) SetTerminal(size terminal.Dimensions) port.Outcome {
	if !size.Valid() {
		return port.TerminalUnavailable
	}
	c.terminal = size
	return ""
}

func (c *runConnection) Resize(ctx context.Context, id string, size terminal.Dimensions) port.Outcome {
	if !size.Valid() {
		return port.TerminalUnavailable
	}
	if !c.pin.Unchanged() {
		return port.Refused
	}
	call, cancel := c.call(ctx)
	defer cancel()
	_, err := c.api.ContainerResize(call, id, client.ContainerResizeOptions{Height: uint(size.Rows), Width: uint(size.Columns)})
	if err != nil {
		return port.TerminalResize
	}
	return ""
}
