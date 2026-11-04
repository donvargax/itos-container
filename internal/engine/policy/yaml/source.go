// Package yaml adapts YAML nodes to policy data, without permissive decoding,
// alias resolution, merge expansion or source-bearing diagnostics.
package yaml

import (
	"bytes"
	"io"
	"slices"
	"strconv"
	"unicode/utf8"

	"go.yaml.in/yaml/v3"

	"github.com/donvargax/itos-container/internal/engine/policy/port"
)

// Source reads the policy through a files port.
type Source struct{ Files port.Files }

// Read checks the whole representation before exposing any policy fields.
func (s Source) Read() (port.Value, []port.Issue) {
	data, failure := s.Files.Read()
	if failure != port.None {
		return rejected(failure)
	}
	if !utf8.Valid(data) {
		return rejected(port.Syntax)
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	var document yaml.Node
	err := decoder.Decode(&document)
	if err == io.EOF {
		return rejected(port.Empty)
	}
	if err != nil {
		return rejected(port.Syntax)
	}
	var next yaml.Node
	err = decoder.Decode(&next)
	if err == nil {
		return rejected(port.MultipleDocuments)
	}
	if err != io.EOF {
		return rejected(port.Syntax)
	}
	root := document.Content[0]
	if root.Kind == yaml.ScalarNode && root.Tag == "!!null" && root.Value == "" && root.Style == 0 {
		return rejected(port.Empty)
	}
	reader := tree{}
	value := reader.visit(root)
	if reader.syntax {
		return rejected(port.Syntax)
	}
	if len(reader.issues) != 0 {
		return port.Value{}, reader.issues
	}
	return value, nil
}

func rejected(kind port.Failure) (port.Value, []port.Issue) {
	return port.Value{}, []port.Issue{{Kind: kind}}
}

type tree struct {
	issues []port.Issue
	syntax bool
	path   []string // Shared traversal stack; only retained issues copy it.
}

func (t *tree) issue(kind port.Failure) {
	t.issues = append(t.issues, port.Issue{Kind: kind, Path: slices.Clone(t.path)})
}

func (t *tree) visit(node *yaml.Node) port.Value {
	if node.Kind == yaml.AliasNode {
		t.issue(port.Alias)
		return port.Value{}
	}
	if node.Anchor != "" {
		t.issue(port.Anchor)
	}
	tag := node.Tag
	if node.Style&yaml.TaggedStyle != 0 {
		switch tag {
		case "!!map":
			if node.Kind != yaml.MappingNode {
				t.syntax = true
			}
		case "!!seq":
			if node.Kind != yaml.SequenceNode {
				t.syntax = true
			}
		case "!!str", "!!int", "!!float", "!!bool", "!!null":
			if node.Kind != yaml.ScalarNode {
				t.syntax = true
			}
		default:
			t.issue(port.Tag)
		}
	}
	switch node.Kind {
	case yaml.MappingNode:
		fields := map[string]port.Value{}
		for i := 0; i < len(node.Content); i += 2 {
			key, value := node.Content[i], node.Content[i+1]
			if key.Kind == yaml.ScalarNode && key.Value == "<<" {
				t.path = append(t.path, "<<")
				t.issue(port.MergeKey)
				t.visit(value)
				t.path = t.path[:len(t.path)-1]
				continue
			}
			k := t.visit(key)
			if k.Kind != port.String {
				t.issue(port.NonStringKey)
				t.visit(value)
				continue
			}
			t.path = append(t.path, k.Text)
			if _, exists := fields[k.Text]; exists {
				t.issue(port.DuplicateKey)
			}
			fields[k.Text] = t.visit(value)
			t.path = t.path[:len(t.path)-1]
		}
		return port.Value{Kind: port.Object, Fields: fields}
	case yaml.SequenceNode:
		for i, value := range node.Content {
			t.path = append(t.path, strconv.Itoa(i))
			t.visit(value)
			t.path = t.path[:len(t.path)-1]
		}
		return port.Value{Kind: port.Array}
	default:
		value, failure := scalar(node)
		switch failure {
		case port.Syntax:
			t.syntax = true
		case port.NonFinite:
			t.issue(failure)
		}
		return value
	}
}
