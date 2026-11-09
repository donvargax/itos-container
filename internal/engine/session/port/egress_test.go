package port_test

import (
	"github.com/donvargax/itos-container/internal/engine/session/port"
	f "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestEgressGrant(t *testing.T) {
	image := "sha256:" + strings.Repeat("a", 64)
	for _, tc := range []struct {
		grant port.EgressGrant
		valid bool
		names []string
	}{
		{port.EgressGrant{}, true, nil},
		{port.EgressGrant{Names: "one.example\ntwo.example", ImageID: image}, true, []string{"one.example", "two.example"}},
		{port.EgressGrant{Names: "UPPER", ImageID: image}, true, []string{"UPPER"}},
		{port.EgressGrant{ImageID: image}, false, nil},
		{port.EgressGrant{Names: "one.example"}, false, nil},
		{port.EgressGrant{Names: "one.example\n", ImageID: image}, false, nil},
		{port.EgressGrant{Names: "\none.example", ImageID: image}, false, nil},
		{port.EgressGrant{Names: "one\n\ntwo", ImageID: image}, false, nil},
		{port.EgressGrant{Names: "one", ImageID: strings.ToUpper(image)}, false, nil},
		{port.EgressGrant{Names: "one", ImageID: image[:len(image)-1]}, false, nil},
	} {
		if got := tc.grant.Valid(); got != tc.valid {
			t.Errorf("%#v validity = %v, want %v", tc.grant, got, tc.valid)
		}
		if tc.valid && !slices.Equal(tc.grant.List(), tc.names) {
			t.Errorf("List = %q, want %q", tc.grant.List(), tc.names)
		}
	}
}
func TestValidEgress(t *testing.T) {
	b, _, _ := f.Fixture()
	b = f.EgressBinding(b)
	records := f.EgressRecords(b)
	for n, r := range records {
		if !r.ValidEgress(b) {
			t.Errorf("valid record %d refused", n)
		}
	}
	base := records[6]
	defects := map[string]func(*port.Record){
		"schema":              func(r *port.Record) { r.Schema = 2 },
		"internal name":       func(r *port.Record) { r.Egress.Internal.Name = "foreign" },
		"network name":        func(r *port.Record) { r.Egress.Network.Name = "foreign" },
		"proxy name":          func(r *port.Record) { r.Egress.Proxy.Name = "foreign" },
		"stage":               func(r *port.Record) { r.Egress.Proxy.Stage = "running" },
		"short ID":            func(r *port.Record) { r.Egress.Proxy.ID = "abc" },
		"upper ID":            func(r *port.Record) { r.Egress.Proxy.ID = strings.Repeat("A", 64) },
		"intent ID":           func(r *port.Record) { r.Egress.Proxy.Stage = "intent" },
		"skipped predecessor": func(r *port.Record) { r.Egress.Internal.Stage = ""; r.Egress.Internal.ID = "" },
		"absent predecessor":  func(r *port.Record) { r.Egress.Internal.Stage = "absent" },
		"worker before proxy": func(r *port.Record) { r.Stage = "intent"; r.Egress.Proxy.Stage = "intent"; r.Egress.Proxy.ID = "" },
		"anchor released":     func(r *port.Record) { r.Anchor.Stage = "absent" },
	}
	for name, mutate := range defects {
		t.Run(name, func(t *testing.T) {
			r := base
			mutate(&r)
			if r.ValidEgress(b) {
				t.Fatal("invalid egress accepted")
			}
		})
	}
	offline := b
	offline.Egress = port.EgressGrant{}
	if base.ValidEgress(offline) {
		t.Error("schema 3 accepted offline")
	}
	for _, schema := range []int{1, 2} {
		r := b.Intent(strings.Repeat("1", 64))
		r.Schema = schema
		if r.ValidEgress(b) {
			t.Error("worker intent accepted without schema 3")
		}
	}
}
func TestEgressTransitions(t *testing.T) {
	b, _, _ := f.Fixture()
	b = f.EgressBinding(b)
	rs := f.EgressRecords(b)
	for n, r := range rs {
		if n > 0 && !rs[n-1].ValidTransition(r) {
			t.Errorf("transition %d refused", n)
		}
		if n > 0 && r.ValidTransition(rs[n-1]) {
			t.Errorf("transition %d rolled back", n)
		}
		if r.Completed() != (n == len(rs)-1) {
			t.Errorf("completion at %d", n)
		}
		if n > 0 && !r.ValidTransition(r) {
			t.Errorf("repeat %d refused", n)
		}
	}
	for _, pair := range [][2]int{{0, 2}, {1, 3}, {2, 4}, {3, 5}, {4, 6}, {5, 7}, {6, 8}, {8, 10}, {9, 11}, {10, 12}, {11, 13}} {
		if rs[pair[0]].ValidTransition(rs[pair[1]]) {
			t.Errorf("skipped durable boundary %v", pair)
		}
	}
	for _, n := range []int{1, 3, 5} {
		r := rs[n]
		r.Egress.Proxy.Stage = "absent"
		if rs[n].ValidTransition(r) {
			t.Errorf("intent became absent at %d", n)
		}
	}
}
func TestEgressOwnership(t *testing.T) {
	b, _, _ := f.Fixture()
	b = f.EgressBinding(b)
	r := f.EgressRecords(b)[6]
	for _, kind := range []port.NetworkKind{port.InternalNetwork, port.EgressNetwork} {
		o := f.EgressNetwork(r, kind)
		if !r.OwnsNetwork(o, kind) || !r.ExactNetwork(o, kind) {
			t.Errorf("healthy network refused: %s", kind)
		}
		for _, field := range []string{"ID", "Name", "Driver", "Scope", "Token", "Project", "Fingerprint", "Session", "Kind"} {
			changed := o
			reflect.ValueOf(&changed).Elem().FieldByName(field).SetString("foreign")
			if r.OwnsNetwork(changed, kind) {
				t.Errorf("%s: foreign %s accepted", kind, field)
			}
		}
		changed := o
		changed.ExactLabels = false
		if r.OwnsNetwork(changed, kind) {
			t.Error("extra label accepted")
		}
		for _, field := range []string{"IPv6", "ConfigOnly", "Ingress", "Attachable", "FromConfig"} {
			changed := o
			reflect.ValueOf(&changed).Elem().FieldByName(field).SetBool(true)
			if r.ExactNetwork(changed, kind) {
				t.Errorf("%s: unsafe %s accepted", kind, field)
			}
		}
		changed = o
		changed.Internal = !o.Internal
		if r.ExactNetwork(changed, kind) {
			t.Error("wrong internal accepted")
		}
		if kind == port.InternalNetwork {
			changed = o
			changed.Isolated = false
			if r.ExactNetwork(changed, kind) {
				t.Error("nonisolated accepted")
			}
			changed = o
			changed.Gateway = true
			if r.ExactNetwork(changed, kind) {
				t.Error("gateway accepted")
			}
		}
	}
	o := f.EgressProxy(r, b)
	if !r.OwnsProxy(o, b) {
		t.Error("healthy proxy refused")
	}
	for _, field := range []string{"ID", "Name", "ImageID", "Token", "Project", "Fingerprint", "Session", "Kind"} {
		changed := o
		reflect.ValueOf(&changed).Elem().FieldByName(field).SetString("foreign")
		if r.OwnsProxy(changed, b) {
			t.Errorf("foreign proxy %s accepted", field)
		}
	}
}
