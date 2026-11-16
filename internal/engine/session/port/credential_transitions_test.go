package port

import (
	"strings"
	"testing"
)

func TestCredentialTransitionRefusals(t *testing.T) {
	r := Record{Schema: 4, Stage: "reserved", Credentials: CredentialStaging{
		Directory: "/dev/shm/itos-container-" + strings.Repeat("a", 32),
		Stage:     "owned", Device: "1", Inode: "2", Kinds: CredentialKinds{Claude: true, GitHub: true},
	}}
	for _, fault := range []string{"schema", "invalid staging", "running file publication", "two files together"} {
		t.Run(fault, func(t *testing.T) {
			before, after := r, r
			switch fault {
			case "schema":
				after.Schema = 3
			case "invalid staging":
				after.Credentials.Directory = "/foreign"
			case "running file publication":
				before.Stage, after.Stage = "owned", "owned"
				after.Credentials.Files[0] = "1:3"
			case "two files together":
				after.Credentials.Files = [2]string{"1:3", "1:4"}
			}
			if before.credentialTransition(after) {
				t.Fatal("credential transition admitted a schema, authority or publication-order change")
			}
		})
	}
	invalid := r.Credentials
	invalid.Files[0] = "not-an-identity"
	if invalid.Valid() {
		t.Fatal("staging accepted a malformed file identity")
	}
	if CredentialTarget("unknown") != "" {
		t.Fatal("unknown credential kind acquired a mount target")
	}
}

func TestCredentialMountOwnershipClosedKinds(t *testing.T) {
	for _, kinds := range []CredentialKinds{{}, {Claude: true}, {GitHub: true}, {Claude: true, GitHub: true}} {
		r := Record{Schema: 4, Credentials: CredentialStaging{Directory: "/dev/shm/itos-container-" + strings.Repeat("a", 32), Kinds: kinds}}
		observation := Observation{CredentialExact: true}
		if kinds.Claude {
			observation.CredentialMounts[0] = CredentialMount{"claude-token", r.Credentials.Directory + "/claude-token", "/run/itos-container/credentials/claude-token"}
		}
		if kinds.GitHub {
			observation.CredentialMounts[1] = CredentialMount{"github-token", r.Credentials.Directory + "/github-token", "/run/itos-container/credentials/github-token"}
		}
		if !r.OwnsCredentialMounts(observation) {
			t.Fatal("exact mounts for the closed kind set lost ownership")
		}
		observation.CredentialMounts[0].Source = "/foreign"
		if r.OwnsCredentialMounts(observation) {
			t.Fatal("foreign or unexpected credential mount acquired ownership")
		}
	}
}
