# Slice-29: credential inspection scope and protected-tree aliases

Recorded on 2026-11-09 with the revised
[credential scenarios](../../features/credentials-policy.feature). This is a drafter's
design amendment under the charter's architecture hypothesis, not implemented behavior,
runtime evidence or permission to use real credentials. Slice-29 still waits for T-47
and slice-25. The coordinator must retain T-47 and add slice-25 to the registry's
`depends_on` before scheduling implementation; this spec does not change that registry.

## The sections this decision overrides

T-47's draft is `docs/decisions/0025-credentials-passed-in.md` at
`389f5585b64557f545ce05c88a914c9461c3f88f` (the record was introduced by `01cfa825`).
That prerequisite is still on its own branch. This amendment supersedes:

- **Policy version 3 and read-only source observation**, step 3: comparing source
  ancestors only with the protected root identities does not detect all aliases.
- **Gates between the increments**, slice-29's newly permitted column: policy and
  manifest parsing alone cannot provide slice-29's promised source inspection.
- **Authority, sibling handoff and architecture changes**, slice-29's handoff:
  take the strengthened shared source check and the explicit metadata mode below.

In the same spec commit, [arc42](../architecture/arc42.md) changes **Constraints**,
**Building-block view** (doctor, policy, readiness and the increment table),
**Runtime view** (doctor scopes and version gates), **Cross-cutting concepts**
(source-location checks), **Decisions** and **Quality requirements**. These sections
previously described only file-only and full-readiness inspection. They now identify
the proposed third scope and its unproven source contract. Preserve these amendments
when integrating T-47's overlapping arc42 edits; do not postpone them to implementation.

The rest of T-47's proposal remains with that item, including its six questions for
the person. This decision selects no Codex kind, delivery mechanism, approval grant,
refresh policy, live-account permission or refusal-log change. It amends neither the
charter nor the pilot contract.

## A separate metadata scope

Slice-29 introduces `doctor --credentials-only`: parse the policy, resolve the effective
host UID and pin checkout/state boundaries, then inspect requested source metadata.
It does not read source contents or state-record contents, require a manifest, contact
an engine, execute helpers, create state or record approval. Its schema-1 JSON has
`scope: "credentials"`, the sorted grants and located problems; plain output explicitly
names what was not checked. A valid policy without grants yields an empty array without
a source/exposure scan. Policy problems retain the existing refusal envelope without
partial effective data.

The three explicit doctor scopes are mutually exclusive. `--policy-only` and
`--manifest-only` continue to inspect only their existing inputs. The staged gate is:

| Increment | Newly permitted | Still refused |
| --- | --- | --- |
| slice-29 | Policy-only and manifest-only parse version 3; credentials-only observes sources after policy validation. | Ordinary doctor, approve and run reject valid version 3 with `policy.version`, exit 2, before manifest, sources, engine or publication. Invalid version 3 reports its policy problems first. |
| slice-30 | Ordinary doctor and approve consume the same shared source observations and bind grants. | Run keeps T-47's delivery/egress gates until their owning increments supply proof. |

This preserves the separation between a request, a metadata observation and approval.
Silently dropping credentials to obtain an offline approval is forbidden. Version 2's
allowlist semantics and any sibling gate remain intact. The new mode resolves an
increment-scope omission; it does not bring slice-30's approval work forward.

## Compare all exposed identities, not just roots

If `/checkout/private` is bound at `/outside`, `/outside/token` need not have the
checkout root in its walked ancestry. A bind of `/checkout/private/token` as one file
also supplies a second path while the file still has one hard link. Root-only ancestor
comparisons, lexical prefixes and a link-count check cannot establish non-exposure.
The inverse also matters: a source under `/vault` can be bound into the checkout or
protected state while its original pathname and ancestors remain outside both.

After the existing no-symlink source walk and safe-ancestor checks, compare the pinned
source and each source ancestor's `(device, inode)` with the identities of both protected
roots and **every entry below them**. Any match is `credential.location` (exit 1),
located at that grant's written `/credentials/<index>/source`. The protected roots are
the selected checkout and the managed `itos-container` state tree. An absent state
root stays absent; an inaccessible existing tree is not an absent one.

The disk observation enumerates pinned directories without reading file contents or
following symlinks. Include Git, hidden, ignored and untracked entries. Traverse every
directory mount point and observe mounted file identities, even across device changes;
do not prune repeated directory identities. Pin each opened child and compare its
identity with the entry just observed, as readiness's existing traversal does. No
privileged mount operation, mount discovery helper or engine connection is involved.

