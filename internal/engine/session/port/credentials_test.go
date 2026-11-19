package port_test

import (
	"github.com/donvargax/itos-container/internal/engine/session/port"
	f "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
	"strings"
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

func TestCredentialProgressIntentAbsence(t *testing.T) {
	r := f.CredentialIntent(port.CredentialKinds{Claude: true, GitHub: true})
	n := r
	n.Credentials.Stage = "absent"
	if !r.ValidTransition(n) {
		t.Fatal("absence before directory publication refused")
	}
	n.Credentials.Device, n.Credentials.Inode = "1", "2"
	if !n.Credentials.Valid() {
		t.Fatal("known absent directory fixture invalid")
	}
	if r.ValidTransition(n) {
		t.Fatal("absence introduced an unpublished directory identity")
	}
	n.Credentials.Stage = "owned"
	if !r.ValidTransition(n) {
		t.Fatal("directory identity publication refused")
	}
	absent := n
	absent.Credentials.Stage = "absent"
	if !n.ValidTransition(absent) {
		t.Fatal("absence preserving the published directory identity refused")
	}
}

func TestCredentialProgressCancellation(t *testing.T) {
	r := f.CredentialIntent(port.CredentialKinds{Claude: true})
	r.Credentials.Stage, r.Anchor.Stage = "absent", "absent"
	if r.Completed() {
		t.Fatal("reserved worker was completed before cancellation")
	}
	cancelled := r
	cancelled.Stage = "cancelled"
	if !r.ValidTransition(cancelled) || !cancelled.Completed() {
		t.Fatal("cancellation after confirmed staging and anchor removal was refused")
	}
	if !cancelled.ValidTransition(cancelled) || cancelled.ValidTransition(r) {
		t.Fatal("confirmed cancellation was not terminal and idempotent")
	}
}

func TestCredentialProgressFileIdentities(t *testing.T) {
	r := f.CredentialIntent(port.CredentialKinds{Claude: true, GitHub: true})
	r.Credentials.Stage, r.Credentials.Device, r.Credentials.Inode = "owned", "1", "2"
	for index, identity := range [2]string{"1:3", "1:4"} {
		n := r
		n.Credentials.Files[index] = identity
		if !r.ValidTransition(n) {
			t.Fatal("staged file identity publication refused", index)
		}
		r = n
		if !r.ValidTransition(r) {
			t.Fatal("unchanged published file identity refused", index)
		}
		for _, replacement := range []string{"", "1:9"} {
			bad := r
			bad.Credentials.Files[index] = replacement
			if r.ValidTransition(bad) {
				t.Fatal("published file identity was removed or replaced", index)
			}
		}
	}
	n := r
	n.Credentials.Stage = "absent"
	if !r.ValidTransition(n) {
		t.Fatal("staging absence preserving file identities refused")
	}
	if !n.ValidTransition(n) {
		t.Fatal("confirmed staging absence is not idempotent")
	}
}

func TestCredentialProgressAnchorPublication(t *testing.T) {
	r := f.CredentialIntent(port.CredentialKinds{Claude: true})
	n := r
	r.Anchor.Stage, r.Anchor.Created = "intent", ""
	if !r.ValidTransition(n) {
		t.Fatal("anchor ownership publication refused during credential preparation")
	}
	n.Stage = "cancelled"
	if r.ValidTransition(n) {
		t.Fatal("anchor ownership publication changed the worker stage")
	}
}

func TestCredentialProgressAnchorCleanup(t *testing.T) {
	for _, stage := range []string{"reserved", "cancelled", "absent"} {
		t.Run(stage, func(t *testing.T) {
			r := f.CredentialIntent(port.CredentialKinds{Claude: true})
			r.Stage, r.Credentials.Stage = stage, "absent"
			if stage == "absent" {
				r.ContainerID = strings.Repeat("c", 64)
			}
			n := r
			n.Anchor.Stage = "absent"
			if !r.ValidTransition(n) {
				t.Fatal("confirmed anchor removal refused after staging absence")
			}
			if n.Completed() != (stage != "reserved") {
				t.Fatal("completion did not retain the worker's preparation or termination state")
			}
			if !n.ValidTransition(n) || n.ValidTransition(r) {
				t.Fatal("anchor absence is not terminal and idempotent")
			}
		})
	}
}

func TestCredentialProgressSchemaUpgrade(t *testing.T) {
	for _, egress := range []bool{false, true} {
		r := f.CredentialIntent(port.CredentialKinds{Claude: true})
		staging := r.Credentials
		r.Schema, r.Credentials = 2, port.CredentialStaging{}
		if egress {
			r.Schema = 3
			r.Egress = ownedCredentialEgress(r)
		}
		n := r
		n.Schema, n.Credentials = 4, staging
		if !r.ValidTransition(n) {
			t.Fatal("credential intent upgrade refused", egress)
		}
		for _, fault := range []string{"owned staging", "worker advanced", "anchor changed", "already cancelled"} {
			t.Run(fault+"/"+r.Egress.Proxy.Stage, func(t *testing.T) {
				before, after := r, n
				switch fault {
				case "owned staging":
					after.Credentials.Stage, after.Credentials.Device, after.Credentials.Inode = "owned", "1", "2"
				case "worker advanced":
					after.Stage = "cancelled"
				case "anchor changed":
					after.Anchor.Stage = "absent"
				case "already cancelled":
					before.Stage, after.Stage = "cancelled", "cancelled"
				}
				if before.ValidTransition(after) {
					t.Fatal("schema upgrade changed progress or bypassed staging intent")
				}
			})
		}
	}
}

func TestCredentialProgressWithEgress(t *testing.T) {
	r := f.CredentialIntent(port.CredentialKinds{Claude: true, GitHub: true})
	r.Egress = ownedCredentialEgress(r)
	if !r.ValidTransition(r) {
		t.Fatal("unchanged credential and egress authority refused")
	}
	n := r
	n.Credentials.Stage, n.Credentials.Device, n.Credentials.Inode = "owned", "1", "2"
	if !r.ValidTransition(n) {
		t.Fatal("credential publication lost existing egress authority")
	}
	r = n
	n.Credentials.Stage = "absent"
	if !r.ValidTransition(n) {
		t.Fatal("staging cleanup with owned egress refused")
	}
	r = n
	r.Stage = "cancelled"
	for _, resource := range []string{"proxy", "network", "internal"} {
		n = r
		n.Anchor.Stage = "absent"
		if r.ValidTransition(n) || r.Completed() {
			t.Fatal("live egress allowed anchor removal or session completion")
		}
		n = r
		switch resource {
		case "proxy":
			n.Egress.Proxy.Stage = "absent"
		case "network":
			n.Egress.Network.Stage = "absent"
		case "internal":
			n.Egress.Internal.Stage = "absent"
		}
		if !r.ValidTransition(n) {
			t.Fatal("confirmed egress cleanup refused with credential metadata", resource)
		}
		r = n
	}
	n = r
	n.Anchor.Stage = "absent"
	if !r.ValidTransition(n) || !n.Completed() {
		t.Fatal("confirmed removal of every owned resource did not complete the session")
	}
}

func ownedCredentialEgress(r port.Record) port.EgressRecord {
	return port.EgressRecord{
		Internal: port.EgressResource{Name: "itos-container-internal-" + r.NameToken(), ID: strings.Repeat("a", 64), Stage: "owned"},
		Network:  port.EgressResource{Name: "itos-container-egress-" + r.NameToken(), ID: strings.Repeat("b", 64), Stage: "owned"},
		Proxy:    port.EgressResource{Name: "itos-container-proxy-" + r.NameToken(), ID: strings.Repeat("c", 64), Stage: "owned"},
	}
}

func TestCredentialProgressEgressCleanupBeforeLaunch(t *testing.T) {
	r := f.CredentialIntent(port.CredentialKinds{Claude: true})
	r.Credentials.Stage = "absent"
	r.Egress = ownedCredentialEgress(r)
	for _, resource := range []string{"proxy", "network", "internal"} {
		n := r
		switch resource {
		case "proxy":
			n.Egress.Proxy.Stage = "absent"
		case "network":
			n.Egress.Network.Stage = "absent"
		case "internal":
			n.Egress.Internal.Stage = "absent"
		}
		if !r.ValidTransition(n) || n.Completed() {
			t.Fatal("pre-launch egress cleanup refused or prematurely completed the session", resource)
		}
		r = n
	}
	n := r
	n.Anchor.Stage = "absent"
	if !r.ValidTransition(n) || n.Completed() {
		t.Fatal("pre-launch anchor cleanup lost the pending cancellation")
	}
	r = n
	n.Stage = "cancelled"
	if !r.ValidTransition(n) || !n.Completed() {
		t.Fatal("confirmed pre-launch cleanup did not permit terminal cancellation")
	}
}

func TestCredentialProgressRefusesMalformedEgressOrigin(t *testing.T) {
	for _, schema := range []int{3, 4} {
		r := f.CredentialIntent(port.CredentialKinds{Claude: true})
		r.Schema = schema
		if schema == 3 {
			r.Credentials = port.CredentialStaging{}
		}
		r.Egress = ownedCredentialEgress(r)
		r.Egress.Internal.Stage, r.Egress.Internal.ID = "intent", ""
		r.Egress.Network.Stage, r.Egress.Network.ID = "", ""
		r.Egress.Proxy.Stage, r.Egress.Proxy.ID = "", ""
		n := r
		n.Egress.Internal.Stage, n.Egress.Internal.ID = "owned", strings.Repeat("a", 64)
		if !r.ValidTransition(n) {
			t.Fatal("valid egress ownership publication refused", schema)
		}
		// An intent cannot already claim an engine ID. A later well-formed
		// observation must not legitimize corrupt original recovery evidence.
		r.Egress.Internal.ID = strings.Repeat("b", 64)
		if r.ValidTransition(n) {
			t.Fatal("malformed original egress authority was accepted", schema)
		}
	}
}
