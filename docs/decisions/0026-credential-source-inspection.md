# Slice-29: credential inspection scope and protected-tree aliases

Recorded on 2026-11-09 with the revised
[credential scenarios](../../features/credentials-policy.feature). This is a drafter's
design amendment under the charter's architecture hypothesis, not permission to use
real credentials. T-47 and slice-25 have landed; no dependency change is needed.

Amended by **q-57**, the coordinator's settled decision of 2026-11-10 02:12 UTC:
match descriptor `statx(STATX_MNT_ID)` to mountinfo's first field, re-read it after
mountinfo and refuse a change. Drop descriptor `st_dev` equality with the row's
device. A missing `STATX_MNT_ID` refuses with its own actionable diagnostic.
This supersedes the original device-equality requirement and its refusal fixture.

## The sections this decision overrides

T-47's record is [decision 0025](0025-credentials-passed-in.md). The first version
of this amendment used its draft at `389f5585b64557f545ce05c88a914c9461c3f88f`.
The spec review correctly rejected this amendment's recursive shared-state scan:
unrelated record publication and cleanup could refuse a good source, and retained
history could exhaust its budget forever. This revision withdraws that scan and
settles T-47's state-exclusion specification prerequisite below. It supersedes:

- **Policy version 3 and read-only source observation**, step 3 and its unresolved
  state-side mechanism: use checkout enumeration and state mount projections, with
  the link-count check closing hard-link aliases. Root-only ancestry is insufficient.
- **Gates between the increments**, slice-29's newly permitted column: policy and
  manifest parsing alone cannot provide slice-29's promised source inspection.
- **Authority, sibling handoff and architecture changes**, slice-29's handoff:
  take the strengthened shared source check and the explicit metadata mode below.

In the same spec commit, [arc42](../architecture/arc42.md) changes **Constraints**,
**Building-block view** (doctor, policy, readiness and the increment table),
**Runtime view** (doctor scopes and version gates), **Cross-cutting concepts**
(source-location checks), **Decisions** and **Quality requirements**. These sections
now identify the third scope, the bounded state mechanism and its unproven source
contract. Decision 0025 is reconciled in this same commit; no conflicting scan
instruction is left for the implementer to choose between.

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

## Checkout enumeration and state mount projections

If `/checkout/private` is bound at `/outside`, `/outside/token` need not have the
checkout root in its walked ancestry. A bind of `/checkout/private/token` as one file
also supplies a second path while the file still has one hard link. Root-only ancestor
comparisons, lexical prefixes and a link-count check cannot establish non-exposure.
The inverse also matters: a source under `/vault` can be bound into the checkout or
protected state while its original pathname and ancestors remain outside both.

After the no-symlink source walk and safe-ancestor checks, compare the source and
each source ancestor's `(device, inode)` with both protected roots and every entry
in the **checkout only**. A match is `credential.location` (exit 1), located at that
grant's written `/credentials/<index>/source`. Traverse pinned checkout directories,
including Git, hidden, ignored and untracked entries, crossing directory/file mounts
and device changes. Follow no symlinks, read no file data and do not prune repeated
directory identities. Compare each child pin to its preceding metadata observation.
Unavailable metadata, disappearance, substitution or incomplete enumeration refuses
with `credential.unavailable` (exit 3); a known match retains location precedence.

The checkout alone keeps readiness's batches of 128, 100,000 total entries and depth
64 (root depth zero), inclusive. All credential metadata work shares one 10-second
monotonic deadline. Reaching it refuses; synchronous kernel calls are not preempted.
No background scanner survives refusal and no inventory is cached between commands.
These limits never count state records. This mode does not impose unrelated checkout
link-count or readiness checks.

### State exclusion without enumerating records

The selected `itos-container` state root and the fixed
`<account home>/.local/state/itos-container` control-state root both reserve filesystem
subtrees. The latter uses the effective UID's account record, as the approval engine
does; neither `HOME` nor `XDG_STATE_HOME` moves it. Apply the following observation to
both roots with one checkout inventory and one shared deadline. This does not read
the coordination catalog or enumerate other enrolled locations. Observe their
mount mappings, not their contents. The following is an algorithm to implement and
prove, not a claim of an atomic kernel snapshot:

