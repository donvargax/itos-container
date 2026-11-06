@phase-1
Feature: Host Unix sockets in the checkout stay outside the worker boundary
  An approved offline worker may edit its checkout without using a pathname
  Unix socket there to contact a host process. This boundary holds from the
  first workload operation and when host processes add or replace sockets
  during the run. Worker-private Unix IPC in temporary storage still works.

  # Slice-19 authorities: docs/charter.md, docs/plans/container-pilot.md
  # (Goal and limits, Runtime and network boundary, AC-07/10/25),
  # docs/plans/offline-run-lifecycle.md (Foreground lifecycle),
  # docs/architecture.md (Slices), and arc42's Foreground run and its failure
  # paths. This closes the checkout exception to OFFLINE-10: its host/SSH
  # fixture listeners are outside the mount, so that case cannot prove this.
  #
  # Decisions for this item:
  # - Deny communication with host pathname AF_UNIX sockets, including stream,
  #   sequenced-packet and datagram sockets. Datagram delivery without connect
  #   is also forbidden. NetworkMode none, private IPC namespaces and socket
  #   mode bits alone do not establish this boundary.
  # - Preserve the existing read/write checkout bind and immediate file-edit
  #   visibility. Do not introduce a copied checkout or synchronization protocol.
  #   A socket's presence must not prevent these otherwise valid workloads from
  #   running on a supported host. A startup scan or polling/deleting/chmodding
  #   host sockets is not enforcement; denial must hold continuously, including
  #   new paths and replacements. The product leaves host listeners intact.
  # - Retain successful worker-private pathname Unix IPC in /tmp, as OFFLINE-10
  #   requires. A blanket AF_UNIX ban cannot satisfy these scenarios. No promise
  #   is added that a worker may offer a service to the host through checkout.
  # - Apply denial to the foreground process and its children, irrespective of
  #   absolute, relative or symlink path spelling. No command wrapper, cooperative
  #   client or environment convention may be the security boundary.
  # - Keep the approved image, host non-root UID:GID, sole checkout host mount,
  #   read-only root, tmpfs HOME/TMPDIR, dropped capabilities, no-new-privileges,
  #   offline network and owned cleanup. No extra mount, host bridge, privilege,
  #   image acquisition, network service or credential is authorized here.
  #   An unavailable or unverifiable enforcement boundary must refuse startup
  #   under the existing isolation-refusal contract, never fall back to today's
  #   bind alone. Such a refusal is not a pass for the supported-host cases below.
  #
  # Implementation stays in run's vertical slice and shared session engine,
  # behind ports; only cmd assembles adapters. Reuse pinned Moby client v0.6.2 /
  # API v1.56.1, Go 1.27.3 and x/sys v0.48.0 for existing engine/native APIs.
  # No new dependency is selected. Domain and porttest fakes do no I/O; shared
  # contracts and adapter fixtures accompany any new production behavior.
  # Do not weaken existing verification or rewrite live scenarios to admit an
  # unsafe configuration. If implementation changes arc42's design, record the
  # overriding decision and update that section in the implementation PR.
  #
  # Step handoff: ALL step phrases below are NEW, including the common Then
  # phrases. Add definitions and a trusted socket probe; no steps are supplied
  # by this specification. Reuse offline_runtime_fixture_test.go's guarded
  # binary execution, session_fixture_test.go's independent approval seeds,
  # offline_runtime_engine_test.go's native recorder, and the observations in
  # offline_isolation_{fixture,controls,assertions}_test.go. OFFLINE-10's
  # connect-only unixAttempt and outside-mount listeners are insufficient here.
  # Invoke the real binary as run -- <probe argv>; feature steps import no
  # product packages and never fabricate a product result or invoke approve.
  #
  # Givens only prepare disposable checkout/control state, independently encoded
  # matching approval, trusted probe inputs and fixture-owned listeners or future
  # publication plans. They never run the product or assert its behavior. Use
  # features/README.md's guarded Linux amd64 CI engine tier and independently
  # inspected cached image inputs; setup and cleanup also remain in that tier.
  # No real tmux, SSH agent, engine endpoint, user service or credential is a
  # socket target. Listeners only record and echo synthetic nonces, never execute
  # messages or forward traffic. Use short paths within sockaddr_un limits and
  # the same effective host UID:GID as the worker, with traversable parents and
  # owner-readable/writable sockets. Do not manufacture denial with permissions.
  #
  # Each scenario uses this complete operation inventory, with separate sockets
  # and per-operation/per-phase nonces; table rows are not optional examples:
  #   SOCK_STREAM     connect, then nonce exchange if unexpectedly connected
  #   SOCK_SEQPACKET  connect, then nonce exchange if unexpectedly connected
  #   SOCK_DGRAM      connect, then send if unexpectedly connected
  #   SOCK_DGRAM      unconnected sendto to the pathname
  #   SOCK_DGRAM      unconnected sendmsg with a pathname destination
  # Use direct socket operations in the trusted probe, not missing utilities.
  # Any successful connect or addressed send is a failure even without a reply;
  # any worker-nonce listener receipt is a failure even if the client reports an
  # error. Datagram controls need listener receipts, not send success alone.
  # Do not require a particular denial errno: a complete denied operation with
  # working controls is the assertion. A timeout or missing path alone is not.
  #
  # The host control uses the same operation code and corresponding host path,
  # while the worker uses /workspace paths. Keep the trusted host control code
  # outside the disposable consumer checkout; execute no consumer code on host.
  # Controls perform a nonce exchange/receipt before and after every worker phase
  # against the SAME live listener. Socket inode/owner/mode and listener liveness
  # observations distinguish denial from deletion, replacement or a broken server.
  # The worker also exchanges nonces using every listed operation with its own
  # listeners in /tmp, proving that the client and private IPC still work.
  #
  # Whens collect observations, including all controls, even after an unexpected
  # successful connection. Reports enumerate every expected operation, target,
  # phase, process and syscall outcome. Denial steps first require this complete
  # execution and working controls; report fixture/control failures separately,
  # never as product red evidence. Missing reports or an empty receipt list do
  # not pass. Record host and worker observations independently of the product.
  #
  # Use nonce-bearing stdin/stdout barriers, raced against product exit and a
  # finite harness deadline; no sleep-based publication ordering. Early refusal,
  # unreached barrier or missing child report remains data for Then, not a hung
  # setup or a skipped scenario. Host publications happen only after independent
  # full-ID inspection confirms the same worker is running. No restart, reapproval
  # or second run may hide a late socket behind a new startup scan.
  #
  # Before releasing each probe to exit, independently read its nonce-bearing
  # regular checkout edit from the host. It must be visible while the worker is
  # still running, owned by the host UID:GID, and survive exit. Product success
  # requires observed exit zero and full-ID absence before harness cleanup, plus
  # the protected completion record. Preserve approval/snapshot bytes, listener
  # inodes/modes/owners and unrelated files/resources. Record planned fixture
  # publications separately from product writes. Always perform bounded,
  # identity-checked harness cleanup AFTER observations; it never supplies proof
  # of product cleanup. Never prune or delete shared images or operator state.
  #
  # Evidence handoff: at specification base f2a8c85702df50a749a45e9ad87b52b99eac491e,
  # session/docker/run.go binds the whole checkout and verifies no socket-specific
  # boundary. Each denial Then below is EXPECTED to fail because the worker can
  # reach the live host socket inode, including after host publication/replacement.
  # This is static reasoning, not executed red evidence or a host-support claim.
  # Commit complete steps red first with @wip @engine retained; the coordinator
  # dispatches ID-SOCKETS-01/02/03 through README's guarded red_scenarios route.
  # Require actual assertion-level red before implementation, then remove @wip
  # and obtain ordinary green engine CI at the pushed head. Undefined steps,
  # fixture failures and selection skips supply no evidence. Agents use no engine
  # and run no local mutation proof; ordinary code proof remains engine-disabled.
  #
  # Coordinator decision, 2026-11-06: run 42308513561 at dbf75d4 is NOT accepted
  # red evidence. All three cases exited 75 with run.execution-unavailable before
  # the probe barrier; no socket operation ran. Diagnose startup in the harness,
  # retaining controller stderr, recorder refusals, engine response messages and
  # observed engine state in incomplete-workload failures. Fix fixture blockers,
  # then stop for another guarded red run of all three IDs. Valid red requires the
  # probe inside the worker and a host listener nonce receipt or a successful
  # socket operation, with the controls above. Do not implement denial first.
  # If product refusal of checkouts containing sockets prevents startup instead,
  # report that evidence for the coordinator to decide the specification change.

  # Prepare each operation's listener at both a short root path and a nested
  # checkout path before invoking run. The probe's earliest operations attempt
  # every host target before its first blocking handshake; runtime initialization
  # is not counted as an application operation. Repeat the full inventory after
  # a running-state barrier. An image entrypoint/helper must not precede it.
  @wip @slice-19 @ID-SOCKETS-01 @engine
  Scenario: Host sockets already in the checkout are denied from the first workload operation
    Given a CI socket checkout with the cached image and live host listeners for the complete Unix socket operation inventory
    When the product runs the checkout socket probe before its first handshake and again while running
    Then every planned checkout host socket operation is denied in both phases with no worker nonce received by a host listener
    And all checkout socket host controls succeed before and after each worker phase
    And all checkout socket worker-private controls succeed in tmpfs
    And the socket probe's regular checkout edit is visible to the host before exit and survives confirmed owned cleanup
    And the product preserves the checkout host listeners protected approval snapshots and unrelated resources

  # At launch none of these socket paths exists. The Given prepares directories
  # and publication plans only. After the live barrier, the host binds listeners
  # both in an existing checkout directory and in a newly created nested tree.
  # Publish and run the positive controls before releasing the worker, without
  # a delay for a controller scanner. The same foreground process and a freshly
  # executed child each attempt the complete inventory; neither reports for the
  # other. The host listeners remain bound throughout both attempts and controls.
  @wip @slice-19 @ID-SOCKETS-02 @engine
  Scenario: Host sockets created after launch remain unreachable to the worker and its tools
    Given a CI socket checkout with the cached image and plans to publish the complete Unix socket operation inventory after launch
    When the host publishes those listeners after the product's live barrier and the same worker and its child attempt every socket
    Then every newly published checkout host socket operation is denied in both processes with no worker nonce received by a host listener
    And all checkout socket host controls succeed before and after each worker phase
    And all checkout socket worker-private controls succeed in tmpfs
    And the socket probe's regular checkout edit is visible to the host before exit and survives confirmed owned cleanup
    And the product preserves the checkout host listeners protected approval snapshots and unrelated resources

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
  @wip @slice-19 @ID-SOCKETS-03 @engine
  Scenario: Replacing ordinary checkout files with host sockets cannot bypass denial through aliases
    Given a CI socket checkout with the cached image and ordinary files at every planned host socket path
    When both probe processes read those files and create aliases then the host replaces them with live sockets before either process retries
    Then every replaced checkout host socket operation is denied through absolute relative and tmpfs symlink paths with no worker nonce received by a host listener
    And the socket reports contain both processes' original file reads and every replacement attempt
    And all checkout socket host controls succeed before and after each worker phase
    And all checkout socket worker-private controls succeed in tmpfs
    And the socket probe's regular checkout edit is visible to the host before exit and survives confirmed owned cleanup
    And the product preserves the checkout host listeners protected approval snapshots and unrelated resources
