package domain

import (
	"github.com/donvargax/itos-container/internal/engine/approval/port"
	"github.com/donvargax/itos-container/internal/engine/approval/port/porttest"
	"strings"
	"testing"
)

func serviceFixture() (Service, *porttest.Readiness, *porttest.Host, *porttest.Store) {
	c := port.Candidate{UID: "2000", GID: "3000", Project: "project", Canonical: "A", Fingerprint: "sha256:" + strings.Repeat("a", 64)}
	ready := &porttest.Readiness{Result: porttest.Ready()}
	host := &porttest.Host{UID: "2000", GID: "3000", Stamp: "socket"}
	store := &porttest.Store{}
	return Service{Readiness: ready, Host: host, Store: store, Codec: &porttest.Codec{Values: map[string]port.Candidate{"2000:3000": c}}}, ready, host, store
}
func TestResolveIdentityAndManifest(t *testing.T) {
	service, ready, _, _ := serviceFixture()
	for _, tc := range []struct {
		uid, gid string
		failure  port.Failure
	}{{"0", "3000", "identity.root"}, {"", "3000", "identity.unavailable"}, {"2000", "-1", "identity.unavailable"}, {"01", "3", "identity.unavailable"}, {"4294967296", "1", "identity.unavailable"}, {"2000", "3000", ""}} {
		c, f := Resolve(ready.Result, tc.uid, tc.gid, service.Codec)
		if f != tc.failure || (f != "" && c.Canonical != "") {
			t.Errorf("identity %q:%q = %#v %s", tc.uid, tc.gid, c, f)
		}
	}
	for _, field := range []string{"containerUser", "remoteUser"} {
		r := porttest.Ready()
		other := "999"
		if field == "containerUser" {
			r.Manifest.Effective.ContainerUser = &other
		} else {
			r.Manifest.Effective.RemoteUser = &other
		}
		if _, f := Resolve(r, "2000", "3000", service.Codec); f != "identity.mismatch" || Mismatch(r, "2000") != field {
			t.Errorf("mismatch %s = %s", field, f)
		}
	}
	r := porttest.Ready()
	r.Ready = false
	if _, f := Resolve(r, "2000", "3000", service.Codec); f != "prerequisite" {
		t.Fatal(f)
	}
}
func TestInspectAndApprove(t *testing.T) {
	for _, kind := range []string{"missing", "matching", "changed", "invalid", "root", "socket", "prerequisite"} {
		t.Run(kind, func(t *testing.T) {
			service, ready, host, store := serviceFixture()
			c, _ := Resolve(ready.Result, host.UID, host.GID, service.Codec)
			switch kind {
			case "matching", "changed":
				old := c
				if kind == "changed" {
					old.Canonical = "old"
				}
				store.Records = map[string]port.Record{c.Project: {Candidate: old, Generation: "7"}}
			case "invalid":
				store.Failure = "state.invalid"
			case "root":
				host.UID = "0"
			case "socket":
				host.Stamp = ""
			case "prerequisite":
				ready.Result.Ready = false
			}
			v := service.Inspect()
			switch kind {
			case "missing", "matching", "changed":
				if v.Failure != "" || v.Status != kind {
					t.Fatal(v)
				}
			case "invalid":
				if v.Status != "invalid" || v.Failure != "state.invalid" {
					t.Fatal(v)
				}
			case "root":
				if v.Failure != "identity.root" {
					t.Fatal(v)
				}
			case "socket":
				if v.Failure != "identity.unavailable" {
					t.Fatal(v)
				}
			case "prerequisite":
				if v.Candidate.Canonical != "" {
					t.Fatal(v)
				}
			}
		})
	}
	for _, kind := range []string{"success", "settings", "socket", "prerequisite", "invalid", "write", "conflict"} {
		t.Run(kind, func(t *testing.T) {
			service, ready, host, store := serviceFixture()
			review := service.Inspect()
			switch kind {
			case "settings":
				host.GID = "4000"
			case "socket":
				host.Stamp = "replacement"
			case "prerequisite":
				ready.Result.Ready = false
			case "invalid":
				store.Failure = "state.invalid"
			case "write":
				store.CommitFailure = "state.write"
			case "conflict":
				store.Records = map[string]port.Record{review.Candidate.Project: {Candidate: review.Candidate, Generation: "2"}}
			}
			v := service.Approve(review)
			want := port.Failure("")
			switch kind {
			case "settings", "socket", "prerequisite":
				want = "approve.changed"
			case "invalid":
				want = "state.invalid"
			case "write":
				want = "state.write"
			case "conflict":
				want = "approve.conflict"
			}
			if v.Failure != want {
				t.Fatal(v)
			}
			if kind == "success" {
				if v.Status != "matching" || v.Record.Generation != "1" || store.Writes != 1 {
					t.Fatal(v)
				}
				f, err := service.Freeze(v)
				if err != "" || f.Settings != v.Candidate.Canonical {
					t.Fatal(f, err)
				}
			} else if store.Writes != 0 {
				t.Fatal("refusal wrote state")
			}
		})
	}
}
func TestFingerprintFormat(t *testing.T) {
	good := "sha256:" + strings.Repeat("a", 64)
	for _, value := range []string{"", good, strings.ToUpper(good), strings.TrimPrefix(good, "sha256:"), good + "0", "sha256:" + strings.Repeat("g", 64)} {
		if ValidFingerprint(value) != (value == good) {
			t.Fatal(value)
		}
	}
}
