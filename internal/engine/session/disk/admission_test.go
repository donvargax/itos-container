package disk

import (
	"context"
	"strings"
	"testing"

	ap "github.com/donvargax/itos-container/internal/engine/approval/port"
	af "github.com/donvargax/itos-container/internal/engine/approval/port/porttest"
	sp "github.com/donvargax/itos-container/internal/engine/session/port"
)

func TestPendingAdmissionRequiresPublicRegistrationToBlock(t *testing.T) {
	for _, state := range []string{"no directory", "no record", "registered", "corrupt", "legacy"} {
		t.Run(state, func(t *testing.T) {
			m := newRegistryFixture(t)
			claim := sp.AdmissionRecord{Schema: 1, Root: m.binding.Checkout.Root, ID: m.record.ID, Intent: m.record.Intent, Pending: state != "legacy"}
			name := admissionName(registryKey(claim.Root, m.binding.UID), claim.ID, m.binding.UID)
			m.files.Admissions = map[string][]byte{name: m.codec.EncodeAdmission(claim)}
			want := sp.Outcome("")
			if state != "no directory" && state != "legacy" {
				files := m.seed()
				switch state {
				case "no record":
					files.Data = nil
				case "registered":
					want = sp.CheckoutOverlap
				case "corrupt":
					files.Data = []byte("invalid")
					want = sp.Invalid
				}
			}
			if state == "legacy" {
				want = sp.CheckoutOverlap
			}
			c := ap.Candidate{UID: m.binding.UID, GID: m.binding.GID, Root: claim.Root + "/child"}
			d, failure := m.files.Admission(t.Context(), c)
			if failure != "" {
				t.Fatal(failure)
			}
			defer d.Close()
			if got := m.registry.checkAdmission(t.Context(), c, d); got != want {
				t.Fatalf("admission = %s, want %s", got, want)
			}
		})
	}
}

type admissionFaultStorage struct {
	ap.RegistryStorage
	fault     string
	afterOpen func()
	codec     sp.PublicCodec
}

func (s admissionFaultStorage) Admission(ctx context.Context, c ap.Candidate) (ap.AdmissionDirectory, ap.Failure) {
	if s.fault == "open" {
		return nil, "state.unavailable"
	}
	d, f := s.RegistryStorage.Admission(ctx, c)
	if f != "" {
		return nil, f
	}
	if s.afterOpen != nil {
		s.afterOpen()
	}
	return admissionFaultDirectory{AdmissionDirectory: d, fault: s.fault, codec: s.codec}, ""
}

type admissionFaultDirectory struct {
	ap.AdmissionDirectory
	fault string
	codec sp.PublicCodec
}

func (d admissionFaultDirectory) Names() ([]string, ap.Failure) {
	if d.fault == "names" {
		return nil, "state.unavailable"
	}
	return d.AdmissionDirectory.Names()
}
func (d admissionFaultDirectory) Read(name string) ([]byte, ap.Failure) {
	if d.fault == "read" {
		return nil, "state.unavailable"
	}
	return d.AdmissionDirectory.Read(name)
}
func (d admissionFaultDirectory) Publish(name string, old, next []byte) ap.Failure {
	if d.fault == "publish" {
		return "state.write"
	}
	if d.fault == "activation" {
		r, failure := d.codec.DecodeAdmission(next)
		if failure == "" && !r.Pending {
			return "state.write"
		}
	}
	return d.AdmissionDirectory.Publish(name, old, next)
}

func TestAdmissionActivationFailureRetainsRecoverableRegistrationAndClosesLease(t *testing.T) {
	m := newRegistryFixture(t)
	m.registry.Storage = admissionFaultStorage{RegistryStorage: m.files, fault: "activation", codec: m.codec}
	lease, failure := m.registry.Register(t.Context(), ap.Durable{Frozen: m.binding.Frozen}, m.record.ID, strings.Repeat("c", 64))
	if lease != nil || failure != sp.IntentWrite {
		t.Fatal("undurable activation granted a lease", failure)
	}
	index := registryKey(m.binding.Checkout.Root, m.binding.UID)
	files := m.files.Sessions[index+"/"+m.record.ID]
	if files == nil || files.Controller || len(files.Data) == 0 {
		t.Fatal("failed activation lost registration or retained its controller")
	}
	r, failure := m.codec.DecodeAdmission(m.files.Admissions[admissionName(index, m.record.ID, m.binding.UID)])
	if failure != "" || !r.Pending || r.Complete {
		t.Fatal("failed activation discarded pending evidence", r, failure)
	}
}
func (d admissionFaultDirectory) Sessions() ([]ap.RegisteredSession, ap.Failure) {
	if d.fault == "sessions" {
		return nil, "state.unavailable"
	}
	return d.AdmissionDirectory.Sessions()
}
func (d admissionFaultDirectory) Unchanged() bool {
	return d.fault != "changed" && d.AdmissionDirectory.Unchanged()
}

