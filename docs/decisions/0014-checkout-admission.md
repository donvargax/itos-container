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

After preflight, publish a `reserved` intent bound to the frozen snapshot before
public registration. It proves no create may yet have been sent. Before
registration, hold the common lock, check every unreleased reservation and legacy
registration, and publish a durable reservation
bound to the validated canonical root, public session ID and original intent.
The reservation initially carries `pending: true`. Register the public session,
then durably clear `pending` before returning the controller lease. Neither
project hashes nor daemon identities partition this lock. Paths compare whole
components, in either order. The existing public schema stays unchanged.

The lock spans only storage transactions. It never spans engine calls, foreground
wait or session operation-lock acquisition. Legacy scanning reads atomic public
records and validates original protected intent/snapshot bindings without consulting
mutable checkouts or other projects' daemons. Missing, unreadable or unsafe evidence
refuses admission. Under the same lock, a pending claim with no public record is
an interrupted publisher that never received launch authority; it cannot conflict.
Retain its evidence without deleting or adopting it. A missing public record with
an active or legacy claim still retains the conflict. A pending claim with a visible
public record remains a conflict and has its already-published intent for stop.
Unsafe/unreadable records never count as absent. This closes the publication gap
for new runs; existing legacy registrations without intents remain unresolved.

An unlocked public-record read can race normal atomic progress publication. Retry
the complete checked read up to three times on path or record-safety failures,
including the zero-link old descriptor left by a concurrent replacement. Every
attempt retains pinned-directory, no-follow, owner/mode/link and inode validation.
Stop immediately if the pinned directory changed or the context ended. A stable
unsafe record or exhausted retry bound still refuses; other failure classifications
return immediately. Locked storage readers keep their existing single-read semantics.

Cleanup holds its existing session operation lock. Fresh full-ID absence and durable
absent intent plus stopped public record precede publication of a completion
certificate under the common lock. A failed completion write cannot publish that
certificate before the required durable ownership facts. A certificate whose own
parent fsync fails can become visible safely because those facts are already durable.
Retained certificates prevent completed records from blocking future admission.
Repeated completion resyncs the existing certificate without replacing its inode.
Before sending create, advance `reserved` to `intent` durably under the session
operation lock. Stop may instead advance `reserved` to terminal `cancelled` and
durably publish the stopped public record and completion certificate. It contacts
no engine and invents neither a container ID nor a workload exit. A surviving
controller cannot cross that terminal state after readmission. Cancellation write
failures retain the claim until a retry establishes durability. `intent` still
means create may have been sent; missing-name inspection cannot cancel it.
An old registration without a certificate remains unresolved for admission until
product stop re-confirms and durably completes it; scanning does not repair it.

Controller exit, lease release, workload exit and a DELETE response alone cannot
release a registered reservation. Existing launch state checks under the operation
lock fence a paused controller after stop, even while its lease is held. Completing
one same-root session leaves every other session's reservation intact. Status and
stop remain project scoped.

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

Review regression OVERLAP-10 fails at its disjoint-worker activity assertion with
`run.session-registration-ownership` before the read retry. Adapter regressions
force replacement before open, after open and after read, keep the operation lock
held, and bound continuous replacement to three reads. Safety controls retain
refusal of symlinks, hard links, unsafe modes, changed directories and cancellation.
The new scenario and all replacement/safety regressions pass after the retry.

The same change restores Windows test compilation by spelling Linux-only diagnostic
signals numerically behind the existing OS-value guard. In-memory registry fixtures
now supply host-absolute paths, preserving the adapter's absolute-path refusal on
Windows as well as Linux. This adds no non-Linux container-host support claim.

Review regressions OVERLAP-11 (registration file fsync, parent fsync and controller
death before registration) and OVERLAP-12 (pre-create controller killed or resumed)
were committed red at `3cbb600`. All five failed at their Then assertions with
`run.checkout-overlap`; visible sessions could not be completed by stop. All five
pass with the pending-publication and pre-create cancellation protocol. Shared
store and admission contracts exercise both disk and memory adapters. Recovery
tests retain uncertainty after the create boundary and retry failed cancellation
writes without granting launch authority.

Coordinator q-28 authorizes revising live RECOVERY-25 in this fix: durable
pre-create proof permits cancellation without a concrete ID, while RECOVERY-14
continues to require uncertainty after create may have been issued. Before the
fix, the revised RECOVERY-25 failed at its first Then with recovery.temporary,
exit 75 and unknown cleanup instead of stopped/complete. The five overlap
regressions also failed again at their behavior assertions. Local filesystem
fixtures require real host ownership: the agent sandbox remaps root-owned
ancestors to UID 65534 and cannot supply this evidence.
