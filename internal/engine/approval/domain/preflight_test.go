package domain

import (
	"github.com/donvargax/itos-container/internal/engine/approval/port"
	"github.com/donvargax/itos-container/internal/engine/approval/port/porttest"
	readiness "github.com/donvargax/itos-container/internal/engine/readiness/port"
	"testing"
)

func TestPreflightOrdersLocalApprovalBeforeFreshObservations(t *testing.T) {
	for _, condition := range []string{"matching", "missing", "invalid", "locally changed", "engine changed", "engine failure", "host unavailable", "root", "manifest mismatch", "socket unavailable", "local failure", "identity changes during inspection"} {
		t.Run(condition, func(t *testing.T) {
			service, ready, host, store := serviceFixture()
			codec := service.Codec.(*porttest.Codec)
			c := codec.Values["2000:3000"]
			store.Records = map[string]port.Record{c.Project: {Candidate: c, Generation: "7"}}
			wantStatus, wantFailure, engines := "matching", port.Failure(""), 1
			switch condition {
			case "missing":
				store.Records = nil
				wantStatus = "missing"
				engines = 0
			case "invalid":
				store.Failure = "state.invalid"
				wantStatus = "invalid"
				wantFailure = "state.invalid"
				engines = 0
			case "locally changed":
				codec.LocalMismatch = true
				codec.LocalPaths = []string{"/policy/network/allow"}
				wantStatus = "changed"
				engines = 0
			case "engine changed":
				codec.Local = c
				newCandidate := c
				newCandidate.Canonical = "B"
				codec.Values["2000:3000"] = newCandidate
				codec.Paths = []string{"/image/id"}
				wantStatus = "changed"
			case "engine failure":
				local := ready.Result
				ready.LocalResult = &local
				ready.Result.Ready = false
				ready.Result.Issues = []readiness.Issue{{Kind: readiness.ImageMissing}}
				wantStatus = "changed"
			case "host unavailable":
				host.GID = ""
				wantStatus = "missing"
				wantFailure = "identity.unavailable"
				engines = 0
			case "root":
				host.UID = "0"
				wantStatus = "missing"
				wantFailure = "identity.root"
				engines = 0
			case "manifest mismatch":
				other := "3000"
				ready.Result.Manifest.Effective.ContainerUser = &other
				wantStatus = "missing"
				wantFailure = "identity.mismatch"
				engines = 0
			case "identity changes during inspection":
				local := porttest.Ready()
				ready.LocalResult = &local
				other := "3000"
				ready.Result.Manifest.Effective.RemoteUser = &other
				wantFailure = "identity.mismatch"
				wantStatus = "changed"
			case "socket unavailable":
				host.Stamp = ""
				wantFailure = "identity.unavailable"
				wantStatus = "changed"
			case "local failure":
				ready.Result.Ready = false
				ready.Result.Issues = []readiness.Issue{{Kind: readiness.CheckoutPath}}
				wantStatus = "missing"
				engines = 0
			}
			v := (Preflight{Service: service, Local: ready, Canonical: codec}).Inspect()
			if v.Status != wantStatus || v.Failure != wantFailure || ready.EngineReads != engines || store.Writes != 0 {
				t.Fatalf("view %#v, engine reads %d writes %d", v, ready.EngineReads, store.Writes)
			}
			if condition == "locally changed" && (len(v.ChangedPaths) != 1 || v.ChangedPaths[0] != "/policy/network/allow") {
				t.Fatalf("lost local change paths: %#v", v)
			}
			if condition == "engine changed" && (len(v.ChangedPaths) != 1 || v.ChangedPaths[0] != "/image/id") {
				t.Fatalf("lost observed change paths: %#v", v)
			}
			if condition == "matching" && (v.Candidate != c || v.Record.Generation != "7" || v.Socket != "socket") {
				t.Fatalf("lost fresh candidate/selected generation: %#v", v)
			}
			if condition == "manifest mismatch" && v.Field != "containerUser" {
				t.Fatal(v)
			}
			if condition == "engine failure" && (v.Inspection.Ready || v.Inspection.Issues[0].Kind != readiness.ImageMissing) {
				t.Fatal(v)
			}
		})
	}
}
