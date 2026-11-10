package port_test

import (
	"strings"
	"testing"

	"github.com/donvargax/itos-container/internal/engine/session/port"
	f "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
)

func TestAnchorUpgradeRequiresCompleteIdentity(t *testing.T) {
	b, _, _ := f.Fixture()
	old := b.Intent(strings.Repeat("1", 64))
	old.Stage = "reserved"
	next := old
	next.Schema = 2
	next.Anchor = port.Anchor{Session: strings.Repeat("2", 64), Name: "itos-container-anchor-" + old.NameToken(), Source: "/proc/123/fd/9", Device: b.Checkout.Device, Inode: b.Checkout.Inode, Stage: "intent"}
	if !old.ValidTransition(next) {
		t.Fatal("valid anchor intent refused")
	}
	for name, change := range map[string]func(*port.Record){
		"session": func(r *port.Record) { r.Anchor.Session = "short" },
		"name":    func(r *port.Record) { r.Anchor.Name = "foreign" },
		"source":  func(r *port.Record) { r.Anchor.Source = "/checkout" },
		"stage":   func(r *port.Record) { r.Anchor.Stage = "owned" },
		"created": func(r *port.Record) { r.Anchor.Created = "already created" },
	} {
		t.Run(name, func(t *testing.T) {
			invalid := next
			change(&invalid)
			if old.ValidTransition(invalid) {
				t.Fatal("anchor upgrade accepted incomplete intent authority", invalid.Anchor)
			}
		})
	}
}

func TestDescriptorPathRequiresCanonicalProcessAndDescriptor(t *testing.T) {
	for _, path := range []string{"/proc/1/fd/0", "/proc/123/fd/9"} {
		if !port.DescriptorPath(path) {
			t.Errorf("refused descriptor %q", path)
		}
	}
	for _, path := range []string{"", "/checkout", "proc/1/fd/0", "/proc/1/root/0", "/proc/1/fd/0/extra", "/proc/0/fd/1", "/proc/01/fd/1", "/proc/1/fd/01", "/proc/-1/fd/1", "/proc/1/fd/-1", "/proc/4294967296/fd/1", "/proc/1/fd/4294967296"} {
		if port.DescriptorPath(path) {
			t.Errorf("accepted noncanonical descriptor %q", path)
		}
	}
}

func TestAnchorTransitionCannotDropOrReplaceOwnedIdentity(t *testing.T) {
	r := port.Record{Schema: 2, Stage: "owned", ContainerID: strings.Repeat("c", 64), Anchor: port.Anchor{
		Session: strings.Repeat("a", 64), Name: "original", Source: "/proc/123/fd/9", Device: "2049", Inode: "12345", Created: "original creation", Stage: "owned",
	}}
	absent := r
	absent.Stage, absent.Anchor.Stage = "absent", "absent"
	if !r.ValidTransition(absent) {
		t.Fatal("valid anchor completion refused")
	}
	for name, change := range map[string]func(*port.Record){
		"legacy schema":    func(r *port.Record) { r.Schema = 1; r.Anchor = port.Anchor{} },
		"future schema":    func(r *port.Record) { r.Schema = 3 },
		"session":          func(r *port.Record) { r.Anchor.Session = strings.Repeat("b", 64) },
		"name":             func(r *port.Record) { r.Anchor.Name = "replacement" },
		"descriptor":       func(r *port.Record) { r.Anchor.Source = "/proc/456/fd/9" },
		"device":           func(r *port.Record) { r.Anchor.Device = "2050" },
		"inode":            func(r *port.Record) { r.Anchor.Inode = "12346" },
		"creation changed": func(r *port.Record) { r.Anchor.Created = "replacement creation" },
		"creation lost":    func(r *port.Record) { r.Anchor.Created = "" },
	} {
		t.Run(name, func(t *testing.T) {
			next := absent
			change(&next)
			if r.ValidTransition(next) {
				t.Fatal("completion replaced original anchor authority", next)
			}
		})
	}
}
