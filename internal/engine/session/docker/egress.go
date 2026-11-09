package docker

import (
	"context"
	"github.com/donvargax/itos-container/internal/engine/session/port"
)

func (*runConnection) CreateNetwork(context.Context, port.Record, port.NetworkKind) (string, port.Outcome) {
	return "", port.Refused
}
func (*runConnection) InspectNetwork(context.Context, string) (port.NetworkObservation, port.Inspection) {
	return port.NetworkObservation{}, port.Unavailable
}
func (*runConnection) RemoveNetwork(context.Context, string) {}
func (*runConnection) CreateProxy(context.Context, port.Record) (string, port.Outcome) {
	return "", port.Refused
}
func (*runConnection) VerifyProxy(context.Context, string) port.Outcome { return port.Refused }
func (*runConnection) ProxyReady(context.Context, string) port.Outcome  { return port.ProxyFailed }
func (*runConnection) CopyProxyLog(context.Context, string, port.Writer) port.Outcome {
	return port.ExecutionFailed
}
