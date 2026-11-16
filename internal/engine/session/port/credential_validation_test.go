package port_test

import (
	"strconv"
	"testing"

	"github.com/donvargax/itos-container/internal/engine/session/port"
	f "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
)

func TestCredentialStagingValidation(t *testing.T) {
	for _, kinds := range []port.CredentialKinds{{Claude: true}, {GitHub: true}, {Claude: true, GitHub: true}} {
		for _, kind := range kinds.List() {
			t.Run(strconv.Itoa(len(kinds.List()))+"/"+kind, func(t *testing.T) {
				staging := f.CredentialIntent(kinds).Credentials
				staging.Stage, staging.Device, staging.Inode = "owned", "1", "2"
				index := port.CredentialIndex(kind)
				for _, identity := range []string{"1:3", "18446744073709551615:18446744073709551615"} {
					staging.Files[index] = identity
					if !staging.Valid() {
						t.Fatal("canonical file identity refused")
					}
				}
				for _, identity := range []string{"0:3", "1:0", "01:3", "1:03", ":3", "1:", "-1:3", "1:-3", "+1:3", "1:+3", "x:3", "1:x", "1:2:3", "1", "18446744073709551616:3", "1:18446744073709551616"} {
					staging.Files[index] = identity
					if staging.Valid() {
						t.Errorf("invalid file identity accepted: %q", identity)
					}
				}
			})
		}
	}
	for _, selected := range []port.CredentialKinds{{Claude: true}, {GitHub: true}} {
		staging := f.CredentialIntent(selected).Credentials
		staging.Stage, staging.Device, staging.Inode = "owned", "1", "2"
		staging.Files = [2]string{"1:3", "1:4"}
		if staging.Valid() {
			t.Fatal("file authority for an unselected kind accepted")
		}
	}
}

func TestCredentialStagingReady(t *testing.T) {
	if (port.CredentialStaging{}).Ready() {
		t.Fatal("empty staging is ready")
	}
	for _, kinds := range []port.CredentialKinds{{Claude: true}, {GitHub: true}, {Claude: true, GitHub: true}} {
		staging := f.CredentialIntent(kinds).Credentials
		if staging.Ready() {
			t.Fatal("unpublished staging is ready")
		}
		staging.Stage, staging.Device, staging.Inode = "owned", "1", "2"
		if staging.Ready() {
			t.Fatal("directory without credential files is ready")
		}
		for _, kind := range kinds.List() {
			staging.Files[port.CredentialIndex(kind)] = "1:3"
		}
		if !staging.Ready() {
			t.Fatal("complete owned staging is not ready")
		}
		for _, kind := range kinds.List() {
			partial := staging
			partial.Files[port.CredentialIndex(kind)] = ""
			if partial.Ready() {
				t.Fatal("staging missing a selected file is ready", kind)
			}
		}
		invalid := staging
		invalid.Device = "0"
		if invalid.Ready() {
			t.Fatal("invalid staging is ready")
		}
		staging.Stage = "absent"
		if staging.Ready() {
			t.Fatal("removed staging is ready")
		}
	}
}

func TestCredentialRecordBinding(t *testing.T) {
	kinds := []port.CredentialKinds{{}, {Claude: true}, {GitHub: true}, {Claude: true, GitHub: true}}
	for _, selected := range kinds[1:] {
		r := f.CredentialIntent(selected)
		for _, granted := range kinds {
			if got := r.ValidCredentials(port.Binding{Credentials: granted}); got != (selected == granted) {
				t.Fatal("credential record did not require the exact frozen grants")
			}
		}
		r.Credentials.Directory = "/foreign"
		if r.ValidCredentials(port.Binding{Credentials: selected}) {
			t.Fatal("matching grants admitted malformed staging")
		}
	}
	for _, schema := range []int{1, 2, 3} {
		for _, stage := range []string{"reserved", "cancelled", "intent", "owned", "absent"} {
			r := port.Record{Schema: schema, Stage: stage}
			if !r.ValidCredentials(port.Binding{}) {
				t.Fatal("legacy no-grant record refused")
			}
			want := stage == "reserved" || stage == "cancelled"
			if got := r.ValidCredentials(port.Binding{Credentials: kinds[1]}); got != want {
				t.Fatal("legacy record admitted grants beyond preparation or cancellation", schema, stage)
			}
			r.Credentials = f.CredentialIntent(kinds[1]).Credentials
			if r.ValidCredentials(port.Binding{}) || r.CredentialShape() {
				t.Fatal("legacy schema accepted credential metadata")
			}
		}
	}
}

func TestCredentialRecordWorkerShape(t *testing.T) {
	for _, workerStage := range []string{"intent", "owned"} {
		for _, kinds := range []port.CredentialKinds{{Claude: true}, {GitHub: true}, {Claude: true, GitHub: true}} {
			r := f.CredentialIntent(kinds)
			r.Stage = workerStage
			if r.CredentialShape() {
				t.Fatal("worker accepted unpublished staging", workerStage)
			}
			r.Credentials.Stage, r.Credentials.Device, r.Credentials.Inode = "owned", "1", "2"
			for _, kind := range kinds.List() {
				r.Credentials.Files[port.CredentialIndex(kind)] = "1:3"
			}
			if !r.CredentialShape() {
				t.Fatal("worker with complete staging refused", workerStage)
			}
			for _, kind := range kinds.List() {
				partial := r
				partial.Credentials.Files[port.CredentialIndex(kind)] = ""
				if partial.CredentialShape() {
					t.Fatal("worker accepted incomplete staging", workerStage, kind)
				}
			}
			r.Credentials.Stage = "absent"
			if !r.CredentialShape() {
				t.Fatal("unlink discarded the worker's acquired credential authority", workerStage)
			}
		}
	}
}

func TestCredentialRecordAnchorBinding(t *testing.T) {
	b, _, _ := f.Fixture()
	r := f.CredentialIntent(port.CredentialKinds{Claude: true})
	if !r.ValidAnchor(b) {
		t.Fatal("credential record lost its original anchor binding")
	}
	for _, field := range []string{"session", "name", "source", "device", "inode", "created", "stage"} {
		t.Run(field, func(t *testing.T) {
			bad := r
			switch field {
			case "session":
				bad.Anchor.Session = "foreign"
			case "name":
				bad.Anchor.Name += "foreign"
			case "source":
				bad.Anchor.Source = "/original/checkout"
			case "device":
				bad.Anchor.Device += "9"
			case "inode":
				bad.Anchor.Inode += "9"
			case "created":
				bad.Anchor.Created = ""
			case "stage":
				bad.Anchor.Stage = "unknown"
			}
			if bad.ValidAnchor(b) {
				t.Fatal("credential record accepted changed or invalid anchor authority")
			}
		})
	}
}

func TestCredentialCompletionRefusesUnknownSchemaProgress(t *testing.T) {
	b, _, _ := f.Fixture()
	r := f.RunIntent(b)
	r.Stage = "cancelled"
	n := r
	n.Anchor.Stage = "absent"
	if !r.ValidTransition(n) {
		t.Fatal("known schema refused terminal anchor cleanup")
	}
	for _, schema := range []int{0, -1} {
		r.Schema, n.Schema = schema, schema
		if r.ValidTransition(n) {
			t.Fatal("unknown legacy schema acquired terminal cleanup progress", schema)
		}
	}
}
