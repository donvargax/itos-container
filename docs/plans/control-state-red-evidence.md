# Slice-21 red-first handoff

Coordinator q-31 (2026-11-07, approximately 16:25 UTC) accepted the red evidence
below and authorized implementation. All 45 examples use recording HTTP engine
fixtures, not `@engine`; no CI red run is required. The production change and
[architecture decision](../decisions/0021-control-state-publication.md) implement
the common account-home transaction, durable catalog and separation checks. All
45 examples now pass locally with `@wip` removed. CI proof and closure remain with
the coordinating loop; this adds no runtime-isolation claim.

## Executed red evidence

At `14d525c802fadf623910536efc3a1daba97908e1`, temporarily remove `@wip` from
this feature and run:

```sh
go test ./features -count=1 -run='TestControlStateTrace|TestFeatures' -scenarios='^@ID-CONTROL-'
```

All 45 examples failed at their first behavior assertion. All 97 setup/action
steps passed, 45 assertion steps failed and 142 later steps were skipped. There
were no undefined steps or setup failures. The tags were restored after the run;
no scenario text or expected behavior changed.

| IDs | Examples | First assertion failure |
| --- | ---: | --- |
| CONTROL-01–05 | 19 | Doctor/approve returned 0 instead of `approval.state.exposed`, exit 1. The worker/reservation checks succeeded before testing the refusal. |
| CONTROL-06 | 1 | The worker acquired admission while approval publication was held. |
| CONTROL-07 | 11 | Both commands returned 0 instead of the specified tracking refusal (exit 1 or 3). |
| CONTROL-08 | 4 | Approval returned 0 instead of `approval.state.write`, exit 75. |
| CONTROL-09 | 2 | No location binding matched the independently observed filesystem identity. |
| CONTROL-10 | 1 | A bootstrap directory was created before account-home exclusion. |
| CONTROL-11 | 7 | Approval returned 0 instead of `approval.state.write`, exit 75. |

`TestControlStateTrace` passed its positive observation and seven bootstrap-fault
controls in the same run. The parent-fsync fault requires an existing coordination
directory, keeping ordinary approval storage healthy on the baseline. Concurrent
processes have separate trace/PID filenames; CONTROL-06's earlier journal collision
was fixed before this final red run.

The following neighboring run also passed after the test changes:

```sh
go test ./features -count=1 -run='TestControlStateTrace|TestFeatures' -scenarios='^@ID-OVERLAP-(01|02|04|10|11|12)$'
```

Each test commit passed its configured hooks, including lint and vulnerability
checks. No local mutation run or generated coverage evidence was produced. The
filesystem scenarios ran outside the agent sandbox because its remapped ancestor
ownership would cause an unrelated protected-path refusal. All product commands
used disposable fixture state and recording Unix HTTP Engine API servers, never
a real daemon or the person's approval state.

## Test assembly and implementation

`cmd/itos-container/control_state_probe_test.go` supplies a temporary account-home
host port in the test executable. The production account-home port now consumes that observation. The same
isolated account lookup is supplied to the other approval, run and public
recovery test assemblies. `features/control_state_trace_test.go` extends the existing
child-only syscall observer to record directory creation, locks, publication and
fsync, and inject only the specified filesystem failures. No production testing
flag was introduced.

The binding oracle currently represents schema 1 with `uid`, `path`, `complete`
and ordered `ancestors` entries (`path`, `device`, `inode`). `self.json` binds the
fixed tree including its coordination-directory identity; other filenames hash
the JSON-encoded absolute managed path. The implementation uses this binding format and validates its identities,
required fields and unique keys. The behavioral scenarios remain authoritative.

Shared memory/disk contracts now cover read-only inspection, exposure, publication
failure/retry, retained protection, cross-root exclusion and replacement. Adapter
tests cover bootstrap filesystem failures, malformed bindings and account lookup.
The previously skipped positive controls, post-recovery publication, retry durability,
read-only inspection and no-rewrite observations now run successfully.