func TestAdmissionFailuresDoNotGrantRegistration(t *testing.T) {
	for _, fault := range []string{"open", "names", "read", "publish", "sessions", "changed", "invalid claim", "wrong claim name", "overlap", "invalid legacy", "wrong legacy ID", "missing legacy intent", "wrong legacy index"} {
		t.Run(fault, func(t *testing.T) {
			m := newRegistryFixture(t)
			m.registry.Storage = admissionFaultStorage{RegistryStorage: m.files, fault: fault}
			want := sp.Incomplete
			switch fault {
			case "open", "publish":
				want = sp.IntentWrite
			case "changed":
				want = sp.Unsafe
			case "invalid claim", "wrong claim name", "invalid legacy", "wrong legacy ID", "missing legacy intent", "wrong legacy index":
				want = sp.Invalid
			case "overlap":
				want = sp.CheckoutOverlap
			}
			if fault == "invalid claim" || fault == "wrong claim name" || fault == "overlap" {
				r := sp.AdmissionRecord{Schema: 1, Root: m.binding.Checkout.Root + "/child", ID: strings.Repeat("e", 64), Intent: m.record.Intent}
				name := admissionName(registryKey(r.Root, m.binding.UID), r.ID, m.binding.UID)
				data := m.codec.EncodeAdmission(r)
				if fault == "invalid claim" {
					data = []byte("invalid")
				}
				if fault == "wrong claim name" {
					name = strings.Repeat("0", 64) + ".json"
				}
				m.files.Admissions = map[string][]byte{name: data}
			}
			if strings.Contains(fault, "legacy") {
				files := m.seed()
				switch fault {
				case "invalid legacy":
					files.Data = []byte("invalid")
				case "wrong legacy ID":
					r := m.record
					r.ID = strings.Repeat("e", 64)
					m.codec.Records["other"] = r
					files.Data = []byte("other")
				case "missing legacy intent":
					delete(m.storage.Directories["intents/"+m.binding.Frozen.Project].Files, strings.Repeat("c", 64)+".json")
				case "wrong legacy index":
					delete(m.files.Sessions, registryKey(m.binding.Checkout.Root, m.binding.UID)+"/"+m.record.ID)
					m.files.Sessions[strings.Repeat("0", 64)+"/"+m.record.ID] = files
				}
			}
			lease, failure := m.registry.Register(t.Context(), ap.Durable{Frozen: m.binding.Frozen}, m.record.ID, strings.Repeat("c", 64))
			if lease != nil {
				lease.Close()
			}
			if lease != nil || failure != want {
				t.Fatal("unsafe reservation granted authority", failure, want)
			}
		})
	}
}
func TestAdmissionCompletionRequiresMatchingReservationAndDurability(t *testing.T) {
	for _, fault := range []string{"", "open", "read", "publish", "invalid claim", "different claim", "validation"} {
		t.Run(fault, func(t *testing.T) {
			m := newRegistryFixture(t)
			m.seed()
			r := m.binding.Intent(strings.Repeat("c", 64))
			r.Stage, r.ContainerID = "absent", strings.Repeat("e", 64)
			m.storage.Directories["intents/"+m.binding.Frozen.Project].Files[r.NameToken()+".json"] = m.registry.Store.Codec.Encode(r)
			entry, failure := m.registry.Open(t.Context(), m.record.ID, false)
			if failure != "" {
				t.Fatal(failure)
			}
			defer entry.Close()
			s := entry.(*publicSession)
			s.registry.Storage = admissionFaultStorage{RegistryStorage: m.files, fault: fault}
			claim := sp.AdmissionRecord{Schema: 1, Root: m.binding.Checkout.Root, ID: m.record.ID, Intent: m.record.Intent}
			name := admissionName(registryKey(claim.Root, m.binding.UID), claim.ID, m.binding.UID)
			if fault == "different claim" {
				claim.Intent = sp.Reference(m.record.Project, strings.Repeat("0", 64))
			}
			data := m.codec.EncodeAdmission(claim)
			if fault == "invalid claim" {
				data = []byte("invalid")
			}
			m.files.Admissions = map[string][]byte{name: data}
			stopped := m.record
			stopped.State = "stopped"
			m.codec.Records["stopped"] = stopped
			want := sp.Outcome("")
			switch fault {
			case "open", "publish":
				want = sp.IntentWrite
			case "read":
				want = sp.Incomplete
			case "invalid claim":
				want = sp.Invalid
			case "different claim", "validation":
				want = sp.Refused
			}
			if fault == "validation" {
				m.files.Sessions[registryKey(claim.Root, m.binding.UID)+"/"+claim.ID].Changed = true
			}
			if failure := s.Save(stopped); failure != want {
				t.Fatal(failure, want)
			}
			if fault == "" {
				completed, f := m.codec.DecodeAdmission(m.files.Admissions[name])
				if f != "" || !completed.Complete {
					t.Fatal("durable cleanup retained reservation", f)
				}
				if f := s.Sync(); f != "" {
					t.Fatal("completion retry", f)
				}
			}
		})
	}
}

