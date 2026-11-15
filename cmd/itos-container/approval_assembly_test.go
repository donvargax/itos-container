package main

import (
	"io"
	"testing"

	"github.com/donvargax/itos-container/internal/slices/doctor/inspection"
)

func TestEveryPolicyInspectionHasNamesAndOnlyReviewCommandsAdmitVersion2(t *testing.T) {
	if doctorPolicies().Names == nil {
		t.Fatal("file-only doctor has no names adapter")
	}
	for _, command := range []string{"doctor", "approve", "run", "status", "stop", ""} {
		t.Run(command, func(t *testing.T) {
			service, _ := approvalServices(command, nil, io.Discard, io.Discard)
			source, ok := service.Readiness.(inspection.Readiness)
			if !ok || source.Inspector.Names == nil {
				t.Fatal("readiness has no names adapter")
			}
			if source.Inspector.AllowVersion2 != (command == "doctor" || command == "approve") {
				t.Fatal("wrong version gate for", command)
			}
		})
	}
}
