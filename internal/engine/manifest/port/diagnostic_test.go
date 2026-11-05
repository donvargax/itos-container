package port_test

import (
	"testing"

	"github.com/donvargax/itos-container/internal/engine/manifest/port"
)

func TestUnknownDiagnostic(t *testing.T) {
	result := port.Result{Path: "worker.json", Image: "image-subject"}
	got := port.Diagnostic(result, port.Issue{Kind: port.Failure(255), Path: []string{"a/b~c", ""}})
	want := port.Problem{
		Rule: "manifest.internal", Exit: 2, File: "worker.json", Path: "/a~1b~0c/",
		Message: "Unknown manifest failure", Fix: "report this as a bug",
	}
	if got != want {
		t.Fatalf("unknown manifest diagnostic = %#v; want %#v", got, want)
	}
}
