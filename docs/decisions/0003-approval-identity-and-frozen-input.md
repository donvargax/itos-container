# Slice-4 approval, host identity and frozen input

## Authority and status

Specified on 2026-11-04 from the coordinator's settled slice-4 assignment and the
person's q-2 answer. The specification is implemented in slice-4; local fixture evidence is recorded
below and does not establish runtime isolation. [The scenarios](../../features/approval.feature) carry every behavior
decision and the implementer's red-first handoff. The charter's offline scope holds.

## Decision

The coordinator assigns effective host identity, UID resolution and frozen snapshot
input to slice-4. The effective UID identifies the host user; root is refused.
Both manifest users resolve to that UID and explicit different UIDs are refused.
The worker uses the effective host UID/GID, overriding image Config.User without
username lookup or chown. File-only manifest inspection still reports omitted users
as unspecified. This supersedes ID-DOCTOR-29's image-user comparison.

Approval follows q-2's terminal settings/difference review and y/N consent. The
optional exact `--fingerprint` route is available without a terminal; bare approve
without a terminal refuses. Doctor exposes the fingerprint. Host agents can use
the option or fake a terminal, so approval records what was approved, not who.

The spec defines canonical JSON SHA-256, a single active generation per host user
and checkout, protected host storage, atomic replacement and frozen-input selection.
It chooses standard-library JSON/hash operations and existing pinned terminal and
OS libraries, adding no dependency. Schema, canonical fields, storage paths,
failure rules, concurrency and independent fixture oracles live in the feature's
comments so the implementer does not need to infer them from this summary.

Slice-4 wires run's approval refusal and guidance while keeping approved startup
unimplemented. This supersedes ID-RUN-06/07's placement of the interim refusal
before prerequisite inspection. Their argument-preservation assertions remain
required with approved temporary fixtures. Successful run, durable session snapshot
publication and actual worker isolation remain later slices' responsibilities.

## Architecture sections clarified

This overrides the allocation of effective UID work to slices 3/8 in arc42's
**Building-block view**, **Runtime view / Foreground run**, **Deployment view** and
**Decisions**. The matching table entries now assign resolution and frozen input
to slice-4; slice-8 consumes that input and publishes the session snapshot.
It refines **Cross-cutting concepts / Approval and ownership** and the approval row
in **Risks and open questions** with the linked executable specification.

## Consequences and evidence still required

Implementers must update the explicitly superseded doctor/run expectations in the
same implementation, preserving syntax, metadata, argument and no-effect checks.
Specification-only commits do not claim failing-then-passing evidence. The step
commit must first make every new scenario fail at its stated behavior assertion.

Local CLI cases use temporary state and an HTTP Engine API fixture, never Docker.
Contract cases cover root/unavailable identities and publication faults without
privileged processes or production test switches. Shared contracts run against
fakes and adapters. Frozen-input independence and storage placement do not prove
worker inaccessibility: slice-10 owes that runtime evidence in CI.

## Implementation evidence

Red steps were committed as `6b899b5`, with the special-file snapshot correction in
`d3b6c0a`. With `@wip` removed locally, all 120 approval examples failed at their first
Then assertion, with no undefined or failing setup steps. The implemented approval,
doctor and run selection subsequently passed all 269 local examples (1944 steps).
The broader policy/manifest/doctor/approval/run selection also passed 682 examples
(4599 steps). Real engines and real operator state were never used.

The shared Store contract runs against the memory fake, the filesystem adapter and
cmd's real canonical-codec/store assembly. Adapter faults cover private-file creation,
short writes, file fsync, rename, parent fsync and pinned-directory replacement.
A restrictive-umask regression additionally checks descriptor-based initialization of
new private directories. Existing unsafe ownership or permissions are never repaired.
CI still owns fresh changed-code proof; session publication and actual worker access
restrictions remain the later slices' obligations.

PR #28's older-kernel regression was committed red first as `1b6a600`.
`ID-APPROVAL-32` runs the real binary in a disposable child whose syscall filter
returns `ENOSYS` for `fchmodat2`, using only temporary state and the synthetic
Engine API. Both umask `0022` and `0777` cases failed at the matching-approval
assertion with `approval.state.write` (exit 75), then passed after initialization
switched to chmod through the pinned descriptor's `/proc/self/fd` link.
The no-follow open and inode comparisons remain in place. Disk tests also verify
that a renamed descriptor cannot chmod a replacement symlink's target and that
replacement directories/symlinks are refused without changing their permissions.
This is syscall-compatibility evidence, not a native older-kernel runtime claim.
The disk package tests and the combined slice-3/slice-4/slice-7 scenario selection,
including the feature harness regressions, passed locally. The run-selection test
now supplies the host architecture and checks the Linux-amd64-only approval
fixtures separately from portable refusals and Linux PTY cases. Module tidying
only reclassified the already pinned `golang.org/x/sys` as a direct dependency.

PR #28's Unicode-control regression was committed red first as `1192906`.
With `@wip` removed, all six `ID-APPROVAL-33/34` examples failed at their
presentation assertions on raw C1 or bidirectional controls. They then passed
with display-only JSON escaping of non-printing Unicode in approval reviews,
prior-value differences and plain status. The fixtures independently compare
decoded displayed settings, stored canonical bytes and fingerprints. Renderer
tests also cover every C1 code point, notices, difference paths and supplementary
Unicode escapes while retaining printable Unicode and structured JSON values.
The affected slice-3/slice-4/slice-7 selection and platform-selection regressions
passed. Fixtures used disposable state and the synthetic Engine API; CI still
owns fresh changed-code proof.