1. Pin the source,
   checkout and existing state root using the existing no-follow component walks.
   Read each relevant descriptor's device/inode and mount ID using
   `statx(fd, "", AT_EMPTY_PATH | AT_SYMLINK_NOFOLLOW, STATX_MNT_ID)` before
   reading mountinfo, and its current absolute pathname from
   `/proc/self/fd/<fd>`. Rewalk that pathname without following symlinks and require
   the same device, inode and mount ID. Do not strip a ` (deleted)` suffix and use
   what remains: an unlinked, unreachable or ambiguous descriptor path refuses.
   Retain the source descriptor through observation and any later delivery read.
   Then acquire the first bounded mount table, retaining any error until the
   location check so source symlink/ancestor refusals keep their precedence.
   Re-read each held descriptor's mount ID after the table; refuse a change.
   Repeat the mount-ID recheck after the second table, including boundary pins.
2. The table is a complete `/proc/self/mountinfo` in a bounded buffer. For a descriptor
   with pathname `p`, select its row by **mount ID**, not longest pathname prefix.
   Require `p` to be the row's mountpoint or a component-descendant. Do not compare
   `st_dev` with mountinfo's device: subvolume/overlay getattr identities can differ
   from the superblock device. Continue comparing device/inode between held pins
   and no-follow pathname rewalks. Its filesystem coordinate is the row's decoded `root`
   followed by the relative components from `mountpoint` to `p`, keyed by device.
   A bind alias has the same coordinate even though its visible path differs.
   Apply this to source ancestors too. Mount sources such as `/dev/sda1`, filesystem
   type names alone and equal device numbers alone are not location evidence.
3. The state exclusion set contains the state root's coordinate subtree, plus the
   coordinate subtree rooted at **every mountpoint at or below the pinned state's
   visible path**, using each such row's device and root. A file mount contributes
   its exact coordinate (treating it as a subtree is conservative: no real path can
   descend through a file). Include nested mounts, other devices and repeated bind
   roots; compare components, never string prefixes. Keep covered backing subtrees
   reserved too: an overmount does not make the underlying state a credential store.
   Project the state path through every covering mount row as well, including the
   parent of a mount covering the state root itself or an ancestor. This reserves
   hidden backing coordinates even when the held state descriptor sees another device.
   Thus outward state-descendant/file binds match the base subtree; an outside
   directory/file bound into state matches an added subtree. Nothing enumerates,
   stats or reads ordinary state descendants. An unreadable unrelated record
   directory has no bearing on this calculation.
4. Take the second mount table after the checkout scan. Recheck source and boundary
   pins, pathname walks and mount
   IDs at the end. Build the coordinates/exclusion set independently from each
   table. Either observation locating a source/ancestor in a reserved subtree is
   a location refusal. Success requires both complete observations to exclude it
   and unchanged source/boundary mappings and identities. A changed relevant
   mapping, unavailable evidence or inconsistency is unavailable, with no retry
   loop or cached-safe fallback. Do not compare the whole tables byte for byte:
   unrelated mounts whose changes affect neither the source/boundary mappings nor
   either exclusion result do not refuse an otherwise valid source.
5. The observer never creates an initially absent managed root. Pin the
   nearest existing safe ancestor, reserve the missing path's projected subtree,
   and check the missing suffix again at the end. If another controller creates
   it, pin and validate it and compute its exclusion set in the final observation;
   accept only the same reserved coordinate, with no new source match. This normal
   first publication is not itself a failure. An existing root disappearing or
   changing identity, an unsafe newly created root or an unprojectable path refuses
   under the shared boundary diagnostics. No lock or state writer/index is added.

