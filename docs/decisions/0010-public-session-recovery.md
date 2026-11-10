# Public session recovery

Slice-11 implements the settled recovery contract in
[recovery.feature](../../features/recovery.feature). This decision completes the
details reserved to slice-11 in arc42's **Building blocks**, **Recovery** and
**Decisions** sections; it adds no runtime platform or isolation claim.

Slice-16's [admission decision](0014-checkout-admission.md), following coordinator
q-28, supersedes this decision's pre-create uncertainty rule. A durable reserved
intent permits cancellation under the operation lock before create is issued;
an issued or uncertain create retains the concrete-ID recovery requirements below.
RECOVERY-25 now covers proven cancellation; RECOVERY-14 retains uncertain create.

Each invocation allocates a cryptographically random 32-byte public ID independently
of its existing operation token. The seven-field record at
`sessions/<project>/<id>/record.json` refers to the original protected intent and
snapshot. It contains lifecycle progress, a nullable observed workload exit and a
nullable typed failure, never workload inputs or output. Existing intent-only
artifacts are preserved without migration or adoption.

Persistent `controller.lock` and `operation.lock` files separate controller
presence from mutation exclusion. The exclusive controller lease lasts for run's
lifetime; status probes it with a nonblocking shared lock. The operation lock
serializes run transitions and stop, including engine mutation and durable
completion, but is released across foreground wait and stream drain. Project
storage locks cover short transactions only. Paths, bytes, owners, modes and
lock identities remain checked; no stale-PID inference or state repair is added.

Status is read-only and has a ten-second invocation budget. Stop has thirty
seconds total, requests SIGTERM with ten seconds of grace and escalates only
after fresh ownership and running-state observations. After create may have been
issued, a missing intent name without a concrete ID remains uncertain. Cleanup
that wins before create is
durable and terminal: resumed run cannot create, start or overwrite that progress.

Completion of an issued create requires freshly confirmed concrete-ID absence,
a durable absent intent and a durable stopped session. Repeated stop confirms and syncs existing
completion without rewriting it. A later explicit workload observation may fill
a previously null exit while retaining stopped state; it cannot change a known
exit. Stop's zero result denotes cleanup, independently of workload success.

Fresh status/stop use the effective checkout and host user to select a project.
Foreground cleanup retains arc42's existing original-binding rule even if a
worker changes checkout metadata. Both require the original protected snapshot
and host identity. Issued creates also require the pinned socket, daemon and
concrete resource ownership. Current requests and current approval do not
authorize cleanup.

Recovery reports schema-1 facts and typed, sorted problems; incomplete run prints
`itos container stop <actual-id>` with same-project guidance. This supersedes
OFFLINE-15's temporary retry ban and OFFLINE-16's intent-only diagnostic expectation
without changing their result precedence or marker exclusions.
The earlier preflight preservation assertions now also require these exact
project-scoped registrations and protected empty locks, one per created worker.
Their existing approval, snapshot, checkout and unrelated-file checks remain.

The local scenarios exercise disposable filesystem and HTTP fixtures; shared
contracts exercise I/O-free fakes and adapters. Scenarios naming both backends
also run the independent command-level memory matrix, including strict records,
mixed discovery, ownership, registration and controlled-clock deadlines.
Native controller loss, retry and
two-project evidence remains CI-only. The coordinator supplied assertion-level
red evidence for RECOVERY-21/22/23 at `fea67dd` in run `42180081092`; the coordinating
loop must obtain green evidence and changed-code proof at the final pushed head.

## Implementation evidence

The local step checkpoints are `9b4d4a8`, `e7e415d` and `3d42e97`, committed
while the scenarios remained `@wip`. Against the pre-implementation product at
`3d42e97`, with those tags temporarily live, all 106 local examples failed at
their behavior assertions: 213 setup/action steps passed, 106 assertions failed
and 158 subsequent steps were skipped. No step was undefined or failed in setup.
The first failed assertion for each local ID is summarized below; outline rows
failed at the same assertion with their respective expected values.

| RECOVERY ID | Failed assertion before implementation |
| --- | --- |
| 01 | Empty status succeeds with no owned sessions or problems. |
| 02 | Status reports the specified lifecycle, resource, cleanup and observed exit. |
| 03 | Releasing the controller lease changes presence and the exact stop candidate. |
| 04 | Partial status retains safe entries and returns sorted typed problems. |
| 05 | Each create follows durable registration and intent, with independent IDs. |
| 06 | Registration refuses with its specified cause before launch or invalid retry. |
| 07 | Both stops confirm durable cleanup and preserve known or unknown exit. |
| 08 | Run preserves exit precedence and its printed registered retry succeeds. |
| 09 | Status and stop use the original fingerprint and protected snapshot. |
| 10 | Invalid invocation produces the exact recovery usage envelope. |
| 11 | Foreign ownership produces not-owned without mutation or adoption. |
| 12 | Unsafe/corrupt material produces its typed refusal without unsafe access. |
| 13 | Substituted authority is refused before the barred destructive operation. |
| 14 | Invisible uncertain create produces temporary uncertainty, never absence. |
| 15 | Interrupted completion follows actual removal without claiming durability. |
| 16 | Overlapping stops complete through one serialized mutation sequence. |
| 17 | Stop completion prevents resumed run from starting or recreating work. |
| 18 | Independent sessions and equal-basename projects remain independent. |
| 19 | Unavailable observations retain their cause within whole-call budgets. |
| 20 | Status gives useful original facts and retry without synthetic markers. |
| 24 | Both help commands succeed and describe the public recovery contract. |
| 25 | Stop before create and later status retain failed/unknown recoverable state. |