Neighboring fixtures now provide disposable account homes and independently seeded
catalogs when their premise is current approved session state. Multi-project fixtures
share one account home. CONTROL retains its explicit missing-catalog and legacy
fixtures. The plain diagnostic observer recognizes doctor's existing CLI prefix
without relaxing exact rule/message/fix checks. An occupied-public-ID fixture now
uses valid completed ownership, keeping its assertion focused on collision at
registration instead of malformed-state rejection during inspection.

No local mutation run or generated proof evidence is committed. The loop owns the
actual pushed-head CI proof, push, review and closure.

The final affected-scenario run passed all 560 selected examples (including all
45 CONTROL examples), with the observer controls included:

```sh
go test ./features -count=1 -run='TestControlStateTrace|TestFeatures' -scenarios='^@ID-(CONTROL|APPROVAL|OVERLAP|RUN|RECOVERY|DOCTOR)-'
```

Affected approval/session storage, domain, fake, account lookup and command-assembly
unit tests also passed. These local filesystem checks used unremapped host ownership
and exclusively disposable state plus recording HTTP engines. Configured gates run
through the feature commit's hooks; fresh changed-code proof remains CI-only.

## CI fixture regressions after integration

At `11b539f`, both doctor harness controls failed before reaching their assertions:
their worlds omitted the module root, so the isolated session-probe build searched
for `features/cmd/itos-container`. Each fixture now supplies `moduleRoot()` as the
scenario harness does. The journal and tree-change assertions are unchanged.

The catalog symlink fixture renamed its original binding to a `-old` filename
inside the catalog. This also introduced an unknown catalog entry. Depending on
filesystem directory order, inspection correctly refused that entry as
`state.invalid` before observing the symlink's `state.path`. The original fixture
failed 3 of 10 repetitions with temporary state on the checkout filesystem, while
40 repetitions on `/tmp`'s tmpfs passed. Move the symlink target outside the catalog
so this row exercises only substitution; the separate unknown-name row continues
to require `state.invalid`. Production refusal rules and assertions are unchanged.

After these fixture corrections, all `TestDoctor*` controls and all cases of
`TestControlBindingsRefuseMalformedOrReplacedEvidence` pass. The corrected symlink
case also passes 40 repetitions on the checkout filesystem. Filesystem checks use
unremapped ancestor ownership: the agent sandbox otherwise produces unrelated
`state.unsafe` refusals. All command state is disposable and engine observations
use recording HTTP fixtures. No real approval state, daemon or mutation proof is
used by this follow-up.

## Private coordination lock revision (q-35)

The coordinator's 2026-11-07 decision replaces the publicly readable account-home
lock with a validated private inode. Commits `3d7fa1b` and `76c7ec3` add pending
CONTROL-12/13 steps before implementation. The second preserves run's existing
call-timeout diagnostic; doctor and approve require approval.state.busy, all exit
75. The deadline fixture uses a 15 s outer guard to observe the product's 10 s
limit, rather than the ordinary doctor harness's 8 s guard.

All six new examples failed at their behavior assertions on the original code.
A further run with current steps and a temporary Go overlay of `76c7ec3` production
code also includes the coordinator-revised CONTROL-10. All seven fail at assertions:
20 setup/action steps pass, seven assertions fail, and five later steps are skipped.
CONTROL-10 observes the forbidden home flock; CONTROL-12 observes blocked commands
or no worker activity; CONTROL-13 receives exit 1 instead of the retryable exit 75.
There are no undefined steps. The home-lock helper holds a real read-only descriptor
in another process, modeling the capability available to another UID on a 0755
home; it does not change UIDs or require a privileged process.

After implementation, all 51 CONTROL examples pass, including bootstrap fault and
retry durability, no-write doctor inspection, retained identities, admission
exclusion, both new lock outlines and the revised private bootstrap ordering.
Adapter tests additionally cover unsafe lock metadata, missing/replaced lock
inodes, descriptor validation failures, concurrent mkdir creators converging on
one lock, and reservation evidence appearing during bootstrap. Shared disk/fake
contracts require retryable contention. Approval/session packages, run preflight
and command assembly pass their unit tests. The full affected local selection
(CONTROL, APPROVAL, OVERLAP, RUN, RECOVERY and DOCTOR), including syscall observer
controls, passes in 260 s. All state is disposable and all local
engine observations use recording HTTP fixtures.

