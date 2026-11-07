@phase-1
Feature: Bounded offline lifecycle, cancellation and loss
  Controller transitions finish within fixed budgets without limiting workload life.
  Cancellation preserves observed exits and accounts for operations already accepted.
  Lost observations retain ownership and uncertainty; controller death grants no access.

  # Authorities: docs/charter.md, docs/plans/offline-run-lifecycle.md's AC-22/23
  # cases, docs/architecture.md (Slices), and arc42's Foreground run, Recovery
  # and Bounded operations. This file specifies slice-12 and slice-15's
  # LIFECYCLE-15; it is not runtime evidence.
  # Baseline reviewed at afc15b2, with slice-11 merged and done. Registered run
  # already uses its Recovery.stop/terminateSession path for owned termination.
  # Runner already forwards signals, consumes repeats and shares one 30 s
  # teardown context across grace, drain and cleanup. Preflight still precedes
  # the 120 s startup timer and signal capture; grace starts after signal delivery;
  # drain still blocks cleanup; timeout/loss/cancellation use generic failures.
  # Preserve those working controls; generic execution-unavailable is not the
  # specific outcome below. Existing local proof is not native signal evidence.
  # Slice-10 retains OFFLINE-02/03/06/07/13 and foreground_recovery.feature;
  # q-7 moves launch/cleanup to slice-13
  # and isolation/teardown proof to slice-14. Do not reclaim those scenarios.
  # Slice-11 supplies public status/stop, session IDs, recovery records, locks,
  # actionable retries and the shared engine-side termination contract. Read the
  # merged recovery.feature and internal/engine/session/domain/recovery.go;
  # reuse them, with no second record schema, ID grammar or ownership algorithm.
  # Use project-scoped status (no argument), stop <session> and run's actual
  # public session ID for the exact retry; an intent reference is not that ID.
  # Settled order (q-11, 2026-11-06): slice-11 precedes slice-12. LIFECYCLE-05,
  # 11, 12 and 16 activate on merged slice-11 with the rest of slice-12; there
  # is no remaining recovery prerequisite. Apply the standing regression rule.
  # LIFECYCLE-15 belongs to slice-15, still @wip, after BOTH slice-12 and
  # slice-14. It reuses slice-14's independent boundary probes/controls, built on
  # slice-13's launch/cleanup contract. First-release T-25 waits for slice-14
  # and slice-15; this spec adds no runtime support claim.
  # No init/hooks, persistent worker volumes, credentials, egress, watchdog,
  # restart, detached mode, exec, new platform or support claim is introduced.

  # Supersession belongs to the IMPLEMENTER's behaviour commit, not this draft:
  # - OFFLINE-19/20 currently expect wrapper 75 even after an observed zero exit.
  #   Replace only that provisional result with zero when cleanup and streaming
  #   succeed, and report run.cancelled separately. Keep exact signal counts,
  #   escalation, nonzero exit precedence, trailing bytes and owned cleanup.
  #   The ignored-signal fixture also supplies observed zero today; a signal
  #   request alone cannot turn that fixture observation into invented exit 137.
  # - OFFLINE-24 currently checks OS signal death during preflight. Preserve its
  #   prompt abort while inspection is still blocked, zero snapshot publication
  #   and zero worker mutations; require the typed 130/143 cancellation result.
  #   Merely catching the signal and waiting for the fixture release must fail.
  # - OFFLINE-23's classified isolation/ownership refusal still wins over a later
  #   interrupt. OFFLINE-17/18/21/22 retain failure, authority and stream semantics.
  #   A real stream read/write error remains distinct from a drain timeout.
  # Record these replacements in the behaviour commit, keep the live regressions
  # live, and update arc42's Foreground run, State and result interpretation and
  # Bounded operations with a decision naming the affected sections and 0009's
  # provisional cancellation mapping. No sibling feature or design changes here.

  # Implementation placement: extend internal/slices/run/{command,domain,port}
  # and internal/engine/session/{domain,port,<named adapters>}. Reuse slice-11's
  # shared Recovery.stop/terminateSession contract, already used by registered
  # run: owned SIGTERM once for explicit stop, 10 s grace, fresh revalidation
  # and SIGKILL only while still running. Apply run's first-signal and original
  # cancellation-deadline requirements through that same engine path, preserving
  # stop's public contract; do not define a parallel termination implementation.
  # Shared budgets, cancellation outcomes and reconciliation belong in that
  # engine; status/stop consume engine APIs, never another slice. Only cmd
  # assembles signal, clock, stream, filesystem and SDK adapters. Fakes in
  # port/porttest do no I/O; run the same contracts against fakes and adapters.
  # Domain decisions take OS as a value; retain the exact allowlist and existing
  # architecture enforcement.
  # Reuse Go 1.27.3 context/time and os/signal in adapters, the pinned Moby client
  # v0.6.2 / API v1.56.1 for inspect/wait/kill/remove and stream framing, and
  # golang.org/x/sys v0.48.0 where a native call is needed. No new dependency,
  # shell engine CLI, handwritten protocol, test flag or network service.

  # Settled limits, measured using monotonic elapsed time:
  # - Each finite external operation gets min(10 s, remaining phase budget),
  #   including connection/header establishment, engine/state inspection, safe
  #   file reads/publication, locks, create/start, signal delivery and removal.
  #   DNS/dial/SDK negotiation/reconnects share the same operation budget.
  # - Startup has 120 s from valid invocation BEFORE preflight through frozen
  #   publication, intent, create, verification, attach and start acknowledgement.
  #   Successful progress, retries and reconnects never reset that deadline.
  # - Teardown has a SEPARATE 30 s from the first observed exit, cancellation,
  #   startup timeout or fatal controller/transport failure, whichever occurs first.
  #   It includes reconciliation, signal grace/escalation, stream drain, locks,
  #   removal, absence confirmation, durable completion and diagnostics, not
  #   30 s per stage. Registered cleanup reserves the final second for reporting;
  #   exhaustion is separate from the original run cause and workload result.
  #   It must still run when the startup/caller context has expired or cancelled.
  # - Slice-11 gives status one fresh 10 s total and stop one fresh 30 s total,
  #   including lock acquisition and all inspections/mutations; calls cap at
  #   min(10 s, remaining total). Neither resumes an expired timer nor restarts
  #   work. Automatic reconciliation uses the original attempt's remaining budget.
  # - Acknowledged foreground wait/stream subscriptions are not finite startup
  #   calls. Quiet work and idle stdin can run indefinitely after launch. Bound
  #   establishment and subsequent finite requests; do not impose a heartbeat,
  #   output-idle timeout, 10 s poll exit or 120 s workload lifetime.
  # Expiring a context is not proof an accepted mutation or filesystem syscall
  # was undone. End the controller wait, forbid later launch phases, and retain
  # uncertain publication/create/start/removal evidence. No blind create/start
  # retry, no deletion by name, no unfenced not-found becoming confirmed absence.
  # Late results cannot re-enter ready/running or silently replace a returned
  # failure. Only fresh identity-checked reconciliation can advance recovery.

  # Linux signal/result choices within those settled limits:
  # The first SIGINT or SIGTERM observed by run starts cancellation. Before launch
  # send no worker signal and never enter another startup phase. If start was
  # accepted but its reply is missing, reconcile it; missing reply is not proof
  # no process ran. When a live owned primary exists, send the SAME signal once
  # through the engine to that worker's primary process, never a host PID/group.
  # Revalidate daemon/record/full resource ID before every signal or removal.
  # Give the primary until 10 s after cancellation was first observed, INCLUDING
  # signal-delivery time. Do not use image StopSignal or Docker's default grace.
  # If a fresh inspection still shows the owned primary running at that deadline,
  # attempt SIGKILL once; then observe exit and remove it. Unavailable or conflicting
  # identity/state never permits a speculative kill: retain incomplete recovery.
  # Delivery acknowledgement is not termination; a lost acknowledgement never
  # permits repeating the original signal. No forced removal before grace ends
  # while the primary is known live; confirmed primary exit permits immediate
  # owned removal of descendants. Remaining teardown time bounds all work.
  # Later SIGINT/SIGTERM, even a different signal or one during cleanup, neither
  # forward again, shorten/restart grace, reset teardown, abandon recovery nor
  # replace an earlier cause. Cancellation after observed exit only continues
  # cleanup. SIGKILL of the controller cannot execute a cleanup handler.
  # If teardown already began for a timeout or transport failure, a later signal
  # cannot replace that primary failure with 130/143 or create another teardown.
  # A3 is settled: a surviving worker stays isolated until explicit stop; no
  # controller-liveness lease or automatic worker kill/restart is authorized.

  # Public run result choices (typed outcomes in domain/ports, codes in command):
  # run.cancelled reports the first signal; with no observed workload exit use
  # 130 for SIGINT and 143 for SIGTERM. Say not-started only when known; otherwise
  # say unknown. These are wrapper cancellation outcomes, not invented child exits.
  # Preserve every observed nonzero primary exit, including 130/137/143 and codes
  # which resemble wrapper failures. An observed zero plus complete cleanup wins
  # over cancellation and returns zero; report cancellation separately. Zero plus
  # failed cleanup returns the classified cleanup failure, as OFFLINE-15 specifies.
  # An engine-observed SIGKILL status may be 137; never manufacture it from a kill
  # request. A cancellation with unknown exit keeps 130/143 even if cleanup fails,
  # with that cleanup failure separately classified and recovery retained.
  # run.call-timeout, run.startup-timeout and run.teardown-timeout are temporary
  # wrapper failures (75). Give the safe phase/operation, not raw error payloads.
  # A phase deadline takes precedence when it expires with the call deadline.
  # run.engine-lost (75) covers injected transport loss, with unknown workload
  # outcome unless an authoritative exit was actually observed. A successful
  # removal or later not-found cannot reconstruct a missing workload exit.
  # Preserve an observed nonzero exit over timeout/loss and cleanup failures;
  # observed zero cannot hide a stream/controller failure. Keep causes separate.
  # Recovery status/stop retain their slice-11 result structure, with an explicit
  # recovery.temporary (75) on budget exhaustion, never success or fabricated absence.
  # Diagnostics and recovery exclude raw argv/environment/output/response bodies;
  # deliberate workload streaming is separate. Retain the original snapshot,
  # approval, checkout edits and unrelated resources in every case.

  # Stream choices: idle stdin and blocked output while healthy work runs do not
  # create a workload timer. They must not block cancellation or engine-loss
  # observation. At teardown, drain for at most min(10 s, remaining teardown),
  # concurrently with owned stopping/cleanup, preserving available final bytes.
  # A blocked sink or missing EOF then yields run.stream-timeout (75), not silent
  # truncation/success. An observed nonzero workload exit still wins. Close only
  # owned stream handles; no unbounded wait for a goroutine stuck on caller I/O.
  # Controller diagnostic writes cannot hold teardown indefinitely either. If
  # stderr is blocked, preserve safe failure information in protected recovery
  # state when storage is available; do not promise delivery to an unread pipe.

  # Fixture/step handoff: ALL phrases below are new; no step definitions in this
  # spec commit. Reuse mechanisms from session_*, slice-10's offline_local_* and
  # offline_runtime_*, slice-11's recovery_* and cmd contract assemblies, not
  # their assertions as Givens. Include registered run in the local assemblies
  # so they exercise slice-11's shared termination and public recovery contracts.
  # Givens prepare disposable checkouts/state, independent canonical approval
  # seeds, original frozen/ownership bindings, trusted probes, clocks and faults.
  # "Accepted", "observed", "live" and "ready" in a Given describe scripted
  # observations or probe behaviour configured for When, not a successful run in
  # setup. Local fixtures write those scripts and seed only the stated preexisting
  # recovery records; runtime fixtures write the probe and inert fault barriers.
  # They never invoke approve, run, status or stop or construct expected records
  # by calling the production code being tested. Checkpoint fixture changes.
  # Whens perform all product actions, retain early exits/timeouts as data and
  # race each barrier against subprocess exit. A missing barrier fails Then, not
  # setup. Require successful same-path healthy controls, not absence alone.
  # Test-only process guards end broken runs and make the named Then fail; a
  # harness timeout/kill is never product cancellation or successful cleanup.
  # Record observations before identity-checked bounded harness-only cleanup.
  # Keep synthetic sensitive markers in inputs and test they stay out of records.
  # No real state folder, engine acquisition, helper execution from a consumer
  # checkout, production injection switch or actual credential enters a fixture.
  # Trusted Go probes may be compiled into the disposable checkout, then execute
  # ONLY inside the CI worker with T-20's independently inspected cached image.

  # Local @linux cases exercise production command/domain/engine contracts via
  # test-only cmd assemblies with I/O-free fakes and disk/SDK fixtures. Start at
  # the real command entry for whole-invocation budgets and preflight cancellation;
  # starting the observation after frozen handoff cannot prove LIFECYCLE-01/02/06.
  # Reuse standard-library testing/synctest and the in-memory SDK transport pattern
  # in session/docker/run_wait_test.go for full-budget controlled-time checks.
  # Socket-backed adapter tests separately inspect propagated deadlines and actual
  # request cancellation; a virtual clock does not advance an external HTTP server.
  # Use controlled barriers, not 120 s sleeps in hooks, shortened product defaults
  # or fakes returning the desired timeout/result. No wall-clock value comes from
  # checkout data. Include just-before/at-boundary tests beneath these cases.
  # Shared fake/adapter contracts must observe the same deadlines and sequencing;
  # fakes model time/events, never do I/O. Capture all API attempts, including
  # reconnects, failed/refused requests and late completions. Export product
  # coverage through existing test-assembly GOCOVERDIR support.
  # Runtime @engine cases require the guarded Linux amd64 native-Docker CI tier
  # in features/README.md. Actual signals go to the product controller only in
  # When; native engine observations and probe receipts establish worker effects.
  # Product-facing disconnects never shut down the shared daemon or independent
  # observer. Add only identity-checked kill/wait fault forwarding to the recorder.
  # Runtime timing allows at most 2 s scheduling tolerance per measured bound,
  # never early escalation; no resetting deadlines or widening product defaults.
  # Fixed-seed domain contracts cover exact boundaries; CI proves real delivery.
  # Network denial after controller loss uses slice-14's SAME clients and controlled
  # IPv4/IPv6 TCP/UDP/gateway/private/synthetic-metadata and DNS/DoH endpoints.
  # Trusted positive controls before/after each worker attempt must succeed;
  # missing probes, unavailable controls or empty reports fail, never skip.
  # Host sentinels must exist and worker-local file/socket controls must work.

  # Standing coordinator decision for slice-12, extending q-6: a scenario that
  # already passes against behaviour built by earlier slices is ACCEPTED as
  # regression coverage. Confirm the complete case passes at its required tier
  # because it observes product behaviour, not fixture assignments or absence of
  # actions. Comment above its tag which earlier slice it exercises, remove @wip
  # and continue; no further regression approval or artificial red is required.
  # Passing outline rows are regressions too; keep them while implementing failing
  # rows, and turn the outline live when every row passes. A passing assertion
  # alone does not establish that the rest of a scenario works. Stop only for a
  # scenario that fails in setup where no step can fix it, or whose assertions
  # cannot be made to fail against a wrong build. Missing steps are work to add.
  #
  # The implementer commits complete steps first with @wip retained and runs them
  # against the merged dependency heads. For behaviour still missing, obtain
  # assertion-level local red or coordinator-only CI red_scenarios evidence before
  # implementing, then passing evidence at the same tier. The standing decision
  # replaces the red requirement for verified regressions; do not report them as
  # newly red. Existing deadlines, signal forwarding, repeat suppression and
  # ownership are controls. This rule explicitly includes the exit-17 rows and
  # LIFECYCLE-09, as well as LIFECYCLE-05/11/16's merged slice-11 integration.
  # Do not demand artificial red for their existing behavior or stop for another
  # decision; preserve all assertions and verify the complete case at its tier.
  # Native definitions and coordinator red evidence cover 08/09/11/12/13/16.
  # Local steps are committed at df8b8e6, 9b9ebd3 and e67210a. All 37 local
  # rows of 01–07, 10 and 14 have passing command evidence; assertion-level red
  # and verified regressions are recorded in decision 0014. Native proof remains
  # separate; run 42312330532 at 41529b1 passed all 18 slice-12 engine cases.

  # Individual waits: existing generic SDK errors do not identify the timed-out
  # phase; preflight/state waits currently lack the complete shared deadline.
  # For LIFECYCLE-01/02, capture cancellation and phase-entry observations at the
  # deadline, then advance controlled time through the separate cleanup attempt.
  # Include accepted-create/start faults whose cleanup remains pending beyond the
  # timeout; allow final return after the cleanup attempt finishes or exhausts its
  # budget. Cancelling the controller wait does not undo an accepted side effect.
  # No authoritative nonzero workload exit is supplied here; LIFECYCLE-03 covers
  # that precedence. Already-bounded calls may pass the cancellation assertion;
  # final timeout classification and the cleanup bound still need their own proof.
  @slice-12 @ID-LIFECYCLE-01 @linux
  Scenario Outline: A stalled finite operation consumes at most its call budget
    Given a lifecycle contract fixture with a stalled "<operation>" and a healthy control
    When the real run contract reaches the operation and controlled time advances through its 10 second deadline and the ensuing cleanup through each local backend
    Then each run cancels its wait for the pending operation by its call deadline and begins no subsequent startup phase
    And any accepted mutation remains accountable to the original intent
    And after the cleanup attempt each run reports "run.call-timeout" with exit 75 and the named operation no later than 30 seconds after its call deadline
    And the healthy control launches and cleans exactly one owned worker with unchanged unrelated state

    Examples:
      | operation                       |
      | preflight engine inspection     |
      | preflight approval state read   |
      | frozen snapshot publication     |
      | intent lock acquisition         |
      | intent publication              |
      | create response                 |
      | boundary inspection             |
      | attach establishment            |
      | start acknowledgement           |

  # Each successful operation takes less than 10 s. Place 115 s BEFORE Runner
  # launch, then leave attach/start pending across t=120. This catches the old
  # budget starting only after preflight. Repeat with progress/reconnect events;
  # an implementation need not invent retries simply to satisfy a test.
  @slice-12 @ID-LIFECYCLE-02 @linux
  Scenario: Startup has one deadline beginning before preflight
    Given a lifecycle contract fixture with 115 seconds of successful preflight and publication progress and delayed launch observations
    When run proceeds through the full invocation and controlled time advances through the 120 second startup deadline and the ensuing cleanup through each local backend
    Then each run cancels its wait for the pending startup operation at the original 120 second deadline and begins no subsequent startup phase
    And progress reconnects and any retries never extend startup or begin a phase after the deadline
    And the pending operation receives only the remaining startup time and cleanup receives its own 30 second budget
    And after the cleanup attempt each run reports "run.startup-timeout" with exit 75 no later than 30 seconds after the original startup deadline
    And delayed replies cannot launch work or replace the reported timeout

  # Acknowledgement loss after accepted start is different from confirmed failure
  # to start. Independent inspection/wait supplies 17 or remains unavailable.
  # Regression attribution: slice-10 preserves observed nonzero exits; slice-11
  # reconciles registered starts through shared stop. Verify the exit-17 row as
  # written, including its separate timeout report, under the standing rule.
  @slice-12 @ID-LIFECYCLE-03 @linux
  Scenario Outline: A timed-out start is reconciled without inventing a child result
    Given a lifecycle contract fixture with accepted start and a withheld reply followed by "<observation>"
    When the start call expires and run reconciles and cleans through each local backend
    Then each run reports the start call timeout separately and returns <result> with workload outcome "<outcome>"
    And exactly one create and one start were attempted and no late reply restarts or duplicates work
    And cleanup uses fresh ownership and full ID inspection and retains recovery when absence is unconfirmed

    Examples:
      | observation                     | result | outcome |
      | authoritative primary exit 17   | 17     | 17      |
      | no authoritative exit available | 75     | unknown |

  # At t=0 observe primary exit. Hold drain 9 s and make successive healthy
  # cleanup operations consume the total; keep the last observation pending at
  # t=30 without any earlier per-call expiry. The baseline already shares 30 s;
  # distinguish its generic cleanup failure from the required teardown timeout,
  # and prove that a pending drain does not prevent owned reconciliation.
  # Regression attribution: slice-10 supplies the shared teardown total and
  # exit-17 precedence; slice-11 supplies registered cleanup. Preserve passing
  # controls while checking the complete rows under the standing rule.
  # Both complete rows pass at 41529b1: slice-10 totals/exit precedence,
  # slice-11 recovery and earlier slice-12 typed teardown reporting.
  @slice-12 @ID-LIFECYCLE-04 @linux
  Scenario Outline: Drain and cleanup share one teardown deadline
    Given a lifecycle contract fixture with observed exit <child> and slow drain reconciliation removal and completion
    When run begins teardown and controlled time reaches 30 seconds through each local backend
    Then each run reports "run.teardown-timeout" by the original teardown deadline and returns <result>
    And the workload exit remains <child> with incomplete cleanup and retained recovery evidence
    And owned reconciliation begins while final output drain is still pending without discarding available trailing bytes
    And no drain progress retry reconnect or repeated cancellation restarts the teardown clock
    And no successful removal reply is reported as confirmed durable absence

    Examples:
      | child | result |
      | 0     | 75     |
      | 17    | 17     |

  # These integration cases adopt slice-11's protected records/public references;
  # no broad listing/prune or new session schema is specified here. Setup seeds
  # an owned candidate, not a prior successful product run or stop.
  # Merged slice-11's RECOVERY-19 covers recovery timeout classes and totals.
  # Recheck every row, including the fresh invocation. Cases passing for the
  # right reason are accepted regressions under the standing rule;
  # retain this attribution when turning the complete outline live.
  # Both status rows pass before this change through slice-11 recovery.
  # Stop rows were red at their finite-call deadline assertions.
  @slice-12 @ID-LIFECYCLE-05 @linux
  Scenario Outline: Each explicit recovery invocation has a fresh finite budget
    Given a lifecycle recovery fixture with a protected owned session and stalled "<operation>" observations
    When "<command>" exhausts its recovery budget and a fresh invocation follows after the fault clears through each local backend
    Then the first recovery invocation reports recovery.temporary with exit 75 within <budget> seconds with every finite call bounded to the remaining budget or 10 seconds
    And the first result retains the session identity and uncertainty without claiming stopped resources or a workload exit
    And the fresh invocation gets a new <budget> second budget and completes only its requested inspection or owned cleanup
    And neither invocation creates starts or adopts a worker or changes another session

    Examples:
      | command | operation                       | budget |
      | status  | engine inspection               | 10     |
      | status  | aggregate record reconciliation | 10     |
      | stop    | session lock acquisition        | 30     |
      | stop    | signal delivery                 | 30     |
      | stop    | removal                         | 30     |
      | stop    | absence confirmation            | 30     |
      | stop    | aggregate cleanup retries       | 30     |

  # Deterministic cancellation before launch, including a phase whose operation
  # has returned just as cancellation becomes observable. No lifecycle hooks exist.
  @slice-12 @ID-LIFECYCLE-06 @linux
  Scenario Outline: Cancellation prevents the next startup phase
    Given a lifecycle contract fixture with a cancellation barrier "<barrier>"
    When the run contract receives "<signal>" at that barrier and the pending operation returns through each local backend
    Then each run reports "run.cancelled" with exit <result> and a known not-started workload
    And no operation belonging to a later startup phase or worker signal is attempted
    And any accepted unstarted worker is reconciled and cleaned within the separate teardown budget

    Examples:
      | barrier                      | signal  | result |
      | before preflight             | SIGINT  | 130    |
      | before preflight             | SIGTERM | 143    |
      | before create                | SIGINT  | 130    |
      | before create                | SIGTERM | 143    |
      | after ownership publication  | SIGINT  | 130    |
      | after ownership publication  | SIGTERM | 143    |
      | after boundary verification  | SIGINT  | 130    |
      | after boundary verification  | SIGTERM | 143    |
      | after attach before start    | SIGINT  | 130    |
      | after attach before start    | SIGTERM | 143    |

  # Delay accepted create until after cancellation's first cleanup ends. Name
  # not-found cannot fence that request; preserve slice-9's uncertain-create rule.
  # Accepted start already passes through slice-11 protected recovery and
  # the earlier slice-12 cancellation mapping; accepted create was red.
  @slice-12 @ID-LIFECYCLE-07 @linux
  Scenario Outline: Cancellation cannot forget an accepted create or start
    Given a lifecycle contract fixture with an accepted "<operation>" whose response and observations are withheld
    When run receives SIGTERM in flight and returns before the operation becomes observable and a fresh controller later reconciles it
    Then the first run returns 143 with "run.cancelled" and workload outcome "<outcome>" while retaining the original protected intent
    And the first cleanup neither records unfenced absence nor retries create or start
    And the later controller reconciles the original full identity and cleans without launching or restarting work
    And no accepted resource or checkout edit is lost from the independent audit

    Examples:
      | operation | outcome     |
      | create    | not-started |
      | start     | unknown     |

  # Prove actual delivery and precedence, including zero. Handlers install before
  # their ready frame, write a signal receipt to checkout, then exit as requested.
  # Regression attribution: slice-10 forwards signals and preserves exit 17;
  # slice-11 supplies registered termination. The merged zero case still returns
  # 75 and neither case reports cancellation separately from execution failure.
  # Accept any complete exit-17 row that passes at its required tier under the
  # standing rule; do not replace observed exit with a wrapper cancellation code.
  @slice-12 @ID-LIFECYCLE-08 @engine
  Scenario Outline: The primary receives the first signal and its observed exit wins
    Given a CI lifecycle checkout with a ready signal-handling probe that exits <child>
    When run launches the probe and the harness sends "<signal>" to the controller after the ready frame
    Then the worker records exactly one "<signal>" and the product returns the observed exit <child> after confirmed cleanup
    And run reports "run.cancelled" with the first signal separately from the authoritative workload exit and cleanup result
    And the engine journal has exactly one original signal request targeting only the freshly revalidated owned primary with no early SIGKILL or host process signal
    And the final worker frame and checkout edit survive while the exact worker and temporary storage are absent

    Examples:
      | signal  | child |
      | SIGINT  | 0     |
      | SIGINT  | 17    |
      | SIGTERM | 0     |
      | SIGTERM | 17    |

  # An ignoring probe records signals but keeps running. Send repetitions at
  # elapsed 2 s and 8 s, including the other signal; no repeated input extends grace.
  # The recorder forwards the first signal immediately, then delays only its
  # acknowledgement by the example duration. Native receipts prove delivery.
  # Six seconds is inside the 10 s call limit but makes acknowledgement-relative
  # grace observably late. The immediate rows retain the healthy timing control.
  # Regression attribution: slice-10 supplies first-signal/repeat suppression;
  # slice-11 supplies shared owned termination and escalation. The standing rule
  # applies to LIFECYCLE-09, including any complete already-passing rows. Verify
  # native receipts and timings; fixture exit values are not that evidence.
  @slice-12 @ID-LIFECYCLE-09 @engine
  Scenario Outline: Ignored and repeated cancellation escalates once after the fixed grace
    Given a CI lifecycle checkout with a ready signal-recording probe that keeps running and a signal acknowledgement delayed <delay> seconds
    When run launches the probe and the harness sends "<signal>" followed by repeated SIGINT and SIGTERM during grace
    Then the worker records only the first "<signal>" and native observations show it alive before the 10 second grace expires
    And the journal records exactly one original signal request and one SIGKILL request at the original grace deadline and the product returns the engine-observed exit 137
    And run reports "run.cancelled" with the first signal separately from the authoritative workload exit and cleanup result
    And owned removal and durable absence complete within the original 30 second teardown budget
    And no unrelated process resource approval snapshot or checkout edit is changed

    Examples:
      | signal  | delay |
      | SIGINT  | 0     |
      | SIGTERM | 0     |
      | SIGINT  | 6     |
      | SIGTERM | 6     |

  # The first signal's acknowledgement stays unavailable for 10 s. Grace starts
  # at cancellation, not acknowledgement; the cancellation result is NOT 137
  # when both wait and exit inspection remain unavailable. Same signal delivery
  # must not be repeated even if the first request may have reached the primary.
  # Fresh full-identity inspection can still establish running before escalation;
  # after escalation it cannot establish a primary exit. Safe mutation authority
  # is available, but authoritative exit and final absence observations are not.
  # Both rows pass against 41529b1, exercising slice-11 owned termination
  # and the earlier slice-12 fixed grace/cancellation result implementation.
  @slice-12 @ID-LIFECYCLE-10 @linux
  Scenario Outline: Lost signal acknowledgement never becomes an observed workload exit
    Given a lifecycle contract fixture with a live owned worker and lost signal acknowledgement and exit observations
    When run receives "<signal>" and controlled time crosses grace and teardown deadlines through each local backend
    Then each run preserves "run.cancelled" with exit <result> and an explicitly unknown workload outcome
    And the first signal is attempted once and escalation is attempted at the original 10 second deadline without a second grace period
    And each mutation revalidates the original daemon record and full ID and uncertain cleanup retains recovery
    And successful signal or removal acknowledgements never synthesize exit zero or 137

    Examples:
      | signal  | result |
      | SIGINT  | 130    |
      | SIGTERM | 143    |

  # Signals after observed primary exit cannot erase 17 or abort recovery. Keep
  # removal unavailable until after the original teardown expires, then retry.
  # Regression attribution: slice-10 supplies exit-17 precedence and repeat
  # suppression; merged slice-11 supplies the public ID, retained record and stop.
  # Verified at 9ac4f11 by CI run 42297748701; accepted under q-6/q-11.
  @slice-12 @ID-LIFECYCLE-11 @engine
  Scenario: Repeated signals during cleanup preserve the workload result and recovery
    Given a CI lifecycle checkout with an exit-17 probe and an unavailable removal barrier
    When run observes the probe exit and the harness sends SIGINT then SIGTERM during cleanup before restoring removal for an explicit stop
    Then run returns 17 within the original teardown budget with a separate incomplete cleanup result and usable recovery identity
    And neither signal is forwarded to an already exited primary or causes duplicate unsafe removal or loss of the protected record
    And the later stop removes only the original full ID and confirms durable absence without restarting the worker
    And the checkout edit approvals snapshots and unrelated resources remain intact

  # Block a finite transport request after the healthy control, not the native
  # daemon. The probe stays alive and edits checkout; all authoritative workload
  # exit observations remain unavailable to the controller during this attempt.
  # Use merged slice-11's public recovery identity and status/stop in this case;
  # activation does not wait for a later recovery interface.
  @slice-12 @ID-LIFECYCLE-12 @engine
  Scenario: Engine transport loss reports an unknown workload outcome and retains recovery
    Given a CI lifecycle checkout with a live marker probe and a product-only engine disconnect barrier
    When run launches the probe and loses its wait and inspection transport before any exit observation and a later status and stop follow restoration
    Then run returns 75 with "run.engine-lost" and an explicitly unknown workload outcome within bounded teardown
    And the first result retains ownership and incomplete cleanup rather than a success exit or false absent claim
    And native observations show the surviving worker unchanged and isolated during disconnection
    And the later status and stop revalidate the original resource and confirm owned cleanup without rewriting unknown history as workload success
    And checkout edits approvals snapshots and unrelated resources remain intact

  # Signal responsiveness must survive actual pipe backpressure. For the quiet
  # row wait 125 s after ready with no output/input; for blocked rows fill only
  # the named pipe after ready. The handler writes its signal receipt to checkout
  # and exits 17 without waiting on a blocked writer. Include a same-path quiet
  # control; one 125 s control may be shared by this outline's runtime cases.
  # The harness observes controller termination independently before unblocking
  # drains. Do not make a test-owned reader unblock the product before its bound.
  # Quiet work and open stdin are slice-10 controls. Readable diagnostics must
  # distinguish cancellation from the baseline's generic execution error; the
  # unread-stderr row instead proves bounded return and safe retained failure state.
  @slice-12 @ID-LIFECYCLE-13 @engine
  Scenario Outline: Quiet work and blocked streams do not prevent cancellation
    Given a CI lifecycle checkout with a ready signal-handling probe and stream condition "<condition>"
    When run remains live under that condition and the harness sends SIGTERM while leaving the condition in place
    Then the probe records SIGTERM and the product completes owned teardown within 30 seconds of cancellation without waiting for the blocked stream
    And when controller diagnostics are writable they name "run.cancelled" and SIGTERM separately from observed exit 17
    And the quiet control remains running past both the call and startup durations until cancellation
    And any incomplete output drain is reported as a stream timeout without changing observed exit 17 or claiming complete delivery
    And blocked diagnostic output cannot prevent termination and safe failure state survives when storage is available

    Examples:
      | condition                  |
      | quiet workload and stdin   |
      | unread workload stdout     |
      | unread workload stderr     |
      | stdin writer remains open  |

  # Local exact-boundary counterpart to blocked real pipes. The output stream
  # never closes, although the engine has already observed the primary's exit.
  # Regression attribution: slice-10 supplies exit-17 precedence; the separate
  # stream-timeout assertion still needs proof. Apply the standing rule to the
  # complete rows, without requiring existing precedence to fail anew.
  @slice-12 @ID-LIFECYCLE-14 @linux
  Scenario Outline: A stalled final drain is a failure distinct from workload exit
    Given a lifecycle contract fixture with observed exit <child> and a final output drain that never completes
    When run cleans the worker and controlled time crosses the 10 second drain deadline through each local backend
    Then each run reports "run.stream-timeout" and returns <result> without waiting beyond the shared teardown deadline
    And the authoritative workload exit remains <child> and confirmed owned cleanup remains complete
    And available trailing bytes are preserved without storing workload output in diagnostics or recovery

    Examples:
      | child | result |
      | 0     | 75     |
      | 17    | 17     |

  # Independent native observations are essential: flag inspection alone cannot
  # prove A3. Use a continuous probe with externally released rounds so a fresh
  # report after SIGKILL cannot be a buffered pre-kill result. No watchdog claim.
  # Slice-11's RECOVERY-22 specifies the first Then's discovery and stop behavior;
  # treat it as an integration control. Fresh post-loss boundary assertions need
  # independent CI evidence with slice-14's probes, not that discovery result.
  # If earlier slices already satisfy the complete case, name them here and turn
  # it live under the standing regression decision after confirming that evidence.
  # Q-11 assigns this case to slice-15 after slice-12 and slice-14. Keep it @wip
  # until both prerequisites and its independent post-loss evidence are available.
  @wip @slice-15 @ID-LIFECYCLE-15 @engine
  Scenario: Uncatchable controller loss leaves a live isolated candidate for explicit stop
    Given a CI lifecycle checkout with continuous boundary probes and working external route and host-access controls
    When run launches the probe and the harness kills only the controller with SIGKILL then requests a fresh probe round before status and stop
    Then status identifies the lost-controller session as a stop candidate with unknown workload outcome and its exact retry removes only that owned worker
    And the same worker reports a fresh post-loss round under the unchanged offline filesystem identity and network boundary while remaining alive until stop
    And all external route and resolver controls succeed while every worker attempt is denied with no matching listener receipt
    And host sentinels sockets approval and recovery state remain inaccessible while worker-local controls and checkout writes succeed
    And stop revalidates the full ID and confirms durable absence without changing the original unknown workload history solely because removal succeeded
    And no automatic restart replacement worker new permission or synthetic workload success is reported and checkout edits survive

  # Crash windows use actual controller SIGKILL, not a returned fake error. The
  # observer retains IDs only for assertions/always-cleanup, never passes them as
  # authority to the new controller. Before absence publication, engine removal
  # may already have succeeded; recovery must inspect, not infer, that fact.
  # Slice-9's uncertain-create ownership and slice-10's foreground cleanup are
  # existing controls. Merged slice-11 supplies RECOVERY-14/15/22's public recovery;
  # all four windows passed at 9ac4f11 in CI run 42298705984 (24 steps).
  # Accepted as slice-11 recovery regressions under q-6/q-11.
  @slice-12 @ID-LIFECYCLE-16 @engine
  Scenario Outline: Controller death during an in-flight transition retains recoverable ownership
    Given a CI lifecycle checkout with a trusted controller-kill barrier "<barrier>"
    When run reaches the barrier and the harness kills only its controller then invokes fresh status and stop after releasing the observation fault
    Then fresh status reports controller absent with the original recovery identity and pending or unknown cleanup without inventing workload success or completed cleanup
    And fresh stop reconciles only the original intent daemon snapshot and concrete resource identity and confirms durable absence
    And no replacement create start name-only removal or unrelated resource mutation is attempted
    And pre-start interruptions execute no worker instruction and any already running worker retains its offline boundary until removal

    Examples:
      | barrier                                      |
      | after accepted create before reply           |
      | after accepted start before reply            |
      | during removal before its reply              |
      | after removal before durable absence update  |

  # Regression for abandoned filesystem calls: the pending operation really
  # resumes after the command returns. Policy and manifest readers borrow the
  # same checkout pin. A late pin must also be released after interruption.
  @slice-12 @ID-LIFECYCLE-17 @linux
  Scenario Outline: Interrupted checkout operations finish safely after run returns
    Given a lifecycle checkout with a blocked "<operation>" interrupted by "<cause>"
    When run stops waiting and the abandoned checkout operation later finishes through each local backend
    Then run returns its bounded interruption result without closing checkout state used by the pending operation
    And the late completion releases its checkout pins without launching a worker or replacing the result
    And the healthy control launches and cleans exactly one owned worker with unchanged unrelated state

    Examples:
      | operation             | cause   |
      | checkout pin          | timeout |
      | checkout inspection   | timeout |
      | checkout revalidation | timeout |
      | policy read           | timeout |
      | manifest read         | timeout |
      | checkout pin          | SIGINT  |
      | checkout inspection   | SIGINT  |
      | checkout revalidation | SIGINT  |
      | policy read           | SIGINT  |
      | manifest read         | SIGINT  |

  # Review regression: capture diagnostics at command return, before any harness
  # goroutine flush. The real entry point exits immediately. A briefly delayed
  # writable sink distinguishes reserved delivery time from scheduler luck.
  # Blocked-sink rows retain slice-12's existing deadline/exit/recovery controls
  # as regression coverage under the standing coordinator decision.
  @slice-12 @ID-LIFECYCLE-18 @linux
  Scenario Outline: Cleanup exhaustion leaves time to report its retained session
    Given a lifecycle teardown fixture with observed exit <child> and "<sink>" diagnostic output
    When run exhausts cleanup and its return-time diagnostics are captured through each local backend
    Then writable diagnostics contain run.teardown-timeout and the exact retained session retry before run returns
    And blocked diagnostics cannot extend the original 30 second teardown deadline
    And observed workload exits keep precedence and missing observations remain unknown
    And fresh recovery cleans only the retained owned worker without restarting it

    Examples:
      | child | sink     |
      | 0     | writable |
      | 17    | writable |
      | 0     | delayed  |
      | 17    | delayed  |
      | 0     | blocked  |
      | 17    | blocked  |

  # Review regression: cleanup timeout is a separate failure, never a replacement
  # for the transport/controller cause. Exercise both termination and removal;
  # only the latter receives a fresh authoritative exit before cleanup stalls.
  @slice-12 @ID-LIFECYCLE-19 @linux
  Scenario Outline: Cleanup exhaustion preserves the original run failure
    Given a lifecycle teardown fixture with "<cause>" followed by stalled "<phase>"
    When run exhausts cleanup and its return-time diagnostics are captured through each local backend
    Then the original run failure is reported separately from run.teardown-timeout
    And observed workload exits keep precedence and missing observations remain unknown
    And fresh recovery cleans only the retained owned worker without restarting it

    Examples:
      | cause                 | phase       |
      | engine-lost           | termination |
      | execution-unavailable | termination |
      | engine-lost           | removal     |
      | execution-unavailable | removal     |

  # Review regression: a socket identity syscall can ignore its request context.
  # Release it only after command return (or the failing bounded harness guard).
  # Create is accepted before its reply stalls; attach holds a late handle;
  # start stalls before issuing its request. Preserve the connection until the
  # operation actually finishes, with no subsequent launch or repeated mutation.
  @slice-12 @ID-LIFECYCLE-20 @linux
  Scenario Outline: Non-cancellable launch operations cannot extend the controller wait
    Given a lifecycle launch with a cancellation-ignoring "<operation>" interrupted by "<cause>"
    When run returns before the pending launch operation is released through each local backend
    Then run reports the interruption within its original operation and teardown budgets while retaining the pending connection
    And late launch completion closes its connection and any attachment without resuming launch retrying mutations or changing the returned result
    And the healthy control launches and cleans exactly one owned worker with unchanged unrelated state

    Examples:
      | operation             | cause           |
      | create response       | call timeout    |
      | boundary inspection   | call timeout    |
      | attach establishment  | call timeout    |
      | start acknowledgement | call timeout    |
      | create response       | startup timeout |
      | boundary inspection   | startup timeout |
      | attach establishment  | startup timeout |
      | start acknowledgement | startup timeout |
      | create response       | SIGINT          |
      | boundary inspection   | SIGINT          |
      | attach establishment  | SIGINT          |
      | start acknowledgement | SIGINT          |

  # Review regression: native wait can stall in a socket identity syscall before
  # its cancellable request. An exited primary has no recorded exit yet; hold
  # that recovery wait past teardown and release it only after command return.
  @slice-12 @ID-LIFECYCLE-21 @linux
  Scenario: A non-cancellable exit observation cannot extend teardown
    Given a lifecycle exited worker with an unrecorded exit and a cancellation-ignoring recovery wait
    When run returns before the pending exit observation is released through each local backend
    Then run reports its original failure and teardown timeout within 30 seconds with unknown workload outcome and retained recovery
    And the pending exit observation retains its connection until completion without changing the returned result or recovery record
    And a healthy foreground wait longer than startup still returns its observed exit and cleans only its owned worker

  # Review regression: a pending workspace identity check supplies no evidence
  # of substitution. Preserve its call/startup timeout or first signal, and keep
  # a completed substitution check's ownership refusal as a separate control.
  # All eight rows failed at their result assertion on 029a406 with @wip removed:
  # each returned ownership refusal (1) through both local backends.
  @slice-12 @ID-LIFECYCLE-22 @linux
  Scenario Outline: Interrupted workspace revalidation preserves its original cause
    Given a lifecycle workspace revalidation blocked before "<phase>" and interrupted by "<cause>"
    When run returns before the pending workspace revalidation is released through each local backend
    Then run reports the original revalidation timeout or cancellation rather than an ownership refusal within its original budgets
    And no later launch phase runs and the pending workspace remains pinned until its late completion
    And a confirmed workspace substitution still refuses launch while an unchanged workspace launches and cleans normally

    Examples:
      | phase  | cause           |
      | create | call timeout    |
      | start  | call timeout    |
      | create | startup timeout |
      | start  | startup timeout |
      | create | SIGINT          |
      | start  | SIGINT          |
      | create | SIGTERM         |
      | start  | SIGTERM         |

  # Review regression: cancellation arrives after the finite-call precheck but
  # before the workspace operation is dispatched. Exercise both ready select
  # alternatives; neither may acquire a pin after its caller has been cancelled.
  # Both rows failed at their no-pin assertion on 88dd0e5 with @wip removed,
  # through both local backends; the cancelled dispatch still called Pin.
  @slice-12 @ID-LIFECYCLE-23 @linux
  Scenario Outline: Cancellation before workspace acquisition prevents late pins
    Given a lifecycle workspace acquisition interrupted by "<signal>" before adapter dispatch
    When run returns and all dispatched workspace calls settle through each local backend
    Then run reports "run.cancelled" with exit <result> and no workspace pin or worker launch occurs after cancellation
    And the healthy control launches and cleans exactly one owned worker with unchanged unrelated state

    Examples:
      | signal  | result |
      | SIGINT  | 130    |
      | SIGTERM | 143    |
