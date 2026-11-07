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
  # Fresh red handoff: prior connectivity reds were accepted for 01/03 in run
  # 42312676619 at 1662753 and for 02 in run 42323471709 at 69b02a8. They prove
  # the gap, but q-30 requires ONE fresh CI red run of reworded ID-SOCKETS-01
  # before implementing startup refusal. Keep @wip @engine and use README's
  # guarded red_scenarios route for @ID-SOCKETS-01 alone (all outline rows).
  # Current product is expected to create/run the worker instead of refusing.
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
  @wip @slice-19 @ID-SOCKETS-01 @engine
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
