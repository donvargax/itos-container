# Security and trust boundaries

Use templates you trust: a made project contains their files and workflows. Do not put
credentials in answers, committed files, fixtures or logs. Owner and module answers identify a
public project, not authentication data.

The counting domain accesses input through its Files port. Its disk adapter reads files without
executing or modifying their contents. It is not a filesystem sandbox; a caller must decide
which paths it permits. The `count` CLI reads the path its own arguments name and writes no
file, so what a script may read is the script's decision, not the binary's.

Dependencies are locked with go.mod/go.sum and verified in CI. Tools keep their checked pins,
upstream identities, licenses and notices. Go's vulnerability scan examines used dependencies,
including test dependencies and the standard library. Mutation proof and its CI sample judge
recorded behavior; these controls do not isolate untrusted programs.

No render credential scan is installed at this stage. The template-check piece adds a pinned
scanner and proves every supported combination before claiming that protection. A template's
check commands can execute programs, so run them only in an environment appropriate to that
trust. itos's hooks and commit rules are workflow controls, not a security sandbox.

## Checkout Unix sockets: known milestone limit

The writable checkout bind exposes host pathname Unix sockets to the worker.
Network mode `none` and a private IPC namespace do not prevent this access.
Host sockets created or swapped into the checkout during a run remain reachable.
A host service listening there can therefore give the worker access to host
actions or networking even though the worker has no external network interface.

Following the person's q-30 decision, run scans the complete pinned checkout
before creating a worker, including nested directories and `.git`, without
following symlinks. A live or stale Unix socket causes `run.isolation-unverified`
(exit 1); an incomplete scan also prevents creation. The scan changes no files
or sockets. Its CI red was accepted in q-34; runtime green remains required.
It does not close the race after inspection or provide continuous enforcement.
Do not treat a socket-free startup check as proof of host IPC isolation.

Continuous enforcement is deferred to milestone 2 under
`p1-checkout-socket-kernel-enforcement`; see the
[decision](decisions/0019-checkout-socket-milestone-limit.md) and
[Landlock/gVisor research](plans/checkout-socket-enforcement.md).

## Allowlist egress and its limits

An approved allowlist run uses an isolated internal IPv4 bridge with no gateway.
The worker joins only that network; the proxy joins it and an owned egress bridge.
Its HTTP proxy variables are conveniences, while network isolation must prevent
direct routes independently of client cooperation. The controller verifies each
created resource before starting the worker and never falls back to another route.
Doctor and approve inspect both cached images and the installed static Linux amd64
controller outside the checkout. They never pull an image or execute a helper.

Status derives the grant from each session's validated original snapshot, even
when current requests or approval change or disappear. It names allowlist-v1 and
its original canonical names, for completed sessions too. Offline entries omit
the egress object. Approval, status and this document disclose the same limits:

- No TLS interception: the proxy verifies the CONNECT name and the first ClientHello's SNI.
- It cannot see the HTTP Host inside TLS or anything after that hello, a second hello after a HelloRetryRequest included, so fronting remains a limit, as does a hello the upstream reads differently, and it cannot stop data sent to an allowed service.
- Clients that omit SNI or send ECH, and SSH over 443, are refused.
- The proxy logs each refusal, but a stored log is no proof that every refusal was kept: a log that ends early reads as a complete one.

The proxy config ID observed at launch is execution evidence stored beside the
protected intent, independently of the approved proxy manifest digest. Recovery
requires that original observation and never derives it from current policy,
approval or a replacement cached image. The snapshot and fingerprint do not change.
Cleanup stops the worker and proxy, streams the refusal log into the protected
session directory, then removes the proxy, networks and anchor. A failed copy
retains the stopped proxy and its log for recovery. Slice-28 owns the additional
native proxy-failure and log-gap reporting proof. None of this closes the late
checkout Unix-socket gap described above.

## Approval boundary

