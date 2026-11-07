# Slice-21 red-first handoff

Coordinator q-31 (2026-11-07, approximately 16:25 UTC) accepted the red evidence
below and authorized implementation. All 45 examples use recording HTTP engine
fixtures, not `@engine`; no CI red run is required. The production change and
[architecture decision](../decisions/0019-control-state-publication.md) implement
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
