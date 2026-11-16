@phase-1
Feature: Claude starts with onboarding completed in its private persistent home
  A person can select nonsecret Claude initialization for an approved harness home.
  The first workload sees completed onboarding without inheriting the host's state.
  Later runs preserve Claude's own state and another checkout receives a separate home.

  # Slice-36 specification only. EVERY step phrase below is NEW; this draft adds
  # no bindings, implementation or observed red/green evidence.
  # Authorities: charter milestone two step 3; architecture.md Slices; slice-34's
  # harness-home.feature and decision 0030-persistent-harness-home.md; decision
  # 0025-credentials-passed-in.md; run/offline/approval/egress/lifecycle scenarios.
  # Read against main 2897e1024a57632e22e181383488b4e52af1cebb,
  # slice-34 abc2174916395f4ca67edd70c5e96a93324701d2,
  # slice-35 f20c946d9454a78f5bd0a2fb259a5c042d3549df and
  # slice-31 3dedaf1c20f71de13f6a93680fc28f49d4cf76eb.
  # These are dependency inputs, not claims that their unfinished behavior works.
  # Consume slice-34 first. CLAUDEHOME-10 also uses the approved egress proxy.
  # Token delivery stays in slice-31; none of these fixtures requests a grant.
  #
  # Selection and approval, decided here:
  # - Extend the closed home object with optional harness: claude, a literal
  #   case-sensitive string, alongside storage: volume in supported versions 1/2/3.
  #   This shares slice-35's home.harness selector; its codex value is a sibling
  #   implementation, not an alias or a value to reject once that slice lands.
  #   No top-level harness key, argv sniffing, automatic installation, manifest
  #   hook, caller-selected state path or caller-supplied seed content is added.
  # - File-only doctor retains harness: claude in normalized home. Null and
  #   non-string values use policy.type at /home/harness; unsupported strings,
  #   including empty and "Claude", use policy.home_harness there (exit 2).
  #   Missing storage remains policy.required at /home/storage. Duplicate and
  #   unknown fields keep their existing rules. No other policy-version gate lifts.
  # - Add exactly harness:{kind:"claude",seed:"claude-v1",target:"/home/itos/.claude"}
  #   to slice-34's effective home object. Retain its profile name, owner, storage,
  #   target and lifetime. The seed revision binds the fixed file/environment
  #   contract below; a future revision requires approval. No-home and plain-home
  #   canonical bytes remain unchanged. No home contents enter the fingerprint.
  # - Doctor and approve disclose the harness/revision, both fixed paths below,
  #   nonessential-traffic default, create-if-missing behavior, retention and
  #   refusal of incompatible retained state. Adding/removing/changing the harness
  #   needs approval, with /home/harness in sorted differences. Existing consent
  #   rules still apply. Review never allocates, seeds, runs Claude or reads a token.
  # - Freeze the selector with the existing snapshot. Selection changes neither
  #   home scope nor generation. Removing it leaves files intact and stops only
  #   Claude seeding/environment overrides; selecting it again reuses that home.
  #   Image changes and reapproval do not reset state. A plain home remains usable
  #   without Claude validation; only separate consented home management deletes it.
  #
  # claude-v1 is deliberately minimal nonsecret initialization:
  # - Set CLAUDE_CONFIG_DIR=/home/itos/.claude and
  #   CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1 in the worker's actual environment,
  #   overriding conflicting image defaults. Keep HOME/XDG/TMPDIR from slice-34.
  #   Manifest CLAUDE_ names remain reserved. Selection applies to ANY literal argv;
  #   it does not wrap argv, append flags, export a token or launch Claude for it.
  # - Create a missing /home/itos/.claude directory as approved UID:GID, mode 0700.
  #   Create a missing /home/itos/.claude/.claude.json as that owner, mode 0600,
  #   containing exactly the JSON object {"hasCompletedOnboarding":true}.
  #   JSON whitespace/key ordering is not prescribed for this new file.
  #   With CLAUDE_CONFIG_DIR set, .claude.json is INSIDE that directory, not beside
  #   it at /home/itos/.claude.json. Never copy the host's or image's Claude files.
  # - No settings.json, theme, trust decision, accepted permission bypass, account,
  #   token, credential file, MCP server, plugin, model or provider is generated.
  #   Directory trust and tool permissions are separate from initial onboarding.
  #   The caller's literal workload and slice-31 own any token-file consumption.
  #   Missing Claude keeps ordinary workload exit semantics, not auto-installation.
  # - These are startup defaults, not another isolation boundary. Explicit Claude
  #   arguments, project/user/managed settings and later worker edits can change
  #   harness behavior. The approved container and proxy remain the boundary.
  #   No promise is made to silence user-configured hooks, MCPs, plugins or services.
  #
  # Compatibility baseline and primary evidence, inspected without running Claude:
  # https://code.claude.com/docs/en/env-vars
  # documents the config directory and nonessential-traffic switch.
  # https://code.claude.com/docs/en/settings
  # distinguishes Claude's mutable .claude.json from settings.json.
  # https://github.com/anthropics/claude-code/releases/tag/v2.1.325
  # The published @anthropic-ai/claude-code-linux-x64 2.1.325 package resolves
  # .claude.json under CLAUDE_CONFIG_DIR, gates initial onboarding on the true
  # hasCompletedOnboarding flag, and can prefer legacy .config.json when present.
  # Its source inspection is a design input; CLAUDEHOME-10 must prove consumption.
  # Pin that CI-only package, fetched from the official npm registry, to integrity
  # sha512-BqZzMvP/zgLLL34S+7OzbG+cyoFp7gwWHU5koVwsyMlTAz+0zZAmbY+UkzvtN2RXxp6AcMenbYIjQJxoXfUY3w==
  # and bind the prepared fixture image by digest. Keep its license/notices; it is
  # not relicensed under this repository's AGPL. No floating latest installation
  # or production version-discovery execution. Other versions gain no support claim.
  #
  # Retention and path decisions:
  # - On EVERY selected run inspect the owned home and fixed Claude paths before
  #   start. An existing directory must be a real directory with the approved
  #   owner and 0700 mode; an existing state file must be regular, owned by that
  #   UID:GID and mode 0600. Refuse symlinks even within the home, wrong kinds,
  #   owners and modes before mutation. Never follow, chmod/chown or repair them.
  # - Require /home/itos/.claude/.config.json to be absent. Any entry there refuses
  #   without reading its contents or following it: the pinned Claude can prefer
  #   that legacy file over the verified state. No automatic migration or deletion.
  # - A present state file must be a complete UTF-8 JSON object with exactly one
  #   hasCompletedOnboarding member whose value is boolean true. Reject duplicate
  #   object keys at any depth, trailing data, non-objects, invalid/truncated JSON,
  #   missing/false/non-boolean onboarding, more than 32 nested containers, and
  #   file content over 1 MiB. Unknown members are allowed and never interpreted.
  # - Preserve a valid present file BYTE FOR BYTE, including all unknown members,
  #   and preserve its UID/GID, mode and mtime. Never rewrite it even when minimal.
  #   This avoids discarding Claude's per-project/account/preferences state and
  #   makes read-only reuse safe even if a regular file has another hard link.
  #   Do not promise an Engine link-count observation that its API cannot supply.
  # - Missing state in an existing safe directory is seeded as above, preserving
  #   other files. Incomplete/corrupt state is refused, not reset to an empty object
  #   or treated as absent, even after a failed prior copy. The diagnostic names
  #   the fixed path and retained-state problem; it does not initiate reset/consent.
  #   A person can preserve/repair their state or use slice-34's explicit reset.
  # - Leave every sibling alone, including settings.json, projects, history,
  #   caches, other harnesses and .credentials.json. Never open a credential file,
  #   resolve its links, copy it into the seed, or serialize state contents into
  #   host snapshots, recovery, status or diagnostics. Only the fixed state JSON
  #   is read; no recursive home inventory. Normal parent-directory timestamp
  #   changes from creating a missing entry are permitted; atime is not an oracle.
  #
  # Sequencing, errors and recovery:
  # - Reuse slice-34's bounded Engine archive seam on the unstarted owned worker.
  #   Durable worker/home ownership and the exclusive home claim precede any seed
  #   read/write. Validate all fixed paths, including the legacy name, before
  #   creating missing entries. No host volume traversal, shell, root helper,
  #   container exec, image entrypoint or checkout hook performs initialization.
  # - Inspect directory kind/mode and its first archive root header for UID/GID,
  #   within 64 KiB including extended headers, then close; never read children.
  #   Bound each state-file archive to 1 MiB content plus 64 KiB framing/headers.
  #   Accept only that regular entry, rejecting extra entries, invalid headers and
  #   truncated streams. Never extract to the host or log an archive/error body.
  # - Independently read back newly generated state and verify its exact object,
  #   metadata and actual environment, not only copy arguments or an HTTP success.
  #   Verify present state as well, and recheck the seed boundary after attach
  #   before start alongside existing ownership/home/isolation checks. Neither a
  #   home success marker nor a previous run's inspection authorizes skipping it.
  # - Unsafe/incompatible retained paths or JSON use run.home-refused (exit 1).
  #   Unavailable reads/copy, malformed Engine archive and unconfirmed writes use
  #   run.home-unavailable (75). Confirmed wrong post-publication/final bytes,
  #   metadata or environment use run.isolation-unverified (1). Call/startup expiry
  #   retains run.call-timeout/run.startup-timeout (75), startup taking precedence
  #   when both expire. Preserve existing ownership-refusal precedence.
  # - A failed/partial/lost copy reply never allows start or an in-invocation
  #   retry. There is no atomic-file-publication claim for Engine archive copy.
  #   Cleanup retains even a partial home and confirms owned session absence or
  #   retains the original exact stop retry. No unobserved workload exit is invented.
  #   After confirmed worker absence a fresh run may seed an absent file or reuse
  #   a complete valid file; a partial/corrupt file still refuses without mutation.
  #   Status/stop never seed, repair, remove home contents or start a worker.
  # - Share existing 10 s call, 120 s aggregate startup and separate 30 s cleanup
  #   budgets. No retry/reconnect resets them. Cancellation/expiry fences further
  #   start/mutation; no new workload timeout. Home claims still fence retirement
  #   and a second worker across selected state roots, including controller death.
  #
  # Architecture and implementation handoff:
  # Extend internal/engine/policy for selection and internal/engine/approval for
  # canonical/frozen settings and disclosure. Extend internal/engine/session's
  # domain/port and named Docker adapter for seed sequencing and bounded archive
  # I/O; keep strict JSON/archive processing in an adapter, not a domain import.
  # Reuse its disk ownership/claims/recovery and internal/engine/budget. Integrate
  # through the existing run/doctor/approve slices and status/stop recovery; each
  # keeps its command/domain/port/adapter layout. Thin handlers, only cmd assembly,
  # no slice imports another, and identical contracts for I/O-free fakes/adapters.
  # Reuse pinned Moby client v0.6.2/API v1.56.1, Go 1.27.4 encoding/json and
  # archive/tar; no new production library/service or handwritten JSON parser.
  # The native in-worker PTY fixture can reuse creack/pty v1.1.24. No dependency
  # allowlist change is selected. Update CLI/CONFIG/schema and arc42's home runtime
  # and building-block description with implementation; this refines decision
  # 0030's explicitly delegated harness seeding, not its storage/claims decision.
  #
  # Evidence handoff:
  # - Givens create only private Git checkouts, literal requests, independently
  #   encoded approval/home records, raw metadata, fault barriers and synthetic
  #   sentinels. Isolate account home as well as HOME/XDG/engine selection. They
  #   never invoke product approve/run/status/stop or assert product success.
  #   Whens collect early refusal and missing probe as results for Then; no setup
  #   may depend on this not-yet-implemented harness selector already working.
  # - Both local backends means the command test assembly with memory ports and
  #   real disk/Moby against a recording Unix HTTP fixture. Read slice-34's
  #   harness_home_policy_steps_test.go and harness_home_native_*_test.go, plus
  #   offline_launch_steps_test.go and session_harness_test.go. Assertions use
  #   public CLI results, protected artifacts and independent API/file observations,
  #   never product functions, production-generated expectations or source checks.
  # - Each refusal/fault row includes a successful CLAUDE home control, so blanket
  #   rejection cannot pass. Retained files are seeded independently, not through
  #   a successful product run hidden in Given. Observe each invocation separately.
  # - Add bindings alone in a red test commit with @wip retained. Non-engine red
  #   is local; ALL @engine red/green is CI-only, with no agent Docker socket.
  #   Native assertions precede fixture teardown; teardown only reclaims tracked
  #   disposable resources after product cleanup has been independently observed.
  # - This draft owns only this feature file. Before its CI tests/red dispatch,
  #   assign it to one engine shard and update inventory pins: TestEngineShards
  #   inventories @wip engine files too, so waiting until activation is too late.
  #   Add a smoke entry when a scenario becomes live. No local engine, full-features
  #   race run, generated coverage or mutation evidence belongs to this draft.
  # - Neighbours: HOME policy/approval/runtime/recovery and retained-state cases,
  #   CODEXHOME selection once merged, POLICY/MANIFEST reserved-variable checks,
  #   APPROVAL/RUN frozen inputs and literal argv, OFFLINE/ANCHORS/LIFECYCLE/RECOVERY,
  #   EGRESS, and CREDRUN's explicit token delivery. None is weakened or retagged.

  # Policy and approval. The positive Claude case fails before malformed controls.
  @wip @slice-36 @ID-CLAUDEHOME-01
  Scenario Outline: Claude initialization is an explicit closed home selection
    Given a Claude home policy fixture for version <version> with omitted plain Claude and malformed harness requests
    When file-only doctor reads each Claude home policy request
    Then only the Claude selection adds harness claude to normalized home and omitted and plain home outputs are unchanged
    And malformed Claude requests have the specified rules subjects and exits without state engine or harness access

    Examples:
      | version |
      | 1       |
      | 2       |
      | 3       |

  # Malformed requests include null/list/boolean, empty/"Claude"/"other" strings,
  # missing storage, duplicate harness, and source/config/target keys under home.
  @wip @slice-36 @ID-CLAUDEHOME-02 @local-engine-fixture @linux
  Scenario: Approval binds Claude initialization without resetting retained state
    Given Claude review fixtures with independently recorded plain and Claude approvals retained sentinels and isolated consent terminals
    When doctor and consenting approve review Claude selection and run attempts changed matching disabled and re-enabled selections
    Then review and approval bind the exact claude-v1 home settings with their paths retention refusal and traffic disclosures and a changed fingerprint
    And unapproved additions and removals report changed approval at /home/harness before mutation while matching controls start
    And review creates no home or seed and disabling then re-enabling preserves the same home generation and retained file bytes

  # The selector, not argv[0], controls seeding. Include conflicting image
  # CLAUDE_CONFIG_DIR and an empty nonessential-traffic switch in the fixture.
  @wip @slice-36 @ID-CLAUDEHOME-03 @local-engine-fixture @linux
  Scenario: Frozen Claude initialization precedes the literal first instruction
    Given a Claude launch fixture with an empty owned home conflicting image defaults and non-Claude argv containing empty spaced and shell-looking arguments
    When run selects Claude settings then requests change to a plain home before launch through both local backends
    Then durable ownership and the home claim precede verified claude-v1 files and environment and every worker start
    And create and start preserve all literal arguments approved hardening and HOME XDG TMPDIR mappings without a helper hook or entrypoint
    And a fresh independently approved plain-home run keeps its original environment and does not seed while the selected snapshot remains unchanged

  # The fixture independently seeds sibling state but leaves .claude.json absent.
  # Observe the workload's edit independently before launching the next controller.
  # The richer completed JSON includes a nested unknown object, a large integer,
  # escaped Unicode, preferences and project data; retain its exact bytes/mtime.
  @wip @slice-36 @ID-CLAUDEHOME-04 @local-engine-fixture @linux
  Scenario Outline: Repeated Claude runs preserve state and seed only a missing file
    Given a Claude retained-home fixture with independent history settings credential-link other-harness and private-marker sentinels
    When a first selected run observes its seed then its workload leaves "<state>" through both local backends
    Then the first instruction saw the complete minimal Claude seed and independent inspection records the workload's retained state before another run
    When two fresh Claude controllers reuse that home and another checkout runs through both local backends
    Then a missing state file alone is created once and every complete retained state file is reused byte and metadata identical without a seed write
    And all unrelated sentinels are unchanged no credential target is opened and the other checkout receives its own minimal seed without private markers
    And every session cleanup retains the original homes and confirms only owned session resources absent

    Examples:
      | state                        |
      | the unchanged completed seed |
      | richer completed JSON state  |
      | the state file removed       |

  # Retained-path safety. Even an in-home symlink is refused. The legacy entry
  # exists independently of the correct .claude.json, so checking only the latter
  # cannot satisfy this row. All faults have a separate successful selected control.
  @wip @slice-36 @ID-CLAUDEHOME-05 @local-engine-fixture @linux
  Scenario Outline: Retained paths cannot redirect Claude initialization
    Given a Claude seed fixture with path defect "<defect>" outside sentinels and an independent safe Claude control
    When both Claude launches are attempted through both local backends
    Then the unsafe path reports run.home-refused with exit 1 before seed mutation or start and the safe Claude control starts
    And no unsafe entry or target is followed repaired read for contents or changed and no unrelated resource changes

    Examples:
      | defect                                  |
      | Claude directory is a checkout symlink  |
      | Claude directory is a regular file      |
      | Claude directory has another owner      |
      | Claude directory mode is 0777           |
      | state file is an in-home symlink        |
      | state file is a credential-link symlink |
      | state file is a directory               |
      | state file is a FIFO                    |
      | state file has another owner            |
      | state file mode is 0666                 |
      | legacy .config.json is a regular file   |
      | legacy .config.json is a symlink        |

  # The size/depth rows' completed-state controls exercise a valid exactly-1-MiB
  # object and exactly 32 nested containers, so smaller accidental limits fail.
  @wip @slice-36 @ID-CLAUDEHOME-06 @local-engine-fixture @linux
  Scenario Outline: Incompatible retained state is preserved and refused
    Given a safe Claude directory with retained state "<state>" private sentinels and an independent completed-state control
    When selected Claude runs inspect both retained states through both local backends
    Then incompatible state reports run.home-refused with exit 1 without writes or start while the completed-state control starts without rewriting its file
    And retained bytes metadata and all sibling state stay intact and diagnostics identify only the fixed path and safe reason without file contents

    Examples:
      | state                                   |
      | missing onboarding member               |
      | false onboarding member                 |
      | string-valued onboarding member         |
      | duplicate onboarding members            |
      | duplicate nested object keys            |
      | non-object JSON                         |
      | empty or truncated JSON                 |
      | valid object followed by another object |
      | invalid UTF-8                           |
      | 33 nested containers                    |
      | 1 MiB plus one byte                     |

  # Copy faults begin with an absent state file; the retained-read fault starts
  # with an independently seeded completed object. Each timely control has the
  # same initial state without the fault. Verification faults occur after copy.
  # Preserve partial bytes on failure, never fabricate an absent file.
  # Stop recovers resources only. A later run can accept a complete accepted copy
  # after a lost reply, but must refuse a partial JSON left by a partial copy.
  @wip @slice-36 @ID-CLAUDEHOME-07 @local-engine-fixture @linux
  Scenario Outline: Uncertain initialization never starts Claude or resets its state
    Given a Claude seed operation fixture with fault "<fault>" retained sentinels and an independent timely Claude control
    When launch encounters the fault then fresh status and stop recover it and another selected run follows through both local backends
    Then the failed launch reports "<rule>" with exit 75 and no start within its original budget while the timely control starts
    And no copy retries within that invocation and no late reply causes a start mutation or success publication after expiry
    And recovery retains the home and original workload knowledge and confirms owned session absence or reports the exact original stop retry
    And status and stop never seed and the later run accepts only absent or complete valid state while preserving and refusing partial state

    Examples:
      | fault                                    | rule                 |
      | retained file read unavailable           | run.home-unavailable |
      | seed copy fails before writing           | run.home-unavailable |
      | seed copy leaves partial JSON            | run.home-unavailable |
      | complete copy reply is lost              | run.home-unavailable |
      | verification archive is truncated        | run.home-unavailable |
      | verification archive has an extra entry  | run.home-unavailable |
      | verification framing exceeds 64 KiB      | run.home-unavailable |
      | seed operation exceeds call deadline     | run.call-timeout     |
      | seed operation exhausts startup deadline | run.startup-timeout  |

  @wip @slice-36 @ID-CLAUDEHOME-08 @local-engine-fixture @linux
  Scenario Outline: Final inspection checks the actual Claude seed boundary
    Given a Claude seed fixture reporting successful preparation but final defect "<defect>" and an independent correctly inspected Claude control
    When both launches reach verification after attach through both local backends
    Then the defective launch reports run.isolation-unverified with exit 1 and no start while the correct Claude control starts
    And owned session resources are cleaned and the home is retained without leaking state contents or raw engine errors

    Examples:
      | defect                                        |
      | CLAUDE_CONFIG_DIR still names the image path  |
      | nonessential-traffic switch is empty          |
      | newly generated file lacks true onboarding    |
      | newly generated file contains an extra member |
      | state file has wrong owner                    |
      | state file has wrong mode                     |
      | state path is now a symlink                   |
      | legacy .config.json is now present            |

  # Real-engine evidence: observers run as the approved non-root identity and
  # report files/environment BEFORE mutating their own home. No Claude installation
  # is needed for this filesystem probe. Equal basenames cannot share state.
  @wip @slice-36 @ID-CLAUDEHOME-09 @engine @linux
  Scenario: Native first instructions see private Claude state across runs and checkouts
    Given two CI Claude home checkouts with equal basenames separate approvals for the same non-root UID GID and isolated host image and checkout state sentinels
    When literal native probes inspect their initial Claude state write distinct private markers and rerun each checkout
    Then each first probe reads exactly the minimal seed at the fixed Claude path with approved owner modes and both environment values before any other work
    And later probes read their own retained state and markers but cannot read another checkout's home or host state while same-home controls succeed
    And no host image or checkout state was copied and argv and hardening remain exact
    And all owned session resources are independently absent before fixture teardown while both private homes remain

  # REAL pinned Claude, fake service only. The trusted fixture workload opens an
  # in-worker PTY for the interactive onboarding observation, independently of
  # product --tty (slice-37). Do not claim product terminal support from it.
  # The same image/environment, except for a separately disposable unseeded config
  # directory, forms the control. Send NO onboarding/theme/login/trust answers.
  # The seeded process must reach its ordinary directory-trust prompt; the
  # unseeded one must show initial onboarding. Stop/join both after observation.
  # A headless -p success alone does not prove the onboarding flag was consumed.
  # Then run real -p and --resume through two product runs and a deterministic fake
  # Messages service with test-only DNS/CA and an explicitly approved proxy route.
  # Only the fixture child environment supplies the fake endpoint and a synthetic
  # nonsecret auth marker; no credential grant, real token, account or model. It
  # must preserve both selected environment values and state for every seeded
  # child; only the unseeded control selects the separate config directory.
  # Clean fixture project/user settings have no hooks/MCPs/plugins. No flags that
  # skip onboarding, setup-completing input or test patch to Claude are allowed.
  # Observe all incidental destinations with controlled recorders/proxy refusals;
  # positive recorder canaries run separately. Include normal shutdown/flush.
  # Successful fake exchanges, matching session IDs and the prior turn in the
  # resumed request prove startup and persistence. Timeouts, missing transcripts,
  # refused fake requests and a silent/dead recorder never count as success.
  @wip @slice-36 @ID-CLAUDEHOME-10 @engine @linux
  Scenario: Pinned Claude consumes onboarding state and resumes without incidental traffic
    Given a CI Claude 2.1.325 image an approved Claude home a controlled Messages endpoint and independently observable incidental-service recorders
    When a trusted in-worker PTY probe observes seeded and unseeded startup then product runs execute and resume deterministic headless fake turns
    Then seeded Claude reaches directory trust without onboarding input while the unseeded control shows initial onboarding and both children are joined
    And both headless turns complete without login input and the second resumes the first session with its prior fake turn visible to the endpoint
    And only expected fake model requests occur with no incidental update telemetry error-reporting or startup request through normal shutdown
    And independent recorder canaries succeed no real account credential or service was used and product session cleanup is confirmed before fixture teardown

  # Native archive/path behavior cannot be established by a recording HTTP API.
  # A completed hard-linked file is read-only reuse: neither name may be rewritten.
  @wip @slice-36 @ID-CLAUDEHOME-11 @engine @linux
  Scenario Outline: Native seeding cannot overwrite a retained alias or unrelated target
    Given a disposable CI Claude home with "<entry>" an unrelated sentinel and an independent safe Claude control
    When the product attempts native Claude initialization and the control runs before fixture teardown
    Then the native result is "<result>" and the safe control starts with verified claude-v1 state
    And the retained entry target credential links and unrelated sentinel keep their original bytes metadata and identities
    And refused workers never start and all owned session resources are absent before fixture teardown while homes remain

    Examples:
      | entry                                      | result                                                |
      | state symlink to a checkout sentinel       | run.home-refused with exit 1 before mutation or start |
      | Claude directory symlink to the checkout   | run.home-refused with exit 1 before mutation or start |
      | completed state hard-linked to a sentinel  | exit 0 with no state write and both inodes unchanged  |
      | incomplete state hard-linked to a sentinel | run.home-refused with exit 1 before mutation or start |
