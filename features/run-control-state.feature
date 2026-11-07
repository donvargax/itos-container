@phase-1
Feature: Run keeps every tracked control-state tree outside worker mounts
  Approval for one checkout does not permit mounting another project's control
  records. Persistent state stays protected after its workers and controllers
  exit, and publication cannot race an exposing worker into existence.

  # Authorities: docs/charter.md, container-pilot.md AC-02/06/08/23/28,
  # docs/architecture.md (Slices), arc42 Building-block view and Foreground run,
  # and slice-21's control-state.feature with slice-16's checkout-overlap.feature.
  # Draft baselines:
  # main     88da3e19a01a26fa7171bd2bae2950d9cedb492c
  # slice-16 be7ac98de886557b14332e487c08b5c5d8f0d1d1
  # slice-21 d78be4b6fb4bb75cd1c7007d3e8df56e982e1040 (spec only)
  # Also read slice-16's docs/decisions/0014-checkout-admission.md. Its disk
  # Registry.checkAdmission compares worker checkout roots in one selected state
  # tree; it does not exclude another tree's retained control records. Main has
  # no shared admission yet. Slice-21 supplies cross-root location tracking and
  # atomic publication/registration exclusion; this item consumes those locations
  # when choosing a mount. Implement against the completed dependencies, not by
  # copying their unfinished code or substituting a second coordination scheme.
  # Expected failures below are from inspection, NOT executed red evidence.
  #
  # Admission contract:
  # Use slice-21's effective-UID catalog, independently of the invocation's
  # XDG_STATE_HOME, HOME, project hash or selected local daemon. Reuse its fixed
  # account-home descriptor lock, bootstrap/self-binding, pending claims, pinned
  # identities, durability and legacy-evidence rules unchanged. The entire
  # managed itos-container tree is protected, including approvals, snapshots,
  # intents, public recovery records, admission records and coordination. The
  # fixed bootstrap footprint remains protected before its catalog exists.
  # No missing/empty catalog, changed environment or completed worker is permission
  # to forget a location. Discovery still covers enrolled and explicitly selected
  # roots, not arbitrary unseen pre-upgrade roots or mixed old/new controllers.
  #
  # Compare the validated canonical checkout with EVERY protected tree by whole
  # path components. Equality, a checkout containing that tree at any depth, and
  # a checkout rooted inside that tree all intersect protected control state and
  # refuse. The last case prevents mounting a snapshots/project directory while
  # its managed-tree root is outside the mount. Reserve the whole managed tree;
  # do not authorize a subtree by enumerating whichever records happen to exist.
  # A textual prefix or equal basename alone is not an intersection. Existing
  # own-state/anchor exclusion, symlink, inode and hard-link checks still apply.
  # Safe same-checkout concurrency and disjoint projects keep slice-16's behavior.
  #
  # Check retained locations under the SAME cross-process exclusion that guards
  # location publication and worker admission, before granting public registration
  # or create authority. Keep exclusion through the durable reservation and
  # registration handoff. Recheck after preflight: a successful earlier inspection
  # or frozen snapshot cannot authorize a mount over a newly enrolled location.
  # A publisher holding exclusion completes before the waiting run decides; after
  # publication, the run refuses instead of waiting for persistent state to expire.
  # If worker admission wins first, slice-21 refuses the exposing publication.
  # Never hold common exclusion across consent, engine I/O, workload lifetime or
  # acquisition of a session operation lock. Preserve the existing startup budget.
  #
  # Retained locations and pending location claims are NOT worker reservations.
  # Worker exit, controller death, absent full IDs, successful stop and durable
  # reservation completion cannot release them. Slice-16's exception for a pending
  # worker reservation with no public record must not discard a pending LOCATION
  # claim: publication may have left persistent state or may safely resume later.
  # No automatic deletion, pruning, relocation, reapproval, repair or new location-
  # removal command is authorized. Never contact another project's daemon to
  # decide whether its control state still matters. Status/stop trust of exposed
  # legacy records belongs to slice-23; mount-to-inode handoff belongs to slice-17.
  #
  # A known intersection at run admission returns exit 1 with the stable rule
  # run.control-state-exposed in prefixed stderr and empty stdout. Explain that
  # the checkout exposes protected control state and direct the person to use a
  # disjoint checkout. Stopping a worker or reapproving alone does not fix it.
  # Do not suggest force, automatic cleanup or an invented relocation command.
  # Report only a safely escaped path/field subject; no raw record, workload argv,
  # environment value or engine response body. No contender public registration,
  # resource, workload or engine mutation is permitted. Ordinary prerequisite
  # reads and valid own-location enrollment, immutable snapshot publication and
  # a non-authorizing pre-create intent may precede refusal; retain them safely.
  # Preserve all other projects' records and locations, including their identities.
  # Existing usage, readiness, approval and own-state/anchor failures keep their
  # precedence and classifications. An unsafe/unreadable catalog is not a known
  # safe tree: preserve slice-21's classified refusal, never assume no exposure.
  # All new exposure fixtures keep these prerequisites valid, so another refusal
  # (including run.checkout-overlap) cannot substitute for the new rule.
  #
  # Keep run a thin vertical slice. Shared comparison, transactions and typed
  # outcomes belong in engine domain/port APIs and named adapters, using slice-21's
  # factoring to avoid cycles. Only cmd assembles concrete adapters. Extend the
  # same I/O-free fake/disk contracts for retained and pending locations, component
  # comparisons and cross-process interleavings. Reuse Go 1.27.3, x/sys v0.48.0
  # descriptor primitives and Moby client v0.6.2/api v1.56.1 already selected by
  # the dependencies; no new library, service, helper CLI or allowlist is needed.
  # Record the affected arc42 sections and decision with implementation, not here.
  #
  # Fixture handoff: ALL step sentences in this file are NEW. Reuse the independent
  # approval oracle/seeding in approval_fixture_test.go, command observations in
  # approval_steps_test.go, slice-16's overlap_steps_test.go/overlap_fixture_test.go,
  # and public_recovery_probe_test.go's test-only command assembly/barriers.
  # Reuse slice-21's eventual catalog fixture and account-home substitution. Do
  # not copy overlapSetup's single state root: each scenario needs distinct roots.
  #
  # P owns the state to protect; W is the prospective exposing checkout. Their
  # ordinary Git checkouts are disjoint (neither contains the other). Their own
  # policy, manifest, image, UID and engine inputs are valid. W's selected managed
  # tree and the account-home coordination footprint are outside EVERY checkout.
  # P's selected tree T is outside P but intersects W as the row says. Ordinary
  # files have independent inodes; API sockets are outside all checkouts. No
  # existing own-state, nested-checkout, hard-link or socket refusal masks exposure.
  # For the inside-snapshots row, W is T/snapshots/<P-project>, containing P's
  # valid generation file; its .git and request files are ordinary fixture data.
  # Create that checkout before calculating its approval, without moving records.
  #
  # Givens only create disposable inputs, independently seeded approvals/bindings
  # when specified, workload plans, faults and barriers. They never run commands
  # or assert product outcomes. Histories and consent occur in When. Initial W
  # approval is independently seeded in safe state; P starts unapproved except
  # the concurrent cases, which seed its valid approval and durable location too.
  # Use the dependency's real binding schema and independent fixture validation,
  # not a fake verdict or the production conflict predicate as the oracle.
  #
  # ALL children use one temporary account home supplied through a test-only cmd
  # assembly lookup port, with real protected disk/SDK adapters otherwise. Child
  # HOME values differ from that home and from each other; state roots, catalog,
  # sockets and output remain under the fixture's private parent. HOME alone no
  # longer isolates the fixed anchor. Never resolve/write the person's actual
  # control state or invoke real-state approve. Test-only barriers change timing
  # or I/O observations, never authority or results; add no production test flag.
  # Production account-home wiring retains slice-21's separate read-only checks.
  #
  # Non-engine rows use recording Unix HTTP Engine API fixtures on Linux amd64;
  # distinct-daemon rows use different endpoints/IDs with the SAME effective UID
  # and temporary account home. All competitors are independent processes. Every
  # When records outputs, exits, request journals and bounded barrier outcomes,
  # including an unexpected launch/refusal or missing barrier, for Then to judge.
  # Synchronize on actual lock/publication/registration events, never sleeps.
  # A failed setup, undefined step, timeout or failed prerequisite is not red proof.
  # For main without the dependencies, missing tracking events are not evidence:
  # the assertion-level red baseline must first contain slice-16 and slice-21.
  #
  # Capture inventories before each contender: bytes, record inode/mtime, owner,
  # mode, link count/target and directory identity. Independently decode location,
  # snapshot, intent, admission and public bindings and observe successful file/
  # parent fsync where durability is required. Mere file visibility is not proof.
  # Allow only each command's exact legitimate own publication/progress changes;
  # never whitelist a whole coordination tree. Attribute engine requests by
  # invocation and full resource ID, and preserve unrelated resources and edits.
  # Positive controls must really start with their own full IDs and activity.
  # Textual-prefix control roots are siblings T-more of T; equal-basename roots
  # are elsewhere outside T, P, W and the fixed coordination footprint. Their
  # own approved state is disjoint. They must not be rejected merely because
  # another registered location or an unrelated active worker exists.
  #
  # Native cases run ONLY in README's guarded CI engine tier with the independently
  # inspected cached image. Use synthetic data, trusted in-worker probes and the
  # same temporary account-home assembly, actual Docker adapters and owned cleanup.
  # Observe full IDs, actual checkout mounts and in-worker results independently
  # of request journals. Probes may read/open synthetic records for write access,
  # but need not overwrite them and obstruct cleanup. Never run a worker helper
  # on the host. Harness cleanup is identity-checked and cannot satisfy a product
  # absence/completion assertion. No local engine, credentials or host helper runs.
  #
  # Implementer commits complete steps red first with @wip retained. Obtain local
  # assertion-level red for EXPOSURE-01..06 with @wip removed for the run; obtain
  # EXPOSURE-07/08 red only through the coordinator's CI selector. Then implement,
  # remove @wip and require green in the same respective tiers. Controls following
  # a new refusal may already pass; they are not independent red requirements.

  # Both operation orders end at the NEW mount refusal. In the worker-first row,
  # When starts W before P has a location claim, attempts P approval (slice-21
  # must refuse it), completes W with product stop, then approves P and attempts
  # a fresh W run. Observe original full-ID absence and durable completion before
  # approval. Do not mistake that earlier, already-specified approval refusal for
  # this item's red. Without location consumption the FINAL W run still launches.
  @wip @slice-22 @ID-EXPOSURE-01 @linux @local-engine-fixture
  Scenario Outline: Either publication order prevents a later mount over approved state
    Given an exposure fixture with unapproved P state beneath W and "<engines>"
    When the product follows "<order>" before attempting a fresh W run
    Then the final W run reports run.control-state-exposed with exit 1 and disjoint-checkout guidance before public registration or engine mutation
    And P approval and its durable location binding exist unchanged after the refused run
    And the recorded order shows P published only after every exposing W reservation was durably completed

    Examples:
      | engines          | order                                                   |
      | shared daemon    | approve P first                                         |
      | distinct daemons | run W then refuse P approval then stop W before approval |

  # T protects records, not merely live P workers. Histories are product actions;
  # no Given forges completion. All P checkouts are disjoint from W, including
  # the inside-snapshots row, so checkout-overlap cannot explain any refusal.
  # Today W launches even after another project leaves these records in its mount.
  @wip @slice-22 @ID-EXPOSURE-02 @linux @local-engine-fixture
  Scenario Outline: Persistent control records exclude exposing mounts after controller exit
    Given an exposure history fixture with W "<placement>" relative to P managed tree and "<engines>"
    When P is approved and reaches "<history>" before its controller exits and a fresh W run is attempted
    Then W reports run.control-state-exposed with exit 1 before public registration or engine mutation despite the ended P controller
    And the observed P history and retained approvals snapshots intents recovery and admission records keep their original bindings
    And the W refusal neither releases P location tracking nor contacts P engine for cleanup or discovery
    When fresh runs start the disjoint textual-prefix and equal-basename controls
    Then both control workloads are independently active while P records and all protected locations remain intact

    Examples:
      | placement                       | engines          | history                                    |
      | contains T several levels below | shared daemon    | successful foreground exit and cleanup     |
      | equals T                        | distinct daemons | successful foreground exit and cleanup     |
      | contains T several levels below | distinct daemons | controller killed with worker still running |
      | contains T several levels below | shared daemon    | workload exit with removal refused         |
      | contains T several levels below | distinct daemons | controller loss followed by successful stop |
      | is inside T at P snapshot folder | distinct daemons | successful foreground exit and cleanup     |

  # Pause P only after a durable location claim, at the specified boundary, then
  # kill that controller. The before-create row leaves T absent and the claim
  # pending; the other leaves a bound tree but no approval. Slice-21 supplies these
  # publication seams. The common descriptor lock is released by process death.
  # Do not erase surviving evidence. Without this item's check W mounts either
  # location, treating absence of a worker/approval as absence of protected state.
  @wip @slice-22 @ID-EXPOSURE-03 @linux @local-engine-fixture
  Scenario Outline: Interrupted location publication still excludes an exposing mount
    Given an exposure fixture with P publication paused "<boundary>" after durable location protection
    When P starts exact-fingerprint approval and its paused controller is killed before a fresh W run
    Then W reports run.control-state-exposed with exit 1 before public registration or engine mutation using the retained location claim
    And the observed boundary has no P approval or worker and its pending or completed location evidence survives W unchanged

    Examples:
      | boundary                                              |
      | before the first selected managed-tree directory       |
      | after managed-tree binding before approval publication |

  # Complete W preflight, including snapshot publication, outside ALL admission
  # locks; hold just before admission. P then enrolls T and publishes approval.
  # A cached preflight decision cannot see this new protected location. Today's
  # resumed W creates a worker; the first Then is the expected failing assertion.
  @wip @slice-22 @ID-EXPOSURE-04 @linux @local-engine-fixture
  Scenario Outline: Run rechecks locations published after its successful preflight
    Given an exposure fixture with a W barrier after full preflight before admission and "<engines>"
    When W pauses and P durably approves its previously untracked state beneath W before W resumes
    Then resumed W reports run.control-state-exposed with exit 1 before public registration or engine mutation
    And observations show W finished preflight before P publication and P complete approval and location binding remain unchanged by W

    Examples:
      | engines          |
      | shared daemon    |
      | distinct daemons |

  # Unlike slice-21 CONTROL-06, do NOT cancel the waiting run. W finishes preflight
  # before P holds the common transaction. Hold P after its location claim and
  # before its first approval mutation; observe W's real lock attempt, then let P
  # finish. Record premature W registration/create instead of hanging. Slice-21
  # provides exclusion; without this item W is admitted when that lock is released.
  @wip @slice-22 @ID-EXPOSURE-05 @linux @local-engine-fixture
  Scenario: A run waiting for publication refuses the newly protected mount
    Given an exposure fixture with independent W admission and P publication barriers across state roots
    When W finishes preflight then attempts admission while P holds publication and P is allowed to finish
    Then the waiting W reports run.control-state-exposed with exit 1 after P publication without public registration or engine mutation
    And actual lock and write observations show no W admission during publication and one complete durable P approval after release
    And neither invocation loses location evidence or changes unrelated approvals checkouts or resources

  # Seed both matching approvals and location bindings independently; T is under
  # W, while W's selected state is outside P and W. No worker exists yet. Hold
  # both processes at command entry, then release together. The winner is
  # NOT arbitrary: retained P state already excludes W, so P must remain runnable.
  # Observe creates/refusals before releasing bounded create replies; two creates
  # are assertion data, never a deadlock. Main admits both; per-root admission
  # also misses the conflict. Waiting for a live P worker is insufficient. An
  # early W exposure refusal is valid; do not require W to finish preflight or
  # reach an admission barrier after it has already refused the known location.
  @wip @slice-22 @ID-EXPOSURE-06 @linux @local-engine-fixture
  Scenario Outline: Concurrent disjoint checkouts admit only the one that cannot expose control state
    Given a concurrent exposure fixture with both approvals tracked and "<engines>"
    When independent P and W runs leave their command-entry rendezvous together
    Then P alone creates and starts an owned worker while W reports run.control-state-exposed with exit 1 before public registration or engine mutation
    And P activity and full-ID ownership are observed while both approvals and location bindings remain unchanged
    When product stop completes P and a fresh process attempts W using a different child HOME with its original selected state
    Then W still reports run.control-state-exposed with exit 1 and no worker while completed P records and both location bindings persist

    Examples:
      | engines          |
      | shared daemon    |
      | distinct daemons |

  # CI counterpart to retained-record refusal. P executes a trusted marker and
  # exits, leaving actual snapshots/intents/completed recovery and admission state.
  # W's attempted argv is a trusted access probe with its own workload marker.
  # The safe disjoint control reads/writes a checkout marker, then tries read and
  # write opens of the independently observed host approval/snapshot/intent/public
  # record and coordination binding paths, directly and via checkout symlinks.
  # Require denial for every protected target and success for the checkout marker.
  # Actual mounts must contain only that control checkout, with no state/anchor
  # source. Missing native resources, an unexecuted probe or API journals alone
  # cannot prove isolation. Today the first Then fails because W is created.
  @wip @slice-22 @ID-EXPOSURE-07 @engine @linux
  Scenario Outline: Native run refuses retained control state while a safe worker remains isolated
    Given a CI exposure fixture with the cached image and W "<placement>" relative to P managed tree
    When P is approved and completes its native workload and cleanup before W attempts the control-state access probe
    Then native W reports run.control-state-exposed with exit 1 and has no created worker workload output or marker
    And original P full-ID absence durable completion and unchanged retained control records are independently observed
    When a disjoint approved native control runs the checkout and protected-state access probes
    Then the native control reads and writes its checkout but cannot read or open any protected record for writing
    And independent inspection observes only the control checkout mounted and all protected records and unrelated resources remain unchanged

    Examples:
      | placement                       |
      | contains T several levels below |
      | is inside T at P snapshot folder |

  # CI counterpart to concurrent admission. Both approvals/locations preexist;
  # synchronize command entry through test-only assembly barriers, allowing early
  # exposure refusal without forged daemon observations. Heartbeat helpers run only in
  # native workers. Inspect actual IDs, mounts and advancing P activity before
  # harness cleanup. Today's first Then fails when W also obtains a worker.
  @wip @slice-22 @ID-EXPOSURE-08 @engine @linux
  Scenario: Concurrent native runs cannot mount another project's protected state
    Given a CI concurrent exposure fixture with tracked approvals the cached image and independent heartbeat workloads
    When native P and W run processes leave their command-entry rendezvous together
    Then only native P obtains an owned worker and advancing heartbeat while W reports run.control-state-exposed with exit 1 and no created worker or marker
    And independent native inspection finds only P checkout mounted for these invocations and all control-state bindings remain unchanged
    When product stop durably completes native P and a fresh native W run is attempted
    Then native W is still refused with run.control-state-exposed and no created worker while P retained control records survive cleanup
