# itos-container

A planned `itos container` extension to run a coding agent and every tool it executes in
a local container sandbox for one checkout, maintained by donvargax. Core itos retains
work routing, commit rules and CI proof.

Read [docs/charter.md](docs/charter.md) first for the selected scope and
[docs/plans/container-pilot.md](docs/plans/container-pilot.md) for the binding contract.
They are authoritative; [PLAN.md](PLAN.md) describes the path to the first milestone.

## What works today

Container commands and isolation are not implemented yet. The current entry point,
`cmd/itos-container/main.go`, exposes the inherited `itos-container count <file>` example,
version and shell completion. Count accepts `-` for standard input and prints lines and
words as text or one JSON object with `--json`.

The counting domain in `internal/count` uses a Files port, a read-only disk adapter and
an in-memory fake; `internal/cli` maps typed failures to exit codes. Black-box scenarios
in `features/` run through godog against the binary built for the test platform. This Go
foundation does not demonstrate container isolation.

Lines are logical text lines separated by LF; CRLF is one separator, and a nonempty final
line without a newline counts. Empty input has zero lines. Words are runs separated by
Unicode whitespace using Go's standard library; punctuation does not split a word, so
`hello-world` is one word. The domain has no JSON, flags or exit codes. Its additive
property uses newline-terminated pieces to preserve word boundaries.

## First milestone: an offline owned run

The planned target is `itos container run -- <argv>` in an approved, hardened container
with no network, the checkout as its only host mount, non-root identity, recorded
ownership, bounded startup, foreground exit and safe owned cleanup. These properties
need failing-then-passing scenario evidence before they can be claimed. Foundation work
comes first, then configuration/doctor/approval, then the offline owned run.

Linux amd64 with native Docker is the target, with no support claim before runtime proof.
Proxy egress, credentials/accounts, network Git, LAN ingress, Compose services and other
platforms are outside this milestone. The inherited multi-platform Go checks and release
tooling do not establish container-host support.

The threat model assumes hostile checkout code and arbitrary tool execution, including
dependency scripts and prompt-injected instructions. Hooks and agent instructions are
workflow safeguards, not the isolation boundary. The checkout remains writable and can
be damaged by the workload. Containers do not protect against host administrators or
kernel/container escapes.

## Go foundation checks

```sh
go build ./...
go test ./...
go test ./features -count=1
```

The inherited CI configuration runs Go gates and tests on Linux, macOS and Windows.
Domain coverage must stay at least 80 percent. Changed code requires fresh mutation
proof; CI samples recorded outcomes. T-4 reported zero eligible sample records, so that
passing sample is not a refreshed foundation baseline. Baseline work is separately
recorded. Properties run one random case per CI push and more in ordinary local runs.

See [docs/architecture.md](docs/architecture.md) for the counting example's layers and
[docs/upgrading.md](docs/upgrading.md) before changing tool pins. Retargeting `docs/CLI.md`
and `docs/CONFIG.md` to the container extension is separate foundation work.

## Working here

Read the charter, then [PLAN.md](PLAN.md) and [AGENTS.md](AGENTS.md). `itos go` coordinates;
follow the assigned brief, `itos guide work` and `itos guide commit`. Agents commit through
`itos commit`; only the coordinating loop pushes, marks pull requests ready and closes
items after the actual pushed head is green and required proof passes. Hooks and CI judge
the configured rules; published history is not rewritten.

## License and provenance

This project is AGPL-3.0 licensed, with copyright attributed to donvargax. Infrastructure comes
from the copyright owner's [itos](https://github.com/donvargax/itos) and
[itos-template](https://github.com/donvargax/itos-template), with permission to distribute
that owner's copied work under AGPL-3.0 here. The original projects and third-party dependencies
retain their own licenses and notices. See [LICENSE](LICENSE) and
[CONTRIBUTORS.md](CONTRIBUTORS.md).