The supplied engine failure at `daa7f80` is a five-minute suite timeout, not a
failing CONTROL assertion. Registry item `p1-engine-suite-within-its-limit` already
records this failure on main (`b2ad2ad`, run 42392045565) even with all 49 scenarios
passing. LIFECYCLE-13 deliberately observes a quiet worker for 125 s and shares
that control; removing this wait would weaken its contract. The supplied log tail
contains transport wait stacks without identifying the timed-out scenario. No
real-engine run or timeout-fix claim is made locally; the coordinator needs the
full failed log and fresh CI evidence. Gates and their limits remain unchanged.

## Full feature-package timeout at 4f2adf1

PR run `42399749023` also failed the ordinary platform feature package at
600.815 seconds. This is distinct from the five-minute engine-job limit above.
The unchanged full local suite passes on tmpfs in 410.569 seconds. With temporary
files on `/var/tmp` (ext4), affinity restricted to two CPUs and `GOMAXPROCS=2`,
the same suite times out at 600.280 seconds after 808 passing scenario examples,
without an earlier assertion failure. Its active cleanup example had run for
only three seconds: serial accumulation of real filesystem I/O and required
deadline observations exhausts the package budget. Two local workers still
exhaust that budget while making further progress through recovery; the final
runner therefore uses four bounded workers.

The runner now overlaps four independent non-engine scenarios, retaining their
private processes, account homes, state roots, recording engines and assertions.
Engine and red-evidence runs remain serial. Compiled helpers are built under
exclusion and atomically published outside the fixture checkouts; incomplete or
failed builds cannot be reused by a concurrent scenario. Selection, coverage
collection, syscall observations and every existing timeout stay unchanged.

The final concurrency regression fails against a Go overlay of the original
runner because local fixtures never overlap (peak one instead of four). The
overlay adds only the worker-count constant needed to compile the current test.
It passes with the runner change, including an intentional assertion failure
that must still fail the suite, exactly-once execution of all six fixtures and
serial engine/red controls. The concurrency, atomic helper publication/retry
and selection regressions pass under the race
detector. These are harness checks, not new product behavior or native-engine
evidence; the accepted CONTROL red-first evidence above still applies.

The complete `go test ./features -count=1 -json` package now passes in 493.519
seconds under the same ext4 temporary-file, two-CPU and `GOMAXPROCS=2` conditions.
All 1,325 scenario examples pass, including the 51 CONTROL examples, along with
the package's coverage and harness controls. A separate real-fixture selection
of CONTROL-01/10, OVERLAP-01, APPROVAL-01 and MANIFEST-01 passes under `-race` in
11.687 seconds. All state is disposable and engines are recording HTTP fixtures.
No gate or scenario time limit was raised. Native-engine evidence and fresh CI
at the committed head remain with the coordinator.

The engine timeout discussion above is historical. The coordinator's
2026-11-07 20:40 UTC decision accepts T-32's landed limits: 8 minutes for the
engine step, 7 minutes for the test process and 13 minutes for the job. The
existing main merges through `9d4c91d` already include those limits. No old-run
log or timeout change is required from slice-21; the loop judges the fresh run.

## Peer-created private lock after unlocked inspection

Review thread PRRT_kwDOVkB0ws6050sF identified a writer rejecting a peer-created
control.lock as path substitution. Test commit `03ae9e4` adds pending CONTROL-14:
a consenting writer pauses after real unlocked reservation discovery, then an
independent process holds a newly created safe private lock. With `@wip` removed,
both setup/action steps pass and the behavior assertion fails because approval
returns approval.state.path (1) without attempting that lock.

Writers now revalidate their pinned directories before opening and acquiring the
peer's validated lock, then repeat inspection under exclusion. Read-only callers
still require an absent lock to remain absent. CONTROL-14 passes, proving actual
flock contention, publication after release, generation 1 and the unchanged lock
inode. The focused disk control tests pass, including new cases for peer
publication with absent or existing coordination directories, renewed reservation
checks, unsafe peer locks, replaced directories and read-only refusal. Before the
fix, the two safe writer cases and renewed reservation/unsafe-lock classifications
failed with state.path. No real approval state or container daemon is used.

