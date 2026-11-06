# Public session recovery

Slice-11 implements the settled recovery contract in
[recovery.feature](../../features/recovery.feature). This decision completes the
details reserved to slice-11 in arc42's **Building blocks**, **Recovery** and
**Decisions** sections; it adds no runtime platform or isolation claim.

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
after fresh ownership and running-state observations. A missing intent name
without a concrete ID remains uncertain. Cleanup that wins before create is
durable and terminal: resumed run cannot create, start or overwrite that progress.

Completion requires freshly confirmed concrete-ID absence, a durable absent
intent and a durable stopped session. Repeated stop confirms and syncs existing
completion without rewriting it. A later explicit workload observation may fill
a previously null exit while retaining stopped state; it cannot change a known
exit. Stop's zero result denotes cleanup, independently of workload success.

Fresh status/stop use the effective checkout and host user to select a project.
Foreground cleanup retains arc42's existing original-binding rule even if a
worker changes checkout metadata. Both require the original protected snapshot,
host identity, pinned socket, daemon and concrete resource ownership. Current
requests and current approval do not authorize cleanup.

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
