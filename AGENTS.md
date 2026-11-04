# Working rules

Read [docs/charter.md](docs/charter.md) first: it selects what may be built from the binding
[pilot contract](docs/plans/container-pilot.md). Then read [PLAN.md](PLAN.md), including its
[milestone limits](PLAN.md#milestone-limits). Those authorities bound this project's planned
`itos container` extension and offline owned-run milestone. The inherited counting CLI works;
container commands and isolation are not implemented yet.

The person's session coordinates with `itos go`. Follow the assigned brief and role; read
`itos guide work` and `itos guide commit` before working and committing. Implement directly,
without subagents. Read the registry with `itos work`; respect its ownership and never edit
owners or statuses by hand. Leave coordinator-owned lifecycle actions to the loop; a drafter
does not take an item. Report gaps and follow-up ideas with why they matter to the coordinator.

## Changes and commits

- Specify behavior before building it. Features and fixes follow black-box scenarios. Commit
  their step definitions red first in a test commit while scenarios remain `@wip`; run them
  locally with `@wip` removed and see each fail at its behavior assertion before implementing.
  Undefined steps are not evidence. Never add unspecified behavior or bend scenarios to code.
- Foundation and other non-behavior work name a ledger task and its checks. Decide each
  commit's type and paths before editing.
- Commit with `itos commit -F <file>`, never `git commit`. Bodies explain what changed and why;
  no body line starts with a word and a colon, or with `with #,`.
- Read `git status --short` before staging; stage only explicit file paths, never a whole folder,
  `.`, or with `git add -A` or `-u`.
- Never bypass hooks, force-push or rewrite published history. Never pipe a commit or push.
  Agents commit; only the coordinating loop pushes, marks pull requests ready and closes items.
- Gates run through their configured hooks and CI, not manually before committing. Fix a valid
  check's failure in the work, not by weakening the check; stop on a wrong gate.

## Code and isolation boundaries

- Keep thin entry points over domain-owned behavior through ports. The domain knows no UI,
  filesystem implementation, JSON or exit codes. Use our fakes in its tests, never mocks.
- Look for a maintained library before building what one likely solves; settle the choice and
  reason in the spec and pin its version. The count example uses Go's standard-library
  whitespace tokenization, not a custom word parser; preserve its Files port and in-memory fake.
- Code differing by OS takes the OS as a value. Build tags hold only a call unavailable on one
  OS, with no mutation site; never branch on runtime.GOOS inside logic.
- The additive counting property uses newline-terminated pieces. Arbitrary concatenation can
  join words and is not additive.
- The domain reaches the container engine through a port. Domain tests use an engine fake;
  real-engine scenarios run only in CI. Agents never receive the Docker socket.
- Assume hostile checkout code and tool execution. Hooks and these instructions are workflow
  safeguards, not isolation. Claim container-host support only with passing runtime evidence;
  inherited Go checks and releases do not supply it. Follow the charter's scope and stop for
  open security decisions rather than inventing protections.
- No credentials in answers, files, fixtures or logs. Go vulnerability checks are not credential
  scans. Never weaken gates or scenarios to obtain a pass.
- Preserve the counting example and existing checks. This project is AGPL-3.0 licensed with
  attribution to donvargax; see [LICENSE](LICENSE) and [CONTRIBUTORS.md](CONTRIBUTORS.md).
  The owner authorized copied infrastructure from [itos](https://github.com/donvargax/itos) and
  [itos-template](https://github.com/donvargax/itos-template) under AGPL-3.0 here. Preserve original
  projects' and dependencies' licenses and notices; that permission does not relicense them.

## Finishing

Before handing code changes to the loop for its final push, record changed-code proof with the
pinned itos-cc, all-tests, fail-uncovered and no-annotate; commit project-local `.metrics/mutate/`
evidence by explicit path. Use the parent of the item's actual first commit, specs included,
not an assumed take commit. Kill survivors with tests; only truly equivalent mutants may be
excepted with a reason for the person to review. Uncovered mutants and executable code need
tests, never exceptions. Never reshape code merely to remove mutation sites, hand-edit evidence
or copy another project's cache. Documentation-only work touches no `proof.code` path and must
not fabricate mutation results; a passing cached sample is not a fresh foundation baseline.

Report commits, checks, behavior scenarios' failing-then-passing evidence when applicable, and
remaining gaps. Stop after committing: the loop owns push, CI watching and closure. It closes
an item only after the actual pushed head is green and required proof passes. Generated rules
below describe this branch's actual configuration; preserve the block unchanged.

<!-- itos:begin -->

## itos

Stage only your own files, by explicit path: untracked files you did not make are someone else's work in progress.

<!-- itos:end -->
