# This repository's own notes

`itos go` prints these notes after its coordinator guide.

## Start of a session

- Read [the charter](charter.md) first, then [PLAN.md](../PLAN.md), including its
  [milestone limits](../PLAN.md#milestone-limits), the binding
  [pilot contract](plans/container-pilot.md), [AGENTS.md](../AGENTS.md) and the actual
  [itos.yaml](../itos.yaml). The charter selects the offline owned run, then egress through
  an allowlisting proxy, passed-in credentials, a harness home, interactive use and
  unreachable late host sockets.
- Linux amd64 with native Docker is the target. Support claims require passing runtime
  scenarios. LAN ingress, Compose, other container platforms and worker access to the
  Docker socket are deferred. The person settles open security decisions. Assume hostile
  checkout code. Hooks and agent instructions are workflow safeguards. Isolation relies
  on container controls.
- One project on `main` and work branches. [The entry point](../cmd/itos-container/main.go)
  dispatches `doctor`, `approve`, `run`, `status` and `stop` alongside `count`, `version`
  and `completion`. [The scenario guide](../features/README.md) covers the godog runner.
- Each item reaches `main` through one pull request (itos.yaml's `push`): a draft, which
  runs no CI job, until the loop marks it ready, then merged once its checks pass and
  Cursor Bugbot has reviewed it; Codex is still asked, best effort. Docs-only commits
  outside `features/` go straight to `main`. The loop makes every push.
- [CI](../.github/workflows/ci.yml) runs on ready pull requests, pushes to `main` and manual
  dispatch; macOS/Windows jobs run only on dispatch. [Releases](../.github/workflows/release.yml)
  need `ci`, `engine` and Linux `platform` green at the pushed `main` SHA; pull requests
  and dispatch never publish. The five archives are distribution targets, not
  container-host support.
- Preserve the counting example's standard-library Unicode whitespace and logical lines;
  its additive property uses newline-terminated pieces, not arbitrary concatenation.
  Preserve attribution to donvargax and infrastructure provenance from itos and itos-template
  ([LICENSE](../LICENSE), [CONTRIBUTORS.md](../CONTRIBUTORS.md)); no global handle substitutions.
  Renovate configuration alone proves neither App installation nor bot activity.

## Inspect and brief

- Inspect with `itos work`, `itos work show <id>` and [tasks/phase-1.yaml](../tasks/phase-1.yaml).
  `itos task` executes checks that may be expensive or write state. When drafting a brief,
  do not infer commands from a closed task's empty `done_when`, reopen it or run a measurement.
- Read `itos guide work` and `itos guide commit`; use `itos guide brief` for the handoff.
  Resolve all placeholders before handing off. Name the item, drafter or implementer
  role, ledger type, spec path, allowed edit paths, exact checks and neighbouring checks,
  required proof and commit type/footer.
- Non-behaviour tasks follow the ledger's `why` and `done_when`, without inventing scenarios
  or step definitions. T-9 is a docs task: only this file, cheap
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

- Set `prose: true` on each docs task check that reads documents. On ranges matching
  `ci.prose.paths`, CI keeps only static checks and those marked `prose: true`.
  `itos work done` runs no late check. Recorded 2026-11-25. Last seen 2026-11-09 in
  T-38's spec review (itos 8.46.4, also T-26 and T-6). Exits with the upstream itos
  fix, which no registry item holds.
