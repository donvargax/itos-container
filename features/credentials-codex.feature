@phase-1
Feature: Share the approved Codex login through one dedicated writable directory
  Atomic replacement of auth.json reaches both the host and the worker.
  Approval grants that directory and a shared lock explicitly, and cleanup reports
  unsafe source changes without repairing or deleting the person's login.

  # Authority: charter milestone 2 step 2, q-72 and the coordinator's settled
  # 2026-11-16 00:50 UTC answer, overriding T-47 question 1 and the registry's
  # single-file wording. The person chose sharing the existing host login RW on
  # 2026-11-06, accepting exposure/corruption because copies diverge on refresh.
  # Do not pin an old in-place writer, create a worker login/API key, copy in/back,
  # or ask question 1 again. Question 5 still bars ALL real credentials, including
  # a person's smoke check. No scenario calls Codex, a provider, account or model.
  # Decision 0030-codex-shared-directory records the architecture amendment.
  #
  # Drafting baselines (read their current successors before implementation):
  # main dc5b9d440798bf5959acd8185c6e4a12a70f3050;
  # slice-30 ac944f4e96a1b10ba2884ab88cbb67cfec329eea, credential approval spec/steps;
  # T-48 fff89462fe9f77be1e0db5240d3ac89980b3337e, session credential seam.
  # Both dependencies are still being built. This draft changes no sibling feature
  # or ledger. T-48 currently stages ONLY the two token kinds. Extend its session
  # ports/fake/adapters here for shared sources; NEVER send Codex sources through
  # its staging unlink/removal path. Policy schema version and recovery schema
  # numbers are independent; T-48 already uses recovery schema 4 for token staging.
  #
  # Policy decision: version 4 is required. Versions 1/2/3 keep their exact meaning
  # and version 3 still refuses codex-auth. Version 4 carries version 3's optional
  # nonempty credentials set and token forms unchanged, plus at most one closed
  # {kind: codex-auth, source: <dedicated directory>, lock: <host auth.lock file>}.
  # source and lock are REQUIRED literal absolute paths with slice-29's 4095-byte
  # grammar; no implicit host-home lookup, expansion, target override or login.
  # Missing/wrong-type/extra fields retain policy.required/type/unknown_key; invalid
  # lock uses policy.credential_source at /credentials/<i>/lock. Duplicate kind
  # and unknown kind retain policy.credential_duplicate/kind. A lock field on a
  # token remains unknown_key. Parsing never observes sources. Publish schema 4
  # and update CONFIG/CLI when implementing, preserving older schemas.
  #
  # Pin the source contract reviewed by T-47 to Codex rust-v0.167.0: storage_file
  # follows the auth.json symlink and writes/renames a temporary sibling at its
  # resolved target; auth.lock is opened/locked at CODEX_HOME without replacement.
  # Select a SEPARATE single-file RW lock bind, not a lock in the auth directory.
  # The person prepares the host's auth.json symlink and an existing private lock
  # (empty is valid); this tool never creates/moves/rewrites host login material,
  # the host symlink, or a missing lock. The lock is mandatory for this pin, which
  # uses it for refresh. Replacing the host lock inode requires new approval.
  # Do not infer success with an arbitrary newer Codex; harness-version changes
  # must recheck this source contract. Synthetic writers test the mount semantics.
  #
  # Fixed worker targets: directory /run/itos-container/credentials/codex-auth,
  # lock file /run/itos-container/credentials/codex-auth.lock, both RW. Only that
  # directory contains the shared auth.json. No host CODEX_HOME or parent is mounted.
  # A harness can use symlinks from its isolated CODEX_HOME/auth.json and auth.lock
  # to these targets; slice-34 owns home/bootstrap integration. Here literal argv
  # builds those links under worker /tmp solely for synthetic writers. It never
  # makes CODEX_HOME the shared directory (Codex writes other state there).
  #
  # Metadata checks in shared readiness, no credential content reads or hashes:
  # no-follow every source/lock path component, trusted ancestors, host UID;
  # source is a directory of exactly mode 0700, with exactly ONE entry auth.json;
  # auth.json is no-follow regular, link count 1, host UID, mode 0600, size 1..65536;
  # lock is no-follow regular, link count 1, host UID, mode 0600, size 0..65536.
  # The host's person-made symlink is NOT a source path and is never traversed by
  # the controller. Reject extra entries, including hidden files, auth.lock and
  # abandoned temporary siblings; never clean them. A concurrent refresh can cause
  # a transient refusal. No polling until an unsafe observation happens to pass.
  # Share slice-30's q-64 ext4/btrfs/xfs allowlist, pinned checkout comparison,
  # checkout inventory, protected-state projections, deadlines and both complete
  # mount observations. Exclude source directory, auth.json, lock and ancestors
  # from checkout/state aliases in BOTH directions, including a protected subtree
  # hidden below a host submount. Refuse submounts inside the auth directory; use
  # a nonrecursive bind. Sharing a device alone does not mean sharing a subtree.
  # No new state-tree enumeration or permissive fallback. Directory link counts
  # are not file link counts; never require the directory to have one link.
  #
  # Existing credential.* rules/exits apply to source/lock metadata. New rules:
  # credential.directory (1), source is not a directory;
  # credential.directory_entries (1), anything besides exactly auth.json;
  # credential.changed (1), a checked directory/lock identity changed during one
  # observation/startup; incomplete evidence is credential.unavailable (3).
  # auth.json failures locate /credentials/<i>/source; lock failures locate /lock.
  # Exact-mode failures use credential.permissions. Keep location/symlink/ancestor
  # precedence from slice-29. Missing auth.json alone is credential.missing (3).
  # Observe before engine access, after engine inspection, after consent and
  # immediately before create AND start. Late failure forbids start and removes
  # any owned pre-start resources. Content replacement by a new valid auth.json in
  # the SAME directory is allowed at all these points, not a credential.changed.
  #
  # Approval extends slice-30's canonical credentials array. For codex-auth the
  # closed entry is {kind,source,target,access,directory,lock}; access is read-write;
  # directory is {device,inode}; lock is {source,target,device,inode,access}, also
  # read-write. Device/inode are canonical unsigned decimal strings (inode > 0),
  # taken from no-follow descriptor metadata, NOT mountinfo's device (q-57).
  # Contents, hashes, auth.json inode, timestamps and sizes NEVER enter settings.
  # Bind source directory identity so replacing a safe directory at the same path
  # needs consent; bind lock identity so two independent locks cannot look approved.
  # Token entries remain {kind,source,target}. Sort by kind, one /credentials diff
  # for any entry change. With codex-auth choose offline-codex-shared-v1 or
  # allowlist-codex-shared-v1; without it retain slice-30's profiles and shapes,
  # retaining declared policy.version 4. Older approval bytes remain readable.
  # Historical valid targets remain readable but changed, as in slice-30; only
  # the current targets/access can match a new candidate. Invalid closed shapes,
  # identities, order or profile combinations refuse, never fill from live policy.
  #
  # Review shows complete canonical settings, sorted grants including the directory
  # and lock paths, identities, targets and RW access, differences, then y/N.
  # Replace the token-only checkout-directory claim for these profiles. State:
  # "The worker can read, replace or corrupt the host's shared Codex login."
  # "The host and worker share the auth directory and auth.lock; no login copy is made."
  # "Checks before start and at stop do not protect the interval between them; the host's Codex follows symlinks planted by the worker."
  # "Only the person sets up the host login symlink. Real credentials remain barred until decision 0025 question 5 is answered."
  # Also retain provider-scope, unredacted-output and (if allowlisted) exfiltration
  # and refusal-log warnings from slice-30, without falsely saying Codex is copied
  # or its original source is never written. Mixed grants explain token copies
  # separately. Unchanged terminal approval repeats the review without prompt or
  # write. Every --fingerprint refuses approve.terminal-required (1), even matching,
  # malformed or already approved; bare redirected/JSON approval refuses
  # approve.confirmation-required (1). Both fixes name terminal bare approve only.
  #
  # Session delivery: reuse slice-17's descriptor-backed owned directory anchor
  # design for the dedicated directory, with separate intent, identity and mount
  # target. It binds the approved pinned directory, not a later path lookup.
  # The separate lock bind names the revalidated lock. Verify exact RW mounts,
  # nonrecursive directory semantics and all existing hardening before start;
  # no arbitrary additional mount or credentials in create Env/argv/labels.
  # Freeze only paths/identities/access. Recovery distinguishes these EXTERNAL
  # sources from OWNED anchors and staging: remove owned worker/anchor resources,
  # never unlink, chmod, chown, replace or repair source entries or the host lock.
  # Status uses frozen grants, not current requests, approval or source existence.
  # Exec when implemented inherits the session mounts; no per-exec credential copy.
  # Lift run's gate only for implemented kinds/network modes. An absent delivery
  # or allowlist implementation still refuses before source/engine/state access.
  # Do not activate token delivery or proxy egress merely by accepting version 4.
  #
  # After owned worker termination/removal is confirmed, BEFORE releasing its
  # anchor, inspect the recorded directory/lock and current auth.json again, with
  # no-follow metadata checks and the recorded directory/lock identities. Do this
  # on foreground completion, cancellation and recovery stop. A safe auth.json
  # inode replacement is expected. An unsafe source never blocks terminating the
  # worker or causes repair. Report unsafe as recovery.credential-source-unsafe
  # (1), missing/unobservable as recovery.credential-source-unavailable (3), naming
  # path and underlying metadata rule without following/reading the offending node.
  # A zero workload exit becomes that nonzero controller exit; preserve any already
  # observed nonzero workload exit and surface the check problem alongside it.
  # Persist a metadata-only credential_check {state: safe|unsafe|unknown, rule}
  # in protected session history (rule is empty for safe). Status exposes that last
  # completed check, not a new safety claim; before a completed check use unknown.
  # Owned-resource removal and source-check state are independent facts. Completed
  # teardown may record unsafe/unknown; do not keep a worker running to await repair.
  # Repeated stop keeps the recorded warning and does not rewrite or follow sources.
  # If worker absence is unconfirmed, retain recovery and the exact stop retry,
  # check state unknown, never claim safe/complete. Lost start acknowledgement
  # never proves no workload ran: reconcile the original full ID, never retry start.
  #
  # Evidence/implementation handoff: every Codex-specific phrase below is NEW.
  # Reuse generic invocation in steps_test.go, slice-30's approval fixture/oracle,
  # terminal transcript and source-access observers; compose new fixture builders.
  # Givens only write private synthetic files/policy/independent protected records
  # or configure observers. They never call approve or require this feature to work.
  # Whens collect refusal/timeouts as observations; assertion failures belong in
  # Then, including a refused start in engine rows (do not fail setup waiting for it).
  # Independent canonical fixtures must not ask the product for expected hashes.
  # Local Engine API fixtures are NOT native mount evidence. Foreign UID, bind-alias
  # and unavailable metadata cases use test-only assembly of real domains/ports;
  # never need root/chown/mount on the agent's host. Run shared fake/disk/Docker
  # contracts for the new capability, including mutation-safe returned values.
  # Features import no production packages. New source logic stays in engine/policy,
  # readiness, approval and session; existing command slices stay thin; only cmd
  # assembles adapters. Reuse pinned Go/x/sys/mountinfo/Moby/YAML; no new dependency.
  #
  # Fixtures use private host-UID-owned parents on a named permitted filesystem,
  # isolated HOME/state, non-root worker UID, random itos-synthetic-credential-<hex>
  # markers, and sentinels OUTSIDE the shared directory. Never use real user paths.
  # Metadata-only access observers have a deliberate read positive control.
  # Native writers report hashes and acknowledgements, never marker bytes; fixture
  # observers alone compare bytes. Marker scans test raw/base64 at all offsets with
  # a positive control. They cover provisioning output/Engine API/inspect, checkout
  # and protected records, not deliberate workload copies or unresolved hostname
  # logging. No AC-36 closure or live refresh claim. @engine rows run only in CI.
  # Implementer commits steps red while @wip, then removes @wip only for each red
  # run; no undefined step is evidence. Expected reds are the named Then assertions,
  # currently policy.version for version 4, absent Codex observation/recovery or
  # missing help; observed red/green comes later. Preserve neighbours in credentials-policy/approval, lifecycle, anchor,
  # control-state, offline and token-delivery features. This spec runs no engine.

  @wip @slice-32 @ID-CODEXAUTH-01
  Scenario: Version four describes directory sharing without changing older policies
    Given a Codex policy fixture with synthetic absolute directory and lock paths
    When file-only doctor inspects version four with Codex and both token kinds then versions one two and three with their existing forms
    Then all valid policies retain their declared version and exactly their requested grants without observing sources
    And version three still refuses the Codex kind with "policy.credential_kind" and exit 2

  @wip @slice-32 @ID-CODEXAUTH-02
  Scenario Outline: The Codex policy entry has no implicit or extensible fields
    Given a Codex policy fixture with synthetic absolute directory and lock paths
    And its version four Codex entry has "<defect>"
    When file-only doctor inspects the Codex request
    Then the Codex policy refusal is "<rule>" at "<path>" with exit 2 and no source observation

    Examples:
      | defect                 | rule                        | path                  |
      | missing source         | policy.required             | /credentials/0/source |
      | missing lock           | policy.required             | /credentials/0/lock   |
      | numeric lock           | policy.type                 | /credentials/0/lock   |
      | relative lock          | policy.credential_source    | /credentials/0/lock   |
      | lock with dotdot       | policy.credential_source    | /credentials/0/lock   |
      | inline auth field      | policy.unknown_key          | /credentials/0/auth   |
      | caller-selected target | policy.unknown_key          | /credentials/0/target |
      | duplicate Codex kind   | policy.credential_duplicate | /credentials/1/kind   |
      | token entry with lock  | policy.unknown_key          | /credentials/0/lock   |

  @wip @slice-32 @ID-CODEXAUTH-03
  Scenario: Credentials-only inspection validates metadata without reading login bytes
    Given a private Codex source fixture with exactly auth.json and a separate empty lock
    And the Codex source data-access observer has a deliberate-read control
    When credentials-only doctor inspects the Codex request in text and JSON
    Then both Codex inspections succeed with the exact directory and lock grants and no content access
    And the host sources and symlink are unchanged and no approval engine call or session is produced

  @wip @slice-32 @ID-CODEXAUTH-04
  Scenario Outline: Source inspection refuses unsafe directory entries and locks
    Given a private Codex source observation fixture with "<defect>"
    When credentials-only doctor consumes the Codex source observations
    Then the Codex source refusal is "<rule>" at "<path>" with exit "<exit>" and no content access
    And no Codex source repair approval engine call or session occurs

    Examples:
      | defect                        | rule                         | path                  | exit |
      | source is a regular file      | credential.directory         | /credentials/0/source | 1    |
      | source path is a symlink      | credential.symlink           | /credentials/0/source | 1    |
      | source ancestor is a symlink  | credential.symlink           | /credentials/0/source | 1    |
      | directory has another owner   | credential.owner             | /credentials/0/source | 1    |
      | directory mode is 0750        | credential.permissions       | /credentials/0/source | 1    |
      | auth.json is absent           | credential.missing           | /credentials/0/source | 3    |
      | auth.json is a symlink        | credential.symlink           | /credentials/0/source | 1    |
      | auth.json is a FIFO           | credential.regular           | /credentials/0/source | 1    |
      | auth.json is a socket         | credential.regular           | /credentials/0/source | 1    |
      | auth.json is a directory      | credential.regular           | /credentials/0/source | 1    |
      | auth.json has two links       | credential.links             | /credentials/0/source | 1    |
      | auth.json has another owner   | credential.owner             | /credentials/0/source | 1    |
      | auth.json mode is 0640        | credential.permissions       | /credentials/0/source | 1    |
      | auth.json has zero bytes      | credential.size              | /credentials/0/source | 1    |
      | auth.json exceeds 65536 bytes | credential.size              | /credentials/0/source | 1    |
      | extra hidden sibling          | credential.directory_entries | /credentials/0/source | 1    |
      | leftover temporary sibling    | credential.directory_entries | /credentials/0/source | 1    |
      | auth.lock inside directory    | credential.directory_entries | /credentials/0/source | 1    |
      | lock is absent                | credential.missing           | /credentials/0/lock   | 3    |
      | lock is a symlink             | credential.symlink           | /credentials/0/lock   | 1    |
      | lock is a FIFO                | credential.regular           | /credentials/0/lock   | 1    |
      | lock has two links            | credential.links             | /credentials/0/lock   | 1    |
      | lock has another owner        | credential.owner             | /credentials/0/lock   | 1    |
      | lock mode is 0644             | credential.permissions       | /credentials/0/lock   | 1    |
      | metadata is unavailable       | credential.unavailable       | /credentials/0/source | 3    |

  @wip @slice-32 @ID-CODEXAUTH-05
  Scenario Outline: The directory grant retains protected-tree and filesystem exclusion
    Given a Codex boundary observation fixture with an independent ext4 source and "<boundary>"
    When the shared Codex source observer examines the independent control then the boundary case
    Then the independent Codex source succeeds and the boundary reports "<rule>" without reading source data or enumerating state records

    Examples:
      | boundary                                | rule                    |
      | auth directory aliases checkout child   | credential.location     |
      | auth.json aliases protected state file  | credential.location     |
      | lock aliases checkout file              | credential.location     |
      | lock aliases protected state child      | credential.location     |
      | protected state lies under source       | credential.location     |
      | checkout lies under source              | credential.location     |
      | covered backing checkout alias          | credential.location     |
      | submount inside auth directory          | credential.unavailable  |
      | source uses tmpfs                       | credential.filesystem   |
      | lock uses nfs                           | credential.filesystem   |
      | final mount observation unavailable     | credential.unavailable  |
      | source-checkout pin differs from policy | doctor.checkout.changed |

  @wip @slice-32 @ID-CODEXAUTH-06 @local-engine-fixture
  Scenario: Terminal review binds the directory and lock identities and shows shared-write exposure
    Given a ready Codex approval fixture with private synthetic sources and no previous approval
    When terminal Codex approval receives yes after displaying its review
    Then the review precedes consent and contains the complete specified Codex disclosure and independently calculated settings
    And generation 1 binds exactly the directory and lock paths identities targets and read-write access but no auth.json identity or bytes
    And approval reads no login data changes no source and creates no worker

  @wip @slice-32 @ID-CODEXAUTH-07 @local-engine-fixture
  Scenario Outline: No fingerprint or redirected approval grants Codex access
    Given a ready Codex approval fixture with an independently seeded matching approval
    When Codex approval is attempted using "<route>"
    Then it refuses "<rule>" with exit 1 and names only bare terminal approval as the fix
    And the Codex approval generation bytes and source metadata remain unchanged

    Examples:
      | route                           | rule                          |
      | terminal matching fingerprint   | approve.terminal-required     |
      | terminal malformed fingerprint  | approve.terminal-required     |
      | redirected matching fingerprint | approve.terminal-required     |
      | redirected bare approve         | approve.confirmation-required |
      | terminal JSON approve           | approve.confirmation-required |

  @wip @slice-32 @ID-CODEXAUTH-08 @local-engine-fixture
  Scenario: Safe auth replacement leaves the grant unchanged
    Given a ready Codex approval fixture with an independently seeded matching approval
    And the host replaces only auth.json by a private temporary sibling and rename
    When ordinary doctor and unchanged terminal Codex approval inspect the request
    Then both report the original independently calculated fingerprint and terminal approval does not prompt or rewrite its record
    And the new auth.json inode is accepted while directory and lock identities remain bound

  @wip @slice-32 @ID-CODEXAUTH-09 @local-engine-fixture
  Scenario Outline: A different directory or lock requires a fresh grant
    Given a ready Codex approval fixture with an independently seeded matching approval
    And the fixture makes the safe grant change "<change>"
    When ordinary doctor reviews the Codex request and public run is invoked
    Then doctor reports changed approval with exactly one complete old-new "/credentials" difference
    And run refuses changed approval before engine access or worker creation and names terminal approval

    Examples:
      | change                              |
      | replace directory at the same path  |
      | repoint source to another directory |
      | replace lock at the same path       |
      | repoint lock to another file        |

  @wip @slice-32 @ID-CODEXAUTH-10 @local-engine-fixture
  Scenario: Consent cannot publish a substituted directory
    Given a ready Codex approval fixture with private synthetic sources and no previous approval
    And the fixture schedules a safe directory replacement after review and before yes
    When terminal Codex approval receives yes after displaying its review
    Then Codex approval reports "approve.changed" with exit 1 and publishes no approval or frozen input
    And no Codex source is repaired and no worker is created

  @wip @slice-32 @ID-CODEXAUTH-11 @local-engine-fixture
  Scenario Outline: Run rechecks shared source safety before creating a worker
    Given a ready Codex run fixture with independently approved directory and lock identities
    And after approval the fixture applies "<defect>" to the shared source
    When public Codex run is invoked with literal synthetic argv
    Then run refuses "<rule>" before any worker create or start and reads no login contents
    And owned resources are cleaned without changing the external Codex sources

    Examples:
      | defect                   | rule                         |
      | symlink auth.json        | credential.symlink           |
      | FIFO auth.json           | credential.regular           |
      | second link to auth.json | credential.links             |
      | foreign auth.json owner  | credential.owner             |
      | extra sibling            | credential.directory_entries |
      | missing auth.json        | credential.missing           |
      | symlink lock             | credential.symlink           |

  @wip @slice-32 @ID-CODEXAUTH-12 @local-engine-fixture
  Scenario Outline: Late source and mount changes prevent start
    Given a ready Codex run fixture with independently approved directory and lock identities
    And the recording engine schedules "<change>" after worker create and before start verification
    When public Codex run is invoked with literal synthetic argv
    Then the Codex run reports a pre-start refusal and the engine journal has one create and zero start calls
    And the exact owned worker and anchor are removed while every external source and sentinel is unchanged by cleanup

    Examples:
      | change                           |
      | replace auth.json with a symlink |
      | replace lock inode               |
      | change auth mount to read-only   |
      | change auth mount source         |
      | add an unapproved parent mount   |

  @wip @slice-32 @ID-CODEXAUTH-13 @local-engine-fixture
  Scenario: Status describes frozen grants even when current sources disappear
    Given an independently seeded stopped Codex session with an unsafe source-check result
    And current policy approval directory and lock have been removed from that private fixture
    When Codex session status is invoked in text and JSON
    Then status succeeds with exactly the frozen Codex grant and recorded unsafe check and the shared-write disclosure
    And status observes no current source contents or policy and changes no protected record

  @wip @slice-32 @ID-CODEXAUTH-14
  Scenario: Help discloses shared login mutation and the interval between checks
    When itos-container runs with "approve --help"
    Then approval help includes every specified Codex sharing disclosure without promising continuous protection
    And it exits with code 0

  # The synthetic writer resolves the fixture-created symlink exactly as T-47's
  # pinned storage_file contract does, writes a new mode-0600 sibling then renames
  # it. Separate acknowledgements gate each direction; no sleeps/racy polling.
  # A single-file bind fails this at replacement or cross-side observation, and
  # copies fail it at the next opposite-side read. Never fall back to truncation.
  @wip @slice-32 @ID-CODEXAUTH-15 @engine
  Scenario: Host and worker observe each other's atomic auth replacements
    Given a native Codex fixture with synthetic auth and a shared empty lock and an independently seeded approval
    When run starts the synthetic writer and the worker then host each replace auth.json by temporary sibling and rename
    Then the worker replacement is observed as new bytes by the host through its fixture symlink
    And the host replacement is observed as new bytes by the still-running worker through its fixture symlink
    And both replacements change the auth.json inode and preserve the approved directory identity
    And run exits zero after confirmed owned cleanup with a safe source check and preserves the latest host auth and lock

  @wip @slice-32 @ID-CODEXAUTH-16 @engine
  Scenario: Host and worker contend on the same auth lock
    Given a native Codex fixture with synthetic auth and a shared empty lock and an independently seeded approval
    When run starts the lock probe and host and worker alternately hold and attempt the pinned advisory auth lock through their respective paths
    Then both contenders report contention while the other holds the lock and acquire it after the explicit release acknowledgement
    And the worker lock is the approved host lock inode and the dedicated auth directory still contains only auth.json
    And run exits zero after confirmed owned cleanup without deleting the lock

  @wip @slice-32 @ID-CODEXAUTH-17 @engine
  Scenario: Shared auth exposes no host home or unrelated credential and stores no login bytes
    Given a native Codex fixture with approved synthetic auth and unapproved host-home sibling and other-project sentinels
    And the fixture marker scanner has raw and base64 positive controls
    When run executes a cooperative synthetic auth reader and boundary probe
    Then the worker reads the approved auth and lock but cannot read any unapproved sentinel or protected host state
    And its inspected mounts have exactly the approved shared directory and lock plus the existing allowed runtime mounts
    And no auth marker appears in provisioning output engine traffic inspect checkout approval intent recovery or status
    And confirmed owned teardown preserves every external source and sentinel

  # The worker is hostile, so poisoning is allowed during its lifetime. The host
  # reader never follows these poisoned targets in a scenario. Independent lstat
  # observations prove the hazard was installed; cleanup must not read/repair it.
  @wip @slice-32 @ID-CODEXAUTH-18 @engine
  Scenario Outline: Stop reports worker-planted unsafe auth after removing the worker
    Given a native Codex fixture with synthetic auth and a shared empty lock and an independently seeded approval
    When the worker installs "<poison>" and the session ends by "<ending>"
    Then the installed poison is observed and the exact owned worker and anchor are removed
    And the controller reports "recovery.credential-source-unsafe" with the underlying "<rule>" and a nonzero exit
    And status retains an unsafe source check and a repeated stop preserves that warning without following or repairing the poison
    And the outside sentinel and host lock are unchanged

    Examples:
      | poison                        | ending          | rule                         |
      | auth.json symlink to sentinel | foreground zero | credential.symlink           |
      | auth.json FIFO                | foreground zero | credential.regular           |
      | auth.json directory           | recovery stop   | credential.regular           |
      | auth.json socket              | cancellation    | credential.regular           |
      | extra hidden sibling          | recovery stop   | credential.directory_entries |

  @wip @slice-32 @ID-CODEXAUTH-19 @local-engine-fixture
  Scenario Outline: Final checks preserve uncertainty and report every unsafe metadata class
    Given an independently seeded owned Codex recovery fixture with "<observation>" at final source inspection
    When stop confirms owned worker removal and performs the final Codex check
    Then owned cleanup completes independently of the Codex check and reports "<diagnostic>" with state "<state>"
    And the persisted Codex check contains the underlying "<rule>" and no contents
    And neither stop nor repeated stop repairs follows deletes or reads the external sources

    Examples:
      | observation                    | diagnostic                             | state   | rule                         |
      | auth.json has two links        | recovery.credential-source-unsafe      | unsafe  | credential.links             |
      | auth.json has another owner    | recovery.credential-source-unsafe      | unsafe  | credential.owner             |
      | auth.json is group-readable    | recovery.credential-source-unsafe      | unsafe  | credential.permissions       |
      | directory has an extra entry   | recovery.credential-source-unsafe      | unsafe  | credential.directory_entries |
      | directory identity changed     | recovery.credential-source-unsafe      | unsafe  | credential.changed           |
      | lock identity changed          | recovery.credential-source-unsafe      | unsafe  | credential.changed           |
      | auth.json is absent            | recovery.credential-source-unavailable | unknown | credential.missing           |
      | source metadata is unavailable | recovery.credential-source-unavailable | unknown | credential.unavailable       |

  # Preserve T-48/lifecycle uncertainty; directory mounts are not staged copies
  # and have nothing to unlink to fence a late start. The first cleanup observation
  # is deliberately unavailable; a later stop must use the SAME protected ID.
  @wip @slice-32 @ID-CODEXAUTH-20 @engine
  Scenario: Lost start acknowledgement retains recovery without revoking or deleting shared auth
    Given a native Codex fixture with synthetic auth and a shared empty lock and an independently seeded approval
    And the engine recorder will lose start acknowledgement after mount acquisition and make the first cleanup observation unavailable
    When run attempts the synthetic writer and a later stop retries its recorded recovery
    Then the acquired mount permits a synthetic auth replacement despite the lost acknowledgement
    And exactly one start is attempted and the initial result retains the original recovery identity and unknown source check without claiming no workload ran
    And the later stop reconciles that original full worker ID and confirms owned teardown and a safe final source check
    And the current shared auth and lock survive every cleanup attempt without copy-back or source repair
