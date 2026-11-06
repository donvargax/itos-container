package disk

import (
	"bytes"
	"testing"

	rp "github.com/donvargax/itos-container/internal/engine/readiness/port"
	rf "github.com/donvargax/itos-container/internal/engine/readiness/port/porttest"
	sp "github.com/donvargax/itos-container/internal/engine/session/port"
)

func TestRegistryLaunchRevalidatesCurrentCheckoutWithoutChangingRecovery(t *testing.T) {
	for _, fault := range []string{"root inode", "Git inode", "pin", "inspect", "changed"} {
		t.Run(fault, func(t *testing.T) {
			m := newRegistryFixture()
			files := m.seed()
			before := bytes.Clone(files.Data)
			checkout := m.registry.Store.Checkout.(*rf.Checkout)
			entry, failure := m.registry.Open(t.Context(), m.record.ID, false)
			if failure != "" {
				t.Fatal(failure)
			}
			defer entry.Close()
			if f := entry.ValidateLaunch(); f != "" || checkout.Opened {
				t.Fatal("approved launch failed or retained the checkout pin", f)
			}
			switch fault {
			case "root inode":
				checkout.Identity.Inode += "1"
			case "Git inode":
				checkout.Identity.GitInode += "1"
			case "pin":
				checkout.Failure = rp.CheckoutUnavailable
			case "inspect":
				checkout.InspectFailure = rp.CheckoutUnavailable
			case "changed":
				checkout.Changed = true
			}
			if f := entry.ValidateLaunch(); f != sp.Refused || checkout.Opened {
				t.Fatal("changed checkout authorized launch or leaked a pin", f)
			}
			if f := entry.Validate(); f != "" || checkout.Opened || !bytes.Equal(files.Data, before) || len(m.files.Events) != 0 {
				t.Fatal("launch refusal vetoed or altered recovery authority", f)
			}
			intent, f := m.registry.Store.OpenCleanup(t.Context(), m.record.Intent)
			if f != "" {
				t.Fatal("launch validation retained the intent lock", f)
			}
			intent.Close()
		})
	}
}
