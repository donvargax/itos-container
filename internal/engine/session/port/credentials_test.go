package port_test

import (
	"github.com/donvargax/itos-container/internal/engine/session/port"
	f "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
	"testing"
)

func TestCredentialProgress(t *testing.T) {
	r := f.CredentialIntent(port.CredentialKinds{Claude: true, GitHub: true})
	if !r.Credentials.Valid() {
		t.Fatal("valid staging intent refused")
	}
	n := r
	n.Credentials.Device = "1"
	n.Credentials.Inode = "2"
	n.Credentials.Stage = "owned"
	if !r.ValidTransition(n) {
		t.Fatal("directory ownership publication refused")
	}
	for _, change := range []func(*port.Record){func(n *port.Record) { n.Credentials.Directory += "x" }, func(n *port.Record) { n.Credentials.Kinds.GitHub = false }, func(n *port.Record) { n.Project = "foreign" }} {
		bad := n
		change(&bad)
		if n.ValidTransition(bad) {
			t.Fatal("credential authority changed")
		}
	}
	r = n
	n.Credentials.Stage = "absent"
	if !r.ValidTransition(n) {
		t.Fatal("confirmed staging absence refused")
	}
	r = n
	n.Credentials.Stage = "owned"
	if r.ValidTransition(n) {
		t.Fatal("absent staging revived")
	}
	r.Stage = "absent"
	r.ContainerID = "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	r.Anchor.Stage = "absent"
	if !r.Completed() {
		t.Fatal("confirmed completion refused")
	}
	r.Credentials.Stage = "owned"
	if r.Completed() {
		t.Fatal("live staging counted as complete")
	}
}
