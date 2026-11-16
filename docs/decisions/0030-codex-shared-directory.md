# Slice-32: a shared Codex auth directory and lock

## Authority and status

Specified on 2026-11-16; **not implemented or runtime evidence**. The normative
scenarios and implementation handoff are in
[credentials-codex.feature](../../features/credentials-codex.feature), all `@wip`.
Slice-30 and T-48 are still being built. Their branch baselines are recorded there;
the implementer must consume their finished contracts, not replace their work.

The coordinator's 2026-11-16 00:50 UTC answer settles q-72 and
[decision 0025](0025-credentials-passed-in.md)'s question 1. The person's charter
amendment has landed. The person chose on 2026-11-06 to share the host's existing
login read-write, accepting exposure and corruption because copies diverge on
refresh. The dedicated host directory holds only `auth.json`. The host's Codex
reaches it through a symlink **the person sets up**, never this tool. The worker
receives a read-write directory mount, and the auth lock is shared too.

An older in-place-writing Codex, a worker login/API key and copying in/back are
rejected alternatives. Question 1 is closed; no additional person's answer is
owed before specifying this behavior. Question 5 still bars every real-credential
run, including a person's smoke check. Every scenario is synthetic and uses no
Codex account, provider, model or actual harness execution.

This decision supersedes decision 0025's unresolved Codex premise, question-1
row, and wait instructions for slice-32/T-48. Its token-copy design and other open
questions are unchanged. The charter's narrow exception permits shared writable
Codex source access; it does not establish the pilot's original AC-29 promise of
no cross-project source mutation. Projects deliberately sharing this login share
that exposure. There is no claim of AC-30 refresh support or AC-36 closure.

## Source contract and selected layout

