# Slice-17 descriptor-backed checkout anchor

## Authority and status

The coordinator's q-19 decision of 2026-11-06, approximately 22:55 UTC,
selects this mechanism; the person reviews it later. It overrides earlier
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
by reading that link. Before releasing the workload, the controller must
verify from the engine side that the mounted workspace's device and inode
equal the validated identity. Mismatch or unavailable proof means owned
teardown and refusal, never a checkout-path fallback.

Durable intent precedes volume allocation. Independent inspection of the
volume's identity, labels and exact options precedes durable concrete ownership
and worker creation. Creating a local volume does not itself establish the
backing mount, so mounted-inode verification belongs before workload release.
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
