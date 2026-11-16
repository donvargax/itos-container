package features

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/cucumber/godog"
)

func (w *world) egressLifePayloadFixture(table *godog.Table) error {
	if err := w.egressLifeTrafficFixture("payload"); err != nil {
		return err
	}
	n, l := w.egress.native, w.egress.lifeTraffic
	if _, err := n.endpoint("payload", "public", []string{"api.example.com"}, ""); err != nil {
		return err
	}
	n.controlAll()
	l.rows = egressLifeTable(table)
	if len(l.rows) != 9 {
		return errors.New("unexpected payload surface table")
	}
	w.markers = &syntheticMarkerWorld{trees: map[string]string{"protected product state": w.egressStatePath()}, outputs: map[string][]byte{}}
	l.steps = []egressStep{n.canonicalControl()}
	for i, row := range l.rows {
		marker := "SYNTHETIC_SLICE28_" + egressNonce() + "_MARKER"
		w.markers.values = append(w.markers.values, marker)
		step := egressStep{Op: "raw", Method: "CONNECT", Name: "api.example.com", Mode: fmt.Sprint(i)}
		switch row["surface"] {
		case "CONNECT userinfo before api.example.com:443":
			step.Target = marker + "@api.example.com:443"
		case "CONNECT a/path?token=marker:443":
			step.Target = "a/path?token=" + marker + ":443"
		case "CONNECT api.example.com:443/path?token=marker":
			step.Target = "api.example.com:443/path?token=" + marker
		case "GET http://api.example.com/path?token=marker":
			step.Method = "GET"
			step.Target = "http://api.example.com/path?token=" + marker
		case "unlisted CONNECT headers including Proxy-Authorization":
			step.Target = "unlisted.example:443"
			step.Headers = "Proxy-Authorization: Bearer " + marker + "\r\nX-Fixture: " + marker + "\r\n"
		case "accepted TLS URL query headers and body":
			step.Op = "tls"
			step.Mode = "payload"
			step.Nonce = "payload-" + egressNonce()
			step.Payload = marker
		case "mismatched ClientHello SNI":
			step.Op = "hello"
			step.Input = "ClientHello with " + marker + ".invalid SNI"
		case "non-TLS bytes after CONNECT":
			step.Op = "hello"
			step.Input = "payload bytes"
			step.Payload = "GET /" + marker + " HTTP/1.1\r\n\r\n"
		case "engine error body during the first proxy removal":
			l.fault = &egressLifeRemoveFault{body: `{"message":"` + marker + `"}`}
			l.recorder.refuse = l.fault.refuse
			continue
		default:
			return errors.New("unknown payload surface")
		}
		l.steps = append(l.steps, step)
	}
	l.steps = append(l.steps, n.canonicalControl(), egressStep{Op: "barrier", Name: "evidence"})
	return nil
}

func (w *world) egressLifeMarkerOutputs() {
	l := w.egress.lifeTraffic
	w.markers.outputs["proxy stdout and Docker log frames"] = append(bytes.Clone(l.frames), l.stdout...)
	output := append(l.run.stdout.bytes(), l.run.stderr.bytes()...)
	for _, r := range w.egress.results {
		output = append(output, []byte(r.result.stdout+r.result.stderr)...)
	}
	w.markers.outputs["controller diagnostics and all plain and JSON status"] = output
}

