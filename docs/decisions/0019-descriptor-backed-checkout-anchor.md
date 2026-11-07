# Slice-17 descriptor-backed checkout anchor

## Authority and status

The coordinator's q-19 and q-27 decisions of 2026-11-06, approximately
22:55 and 23:50 UTC, select this mechanism and detector; the person reviews
them later. They override earlier
slice-17 requirements where they differ. This is an unproven Moby/kernel
hypothesis, not an implemented capability or a host-support claim.

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
the volume from the current checkout pathname. Slice-18 adds recovery's release
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
Production implementation and native green remain pending; the current product
still uses the unsafe pathname mount.

## Local checkpoint and recovery integration gap

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
without its specification is authorized by the current handoff. The coordinator
needs to reconcile the slice boundary with the live recovery contract. This does
not reopen q-19's anchor or q-27's detector decisions.

The draft was saved as `/tmp/slice-17-anchor-prototype.patch`, based on `4c57651`,
then removed from the worktree. It is incomplete: besides the recovery conflict,
existing adapter/unit fixtures still need the new mount contract and new
production paths need shared-contract and unit coverage. Do not treat that patch
as a mergeable implementation or its local green as full regression evidence.
The committed branch retains the red steps with every slice-17 scenario `@wip`.
