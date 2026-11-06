package features

import (
	"strings"
	"testing"
)

func TestOfflineOwnedDurabilityIncludesResync(t *testing.T) {
	w := newSessionHarnessWorld(t)
	id := strings.Repeat("a", 64)
	data := string(approvalJSON(w.sessionExpectedRecord("owned", id)))
	event := func(operation string) sessionEvent {
		return sessionEvent{Operation: operation, Stage: "owned", Bytes: data}
	}
	publication := []sessionEvent{event("write"), event("file fsync"), event("atomic publication"), event("parent fsync")}
	resynced := append(append([]sessionEvent{}, publication...), event("file fsync"), event("parent fsync"))
	for _, valid := range [][]sessionEvent{publication, resynced} {
		if err := w.offlineOwnedDurability(valid, id); err != nil {
			t.Fatal(err)
		}
	}
	for i := range publication {
		broken := append([]sessionEvent{}, publication...)
		broken[i].Failed = true
		if w.offlineOwnedDurability(broken, id) == nil {
			t.Fatalf("accepted failed %s", broken[i].Operation)
		}
	}
	for i := range resynced {
		if i < len(publication) {
			continue
		}
		broken := append([]sessionEvent{}, resynced...)
		broken[i].Failed = true
		if w.offlineOwnedDurability(broken, id) == nil {
			t.Fatalf("accepted failed re-sync %s", broken[i].Operation)
		}
	}
	if w.offlineOwnedDurability(resynced[:len(resynced)-1], id) == nil {
		t.Fatal("accepted incomplete re-sync")
	}
	reordered := append([]sessionEvent{}, publication...)
	reordered[1], reordered[2] = reordered[2], reordered[1]
	if w.offlineOwnedDurability(reordered, id) == nil {
		t.Fatal("accepted publication before file fsync")
	}
	wrong := append([]sessionEvent{}, publication...)
	wrong[0].Bytes = string(approvalJSON(w.sessionExpectedRecord("owned", strings.Repeat("b", 64))))
	if w.offlineOwnedDurability(wrong, id) == nil {
		t.Fatal("accepted a different resource binding")
	}
	if w.offlineOwnedDurability(nil, id) == nil {
		t.Fatal("accepted absent durability evidence")
	}
}