A source with no known identity match requires a **complete** inventory. Failed reads
of directory entries, unavailable metadata, disappearing entries, observed substitution
or budget exhaustion produce `credential.unavailable` (exit 3), never success from the
part visited. A known match retains `credential.location` if a different subtree cannot
be enumerated. Shared state-location/identity failures keep their existing diagnostics.
The inventory may serve the grants in one invocation but is never cached across commands.

Reuse readiness's bounded directory traversal primitives: batches of at most 128 entries,
a total of 100,000 entries across both trees, directory depth at most 64 from each root
at depth zero, and one 10-second monotonic scan deadline. Exactly 100,000 entries and
depth 64 are admissible if enumeration completes before the deadline; needing another
entry, deeper directory or reaching the deadline refuses. Time checks between synchronous
filesystem calls cannot preempt a blocked kernel call. No abandoned background scanner
continues after refusal. A cyclic mount eventually exhausts these bounds and refuses.

Only after location is established do the existing regular-file, owner, permissions,
link-count, size and duplicate-source checks apply. Thus a source hard-linked into the
checkout reports location before links; an unrelated outside hard link still reports
`credential.links`. This intentionally corrects that row in ID-CREDENTIALS-23. Distinct
inodes sharing a device or a protected pathname prefix remain admissible.

This is a metadata observation, not an atomic filesystem snapshot or continuous defense
against trusted host changes. Later approve/run consumers reuse and refresh the same
check at their specified validation points, retaining T-47's checked-descriptor rule.
The protected-state walk observes no record contents or other project's admission
history. Unrelated checkout link-count/readiness checks are not added to this mode.

## Shared implementation and evidence

The owning packages remain `internal/slices/doctor/{command,domain,port,inspection}`,
`internal/engine/policy` and `internal/engine/readiness`, with only `cmd` assembling
adapters. Policy validates the pure version-3 shape. Readiness owns source metadata,
complete protected-tree observations and the shared validation contract; doctor renders
its results. Domain decisions receive metadata through ports. Reuse Go's filesystem
primitives and the already pinned `golang.org/x/sys/unix`, plus the existing YAML v4
parser. No new library, custom filesystem library or service is selected.

The source-observation contract runs unchanged against its I/O-free fake and disk
adapter. It must cover each of these for **both** protected trees:

| Observation fixture | Required result |
| --- | --- |
| Protected root bound outside | Location refusal by shared root identity. |
| Protected descendant directory bound outside | Location refusal although source ancestry contains neither protected root. |
| Single protected file bound outside | Location refusal with one link and no matching protected root ancestor. |
| Outside directory or individual file bound into a protected descendant | Location refusal from the protected-tree scan. |
| Distinct inode, same device and shared pathname prefix | Accepted when all other source checks succeed. |
| Unreadable/disappearing entry, unavailable identity or directory substitution | Unavailable refusal; never an incomplete-safe result. |
| Exact entry/depth limits completed before deadline; limits exceeded or deadline reached | Accepted at the count/depth boundaries; unavailable on exhaustion. |
| Known alias plus another unavailable subtree | Location refusal for the known alias; unavailable for an otherwise unmatched source. |

Alias fixtures have safe owners/permissions, one-link nonempty regular files and no
symlink, so an unrelated refusal cannot satisfy them. Include file and directory mount
crossings and a descendant on a different device. Adapter-local observation seams supply
privileged metadata; no test mounts host paths or introduces a production environment
override. These are observation-contract tests, not native bind-mount evidence.

ID-CREDENTIALS-29 exercises the real validator and doctor presentation through a
test-only command assembly with raw in-memory topology, as the manifest contract
scenarios do. Its positive control accepts distinct identities before its alias refusal.
The fixtures cannot supply a precomputed credential decision. ID-CREDENTIALS-30 uses the
product CLI and an unreadable descendant in a temporary protected tree to require
refusal of incomplete native enumeration. The existing data-access observer and
synthetic-marker cases still require inspection to read no source contents.

All credential scenarios remain `@wip`. Their implementing agent supplies missing step
definitions in a red-first test commit, then obtains assertion-failing and passing
evidence. This spec amendment supplies no executed-red result. The neighboring policy,
parsing, manifest, doctor, approval, run and control-state scenarios remain relevant;
slice-25 supplies version 2 and the manifest-policy/recorder steps this spec consumes.
