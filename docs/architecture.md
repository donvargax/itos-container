# Architecture

The private counting domain owns line/word semantics and its result. It reads through a Files
port whose outcomes describe the required file input without exposing OS-specific models. A
disk adapter implements real read-only access; an in-memory fake holds domain test inputs and
failures. Unit tests assert outcomes, and the rapid property checks addition for
newline-terminated pieces.

The domain does not know command-line flags, JSON or exit codes. Those belong to the `count`
handler, which carries the command's domain data into the domain through the port and holds no
counting of its own, and to the kong UI in front of it, which maps each typed failure to its
exit code and prints the JSON or the text answer. `cmd/itos-container` is that UI's entry
point; `features/` holds the public behaviour as scenarios, which godog runs against the binary
the harness builds for the platform it is on.

Line separators are LF, with CRLF counted once; a nonempty final unterminated line counts.
Words follow the standard-library Unicode-whitespace rule. No encoding detection or linguistic
word segmentation is performed. File data is not executed or changed.

Root policy and documentation are inherited from `main`. Language-specific gates travel with
their code; stack and feature branches remain buildable projects. Rendering and updating belong
to itos-template, not to this example's domain.

## Slices

Each command lives in `internal/slices/<command>/`: `command` is the thin handler,
`domain` owns behaviour, `port` declares its contracts, and named adapter siblings
(`disk`, `input`) implement them. Shared behaviour lives in
`internal/engine/<name>/{domain,port,<adapter>}` with the same boundaries. Nested
packages inherit their role; an owner needs no empty domain or port directory.

Domains and ports have an exact production standard-library allowlist: `bytes`,
`context`, `errors`, `fmt`, `maps`, `slices`, `sort`, `strconv`, `strings`, `time`
(for contract types) and `unicode`. Use their pure APIs; I/O belongs behind ports.
A domain imports its own domain/port and engine domain/port
APIs; a port imports only its own port subpackages and engine ports. Ports own
contract values, keeping dependencies acyclic. A command imports its domain,
ports, engine APIs and `internal/cli`. An adapter may import its own domain, port
and helper subpackages, plus engine APIs. No slice imports another slice; no
engine imports a slice. No command or domain imports an adapter. Only `cmd`
binds concrete adapters, including input's disk port.

Commands and port/porttest production code refuse process, network and filesystem
imports (`os`, `net`, `syscall`, `golang.org/x/sys`, `io/ioutil`, `path/filepath`
and `plugin`). The separate `command-io` and itos-derived `store-ports-io` rules
hold these restrictions even where a role allows standard-library helpers.
Test files may read fixtures; production fakes keep their state in memory.

The shared UI imports no other project package. It owns rendering, joined-error
traversal and the unknown-error fallback; `cmd` supplies count's failure classifier
from `internal/slices/count/command`. `internal/version` is a leaf. Release code
remains separate; only its tests use `internal/release/releasetest`.

Fakes and shared contracts live in `port/porttest`, reachable only by tests and
test-support code. `FilesContract` runs unchanged against count's fake, disk and
named-input route, covering bytes, failures, repeated reads and independent
returned storage. Adapter-specific I/O tests stay with their adapter; assembly
tests belong in `cmd`. Project import arrows apply to tests too; external tests
may import their package under test. Only domain test files may import rapid.

Static depguard globs classify roles for new owner names without generating
configuration. `tools/bin/architecture-check` supplements them with `go list`
imports, test imports, role coverage and binary dependencies. Its composable
`--siblings --outside --linked --toplevel` checks run by default, in the hook and
CI. Import-free unclassified packages, production test support and Go comments
suppressing depguard fail; targeted `nolint:gosec` remains permitted. Usage/load
errors exit 2, named architecture violations exit 1. Module identity comes from
the inspected packages, so a Go workspace's other modules do not change it.

The only top-level Go roots are `cmd`, `internal`, `features` and `tools`; every
other root fails even when cmd imports it. Imports with `testdata`, underscore-
or dot-prefixed path elements fail because `go list ./...` skips those packages.
Domains and ports refuse `IgnoredGoFiles`, including build-tagged and OS-specific
files; source-directory discovery also catches wholly excluded domain/port packages.
Test-support helpers under `port/porttest` may use build constraints.
Suppression checks trim names in every comma-separated list position, ignore case
and recognize both a trailing comma and an adjacent `//` explanation. They inspect
comments, so directive text in strings and targeted suppressions remain valid.

`--fixtures` runs the compile-first negative/positive matrix against the actual
pinned linter and policy. The hook's `--fixtures-if-staged` and CI's
`--fixtures-if-changed <base> <head>` select it only for changes to enforcement
inputs, including deletions and both renamed paths. Fixture tests stay under
`tools/testdata/architecture`, outside ordinary `go test ./...`. Domain coverage
uses actual slice/engine domain packages, retaining a separate 80% statement
floor for each nested package and refusing empty or unmeasurable discovery.

The rationale and source attribution are in
[the T-14 decision](decisions/0001-enforce-slice-and-engine-boundaries.md).
