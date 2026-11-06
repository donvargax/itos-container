package main

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/donvargax/itos-container/internal/engine/manifest/jsonc"
	"github.com/donvargax/itos-container/internal/engine/session/codec"
	sp "github.com/donvargax/itos-container/internal/engine/session/port"
)

func TestPublicCodecRejectsMalformedRecoveryFacts(t *testing.T) {
	j := codec.JSON{Parser: jsonc.Parser{}}
	r := sp.PublicRecord{Schema: 1, ID: strings.Repeat("a", 64), Project: strings.Repeat("b", 64), Intent: sp.Reference(strings.Repeat("b", 64), strings.Repeat("c", 64)), State: "failed"}
	original := j.EncodePublic(r)
	for _, tc := range []struct{ field, value string }{
		{"schema", `"1"`}, {"id", `1`}, {"project", `true`}, {"intent", `[]`}, {"state", `null`},
		{"id", `"invalid"`}, {"intent", `"invalid"`}, {"state", `"invalid"`},
		{"workload_exit", `-1`}, {"workload_exit", `256`}, {"workload_exit", `-0`}, {"workload_exit", `1.5`},
		{"last_problem", `"unrecognized failure"`}, {"last_problem", `123`},
		{"workload_exit", `{"nested":[1,2]}`},
	} {
		t.Run(tc.field+"="+tc.value, func(t *testing.T) {
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(original, &fields); err != nil {
				t.Fatal(err)
			}
			fields[tc.field] = json.RawMessage(tc.value)
			data, err := json.Marshal(fields)
			if err != nil {
				t.Fatal(err)
			}
			if got, f := j.DecodePublic(data); f != sp.Invalid || !reflect.DeepEqual(got, sp.PublicRecord{}) {
				t.Fatal("malformed fact retained usable authority", got, f)
			}
		})
	}
	for _, data := range [][]byte{nil, []byte("{"), []byte("["), []byte(`{"id":`), append(bytes.Clone(original), []byte(" trailing")...), bytes.Replace(original, []byte(`"workload_exit":null`), []byte(`"extra":null`), 1), bytes.Replace(original, []byte(`"last_problem":null`), []byte(`"extra":null`), 1)} {
		if got, f := j.DecodePublic(data); f != sp.Invalid || !reflect.DeepEqual(got, sp.PublicRecord{}) {
			t.Fatal("incomplete record retained authority", got, f)
		}
	}
	for _, exit := range []int{0, 255} {
		r.WorkloadExit = &exit
		problem := sp.EnvironmentMissing
		r.LastProblem = &problem
		got, f := j.DecodePublic(j.EncodePublic(r))
		if f != "" || !reflect.DeepEqual(got, r) {
			t.Fatal("boundary record lost observed facts", got, f)
		}
	}
}
