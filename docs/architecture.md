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

Each command is a vertical slice under `internal/<command>/`, laid out as `internal/count` is:
`command` (the handler the CLI calls: flags in, domain data out, no logic of its own), `domain`
(the behaviour, knowing no flags, JSON, exit codes, files, processes or network), `port` (the
interfaces the domain reaches the world through) and one package per adapter (`disk` for count;
for the container commands, for example, an engine adapter over the Docker API). Each adapter
imports only the port it implements, never the domain; a fake of each port lives beside the
domain's tests. Code two slices share is an engine package of its own with the same rules,
never one slice importing another's domain.

`tools/bin/check-slice-layout` discovers slices by their `domain` or `port` directory and
generates concrete depguard rules in a disposable effective configuration. `.golangci.yml`
remains the base for the full lint suite, with paths resolved from the checkout. Every
adapter name and nested package is protected; an external adapter test may import only its
exact subject and its slice's port subtree within the module. Rapid is available only to
domain tests. The checker then runs compilable import and coverage fixtures through the same
discovery and pinned linter. These import boundaries do not prove runtime isolation.

`tools/bin/domain-coverage` uses that same discovery to hold each concrete domain package,
including nested domains, to 80% statement coverage separately. Ports, adapters, command
handlers, fixtures and caches do not enter those denominators. Empty discovery and packages
without measurable statements fail explicitly.
