@phase-1
Feature: Run refuses overlapping worker checkouts
  A worker with a parent checkout could replace another worker's checkout path.
  Admit at most one side of a strict ancestor/descendant pair until its owned
  resources are cleaned up, while independent projects can continue running.

  # Authorities: docs/charter.md, docs/architecture.md (Slices), arc42 Foreground
  # run and Recovery, offline-run-lifecycle.md (AC-05/06/23/28), and slice-16's
  # settled requirement for atomic refusal in either order, including concurrency.
  # This file is the slice-16 spec only. Every step phrase below is NEW; reuse
  # fixture helpers, not existing steps whose single-project assertions differ.
  # Implementers first commit complete steps with these tags still @wip. Obtain
  # assertion-level red before implementation and green afterward. Native red
  # and green evidence is CI-only under features/README.md, never local Docker.
  #
  # Admission choices:
  # - Compare canonical absolute checkout roots from the validated frozen binding,
  #   by whole path components. A distinct proper ancestor or descendant conflicts
  #   at any depth, in either launch order. A string prefix or equal basename is
  #   not ancestry. Existing symlink/layout and fresh identity checks still apply.
  # - Equal canonical roots retain the existing same-checkout concurrency contract
  #   (RECOVERY-05/18). This increment prevents distinct nested checkouts; it does
  #   not introduce an exclusive-writer rule for one checkout. Every same-root
  #   session must finish cleanup before a nested root becomes admissible.
  # - Coordinate all projects of the effective host user in the same protected
  #   control-state root, including different selected local daemon identities.
  #   Neither an approval/project hash nor the daemon partitions filesystem
  #   ancestry. No cross-user or separately configured state-root guarantee is
  #   added here. Control state and fixture sockets stay outside every checkout.
  # - Checking conflicts and reserving the selected root must be one atomic
  #   cross-process admission, durable before any create request. A preparing
  #   session already excludes overlapping roots; looking only for running
  #   containers, checking then publishing without exclusion, or holding only
  #   an in-process mutex cannot satisfy this rule. Do not contact other projects'
  #   daemons or list/prune/adopt their resources to decide admission.
  # - The reservation lasts through startup, execution and incomplete cleanup.
  #   Controller death, lease release, workload exit, an unavailable observation
  #   or a successful DELETE response alone cannot release it. Retain uncertainty
  #   after a lost create reply. For a created resource, release only after fresh
  #   full-ID absence and durable intent/session completion. Release and new
  #   admission serialize so a stale controller cannot launch after release.
  #   A failure before any create may release only when no create was sent and
  #   no continuation can send one; do not infer that from a missing resource.
  #   Preserve unresolved legacy ownership; absence of new admission metadata
  #   cannot establish that existing registered sessions have no conflict.
  # - Shared admission/ownership belongs in internal/engine/session domain/port
  #   and adapters, reusing approval's protected storage and readiness identities.
  #   Run/status/stop never import each other. Only cmd assembles adapters. Keep
  #   domain fakes I/O-free and run shared contracts against fakes and adapters.
  #   Reuse Go 1.27.3 and golang.org/x/sys v0.48.0 descriptor-lock/filesystem
  #   primitives and Moby client v0.6.2/api v1.56.1, already pinned in go.mod.
  #   These supply locking and engine access; add no dependency, helper CLI or
  #   domain allowlist entry for admission.
  #   Short admission transactions must not span engine calls or foreground wait,
  #   and must not obstruct unrelated recovery. Preserve the existing budgets.
  #   Apply the existing pinned/no-follow, owner/mode/link and durable-publication
  #   rules to admission state. Unsafe or unreadable state refuses, never repairs
  #   or assumes no conflict. Status/stop keep their existing project scope.
  #   Record any required arc42 change and its decision in the implementation PR.
  # - A known overlap returns exit 1 with rule run.checkout-overlap in prefixed
  #   stderr and empty stdout. Explain the overlap and direct the person to finish
  #   the other session or inspect/stop it from its original checkout. Do not
  #   suggest reapproval, force, a wait-for-exit queue or automatic stop of another
  #   worker. The refused invocation sends no create/start or other engine mutation;
  #   its ordinary preflight reads and immutable snapshot publication may occur.
  #   Preserve the blocker, its records and approvals. No raw argv, environment or
  #   engine response body belongs in the diagnostic. Existing invalid invocation,
  #   prerequisite and approval refusals retain their precedence and classification.
  #
  # Fixture contract for ALL Givens below:
  # Create disposable ordinary checkouts with real .git directories, policy and
  # manifests, independently seeded matching approvals, and one shared protected
  # host-state root. Never invoke approve or execute checkout helpers on the host.
  # Use ancestor <scratch>/projects/work and descendant work/<relative child>.
  # Disjoint controls are projects/work-more (textual prefix) and other/work
  # (equal basename). Each fixture records initial identities, bytes and modes.
  # Given only writes fixtures/configures faults, workloads and barriers; no run,
  # status, stop or product-outcome assertion occurs until When/Then.
  #
  # Local cases use the built CLI with production filesystem/SDK adapters against
  # recording Unix HTTP Engine API fixtures. Distinct-daemon rows use two safe
  # endpoints/IDs but the SAME host user and state root. Use separate processes
  # for competing runs, not goroutines sharing an in-memory registry. Test-only
  # cmd assembly may supply lifecycle/storage barriers; no product testing flag.
  # Supplement with the same admission contract on in-memory port fakes, including
  # forced check/publication interleavings and failed durable reservation writes.
  # Inspect actual cross-process exclusion and durable writes, not a fake counter.
  # Registration/intent fixtures and syscall observers in recovery_*_steps_test.go
  # and recovery_native_fixture_test.go supply reusable machinery. Do not copy
  # recoveryPairFixture's separate state homes: that would miss this conflict.
  #
  # Actions record refusals, unexpected launches/exits and bounded barrier timeouts
  # as data for Then, never fail merely because the expected overlap guard is
  # absent. Journals identify requests by invocation/token/full worker ID. A
  # refusal requires the stated rule plus no candidate resource/workload, not
  # merely a nonzero exit or missing marker. Positive controls must really start
  # through the same run path and remain observable while a nested run refuses.
  # All waits are bounded; synchronize by observed barriers, never sleep-based
  # ordering. Harness cleanup is identity-checked and cannot satisfy product cleanup.
  #
  # Native cases use only README's guarded Linux amd64 CI tier and independently
  # inspected cached fixture image. Launch the product with disposable seeded
  # approval. Trusted heartbeat helpers run only inside workers. Independently
  # inspect full IDs, mounts and advancing heartbeats; a journal alone is not
  # native evidence. Refused workloads have distinct marker names. Preserve both
  # roots and .git identities; no helper needs to replace a real checkout path.
  # Inode-bound mount handoff remains slice-17, not a claim of these admission tests.

  # Sequential admission, with positive controls for both common bad ancestry
  # tests. The different-daemon rows prevent a daemon-scoped admission shortcut.
  @wip @slice-16 @ID-OVERLAP-01 @linux
  Scenario Outline: A running checkout excludes its ancestors and descendants
    Given an overlap fixture with descendant "<child>" and "<engines>" local engine fixtures
    When run starts "<first>" and a fresh run attempts the other nested checkout while the first workload remains active
    Then the nested contender reports run.checkout-overlap with exit 1 and original-checkout recovery guidance before any resource creation
    And the first workload remains active with its original full ID and unchanged ownership
    When runs start the textual-prefix and equal-basename disjoint controls while the first workload remains active
    Then both disjoint workloads start and remain independently observable without changing either nested checkout or its approval

    Examples:
      | first      | child         | engines          |
      | ancestor   | child         | shared daemon    |
      | descendant | child         | shared daemon    |
      | ancestor   | middle/child  | distinct daemons |
      | descendant | middle/child  | distinct daemons |

  # Native counterpart beside its local rule. A fresh successful run after
  # product cleanup proves the refusal is tied to an outstanding session.
  @wip @slice-16 @ID-OVERLAP-06 @engine @linux
  Scenario Outline: Native nested refusal preserves the running worker and releases after cleanup
    Given a CI overlap fixture with nested checkouts the cached image and distinct heartbeat workloads
    When the product runs "<first>" to its first heartbeat and attempts the other nested checkout
    Then the native nested contender reports run.checkout-overlap with exit 1 and creates no worker or workload marker
    And the original native worker keeps the same full ID mount and advancing heartbeat through the refusal
    When product run starts a disjoint heartbeat control and product stop completes the original nested session before a fresh contender run
    Then the formerly refused checkout starts its own native heartbeat only after confirmed original full-ID absence and durable cleanup
    And the disjoint native heartbeat continues while checkout edits approvals snapshots and unrelated resources survive

    Examples:
      | first      |
      | ancestor   |
      | descendant |

  # Existing registration/intent publication supplies a pre-create observation
  # point even on the red baseline. Pause OUTSIDE its project/session operation
  # locks after both publications and before create. The fixture exposes no
  # worker for the first run, so an engine-running-only scan would admit the second.
  @wip @slice-16 @ID-OVERLAP-02 @linux
  Scenario Outline: Preparing a checkout already excludes an overlapping run
    Given an overlap fixture with a pre-create barrier for "<first>" after durable registration and intent publication
    When the first run pauses at the pre-create barrier and a fresh run attempts the other nested checkout
    Then the nested contender reports run.checkout-overlap with exit 1 and original-checkout recovery guidance before any resource creation
    And neither nested invocation has created or started a worker while the first barrier is held
    When a disjoint run starts before the first barrier is released and the first run then resumes
    Then the disjoint workload starts without waiting for the barrier and the first nested run starts exactly one owned worker after release

    Examples:
      | first      |
      | ancestor   |
      | descendant |

  # Rendezvous both processes after successful full preflight, before lifecycle
  # admission. Release them together; hold each create reply until the other
  # invocation has either refused or also reached create. Capture both-create as
  # a failed assertion, not a deadlock. Release replies within the existing call
  # budget; keep admitted workloads alive through the admission observations.
  # No particular winner is required. Local contracts additionally force the
  # check/publication race with storage barriers under real descriptor locks.
  @wip @slice-16 @ID-OVERLAP-03 @linux
  Scenario Outline: Concurrent nested launches admit exactly one checkout
    Given an overlap fixture with "<engines>" local engine fixtures and a shared preflight rendezvous
    When independent ancestor and descendant run processes leave the rendezvous together and their create replies are observed before release
    Then exactly one nested invocation reaches create and the other reports run.checkout-overlap with exit 1 before any engine mutation
    And after reply release exactly one owned nested worker starts and the losing invocation has no workload output or marker
    When a disjoint run starts while the winning nested workload remains active
    Then both admitted workloads remain independently active and the losing invocation has not changed their ownership or approvals

    Examples:
      | engines          |
      | shared daemon    |
      | distinct daemons |

  # Native concurrency holds the final preflight inspection replies until both
  # product processes arrive, then releases them together through the trusted
  # API fixture. No daemon flag or controller testing switch. Count actual created
  # resources and advancing heartbeats before any product or harness cleanup.
  @wip @slice-16 @ID-OVERLAP-07 @engine @linux
  Scenario: Concurrent native nested launches leave exactly one running worker
    Given a CI overlap fixture with nested checkouts the cached image and a shared preflight rendezvous
    When independent native ancestor and descendant run processes leave the rendezvous together
    Then exactly one native nested run produces an owned worker and heartbeat while the other reports run.checkout-overlap with exit 1 and no created resource
    And independent native inspection finds a checkout mount only for the winning nested invocation and the losing workload marker is absent
    When product stop completes the winning session and a fresh run targets the losing checkout
    Then the losing checkout starts a new owned native worker after durable winner cleanup without changing approvals or unrelated resources

  # Keep readiness/image inspection usable for the prospective run. Resource
  # observation faults affect the original worker only, so preflight failure
  # cannot masquerade as an overlap refusal. Each fault is installed before When.
  # The lost-create fixture initially hides the exact random name, then exposes
  # the matching unstarted worker on fault clearance; missing name is not absence.
  # The successful-DELETE fixture returns HTTP 204 but keeps the original exited
  # full ID observable. Clearing that fault only permits a later product removal;
  # the harness does not remove it or publish completion on the product's behalf.
  # The completion fault follows actual removal but prevents durable completion.
  # Stop must re-establish durability before a later admission succeeds.
  @wip @slice-16 @ID-OVERLAP-04 @linux
  Scenario Outline: Controller loss and incomplete cleanup retain the checkout conflict
    Given an overlap fixture with an ancestor run configured to leave "<condition>"
    When run reaches that condition and a fresh descendant run is attempted before product recovery
    Then the nested contender reports run.checkout-overlap with exit 1 and original-checkout recovery guidance before any resource creation
    And the original registration intent snapshot and unresolved ownership remain available without contender-triggered cleanup or adoption
    When the fixture clears the fault and a fresh product stop completes the ancestor session before another descendant run
    Then the descendant run starts only after confirmed original full-ID absence and durable cleanup while original edits approvals and snapshots remain

    Examples:
      | condition                                                      |
      | controller absent with worker still running                    |
      | workload exited with removal refused                           |
      | workload exited with successful DELETE but worker still present |
      | controller absent with resource observation unavailable         |
      | resource removed with session completion fsync failed           |
      | lost create reply with its worker initially invisible           |

  # Release must fence a surviving controller before admitting an overlapping
  # root. Use the existing "created before start" test-assembly barrier after
  # durable intent ownership and public ready publication, outside operation,
  # project and admission locks; the original controller lease remains held.
  # Given configures that barrier without running a command. When observes the
  # unstarted full ID before attempting the contender. The first refusal is the
  # expected red assertion: today's separate checkout indexes admit the contender.
  #
  # Keep the original controller paused through product stop AND the successful
  # replacement run. Neither controller exit nor lease release is a prerequisite
  # for readmission after confirmed durable cleanup. Resume it only after the new
  # worker is independently observable. Attribute every later request to its
  # invocation so the replacement's legitimate create cannot hide a stale launch.
  # Fresh original-checkout status must retain a null workload exit: an unstarted
  # worker has no result to preserve or invent. This is local lifecycle evidence;
  # native admission remains OVERLAP-06/07 and mount identity remains slice-17.
  @wip @slice-16 @ID-OVERLAP-09 @linux
  Scenario Outline: Cleanup admits the other checkout before a stopped controller resumes
    Given an overlap fixture with a post-create pre-start barrier for "<first>" outside its session operation lock
    When the first run pauses after durable ownership and a fresh run attempts the other nested checkout
    Then the nested contender reports run.checkout-overlap with exit 1 and original-checkout recovery guidance before any resource creation
    And the paused run has exactly one durably owned unstarted worker and no workload output
    When product stop completes the paused session and a fresh run targets the other nested checkout before the original controller is released
    Then stop confirms original full-ID absence and durable cleanup and the fresh contender starts while the original controller remains paused
    When the original controller resumes while the newly admitted nested worker remains active
    Then the resumed controller exits nonzero with empty workload output and no create or start request after completed cleanup
    And fresh original-checkout status reports stopped absent complete and null workload exit while the newly admitted worker keeps its full ID ownership and activity
    And both nested checkouts approvals and snapshots remain unchanged by the stale continuation

    Examples:
      | first      |
      | ancestor   |
      | descendant |

  # Seed the EXISTING seven-field public record, original intent/snapshot and
  # matching running resource directly, with an absent controller. Do not seed
  # any new admission-specific record. These are valid pre-upgrade sessions,
  # not unknown schemas to accept or records to repair/adopt. The Given writes
  # data only; run's refusal and stop's cleanup remain product observations.
  @wip @slice-16 @ID-OVERLAP-08 @linux
  Scenario Outline: Existing registered workers also exclude nested checkouts
    Given an overlap fixture with an existing registered "<first>" worker and no admission-specific metadata
    When a fresh run attempts the other nested checkout before recovery of the existing worker
    Then the nested contender reports run.checkout-overlap with exit 1 and original-checkout recovery guidance before any resource creation
    And the existing worker and its original protected ownership remain unchanged without migration or adoption
    When product stop completes the existing session and a fresh contender run is attempted
    Then the contender starts after confirmed original full-ID absence and durable cleanup while the original completed records remain discoverable

    Examples:
      | first      |
      | ancestor   |
      | descendant |

  # Same-root concurrency is intentionally retained. Releasing A's first
  # session cannot free the checkout still occupied by A's second session;
  # completed tombstones must not block forever after both are cleaned up.
  @wip @slice-16 @ID-OVERLAP-05 @linux
  Scenario: A nested checkout stays refused until every session of the overlapping root is cleaned up
    Given an overlap fixture with two independently approved invocations of the ancestor checkout
    When both ancestor runs start and the descendant run is attempted before and after product stop completes only the first ancestor session
    Then both descendant attempts report run.checkout-overlap with exit 1 before resource creation while the second ancestor worker stays active
    And the first ancestor session alone has confirmed durable cleanup and the second retains its own original ownership
    When product stop completes the second ancestor session and a fresh descendant run is attempted
    Then the descendant run starts despite retained completed ancestor records and preserves both ancestors' workload outcomes edits approvals and snapshots