## Run/stop cancellation journal ordering

The supplied CI failure observed probe-start before run-wait-canceled. The finite
wait adapter intentionally returns on cancellation without waiting for its
underlying goroutine; the fixture journal recorded cancellation in that later
goroutine. Parallel scenario execution made the missing synchronization visible.
Fifty ordinary fixture repetitions did not reproduce it, so a fake-time regression
delays that goroutine by one millisecond. Both stop schedules then fail the event
ordering assertion before the fixture fix.

The fixture now acknowledges the canceled wait in its journal before allowing
its storage probe to proceed. The wait remains bounded by the probe's context.
Production run behavior, scenario assertions, time budgets and the parallel
runner are unchanged. Both deterministic schedules pass twenty repetitions under
the race detector. The scheduling delay and drain use only synctest time, never
wall-clock sleeps or a real engine.

The complete `go test ./features -count=1 -json` run passes in 158.304 seconds:
all 1,326 scenario examples, including both RUNEXIT schedules and all 52 CONTROL
examples, plus the package's harness controls. Configured commit hooks pass lint
and vulnerability checks. The coordinator still owns push, fresh changed-code
proof, CI watching and closure; no local mutation or generated proof was run.

## Account-database provenance (PRRT_kwDOVkB0ws606rqO)

Test commits `7c7b1b8` and `c049376` add pending CONTROL-15. The read-only test
entry point calls the production host adapter, without the usual substituted
account home. Its executable explicitly uses CGO_ENABLED=0, matching the release.
A child-only syscall observer makes `/etc/passwd` empty or unreadable; two fresh
processes per example use different HOME, USER and LOGNAME values. No host account
file changes, real approval command, daemon or real control-state write occurs.

With `@wip` removed on `c049376`, both examples fail at the behavior assertion:
the lookup selects an unproven home with no failure instead of state.unavailable.
All four setup/action steps pass; two assertions fail, with no undefined steps.
The observer confirms the account-file faults before checking the result.

The host adapter now reads the fixed local account database directly and requires
a complete effective-UID record with valid numeric IDs and an absolute home.
NSS-only or otherwise unavailable local evidence refuses without consulting an
environment fallback. The feature spec, decision 0019 and arc42 record this source
and the standard-library choice; no dependency or account-environment mutation is
introduced.

Both CONTROL-15 examples pass after implementation, as do all 54 CONTROL examples
and the syscall observer controls:

```sh
go test ./features -count=1 -run='TestControlStateTrace|TestFeatures' -scenarios='^@ID-CONTROL-'
go test ./internal/engine/approval/host ./cmd/itos-container -count=1
CGO_ENABLED=0 go test ./internal/engine/approval/host -count=1
```

Adapter cases cover a valid local record despite conflicting environment values,
effective-UID selection, incomplete/invalid records, NIS directives, scanner/read
failures and descriptor closure. Command disk contracts require unremapped host
ownership; the sandboxed attempt reported unrelated ownership refusals, and the
same tests pass with unremapped ownership. CONTROL uses disposable state and local
recording HTTP fixtures. Fresh changed-code proof and pushed-head CI remain the
coordinator's responsibility; no mutation or coverage evidence was generated locally.

## Retained location inside a later checkout (PRRT_kwDOVkB0ws61FpuQ)

Test commits `81a8210` and `b5b1788` add pending CONTROL-16. P publishes a
managed location equal to or beneath W's checkout, using a different selected
state root and daemon. The fresh-run schedule starts W after P exits; the other
holds W after successful preflight until P finishes publication. The child syscall
observer preserves the barrier and verifies P's binding durability independently.
No Given runs a product command, and all state is disposable.

With `@wip` removed on `b5b1788`, all four examples fail at the first Then:
P's approval and durable binding succeed, but W reaches create and remains live
instead of returning exit 1 with the specified refusal. Eight setup/action steps
pass, four assertions fail and four preservation checks are skipped. There are no
undefined steps. Earlier fixture attempts are not red evidence: sandbox ownership
remapping prevented publication, the ordinary process helper dropped the barrier,
and an overly broad permission observer included ordinary checkout hooks when the
managed tree equaled W's root. The corrected committed observer fixes those causes.

