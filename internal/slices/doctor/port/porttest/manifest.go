package porttest

import (
	"reflect"

	manifest "github.com/donvargax/itos-container/internal/engine/manifest/port"
	"github.com/donvargax/itos-container/internal/slices/doctor/port"
)

// Manifests returns a supplied file-validation outcome and records its selection.
type Manifests struct {
	Result manifest.Result
	Paths  []string
}

// Inspect records the policy's exact relative path.
func (m *Manifests) Inspect(path string) manifest.Result {
	m.Paths = append(m.Paths, path)
	return m.Result
}

// ManifestFixture supplies parsed data or a file failure and its expected result.
type ManifestFixture struct {
	Value   manifest.Value
	Failure manifest.Failure
	Result  manifest.Result
}

// ManifestsContract runs the same file-validation expectations against the fake
// and inspection adapter. The adapter binds memory-only owning port fakes.
func ManifestsContract(t Reporter, open func(ManifestFixture) port.Manifests) {
	t.Helper()
	for _, fixture := range []ManifestFixture{
		{Value: manifest.Value{Kind: manifest.Object, Fields: map[string]manifest.Value{"image": {Kind: manifest.String, Text: "requested"}}}, Result: manifest.Result{Path: "selected.json", Image: "requested", Effective: &manifest.Settings{Image: "requested", WorkspaceFolder: "/workspace", ContainerEnv: map[string]string{}}}},
		{Failure: manifest.Missing, Result: manifest.Result{Path: "selected.json", Issues: []manifest.Issue{{Kind: manifest.Missing}}}},
		{Failure: manifest.Path, Result: manifest.Result{Path: "selected.json", Issues: []manifest.Issue{{Kind: manifest.Path}}}},
		{Value: manifest.Value{Kind: manifest.Object}, Result: manifest.Result{Path: "selected.json", Issues: []manifest.Issue{{Kind: manifest.Image, Path: []string{"image"}}}}},
	} {
		m := open(fixture)
		for range 2 {
			if got := m.Inspect("selected.json"); !reflect.DeepEqual(got, fixture.Result) {
				t.Errorf("manifest inspection = %#v, want %#v", got, fixture.Result)
			}
		}
	}
}
