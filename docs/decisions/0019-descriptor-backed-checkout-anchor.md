# Slice-17 descriptor-backed checkout anchor

## Authority and status

The coordinator's q-19 and q-27 decisions of 2026-11-06, approximately
22:55 and 23:50 UTC, select this mechanism and detector; the person reviews
them later. They override earlier
slice-17 requirements where they differ. This is an unproven Moby/kernel
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

Immediately after `ContainerStart` returns, inspect the container's `State.Pid`
and stat `/proc/<pid>/root/workspace` from the host. Compare its device and inode
with the validated identity. The worker runs as the host UID with
no-new-privileges. If the stat cannot be made or the identity differs, kill and
remove the worker immediately through owned teardown and report a mount refusal.
There is no checkout-path fallback. A detector failure is evidence against the
anchor hypothesis, not an acceptable native green result on supported Docker.

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

## Short-lived workload conflict found in PR 13

Review thread `PRRT_kwDOVkB0ws606Rfz` identifies a real consequence of q-27:
a successful workload can exit before the post-start inspect, leaving no live
init PID for `/proc/<pid>/root/workspace`. It can also exit between inspect and
stat. The existing detector refuses these cases and invokes owned teardown,
as q-27 requires when that stat cannot be made. This conflicts with the
foreground contract's successful-exit preservation for ordinary short commands.

The volume's options and labels prove its requested source and ownership; they
do not observe the inode actually mounted in a worker which has already exited.
Neither a successful engine wait nor an exited state supplies that observation.
Returning success from either would silently waive q-27. The trusted native
race probe waits for a harness release and therefore does not resolve this
ordinary-workload race. Retrying the workload would execute it twice and is not
authorized either.

The implementer leaves the detector intact and the review thread unresolved.
The coordinator must reconcile short-lived success with q-27's mandatory stat
and refusal rule before this item can be complete; this document selects no
new helper, privilege, mount, or detector fallback. A regression scenario must
cover exit before inspection and exit between inspection and stat once that
behavior is settled.

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

Ordinary CI at the pushed head still has to confirm the timeout is gone. No
native-engine or mutation evidence is supplied by these local runs.

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
