@phase-1
Feature: Checkout Unix sockets have an explicit milestone boundary
  Run refuses a checkout containing a Unix socket before creating a worker.
  This startup check does not prevent access to sockets created or swapped in
  after inspection. Continuous enforcement is deferred to milestone 2.

  # The person's q-30 decision, 2026-11-07 ~01:50 UTC, supersedes the original
  # slice-19 continuous-denial spec. Authorities: docs/charter.md,
  # docs/plans/container-pilot.md (Goal and limits), docs/architecture.md (Slices),
  # docs/decisions/0019-checkout-socket-milestone-limit.md and
  # docs/plans/checkout-socket-enforcement.md.
  #
  # Slice-19 requires a startup scan before create. Any Unix socket, live or
  # stale, at any depth including .git, causes the existing typed isolation
  # refusal (exit 1, run.isolation-unverified), with no worker created. Traverse
  # real directory entries without following symlinks outside the checkout.
  # Never delete/chmod sockets, execute checkout code or alter approval to make
  # a checkout pass. Keep the read/write checkout bind and private worker IPC.
  # Filesystem I/O stays behind a port; use Go 1.27.3's standard-library file
  # types/traversal and existing pinned native APIs, not a new dependency.
  #
  # Document plainly in docs/security.md, the threat model and run's isolation
  # help that sockets created or swapped in after inspection remain reachable.
  # No polling, kernel bootstrap or alternate runtime is authorized this slice.
  #
  # Accepted evidence: prior connectivity reds were accepted for 01/03 in run
  # 42312676619 at 1662753 and for 02 in run 42323471709 at 69b02a8. They prove
  # the gap. Coordinator q-34 accepted the fresh reworded ID-SOCKETS-01 red
  # in run 42391893492 at f33853b on 2026-11-07 ~16:45 UTC: all 12 rows failed
  # at Then, "socket checkout reached worker creation: exit=0 create requests=1".
  # Implementation now requires ordinary green CI with no red selector.
  # A fixture error or unrelated startup refusal is not attributable red. Report
  # controller stderr and recorder/engine diagnostics at the Then assertion.
  #
  # One socket per outline row ensures shallow scans, .git exclusions and
  # type-specific omissions cannot hide behind another socket's refusal.
  # The trusted host listener only echoes synthetic nonces. Before/after host
  # controls prove its identity and usability; no real service or credential is
  # a target. Given only prepares the fixture and independent approval. When
  # invokes run -- /bin/true against the independently inspected cached image.
  # Then requires the typed refusal and both zero create requests (including
  # failed requests) and zero recorded full worker IDs. Cleanup is bounded and
  # identity checked after all observations, never evidence of product refusal.
  @slice-19 @ID-SOCKETS-01 @engine
  Scenario Outline: Run refuses a checkout containing a Unix socket before creating a worker
    Given a CI socket checkout with a live "<operation>" host socket at "<path>"
    When the product attempts an approved run of the socket checkout
    Then the socket checkout is refused with exit 1 and run.isolation-unverified before any worker create
    And the startup socket listener remains usable and unchanged
    And the socket refusal preserves existing checkout files approval and unrelated engine resources

    Examples:
      | operation         | path                          |
      | stream-connect    | socket                        |
      | seqpacket-connect | socket                        |
      | dgram-sendto      | socket                        |
      | stream-connect    | nested/a/b/c/d/e/socket        |
      | seqpacket-connect | nested/a/b/c/d/e/socket        |
      | dgram-sendto      | nested/a/b/c/d/e/socket        |
      | stream-connect    | .git/socket                   |
      | seqpacket-connect | .git/socket                   |
      | dgram-sendto      | .git/socket                   |
      | stream-connect    | .git/nested/a/b/c/d/e/socket   |
      | seqpacket-connect | .git/nested/a/b/c/d/e/socket   |
      | dgram-sendto      | .git/nested/a/b/c/d/e/socket   |

  # Slice-42: ordinary doctor names the checkout socket backend the host offers,
  # so the person sees it before approving. Authorities: docs/charter.md, second
  # milestone step 5 (Landlock ABI 9 or later by default, gVisor as fallback,
  # with neither only "late host sockets reachable"), T-64's ledger entry and
  # T-66's fixture (features/README.md). These scenarios live here, not in
  # doctor.feature, because TestDoctorStepDefinitions requires a definition for
  # every doctor.feature pickle, @wip included, so a spec-only commit there turns
  # go test ./features red (p2-harness-lists-undefined-wip-steps).
  #
  # Detection: ordinary doctor (no --policy-only, --manifest-only or
  # --credentials-only; those modes make no Landlock query and no runtime lookup)
  # calls T-64's checkoutsockets Detector once per invocation, afresh, with
  # cmd's native Landlock probe and docker Runtimes adapter bound to the
  # selection doctor already validated. cmd adds no Landlock seam, default or
  # environment switch. Detection runs only once the checkout and engine
  # inspections have succeeded, whatever the image outcome: a checkout, host or
  # engine refusal leaves no Landlock query, no member and no sockets problem,
  # as there is no cascaded image problem today. It runs inside doctor's
  # existing five-second engine inspection bound. A detection that bound
  # cancels reports the existing doctor.engine.timeout with exit 75. Doctor
  # persists nothing about the selection.
  #
  # Output: a confirmed selection adds /inspection/checkout_sockets, exactly
  # {"backend":"landlock"}, {"backend":"gvisor"} or {"backend":"neither"}.
  # Neither is a confirmed outcome, not a problem: ok and inspection.ready
  # stay true and the exit stays 0, because slice-43's consent to "late host
  # sockets reachable" needs doctor and approve to reach it. Plain output adds
  # the one line "checkout socket backend: landlock", "checkout socket backend:
  # gvisor" or "checkout socket backend: neither; late host sockets reachable"
  # after the image line, and still says "runtime isolation is not
  # established". Naming a backend is not enforcement: run does not use it
  # until slice-44, slice-46 and slice-47. docs/CLI.md documents the member,
  # the line and the rules below. docs/security.md says the report is not
  # enforcement.
  #
  # A typed detection failure omits the member, sets ok and inspection.ready
  # false and adds one problem, listed after any image problems; the process
  # exit is the highest problem exit, as ID-DOCTOR-19 settles. Each problem has
  # rule, exit, message, fix and the flat string subject field, fixed as follows:
  #   ABIQuery, ABIInvalid  doctor.sockets.kernel    exit 75  field landlock_abi
  #   RuntimeInspect        doctor.sockets.runtimes  exit 75  field runtimes
  #   RuntimeRegistration   doctor.sockets.runsc     exit 1   field runtimes.runsc
  # Exit 1 is used because a runsc that permits host sockets, or that cannot
  # be identified, is a host configuration doctor does not support. Messages
  # carry no errno text, daemon text or runtime arguments. Unsupported and
  # NoProbe cannot reach a correctly assembled doctor on Linux amd64: the
  # earlier doctor.host.unsupported refusal and cmd's assembly rule them out,
  # and unit tests cover them as doctor.sockets.kernel exit 75. Doctor's help
  # names backend detection under exits 1 and 75. In this slice the backend
  # enters no fingerprint, approval record, review or run decision; approve,
  # run and status are unchanged (slice-43 to slice-45).
  #
  # Given steps only pin the kernel's answer (pinKernelAnswer) and name the
  # fixture daemon's inventory (chooseRuntimeInventory), passing the texts and
  # names through unchanged. The rows run in the ordinary local tier on Linux
  # amd64. Red-first: commit the new step definitions in a test commit with the
  # tags still @wip, then lift @wip locally for the evidence run. Today each
  # row fails at its first Then, as the commit body names, because doctor
  # reads neither the kernel nor /info's Runtimes. The implementer may commit
  # freely in its workspace; the host makes the landing commits.
  @wip @slice-42 @ID-SOCKETS-04 @local-engine-fixture
  Scenario Outline: Doctor reports the checkout socket backend the host offers
    Given a doctor checkout
    And a local Engine API fixture
    And the kernel answers the Landlock version query with "<answer>"
    And the fixture daemon reports the runtime inventory "<inventory>"
    When itos-container runs with "doctor --json"
    Then its JSON value at "/inspection/checkout_sockets" is:
      """
      {"backend":"<backend>"}
      """
    And its JSON inspection is ready without asserting approval or runtime isolation
    And it exits with code 0
    And doctor asked the kernel for its Landlock ABI once and made no other Landlock call
    And the Engine API journal contains only ping, version, info and the requested image inspection
    And the checkout and host control state are unchanged

    # ABI 9 with a missing inventory shows Landlock settles without the lookup.
    Examples:
      | answer     | inventory         | backend  |
      | ABI 9      | runc only         | landlock |
      | ABI 12     | runc and runsc    | landlock |
      | ABI 9      | missing inventory | landlock |
      | ABI 8      | runc and runsc    | gvisor   |
      | ENOSYS     | runc and runsc    | gvisor   |
      | EOPNOTSUPP | runc and runsc    | gvisor   |
      | ABI 8      | runc only         | neither  |
      | ENOSYS     | runc only         | neither  |
      | EOPNOTSUPP | runc only         | neither  |

  @wip @slice-42 @ID-SOCKETS-05 @local-engine-fixture
  Scenario Outline: Plain doctor names the backend and says when late host sockets stay reachable
    Given a doctor checkout
    And a local Engine API fixture
    And the kernel answers the Landlock version query with "<answer>"
    And the fixture daemon reports the runtime inventory "<inventory>"
    When itos-container runs with "doctor"
    Then its standard output lists "<line>"
    And its standard output says "runtime isolation is not established"
    And it exits with code 0

    Examples:
      | answer | inventory      | line                                                          |
      | ABI 9  | runc only      | checkout socket backend: landlock                             |
      | ABI 8  | runc and runsc | checkout socket backend: gvisor                               |
      | ABI 8  | runc only      | checkout socket backend: neither; late host sockets reachable |

  # A failure never carries a backend. EPERM with runsc registered shows a
  # kernel failure stops detection before the fallback could choose gVisor.
  @wip @slice-42 @ID-SOCKETS-06 @local-engine-fixture
  Scenario Outline: Doctor refuses to name a backend it could not detect
    Given a doctor checkout
    And a local Engine API fixture
    And the kernel answers the Landlock version query with "<answer>"
    And the fixture daemon reports the runtime inventory "<inventory>"
    When itos-container runs with "doctor --json"
    Then its JSON output has a doctor problem "<rule>" with exit <exit>
    And the problem identifies the field "<field>"
    And its JSON inspection reports no checkout socket backend
    And doctor asked the kernel for its Landlock ABI once and made no other Landlock call
    And no engine mutation or image acquisition was attempted
    And the checkout and host control state are unchanged

    Examples:
      | answer | inventory                    | rule                    | exit | field          |
      | EPERM  | runc and runsc               | doctor.sockets.kernel   | 75   | landlock_abi   |
      | ABI 0  | runc and runsc               | doctor.sockets.kernel   | 75   | landlock_abi   |
      | ABI 8  | missing inventory            | doctor.sockets.runtimes | 75   | runtimes       |
      | ABI 8  | runsc permitting host sockets | doctor.sockets.runsc    | 1    | runtimes.runsc |
      | ENOSYS | unidentified runsc           | doctor.sockets.runsc    | 1    | runtimes.runsc |

  # The remaining scenarios are deferred continuous-enforcement specifications.
  # Preserve all five operations: stream/seqpacket connect, datagram connect,
  # unconnected sendto and sendmsg. Every phase/process/alias must report, with
  # host nonce receipts observed independently. A successful socket operation
  # or host receipt fails denial even without an echo. Missing paths, timeouts,
  # skipped operations or failed host/private controls are never denial proof.
  # Host controls surround each phase against the same live listeners; private
  # worker controls exchange all operations in /tmp. Require visible regular
  # checkout edits while running, literal argv, protected state preservation,
  # same full-ID worker observations and confirmed owned cleanup. No preceding
  # helper or different runtime is selected by these deferred specifications.

  # Deferred to p1-checkout-socket-kernel-enforcement (milestone 2); see
  # docs/plans/checkout-socket-enforcement.md. Keep @wip, outside slice-19.
  # At launch none of these socket paths exists. The Given prepares directories
  # and publication plans only. After the live barrier, the host binds listeners
  # both in an existing checkout directory and in a newly created nested tree.
  # Publish and run the positive controls before releasing the worker, without
  # a delay for a controller scanner. The same foreground process and a freshly
  # executed child each attempt the complete inventory; neither reports for the
  # other. The host listeners remain bound throughout both attempts and controls.
  @wip @ID-SOCKETS-02 @engine
  Scenario: Host sockets created after launch remain unreachable to the worker and its tools
    Given a CI socket checkout with the cached image and plans to publish the complete Unix socket operation inventory after launch
    When the host publishes those listeners after the product's live barrier and the same worker and its child attempt every socket
    Then every newly published checkout host socket operation is denied in both processes with no worker nonce received by a host listener
    And all checkout socket host controls succeed before and after each worker phase
    And all checkout socket worker-private controls succeed in tmpfs
    And the socket probe's regular checkout edit is visible to the host before exit and survives confirmed owned cleanup
    And the product preserves the checkout host listeners protected approval snapshots and unrelated resources

  # Deferred to p1-checkout-socket-kernel-enforcement (milestone 2); see
  # docs/plans/checkout-socket-enforcement.md. Keep @wip, outside slice-19.
  # Seed nonce-bearing regular files where socket paths will appear. The parent
  # and an already-running child each read them and make their own /tmp symlinks
  # to those paths, then wait at barriers. Both must resume in the same processes.
  # The host replaces each regular file by atomically renaming a live fixture
  # socket over it on the same filesystem; no checkout root or .git is replaced.
  # Record the old file and new socket identities, retaining each listener's
  # bound descriptor. Host controls address the NEW pathname, not the old bind
  # name. The worker and its child attempt every operation via absolute paths,
  # relative paths from the checkout directory and the already-created /tmp
  # symlinks. This prevents a path allowlist, startup inode cache or caller-path
  # string check from substituting for the socket boundary. File and mount-source
  # identity substitution outside this socket path remains slice-17's concern.
  @wip @ID-SOCKETS-03 @engine
  Scenario: Replacing ordinary checkout files with host sockets cannot bypass denial through aliases
    Given a CI socket checkout with the cached image and ordinary files at every planned host socket path
    When both probe processes read those files and create aliases then the host replaces them with live sockets before either process retries
    Then every replaced checkout host socket operation is denied through absolute relative and tmpfs symlink paths with no worker nonce received by a host listener
    And the socket reports contain both processes' original file reads and every replacement attempt
    And all checkout socket host controls succeed before and after each worker phase
    And all checkout socket worker-private controls succeed in tmpfs
    And the socket probe's regular checkout edit is visible to the host before exit and survives confirmed owned cleanup
    And the product preserves the checkout host listeners protected approval snapshots and unrelated resources
