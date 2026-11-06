package main

import (
	"bytes"
	"strings"
	"testing"

	parser "github.com/donvargax/itos-container/internal/engine/manifest/jsonc"
	"github.com/donvargax/itos-container/internal/engine/session/codec"
	"github.com/donvargax/itos-container/internal/engine/session/port"
)

func TestPublicRecordStrictSchema(t *testing.T) {
	c := codec.JSON{Parser: parser.Parser{}}
	id, project, token := strings.Repeat("a", 64), strings.Repeat("b", 64), strings.Repeat("c", 64)
	base := `{"schema":1,"id":"` + id + `","project":"` + project + `","intent":"intents/` + project + `/` + token + `.json","state":"failed","workload_exit":null,"last_problem":null}`
	for _, state := range []string{"preparing", "ready", "running", "stopping", "stopped", "failed"} {
		for _, exit := range []string{"null", "0", "7", "255"} {
			data := strings.Replace(base, `"state":"failed"`, `"state":"`+state+`"`, 1)
			data = strings.Replace(data, `"workload_exit":null`, `"workload_exit":`+exit, 1)
			r, f := c.DecodePublic([]byte(data))
			if f != "" || r.State != state {
				t.Fatal(f, r)
			}
			again, f := c.DecodePublic(c.EncodePublic(r))
			if f != "" || !bytes.Equal(c.EncodePublic(r), c.EncodePublic(again)) {
				t.Fatal("record round trip lost facts")
			}
		}
	}
	for _, data := range []string{"", "null", "[]", "{}", "{", base + " {}", strings.Repeat(" ", 65537), strings.Repeat("[", 17) + "0" + strings.Repeat("]", 17), strings.Replace(base, `"schema":1`, `"schema":2`, 1), strings.Replace(base, `"schema":1`, `"schema":1,"schema":1`, 1), strings.Replace(base, `"state":"failed"`, `"state":"future"`, 1), strings.Replace(base, `"state":"failed"`, `"state":null`, 1), strings.Replace(base, `"last_problem":null`, `"last_problem":"raw engine body"`, 1), strings.Replace(base, `"last_problem":null`, `"extra":null`, 1), strings.Replace(base, id, id[:63], 1), strings.Replace(base, `intents/`+project, `intents/`+id, 1)} {
		if _, f := c.DecodePublic([]byte(data)); f != port.Invalid {
			t.Fatalf("invalid public record accepted: %q", data)
		}
	}
	for _, value := range []string{`"7"`, `false`, `{}`, `[]`, `-1`, `256`, `1.5`, `1e0`} {
		data := strings.Replace(base, `"workload_exit":null`, `"workload_exit":`+value, 1)
		if _, f := c.DecodePublic([]byte(data)); f != port.Invalid {
			t.Fatalf("invalid workload exit accepted: %s", value)
		}
	}
	for _, failure := range []port.Outcome{port.Refused, port.Unsafe, port.Invalid, port.Environment, port.Internal, port.Incomplete, port.Uncertain, port.IntentWrite, port.OwnershipWrite, port.CompletionWrite, port.ExecutionFailed, port.IsolationInspection, port.IsolationUnverified} {
		r, f := c.DecodePublic([]byte(base))
		if f != "" {
			t.Fatal(f)
		}
		r.LastProblem = &failure
		if _, f = c.DecodePublic(c.EncodePublic(r)); f != "" {
			t.Fatal(failure, f)
		}
	}
}