func (w *world) egressLifePayloadReceipts() error {
	l, n := w.egress.lifeTraffic, w.egress.native
	if err := w.egressLifeTrafficHealthy(); err != nil {
		return err
	}
	if l.run.exit != 75 || !strings.Contains(string(l.run.stderr.bytes()), "run.cleanup-temporary") {
		return errors.New("engine removal failure was not observed")
	}
	if err := n.exchanged(l.run, 3); err != nil {
		return err
	}
	// The probe report echoes the exact sent operation and supplies length/digest
	// receipts. Neither expected nor observed payload bytes appear in diagnostics.
	raw, hello := l.run.results("raw"), l.run.results("hello")
	if len(raw) != 5 || len(hello) != 2 {
		return errors.New("not every payload operation completed")
	}
	for i, r := range raw {
		s := l.steps[i+1]
		wire := []byte(fmt.Sprintf("%s %s HTTP/1.1\r\nHost: %s\r\n%s\r\n%s", s.Method, s.Target, s.Target, s.Headers, s.Payload))
		if !egressLifeEmptyResponse(r, 403) || r.Sent != len(wire) || r.Digest != egressLifeDigest(wire) {
			return fmt.Errorf("payload row %d lacks its sent bytes and refusal receipt", i+1)
		}
	}
	for i, r := range hello {
		var wire []byte
		if i == 0 {
			wire = egressLifeHello(w.markers.values[6] + ".invalid")
		} else {
			wire = []byte(l.steps[8].Payload)
		}
		if !r.OK || r.Status != 200 || !r.Closed || r.Received != 0 || r.Sent != len(wire) || r.Digest != egressLifeDigest(wire) {
			return errors.New("post-CONNECT payload was not sent and closed without response")
		}
	}
	tls := l.run.results("tls")[1]
	payload := []byte(strings.Repeat(w.markers.values[5], 3))
	if tls.Sent != len(payload) || tls.Digest != egressLifeDigest(payload) {
		return errors.New("allowed payload has no send receipt")
	}
	receipts, err := n.endpoints["payload"].receipts()
	if err != nil {
		return err
	}
	found := 0
	for _, r := range receipts {
		if r.Kind == "payload" {
			found++
			if r.Digest != tls.Digest || r.PayloadBytes != tls.Sent || r.Nonce != l.steps[6].Nonce {
				return errors.New("allowed payload differs at endpoint")
			}
		}
		if r.Kind == "conn" && !r.Handshake && r.Bytes != 0 {
			return errors.New("refused payload reached upstream")
		}
	}
	if found != 1 {
		return errors.New("missing independent allowed payload receipt")
	}
	l.fault.mu.Lock()
	hits, path, body := l.fault.hits, l.fault.path, l.fault.body
	l.fault.mu.Unlock()
	if hits != 1 {
		return errors.New("engine fault body was not delivered once")
	}
	observed := 0
	for _, r := range l.journal {
		if r.method == "DELETE" && r.path == path && r.status == 500 {
			observed++
			if len(r.response) != len(body) || egressLifeDigest(r.response) != egressLifeDigest([]byte(body)) {
				return errors.New("engine fault delivery digest differs")
			}
		}
	}
	if observed != 1 {
		return errors.New("missing engine error receipt")
	}
	if len(w.egress.results) != 3 || w.egress.results[0].result.exit != 0 {
		return errors.New("fresh stop did not recover the one-shot removal fault")
	}
	saved, err := os.ReadFile(l.path)
	if err != nil || !bytes.Equal(saved, l.saved) {
		return errors.New("recovery changed refusal bytes")
	}
	return nil
}

// Independently encode the fixed TLS 1.2 record used as a ClientHello parser
// probe. This is a test payload, with no TLS termination or product parser.
func egressLifeHello(name string) []byte {
	u16 := func(n int) []byte { return []byte{byte(n >> 8), byte(n)} }
	entry := append([]byte{0}, append(u16(len(name)), []byte(name)...)...)
	names := append(u16(len(entry)), entry...)
	extensions := append([]byte{0, 0}, append(u16(len(names)), names...)...)
	extensions = append(extensions, 0, 43, 0, 3, 2, 3, 4)
	body := append([]byte{3, 3}, bytes.Repeat([]byte{0x27}, 32)...)
	body = append(body, 0, 0, 2, 0x13, 0x01, 1, 0)
	body = append(body, append(u16(len(extensions)), extensions...)...)
	handshake := append([]byte{1, 0}, append(u16(len(body)), body...)...)
	return append([]byte{22, 3, 1}, append(u16(len(handshake)), handshake...)...)
}
