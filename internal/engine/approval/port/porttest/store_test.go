package porttest

import (
	"github.com/donvargax/itos-container/internal/engine/approval/port"
	"testing"
)

func TestMemoryStoreContract(t *testing.T) {
	StoreContract(t, func() (port.Store, port.Candidate, port.Candidate) {
		a := port.Candidate{Project: "p", Canonical: "A", Fingerprint: "a"}
		b := a
		b.Canonical = "B"
		b.Fingerprint = "b"
		return &Store{}, a, b
	})
}
func TestMemoryOverflowAndFailure(t *testing.T) {
	a := port.Candidate{Project: "p", Canonical: "A"}
	b := a
	b.Canonical = "B"
	s := &Store{Records: map[string]port.Record{"p": {Candidate: a, Generation: "18446744073709551615"}}}
	if _, failure := s.Commit(b, "18446744073709551615"); failure != "state.invalid" {
		t.Fatal(failure)
	}
	s.Failure = "state.unsafe"
	if _, failure := s.Select(a, "1"); failure != s.Failure {
		t.Fatal(failure)
	}
}
