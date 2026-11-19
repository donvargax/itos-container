@phase-1
Feature: Deliver the approved Claude token to one owned run
  Run copies the approved source's current bytes to a private read-only worker file.
  Approval binds the grant, not its contents. Provisioning exposes no token in
  configuration, records or diagnostics, and owned teardown removes the copy.

  # Slice-31 spec only. Authorities: charter milestone two step 2, contract
  # AC-02/03/22/23/28/29 and AC-36 AS REVISED BY q-56, decisions 0025 and 0027,
  # architecture.md Slices, arc42 Credential run, and q-52/64/67/69.
  # Drafting baselines, not instructions to reset dependencies:
  # main 765b0c1ce1e9cdfe9d0266a1de28acbdbcfd189d;
  # slice-30 1e07f420d362e990c74253ac9d7e1fd2d70fdf7a (credentials-approval.feature);
  # T-48 2acb220ce6d147e84bc2f35d468f39543d8b988c (session token seam);
  # slice-27 8a15b6209d936bd16620c791ed0d265f73dd1673 (egress.feature).
  # Implementation waits for slice-30 and T-48. The allowlist row also consumes
  # slice-27's completed public wiring; the coordinator must order that handoff
  # before activating it. Do not implement egress here or drop its runtime row.
  # q-67 confirms slice-29's state-exclusion prerequisite is already satisfied.
  #
  # q-56 supersedes ADR 0025's open question 6 and blanket log-absence wording:
  # provisioning must not leak, but hostile workload output/traffic is unredacted.
  # A workload can put token bytes in a refused hostname; proxy stdout, Docker's
  # proxy log and the retained refusal log can then contain them. Verbatim target
  # logging remains. p1-platform-log-redaction owns scrubbing and the adversarial
  # hostname-marker probe; neither blocks this slice. Preserve that disclosure,
  # do not hash hostnames here, and do not claim general AC-36 closure. Reconcile
  # the stale question-6 prose in ADR 0025/arc42 with q-56 at implementation, and
  # in the same feat commit revise AC-36's own row in docs/plans/container-pilot.md
  # to q-56's wording: credential absence from persisted logs is a platform
  # guarantee kept by the persisting layer, not by each writer, and until
  # p1-platform-log-redaction lands a workload can place credential bytes in a
  # verbatim refused hostname persisted in proxy and refusal logs. That row cites
  # q-56; the broader redaction proof stays with that follow-up.
  # No real credential, provider/model call, login, refresh, Codex kind, GitHub
  # delivery, automatic environment export, harness init or new privilege belongs
  # here. ADR 0025 question 5 still bars real-credential use, including smoke runs.
  #
  # Admission and consent:
  # Admit version 3 with no credentials or only claude-token. Preserve versions
  # 1/2 without grants. A request containing github-token, alone or with Claude,
  # keeps the early policy.version /version exit-2 gate until slice-33, before
  # manifest, source, engine or protected-state access. Malformed policy and run's
  # argv/JSON checks retain their existing precedence. No partial Claude delivery
  # from a request containing an unsupported kind. Allowlist admission still
  # requires all of slice-27's prerequisites and frozen network restrictions.
  # Carry forward slice-30's canonical profiles, fingerprint, generation and
  # terminal approvals without new consent. Sources are selected from the frozen
  # grants, never a later policy edit. Adding/removing/repointing a grant is an
  # approval change; rotating valid contents/inode at the same path between runs
  # is not. No source data read, staging or engine mutation before matching
  # approval and successful metadata checks. A local approval mismatch precedes
  # engine access and reports sorted pointers with no values, as slice-26/27 do.
  #
  # Reuse readiness's complete source observation before engine inspection and
  # after it, then T-48's complete revalidation before copying. Preserve located
  # credential.* problems, their existing exits, and the written source index.
  # Recheck mode, owner, links, size, ancestors, mount identities, checkout and
  # BOTH selected/fixed control-state exclusions. No state record enumeration.
  # q-64/69 restrict source filesystem types to ext4, btrfs and xfs; every other
  # type refuses. Consume slice-30's implementation of that rule, never weaken it
  # to make a temporary fixture pass. A newly unsafe source before preflight ends
  # prevents all engine resource creation and staging. A later staging failure
  # may leave an owned anchor/intent for cleanup, never a started workload.
  #
  # Delivery is T-48's existing mechanism, not a new session lifecycle:
  # retain checked descriptors, reopen /proc/self/fd/<n> rather than source names,
  # and copy once without trimming, including final newlines. Refuse failed,
  # short or changed-size reads and changed checked metadata. Close retained
  # handles on success/refusal/cancellation, after any pending operation returns.
  # Only the copying adapter handles bytes, with its bounded cleared buffer; no
  # Go-memory zeroization guarantee. Intent precedes private tmpfs staging under
  # /dev/shm/itos-container-<32 lowercase hex digits>, directory 0700, file 0400,
  # host UID. Bind ONLY the staged regular file read-only at
  # /run/itos-container/credentials/claude-token. The original source, siblings
  # and host staging directory are never mounted. Keep all existing hardening,
  # private IPC, worker /dev/shm, literal argv as the main process and no restart.
  # Independently inspect exact mounts before start. Never put source bytes in
  # controller-set Env/argv, Engine API bodies, labels, inspect, approvals,
  # snapshots, intents, recovery, diagnostics or files under checkout/state.
  # The workload may explicitly read the file and export CLAUDE_CODE_OAUTH_TOKEN;
  # the controller does not do that or call a shell on its behalf.
  #
  # After start wait and the applicable mount detector, attempt staged unlink on
  # every path. An acquired mount survives unlink. Lost acknowledgement is not
  # evidence of no execution. Never retry start; reconcile the original owned
  # full ID under existing 10-second call/120-second startup/30-second teardown
  # bounds. No new workload time limit. Completion requires worker AND staging
  # absence plus every existing owned resource, never just a missing pathname.
  # Unknown workload outcome remains unknown; observed nonzero exit wins.
  # Controller loss follows A3 and existing stop, without rereading source,
  # current policy or approval. Unsafe/uncertain staging retains recovery and the
  # exact stop retry; no recursive deletion or adoption of a substituted inode.
  #
  # Presentation choices for this slice: source-copy refusal is
  # run.credential-refused exit 1; unavailable staging is
  # run.credential-unavailable exit 3. Preserve earlier located metadata errors,
  # isolation rules, budget failures and cleanup classification. Guidance names
  # the source permissions/identity or host tmpfs requirement as appropriate,
  # never source bytes or wrapped OS/API error text. Status keeps slice-30's
  # frozen credentials array. Add credential_staging to credential session JSON
  # entries with present/absent/unknown from T-48's observation, and a session-ID
  # labeled "credential staging: <value>" line in plain mode. Omit both for
  # no-grant sessions. Staging absence never labels an extant worker completed.
  #
  # Architecture/dependencies: extend existing run/status/stop vertical slices
  # and engine/readiness, engine/approval and engine/session responsibilities.
  # Only cmd binds CheckedSources, CredentialFiles, staging, entropy and recovery;
  # no slice-to-slice imports and no I/O in domain/porttest. Reuse T-48's schema-4
  # intent, locked operations, fake and disk/SDK contracts. The pinned Moby client
  # v0.6.2/API v1.56.1, x/sys v0.48.0, mountinfo v0.7.2 and Go standard library
  # already supply the needed capabilities; no new dependency or parser is needed.
  # Preserve notices and OS-as-value boundaries. Update CLI/CONFIG/security/run
  # help and implemented-versus-proposed architecture status with the feat commit.
  #
  # Required raw-port contract repair (triage's slice-31 finding):
  # CredentialContract currently observes only the domain's CredentialResult.
  # Add a raw CredentialFS section shared unchanged by the I/O-free fake and disk
  # adapter. Any refused Open returns handle 0, CredentialNode{} and its failure
  # status, retaining NO new pin; an already held valid source pin stays valid.
  # Failed Entries returns exactly nil, false, false, including failure after a
  # partial batch. Cover invalid/closed handles, missing/open/stat/metadata/identity
  # refusal paths and directory failures with raw result and retained-pin oracles.
  # State these shapes in port/credentials.go's CredentialFS comment. Do not infer
  # them from the domain's scan result. Keep the existing adapter-local assertions.
  # These shapes already hold in today's adapters, so these are regression checks,
  # not invented failing behaviour scenarios. Prove the shared oracle rejects a
  # deliberately faulty test fake (nonzero refusal handle/node or done on failure).
  # Run T-48's ReadPath/checked-source contracts too: a closed/refused capability
  # cannot read or alias a retained source. No source bytes enter the metadata port.
  #
  # Step handoff: every phrase below is NEW; no implementation or steps in this
  # commit. Reuse approval fixture/terminal/independent canonical-record helpers,
  # credentials_native/observation/observer steps, run_preflight steps and local
  # foreground SDK recorder, lifecycle_native_actions and slice-27's egress
  # fixture/recorder. The old credentials-only fixture lacks normal readiness;
  # compose a complete valid Git/manifest/host fixture instead. All Given steps
  # only arrange inputs, independent records, probes and barriers; none calls
  # approve/run or asserts product behaviour. Expected settings never come from
  # the product. Seed terminal-consent-equivalent approvals independently in a
  # private temporary home/state, never the operator's real state folder.
  #
  # Local rows execute the public binary where native metadata permits, otherwise
  # a test-only cmd assembly of the SAME command/preflight/runner/classifier with
  # raw filesystem observations and real disk/SDK adapters. No canned decision,
  # bypassed gate, production test flag, host mount operation or real engine.
  # Raw UID/mount/alias/error observations use slice-29's fake/adapter protocol;
  # positive and negative inputs share the same route. Distinguish that evidence
  # from native mounts. 05's pathname row barrier runs AFTER the final
  # complete validation but BEFORE data reopen; rename the old source to a safe
  # sibling (keep one link), then replace its original name. The checked inode
  # still qualifies; reading by pathname would return different bytes. 05's
  # policy row barrier runs after durable snapshot publication and BEFORE the
  # runner's delivery-phase source revalidation and selection; it rewrites the
  # policy's Claude grant to a second valid private synthetic source. Selection
  # from the frozen grant and retained pin never opens or walks that second one.
  # Faults after copying/read-only inspect are synchronized, never timed sleeps.
  # 06's size row uses the copying adapter's read seam: the read returns exactly
  # the original size, then the fixture changes the size before the final
  # descriptor stat, so only post-read metadata revalidation can refuse it. Its
  # boundary evidence is both journaled observations and the refusal before start.
  # In 06, inspected-mount faults affect the pre-start verification response;
  # subsequent cleanup observations expose the actual owned resource unchanged.
  # Source-size edits are fixture effects, not forbidden product modifications.
  # When captures early refusal/missing barrier/exit as data and returns for Then;
  # no setup assertion or missing-result panic can mask the expected red.
  #
  # Sources contain only fresh "itos-synthetic-credential-" plus 32 random lower
  # hex digits, with optional padding/final newlines. Hashes are calculated by the
  # harness and cooperative worker only, never production approval/provisioning.
  # Hash-only probe reports include nonce, path, mode/UID and attempted operations;
  # absent reports cannot satisfy either positive or isolation assertions.
  # Scan raw markers and base64 at all three offsets, with positive-control
  # files for EVERY detector. Scan all request/response metadata (including error
  # bodies), full worker inspect, text/JSON status, doctor/approve output, run/stop
  # diagnostics, approval/snapshot/intent/recovery and all checkout/state files.
  # Checkout absence is also continuous, not only final: each When first arms a
  # recursive inotify write observer on every checkout directory (create,
  # modify, attrib, close-write, moves, delete and queue overflow) and proves it
  # with a fixture-made transient create, write and remove of its own. Later
  # fixture or probe control writes go only to one fixture control directory and
  # are journaled by exact name. Any other checkout event, an unjournaled name or
  # an overflow fails every provisioning-surface assertion, even when the final
  # marker scan and checkout comparison pass.
  # Include proxy stdout, its Docker log and retained refusal log in 08's allowlist
  # row, whose ordinary refusal hostname contains NO marker. Require a nonempty
  # observed refusal and a successful controlled TLS exchange, not empty logs.
  # In 08 invoke doctor in plain/JSON and matching bare approve at a terminal
  # without an answer, retaining their output before run; collect plain/JSON
  # status while the hash probe waits after startup. Require successful results
  # and no new consent/publication. Do not scan absent command transcripts.
  # Fixture-owned sources, staged bytes while authorized, positive controls and
  # fixture-only journals containing inputs are excluded from searches of product
  # persistence; actual Engine API captures and product state are never excluded.
  # Workload transport carries only hashes in these cooperative probes. Failure
  # messages identify the leaking surface, never print markers or full records.
  #
  # @engine means guarded Linux amd64 native Docker CI ONLY, using the cached
  # image and non-root host UID/GID. Native fixture: one private 0700 root under
  # /var/tmp, as slice-29's credential fixture uses, holds the checkout, the
  # selected XDG_STATE_HOME, the source directory and the account home that the
  # existing engine probe assembly receives as SLICE_21_ACCOUNT_HOME. Only that
  # seam relocates the fixed <account home>/.local/state/itos-container boundary;
  # a private HOME or XDG_STATE_HOME does not, and the runner's real account
  # state is never used. Before any product command the Given independently
  # verifies, by its own statfs, statx(STATX_MNT_ID) and mountinfo parse and
  # never the product observer, for the source, checkout, selected state root,
  # fixed account state root and every ancestor of each: ext4, btrfs or xfs; a
  # mount ID naming exactly one row with a path root; no row of any other type
  # at, below or covering those four paths; every ancestor owned by root or the
  # host UID and not group/other-writable unless root-owned sticky; and /dev/shm
  # a root-owned sticky tmpfs. It records the result as data. When verification
  # failed, the When runs no product command and the first Then reports a
  # fixture blocker naming only path, type and UID; that is never red evidence,
  # and the allowlist is never weakened to admit a runner. Each @engine When then
  # runs JSON doctor on every fixture checkout before run. Doctor's readiness
  # passes today, so it proves the fixture clears shared readiness and an
  # unrelated readiness refusal cannot stand in for the credential red.
  #
  # Shard placement is the host's before red dispatch: .github is outside this
  # item's paths. TestEngineShards fails on this file until the whole
  # credentials-run.feature joins the offline shard, in ci.yml's matrix entry and
  # in shardOfflineFiles with its inventory, negative-control and selector
  # regressions, plus a slice-31 placement note in decision 0024 citing measured
  # offline step times. Keep four shards, the unchanged 180-second step gate and
  # whole files. Red dispatch and activation must each show the offline shard
  # selecting exactly the ten CREDRUN-08 to -12 rows (3, 2, 1, 2, 2) and no other
  # shard selecting any. If measured time leaves no room, stop for the coordinator.
  #
  # Helpers built by the harness run only inside disposable workers. No real
  # service/credential, pull/build, fallback image or agent Docker access. The
  # allowlist row reuses slice-27's isolated fake endpoint and positive controls.
  # Audit product effects before bounded identity-checked harness cleanup; that
  # cleanup is never evidence of product teardown. No credential bytes are
  # deliberately written into the checkout, even for a negative-control probe.
  #
  # Supersession only in the implementing feat commit: replace exactly
  # CREDENTIALS-13's obsolete blanket version-3 run-gate rows and CREDAPPROVAL-17's
  # Claude row with these cases. Preserve CREDENTIALS-14, CREDAPPROVAL-18 and -19
  # unchanged, and CREDAPPROVAL-17's GitHub and mixed-kind rows. Remove the
  # blanket temporary version-3 run warning for supported requests, retain a
  # truthful GitHub delivery warning for requests still gated, and keep every
  # consent/exposure/log disclosure.
  # Do not change neighbours in this drafting commit. Neighbours include
  # credentials-policy/approval, approval, run/run-preflight, offline, lifecycle,
  # recovery/control-state and egress scenarios plus the readiness raw/source and
  # session staging/mount/uncertain-start/recovery contracts. Keep no-grant v1/v2
  # regressions live; 01 proves v3 no-grant admission without credential effects.
  #
  # Commit missing steps red first with @wip retained. Remove @wip for local
  # assertion-level red/green runs; use only coordinator-dispatched CI for engine
  # red/green evidence. Today's version-3 gate makes 01..07 fail at their first
  # Then (or their first control Then); 08..12's first Then is the fixture and
  # doctor control, which passes today, and they fail at the next Then. 13 fails
  # at missing help and 14 at the missing public staging projection. These are
  # expected failures, not executed evidence. No local mutation or
  # whole-features race run. The implementer may commit freely in its own
  # workspace; the host makes the landing commits and handles itos, submission,
  # publication and review.

  # Admission and exact per-command configuration, with a no-grant control.
  @wip @slice-31 @ID-CREDRUN-01 @local-engine-fixture @linux
  Scenario Outline: An unchanged approved version three request reaches owned execution
    Given a local Claude run fixture requesting "<request>" with an independently matching generation 4 approval
    When the real run command executes the fixture's literal probe argv
    Then the local Claude run completes owned execution with exit 0 using profile "<profile>"
    And its independently observed create and inspect configuration has "<mounts>" and unchanged hardening
    And its protected snapshot is exactly generation 4 and the active approval is unchanged without consent
    And its credential effects are exactly "<effects>" in durable lifecycle order
    And all cooperative credential provisioning surfaces contain no synthetic marker

    Examples:
      | request | profile                | mounts                       | effects                         |
      | claude  | offline-credentials-v1 | one exact staged Claude bind | copy once then unlink and clean |
      | none    | offline-v1             | no credential mount          | no source read or staging       |

  @wip @slice-31 @ID-CREDRUN-02 @local-engine-fixture @linux
  Scenario Outline: A requested Claude grant needs the matching current approval
    Given a local Claude run fixture with prior approval "<prior>" and version three request "<current>"
    When the real run command executes the fixture's literal probe argv
    Then Claude run reports "<rule>" with exit 1 and exactly changed pointers "<pointers>"
    And it names terminal review with "itos container approve" without requesting consent itself
    And it reads no source data contacts no engine and publishes no approval snapshot intent session or staging
    And its existing approval checkout and sources are unchanged

    Examples:
      | prior              | current          | rule                 | pointers              |
      | missing            | claude           | run.approval-missing |                       |
      | checkout-only      | claude           | run.approval-missing |                       |
      | version three none | claude           | run.approval-changed | /credentials /profile |
      | claude             | repointed claude | run.approval-changed | /credentials          |
      | claude             | none             | run.approval-changed | /credentials /profile |

  # Raw metadata fixtures cover non-root-inexpressible owner and mount changes;
  # aliases have one link so link-count refusal cannot hide a missing exclusion.
  @wip @slice-31 @ID-CREDRUN-03 @local-engine-fixture @linux
  Scenario Outline: Approval cannot bless a source that is now unsafe or unavailable
    Given a local Claude run fixture requesting "claude" with an independently matching generation 4 approval
    And its approved Claude source now has raw observation "<defect>"
    When the real run command executes the fixture's literal probe argv
    Then Claude run reports source problem "<rule>" at "/credentials/0/source" with exit <exit>
    And it reads no source data creates no engine resource and stages no credential
    And its existing approval checkout and sources are unchanged apart from fixture edits

    Examples:
      | defect                                    | rule                   | exit |
      | source missing                            | credential.missing     | 3    |
      | group-readable source                     | credential.permissions | 1    |
      | another UID owns the source               | credential.owner       | 1    |
      | source path contains a symlink            | credential.symlink     | 1    |
      | source has a second hard link             | credential.links       | 1    |
      | checkout descendant bind alias            | credential.location    | 1    |
      | selected state descendant bind alias      | credential.location    | 1    |
      | fixed account state descendant bind alias | credential.location    | 1    |
      | source filesystem is tmpfs                | credential.filesystem  | 3    |
      | source filesystem is nfs4                 | credential.filesystem  | 3    |
      | complete mount evidence unavailable       | credential.unavailable | 3    |

  @wip @slice-31 @ID-CREDRUN-04 @local-engine-fixture @linux
  Scenario Outline: Run rechecks source safety after engine inspection
    Given a local Claude run fixture requesting "claude" with an independently matching generation 4 approval
    And the fixture will change the source to "<defect>" at the image inspection barrier
    When the real run command executes the fixture's literal probe argv
    Then Claude run reports source problem "<rule>" at "/credentials/0/source" with exit 1
    And the source was inspected before and after the barrier without a data read
    And it creates no engine resource stages no credential and leaves approval unchanged

    Examples:
      | defect                                    | rule                   |
      | group-readable source                     | credential.permissions |
      | another UID owns the source               | credential.owner       |
      | checkout descendant bind alias            | credential.location    |
      | selected state descendant bind alias      | credential.location    |
      | fixed account state descendant bind alias | credential.location    |

  # A pathname reopen would copy the replacement, even though the old pin is safe.
  # Rereading the current policy after the freeze would select the second source.
  @wip @slice-31 @ID-CREDRUN-05 @local-engine-fixture @linux
  Scenario Outline: Token copying reads the frozen grant's checked inode, never a later policy or pathname
    Given a local Claude run fixture requesting "claude" with an independently matching generation 4 approval
    And the fixture will "<substitution>"
    When the real run command executes the fixture's literal probe argv
    Then the staged Claude bytes equal the original checked bytes including final newlines and differ from the replacement
    And the substitution barrier was reached at its scheduled boundary
    And the replacement source was never opened walked or read and both original and replacement sources are unchanged
    And the source data reopen uses only the retained descriptor
    And its protected snapshot and session record keep the original frozen Claude grant and source path at generation 4
    And the local Claude run completes owned execution with exit 0 using profile "offline-credentials-v1"
    And all source handles and owned staged paths are released after their operations finish
    And all cooperative credential provisioning surfaces contain no synthetic marker

    Examples:
      | substitution                                                                                            |
      | rebind the source name after final validation while preserving the checked inode at a safe sibling      |
      | repoint the policy grant to a second valid source after durable freeze and before delivery revalidation |

  @wip @slice-31 @ID-CREDRUN-06 @local-engine-fixture @linux
  Scenario Outline: Failed copying or an unsafe credential mount never starts the workload
    Given a local Claude run fixture requesting "claude" with an independently matching generation 4 approval
    And its credential preparation will encounter "<fault>"
    When the real run command executes the fixture's literal probe argv
    Then Claude run reports "<rule>" with exit <exit> without source bytes or raw error text
    And the failure occurred at the scheduled boundary and no worker start was attempted
    And owned cleanup is attempted for every created resource and partial staged file under original protected authority
    And cleanup either confirms all owned resources absent or retains original recovery with the exact stop retry
    And all source handles are released and the original source and approval are unchanged apart from fixture edits
    And all cooperative credential provisioning surfaces contain no synthetic marker

    Examples:
      | fault                                           | rule                       | exit |
      | checked source data open denied                 | run.credential-refused     | 1    |
      | short read of the checked source                | run.credential-refused     | 1    |
      | source size changes after a full original read  | run.credential-refused     | 1    |
      | staging root is not tmpfs                       | run.credential-unavailable | 3    |
      | short write into the newly owned staged file    | run.credential-unavailable | 3    |
      | inspected Claude bind is writable               | run.isolation-unverified   | 1    |
      | inspected Claude bind names the original source | run.isolation-unverified   | 1    |
      | inspected Claude bind has a different target    | run.isolation-unverified   | 1    |

  # The first control fails today; the refusal alone already passes the v3 gate.
  @wip @slice-31 @ID-CREDRUN-07 @local-engine-fixture @linux
  Scenario Outline: Enabling Claude delivery cannot partly admit a GitHub request
    Given independent local Claude run fixtures for an approved Claude control and request "<request>"
    And the unsupported request has no readable manifest source engine or protected state
    When the real run command is invoked for both fixtures
    Then the Claude control completes owned execution with its one exact staged bind
    And the unsupported request reports "policy.version" at "/version" with exit 2 before any downstream access
    And the unsupported request publishes no snapshot intent session or staged material

    Examples:
      | request           |
      | github            |
      | claude and github |

  # Real mount acceptance and cooperative provisioning absence, including a
  # nonempty ordinary refusal log in the allowlist row under q-56's limit.
  @wip @slice-31 @ID-CREDRUN-08 @engine @linux
  Scenario Outline: The approved workload alone reads a read-only staged Claude file
    Given a CI Claude checkout with a private synthetic source and matching "<network>" approval
    And the cooperative credential probe and every marker detector have independent positive controls
    When the product inspects with doctor and matching terminal approve then runs the credential probe through active status and workload exit <exit>
    Then the native fixture verification passed and JSON doctor reported every readiness check and the Claude source ready
    And the worker reports the exact source hash including final newlines from the fixed Claude path as the non-root host UID
    And native inspection and worker probes show one read-only mode-0400 regular Claude file and all existing hardening
    And write attempts fail and the original host source its sibling and host staging directory cannot be opened from the worker
    And no controller-set environment or argv contains a token or automatically exports CLAUDE_CODE_OAUTH_TOKEN
    And its network evidence is "<network evidence>"
    And every specified cooperative provisioning surface contains neither the raw marker nor any base64-offset encoding
    And staged paths are absent after acknowledged startup while the worker can still read the copy
    And run returns the observed workload exit <exit> after confirmed removal of the worker staging and all other owned resources
    And the checkout source approval and unrelated resources are unchanged

    Examples:
      | network   | exit | network evidence                                                            |
      | offline   | 0    | network none with no proxy or refusal log                                   |
      | offline   | 17   | network none with no proxy or refusal log                                   |
      | allowlist | 0    | successful controlled TLS and an ordinary denied hostname in all log copies |

  @wip @slice-31 @ID-CREDRUN-09 @engine @linux
  Scenario Outline: A live worker keeps its copy and the next run reads the rotated source
    Given a CI Claude checkout with a private synthetic source and matching "offline" approval
    And a second distinct synthetic source value is reserved for rotation by "<rotation>"
    When the product inspects with JSON doctor then starts a waiting hash probe then the fixture rotates the source and releases a second read
    Then the native fixture verification passed and JSON doctor reported every readiness check and the Claude source ready
    And both reads in that worker report the original hash and the source reports only the rotated value
    When that worker exits and the product runs a fresh hash probe without another approval
    Then the fresh worker reports the rotated hash through a newly staged Claude file
    And both runs keep the original approval fingerprint and generation without a consent prompt
    And both runs confirm complete owned cleanup and no cooperative provisioning leak

    Examples:
      | rotation                            |
      | rewriting the private file in place |
      | replacing it with a valid new inode |

  @wip @slice-31 @ID-CREDRUN-10 @engine @linux
  Scenario: One project's grant cannot supply another worker or a later no-grant run
    Given two independent CI Claude projects with distinct synthetic sources and separately matching approvals
    And a third independent approved version three project requests no credentials
    When the product inspects every project with JSON doctor then runs their probes concurrently and stops the first project while the other probes remain active
    Then the native fixture verification passed and JSON doctor reported every readiness check and each Claude source ready
    And each credential worker reports only its own source hash and cannot open the other project's source or staged path
    And the no-grant worker executes its probe and cannot open either token or the fixed Claude path
    And stopping the first project removes only its resources while the second still reads its original copy
    When the remaining probes finish and the fixture removes the first project's grant and independently seeds its matching no-grant approval
    And the product runs the no-grant token probe in the first project's same checkout and project identity
    Then that later run has a fresh session and its probe reports a nonce-bearing denial from the fixed Claude path
    And that later run executes without source reads staging credential mounts or residual token access
    And every run confirms its owned cleanup without modifying any original source or leaking a marker through provisioning

  # The read is deliberately released AFTER unlink. The runtime has acquired its
  # mount, but the controller never receives the successful start acknowledgement.
  # Staged unlink follows the start wait and precedes every request that could
  # end the original worker. The recorder holds the first such request (kill,
  # stop, signal or forced remove) before forwarding it, confirms staged unlink,
  # releases the probe, captures its hash, then forwards the held request. The
  # hold is bounded at half the engine tier's call budget; at that bound it
  # forwards and records expiry, which fails the hash Then. Existing call,
  # startup and teardown budgets are unchanged. The removal fault applies only
  # to removal requests after that release, so both rows reach the barrier.
  # Missing start/barrier/hash evidence is a failing Then, never a skipped probe.
  @wip @slice-31 @ID-CREDRUN-11 @engine @linux
  Scenario Outline: Lost start acknowledgement cannot turn unlink into proof of no execution
    Given a CI Claude checkout with a private synthetic source and matching "offline" approval
    And the engine recorder will lose the start acknowledgement after mount acquisition and delay the probe until staged unlink
    And the engine recorder will hold the first request that could end the original worker until the delayed probe reports
    And original worker removal will be "<removal>" until the explicit stop retry
    When the product inspects with JSON doctor then attempts the credential run while the recorder releases the delayed hash probe after unlink
    Then the native fixture verification passed and JSON doctor reported every readiness check and the Claude source ready
    And the recorder held the first worker-ending request until confirmed unlink and the probe's hash report within its bound
    And the acquired worker mount supplies the exact source hash despite absent staged paths and the lost acknowledgement
    And exactly one start was attempted and reconciliation uses only the original owned full worker ID
    And run reports an observed workload outcome or explicit uncertainty without inferring success or non-execution from unlink
    And cleanup reports "<cleanup>" only from worker and staging observations within existing budgets
    When the fixture restores removal and the exact reported session is stopped if recovery remains
    Then the worker staging and all other owned resources are confirmed absent with no repeated start or source read
    And all cooperative credential provisioning surfaces contain no synthetic marker

    Examples:
      | removal     | cleanup                                                     |
      | available   | complete only after both worker and staging are absent      |
      | unavailable | incomplete with protected recovery and the exact stop retry |

  @wip @slice-31 @ID-CREDRUN-12 @engine @linux
  Scenario Outline: A fresh controller recovers staged credentials without the mutable request or source
    Given a CI Claude checkout with a private synthetic source and matching "offline" approval
    And the controller will be killed at "<barrier>" with its original staging identity durably recorded
    When the product inspects with JSON doctor then attempts the run then the fixture kills that controller and removes the source current policy and active approval
    Then the native fixture verification passed and JSON doctor reported every readiness check and the Claude source ready
    And original protected session evidence identifies the staged credential and every acquired owned resource without source bytes
    When fresh product processes request plain and JSON status then stop the original session twice
    Then both status modes show the frozen Claude grant and credential staging "present" without claiming completion
    And stop confirms worker staging and all other owned resources absent using only original protected authority
    And neither status nor stop reads source data mutable policy or active approval and the second stop is idempotent
    And no unobserved workload success is invented and no cooperative provisioning surface leaks the marker

    Examples:
      | barrier                                            |
      | after staged copy before worker create             |
      | after accepted start before acknowledgement unlink |

  # This local recovery projection uses independently seeded schema-4 intent,
  # snapshot and public session records, a durably absent worker and an owned
  # staging directory. Status must observe the stager, not just decode intent.
  # The public record is failed with pending cleanup, never a completed tombstone.
  # After the first successful status the fixture schedules one changed raw
  # observation; raw faults require no native privilege or actual tmpfs here.
  # Safe absence can complete; uncertainty/substitution must retain authority.
  @wip @slice-31 @ID-CREDRUN-14 @local-engine-fixture @linux
  Scenario Outline: Staging recovery distinguishes confirmed absence from unsafe or unavailable observations
    Given a local stopped Claude session with independently seeded original authority and present owned staging
    And the staging observation will change to "<observation>" after the first status
    When fresh product processes read status before and after that change then stop the original session
    Then the first plain and JSON status reports the frozen Claude grant and credential staging "present"
    And the second status reports credential staging "<presence>" without reading source data
    And stop reports "<result>" and never deletes a substituted or unexpected entry
    And retained recovery contains only original identities and the exact stop retry when cleanup is incomplete
    And all cooperative credential provisioning surfaces contain no synthetic marker

    Examples:
      | observation                          | presence | result                                     |
      | original staging paths absent        | absent   | confirmed complete owned cleanup           |
      | staging observation unavailable      | unknown  | incomplete cleanup with original authority |
      | another inode at the staging path    | unknown  | refused cleanup with original authority    |
      | an unexpected entry in the directory | unknown  | refused cleanup with original authority    |
      | a symlink replaces the staged file   | unknown  | refused cleanup with original authority    |

  # Public explanation belongs to the delivery change; it must not promise
  # automatic Claude invocation or redaction that q-56 explicitly deferred.
  @wip @slice-31 @ID-CREDRUN-13
  Scenario: Run help describes explicit file delivery and its exposure limits
    When the product's run help is requested
    Then help names the fixed Claude token file its read-only per-run copy and the need for a matching terminal-approved grant
    And help explains that workload argv must read the file itself and source rotation affects only subsequent runs
    And help discloses unredacted workload output and traffic and possible credential-bearing refused hostnames in persisted logs
    And help retains the existing late-checkout-socket limitation and does not claim GitHub or Codex delivery
    And help exits with code 0 without inspecting sources state or the engine
