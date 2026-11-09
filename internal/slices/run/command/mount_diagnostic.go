package command

import (
	"fmt"
	"strconv"

	"github.com/donvargax/itos-container/internal/engine/session/port"
)

type mountFailure struct {
	runFailure
	diagnostic port.MountDiagnostic
}

func (f mountFailure) Unwrap() error { return f.runFailure }

func mountDiagnosticText(d port.MountDiagnostic) string {
	// Identity values may originate in an engine response. Log only numbers;
	// never reproduce arbitrary response strings as diagnostics.
	number := func(value string) string {
		n, err := strconv.ParseUint(value, 10, 64)
		if err != nil {
			return "unavailable"
		}
		return strconv.FormatUint(n, 10)
	}
	text := fmt.Sprintf("check=%s expected=%s:%s observed=%s:%s pid=%d observed-pid=%d reason=%s",
		d.Check, number(d.ExpectedDevice), number(d.ExpectedInode), number(d.ObservedDevice), number(d.ObservedInode), d.PID, d.ObservedPID, strconv.QuoteToASCII(d.Reason))
	if d.System.Message != "" {
		text += fmt.Sprintf(" errno=%d error=%s", d.System.Errno, strconv.QuoteToASCII(d.System.Message))
	}
	return text
}