Read each mount table at most once per observation: at most 8 MiB, 16,384 rows and
65,535 bytes per line excluding the newline. Require EOF and a final complete line;
probe beyond inclusive bounds to distinguish a complete input from truncation.
Each decoded root/mountpoint and descriptor pathname is at most 4096 bytes; a joined
coordinate is at most 8192 by construction: the decoded root is at most 4096,
its separator is one byte, and the relative visible suffix is at most 4095. The same
bound holds for an absent root's suffix, whose complete visible path is bounded to
4096. There is no independent coordinate-overflow input after those path checks.
Cap each fdinfo at 64 KiB with EOF and require one valid mount ID. Reject malformed rows, duplicate mount IDs, invalid/non-absolute components,
missing descriptor rows and missing observations. Keep the bounded fdinfo
mount ID as a consistency check against statx, never as its fallback. A missing
statx mask, unavailable statx call or unusable mount ID produces
`credential.mount_id` (exit 3), naming `STATX_MNT_ID` and advising a host kernel
that exposes it. Other inconsistent mapping evidence remains `credential.unavailable`. Oversize or
unavailable evidence yields `credential.unavailable`, never a shortened table.
Unknown optional mount fields remain allowed. The limits are on topology, paths
and elapsed time, independent of the number of retained state entries. Work is
bounded by those limits and at most two grants, not another project's history.

The ordinary regular-file, owner, permissions, exactly-one-link, size and
duplicate-source checks still follow location. State aliases through a second hard
link need no namespace scan: the source's link count is not one, so they refuse as
`credential.links`. Recheck this metadata on the held descriptor before success.
A hard link found in the checkout scan keeps `credential.location` precedence;
state-only or outside-only additional links use `credential.links`. An alias that
passes all these checks has one filesystem name; bind mounts change its visible
route, not that name. Distinct sibling paths sharing a device remain admissible.

### Race argument and limits

Normal controllers publish new files by rename inside the reserved state subtrees
and remove old records. They do not change the state root, move an unrelated source
into state or mount a new filesystem over its ancestors. Such writes change neither
the subtree exclusion nor the outside source's identity/path/link count. No record
is enumerated, so a temporary entry disappearing cannot cause a false unavailable
result, and 100,001 or more retained entries cannot exhaust admission. Even a source
alias to a state file being renamed stays within the reserved subtree; if unlinked,
it fails the path/link checks. Bind exposure into a state descendant is in the
mount table even with one link and no shared root in the source's visible ancestry.
Cross-directory hard links fail the independent link-count check.

The two observations detect observed changes to the objects or mappings on which
success depends. They do **not** prove absence of a trusted host's transient
mount/rename/link operation between observations, or prevent changes after return.
The existing host-user/admin trust assumption remains; no global lock, privileged
namespace operation, mount helper, engine connection or continuous enforcement is
introduced. The controller uses its host mount namespace, as later native Docker
delivery requires. An inaccessible or incomplete view refuses, never falls back to
root-only comparison. Approve/run must repeat this entire check at their specified
validation points and run must read the retained checked descriptor. Ordinary
credential admission and delivery remain gated until their own evidence exists.

### Kernel interface basis and library choice

