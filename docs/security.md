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

## Approval boundary

[Slice-4's approval scenarios](../features/approval.feature) exercise the implemented
approval flow with temporary host state and a synthetic Engine API. The host identity is
the effective UID, root is refused, and the worker uses the effective host UID/GID.
Checkout policy and manifest files remain requests, never approval records.

Bare `itos container approve` at a terminal displays the effective settings and,
on re-approval, changes since the last approval, then asks y/N. A yes records those
settings; unchanged approved settings need no further prompt. A person never types
a hash. Without a terminal, bare approve refuses and names the optional
`--fingerprint <value>` route. `doctor` prints the fingerprint; the option approves
only those exact settings and refuses a mismatch.

Approval's plain review, differences and status, including the effective settings
printed by ordinary `doctor`, render non-printing Unicode as visible JSON escapes.
This includes C1 and bidirectional controls in environment values and manifest
names. Printable Unicode stays readable. This display escaping does not change
the canonical settings, stored approval, fingerprint or structured JSON values.

An agent on the host can approve its own sandbox through `--fingerprint` or a
faked terminal. Approval records **what was approved, not who**. Neither route
proves human presence or authenticates a separate approver. Revisit this limitation
before adding egress or credentials, as q-2 requires.

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
refused as `approval.state.unavailable`. The home directory itself is never locked,
because another user can lock a readable home and stall every command. A person who
changes XDG_STATE_HOME keeps every earlier location in the catalog; a state tree
another worker's checkout could reach refuses with `approval.state.exposed`, which
reapproval alone cannot resolve.

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
`run.call-timeout` for run).
Symlinks, hard-linked records, foreign ownership and unsafe permissions are refused,
not repaired. State ancestors must be trusted; root-owned sticky `/tmp` above a
private fixture parent is allowed.
