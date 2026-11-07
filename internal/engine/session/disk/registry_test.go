package disk

import (
	"context"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	ap "github.com/donvargax/itos-container/internal/engine/approval/port"
	af "github.com/donvargax/itos-container/internal/engine/approval/port/porttest"
	rp "github.com/donvargax/itos-container/internal/engine/readiness/port"
	rf "github.com/donvargax/itos-container/internal/engine/readiness/port/porttest"
	sp "github.com/donvargax/itos-container/internal/engine/session/port"
	sf "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
)

func TestRegistryRejectsInvalidAuthorityBeforeDiscoveryOrRegistration(t *testing.T) {
	for _, fault := range []string{"host", "invalid frozen", "frozen UID", "frozen GID"} {
		t.Run(fault, func(t *testing.T) {
			b, _, _ := sf.Fixture()
			b.Checkout.Root, _ = filepath.Abs(b.Checkout.Root)
			d := ap.Durable{Frozen: b.Frozen}
			host := &af.Host{UID: b.UID, GID: b.GID}
			checkout := &rf.Checkout{Identity: b.Checkout}
			files := &af.Registry{Failure: "state.unavailable"}
			r := Registry{Store: Store{Codec: &sf.Codec{Value: b}, Host: host, Checkout: checkout,
				Input: rp.Input{Root: b.Checkout.Root, OS: "linux", Architecture: "amd64"}}, Storage: files}
			want := sp.Refused
			switch fault {
			case "host":
				host.UID = "0"
			case "invalid frozen":
				d.Frozen.Settings, want = "different settings", sp.Invalid
			case "frozen UID":
				host.UID = "1001"
			case "frozen GID":
				host.GID = "1001"
			}
			registry := r.ForRun(d)
			id, token := strings.Repeat("b", 64), strings.Repeat("c", 64)
			if ids, failure := registry.List(t.Context()); ids != nil || failure != want {
				t.Fatal("discovery accepted invalid authority", ids, failure)
			}
			if entry, failure := registry.Open(t.Context(), id, true); entry != nil || failure != want {
				if entry != nil {
					entry.Close()
				}
				t.Fatal("open accepted invalid authority", failure)
			}
			lease, failure := registry.Register(t.Context(), d, id, token)
			if lease != nil {
				lease.Close()
			}
			if lease != nil || failure != want || len(files.Sessions) != 0 || len(files.Events) != 0 || len(checkout.Roots) != 0 {
				t.Fatal("invalid authority reached registration or checkout access", failure)
			}
		})
	}
}

func TestRegistryDiscoveryRejectsRelativeRoots(t *testing.T) {
	for _, root := range []string{"", ".", "relative/checkout"} {
		t.Run(root, func(t *testing.T) {
			b, _, _ := sf.Fixture()
			b.Checkout.Root, _ = filepath.Abs(b.Checkout.Root)
			files := &af.Registry{Failure: "state.unavailable"}
			r := Registry{Store: Store{Host: &af.Host{UID: b.UID, GID: b.GID},
				Input: rp.Input{Root: root, OS: "linux", Architecture: "amd64"}}, Storage: files}
			if ids, failure := r.List(t.Context()); ids != nil || failure != sp.Refused {
				t.Fatal("relative root reached discovery", ids, failure)
			}
			if entry, failure := r.Open(t.Context(), strings.Repeat("b", 64), true); entry != nil || failure != sp.Refused {
				t.Fatal("relative root reached protected authority", entry, failure)
			}
		})
	}
}

func TestRegistryCheckoutFaultsPreventRegistrationButNotDiscovery(t *testing.T) {
	for _, fault := range []string{"pin", "inspect", "changed"} {
		t.Run(fault, func(t *testing.T) {
			b, _, _ := sf.Fixture()
			b.Checkout.Root = t.TempDir()
			id, token := strings.Repeat("b", 64), strings.Repeat("c", 64)
			key := registryKey(b.Checkout.Root, b.UID) + "/" + id
			files := &af.Registry{Sessions: map[string]*af.SessionFiles{key: {Data: []byte("original")}}}
			checkout := &rf.Checkout{Identity: b.Checkout}
			switch fault {
			case "pin":
				checkout.Failure = rp.CheckoutUnavailable
			case "inspect":
				checkout.InspectFailure = rp.CheckoutUnavailable
			case "changed":
				checkout.Changed = true
			}
			r := Registry{Store: Store{Host: &af.Host{UID: b.UID, GID: b.GID}, Checkout: checkout,
				Input: rp.Input{Root: b.Checkout.Root, OS: "linux", Architecture: "amd64"}}, Storage: files}
			if ids, failure := r.List(t.Context()); failure != "" || !reflect.DeepEqual(ids, []string{id}) || len(checkout.Roots) != 0 {
				t.Fatal("checkout fault vetoed protected discovery", ids, failure)
			}
			lease, failure := r.Register(t.Context(), ap.Durable{Frozen: b.Frozen}, id, token)
			if lease != nil {
				lease.Close()
			}
			if lease != nil || failure != sp.Refused || checkout.Opened || len(files.Events) != 0 || string(files.Sessions[key].Data) != "original" {
				t.Fatal("registration accepted an unusable checkout or changed evidence", failure)
			}
		})
	}
}

func TestRegistryListClassifiesReadFailuresWithoutWriting(t *testing.T) {
	b, _, _ := sf.Fixture()
	b.Checkout.Root = t.TempDir()
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