The common transaction now checks every retained location against the candidate
root before granting read or write authority, including pending absent locations.
Run preserves the specified shared state.path preflight diagnostic; admission uses
its existing ownership-unsafe classification. All four examples now pass, including
zero registration or engine mutation and exact post-publication state preservation:

```sh
go test ./features -count=1 -run '^TestFeatures$' -scenarios '^@ID-CONTROL-16$'
```

The shared disk/fake contract also checks equality, ancestor containment, pending
claims, harmless textual prefixes and equal basenames without worker reservations.
The changed approval disk, approval porttest and run domain packages pass under
`-race`. That targeted check found a preexisting admission-test race: its goroutine
sent completion before closing the captured directory, allowing the receiver to
reassign it. Completion now follows close, retaining all exclusion assertions.
Neighboring session packages and command assembly tests pass. No race detector was
run over the whole feature package; no real engine, real approval state or local
mutation proof was used.

The full `go test ./features -count=1 -json` package passes in 291.694 seconds:
all 1,434 scenario examples and the harness controls pass, with zero failing tests.
Configured gates run through the final fix commit's hooks. Fresh pushed-head CI
and changed-code proof remain with the coordinator; the branch already includes
origin/main's settled engine time limits.

## Selected state inside catalog metadata (PRRT_kwDOVkB0ws61FzgB)

Test commit `ff3a49e` adds pending CONTROL-17 and its step definitions. Eight
examples select the coordination directory or a child as XDG_STATE_HOME, with
an absent or initialized catalog, through doctor or exact-fingerprint approve.
Every checkout, account home and selected state tree is disposable; the engine
is a local recording fixture.

With only `@wip` removed, all eight examples fail at the first Then, which requires
approval.state.path and exit 1. Each command instead exits 0: doctor reports missing
approval and approve publishes into the catalog namespace. All 16 setup/action
steps pass, eight behavior assertions fail and 24 later steps are skipped, with
no undefined steps. The first restricted-runner attempt is not red evidence:
it remaps /tmp ownership to UID 65534, triggering unrelated state.unsafe refusals.
Normal host metadata restores the existing CONTROL-10 baseline and produces the
assertion-level red above.

The shared disk/fake contract also failed before implementation. Direct write
transactions accepted the unsafe selection and changed state; disk transactions
then refused safe selections with state.invalid, reproducing account-wide damage.

The common transaction now refuses selected managed roots equal to or below
coordination before bootstrap, bindings or selected-tree creation. The fake follows
the same contract. All eight examples pass, including exact refusal inventories,
default-tree and catalog-prefix sibling approval, read-only matching inspection,
and preserved matching approval in another checkout.

```sh
rtk proxy go test ./features -count=1 -run '^TestFeatures$' -scenarios '^@ID-CONTROL-17$'
rtk proxy go test -race ./internal/engine/approval/disk ./internal/engine/approval/port/porttest -count=1
```

The focused feature selection passes in 4.253 seconds, and both changed packages
pass with race detection. The full `rtk proxy go test ./features -count=1` suite
passes in 262.360 seconds without race detection. Configured gates run through
the final fix commit's hooks. No real engine, real state folder or local mutation
proof is used. Fresh pushed-head CI and changed-code proof remain with the
coordinator; origin/main is already an ancestor of this branch.

## Selected state inside reservation collections (PRRT_kwDOVkB0ws61F-IT)

Test commit `fe55866738fd9430cf3dafae6577d826f8e61dad` adds pending CONTROL-18
and its steps. Its 24 examples select sessions or admission, directly or through
a child, in absent fixed trees, initialized fixed trees and previously enrolled
locations. Each selection is inspected by doctor or exact-fingerprint approve.
Every command uses disposable account and state trees and recording engine fixtures.

