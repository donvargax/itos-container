package porttest

import (
	"context"
	"strings"
	"time"

	"github.com/donvargax/itos-container/internal/engine/approval/port"
)

func AdmissionStorageContract(t Reporter, open func() (port.RegistryStorage, port.Candidate)) {
	t.Helper()
	s, c := open()
	ctx := context.Background()
	d, failure := s.Admission(ctx, c)
	if failure != "" {
		t.Errorf("admission lock: %s", failure)
		return
	}
	name := strings.Repeat("a", 64) + ".json"
	if names, failure := d.Names(); failure != "" || len(names) != 0 {
		t.Errorf("empty reservations: %v %s", names, failure)
	}
	if records, failure := d.Sessions(); failure != "" || len(records) != 0 {
		t.Errorf("empty registrations: %v %s", records, failure)
	}
	other := c
	other.Project = strings.Repeat("b", 64)
	deadline, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
	if contender, failure := s.Admission(deadline, other); failure != "state.unavailable" || contender != nil {
		t.Errorf("other project bypassed held admission lock: %s", failure)
	}
	cancel()
	if failure := d.Publish(name, nil, []byte("reserved")); failure != "" {
		t.Errorf("reservation publication: %s", failure)
	}
	data, failure := d.Read(name)
	if failure != "" || string(data) != "reserved" {
		t.Errorf("reservation read: %q %s", data, failure)
	}
	if len(data) > 0 {
		data[0] = 'X'
	}
	if failure := d.Sync(name, []byte("reserved")); failure != "" {
		t.Errorf("reservation sync: %s", failure)
	}
	if failure := d.Publish(name, []byte("stale"), []byte("released")); failure != "state.conflict" {
		t.Errorf("stale reservation overwrite: %s", failure)
	}
	d.Close()
	d, failure = s.Admission(ctx, other)
	if failure != "" {
		t.Errorf("released admission lock: %s", failure)
		return
	}
	defer d.Close()
	if names, failure := d.Names(); failure != "" || len(names) != 1 || names[0] != name {
		t.Errorf("cross-project reservation discovery: %v %s", names, failure)
	}
	id := strings.Repeat("c", 64)
	lease, failure := s.Register(ctx, c, id, []byte("public"))
	if failure != "" {
		t.Errorf("registration: %s", failure)
		return
	}
	defer lease.Close()
	session, failure := s.Open(ctx, c, id, false)
	if failure != "" {
		t.Errorf("operation lock: %s", failure)
		return
	}
	defer session.Close()
	// Scanning never waits for a session operation that can span engine calls.
	records, failure := d.Sessions()
	if failure != "" || len(records) != 1 || records[0].Index != c.Project || records[0].ID != id || string(records[0].Data) != "public" {
		t.Errorf("unlocked registration scan: %v %s", records, failure)
	}
}
