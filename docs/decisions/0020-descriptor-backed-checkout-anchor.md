# Slice-17 descriptor-backed checkout anchor

## Authority and status

Renumbered from 0019 when merging main, which already assigns that number to
the checkout-socket milestone decision. Historical coordinator records retain
the original number.

The coordinator's q-19 and q-27 decisions of 2026-11-06, approximately
22:55 and 23:50 UTC, select this mechanism and detector; the person reviews
them later. Q-39 of 2026-11-08 refines short-lived workload observation as
described below. These decisions override earlier slice-17 requirements where
they differ. This is an unproven Moby/kernel
hypothesis about native behavior. The combined implementation is described below;
it is not a host-support claim.

This decision updates arc42's **Building-block view**, **Foreground run and its
failure paths**, **Recovery**, **Deployment view**, and ownership/evidence
descriptions. The charter's offline scope and existing isolation boundary hold.

## Selected mechanism

The controller opens the validated checkout with `O_PATH` and keeps that
descriptor open from the final identity check until `ContainerStart` returns.
It publishes an owned Docker volume using the local driver with exactly
`type=none`, `o=bind`, and `device=/proc/<controller-pid>/fd/<n>`. The worker
mounts that volume, with copying disabled, at `/workspace`. The original
checkout identity remains in frozen settings; this is not a copied workspace
or a persistent worker data volume.

Dockerd performs the bind. The controller gains no capability, performs no
host mount and starts no new long-lived helper. The kernel must follow the
descriptor link to its pinned inode, rather than resolve a pathname obtained
by reading that link. This anchor prevents substitution; the following check
detects an anchor failure and does not provide the protection itself.

Immediately after `ContainerStart` returns, inspect the container's `State.Pid`.
For a running worker, hold a pidfd and freshly inspect the owned full ID again,
requiring the same live init PID before statting `/proc/<pid>/root/workspace`.
Compare device and inode with the validated identity. The held process is checked
on both sides of stat so a recycled numeric PID cannot supply mount authority.
The worker runs as the host UID with no-new-privileges. An identity mismatch or
an unreadable workspace outside q-42's root-change errors requires immediate
owned kill/removal and mount refusal, with no checkout-path fallback.

Q-42 (2026-11-08, approximately 09:45 UTC) refines unavailable stat: ENOENT,
ESRCH and EACCES require polling the held pidfd. An exited process follows
q-39 below. A still-live process may have changed its root or namespaces;
continue with `not observed: worker root changed` in the durable record and
JSON/text status. This records an unobserved mount, not proof of why stat failed
or a verified identity. Other stat failures and failed process observations
still refuse. No engine wait is used to turn an arbitrary stat error into exit.

Q-39 (2026-11-08, approximately 06:35 UTC) refines q-27: if the owned worker has
already exited before inspection, pidfd acquisition or stat, retain its real
exit through the ordinary engine wait and teardown. A fresh full-ID inspection
must establish exit if pidfd acquisition, process observation or stat fails.
Neither PID disappearance alone nor a permission/transport error establishes
exit. Record `not observed: worker exited`; do not claim mount verification.
If kernel process absence arrives before Docker publishes the exit, reconcile
the same owned full ID through a not-running wait and fresh exited inspection
within one ten-second call budget. Keep that wait's real exit result instead of
reacquiring it. Changed PID/ownership, failed confirmation or expiry still
refuses; other syscall errors do not authorize this wait.
The descriptor volume remains the protection, and its native proof requirement
is unchanged. No helper, extra mount or new privilege is introduced.

Durable intent precedes volume allocation. Independent inspection of the
volume's identity, labels and exact options precedes durable concrete ownership
and worker creation. Creating a local volume does not itself establish the
backing mount; Docker establishes it during start. The descriptor remains open
through that call. The detector runs immediately afterwards, without a gate
binary, extra worker mount, capability or helper.
The volume is the durable anchor record and is released last, after confirmed
worker removal. No name-only absence authorizes release.

Once started, the worker's mount retains the original inode for its lifetime.
The descriptor need not outlive the controller. If worker removal fails, retain
the original volume and protected ownership; do not require an expired `/proc`
path or a stopped worker's mount to remain usable. Recovery must not recreate
the volume from the current checkout pathname. Slice-18, integrated here under q-36, adds recovery's release
of the retained volume. Existing recovery must refuse records it cannot account
for rather than declare their cleanup complete.

## Evidence required before implementation

The native race pauses the actual submitted start after the final identity
check, substitutes the checkout or an ancestor, then forwards the same request.
The worker must see the original inode or never receive the workload. A healthy
control must succeed. Original/replacement nonces and independent device/inode
observations distinguish a pinned mount from copies or path equality.

