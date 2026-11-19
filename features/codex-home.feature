@phase-1
Feature: Codex starts from approved defaults in its private persistent home
  A person can select Codex defaults for an approved harness home.
  Every run prepares those defaults before the literal workload starts.
  Codex keeps its sessions inside that checkout's home without inheriting host state.

  # Slice-35 specification only; every step phrase below is NEW. No implementation,
  # step definitions or observed red/green evidence accompanies this draft.
  # Authorities: charter milestone two steps 2 and 3; architecture.md Slices;
  # decision 0030 and harness-home.feature for the landed persistent home (slice-34);
  # decision 0028 and egress.feature for egress run wiring (slice-27); decision 0025
  # question 1, q-72 and the charter's step 2 for the Codex credential directory;
  # manifest.feature, offline.feature, lifecycle.feature, recovery.feature and
  # checkout-overlap.feature for the rules reused below.
  # Read against 8413500, this branch's merge of main 6798ca3. slice-34 and
  # slice-27 are done there. These scenarios use no credential grant or account.
  #
  # Placement. The local scenarios CODEXHOME-01..07 are here. The native ones,
  # CODEXHOME-08..10, are in harness-home.feature beside HOME-13/14. That file is
  # already in the offline engine shard. TestEngineShards (engine_shards_test.go)
  # puts every file with an @engine case, live or @wip, in exactly one pinned shard
  # set, and .github is outside this item's paths. So this file keeps no @engine case
  # and needs no shard assignment. Do not weaken that inventory check. If measured
  # offline-shard time later needs a move, the host makes it as a ci commit with the
  # inventory pins, as q-76 did for slice-34. A feature file is never split.
  #
  # Commits. The implementer may commit freely in its workspace; the host makes the
  # landing commits and handles itos, submission, publication and review.
  #
  # Selection and approval, decided here:
  # - Extend the closed home object with optional harness: codex, a literal
  #   case-sensitive string, only alongside storage: volume. Apply to supported
  #   versions 1/2/3; any later policy version adopts this shape in its own spec.
  #   No argv sniffing, host Codex discovery, auto-install, manifest lifecycle
  #   command, top-level harness key or caller-selected seed path/content.
  # - File-only doctor retains harness: codex in its normalized home object.
  #   Null/non-string is policy.type at /home/harness; another string (including
  #   empty) is policy.home_harness there, exit 2. Missing storage retains
  #   policy.required at /home/storage. Duplicate/unknown fields keep their rules.
  # - Manifest reservation is scoped to the Codex selection. Every command that
  #   validates the manifest with the policy (doctor --manifest-only, doctor,
  #   approve, run) applies it. With harness: codex, a containerEnv name equal to
  #   CODEX_HOME under the existing ASCII case folding (so CODEX_HOME and codex_Home)
  #   reports manifest.reserved at /containerEnv/<name> as written, exit 2, through
  #   the existing manifest problem envelope. With an omitted or plain home the same
  #   manifest stays valid: the variable is retained in effective containerEnv and
  #   reaches the worker as before. No other CODEX_ name becomes reserved, and
  #   manifest.feature's context-free reserved list and its rows are unchanged.
  #   Run's existing version-3 refusal before manifest access keeps its precedence.
  # - Add to slice-34's effective home object exactly
  #   harness:{kind:"codex",seed:"codex-v1",target:"/home/itos/.codex"}.
  #   Retain the selected home profile name and other fields. This nested revision
  #   binds the fixed settings below; changing it requires a new fingerprint.
  #   An omitted harness retains the previous canonical bytes and empty-home rule.
  #   No-home settings also remain byte-identical. There is no new grant or mount.
  # - Doctor/approve show the harness, seed revision, CODEX_HOME, disabled services
  #   and inner sandbox, and that config.toml is regenerated on EVERY run while
  #   sessions persist. Adding/removing it is a /home/harness difference and needs
  #   approval. Existing q-52 terminal rules for network/credentials still apply.
  #   Inspecting/approving never seeds, runs Codex or allocates a home.
  # - Freeze selection with the existing snapshot. Neither mutable requests nor
  #   the executable's basename may select a different seed after that point.
  #   Harness choice does not change slice-34's home scope/generation. Disabling
  #   seeding does not delete files; it simply stops generating them or overriding
  #   CODEX_HOME. Only slice-34's separately consented reset discards home state.
  #
  # The generated codex-v1 config.toml has exactly these TOML values (formatting
  # and comments are not the public contract; there are no other settings):
  #   check_for_update_on_startup = false
  #   sandbox_mode = "danger-full-access"
  #   approval_policy = "never"
  #   cli_auth_credentials_store = "file"
  #   [analytics]
  #   enabled = false
  #   [features]
  #   plugins = false
  #   remote_plugin = false
  #   [otel]
  #   exporter = "none"
  #   trace_exporter = "none"
  #   metrics_exporter = "none"
  #   log_user_prompt = false
  #
  # Codex source baseline is rust-v0.167.0, the release decision 0025 examined:
  # https://github.com/openai/codex/blob/rust-v0.167.0/codex-rs/core/config.schema.json
  # Official configuration reference, consulted for meanings, not a version pin:
  # https://learn.chatgpt.com/docs/config-file/config-reference
  # Analytics alone does not disable explicitly configured OTLP export; hence the
  # explicit exporters. plugins=false disables the plugin subsystem, rather than
  # merely disabling named installed plugins while permitting their refresh.
  # The inner sandbox is off because the approved container is the sandbox;
  # approval_policy=never avoids unattended tool approval prompts inside it.
  # These are defaults, NOT an additional security boundary or a promise against
  # explicit argv/project/system Codex overrides or hostile worker edits. The
  # container's hardening and approved proxy still hold regardless of Codex config.
  # This slice does not configure a model, provider, API key or login, mark a project
  # trusted, import host config/plugins, seed auth.json or acquire a harness image.
  # The approved image supplies Codex. Missing Codex keeps ordinary workload exit
  # semantics. Production does not execute it to discover versions or settings.
  #
  # Credentials. This slice delivers none and prescribes no credential layout. The
  # q-72 dedicated directory and its worker-side arrangement belong to slice-32.
  # Here auth.json and auth.lock inside .codex, whatever their kind (absent, regular
  # file or symlink), are never read, created, resolved, repaired, truncated or
  # removed. Their bytes, metadata and link targets are preserved.
  #
  # Initialization and repeat-run decisions:
  # - With the frozen Codex selection, override any image CODEX_HOME with the
  #   literal /home/itos/.codex. Slice-34's HOME/XDG/TMPDIR mappings are unchanged.
  #   A manifest cannot supply CODEX_HOME under this selection (see above). Seed even
  #   for a non-Codex argv: selection is explicit, never an implicit launcher or
  #   shell/argv rewrite.
  # - After durable worker/home ownership and before ANY worker start, create a
  #   missing .codex directory as approved UID:GID, mode 0700, and publish only
  #   config.toml as that owner, mode 0600. When .codex already exists the archive
  #   holds config.toml alone, so no directory header is applied to existing state.
  #   Regenerate this one file on every run, including a run reusing a home created
  #   before this feature. Previous custom content in this reserved generated file
  #   is replaced, not merged or imported. Users who choose this profile use explicit
  #   Codex argv options for overrides.
  # - Preserve unrelated files' bytes and metadata, including sessions, history,
  #   caches, preferences in other files and other harnesses' state. Here metadata
  #   means UID/GID, mode, mtime and symlink target. Normal parent timestamps and
  #   link-count/ctime changes when removing the old config name are permitted.
  #   Disabling/re-enabling seeding, image changes and new approval generations do
  #   not reset state or select another volume. A new checkout gets its own home.
  # - Use slice-34's bounded Engine archive seam on the unstarted owned worker.
  #   No image entrypoint, root helper, host shell, checkout hook, container exec,
  #   host traversal of the volume mountpoint or recursive home scan. Check home
  #   ownership and .codex metadata without following links before copying.
  #   An existing .codex must be a real directory with approved owner/mode; an
  #   existing config must be a regular file owned by that UID:GID, mode 0600.
  #   Refuse symlinks (even in-home), directories at config, special files and
  #   unsafe ownership/modes before seed mutation. Do not chmod/chown old state.
  # - Replacing a regular config must create a separate file, never modify an
  #   inode through another hard link. Native proof includes a config hard-linked
  #   to an unrelated sentinel. It must preserve the sentinel; if the pinned
  #   Engine archive seam cannot do this, stop with the evidence, not a host helper.
  # - Verify generated bytes, metadata and actual CODEX_HOME before start. For
  #   directory metadata, consume only the first root header within 64 KiB including
  #   extended headers, then close, as slice-34 does; never inventory its children.
  #   Bound the generated-file readback archive to 64 KiB including extended
  #   headers, reject malformed/truncated/extra entries, never extract to the host.
  #   Readback reads only the generated file, not old contents or sibling state; no
  #   raw contents, engine errors or workload settings enter diagnostics/recovery
  #   records. Recheck boundary after attachment as existing launch does. Existing
  #   home claims exclude another worker/retirement while seeding; do not weaken them.
  #
  # Failure classification:
  # - Unsafe seed paths use run.home-refused (1). Unavailable copy/inspection or an
  #   unconfirmed write uses run.home-unavailable (75). A confirmed wrong final
  #   environment/seed uses run.isolation-unverified (1).
  # - A partial write means the copy call stored part of the archive and then its
  #   transport failed: no successful reply, so run.home-unavailable (75). A
  #   successful copy reply whose readback then shows wrong or truncated bytes is a
  #   confirmed wrong seed, so run.isolation-unverified (1), as in CODEXHOME-07.
  # - A lost copy reply is a prompt transport failure after accepted publication,
  #   not a delayed response: run.home-unavailable (75), as slice-34 classifies
  #   uncertain publication.
  # - Budget exhaustion retains lifecycle.feature's rules: a seed call alone passing
  #   its 10 s call deadline is run.call-timeout (75); when the call and the 120 s
  #   startup deadline expire together, run.startup-timeout (75) takes precedence.
  #   Neither is reclassified as run.home-unavailable. Show the fixed seed path and
  #   safe recovery guidance, never an observed target or contents. Preserve existing
  #   ownership-refusal precedence and known workload exits.
  # - A failed/partial/lost copy reply never permits start or an in-invocation
  #   retry. Each invocation makes at most one config publication call and at most
  #   one readback. Teardown retains the home and removes owned session resources.
  #   When worker absence cannot be confirmed, run reports existing
  #   run.cleanup-temporary separately, with its registered session ID and exact
  #   stop retry. The checkout reservation is held through that incomplete cleanup,
  #   as checkout-overlap.feature requires. Until then another run of that checkout
  #   reports run.checkout-overlap (1) before any create, archive copy or start.
  #   Stop/status never seed or start a worker. Stop removes only the exact retained
  #   worker and confirms its absence; only a later run then regenerates. No success
  #   marker in hostile home state authorizes skipping verification.
  # - Calls share existing 10 s call/120 s startup totals; cancellation/expiry
  #   fences later start. Cleanup keeps its independent 30 s budget. No new timer
  #   resets, unbounded readback or workload lifetime limit.
  #
  # Architecture and dependencies:
  # Extend existing internal/slices/run, internal/slices/doctor and
  # internal/slices/approve in their command/domain/port and named-adapter layout.
  # Existing internal/slices/status and internal/slices/stop consume shared session
  # recovery for seed failures, retaining read-only status and seed-free stop.
  # No new command slice is created; command handlers stay thin.
  # internal/engine/policy owns the closed harness selection; internal/engine/manifest
  # takes the selection as a value for the scoped CODEX_HOME reservation, keeping
  # its context-free rule unchanged; internal/engine/approval owns canonical/frozen
  # settings, differences and disclosure. Reuse read-only identity/image observations
  # in internal/engine/readiness, without seeding there.
  # Refine slice-34's internal/engine/session domain/port contracts and Docker
  # adapter for seed sequencing, bounded archive publication and verification;
  # reuse its disk ownership/claims/recovery mechanism and internal/engine/budget.
  # Only cmd binds adapters. Domain/port fakes are I/O-free, with one shared seed
  # contract against fake and Docker adapter. No slice imports another. Update
  # arc42's existing home building block/runtime description and CLI/CONFIG in the
  # implementation PR; no departure from decision 0030's storage/claims mechanism.
  # Reuse Moby client v0.6.2/API v1.56.1 and Go 1.27.4 archive/tar: a fixed literal
  # TOML document needs no custom parser or new production library. Codex itself
  # parses the effective config in native tests.
  #
  # Native fixture preparation. The shared `tools/bin/engine-fixture prepare` step
  # serves every shard and is within this item's paths; its workflow budget is not.
  # It adds a CI-only Codex 0.167.0 image built from the pinned Linux amd64 release
  # asset. A SHA-256 recorded in the tree is verified before use, then the image
  # digest. Codex's Apache-2.0 license/notices are preserved; no floating latest
  # install. If this cannot be prepared within the existing prepare budget, stop and
  # report rather than editing .github or another shard.
  #
  # Handoff and evidence rules:
  # - Add all phrases below in a red test commit, @wip retained. Givens ONLY create
  #   private checkouts, literal requests, independently encoded approval/home
  #   records, metadata, faults and sentinels. They never call product commands.
  #   Each When records early refusal/exit/missing probe as data for Then, never
  #   fails merely because product setup cannot yet recognize the new selection.
  # - Both local backends means the existing command test assembly over I/O-free
  #   ports and real disk/Moby with a recording Unix HTTP Engine fixture. See
  #   offline_launch_steps_test.go, session_harness_test.go and slice-34's
  #   harness_home_policy_steps_test.go/harness_home_local_steps_test.go. Timeout
  #   rows use the controlled time of lifecycle_budget_steps_test.go. Assertions
  #   inspect public CLI results and independent artifact/API traces, never product
  #   functions or a production-generated expected config.
  # - Refusal/fault cases need an independent successful CODEX home control, not
  #   merely a plain-home run. Otherwise a blanket refusal can falsely pass.
  #   Seed synthetic retained state directly; first-run success is not a Given.
  # - Local assertions prove orchestration only. Engine rows run exclusively in
  #   CI; agents have no socket. Seed-path safety is observed at the real boundary
  #   in harness-home.feature's native rows, not inferred from an Engine request.
  # - Auth path sentinels are a regular auth.json and an auth.lock symlink to an
  #   outside sentinel, both inside .codex; neither layout is a credential contract.
  # - The smoke manifest needs a selection only when a scenario here goes live.
  #   Neighbours: HOME-01..15/24, POLICY/MANIFEST reserved-variable cases
  #   (MANIFEST-15 unchanged), APPROVAL/RUN frozen input and literal argv cases,
  #   OFFLINE/ANCHOR/LIFECYCLE/RECOVERY/OVERLAP and EGRESS.

  # Policy, manifest and consent. First Then is red on the new harness selection,
  # not on a malformed-input refusal that the old parser already supplies.
  # Malformed requests cover null, list, boolean, empty string, "Codex", "other",
  # absent storage, duplicate harness and a source/config/target key in home.
  # The two manifests each set one variable, CODEX_HOME or codex_Home, to a literal
  # value. The omitted- and plain-home rows are controls: they pass today and fail
  # a global reservation.
  @wip @slice-35 @ID-CODEXHOME-01
  Scenario Outline: Codex seeding is an explicit closed home selection
    Given a Codex home policy fixture for version <version> with omitted plain Codex and malformed harness requests and CODEX_HOME and codex_Home manifests
    When file-only doctor reads each Codex home policy request
    Then only the Codex selection adds harness codex to normalized home and omitted and plain home outputs are unchanged
    And every malformed selection has the specified rule subject and exit without engine state or harness access
    When manifest-only doctor reads both CODEX_HOME manifests under the omitted plain and Codex home selections
    Then under the Codex selection each manifest reports manifest.reserved at its own containerEnv name with exit 2 and no engine access
    And under the omitted and plain home selections both manifests are valid with that variable and value in effective containerEnv

    Examples:
      | version |
      | 1       |
      | 2       |
      | 3       |

  @wip @slice-35 @ID-CODEXHOME-02 @local-engine-fixture @linux
  Scenario: Approval binds and discloses the generated Codex defaults
    Given Codex home review fixtures with independently recorded plain and Codex approvals and isolated consent terminals
    When doctor and consenting approve review Codex seeding and run attempts both changed and matching selections
    Then review and the new approval contain the exact codex-v1 home settings and regeneration disclosure with a changed fingerprint
    And adding and removing the harness without consent report changed approval at /home/harness before mutation while matching controls start
    And review creates no home or seed and requesting a plain home after seeding leaves retained files and generation intact

  # Local launch and state lifetime. The policy, not argv[0], selects seeding.
  # The codex_Home sibling is an independent version 1 Codex checkout; its refusal
  # must come before any engine mutation, beside the image-environment override.
  @wip @slice-35 @ID-CODEXHOME-03 @local-engine-fixture @linux
  Scenario: Frozen Codex defaults exist before the literal workload starts
    Given a Codex launch fixture with an empty owned home conflicting image CODEX_HOME and literal non-Codex argv containing empty spaced and shell-looking arguments and a sibling Codex checkout whose manifest sets codex_Home
    When run selects Codex settings then requests change to a plain home before launch through both local backends
    Then the first start follows durable ownership and verified codex-v1 bytes and metadata with CODEX_HOME /home/itos/.codex
    And create and start preserve every literal argument and the approved home hardening without helpers hooks or image entrypoints
    And a fresh independently approved plain-home run keeps its old environment and does not seed while the selected snapshot is unchanged
    And the sibling run reports manifest.reserved at /containerEnv/codex_Home with exit 2 before any engine mutation

  # Observe the first workload's edit before invoking another controller. The
  # replacement is the independent literal check_for_update_on_startup = true;
  # replacement/truncation retain the approved owner and mode. Local backends
  # model these edits only after worker start, never as Given side effects.
  # Record each invocation's successful publication and readback separately;
  # identical final bytes or an earlier run's trace cannot prove regeneration.
  # The third run begins with the second run's unchanged complete config and
  # still must publish it again, distinguishing unconditional from repair-only seeding.
  @wip @slice-35 @ID-CODEXHOME-04 @local-engine-fixture @linux
  Scenario Outline: Every selected run refreshes only the generated config
    Given a retained Codex home fixture with "<config>" and independent session history cache other-harness and auth path sentinels
    When the first selected run records its initial config then its workload "<edit>" through both local backends
    Then the first workload saw the complete codex-v1 config and independent inspection before another run confirms "<retained>"
    When two fresh Codex run controllers reuse that home without further config edits and a separate checkout runs through both local backends
    Then every selected run publishes and verifies complete codex-v1 bytes and approved metadata before start including the third run with already correct config
    And all three runs use the original home while every unrelated sentinel remains byte and metadata identical
    And the separate checkout receives its own seeded home without the retained markers and no auth path or its target is read or changed
    And all session cleanups retain their homes without any home reset or volume removal

    Examples:
      | config                                  | edit                                      | retained                           |
      | missing config in an existing directory | removes config.toml                       | config.toml is absent              |
      | changed regular generated config        | replaces config.toml with enabled updates | config.toml contains only the edit |
      | truncated regular generated config      | truncates config.toml                     | config.toml is a zero-length file  |

  @wip @slice-35 @ID-CODEXHOME-05 @local-engine-fixture @linux
  Scenario Outline: Unsafe retained seed paths cannot redirect initialization
    Given a Codex home fixture with "<defect>" and an independent safe Codex control and outside sentinels
    When both seed launches are attempted through both local backends
    Then the unsafe seed reports run.home-refused with exit 1 before seed writes or start and the safe Codex control starts
    And neither the unsafe entry nor its target is followed repaired read for contents or changed and unrelated resources remain intact

    Examples:
      | defect                            |
      | Codex directory is a symlink      |
      | Codex directory is a regular file |
      | Codex directory has another owner |
      | Codex directory mode is 0777      |
      | config is an in-home symlink      |
      | config is a checkout symlink      |
      | config is an auth-target symlink  |
      | config is a directory             |
      | config is a FIFO                  |
      | config has another owner          |
      | config mode is 0666               |

  # Every fault is one-shot: it affects exactly one engine call and then the
  # engine is healthy. A retry inside the failing invocation would therefore
  # succeed and start, so the oracle counts calls: exactly one config publication
  # call, at most one readback and no start in the first invocation, no archive
  # copy by stop. The removal fault of the last row persists until the fixture
  # clears it after the first run's result is captured.
  # Non-timeout faults return before either deadline. The call-timeout row keeps at
  # least 30 s of startup budget when its call passes 10 s. The startup row places
  # 115 s of successful progress before seeding and leaves the seed call pending
  # across both deadlines, under controlled time.
  # The early run is a fresh run of the same checkout after the faults clear and
  # before stop. In the confirmed rows it is an ordinary seeded run. In the last
  # row the held checkout reservation must refuse it before any create or copy.
  @wip @slice-35 @ID-CODEXHOME-06 @local-engine-fixture @linux
  Scenario Outline: Incomplete seeding never authorizes a worker start
    Given a Codex seed fixture with one-shot fault "<fault>" a timely Codex control and unrelated retained markers
    When the faulted seed launch runs then the faults clear and an early run a fresh stop of the first session and a later run follow through both local backends
    Then the first run reports "<diagnostic>" with exit <exit> after exactly one config publication call at most one readback and no start while the timely control starts
    And its cleanup "<cleanup>" while the home and unrelated markers are retained and no workload exit is claimed
    And the early run "<early>"
    And the fresh stop makes no archive copy or start and removes only an exact first-session worker that still remains
    And the later run regenerates and verifies the complete config before starting only after first-session worker absence is confirmed

    Examples:
      | fault                                                         | diagnostic           | exit | cleanup                                                                   | early                                                               |
      | seed copy fails before writing                                | run.home-unavailable | 75   | confirms owned session absence                                            | seeds verifies and starts                                           |
      | seed copy stores a partial archive then its transport fails   | run.home-unavailable | 75   | confirms owned session absence                                            | seeds verifies and starts                                           |
      | seed copy is accepted but its reply is lost                   | run.home-unavailable | 75   | confirms owned session absence                                            | seeds verifies and starts                                           |
      | generated config readback is unavailable                      | run.home-unavailable | 75   | confirms owned session absence                                            | seeds verifies and starts                                           |
      | readback archive is truncated                                 | run.home-unavailable | 75   | confirms owned session absence                                            | seeds verifies and starts                                           |
      | readback exceeds the 64 KiB limit                             | run.home-unavailable | 75   | confirms owned session absence                                            | seeds verifies and starts                                           |
      | seed call alone exceeds its 10 second call deadline           | run.call-timeout     | 75   | confirms owned session absence                                            | seeds verifies and starts                                           |
      | seed call is pending when call and startup deadlines expire   | run.startup-timeout  | 75   | confirms owned session absence                                            | seeds verifies and starts                                           |
      | seed copy fails before writing and worker removal leaves it   | run.home-unavailable | 75   | reports run.cleanup-temporary with its session ID and exact stop retry    | reports run.checkout-overlap with exit 1 before any create or copy  |

  # The truncated-readback row has a successful copy reply; the injected readback
  # holds a valid shortened config.toml entry, so it is a confirmed wrong seed.
  @wip @slice-35 @ID-CODEXHOME-07 @local-engine-fixture @linux
  Scenario Outline: Final inspection verifies the actual Codex seed boundary
    Given a Codex seed fixture reporting successful copy but final defect "<defect>" and an independent correctly inspected Codex control
    When both launches reach post-attach verification through both local backends
    Then the defective boundary reports run.isolation-unverified with exit 1 and no start while the correct Codex control starts
    And the unstarted owned worker is cleaned and the home is retained without diagnostic copies of file contents or raw engine errors

    Examples:
      | defect                                  |
      | CODEX_HOME still names the image home   |
      | config has the old enabled settings     |
      | readback shows a truncated config       |
      | generated config has wrong owner        |
      | generated config has wrong mode         |

  # Native evidence for this slice is in harness-home.feature, beside HOME-13/14,
  # because that file is in the offline engine shard: CODEXHOME-08 (first instructions
  # across runs and checkouts), CODEXHOME-09 (pinned Codex against a fake service)
  # and CODEXHOME-10 (archive replacement cannot overwrite an unrelated target).
