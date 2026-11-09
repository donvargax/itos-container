package main

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	approvalcanonical "github.com/donvargax/itos-container/internal/engine/approval/canonical"
	approvaldisk "github.com/donvargax/itos-container/internal/engine/approval/disk"
	approvaldomain "github.com/donvargax/itos-container/internal/engine/approval/domain"
	approvalhost "github.com/donvargax/itos-container/internal/engine/approval/host"
	ap "github.com/donvargax/itos-container/internal/engine/approval/port"
	approvalporttest "github.com/donvargax/itos-container/internal/engine/approval/port/porttest"
	"github.com/donvargax/itos-container/internal/slices/doctor/inspection"
)

// A command whose call budget has ended gives up with the retryable
// state.busy, not with the path refusal the real reservation scan returns for an
// ended context. This runs the assembly that main builds: the session engine's
// scan over the approval engine's protected locations, with no fake in between.
// The context is ended before the call, so the outcome does not depend on timing.
func TestEndedCallBudgetOverTheRealScanIsBusy(t *testing.T) {
	for _, bootstrapped := range []bool{false, true} {
		for _, write := range []bool{false, true} {
			name := map[bool]string{false: "fresh account", true: "enrolled account"}[bootstrapped] + map[bool]string{false: ", reader", true: ", writer"}[write]
			t.Run(name, func(t *testing.T) {
				base := t.TempDir()
				home := filepath.Join(base, "account")
				if err := os.Mkdir(home, 0700); err != nil {
					t.Fatal(err)
				}
				checkout := filepath.Join(base, "checkout")
				if err := os.Mkdir(checkout, 0700); err != nil {
					t.Fatal(err)
				}
				codec := approvalcanonical.JSON{}
				service := approvaldomain.Service{Readiness: inspection.Readiness{}, Host: approvalhost.Process{}, Codec: codec,
					Store: approvaldisk.Store{StateHome: filepath.Join(base, "state"), Home: home, Records: codec}}
				control := coordinatedApproval(service, &approvalporttest.Account{Home: home}).Store.(approvaldisk.Store).Control
				c := ap.Candidate{UID: strconv.Itoa(os.Geteuid()), GID: strconv.Itoa(os.Getegid()), Root: checkout, Project: strings.Repeat("a", 64), Canonical: "A", Fingerprint: "a"}
				if bootstrapped {
					tx, failure := control.Begin(t.Context(), c, true)
					if failure != "" {
						t.Fatal(failure)
					}
					tx.Close()
				}
				before := snapshotTree(t, base)
				ended, cancel := context.WithCancel(t.Context())
				cancel()
				tx, failure := control.Begin(ended, c, write)
				if tx != nil {
					tx.Close()
				}
				if tx != nil || failure != "state.busy" {
					t.Fatalf("ended budget: transaction %v, %q; want state.busy", tx, failure)
				}
				if after := snapshotTree(t, base); after != before {
					t.Fatalf("the ended call changed the account:\n%s\n--- after\n%s", before, after)
				}
			})
		}
	}
}

func snapshotTree(t *testing.T, root string) string {
	t.Helper()
	var names []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		names = append(names, path+" "+info.Mode().String()+" "+info.ModTime().String())
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return strings.Join(names, "\n")
}
