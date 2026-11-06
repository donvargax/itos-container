# Cross-project checkout admission

Slice-16 extends arc42's **Building-block view**, **Foreground run and its failure
paths** and **Recovery**. Its [scenarios](../../features/checkout-overlap.feature)
require atomic refusal of strict ancestor/descendant checkouts across projects and
selected daemons in one effective user's protected control-state root. Equal roots
retain their existing concurrency contract; disjoint roots remain independent.

## Decision

The session engine owns comparison, reservation lifetime and legacy ownership.
Approval's protected storage port supplies one pinned `admission` directory lock,
reservation files and read-only enumeration of all protected public registrations.
This reuses the pinned Go and x/sys filesystem primitives; no dependency is added.

After preflight and before public registration, hold the common lock, check every
unreleased reservation and legacy registration, and publish a durable reservation
bound to the validated canonical root, public session ID and original intent.
Only then register and return the controller lease. Neither project hashes nor
daemon identities partition this lock. Paths compare whole components, in either
order. The existing public schema stays unchanged.

The lock spans only storage transactions. It never spans engine calls, foreground
wait or session operation-lock acquisition. Legacy scanning reads atomic public
records and validates original protected intent/snapshot bindings without consulting
mutable checkouts or other projects' daemons. Missing, unreadable or unsafe evidence
refuses admission. An interrupted reservation or registration remains conservative;
the existing registration-before-intent recovery gap is tracked separately.

Cleanup holds its existing session operation lock. Fresh full-ID absence and durable
absent intent plus stopped public record precede publication of a completion
certificate under the common lock. A failed completion write cannot publish that
certificate before the required durable ownership facts. A certificate whose own
parent fsync fails can become visible safely because those facts are already durable.
Retained certificates prevent completed records from blocking future admission.
Repeated completion resyncs the existing certificate without replacing its inode.
An old registration without a certificate remains unresolved for admission until
product stop re-confirms and durably completes it; scanning does not repair it.

Controller exit, lease release, workload exit and a DELETE response cannot release
a reservation. Existing launch state checks under the operation lock fence a paused
controller after stop, even while its lease is held. Completing one same-root session
leaves every other session's reservation intact. Status and stop remain project scoped.

A known conflict returns exit 1 and `run.checkout-overlap`, with original-checkout
recovery guidance and no contender engine mutation. Existing invocation, prerequisite
and approval refusals precede admission.

## Evidence

The coordinator accepted the 19 local assertion-level red examples from the committed
steps at `56f860c`, and native OVERLAP-06 (ancestor and descendant) and OVERLAP-07
assertion failures in run 42307612900 at `d790e7a`, on 2026-11-06 around 23:00 UTC.
That settled decision authorizes this implementation and supersedes the old handoff.
All 19 local examples pass after implementation. Shared contracts exercise memory and
disk admission, reservation-write failure, equal roots, disjoint controls, lease loss
and release after every session completes. Storage tests force a publication barrier
under real descriptor locks and check unsafe paths. Existing local and native state
inventories validate each added certificate's exact binding, completion, owner, mode
and link count; negative harness controls retain rejection of unrelated changes.
Native green evidence and fresh
changed-code proof belong exclusively to the coordinating loop's CI run.

The same change restores Windows test compilation by spelling Linux-only diagnostic
signals numerically behind the existing OS-value guard. In-memory registry fixtures
now supply host-absolute paths, preserving the adapter's absolute-path refusal on
Windows as well as Linux. This adds no non-Linux container-host support claim.
