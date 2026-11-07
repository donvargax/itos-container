package porttest

import (
	"testing"

	"github.com/donvargax/itos-container/internal/engine/readiness/port"
)

func TestEngineContract(t *testing.T) {
	EngineContract(t, func(s *Session) (port.Engine, string, string) { return &Engine{Session: s}, s.Address, "/checkout" })
}

func TestCheckoutContract(t *testing.T) {
	CheckoutContract(t, func() (port.Checkout, port.CheckoutIdentity, func()) {
		identity := port.CheckoutIdentity{Root: "/checkout", Device: "1", Inode: "2", GitDevice: "1", GitInode: "3"}
		c := &Checkout{Identity: identity}
		return c, identity, func() { c.Changed = true }
	})
}

func TestCheckoutIndependenceContract(t *testing.T) {
	CheckoutIndependenceContract(t, func() (port.Checkout, string, func(string)) {
		c := &Checkout{Identity: port.CheckoutIdentity{Root: "/checkout"}, LinkCounts: map[string]uint64{"nested/file": 1}}
		return c, c.Identity.Root, func(change string) {
			c.Unreadable = change == "unavailable"
			c.LinkCounts["nested/file"] = 1
			if change == "shared" {
				c.LinkCounts["nested/file"] = 2
			}
		}
	})
}
