# Protect control-state publication across selected roots

Slice-21 updates arc42's **Building-block view**, **Foreground run and its failure
paths**, **Recovery** and **Deployment view**. It supersedes decision 0014's
per-selected-root exclusion for control-state publication and admission. It adds
no runtime-isolation or migration support claim.

## Decision

The approval engine owns the account-home observation and common control transaction
ports, whole-component exposure decision, persistent location bindings, and protected
disk adapter. The session engine supplies reservation validation through an approval
port. Only cmd connects those adapters. The slices retain their existing thin handlers.
No dependency changes: use pinned Go's bufio/strings for local account records and
existing x/sys descriptor locks, protected directory primitives and JSON encoding.

Review PRRT_kwDOVkB0ws606rqO supersedes the original os/user.LookupId choice and
clarifies arc42's **Recovery** account-home source. In the CGO-disabled Linux release,
LookupId can return Current's USER/HOME-derived result when the UID is missing from
the account database. Its result alone cannot prove the shared anchor's provenance.
The host adapter instead reads `/etc/passwd` directly and requires a complete
seven-field local record for the effective UID, a valid numeric GID and an absolute
home. Ignore comments, unrelated rows and NIS directives; absent or unreadable
evidence returns state.unavailable. NSS-only accounts are refused, not assigned an
environment-selected anchor. No process environment is changed or subprocess used.