With only `@wip` removed, all 24 examples fail at the first Then, which requires
approval.state.path, exit 1 and invalid approval. Each instead exits 0: doctor
reports missing approval and approve publishes a matching approval. All 48
setup/action steps pass, 24 behavior assertions fail and 96 later steps are
skipped. There are no undefined steps or setup failures.

```sh
rtk proxy go test ./features -count=1 -run '^TestFeatures$' -scenarios '^@ID-CONTROL-18$'
```

The shared disk/fake contracts also fail before implementation: reads and direct
writes accept these selections, and writes alter state. The pending-claim case
reproduces the same failure before its managed root has been created.

The common transaction now refuses fixed-tree collection collisions before
bootstrap and checks discovered locations under the common lock before enrollment.
The fake implements the same contract, including pending claims. CONTROL-18's
24 examples and CONTROL-17's eight neighboring examples pass in 5.664 seconds,
including unchanged inventories and successful fresh commands using the default
tree, collection-prefix siblings, a catalog-prefix sibling and another checkout's
existing approval. The changed disk and porttest packages pass with race detection.

```sh
rtk proxy go test ./features -count=1 -run '^TestFeatures$' -scenarios '^@ID-CONTROL-(17|18)$'
rtk proxy go test -race ./internal/engine/approval/disk ./internal/engine/approval/port/porttest -count=1
```

The full `rtk proxy go test ./features -count=1 -json` package passes in
258.394 seconds without race detection: all 1,466 scenario examples and harness
checks pass, with no failing tests. Configured gates run through the final fix
commit's hooks. T-32's settled timeout change is already an ancestor of this
branch. No real engine, real state folder or local mutation proof is used;
push and fresh changed-code CI proof remain with the coordinator.

## Checkouts nested inside protected state (PRRT_kwDOVkB0ws61IKmj)

Test commit `980eeb1e2b1a172e6434b8a98fb1661881455c47` adds pending CONTROL-19
and CONTROL-20 with their steps. CONTROL-19 roots the checkout at the approvals
collection of the selected, fixed and previously enrolled managed trees and
inspects it with doctor or exact-fingerprint approve. CONTROL-20 roots the worker
at the approvals collection of another project's selected tree, then publishes
that project's approval before a fresh run or after run preflight. Every command
uses disposable account and state trees and recording engine fixtures; neither
scenario is `@engine`, so no CI red run is needed.

With only `@wip` removed and the production change reverted, all eight examples
fail at the first Then. The eight scenarios run 32 steps: 16 setup/action steps
pass, 8 assertions fail and 8 later steps are skipped. There are no undefined
steps or setup failures.

| IDs | Examples | First assertion failure |
| --- | ---: | --- |
| CONTROL-19 | 6 | `itos-container exited 0, not 1`: doctor reported a missing approval and approve published a matching record for a checkout inside the protected tree. |
| CONTROL-20 | 2 | The worker's preflight did not exit 1 (it ran until the fixture ended it with exit -1): admission was granted although the publisher's tree contains the worker, both on a fresh run and after preflight. |

```sh
go test ./features -count=1 -run '^TestFeatures$' -scenarios '^@ID-CONTROL-(19|20)$'
```

The shared disk and fake contracts also fail before implementation: nested
candidates are accepted against retained and pending locations, and against the
fixed and selected trees, on reads and direct writes.

The common transaction and the store's location selection now refuse a checkout
that equals, contains or lies beneath a protected tree, using one symmetric
whole-component comparison. The fake implements the same rule. All 20 control
scenarios pass locally, including CONTROL-19 and CONTROL-20 and the neighboring
CONTROL-15 through CONTROL-18, and the unit contracts pass with race detection on
the changed disk, porttest and domain packages.

```sh
go test ./features -count=1 -run '^TestFeatures$' -scenarios '^@ID-CONTROL-'
go test -race ./internal/engine/approval/disk ./internal/engine/approval/port/porttest ./internal/engine/approval/domain -count=1
```

No real engine, real state folder or local mutation proof is used. The full
feature suite is run without race detection; push and fresh changed-code CI proof
remain with the coordinator.

The full `go test ./features -count=1 -json` package passes in 284.165 seconds
without race detection: 2,426 tests and subtests pass, with none failing.

