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

The counting domain in `internal/slices/count` uses a Files port, a read-only disk adapter and
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
Domain coverage must stay at least 80 percent. CI alone judges fresh changed-code proof:
up to ten mutation sites across changed functions, plus measured coverage of added or
changed lines, including functions without sites. Core itos supplies the judged range's
base for both selection and seed; the same base and eligible sites give the same draw.
Own tests and listed live scenarios contribute coverage. Survivors need tests, with only
truly equivalent mutants excepted for the person's review; uncovered code needs tests.
Agents commit and stop, with no local mutation runs or committed mutation/coverage evidence.
The sampled proof is not a full baseline. T-8's [foundation report](docs/proof/T-8-foundation-baseline.md)
retains the historical full measurement; T-4's earlier sample judged zero eligible records.
Properties run one random case per CI push and more in ordinary local runs.

See [docs/architecture.md](docs/architecture.md) for the counting example's layers and
[docs/upgrading.md](docs/upgrading.md) before changing tool pins. Retargeting `docs/CLI.md`
and `docs/CONFIG.md` to the container extension is separate foundation work.

## Working here

Read the charter, then [PLAN.md](PLAN.md) and [AGENTS.md](AGENTS.md). `itos go` coordinates;
follow the assigned brief, `itos guide work` and `itos guide commit`. Agents commit through
`itos commit`; only the coordinating loop pushes, marks pull requests ready and closes
items after the actual pushed head is green and required proof passes. Hooks and CI judge
the configured rules; published history is not rewritten.

## Local commit hooks

Hooks cannot be committed, so a fresh clone has none. Run these once in each clone. They write
the clone's local git config, shared by its worktrees, and need git 2.54.0 or later:

```sh
itos hook install
git config hook.itos-container-pre-commit.event pre-commit
git config hook.itos-container-pre-commit.command tools/hooks/pre-commit
```

`itos hook install` declares the commit-msg and pre-push hooks; the two `git config` lines
declare this project's pre-commit. None of them touches core.hooksPath, which can stay unset:
git runs config-declared hooks whatever core.hooksPath says. Do not point core.hooksPath at
`tools/hooks`, or git runs that pre-commit a second time beside the declared entry.

To verify, each command below must name its hook. Each prints "no hooks found" and exits
non-zero when the hook is not declared.

```sh
git hook list pre-commit   # itos-container-pre-commit
git hook list commit-msg   # itos-commit-msg
git hook list pre-push     # itos-pre-push
```

A later commit shows the gates are enforced. `itos commit` refuses with exit 3 where the
commit-msg hook is not declared. With it declared, every commit has its message and staged
paths judged. `itos config check` runs only when the commit stages `itos.yaml`, a ledger file,
the work registry, a smoke set or a decision record. The checks of the task the commit names
run too, but a failing one refuses the commit only once that task's item is done; while the
item is open the failure is printed with the task's status and the commit goes through.

A commit that stages any path other than Markdown, `docs/**`, `tasks/**` or feature files runs
the pre-commit's checks (govulncheck needs the network), and any failure refuses the commit.
A prose-only commit skips them on purpose, so seeing no lint output there does not mean the
hook is missing.

On a git older than 2.54.0 the project's pre-commit does not run. Upgrade git.

## License and provenance

This project is AGPL-3.0 licensed, with copyright attributed to donvargax. Infrastructure comes
from the copyright owner's [itos](https://github.com/donvargax/itos) and
[itos-template](https://github.com/donvargax/itos-template), with permission to distribute
that owner's copied work under AGPL-3.0 here. The original projects and third-party dependencies
retain their own licenses and notices. See [LICENSE](LICENSE) and
[CONTRIBUTORS.md](CONTRIBUTORS.md).