The reviewed [Moby sys/user v0.4.0 parser](https://github.com/moby/sys/blob/user/v0.4.0/user/user.go)
silently ignores numeric conversion errors, so it does not meet this boundary's
fail-closed requirements. A small standard-library field reader avoids that
dependency and preserves the spec's no-new-dependency constraint.

Resolve the effective UID's account home independently of HOME, USER, LOGNAME and
XDG_STATE_HOME. Pin its existing directory and ancestors, verify ownership and writes,
and retain their identities without locking the home inode. Coordinator q-35
(2026-11-07, approximately 17:40 UTC) supersedes the original home-lock protocol:
a different UID can flock a readable home and deny service. Homes may remain 0755
or 0750; only other-user writes are forbidden.

Bootstrap with mkdir(0700), accepting EEXIST; open coordination with
O_DIRECTORY|O_NOFOLLOW and validate the descriptor's UID, exact 0700 mode and
pinned identity. Exclusively create control.lock there, or open the existing file
without following links, validating an owned 0600 regular single-link inode.
Racing same-user creators converge on that inode. Never replace, unlink or repair
it on retry. Take its flock before catalog and selected-state locks, revalidate
the directory and file identities after acquisition and through publication.
A writer whose initial unlocked inspection finishes after a peer creates the
lock retains its pinned directory checks, validates and acquires that lock, then
repeats discovery under exclusion. Only read-only transactions require a lock
observed absent to remain absent through their return.

Every lock wait is bounded by the 10-second call budget. Contention returns typed,
retryable approval.state.busy (75); run retains run.call-timeout (75). Neither a
home-directory lock nor a background context can create an unbounded wait.
Read-only inspection takes shared private-file exclusion and never creates or
fsyncs state. If the lock is absent it may inspect only safe bootstrap-only or
legacy-approval state, rechecking absence before returning. Completed catalog or
session evidence without its lock refuses; inspection never restores a lock.
Writers take exclusive exclusion; no common lock spans consent, engine I/O, or
session operation-lock acquisition. Initial intent publication releases common
exclusion before returning authority to its caller. Admission retains it through
reservation and public-registration publication; completion retains it through
its existing durable certificate transaction.

Treat the account home's .local/state/itos-container tree as protected before it
exists. Recheck both that prospective footprint and the selected managed location
against all discoverable outstanding reservations before mkdir, then repeat
discovery and checks after acquiring the private lock. Track roots across
XDG selections without project or daemon filtering. Reuse the original pending,
pre-create cancellation, legacy intent/snapshot validation and completion rules;
controller loss, acknowledged deletion and uncertain completion do not release a
reservation. Discovery never contacts another daemon or acquires an operation lock.

After consent, or validated existing approval, bootstrap only the fixed private
chain and private lock before exclusion. After locking and rechecking, sync the
lock, directories and parents, then atomically
publish self.json binding the entire fixed tree and coordination inode, and finish
file/rename/parent durability before ordinary selected-state writes. The default
selected tree uses this same binding. Keep safe surviving directories and private
unpublished files on failure. A retry revalidates and resyncs them; a visible complete
binding is resynced without replacing its bytes or inode. Inspection never repairs.

The catalog records schema, UID, absolute path, completion and ordered pinned
ancestor identities. A separate location name is SHA-256 of the JSON-encoded path.
Review PRRT_kwDOVkB0ws61FzgB clarifies arc42's **Recovery** selection boundary:
the coordination directory is reserved for catalog metadata. Reject a selected
managed root equal to or below it with state.path before bootstrap, binding
publication or selected-tree creation, even when the catalog does not exist.
Otherwise ordinary state directories become unexpected catalog entries and block
every checkout on the account. Compare whole path components so the fixed default
tree and a coordination-more sibling remain usable. Inspection and direct write
transactions enforce the same rule; refusal preserves all existing state.

Review PRRT_kwDOVkB0ws61F-IT extends arc42's **Recovery** selection boundary to
the sessions and admission collections of the fixed tree and every enrolled
location, including pending claims. A nested selected tree would introduce names
that reservation discovery cannot interpret, blocking future control transactions
across the account even after changing XDG_STATE_HOME. Refuse with state.path before
bootstrap, binding publication or selected-tree creation. Check the prospective
fixed tree before bootstrap and discovered locations under the common lock before
enrollment. Preserve exact path-component boundaries: sessions-more and
admission-more siblings remain usable. Reads and direct writes share the rule,
leave existing records unchanged, and do not repair previously corrupted state.

Review PRRT_kwDOVkB0ws61IKmj makes the candidate-checkout comparison symmetric.
The earlier rule compared a managed location only with a checkout above it, so a
checkout rooted at a managed tree's approvals collection was accepted and then
mounted read/write under the host UID, exposing the records of every project on
the account. Doctor, approve and run admission now refuse state.path when the
checkout equals, contains or lies beneath the fixed tree, the selected tree or any
cataloged location, including pending claims. The selected tree is refused before
any bootstrap or claim; enrolled locations repeat the check under common exclusion
after preflight, so a later enrollment by another project still blocks an already
prepared nested worker. Whole-component boundaries keep textual-prefix siblings
usable. Refusal creates no binding, tree entry, catalog change or launch authority.

Review PRRT_kwDOVkB0ws61Iu4r applies the same symmetry to the reservations of
workers that are already active. The separation check compared each reserved worker
root with the fixed and selected trees one way, so a selection above a worker, whose
checkout is that tree's approvals collection, passed doctor and approve. Enrolling it
then wrote every project's approvals into the worker's read/write workspace. Both
trees are now compared with every reserved root in both directions and refuse
approval.state.exposed (exit 1) before any binding, tree entry or approval record.
Completed and pending-without-record reservations keep their existing semantics, and
whole-component boundaries keep textual-prefix siblings usable.

Review PRRT_kwDOVkB0ws61JLQ0 stops treating a persisted st_dev as identity. Device
numbers are not stable across boots: btrfs subvolumes receive anonymous numbers and
device-mapper or NVMe minors can follow activation or enumeration order. Comparing
the recorded device refused every doctor, approve and run on the account after a
renumbering of the device holding the account home or an enrolled location, and
nothing repaired the binding. A binding is now matched by path and inode. Its device
stays in schema 1 as a shape-checked diagnostic, so existing records remain readable
and a non-numeric device is still invalid. A changed inode, symlink, ownership, mode,
link count, path or ancestor chain still refuses, and the live pins of one transaction
still compare device and inode together. The limit is that a directory on another
filesystem that happens to reuse an enrolled inode number is no longer distinguished
by this record; creating one at an ancestor needs mount privileges, which this
project's hostile-checkout model does not grant a worker. A stable filesystem ID from
statfs is not uniformly populated or stable across filesystem types, and a
person-consented rebind needs a new command and consent surface that the charter does
not authorize here. CONTROL-23 specifies the renumbered rows; CONTROL-07 keeps the
replaced-root refusal.

Before creating an absent selected tree, durably publish its pending claim; retain
that protection after interruption and complete its root identity before success.
Completed missing, replaced, malformed, unsafe or unreadable bindings refuse.
Missing catalogs beside session/admission/intent evidence are unavailable, never an
empty-worker inference. Valid legacy approvals can be inspected without writes and
enrolled without rewriting their generation, bytes, inode or mtime.

Approval checks separation on inspection and inside publication after consent.
Matching reuse also enters the common transaction. Run validates existing approval
before enrollment and rechecks it under common exclusion. Preparing-intent creation
and worker registration use the same private lock, so cancellation while publication
holds exclusion creates neither an intent nor public launch authority. Location
bindings remain after controller exit and worker cleanup.

Review PRRT_kwDOVkB0ws61FpuQ extends the own-checkout exclusion to every cataloged
location in the common transaction. A candidate root equal to or containing any
retained location refuses with state.path, including pending claims for absent
trees, even when no worker reservation remains. Reads check this before trusting
approval; admission repeats it under exclusion after preflight. This closes the
approval-first ordering in which a later worker could otherwise mount another
project's control records. Whole-component comparison retains harmless textual
prefixes and equal basenames. No catalog, approval or worker authority is created
by this refusal. Run carries state.path through its shared prerequisite presenter
as approval.state.path instead of reducing it to generic invalid approval and
suggesting reapproval. Admission keeps run.ownership-unsafe; both exit 1.

Known exposure reports approval.state.exposed with exit 1 and invalid approval,
retaining the independently validated candidate. Guidance selects an unexposed
location or recovery from the blocking worker's original checkout. Reapproval alone
cannot resolve exposure. Other failures preserve the feature's path, unsafe, invalid,
unavailable and write classifications. Policy-only and manifest-only doctor retain
file-only scope and do not resolve the account home.

## Evidence and limits

Coordinator q-31 accepted the red evidence recorded in commit 4d64985: all 45 local
examples failed at behavior assertions against recording HTTP engines. They are not
real-engine scenarios; no CI red run is required. The implementation passes those
examples, including publication/admission interleavings, bootstrap faults and retries.
The shared memory/disk contract tests read-only inspection, exposure, cancellation,
retained claims, write failures, independent roots and replaced identities. Additional
adapter tests exercise malformed records and filesystem failures. Production account
lookup is checked separately without writing account state.

Existing feature assemblies now substitute a disposable account home in the test
executable, use real command parsing, and seed current catalog bindings independently
when their premise is already-approved session state. CONTROL keeps explicit legacy
and absent-catalog fixtures. The plain diagnostic observer accepts doctor's existing
CLI prefix while requiring the same exact rule and nonempty message/fix. Successful
approval inventories validate exact new bindings rather than exempting catalog trees.

Only enrolled roots and the explicitly selected legacy root are discoverable. An
upgrade contract for unseen old XDG roots and mixed old/new controllers is still
required before claiming protection for existing installations. CONTROL-16 proves
the retained-location refusal with recording engines in both fresh-run and
publication-after-preflight schedules. Slice-22 retains broader mount enforcement
and native runtime evidence; slice-23 protects recovery record trust. Fresh
changed-code proof, real-engine evidence, push and closure remain with the
coordinator's CI loop.