Commit executable steps with `@wip` retained, then let the coordinator obtain
assertion-level red evidence through the guarded CI dispatch. Native green at
the implemented head is required before accepting the hypothesis. Neither
local HTTP fixtures nor source inspection provide native evidence. Stop with
evidence if Moby resolves the device by name or rejects the `/proc` path; do not
invent another anchoring mechanism.

As a source-only feasibility check, Moby v28.5.1's
[local volume adapter](https://github.com/moby/moby/blob/v28.5.1/volume/local/local_unix.go)
passes the device unchanged for `type=none` to `mount.Mount`. The
[mount implementation](https://github.com/moby/sys/blob/main/mount/mounter_linux.go)
passes its device argument to `unix.Mount`. This does not pin the CI daemon's
version or prove that its whole execution path, confinement and kernel accept
the descriptor source. Record the actual server version with the native result.

## Implementation checkpoint and settled detector

The coordinator accepted run
[42309692365](https://github.com/donvargax/itos-container/actions/runs/42309692365)
at `51a9f48e81e61f4654208d9779f6a49992e35692` as assertion-level red evidence
for all four OFFLINE-29 substitutions. Docker 28.0.4 ran the replacement's
inode and nonce instead of the validated inode. This establishes the existing
pathname race; it supplies no green evidence for the descriptor-backed volume.

Source inspection of that daemon version leaves the selected anchor intact:
its [local driver](https://github.com/moby/moby/blob/v28.0.4/volume/local/local_unix.go#L115-L167)
passes a `type=none` device unchanged to the mount library. The source evidence
behind q-27 is:

- With copying disabled, [container creation](https://github.com/moby/moby/blob/v28.0.4/daemon/create_unix.go#L71-L86)
  skips volume population. Volume allocation and inspection are not mounted-inode proof.
- [ContainerStart](https://github.com/moby/moby/blob/v28.0.4/daemon/start.go#L130-L240)
  sets up mounts and starts the task within the same API operation. Checking
  after its return cannot ensure verification before the first workload instruction.
- The [inspect and path-stat types](https://github.com/moby/moby/blob/v28.0.4/api/types/container/container.go#L27-L93)
  expose no filesystem device/inode pair. The pinned Moby API v1.56.1 types
  also omit it, and the [local volume status](https://github.com/moby/moby/blob/v28.0.4/volume/local/local.go#L337-L339)
  is empty. Matching a volume name, options or source string is insufficient.
- [Pause](https://github.com/moby/moby/blob/v28.0.4/daemon/pause.go#L21-L29)
  requires an already-running task. Starting then pausing cannot close this gap.

Q-27 settles this ordering: Docker has no hook between mount and task start,
and an added gate binary would require a forbidden extra mount. The anchor
must ensure that the worker never runs in a substituted checkout; the host
stat detects a failure immediately after start. The native workload probe
independently proves which inode executed. A detector refusal after successful
start must fail the native acceptance check, even if the probe produced no
output before teardown. Stop with evidence if Moby rejects the descriptor or
resolves it by name; do not change mechanisms.

OFFLINE-29 has accepted red evidence. The coordinator also accepted run
[42390617135](https://github.com/donvargax/itos-container/actions/runs/42390617135)
at `3aae532` in q-33 on 2026-11-07, approximately 16:40 UTC, as assertion-level
red evidence for both OFFLINE-30 removal-retention rows. Each failed at its Then
with "missing original owned volume and protected recovery evidence after
controller exit". Implementation of that behavior is authorized. The local
ownership/fault/detector cases 25–28/31/32 have the red checkpoint recorded below.
That checkpoint authorized implementation; native green is still required.

## Historical local checkpoint and resolved recovery integration

The local steps are committed in `bb19503`, with contextual preflight diagnostics
and both-backend assertion reporting corrected in `4c57651`. With `@wip` removed
only for the local run, all 25 rows of OFFLINE-25/26/27/28/31/32 failed at their
first behavior assertion on both memory and disk/SDK backends. There were no
undefined steps or setup failures in that evidence run. The failures were missing
anchor allocation, missing publication/source refusal, missing immediate detector
teardown, or missing retained anchor authority, as appropriate to each case.

The filesystem sandbox exposes root ancestors as UID 65534. Its `state.unsafe`
preflight failures also affect the existing OFFLINE-01 control and are not red
evidence. The accepted local run used normal host ancestry, disposable state and
the HTTP fixture, without a Docker socket or an approval command.

A subsequent uncommitted implementation draft passed all 25 local rows, including
both backends and their healthy controls. It added schema-2 anchor ownership,
descriptor-backed volume requests, post-start inspection/stat and foreground
release ordering. That result is local evidence only. No native green, kernel
resolution proof, changed-code proof or support claim was obtained.

That draft exposed an integration conflict: the earlier handoff assigned
fresh-process anchor recovery to slice-18, while existing live scenarios already
required new processes to complete cleanup of newly launched sessions. Q-36
settled the conflict on 2026-11-07 at approximately 18:00 UTC by merging
slice-18 into slice-17. Q-38 reaffirmed this on 2026-11-07 at approximately
20:35 UTC. Fresh-process recovery is implemented in this slice, not a remaining
integration gate; schema-2 readers account for the original anchor.

The combined implementation in `e885ff8`, present at attempt 14's `60b81ed`,
keeps these scenarios live with their existing successful-recovery requirements:

- RECOVERY-21 requires run's printed stop retry to return zero after a failed
  foreground removal.
- RECOVERY-22 requires fresh status and stop after the foreground controller dies.
- RECOVERY-23 requires independent stop processes to complete one of two runs.
- LIFECYCLE-16 requires fresh stop to reconcile uncertain launch boundaries.

The local recovery checks that exposed the draft's schema-2 refusal now pass,
including `TestLifecycleEngineLossRetainsRecoverableFailure`,
`TestPublicRecoveryBudgetRegressions` and
`TestRegisteredWaitReconcilesConcurrentStop`. Native RECOVERY-21/22/23 and
LIFECYCLE-16 remain required passing CI regressions; this local checkpoint is
not native execution evidence. No scenario was weakened or retagged to resolve
the cycle, and q-19's anchor and q-27's detector remain unchanged.

The saved `/tmp/slice-17-anchor-prototype.patch` at `4c57651` is obsolete. The
combined implementation and evidence below supersede its incomplete recovery,
adapter fixtures and unit coverage.

## Shared-contract preparation after q-33

The handoff at that checkpoint permitted implementation preparation while gating
activation on slice-18's specification and integration. At that point,
the registry made slice-18 depend on slice-17, so waiting for slice-17 closure
could not satisfy the live recovery contracts.
The combined implementation follows q-36 without reconsidering q-19 or q-27.
The shared `Recovery.stop` path is the teardown authority for foreground and
fresh-process cleanup.

`internal/engine/session/port/porttest/anchor.go` now supplies reusable contracts
and I/O-free fakes for the already specified anchor ports. They check exact
volume observations, preservation of an existing same-name volume, confirmed
absence, descriptor-shaped sources, source lifetime across pathname substitution,
and mounted identity obtained from an explicitly supplied worker PID. The fake
keeps worker mount observations independent of the pinned identity, so a missing
or mismatched observation cannot silently become a successful detector result.
Tests also exercise cancellation, foreign/unrelated resource preservation,
failed pinning, and rejection of missing controls by the contract oracles.

At this checkpoint these contracts ran against their memory fakes. The disk and SDK
adapters now run the same contracts in the combined implementation below.
The test increment itself activated no slice-17 scenario and supplied no native
proof. The combined implementation now includes recovery and adapter coverage;
native green remains the coordinator's CI responsibility.

## Combined integration under q-36

The coordinator's 2026-11-07 decision at approximately 18:00 UTC resolves the
ordering cycle described above: merge slice-18's accepted `features/anchors.feature`
(commit `769ff5c`) into slice-17 and implement both in one pull request. This
supersedes the separate delivery ordering, not q-19's mechanism or q-27's
post-start detector. Recovery shares the session engine and releases the anchor
only after confirmed worker absence; completion accounts for both resources.
The coordinator owns consolidation of the registry.

The native recovery scenarios `ID-ANCHORS-09` through `ID-ANCHORS-13` require
their own assertion-level CI red evidence before their implementation. Existing
OFFLINE-29/30 evidence does not substitute for those checks. The accepted spec's
pre-workload detector wording is updated to q-27; no gate or helper is added.

## Combined implementation and local evidence

Q-37 accepted native run 42400524477 at slice-17's head as assertion-level red
for all seven rows of ANCHORS-09 through ANCHORS-13. Local steps were committed
in `8a0bb23`, `4a39085` and `ac433f5` before production changes. At `4a39085`,
48 of 49 ANCHORS-01 through ANCHORS-08 rows failed their behavior assertions;
one foreign-user fixture failed setup and was excluded from that evidence.
After the fixture correction in `ac433f5`, all 18 ANCHORS-05 rows failed their
Then assertions. Together these runs establish local red for every one of the
49 rows, without undefined steps or counting setup errors as evidence.

The implementation upgrades only newly launched foreground intents to schema 2.
It adds the O_PATH descriptor, exact owned local-volume requests, pre-create
concrete ownership publication, volume mounting with NoCopy, and q-27's host
stat detector. The source remains open behind the engine operation gate until
start actually returns, including late returns after a controller deadline.
A detector failure uses immediate owned SIGKILL/removal and reports refusal.

Foreground and fresh-process recovery share the existing session teardown.
The protected snapshot and public registration bind the volume to its original
host user, project, session, operation, daemon and checkout identity. Recovery
compares the complete observed volume tuple without opening the expired source.
Only full worker-ID absence, or durably fenced reserved no-create evidence,
permits release. Independently confirmed volume absence precedes durable
completion. Unavailable observations and failed writes retain exact retry
identity; repeated stop reloads and syncs completion. Legacy worker-only records
retain their original behavior. Status exposes separate worker, anchor and
cleanup facts and remains read-only.

The disk and Docker SDK adapters run the same anchor contracts as the I/O-free
fakes. Local scenarios cover both backends, publication faults, uncertain
worker/volume removal, substituted authority, independent sessions and unchanged
10-second status / 30-second stop totals. Native feasibility and retained
isolation still require the coordinator's ordinary CI run; no Docker engine,
local mutation run or generated proof was used in this implementation session.

With `@wip` removed, the final local selection passes all 74 new rows: 25
OFFLINE-25/26/27/28/31/32 rows and 49 ANCHORS-01 through ANCHORS-08 rows.
The affected invocation, foreground-signal and overlap selection passes all
155 rows. Session/command unit tests, both adapter contracts and the feature
harness guards also pass. The wider local regression selection exercised
recovery, lifecycle and exit handling; its old fixture mismatches were corrected
and rerun without weakening their workload, ownership or cleanup assertions.
The local HTTP fixture assemblies supply an explicit simulated mounted root;
native scenarios continue to use the product binary and actual host stat.

## Short-lived workload resolution under q-39

Review thread `PRRT_kwDOVkB0ws606Rfz` correctly identified that the initial
q-27 implementation turned ordinary short-lived results into mount refusals.
Q-39 settles this conflict: the detector is evidence about the anchor, not the
protection. A completed workload retains its actual exit while its record and
fresh JSON/text status explicitly state that the mount was not observed.

The optional `mount_detector` progress field has four values after q-42:
`verified`, `not observed: worker exited`, `not observed: worker root changed`,
and `refused`. It is persisted before
normal wait or immediate teardown and cannot be changed or removed afterwards.
Legacy seven-field public records remain readable; missing detector data never
means verified. Readers unaware of the new field reject it under their strict
schema rather than silently interpreting it. Engine ownership remains in the
schema-2 intent; no new field grants mutation authority.

The red step commit `01adcce` adds OFFLINE-33's four rows (exit 0/7 before
inspection and between inspect and stat) and OFFLINE-34's PID-reuse guard.
With `@wip` removed locally, all five failed at their first Then: short exits
returned mount refusal, and the reused-PID fixture accepted an unguarded stat.
The implemented scenarios pass through the registered command, durable record,
and fresh JSON/text status. Additional tests cover pidfd failures, fresh
inspection failures, exit around acquisition/stat, live-worker stat errors,
identity mismatches and late-handle cleanup. The process-reference contract runs
against the I/O-free fake and a real local child process, without a container.

The supplied concurrent-stop CI failure was a fixture scheduling race. The
finite wait wrapper may return on cancellation before its inner Wait goroutine
journals cancellation. The fixture now holds its probe checkpoint until that
observation is published. Its existing event-order and deadline assertions are
unchanged; no production timeout or check is relaxed.

## Native mount oracle review and local validation

Thread `PRRT_kwDOVkB0ws606Rf2` found that the existing mount and controller-loss
oracles still rejected every volume. The recorder now captures an independent
volume inspection at start and during each loss round. The shared oracle binds
that observation to the original protected intent, public session and approved
checkout identity, checking the exact local driver, creation time, bind options,
labels and mount inventory. Loss rounds also retain the same original anchor
and protected ownership record after controller death.

The new local positive control first failed at the old oracle's rejection of
the owned volume. It passes after the repair, along with 24 negative controls
and an HTTP recorder control which rejects substituted native volume data.
Focused oracle tests also pass with the race detector. These are harness checks;
the existing native scenarios still need CI at the pushed head.

## Local features timeout investigation after q-38

The coordinator identified the reported `FAIL .../features 600.444s` as the
package's default ten-minute Go test timeout. The next supplied CI result at
`e17e5f0` includes three assertion failures as well as `FAIL .../features
600.496s`: `TestFeatureBinaryPublishesOnceAfterBuild`,
`TestFeatureBinaryFailedBuildIsNotCached` and `TestFeatureConcurrency/local`.
These are the available CI results; this attempt has no GitHub access. No
timeout or gate was changed.

The saved engine-disabled baseline at `60b81ed` passed the package in 430.195
seconds, with all 1,348 live local scenarios taking 404.03 seconds. Its timings
showed more than three minutes in serial recovery/anchor/inspection deadline
cases alone, including 10- and 30-second anchor totals and a 40-second recovery
row. Approval and preflight contracts also launched `go test` for each row,
repeating build work for the same assembly. This leaves little margin on CI.

Harness regressions in `e08b711` first failed at the serial scheduling and
partial/failed executable publication assertions. The ordinary local runner now
schedules four isolated scenarios concurrently. Engine and manual red runs stay
serial. The selected rows, assertions, real operation deadlines and cleanup
checks are unchanged. Compiled helpers are published atomically after a single
in-flight build per cache path, outside scenario checkouts. Manifest, approval,
preflight and session contracts reuse one executable per coverage mode, with
fresh processes and separate inputs/results. Per-ID coverage collection remains
checked.

The interrupted attempt's saved checkpoint reports an engine-disabled package
pass in 138.588 seconds, with all 1,348 local scenarios passing in 120.11 seconds.
Its race runs then exposed unsynchronized local HTTP recorder access. The
restored changes guard child startup and result publication with the API mutexes,
copy request snapshots, lock live recovery faults and OFFLINE-24's pause setter,
and pass command-specific inputs without mutating the shared fixture environment.

This attempt compared the 21-file stash with `e17e5f0` before restoring it; only
coordinator-owned registry files had moved since the stash base. All three
reported assertions failed before restoration: a reader saw `partial`, a failed
build poisoned the cache, and local peak concurrency was one instead of four.
They now pass, also under the race detector. Their assertions, including the
deterministic concurrent-reader schedule, remain unchanged. The build coordinator
uses per-build completion channels so those existing scheduling checks remain
valid. The stash remains available.

The complete engine-disabled `go test -race ./features -count=1 -json` run passes
in 265.868 seconds. All 1,348 local scenario rows pass in 207.11 seconds, with no
race reports; the manifest, approval and kernel coverage checks also pass. This
run uses disposable state with host ownership. The filesystem sandbox's remapped
ancestor ownership first caused `state.unsafe` refusals, which are not detector
behavior evidence and were not worked around by weakening state validation.

The coordinator subsequently reported that a full-package race run consumed
22 GB and starved the machine on 2026-11-08. Do not repeat the historical command
above. Run the complete feature suite without the race detector and limit race
detection to explicitly selected changed tests or changed production packages.
A memory-limit kill requires reducing test memory use, never raising the limit.

Ordinary CI at the pushed head still has to confirm the timeout is gone. No
native-engine or mutation evidence is supplied by these local runs.

## Verification after the memory-limit instruction

At `8b341d8`, the interrupted worker stash's implementation is already present
in `62796a9`; no second restoration is needed. The later README-only stash adds
the coordinator's memory restriction and is incorporated here, without dropping
either stash. The three supplied CI failures pass again at this head:
`TestFeatureBinaryPublishesOnceAfterBuild`, `TestFeatureBinaryFailedBuildIsNotCached`
and `TestFeatureConcurrency`. A race run selecting only those tests also passes
in 1.057 seconds.

With `ITOS_CONTAINER_ENGINE_TESTS=0`, the complete non-race feature package
passes in 131.901 seconds, including all 1,348 local scenarios in 116.10 seconds.
This run uses host-visible ownership and disposable fixtures. The initial
filesystem-sandbox run exposed `/` and `/tmp` as UID 65534 and failed protected
state checks; it supplies no product behavior evidence. State validation, test
assertions, timeouts and memory limits remain unchanged. No native engine or
local mutation run was made. The short-lived workload conflict below remains
unresolved; these passing tests do not establish success for that ordering.

## Short-lived workload conflict exposed by review

PR thread `PRRT_kwDOVkB0ws606Rfz` is valid. A command such as `/bin/true` can
exit between successful `ContainerStart` and the post-start inspection, leaving
no live init PID. It can also exit after inspection but before the host stat.
`detectAnchor` returns `MountRefused` in either case. The registered runner skips
normal waiting on that refusal and performs immediate owned teardown; a recorded
zero workload exit does not turn the refusal into success.

Q-27 and live OFFLINE-32 explicitly require refusal when the PID is unusable or
the stat cannot be made. They require the worker's freshly inspected
`/proc/<pid>/root/workspace`, and exclude a volume mountpoint, the current checkout
path, a cached PID or an added gate. Volume metadata and the still-open original
descriptor prove the requested source, not the inode the exited task actually
mounted. The native probe's release barrier keeps its PID alive and therefore
does not exercise this short-lived case.

The pilot's successful-exit contract and this unconditional detector rule cannot
both be met for that ordering. No production workaround is made here: treating
an exited task as verified would bypass the required observation, and another
mount/lifetime mechanism would contradict the settled decisions. This is a new
specification conflict, separate from q-36/q-38's resolved recovery integration.
The coordinator must specify how a short-lived task's mounted identity can be
verified, or explicitly settle the unavailable-observation result. A regression
should cover exit zero and nonzero both before inspect and between inspect/stat,
with a healthy control and retained mismatch refusal; any native rows require
their own CI assertion-level red before implementation.

## Q-39 local verification

After the red checkpoint, OFFLINE-33/34 pass with `@wip` removed. The complete
engine-disabled `go test ./features -count=1` passes in 180.816 seconds, including
all 1,353 live local scenario rows. No full-feature race run was made.

The command assembly suite passes with normal host ownership and disposable
state. Session domain, disk, fake ports, codec and presentation tests pass under
`-race`; the pidfd adapter and fake share the process-reference contract. Focused
race runs also pass for short-lived exit and PID-reuse fixtures, strict recovery
record/status oracles, and the concurrent-stop fixture (20 repetitions of the
before-reconciliation schedule and three of the after-deadline schedule).

Synthetic Engine API fixtures now explicitly supply the process reference for
their synthetic PID, alongside their existing mount-stat seam. Native assemblies
use real pidfds. Exact record/status oracles recognize only the specified optional
detector values; their ownership, preservation and unknown-field refusals remain.
The native engine suite and fresh changed-code proof still belong to CI at the
actual pushed head. No native engine, local mutation run, or new support claim
is part of this local verification.

## No-create status review correction

PR thread `PRRT_kwDOVkB0ws60-04h` exposed a schema-2 recovery defect. After
stop cancelled a launch before worker creation and released its anchor, status
looked up the never-created worker by name and reported uncertainty. A validated
reserved or cancelled intent with no worker ID already proves worker absence.
Status now preserves that fact for either schema and still independently opens
the original engine and inspects the anchor for schema 2. Unavailable or foreign
anchor observations cannot claim complete cleanup. Unsafe session evidence
cannot supply the no-create fact; uncertain create intents retain their existing
inspection and refusal behavior. Status remains read-only.

The red step commit `a5bf597` adds ID-ANCHORS-14 with reserved and cancelled
rows. With `@wip` removed, both rows failed at their first Then through the
memory and disk/SDK backends: stop succeeded but subsequent status returned 75
with worker unknown and cleanup unknown. They pass after the correction,
including repeated stop, exact durable cancellation, one original anchor release,
no worker lookup and preservation of unrelated resources. The shared completion
oracle now expects a cancelled intent with an empty worker ID for these rows;
created-worker cases still require their recorded full ID and absent stage.

Focused domain cases cover retained and absent anchors, foreign or unavailable
observations, engine connection failure and unsafe session evidence. The session
domain and the no-create command regression pass under targeted race detection.
The full engine-disabled feature suite passes without race detection in 175.378
seconds. No full-feature race run or local mutation run was made.

The supplied CI report for run 42459248613 at `8e6655a` lists native scenario
failures but supplies no assertion diagnostics. That head already includes
q-39's short-lived workload correction. This status fix does not establish a
cause or resolution for those native failures; the coordinator must supply the
first failing assertions or full engine log to continue that diagnosis. Native
engine execution and fresh changed-code proof remain CI responsibilities.

## Q-41 native refusal diagnostics

Q-41 (2026-11-08, approximately 08:00 UTC) supplies the full engine log for
run 42459248613. Its 34 assertion failures include fault-free controls returning
wrapper exit 1 instead of workload exits 0 and 7. The recorded product errors
say only `run.mount-refused`; they do not identify which check failed. The later
run 42460442920 at `976445b` also fails its engine tier. Neither log establishes
that Moby rejected the descriptor device path or resolved it by name.

The controller now includes the failed check and original expected device:inode
in mount diagnostics, plus the observed identity or PID where available. Stat
and pidfd failures retain the numeric errno and its standard description. Only
these safe values cross the port; wrapped error paths, engine response bodies,
argv and workload streams are excluded. Diagnostic reads share the observation
gate and never wait for a pending adapter operation or postpone owned teardown.
They do not change refusal classification, observed workload exit precedence,
anchor ownership, the detector's short-lived exit rule, or recovery records.

The feature harness appends controller mount diagnostics to failed assertions
before cleanup, including subordinate runs and fault-free controls. It preserves
the original failure and does not dump full stderr or fixture records.

The red test commit `46754a6` adds ID-OFFLINE-35. With `@wip` removed, all five
rows failed at the first Then because the diagnostic lacked its named check:
device mismatch, inode mismatch, stat permission denial, failed pidfd open and
an invalid init PID. All five pass after implementation, including immediate
owned worker/anchor cleanup and exclusion of synthetic private error paths and
argv. OFFLINE-33/34 continue to pass. Targeted race tests cover the affected
session domain, disk adapter, fake diagnostic contract, run command and harness
failure reporting. The full engine-disabled feature suite passes without race
detection in 169.906 seconds; no full-feature race or local mutation run was made.

Q-41 requires another native CI run before any mechanism change. This checkpoint
is blocked on that evidence: inspect the new `check`, identity and errno fields
in the native failures. If they show Moby refusing the `/proc` device or resolving
it by pathname, stop with that evidence as q-19 requires. Do not infer a new
anchor mechanism, privilege, host mount, helper or support claim from local green.

## Docker exit publication lag after q-41

The coordinator's saved report of 2026-11-08, approximately 08:25 UTC, identifies
the diagnostic failures from [run 42461978939](https://github.com/donvargax/itos-container/actions/runs/42461978939)
at `1852dcec38301a3765ba9632820eb95b9045bf41`: `pidfd-open` and one
`pidfd-before-stat` refusal, with worker exit not yet confirmed. It reports no
device/inode mismatch. The public job API confirms the engine job failed and
the Ubuntu tests passed. The GitHub app required reauthentication and the public
log endpoint returned 403 in this session, so the coordinator's saved diagnostic
report is the source for the failure details, not an independently fetched log.

Kernel exit and Docker state publication are separate observations. The previous
detector inspected once after a failed pidfd open or ended process and refused
while Docker still reported running. The disk adapter now distinguishes ESRCH
from other acquisition errors. Only that process absence, or a successfully
observed ended pidfd, admits bounded exit reconciliation. The same owned full ID
and unchanged PID are required before waiting. A successful wait must be followed
by a fresh owned exited-state inspection within the same budget. Retain the exit
for ordinary teardown and status; a second wait cannot discard a known result.

Step commit `37f8302` adds ID-OFFLINE-36. With `@wip` removed locally, all six rows
failed at the workload-result Then with `short-lived result lost`: exits 0 and 7
before pidfd acquisition, before stat and during stat. The fixture keeps Docker
inspections running until its independent exit notification is consumed. All six
pass after the fix, along with OFFLINE-33/34/35. OFFLINE-35's missing-PID control
still has a running engine worker throughout reconciliation and retains its
refusal and immediate owned cleanup assertions.

Controlled-time domain regressions cover the ten-second bound, wait failure,
continued running state, changed PID, foreign or unavailable final inspection,
missing started proof and retained exit through both runner routes. A shared
process-absence contract runs against the memory fake and Linux disk adapter.
The changed session packages, process contract and selected OFFLINE-36 scenarios
pass targeted race detection. The full engine-disabled feature suite passes
without race detection in 169.674 seconds with host UID ownership. Its initial
sandboxed attempt refused remapped fixture ownership as `state.unsafe`; no
ownership check or memory limit was changed. Native acceptance and fresh
changed-code proof remain CI obligations; these local results do not settle the
descriptor volume's Moby/kernel hypothesis.

## Unreaped worker exit after the native cleanup regression

The coordinator's report of run 42467490909 at `9c99617` identifies a
`mounted-stat` refusal with ENOENT in the ephemeral-storage scenario. The
expected workload result was zero, but the detector could not confirm exit.
The earlier bounded Docker wait only runs after observed kernel exit. Its
pidfd adapter used signal zero, which still succeeds for an exited, unreaped
process even though that process's `/proc/<pid>/root` is already unavailable.

A local child experiment reproduces that combination for exits zero and seven:
signal zero succeeds, the process root returns ENOENT, and its pidfd is readable.
The shared process-reference contract likewise fails while a child is exited
but deliberately unreaped. This explains a detector refusal without evidence
that Moby substituted the checkout or rejected the descriptor source.

Red step commit `5dff477` adds ID-OFFLINE-37 with a real local child and the
production disk pidfd adapter. Its engine fixture continues to report the same
running PID until wait consumes the child's actual exit. With `@wip` removed,
all four rows fail at the workload-result Then with `mounted-stat` and
`worker exit not confirmed`: exits zero and seven before stat and during stat.
These are local syscall/command regressions, not native container evidence.

The adapter now polls the held pidfd with a zero timeout. Readable exit state,
including the later reaped state, admits the existing bounded engine wait;
polling does not reap the process or invent its workload status. Closed
references, syscall errors and unexpected readiness events still refuse.
Fresh full-ID/PID revalidation, the ten-second engine reconciliation budget,
owned teardown and the descriptor-backed volume remain unchanged.

OFFLINE-33 through OFFLINE-37 pass, including the new four rows, with truthful
`not observed: worker exited` records and status. The disk adapter and focused
command assembly regressions pass under the race detector. The complete
engine-disabled feature suite passes without race detection in 171.348 seconds,
using disposable fixtures with normal host ownership. Native engine acceptance
and fresh changed-code proof remain the coordinator's CI work at the pushed
head; no local engine or mutation run supplies that evidence.

## Live worker root changes under q-42

Q-42 also covers workers that change their root or namespaces while remaining
alive. The earlier unreaped-process correction only handled exited workers.
Step commit `f95f9aa` adds OFFLINE-38's six local rows: ENOENT, ESRCH and EACCES
during stat, each followed by actual workload exit zero or seven. The fixture
keeps a real local child alive through the production pidfd adapter and models
the changed-root syscall result through the disk stat seam. It performs no
chroot, namespace change, privileged operation or native container execution.
With `@wip` removed before implementation, all six rows failed their first Then
because run returned mount refusal instead of the workload result.

The disk adapter now identifies exactly those three errors. The detector polls
its held process after stat: observed exit retains q-39's bounded reconciliation;
a live process records `not observed: worker root changed` and waits normally.
Failed process observation, other stat errors, PID changes and device/inode
mismatches retain refusal. The public record and JSON/text status accept the
new unobserved value without granting ownership or claiming mount verification.

OFFLINE-35's generic stat-failure diagnostic control now supplies EIO instead of
EACCES, because q-42 explicitly changes EACCES handling. It still requires the
exact errno, immediate owned teardown and private-payload exclusion. The native
record readers recognize only the newly specified detector value in addition
to the existing values; their resource and ownership assertions are unchanged.

The public job metadata for run 42468606027 at `2ad7081` reports engine and Linux
tests successful and the `ci` job failed. The coordinator supplies its HTTP 504
failure during pinned-linter acquisition. Those results predate this q-42
change; native acceptance and changed-code proof remain required at the next
pushed head.

All six OFFLINE-38 rows now pass. The full engine-disabled feature suite passes
without race detection in 172.562 seconds. Targeted race checks pass for the
changed session domain, disk adapter, codec and command fixture. The fixture
records separate live-process events before and after stat, so its ordered
assertion cannot mistake the first poll for the second. No native engine or
local mutation run was made.

## Fixture identity contract blocks the Docker 29 runner

[Run 42491204197](https://github.com/donvargax/itos-container/actions/runs/42491204197)
at `1036a6e` failed during fixture preparation, before any engine scenario ran.
Its saved engine log identifies Ubuntu 26.04.1 and Docker 29.4.2, API 1.54.
The build produced image ID
`sha256:3f044f1a9f780c8af673250d2e1eadc28965a64b04599311f1d4a58d03dff56d`;
the push reported that same value as the manifest digest. Preparation then
exited with status 1, and its owned cleanup passed. This is not evidence of
an anchor or detector failure.

The helper treats the build ID as the config digest: its manifest predicate
requires `.config.digest == $built`, then it requires the manifest digest to
differ from `$built`. T-20 explicitly specifies both identities this way,
including the distinct config digest exported as `image_id`. The cached-image
check and native harness also require the inspected `Id` to match that output.
However, Moby's
[Docker 29.4.2 containerd image inspection](https://github.com/moby/moby/blob/docker-v29.4.2/daemon/containerd/image_inspect.go)
sets `Id` from the image target descriptor digest, and reads the config
separately. That implementation explains the equal build/push identities in
the log. The manifest response itself was not retained in the saved log, so
the failing predicate is a source diagnosis, not an additional native run.

T-20's specification needs correction before adapting this gate: distinguish
the immutable engine image ID from the config digest and define how both
are independently verified and carried into the fixture. Preserve the exact
repository digest, manifest-byte hash, cold pull, cached metadata, smoke and
owned-cleanup requirements. Merely deleting the config comparison or exporting
the manifest as a supposedly distinct config ID would weaken or contradict
the existing check. No helper, runner, scenario or product change is made at
this checkpoint, following AGENTS.md's instruction to stop on a wrong gate.

The preceding proof corrections are already committed in `1036a6e` and
`359cd0a`. A fresh local coverage run of session's codec, disk, docker, domain
and port packages passes; matching the prior proof log's locations to that
profile confirms execution of all 45 reported uncovered blocks. The existing
tests also assert both supported no-new-privileges encodings and reject
unknown detector values, addressing the two reported survivors. No local
mutation run was made. CI still owes fresh mutation proof and native evidence
at the eventual pushed head after the fixture contract is resolved.