## Selection above an active worker (review PRRT_kwDOVkB0ws61Iu4r)

CONTROL-21 (doctor) and CONTROL-22 (approve) start worker W, then select a state tree
whose approvals collection is W's checkout. They add no step definitions: the existing
CONTROL-01 and CONTROL-02 steps run against the `above W root` fixture. With only
`@wip` removed and the production change absent, all 7 examples (3 doctor, 4 approve)
fail at the first Then, `itos-container exited 0, not 1`: doctor reported a missing or
matching approval and approve published a record into W's workspace. Every setup and
action step passed; there were no undefined steps or setup failures.

```sh
go test ./features -count=1 -run '^TestFeatures$' -scenarios '^@ID-CONTROL-(21|22)$'
```

The shared memory and disk control contract also fails before the change (an exposed
location was accepted on reads and writes, and the refusal wrote state) and passes
after it. All 22 control scenarios and the OVERLAP scenarios pass locally; the unit
packages pass with race detection on the changed packages only:

```sh
go test ./features -count=1 -run '^TestFeatures$' -scenarios '^@ID-CONTROL-'
go test -race ./internal/engine/approval/disk ./internal/engine/approval/port/porttest ./internal/engine/approval/domain -count=1
```

## Renumbered devices (review PRRT_kwDOVkB0ws61JLQ0)

CONTROL-23 rewrites only the recorded device of every ancestor in the fixed tree's
self-binding, an enrolled location's binding or both, in place, then runs doctor and
a matching exact-fingerprint approve. With only `@wip` removed and the production
change absent, all 3 rows fail at the first Then, `itos-container exited 1, not 0`:
doctor reported `approval.state.path` for a tree that had not changed. Every Given
and When step passed; there were no undefined steps or setup failures. After the
change the 3 rows and the CONTROL-07 refusal rows pass, and the disk unit case that
used to pin a changed device now accepts it while a changed inode still refuses.

```sh
go test ./features -count=1 -run '^TestFeatures$' -scenarios '^@ID-CONTROL-(07|23)$'
```

## Run under exposure (review PRRT_kwDOVkB0ws61JLZm)

CONTROL-24 starts worker W on a distinct daemon, then runs P with a matching approval
whose state tree is below, equal to or above W's checkout. Run has no JSON mode, so the
steps read its plain diagnostic. With only `@wip` removed and the production change
absent, all 3 rows fail at the first Then, "P run reports approval.state.exposed with
exit 1 without requesting reapproval": run printed `run.approval-invalid: run approval
invalid; use itos container approve`. Every Given and When step passed and W stayed
live; there were no undefined steps or setup failures.

The selection stage is not reachable through a stable feature schedule, since it needs
a worker admitted between inspection and selection. A domain unit test covers both
stages: it fails before the change for state.exposed at inspection and for
state.exposed and state.path at selection, and passes after it, with unsafe state
still classified as invalid approval.

```sh
go test ./features -count=1 -run '^TestFeatures$' -scenarios '^@ID-CONTROL-24$'
go test ./internal/slices/run/... -count=1
```

## Paths that are not valid UTF-8 (review PRRT_kwDOVkB0ws61J5AN)

CONTROL-25 names the selected state tree with a byte that is not UTF-8, once through
XDG_STATE_HOME and once through the HOME fallback, for doctor and approve. CONTROL-26
gives the account home such a byte. With only `@wip` removed and the production change
absent, all 6 examples fail at their first Then: the four selected-state rows with
"itos-container exited 0, not 2" and the two account-home rows with "itos-container
exited 0, not 3", because doctor reported an approval and approve published one. Every
Given and When step passed; there were no undefined steps or setup failures. After the
change the six rows pass, and the selected-state rows then confirm that fresh selections
of the default tree and another checkout stay usable.

```sh
go test ./features -count=1 -run '^TestFeatures$' -scenarios '^@ID-CONTROL-(25|26)$'
```

A disk unit test covers reads and writes for both paths, requiring that the refusal
changes nothing and that a valid selection on the same account still enrolls; it fails
before the change and passes after it.
