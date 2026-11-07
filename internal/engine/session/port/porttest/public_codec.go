package porttest

import (
	"bytes"
	"strconv"

	"github.com/donvargax/itos-container/internal/engine/session/port"
)

// PublicRecords selects supplied valid representations without implementing a
// wire codec. Each distinct record has one encoding; unknown values are invalid.
type PublicRecords struct {
	Records    map[string]port.PublicRecord
	Admissions map[string]port.AdmissionRecord
}

func (c *PublicRecords) EncodePublic(record port.PublicRecord) []byte {
	for data, known := range c.Records {
		if samePublic(record, known) {
			return []byte(data)
		}
	}
	return nil
}

func (c *PublicRecords) DecodePublic(data []byte) (port.PublicRecord, port.Outcome) {
	record, ok := c.Records[string(data)]
	if !ok {
		return port.PublicRecord{}, port.Invalid
	}
	return copyPublic(record), ""
}

// PublicCodecContract checks value equality, refusal and independent returned
// storage against the table fake and wire adapter using a supplied valid record.
func PublicCodecContract(t Reporter, codec port.PublicCodec, record port.PublicRecord, encoded []byte) {
	t.Helper()
	record = copyPublic(record)
	if data := codec.EncodePublic(copyPublic(record)); !bytes.Equal(data, encoded) {
		t.Errorf("equal record values have different encodings")
	}
	decoded, failure := codec.DecodePublic(encoded)
	if failure != "" || !samePublic(decoded, record) {
		t.Errorf("record did not round trip: %#v, %s", decoded, failure)
		return
	}
	if decoded.WorkloadExit != nil {
		*decoded.WorkloadExit++
	}
	if decoded.LastProblem != nil {
		*decoded.LastProblem = port.Refused
	}
	decoded, failure = codec.DecodePublic(encoded)
	if failure != "" || !samePublic(decoded, record) {
		t.Errorf("caller changed decoded record storage")
	}
	data := codec.EncodePublic(record)
	if len(data) > 0 {
		data[0] ^= 1
	}
	if !bytes.Equal(codec.EncodePublic(record), encoded) {
		t.Errorf("caller changed encoded record storage")
	}
	for _, invalid := range [][]byte{nil, []byte("not a record")} {
		if got, failure := codec.DecodePublic(invalid); failure != port.Invalid || got != (port.PublicRecord{}) {
			t.Errorf("invalid representation acquired a record: %#v, %s", got, failure)
		}
	}
}

func (c *PublicRecords) EncodeAdmission(r port.AdmissionRecord) []byte {
	if c.Admissions == nil {
		c.Admissions = map[string]port.AdmissionRecord{}
	}
	for data, known := range c.Admissions {
		if r == known {
			return []byte(data)
		}
	}
	key := "admission-" + strconv.Itoa(len(c.Admissions))
	c.Admissions[key] = r
	return []byte(key)
}
func (c *PublicRecords) DecodeAdmission(data []byte) (port.AdmissionRecord, port.Outcome) {
	r, ok := c.Admissions[string(data)]
	if !ok {
		return port.AdmissionRecord{}, port.Invalid
	}
	return r, ""
}

func AdmissionCodecContract(t Reporter, codec port.PublicCodec, r port.AdmissionRecord) {
	t.Helper()
	data := codec.EncodeAdmission(r)
	value, failure := codec.DecodeAdmission(data)
	if failure != "" || value != r {
		t.Errorf("admission did not round trip: %#v %s", value, failure)
	}
	if len(data) > 0 {
		data[0] ^= 1
	}
	if got, failure := codec.DecodeAdmission(codec.EncodeAdmission(r)); failure != "" || got != r {
		t.Errorf("caller altered encoded admission: %#v %s", got, failure)
	}
	for _, invalid := range [][]byte{nil, []byte("not a record")} {
		if got, failure := codec.DecodeAdmission(invalid); failure != port.Invalid || got != (port.AdmissionRecord{}) {
			t.Errorf("invalid admission gained authority: %#v %s", got, failure)
		}
	}
}
