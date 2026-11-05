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

## Specified approval boundary

[Slice-4's approval scenarios](../features/approval.feature) specify the planned
approval flow; it is not implemented by this documentation. The host identity is
the effective UID, root is refused, and the worker uses the effective host UID/GID.
Checkout policy and manifest files remain requests, never approval records.

Bare `itos container approve` at a terminal displays the effective settings and,
on re-approval, changes since the last approval, then asks y/N. A yes records those
settings; unchanged approved settings need no further prompt. A person never types
a hash. Without a terminal, bare approve refuses and names the optional
`--fingerprint <value>` route. `doctor` prints the fingerprint; the option approves
only those exact settings and refuses a mismatch.

An agent on the host can approve its own sandbox through `--fingerprint` or a
faked terminal. Approval records **what was approved, not who**. Neither route
proves human presence or authenticates a separate approver. Revisit this limitation
before adding egress or credentials, as q-2 requires.

The specified fingerprint is SHA-256 of canonical effective JSON. Approval records
live under `$XDG_STATE_HOME/itos-container` (default `$HOME/.local/state/itos-container`),
outside the checkout, in effective-UID-owned directories of mode `0700`, with
records of mode `0600`. A hash detects inconsistent data; it does not prevent a
trusted host user from forging or restoring a record. Run must refuse unapproved
settings and name `itos container approve`; it never obtains consent itself.

The shared approval contract supplies an independent frozen input for later
session startup. Subsequent checkout edits or approval cannot change that value.
Protected placement and the checkout-only mount contract are requirements, not
proof that a worker cannot access host state. The later run slices must establish
that property with CI runtime evidence. All approval scenarios use temporary state;
agents must never approve against the person's real state folder.