All 106 local examples now pass with their tags live, as does the complete local
feature suite and the affected command, session, approval, run, status and stop
Go tests. Fault fixtures retain positive controls; actual filesystem durability
and lock exclusion use observed syscalls and competing processes. The inherited
foreground fixture now gives ordinary SIGTERM its normal terminating behavior;
explicit ignored-signal examples still exercise escalation.

The coordinator's [native red run](https://github.com/donvargax/itos-container/actions/runs/42180081092)
at `fea67dd` failed RECOVERY-21 at executable retry/cleanup, RECOVERY-22 at fresh
status discovery, and RECOVERY-23 at independently targeted stop processes, after
fixture smoke passed. Those three scenarios are live for CI; neither native green
evidence nor changed-code mutation/coverage proof is claimed by this local work.

## Cleanup contract integration after the slice-13 merge

The merged OFFLINE-15 scenario required public recovery, but its two renamed
steps still had the intent-only phrases and its test assembly omitted the
registry. The eleven examples passed their cause/exit assertions and then
failed on undefined steps. Completing the steps made all eleven fail on both
backends at the missing registered-ID assertion, with no undefined steps.
That checkpoint is committed in `acb0ec0`.

The cleanup fixture and its fault-free controls now use registered run with
the registry fake or production disk registry. Observers export actual public records and
move the unclassified adapter fault to the registry transition boundary.
The assertions bind the printed retry to the original project and intent,
check protected registration and empty locks, and preserve all existing exit,
ownership, unrelated-state and marker exclusions. Only those exact new paths
are admitted to the earlier preservation check. All eleven examples now pass
on both backends; no product behavior, expected result or scenario step was
weakened to accommodate the assembly. Earlier intent-only launch contracts
continue to exercise that API. The complete non-engine feature suite also
passes. Native evidence remains the coordinator's CI responsibility.

## Cleanup after a launch reopen deadline

RECOVERY-26 covers a deadline while reopening the registered session between
create and start. A failed open is not evidence that stop already won: run must
attempt owned cleanup under its fresh teardown budget. Only a safely loaded
record showing a superseded launch selects the observation-only path.

The complete step checkpoint is `a98329e`, with the scenario still `@wip`.
Running it live against that implementation failed at the cleanup assertion:
the created worker remained. After distinguishing a superseded launch from an
ordinary incomplete operation, it passes with confirmed removal, durable
intent/session completion and unknown workload exit. The controlled-clock
domain regression also expires the actual 120-second startup budget after
create and requires cleanup without a start or fabricated workload result.

All 26 selected local examples in RECOVERY-07/08/17/25/26 and OFFLINE-15 pass,
along with the registered-run domain tests. The eleven OFFLINE-15 examples
also passed before this fix, confirming the reported undefined-step failure
predated the current implementation. These are non-engine fixtures; native
green evidence and changed-code proof remain with the coordinating CI loop.

## Native preservation and interrupted launch observations

The coordinator reported OFFLINE-06/07/13 preservation failures and RECOVERY-22
discovery failure at `a355e47` in CI run `42199501038`. The native preservation
assertion still counted the required public registration as an unrelated host
change. It now requires the exact completed record bound to the original intent,
an independent public ID and protected empty locks before admitting only those
new paths. Existing paths and every other addition remain in the comparison.
Disposable-file controls reject missing registration, unrelated edits, extra
session paths, unsafe locks and changed completion facts; they use no engine.

The controller can die after engine start but before publishing its running
phase. Fresh status now reports an owned running worker as running even when
the record still says preparing or ready. It remains read-only, retains unknown
workload exit and preserves stopping/failed progress. This applies the existing
fresh-observation contract without changing the shared termination policy.

RECOVERY-27's pending step checkpoint is `40f6c15`. With the tag live before the
fix, both examples failed at the lifecycle assertion, reporting preparing or
ready instead of running; all eight RECOVERY-02 controls passed. Both new examples
and the selected existing status, controller-loss, stop and cleanup scenarios
pass after the fix. Domain controls preserve cleanup phases and read-only state.
Native green evidence and changed-code proof still require the coordinator's
CI run at the pushed head; no local engine or mutation run was performed.

