package porttest

import (
	"github.com/donvargax/itos-container/internal/engine/manifest/port"
	"testing"
)

func TestFilesContract(t *testing.T) {
	FilesContract(t, func(data []byte, failure port.Failure) port.Files {
		return &Files{Data: map[string][]byte{"worker.json": data}, Failures: map[string]port.Failure{"worker.json": failure}, Directories: map[string]bool{"src": true}}
	})
}

func TestParserContract(t *testing.T) {
	ParserContract(t, func(f ParserFixture) port.Parser { return &Parser{Value: f.Value, Issues: f.Issues} })
}

func TestReferenceContract(t *testing.T) {
	ReferenceContract(t, func(image string, valid bool) port.References {
		return &References{Images: map[string]bool{image: valid}}
	})
}

func TestMissingFileAndRecordedArguments(t *testing.T) {
	f := &Files{}
	if data, failure := f.Read("missing"); data != nil || failure != port.Missing {
		t.Fatal(data, failure)
	}
	if len(f.Reads) != 1 || f.Reads[0] != "missing" {
		t.Fatal(f.Reads)
	}
	p := &Parser{}
	p.Parse([]byte("raw"), true)
	if len(p.Calls) != 1 || p.Calls[0] != (ParseCall{Data: "raw", Strict: true}) {
		t.Fatal(p.Calls)
	}
}
