package docker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/donvargax/itos-container/internal/engine/session/port"
	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/client"
	"io"
	"strings"
	"time"
)

var errFirstLineDone = errors.New("first stdout line complete")

type readyLine struct {
	data                []byte
	complete, oversized bool
}

func (w *readyLine) Write(p []byte) (int, error) {
	length := len(p)
	at := bytes.IndexByte(p, '\n')
	part := p
	if at >= 0 {
		part = p[:at]
	}
	if len(w.data)+len(part) > 2048 {
		w.oversized = true
		return 0, errFirstLineDone
	}
	w.data = append(w.data, part...)
	if at >= 0 {
		w.complete = true
		return length, errFirstLineDone
	}
	return length, nil
}
func (c *runConnection) ProxyReady(ctx context.Context, id string) port.Outcome {
	if !c.pin.Unchanged() {
		return port.Refused
	}
	call, cancel := c.call(ctx)
	defer cancel()
	stream, err := c.api.ContainerLogs(call, id, client.ContainerLogsOptions{ShowStdout: true, Follow: true})
	if err != nil {
		if !c.pin.Unchanged() {
			return port.Refused
		}
		return port.ExecutionFailed
	}
	defer func() { _ = stream.Close() }()
	line := &readyLine{}
	_, err = stdcopy.StdCopy(line, io.Discard, stream)
	if !c.pin.Unchanged() {
		return port.Refused
	}
	if err != nil && !errors.Is(err, errFirstLineDone) {
		return port.ExecutionFailed
	}
	if line.oversized || !line.complete || !validReady(line.data) {
		return port.ProxyFailed
	}
	return ""
}
func validReady(data []byte) bool {
	d := json.NewDecoder(bytes.NewReader(data))
	token, err := d.Token()
	if err != nil || token != json.Delim('{') {
		return false
	}
	fields := map[string]json.RawMessage{}
	for d.More() {
		token, err := d.Token()
		if err != nil {
			return false
		}
		key, ok := token.(string)
		if !ok || fields[key] != nil {
			return false
		}
		var value json.RawMessage
		if d.Decode(&value) != nil {
			return false
		}
		fields[key] = value
	}
	token, err = d.Token()
	if err != nil || token != json.Delim('}') || d.InputOffset() != int64(len(data)) || len(fields) != 4 {
		return false
	}
	var schema, seq int
	var event, stamp string
	if json.Unmarshal(fields["schema"], &schema) != nil || schema != 1 || json.Unmarshal(fields["seq"], &seq) != nil || seq != 1 || json.Unmarshal(fields["event"], &event) != nil || event != "ready" || json.Unmarshal(fields["time"], &stamp) != nil || !strings.HasSuffix(stamp, "Z") {
		return false
	}
	_, err = time.Parse(time.RFC3339Nano, stamp)
	return err == nil
}
func (c *runConnection) CopyProxyLog(ctx context.Context, id string, w port.Writer) port.Outcome {
	if !c.pin.Unchanged() {
		return port.Refused
	}
	call, cancel := c.call(ctx)
	defer cancel()
	stream, err := c.api.ContainerLogs(call, id, client.ContainerLogsOptions{ShowStdout: true})
	if err != nil {
		if !c.pin.Unchanged() {
			return port.Refused
		}
		return port.ExecutionFailed
	}
	defer func() { _ = stream.Close() }()
	_, err = stdcopy.StdCopy(w, io.Discard, stream)
	if !c.pin.Unchanged() {
		return port.Refused
	}
	if err != nil {
		return port.ExecutionFailed
	}
	return ""
}
