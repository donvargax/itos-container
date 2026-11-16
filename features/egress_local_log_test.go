package features

import (
	"encoding/binary"
	"net/http"
	"strconv"
)

func egressFrame(stream byte, data string) []byte {
	header := make([]byte, 8)
	header[0] = stream
	binary.BigEndian.PutUint32(header[4:], uint32(len(data)))
	return append(header, data...)
}

// copyLog is called with the local fixture mutex held. Only the stopped
// copy is faulted; startup still receives its independent ready stream.
func (a *localEgressAPI) copyLog(out http.ResponseWriter) {
	frame := egressFrame(1, *a.copyStdout)
	if a.copyFault {
		a.faults = append(a.faults, "copy transport")
		// An acknowledged body prefix, then an incomplete Docker frame:
		// the real SDK must observe a failed read, never a clean EOF.
		out.Header().Set("Content-Length", strconv.Itoa(len(frame)))
		_, _ = out.Write(frame[:len(frame)-1])
	} else {
		_, _ = out.Write(frame)
	}
}
