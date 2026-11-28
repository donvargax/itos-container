# Workspace copy mode

Status: proposed, 2026-11-27

This proposal is outside the current milestone. The five steps of milestone two
can use the live checkout mount. The charter calls for recording ideas outside
the milestone without specifying them for implementation. The person described a
copied workspace on 2026-11-09 as “an alternative way of working” in the discussion
recorded in T-98. [Milestone two](../charter.md#the-second-milestone-a-coding-agent-inside-the-sandbox) ·
[Bounds for the loop](../charter.md#bounds-for-the-loop) ·
[T-98 ledger entry](../../tasks/phase-1.yaml#L10064)

## Why copy a checkout

A run could start from a known Git commit in its own volume and return commits for
the host to review and land. The run would never write to the host checkout.
Separate copies would let several runs start from the same checkout independently.
The trade-offs are transfer time and storage for large repositories, losing the live
view in the host editor, and possible merge conflicts if the host branch moves while
the run is in progress.

A remote sandbox has no shared host filesystem to mount. A copied workspace fits
the settled direction that microVMs come later, on several cloud providers
(`docs/charter.md:157`). Under the standing rule, GitHub credentials stay on the
host, which pushes and lands (`docs/decisions/STANDING.md:39`).
[Charter](../charter.md#settled-by-the-person) ·
[Standing decisions](../decisions/STANDING.md#the-persons-calls)

## Getting the checkout in and the commits back

| Direction | Option | Trade-off |
| --- | --- | --- |
| In | Write a Git bundle into the sandbox's own volume. | The starting commit can be seeded without a Git network route from the worker. The host has to prepare and transfer the bundle before the run starts. |
| In | Let the worker clone from a read-only origin, as in Discobox. | The worker can fetch from a served repository. A local run would need an egress allowlist entry for that origin, and the origin must be available during the fetch. |
| Out | Return a bundle with the commits based on the supplied starting commit. | The host can import and review the result after the run using the starting history it already has. The bundle must be collected before its sandbox storage is torn down. |
| Out | Retain the result on a sandbox-local branch the host can fetch. | The host pulls the result directly. The sandbox repository must remain reachable until the fetch finishes. |

Either way in, the host would record the exact starting commit before the run.
For either return path, the worker commits inside the sandbox and the host
imports the result into a review ref outside its active checkout. The recorded
starting commit gives the host a base for reviewing the returned commits even
if its branch advanced during the run. The host reconciles any divergence before
pushing or landing the result and leaves the host working tree untouched during
handoff.

The T-98 ledger records Discobox at `97b2281`, read on 2026-11-23, as a worked
example. It describes a read-only Git origin served to the sandbox, optional
inclusion of uncommitted changes on explicit request, and commits fetched back by
the host. In this proposal the worker returns commits through the handoff, and
only the host pushes to a shared Git remote.
[T-98 ledger entry](../../tasks/phase-1.yaml#L10064)

## What changes at the checkout boundary

A copy would seed a fresh repository from Git data. Host inodes, Unix sockets and
checkout-local Git metadata such as installed hooks would stay behind. The worker
would also lose the live mount as a route to host control state.

| Decision | With a copied workspace | With the live mount |
| --- | --- | --- |
| [0013](../decisions/0013-refuse-shared-checkout-inodes.md) | No host file inode is shared with the worker, so the mount-specific single-link scan is unnecessary for admitting the copy. | Keep bounded traversal and the single-link requirement for regular files, including Git objects. |
| [0019](../decisions/0019-checkout-socket-milestone-limit.md) | The host checkout's socket entries are outside the worker's filesystem, so the copy needs no startup scan for those entries and has no route to sockets added later through that checkout. | Keep startup refusal for checkout sockets. The charter's late-socket policy still applies. |
| [0020](../decisions/0020-descriptor-backed-checkout-anchor.md) | A sandbox-owned volume has no host checkout bind source to swap, so there is no checkout descriptor to hold through start or host checkout inode to verify in the worker. | Keep the descriptor-backed anchor and post-start identity check for the live checkout. |

## Questions open for the person

1. **Should the pilot's mount rule change?** The current rule makes the checkout
   the only host filesystem mount (`docs/plans/container-pilot.md:95`).
   **Proposed.** Keep the rule until copy mode is scheduled, then amend it so
   a copy-mode run has no host mount at all.
   [Pilot contract](container-pilot.md#agreed-scope)
2. **Should copy mode be offered alongside the mount or only for remote
   sandboxes?** **Proposed.** Start with remote sandboxes. Consider local copy
   mode later as an option alongside the live mount.
3. **What happens to uncommitted edits in the host checkout?** **Proposed.**
   Start from committed history unless the person explicitly asks to include
   uncommitted edits on that run. Return results only as commits and leave the
   host working tree untouched during handoff, so existing uncommitted edits
   are never overwritten.
