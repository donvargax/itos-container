package domain_test

import (
	"reflect"
	"testing"

	"github.com/donvargax/itos-container/internal/engine/manifest/domain"
	"github.com/donvargax/itos-container/internal/engine/manifest/port"
	"github.com/donvargax/itos-container/internal/engine/manifest/port/porttest"
)

func inspectReserving(env map[string]port.Value, reserved ...string) port.Result {
	files := &porttest.Files{Data: map[string][]byte{"worker.json": []byte("data")}}
	parser := &porttest.Parser{Value: obj(map[string]port.Value{"image": str("literal-image"), "containerEnv": obj(env)})}
	refs := &porttest.References{Images: map[string]bool{"literal-image": true}}
	return domain.Inspect("worker.json", files, parser, refs, reserved...)
}

func TestCallerNamesReserveAnEnvironmentNameUnderASCIIFolding(t *testing.T) {
	for _, name := range []string{"CODEX_HOME", "codex_home", "Codex_Home"} {
		got := inspectReserving(map[string]port.Value{name: str("/x")}, "CODEX_HOME")
		if got.Effective != nil || !reflect.DeepEqual(got.Issues, []port.Issue{{Kind: port.Reserved, Path: []string{"containerEnv", name}}}) {
			t.Fatal(name, got)
		}
		// Without the caller's name the key is ordinary.
		got = inspectReserving(map[string]port.Value{name: str("/x")})
		if got.Effective == nil || got.Effective.ContainerEnv[name] != "/x" || len(got.Issues) != 0 {
			t.Fatal(name, got)
		}
	}
	near := map[string]port.Value{"CODEX_HOME_DIR": str("/x"), "CODEX_SANDBOX": str("seatbelt"), "CODEXHOME": str("/x"), "MY_CODEX_HOME": str("/x")}
	got := inspectReserving(near, "CODEX_HOME")
	if got.Effective == nil || len(got.Effective.ContainerEnv) != 4 || len(got.Issues) != 0 {
		t.Fatal(got)
	}
	// The fixed names stay reserved beside the caller's.
	got = inspectReserving(map[string]port.Value{"PATH": str("/x")}, "CODEX_HOME")
	if got.Effective != nil || len(got.Issues) != 1 || got.Issues[0].Kind != port.Reserved {
		t.Fatal(got)
	}
}