[Slice-4's approval scenarios](../features/approval.feature) exercise the implemented
approval flow with temporary host state and a synthetic Engine API. The host identity is
the effective UID, root is refused, and the worker uses the effective host UID/GID.
Checkout policy and manifest files remain requests, never approval records.

Bare `itos container approve` at a terminal displays the effective settings and,
on re-approval, changes since the last approval, then asks y/N. A yes records those
settings; unchanged approved settings need no further prompt. A person never types
a hash. Without a terminal, bare approve refuses and names the optional
`--fingerprint <value>` route only for offline grants without credentials. `doctor`
prints the fingerprint; for those grants the option approves only matching settings.
Under q-52 an egress allowlist or credential grant requires the person at a real
terminal. Network and credential approval refuse every `--fingerprint`, even a matching value or
an already-approved request, with `approve.terminal-required` and a fix naming bare
`itos container approve` at a terminal. Bare nonterminal or JSON requests retain
`approve.confirmation-required` and name only that interactive form. Unchanged terminal approvals do not
prompt again; approval is asked again only when the settings fingerprint changes.

Approval's plain review, differences and status, including the effective settings
printed by ordinary `doctor`, render non-printing Unicode as visible JSON escapes.
This includes C1 and bidirectional controls in environment values and manifest
names. Printable Unicode stays readable. This display escaping does not change
the canonical settings, stored approval, fingerprint or structured JSON values.

An agent running unsandboxed on the host could fake a terminal. Terminal detection
therefore does not authenticate a person or a separate approver. Approval records
**what was approved, not who**; no keys or signatures are introduced. The fingerprint
route remains available for offline grants without credentials. This is the remaining
host-agent limitation after q-52, rather than permission to automate network consent.

Allowlist approval binds the canonical names, `allowlist-v1` profile and pinned proxy
image, and reviews the handshake and refusal-log limits. A matching version-2 grant
now starts the owned proxy and its networks before the worker. Existing matching
allowlist-v1 records remain valid without a new prompt. Local fixture tests cover
the wiring; the real-engine scenarios in CI must pass at the pushed head before
claiming runtime enforcement.

Credential approval binds sorted kinds, literal sources and fixed targets, never credential
contents or their hashes. Adding, removing or repointing a grant requires fresh consent;
valid content rotation at the same path does not. Metadata is checked before and after
engine inspection and after consent, including remembered grants. The policy root's
pinned device and inode must match the independent source walk's checkout node. The
shared filesystem rule accepts only ext4, btrfs and xfs; all other types, including NFS
and 9p, refuse. Root and selected-state text above 4095 bytes refuses before source
observation with a diagnostic naming that boundary and limit.

The credential review and help describe prospective exposure. Each listed file will be
readable by the worker; approval does not restrict token provider permissions. Delivery
will copy current source bytes into memory-backed staging retained until that worker's
teardown and never write the original. Workload output and traffic are not redacted.
An allowlisted host can receive credential bytes. A refused hostname can itself contain
credential bytes and is retained in proxy and host refusal logs; credential absence from
those logs is not established. Approval help makes these limits visible before a grant
is reviewed. **Delivery is still unavailable:** run refuses version 3 before manifest,
engine or host-state access, even for approved settings or no-grant version-3 policies.

Status shows only a session's validated protected frozen grants, without reading current
requests, active approval or source data. It cannot repair missing grants from current
requests. Seeded historical sessions prove presentation, not successful delivery. These
metadata observations do not prove atomic host state or continuous protection from
trusted-host transient changes. [Decision 0029](decisions/0029-credential-approval.md)
records the integration and remaining run handoff.

The fingerprint is SHA-256 of canonical effective JSON. Approval records
live under `$XDG_STATE_HOME/itos-container` (default `$HOME/.local/state/itos-container`),
outside the checkout, in effective-UID-owned directories of mode `0700`, with
records of mode `0600`. A hash detects inconsistent data; it does not prevent a
trusted host user from forging or restoring a record. Run refuses unapproved
settings and name `itos container approve`; it never obtains consent itself.

A second, fixed location exists whatever XDG_STATE_HOME selects: the first approve or
run creates `<account home>/.local/state/itos-container/coordination`, a mode `0700`
directory with the private `control.lock` (mode `0600`) and a catalog of every state
location the account has selected. It is protected control state like the records
themselves, and it lets approve, run and worker admission refuse a state tree that a
worker's checkout contains, equals or lies inside. The account home is read from the
effective UID's local `/etc/passwd` record, never from HOME, USER or LOGNAME, which a
checkout or caller could set. An account with no such record (LDAP or SSSD only) is
refused as `approval.state.unavailable` by doctor and approve (run reports `run.approval-invalid`), as is
a home that is missing, relative, `/` or not valid UTF-8, and a home reached through a
symbolic link refuses as `approval.state.path`; see [CONFIG.md](CONFIG.md). The home directory itself is never locked, because another
user can lock a readable home and stall every command. A person who changes XDG_STATE_HOME
keeps every earlier location in the catalog; a state tree another worker's checkout could
reach refuses with `approval.state.exposed`, which reapproval alone cannot resolve.

The catalog trusts what it recorded, so losing a recorded location locks out the whole
account: a retained directory that is gone refuses every doctor and approve with
`approval.state.unavailable` (run refuses it as `run.approval-invalid`), and one recreated
at the same path refuses with `approval.state.path`, because the binding names the original
inode. The product never forgets or repairs a location, since doing so would let a worker checkout reach a tree
that other projects' records may still live in. Recovery is the host user's own act,
described in [CONFIG.md](CONFIG.md): restore the original directory, or remove that
location's record after confirming that no worker uses it. A command that does this with
consent is a decision for the person, not part of this release. The catalog stores each
path as JSON, so a state tree or account home that is not valid UTF-8 refuses before
anything is created.

The shared approval contract supplies an independent frozen input for later
session startup. Subsequent checkout edits or approval cannot change that value.
Protected placement and the checkout-only mount contract are requirements, not
proof that a worker cannot access host state. The later run slices must establish
that property with CI runtime evidence. All approval scenarios use temporary state;
agents must never approve against the person's real state folder.

One active canonical record per effective UID and checkout lives at
`approvals/<project>/current.json`. Changed consent increments its generation;
reverting to an earlier configuration does not reactivate an older record.
Read-only inspection creates no state. Publication rechecks settings after consent,
locks the pinned project directory, compares the reviewed generation, fsyncs a
private temporary file, renames it and fsyncs the parent. A failure after rename is
reported as uncertain durability; the next inspection reads the actual record.
Doctor creates no lock file and writes nothing: it takes a shared lock on the account's
`control.lock` only when that file already exists. Approve and run take the exclusive
lock for publication and admission, never across consent or engine calls. Every wait
ends within the 10-second call budget as the retryable `approval.state.busy` (exit 75;
`run.call-timeout` for run). The budget also covers the discovery scan that follows the
lock, so a scan the budget interrupts ends the same way, never as a path or state
refusal.
Symlinks, hard-linked records, foreign ownership and unsafe permissions are refused,
not repaired. State ancestors must be trusted; root-owned sticky `/tmp` above a
private fixture parent is allowed.
