@phase-1
Feature: Inspect and stop owned offline sessions
  A person can find a session whose controller is gone and finish its cleanup.
  Recovery uses protected ownership, never labels alone, and preserves uncertainty
  about work whose exit was not observed.

  # Authorities: docs/charter.md, docs/architecture.md (Slices), arc42 Recovery,
  # and offline-run-lifecycle.md Cleanup and recovery (AC-06/22/23/28/36 offline).
  # A2 means checkout plus tmpfs, no persistent worker volumes. A3 leaves a worker
  # isolated after controller death until stop; no watchdog or unattended restart.
  # A4 retains pids 4096, with no memory/CPU quota claim. Linux amd64/native Docker
  # is the only runtime target. No list/exec/init, proxy, credentials or acquisition.
  #
  # Dependency baseline inspected for this spec: main 9efa67c and slice-10 fe02965.
  # Slice-9's intents, original snapshots and full-ID reconciliation already exist.
  # Slice-10 adds Runner and foreground cleanup. The person's q-7 split assigns
  # OFFLINE-01/04/05/15 to slice-13 and OFFLINE-08..12/14/16 to slice-14, still wip.
  # These specifications are prerequisites to consume, not evidence they pass.
  # Slice-12 owns signal/cancellation and retained-isolation-through-loss proof;
  # this file owns discovery after loss and the public recovery action. Its totals
  # still apply: 10 s per call, 30 s per stop attempt, including locks and durability,
  # without resetting on retry/reconnect. Status has a 10 s whole-invocation budget.
  # No deadline is a workload lifetime limit. No local Docker or mutation run.
  #
  # Add vertical internal/slices/status and internal/slices/stop packages with
  # command, domain, port and named adapter siblings only where needed. Extend
  # internal/engine/session for shared registration, observation and cleanup; run
  # uses those engine contracts, never imports stop/status. Only cmd assembles.
  # Keep I/O in adapters, pure domain/port APIs and I/O-free port/porttest fakes.
  # Reuse approval's protected-storage primitives, strict frozen codec and host
  # identity, and readiness's checkout/socket/daemon identity checks. This adds
  # no second approval loader or current-policy prerequisite for removal.
  # Dependency choice: existing Go 1.27.3 crypto/rand, hex, JSON and filesystem
  # APIs, golang.org/x/sys v0.48.0 for Linux descriptor locks, and Moby client
  # v0.6.2/api v1.56.1 for inspect/stop/kill/remove. Reuse these pinned maintained
  # dependencies, not a custom Engine client or Docker subprocess. No new library,
  # service or domain allowlist entry. Preserve notices and value-driven OS logic.
  #
  # Public interface choices:
  # - status takes no positional argument; stop takes exactly one session ID.
  #   Both operate only on the safely resolved effective checkout and host user.
  #   Equal basenames are not equal projects. No global scan, prefix match, intent
  #   path, container name/ID alias, --all, force override or consent prompt.
  # - IDs are 32 cryptographically random bytes encoded as 64 lowercase hex digits,
  #   allocated independently of slice-9's operation token. Never derive from PID,
  #   time, argv, checkout text or approval generation. Validate before filesystem
  #   lookup. Entropy failure refuses; an existing ID refuses without overwriting
  #   or silently drawing again. A public ID is a lookup key, never authority.
  # - Retry text is exactly `itos container stop <actual-id>`, with the validated
  #   ID substituted, accompanied by guidance to use the same effective project.
  #   No shell quoting of untrusted paths, unresolved placeholder or prune advice.
  #   JSON exposes retry as ["itos","container","stop",id], or null when complete.
  #   The harness executes that exact suffix as argv in the original project;
  #   it never shells the diagnostic or reaches the operator's installed itos.
  # - --json and ITOS_JSON=1 produce exactly one schema-1 stdout object, including
  #   refusals. Explicit --no-json overrides the environment. Plain status/stop
  #   give the same facts to stdout; diagnostics go to stderr, no colour/prompts.
  #   Help names project scope, JSON, unknown outcomes and stop's cleanup-only exit.
  #
  # JSON contract: {schema:1, ok:bool, problems:[], sessions:[]} for both commands.
  # Stop's successful sessions array contains its one session. Status sorts by ID.
  # Each validated entry has id, project, fingerprint, state, resource, controller,
  # cleanup, workload_exit, stop_candidate and retry. Project/fingerprint come from
  # the original binding, never current requests. workload_exit is an observed
  # integer or null; a resource's running=false/default ExitCode is not observation.
  # Resource is created/running/exited/absent/unknown; controller is present/absent/
  # unknown; cleanup is pending/complete/unknown. State uses arc42's preparing,
  # ready, running, stopping, stopped, failed, interpreting recorded progress with
  # fresh observations. Run/stop report stopped/complete only after concrete-ID
  # absence AND durable completion. Read-only status requires both safely loaded
  # stopped/absent records and fresh concrete-ID absence to report completion;
  # it reports recorded progress, not proof that an earlier fsync succeeded.
  # Unreachable/contradictory observations are failed/unknown, never stopped.
  # An absent controller with pending or unknown cleanup makes stop_candidate
  # true even if the worker is exited or unstarted; otherwise it is false.
  # A present controller does not forbid an explicit stop. Completed sessions
  # remain listed, not GC'd.
  #
  # Status is read-only, including controller discovery: no record rewrite, lock
  # creation, durability repair, ownership adoption, engine mutation or launch.
  # Absent state is an empty successful list, without creating directories or
  # opening the engine. A safely read pending record can be listed with unknown
  # resource/controller facts when inspection fails, plus its typed problem.
  # Invalid/unsafe records are not trusted entries; report a problem and continue
  # inspecting independent safe entries. Never present a partial list as complete
  # success. Problems have rule, exit, message and fix, plus session only when its
  # filename is a validated ID. Sort by session then rule; process exit is the
  # largest problem exit and ok is false when any problem exists.
  # Rules shared by status/stop are recovery.usage (2), recovery.not-owned (1),
  # recovery.unsafe-state (1), recovery.invalid-state (2), recovery.environment (3),
  # recovery.temporary (75), recovery.internal (70). A missing ID in this project's
  # state is not-owned, even if another project or the daemon has a matching name.
  # Unknown schema/data is invalid-state; ownership or identity mismatch is
  # not-owned; unsafe paths/owner/modes/links are unsafe-state. Missing supported
  # engine environment is environment; timeout/unavailable observation, lock
  # contention deadline, uncertain create or uncertain write is temporary. Raw
  # SDK/parser errors never determine presentation. An unclassified defect is internal.
  # Successful status returns 0 even for a known failed session needing cleanup.
  # Successful stop returns 0 for confirmed cleanup, NOT successful workload;
  # preserve a known nonzero workload_exit or null. Run keeps its existing exit
  # precedence and run.cleanup-* classifications while gaining ID/retry guidance.
  # Registration failures use run.session-registration-<cause>, with ownership=1,
  # internal=70 and temporary=75; these occur before create and have no child exit.
  #
  # Protected record choice: sessions/<project>/<id>/record.json beside approvals,
  # snapshots and intents under the existing state root. A schema-1 record has
  # exactly schema, id, project, intent, state, workload_exit and last_problem.
  # intent is the exact relative intents/<project>/<token>.json reference; state
  # is one of the six lifecycle values; workload_exit is null or integer 0..255;
  # last_problem is null or a typed failure identifier, never arbitrary text.
  # Bind record filename/ID/project/intent to the original strict intent/snapshot;
  # do not duplicate argv, environment, settings, stream bytes or engine bodies.
  # Read one object, max 65536 bytes/depth 16, with no missing/unknown/duplicate/
  # wrongly typed fields or trailing data. A malformed record grants no cleanup.
  # Apply existing no-follow, pinned safe ancestors, effective-owner, single-link
  # regular-file protections. Directories are 0700, records/lock files 0600 even
  # under restrictive/permissive umask. Never repair unsafe state with chmod/chown.
  # Updates compare the preceding complete bytes; full write, file fsync, atomic
  # publication and parent/new-ancestor fsync precede a successful handoff.
  #
  # Allocate the public ID and operation token before create. Publish registration
  # and its lock objects durably, then slice-9's durable intent, before any create.
  # Registration is initially preparing/null. A crash between those publications
  # leaves a discoverable entry with a missing intent, never permission to infer
  # absence or create again. Report invalid-state and preserve it for inspection;
  # automatic garbage collection/repair is outside this increment. A known complete
  # intent must remain discoverable if create/start/response/controller is lost.
  # No failure returns an executable retry for an unpublished or invalid ID.
  # Keep old slice-9 intent-only records intact; no automatic migration/adoption of
  # pre-interface artifacts, and no requirement to turn them into public sessions.
  #
  # Each session directory contains two persistent, protected regular lock files:
  # controller.lock and operation.lock. They are never replaced/unlinked while
  # retained records exist. Use kernel descriptor locks through an engine port;
  # no stale lockfile timeout, PID-only liveness or host PID signalling. The run
  # controller acquires an exclusive lease before registration becomes visible
  # and holds it for its lifetime, released by
  # the kernel on death. Descriptors are close-on-exec and not inherited by workers
  # or helper processes. Status tries a nonblocking SHARED lease lock: contention
  # means present, acquisition means absent (release immediately), errors mean
  # unknown. Concurrent status readers cannot make one another look like a live
  # controller. This observes a lease, not workload health or a known reason for loss.
  #
  # A separate per-session operation lock serializes run transitions and stop,
  # including revalidation, engine mutations and durable completion. Never hold
  # it across foreground waiting/stream draining, or use the controller lease to
  # block stop. Short project storage transactions do not span engine calls or
  # one session's foreground lifetime. Status reads a consistent committed view
  # under that operation lock within its total budget; it never creates locks.
  # Lock order is session operation, then short storage transactions; no cross-
  # session acquisition. A waiting stop re-reads state after acquiring the lock.
  # Before starting or mutating, run rechecks that stop has not completed or begun;
  # cleanup is terminal, never followed by a late start/recreate. Persist stopping
  # before destructive operations. Preserve original ownership evidence on errors.
  # If stop wins after durable intent publication but before create, durably record
  # that cleanup began even though no destructive operation is possible. A missing
  # name and empty concrete ID require recovery.temporary, failed state, unknown
  # resource/cleanup and an exact retry; they cannot establish successful cleanup.
  # Retain the intent and registration. Resumed run returns run.cleanup-temporary
  # with exit 75 and unknown workload exit, without a late create/start or overwrite
  # of cleanup progress. No absent-ID evidence or completed tombstone is invented.
  #
  # Stop validates the original snapshot, actual user/checkout, pinned local socket
  # and daemon, then freshly inspects the full resource ID. Revalidate immediately
  # before EACH stop/kill/remove, including after grace waits and storage updates.
  # Match name, token/project/fingerprint labels and image ID against trusted
  # ownership, not just an ID-shaped argument. No engine-wide list/prune/adoption.
  # An intent-only uncertain create uses exact random-name AND full-ID inspection
  # and durable owned-ID publication before mutation, as slice-9 requires. Missing
  # name is still uncertain, not absence proof or permission for replacement create.
  # Current YAML/manifest/approval may be changed, missing or invalid; original
  # protected snapshot and identity remain required. No approve, host helper,
  # cached-image acquisition or launch preflight is part of recovery.
  #
  # For a running owned worker, request SIGTERM once, allowing up to 10 s grace
  # within the original stop total; revalidate and request SIGKILL only if it is
  # still observed running. Do not inherit an image's custom StopSignal. Slice-11
  # owns this shared termination contract in internal/engine/session; slice-12
  # depends on and reuses it (q-11). Never signal a controller PID.
  # Created/exited workers need removal only. Remove by verified full ID without
  # volume deletion; confirm that full ID absent, durably mark the intent absent,
  # then publish session stopped. A DELETE reply alone is not completion. Preserve
  # unknown workload outcome on cancellation/removal; a fresh explicit exit
  # observation may fill null, but never overwrite a previously known result.
  # Repeated stop revalidates and confirms the original ID absent without further
  # engine mutations or rewriting completion. It re-establishes record/directory
  # durability by syncing, including after a visible but uncertain prior update.
  # Same-named replacements, other sessions, images, networks, volumes, checkout
  # edits, approvals and frozen
  # settings survive. Retain tombstones; no retention/GC policy is added here.
  #
  # Integration handoff: public registration extends the inherited preflight
  # preservation assertions with exact protected records and persistent locks.
  # Keep their approval/snapshot/checkout invariants and reject any other paths.
  # On implementation, supersede OFFLINE-15's temporary
  # ban on executable retries and OFFLINE-16's intent-only diagnostic expectation;
  # retain their result precedence and marker exclusions. Their current owner is
  # slice-13/14, not this drafter. Preserve SESSION's unstarted-only API; no relaxed
  # ownership checks to accommodate workers that ran. If implementing these choices
  # departs from an arc42 section, update it with a decision naming that section in
  # the implementation PR. This spec changes neither architecture nor sibling files.
  #
  # EVERY step phrase below is NEW. Reuse fixture machinery from session/offline/
  # approval, not their unstarted-only assertions. Givens write disposable inputs,
  # independently encoded records/snapshots, synthetic resources and fault/barrier
  # configuration; they never invoke run/stop/approve or assert product behavior.
  # For local cases, use the real command assembly with I/O-free port fakes and
  # production disk/SDK adapters against a Unix HTTP fixture. No production test
  # switch, product imports in feature steps or agent socket. Shared contracts run
  # against each fake/adapter; actual locking uses competing subprocesses. Durability
  # evidence observes real operations, not file existence. Record all requests,
  # including refused calls, and separate fixture changes from product effects.
  # Actions capture early refusal, process exit and bounded barrier timeout as data
  # for Then; absence of a new command/registration must not hang or fail a Given.
  # Successful controls use the same command/adapter route as fault cases.
  # Deadline contracts use a controlled monotonic clock for fake operations and
  # measured real deadlines for adapters, with a fixed 2 s scheduling tolerance;
  # that test tolerance is not additional product budget. An unavailable command
  # must reach the positive result/rule assertion, never pass an absence-only test.
  #
  # @engine cases run only in README's guarded Linux amd64 CI tier with T-20's
  # independently inspected cached image. Launch actual product processes with
  # disposable seeded approval, never approve. Read-only independent full-ID and
  # heartbeat observations precede bounded identity-checked harness cleanup, which
  # cannot count as product success. Trusted probes run only inside the worker.
  # No new image, real credentials, runtime support claim or isolation substitute.
  # Commit complete steps red first while @wip remains. Local runs temporarily
  # remove @wip; the coordinator obtains engine red via red_scenarios. Every first
  # Then below checks behavior, not fixture setup. The coordinator's standing
  # q-6 decision accepts already-passing earlier-slice behavior as regression
  # coverage: verify its positive assertion and name that slice above its tag.
  # Stop only for unfixable setup or a case unable to reject a wrong build.
  # Missing steps/setup failures/unknown-command diagnostics alone are never proof.

  # Public help precedes inspection and never opens state or an engine.
  @slice-11 @ID-RECOVERY-24 @linux
  Scenario: Recovery help describes project scope cleanup results and controller uncertainty
    Given a recovery fixture with no engine access and checkout helper sentinels
    When product status help and stop help are requested
    Then both help requests return zero and describe their exact arguments JSON project scope and exit meanings
    And help explains that stop confirms cleanup rather than workload success and controller absence does not authorize restart
    And help performs no state access engine connection approval or checkout helper execution

  # Inspection and representation. No state is a useful successful observation.
  @slice-11 @ID-RECOVERY-01 @linux
  Scenario: Status of a project with no sessions is read-only and empty
    Given a recovery fixture with a safe checkout and no host state directory
    When recovery status is invoked in plain and JSON modes through each local backend
    Then every status succeeds with an empty owned-session list and no problems
    And no recovery invocation creates state, connects to an engine or executes a checkout helper

  # All records and explicit workload observations are independently seeded.
  # The completed row has a durable absent intent and independently absent ID.
  @slice-11 @ID-RECOVERY-02 @linux
  Scenario Outline: Status separates lifecycle resource and workload knowledge
    Given a recovery fixture with recorded phase "<phase>" resource "<resource>" and observed workload exit "<exit>"
    When recovery status is invoked with --json, inherited JSON and inherited JSON overridden by --no-json
    Then every status reports state "<state>" resource "<resource>" cleanup "<cleanup>" and workload exit "<exit>"
    And JSON status has the specified schema and original project and fingerprint while plain status reports the same facts
    And recovery inspection leaves every record and resource unchanged

    Examples:
      | phase     | resource | exit    | state     | cleanup  |
      | preparing | created  | unknown | preparing | pending  |
      | ready     | created  | unknown | ready     | pending  |
      | running   | running  | unknown | running   | pending  |
      | failed    | exited   | 7       | failed    | pending  |
      | failed    | exited   | unknown | failed    | pending  |
      | stopped   | absent   | 0       | stopped   | complete |
      | stopped   | absent   | 7       | stopped   | complete |
      | stopped   | absent   | unknown | stopped   | complete |

  # A harness process holds the lease; releasing it changes only observation.
  # A deliberately unrelated live PID cannot turn an absent lease into present.
  @slice-11 @ID-RECOVERY-03 @linux
  Scenario: Status identifies controller absence without treating a PID as ownership
    Given a recovery fixture with a running worker, a held controller lease and an unrelated live process
    When status is called before and after the fixture lease holder exits
    Then status changes controller from present to absent and stop candidate from false to true with the exact retry argv
    And the worker remains reported running with unknown workload exit
    And status neither signals the unrelated process nor changes resources or protected records

  @slice-11 @ID-RECOVERY-04 @linux
  Scenario: Status retains useful entries while reporting incomplete inspection
    Given a recovery fixture with safe sessions and separate invalid-state, unsafe-state and unavailable-engine entries
    When recovery status is invoked in JSON mode through each local backend
    Then status lists the independently safe sessions in ID order and reports recovery.invalid-state, recovery.unsafe-state and recovery.temporary with exit 75 and ok false
    And the unavailable session has unknown resource and cleanup without a fabricated exit or stopped claim
    And malformed entries confer no ownership and all recovery state remains unchanged

  # Public registration must be available after abrupt loss, not only at run exit.
  @slice-11 @ID-RECOVERY-05 @linux
  Scenario: Run registers a unique recoverable identity before creating a worker
    Given a recovery run fixture with one approved snapshot and two independent invocations
    When both runs reach the create barrier through each local backend and status observes them after release
    Then each create follows durable public registration and intent publication with distinct random session IDs and distinct operation tokens
    And each status entry resolves exactly its own original intent snapshot and full worker ID
    And registration and lock objects have the specified protected paths modes and strict schema without workload data

  @slice-11 @ID-RECOVERY-06 @linux
  Scenario Outline: Failed public registration cannot leave an undiscoverable worker
    Given a recovery run fixture with registration fault "<fault>"
    When run attempts public registration through each local backend
    Then run refuses registration with cause "<cause>" before any create or start and without an executable retry for an unpublished identity
    And existing registration intent and snapshot bytes remain intact with no replacement ID draw

    Examples:
      | fault                   | cause     |
      | entropy unavailable     | internal  |
      | occupied public ID      | ownership |
      | short record write      | temporary |
      | record file fsync       | temporary |
      | atomic publication      | temporary |
      | session directory fsync | temporary |
      | lock object durability  | temporary |

  # Stop is a cleanup result, not a replay of the original workload's exit code.
  @slice-11 @ID-RECOVERY-07 @linux
  Scenario Outline: Stop completes owned cleanup and is safe to repeat
    Given a recovery fixture with an owned "<resource>" worker and observed workload exit "<exit>"
    When stop is invoked twice and status follows through each local backend
    Then both stops return zero with cleanup complete and workload exit "<exit>" after confirmed full-ID absence and durable completion
    And running-worker stop attempts send SIGTERM once and escalate only after the bounded grace and fresh ownership inspection
    And the repeated stop performs no engine mutation or completed-record rewrite
    And only the originally owned worker is removed while checkout edits approvals snapshots and unrelated resources remain

    Examples:
      | resource                 | exit    |
      | created                  | unknown |
      | running                  | unknown |
      | running ignoring SIGTERM | unknown |
      | exited                   | 0       |
      | exited                   | 7       |
      | already absent           | unknown |
      | durable absent           | 7       |

  # The fault is cleared only after run's result and retained worker are captured.
  @slice-11 @ID-RECOVERY-08 @linux
  Scenario Outline: The exact retry printed by run completes interrupted cleanup
    Given a recovery run fixture with observed workload exit <child> and transient removal failure
    When run finishes, the removal fault is cleared and a fresh process executes its printed stop argv in the same project
    Then run returns <result> with run.cleanup-temporary and a registered session ID whose printed retry succeeds with stop exit zero
    And later status reports that session stopped with workload exit <child> and no pending retry
    And recovery preserves run's original result and all unrelated state without creating or starting another worker

    Examples:
      | child | result |
      | 0     | 75     |
      | 7     | 7      |
      | 75    | 75     |

  # Change input bytes only; original checkout identity and protected binding stay.
  @slice-11 @ID-RECOVERY-09 @linux
  Scenario Outline: Recovery does not require current requests or current approval
    Given an independently owned recovery fixture whose current "<input>" is "<change>"
    When status and stop are invoked through each local backend
    Then status reports the original fingerprint and stop completes using the original protected snapshot
    And recovery neither parses the changed request nor prompts approves executes helpers or acquires images

    Examples:
      | input    | change     |
      | policy   | missing    |
      | policy   | invalid    |
      | manifest | changed    |
      | manifest | missing    |
      | approval | missing    |
      | approval | invalid    |
      | approval | superseded |

  # Invalid strings are data in argv, never evaluated by a shell or echoed raw.
  @slice-11 @ID-RECOVERY-10 @linux
  Scenario Outline: Recovery accepts only its complete project-scoped session ID
    Given a recovery fixture with usage variant "<variant>" and outside sentinels
    When the recovery command is invoked in JSON mode with that literal argv
    Then recovery returns one schema-1 failure with recovery.usage exit 2 before state or engine access
    And no raw invalid ID or synthetic marker appears in recovery output and no sentinel changes

    Examples:
      | variant                 |
      | stop missing ID         |
      | stop extra argument     |
      | stop empty ID           |
      | stop uppercase hex ID   |
      | stop abbreviated ID     |
      | stop intent path        |
      | stop path traversal     |
      | stop shell control text |
      | status positional ID    |

  @slice-11 @ID-RECOVERY-11 @linux
  Scenario Outline: Foreign or ambiguous ownership is never adopted by stop
    Given a recovery fixture with ownership defect "<defect>" and an independently observed candidate worker
    When recovery stop and status are invoked through each local backend
    Then stop reports recovery.not-owned with exit 1 and no stop kill remove or ownership adoption
    And every candidate worker and original record remains unchanged despite matching labels or names

    Examples:
      | defect                             |
      | no public registration             |
      | public ID belongs to other project |
      | mutually consistent other UID      |
      | different checkout identity        |
      | selected daemon differs            |
      | full ID observation conflicts      |
      | resource name differs              |
      | resource intent label differs      |
      | resource project label differs     |
      | resource fingerprint differs       |
      | resource image ID differs          |
      | name and full ID disagree          |

  # Missing original snapshot cannot fall back to a matching active approval.
  # Include unsafe ancestor/lock paths; checkpoint fixture substitutions separately.
  @slice-11 @ID-RECOVERY-12 @linux
  Scenario Outline: Unsafe or corrupt recovery material cannot authorize mutation
    Given a recovery fixture with protected material defect "<defect>" and an intact matching current approval
    When recovery status and stop are invoked through each local backend
    Then each recovery command reports "<rule>" with exit <exit> and no engine mutation or unsafe target access
    And original ownership evidence and outside sentinels survive without chmod chown or state repair

    Examples:
      | defect                            | rule                   | exit |
      | session ancestor symlink          | recovery.unsafe-state  | 1    |
      | record symlink                    | recovery.unsafe-state  | 1    |
      | record hard link                  | recovery.unsafe-state  | 1    |
      | record wrong filesystem owner     | recovery.unsafe-state  | 1    |
      | group-writable directory          | recovery.unsafe-state  | 1    |
      | world-readable record             | recovery.unsafe-state  | 1    |
      | controller lock symlink           | recovery.unsafe-state  | 1    |
      | operation lock symlink            | recovery.unsafe-state  | 1    |
      | unknown schema                    | recovery.invalid-state | 2    |
      | duplicate field                   | recovery.invalid-state | 2    |
      | unknown field                     | recovery.invalid-state | 2    |
      | missing field                     | recovery.invalid-state | 2    |
      | wrong workload exit type          | recovery.invalid-state | 2    |
      | trailing JSON                     | recovery.invalid-state | 2    |
      | oversized JSON                    | recovery.invalid-state | 2    |
      | overdeep JSON                     | recovery.invalid-state | 2    |
      | mismatched record ID              | recovery.invalid-state | 2    |
      | mismatched record project         | recovery.invalid-state | 2    |
      | mismatched intent reference       | recovery.invalid-state | 2    |
      | missing intent after registration | recovery.invalid-state | 2    |
      | missing original snapshot         | recovery.invalid-state | 2    |
      | corrupt original snapshot         | recovery.invalid-state | 2    |

  # Place the barrier AFTER the successful earlier inspection. A refusal based
  # on the changed authority must occur before the specific subsequent mutation.
  # Record replacement supplies a different well-formed binding at the pinned
  # filename, not an ordinary update advancing this session's progress.
  @slice-11 @ID-RECOVERY-13 @linux
  Scenario Outline: Stop revalidates authority at each destructive boundary
    Given a recovery fixture with a barrier before "<mutation>" and substitution "<change>"
    When stop reaches the barrier and the fixture substitutes that authority before releasing it
    Then stop returns recovery.not-owned with exit 1 before the barred mutation
    And the replacement resource or path is untouched and incomplete recovery remains discoverable

    Examples:
      | mutation | change                       |
      | SIGTERM  | socket inode replacement     |
      | SIGTERM  | daemon identity replacement  |
      | SIGKILL  | resource identity change     |
      | remove   | resource identity change     |
      | remove   | protected record replacement |
      | remove   | checkout identity change     |

  @slice-11 @ID-RECOVERY-14 @linux
  Scenario: A delayed uncertain create stays discoverable until its concrete identity can be recovered
    Given a recovery fixture with a registered intent-only operation and a delayed create invisible by name
    When status and stop run before visibility and a fresh stop retries after the fixture exposes the matching worker
    Then the first status and stop report recovery.temporary with unknown resource and no absent or stopped claim
    And the later stop reconciles matching name and full ID and durably records ownership before removal and confirmed completion
    And no recovery attempt creates starts duplicates or adopts an unmatched worker

  # The first stop really removes the old ID. Its response/completion can be lost;
  # a new same-named foreign resource must survive the reconstructed controller.
  @slice-11 @ID-RECOVERY-15 @linux
  Scenario Outline: Interrupted completion retains evidence for safe retry
    Given a recovery fixture with fault "<fault>" at the first stop's completion boundary
    When stop is attempted, the fault is cleared, a same-named foreign worker is seeded and a fresh process retries stop
    Then the first stop reaches recorded-ID removal before reporting recovery.temporary or losing its controller at that boundary without reporting successful durable completion
    And the later stop confirms the original full ID absent and publishes durable completion while preserving the replacement
    And the session remains discoverable throughout with its original observed or unknown workload exit

    Examples:
      | fault                                           |
      | lost remove response and inspection unavailable |
      | intent absent publication fsync                 |
      | session completion file fsync                   |
      | session completion parent fsync                 |
      | controller death after removal                  |

  # Assert real cross-process exclusion and reload, not just a fake call counter.
  @slice-11 @ID-RECOVERY-16 @linux
  Scenario: Concurrent stop attempts serialize one session and reload completion
    Given a recovery fixture with a running owned worker and a first-stop removal barrier
    When two independent stop processes overlap and the first barrier is released
    Then both stops complete with one serialized termination and removal sequence followed by confirmed absence
    And the waiting stop reloads durable completion without a second signal remove or completion rewrite
    And a concurrent status returns a consistent session or recovery.temporary within its budget without writing state

  # The run barrier is outside the operation lock, after durable publication but
  # before any create request. Keep run paused until stop acquires that lock and
  # returns; allowing create to finish first would test a different ordering.
  # The fixture configures a missing exact-name observation, not an absent full ID.
  @slice-11 @ID-RECOVERY-25 @linux
  Scenario: Stop before create retains uncertain cleanup and prevents a late launch
    Given a recovery run fixture with a barrier after durable registration and intent publication before any create request
    And the recovery engine fixture is configured with a missing intent name and no concrete resource ID
    When run reaches the barrier, an independent JSON stop finishes before run is released and fresh JSON status follows run's exit
    Then stop and the later status report recovery.temporary with exit 75 state failed resource unknown cleanup unknown workload exit null and the exact retry argv
    And stop durably records that cleanup began before releasing the session operation lock
    And resumed run returns run.cleanup-temporary with exit 75 and unknown workload exit without overwriting cleanup progress
    And no create start stop kill or remove request occurs and the original intent retains its empty concrete ID without an absent update
    And the registration and original snapshot remain discoverable without a stopped or complete claim

  # These boundaries already have a concrete owned ID. Park run outside the
  # operation lock and let stop finish first, then test run's stale continuation.
  @slice-11 @ID-RECOVERY-17 @linux
  Scenario Outline: Run and stop cannot race cleanup into a late launch or stale update
    Given a recovery run fixture with a transition barrier at "<boundary>"
    When run pauses outside the session operation lock at that barrier and an independent stop completes before run resumes
    Then stop returns zero with confirmed full-ID absence and durable cleanup and no run transition starts or recreates the session afterward
    And run preserves any observed workload exit without overwriting completion or inventing success
    And every engine mutation uses the same session lock and freshly validated ownership

    Examples:
      | boundary                     |
      | created before start         |
      | foreground wait              |
      | observed exit before cleanup |

  # A deadline while reopening after create is not evidence that stop won.
  # The test assembly expires only that Open context; teardown has its own budget.
  @wip @slice-11 @ID-RECOVERY-26 @linux
  Scenario: Run cleans its created worker after a session reopen deadline
    Given a recovery run fixture whose session reopen deadline expires after create
    When run attempts the launch and fresh JSON status follows its exit
    Then run removes the created worker and durably completes cleanup despite the reopen failure
    And run returns temporary failure without starting the worker or inventing a workload exit
    And status reports the registered session stopped with unknown workload exit and no pending retry
    And only the originally owned worker is removed while checkout edits approvals snapshots and unrelated resources remain

  # A long-held A operation lock must not block B through a project-wide lock.
  @slice-11 @ID-RECOVERY-18 @linux
  Scenario: Equal project basenames and concurrent sessions have independent recovery
    Given a recovery fixture with equal-basename projects A and B, two sessions in A and one in B
    When A's first stop is paused holding its session lock while A's second session is inspected and stopped and B is inspected
    Then A's second stop completes before the first lock is released and B lists only its own still-running session
    And stopping B's ID from A reports recovery.not-owned with exit 1 and does not contact B's resource
    And releasing A's first stop preserves B's worker and both projects' checkout edits approvals and snapshots

  # Include a completed tombstone whose daemon cannot now be reached: old absent
  # evidence never substitutes for this invocation's required confirmation.
  @slice-11 @ID-RECOVERY-19 @linux
  Scenario Outline: Recovery bounds unavailable observations and classifies their cause
    Given a recovery fixture with observation failure "<failure>"
    When status and stop are invoked through each local backend with measured monotonic deadlines
    Then recovery reports "<rule>" with exit <exit> within 10 seconds for status and 30 seconds for stop without resetting totals
    And no unavailable observation becomes absence a known workload exit or successful cleanup
    And safe ownership remains available for a later retry without resource mutation on an unvalidated identity

    Examples:
      | failure                       | rule                 | exit |
      | required socket missing       | recovery.environment | 3    |
      | inspection transport lost     | recovery.temporary   | 75   |
      | completed session daemon down | recovery.temporary   | 75   |
      | stalled engine call           | recovery.temporary   | 75   |
      | held operation lock           | recovery.temporary   | 75   |
      | malformed engine observation  | recovery.temporary   | 75   |
      | unclassified adapter failure  | recovery.internal    | 70   |

  # Deliberate output is a positive control, captured separately from diagnostics.
  # The approved snapshot may contain its literal environment; metadata may not.
  # Status observes pending cleanup before the fault is cleared. Stop then
  # completes cleanup, so only the earlier status carries an executable retry.
  @slice-11 @ID-RECOVERY-20 @linux
  Scenario: Recovery reports useful facts without copying sensitive input or output
    Given a recovery run fixture with distinct synthetic argv environment stream and engine-body markers and a cleanup fault
    When run streams its probe output, JSON status inspects the retained session and JSON stop is retried after clearing the fault
    Then status reports the real session ID original fingerprint cleanup pending and exact retry argv without any synthetic marker
    And stop returns zero with that session ID original fingerprint cleanup complete and retry null without any synthetic marker
    And controller diagnostics public recovery records intents and lock files contain no raw argv environment response bodies or copied stream bytes
    And the foreground stream still contains its deliberate output marker and the original approved snapshot is preserved

  # Native evidence supplements local fault mapping; DELETE failure happens only
  # after a real observed primary exit and is cleared solely for the public retry.
  @slice-11 @ID-RECOVERY-21 @engine
  Scenario: Run's printed stop command removes the native worker left by failed cleanup
    Given a CI recovery checkout with the cached image a marker workload and a removal-only transient fault
    When the product runs the workload to exit 7 and a fresh product process executes its printed stop after the fault is cleared
    Then the run reports exit 7 with a registered retry and the stop returns zero after native full-ID absence and durable completion
    And status retains workload exit 7 while checkout edits approvals snapshots and unrelated native resources survive
    And the product creates no replacement worker and harness cleanup is not counted as product completion

  # This case proves discovery/action after loss. Slice-12 separately proves the
  # network/filesystem boundary still holds; a heartbeat is not isolation evidence.
  @slice-11 @ID-RECOVERY-22 @engine
  Scenario: A fresh status finds a native worker whose foreground controller was killed
    Given a CI recovery checkout with the cached image and a long-lived heartbeat workload
    When the product starts the workload, the harness kills only its controller, then fresh product status and stop processes run
    Then status identifies the registered running worker with controller absent unknown workload exit and a concrete stop retry
    And the observer sees that worker's heartbeat after controller death and before stop
    And stop removes that exact native worker with confirmed durable completion without claiming successful workload execution
    And all checkout edits approvals snapshots and unrelated native resources remain intact

  @slice-11 @ID-RECOVERY-23 @engine
  Scenario: Native stop is confined to one of two equal-basename projects
    Given two CI recovery checkouts with equal basenames the cached image and independent heartbeat workloads
    When both product runs start and two independent stop processes target only the first project's session
    Then both stops succeed after the first worker's native absence while the second worker continues emitting new heartbeats
    And each project's status lists only its own session with its original fingerprint and cleanup outcome
    And both projects retain their edits approvals and snapshots and no shared image network or volume is removed
