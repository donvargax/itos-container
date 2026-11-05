# Select named red scenarios through manual CI dispatch

Accepted for task T-21, 2026-11-04.

## Problem

The disposable evidence-branch procedure in `features/README.md` required a `test`
commit to remove `@wip`. The scenario-moves gate refuses that transition, while
ordinary selection excludes `@wip` and draft PRs skip automatic CI. Real-engine
behaviour therefore had no usable route to assertion-level red evidence.

## Decision

Supersede the tag-removal instruction for CI red runs in
[arc42, Future behaviour evidence](../architecture/arc42.md#future-behaviour-evidence).
Use the existing workflow's optional `red_scenarios` string input. The coordinator
publishes an item branch containing complete step definitions and these harness/workflow
changes, then dispatches exact IDs without changing feature files or smoke entries.
[The feature guide](../../features/README.md#manual-red-first-evidence) gives the command
and the evidence record.

Accept whitespace-separated complete tags matching `^@ID-[A-Z]+-[0-9]+$` and deduplicate
them. Reject malformed or unknown IDs and a simultaneous `-scenarios` expression before
build/setup. Interpret only validated exact matches, retaining parsed tag inheritance,
outline rows, engine tier and platform restrictions. Remove the `@wip` exclusion only
for named IDs in memory. A fully excluded list explicitly skips with IDs, reasons and
count; it is never evidence. Failed, undefined, pending, skipped or lost selected cases
cannot pass, including in the non-engine manual jobs.

Read the input only after CI, GitHub Actions and manual-dispatch context are established.
Supply it only as step-local environment data to the engine and existing macOS/Windows
Tests steps. Ordinary Linux, proof, preflight and fixture helpers receive none. Preserve
the exact engine switch and actual Linux amd64 process checks; red input grants no engine
access. These are accidental-execution guards, not a hostile-code isolation boundary.
Whole-suite manifest coverage reruns skip an active red selection to avoid unrequested
product cases. Synthetic harness subprocesses and product children strip the variable.

No input retains ordinary live-only selection. Keep the five jobs, event/draft guards,
commands, pins, permissions, budgets, fixture preparation and unconditional owned cleanup.
This explicitly supersedes T-20's no-dispatch-input and five-variable engine-environment
invariants with one optional input and one additional step-local variable. All other
fixture, engine-switch and cleanup checks remain enforced.

## Evidence and consequences

The compiling regression commit `7855625` failed at executed-ID assertions: ordinary
selection ran `@ID-MEMORY-01` instead of requested future `@ID-MEMORY-03`, and engine
selection ran `@ID-MEMORY-02` instead of `@ID-MEMORY-04`. Invalid and partially unknown
lists were accepted, incomplete selections passed, synthetic subprocesses inherited the
input, and workflow assertions reported missing schema and all three bindings. After
implementation, `go test -count=1 -run '^TestEngine' ./features` passed, including the
existing selection, workflow and fixture controls. These are memory/static regressions;
no real engine ran and they provide no runtime isolation or host-support proof.

The coordinator records the resolved tested SHA, IDs, run URL, runner/tier, outline row
and failing Then assertion. Setup/infrastructure failures, undefined steps, excluded or
empty selections and passing runs are not red evidence. Preserve actual failed dispatches;
do not force failure, ignore failures or treat a failed red run as a merge prerequisite.
After implementation removes `@wip`, ordinary CI must pass at the actual pushed head with
no red input. Agents commit and stop; the loop alone publishes, dispatches and closes.

No product behavior, dependency, service, supported host or scenario inventory changes.
The existing Go standard library, pinned godog parser and YAML test parser suffice.