func TestAdmissionScanPreservesLegacyRootsAndReadFailures(t *testing.T) {
	m := newRegistryFixture(t)
	m.seed()
	c := ap.Candidate{UID: m.binding.UID, GID: m.binding.GID, Root: m.binding.Checkout.Root + "/child", Project: m.binding.Frozen.Project}
	d, f := m.files.Admission(t.Context(), c)
	if f != "" {
		t.Fatal(f)
	}
	defer d.Close()
	if f := m.registry.checkAdmission(t.Context(), c, d); f != sp.CheckoutOverlap {
		t.Fatal("legacy nested root lost", f)
	}
	c.Root = m.binding.Checkout.Root + "-more"
	if f := m.registry.checkAdmission(t.Context(), c, d); f != "" {
		t.Fatal("legacy disjoint root refused", f)
	}
	claim := sp.AdmissionRecord{Schema: 1, Root: m.binding.Checkout.Root, ID: m.record.ID, Intent: m.record.Intent}
	name := admissionName(registryKey(claim.Root, m.binding.UID), claim.ID, m.binding.UID)
	m.files.Admissions[name] = m.codec.EncodeAdmission(claim)
	if f := m.registry.checkAdmission(t.Context(), c, admissionFaultDirectory{AdmissionDirectory: d, fault: "read"}); f != sp.Incomplete {
		t.Fatal("unreadable reservation ignored", f)
	}
}
func TestAdmissionCompletionRevalidatesAfterAcquiringGlobalLock(t *testing.T) {
	m := newRegistryFixture(t)
	files := m.seed()
	r := m.binding.Intent(strings.Repeat("c", 64))
	r.Stage, r.ContainerID = "absent", strings.Repeat("e", 64)
	m.storage.Directories["intents/"+m.binding.Frozen.Project].Files[r.NameToken()+".json"] = m.registry.Store.Codec.Encode(r)
	entry, f := m.registry.Open(t.Context(), m.record.ID, false)
	if f != "" {
		t.Fatal(f)
	}
	defer entry.Close()
	s := entry.(*publicSession)
	s.registry.Storage = admissionFaultStorage{RegistryStorage: m.files, afterOpen: func() { files.Changed = true }}
	stopped := m.record
	stopped.State = "stopped"
	m.codec.Records["stopped"] = stopped
	if f := s.Save(stopped); f != sp.Refused {
		t.Fatal("changed session released reservation", f)
	}
	if len(m.files.Admissions) != 0 {
		t.Fatal("changed session wrote completion")
	}
}

func TestAdmissionCertificateCannotHideChangedPublicAuthority(t *testing.T) {
	for _, fault := range []string{"intent", "state", "problem", "running stream timeout", "invalid"} {
		t.Run(fault, func(t *testing.T) {
			m := newRegistryFixture(t)
			files := m.seed()
			r := m.record
			r.State = "stopped"
			claim := sp.AdmissionRecord{Schema: 1, Root: m.binding.Checkout.Root, ID: r.ID, Intent: r.Intent, Complete: true}
			switch fault {
			case "intent":
				r.Intent = sp.Reference(r.Project, strings.Repeat("0", 64))
			case "state":
				r.State = "running"
			case "problem":
				problem := sp.Incomplete
				r.LastProblem = &problem
			case "running stream timeout":
				problem := sp.StreamTimeout
				r.State, r.LastProblem = "running", &problem
			}
			m.codec.Records["changed"] = r
			files.Data = []byte("changed")
			if fault == "invalid" {
				files.Data = []byte("invalid")
			}
			name := admissionName(registryKey(claim.Root, m.binding.UID), claim.ID, m.binding.UID)
			m.files.Admissions = map[string][]byte{name: m.codec.EncodeAdmission(claim)}
			c := ap.Candidate{UID: m.binding.UID, GID: m.binding.GID, Root: m.binding.Checkout.Root + "/child", Project: m.binding.Frozen.Project}
			d, f := m.files.Admission(t.Context(), c)
			if f != "" {
				t.Fatal(f)
			}
			defer d.Close()
			if f := m.registry.checkAdmission(t.Context(), c, d); f != sp.Invalid {
				t.Fatal("inconsistent completed authority admitted checkout", f)
			}
		})
	}
}

