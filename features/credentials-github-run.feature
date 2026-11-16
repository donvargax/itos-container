@phase-1
Feature: Deliver an independently approved GitHub token to one owned run
  Run copies the GitHub grant to its fixed read-only worker file, alone or beside
  Claude's separate grant. The workload can explicitly read it into GH_TOKEN.
  Approval binds paths and kinds; each run owns its copies and their cleanup.

  # Slice-33 spec only. Authorities: charter milestone two step 2, contract
  # AC-02/03/22/23/28/29, the delivery part of AC-31 and AC-36 as revised by q-56;
  # decisions 0025/0027, architecture.md Slices and arc42 Credential run.
  # Read against main 3b0c8f44f21d1feed1ace1731cbe5eec07a7175e,
  # slice-31 3dedaf1c20f71de13f6a93680fc28f49d4cf76eb (credentials-run.feature),
  # slice-30 7751303 (credentials-approval.feature), T-48 b6bcc16 (session seam)
  # and slice-27 8a15b62 (egress.feature). These are reading baselines, not resets.
  # Also read slice-30 b62a252's initial credential_approval_steps_test.go helpers;
  # its request/oracle/observer helpers can be reused, not its five step phrases.
  # Implementation follows slice-31, including slice-30's approval/source rules
  # and T-48's shared delivery. The allowlist runtime row also requires slice-27;
  # the coordinator orders that handoff before activation. Do not fold egress
  # work into this slice. q-67 confirms slice-29's prerequisite is satisfied.
  #
  # Lift slice-31's temporary GitHub gate: version 3 accepts no grants, Claude,
  # GitHub or both, without another schema/profile or another consent for already
  # matching terminal-approved settings. Preserve v1/v2 no-grant behavior and
  # malformed-policy, argv and JSON refusal precedence. Unknown kinds remain
  # parser errors. Both tokens are independent approved grants, never aliases
  # or an implicit bundle. Canonical grants remain sorted by kind, targets follow
  # kind rather than source filename, and diagnostics retain written indices.
  # Adding/removing/repointing a grant changes /credentials; adding the first or
  # removing the last also changes /profile. Reordering changes neither. Valid
  # rotation at the same path between runs changes no fingerprint or generation.
  # Sources come from the frozen grants, not a subsequent mutable policy read.
  # q-52 terminal consent is unchanged; run cannot grant consent itself.
  #
  # Reuse the complete shared metadata observation before and after engine
  # inspection and T-48's final revalidation before copying. All grants must pass
  # before any source data read. Preserve slice-30's policy-pin/root comparison,
  # 4095-byte boundary limits, q-64/69 ext4/btrfs/xfs allowlist, q-57 mount identity,
  # trusted ancestors, private regular host-owned single-link source and size
  # limits, and BOTH selected/fixed state exclusions without enumerating records.
  # Descendant-bind aliases use one link so another rule cannot mask exclusion.
  # An invalid second grant never permits a partial run with the first.
  #
  # Delivery reuses T-48 and slice-31, with no new mechanism or security exception:
  # read retained descriptors through /proc/self/fd/<n>, copy exactly once without
  # trimming, and close pins after pending operations finish on every exit path.
  # Intent precedes exclusive staging under /dev/shm/itos-container-<32 lower hex>;
  # verify tmpfs, host ownership, directory 0700 and regular staged files 0400.
  # Add exactly the approved staged read-only binds, GitHub at
  # /run/itos-container/credentials/github-token and Claude at
  # /run/itos-container/credentials/claude-token. Independently verify sources,
  # targets and flags before start. Never mount an original source, source parent
  # or staging directory. Keep non-root identity, private IPC, existing /tmp and
  # checkout mounts, all hardening, literal argv as main process and no restart.
  # No token bytes enter controller-set Env/argv, Engine API bodies, inspect,
  # labels, approval/snapshot/intent/recovery, diagnostics or checkout/state files.
  # Source-copy failure keeps run.credential-refused exit 1; unavailable staging
  # keeps run.credential-unavailable exit 3. Preserve earlier located metadata
  # errors and existing isolation/budget/cleanup classifications. Never show raw
  # OS/API errors or contents. A failure after partial staging cleans every owned
  # file, or retains original protected authority and the exact stop retry.
  #
  # The GitHub interface is a FILE, not automatic GH_TOKEN/GITHUB_TOKEN injection.
  # The person's explicit argv may read it and export GH_TOKEN inside the worker.
  # A synthetic gh probe checks that environment interface without contacting
  # GitHub or invoking an actual client. Direct Git does not acquire credentials
  # merely from GH_TOKEN: configuring a credential helper, actual pushes, SSH,
  # scope/expiry validation, login, refresh, accounts and harness setup are outside
  # this delivery slice. No real token, account or model is used; ADR 0025 question
  # 5 still bars live credentials. Codex's open decision does not block this kind.
  #
  # After every start wait and applicable detector, attempt unlink of all staged
  # paths. An acquired mount remains readable. Lost acknowledgement or unlink
  # never proves no execution or completed cleanup. Do not retry start; reconcile
  # the original full worker ID under existing operation/startup/teardown budgets.
  # Keep observed exit precedence and explicit unknown outcomes. Completion needs
  # confirmed worker AND staging absence plus all existing owned resources.
  # Status/stop use original protected intent and frozen grants without mutable
  # policy, active approval or source reads; no adoption of substituted objects.
  # Status retains slice-31's credential_staging present/absent/unknown and
  # slice-30's sorted credentials array/lines; a missing staging path alone never
  # completes a worker. Unsafe or unavailable cleanup retains the exact retry.
  #
  # q-56 replaces ADR 0025 question 6's blocking log rule. Provisioning must not
  # leak known bytes. Hostile workload output/traffic remains unredacted and can
  # put them in refused hostnames, proxy stdout, Docker logs and retained refusal
  # logs. Keep the disclosure and verbatim hostname logging. Platform scrubbing
  # and the adversarial hostname-marker probe belong to p1-platform-log-redaction;
  # neither blocks this slice. Claim cooperative provisioning absence only, not
  # general AC-36 closure. Reconcile stale ADR/arc42 wording at implementation if
  # slice-31 has not already done so; do not quietly weaken another scenario.
  #
  # Architecture/library choice: extend existing run/status/stop vertical slices
  # and shared readiness/approval/session engines. Only cmd binds adapters. Reuse
  # T-48's schema-4 intent, CheckedSources, CredentialFiles, entropy, SDK mount
  # verification and shared fake/adapter contracts, plus slice-31's raw source
  # refusal contracts. Domain/porttest remain I/O-free; no slice imports another.
  # Existing Go stdlib, Moby client v0.6.2/API v1.56.1, x/sys v0.48.0 and mountinfo
  # v0.7.2 already provide the mechanism; add no dependency or credential parser.
  # Preserve licenses, OS-as-value rules and all no-grant regression checks.
  #
  # Fixture/step handoff: ALL phrases below are NEW. Generalize slice-31's future
  # credential run steps rather than duplicating delivery helpers. Existing reads:
  # credentials_native_steps_test.go, credentials_observation_steps_test.go,
  # credentials_observer_test.go, run_preflight_steps_test.go and cmd's run/session
  # assembly; use approval's independent-record/terminal helpers and lifecycle's
  # recorder/barriers. Slice-29's credentials-only fixture lacks full readiness;
  # compose valid Git, manifest, host identity and independent canonical records.
  # Every Given writes inputs, seeds records or schedules faults; none runs a
  # product command or asserts readiness. Use private fixture homes/state only.
  # "matching approval" seeds terminal-consent-equivalent generation 4 records;
  # expected canonical bytes never come from the product. "both" writes GitHub
  # first unless the row says otherwise, but canonical order is Claude then GitHub.
  # "swapped sources" approves each kind at the other's distinct safe source path.
  # "repointed github" changes only its source path to a second distinct safe
  # GitHub fixture file. "missing" seeds no record; "checkout-only" writes one
  # only inside checkout; other prior values seed the protected record for that
  # version-3 grant set. Current requests are written to itos-container.yaml
  # beside the valid manifest; all sources remain outside checkout/control state.
  # Source basename and content have no meaning to the controller.
  #
  # Local cases exercise the public binary with a recording Engine API, or a
  # test-only cmd assembly of the SAME handler/preflight/runner/classifier when
  # raw UID/mount/fault observations are needed. No production override, canned
  # readiness verdict or local engine/mount operation. Controls and faults share
  # that route. Raw fixture filesystems are ext4; never call native tmpfs ext4.
  # Real native sources in CI must independently satisfy the filesystem rule.
  # Source observers distinguish fixture reads from product data opens/reads.
  # 04 changes metadata at an image-response barrier; 05 renames the pinned source
  # to a safe single-link sibling AFTER final revalidation, BEFORE data reopen,
  # replacing its old name with different valid bytes. 06 schedules GitHub copy
  # faults after Claude's copy; its Claude write fault needs no earlier copy.
  # Inspect faults change only the pre-start verification reply, leaving real
  # ownership observations intact for cleanup.
  # When retains early refusal, missing barrier/report and every exit as data;
  # the Then assertion must expose red, never a setup check or missing-result panic.
  #
  # Each fixture prepares distinct GitHub and Claude sources even when only one
  # is requested, so ungranted-read and cross-kind marker checks are meaningful.
  # Each source holds a distinct fresh "itos-synthetic-credential-" plus 32 random
  # lowercase hex digits, optionally padded/final-newline terminated. Only harness
  # and cooperative worker compute SHA-256; product provisioning never hashes or
  # compares contents. Probe reports carry nonce, kind, path, hash, UID/mode and
  # operation outcomes, never bytes. No report means failure, not isolation proof.
  # In 07 direct reads prove byte-for-byte newline preservation. A separate child
  # launched by the explicit shell argv exports GH_TOKEN from that same file and
  # execs the synthetic gh probe; its hash follows shell trailing-newline removal.
  # Before that export the worker reports GH_TOKEN and GITHUB_TOKEN absent. No
  # marker is supplied in argv, environment from the host, or a probe's executable.
  #
  # "all provisioning surfaces" scans each marker raw and base64 at all three
  # offsets, including replacement/rotation markers, with a positive control for
  # EVERY detector: full Engine API requests/responses including errors, worker
  # inspect, all checkout/protected-state files,
  # approval/snapshots/intents/recovery, doctor/approve/run/stop diagnostics and
  # plain/JSON status. 07 invokes doctor plain/JSON and matching bare terminal
  # approve without an answer before run, then status plain/JSON while the worker
  # waits. Require success, no prompt and unchanged approval, not empty transcripts.
  # Its allowlist row additionally requires controlled TLS and an ordinary refused
  # hostname containing no marker, present in proxy stdout, Docker's proxy log and
  # the retained refusal log; scan all three. No real endpoint or token is used.
  # Exclude only fixture source files, authorized staged bytes, detector controls
  # and fixture-only input journals; never exclude real API captures/product state.
  # Diagnostics name a leaking surface without dumping markers or complete records.
  #
  # @engine cases run ONLY in guarded Linux amd64 native Docker CI, using cached
  # images and in-worker probes. Never pull/build, use a real credential or give
  # agents the socket. Product cleanup is audited before bounded identity-checked
  # harness cleanup, which cannot satisfy a product assertion. Never write a token
  # into the checkout, even as a probe. 10 releases the hash read AFTER unlink and
  # before reconciliation is released, so delayed acquisition evidence is decisive.
  # 11 kills at a deterministic recorder barrier before unlink, not after a sleep.
  #
  # Supersession belongs to the implementing feat commit, not this draft:
  # replace CREDRUN-07's temporary GitHub gate with 01/02/03/06 here and replace
  # CREDRUN-13's "no GitHub delivery" help assertion with 12, keeping Claude's
  # help and all limits. Replace any remaining supported-version-3 run gate rows
  # in CREDENTIALS-13/14 and CREDAPPROVAL-17, preserving invalid-policy precedence.
  # Carry CREDAPPROVAL-18/19's grant comparisons into public run for GitHub too.
  # Remove the temporary GitHub delivery warning, preserving consent/exposure
  # disclosures. Update CLI/CONFIG/security/help and architecture implementation
  # status. No public exec command, new permission, or full network-Git claim.
  # Neighbors: credentials-policy/approval/run, approval, run-preflight, offline,
  # lifecycle, recovery/control-state and egress. Run the shared source, staging,
  # mount, multi-kind, uncertain-start and recovery contracts after implementation.
  # Commit steps red first with @wip retained, obtain assertion-level red with it
  # removed for the run, then build. Engine red/green is CI-only; no local mutation
  # or whole-features race run. The first Then of each case is expected to fail:
  # 01..11 encounter today's v3 gate (slice-31 retains it for GitHub); 12 lacks
  # GitHub delivery help. This is predicted failure, not executed red evidence.

  # Admission and kind-to-source mapping, including canonical order independence.
  @wip @slice-33 @ID-GITHUBRUN-01 @local-engine-fixture @linux
  Scenario Outline: Matching GitHub grants reach owned execution without fresh consent
    Given a local GitHub run fixture with matching approval and request "<request>"
    When the GitHub fixture invokes public run with its literal probe argv
    Then the GitHub run completes owned execution with exit 0 and profile "offline-credentials-v1"
    And its verified create mounts deliver exactly "<mapping>" with unchanged worker hardening
    And its frozen grants fingerprint and generation 4 match the independent approval without consent or approval rewrite
    And each granted source is copied once through its checked descriptor with no ungranted source read
    And staged files are unlinked after startup and all owned resources are removed after exit
    And all GitHub provisioning surfaces contain no synthetic marker

    Examples:
      | request                           | mapping                                       |
      | github                            | GitHub source at GitHub target only           |
      | both written github then claude    | each source at its own kind's target          |
      | both written claude then github    | each source at its own kind's target          |
      | both with approved swapped sources | each approved source at its grant kind target |

  @wip @slice-33 @ID-GITHUBRUN-02 @local-engine-fixture @linux
  Scenario Outline: GitHub delivery requires the exact independently approved grant set
    Given a local GitHub run fixture with prior approval "<prior>" and current request "<request>"
    When the GitHub fixture invokes public run with its literal probe argv
    Then the GitHub run refuses with "<rule>" exit 1 and exactly changed pointers "<pointers>"
    And it names terminal review with "itos container approve" without requesting consent
    And it reads no source data contacts no engine and publishes no snapshot intent session or staging
    And approval checkout and original sources remain unchanged

    Examples:
      | prior              | request          | rule                 | pointers              |
      | missing            | github           | run.approval-missing |                       |
      | checkout-only      | github           | run.approval-missing |                       |
      | version three none | github           | run.approval-changed | /credentials /profile |
      | claude             | github           | run.approval-changed | /credentials          |
      | claude             | both             | run.approval-changed | /credentials          |
      | github             | both             | run.approval-changed | /credentials          |
      | both               | github           | run.approval-changed | /credentials          |
      | github             | repointed github | run.approval-changed | /credentials          |

  # Every request includes GitHub, and both grants must pass before either is read.
  @wip @slice-33 @ID-GITHUBRUN-03 @local-engine-fixture @linux
  Scenario Outline: An unsafe source prevents delivery of every requested grant
    Given a local GitHub run fixture with matching approval and request "<request>"
    And source "<kind>" now has raw credential observation "<defect>"
    When the GitHub fixture invokes public run with its literal probe argv
    Then the GitHub run reports "<rule>" at "<pointer>" with exit <exit>
    And neither source data is read and no credential staging or engine resource is created
    And approval checkout and sources remain unchanged apart from the arranged defect

    Examples:
      | request                        | kind         | defect                               | rule                   | pointer               | exit |
      | github                         | github-token | missing source                       | credential.missing     | /credentials/0/source | 3    |
      | github                         | github-token | group-readable source                | credential.permissions | /credentials/0/source | 1    |
      | github                         | github-token | foreign source owner                 | credential.owner       | /credentials/0/source | 1    |
      | github                         | github-token | symlink in source path               | credential.symlink     | /credentials/0/source | 1    |
      | github                         | github-token | second hard link outside boundaries  | credential.links       | /credentials/0/source | 1    |
      | both written claude then github | github-token | checkout descendant bind alias       | credential.location    | /credentials/1/source | 1    |
      | both written claude then github | github-token | selected state descendant bind alias | credential.location    | /credentials/1/source | 1    |
      | both written claude then github | github-token | fixed state descendant bind alias    | credential.location    | /credentials/1/source | 1    |
      | both written github then claude | claude-token | group-readable source                | credential.permissions | /credentials/1/source | 1    |
      | github                         | github-token | source filesystem is tmpfs           | credential.filesystem  | /credentials/0/source | 3    |
      | github                         | github-token | final mount evidence unavailable     | credential.unavailable | /credentials/0/source | 3    |

  @wip @slice-33 @ID-GITHUBRUN-04 @local-engine-fixture @linux
  Scenario Outline: GitHub source safety is rechecked after engine inspection
    Given a local GitHub run fixture with matching approval and request "both written claude then github"
    And its GitHub source will change to "<defect>" at the image inspection barrier
    When the GitHub fixture invokes public run with its literal probe argv
    Then the GitHub run reports "<rule>" at "/credentials/1/source" with exit 1
    And both sources were observed before the barrier and GitHub was rechecked after it
    And neither source data is read and no credential staging or engine resource is created
    And approval and sources remain unchanged apart from the arranged change

    Examples:
      | defect                               | rule                   |
      | group-readable source                | credential.permissions |
      | checkout descendant bind alias       | credential.location    |
      | selected state descendant bind alias | credential.location    |
      | fixed state descendant bind alias    | credential.location    |

  @wip @slice-33 @ID-GITHUBRUN-05 @local-engine-fixture @linux
  Scenario: GitHub copying uses the checked inode without confusing the two grants
    Given a local GitHub run fixture with matching approval and request "both"
    And the GitHub source name will be replaced after final validation while its checked inode remains at a safe sibling
    When the GitHub fixture invokes public run with its literal probe argv
    Then the staged GitHub file contains exactly the checked GitHub bytes including final newlines and not the replacement or Claude bytes
    And the staged Claude file contains exactly its own unchanged source bytes
    And source data is opened only through retained descriptors and all pins are released after pending operations finish
    And the GitHub run completes owned execution and cleanup without modifying either original source or approval
    And all GitHub provisioning surfaces contain no synthetic marker

  # A failed second copy must clean the first; an inspected bind fault must be
  # detected before start even when another credential mount is valid.
  @wip @slice-33 @ID-GITHUBRUN-06 @local-engine-fixture @linux
  Scenario Outline: Partial copying or an unverified GitHub mount cannot launch a worker
    Given a local GitHub run fixture with matching approval and request "both"
    And credential preparation will encounter GitHub fixture fault "<fault>"
    When the GitHub fixture invokes public run with its literal probe argv
    Then the GitHub run refuses with "<rule>" and exit <exit> at the scheduled preparation boundary
    And no worker start is attempted and every created staged file and owned resource enters cleanup
    And cleanup confirms complete absence or retains original recovery and the exact stop retry without adopting replacements
    And all source pins are released after pending operations finish and original sources and approval remain unchanged apart from fixture edits
    And all GitHub provisioning surfaces contain no synthetic marker or raw error text

    Examples:
      | fault                                               | rule                       | exit |
      | GitHub data open denied after Claude copy            | run.credential-refused     | 1    |
      | GitHub short read after Claude copy                  | run.credential-refused     | 1    |
      | GitHub size changes during read after Claude copy    | run.credential-refused     | 1    |
      | GitHub staged write fails after Claude copy          | run.credential-unavailable | 3    |
      | Claude staged write fails                           | run.credential-unavailable | 3    |
      | inspected GitHub bind is missing                     | run.isolation-unverified   | 1    |
      | inspected GitHub bind is writable                    | run.isolation-unverified   | 1    |
      | inspected GitHub bind names the original source      | run.isolation-unverified   | 1    |
      | inspected GitHub bind targets the Claude path        | run.isolation-unverified   | 1    |
      | inspected GitHub bind uses the Claude staged source  | run.isolation-unverified   | 1    |

  # Delivery and GH_TOKEN compatibility are proven without any GitHub connection.
  @wip @slice-33 @ID-GITHUBRUN-07 @engine @linux
  Scenario Outline: The workload explicitly reads its private GitHub file into GH_TOKEN
    Given a CI GitHub checkout requesting "<request>" with independently matching "<network>" approval
    And independent marker detectors and cooperative file and synthetic gh probes are prepared
    When doctor and matching terminal approve inspect the fixture then run executes the explicit probe argv through active status and exit <exit>
    Then the worker reports the exact GitHub source hash from "/run/itos-container/credentials/github-token" and the synthetic gh probe reports the expected GH_TOKEN hash
    And before the workload export GH_TOKEN and GITHUB_TOKEN are absent and controller-set Env and argv contain no token
    And each approved kind has its own read-only mode-0400 regular file owned by the non-root host UID with all prior hardening
    And ungranted credential targets original sources source siblings and host staging directories cannot be opened and writes to granted files fail
    And the native worker's network evidence is "<evidence>"
    And doctor approve and active status succeed with exactly the frozen grants and no new consent or approval rewrite
    And all required provisioning surfaces and applicable nonempty refusal logs contain neither marker nor any base64-offset encoding
    And staged paths disappear after startup while mounted copies remain readable until the worker exits
    And run returns the observed workload exit <exit> after confirmed worker staging and all other owned resource removal
    And sources checkout approval and unrelated resources remain unchanged

    Examples:
      | request | network   | exit | evidence                                                                  |
      | github  | offline   | 0    | network none and no proxy                                                  |
      | both    | offline   | 17   | network none and no proxy                                                  |
      | both    | allowlist | 0    | controlled TLS succeeds and an ordinary denied hostname reaches every log |

  @wip @slice-33 @ID-GITHUBRUN-08 @engine @linux
  Scenario Outline: Rotating GitHub changes the next run without changing a live copy or Claude
    Given a CI GitHub checkout requesting "both" with independently matching "offline" approval
    And a distinct synthetic GitHub value is reserved for rotation by "<rotation>"
    When run starts the waiting two-file probe and the fixture rotates GitHub before releasing its second read
    Then both live reads report the original GitHub hash and unchanged Claude hash
    When that worker exits and run starts the fresh two-file probe without another approval
    Then the fresh worker reports the rotated GitHub hash and unchanged Claude hash from their respective fixed paths
    And both runs retain the approved fingerprint and generation without consent and leave the original sources at their fixture-selected values
    And both runs confirm owned cleanup with no marker on any provisioning surface

    Examples:
      | rotation                            |
      | rewriting the private file in place |
      | replacing it with a valid new inode |

  @wip @slice-33 @ID-GITHUBRUN-09 @engine @linux
  Scenario: GitHub grants remain separate across projects and after grant removal
    Given independent CI projects A and B have distinct GitHub sources and matching GitHub-only approvals
    And independent CI project C has a Claude-only approval and no GitHub grant
    When run starts all three waiting probes and stop ends project A while B and C remain active
    Then A and B report only their own GitHub hash and C reports only its Claude hash
    And no worker can open another project's source or staged path or its own ungranted credential target
    And stopping A removes only A's resources while B and C can still read their own copies
    When the remaining workers exit and A's fixture independently seeds a matching no-grant approval for another run
    Then A's new worker executes with no source reads staging or credential mounts and cannot read either token target
    And every run confirms its own cleanup without changing original sources or leaking a marker through provisioning

  @wip @slice-33 @ID-GITHUBRUN-10 @engine @linux
  Scenario Outline: Lost start acknowledgement does not erase an acquired GitHub mount
    Given a CI GitHub checkout requesting "both" with independently matching "offline" approval
    And the recorder will lose start acknowledgement after both mounts are acquired and release the probe after staged unlink
    And owned worker removal will be "<removal>" until the explicit recovery retry
    When run attempts startup and the recorder releases the delayed two-file probe after unlink
    Then the acquired worker mounts report both exact source hashes despite absent staging paths and lost acknowledgement
    And only one start occurs and reconciliation uses the original full owned worker ID
    And run keeps the observed workload outcome or explicit uncertainty and reports "<cleanup>" within existing budgets
    When the fixture restores removal and stop retries the exact original session if recovery remains
    Then worker staging and all other owned resources are confirmed absent without another start or source read
    And all GitHub provisioning surfaces contain no synthetic marker

    Examples:
      | removal     | cleanup                                                     |
      | available   | complete only after worker and staging absence are confirmed |
      | unavailable | incomplete with original recovery and the exact stop retry   |

  @wip @slice-33 @ID-GITHUBRUN-11 @engine @linux
  Scenario Outline: Fresh controllers recover both staged grants from protected authority alone
    Given a CI GitHub checkout requesting "both" with independently matching "offline" approval
    And its controller will be killed at "<barrier>" before staged unlink
    When run is attempted and the fixture kills the controller then removes both sources current policy and active approval
    Then protected evidence identifies both staged kinds and every acquired owned resource without token bytes
    When fresh product processes inspect plain and JSON status and stop the original session twice
    Then status reports the two sorted frozen grants and credential staging "present" without claiming completed cleanup
    And stop confirms removal of both staged copies the directory worker and every other owned resource using only original authority
    And status and stop never read source data mutable policy or active approval and the second stop is idempotent
    And no unobserved success is invented and all GitHub provisioning surfaces contain no synthetic marker

    Examples:
      | barrier                                            |
      | after both staged copies before worker create      |
      | after accepted start before acknowledgement unlink |

  @wip @slice-33 @ID-GITHUBRUN-12
  Scenario: Run help explains GitHub file delivery without promising automatic authentication
    When the product run help is requested for GitHub delivery
    Then help names the fixed GitHub token file and its separate matching terminal-approved grant
    And help describes its read-only per-run copy and explicit workload export to GH_TOKEN without automatic environment injection
    And help explains that rotation affects subsequent runs and token provider permissions are not narrowed or checked
    And help discloses unredacted workload output and credential-bearing refused hostnames in persisted logs
    And help preserves Claude delivery and the late-checkout-socket limitation without claiming Codex delivery automatic Git credential setup or network pushes
    And help returns exit 0 without source state or engine access