Use decision 0025's source review of **Codex `rust-v0.167.0`**:
[storage.rs](https://github.com/openai/codex/blob/rust-v0.167.0/codex-rs/login/src/auth/storage.rs)
and [storage_file.rs](https://github.com/openai/codex/blob/rust-v0.167.0/codex-rs/login/src/auth/storage_file.rs).
The writer resolves the auth-file symlink, writes a temporary sibling and renames
it over the resolved target. Its `auth.lock` is opened and locked at `CODEX_HOME`,
without replacement. These are source-review inputs, not new runtime evidence.
[Official authentication documentation](https://learn.chatgpt.com/docs/auth)
describes file-based storage; it is not proof of this mount design.

Select a separate, existing, single-file read-write lock mount. The person supplies
its path and prepares a private empty file if needed. Missing lock means refusal,
never creation or an unshared fallback. This avoids adding any other entry to the
shared directory and matches this pin's fixed lock location. No host home or host
`CODEX_HOME` is mounted or discovered. An upgrade of the harness pin must recheck
these source assumptions.

| Host source | Fixed worker target | Access |
| --- | --- | --- |
| Dedicated directory containing only `auth.json` | `/run/itos-container/credentials/codex-auth` | Read-write directory |
| Separate `auth.lock` file | `/run/itos-container/credentials/codex-auth.lock` | Read-write single file |

An isolated worker `CODEX_HOME` can link its `auth.json` and `auth.lock` to these
targets. Harness-home integration belongs to slice-34. Slice-32's synthetic argv
creates those links under worker `/tmp`; it never makes the shared directory the
whole harness home. The host-side symlink is fixture-created in tests and remains
the person's responsibility in real setup.

## Version and approval

**Version 4 is required.** Version 3's closed two-token schema keeps its meaning.
The new version preserves those token forms and adds the closed entry
`{kind: codex-auth, source: <absolute directory>, lock: <absolute file>}`. Both
paths are mandatory and retain slice-29's literal path grammar. There is no generic
directory permission, caller-selected target or inferred host path.

Extend slice-30's canonical array with one Codex entry containing `kind`, `source`,
`target`, `access`, `directory` and `lock`. `directory` binds device/inode; `lock`
binds source/target, device/inode and read-write access. Identities use canonical
unsigned decimal strings from descriptors, not mountinfo's device. Token entries
remain unchanged. `auth.json`'s inode, bytes and hashes are excluded: a refresh must
replace that file without requiring consent. A safe directory or lock replacement
at the same path, or either path changing, requires new approval.

The new profiles are `offline-codex-shared-v1` and `allowlist-codex-shared-v1`.
Without Codex, retain existing profiles and shapes, with the declared policy version.
Changing any grant produces slice-30's single complete `/credentials` difference.
Older approval formats remain readable; historical grants cannot be filled from
mutable checkout requests. New profile validation is closed, including identities.

Review and status show directory/lock paths, identities, targets and writable
access. They explain source corruption, symlink following, the interval between
checks, person-owned setup and the continuing real-use bar. They retain provider
scope, workload-output and allowlist/refusal-log warnings. Token copies, when also
granted, are explained separately. Exact disclosure text is in the feature.
q-52 applies unchanged: terminal review and yes, no fingerprint grant route, and
no new prompt/write for unchanged approval. A host agent can fake a terminal;
approval still records what, not who.

## Checks and their limit

Shared readiness observes metadata only, with no-follow traversal, trusted ancestors,
slice-30's ext4/btrfs/xfs decision, bounded checkout inventory and protected-state
projections. The directory is host-UID-owned, exactly mode `0700`, with only
`auth.json`. That entry is a regular, single-link, host-UID-owned mode-`0600` file
of 1–65536 bytes. The separate lock has the same file rules but permits zero bytes.
Neither path nor any component can be a symlink. This does not prohibit the
person's separate host-side link, which the controller never traverses.

Exclude source and lock from checkout and protected-state aliases in both directions;
refuse submounts inside the auth directory. Reuse the existing metadata budgets and
location precedence. Extra entries, including abandoned temporary siblings, refuse;
the controller never cleans them. A concurrent legitimate refresh can therefore
cause a transient refusal. The feature specifies diagnostics and exit codes.

Observe before engine access, after inspection, after consent, and before create and
start. Directory/lock replacement during an observation or startup refuses. A valid
replacement of `auth.json` within the same directory is allowed. After worker removal,
check again before releasing its directory anchor, on foreground completion,
cancellation and recovery stop. Unsafe or unavailable evidence produces a nonzero
controller result and a retained metadata-only `credential_check`, without blocking
owned teardown or repairing the source. Preserve an observed nonzero workload exit.
Status shows the last completed observation, not a fresh assurance of safety.

The worker can plant a symlink, FIFO, socket, hard link or other entry between
checks. The host's Codex follows auth symlinks. A malicious target could therefore
cause host-side reads or replacement outside the dedicated directory, with the
host user's authority. A passing final check cannot prove that no transient poison
existed. The shared lock coordinates cooperative writers, not hostile code. This
accepted exposure and the lack of continuous protection are disclosed in
[security.md](../security.md#codex-shared-login-specified-limit).

## Delivery, recovery and package ownership

Extend the existing policy, readiness, approval and session engines; doctor, approve,
run, status and stop remain their command slices. Domains/ports gain no I/O imports.
Only `cmd` assembles adapters. Reuse the pinned standard library, YAML, x/sys,
mountinfo and Moby dependencies; this needs no new library or network service.

Reuse slice-17's descriptor-backed directory anchor design to bind the directory
that was approved, with separate durable intent and owned-resource identity. Verify
the exact writable directory and lock mounts alongside all existing hardening.
T-48's existing two-token staging seam must gain a distinct shared-source capability;
its unlink/remove operation must never see a Codex source. Record external sources
separately from owned anchors, workers and token staging. Policy version 4 is not
T-48's already-selected recovery record schema 4.

Stop removes only owned resources. Never delete, chmod, chown, repair or copy back
the auth directory, its entries, the lock or the host link. Source safety and resource
removal are separate recorded outcomes. If worker absence is not confirmed, retain
recovery, unknown source-check state and the exact retry. A lost start acknowledgement
does not prove no workload ran, and absent staging cannot fence this directory mount.
Reconcile the original full ID, never repeat start. A final unsafe source warning
survives repeated stop; automatic account repair is outside this item.

Run admits only delivery kinds and network modes that have landed. Schema parsing
does not enable unfinished token delivery or proxy egress. Existing early gates and
their neighbouring scenarios must be narrowed only where the implemented capability
replaces them, with that supersession explained in the implementation commit.

## Evidence and architecture amendment

The twenty allocated `ID-CODEXAUTH` scenarios cover policy, inspection, consent,
identity changes, pre-start checks, frozen status, disclosure, native two-way atomic
replacement, shared locking, absence/boundaries, poisoned sources and uncertain start.
All Codex-specific step phrases are new. The generic invocation and exit steps exist.
Fixtures seed independent canonical records and private sources, never approve as a
Given. New Whens capture early refusal; they must reach the behaviour assertion.
No step definitions, implementation, red/green results or engine runs accompany this
draft. The implementer owes committed red steps and observed assertion failures,
then passing evidence. Native mount/lock/poison/recovery rows run only in CI.

This record overrides arc42's statements that Codex remains blocked on question 1,
that credentials add only staged single-file binds, and that all original sources
remain unwritable. Its **Constraints**, **Building-block view**, **Runtime view**,
**Deployment view**, **Cross-cutting concepts**, **Decisions**, **Quality requirements**
and **Risks and open questions** now distinguish the specified directory exception
from token delivery and from evidence. No support claim is added.
