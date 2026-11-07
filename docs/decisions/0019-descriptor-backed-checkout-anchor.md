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

## Historical local checkpoint and recovery integration gap

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

The draft exposed an integration conflict that must be resolved before the
behavior commit. This decision and OFFLINE's handoff assign fresh-process anchor
recovery to slice-18 and require readers that cannot account for the anchor to
refuse completion. However, these existing live scenarios launch new sessions
and require fresh processes to complete their cleanup:

- RECOVERY-21 requires run's printed stop retry to return zero after a failed
  foreground removal.
- RECOVERY-22 requires fresh status and stop after the foreground controller dies.
- RECOVERY-23 requires independent stop processes to complete one of two runs.
- LIFECYCLE-16 requires fresh stop to reconcile uncertain launch boundaries.

The draft's schema-2 refusal also fails existing local recovery checks, including
`TestLifecycleEngineLossRetainsRecoverableFailure`,
`TestPublicRecoveryBudgetRegressions` and
`TestRegisteredWaitReconcilesConcurrentStop`. These tests require actual recovery;
an early ownership refusal cannot replace it. No native scenario was run locally
to reproduce the corresponding engine cases.

Neither weakening or retagging those live scenarios nor implementing slice-18
without its specification is authorized by that handoff. The coordinator
needed to reconcile the slice boundary with the live recovery contract. This does
not reopen q-19's anchor or q-27's detector decisions.

The draft was saved as `/tmp/slice-17-anchor-prototype.patch`, based on `4c57651`,
then removed from the worktree. It is incomplete: besides the recovery conflict,
existing adapter/unit fixtures still need the new mount contract and new
production paths need shared-contract and unit coverage. Do not treat that patch
as a mergeable implementation or its local green as full regression evidence.
That checkpoint retained every slice-17 scenario as `@wip`; the combined
implementation below supersedes it.

## Shared-contract preparation after q-33

The handoff at that checkpoint permitted implementation preparation while gating
activation on slice-18's specification and integration. At that point:
the registry makes slice-18 depend on slice-17, so waiting for slice-17 closure
before preparing the recovery integration cannot satisfy the live contracts.
Q-36 below now arranges that integration without reconsidering q-19 or q-27. The shared `Recovery.stop` path must remain the teardown authority
for foreground and fresh-process cleanup.

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
The test increment itself activated no slice-17 scenario and supplied no native proof. The saved prototype still
needs the recovery integration and adapter coverage described above before a
behavior commit, and native green remains the coordinator's CI responsibility.

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

The reported CI line `FAIL .../features 600.444s` does not identify the active
test or its goroutine stack. A complete engine-disabled local package run passes
in 430.195 seconds, including all 1,348 live local scenarios in 404.03 seconds.
The sandbox's remapped root ownership first caused fixture refusals; this full
run used host ownership and disposable state, with the engine tier disabled.
No timeout or gate was changed. The CI timeout remains unverified pending its
full failure context; this passing run is not a claimed fix for that failure.
