// Package port defines manifest data and validation capabilities without parser
// or filesystem types. Validation never grants approval or runtime authority.
package port

import "strings"

// Kind identifies a JSON value's type. Number and boolean text stays opaque.
type Kind uint8

const (
	Null Kind = iota
	Object
	Array
	String
	Number
	Boolean
)

// Value preserves decoded strings, array order and unambiguous object members.
type Value struct {
	Kind     Kind
	Text     string
	Fields   map[string]Value
	Elements []Value
}

// Failure identifies a requirement independently of public diagnostics.
type Failure uint8

const (
	None Failure = iota
	Missing
	Path
	Syntax
	Duplicate
	Unsupported
	InvalidValue
	Image
	Workspace
	Identity
	Reserved
	Interpolation
	MetadataFailure
)

// Issue carries decoded path components, never rejected source values.
type Issue struct {
	Kind Failure
	Path []string
}

// Files reads one selected regular file and checks real checkout directories.
// Every path is relative to the injected checkout root. Links are never accepted.
type Files interface {
	Read(path string) ([]byte, Failure)
	Directory(path string) bool
}

// Parser checks syntax and decoded-key uniqueness before returning data. Strict
// mode refuses comments and trailing commas. A failure returns no partial value.
type Parser interface {
	Parse(data []byte, strict bool) (Value, []Issue)
}

// References validates a literal named sha256-digested reference without lookup,
// normalization, implicit registries or namespaces, or approval comparisons.
type References interface{ Valid(image string) bool }

// Settings is the effective request. Nil strings represent omissions, not null
// input, and no omitted image or identity is sourced from host, policy or labels.
type Settings struct {
	Name                      *string
	Image                     string
	WorkspaceFolder           string
	ContainerEnv              map[string]string
	ContainerUser, RemoteUser *string
}

// Label is supplied only after an actual inspection. File-only validation does
// not construct one. Present=false means positively established label absence.
type Label struct {
	Present bool
	Text    string
}

// Metadata records inspection and descriptive fields that remain unused.
type Metadata struct {
	Checked bool
	Present *bool
	Unused  []string
}

// Result holds either complete settings or issues. Image remains available for
// attribution if supplied metadata invalidates an otherwise valid request.
type Result struct {
	Path, Image string
	Effective   *Settings
	Issues      []Issue
	Unused      []string
	Metadata    Metadata
}

// Pointer encodes decoded keys, including literal slashes and tildes.
func Pointer(path []string) string {
	var result string
	for _, part := range path {
		result += "/" + strings.ReplaceAll(strings.ReplaceAll(part, "~", "~0"), "/", "~1")
	}
	return result
}
