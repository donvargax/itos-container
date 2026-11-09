package disk

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/donvargax/itos-container/internal/engine/approval/port"
)

// A temporary as Publish names one: ".intent-" and a 128-bit hex nonce.
const leftoverTemporary = ".intent-00000000000000000000000000000000.tmp"

func claimName() string { return strings.Repeat("a", 64) + ".json" }

// publishedClaim publishes one admission claim and returns its directory.
func publishedClaim(t *testing.T, store Store, c port.Candidate) (*admissionDirectory, string) {
	t.Helper()
	d, failure := (Registry{Store: store}).Admission(t.Context(), c)
	if failure != "" {
		t.Fatal(failure)
	}
	t.Cleanup(d.Close)
	if failure = d.Publish(claimName(), nil, []byte("reservation")); failure != "" {
		t.Fatal(failure)
	}
	real := d.(*admissionDirectory)
	return real, filepath.Join(real.pin.path, claimName())
}

// Publish links a new record to its final name and then removes the temporary.
// A publisher killed between the two leaves the record with two links, and the
// scan reads the record of every enrolled location in every control transaction.
// A leftover that is the publisher's own temporary, beside the record in its
// pinned private directory, is the state after the removal that did not run: it
// is read, and left where it is. Names has always skipped such a temporary.
func TestReadAcceptsRecordWhoseTemporaryOutlivedItsPublisher(t *testing.T) {
	t.Run("admission claim", func(t *testing.T) {
		store, c, _ := diskFixture(t)
		d, path := publishedClaim(t, store, c)
		leftover := filepath.Join(d.pin.path, leftoverTemporary)
		if err := os.Link(path, leftover); err != nil {
			t.Fatal(err)
		}
		data, failure := d.Read(claimName())
		if failure != "" || string(data) != "reservation" {
			t.Fatalf("read = %q, %q; want the claim", data, failure)
		}
		names, failure := d.Names()
		if failure != "" || len(names) != 1 || names[0] != claimName() {
			t.Fatalf("names = %v, %q; want only the claim", names, failure)
		}
		for _, p := range []string{path, leftover} {
			if _, err := os.Lstat(p); err != nil {
				t.Fatalf("reading removed %s: %v", p, err)
			}
		}
	})
	t.Run("session record", func(t *testing.T) {
		store, c, _ := diskFixture(t)
		d, _ := publishedClaim(t, store, c)
		session := filepath.Join(d.sessions, c.Project, strings.Repeat("b", 64))
		if err := os.MkdirAll(session, 0700); err != nil {
			t.Fatal(err)
		}
		record := filepath.Join(session, "record.json")
		if err := os.WriteFile(record, []byte("public"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Link(record, filepath.Join(session, leftoverTemporary)); err != nil {
			t.Fatal(err)
		}
		sessions, failure := d.Sessions()
		if failure != "" || len(sessions) != 1 || string(sessions[0].Data) != "public" {
			t.Fatalf("sessions = %v, %q; want the record", sessions, failure)
		}
	})
}

// Only that exact leftover is tolerated. Every other second link keeps the
// record unsafe: a name the publisher never makes, a temporary that is another
// file, a further link beside the temporary, and a temporary that cannot be listed.
func TestReadStillRefusesEveryOtherSecondLink(t *testing.T) {
	for _, kind := range []string{"other name", "dead publisher's name", "temporary of another file", "third link", "directory cannot be listed"} {
		t.Run(kind, func(t *testing.T) {
			store, c, _ := diskFixture(t)
			d, path := publishedClaim(t, store, c)
			dir := d.pin.path
			switch kind {
			case "other name":
				mustLink(t, path, filepath.Join(dir, "linked"))
			case "dead publisher's name":
				mustLink(t, path, filepath.Join(dir, ".intent-dead.tmp"))
			case "temporary of another file":
				mustLink(t, path, filepath.Join(dir, "linked"))
				if err := os.WriteFile(filepath.Join(dir, leftoverTemporary), []byte("reservation"), 0600); err != nil {
					t.Fatal(err)
				}
			case "third link":
				mustLink(t, path, filepath.Join(dir, leftoverTemporary))
				mustLink(t, path, filepath.Join(filepath.Dir(dir), "outside"))
			case "directory cannot be listed":
				mustLink(t, path, filepath.Join(dir, leftoverTemporary))
				fault := errors.New("fixture directory failure")
				d.pin.ops.openDir = func(*os.Root, string) (*os.File, error) { return nil, fault }
			}
			if data, failure := d.Read(claimName()); failure != "state.unsafe" || data != nil {
				t.Fatalf("read = %q, %q; want state.unsafe", data, failure)
			}
		})
	}
}

func mustLink(t *testing.T, oldname, newname string) {
	t.Helper()
	if err := os.Link(oldname, newname); err != nil {
		t.Fatal(err)
	}
}

// admissionScan is the part of the session engine's reservation scan that reads
// a location's admission directory, over the adapter under test.
type admissionScan struct{ locations Locations }

func (s admissionScan) Roots(ctx context.Context, c port.Candidate, path string) ([]string, port.Failure) {
	d, failure := s.locations.Inspect(ctx, c, path)
	if failure != "" {
		return nil, failure
	}
	defer d.Close()
	names, failure := d.Names()
	if failure != "" {
		return nil, failure
	}
	for _, name := range names {
		if _, failure = d.Read(name); failure != "" {
			return nil, failure
		}
	}
	return nil, ""
}

// The account was locked out: the scan refused the enrolled location's claim,
// and so every command on the account, whichever state it selected.
func TestControlBeginSurvivesTheTemporaryOfAKilledPublisher(t *testing.T) {
	for _, write := range []bool{false, true} {
		name := map[bool]string{false: "reader", true: "writer"}[write]
		t.Run(name, func(t *testing.T) {
			control, c, _, _, _ := bootstrapped(t)
			control.Reservations = admissionScan{Locations{Store: control.Store}}
			d, path := publishedClaim(t, control.Store, c)
			mustLink(t, path, filepath.Join(d.pin.path, leftoverTemporary))
			d.Close()
			tx, failure := control.Begin(t.Context(), c, write)
			if tx != nil {
				tx.Close()
			}
			if tx == nil || failure != "" {
				t.Fatalf("Begin = %v, %q; want a transaction", tx, failure)
			}
		})
	}
}
