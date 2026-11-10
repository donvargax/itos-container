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

// Decimal represents Coefficient * 10^Exponent without expanding the exponent.
// Both parts are canonical signed decimal integers without a leading plus or
// leading zeros. A nonzero coefficient has no trailing zeros; zero is {"0", "0"}.
// Sources normalize both parts, retaining exactness with storage bounded by input
// length rather than exponent magnitude. A negative exponent means a fraction.
type Decimal struct {
	Coefficient string
	Exponent    string
}

// Value retains exact numbers so version checks cannot round a fractional input.
// Items holds an array's elements in order, and is nil for an empty array.
type Value struct {
	Kind   Kind
	Fields map[string]Value
	Items  []Value
	Text   string
	Number Decimal
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
	Complexity
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
	HostName
	HostDuplicate
	AllowEmpty
	AllowOffline
	CredentialsEmpty
	CredentialKind
	CredentialDuplicate
	CredentialSource
)

// Issue locates one failure using decoded path components. Version is the
// supported policy version the file declared, or zero where none was read: the
// fix a person needs depends on which schema the file was judged against.
type Issue struct {
	Kind    Failure
	Path    []string
	Version int
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
	Version     int
	Manifest    string
	Network     Network
	Credentials []Credential
}

// Credential keeps the written order for located source observations. Display
// orders requests by kind; neither a parsed request nor a path grants access.
type Credential struct {
	Kind   string `json:"kind"`
	Source string `json:"source"`
}

// Network describes the requested network boundary. Allow holds the canonical
// host names of an allowlist in ascending byte order, and is nil when offline.
type Network struct {
	Mode  string
	Allow []string
}

// Result contains either a complete policy or issues, never partial policy data.
type Result struct {
	Policy *Policy
	Issues []Issue
}
