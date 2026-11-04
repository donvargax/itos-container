// Package porttest holds memory-only fakes and shared manifest port contracts.
package porttest

import (
	"slices"

	"github.com/donvargax/itos-container/internal/engine/manifest/port"
)

// Files supplies isolated copies of fixture bytes and records requested paths.
type Files struct {
	Data                  map[string][]byte
	Failures              map[string]port.Failure
	Directories           map[string]bool
	Reads, DirectoryReads []string
}

// Read returns configured bytes or a typed failure without partial data.
func (f *Files) Read(path string) ([]byte, port.Failure) {
	f.Reads = append(f.Reads, path)
	if failure := f.Failures[path]; failure != port.None {
		return nil, failure
	}
	data, exists := f.Data[path]
	if !exists {
		return nil, port.Missing
	}
	return slices.Clone(data), port.None
}

// Directory reports only explicitly supplied real-directory fixtures.
func (f *Files) Directory(path string) bool {
	f.DirectoryReads = append(f.DirectoryReads, path)
	return f.Directories[path]
}

// ParseCall captures data and the strict-JSON mode without retaining caller bytes.
type ParseCall struct {
	Data   string
	Strict bool
}

// Parser supplies representation outcomes; it does not contain a second parser.
type Parser struct {
	Value  port.Value
	Issues []port.Issue
	Calls  []ParseCall
}

// Parse returns independent values so a caller cannot alter later results.
func (p *Parser) Parse(data []byte, strict bool) (port.Value, []port.Issue) {
	p.Calls = append(p.Calls, ParseCall{string(data), strict})
	return Clone(p.Value), cloneIssues(p.Issues)
}

// References supplies syntactic validity, never approval or image availability.
type References struct {
	Images map[string]bool
	Calls  []string
}

// Valid records the unchanged literal image before returning fixture validity.
func (r *References) Valid(image string) bool {
	r.Calls = append(r.Calls, image)
	return r.Images[image]
}

// Clone copies a value's owned storage for fixture isolation.
func Clone(value port.Value) port.Value {
	result := value
	if value.Fields != nil {
		result.Fields = make(map[string]port.Value, len(value.Fields))
		for key, value := range value.Fields {
			result.Fields[key] = Clone(value)
		}
	}
	if value.Elements != nil {
		result.Elements = make([]port.Value, len(value.Elements))
		for i, value := range value.Elements {
			result.Elements[i] = Clone(value)
		}
	}
	return result
}

func cloneIssues(issues []port.Issue) []port.Issue {
	result := slices.Clone(issues)
	for i := range result {
		result[i].Path = slices.Clone(result[i].Path)
	}
	return result
}
