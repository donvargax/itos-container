@phase-1
Feature: Recover owned checkout mount anchors
  Fresh status and stop processes account for the checkout anchor as well as
  the worker after interrupted startup, foreground cleanup or controller loss.
  Cleanup releases only the original owned volume after worker absence is
  established, preserving checkout edits and other sessions.

  # Authorities: docs/charter.md, docs/architecture.md (Slices), arc42 Recovery,
  # the offline lifecycle contract (AC-05/06/07/22/23/28), recovery.feature,
  # offline.feature's slice-17 block and the answered q-19 in tasks/asks.yaml.
  # Baselines inspected: main 75d9238 and slice-17 51a9f48. The latter contains
  # the anchor spec, decision 0020-descriptor-backed-checkout-anchor.md and
  # executable OFFLINE-29 steps, not the anchor implementation. q-22 records
  # native red for OFFLINE-29; it is not native green or slice-18 evidence.
  # On main, session/domain/recovery.go finishes after worker absence alone;
  # its ports, protected registry and presentation do not account for an anchor.
  # The expected failures below are source-based handoff, not executed red proof.
  # Coordinator q-36 (2026-11-07 ~18:00 UTC) merges this accepted specification
  # into slice-17. Implement producer and recovery together in that pull request.
  # Keep these IDs and their own red-first evidence; native 09-13 require CI red
  # before recovery implementation. The coordinator owns registry consolidation.
  #
  # Settled mechanism, inherited unchanged from q-19 and slice-17:
  # - An owned Docker local volume has type=none, o=bind and device naming the
  #   controller's O_PATH descriptor at /proc/<pid>/fd/<n>. The controller holds
  #   the descriptor until ContainerStart returns; dockerd binds the inode.
  # - Volume intent is durable before allocation; independently verified concrete
  #   volume ownership is durable before worker create. Q-27 selects detection
  #   immediately after ContainerStart returns. Q-39 refines it: hold a pidfd,
  #   re-inspect the same live PID, then stat /proc/<pid>/root/workspace against
  #   the validated device/inode. Mismatch or unavailable stat while still running
  #   requires immediate owned kill/removal and refusal. Confirmed exit before
  #   observation retains the actual wait result with mount_detector recorded
  #   and shown in status as "not observed: worker exited", never verified.
  #   The descriptor-backed volume prevents substitution; this detector cannot
  #   precede the first workload instruction. Detector failure fails native
  #   acceptance; no gate binary, extra mount or fallback is authorized.
  # - The worker's mount retains that inode after controller death. No helper,
  #   added host privilege, copied checkout or persistent worker data volume is
  #   introduced. The volume is session control infrastructure, released last.
  # - Preserve the sole nonrecursive writable checkout source, non-root identity,
  #   read-only root, NetworkMode none, capability restrictions and existing tmpfs.
  #   Neither the anchor records nor the engine socket enters the worker mount.
  # Slice-17 must supply its native green feasibility evidence. If Moby rejects
  # the descriptor source or resolves it by mutable pathname, stop under q-19;
  # this spec does not select a different mechanism or expand the charter.
  #
  # Recovery authority and lifetime decisions:
  # Reuse the public ID, original protected registration/intent/snapshot, anchor
  # discriminator and ownership binding produced by slice-17. Do not define a
  # competing anchor schema, derive ownership from engine labels alone, migrate
  # old records, or make a global resource scan. The original session/project,
  # fingerprint, host IDs, checkout identity, operation identity, socket/daemon,
  # concrete volume tuple and worker relationship must agree. Validate the exact
  # local driver/options and ownership labels against that original binding.
  # An expired descriptor path is stored identity data, not a path to reopen,
  # stat, readlink, remount or substitute. PID/descriptor reuse grants no authority.
  # Never reconstruct the anchor from the current checkout path, allocate a new
  # volume, restart a worker, execute a probe in it, or call approve in recovery.
  #
  # Use recovery's existing host-user/absolute-effective-root index without
  # opening the mutable checkout. Invoke from a separate accessible directory
  # with the original effective root when its path was replaced or made unusable.
  # Current YAML, manifest, Git entries and approval are not cleanup authority.
  # A record transplanted into a different root's index still confers none.
  # Preserve original protected records, approvals, snapshots and checkout bytes;
  # progress writes may change only this session's explicitly owned records.
  # An unknown mount-bearing version or missing required anchor record refuses,
  # never silently falls back to worker-only cleanup. Legacy no-anchor sessions
  # keep their existing interface and cleanup contract, including the separate
  # unstarted-only API; nearby similarly labelled volumes remain unrelated.
  #
  # Status remains read-only: no creation of locks/state, repair, fsync handoff,
  # ownership adoption or engine mutation. Stop shares run's per-session operation
  # lock and revalidates the original records, local socket/daemon and concrete
  # identities before each destructive operation, including the new volume delete.
  # Never hold a project transaction across engine calls or acquire another
  # session's operation lock. Controller absence is advisory, never authorization
  # to restart and never a prerequisite for explicit owned stop.
  #
  # Remove the revalidated worker by full ID, without automatic volume deletion,
  # then freshly confirm that full ID absent before releasing the owned anchor.
  # Workload exit, created/exited state, a DELETE reply, controller death, an
  # unavailable daemon and a name-only 404 with no worker ID are insufficient.
  # If the producer supplies durable proof that no worker create could have been
  # issued, recovery may use it only under the operation lock after fencing later
  # launch. An empty ID or a pre-create intent alone is NOT that proof. This item
  # invents no such proof or absent worker ID for legacy/uncertain records.
  # Reconcile an uncertain worker create through the exact original random name
  # and independently inspected full ID, recording concrete ownership durably
  # before mutation. Also verify its relationship to the original owned anchor.
  # Do not issue a replacement create or release a source an in-flight create
  # could still consume. Cleanup progress remains terminal to a resumed run.
  #
  # Volume release uses the recorded identity on the original daemon after fresh
  # inspection of its complete owned tuple. No prune, remove-volumes flag, force
  # override, host unmount, lazy detach or deletion of backing checkout contents.
  # A same-named volume with a different tuple is foreign even after an earlier
  # completion; refuse it rather than treating its name as the original's identity.
  # An in-use response is temporary uncertainty, never permission to remove an
  # additional worker. Confirm volume absence independently after delete; a lost
  # reply can still succeed if absence and subsequent durable completion are proven.
  # If absence cannot be confirmed, retain original ownership and exact retry.
  # Publish anchor absence and session completion durably using the existing
  # complete-write/file-fsync/atomic-publication/parent-fsync protocol. A visible
  # record after failed fsync is not a successful handoff. Repeated stop reloads,
  # revalidates and syncs completed evidence without repeated engine mutation or
  # rewriting completed records. Completion requires BOTH resource absences.
  #
  # Presentation choices extend recovery.feature's existing schema-1 envelope.
  # For a safely decoded anchor-bearing entry add anchor, one of present, absent,
  # unknown. Present means a freshly observed matching owned volume; absent means
  # independently confirmed absence of that recorded volume; unknown includes
  # unavailable or conflicting observations. This describes the VOLUME, not a
  # guarantee that its expired device path or stopped worker mount is usable.
  # Keep resource for the worker. Legacy no-anchor entries keep their old fields
  # without inventing an anchor or changing their completed-state interpretation.
  # Plain output reports the same separate worker/anchor/cleanup facts as JSON.
  # Never print the raw device path, engine response, argv, environment or output.
  #
  # A known remaining anchor means cleanup pending even with resource absent.
  # Two absent resources with incomplete records still mean pending. Status can
  # report complete only from safely loaded completed worker/anchor/session
  # records AND fresh absence of both resources; it never repairs durability.
  # An unavailable observation or uncertain write means failed/cleanup unknown,
  # never stopped/complete. Preserve independently verified worker facts when only
  # anchor observation fails. Existing controller/stop_candidate/retry rules hold;
  # incomplete safely identified sessions retain [itos,container,stop,<id>].
  # Preserve known workload_exit, including nonzero; unknown stays null unless a
  # genuine workload-exit observation is obtained. Stop zero means cleanup only.
  # Reuse recovery.not-owned (1), unsafe-state (1), invalid-state (2), environment
  # (3), temporary (75) and internal (70), with the recovery. prefix on each rule.
  # Unknown/missing/malformed required records are invalid-state; unsafe storage
  # is unsafe-state; mismatched ownership is not-owned. Unavailable inspection,
  # in-use/delete failure or uncertain durability is temporary. Safe independent
  # entries remain visible when another entry fails; retain sorting/exit aggregation.
  #
  # Existing totals include the new operations: 10 s per engine call, 10 s whole
  # status and 30 s whole stop, including lock waiting and durability. Retries and
  # reconnects never reset them. Reuse once-only SIGTERM, up to 10 s grace and
  # freshly revalidated SIGKILL within stop's total. No workload lifetime limit.
  # Do not write progress or start another destructive call after total expiry.
  # A later invocation may retry with a new budget and the original ownership.
  #
  # Implementation boundaries and dependencies:
  # Extend internal/slices/status and internal/slices/stop's thin command/domain
  # routes and shared internal/engine/session/{domain,port,<named adapters>}.
  # Run and recovery share anchor contracts/cleanup; no slice imports another.
  # Only cmd assembles, and I/O-free port/porttest fakes run the same contracts
  # as adapters. Reuse slice-17's anchor implementation and pinned maintained
  # Moby client v0.6.2/api v1.56.1, Go 1.27.3 and x/sys v0.48.0. No new library,
  # network service, privilege or domain allowlist expansion is selected. OS is
  # a value; preserve notices. Update arc42 Recovery/Building-block/Deployment
  # descriptions in the implementation and record a decision naming any section
  # overridden, including completion's extension from worker to worker plus anchor.
  # Existing recovery/foreground/isolation scenarios retain every ownership,
  # checkout, result and foreign-resource assertion. Only this session's proven
  # anchor release extends their old prohibition on any volume removal; adapt
  # independent inventories/oracles, never permit deletion of unrelated volumes.
  #
  # EVERY step phrase below is NEW. Add complete step definitions in a separate
  # red test commit with @wip retained. Reuse recovery_local_steps_test.go,
  # recovery_fault_steps_test.go, recovery_native_steps_test.go and their fixture
  # machinery; slice-17's offline_anchor_steps_test.go currently binds OFFLINE-29
  # only. Extend native anchor observations/recorders, not product test switches.
  # The new feature needs a smoke selection when its first scenario becomes live.
  #
  # Given handoff: local fixtures write disposable roots and protected state,
  # independently encoded slice-17 records, synthetic engine objects, probe files
  # and faults/barriers. No Given invokes product run/status/stop/approve or asserts
  # its behavior. Both local backends mean the real registered command/domain with
  # I/O-free memory ports AND production disk/SDK adapters against a Unix HTTP
  # fixture. Exercise the same positive control through both routes. Do not put
  # filesystem/network access in a fake or precompute command results in a stub.
  # Observe actual writes/fsyncs and all failed/successful API requests, not merely
  # file existence. Feature steps import no production packages; assembly is in cmd.
  #
  # Native Givens prepare independent approvals, trusted probes and recorder
  # settings only. Use features/README.md's guarded Linux amd64 CI tier and T-20's
  # inspected cached image, never a local engine, credentials or real state folder.
  # All real-engine setup, actions and cleanup stay @engine. Product processes use
  # actual disk/SDK adapters; native fault recorders may hold or reject named calls
  # but cannot fabricate successful create/inspect/remove or erase real resources.
  # Trusted probes execute only inside the worker. Launch/control failures, early
  # process exits and unreached barriers reach Then as data, never a hanging When
  # or a setup assertion. Bound barriers and retain complete per-process results.
  # Compare state before/after product actions, separating deliberate fixture
  # substitutions from product effects. Bounded identity-checked harness cleanup
  # runs even on failure, AFTER observations, and is never product cleanup proof.
  # Local assertion red/green is required for 01-08, and coordinator-dispatched
  # native red then ordinary green for every row of 09-13. A missing definition,
  # broken fixture/control, skip or prerequisite failure is not red evidence.

  # Inspection: seed all states independently, including incomplete records with
  # physically absent resources. Include a legacy no-anchor completed session
  # beside each row and a similarly labelled unrelated volume. Descriptor paths
  # refer to a gone fixture process; no process keeps a checkout descriptor alive.
  # All controller leases are absent: pending rows are stop candidates with the
  # exact retry, and the complete row is not a candidate and has retry null.
  # Today the first Then fails: no anchor observation exists in the public result.
  @slice-18 @ID-ANCHORS-01 @linux
  Scenario Outline: Status distinguishes retained anchors from completed cleanup
    Given an anchor recovery fixture with phase "<phase>" worker "<worker>" volume "<volume>" completion "<completion>" and observed exit "<exit>"
    When fresh anchor status processes inspect each local backend in plain and JSON modes
    Then each status reports state "<phase>" worker "<worker>" anchor "<volume>" cleanup "<cleanup>" and observed exit "<exit>" for the original session
    And each status retains the original fingerprint and the specified controller candidate and exact retry facts
    And anchor status leaves records locks resources and outside sentinels unchanged without opening checkout or descriptor paths
    And the legacy session keeps its original result without adopting or inspecting the unrelated volume

    Examples:
      | phase   | worker  | volume  | completion | exit    | cleanup  |
      | running | running | present | pending    | unknown | pending  |
      | failed  | exited  | present | pending    | 7       | pending  |
      | failed  | absent  | present | pending    | 7       | pending  |
      | failed  | absent  | absent  | pending    | unknown | pending  |
      | stopped | absent  | absent  | durable    | 7       | complete |

  # Normal recovery and idempotence. The last row is physically absent with
  # pending records, not a durable completion. Running fixtures supply no wait
  # exit observation, so termination cannot turn unknown into fabricated success.
  # Current input changes happen before checkpointing; the original effective
  # root remains the lookup key. The first Then fails today at anchor release or
  # acceptance of the supported mount-bearing ownership, not an undefined step.
  @slice-18 @ID-ANCHORS-02 @linux
  Scenario Outline: Stop releases an owned anchor last and is safe to repeat
    Given an anchor recovery fixture with an owned "<worker>" worker volume "<volume>" observed exit "<exit>" and changed current "<input>"
    When fresh anchor stop processes run twice and status follows through both local backends
    Then both stops confirm worker absence before anchor absence and durable completion with exit zero and workload exit "<exit>"
    And running workers receive only the bounded owned termination sequence and anchor deletion never requests force or other volume removal
    And the second stop reloads and syncs completion without another mutation completed-record rewrite or volume allocation
    And status reports stopped resource absent anchor absent cleanup complete and retry null for that session
    And anchor recovery preserves checkout edits approvals snapshots replacements and unrelated resources without reading current inputs

    Examples:
      | worker                   | volume  | exit    | input                   |
      | created                  | present | unknown | policy missing          |
      | running                  | present | unknown | Git directory replaced  |
      | running ignoring SIGTERM | present | unknown | checkout inaccessible   |
      | exited                   | present | 7       | approval superseded     |
      | recorded full ID absent  | present | 0       | checkout path replaced  |
      | recorded full ID absent  | absent  | unknown | manifest invalid        |

  # Worker uncertainty precedes volume release. In the lost-create row no worker
  # ID is recorded and exact-name inspection is 404 while the request can finish;
  # later expose its matching unstarted full ID and original anchor relationship.
  # Other rows retain a concrete ID and respectively fail removal with presence,
  # or lose removal/absence observations. Clear only the named fault for retry.
  # Today the first Then cannot describe and retain this owned anchor; a generic
  # invalid-state refusal for unsupported records is not the expected temporary.
  @slice-18 @ID-ANCHORS-03 @linux
  Scenario Outline: Uncertain worker cleanup retains the source until a fresh retry proves absence
    Given an anchor recovery fixture with worker uncertainty "<fault>" and a retained original owned volume
    When status and stop inspect the uncertainty and a fresh stop retries after the fixture resolves it through both local backends
    Then the first stop reports recovery.temporary with exit 75 and retained anchor present without any anchor delete or complete claim
    And the earlier status retains the original session anchor observation unknown workload exit and exact retry without mutating state
    And the retry durably owns any reconciled full ID before removal and releases only the original anchor after confirmed worker absence
    And neither attempt creates starts reallocates remounts or substitutes a checkout path and the retry completes durably

    Examples:
      | fault                                              |
      | create reply lost and exact-name lookup absent      |
      | worker removal fails with the full ID still present |
      | worker removal and full-ID absence remain unknown   |

  # Anchor-only failures must be reachable after real worker absence. A lost
  # DELETE reply alone is not failure: that row also loses follow-up inspection.
  # For durability faults the volume really is gone; retain its protected evidence
  # and do not report that it is still present. Clear the fault and retry from a
  # new process. Every row also runs a fault-free control with the same adapters.
  # The first Then fails today because stop never attempts an owned volume release.
  @slice-18 @ID-ANCHORS-04 @linux
  Scenario Outline: Anchor release and completion failures remain recoverable after worker removal
    Given an anchor recovery fixture with a recorded exit of 7 and persistent cleanup fault "<fault>"
    When stop is attempted and a fresh stop retries after clearing the fault through both anchor backends
    Then the first stop reaches confirmed worker absence and the anchor cleanup boundary before reporting recovery.temporary with exit 75
    And its anchor fact matches the actual observation and it retains original ownership exit 7 and exact retry without successful completion
    And the retry confirms both resources absent and establishes durable completion without recreating a volume or repeating confirmed removal
    And the fault-free anchor control completes worker removal volume release and durable completion in that order
    And anchor recovery preserves checkout edits approvals snapshots replacements and unrelated resources without reading current inputs

    Examples:
      | fault                                              |
      | volume deletion fails and the owned volume remains  |
      | volume deletion reports in use                     |
      | volume delete reply lost and inspection unavailable |
      | volume delete acknowledged but volume still present |
      | anchor absence short write                         |
      | anchor absence file fsync                          |
      | anchor absence parent fsync                        |
      | session completion file fsync                      |
      | session completion parent fsync                    |

  # Refuse anchor authority defects independently of worker absence. Each row
  # includes another valid session on the same effective root with both resources
  # already physically absent, so no overlapping worker is needed. Status must
  # retain that safe entry; an unrelated fault-free control owns/releases a real
  # synthetic anchor. Bind the safe entry/control to the actual selected daemon,
  # even when the defective entry binds a different one. Only the wrong-worker-
  # mount row needs a present unstarted worker to observe its conflicting source;
  # other rows use a confirmed-absent recorded full ID to isolate anchor checks.
  # The same-name foreign-volume row has prior durable completion, which cannot
  # authorize deletion of its replacement. Invalid rows that already refuse are
  # regression coverage; the positive anchor control distinguishes missing recovery.
  # Same-name replacement changes its concrete tuple, never asks the product to
  # infer identity from identical text. A /proc path may point at an unrelated
  # fixture descriptor; ownership comparison must not dereference it.
  @slice-18 @ID-ANCHORS-05 @linux
  Scenario Outline: Foreign unsafe or unsupported anchor evidence grants no cleanup authority
    Given an anchor recovery fixture with defect "<defect>" a safe independent entry and a fault-free anchor control
    When fresh status and stop inspect the defective session and the anchor control runs through both local backends
    Then the control releases its own anchor and the defective session reports "<rule>" with exit <exit> without anchor mutation or adoption
    And status retains the safe entry with anchor absent while the defective entry cannot claim completed cleanup
    And protected evidence foreign volumes workers and outside targets remain unchanged without repair or descriptor access

    Examples:
      | defect                                         | rule                   | exit |
      | selected daemon differs                        | recovery.not-owned     | 1    |
      | anchor belongs to a different host user         | recovery.not-owned     | 1    |
      | anchor belongs to a different project           | recovery.not-owned     | 1    |
      | anchor session label differs                    | recovery.not-owned     | 1    |
      | anchor operation label differs                  | recovery.not-owned     | 1    |
      | anchor fingerprint label differs               | recovery.not-owned     | 1    |
      | anchor driver differs                          | recovery.not-owned     | 1    |
      | anchor bind type differs                        | recovery.not-owned     | 1    |
      | anchor bind options differ                      | recovery.not-owned     | 1    |
      | anchor device names a reused foreign descriptor | recovery.not-owned     | 1    |
      | worker mount names a different anchor           | recovery.not-owned     | 1    |
      | recorded volume name now holds a foreign volume | recovery.not-owned     | 1    |
      | required protected anchor record missing        | recovery.invalid-state | 2    |
      | unsupported mount-bearing record version        | recovery.invalid-state | 2    |
      | duplicate anchor ownership field                | recovery.invalid-state | 2    |
      | anchor record symlink                           | recovery.unsafe-state  | 1    |
      | anchor record hard link                         | recovery.unsafe-state  | 1    |
      | anchor state ancestor writable by others        | recovery.unsafe-state  | 1    |

  # Pause after successful earlier observation and before the final authority
  # check for volume DELETE, with worker absence already confirmed. The fixture
  # performs the substitution, checkpoints it, then releases the barrier. A
  # decoder rejecting all new records cannot satisfy reaching this boundary.
  # Today the first Then fails because no anchor release boundary is reached.
  @slice-18 @ID-ANCHORS-06 @linux
  Scenario Outline: Stop revalidates anchor authority immediately before release
    Given an anchor recovery fixture with a final-release barrier and substitution "<change>"
    When stop reaches the anchor release barrier and the fixture substitutes authority before releasing it through both local backends
    Then stop reaches that boundary after worker absence and returns recovery.not-owned with exit 1 before the barred volume delete
    And no replacement target is changed and original incomplete anchor recovery stays discoverable with its exact retry
    And the matching no-substitution anchor control releases its owned volume and completes durably

    Examples:
      | change                           |
      | pinned socket inode replaced     |
      | selected daemon changed          |
      | same-named volume tuple changed   |
      | protected anchor record replaced |

  # Two stops contend on A's operation lock specifically during anchor release,
  # after worker removal. B is a disjoint equal-basename root, not an overlapping
  # checkout prohibited by slice-16. Use competing real processes for disk locks;
  # deterministic fake scheduling exercises the same contract. No project lock
  # may prevent B from completing while A remains parked.
  # Today the first Then lacks the serialized anchor release/completion sequence.
  @slice-18 @ID-ANCHORS-07 @linux
  Scenario: Concurrent anchor recovery serializes one session without blocking another
    Given disjoint equal-basename anchor recovery fixtures A and B with an A volume-release barrier
    When two fresh A stops overlap at that barrier while B is inspected and stopped before A is released through both local backends
    Then B completes its own worker and anchor cleanup while A is paused and both A stops later complete through one owned anchor deletion
    And the waiting A stop reloads completion without repeated termination removal volume deletion or completion rewrite
    And concurrent A status observes a consistent worker and anchor state or bounded recovery.temporary without state writes
    And stopping B's ID from A refuses ownership without contacting B and all unrelated resources and checkout edits survive

  # Spend most of the total before the new operation: status gets 8 s of prior
  # inspection; stop gets 25 s of lock/worker work. Hold the named call beyond
  # the remaining total. Fake clocks are controlled; adapter wall-clock checks
  # allow the existing fixed 2 s scheduling tolerance, not extra product budget.
  # With no new operation today, the first Then returns early without its fault
  # being exercised or lacks the required temporary anchor result.
  @slice-18 @ID-ANCHORS-08 @linux
  Scenario Outline: Anchor operations share recovery's original total budget
    Given an anchor recovery fixture configured to stall "<operation>" after prior delays consume most of the invocation budget
    When the anchor recovery command runs and a fresh stop retries after the stall is cleared through both local backends
    Then the named anchor operation is reached and reports recovery.temporary with exit 75 within the original <seconds> second total
    And no expiry becomes anchor absence successful cleanup or authority for another destructive call or progress write
    And original ownership and workload knowledge survive and the fresh retry completes both resource absences durably

    Examples:
      | operation             | seconds |
      | status anchor inspect | 10      |
      | stop anchor inspect   | 30      |
      | stop anchor delete    | 30      |

  # Native loss: prepare original/replacement nonce trees and a trusted live
  # probe. After a healthy in-container handshake, move the original aside,
  # install a harmless replacement at the old path and kill ONLY the controller.
  # Recover from outside with the original effective root. The probe reports a
  # second device/inode, original nonce and successful original-tree edit after
  # controller death, while denying the replacement nonce and actual protected
  # anchor records. Its worker-local read/write controls succeed before/after.
  # Observe the same volume's identity/options without trying its expired device
  # path; no keeper process or extra descriptor extends the controller's life.
  # Status precedes stop; capture all observations before harness cleanup.
  # This proves mount continuity after loss, not slice-15's network probe matrix.
  # Today the first Then lacks the anchor-aware discovery even if worker discovery
  # already works. After slice-17 it must still fail specifically in recovery.
  @slice-18 @ID-ANCHORS-09 @engine
  Scenario: Fresh recovery preserves the anchored checkout after controller loss and path replacement
    Given a CI anchor recovery checkout with the cached image original and replacement nonce trees and a live identity probe
    When run reaches the live probe barrier the fixture replaces the checkout path kills its controller and fresh status and stop processes run
    Then status identifies the original running session with controller absent anchor present unknown workload exit and the exact stop retry
    And the live probe after controller death still sees the original checkout inode and edits while replacement and anchor control reads are denied with working controls
    And native inspection retains the same owned volume and unchanged offline boundary until stop confirms worker absence and releases that volume last
    And fresh repeated stop and status confirm both absences and durable completion without restarting work or claiming workload success
    And native anchor recovery preserves original edits replacement trees approvals snapshots and unrelated resources before harness cleanup

  # Follow OFFLINE-30's retained state with an actual public retry. The recorder
  # rejects only worker removal while real create/start/wait/inspect work. Allow
  # the probe to exit and the controller to return before clearing that fault;
  # the descriptor is gone. Execute exactly the printed stop suffix as argv,
  # never a shell. Observe both resource absences before harness cleanup.
  # The first Then fails today at recovery of a retained original anchor; no
  # anchor record is not an acceptable substitute for successful anchor cleanup.
  @slice-18 @ID-ANCHORS-10 @engine
  Scenario Outline: Run's retry releases its retained native anchor after foreground cleanup fails
    Given a CI anchor recovery checkout with the cached image a marker workload and a worker-removal-only transient fault
    When the anchored workload exits <child> and run returns then fresh processes execute its exact stop retry twice after the fault is cleared
    Then both stops return zero after confirmed native worker and original volume absence and durable anchor completion
    And run preserves observed exit <child> returns <result> and reports separate run.cleanup-temporary with the registered retry
    And the original volume remains owned through controller exit and is deleted only after worker absence without opening its expired descriptor path
    And status retains workload exit <child> with anchor absent cleanup complete and retry null without repeated resource mutation
    And native anchor recovery preserves original edits replacement trees approvals snapshots and unrelated resources before harness cleanup

    Examples:
      | child | result |
      | 0     | 75     |
      | 7     | 7      |

  # Submit real create to the daemon but hold its response before the controller
  # receives the full ID; kill only that controller. Native inspection must show
  # the concrete unstarted worker and original volume. Also cover acknowledged
  # create followed by start succeeding with its response held: the worker may
  # run and keeps its inode after death. No lost response authorizes another
  # launch. The recorder must forward responses normally for the fresh recovery
  # process. Both barriers race process exit/deadline into the Then observations.
  # Today worker-only reconciliation cannot satisfy the first Then's original
  # anchor relationship and last-release observation.
  @slice-18 @ID-ANCHORS-11 @engine
  Scenario Outline: Native recovery reconciles uncertain startup without recreating the anchor
    Given a CI anchor recovery checkout with the cached image and a recorder holding the successful "<operation>" response
    When run submits that operation the fixture kills only its controller and fresh status and stop processes recover the session
    Then recovery verifies the original worker and anchor relationship and confirms worker removal before deleting that same native volume
    And status before stop reports the retained anchor and original session without adopting ownership or inventing a workload exit
    And any recovered worker ID is durably recorded before mutation and recovery issues no create start exec mount or volume allocation
    And stop and its fresh retry confirm durable completion while preserving any observed workload result and all unrelated native state

    Examples:
      | operation |
      | create    |
      | start     |

  # Remove the worker normally, but block product volume deletion so the first
  # stop retains a retry. Only AFTER independent full-ID absence may the trusted
  # CI fixture remove that original volume and create a same-named foreign local
  # volume with a different ownership tuple. Checkpoint these fixture mutations.
  # Fresh recovery must refuse even though the name matches, while a separate
  # valid control still releases its own anchor. The first Then demands that
  # product anchor-release boundary; missing launch is never a passing refusal.
  @slice-18 @ID-ANCHORS-12 @engine
  Scenario: A foreign native volume at the recorded anchor name is never removed
    Given a CI anchor recovery checkout with the cached image and a volume-delete-only fault plus an independent valid control
    When run reaches its live barrier stop removes the worker and the fixture substitutes a foreign volume before fresh status and stop inspect it
    Then the first stop has reached owned volume release after confirmed worker absence and the fresh commands report recovery.not-owned with exit 1
    And native inspection shows the foreign replacement unchanged with no product volume deletion adoption or completed-session claim
    And the independent native control completes worker and anchor cleanup and both checkouts retain their edits approvals and snapshots

  # Use disjoint equal-basename projects A/B, each with a working heartbeat and
  # inode probe. Kill A's controller only; keep B live. Park A's first stop at
  # volume release after worker absence, start its second stop and observe B's
  # increasing heartbeat/unchanged volume before releasing A. Product status
  # cannot inspect B's resources through A. Harness removes B only afterward.
  # Today the first Then cannot observe A's serialized last-anchor release.
  @slice-18 @ID-ANCHORS-13 @engine
  Scenario: Recovering one native anchor leaves an independent anchored session live
    Given two disjoint equal-basename CI anchor checkouts with the cached image independent inode probes and an A anchor-release barrier
    When both workloads reach their barriers A loses its controller and two fresh A stops overlap while B continues running
    Then both A stops complete with one native anchor deletion after A worker absence while B produces later heartbeats from its original checkout inode
    And B retains its original worker volume ownership and offline boundary and each status lists only its own session and anchor
    And A cannot stop B's ID and neither repeated stop nor harness observations remove any unrelated native resource
    And native anchor recovery preserves original edits replacement trees approvals snapshots and unrelated resources before harness cleanup
