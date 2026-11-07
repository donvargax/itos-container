@phase-1
Feature: Doctor and approve keep control state outside another worker's checkout
  A private directory is still worker-writable when it is inside that worker's
  checkout. Inspection must refuse that location, and consent must establish
  separation atomically before publishing approval there.

  # Authorities: docs/charter.md, container-pilot.md AC-02/04/06/08/23/28,
  # docs/architecture.md (Slices), arc42 Building-block view and Recovery,
  # approval.feature, and slice-21's dependency on slice-16.
  # Draft baseline: main 88da3e19a01a26fa7171bd2bae2950d9cedb492c and
  # slice-16 21b35193274064b26f0277d0f73c2eb42a0fa992. Read that branch's
  # checkout-overlap.feature and docs/decisions/0014-checkout-admission.md before
  # implementing. Its admission lock is per selected state root; approval's
  # Store.Read/Commit currently check only the candidate checkout, never another
  # project's reservation. Its preparing, pending and completion rules still hold.
  # Coordinator q-31 accepts the executed red evidence recorded at 4d64985:
  # all 45 examples failed at behavior assertions against local recording engines.
  # No @engine scenarios or CI red run are required for this item.
  #
  # Scope and shared contract:
  # Extend shared admission to track protected control-state locations across
  # XDG_STATE_HOME selections for the effective host UID. Preserve slice-4's
  # XDG_STATE_HOME / HOME fallback for approval, snapshot and recovery storage.
  # State location remains excluded from the canonical approval fingerprint.
  # A tracked location covers the entire selected itos-container managed tree,
  # including approvals, snapshots, intents, public sessions and coordination.
  # Keep registrations after command/controller exit and after worker cleanup:
  # persistent records still need protection. No automatic relocation, deletion,
  # ownership repair, reapproval or cleanup of another project is authorized.
  #
  # Use one durable coordination anchor per effective UID, independent of project,
  # daemon, XDG_STATE_HOME and process HOME. Select
  # <host-account-home>/.local/state/itos-container/coordination through a host
  # port resolving the effective UID's account home, not USER, LOGNAME or HOME.
  # It is protected control state too. No environment/checkout override, service,
  # privileged helper or worker-visible coordination path is introduced. An
  # unavailable account home refuses with approval.state.unavailable (exit 3).
  # Reuse pinned Go 1.27.3 os/user.LookupId behind the host adapter, the existing
  # protected disk primitives, and golang.org/x/sys v0.48.0 descriptor locks.
  # Reuse Moby client v0.6.2/api v1.56.1 for existing engine inspection only.
  # Add no dependency, custom lock protocol based on PID/time, or domain import.
  #
  # Bootstrap and the shared lock:
  # The account home must already exist. Pin and validate it and its ancestors
  # without following links, using the existing protected-ancestor rules; the
  # home itself must belong to the effective UID and forbid other-user writes.
  # Never flock the home inode: other UIDs may open a 0755 or 0750 home.
  # After the read-only safety checks, a consenting writer bootstraps the fixed
  # chain with mkdir(0700), accepting EEXIST, then opens coordination with
  # O_DIRECTORY|O_NOFOLLOW and validates its descriptor's UID and exact 0700 mode.
  # Create control.lock exclusively inside it, accepting EEXIST only after opening
  # without following links and validating an owned 0600 regular single-link inode.
  # Same-user racing creators converge on that private inode; unsafe entries refuse.
  # Take that file's shared/exclusive flock before catalog or selected-root locks.
  # Every wait has the 10 s call deadline, including approval reads and commits;
  # timeout is retryable approval.state.busy (75), or run.call-timeout (75) under
  # run's existing call budget. No wait uses an unbounded background context.
  # Revalidate home, coordination and lock identities after locking and publication.
  # Read-only inspection never creates a lock: an absent lock permits only safe
  # bootstrap-only or legacy-approval inspection, rechecking absence before return;
  # completed catalog/session evidence without a lock is unavailable, never repaired.
  # No common lock spans consent, engine I/O or acquiring a session operation lock.
  #
  # The fixed bootstrap footprint is the account home's .local/state/itos-container
  # tree, including coordination. Treat it as a prospective protected location
  # even with no catalog; absence is not permission to bypass initialization.
  # Before bootstrap creation, recheck the candidate, selected legacy
  # state, any existing catalog and every discoverable outstanding reservation
  # against both this footprint and the selected location BEFORE any mkdir.
  # After acquiring the private lock, repeat discovery and checks under exclusion.
  # The own-checkout, exposure and missing-evidence refusals still apply. Neither
  # an empty catalog nor a bootstrap retry can erase legacy session evidence.
  #
  # Only after consent (or validated existing approval for matching reuse/run),
  # bootstrap may create missing .local, state, itos-container and coordination
  # directories along that fixed chain, plus the private lock and self-binding publication
  # files inside coordination. This is the sole exception to claim-before-create:
  # it permits no approvals, snapshots, intents, sessions or other selected-root
  # entries yet. It applies even when HOME equals the account home and unset
  # XDG_STATE_HOME selects the bootstrap tree itself. New directories are owned
  # 0700 and records 0600 regardless of umask; existing unsafe entries refuse.
  # Pin/revalidate each created component and fsync it and its parent. Then
  # atomically publish and fsync a catalog self-binding for the WHOLE fixed tree,
  # its actual root identity, pinned ancestors and effective UID, including the
  # coordination directory identity. File fsync, rename and parent fsync must
  # finish before any ordinary selected-tree write or worker registration.
  # When selected and fixed trees coincide this one binding serves both roles;
  # do not require a second claim before creating the catalog that stores it.
  # Keep the same exclusion through subsequent enrollment/publication as below.
  #
  # Failure or controller death during bootstrap grants no approval or worker
  # launch authority. Retain surviving bootstrap entries; never delete/reset the
  # tree or change the private lock inode to retry. Admission cannot pass an initializing
  # publisher; after interruption it must complete this same protocol under the
  # private lock using validated existing approval, or refuse before registration.
  # A fresh consenting approve revalidates surviving safe directories and bindings
  # and redoes every required directory/file/parent fsync before proceeding.
  # A safe directory-only prefix or unpublished private self-binding temp file
  # is incomplete bootstrap, not a completed catalog or a worker reservation.
  # Such temp files grant no authority and are retained as non-authoritative
  # evidence. Malformed final bindings, unsafe entries or session evidence without
  # a catalog still refuse with the classifications below; retry is not repair.
  # A visible complete self-binding after uncertain parent fsync must be resynced,
  # preserving its binding and inode. Doctor may report missing approval for an
  # otherwise safe bootstrap-only tree, but never completes or fsyncs it.
  #
  # The shared catalog binds each absolute managed location to its validated
  # filesystem identity and host UID. For an absent tree, a pending claim binds
  # the intended path and pinned existing ancestors; complete its root identity
  # before reporting publication success. Pending claims remain protective and
  # are never treated as absent. A retry may finish the same validated claim
  # under exclusion, redoing durability without changing its bound location.
  # Read the selected root as well as cataloged roots; no project/daemon filter
  # may hide another outstanding reservation.
  # A missing selected managed tree is a prospective location, not proof of safety.
  # Compare whole canonical path components: a worker root equal to or an ancestor
  # of a managed location exposes it, even before the location exists. Ordinary
  # string prefixes and equal basenames do not. Revalidate pinned ancestors and
  # existing identities through publication; a replaced path cannot redirect a
  # registration or a write. Check the coordination anchor by the same rule.
  # Preserve the own-checkout exclusion for both the selected tree and anchor.
  #
  # Treat all reservations that can still authorize a worker as blockers, from
  # preparing through uncertain cleanup, including controller loss. Reuse
  # slice-16's legacy public-record/intent/snapshot validation and proven
  # pre-create cancellation, rather than interpreting liveness or DELETE as
  # completion. Never contact another project's daemon to prove separation.
  # Selected legacy roots are inspected before enrollment, without rewriting
  # their approval or recovery records. Discovery covers roots enrolled by this
  # protocol and the explicitly selected root; it cannot discover arbitrary old
  # XDG roots never presented to it. Mixed old/new controllers and migration of
  # unseen pre-upgrade roots need a separate upgrade contract before claiming
  # host-wide protection for existing installations. Do not silently claim it here.
  #
  # Doctor is read-only, even with no catalog or selected state directories.
  # It must not create a catalog, location claim, directory or lock file, enroll
  # legacy state, repair anything, or turn missing evidence into matching approval.
  # A missing catalog and empty selected state permit read-only missing approval;
  # a root containing only valid legacy approvals can be inspected read-only and
  # enrolled by approve without rewriting those approvals. In either case,
  # a missing catalog beside existing session/admission evidence is unverifiable,
  # not evidence that no worker exists. A completed catalog entry whose target is
  # missing, replaced, unsafe or unreadable also refuses; do not forget the entry.
  # Policy-only and manifest-only doctor retain their file-only scope and never
  # resolve the account home or read this catalog.
  #
  # Approve checks separation on inspection, including an otherwise matching
  # approval, then again inside publication's shared transaction after consent.
  # A matching, safely separated legacy approval may enroll its location without
  # another prompt or approval rewrite; generation, bytes, inode and mtime stay
  # unchanged. No consent or a declined review creates no coordination state.
  # Do not hold the shared lock while awaiting consent or inspecting an engine.
  #
  # Location enrollment, conflict checking and approval publication serialize
  # with slice-16's worker registration across state roots and processes. Except
  # for the fixed bootstrap entries above, before creating any selected managed
  # entry or approval temp file, durably claim its intended location under that
  # shared exclusion. Keep the claim and exclusion through approval file fsync,
  # rename and parent fsync. A failed or interrupted
  # publication must not expose an untracked approval or report success; retain
  # conservative location evidence on uncertain writes. Reuse existing compare-
  # generation and complete-old-or-new approval publication semantics. Retrying
  # must re-establish durability, never delete evidence to get a fresh attempt.
  # Newly enrolled roots participate in the common worker-admission transaction;
  # a per-root lock or process mutex alone cannot implement this contract.
  #
  # This item supplies the catalog and atomic publication/registration exclusion.
  # Slice-22 consumes retained locations to refuse subsequent exposing mounts,
  # including approval-first operation order and real mount evidence. Slice-23
  # checks separation before trusting recovery records. CONTROL-06 cancels its
  # waiting contender before releasing publication: it tests the transaction,
  # not that later mount decision. No runtime-isolation claim is added here.
  #
  # Known exposure uses approval.state.exposed, exit 1, approval.status invalid,
  # with a safe field/path subject and guidance to choose a location outside
  # worker checkouts or finish recovery from the blocking worker's original
  # checkout. Reapproval alone cannot fix exposure. No force/auto-stop suggestion,
  # raw record, environment value, argv or engine body belongs in the diagnostic.
  # JSON retains schema 1, ok=false and complete rule/exit/message/fix problems;
  # plain output uses the same rule and meaning. Preserve the independently
  # validated candidate when available, never report matching or success.
  # Catalog symlinks/substitution use approval.state.path (1); unsafe owner,
  # mode, file type or hard links use approval.state.unsafe (1); malformed or
  # inconsistent bindings use approval.state.invalid (1); missing required or
  # unreadable evidence uses approval.state.unavailable (3); failed durable
  # publication uses approval.state.write (75). Existing prerequisite/usage
  # precedence remains. Failure never authorizes a catalog repair or engine action.
  #
  # Keep doctor and approve vertical slices thin. Shared decisions, ports,
  # I/O-free porttest fakes and disk adapters belong in engine packages. Factor
  # reusable admission below approval/session if needed to avoid a dependency
  # cycle; neither engine nor a slice may import a sibling slice. Only cmd binds
  # adapters. Run the same separation/publication contracts against memory and
  # disk, including interleavings, bootstrap interruption and write faults.
  # Record the coordination change
  # and affected arc42 sections in a decision with the implementation, as required
  # by the charter; this draft changes only this feature file.
  #
  # Fixture and red-first handoff (ALL step sentences in this file are NEW):
  # Reuse approval_fixture_test.go's independent canonical oracle and seed data,
  # approval_steps_test.go / approval_assertions_test.go's command/result handling,
  # and slice-16's overlap_fixture_test.go / overlap_steps_test.go plus recovery
  # syscall observers. Do not reuse overlapSetup's single shared state root.
  # Givens create ordinary disjoint Git checkouts P (inspected) and W (worker),
  # policies/manifests, private temporary state roots and recording Unix HTTP
  # Engine API fixtures. Seed W's matching approval independently. P's selected
  # itos-container path is equal to or beneath W's checkout as the row says;
  # W's state and the account coordination anchor are outside both checkouts.
  # Seed any prior P record directly, without enrolling its exposed location.
  # This exercises inspection of preexisting exposed state, not its authorization.
  # No Given invokes the product or checks an outcome. Fault Givens only install
  # fixture records, fault plans, descriptors and barriers; actions occur in When.
  # CONTROL-10/11 instead create only P, its ordinary inputs and API fixture,
  # and a private existing account home outside P. Set child HOME to exactly
  # that home, REMOVE XDG_STATE_HOME, and leave .local and all managed state
  # absent. Do not seed a catalog, approval or worker, nor precreate directories
  # to make a lock available. Fault plans affect only bootstrap operations;
  # ordinary approval storage and candidate inspection remain healthy.
  #
  # Use a test-only cmd assembly that substitutes the account-home lookup with
  # a temporary host-account-home. Apart from that input and specified faults/
  # barriers, use real command composition, protected disk adapters and SDK
  # against local HTTP fixtures. No production testing flag.
  # All child HOME/XDG selections, catalog roots and sockets stay under the
  # scenario's private parent; none may consult/write the person's actual state.
  # Test the production account-home adapter/wiring separately with read-only
  # lookup/port contracts. Existing approval/run harnesses need the same isolation
  # when the common anchor is wired; setting child HOME alone no longer isolates it.
  # Exact fingerprints come from fixture values, never a successful doctor call.
  # No real daemon, credentials, privileged process or host checkout helper runs.
  # These Linux amd64 local Engine API cases are not @engine scenarios.
  #
  # When starts W through the product and retains every output, exit, journal
  # and publication observation. Unexpected refusal, early exit or missing
  # barrier is data for Then, not an assertion in setup/action. Bounded barriers
  # observe actual inspection/publication syscalls or test-only storage seams;
  # no sleeps and no fake verdicts. Competing commands are separate processes.
  # Stop-only faults must leave candidate prerequisite inspection usable.
  # Prove each W reservation/activity and full-ID binding in Then, so a failed
  # worker launch cannot count as separation evidence. For CONTROL-03's preparing
  # row, prove durable registration/intent and zero creates instead of activity.
  # Preserve snapshot/intent/public ownership and approvals, with only legitimate
  # progress/completion changes allowed. Attribute engine operations by command.
  # Doctor/approve send inspections only: no worker mutation, acquisition or probe.
  #
  # Capture checkout/state inventories immediately before the inspected command,
  # excluding only fixture journals and independently verified worker progress.
  # Check bytes, modes, owners, link targets, record inode/mtime and new entries;
  # do not whitelist arbitrary changes under a coordination directory. Positive
  # publication assertions independently decode the location bindings and approval,
  # and observe successful file/parent fsync before approval becomes visible.
  # For bootstrap, observe the home-descriptor lock before the first mkdir, each
  # new directory/parent fsync, the complete self-binding's file/parent fsync,
  # and only then the first approvals-directory or approval-temp creation. Record
  # actual syscall/storage events; a missing event fails Then, never the action.
  # Bootstrap fault retries keep every surviving directory and completed binding
  # inode. Fresh-process doctor/retry checks run after the first process exits;
  # invent neither power-loss evidence nor success from mere file visibility.
  # Refusal needs the exact classified problem and no unauthorized state effects,
  # never just a nonzero exit. Safe controls use the same path and remain separate
  # from W by whole components (work-more versus work, and equal basenames).
  #
  # Commit definitions red first while these tags remain @wip. Temporarily remove
  # @wip for local assertion-level red, then implement and obtain green. Each
  # scenario's expected first failing Then is documented beside it. An undefined
  # step, setup failure or an already-passing control is not red evidence.

  # Read-only location inspection. Today doctor reports missing/matching and
  # exit 0 because it compares the managed path only with P's disjoint checkout.
  @slice-21 @ID-CONTROL-01 @local-engine-fixture
  Scenario Outline: Doctor refuses prospective and existing state inside another worker checkout
    Given a control-state fixture with P approval "<approval>" and managed location "<location>" relative to W on "<engines>"
    When W starts and a fresh P doctor inspects that location in "<format>"
    Then W has an outstanding owned worker and P reports approval.state.exposed with exit 1 instead of usable approval
    And the doctor invocation leaves all checkout control-state and outside-target entries unchanged
    When a fresh P doctor selects a missing safe textual-prefix state root while W remains active
    Then doctor reports missing approval with exit 0 without enrolling or creating that safe location
    And neither doctor invocation mutates an engine or executes checkout code

    Examples:
      | approval | location        | engines          | format |
      | absent   | equal to W root | shared daemon    | plain  |
      | absent   | below W root    | distinct daemons | JSON   |
      | matching | below W root    | shared daemon    | plain  |
      | matching | equal to W root | distinct daemons | JSON   |

  # Publication and the matching fast path both need inspection. Today approve
  # either writes into W's checkout or returns matching without checking W.
  @slice-21 @ID-CONTROL-02 @local-engine-fixture
  Scenario Outline: Consent cannot publish or reuse worker-exposed approval
    Given a control-state fixture with P approval "<approval>" and managed location "<location>" relative to W on "<engines>"
    When W starts and P approve receives its independently calculated exact fingerprint in "<format>"
    Then W has an outstanding owned worker and P reports approval.state.exposed with exit 1 instead of usable approval
    And P creates no managed directory location claim lock file or approval temp file and preserves any prior approval exactly
    And W retains its full ID activity and original protected ownership without an approve-triggered engine mutation

    Examples:
      | approval | location        | engines          | format |
      | absent   | below W root    | shared daemon    | JSON   |
      | absent   | equal to W root | distinct daemons | plain  |
      | changed  | below W root    | distinct daemons | JSON   |
      | matching | below W root    | shared daemon    | JSON   |
      | matching | equal to W root | distinct daemons | plain  |

  # Reuse slice-16's completion semantics, including retained controller-loss
  # evidence. Today none of these reservations is consulted by doctor/approve.
  # The post-recovery success is a control, not a second red requirement.
  @slice-21 @ID-CONTROL-03 @local-engine-fixture
  Scenario Outline: Separation remains unproven until every exposing reservation is completed
    Given a control-state fixture whose W reservation will be "<condition>" and whose P managed location is below W
    When W reaches that condition and P invokes "<command>" with valid approval inputs from a different state root
    Then the exposing reservation is evidenced and P reports approval.state.exposed with exit 1 without trusting or publishing approval
    And P leaves W ownership evidence and all existing approval bytes unchanged
    When the fixture clears its fault and product recovery completes every W reservation before fresh P approval
    Then P publishes matching approval only after durable completion and retains W approvals snapshots and completed recovery records

    Examples:
      | condition                                               | command |
      | preparing after registration before create              | doctor  |
      | preparing after registration before create              | approve |
      | controller absent with worker running                   | doctor  |
      | controller absent with resource observation unavailable | approve |
      | DELETE acknowledged but full worker ID still present    | doctor  |
      | resource absent but completion publication failed       | approve |
      | one of two same-root sessions completed                 | approve |

  # No catalog lock may span the terminal wait. Today W can start while the
  # review is open and the resumed approval still writes into its checkout.
  @slice-21 @ID-CONTROL-04 @local-engine-fixture
  Scenario: A worker admitted during terminal review invalidates the proposed state location
    Given a control-state review fixture with an absent P approval and a prospective location below W
    When P shows its terminal review and W starts from another state root before the fixture answers yes
    Then W is independently active before consent and P reports approval.state.exposed with exit 1 after consent
    And the unanswered review created no coordination or approval state
    And the refused approval adds no state beneath W and neither changes nor stops W

  # Pause AFTER final input reinspection, BEFORE entering the shared publication
  # transaction. This defeats a check added only to Service.Inspect. Today
  # Store.Commit publishes without consulting W even after W has registered.
  @slice-21 @ID-CONTROL-05 @local-engine-fixture
  Scenario Outline: Publication rechecks separation after another process wins worker admission
    Given a control-state fixture configured to pause P publication after final reinspection with "<engines>"
    When P pauses at that barrier and W becomes durably admitted from another state root before P resumes
    Then the admitted W reservation is evidenced and resumed P reports approval.state.exposed with exit 1 before any selected-state write
    And the race leaves no P approval or location enrollment beneath W and preserves W original ownership

    Examples:
      | engines          |
      | shared daemon    |
      | distinct daemons |

  # Pause immediately before the first selected-tree mutation, AFTER P has
  # acquired its publication transaction. The red baseline's beforePublish seam
  # is usable with a precreated empty project directory. W must reach its
  # admission attempt, not be blocked by a readiness fault. Complete W preflight
  # before P acquires publication exclusion, holding W just before admission.
  # Observe the actual cross-process lock attempt; cancel W while P is still
  # held, then release P.
  # Today W registers/creates while P is paused under only its project lock.
  @slice-21 @ID-CONTROL-06 @local-engine-fixture
  Scenario: A publishing approval excludes concurrent worker registration across state roots
    Given a control-state fixture configured to pause P inside publication before its first selected-tree mutation
    When an independent W run finishes preflight before P holds publication and then attempts admission from another state root
    Then W reaches admission but obtains no registration or create authority while P holds publication exclusion
    When the waiting W invocation is cancelled before the fixture releases P publication
    Then P publishes one complete matching approval after a durable protected location claim and W has no worker or launch continuation
    And P publication changes only its recorded location and approval while unrelated state remains intact

  # Defects affect shared discovery, not P's current approval record or normal
  # input/engine prerequisites. Other-root defects use a cataloged OTHER root;
  # the missing-catalog row instead seeds valid legacy sessions in P's selected
  # tree. Today's commands ignore each defect and succeed. The final file-only
  # calls are unchanged-scope controls. Inject the unreadable-reservation error
  # after successful metadata checks; unsafe permissions are a different case.
  @slice-21 @ID-CONTROL-07 @local-engine-fixture
  Scenario Outline: Unverifiable shared location tracking refuses inspection and publication
    Given a control-state fixture with shared tracking defect "<defect>" outside P active approval record
    When fresh P doctor and exact-fingerprint approve inspect the defect as JSON
    Then both commands report "<rule>" with exit <exit> and invalid approval without a successful publication
    And both commands preserve all state defects targets and prior records without repair or engine mutation
    When P invokes policy-only and manifest-only doctor against the same fixture
    Then both file-only inspections succeed without account-home lookup or shared-state access

    Examples:
      | defect                                                 | rule                       | exit |
      | coordination ancestor is a symlink                     | approval.state.path        | 1    |
      | coordination anchor is below P checkout                | approval.state.path        | 1    |
      | cataloged root was replaced at the same path           | approval.state.path        | 1    |
      | location record has a second hard link                 | approval.state.unsafe      | 1    |
      | coordination directory permits another user to write   | approval.state.unsafe      | 1    |
      | location record binds another effective UID            | approval.state.invalid     | 1    |
      | location record is truncated                           | approval.state.invalid     | 1    |
      | previously cataloged root is absent                    | approval.state.unavailable | 3    |
      | cataloged reservation cannot be read                   | approval.state.unavailable | 3    |
      | catalog is absent beside selected legacy session state | approval.state.unavailable | 3    |
      | host account home lookup is unavailable                | approval.state.unavailable | 3    |

  # q-35 supersedes CONTROL-10's original home-lock ordering. The fixed private
  # bootstrap chain precedes exclusion; self-binding and approval remain inside
  # it. The later doctor and matching-reuse checks retain no-write guarantees.
  @slice-21 @ID-CONTROL-10 @local-engine-fixture
  Scenario: First approval bootstraps private coordination before publishing its default-tree catalog
    Given a first-approval control-state fixture with HOME equal to the account home and XDG_STATE_HOME unset and no state directories
    When P consents with its independently calculated exact fingerprint in a fresh process
    Then P validates private bootstrap entries before taking private coordination exclusion through approval publication and completes durable self-binding before any ordinary state write
    And P reports matching approval with exit 0 and exactly generation 1 beneath the default tree bound in its own catalog
    And only the fixed bootstrap chain self-binding and P approval were created with private ownership and no engine mutation or checkout execution
    When the approval process has exited and fresh P doctor and matching approve use the same default layout
    Then both report matching approval with exit 0 while the completed self-binding and approval keep their bytes identities and generation
    And the fresh doctor writes nothing and the matching approve uses the same private coordination lock without creating a duplicate location binding

  # Faults never target ordinary approval writes. Today's untracked approval
  # succeeds, so the exact state.write refusal is the first failing Then in
  # every row. The binding-parent-fsync row leaves a visible complete binding;
  # retries must resync it without replacement, not treat visibility as durability.
  @slice-21 @ID-CONTROL-11 @local-engine-fixture
  Scenario Outline: Failed default-tree bootstrap retains evidence for a safe fresh retry
    Given a first-approval control-state fixture with HOME equal to the account home and XDG_STATE_HOME unset and no state directories
    And default-tree bootstrap has fault "<fault>" while ordinary approval writes remain healthy
    When P consents with its independently calculated exact fingerprint in a fresh process
    Then P reports approval.state.write with exit 75 and no approval directory record or temp file was created
    And only safe bootstrap entries may remain with no engine mutation or checkout change
    When the failed process has exited and the fixture clears only the fault before fresh P doctor and exact-fingerprint approval
    Then doctor reports missing approval with exit 0 and writes nothing before approval completes durable bootstrap and publishes generation 1
    And the retry preserves surviving bootstrap identities and completed binding bytes while redoing required fsyncs before any ordinary state write

    Examples:
      | fault                         |
      | coordination directory create |
      | coordination directory fsync  |
      | coordination parent fsync     |
      | self-binding record write     |
      | self-binding file fsync       |
      | self-binding rename           |
      | self-binding parent fsync     |

  # Faults target ONLY the location transaction. P's old approval and its normal
  # store operations remain healthy. Today no location publication occurs, so
  # these injected failures cannot prevent a successful untracked approval.
  @slice-21 @ID-CONTROL-08 @local-engine-fixture
  Scenario Outline: Approval cannot precede durable protection of its state location
    Given a safe control-state publication fixture with prior P approval and location fault "<fault>"
    When P consents to changed settings while the location fault is active
    Then P reports approval.state.write with exit 75 before creating or replacing an approval record
    And the prior approval bytes generation inode and mtime remain unchanged and no untracked approval is visible
    When the fixture clears only the fault and a fresh P process repeats the same consent
    Then P durably enrolls the selected location before publishing the complete next approval generation without discarding earlier location evidence

    Examples:
      | fault                       |
      | location record write       |
      | location record file fsync  |
      | location publication rename |
      | location parent fsync       |

  # The new positive behavior is persistent, shared enrollment, not merely
  # successful approval (which already works). Today no shared location record
  # is published. Child HOME is deliberately distinct from the fixture's fixed
  # account home, and both roots must remain discoverable after process exit.
  @slice-21 @ID-CONTROL-09 @local-engine-fixture
  Scenario Outline: Safe independent state roots stay tracked across environment selections
    Given safe independent control-state projects with "<layout>" and one preexisting matching approval
    When an unrelated W worker stays active while fresh approval processes use distinct XDG state homes and different child HOME values
    Then both selected locations have durable bindings in the same effective-user catalog before successful approval results
    And both approvals match their independent candidates while the preexisting approval keeps its generation bytes inode and mtime
    And W retains its original full ID activity and ownership through both publications
    When all approval processes exit and a fresh doctor selects an empty third state root using the HOME fallback
    Then doctor reports missing approval with exit 0 and writes nothing while both earlier location bindings and approvals remain intact
    And selecting a state root changes neither the independently calculated candidate fingerprint nor the authority of approvals in other roots

    Examples:
      | layout                                         |
      | state path shares only a textual prefix with W |
      | disjoint project paths have equal basenames    |

  # A read-only home descriptor models the locking capability of another UID on
  # a 0755 home. flock conflicts depend on the inode, not the holder's UID.
  @slice-21 @ID-CONTROL-12 @local-engine-fixture
  Scenario Outline: Another UID's home-directory lock cannot delay control commands
    Given a first-approval control-state fixture with HOME equal to the account home and XDG_STATE_HOME unset and no state directories
    And a readable account home and valid inputs for locked-home "<command>"
    When P invokes "<command>" while an independent read-only descriptor holds the home lock
    Then the command makes its expected progress before the home lock is released

    Examples:
      | command |
      | doctor  |
      | approve |
      | run     |

  @slice-21 @ID-CONTROL-13 @local-engine-fixture
  Scenario Outline: Contended private coordination locks have a retryable call deadline
    Given a first-approval control-state fixture with HOME equal to the account home and XDG_STATE_HOME unset and no state directories
    And valid "<command>" inputs and a private coordination lock held by another process
    When P invokes "<command>" while that private lock remains held
    Then P reports "<rule>" with exit 75 within the ten-second call budget without writes or worker creation

    Examples:
      | command | rule                |
      | doctor  | approval.state.busy |
      | approve | approval.state.busy |
      | run     | run.call-timeout    |
