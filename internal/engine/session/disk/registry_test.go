package disk

import (
	"context"
	"reflect"
	"strings"
	"testing"

	ap "github.com/donvargax/itos-container/internal/engine/approval/port"
	af "github.com/donvargax/itos-container/internal/engine/approval/port/porttest"
	rp "github.com/donvargax/itos-container/internal/engine/readiness/port"
	sp "github.com/donvargax/itos-container/internal/engine/session/port"
	sf "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
)

func TestRegistryListClassifiesReadFailuresWithoutWriting(t *testing.T) {
	b, _, _ := sf.Fixture()
	d := ap.Durable{Frozen: b.Frozen}
	id := strings.Repeat("b", 64)
	key := registryKey(b.Checkout.Root, b.UID) + "/" + id
	files := &af.Registry{Sessions: map[string]*af.SessionFiles{key: {Data: []byte("original")}}}
	r := Registry{Store: Store{Codec: &sf.Codec{Value: b}, Host: &af.Host{UID: b.UID, GID: b.GID}, Input: rp.Input{OS: "linux", Architecture: "amd64"}}, Storage: files}
	registry := r.ForRun(d)
	for _, tc := range []struct {
		failure ap.Failure
		want    sp.Outcome
	}{
		{"state.unavailable", sp.Incomplete}, {"state.path", sp.Unsafe}, {"state.invalid", sp.Invalid}, {"state.conflict", sp.Refused},
	} {
		files.Failure = tc.failure
		if ids, failure := registry.List(context.Background()); ids != nil || failure != tc.want {
			t.Fatal(ids, failure, tc.want)
		}
	}
	files.Failure = ""
	if ids, failure := registry.List(context.Background()); failure != "" || !reflect.DeepEqual(ids, []string{id}) {
		t.Fatal("retry lost registered session", ids, failure)
	}
	if len(files.Events) != 0 || string(files.Sessions[key].Data) != "original" {
		t.Fatal("discovery mutated protected state")
	}
}