The kernel documents mount IDs, filesystem roots and mountpoints in
[/proc mountinfo and fdinfo](https://www.kernel.org/doc/html/latest/filesystems/proc.html).
The coordinate and concurrency argument above is our design inferred from those
interfaces; those documents do not promise an atomic snapshot.

Select `github.com/moby/sys/mountinfo` **v0.7.2** for the implementing commit. Its
[reader API and escape handling](https://github.com/moby/sys/blob/mountinfo/v0.7.2/mountinfo/mountinfo_linux.go)
already parse the kernel format, avoiding a new mount-table parser. Pass the bounded
buffer to `GetMountsFromReader` without a filter that stops early, then validate
the required numeric/identity fields (the parser ignores some numeric errors).
Retain its [Apache-2.0 license](https://github.com/moby/sys/blob/mountinfo/v0.7.2/LICENSE)
and notices. This adds no service; the spec does not
edit go.mod. Use the existing pinned x/sys/unix and Go primitives for descriptors.
Linux-only calls live in call-only shims; OS remains a value in the decision logic.

## Shared implementation and evidence

The owning packages remain `internal/slices/doctor/{command,domain,port,inspection}`,
`internal/engine/policy` and `internal/engine/readiness`, with only `cmd` assembling
adapters. Policy validates the pure version-3 shape. Readiness owns source metadata,
checkout inventory, state projections and the shared validation contract; doctor renders
its results. Domain decisions receive raw metadata through ports. The disk adapter
owns procfs and filesystem I/O; no domain or port imports the mountinfo library.
Policy parsing continues to use the pinned YAML v4 parser.

The source-observation contract runs unchanged against its I/O-free fake and disk
adapter. Alias rows apply to **both** protected trees; other rows name their scope:

| Observation fixture | Required result |
| --- | --- |
| Protected root bound outside | Location refusal by shared root identity. |
| Protected descendant directory bound outside | Location refusal although source ancestry contains neither protected root. |
| Single protected file bound outside | Location refusal with one link and no matching protected root ancestor. |
| Outside directory or individual file bound into a protected descendant | Location refusal from checkout inventory or the state mount projection. |
| Distinct inode, same device and shared pathname prefix | Accepted when all other source checks succeed. |
| Subvolume-like st_dev differs from mountinfo at source and boundary pins | Accepted by stable statx mount IDs; descendant aliases still refuse. |
| Descriptor mount ID changes during the first mountinfo read | Unavailable even when both rows project to the same coordinate. |
| STATX_MNT_ID is unavailable | Dedicated mount-ID diagnostic, no fdinfo or device fallback. |
| Unreadable/disappearing checkout entry, unavailable identity or directory substitution | Unavailable refusal; never an incomplete-safe result. |
| Exact checkout entry/depth and state topology limits; limits exceeded or deadline reached | Accepted at inclusive limits when complete; unavailable on exhaustion. |
| Known alias plus unavailable checkout metadata | Location refusal for the alias; unavailable for an otherwise unmatched source. |
| Concurrent approval/intent/snapshot publication and session cleanup in another project | Distinct source accepted; state-descendant alias still refused; no state enumeration. |
| 100,001 retained state entries, including an unreadable unrelated directory | Distinct source accepted with zero state record visits; state-descendant alias still refused. |
| State-only hard link, including one created before the final source stat | Links refusal without scanning state. |
| Missing/malformed/truncated mount data, changed source mapping or boundary identity | Unavailable or existing boundary refusal; never root-only fallback. |
| Unrelated mount changes; absent state root created safely at its reserved coordinate | Accepted if both complete observations exclude the unchanged source. |

Alias fixtures have safe owners/permissions, one-link nonempty regular files and no
symlink, so an unrelated refusal cannot satisfy them. Include file and directory mount
crossings and a descendant on a different device. Adapter-local observation seams supply
privileged metadata; no test mounts host paths or introduces a production environment
override. These are observation-contract tests, not native bind-mount evidence.

ID-CREDENTIALS-29 exercises the real validator and doctor presentation through a
test-only command assembly with raw in-memory topology, as the manifest contract
scenarios do. Its positive control accepts distinct identities before its alias refusal.
The fixtures cannot supply a precomputed credential decision. ID-CREDENTIALS-30 keeps
the product CLI's unreadable-checkout refusal; 35 accepts unreadable unrelated state.
31 observes source, checkout and mount-table calls separately, requiring none with no
grants. New 32/33 pair concurrent writes and retained history with state-alias refusals;
34 exercises complete topology and race evidence. Their fixture schedules and lazy
history live in the feature comments and shared fake/disk observation contract. No
test needs a host mount or engine. The data-access observer and synthetic-marker
cases still require inspection to read no source contents. Privileged native mount
evidence remains CI-only downstream work, not a claim of these simulated contracts.

The credential scenarios are implemented with synthetic fake/disk and CLI evidence.
The initial specification supplied no executed-red result; implementation and review
regressions supply assertion-failing then passing runs. Native bind-mount proof remains
downstream CI work. The neighboring policy,
parsing, manifest, doctor, approval, run and control-state scenarios remain relevant;
slice-25 supplies version 2 and the manifest-policy/recorder steps this spec consumes.
