package port_test

import (
	"testing"

	"github.com/donvargax/itos-container/internal/engine/policy/port"
)

func TestUnknownDiagnostic(t *testing.T) {
	got := port.Diagnostic(port.Issue{Kind: port.Failure(255), Path: []string{"a/b~c", ""}})
	want := port.Problem{
		Rule: "policy.internal", Exit: 70, File: "itos-container.yaml", Path: "/a~1b~0c/",
		Message: "Unknown policy failure", Fix: "report this as a bug",
	}
	if got != want {
		t.Fatalf("unknown policy diagnostic = %#v; want %#v", got, want)
	}
}
