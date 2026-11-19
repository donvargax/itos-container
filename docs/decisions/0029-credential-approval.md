# Credential grants in approval and frozen status

Slice-30, 2026-11-16. Implements
[credentials-approval.feature](../../features/credentials-approval.feature) after
slice-26 and slice-29. q-52 supplies terminal consent; q-64 supplies the exact ext4,
btrfs and xfs allowlist, including outright NFS and 9p refusal. These are settled
choices, not new questions. No dependency is added.

## Approved authority

Schema 1 keeps its existing envelope and gains optional sorted closed
`{kind,source,target}` objects under one top-level `credentials` array. A nonempty
set selects `offline-credentials-v1` or `allowlist-credentials-v1`, with policy
version 3. No-grant version-1/2 bytes and fingerprints stay unchanged. No-grant
version 3 keeps the base profile and declares version 3. First/last grant changes
also change `/profile`; grant differences are one `/credentials` value, with
absence represented by null. Source rotation at an unchanged valid path is not a
settings change. Records never hold source bytes, hashes or inode metadata.

Stored grants are canonical, closed, sorted and validated independently of their
hash. A valid historical target under the reserved directory is readable history
that requires new approval; malformed grants are invalid state. Existing codec,
store, generation, atomic publication and freeze contracts remain in force.

Current credential or network grants require terminal stdin and stdout and plain
mode. Every supplied fingerprint refuses after readiness, before fingerprint
parsing, equality or matching shortcuts. Bare redirected/JSON approval keeps its
confirmation refusal. An unchanged terminal approval revalidates and shows the
complete disclosures without another prompt or rewriting its approval. The
terminal cannot authenticate a person: an unsandboxed host agent can fake it.

The review and help describe provider permissions, prospective memory-backed
copying retained until worker teardown, unchanged originals, lack of workload
redaction, allowed-host exfiltration and credential-bearing hostname refusal
logs. This increment implements records and presentation, not delivery. Run keeps
its early version-3 gate, including approved and no-grant requests.

## Source admission and architecture changes

This overrides arc42's **Credential source location**, **Approval and ownership
are distinct** and the corresponding building-block rows. It also supersedes
decision 0027's accepted read-only gap between the policy pin and independent
source walk, and its note retaining length-free boundary messages.

Ordinary doctor and approve consume the shared readiness observer before engine
inspection, after inspection and after terminal consent, including remembered
grants. The scoped credentials-only command uses the same observer. Each walk's
checkout device and inode must match the retained policy descriptor's identity;
mountinfo's device is a different observation and is never substituted for it.
A mismatch is `doctor.checkout.changed`, exit 1, before a usable candidate, or
`approve.changed` after consent. This compares two observations; it does not make
them atomic, prevent transient trusted-host changes or bind a later run mount.

Root text above 4095 bytes refuses before policy pin/open. Selected state text is
checked after valid policy and before source/boundary observation; its length
includes `/itos-container`. Other short invalid state selections retain the existing
store/control refusal order without grants, including account-home error precedence.
Ordinary consumers apply both bounds even without
grants. Both diagnostic message and fix name the boundary and 4095-byte limit.
Credentials-only retains its no-grant no-scan result. Run receives these checks
when its delivery gate lifts, not through this change.

The shared `readiness/source` adapter supplies account-boundary selection and the
retained policy pin to the metadata domain. It reuses approval's Account port for
the fixed account home; readiness's domain and ports still do not call approval.
Only cmd binds this adapter. Doctor's scoped adapter retains pin/close sequencing
and delegates observation through the readiness port. Shared credential and
state diagnostics live in readiness ports so all consumers preserve written
source indices, rules, exits and fixes. File-only scopes do not inspect sources.

The source observer accepts only ext4, btrfs and xfs in both mount observations,
under decision 0027's source/ancestor and checkout/state relevance conditions.
Unrelated rows stay harmless; missing mapping and known location precedence stay
in force. Synthetic observations exercise the complete matrix; they do not
relabel a native unsupported filesystem or establish native bind-mount support.

Recovery's existing codec projects credential grants only after validating the
protected frozen snapshot. Binding values retain two independently owned grant
slots because this schema permits exactly two kinds; no new runtime progress or
resource field is stored. Status adds the exact sorted array only for credential
sessions and uses the shared approval display through a cmd-supplied callback.
Plain status labels grants by session and omits the temporary delivery warning.
Current policy, active approval and absent sources cannot repair invalid history.

## Verification and supersession

Steps were committed while all 29 scenarios remained wip. With wip removed for
the baseline run, all 126 examples failed at their first Then, with no undefined
steps, ambiguity or setup failure. After implementation all 126 examples and all
983 steps pass using private synthetic sources, temporary protected state and a
recording HTTP Engine API. No Docker daemon, provider account or real credential is used.
Native checks need actual root metadata; a filesystem sandbox mapping root to
UID 65534 correctly fails the trusted-ancestor rule and is not positive evidence.

Slice-29 ID-CREDENTIALS-13 retains its run rows and invalid-policy precedence;
its ordinary doctor/approve gate rows are replaced by this admission coverage.
Slice-26 ID-APPROVAL-50 loses only its two version-3 base-profile corruption rows,
now valid no-grant records covered by ID-CREDAPPROVAL-04/26 and codec/store checks.
The shared fake/disk source contracts use named filesystem controls and extend
the policy-pin and filesystem matrices. No real mount is made. Fresh changed-code
proof and all real-engine evidence remain CI-owned.
