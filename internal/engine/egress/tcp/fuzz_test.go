package tcp

import (
	"bytes"
	"slices"
	"testing"

	"github.com/donvargax/itos-container/internal/engine/egress/domain"
)

// FuzzPeek feeds the peek arbitrary bytes, a cut of them as the bytes that came
// after the head and the rest from a scripted client socket. Nothing may panic or
// reach the client; the recording is a prefix of the input within HelloLimit; a
// pass has the exact server name and no 0xfe0d; and a pass forwards the whole
// input: what was recorded, what was left unserved and what the socket still holds
// together equal the input. The recording alone is a prefix of the input either
// way, so only this last assertion catches bytes the peek never asked for being
// lost.
func FuzzPeek(f *testing.F) {
	good := helloFor(f, "listed.example")
	for _, seed := range [][]byte{
		good,
		helloFor(f, "other.example"),
		helloFor(f, ""),
		echHello(f, "public.example"),
		append(bytes.Clone(good), bytes.Repeat([]byte{0xa5}, 6000)...),
		withExtension(f, good, 0xfe0a, []byte{1, 2, 3}),
		withExtension(f, good, 0xffce, []byte{1}),
		nil,
		[]byte("GET / HTTP/1.1\r\nHost: listed.example\r\n\r\n"),
		good[:100],
	} {
		f.Add(seed, uint16(0))
		f.Add(seed, uint16(100))
		f.Add(seed, uint16(len(seed)))
	}
	f.Fuzz(func(t *testing.T, input []byte, cut uint16) {
		tailLen := min(int(cut), len(input), domain.HeadLimit)
		tail, rest := bytes.Clone(input[:tailLen]), bytes.Clone(input[tailLen:])
		client := &scripted{data: rest}
		sh := newShim(client, tail)
		hello := peek(sh)
		if client.written.Load() != 0 {
			t.Fatalf("%d bytes reached the client", client.written.Load())
		}
		if len(sh.recorded) > domain.HelloLimit || !bytes.HasPrefix(input, sh.recorded) {
			t.Fatalf("recording of %d bytes is not a prefix of the input within %d", len(sh.recorded), domain.HelloLimit)
		}
		if !domain.JudgeHello(hello, "listed.example").Pass {
			return
		}
		if hello.ServerName != "listed.example" || slices.Contains(hello.Extensions, 0xfe0d) {
			t.Fatalf("a pass with server name %q and extensions %x", hello.ServerName, hello.Extensions)
		}
		whole := append(append(bytes.Clone(sh.recorded), sh.unserved()...), client.data...)
		if !bytes.Equal(whole, input) {
			t.Fatalf("a pass would forward %d bytes, not the %d-byte input", len(whole), len(input))
		}
	})
}
