@phase-1
Feature: Recovery refuses worker-exposed control state
  A session ID and matching resource labels cannot authorize cleanup from records
  a worker can change. Status and stop establish separation before trusting the
  selected recovery state, including after controller loss or state-root changes.

  # Authorities: docs/charter.md, container-pilot.md AC-02/06/08/23/28/36,
  # docs/architecture.md (Slices), arc42 Recovery, recovery.feature, slice-21's
  # shared control-state contract and slice-22's mount exclusion. This file
  # specifies slice-23 only.
  # Inspected baselines:
  # main     88da3e19a01a26fa7171bd2bae2950d9cedb492c
  # slice-16 a15e59b96ef0b7f145054cde34dfb3abd6776edb
  # slice-21 d78be4b6fb4bb75cd1c7007d3e8df56e982e1040 (spec only)
  # slice-22 148f6546776ca30c7a72de334d839b6c51058119 (spec only)
  # Read control-state.feature on slice-21 and checkout-overlap.feature plus
  # docs/decisions/0014-checkout-admission.md on slice-16. Main's Registry.List,
  # Registry.Open and Session.Validate check selected storage and original
  # ownership, but never another root's reservations. Slice-16 adds admission
  # completion and reserved/cancelled pre-create intents, not this trust check.
  # Require COMPLETED slice-16, slice-21 AND slice-22 before implementing this
  # item or collecting its assertion-level red evidence. Slice-21 serializes
  # publication/registration but leaves refusal of subsequent exposing mounts
  # to slice-22. Its completed binding alone cannot prove continuous protection.
  # Implement against completed dependencies, not copies of branch code. The
  # registry still lists only slice-21; the coordinator must add slice-22 before
  # dispatch. No dependency is considered complete merely because its spec exists.
  #
  # Required dependency correction, also recorded in slice-22's
  # run-control-state.feature: pinned slice-21 compares a worker equal to or above
  # a managed tree, missing a worker INSIDE that tree. Its shared inspection and
  # locked publication recheck must reject both containment directions, with
  # red/green evidence, before this item's descendant case can be implemented.
  # Do not put a divergent comparison or second catalog in status/stop to hide
  # that gap. Record the corrected dependency revision with the step handoff.
  #
  # Shared separation and trust:
  # Reuse slice-21's effective-UID catalog and fixed account-home descriptor lock,
  # independent of process HOME, XDG_STATE_HOME, project and selected daemon.
  # Reuse its pinned identities, self-binding, pending location claims, durable
  # enrollment and legacy-evidence validation. The fixed bootstrap footprint and
  # each entire managed itos-container tree remain protected, not just today's
  # record files. Whole-component equality or containment in EITHER direction
  # intersects a worker checkout; textual prefixes and equal basenames do not.
  # Preserve own-checkout exclusion. No daemon query proves filesystem separation.
  #
  # Before accepting the selected session index (including an empty result),
  # original snapshot or intent, establish separation using the shared catalog
  # and all discoverable outstanding reservations across selected/enrolled roots.
  # Check known conflicts from independently protected evidence before treating
  # an untracked selected root as merely unavailable. Preparing, uncertain-create,
  # controller-absent and incomplete-cleanup reservations still block. Use
  # slice-16's durable completion or proven pre-create cancellation rules; a PID,
  # lease release, workload exit, DELETE reply or one completed session is not
  # completion of every reservation. Exposed bytes can never clear a blocker or
  # authorize a resource operation, even when their hashes and labels agree.
  #
  # A selected root containing recovery evidence needs a completed, validated
  # location binding AND independently protected evidence that publication and
  # mount exclusion covered it before those bytes were first written and have
  # remained in force. Current separation is necessary but cannot prove that
  # history. A completed binding created before slice-22's exclusion, or with
  # unknown protection provenance, remains untrusted legacy evidence even when
  # its path, UID and inode still match and every exposing W reservation completed.
  # Never infer provenance from the running binary version, file timestamps,
  # matching hashes/labels, a marker inside the selected tree or an empty scan.
  #
  # The shared catalog contract must distinguish full-exclusion bindings from
  # pre-exclusion/unknown-origin bindings durably and without trusting selected
  # recovery bytes. Its protected provenance is established under the shared
  # transaction before the first recovery publication; only the protocol with
  # slice-22 mount exclusion may issue it. Preserve old bindings as location
  # blockers, but never upgrade their protection provenance by rereading/resyncing
  # them, reapproving, reenrolling or stamping already-existing recovery records.
  # Use the shared engine contract and fake/disk fixtures, not a recovery-only
  # catalog or a flag inferred by status/stop. The step handoff must identify the
  # reviewed writer/reader representation and dependency revisions that supply
  # this distinction; pinned slice-21's spec does not supply it yet.
  #
  # A missing binding or pending claim also leaves old bytes unproven. Recovery
  # never enrolls, manufactures or upgrades protection evidence. With no known
  # outstanding exposure, missing historical protection is recovery.temporary
  # (75), including a structurally valid pre-exclusion binding; it is not a
  # malformed-record error. W cleanup and upgrading the controller cannot
  # authenticate these bytes. This specifies refusal, not migration, automatic
  # quarantine/repair, or a new way to approve old records. Unseen pre-upgrade
  # roots and mixed old/new controllers remain slice-21's upgrade limitation.
  #
  # Catalog discovery supplies separation evidence only. Public discovery stays
  # within the effective checkout/UID and SELECTED state root; never search other
  # roots for a requested ID, silently fall back to the old root, copy records or
  # adopt an engine resource. Restoring the original safely bound root permits
  # ordinary ownership-checked recovery. A safely separated empty selected root
  # still gives status's empty success and stop's recovery.not-owned, without
  # enrollment, directories, lock files or an engine connection. With no catalog,
  # only slice-21's safe empty/bootstrap-only case permits that empty observation;
  # missing required discovery evidence is not equivalent to no sessions.
  #
  # Status never creates, enrolls, rewrites, syncs, repairs or removes state. Stop
  # may change only its safely validated target's ordinary progress, absent or
  # cancelled intent, stopped record and admission completion, using existing
  # ownership checks. Preserve location bindings, approvals, snapshots, completed
  # recovery records, other sessions, checkout edits and unrelated resources.
  # Neither command approves, launches, prunes or executes checkout helpers.
  # Current policy/manifest/approval and writable checkout/Git access are still
  # unnecessary for recovery; do not replace the check with launch readiness.
  # A safe location never substitutes for original UID/root/socket/daemon/name/
  # full-ID/image/label validation. Unknown workload exit remains unknown unless
  # explicitly observed; stop success means cleanup, not workload success.
  #
  # Freshness, locks and failure boundaries:
  # Inspect under the shared exclusion and pinned-identity rules, without holding
  # it while acquiring a session operation lock, doing engine I/O or waiting for
  # termination. After acquiring that operation lock, re-establish separation
  # before using a loaded index/record/snapshot as authority. Revalidate through
  # the existing Session/Intent storage boundaries and immediately before each
  # signal/remove, progress publication, completion certificate and repeated-stop
  # durability sync. Include the engine-free reserved/cancelled path. Reuse the
  # shared lock order; scanning must not acquire another session's operation lock.
  # A stale successful check cannot survive unavailable or substituted discovery
  # evidence. No new timeout: status totals 10 s; stop totals 30 s, including
  # separation, locks, calls, waits and durability, without resetting on retry.
  #
  # A separation failure BEFORE loading authority produces no trusted session
  # entry, stop candidate or executable retry from that state. If separation is
  # lost after an earlier valid observation, omit that now-unproven entry too.
  # Do not write last_problem, cleanup progress, an absent tombstone or completion
  # certificate using failed separation. Earlier legitimate effects remain;
  # neither roll them back nor continue mutations. Retain evidence for host review.
  # Preserve recovery.feature's independent safe-entry handling for per-record
  # defects AFTER successful root separation; a root-level failure covers all
  # its entries and must not be presented as a successful empty inspection.
  #
  # Reuse recovery's public rules and schema, with these explicit mappings:
  # known outstanding exposure or unsafe path/owner/mode/link -> recovery.unsafe-state (1);
  # malformed/inconsistent catalog binding -> recovery.invalid-state (2);
  # missing required binding/catalog/root or full-exclusion provenance,
  # unavailable lookup/read/lock, or deadline exhaustion -> recovery.temporary (75).
  # These are state failures, not recovery.environment's engine-environment
  # failure. Usage still wins
  # before state access. Do not turn failed separation into recovery.not-owned.
  # JSON is one schema-1 object, ok=false, nonempty complete typed problems and
  # sessions:[] for a root refusal. Plain output has the same rule/meaning.
  # A problem may name the lexically validated stop argument; it must not derive
  # project/fingerprint/exit/retry facts from untrusted records. Explain exposure
  # or unverified separation and direct the host user to inspect original protected
  # state outside worker checkouts. No force, reapproval-as-repair, auto-stop,
  # prune or relocation command is suggested. Never echo raw records, environment,
  # argv, engine bodies or synthetic sensitive markers into output or new metadata.
  #
  # Architecture and dependencies:
  # Keep status/stop command slices thin. Shared separation/recovery contracts,
  # pure decisions, I/O-free porttest fakes and named disk adapters belong in
  # engine packages, reusing slice-21 rather than importing another slice. Only
  # cmd wires adapters. Run shared contracts against memory and disk. Reuse the
  # pinned Go 1.27.3 APIs, golang.org/x/sys v0.48.0 descriptor locks and Moby
  # client v0.6.2/api v1.56.1 already selected by the dependency specs; no new
  # library, service or allowlist change. The implementation records affected
  # arc42 Building-block view/Recovery sections in a decision and updates them.
  #
  # Fixture and red-first handoff -- EVERY step sentence in this file is NEW:
  # Reuse recovery_local_steps_test.go, recovery_state_steps_test.go,
  # recovery_fault_steps_test.go, approval_fixture_test.go's independent oracle,
  # cmd/itos-container/public_recovery_probe_test.go's assembly seams and the
  # recovery syscall observers. Reuse slice-16's multi-project HTTP fixture and
  # slice-21's eventual catalog fixtures; do not copy their single-root defaults.
  # These are Linux amd64 local Engine API/real disk cases, NOT @engine cases.
  # They prove trust/refusal and persistence, not effective native isolation;
  # slice-22 owns actual mount evidence in CI. No daemon or Docker CLI is used.
  #
  # Givens create private temporary ordinary Git checkouts P and W, disjoint from
  # one another, safe candidate inputs, synthetic matching workers and independent
  # original records. P is the recovery target. W has its own safely enrolled
  # state outside BOTH checkouts and an independently seeded matching approval.
  # The account-home anchor is outside both. API sockets and journals are outside
  # every checkout. Shared/distinct-daemon rows retain the SAME effective UID and
  # account home. Ordinary files have one link; private modes and matching ownership
  # eliminate existing path/mode/foreign-resource refusals as explanations.
  #
  # In the unbound exposed-legacy cases, P's selected managed tree T intersects W
  # as the row says but is NOT enrolled. Seed P's consistent session/index,
  # intent/snapshot and matching HTTP worker independently there; do not obtain
  # them through a product launch that the dependencies must already refuse.
  # W is admitted through the product from its separate safe root in When; an
  # unseen legacy T cannot be discovered until P selects it. For a descendant,
  # W is inside T at P's snapshot folder; ordinary .git/request files coexist
  # with the seeded snapshot, but no symlink, extra link or overlapping P checkout
  # is used. This is an unsafe existing-state fixture, never permission to create
  # that layout. Preparing P has a durable reserved intent and public record;
  # cancelled P has cancelled/stopped records; completed P has an absent concrete
  # ID and completed records. The slice-16 dependency supplies those schemas.
  #
  # Safe controls and discovery-fault cases instead seed a completed original
  # binding with independently protected full-exclusion provenance predating
  # the recovery records, except where that evidence is the named defect.
  # Their original recovery authority is intact. Unknown workload results use
  # null; include a known nonzero result in completion controls.
  # Use distinct synthetic markers in original snapshots/engine errors; no real
  # credential. Fixtures never execute a checkout helper on the host.
  # All child HOME/XDG/catalog paths must stay under the scenario's private parent.
  # Substitute the account-home lookup through slice-21's test-only cmd assembly,
  # not HOME alone: production lookup must never reach the person's state.
  # Use production command composition/disk/SDK adapters otherwise. No production
  # testing flag, fake separation verdict or weakened ownership validator.
  # Existing positive recovery fixtures must carry independently seeded safe
  # bindings with that provenance too; preserve their ownership/refusal assertions.
  # Keep unbound AND pre-exclusion bound legacy cases explicit instead of silently
  # enrolling or upgrading them in a common helper. Do not weaken inventories.
  #
  # No Given invokes a product command or asserts an outcome. When records all
  # results, exits, journals and bounded barrier outcomes, including an unexpected
  # refusal, success or missing barrier; Then judges them. Product W launches and
  # recoveries are independently evidenced, not assumed by setup. Use separate
  # processes for locks and fresh recovery; barriers observe real operations or
  # test-only I/O seams, never sleeps. Faults affect shared discovery only, leaving
  # P's original records and engine route valid so old checks cannot mask the gap.
  #
  # Inventory bytes, inode/mtime, owner/mode/link targets and new entries before
  # each target command. Separate fixture mutations and legitimate W/Q progress
  # from P effects; never whitelist a whole state/catalog directory. Record every
  # engine request by invocation/full ID. Known-exposure refusals before loading
  # P authority must not connect to P's engine or W's daemon. Independent harness
  # observations are separate. For successful stop, confirm full-ID absence and
  # actual successful record/file/parent fsync; mere visibility is not durability.
  #
  # Commit complete definitions red first with @wip retained. Temporarily remove
  # @wip for local assertion-level red, then implement and obtain green. Use a
  # baseline containing completed slice-16/21/22, the containment correction and
  # the shared provenance distinction above; document exact revisions. Missing
  # catalog/provenance fixtures, setup/undefined-step failures or already-passing
  # controls are not red evidence. The first failing
  # Then expected from inspected code is stated beside each scenario below.

  # The first Then fails today: recovery trusts matching P records, reports them
  # and stops their worker (or syncs/cancels completion) without consulting W.
  # Known exposure precedes the missing-enrollment refusal for this legacy T.
  @wip @slice-23 @ID-RECSTATE-01 @linux @local-engine-fixture
  Scenario Outline: Recovery refuses a managed tree intersecting another worker checkout
    Given a recovery-separation fixture with untracked P tree "<placement>" relative to W and P authority "<authority>" on "<engines>"
    When W starts and fresh P status and stop inspect the selected tree in "<format>"
    Then both P commands report recovery.unsafe-state with exit 1 and exposure guidance without trusted session entries or a stop retry
    And W is independently running with its original full ID and an outstanding protected reservation
    And the refused P commands make no engine connection and leave all records locks locations checkout edits and unrelated resources unchanged
    And recovery output and metadata contain none of the synthetic sensitive markers

    Examples:
      | placement                     | authority            | engines          | format |
      | below W root                  | running worker       | shared daemon    | JSON   |
      | equal to W root               | created worker       | distinct daemons | plain  |
      | contains W at snapshot folder | running worker       | distinct daemons | JSON   |
      | below W root                  | reserved pre-create  | shared daemon    | JSON   |
      | below W root                  | cancelled pre-create | distinct daemons | plain  |
      | below W root                  | completed absent     | shared daemon    | JSON   |

  # The first Then fails today because W's outstanding reservation, including
  # after lease loss, is not consulted. Preparing has registration but no create;
  # other rows observe their specified worker/fault, not merely a killed process.
  @wip @slice-23 @ID-RECSTATE-02 @linux @local-engine-fixture
  Scenario Outline: Controller loss and uncertain cleanup do not make exposed recovery trustworthy
    Given a recovery-separation fixture with untracked P state below W and a W transition plan "<condition>"
    When W reaches the planned condition and fresh P status and stop run from another state root
    Then both P commands report recovery.unsafe-state with exit 1 while the independently evidenced W reservation remains unresolved
    And neither command trusts P entries contacts either engine mutates resources or changes persistent state

    Examples:
      | condition                                                 |
      | preparing after registration before create                |
      | controller killed with worker still running               |
      | controller killed and W daemon observation unavailable    |
      | DELETE acknowledged while the full worker ID remains      |
      | full worker ID absent but durable completion failed       |
      | one of two same-root W reservations durably completed     |

  # Run P from its original safe enrolled tree and W from its own safe tree.
  # Kill only P's controller, then copy complete P records or leave no P index in
  # the newly selected exposed tree. No new location binding is forged. Changes
  # to HOME affect selection only, not the fixture's fixed account-home anchor.
  # The first Then fails today: the copy is trusted, or empty state hides the
  # conflict with status success/not-owned. Later safe controls are regressions,
  # including original-root recovery despite invalid current policy and lost Git.
  @wip @slice-23 @ID-RECSTATE-03 @linux @local-engine-fixture
  Scenario Outline: Changing state selection cannot hide exposure or adopt copied recovery
    Given a recovery-separation fixture with safe enrolled P and W state and a planned "<selection>" containing "<contents>"
    When P and W start then P loses its controller and fresh P status and stop use the planned exposed selection
    Then both P commands report recovery.unsafe-state with exit 1 and no trusted entries instead of using copied records or reporting an empty success
    And original P and W workers remain independently present and both original state trees and location bindings remain unchanged
    When P selects an empty disjoint textual-prefix state root and repeats status and stop
    Then status succeeds empty and stop reports recovery.not-owned with exit 1 without searching other roots creating state or contacting an engine
    When P restores its original state selection with invalid current policy and missing Git and fresh status and stop run outside its checkout
    Then status identifies the original controller-absent session and stop durably removes only its originally owned full worker ID
    And W stays active while original approvals snapshots location bindings edits and unrelated resources survive with no invented workload success

    Examples:
      | selection              | contents                 |
      | changed XDG_STATE_HOME | a complete P record copy |
      | changed XDG_STATE_HOME | no P session index       |
      | changed HOME fallback  | a complete P record copy |
      | changed HOME fallback  | no P session index       |

  # Every defect is outside the existing P record/intent/snapshot validation
  # path; selected-root binding faults leave those bytes and engine observations
  # valid. The first Then fails today because discovery is ignored. Missing
  # binding/catalog rows contain real recovery evidence, not an empty bootstrap.
  @wip @slice-23 @ID-RECSTATE-04 @linux @local-engine-fixture
  Scenario Outline: Unverifiable shared separation refuses recovery without repairing state
    Given a safely owned recovery-separation fixture with shared discovery defect "<defect>"
    When fresh JSON P status and stop inspect that fixture within their existing total budgets
    Then both P commands report "<rule>" with exit <exit> and no trusted entries or executable retry
    And neither command connects to an engine creates or syncs state enrolls a location repairs the defect or changes any original record
    And the failure output identifies unverified separation without leaking synthetic record or engine markers

    Examples:
      | defect                                                  | rule                   | exit |
      | selected recovery tree has no location binding           | recovery.temporary     | 75   |
      | selected recovery tree has only a pending location claim | recovery.temporary     | 75   |
      | completed binding predates mount exclusion               | recovery.temporary     | 75   |
      | catalog absent beside existing recovery evidence         | recovery.temporary     | 75   |
      | another cataloged tree is missing                        | recovery.temporary     | 75   |
      | another cataloged reservation cannot be read             | recovery.temporary     | 75   |
      | host account home lookup is unavailable                  | recovery.temporary     | 75   |
      | another cataloged tree is replaced at the same path       | recovery.unsafe-state  | 1    |
      | coordination ancestor is a symlink                       | recovery.unsafe-state  | 1    |
      | catalog binding has a second hard link                   | recovery.unsafe-state  | 1    |
      | coordination directory allows another user to write      | recovery.unsafe-state  | 1    |
      | catalog binding is truncated                            | recovery.invalid-state | 2    |
      | catalog binding names another effective UID              | recovery.invalid-state | 2    |

  # Inject an unavailable shared-discovery read after an earlier successful
  # separation check, at the last revalidation boundary BEFORE the named effect.
  # P's ordinary storage and engine stay healthy. Before SIGKILL, SIGTERM/grace
  # already occurred; before completion the full ID is already confirmed absent.
  # For repeated sync, completion already exists. Observe exact earlier effects
  # and inventory at the barrier. The first Then fails today because stop carries
  # on with the barred effect. A cleared read fault is not repair of exposed data.
  @wip @slice-23 @ID-RECSTATE-05 @linux @local-engine-fixture
  Scenario Outline: Stop revalidates separation before each mutation and durable handoff
    Given a safely bound recovery-separation fixture with a stop barrier before "<boundary>"
    When stop reaches the barrier and shared discovery becomes unreadable before the command resumes
    Then stop reports recovery.temporary with exit 75 and no trusted entry or retry before the barred effect
    And no engine mutation progress write completion certificate or durability sync occurs after separation becomes unavailable
    And the earlier legitimate effects remain recorded without changing unrelated state or releasing persistent location protection
    When only the discovery read fault clears and a fresh process retries the same stop
    Then owned cleanup completes durably with fresh identity checks while original locations approvals snapshots and known or unknown workload outcomes are preserved

    Examples:
      | boundary                           |
      | stopping progress publication      |
      | SIGTERM                            |
      | SIGKILL                            |
      | remove                             |
      | absent intent publication          |
      | admission completion publication   |
      | repeated completed-record sync     |

  # Hold P's operation lock in a separate fixture process. Once the command has
  # reached that lock attempt, let disjoint Q's product stop finish, then inject
  # a P-only shared-read fault and release P. Q has independently valid bound
  # records and its own worker. This observes both no common lock across the wait
  # and revalidation afterward. The first Then fails today: P resumes with the
  # stale successful view and reports/cleans its session instead of the refusal.
  @wip @slice-23 @ID-RECSTATE-06 @linux @local-engine-fixture
  Scenario Outline: Waiting for one session cannot freeze discovery or retain stale separation
    Given safely bound recovery-separation projects P and Q with an independent holder of P operation lock
    When P "<command>" waits for its operation lock while Q stop runs then P shared discovery fails before the holder releases P
    Then P reports recovery.temporary with exit 75 and no trusted entry or retry after acquiring the operation lock
    And Q completes owned cleanup before P lock release without changing P records or either project's location binding
    And P performs no engine operation or state write using its earlier separation observation

    Examples:
      | command |
      | status  |
      | stop    |

  # Seed a historical state, not an unsafe launch through the corrected product:
  # P's completed binding predates mount exclusion; later W used a separate state
  # root and mounted a checkout containing T, changed P's public workload_exit
  # from null to 17, then completed every reservation. P and W checkouts are
  # disjoint; W state and the catalog are outside both and were not exposed.
  # Given writes the pre-exclusion binding, otherwise consistent original
  # P snapshot/intent/index and edited public record, plus W's retained absent
  # intent, stopped record and durable completion certificate. The HTTP fixture
  # has no W full ID. Keep independent pre/post-edit byte images; fixture fsync
  # supplies persisted history, not a claim that this product performed cleanup.
  # No old binary, Docker worker or host checkout helper is executed. Do not
  # delete P's completed binding or leave an outstanding W blocker to get refusal.
  # Original P resource bindings still match, and all modes/links are safe.
  # The first Then fails today: current recovery reports the edited facts or
  # cleans/syncs P. A current-reservations-only check would also accept these
  # bytes. Both fresh attempts must refuse the missing historical protection;
  # the running row guards cleanup and the completed row guards read/sync paths.
  @wip @slice-23 @ID-RECSTATE-08 @linux @local-engine-fixture
  Scenario Outline: Completed exposing workers cannot restore authority to pre-exclusion bound records
    Given a recovery-separation history with P authority "<authority>" bound before mount exclusion and W completed after altering P records
    When fresh P status and stop inspect that retained bound history twice in "<format>"
    Then every P command reports recovery.temporary with exit 75 and unproven-protection guidance without trusted entries or an executable retry
    And independent observations show the completed P binding survives the record edit and all exposing W reservations have durable completion with no W worker remaining
    And no P invocation connects to an engine rewrites or syncs records upgrades a binding or changes P resources checkout edits or retained W state
    And no edited workload outcome or synthetic sensitive marker appears in recovery output or new metadata

    Examples:
      | authority        | format |
      | running worker   | JSON   |
      | completed absent | plain  |

  # Do not require an unsafe P invocation to succeed during setup. Start W, kill
  # its controller, and recover W using its original safely enrolled root in
  # When. Only then inspect P's still-untracked legacy T. The first Then fails
  # today because recovery trusts P after W exits; merely checking currently live
  # workers would also miss the unproven origin of those records.
  @wip @slice-23 @ID-RECSTATE-07 @linux @local-engine-fixture
  Scenario: Cleaning up an exposing worker does not authenticate legacy recovery records
    Given a recovery-separation fixture with untracked P recovery state below W and safely enrolled W ownership
    When W starts loses its controller and is durably stopped from its original state before fresh P status and stop inspect the legacy tree
    Then both P commands report recovery.temporary with exit 75 and no trusted entries because the legacy recovery location remains unbound
    And independent observations show W recovery removed only W full ID and retained its completed records approvals snapshots and location binding
    And P recovery records worker and edits remain unchanged without enrollment copying reapproval repair or automatic cleanup
    When fresh W status inspects its original state
    Then W status reports completed cleanup with its original workload knowledge and writes nothing while all persistent control-state locations remain protected
