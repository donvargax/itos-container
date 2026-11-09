# T-47: credentials passed in per command, approval and the remaining decisions

Recorded on 2026-11-09 from [T-47's ledger](../../tasks/phase-1.yaml), for the
[charter](../charter.md)'s second milestone, step 2. This is a **proposed** design
under the architecture hypothesis, for the person to confirm or overturn. It adds
no code, dependency, scenario, step definition, workflow or engine run. Source
readings are not runtime evidence or a container-host support claim.

The workload remains the worker's main process, invoked as literal argv. Credentials
arrive per command, outside the checkout and without their values entering the create
configuration or `docker inspect`. The two token kinds below have a proposed delivery;
Codex does not. Real credentials remain barred pending question 5, including a person's
smoke check. Synthetic scenarios need neither an account nor a real model.

## Codex: the charter's single-file premise no longer holds

The charter says Codex rewrites `auth.json` in place. It did so with a truncating
`OpenOptions` write and mode `0600` before OpenAI Codex commit
[`582a8812e023b08cbf10b4603ac41644b8d99261`](https://github.com/openai/codex/commit/582a8812e023b08cbf10b4603ac41644b8d99261)
of 2026-10-30 (#58251). Release
[0.167.0](https://github.com/openai/codex/releases/tag/rust-v0.167.0), published
2026-11-06, includes that change; 2026-10-30 is the commit date, not the release date.
In [`FileAuthStorage::save` at that tag](https://github.com/openai/codex/blob/rust-v0.167.0/codex-rs/login/src/auth/storage.rs),
Codex writes a temporary sibling and renames it over `auth.json`.
[`storage_file.rs`](https://github.com/openai/codex/blob/rust-v0.167.0/codex-rs/login/src/auth/storage_file.rs)
follows symlinks and refuses hard-linked or read-only targets. The ledger's source
comparison used the commit's parent, `c15d0ce`. Its write lock, `auth.lock` in
`CODEX_HOME`, is opened without truncation and locked, never renamed.

The consequence is inferred from these sources and Linux `vfs_rename` in
`fs/namei.c`, not tested here: replacing a single-file bind mount targets a mount
point and fails with `EBUSY`. A worker cannot persist a refreshed login that way.
When the host's Codex replaces its file, the worker's mount retains the old inode;
the two sides stop seeing each other's login updates. Only `auth.lock` retains the
single-file-mount shape. OpenAI's [authentication documentation](https://learn.chatgpt.com/docs/auth)
describes the file store and refresh, but does not make a bind-mounted file compatible
with atomic replacement.

Only the person can change the charter. Question 1 asks how to share the login now.
This record defines **no Codex kind and no Codex delivery**. Slice-29 must not add one;
slice-32 and the bind-mounted-file half of T-48 wait on the answer. The older registry
wording and the charter's in-place statement are not permission to implement them.

## The two token kinds and the workload's interface

| Kind | Source | Fixed path in the worker |
| --- | --- | --- |
| `claude-token` | The long-lived token file the person keeps on the host | `/run/itos-container/credentials/claude-token` |
| `github-token` | The GitHub token file the person keeps on the host | `/run/itos-container/credentials/github-token` |

Each target is a read-only regular file, owned by the host UID, mode `0400`. The
runtime creates its mount point before making the root filesystem read-only, as it
does for `/tmp`. The approved image is trusted for these paths under the contract's
"Goal and limits". No command writes a token into the container's `Env` or argv.

[Claude Code's environment reference](https://code.claude.com/docs/en/env-vars)
documents `CLAUDE_CODE_OAUTH_TOKEN`, generated with `claude setup-token`, and excludes
it from project settings. [GitHub CLI's environment reference](https://cli.github.com/manual/gh_help_environment)
documents `GH_TOKEN`. Neither reference documents a token-file form. The person's
explicit workload argv can read the file and export the variable inside the worker,
for example:

```sh
sh -c 'CLAUDE_CODE_OAUTH_TOKEN="$(cat /run/itos-container/credentials/claude-token)" exec claude -p "$1"' sh '<task>'
```

The example is an interface illustration, not an authorized live-account trial.
The controller copies source bytes without trimming; shell command substitution in
the chosen argv has its ordinary newline semantics. The manifest already reserves
`CLAUDE_`, `GH_` and `GITHUB_`, so checkout configuration cannot shadow these names.
Automatic exporting belongs to step 3 (`p1-harness-home`) or step 5's init if their
specs choose it. This record fixes only the files. GitHub HTTPS/API use is distinct
from the deferred sandbox SSH identity; network Git is not implemented here.

## Policy version 3 and read-only source observation

Following [CONFIG rules 6, 7 and 12](../CONFIG.md), version 3 is slice-25's version 2
(decision 0023 on its branch) plus an optional top-level `credentials` list. Versions
1 and 2 retain their meaning and refuse this unknown key. An absent list means no
grants; an empty list is refused, as an empty allowlist is. Each list entry is a closed
object with exactly `kind` and `source`:

- `kind` is exactly `claude-token` or `github-token`, case-sensitive and at most once.
- `source` is a literal absolute host path, at most 4096 bytes, slash-separated, with
  no empty, dot or dot-dot component, trailing slash, backslash or ASCII control.
  There is no tilde, variable, interpolation or other expansion.

Entries retain their written order for diagnostics; everywhere else they are a set
keyed by kind. Parsing a valid file reads no host source. Collect located problems
at `/credentials/<index>/kind` or `/source` as in version 1, never echoing a rejected
value. Slice-29 owns rule IDs, the version 3 schema beside version 2's, and the changes
to `docs/CONFIG.md` and `docs/CLI.md`. A future Codex answer defines its own kind and
whether it needs version 4; version 3 currently refuses it as unknown.

Doctor, approve and run share one source observation in readiness, beside slice-20's
checkout scan. It reads metadata, never contents. For each grant, in this order:

1. Open with `O_PATH` and `O_NOFOLLOW` at every component. Use `openat2` with
   `RESOLVE_NO_SYMLINKS` or a component walk; a symbolic link anywhere refuses.
2. Require every ancestor directory to be root- or host-UID-owned, with no group or
   other write permission unless root-owned and sticky. This is `directory()`'s rule
   in `internal/engine/approval/disk/store.go` for state-root ancestors.
3. Compare device and inode: neither the source nor any ancestor may be the checkout
   root or protected state root. An alias through another host mount also refuses.
4. Require a regular file owned by the host UID, no group or other permission bits,
   exactly one link and a size from 1 through 65536 bytes. A second name could expose
   or replace the source, including from the checkout.
5. Refuse two grants naming the same device and inode.

A missing source or failed check is a located problem. Doctor still shows effective
grants; approve records nothing; run refuses before staging or creating anything.
Only run reads, after every check succeeds, by reopening the checked descriptor through
`/proc/self/fd/<n>`. Thus it reads the checked inode, not a newly resolved pathname.
A short read or size different from `fstat` refuses. Commands never print, hash, log
or compare contents. Changing a token's contents does not require new approval;
rotation remains the person's responsibility, subject to question 2.

## Delivery: staged read-only binds under /dev/shm

This is the record's own proposal, not a settled charter amendment. Each run reads
the current sources once and keeps nothing between runs, in an image, a volume or
the checkout. It assumes rootful dockerd in the host mount namespace, as slice-17's
descriptor-backed anchor does. The controller stages copies, never binds the original
token sources directly.

1. Publish durable intent naming the staging directory before creating it. Its name
   under `/dev/shm` is `itos-container-` followed by 32 random lowercase hexadecimal
   digits. Confirm with `statfs` that `/dev/shm` is tmpfs or delivery is unavailable.
   This avoids disk-backed control state and dependence on an unset `XDG_RUNTIME_DIR`;
   host swap can still persist memory. Create mode `0700`, owned by the host UID, and
   validate by descriptor after `mkdir` as q-35's coordination directory does, except
   that an existing name refuses rather than being reused.
2. Create one new staged file per grant with `O_CREAT`, `O_EXCL`, `O_NOFOLLOW` and mode
   `0400`. Write exactly the bytes read from the checked source, without trimming.
   Workers have private IPC (`IpcMode` private) and their own `/dev/shm`; none receives
   the host directory. Each sees only its own files through individual binds.
3. Create the worker with one `HostConfig.Mounts` entry per grant: type bind, read-only,
   staged source and the kind's exact target. Docker checks source existence at create
   and binds at start, so the files must remain until then. `docker inspect` and create
   configuration expose paths, never values. Extend `VerifyRun`'s `safeHost` and
   `safeMounts` in `internal/engine/session/docker/run.go` to require exactly the
   checkout volume, `/tmp` tmpfs and these binds, including exact sources, targets and
   read-only flags, before start.
4. After the controller's start wait returns and any applicable existing mount detector
   has run, whatever either answered, attempt to unlink every staged file and remove
   the directory. Record absence only after confirmation. The acquired mount keeps
   the copy available to the worker until its mount is torn down. No restart is
   authorized; the next command needs fresh sources and staging.

Checks, reads, staging, create and verification failures **before any start attempt**
prevent launch and enter owned cleanup as needed. That qualification is essential.
Unlink removes a pathname, not a mount or open reference the runtime already acquired.
A delayed start can complete with those bytes after unlink. Neither a start error or
timeout, a lost start acknowledgement, restart policy `no`, nor missing staged paths
proves that no workload ran.

An **uncertain start** retains the original protected identity and follows
[lifecycle scenarios](../../features/lifecycle.feature) and
[decision 0017](0017-controller-operation-budgets.md): do not retry start; reconcile
the original owned full ID within the existing budgets; attempt owned termination and
removal. Keep an unobserved workload outcome unknown. Staging-path absence is neither
worker absence nor completed credential cleanup. Completion requires confirmed owned
worker removal and staged-path removal, alongside the existing session resources.
Unavailable observations or incomplete cleanup retain recovery and the exact stop retry.

If the controller dies before unlink, stop uses the intent's directory name, verifies
owner and mode and that only expected names are present, then removes it. Otherwise
it refuses and retains recovery with the exact retry. Reboot clearing `/dev/shm` says
nothing about the prior workload outcome. Status shows the frozen grants and, during
recovery, staging-directory presence as present, absent or unknown.

T-48 and slice-31 must prove this on the real engine in CI. A rejected staged bind or
a copy that outlives its worker reopens the mechanism with that evidence; it does not
select a fallback.

## Delivery alternatives rejected in the ledger's source review

These are source-based reasons, not experiments run for T-47:

| Alternative | Why it does not preserve the current run contract |
| --- | --- |
| A pre-start archive copy, `PUT /containers/{id}/archive` | Moby's `checkWritablePath` in `daemon/archive_unix.go` permits a writable `MountPoints` entry, which `HostConfig.Tmpfs`'s `/tmp` is not; otherwise the read-only root refuses. A tmpfs specified through Mounts exists only while running. A tmpfs local volume is emptied on its last unmount (`Unmount` in `daemon/volume/local/local.go`), between copy and start. Only a disk-backed volume retains the bytes, persisting them in Docker's data root, requiring a writable worker mount and another owned/recoverable resource. |
| A stdin handoff | Literal argv would need our own init to consume the frame first, requiring a mounted executable. Decision 0020 excludes that extra gate in milestone one; charter step 5 introduces an init later for Landlock. Step 4's TTY would also echo the frame before a process can disable echo. |
| An exec-time environment | An exec's Env is absent from container inspect and from exec inspect (`ProcessConfig` in pinned Moby API v1.56.1 has no Env). But exec needs a different running main process. Run's wait, signals, exit status and controller-loss isolation are proven around the workload as PID 1; `ContainerKill` targets it and there is no exec-signal endpoint. The later exec command inherits the session mounts and needs no separate delivery. |
| A direct read-only source bind | Violates the contract's "Do not mount the sources directly", exposes the person's source path in inspect, shows later edits and binds the pathname at start rather than the checked inode. |
| A descriptor source, `/proc/<pid>/fd/<n>` | Slice-17's local-volume driver binds in the host namespace onto a directory, which a file cannot replace (`ENOTDIR`). The runtime makes an ordinary bind in the worker's new mount namespace; Linux `may_copy_tree` in `fs/namespace.c` refuses a source resolving into another namespace's mount. |

No custom credential library or network service is introduced. These findings constrain
the delivery choice rather than authorizing a host shell, HTTP bridge or second workload
lifecycle.

## What approval binds and shows

Slice-30 extends slice-26's canonical settings. Grants select one of two new profiles:
`offline-credentials-v1` or `allowlist-credentials-v1`. Each keeps its base profile's
boundary, changing only two clauses from approval's presentation:

- The checkout is the only host **directory**, at `/workspace`, plus the read-only
  credential files under `/run/itos-container/credentials`.
- The prohibition on ports, grants and hooks becomes a prohibition on ports and hooks.

The new canonical member `credentials` is an array of `{kind, source, target}`, strictly
ascending by kind. Targets are the fixed paths above, so changing a target in a release
requires new consent, as changing `proxy.image` does in slice-26. `valid()` accepts the
new profiles only with a nonempty, strictly ascending set of known kinds and lexically
valid sources. The difference walk reports any grant change as one `/credentials`
difference, with the entire old and new arrays. Adding, removing or repointing a grant
changes approval; reordering entries does not. Contents never enter canonical settings.

Without grants, `offline-v1` or `allowlist-v1` retains its bytes and identity. As in
slice-26, `policy.version` remains the declared version, so moving to version 3 without
grants changes only `/policy/version`. Existing approvals are not silently repurposed
as credential approvals.

Approve review and status show each kind, source and in-worker path and explain that:

- The worker can read the files and do anything the provider permits; approval narrows
  no token scope.
- Each run copies the source's current contents into memory-backed staging, keeps the
  copy with that worker, drops it at teardown and never writes the original source.
- Workload output and traffic are not redacted. An allowlist credential run can send
  the bytes to any listed host. The refusal-log leak below also remains disclosed.

Slice-30 fixes the exact review words, as slice-26 does for egress. Status reads grants
from the frozen snapshot, not current checkout requests.

### q-52: approval requires a terminal when there is a grant

The person's [q-52 answer](../../tasks/asks.yaml) revisits A1: network or credential
grants require the person at a real terminal, with review and yes; no new keys or prompt
on unchanged settings. For a ready candidate with any credential grant, `--fingerprint`
always refuses with `approve.terminal-required`, exit 1: malformed, matching, mismatching
and already-approved cases alike. Its fix names bare `itos container approve` at a
terminal. This check follows inspection, so an unready candidate retains its problem.

Bare approve without a terminal, or with `--json`, retains
`approve.confirmation-required`; for these candidates its fix names only the terminal
form, never `--fingerprint`. The same rule covers an egress allowlist. Whichever of
slice-26, slice-27 and slice-30 lands first implements it; the others widen its condition.
No non-interactive credential or network grant can be created to become usable later.
A terminal credential approval of the same canonical grants therefore carries forward
when delivery lands; lifting the run gate alone does not request consent again.

An unsandboxed host agent can still fake a terminal. Approval records **what, not who**;
terminal detection does not authenticate a human. Slice-30 puts this and the rule into
approve help and [docs/security.md](../security.md), answering its old instruction to
revisit the limitation. T-47 edits neither that document nor slice-26's older spec.

## Gates between the increments

| Increment | Newly permitted | Still refused |
| --- | --- | --- |
| slice-29 | `doctor --policy-only` and `--manifest-only` parse valid version 3 | Ordinary doctor, approve and run refuse with `policy.version`. No Codex kind. |
| slice-30 | Ordinary doctor and approve inspect version 3 and its grants | Run refuses any kind whose delivery is absent and any allowlist until slice-27, before the manifest, engine and host state. |
| T-48 and slice-31 | Shared staging seam, then `claude-token` delivery with synthetic proof | `github-token` until slice-33; Codex pending question 1; real credentials pending question 5. |
| slice-33 | `github-token` through the same staging seam | No SSH identity, live-account trial or unproven credential-absence claim. |

## Provisioning redaction and the refusal-log conflict

Controller provisioning moves bytes only from the checked descriptor to the staged file.
They never enter policy, approval, snapshot, intent or recovery records; labels; create
configuration; controller-set Env or argv; Engine API request bodies; status, doctor,
approve or run diagnostics; error text; logs; or files under the checkout or state root.
Paths are not secrets and may appear with grants. Source problems name rule, location
and path, never bytes or a wrapped Go error. The controller uses one read/write buffer
and overwrites it afterward; Go may retain other copies, so this is hygiene rather than
a zeroization guarantee.

As arc42's "Diagnostics and streaming" states, arbitrary workload output and traffic
remain unredacted. A hostile workload can copy a credential wherever it can write or
send. This includes a specific persisted-log conflict that a blanket absence claim
would conceal. [Decision 0022](0022-egress-proxy.md) excludes userinfo, paths, queries
and headers, but logs a syntactically permitted target hostname verbatim. A CONNECT to
`<synthetic-marker>.example.com:443`, unlisted but composed of allowed letters, digits
and hyphens, puts the marker in proxy stdout, dockerd's json-file log and the teardown
copy at `sessions/<lookup>/<id>/egress-refusals.jsonl` under protected state.

That is persisted logging, not merely the workload's foreground stream. Omitting the
other request fields does not close it. Question 6 asks the person how the target is to
be represented or whether the guarantee must be explicitly revised. This record changes
neither decision 0022 nor AC-36, chooses no sanitization scheme and promises no credential
absence from refusal logs pending the answer and downstream proof.

## Required evidence, all synthetic

Each credential scenario creates a private temporary home and a source containing a
fresh synthetic marker: `itos-synthetic-credential-` followed by 32 random lowercase
hexadecimal digits. These bytes survive JSON and shell quoting unchanged. A cooperative
workload proves the read by printing its SHA-256, never the marker. No real token,
account, model, provider call or credential scanner is involved.

Absence searches cover the raw marker and its base64 at all three offsets in:

- The worker's `docker inspect` and every request and response in the harness's Engine
  API recorder.
- The whole host state tree, including approval, snapshot, intent and recovery records;
  status in text and JSON; doctor, approve and run error output.
- Proxy stdout, both persisted refusal-log copies when present, and every checkout file.

Each search must find the marker in a positive-control file seeded with it. Also require
no staging directory under `/dev/shm` after acknowledged startup and staging cleanup,
or after confirmed teardown. The absence of that pathname alone proves nothing about a
worker's outcome or an acquired mount.

Require a separate adversarial **hostname-marker probe**: send the marker as a label of
an unlisted, syntactically valid CONNECT hostname. Search the refusal in proxy output,
Docker logs and the protected-state copy. Under decision 0022 as written, this must fail
credential absence. Do not omit the hostname or either persisted copy, skip the probe,
or count the expected leak as absence evidence. T-39 and slice-28 own the logging
contract; slice-31 and slice-33 need the integrated probe after question 6 is answered.
Their drafters reconcile that answer before claiming AC-36 closure.

Session domain tests use the I/O-free port fake to cover staging before create, unlink
after the start attempt, provisioning refusal before any start attempt, and no marker
in recorded configuration or recovery values. T-48 runs the shared contract against
fake and adapter, including a lost start acknowledgement after the runtime acquired the
credential mount and delayed start completion after unlink. The fake models acquisition
without I/O. Slice-31 and slice-33 require the corresponding real-engine case in CI:
the acquired mount still supplies the synthetic bytes, proven by their hash, despite
the missing acknowledgement. Assert no repeated start, reconciliation of the original
owned full ID, observed outcome or explicit uncertainty, and bounded owned cleanup.
Failed cleanup retains recovery and the exact stop retry; success confirms worker
absence and staged-path removal. Neither a failed acknowledgement nor absent staging
may satisfy an assertion that no workload ran or cleanup completed.

These extend LIFECYCLE-03/07, not replace their rules: AC-22/23 for uncertain start and
cleanup; AC-29 for approved-material-only access, source-modification refusal and project
separation; AC-31's HTTPS/API half; AC-36 subject to question 6. AC-30 login/refresh and
AC-31's SSH half remain deferred. Real-engine proof is CI-only; agents get no Docker
socket. There is no runtime evidence in this documentation change.

## Questions for the person

These go through `itos decision`, not an implementer's security choice. They are recorded
here for the coordinating loop; no unanswered question is silently resolved by this ADR.

| Question | What needs the person's answer | What waits |
| --- | --- | --- |
| 1. Codex login sharing | How should the worker share the host login now that Codex replaces `auth.json` by rename? Options, none selected: pin a still-in-place-writing Codex on host and worker; mount a dedicated host directory holding only `auth.json`, reached by both installs through symlinks Codex follows, read-write, with `auth.lock` as a single file (source reading only); use a worker login or API key delivered like a token; or copy in and back out, risking lost refreshes made on the other side. | A person-owned charter amendment, slice-32, T-48's file half, and any Codex kind in slice-29. The answer's spec decides if version 4 is needed. |
| 2. Refresh and revocation | Neither token kind refreshes. Provider revocation is external. Removing a grant requires approval without it before the next run, while a live session keeps its copy. Is that sufficient, and who owns refresh for a future kind that supports it? | Refresh/revocation promises beyond this per-command copy contract and later credential kinds. |
| 3. Deletion and account management | No command creates, rotates or deletes an original source; only staged copies are managed here. Should account-management commands exist, or is source-file deletion the person's act? | Account/profile management commands and deletion promises. |
| 4. GitHub scope | No scope or expiry is required or checked; checking would require a GitHub call. Should approval require a fine-grained token restricted to the checkout's repository, or an expiry? | Any such GitHub scope/expiry guarantee; delivery alone supplies none. |
| 5. Permission to use real credentials | Does this record satisfy, for these two token kinds, the contract's requirement that granting/revoking accounts, deleting profiles and rotating the sandbox SSH key and inspection CA be settled **before real credentials are used**? | Every real-credential run, including the person's smoke check, until answered. Synthetic scenarios remain possible. |
| 6. Credential-bearing refusal hostnames | How should logs represent a hostile CONNECT hostname containing credential bytes given 0022's verbatim target and AC-36's marker exclusion? Changing representation or explicitly revising the guarantee belongs to the person. | The hostname-marker probe passing, affected T-39/slice-28 logging specs, and slice-31/slice-33 accepting credential-bearing egress under AC-36. Disclose the current leak until the chosen rule has proof. |

No question authorizes quietly weakening the contract or its probes. Question 1 has
no replacement implementation selected; question 6 has no selected log fix.

## Contract passages superseded by the proposal or deferred

The person can amend the [pilot contract](../plans/container-pilot.md) from this list;
T-47 does not edit it or the charter.

- The Agreed scope Mounts row's "do not bind-mount additional host paths" and
  Credentials and Git's "This is not an exception allowing host-file mounts" would
  give way for staged copies. The proposed exception replaces copying into
  **isolated container-managed storage**, which fails before start as explained above.
- The Authentication row's "Do not mount the sources directly" and the prohibition
  on directly reading/writing host-side sources still hold for both token kinds.
  They, and the prohibition on sharing a writable credential store between projects,
  would give way to the charter's Codex read-write mount. Question 1 leaves that open.
- Credentials and Git's unresolved safe provisioning, callback, refresh and
  account-source updates, the corresponding "Decisions still open" entry, and
  Experiments item 3: this proposal selects provisioning for two token kinds only.
  No login runs in the worker and no callback is needed for them; questions 2 to 5
  retain refresh, revocation, deletion, account management and live-use consent.
- The Logging row and AC-36 remain required. Question 6 records a conflict with
  decision 0022, not a supersession or a reduced acceptance rule.
- The Git row's sandbox SSH identity, AC-31's SSH half and OpenCode remain deferred.

## Authority, sibling handoff and architecture changes

q-52 is the person's settled approval decision. The charter supplies the credential
shape, including the now-stale Codex premise that only the person may change. Grants
in `itos-container.yaml` with absolute host paths, and approval binding kinds and paths
rather than contents, come from the coordinator's slice-29/30 breakdown.

The staged binds, `/dev/shm`, unlink timing, fixed targets, kind names, version 3,
host checks, profiles, `approve.terminal-required` and its precedence, carried-forward
approvals, redaction and marker approach are **this record's own proposals**, for the
person to confirm or overturn. Their presence here supplies design to downstream
specs, not proof, charter amendments or authorization to use real credentials.

| Owner | Handoff |
| --- | --- |
| slice-29 | Take the closed version 3 shape, checks and refusal gates; omit the Codex kind its older why named. |
| slice-30 | Take canonical grants, profiles, review and q-52; update help and `docs/security.md`. |
| T-48 | Stage and recover through session ports, fake and adapter; the file half waits for question 1. Its older "failed delivery stops the workload before it starts" applies only before a start attempt. Add acquired-mount/lost-acknowledgement coverage. |
| slice-31 and slice-33 | Prove token delivery, isolation and uncertain-start cleanup in CI. For slice-33, "where gh expects it" means `GH_TOKEN` exported by workload argv from its file. Neither can claim refusal-log absence before question 6 and its proof. |
| slice-32 | Wait for question 1; the old in-place-write premise is invalid. |
| slice-26 and slice-27 | Apply q-52 wherever the first network-granting approval lands; slice-26's branch spec still calls the approver open. |
| T-39 and slice-28 | Reconcile question 6's eventual answer in the logging contract before downstream credential claims. Decision 0022 is unchanged here. |

Under the charter's drift rule, this change updates
[arc42](../architecture/arc42.md)'s opening and these sections, retaining its twelve
headings and all still-applicable offline limits:

- **Constraints** and **Context and scope** distinguish proposed credential grants
  from offline limits and deferred account management/network Git.
- **Building-block view** adds policy, readiness, approval and session responsibilities
  and the T-47, T-48 and slice-29 through slice-33 handoff.
- **Runtime view** adds q-52 and Credential run, including uncertain start rather than
  an unlink-based claim that a late start is fenced.
- **Deployment view** places staged copies and individual worker binds.
- **Cross-cutting concepts** records q-52, provisioning redaction, the Codex finding
  and the persisted-log conflict with its hostname-marker probe.
- **Decisions** links this proposal and records that q-52 revisited A1.
- **Quality requirements** maps unproven AC-29/31/36 and AC-22/23 credential evidence
  to their owners; **Risks and open questions** carries questions 1 through 6.

PLAN.md, README.md and `docs/ORCHESTRATING.md` still exclude credentials from the older
milestone. The charter governs; `p1-docs-admit-milestone-two-egress` owns their scope
refresh. This item edits only this record and arc42, leaving the ledger, registry,
contract, decision 0022 and sibling specs to their owners. T-47's two prose checks
failed before this work at the missing record and missing `### Credential run` section.
They check presence and shape, not security correctness, scenario success or AC closure.
