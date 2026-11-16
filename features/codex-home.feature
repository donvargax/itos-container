@phase-1
Feature: Codex starts from approved defaults in its private persistent home
  A person can select Codex defaults for an approved harness home.
  Every run prepares those defaults before the literal workload starts.
  Codex keeps its sessions inside that checkout's home without inheriting host state.

  # Slice-35 specification only; every step phrase below is NEW. No implementation,
  # step definitions or observed red/green evidence accompanies this draft.
  # Authorities: charter milestone two step 3; architecture.md Slices; slice-34's
  # harness-home.feature and decision 0028; slice-32's credentials-codex.feature
  # and decision 0030; offline.feature, run.feature and egress.feature.
  # Read against main d95856b2e594a2fc80625ff3b976a8ebebaa3378,
  # slice-34 68be535a07039b29ae294f7d72e4be00b6881e6f,
  # slice-32 20cb15bfaacf8f1d8b7f210f8eea8087b4f92b89 and
  # slice-27 9ee8751184313d9961dae4a76baa4d4aa40b5068.
  # These are design inputs, not claims that their unfinished behavior has landed.
  # Consume slice-34 first. CODEXHOME-09 additionally needs slice-27's working
  # approved proxy route. No real-account or credential-delivery dependency is
  # introduced: all these scenarios use no credential grants or real accounts.
  #
  # Selection and approval, decided here:
  # - Extend the closed home object with optional harness: codex, a literal
  #   case-sensitive string, only alongside storage: volume. Apply to supported
  #   versions 1/2/3; a later version 4 uses the same shape when slice-32 lands.
  #   No argv sniffing, host Codex discovery, auto-install, manifest lifecycle
  #   command, top-level harness key or caller-selected seed path/content.
  # - File-only doctor retains harness: codex in its normalized home object.
  #   Null/non-string is policy.type at /home/harness; another string (including
  #   empty) is policy.home_harness there, exit 2. Missing storage retains
  #   policy.required at /home/storage. Duplicate/unknown fields keep their rules.
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
  # Codex source baseline is rust-v0.167.0, also selected by decision 0030:
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
  # Initialization and repeat-run decisions:
  # - With the frozen Codex selection, override any image CODEX_HOME with the
  #   literal /home/itos/.codex. Slice-34's HOME/XDG/TMPDIR mappings are unchanged.
  #   Manifest CODEX_ variables stay reserved. Seed even for a non-Codex argv:
  #   selection is explicit, never an implicit launcher or shell/argv rewrite.
  # - After durable worker/home ownership and before ANY worker start, create a
  #   missing .codex directory as approved UID:GID, mode 0700, and publish only
  #   config.toml as that owner, mode 0600. Regenerate this one file on every run,
  #   including a run reusing a home created before this feature. Previous custom
  #   content in this reserved generated file is replaced, not merged or imported.
  #   Users who choose this profile use explicit Codex argv options for overrides.
  # - Preserve unrelated files' bytes and metadata, including sessions, history,
  #   caches, preferences in other files and other harnesses' state. Here metadata
  #   means UID/GID, mode, mtime and symlink target. Normal parent timestamps and
  #   link-count/ctime changes when removing the old config name are permitted.
  #   Never read, create, resolve, repair, truncate or remove auth.json/auth.lock.
  #   Those paths may be
  #   links prepared by the separate slice-32 integration to its approved directory
  #   and lock targets; CODEX_HOME is never that shared credential directory.
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
  #   Readback reads
  #   only the generated file, not old contents or sibling state; no raw contents,
  #   engine errors or workload settings enter diagnostics/recovery records.
  #   Recheck boundary after attachment as existing launch does. Existing home
  #   claims exclude another worker/retirement while seeding; do not weaken them.
  # - Unsafe seed paths use run.home-refused (1); unavailable copy/inspection or
  #   an unconfirmed write uses run.home-unavailable (75); a confirmed wrong
  #   final environment/seed uses run.isolation-unverified (1). Budget exhaustion
  #   retains run.call-timeout or run.startup-timeout (75), with startup taking
  #   precedence when both deadlines expire, as lifecycle.feature requires.
  #   It must not be reclassified as run.home-unavailable. Show the fixed
  #   seed path and safe recovery guidance, never an observed target or contents.
  #   Preserve existing ownership-refusal precedence and known workload exits.
  # - A failed/partial/lost copy reply never permits start or an in-invocation
  #   retry. Teardown retains the home, removes owned session resources, or keeps
  #   their exact stop retry on uncertainty. A later run may regenerate after
  #   prior worker absence is confirmed; stop/status never seed or start a worker.
  #   No success marker in hostile home state authorizes skipping verification.
  #   Calls share existing 10 s call/120 s startup totals; cancellation/expiry
  #   fences later start. Cleanup keeps its independent 30 s budget. No new timer
  #   resets, unbounded readback or workload lifetime limit.
  #
  # Architecture and dependencies:
  # Extend existing internal/slices/run, internal/slices/doctor and
  # internal/slices/approve in their command/domain/port and named-adapter layout.
  # Existing internal/slices/status and internal/slices/stop consume shared session
  # recovery for seed failures, retaining read-only status and seed-free stop.
  # No new command slice is created; command handlers stay thin.
  # internal/engine/policy owns the closed harness selection; internal/engine/approval
  # owns canonical/frozen settings, differences and disclosure. Reuse read-only
  # identity/image observations in internal/engine/readiness, without seeding there.
  # Refine slice-34's internal/engine/session domain/port contracts and Docker
  # adapter for seed sequencing, bounded archive publication and verification;
  # reuse its disk ownership/claims/recovery mechanism and internal/engine/budget.
  # Only cmd binds adapters. Domain/port fakes are I/O-free, with one shared seed
  # contract against fake and Docker adapter. No slice imports another. Update
  # arc42's existing home building block/runtime description and CLI/CONFIG in the
  # implementation PR; no departure from decision 0028's storage/claims mechanism.
  # Reuse Moby client v0.6.2/API v1.56.1 and Go 1.27.4 archive/tar: a fixed literal
  # TOML document needs no custom parser or new production library. Codex itself
  # parses the effective config in native tests. The CI-only fixture pins Codex
  # 0.167.0, verifies its release checksum and image digest before test execution,
  # and preserves its Apache-2.0 license/notices. No floating latest install.
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
  #   harness_home_policy_steps_test.go/harness_home_native_*_test.go. New feature
  #   assertions inspect public CLI results and independent artifact/API traces,
  #   never product functions or a production-generated expected config.
  # - Refusal/fault cases need an independent successful CODEX home control, not
  #   merely a plain-home run. Otherwise a blanket refusal can falsely pass.
  #   Seed synthetic retained state directly; first-run success is not a Given.
  # - Local assertions prove orchestration only. Engine rows run exclusively in
  #   CI; agents have no socket. Native assertions precede fixture cleanup, which
  #   only reclaims independently tracked disposable resources. Seed-path safety
  #   is observed at the real boundary, not inferred from an Engine request.
  # - CODEXHOME-09 runs the REAL pinned harness with a fake Responses endpoint
  #   allowed through slice-27's proxy and test-only DNS/CA controls. No model,
  #   account, provider login, API key or internet request. Custom provider config
  #   in the fixture's literal argv uses requires_openai_auth=false, a fixed local
  #   model description and the fake URL; it must not override the seeded settings.
  #   The fake emits a deterministic tool call and completion, never invokes a
  #   model. All incidental destinations terminate at controlled recorders or
  #   are observed refusals; direct network escape is not a testing shortcut.
  # - Inspect Codex's own effective settings (config/read and
  #   experimentalFeature/list in the pinned app-server protocol) as well as exec
  #   behavior. An independent
  #   unseeded home control must show differing effective defaults. A completed
  #   fake response and a tool-created home marker establish actual startup and
  #   command execution without the inner sandbox/approval wait. No-traffic alone
  #   is insufficient: exec need not exercise the TUI's update-check code path.
  #   Bound the entire fixture interaction, include the normal shutdown flush,
  #   and send recorder canaries separately to prove observation is live. Missing
  #   reports, refused requests, timeouts and empty transcripts are never success.
  # - Before activation, assign this file to exactly one engine shard, add the
  #   smoke selection and retain all no-harness HOME cases unchanged. Neighbours:
  #   HOME-01..15/24, POLICY/MANIFEST reserved-variable cases, APPROVAL/RUN frozen
  #   input and literal argv cases, OFFLINE/ANCHOR/LIFECYCLE/RECOVERY, EGRESS, and
  #   CODEXAUTH once integrated. Preserve slice-32's auth/lock path responsibility;
  #   this file supplies its private CODEX_HOME destination, not auth delivery.

  # Policy and consent. First Then is red on the new harness selection, not on
  # a malformed-input refusal that the old parser already supplies.
  @wip @slice-35 @ID-CODEXHOME-01
  Scenario Outline: Codex seeding is an explicit closed home selection
    Given a Codex home policy fixture for version <version> with omitted plain Codex and malformed harness requests
    When file-only doctor reads each Codex home policy request
    Then only the Codex selection adds harness codex to normalized home and omitted and plain home outputs are unchanged
    And every malformed selection has the specified rule subject and exit without engine state or harness access

    Examples:
      | version |
      | 1       |
      | 2       |
      | 3       |

  # Malformed requests cover null, list, boolean, empty string, "Codex", "other",
  # absent storage, duplicate harness and a source/config/target key in home.
  @wip @slice-35 @ID-CODEXHOME-02 @local-engine-fixture @linux
  Scenario: Approval binds and discloses the generated Codex defaults
    Given Codex home review fixtures with independently recorded plain and Codex approvals and isolated consent terminals
    When doctor and consenting approve review Codex seeding and run attempts both changed and matching selections
    Then review and the new approval contain the exact codex-v1 home settings and regeneration disclosure with a changed fingerprint
    And adding and removing the harness without consent report changed approval at /home/harness before mutation while matching controls start
    And review creates no home or seed and requesting a plain home after seeding leaves retained files and generation intact

  # Local launch and state lifetime. The policy, not argv[0], selects seeding.
  @wip @slice-35 @ID-CODEXHOME-03 @local-engine-fixture @linux
  Scenario: Frozen Codex defaults exist before the literal workload starts
    Given a Codex launch fixture with an empty owned home conflicting image CODEX_HOME and literal non-Codex argv containing empty spaced and shell-looking arguments
    When run selects Codex settings then requests change to a plain home before launch through both local backends
    Then the first start follows durable ownership and verified codex-v1 bytes and metadata with CODEX_HOME /home/itos/.codex
    And create and start preserve every literal argument and the approved home hardening without helpers hooks or image entrypoints
    And a fresh independently approved plain-home run keeps its old environment and does not seed while the selected snapshot is unchanged

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
    Given a retained Codex home fixture with "<config>" and independent session history cache other-harness and auth-link sentinels
    When the first selected run records its initial config then its workload "<edit>" through both local backends
    Then the first workload saw the complete codex-v1 config and independent inspection before another run confirms "<retained>"
    When two fresh Codex run controllers reuse that home without further config edits and a separate checkout runs through both local backends
    Then every selected run publishes and verifies complete codex-v1 bytes and approved metadata before start including the third run with already correct config
    And all three runs use the original home while every unrelated sentinel remains byte and metadata identical
    And the separate checkout receives its own seeded home without the retained markers and no auth target is read or changed
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

  # Non-timeout faults return before either deadline. A lost copy reply is a
  # prompt transport failure after accepted publication, not a delayed response.
  # Exhausting the original startup budget retains the lifecycle timeout diagnostic.
  @wip @slice-35 @ID-CODEXHOME-06 @local-engine-fixture @linux
  Scenario Outline: Incomplete seeding never authorizes a worker start
    Given a Codex seed fixture with fault "<fault>" a timely Codex control and unrelated retained markers
    When seed launch fails then the fault clears and fresh stop recovers it before a later run through both local backends
    Then the first run reports "<diagnostic>" with exit <exit> and no start within the original budget while the timely control starts
    And cleanup preserves home state and either confirms owned session absence or retains the exact stop retry without claiming a workload exit
    And stop never seeds and only the later run after confirmed worker absence regenerates and verifies the complete config before starting

    Examples:
      | fault                                     | diagnostic           | exit |
      | seed copy fails before writing            | run.home-unavailable | 75   |
      | seed copy returns after a partial write   | run.home-unavailable | 75   |
      | copy succeeds but its reply is lost       | run.home-unavailable | 75   |
      | generated config readback is unavailable  | run.home-unavailable | 75   |
      | readback archive is truncated             | run.home-unavailable | 75   |
      | readback exceeds the 64 KiB limit         | run.home-unavailable | 75   |
      | seed call exhausts remaining startup time | run.startup-timeout  | 75   |

  @wip @slice-35 @ID-CODEXHOME-07 @local-engine-fixture @linux
  Scenario Outline: Final inspection verifies the actual Codex seed boundary
    Given a Codex seed fixture reporting successful copy but final defect "<defect>" and an independent correctly inspected Codex control
    When both launches reach post-attach verification through both local backends
    Then the defective boundary reports run.isolation-unverified with exit 1 and no start while the correct Codex control starts
    And the unstarted owned worker is cleaned and the home is retained without diagnostic copies of file contents or raw engine errors

    Examples:
      | defect                                |
      | CODEX_HOME still names the image home |
      | config has the old enabled settings   |
      | generated config has wrong owner      |
      | generated config has wrong mode       |

  # Native evidence proves files visible to the actual non-root first instruction,
  # not merely bytes supplied to a recording Engine API.
  @wip @slice-35 @ID-CODEXHOME-08 @engine @linux
  Scenario: Native first instructions see private Codex defaults across runs and checkouts
    Given two CI Codex home checkouts with independently approved images non-root IDs and isolated host image and checkout sentinels
    When literal worker probes inspect the initial Codex files write private markers and rerun each checkout
    Then every probe reads the complete codex-v1 settings with the approved ownership modes and CODEX_HOME before doing other work
    And each later probe sees only its own persistent marker with no host image or checkout configuration imported
    And actual argv and hardening are unchanged and all session resources are absent before fixture cleanup while both homes remain

  @wip @slice-35 @ID-CODEXHOME-09 @engine @linux
  Scenario: Pinned Codex starts and resumes against a fake service without unwanted startup activity
    Given a CI Codex 0.167.0 image an approved Codex home a controlled Responses endpoint and independently observable incidental-service recorders
    When product runs query Codex effective settings then execute and resume a deterministic fake turn and an independent unseeded control is observed
    Then Codex reports all codex-v1 effective settings and disabled plugin features while the unseeded control reports differing defaults
    And both fake turns complete without login onboarding or tool approval input and their tool calls write and reread the same private home marker
    And only the expected fake model requests occur with no update analytics telemetry or plugin startup request through normal shutdown
    And recorder canaries are observed separately and no real service credentials or account was used and product session cleanup is confirmed

  # A real archive extractor must refuse symlinks and replace a regular file
  # without writing through another hard link. Pure metadata fakes cannot prove it.
  @wip @slice-35 @ID-CODEXHOME-10 @engine @linux
  Scenario Outline: Native seed publication cannot overwrite an unrelated target
    Given a disposable CI retained Codex home with "<entry>" an unrelated sentinel and an independent safe Codex control
    When the product attempts native seeding and the control runs before fixture cleanup
    Then the native result is "<result>" and the safe control starts with verified codex-v1 settings
    And the unrelated sentinel and auth links retain their original bytes metadata and targets and the home is retained
    And no refused worker starts and all owned session resources are confirmed absent before fixture cleanup

    Examples:
      | entry                                   | result                                             |
      | config symlink to a checkout sentinel   | run.home-refused with exit 1 before write or start |
      | Codex directory symlink to the checkout | run.home-refused with exit 1 before write or start |
      | config hard link to a home sentinel     | exit 0 with a distinct generated config inode      |
