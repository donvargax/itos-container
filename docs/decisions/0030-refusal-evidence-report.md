# The public refusal evidence report

Accepted for slice-28, 2026-11-17. This implements the refusal-evidence half of
the charter's second-milestone egress step over T-51's streaming writer and
slice-27's bound run. No new library, service, key or signature is introduced;
the report extends the existing `RefusalLog`/`RefusalCopy` contracts, approval's
`SessionLogs`/`StagedLog`, and the shared porttest contract that already supplies
`SessionID` and the one ready-record table.

`features/egress_lifecycle.feature` is the binding specification: the report is
`refusal_log {path, state, records, missing}` inside `sessions[].egress`, read
from durable evidence, never a live proxy or a controller's memory. `path` is the
absolute protected destination `sessions/<lookup>/<id>/egress-refusals.jsonl`.
`state` is `pending` until a committed copy exists, `stored` for a copy without
observed gaps, `gapped` for one with gaps, `unknown` when publication or its
evidence cannot be confirmed. `records` is null for pending/unknown, otherwise
the number of published lines (log-full counts, ready does not). `missing` holds
ordered inclusive `{first,last}` sequence ranges. Plain status prints the same
path quoted, the state and the stored count or ranges, with approval's
control/C1/bidi escaping; JSON keeps the filesystem identity unchanged. Offline
entries omit `egress`, so they omit the report. Status stays read-only and never
claims a complete history; the lost-tail limitation stays verbatim.

## Binding the report to its copy

Commit derives the report from the bytes it staged: every complete line's `seq`
is observed before ready filtering, so a missing initial `seq=1` is a gap while
ready's deliberate omission from the published file is not; gaps are the
inclusive ranges the source never delivered up to its last observed sequence;
an empty valid copy reports a clean zero. The digest, record count and ranges
are published as `egress-refusals.json` beside the log through the same staged
handle: the log's own fsync, rename and parent fsync come first, the report's
rename and parent fsync after. A copy interrupted at the last durable step
exposes its log without claiming a committed report, which reads back as
`unknown`, never a clean zero; a report that does not describe the log beside
it, by digest, is also `unknown`. Storage admits both files only through the
pinned session directory; a second link or a foreign file is unsafe, never
evidence.

`RefusalLog` gains `Read`, so a fresh process derives the report from the
committed files alone. A retrying stop never rewrites a committed copy: when
durable evidence is stored or gapped, cleanup keeps the published bytes and
their inode, and the copy is not begun again.

## Diagnostics

A committed gap is its own diagnosis, not a cleanup failure: run reports
`run.refusal-log-gap` and status/stop report `recovery.refusal-log-gap`, each
exit 1, naming the session, the quoted protected destination and the inclusive
ranges, with guidance that lost records cannot be recovered by retrying. An
observed nonzero workload exit and a cancellation still take their own exit
precedence; a gap keeps its exit 1 over a separate cleanup diagnosis, and both
diagnoses remain when removal also fails. Cleanup may be complete alongside
gapped evidence, and repeated stop preserves the gap without rewriting the file
or finding new cleanup work.

## Scope and affected sections

q-56 stands: the refusal hostname stays verbatim under decision 0022, the
hostname-marker absence probe belongs to `p1-platform-log-redaction`, and this
report's payload-field exclusions do not close platform AC-36. arc42's refusal
log, building-block and decision-index rows now name the report;
[CLI.md](../CLI.md) documents the two rules and the status lines. The
native-run measurement of the full log's copy and commit times belongs to
CI's cap scenario, with no product flag or environment seam: the harness alone
selects the unmodified-budget product for the scenarios that judge those
budgets.
