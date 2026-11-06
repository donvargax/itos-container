@phase-1
Feature: Hardened offline foreground run
  An approved run executes explicit argv in one owned offline worker.
  Launch uses frozen settings and verified ownership; cleanup retains the
  observed workload result and reports failures without claiming removal.

  # Slice-13 carries ID-OFFLINE-01/04/05/15 from slice-10, as q-7 directs.
  # These IDs were allocated in 7920219 and retagged, still @wip, in dad6d1f;
  # they are preserved, not newly minted or replacements for sibling scenarios.
  # Source reviewed: slice-10 at 2ce7bec52abc114008bd9e17d805841fdbc2def1.
  # Main at ffe148d5a2cfcaad65bd9df721b053105e385c25 still stops after preflight.
  # The dependency branch already adds this same OFFLINE feature. At integration,
  # keep one copy of each ID in this file, retain these slice-13 comments, and
  # preserve all slice-10/14 scenarios and their dependency-branch tag states.
  # Do not copy these IDs into a second feature or activate siblings here.
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
  # Step handoff: every step phrase below is NEW on main and on the reviewed
  # dependency. Reuse the existing feature helpers in offline_local_steps_test.go,
  # session_probe_test.go, session_fixture_test.go and offline_contract_engine_test.go
  # on slice-10, plus its cmd/itos-container/offline_contract_test.go assembly.
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
  # Evidence handoff: this is spec-only, with no step definitions or executed red.
  # On current main, startup-unavailable would fail 01's first launch assertion,
  # 04's first A-launch assertion, every 05 row's exit-1 identity assertion, and
  # every 15 row's separate cleanup-rule assertion. A refusal or undefined step
  # alone is not evidence: the implementer must first commit complete steps with
  # @wip retained, remove @wip only for local assertion-level red/green runs, and
  # record each result. Slice-10 already implements Runner, Workspace, frozen
  # create settings and most result precedence; do not claim they are absent.
  # Its classifier lacks cleanup-environment/3, a concrete remaining 15 failure.
  # Recheck all rows against the actual dependency head before production edits.
  # If a case already passes, report it for coordinator disposition under the
  # red-first rule; never force a failure, rewrite the assertion or claim red.
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
  @wip @slice-13 @ID-OFFLINE-01 @linux
  Scenario: Run verifies durable ownership and the full boundary before launch
    Given an offline run contract fixture with approved settings and successful engine observations
    When the offline contract executes an argv probe through both fake and disk SDK adapters
    Then each offline run starts exactly one inspected owned worker after durable intent, owned ID, boundary verification and attach
    And each create and inspection has the exact frozen image, UID GID, workspace, argv, environment and hardened runtime configuration
    And each offline run observes exit zero and confirms owned removal before returning zero
    And no offline operation acquires images, executes host helpers or changes unrelated resources

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
  @wip @slice-13 @ID-OFFLINE-04 @linux
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
  @wip @slice-13 @ID-OFFLINE-05 @linux
  Scenario Outline: Checkout substitution cannot redirect the worker mount or working directory
    Given an offline run contract fixture with a launch barrier at "<boundary>"
    When the offline run reaches that barrier and the fixture replaces "<path>" with an outside symlink before release
    Then the offline run refuses the changed checkout identity with exit 1 and no start
    And the outside target is neither mounted, modified nor used as the working directory
    And any already created owned worker is cleaned without touching the substituted source

    Examples:
      | boundary                  | path                   |
      | before create             | checkout root          |
      | before create             | embedded .git          |
      | before create             | workspace subdirectory |
      | after create before start | checkout root          |
      | after create before start | workspace subdirectory |

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
  @wip @slice-13 @ID-OFFLINE-15 @linux
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
