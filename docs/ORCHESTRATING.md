# This repository's own notes

`itos go` prints these notes after its coordinator guide.

## Start of a session

- Read [the charter](charter.md) first, then [PLAN.md](../PLAN.md), including its
  [milestone limits](../PLAN.md#milestone-limits), the binding
  [pilot contract](plans/container-pilot.md), [AGENTS.md](../AGENTS.md) and the actual
  [itos.yaml](../itos.yaml). The charter selects foundation, configuration/doctor/approval,
  then the offline owned-run milestone. The broader pilot authorizes no later slice here.
- Linux amd64 with native Docker is a target requiring runtime evidence. Egress, credentials,
  network Git, LAN ingress, Compose and other container hosts stay deferred. Send open security
  decisions to the person; a brief cannot settle them. Assume hostile checkout code: hooks
  and agent instructions are workflow safeguards, not isolation.
- This is one project on `main` and work branches. [cmd/itos-container/main.go](../cmd/itos-container/main.go)
  exposes count, version and completion; [features/README.md](../features/README.md) documents
  the godog runner. The planned `itos container` commands and isolation are unimplemented.
- [.github/workflows/ci.yml](../.github/workflows/ci.yml) runs on pushes and pull requests without
  stack/feature branch filters. Its Linux/macOS/Windows matrix covers inherited Go tests, not
  container-host support. [.github/workflows/release.yml](../.github/workflows/release.yml) is
  reusable binary-release machinery, but `ci.yml` does not call it; releases are not enabled.
- Preserve the counting example's standard-library Unicode whitespace and logical lines;
  its additive property uses newline-terminated pieces, not arbitrary concatenation.
  Preserve attribution to donvargax and infrastructure provenance from itos and itos-template
  ([LICENSE](../LICENSE), [CONTRIBUTORS.md](../CONTRIBUTORS.md)); no global handle substitutions.
  Renovate configuration alone proves neither App installation nor bot activity.

## Inspect and brief

- Inspect with `itos work`, `itos work show <id>` and [tasks/phase-1.yaml](../tasks/phase-1.yaml).
  `itos task` executes checks; it is not a read-only viewer and may run expensive or
  state-writing measurements. T-8's completion checks included a full mutation measurement.
  T-8 is done and those checks were removed on closure: read its ledger `why`, commit history
  and [foundation report](proof/T-8-foundation-baseline.md) for original checks, results and
  environment. Do not infer commands from an empty `done_when`, reopen it or
  run a measurement merely to draft a brief.
- Read `itos guide work` and `itos guide commit`; use `itos guide brief` for the handoff.
  Resolve all placeholders before handing off. Name the item, drafter or implementer
  role, ledger type, spec path, allowed edit paths, exact checks and neighbouring checks,
  required proof and commit type/footer.
- Non-behaviour tasks follow the ledger's `why` and `done_when`, without inventing scenarios
  or step definitions. T-8 is the test task example: tests and generated proof under its
  specified checks, no new behaviour scenarios. T-9 is a docs task: only this file, cheap
  documentation checks, `docs` with `Item: T-9`, no mutation measurement.
- Behaviour briefs require specified black-box scenarios. Commit their step definitions
  red first in a `test` commit while scenarios remain `@wip`. With `@wip` removed for the run,
  each must fail at its behaviour assertion, then pass after implementation; undefined steps
  are not evidence. Run non-engine cases locally; real-engine scenarios are CI-only. Domain
  tests use the engine fake; agents never receive the Docker socket. Preserve smoke coverage
  and never weaken a scenario or gate to obtain green.

## Responsibility at handoff

Override generic guides' lifecycle instructions in every brief. A drafter specifies only and
does not take an item; an implementer follows the assigned spec. Agents read `git status --short`,
stage only their own explicit file paths, commit through `itos commit -F <file>`, then stop.
The loop takes items, pushes, marks pull requests ready, watches CI and closes items only after
the actual pushed head is green and required proof passes. Never hand-edit owners/statuses,
bypass hooks, force-push or rewrite published history. Agents report commits, checks, applicable
red/green scenario evidence, gaps and ideas with why they matter. The coordinator owns registry
changes and upstream reports.

## Proof

`proof.code` in `itos.yaml` already selects Go changes under both `cmd/` and `internal/`.
After the last source/test change, refresh required changed-code evidence using
`tools/bin/pinned itos-cc` with `--all-tests`, `--fail-uncovered` and `--no-annotate`.
Find the range base through `itos work show` and history: the parent of the item's actual
first commit, specs included, never an assumed take commit. Commit project-local
`.metrics/mutate/` evidence by explicit path; never copy another project's cache or hand-edit
results. Kill non-equivalent survivors with tests and exercise uncovered executable code,
including functions without mutation sites. Only genuinely equivalent mutants may have
reasoned exceptions for the person to review; never except uncovered code or remove sites
to avoid proof. Documentation-only changes touch no `proof.code` path and fabricate no
mutation results. Leave configured hooks and CI in control of their gates.

T-8's [completed full foundation baseline](proof/T-8-foundation-baseline.md) supersedes T-4's
sample with zero eligible records. A sample or empty changed-code range is not a fresh baseline;
later test/toolchain changes can stale evidence. Follow the report's Go reproduction environment.
It proves neither other-platform nor container-isolation support. Do not make the full
measurement a recurring gate or run it for documentation work.

## Lessons

Keep at most ten lessons, each with recorded date, last-seen date and exit item or permanent
status. Report tool gaps to the coordinator for upstream follow-up; invent no lesson or issue.
