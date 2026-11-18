// Package terminal obtains explicit consent to discarding one retained home.
package terminal

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"github.com/donvargax/itos-container/internal/engine/session/port"
	"golang.org/x/term"
	"io"
	"os"
	"strings"
)

type Consent struct {
	In  io.Reader
	Err io.Writer
}

func (c Consent) Review(ctx context.Context, e port.HomeEntry, action string) bool {
	in, ok := c.In.(*os.File)
	out, output := c.Err.(*os.File)
	if !ok || !output || !term.IsTerminal(int(in.Fd())) || !term.IsTerminal(int(out.Fd())) || ctx.Err() != nil {
		return false
	}
	data, _ := json.Marshal(e)
	_, _ = fmt.Fprintf(c.Err, "%s home %s\n%s\nPermanent loss of ALL harness contents. This cannot be undone.\nContinue? y/N: ", action, e.ID, data)
	answer := make(chan bool, 1)
	go func() {
		line, err := bufio.NewReader(io.LimitReader(in, 4097)).ReadString('\n')
		text := strings.ToLower(strings.TrimSpace(line))
		answer <- err == nil && (text == "y" || text == "yes")
	}()
	select {
	case <-ctx.Done():
		return false
	case yes := <-answer:
		return yes && ctx.Err() == nil
	}
}
