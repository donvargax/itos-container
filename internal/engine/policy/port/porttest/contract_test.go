package porttest_test

import (
	"reflect"
	"testing"

	"github.com/donvargax/itos-container/internal/engine/policy/port"
	"github.com/donvargax/itos-container/internal/engine/policy/port/porttest"
)

func TestFilesContract(t *testing.T) {
	porttest.FilesContract(t, func(data []byte, failure port.Failure) port.Files {
		return porttest.Files{Data: data, Failure: failure}
	})
}

func TestSourceContract(t *testing.T) {
	porttest.SourceContract(t, func(f porttest.SourceFixture) port.Source {
		return &porttest.Source{Value: f.Value, Issues: f.Issues}
	})
}

func TestNamesContract(t *testing.T) {
	porttest.NamesContract(t, func(fixtures []porttest.NameFixture) port.Names {
		names := &porttest.Names{Table: map[string]string{}}
		for _, fixture := range fixtures {
			if fixture.OK {
				names.Table[fixture.Spelling] = fixture.ASCII
			}
		}
		return names
	})
}

func TestNamesRecordsEveryCall(t *testing.T) {
	names := &porttest.Names{Table: map[string]string{"A.example": "a.example"}}
	for _, spelling := range []string{"A.example", "b.example"} {
		names.ASCII(spelling)
	}
	if !reflect.DeepEqual(names.Calls, []string{"A.example", "b.example"}) {
		t.Fatalf("calls %q", names.Calls)
	}
}
