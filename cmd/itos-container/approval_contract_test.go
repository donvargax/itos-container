package main

import (
	"encoding/json"
	"github.com/donvargax/itos-container/internal/engine/approval/canonical"
	"github.com/donvargax/itos-container/internal/engine/approval/disk"
	"os"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/donvargax/itos-container/internal/engine/approval/domain"
	"github.com/donvargax/itos-container/internal/engine/approval/port"
	"github.com/donvargax/itos-container/internal/engine/approval/port/porttest"
)

// TestApprovalContractProbe is invoked only by feature fixtures through a test
// binary. There is no production flag or environment injection surface.
func TestApprovalContractProbe(t *testing.T) {
	path := os.Getenv("SLICE_4_PROBE_OUTPUT")
	if path == "" {
		t.Skip("feature contract subprocess only")
	}
	kind, condition, uid, gid := os.Getenv("SLICE_4_PROBE_KIND"), os.Getenv("SLICE_4_PROBE_CONDITION"), os.Getenv("SLICE_4_PROBE_UID"), os.Getenv("SLICE_4_PROBE_GID")
	codec := canonical.JSON{}
	a := codec.Candidate(porttest.Ready(), "1000", "1000")
	store := &porttest.Store{Records: map[string]port.Record{a.Project: {Candidate: a, Generation: "1"}}}
	out := map[string]any{}
	switch kind {
	case "identity", "identity-change":
		c, failure := domain.Resolve(porttest.Ready(), uid, gid, codec)
		out["failure"], out["candidate"], out["writes"] = string(failure), c, store.Writes
		status := "missing"
		if previous, _ := store.Read(c); previous.Generation != "" {
			status = "changed"
			if previous.Fingerprint == c.Fingerprint {
				status = "matching"
			}
		}
		out["status"] = status
	case "publication":
		failure := port.Failure("state.write")
		if strings.Contains(condition, "owned") {
			failure = "state.unsafe"
		}
		if strings.Contains(condition, "replaced") {
			failure = "state.path"
		}
		store.Failure = failure
		c := codec.Candidate(porttest.Ready(), "1000", "3000")
		record, failure := domain.Publish(store, c, "1")
		out["failure"], out["record"], out["visible"], out["writes"] = string(failure), record, store.Records[a.Project], store.Writes
	case "selection":
		expected := "1"
		switch condition {
		case "active record is absent":
			delete(store.Records, a.Project)
		case "candidate settings differ":
			a = codec.Candidate(porttest.Ready(), "1000", "3000")
		case "expected generation was superseded", "active generation changes before final verification":
			record := store.Records[a.Project]
			record.Generation = "2"
			store.Records[a.Project] = record
		case "active record is invalid":
			store.Failure = "state.invalid"
		}
		frozen, failure := domain.Select(store, a, expected)
		out["failure"], out["frozen"] = string(failure), frozen
	case "independence":
		a = codec.Candidate(porttest.Ready(), uid, gid)
		store.Records = map[string]port.Record{a.Project: {Candidate: a, Generation: "1"}}
		first, failure := domain.Select(store, a, "1")
		out["failure"] = string(failure)
		out["original"] = a
		bytes := first.Bytes()
		if len(bytes) > 0 {
			bytes[0] = 'X'
		}
		a.Canonical = "caller changed"
		b := codec.Candidate(porttest.Ready(), uid, "4000")
		_, publishFailure := domain.Publish(store, b, "1")
		out["publish_failure"] = string(publishFailure)
		second, selectFailure := domain.Select(store, b, "2")
		out["select_failure"] = string(selectFailure)
		out["first"], out["second"], out["expected_second"] = first, second, b
	}
	data, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestApprovalRealStoreContract(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux protected-state target")
	}
	porttest.StoreContract(t, func() (port.Store, port.Candidate, port.Candidate) {
		codec := canonical.JSON{}
		uid := strconv.Itoa(os.Geteuid())
		a := codec.Candidate(porttest.Ready(), uid, "1000")
		b := codec.Candidate(porttest.Ready(), uid, "3000")
		return disk.Store{StateHome: t.TempDir(), Records: codec}, a, b
	})
}
