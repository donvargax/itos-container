@phase-1
Feature: Bounded offline lifecycle, cancellation and loss
  Controller transitions finish within fixed budgets without limiting workload life.
  Cancellation preserves observed exits and accounts for operations already accepted.
  Lost observations retain ownership and uncertainty; controller death grants no access.

  # Slice-15 carries ID-LIFECYCLE-15 from slice-12, as the person's q-11 directs.
  # Its ID was allocated in e2261dd; retain its title and steps, still @wip.
  # This is a transfer, not a new scenario allocation or executed evidence.
  # Reviewed main 4b7c463fda8df0f829de90198a4a013b4f8b7813,
  # slice-12 ac37a3feed6c172a3c5a42cb652528dee8c6ceb0,
  # slice-14 aa66b4d73cec64e0cd4a6d2b22bb20bfa00dfdec,
  # slice-11 9b4d4a8ba67fe3a0e401c1cbd2a4a8578815657b and
  # slice-13 59e008b9d77acf42ade1d4abe5dcfc2dfb678073.
  # Slice-12 already adds this same lifecycle.feature on its branch. Integrate
  # exactly one copy of each ID in this file, preserving its other scenarios,
  # comments and live/wip tags. Place this block between LIFECYCLE-14 and 16;
  # replace only 15's owner tag and handoff. Do not replace the dependency's
  # full feature with this subset or duplicate this ID in another feature.
  # q-11 supersedes the source header's dependency-cycle warning and ownership:
  # slice-11 builds shared termination in internal/engine/session; slice-12
  # consumes it and depends on slice-11. Slice-15 waits for slice-12 and slice-14
  # (and thus slice-13's launch/cleanup). T-25 waits for slice-14 and slice-15.
  # Dependency specs are contracts to consume, not passing runtime evidence.
  #
  # Authorities: docs/charter.md, container-pilot.md AC-07/10/11/23,
  # docs/plans/offline-run-lifecycle.md's forced-controller-loss case,
  # docs/architecture.md (Slices), arc42 Recovery, A2/A3/A4 and q-11.
  # A3 leaves a surviving worker isolated until explicit stop. Do not introduce
  # a watchdog, automatic kill/restart, detached lifetime, exec or new command.
  # A dead controller cannot return a workload exit or run a cleanup handler.
  # Status discovers the lost-controller candidate; stop is an explicit action.
  # No proxy, guard, network service, credentials, host bridge, extra host mount,
  # persistent worker volume, image acquisition or platform claim is added.
  #
  # Retain slice-13/14's frozen offline boundary through controller loss: approved
  # cached image config ID, effective non-root host UID:GID, no supplementary host
  # groups, capabilities or elevation, no-new-privileges, read-only image root,
  # checkout-only private/nonrecursive host bind and container-owned /tmp tmpfs.
  # HOME/TMPDIR stay /tmp; pids stays 4096 with no CPU/memory quota claim. Private
  # namespaces, NetworkMode none, no published ports or extra network attachment,
  # no host environment/home/engine or SSH-agent sockets, no image hooks/init,
  # daemon workload logging, auto-remove or restart remain unchanged. Independent
  # inspection corroborates actual access attempts; settings alone are not proof.
  #
  # Reuse slice-11's public project-scoped status (no positional argument),
  # stop <session>, protected registrations/intents/snapshots and full-ID checks.
  # Do not select another ID grammar, record schema, lock or ownership algorithm.
  # Before stop, require a schema-1 successful status entry for the actual public
  # ID and original project/fingerprint: state/resource running, controller absent,
  # cleanup pending, workload_exit null, stop_candidate true and retry exactly
  # ["itos","container","stop",id]. Status is read-only. Execute that returned
  # suffix as argv against the built binary in the same disposable project;
  # never shell the retry or invoke an operator-installed extension.
  # Stop gets its fresh 30 s total and status 10 s; finite calls are bounded by
  # min(10 s, remaining total), with no deadline reset or workload lifetime limit.
  # Stop reuses slice-11's shared SIGTERM-once/10 s grace/revalidate/SIGKILL rule.
  # Require stop exit zero only after full-ID absence and durable completion.
  # The subsequent status has stopped/absent/complete and no candidate/retry.
  # Cleanup success alone cannot fill an unknown workload_exit. A fresh explicit
  # engine exit observation may fill null as slice-11 allows; retain its provenance
  # and the earlier unknown observation, never infer zero or 137 from a signal,
  # removal reply, not-found response or the controller's own SIGKILL status.
  #
  # Implementation placement remains run/status/stop vertical slices and shared
  # internal/engine/session/{domain,port,<named adapters>}, never slice imports.
  # Only cmd assembles adapters. Keep I/O behind ports and port/porttest fakes in
  # memory; any product fix needs the shared fake/adapter contracts and ordinary
  # changed-code proof as well as runtime evidence. Use OS as a value and retain
  # the exact allowlist. Reuse pinned Moby client v0.6.2/API v1.56.1, Go 1.27.3
  # standard-library probe/transport helpers and x/sys v0.48.0 where needed.
  # No new dependency or handwritten Engine client; preserve licenses/notices.
  #
  # Step handoff: all eight phrases below need definitions at the reviewed heads.
  # Reuse offline_runtime_{steps,fixture,engine,probe,assertions}_test.go and
  # session_fixture_test.go for independent inputs/observations. Slice-11's
  # recovery_native_{steps,fixture,assertions}_test.go supplies controller-kill,
  # recovery and durability machinery. Its heartbeat proves activity only; it
  # cannot replace boundary attempts. Reuse slice-14 OFFLINE-08..12's completed
  # probe clients and controls once built, not their assertions as Given steps.
  # Feature steps invoke the real binary and observe effects, without importing
  # product packages, adding a product test flag or executing checkout helpers.
  #
  # Given prepares only disposable checkout/state, independent canonical approval
  # seeds, trusted probe binaries, nonce inventories, host sentinels/listeners,
  # control topology and inert barriers. No product run/status/stop/approve occurs
  # in setup; registration, frozen publication and ownership must come from When.
  # "Working controls" describes the prepared fixtures, not asserted success.
  # Record files/modes/owners and unrelated resources after setup, accounting for
  # later harness challenge writes separately from worker edits/product effects.
  # Trusted Go probes are compiled into the disposable checkout but executed only
  # in the subject worker or slice-14's isolated control containers, never on host.
  # No real state folder, credentials, cloud metadata, public endpoint or DNS.
  #
  # When launches the probe via explicit argv through normal approval validation.
  # Collect a complete baseline round and independent live full-ID/start identity,
  # then SIGKILL only the recorded product controller and wait for confirmed death.
  # Keep the native daemon, recorder, observers and controls alive. Do not kill a
  # process group, signal the worker or turn off the network to simulate loss.
  # Only AFTER confirmed controller death generate and release a fresh nonce via
  # a test-only challenge file in the already mounted checkout. The same worker
  # reads it and atomically publishes its round report there, independent of the
  # now-dead attach/stdin connection. The probe tolerates stdin EOF/output loss
  # and stays live until explicit stop. No product host RPC or new mount is needed.
  # Require the exact released nonce and complete route/operation inventory;
  # cached pre-kill output, a heartbeat, an empty report or a replacement process
  # cannot satisfy a post-loss round. Correlate reports with the original native
  # full ID/start identity, unchanged frozen fingerprint and live observation.
  # After that round, invoke fresh status, collect another released round before
  # executing its stop retry, and observe listener receipts until confirmed removal.
  # Record the final status, ownership completion and preserved edits before any
  # harness cleanup. No completed report may be required after worker termination.
  #
  # Each round reuses slice-14's SAME executable/client and complete controls:
  # IPv4/IPv6 TCP/UDP literal, gateway, private and synthetic-metadata routes;
  # system DNS, Docker 127.0.0.11, alternate loopback, direct TCP/UDP DNS and
  # verified-TLS HTTPS DNS. Controls must return the expected per-attempt nonce
  # before and after each denial; worker-local loopback must also work. Never
  # change subject permissions/resolver settings or count certificate failure,
  # NXDOMAIN, send() success, timeout or silence alone as network enforcement.
  # Require attempted-operation reports and zero matching listener/query receipts.
  # Missing IPv6/resolver/route controls fail, never skip or narrow the inventory.
  #
  # Repeat identity/elevation, root-write, host-file/socket and namespace probes
  # with slice-14's positive controls. Host-only sentinels, disposable home/SSH
  # socket and protected approval/snapshot/intent/registration paths must exist
  # and be readable/connectable to the trusted fixture. Worker-local file/socket,
  # checkout and tmpfs operations must succeed. Include the engine-socket connect
  # attempt without sending an API request on unexpected success. Use synthetic
  # data and report access results, never host contents or credentials. Namespace
  # and host-process comparisons retain independent device/inode/start identities.
  # Capture generated protected paths after run publishes them and pass those
  # exact paths as challenge data; denying a nonexistent stand-in is insufficient.
  # Preserve unrelated native resources, original approvals/snapshots and edits;
  # only owned recovery progress and ephemeral worker removal may change.
  #
  # All engine setup, actions and cleanup run only in features/README.md's guarded
  # Linux amd64 native-Docker CI tier, with T-20's independently inspected cached
  # image. Agents receive no socket. Unavailable inputs/controls fail, no fallback.
  # Race every barrier against subprocess exit and finite harness deadlines;
  # early refusals, missing reports and timeouts become data for Then assertions.
  # A harness timeout/kill is not product cleanup or an isolated surviving worker.
  # Retain all attempted API calls, including recorder refusals; a recorder cannot
  # supply the product's enforcement. Use bounded identity-checked harness cleanup
  # after observations even on failure, never as evidence of stop success.
  # Reuse recovery's actual file-fsync/publication/parent-fsync observations and
  # shared local durability contract; a visible record or DELETE alone is not it.
  #
  # Evidence handoff: on reviewed main and dependency heads the first Then is the
  # expected failure: public status/stop and registration are absent. The runner
  # already configures an offline worker; no failing isolation claim is made from
  # source inspection. All phrases are still undefined here, so this draft is
  # neither assertion-level red nor passing runtime evidence. Recheck once the
  # prerequisites merge and commit complete steps first with @wip retained.
  # For missing behavior the coordinator obtains red_scenarios CI evidence for
  # this ID at its actual assertion, then green CI without red input after the fix.
  # Preserve the source spec's standing coordinator regression decision for this
  # transferred case: if the COMPLETE case already passes for product reasons,
  # cite the supplying slices and native evidence above its tag, remove @wip and
  # continue. No fabricated red or further regression approval is required.
  # RECOVERY-22 supplies discovery/stop integration; OFFLINE-08..12 supply boundary
  # controls. Neither alone proves fresh denied access after controller death.
  # Undefined steps, broken setup/controls, missing barriers or inspection flags
  # alone never count as the required red/green evidence or a support claim.
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
