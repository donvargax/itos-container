@phase-1
Feature: Hardened offline foreground run
  An approved run executes explicit argv in one owned offline worker.
  Launch uses frozen settings and verified ownership; cleanup retains the
  observed workload result and reports failures without claiming removal.
  Isolation holds before the first workload instruction; foreground exit ends
  the worker and its temporary storage while preserving checkout edits.
  Slice-10 owns boundary and durable-ownership refusals (02/03), literal argv
  and foreground streams (06/07), and the explicit shell workspace (13).
  Slice-13 owns the launch and cleanup contract (01/04/05/15).
  Slice-14 owns native isolation and teardown proof (08/09/10/11/12/14/16).

  # Authorities: docs/charter.md, docs/plans/offline-run-lifecycle.md (AC-07/08,
  # offline AC-10/11, AC-21/22/23/25/37), docs/architecture.md (Slices), and
  # docs/architecture/arc42.md, Foreground run and the person's A2/A3/A4.
  # Settled: Linux amd64/native Docker; NetworkMode none inspected before start;
  # effective host UID/GID, never root; read-only root; checkout-only host mount;
  # CapDrop ALL and no-new-privileges; pids 4096; no memory/CPU quota claim.
  # A2 excludes persistent worker volumes/profiles/caches: checkout plus tmpfs.
  # No hooks, proxy, service container, session network, published ports, image
  # acquisition, credentials, host helpers, auto-remove or restart policy.
  # A3 leaves a surviving worker isolated after controller death until stop.
  # Slice-10 review regressions in foreground_recovery.feature now cover local
  # signal handling, forwarding failures and checkout-independent owned cleanup.
  # Slice-12 retains native loss/cancellation/signal proof and aggregate
  # 10 s call / 120 s startup / 30 s teardown budgets, not a workload time limit.
  # These budgets still constrain calls here; no unbounded controller operation.
  # Slice-11 owns status/stop, public session IDs and executable retry guidance.
  # This increment retains and reports the existing safe protected intent
  # reference on incomplete cleanup; it MUST NOT advertise an unbuilt stop retry.
  #
  # Extend internal/slices/run/{command,domain,port,<named adapters>} and shared
  # internal/engine/session/{domain,port,<named adapters>}. Reuse the actual
  # approval/readiness/snapshot and durable ownership contracts from slices 8/9.
  # Only cmd assembles; domain/porttest fakes do no I/O. Shared contracts run
  # against memory fakes and adapters; adapters' HTTP fixtures stay test support.
  # Dependency choice: existing github.com/moby/moby/client v0.6.2 and api
  # v1.56.1 provide create/inspect/attach/start/wait/remove and stream framing;
  # reuse their maintained APIs instead of a shell CLI or handwritten client.
  # Use existing Go 1.27.3 and golang.org/x/sys v0.48.0 adapters for filesystem,
  # process and descriptor work; no new library/service or allowlist expansion.
  # Preserve all dependency notices and keep OS-dependent logic value-driven.
  #
  # Concrete runtime choices within the accepted design:
  # - Bind the safely revalidated checkout root read/write at /workspace with
  #   private propagation and without importing nested host mounts. workspaceFolder
  #   selects the existing working directory, never another mount source.
  #   Revalidate root/.git/workspace identity at the
  #   mount/launch boundary; never follow a replacement symlink or create a missing
  #   source. Frozen policy values do not excuse stale filesystem identity.
  # - Add one explicit tmpfs at /tmp, rw,nosuid,nodev,mode=1777. HOME and TMPDIR
  #   are /tmp; tools may make private subdirectories there. No persistent volume
  #   or host tmp/home mount. Do not claim a tmpfs memory quota. Engine-managed
  #   proc/sys/dev, /dev/shm and generated hosts/hostname/resolv.conf files are
  #   ordinary runtime facilities, not extra configurable host-source grants.
  #   The requested mount set has exactly the checkout bind and this tmpfs.
  #   A host-side nested mount below checkout is not another authorized source.
  # - Override image User, WorkingDir, Entrypoint and Cmd with frozen host UID:GID,
  #   workspaceFolder, an empty entrypoint and the complete literal supplied argv.
  #   Image entrypoints and default commands never precede or wrap the workload.
  #   Use the frozen immutable image config ID, never a tag or replacement image.
  #   Preserve approved image environment defaults except controller HOME/TMPDIR;
  #   overlay approved literal containerEnv. Forward no controller environment,
  #   supplementary groups, engine socket or SSH agent. No implicit shell.
  # - Use private PID/IPC/UTS/mount/network namespaces, no host namespace modes,
  #   no privilege/device/capability additions and no disabled engine confinement.
  #   Runtime probes cover these namespaces; no user-namespace support claim.
  # - Attach stdin/stdout/stderr before start, with engine TTY disabled, preserving
  #   separate byte streams even when redirected. No new TTY option is specified.
  #   stdin EOF closes only worker stdin, not output drains or the session. Drain
  #   final stdout/stderr after observed exit; controller diagnostics use stderr.
  #   A successful run adds no controller bytes to workload stdout.
  #
  # Required sequence: normal invocation/preflight/freeze/publication, durable
  # intent, one unstarted create, fresh full-ID inspection, durable concrete
  # ownership, full boundary verification, attach, start, observed primary exit,
  # owned cleanup, full-ID absence confirmation and durable completion. Inspect
  # actual configuration, not just the outgoing create request. No ready/start
  # on absent, contradictory or unavailable observations; no implicit retry create.
  # Reuse slice-9 ownership bindings before each mutation. Run-capable create and
  # cleanup now recognize exactly the frozen checkout/tmpfs boundary and a worker
  # which ran; do not simply remove the existing no-mount/never-started guards.
  # Keep slice-9's unstarted-only contract intact as its own restricted operation.
  # Cleanup of run resources must still refuse foreign/mismatched identity and
  # revalidate original snapshot/daemon/full ID. It need not reread current policy
  # or approval. No deletion by name, prune, shared-image or volume deletion.
  # A boundary mismatch on an otherwise owned unstarted resource forbids start
  # but permits safe owned cleanup; a mismatched ownership identity forbids mutation.
  #
  # Observed nonzero primary exit wins over cleanup failure, including 1/2/3/70/75.
  # Zero is returned only for known workload success AND confirmed durable cleanup.
  # Ordinary primary exit also removes still-running descendants; their lifetime
  # cannot keep the session alive. Required removal may terminate those descendants
  # only on the revalidated owned worker. No detached mode or exec command here.
  # Cleanup cause mapping follows T-11: ownership refusal 1, invalid recovery
  # configuration 2, missing engine environment 3, unclassified controller error
  # 70, transient engine failure 75. Keep workload and cleanup outcomes separate.
  # New plain stderr rules are run.isolation-unverified (observed mismatch, 1),
  # run.isolation-inspection (unavailable observation, 75), and run.cleanup-<cause>
  # with causes ownership/configuration/environment/internal/temporary as below.
  # Existing prerequisite/snapshot/ownership failures keep their specific causes.
  # No raw argv, environment, response body or copied workload output belongs in
  # diagnostics or ownership records. Deliberate workload streams are not logs.
  #
  # Supersession for the IMPLEMENTER, not edits in this specification commit:
  # ID-RUN-06/07/13/19 and ID-APPROVAL-26 lose ONLY their temporary startup-unavailable
  # expectations and snapshot-only effect limits. ID-RUN-14's first successful run
  # can now create/execute/clean; its later refusal still has no startup effects.
  # Update those local HTTP fixtures to model actual owned startup/cleanup and
  # assert exact argv, matching approval, immutable snapshots, fresh preflight,
  # no consent/pull/build/host execution and only owned resource effects. They
  # remain local, never silently become real-engine tests or @wip regressions.
  # For executable-looking --json/--help/--, the fixture observes that exact Cmd;
  # wrapper parsing cannot consume it. Do not require a real executable of that
  # name on the host. This file supplies the new actual execution evidence.
  #
  # Fixture/step handoff: EVERY step phrase in this file is NEW. Givens prepare
  # disposable checkout/state, independent canonical approval seeds, trusted probe
  # files and fault/barrier configuration only. They never invoke approve, product
  # run or production code to manufacture their expected results. Checkpoint after
  # fixture edits; record later fixture writes separately from product effects.
  # Never touch operator state. All actions preserve early returns as observations
  # for Then; missing launch/inspection must not hang or fail a setup prerequisite.
  #
  # Local cases use a test-only cmd assembly of the real run command/domain and
  # session engine, with both I/O-free fake contracts and the disk/SDK adapters
  # against a Unix HTTP Engine API fixture. No real engine, production injection
  # switch or product imports in feature steps. Wire a compileable unavailable
  # boundary in the red step commit if needed; it must return data, not fake success.
  # Positive controls traverse the same assembly and adapters as fault rows.
  # Exact event ordering comes from port-bound observation and real disk durability
  # operations; a file merely existing is not fsync evidence. Capture every request,
  # including refused/failed requests and hijacked attach streams. Fresh inspection
  # and recorded identity must precede each start/removal; create replies are not proof.
  #
  # @engine uses only features/README.md's guarded Linux amd64 CI tier and T-20's
  # independently inspected cached digest/config ID/host UID/GID. Extend existing
  # test-only native request recording for attach/start/wait/remove, preserving
  # doctor's read-only recorder. Runtime cases invoke the actual product binary.
  # Audit resources by independently observed full IDs and preserve unrelated
  # resources, image references, approvals and snapshots. Always-cleanup is bounded,
  # identity-checked harness work, recorded AFTER product observations and never
  # counted as product cleanup. No image pull/build/substitute in any scenario.
  # Trusted probes may be standard-library Go helpers built by the harness into
  # the disposable checkout, run only in the worker, not new image contents or
  # consumer scripts on the host. No test-only helper is production code.
  #
  # Network probes are the explicit executable's earliest application operations,
  # before any blocking handshake; repeat them during foreground execution. There
  # is no earlier image entrypoint or setup. Use per-attempt nonces and listener
  # receipts, not exit status alone. Each denied route has the SAME probe/client
  # reach it from a trusted positive-control context before and after the worker
  # attempt. Use fixture-owned dual-stack listeners/resolvers and an isolated
  # control network; never query public DNS, real cloud metadata or real credentials.
  # The fixture may own a bridge/control container using the SAME cached image;
  # those are test controls, never product-created resources or a product topology.
  # Arrange synthetic private/link-local metadata routes entirely inside that
  # controlled topology. Inability to establish any named positive control fails
  # the test, never skips it or turns a timeout/NXDOMAIN into isolation evidence.
  # Worker loopback can reach its own listeners; it cannot reach host/control
  # listeners, including services on the host's loopback at the same port.
  # Every denial assertion requires a complete, nonempty report identifying each
  # planned route and both phases with an attempted operation and its result;
  # absent execution/reports or an empty receipt list alone cannot satisfy it.
  #
  # Commit step definitions red first with all these @wip tags retained. Local
  # assertion-level red/green runs remove @wip for the run; engine red runs use
  # only the coordinator's guarded CI red_scenarios dispatch, as README specifies.
  # Every example must fail at its named Then before implementation. Undefined
  # steps, unavailable fixture controls and today's startup refusal are not
  # recorded red evidence until the assertions actually run. This spec records
  # expected failures only and supplies no runtime or mutation evidence.
  # Coordinator exception (2026-11-06), also standing for slice-14: cases already
  # passing against earlier slices are accepted regressions after confirming they
  # check product behavior; record the earlier slice above the tag and turn it live.
  # Engine red checkpoint for 06/07/13 is accepted from run 42138046011 at ba71f14;
  # their passing runtime evidence still belongs to CI at the actual pushed head.

  # Slice-13 carries ID-OFFLINE-01/04/05/15 from slice-10, as q-7 directs.
  # These IDs were allocated in 7920219 and retagged, still @wip, in dad6d1f;
  # they are preserved, not newly minted or replacements for sibling scenarios.
  # Specification source: slice-10 at 2ce7bec52abc114008bd9e17d805841fdbc2def1.
  # Main at the specification base ffe148d5a2cfcaad65bd9df721b053105e385c25
  # still stopped after preflight. Slice-10 has since merged into main.
  # Integration retains one copy of each ID and this slice-13 handoff, with
  # all slice-10/14 scenarios and their dependency-branch tag states intact.
  # The following handoff applies only to slice-13 cases 01/04/05/15.
  #
  # Authorities: docs/charter.md, docs/plans/offline-run-lifecycle.md (AC-05,
  # AC-21/22/23/41), docs/architecture.md (Slices), docs/architecture/arc42.md,
  # and the person's A2/A3/A4. The source feature and decision 0008 on slice-10
  # supply the settled foreground boundary. This spec adds no security mechanism.
  # Linux amd64/native Docker is the target, not a runtime support claim.
  # All four cases are LOCAL @linux contracts: no real engine in setup, actions
  # or cleanup. Slice-14 owns native isolation and teardown proof; slice-12 owns
  # cancellation, controller loss and aggregate budgets; slice-11 owns status/stop.
  # Calls/startup/teardown remain bounded to 10/120/30 s, never a workload limit.
  # No hooks, image acquisition, proxy, service/network creation, ports,
  # credentials, persistent worker state, detached execution or exec command.
  # Checkout edits and protected approval/snapshot/ownership records survive.
  #
  # Extend internal/slices/run/{command,domain,port,<named adapters>} and shared
  # internal/engine/session/{domain,port,<named adapters>}; only cmd assembles.
  # Reuse slice-8 preflight/frozen publication and slice-9 durable ownership.
  # Preserve slice-9's separate unstarted-only capability and cleanup contract.
  # Reuse the pinned github.com/moby/moby/client v0.6.2 and api v1.56.1 for
  # engine calls and stream framing, Go 1.27.3 and golang.org/x/sys v0.48.0
  # for existing filesystem/process adapters. No new dependency or service;
  # no shell client, handwritten protocol or standard-library allowlist change.
  # Preserve licenses/notices and value-driven OS logic. Domains and porttest
  # fakes do no I/O; run shared contracts against both fakes and adapters.
  #
  # Exact frozen runtime boundary, inherited from slice-10:
  # - Immutable image config ID, effective non-root host UID:GID, no extra groups;
  #   frozen workspaceFolder, literal supplied argv, empty image entrypoint.
  #   Disable image healthchecks and the daemon init wrapper. Preserve approved
  #   image environment defaults, overlay literal containerEnv, then set HOME
  #   and TMPDIR to /tmp. No controller environment, host helper or chown.
  # - Sole host source: revalidated checkout bound read/write at /workspace,
  #   private/nonrecursive, no imported nested host mounts or source creation.
  #   workspaceFolder selects an existing directory beneath it, not a mount.
  #   One requested tmpfs at /tmp, rw,nosuid,nodev,mode=1777. No named/anonymous
  #   volumes, extra host directory, host /tmp/home/socket or persistent cache.
  #   Engine-managed proc/sys/dev, /dev/shm and generated hostname/hosts/resolver
  #   files are runtime facilities, not configurable host-source grants.
  # - NetworkMode none, no extra network attachment or published ports; private
  #   namespaces, read-only root, CapDrop ALL, no-new-privileges, pids 4096.
  #   No privilege/device/capability additions or disabled engine confinement;
  #   no memory/CPU/tmpfs quota or user-namespace support claim.
  # - No auto-remove/restart policy. Engine workload logging disabled. Attach
  #   separate streams before start with TTY disabled. Observed primary exit,
  #   final output drain and confirmed durable owned cleanup precede success.
  #
  # Required ordering: preflight and durable frozen selection, durable intent,
  # one unstarted create, independent full-ID ownership inspection, durable
  # concrete-ID publication, actual boundary verification, attach, fresh
  # identity/boundary revalidation before start, observed exit, owned removal,
  # full-ID absence confirmation and durable completion. Never trust only a
  # create response or outgoing request. No implicit create retry. Revalidate
  # trusted record/snapshot, daemon and full resource identity before mutation.
  # A boundary mismatch on an owned worker prevents launch but permits owned
  # cleanup; a foreign/mismatched ownership identity permits no mutation.
  # Cleanup uses original protected bindings, not current request/approval values.
  # No name-only removal, prune, shared-image deletion or deletion of host files.
  #
  # Step handoff: these four cases still need step definitions. Reuse the
  # existing feature helpers in offline_local_steps_test.go,
  # session_probe_test.go, session_fixture_test.go and offline_contract_engine_test.go
  # from slice-10, plus its cmd/itos-container/offline_contract_test.go assembly.
  # Those presently bind OFFLINE-02/03, not these four IDs. Extend their protocol
  # and observations; feature steps import no product packages and assert only
  # actual command results, file effects and port/HTTP observations. Do not return
  # a precomputed RunResult or derive expected settings from a create request.
  #
  # Each Given only prepares disposable checkout/state, independently encoded
  # canonical approval/snapshot fixtures, cached-image API responses, synthetic
  # markers and fault/barrier configuration. It invokes neither approve nor run,
  # and never calls production canonicalization to manufacture expected records.
  # No operator state or real credentials. Checkpoint bytes/modes after setup;
  # keep later fixture writes distinct from product effects in the event journal.
  # The When executes the real command, preflight, session domain and classifier
  # through a test-only cmd assembly with both in-memory ports and disk/SDK
  # adapters against a Unix HTTP Engine API fixture. No product injection flag.
  # Both backends traverse production decisions; the SDK tier observes actual
  # create/inspect/attach/start/wait/remove calls and real disk durability calls.
  # In-memory ports never perform filesystem/process/socket work themselves.
  # Every fault case has a fault-free control through the same backend/assembly.
  #
  # Actions retain early returns, missing observations and unreached barriers as
  # data for Then. Race each barrier with subprocess exit and a finite fixture
  # deadline; no missing start can hang or become a Given failure. A successful
  # always-refuse implementation cannot satisfy any case's positive control.
  # Record request failures as well as successes and attach-stream operations.
  # File existence alone is not durability: require successful write, file fsync,
  # atomic publication and parent fsync in the same ordered operation trace.
  # Harness cleanup happens after product observations and cannot satisfy Then.
  #
  # Historical evidence handoff at the slice-13 specification commit:
  # Before slice-10 merged, startup-unavailable was the expected failure at
  # 01's first launch assertion, 04's first A-launch assertion, every 05 row's
  # exit-1 identity assertion and every 15 row's separate cleanup-rule assertion.
  # A refusal or undefined step alone is not evidence: first commit steps with
  # @wip retained, remove @wip only for local assertion-level red/green runs, and
  # record each result. Slice-10 already implements Runner, Workspace, frozen
  # create settings and most result precedence; do not claim they are absent.
  # The reviewed slice-10 head lacked cleanup-environment/3 for case 15.
  # Recheck all rows against the merged dependency before production edits.
  # Settled coordinator decision q-9 (extending q-6): a case already passing
  # against an earlier slice is accepted regression coverage after confirming
  # it checks product behavior. Comment its origin above the tag and turn it live;
  # never force a failure, rewrite the assertion or claim red for that case.
  # The q-7 split does not waive slice-13's evidence requirement. No engine or
  # local mutation run is needed here; CI owns changed-code proof at implementation.

  # Owned launch ordering and refusal boundaries. The positive row must run;
  # an implementation which always refuses cannot satisfy this contract.
  # The fixture seeds one approved checkout with a workspace subdirectory,
  # literal environment values, harmless helper sentinels and an unrelated
  # same-image worker with a different ownership tuple. Inspect requests AND
  # independently held observations against these inputs; record the full
  # create-to-durable-completion trace and both workload/result streams.
  # The local argv probe is a fixture engine response, never a host executable.
  # Regression from slice-9 durable ownership and slice-10 inspected launch;
  # passed through both local backends before slice-13 production changes (q-9).
  @slice-13 @ID-OFFLINE-01 @linux
  Scenario: Run verifies durable ownership and the full boundary before launch
    Given an offline run contract fixture with approved settings and successful engine observations
    When the offline contract executes an argv probe through both fake and disk SDK adapters
    Then each offline run starts exactly one inspected owned worker after durable intent, owned ID, boundary verification and attach
    And each create and inspection has the exact frozen image, UID GID, workspace, argv, environment and hardened runtime configuration
    And each offline run observes exit zero and confirms owned removal before returning zero
    And no offline operation acquires images, executes host helpers or changes unrelated resources

  # Change one inspected field after an otherwise correctly configured create.
  # "missing" and "unavailable" are observation faults, not a compliant default.
  # Full-ID ownership still matches, allowing cleanup without executing the worker.
  # Passed before activation: slice-10 commit f069faa already verifies the fresh
  # inspected boundary and refuses launch while retaining owned cleanup authority.
  # Accepted as regression coverage under the coordinator's 2026-11-06 decision.
  @slice-10 @ID-OFFLINE-02 @linux
  Scenario Outline: A created worker with an unverified boundary never starts
    Given an offline run contract fixture whose pre-start inspection has "<defect>"
    When the offline contract attempts launch through both fake and disk SDK adapters
    Then each offline run reports "<rule>" with exit <exit> before any start
    And each offline run attempted one create but no entrypoint, exec or workload instruction
    And each offline run cleans only the independently revalidated owned unstarted worker

    Examples:
      | defect                              | rule                      | exit |
      | network mode bridge                 | run.isolation-unverified  | 1    |
      | additional network attachment       | run.isolation-unverified  | 1    |
      | published port                      | run.isolation-unverified  | 1    |
      | root user                           | run.isolation-unverified  | 1    |
      | different primary GID               | run.isolation-unverified  | 1    |
      | supplementary group                 | run.isolation-unverified  | 1    |
      | writable root                       | run.isolation-unverified  | 1    |
      | extra host bind                     | run.isolation-unverified  | 1    |
      | different checkout bind source      | run.isolation-unverified  | 1    |
      | shared bind propagation             | run.isolation-unverified  | 1    |
      | missing tmpfs                       | run.isolation-unverified  | 1    |
      | named volume                        | run.isolation-unverified  | 1    |
      | anonymous volume                    | run.isolation-unverified  | 1    |
      | missing CapDrop ALL                 | run.isolation-unverified  | 1    |
      | added capability                    | run.isolation-unverified  | 1    |
      | missing no-new-privileges            | run.isolation-unverified  | 1    |
      | privileged mode                     | run.isolation-unverified  | 1    |
      | host PID mode                       | run.isolation-unverified  | 1    |
      | host IPC mode                       | run.isolation-unverified  | 1    |
      | host UTS mode                       | run.isolation-unverified  | 1    |
      | extra device mapping                | run.isolation-unverified  | 1    |
      | disabled seccomp confinement         | run.isolation-unverified  | 1    |
      | pids limit other than 4096           | run.isolation-unverified  | 1    |
      | restart policy always               | run.isolation-unverified  | 1    |
      | auto-remove enabled                 | run.isolation-unverified  | 1    |
      | image entrypoint retained           | run.isolation-unverified  | 1    |
      | changed command argument            | run.isolation-unverified  | 1    |
      | wrong working directory             | run.isolation-unverified  | 1    |
      | changed literal environment         | run.isolation-unverified  | 1    |
      | missing security configuration      | run.isolation-inspection  | 75   |
      | unavailable boundary inspection     | run.isolation-inspection  | 75   |

  # The write fault applies to every owned-record publication, including cleanup.
  # The complete pre-create intent survives. Clearing the fault is a separate
  # explicit cleanup action, not permission to launch after a failed publication.
  # Passed before slice-10: slice-9 established durable concrete-ID publication
  # before mutation and recovery from retained intent after a persistent fsync fault.
  # This accepted regression now exercises that contract through the foreground runner.
  @slice-10 @ID-OFFLINE-03 @linux
  Scenario: Run cannot execute a worker whose concrete ownership is not durable
    Given an offline run contract fixture with persistent owned-record fsync failure
    When the offline contract attempts launch and later retries cleanup after clearing the fault
    Then each first offline run reports ownership publication failure with no start or usable launch handoff
    And each first offline run retains the complete protected intent and independently observed unstarted worker
    And each later cleanup persists the reconciled ID before removal and confirms absence without starting work
    And the original run failure is not replaced by later cleanup success

  # Only request contents/current approval change; original path identities and
  # snapshot remain intact. One selected session uses A; a new invocation uses B.
  # Prepare /workspace/a and /workspace/b and distinct literal APP values in A/B.
  # Keep checkout identity, engine, cached image and effective UID/GID the same;
  # neither a host identity change nor a new permission is needed for this case.
  # Given stores A as active and B as independently encoded fixture material.
  # When pauses after A's durable selection, edits manifest bytes in place and
  # installs B's active approval as a separate fixture operation, then releases
  # A. Start a fresh invocation only after A finishes. The fixture never approves.
  # Observe both real starts, exact A/B bindings, untouched A snapshot bytes and
  # post-selection input reads for each invocation. Frozen snapshot reads and
  # filesystem identity checks remain allowed; re-reading request values is not.
  # Regression from slice-8 frozen selection and slice-10 launch consumption;
  # passed through both local backends before slice-13 production changes (q-9).
  @slice-13 @ID-OFFLINE-04 @linux
  Scenario: Launch consumes the selected frozen settings after checkout and approval edits
    Given an offline run contract fixture with independently seeded approved settings A and B
    When run freezes A and the fixture replaces manifest environment, workspace request and active approval with B before create and a fresh run follows
    Then the first offline worker actually starts with exactly A's image, UID GID, mounts, working directory and literal environment
    And the fresh offline worker starts with exactly B while A's snapshot remains unchanged
    And neither invocation rereads policy or manifest values after its frozen selection
    And both offline runs preserve ownership, isolation and confirmed cleanup

  # Unlike edits to request contents, replaced filesystem identity cannot be used
  # as the mount source. The When replaces paths inside a disposable parent only.
  # Every barrier races with early process exit so the red implementation reaches Then.
  # Given seeds a real embedded .git directory, an existing workspace subdirectory
  # and a distinct outside target/sentinel, all beneath a private fixture parent.
  # It only arms a barrier; When selects/freezes the valid original checkout.
  # Pause before the last pre-create identity check, or after unstarted create
  # and durable ownership but before the final pre-start identity check. Rename
  # the selected original aside and put an outside symlink at its old name.
  # Real disk cases perform these replacements; memory cases change the matching
  # identity observations without I/O. Keep snapshot, daemon and owned ID intact.
  # Require zero creates for before-create rows, one unstarted owned create for
  # after-create rows, no start in either, and owned removal/confirmed absence for
  # the latter. Outside bytes/modes stay unchanged, and no request uses that source.
  # Restore paths only in harness teardown AFTER observations; restoration cannot
  # rescue product cleanup. Do not relax ownership checks to clean a foreign ID.
  # All five rows regress slice-10 workspace pinning and identity revalidation,
  # including its checkout-independent cleanup fix; baseline passed (q-9).
  @slice-13 @ID-OFFLINE-05 @linux
  Scenario Outline: Checkout substitution cannot redirect the worker mount or working directory
    Given an offline run contract fixture with a launch barrier at "<boundary>"
    When the offline run reaches that barrier and the fixture replaces "<path>" with an outside symlink before release
    Then the offline run refuses the changed checkout identity with exit 1 and no start
    And the outside target is neither mounted, modified nor used as the working directory
    And any already created owned worker is cleaned without touching the substituted source

    Examples:
      | boundary                     | path                  |
      | before create                | checkout root         |
      | before create                | embedded .git         |
      | before create                | workspace subdirectory |
      | after create before start    | checkout root         |
      | after create before start    | workspace subdirectory |

  # Literal arguments and streams. The JSON DocString is fixture data decoded
  # directly into exec argv; never pass it through commandWords or a host shell.
  # The trusted probe emits a length-delimited argument report, compared byte for
  # byte; glob matches and shell marker filenames exist to expose interpretation.
  @slice-10 @ID-OFFLINE-06 @engine
  Scenario: The worker receives every supplied argument without shell interpretation
    Given a CI offline checkout with the cached image and an argument-reporting probe
    And the offline argument fixture contains these exact JSON arguments
      """
      ["", "two words", "tab\tvalue", "line\nvalue", "雪", "'quoted'", "\"double\"", "*?[ab]", "$HOME", "$(touch shell-marker)", "`touch backtick-marker`", "; touch semicolon-marker", "> redirect-marker", "--help", "--json", "--no-json", "--", "--dangerously-skip-permissions"]
      """
    When the product runs the offline argument probe with that argv
    Then the worker report contains exactly those argument bytes in that order with no inserted or missing argument
    And no offline shell marker exists in the checkout or host fixture
    And the product returns zero only after that worker is independently confirmed absent

  # The probe emits readiness on BOTH streams, then waits for one stdin frame.
  # The harness sends it only after observing both frames while the child lives.
  # EOF follows that frame; the probe emits trailing binary frames and exits.
  # Include NUL and non-UTF8 bytes in redirected frames; no JSON/text rewriting.
  @slice-10 @ID-OFFLINE-07 @engine
  Scenario: Redirected foreground streams remain live, separate and complete
    Given a CI offline checkout with the cached image and a framed stdin stdout stderr probe
    When the product runs the offline stream probe and the harness answers its live handshake then closes stdin
    Then both readiness streams arrive before the input reply and before workload exit
    And the worker receives the exact stdin frame and EOF
    And stdout and workload stderr retain their exact separate frames including the trailing bytes
    And controller diagnostics occur only on stderr and the product returns zero after confirmed cleanup

  # Slice-14 carries ID-OFFLINE-08/09/10/11/12/14/16 from slice-10, as q-7 directs.
  # These IDs were allocated in 7920219 and retagged, still @wip, in dad6d1f.
  # Preserve their IDs, scenario text and examples; no new ID is needed here.
  # Reviewed main f13be16397aafad4e7d4fb18e52916fed3cc3239,
  # slice-10 d726afdd003d78559e4684f3ed15177be3aea9e1, and
  # slice-13 b9fa07d78e213d08f94bff3840e46e3c9536ac6b.
  # Slice-10 and slice-13 are now merged into main. This feature retains one
  # copy of each ID, each owner's handoff and the dependency's live/wip tags.
  # The following handoff applies only to slice-14 cases 08/09/10/11/12/14/16.
  # Isolation cases remain between argv/streams and the explicit-shell case;
  # teardown cases remain beside the exit/cleanup block.
  #
  # Authorities: docs/charter.md, docs/plans/offline-run-lifecycle.md (AC-07/08,
  # offline AC-10/11, AC-22/23/25/37), docs/architecture.md (Slices), arc42's
  # Foreground run and the person's A2/A3/A4, plus q-7's ownership split.
  # Consume slice-13's ordered launch/frozen settings/cleanup contract and
  # slice-10's decision 0008; this spec adds no new security mechanism.
  # Slice-10 owns OFFLINE-02/03/06/07/13 and its local foreground regressions;
  # slice-13 owns OFFLINE-01/04/05/15; slice-11 owns public status/stop and retry
  # guidance; slice-12 owns cancellation, controller loss and aggregate budgets.
  # Read recovery.feature at slice-11 9bb080f and lifecycle.feature at slice-12
  # d8e5823 when integrating. Their commands are not prerequisites to these
  # seven cases. LIFECYCLE-15 later reuses these independent boundary controls.
  # A3 leaves a surviving worker isolated until explicit stop, not a watchdog.
  # Calls/startup/teardown retain 10/120/30 s limits, never a workload time limit.
  #
  # Settled runtime boundary, required from the first executable instruction:
  # - Linux amd64/native Docker only; immutable approved cached image config ID;
  #   effective non-root host UID:GID, no forwarded supplementary groups or chown.
  #   Override image user, entrypoint, command and working directory. Disable
  #   image healthchecks and daemon init, so no helper precedes the explicit argv.
  # - Read-only image root; sole host source is the revalidated checkout mounted
  #   read/write at /workspace, private/nonrecursive, without nested host mounts.
  #   workspaceFolder is an existing directory beneath that mount, not a source.
  # - One requested tmpfs at /tmp, rw,nosuid,nodev,mode=1777; HOME and TMPDIR are
  #   /tmp. A2 excludes persistent worker volumes/caches/profiles. Engine-managed
  #   proc/sys/dev, /dev/shm and generated hosts/hostname/resolver files are runtime
  #   facilities, not extra configurable host-source grants. No tmpfs quota claim.
  # - CapDrop ALL, no-new-privileges, private PID/IPC/UTS/mount/network namespaces,
  #   pids 4096, no privilege/device/capability additions or disabled confinement.
  #   No memory/CPU quota or user-namespace support claim.
  # - NetworkMode none, inspected before start; no extra network attachment,
  #   published port, proxy, guard or product-created network/service container.
  # - Preserve approved image environment defaults with literal containerEnv
  #   overlays, then controller-owned HOME/TMPDIR. Forward no host environment,
  #   credentials, home, engine socket or SSH agent. No implicit shell or hooks.
  # - Attach separate non-TTY streams before start; no daemon workload logging,
  #   auto-remove, restart, detached mode or exec command. No image acquisition.
  #
  # Extend internal/slices/run/{command,domain,port,<named adapters>} and shared
  # internal/engine/session/{domain,port,<named adapters>} only as the evidence
  # requires. Only cmd assembles. Reuse approval/readiness/frozen storage and
  # slice-9's durable ownership, keeping its unstarted-only capability intact.
  # Domains and port/porttest fakes do no I/O; shared contracts run unchanged
  # against fakes and adapters. Native probes cannot replace local contract,
  # adapter, changed-line coverage or CI mutation evidence for any product fix.
  # Reuse pinned Moby client v0.6.2/api v1.56.1 for engine operations and stream
  # framing, Go 1.27.3 standard-library probe/transport helpers and x/sys v0.48.0
  # for native adapter calls. No new library/service, handwritten engine client,
  # shell engine invocation or allowlist change. Preserve notices and OS-as-value
  # logic. Test support and trusted probes never enter the production binary.
  #
  # Fixture and step handoff: all slice-14 phrases need new step definitions.
  # Reuse slice-10's offline_runtime_steps_test.go,
  # offline_runtime_fixture_test.go, offline_runtime_engine_test.go,
  # offline_runtime_probe_test.go and offline_runtime_assertions_test.go mechanisms,
  # plus session_fixture_test.go's independent records and approval seeds.
  # Those steps currently bind OFFLINE-06/07/13, not these seven IDs. Extend their
  # observations; do not call their product assertions from a Given. Feature
  # steps invoke the real binary and read effects, never import product packages.
  #
  # Every Given prepares only disposable checkout/state, independently encoded
  # approval data, trusted probe files, controls, sentinels and fault settings.
  # No approve command, product run or production canonicalizer manufactures
  # setup or expected data. No operator state or real credentials. Record files,
  # owners/modes, resources and image references after fixture preparation; keep
  # subsequent harness writes separate from product effects. Compile reviewed
  # probe source into the disposable checkout; execute it only in the subject
  # worker or the expressly described trusted control container, never on host.
  # Use synthetic markers and report their presence/result, not arbitrary host
  # file contents, environments or engine response bodies.
  #
  # All setup, action and cleanup needing an engine stays in features/README.md's
  # guarded CI-only Linux amd64 tier. Independently inspect T-20's supplied cached
  # digest/config ID/UID/GID; refuse absent or incompatible inputs, never acquire
  # or substitute an image. This is not permission for agents to access a socket.
  # Extend the existing test-only native request recorder, including attach/wait,
  # without weakening doctor's or slice-9's restricted recorders. Record refused
  # requests as failures; a recorder refusal cannot count as product isolation.
  # Read native observations independently of product responses and fixture
  # expectations. The product still takes its normal validation/approval path.
  #
  # Whens retain early refusals, missing reports and unreached barriers as data
  # for Then. Race every barrier against subprocess exit and a finite fixture
  # deadline. A never-started probe must reach the named behaviour assertion;
  # undefined steps, broken controls and setup errors are not red evidence.
  # Each denial needs a complete, nonempty attempted-operation report AND its
  # working positive control. Use unique per-attempt nonces, exact route/phase
  # inventories and listener receipts. Exit status, silence, ENOENT, timeout or
  # an empty receipt list alone cannot establish the requested isolation.
  # Controls run before and after the corresponding attempt with the same
  # executable/client and target operation. Never loosen the subject's settings.
  # The control checks IPv6 availability in its own network namespace, enables
  # its namespace's IPv6 interfaces and waits for tentative addresses to settle.
  # It never changes the runner or worker network. Missing kernel IPv6, setuid,
  # nested-mount, resolver or process controls fails with the actual setup error;
  # no skipped route, weaker fallback or manufactured denial result.
  #
  # Observe primary exit, full-ID absence, protected completion record and
  # unrelated resources before harness teardown. The absence record has the
  # original project, fingerprint, image and concrete ID, not just an empty list.
  # Slice-9/13 local contracts prove write/fsync/rename/parent-fsync ordering;
  # runtime re-reading a complete record does not independently prove fsync.
  # Never infer removal from DELETE success, a name lookup or a transport error.
  # Bounded identity-checked harness cleanup runs even after failed assertions,
  # but only AFTER product observations and never supplies product evidence.
  # No prune, shared-image removal, foreign-resource deletion or host-file cleanup
  # by the product. Preserve existing approval/snapshot bytes and checkout edits.
  #
  # Evidence handoff: this specification supplies no executed red/green.
  # Before slice-10 merged, startup-unavailable was the expected failure at
  # 08's process/child identity, 09's two root/write reports, 10's local controls
  # plus denied host access, 11's worker route reports, 12's worker resolver
  # reports, 14's observed primary exit plus cleanup, and 16's observed child
  # exit plus separate cleanup failure. Even 14's status-3 row requires an
  # observed workload exit; wrapper 3 cannot pass. Slice-10 already implements
  # much of this boundary and exit precedence; slice-13 completes the local
  # launch/cleanup contract. Run complete steps before classifying any result.
  # Commit complete steps first, retaining @wip @engine, then hand them to the
  # coordinator for guarded CI red_scenarios dispatch of all seven IDs/rows.
  # Failures must reach the behaviour assertion; undefined steps are not evidence.
  # Standing coordinator decision (q-6, extended to slice-14): an already-passing
  # case against an earlier slice is accepted regression coverage. Confirm it
  # checks product behaviour rather than its fixture, comment above its tag which
  # earlier slice it exercises, turn it live and continue without a new decision.
  # Stop only if setup fails and no step can fix it, or if the scenario cannot
  # fail against a wrong build. Never weaken a case or manufacture a red result.
  # Activation needs assertion-level red/green evidence or that verified
  # regression basis, then green CI at the actual pushed head without red input.
  # Coordinator run 42195113834 at 744f548 (2026-11-06 09:45) passed 08/09/10/11/
  # 14/16 against earlier slices. Their positive controls are required below;
  # add missing controls and activate these regressions under q-6. Scenario 12
  # failed during listener/resolver setup, NOT at a behavior assertion. Repair
  # its control and stop for another coordinator engine run of 12, still @wip.
  # The new head still needs green CI; no runtime/support or mutation claim here.

  # Effective identity comes from the real host process, not fixture environment.
  # Given records its UID/GID and groups, installs a compiled report-only root-owned
  # setuid helper in the checkout, and prepares misleading UID/GID/USER inputs.
  # Record a host supplementary group distinct from the primary GID so accidental
  # inheritance is observable. Group lists may include the primary GID but no
  # other host group. No product chown may be hidden among fixture preparations.
  # When launches the controller with those misleading environment strings but
  # unchanged effective IDs/groups. The worker first reads process/child IDs, all
  # capability sets, NoNewPrivs and actual cgroup pids.max, then attempts setuid(0),
  # the setuid helper and a privileged mount. Observe actual syscall outcomes,
  # not installed-tool names or flags.
  # A separate isolated control executes the SAME setuid file without NNP and
  # must elevate its effective UID before and after the subject attempt. It only
  # reports IDs, uses the same cached image, has no network or extra host mounts,
  # and never executes on host. A nosuid/nonexecuting helper is a broken control.
  # Another control grants root/SETUID/SYS_ADMIN in its private namespaces and
  # disables its AppArmor mount denial. The same setuid and mount syscalls must
  # succeed before and after the subject; no privilege is added to the worker.
  # Missing sudo is not proof; no 4096-process exhaustion or quota assertion.
  # Accepted q-6 regression of slice-10's UID/GID, capabilities, NNP and pids
  # boundary (run 42195113834). Same-file setuid and root/mount controls bracket
  # the worker attempts; observed identity/flags and successful elevation fail.
  @slice-14 @ID-OFFLINE-08 @engine
  Scenario: The first workload process has the host non-root identity without elevation
    Given a CI offline checkout with the cached image and identity capability and setuid probes
    And misleading host identity environment and an unrelated supplementary group are recorded as fixture inputs
    When the product runs the offline identity probe as its first executable
    Then the process and its child have exactly the effective host UID and GID without host supplementary groups
    And the runtime has no effective permitted inheritable ambient or bounding capabilities and NoNewPrivs is one
    And root identity and privileged mount attempts fail and the setuid probe cannot elevate
    And the process reads an effective pids limit of 4096 without any claimed memory or CPU quota
    And the product confirms owned cleanup without host chown or unrelated file changes

  # Given prepares a writable checkout marker and an empty mountpoint below it,
  # then a read-only fixture bind there backed by an outside synthetic sentinel.
  # Record the native mount and its readable nonce before and after both runs;
  # absence of a working nested mount cannot prove nonrecursive isolation.
  # When invokes the product twice with independent session nonces. Each probe
  # observes kernel read-only root mount flags AND attempts an image write (for
  # example creating /slice-14-root-marker). A permission error without the
  # read-only mount observation is insufficient. A root-capable control with a
  # writable image root must write/read the SAME path before and after; the
  # subject still requires kernel ro mount flags as well as its denied write.
  # Successful checkout and tmpfs writes provide the non-root write controls.
  # HOME/TMPDIR remain /tmp; use different paths within it to distinguish the
  # home and temp markers. Verify tmpfs type/options and owner identity, not only
  # environment strings. Run two receives run one's names and must attempt reads;
  # it sees the exact persisted checkout nonce and no prior temporary nonce.
  # The underlying empty directory, not the host mount's sentinel, is visible.
  # Audit both full IDs, mounts and volume inventory before unmounting the fixture.
  # Harness unmount/removal cannot establish ephemeral product teardown.
  # Accepted q-6 regression of slice-10's read-only root, nonrecursive checkout
  # bind and tmpfs, plus slice-13 cleanup (run 42195113834). Writable-root,
  # checkout/tmpfs writes and the readable nested mount are positive controls;
  # writable root flags, imported nested bytes or retained temporary files fail.
  @slice-14 @ID-OFFLINE-09 @engine
  Scenario: Checkout edits persist while writable home and temporary files are ephemeral
    Given a CI offline checkout with the cached image and a filesystem probe
    When the product runs the offline filesystem probe twice with distinct session nonces
    Then both processes observe a read-only image root and fail to modify image files
    And both processes can write the checkout and their tmpfs HOME and TMPDIR as the host UID GID
    And the second process sees the first checkout edit but none of its temporary or home files
    And native inspection and mount observations show only the approved checkout host source and container-owned temporary storage
    And neither worker sees the outside marker from the fixture's nested host mount
    And both exact worker IDs and their ephemeral storage are absent after product cleanup with no created persistent volume

  # Given seeds known-readable synthetic host-only files outside checkout, a
  # disposable host home, protected approval/ownership paths, harmless Unix
  # listeners and a live host process sentinel with a nonce-bearing identity.
  # Host controls read/connect before and after; worker-local files and a Unix
  # listener exercise the same probe operations in tmpfs. Pass host paths as data,
  # never mounts or forwarded environment. Use a harmless fixture socket for the
  # SSH-agent case, never a real agent or credentials; any engine-socket connect
  # success fails without sending a request. Refuse/report unexpected sensitive
  # access without logging its bytes. Symlinks placed in checkout may point at
  # outside sentinel paths but must not expose their host targets in the worker.
  # Compare /proc namespace device/inode identities against independent host
  # observations, and identify the process by nonce/start identity, not a PID
  # alone that could coincide inside the worker. No host-namespace join by a probe.
  # Actual attempts and successful local controls are required for every denial.
  # Accepted q-6 regression of slice-10's checkout-only host mount and private
  # namespaces (run 42195113834). Host reads/connections and process nonce checks
  # bracket the worker's matching attempts; local file/socket controls must pass.
  # A reachable host path/socket or matching host namespace/process fails.
  @slice-14 @ID-OFFLINE-10 @engine
  Scenario: Host files sockets and namespaces are inaccessible to a running worker
    Given a CI offline checkout with the cached image and outside host sentinels and socket listeners
    When the product runs the offline host-access probe
    Then the probe completes its worker-local file and socket controls but cannot read host sentinels or protected approval and ownership records
    And no host home engine socket SSH agent or fixture host socket is reachable from the worker
    And PID IPC UTS mount and network namespace identities differ from the host without access to the host process sentinel
    And host sentinels and unrelated resources are unchanged after confirmed owned cleanup

  # Given creates trusted, fixture-owned dual-stack listeners and an isolated
  # control topology with no public egress, using only the same cached image.
  # Synthetic private/gateway/link-local metadata addresses belong to that
  # topology; no real cloud metadata, public endpoint or external DNS is queried.
  # Control containers/networks are harness resources, never product resources.
  # Record exact addresses/ports and require both TCP and UDP over IPv4 and IPv6,
  # including literal destination, host gateway, private destination and synthetic
  # metadata paths. No proxy variables or client cooperation are needed for denial.
  # TCP success means a complete nonce exchange; UDP needs a nonce response AND
  # listener receipt. send() success is not delivery, timeout alone is not denial.
  # Each phase attempts all routes before emitting its complete framed report.
  # First-instruction means the explicit probe's earliest application operations,
  # before a blocking handshake, not before language-runtime initialization. No
  # image entrypoint/init/setup precedes it. Then use a bounded live barrier to
  # inspect the running worker and trigger phase two. Early product exit is data.
  # Mirror host/control loopback destinations in their own namespaces; at the
  # same port the worker can exchange its own nonce but never the host's nonce.
  # No control is allowed to attach or repair a subject worker's network.
  # Accepted q-6 regression of slice-10's verified NetworkMode none before start
  # (run 42195113834). Every route has same-client before/after nonce exchanges
  # and receipts, including IPv6; any worker answer, receipt or network grant fails.
  @slice-14 @ID-OFFLINE-11 @engine
  Scenario: No external network path exists from the first workload instruction
    Given a CI offline checkout with the cached image and controlled IPv4 IPv6 TCP UDP gateway private and synthetic metadata listeners
    When trusted controls and the product's first-instruction and running network probes attempt every fixture route
    Then every trusted route control succeeds before and after its worker attempt
    And every first-instruction and running worker attempt is denied with no matching listener receipt
    And the worker can reach its own loopback control but no host loopback listener at the same port
    And independent inspection observes NetworkMode none before start and while running with no port publication or extra network attachment
    And the product confirms owned cleanup while preserving all unrelated network resources

  # Given seeds unique fixture-only DNS names, nonce-bearing expected answers,
  # controlled resolvers and a disposable TLS trust root for the HTTPS control.
  # Exercise system lookup without replacing its resolver, explicit UDP/TCP DNS
  # to IPv4/IPv6 resolvers, Docker 127.0.0.11, an alternate 127/8 address and HTTPS
  # DNS. A/AAAA names and per-phase nonces exclude caches and hosts-file success.
  # Capture effective system resolver targets; the isolated control topology
  # must serve those target addresses without public forwarding. Never rewrite
  # the subject's resolver file, daemon DNS settings or frozen permissions to
  # make it testable. Docker's embedded DNS control uses the fixture bridge and
  # a fixture-owned name; other loopback resolvers live in the control namespace.
  # HTTPS uses a real TLS request with the fixture CA and expected nonce answer;
  # certificate failure is not network denial. Trust only this synthetic CA in
  # the probe, not the host store; pin the controlled endpoint to avoid relying
  # on the very DNS path under test. Do not disable TLS verification.
  # Resolver controls must return the answer before and after the corresponding
  # worker attempts; NXDOMAIN, timeout or a broken server is not a control pass.
  # Use the same earliest-operation/live-phase protocol as 11 and require the
  # complete query/phase report plus zero matching resolver receipts. Localhost
  # and hosts-file resolution alone is allowed and is not external DNS traffic.
  # Dual-stack OFFLINE-11 passed in the coordinator run, so that runner had IPv6.
  # Give the DNS control usable IPv6 interfaces in its own namespace too, wait
  # for address assignment/DAD, and avoid re-adding existing resolver aliases.
  # Surface probe setup errors immediately instead of replacing them with a
  # generic readiness timeout. All IPv4/IPv6 controls remain mandatory.
  # The DNS control alone needs a writable disposable root to rewrite its own
  # generated resolv.conf with the observed system resolver targets: Docker
  # makes that file read-only along with the root. This does not change the
  # worker's read-only root or resolver, the daemon, or the network-only control.
  @wip @slice-14 @ID-OFFLINE-12 @engine
  Scenario: Offline execution has no external DNS or alternate resolver path
    Given a CI offline checkout with the cached image and controlled system Docker loopback TCP UDP and HTTPS DNS resolvers
    When trusted controls and the product's first-instruction and running DNS probes query unique fixture names
    Then each resolver control returns its expected nonce-bearing answer before and after the worker attempt
    And the worker receives no external answer through system DNS 127.0.0.11 alternate loopback direct TCP UDP or HTTPS DNS
    And no controlled resolver receives a matching worker query at either execution phase
    And the DNS probes actually execute and the product confirms owned cleanup

  # Explicit shell execution is allowed inside the worker. APP is literal
  # "$(touch env-marker);$HOST_ONLY" in approved containerEnv and is read with
  # printf '%s', never eval. Synthetic HOST_ONLY and dispatch/credential namespace
  # markers exist ONLY in the controller's launch environment. Also plant an
  # outside host helper of the same basename to expose host-side command lookup.
  @slice-10 @ID-OFFLINE-13 @engine
  Scenario: An explicit shell uses the frozen workspace and environment inside the worker
    Given a CI offline checkout with the cached image and approved workspace subdirectory and literal APP environment
    And the offline controller environment and host helper contain synthetic markers absent from approved settings
    When the product runs an explicitly supplied shell which prints its workspace and APP then writes a checkout file
    Then shell interpretation and the file write occur only inside the worker at the approved workspace subdirectory
    And APP retains its exact approved bytes without expansion or an environment marker file
    And the worker has HOME and TMPDIR in its tmpfs and none of the host-only environment markers
    And the host helper never executes and the checkout edit survives confirmed owned cleanup

  # Given prepares the primary/child probe and an unrelated same-image control
  # worker with a different ownership tuple. When runs the product: the primary
  # writes an edit, spawns a long-lived child and waits for its nonce heartbeat,
  # then emits the trailing frame and exits with the example status. The child
  # has no voluntary exit before the fixture deadline. Independent observation
  # ties its live process/start identity to this full worker ID before primary
  # exit; absence of a child at setup or merely stale heartbeat data cannot pass.
  # After the actual primary exit require descendant disappearance and full-ID
  # absence before the wrapper result. Normal PID-namespace teardown may kill
  # descendants at primary exit; the spec does not require them to outlive it.
  # Do not confuse a coincidentally reused host PID or a killed harness process
  # with product teardown. Require the control worker still live with new nonce
  # heartbeats before harness cleanup. Check original approval/snapshot bytes,
  # ownership completion and persisted edit separately from the exit number.
  # Slice-13's local contract owns durability ordering; a DELETE alone is not it.
  # Accepted q-6 regression of slice-10's primary-exit teardown and slice-13's
  # owned cleanup contract (run 42195113834). A live descendant is independently
  # observed before exit and an unrelated same-image worker keeps heartbeating;
  # surviving descendants/resources or a missing durable completion record fail.
  @slice-14 @ID-OFFLINE-14 @engine
  Scenario Outline: Primary exit determines the result and ends the whole owned worker
    Given a CI offline checkout with the cached image and a primary probe exiting <status> with a surviving child
    When the product runs that offline foreground probe
    Then the product returns exactly <status> after the observed primary exit and confirmed durable owned cleanup
    And the trailing workload frame is delivered and the descendant no longer runs
    And native Docker confirms the exact worker ID absent while the unrelated same-image worker remains
    And checkout edits approvals snapshots and unrelated resources remain intact

    Examples:
      | status |
      | 0      |
      | 7      |
      | 1      |
      | 2      |
      | 3      |
      | 70     |
      | 75     |

  # Inject each cause at the cleanup boundary AFTER known workload exit. A fake
  # engine refusal must not mutate a foreign resource; transport errors exercise
  # the real SDK against HTTP. Store/configuration faults exercise real adapters.
  # Record the actual child status separately from wrapper classification, even
  # when numbers coincide. Seed only synthetic sensitive markers in all inputs.
  # Given configures a future wait result and cleanup fault; it does not run the
  # workload or assert its exit. When obtains the actual observed result first,
  # then activates the fault before the cleanup operation and retains its outcome.
  # The fault-free control must produce the same child exit and confirmed cleanup.
  #
  # Concrete fault meanings (held active until after product observations):
  # ownership: a full-ID observation has a foreign ownership tuple; do not remove.
  # configuration: invalid protected recovery bytes at reopen; do not overwrite.
  # environment: the previously valid required local socket is now absent; no
  # fallback endpoint. This is missing environment (3), not transient HTTP (75).
  # internal: an unclassified controller/adapter failure at the cleanup port;
  # use a test-only failing port decorator, not a panic or fabricated CLI result.
  # temporary: the owned DELETE returns 503 with a synthetic body marker while
  # independent inspection still observes the stopped full ID. Do not infer
  # absence from an error or from a resource name. No unrelated resource changes.
  # In-memory equivalents expose the same underlying failure, not the expected
  # public code. The real command classifier must decide cause, rule and result.
  #
  # Every failure retains the protected intent reference and observed child exit;
  # none may publish cleanup completion or return zero. For the invalid-record
  # case retain those invalid bytes for diagnosis, never silently replace them.
  # Diagnostics use stderr and distinguish incomplete cleanup from stopped
  # workload; the existing reference is not a public session ID or restart grant.
  # No executable stop retry until slice-11 supplies it. Raw argv/environment,
  # response bodies and copied output stay out of ownership records/diagnostics;
  # deliberate workload streams and approved frozen snapshots are separate data.
  # Ownership/configuration/temporary rows regress slice-10 result precedence
  # and passed before this item (q-9). Environment/internal rows with child 0/7
  # failed the first Then on both backends: temporary/75 instead of 3 or 70.
  @slice-13 @ID-OFFLINE-15 @linux
  Scenario Outline: Cleanup failure preserves a nonzero workload exit and classifies successful-work failures
    Given an offline run contract fixture with observed workload exit <child> and cleanup cause "<cause>"
    When the offline contract finishes through both fake and disk SDK adapters
    Then each offline result returns <result> and separately reports "run.cleanup-<cause>" with cleanup classification <cleanup>
    And each offline result preserves the observed workload exit and reports incomplete cleanup with its safe protected intent reference
    And no offline result claims stopped resources or advertises an unimplemented stop retry
    And retained offline ownership and diagnostics contain no raw argv environment response body or copied workload output

    Examples:
      | child | cause         | result | cleanup |
      | 0     | ownership     | 1      | 1       |
      | 0     | configuration | 2      | 2       |
      | 0     | environment   | 3      | 3       |
      | 0     | internal      | 70     | 70      |
      | 0     | temporary     | 75     | 75      |
      | 7     | ownership     | 7      | 1       |
      | 7     | configuration | 7      | 2       |
      | 7     | environment   | 7      | 3       |
      | 7     | internal      | 7      | 70      |
      | 7     | temporary     | 7      | 75      |
      | 75    | temporary     | 75     | 75      |

  # Given prepares a removal-only fault rule, not an observed workload result.
  # When forwards real create/attach/start/wait/inspect to native Docker, captures
  # the actual primary exit, then rejects EVERY product DELETE for that owned
  # full ID with synthetic HTTP 503/body marker. Keep inspection working and
  # hold the fault until all product observations are complete. No manufactured
  # wait result or engine-wide outage. A fault-free run of the same probe/status
  # supplies the control and must complete cleanup; its resource is distinct.
  # Independently observe the faulted full ID stopped but PRESENT, the retained
  # owned record and unmodified approval/snapshot. No absent/completed tombstone.
  # Require the real rule run.cleanup-temporary (classification 75) on stderr,
  # distinct from known child exit: 0 maps to wrapper 75; 7 and 75 stay unchanged.
  # Retain the existing safe protected intent reference, not a made-up public ID
  # or unimplemented stop retry. A truthful exited workload is allowed; a stopped
  # success claim means completed session cleanup and must be absent here.
  # Diagnostics and ownership records exclude raw argv/environment, response
  # bodies and copied output; the approved snapshot and deliberate streams are
  # separate data. Clear the fault only for identity-checked harness cleanup after
  # assertions; its removal cannot turn the product failure into a successful run.
  # Engine loss/cancellation remains slice-12, public status/stop retry slice-11.
  # Accepted q-6 regression of slice-10's native exit precedence and slice-13's
  # classified incomplete cleanup (run 42195113834). A fault-free invocation of
  # the same probe/status proves removal works, and the unrelated worker stays
  # live; a changed exit or an absent/completed faulted worker fails.
  @slice-14 @ID-OFFLINE-16 @engine
  Scenario Outline: Native workload exit is retained when required removal fails
    Given a CI offline checkout with the cached image and a removal-only transient Engine API fault
    When the product runs an offline probe that writes a checkout edit and exits <child>
    Then the actual workload exit is <child> and the product returns <result> with a separate temporary cleanup failure
    And native Docker still observes the exact stopped worker and its protected ownership record before harness cleanup
    And the diagnostic carries the safe protected intent reference without the synthetic body marker or a stopped-success claim
    And the checkout edit and unrelated resources remain intact

    Examples:
      | child | result |
      | 0     | 75     |
      | 7     | 7      |
      | 75    | 75     |
