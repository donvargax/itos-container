@phase-1
Feature: Hardened offline foreground run
  An approved run executes explicit argv in one owned offline worker.
  Isolation holds before the first workload instruction; foreground exit ends
  the worker and its temporary storage while preserving checkout edits.

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
  # Slice-12 owns that loss proof, cancellation, signal forwarding and aggregate
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

  # Owned launch ordering and refusal boundaries. The positive row must run;
  # an implementation which always refuses cannot satisfy this contract.
  @wip @slice-10 @ID-OFFLINE-01 @linux
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
  @wip @slice-10 @ID-OFFLINE-02 @linux
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
  @wip @slice-10 @ID-OFFLINE-03 @linux
  Scenario: Run cannot execute a worker whose concrete ownership is not durable
    Given an offline run contract fixture with persistent owned-record fsync failure
    When the offline contract attempts launch and later retries cleanup after clearing the fault
    Then each first offline run reports ownership publication failure with no start or usable launch handoff
    And each first offline run retains the complete protected intent and independently observed unstarted worker
    And each later cleanup persists the reconciled ID before removal and confirms absence without starting work
    And the original run failure is not replaced by later cleanup success

  # Only request contents/current approval change; original path identities and
  # snapshot remain intact. One selected session uses A; a new invocation uses B.
  @wip @slice-10 @ID-OFFLINE-04 @linux
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
  @wip @slice-10 @ID-OFFLINE-05 @linux
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
  @wip @slice-10 @ID-OFFLINE-06 @engine
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
  @wip @slice-10 @ID-OFFLINE-07 @engine
  Scenario: Redirected foreground streams remain live, separate and complete
    Given a CI offline checkout with the cached image and a framed stdin stdout stderr probe
    When the product runs the offline stream probe and the harness answers its live handshake then closes stdin
    Then both readiness streams arrive before the input reply and before workload exit
    And the worker receives the exact stdin frame and EOF
    And stdout and workload stderr retain their exact separate frames including the trailing bytes
    And controller diagnostics occur only on stderr and the product returns zero after confirmed cleanup

  # Effective identity comes from the real host process, not fixture environment.
  # Read UID/GID/groups, all capability sets, NoNewPrivs and actual cgroup pids.max
  # inside the process. Attempt setuid(0) and a privileged mount; both must fail.
  # Trusted CI setup places a root-owned setuid probe in the disposable checkout
  # before approval; this helper only reports IDs. A separate isolated control
  # without NNP must show that this same file can elevate. Execute it under run
  # and require the same non-root identity. Never execute it on the host.
  # This tests NNP even where sudo is absent; lack of sudo alone proves nothing.
  # No 4096-process exhaustion or CPU/memory quota assertion is required.
  @wip @slice-10 @ID-OFFLINE-08 @engine
  Scenario: The first workload process has the host non-root identity without elevation
    Given a CI offline checkout with the cached image and identity capability and setuid probes
    And misleading host identity environment and an unrelated supplementary group are recorded as fixture inputs
    When the product runs the offline identity probe as its first executable
    Then the process and its child have exactly the effective host UID and GID without host supplementary groups
    And the runtime has no effective permitted inheritable ambient or bounding capabilities and NoNewPrivs is one
    And root identity and privileged mount attempts fail and the setuid probe cannot elevate
    And the process reads an effective pids limit of 4096 without any claimed memory or CPU quota
    And the product confirms owned cleanup without host chown or unrelated file changes

  # Check kernel root mount flags AND failed writes, not EACCES alone. Successful
  # checkout/tmpfs writes are positive controls. CI setup also places a read-only
  # fixture bind below the checkout, backed by an outside host sentinel; the
  # worker must not inherit that nested mount or see its marker. Teardown of
  # this fixture mount is harness-only, after product observations.
  # A second independent invocation
  # sees the edit but not the nonce placed in /tmp by the first worker.
  @wip @slice-10 @ID-OFFLINE-09 @engine
  Scenario: Checkout edits persist while writable home and temporary files are ephemeral
    Given a CI offline checkout with the cached image and a filesystem probe
    When the product runs the offline filesystem probe twice with distinct session nonces
    Then both processes observe a read-only image root and fail to modify image files
    And both processes can write the checkout and their tmpfs HOME and TMPDIR as the host UID GID
    And the second process sees the first checkout edit but none of its temporary or home files
    And native inspection and mount observations show only the approved checkout host source and container-owned temporary storage
    And neither worker sees the outside marker from the fixture's nested host mount
    And both exact worker IDs and their ephemeral storage are absent after product cleanup with no created persistent volume

  # Seed random host-only sentinels outside checkout and private fake home/approval
  # directories, plus a harmless Unix socket server. Pass their absolute paths as
  # data to the probe, with no mounts. Compare PID/IPC/UTS/mount/network namespace
  # identities to independent host observations, and require worker-local positive
  # controls for file reads/socket communication. A missing probe is not denial.
  @wip @slice-10 @ID-OFFLINE-10 @engine
  Scenario: Host files sockets and namespaces are inaccessible to a running worker
    Given a CI offline checkout with the cached image and outside host sentinels and socket listeners
    When the product runs the offline host-access probe
    Then the probe completes its worker-local file and socket controls but cannot read host sentinels or protected approval and ownership records
    And no host home engine socket SSH agent or fixture host socket is reachable from the worker
    And PID IPC UTS mount and network namespace identities differ from the host without access to the host process sentinel
    And host sentinels and unrelated resources are unchanged after confirmed owned cleanup

  # First-instruction probes bypass all proxy settings. TCP requires completed
  # nonce exchange; UDP requires a nonce reply AND listener receipt comparison.
  # Inspect before start and after launch, with no published port or extra
  # network attachment. A worker's own loopback remains usable, never host loopback.
  @wip @slice-10 @ID-OFFLINE-11 @engine
  Scenario: No external network path exists from the first workload instruction
    Given a CI offline checkout with the cached image and controlled IPv4 IPv6 TCP UDP gateway private and synthetic metadata listeners
    When trusted controls and the product's first-instruction and running network probes attempt every fixture route
    Then every trusted route control succeeds before and after its worker attempt
    And every first-instruction and running worker attempt is denied with no matching listener receipt
    And the worker can reach its own loopback control but no host loopback listener at the same port
    And independent inspection observes NetworkMode none before start and while running with no port publication or extra network attachment
    And the product confirms owned cleanup while preserving all unrelated network resources

  # Exercise system resolution of unique fixture-only names, explicit UDP/TCP
  # DNS to controlled IPv4/IPv6 resolvers, Docker's 127.0.0.11 and alternate
  # 127/8 addresses, and controlled HTTPS DNS. Control containers resolve via
  # Docker DNS on the fixture bridge; loopback DNS endpoints are controlled in
  # their own namespace. Successful nonce-bearing answers, not NXDOMAIN, prove
  # the controls. Hosts-file names/localhost are not unauthorized DNS traffic.
  @wip @slice-10 @ID-OFFLINE-12 @engine
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
  @wip @slice-10 @ID-OFFLINE-13 @engine
  Scenario: An explicit shell uses the frozen workspace and environment inside the worker
    Given a CI offline checkout with the cached image and approved workspace subdirectory and literal APP environment
    And the offline controller environment and host helper contain synthetic markers absent from approved settings
    When the product runs an explicitly supplied shell which prints its workspace and APP then writes a checkout file
    Then shell interpretation and the file write occur only inside the worker at the approved workspace subdirectory
    And APP retains its exact approved bytes without expansion or an environment marker file
    And the worker has HOME and TMPDIR in its tmpfs and none of the host-only environment markers
    And the host helper never executes and the checkout edit survives confirmed owned cleanup

  # The primary probe spawns a long-lived child, emits a completion frame and
  # exits with the example status. Observe primary exit, actual descendant PID
  # disappearance and engine full-ID absence, not merely a DELETE response.
  # Record ownership completion before judging the product result; fixture cleanup
  # cannot make this pass. Existing approval/snapshot and unrelated engine objects
  # remain, including a same-image control worker with a different ownership tuple.
  @wip @slice-10 @ID-OFFLINE-14 @engine
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
  @wip @slice-10 @ID-OFFLINE-15 @linux
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

  # Forward real create/attach/start/wait to native Docker, then reject every
  # product removal with a synthetic 503 and body marker. Observe the stopped
  # full ID still present before disabling this fault for harness-only cleanup.
  # This proves runtime outcome/cleanup separation, not slice-12's engine-loss
  # or signal behaviour and not slice-11's status/stop retry interface.
  @wip @slice-10 @ID-OFFLINE-16 @engine
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
