// Package port defines the policy engine's input and outcomes without encoding
// or operating-system types. Sources check representation before returning data.
package port

// Kind is a data value's type.
type Kind uint8

const (
	Null Kind = iota
	Object
	Array
	String
	Number
	Boolean
)

// Value retains exact numbers so version checks cannot round a fractional input.
// Number is a canonical reduced rational: a decimal integer, or numerator/denominator
// with a positive denominator greater than one. Sources normalize it before returning.
type Value struct {
	Kind   Kind
	Fields map[string]Value
	Text   string
	Number string
}

// Failure identifies a rejected requirement, independently of UI rule names.
type Failure uint8

const (
	None Failure = iota
	Missing
	Unreadable
	Empty
	Syntax
	MultipleDocuments
	DuplicateKey
	NonStringKey
	Anchor
	Alias
	MergeKey
	Tag
	NonFinite
	Required
	Type
	Version
	UnknownKey
	NetworkMode
	Path
)

// Issue locates one failure using decoded path components.
type Issue struct {
	Kind Failure
	Path []string
}

// Files supplies the policy bytes without exposing filesystem errors.
type Files interface {
	Read() ([]byte, Failure)
}

// Source returns data only after representation validation succeeds.
type Source interface {
	Read() (Value, []Issue)
}

// Policy is the complete effective request. It never records approval.
type Policy struct {
	Version  int
	Manifest string
	Network  Network
}

// Network describes the requested network boundary.
type Network struct{ Mode string }

// Result contains either a complete policy or issues, never partial policy data.
type Result struct {
	Policy *Policy
	Issues []Issue
}
