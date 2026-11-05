package porttest

import (
	"bytes"

	"github.com/donvargax/itos-container/internal/engine/approval/port"
)

// FrozenRecords selects precomputed envelopes without implementing a wire codec.
// Tests supply the valid frozen values and their independently encoded records.
type FrozenRecords struct {
	Values map[port.Frozen]port.Record
	Bytes  map[port.Record]string
}

func (f FrozenRecords) Encode(record port.Record) []byte { return []byte(f.Bytes[record]) }

func (f FrozenRecords) Decode(candidate port.Candidate, data []byte) (port.Record, port.Failure) {
	for record, encoded := range f.Bytes {
		if encoded == string(data) && record.Project == candidate.Project && record.UID == candidate.UID {
			return record, ""
		}
	}
	return port.Record{}, "state.invalid"
}

func (f FrozenRecords) FrozenRecord(value port.Frozen) (port.Record, port.Failure) {
	record, ok := f.Values[value]
	if !ok {
		return port.Record{}, "state.invalid"
	}
	return record, ""
}

// FrozenRecordsContract holds validation and independent returned bytes for both
// the table fake and the canonical adapter, given one valid independent record.
func FrozenRecordsContract(t Reporter, codec port.FrozenRecords, frozen port.Frozen, record port.Record, encoded []byte) {
	t.Helper()
	if got, failure := codec.FrozenRecord(frozen); failure != "" || got != record {
		t.Errorf("frozen selection = %#v, %s", got, failure)
	}
	if got := codec.Encode(record); !bytes.Equal(got, encoded) {
		t.Errorf("encoded record differs from the supplied envelope")
	}
	if got, failure := codec.Decode(record.Candidate, encoded); failure != "" || got != record {
		t.Errorf("decoded record = %#v, %s", got, failure)
	}
	for _, edit := range []func(*port.Frozen){
		func(f *port.Frozen) { f.Project = "other" },
		func(f *port.Frozen) { f.Generation = "01" },
		func(f *port.Frozen) { f.Fingerprint = "wrong" },
		func(f *port.Frozen) { f.Settings += "\n" },
	} {
		invalid := frozen
		edit(&invalid)
		if got, failure := codec.FrozenRecord(invalid); failure != "state.invalid" || got != (port.Record{}) {
			t.Errorf("invalid frozen input returned %#v, %s", got, failure)
		}
	}
	for _, invalid := range [][]byte{nil, append(append([]byte{}, encoded...), '\n')} {
		if got, failure := codec.Decode(record.Candidate, invalid); failure != "state.invalid" || got != (port.Record{}) {
			t.Errorf("invalid envelope returned %#v, %s", got, failure)
		}
	}
	for _, edit := range []func(*port.Candidate){
		func(c *port.Candidate) { c.Project = "other" },
		func(c *port.Candidate) { c.UID = "other" },
	} {
		foreign := record.Candidate
		edit(&foreign)
		if got, failure := codec.Decode(foreign, encoded); failure != "state.invalid" || got != (port.Record{}) {
			t.Errorf("foreign binding returned %#v, %s", got, failure)
		}
	}
	copyBytes := codec.Encode(record)
	if len(copyBytes) > 0 {
		copyBytes[0] ^= 1
	}
	if !bytes.Equal(codec.Encode(record), encoded) {
		t.Errorf("mutating returned bytes changed the stored envelope")
	}
}