## Usage refusals and signal-target regression checks

Recovery usage errors now honor `--json=true` and `--json=false`, including
environment overrides and leading switches. Equals values remain invalid switch
syntax under the shared CLI contract; their refusal uses the requested format.
Leading switches stay in the selected command's scope, so mixed spellings cannot
lose the recovery-specific rule or schema. Parsing still stops at `--`.

RECOVERY-28's pending step checkpoint is `c60b694`. With its tag live before the
fix, all twelve examples failed at the usage-format assertion; setup and action
steps passed. After the fix, all twelve pass, alongside the selected recovery,
run and CLI regressions: 64 scenarios and 416 steps. The command and session
domain/SDK tests, native-registry preservation controls and local syscall-observer
controls also pass. Storage tests required execution outside the agent sandbox,
whose UID mapping presents root-owned ancestors as UID 65534; the ownership
checks and disposable fixtures were unchanged.

The signal/exit audit found no product path that signals a host PID or process
group. Shared termination sends explicit signals through the engine's full-ID
container API. Regression checks now assert the actual SDK signal targets and
exercise stop with observed PIDs -1, 0 and a positive value, a live controller
lease, an unrelated worker, and both SIGTERM and escalation paths. The unrelated
worker and controller lease must survive. These local checks do not replace the
coordinator's native engine run or changed-code proof at the pushed head.

## Exact JSON environment inheritance

Status and stop now use the same output preference function for successful
parsing and usage refusals. Their Kong flags no longer decode `ITOS_JSON` as a
boolean: only literal `1` inherits JSON. Other values keep plain output, and
explicit `--json` or `--no-json` selects the format on either path. This also
prevents arbitrary environment text from rejecting an otherwise valid command.

RECOVERY-29's pending step checkpoint is `36034df`. With its tag live before
the fix, both examples reached the format assertion and failed because successful
status and stop emitted JSON for `ITOS_JSON=true`. After the fix, both pass
across 114 invocations covering successes, parser refusals, argument refusals,
boolean aliases, invalid text and explicit overrides. Selected regressions in
RECOVERY-01/02/07/10/24/28 also pass. The command usage tests, full-ID SDK signal
checks, controller/unrelated-worker preservation checks and local syscall-observer
controls pass. No additional host-signal path was found in the audit. Native
engine validation and changed-code proof remain with the coordinating CI loop.

## Review corrections for deadlines and concurrent completion

The Recovery section in arc42 now makes explicit three existing contract details.
Stop preserves a temporary inspection timeout when its original thirty-second
budget expires after lock acquisition; it does not attempt a progress write
using expired storage authority. A failed controller lease probe remains an
unknown observation, which status reports, and does not veto explicitly requested
cleanup. Stop still validates the original record and full worker identity before
every destructive operation.

Run now represents wait completion separately from observing a workload exit.
After stdin closes, a concurrent stopped/absent record for the same full worker ID
can settle reconciliation with an unknown exit. Remaining output is drained, known
nonzero exits retain their precedence, and null stays null. Contradictory intent
progress or a different worker ID cannot settle the wait.

Slice-24's [unknown workload exit decision](0015-unknown-workload-exit.md)
supersedes this section's original successful-run interpretation of concurrent
completion, including RECOVERY-32 and RECOVERY-35. Successful cleanup with an
unknown workload exit returns wrapper 75 and `run.workload-exit-unknown` unless
another failure already determines the result. It does not report a wait or
cleanup failure, suggest a cleanup retry, or write a synthetic exit/last problem.
Stop and status still succeed. The red/green account below records slice-11's
historical correction, not evidence for that superseded exit-zero expectation.

RECOVERY-30/31/32 were committed pending in `a3d3136`. Running them live before
the fixes failed all three at their Then assertions, with all six setup/action
steps passing. The observed failures were respectively recovery.not-owned/exit 1
after deadline expiry, recovery.temporary/exit 75 without attempting owned cleanup
after a probe error, and run.execution-unavailable/exit 75 after stop had completed
with unknown exit. All three pass after the changes. Their command-level controls
retain retryable ownership, status probe diagnostics, foreign-resource refusal
and no repeated mutations or fabricated exit.

The selected RECOVERY-01/02/03/07/13/17/19/25/26/30/31/32 and OFFLINE-15
regressions pass: 49 scenarios and 254 steps. The session domain, SDK and command
tests pass too, including known/unknown exit, drain-failure and contradictory
completion controls. Disposable filesystem tests again required execution outside
the agent sandbox's UID mapping; no ownership check or fixture was weakened.

The signal/exit audit still finds engine termination targeting full container IDs,
not host PIDs or process groups. Existing SDK-target and controller/unrelated-worker
regressions remain the local checks; no local engine or mutation run is authorized.
The coordinating loop must validate the actual pushed head's native scenarios and
changed-code proof before closure.
