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
No dependency changes: use the pinned Go os/user.LookupId and existing x/sys descriptor
locks, protected directory primitives and JSON encoding.

Resolve the effective UID's account home independently of HOME, USER, LOGNAME and
XDG_STATE_HOME. Pin its existing directory and ancestors, verify ownership and writes,
and lock that same directory inode before all catalog and selected-state locks.
Read-only inspection takes shared exclusion and never creates or fsyncs state.
Writers take exclusive exclusion; no common lock spans consent, engine I/O, or
session operation-lock acquisition. Initial intent publication releases common
exclusion before returning authority to its caller. Admission retains it through
reservation and public-registration publication; completion retains it through
its existing durable certificate transaction.

Treat the account home's .local/state/itos-container tree as protected before it
exists. Recheck both that prospective footprint and the selected managed location
against all discoverable outstanding reservations before mkdir. Track roots across
XDG selections without project or daemon filtering. Reuse the original pending,
pre-create cancellation, legacy intent/snapshot validation and completion rules;
controller loss, acknowledged deletion and uncertain completion do not release a
reservation. Discovery never contacts another daemon or acquires an operation lock.

After consent, or validated existing approval, bootstrap only the fixed private
chain and coordination files. Sync created directories and parents, atomically
publish self.json binding the entire fixed tree and coordination inode, and finish
file/rename/parent durability before ordinary selected-state writes. The default
selected tree uses this same binding. Keep safe surviving directories and private
unpublished files on failure. A retry revalidates and resyncs them; a visible complete
binding is resynced without replacing its bytes or inode. Inspection never repairs.

The catalog records schema, UID, absolute path, completion and ordered pinned
ancestor identities. A separate location name is SHA-256 of the JSON-encoded path.
Before creating an absent selected tree, durably publish its pending claim; retain
that protection after interruption and complete its root identity before success.
Completed missing, replaced, malformed, unsafe or unreadable bindings refuse.
Missing catalogs beside session/admission/intent evidence are unavailable, never an
empty-worker inference. Valid legacy approvals can be inspected without writes and
enrolled without rewriting their generation, bytes, inode or mtime.

Approval checks separation on inspection and inside publication after consent.
Matching reuse also enters the common transaction. Run validates existing approval
before enrollment and rechecks it under common exclusion. Preparing-intent creation
and worker registration use the same account lock, so cancellation while publication
holds exclusion creates neither an intent nor public launch authority. Location
bindings remain after controller exit and worker cleanup.

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
required before claiming protection for existing installations. Slice-22 consumes
retained locations for later exposing mount decisions; slice-23 protects recovery
record trust. Neither behavior is silently added here. Fresh changed-code proof,
real-engine evidence, push and closure remain with the coordinator's CI loop.