func (d admissionFaultDirectory) Sync(name string, expected []byte) ap.Failure {
	if d.fault == "sync" {
		return "state.write"
	}
	return d.AdmissionDirectory.Sync(name, expected)
}
func TestAdmissionRepeatedCompletionPreservesCertificateAndSyncFailure(t *testing.T) {
	m := newRegistryFixture(t)
	m.seed()
	r := m.binding.Intent(strings.Repeat("c", 64))
	r.Stage, r.ContainerID = "absent", strings.Repeat("e", 64)
	m.storage.Directories["intents/"+m.binding.Frozen.Project].Files[r.NameToken()+".json"] = m.registry.Store.Codec.Encode(r)
	entry, f := m.registry.Open(t.Context(), m.record.ID, false)
	if f != "" {
		t.Fatal(f)
	}
	defer entry.Close()
	s := entry.(*publicSession)
	stopped := m.record
	stopped.State = "stopped"
	m.codec.Records["stopped"] = stopped
	if f := s.Save(stopped); f != "" {
		t.Fatal(f)
	}
	// A retry must sync, so a publication-only fault cannot break it.
	s.registry.Storage = admissionFaultStorage{RegistryStorage: m.files, fault: "publish"}
	if f := s.Sync(); f != "" {
		t.Fatal("retry rewrote completed admission", f)
	}
	s.registry.Storage = admissionFaultStorage{RegistryStorage: m.files, fault: "sync"}
	if f := s.Sync(); f != sp.IntentWrite {
		t.Fatal("retry lost admission durability failure", f)
	}
}

// A legacy registration is read back through its original intent. Whatever
// that read reports is classified by the control-state vocabulary, never
// dropped and never turned into a silent success.
func TestReservationRootsClassifyLegacyIntentReadFailures(t *testing.T) {
	for _, tc := range []struct {
		name    string
		failure ap.Failure
		want    ap.Failure
	}{
		{"unsafe", "state.unsafe", "state.unsafe"},
		{"unreadable", "state.unavailable", "state.unavailable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := newRegistryFixture(t)
			m.seed()
			m.storage.Directories["intents/"+m.binding.Frozen.Project].ReadFailure = tc.failure
			c := ap.Candidate{UID: m.binding.UID, GID: m.binding.GID, Root: m.binding.Checkout.Root + "/child", Project: m.binding.Frozen.Project}
			d, failure := m.files.Admission(t.Context(), c)
			if failure != "" {
				t.Fatal(failure)
			}
			defer d.Close()
			if roots, got := m.registry.reservationRoots(t.Context(), c, d); got != tc.want || roots != nil {
				t.Fatalf("reservationRoots = %v, %q, want %q", roots, got, tc.want)
			}
		})
	}
}

// locationsFake serves the shared protected location port from the in-memory
// registry files; it does no I/O.
type locationsFake struct {
	files   *af.Registry
	storage ap.Storage
	failure ap.Failure
}

func (l locationsFake) At(string) ap.Storage { return l.storage }
func (l locationsFake) Inspect(ctx context.Context, c ap.Candidate, _ string) (ap.AdmissionDirectory, ap.Failure) {
	if l.failure != "" {
		return nil, l.failure
	}
	return l.files.Admission(ctx, c)
}

func TestReservationsRootsReportsTheProtectedLocationRefusalAndLegacyRoots(t *testing.T) {
	m := newRegistryFixture(t)
	m.seed()
	c := ap.Candidate{UID: m.binding.UID, GID: m.binding.GID, Root: m.binding.Checkout.Root + "/child", Project: m.binding.Frozen.Project}
	reservations := Reservations{Locations: locationsFake{files: m.files, storage: m.storage, failure: "state.path"}, Store: m.registry.Store, Codec: m.codec}
	if roots, failure := reservations.Roots(t.Context(), c, "/state/itos-container"); roots != nil || failure != "state.path" {
		t.Fatalf("refused location produced %v, %q", roots, failure)
	}
	reservations.Locations = locationsFake{files: m.files, storage: m.storage}
	roots, failure := reservations.Roots(t.Context(), c, "/state/itos-container")
	if failure != "" || len(roots) != 1 || roots[0] != m.binding.Checkout.Root {
		t.Fatalf("legacy registration roots = %v, %q", roots, failure)
	}
}
