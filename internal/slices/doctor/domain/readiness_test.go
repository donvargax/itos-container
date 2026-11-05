package domain

import (
	"reflect"
	"testing"

	readiness "github.com/donvargax/itos-container/internal/engine/readiness/port"
	"github.com/donvargax/itos-container/internal/slices/doctor/port/porttest"
)

func TestInspectReadinessPreservesObservations(t *testing.T) {
	want := readiness.Result{Issues: []readiness.Issue{{Kind: readiness.EngineChanged, Subject: "DOCKER_HOST"}}}
	r := &porttest.Readiness{Result: want}
	if got := InspectReadiness(r); !reflect.DeepEqual(got, want) || r.Reads != 1 {
		t.Fatal(got, r.Reads)
	}
}
