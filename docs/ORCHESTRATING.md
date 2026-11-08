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
- [CI](../.github/workflows/ci.yml) has no stack/feature branch filters. T-25 enables
  [releases](../.github/workflows/release.yml) only when `ci`, `engine` and automatic Linux
  `platform` all succeed at the pushed `main` SHA. Manual macOS/Windows jobs are not
  prerequisites; pull requests and manual dispatch never publish. The five archives are
  distribution targets, not container-host support. Before the first real tag, require
  the [version/notes dry run](upgrading.md#binary-releases-t-25), then verify the pushed
  main run and assets. Preserve the helpers' itos-template/itos attribution.
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
  or step definitions. T-8 is a historical test task example: its generated proof procedure
  was superseded by T-13. T-9 is a docs task: only this file, cheap
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

CI alone judges `proof.code` under `cmd/` and `internal/`: up to ten fresh sites across changed
functions and measured coverage of added/changed lines, including functions without sites.
Core itos supplies the judged range's `{base}` for `--since` and `--seed`. The same base and
eligible sites give the same draw; changing candidates can change it. The `ci` job measures
own tests and listed live scenarios, using the harness's individual-test `GOCOVERDIR` fallback.

Agents commit and stop: no local mutation runs or generated evidence committed or restored
into CI. `.metrics/mutate/` and `.metrics/coverage/` are ignored. Kill survivors and cover
uncovered changed lines with tests. Only equivalent mutants may have reasoned exceptions for
the person's review; never except uncovered code, redraw, remove sites or hand-edit evidence
to pass.

T-8's [report and reproduction commands](proof/T-8-foundation-baseline.md) are historical,
not handoff instructions. A sample or empty range proves neither a full baseline nor
container-host support. Docs-only work fabricates no mutation results.

## Lessons

Keep at most ten lessons, each with recorded date, last-seen date and exit item or permanent
status. Report tool gaps to the coordinator for upstream follow-up; invent no lesson or issue.
