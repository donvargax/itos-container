@phase-1
Feature: Hardened offline foreground run
  An approved run executes explicit argv in one owned offline worker.
  Isolation holds before the first workload instruction; foreground exit ends
  the worker and its temporary storage while preserving checkout edits.

  # Slice-14 carries ID-OFFLINE-08/09/10/11/12/14/16 from slice-10, as q-7 directs.
  # These IDs were allocated in 7920219 and retagged, still @wip, in dad6d1f.
  # Preserve their IDs, scenario text and examples; no new ID is needed here.
  # Reviewed main f13be16397aafad4e7d4fb18e52916fed3cc3239,
  # slice-10 d726afdd003d78559e4684f3ed15177be3aea9e1, and
  # slice-13 b9fa07d78e213d08f94bff3840e46e3c9536ac6b.
  # Both prerequisite branches already add this same OFFLINE feature. Integrate
  # one copy of each ID in this file, retaining each owner's comments and the
  # dependency branch's live/wip tags for siblings. Do not duplicate these IDs
  # in another feature or replace the full dependency feature with this subset.
  # Keep isolation cases between argv/streams and the explicit-shell case, and
  # teardown cases beside the exit/cleanup block when combining the specs.
  #
  # Authorities: docs/charter.md, docs/plans/offline-run-lifecycle.md (AC-07/08,
  # offline AC-10/11, AC-22/23/25/37), docs/architecture.md (Slices), arc42's
  # Foreground run and the person's A2/A3/A4, plus q-7's ownership split.
  # Consume slice-13's ordered launch/frozen settings/cleanup contract and
  # slice-10's decision 0008; this spec adds no new security mechanism.
  # Slice-10 owns OFFLINE-02/03/06/07/13 and its local foreground regressions;
  # slice-13 owns OFFLINE-01/04/05/15; slice-11 owns public status/stop and retry
  # guidance; slice-12 owns cancellation, controller loss and aggregate budgets.
  # Read recovery.feature at slice-11 9bb080f and lifecycle.feature at slice-12
  # d8e5823 when integrating. Their commands are not prerequisites to these
  # seven cases. LIFECYCLE-15 later reuses these independent boundary controls.
  # A3 leaves a surviving worker isolated until explicit stop, not a watchdog.
  # Calls/startup/teardown retain 10/120/30 s limits, never a workload time limit.
  #
  # Settled runtime boundary, required from the first executable instruction:
  # - Linux amd64/native Docker only; immutable approved cached image config ID;
  #   effective non-root host UID:GID, no forwarded supplementary groups or chown.
  #   Override image user, entrypoint, command and working directory. Disable
  #   image healthchecks and daemon init, so no helper precedes the explicit argv.
  # - Read-only image root; sole host source is the revalidated checkout mounted
  #   read/write at /workspace, private/nonrecursive, without nested host mounts.
  #   workspaceFolder is an existing directory beneath that mount, not a source.
  # - One requested tmpfs at /tmp, rw,nosuid,nodev,mode=1777; HOME and TMPDIR are
  #   /tmp. A2 excludes persistent worker volumes/caches/profiles. Engine-managed
  #   proc/sys/dev, /dev/shm and generated hosts/hostname/resolver files are runtime
  #   facilities, not extra configurable host-source grants. No tmpfs quota claim.
  # - CapDrop ALL, no-new-privileges, private PID/IPC/UTS/mount/network namespaces,
  #   pids 4096, no privilege/device/capability additions or disabled confinement.
  #   No memory/CPU quota or user-namespace support claim.
  # - NetworkMode none, inspected before start; no extra network attachment,
  #   published port, proxy, guard or product-created network/service container.
  # - Preserve approved image environment defaults with literal containerEnv
  #   overlays, then controller-owned HOME/TMPDIR. Forward no host environment,
  #   credentials, home, engine socket or SSH agent. No implicit shell or hooks.
  # - Attach separate non-TTY streams before start; no daemon workload logging,
  #   auto-remove, restart, detached mode or exec command. No image acquisition.
  #
  # Extend internal/slices/run/{command,domain,port,<named adapters>} and shared
  # internal/engine/session/{domain,port,<named adapters>} only as the evidence
  # requires. Only cmd assembles. Reuse approval/readiness/frozen storage and
  # slice-9's durable ownership, keeping its unstarted-only capability intact.
  # Domains and port/porttest fakes do no I/O; shared contracts run unchanged
  # against fakes and adapters. Native probes cannot replace local contract,
  # adapter, changed-line coverage or CI mutation evidence for any product fix.
  # Reuse pinned Moby client v0.6.2/api v1.56.1 for engine operations and stream
  # framing, Go 1.27.3 standard-library probe/transport helpers and x/sys v0.48.0
  # for native adapter calls. No new library/service, handwritten engine client,
  # shell engine invocation or allowlist change. Preserve notices and OS-as-value
  # logic. Test support and trusted probes never enter the production binary.
  #
  # Fixture and step handoff: ALL phrases below need new step definitions on
  # both reviewed main and slice-10. Reuse slice-10's offline_runtime_steps_test.go,
  # offline_runtime_fixture_test.go, offline_runtime_engine_test.go,
  # offline_runtime_probe_test.go and offline_runtime_assertions_test.go mechanisms,
  # plus session_fixture_test.go's independent records and approval seeds.
  # Those steps currently bind OFFLINE-06/07/13, not these seven IDs. Extend their
  # observations; do not call their product assertions from a Given. Feature
  # steps invoke the real binary and read effects, never import product packages.
  #
  # Every Given prepares only disposable checkout/state, independently encoded
  # approval data, trusted probe files, controls, sentinels and fault settings.
  # No approve command, product run or production canonicalizer manufactures
  # setup or expected data. No operator state or real credentials. Record files,
  # owners/modes, resources and image references after fixture preparation; keep
  # subsequent harness writes separate from product effects. Compile reviewed
  # probe source into the disposable checkout; execute it only in the subject
  # worker or the expressly described trusted control container, never on host.
  # Use synthetic markers and report their presence/result, not arbitrary host
  # file contents, environments or engine response bodies.
  #
  # All setup, action and cleanup needing an engine stays in features/README.md's
  # guarded CI-only Linux amd64 tier. Independently inspect T-20's supplied cached
  # digest/config ID/UID/GID; refuse absent or incompatible inputs, never acquire
  # or substitute an image. This is not permission for agents to access a socket.
  # Extend the existing test-only native request recorder, including attach/wait,
  # without weakening doctor's or slice-9's restricted recorders. Record refused
  # requests as failures; a recorder refusal cannot count as product isolation.
  # Read native observations independently of product responses and fixture
  # expectations. The product still takes its normal validation/approval path.
  #
  # Whens retain early refusals, missing reports and unreached barriers as data
  # for Then. Race every barrier against subprocess exit and a finite fixture
  # deadline. A never-started probe must reach the named behaviour assertion;
  # undefined steps, broken controls and setup errors are not red evidence.
  # Each denial needs a complete, nonempty attempted-operation report AND its
  # working positive control. Use unique per-attempt nonces, exact route/phase
  # inventories and listener receipts. Exit status, silence, ENOENT, timeout or
  # an empty receipt list alone cannot establish the requested isolation.
  # Controls run before and after the corresponding attempt with the same
  # executable/client and target operation. Never loosen the subject's settings.
  # Missing IPv6, setuid, nested-mount, resolver or process controls fails the
  # scenario; no skipped route, weaker fallback or manufactured denial result.
  #
  # Observe primary exit, full-ID absence, protected completion record and
  # unrelated resources before harness teardown. The absence record has the
  # original project, fingerprint, image and concrete ID, not just an empty list.
  # Slice-9/13 local contracts prove write/fsync/rename/parent-fsync ordering;
  # runtime re-reading a complete record does not independently prove fsync.
  # Never infer removal from DELETE success, a name lookup or a transport error.
  # Bounded identity-checked harness cleanup runs even after failed assertions,
  # but only AFTER product observations and never supplies product evidence.
  # No prune, shared-image removal, foreign-resource deletion or host-file cleanup
  # by the product. Preserve existing approval/snapshot bytes and checkout edits.
  #
  # Evidence handoff: this commit specifies only, with no executed red/green.
  # On reviewed main startup-unavailable would fail 08's process/child identity,
  # 09's two root/write reports, 10's local controls plus denied host access,
  # 11's worker route reports, 12's worker resolver reports, 14's observed primary
  # exit plus cleanup, and 16's observed child exit plus separate cleanup failure.
  # Even 14's status-3 row requires an observed workload exit; wrapper 3 cannot pass.
  # Slice-10 already implements much of this boundary and exit precedence. Do not
  # claim these cases fail on that branch without running their complete steps.
  # After dependencies land, commit complete steps first, retaining @wip @engine,
  # then stop for the coordinator's red_scenarios dispatch of all seven IDs/rows.
  # Each failure must be at its behaviour assertion. If a case already passes,
  # report it for coordinator disposition; q-6's accepted slice-10 regressions do
  # not waive this slice's red-first rule. Never weaken a case or add a forced
  # failure to manufacture red. Implementation/activation needs the resulting
  # evidence or explicit disposition, then green CI at the actual pushed head
  # without red input. This draft supplies no runtime/support or mutation claim.

  # Effective identity comes from the real host process, not fixture environment.
  # Given records its UID/GID and groups, installs a compiled report-only root-owned
  # setuid helper in the checkout, and prepares misleading UID/GID/USER inputs.
  # Record a host supplementary group distinct from the primary GID so accidental
  # inheritance is observable. Group lists may include the primary GID but no
  # other host group. No product chown may be hidden among fixture preparations.
  # When launches the controller with those misleading environment strings but
  # unchanged effective IDs/groups. The worker first reads process/child IDs, all
  # capability sets, NoNewPrivs and actual cgroup pids.max, then attempts setuid(0),
  # the setuid helper and a privileged mount. Observe actual syscall outcomes,
  # not installed-tool names or flags.
  # A separate isolated control executes the SAME setuid file without NNP and
  # must elevate its effective UID before and after the subject attempt. It only
  # reports IDs, uses the same cached image, has no network or extra host mounts,
  # and never executes on host. A nosuid/nonexecuting helper is a broken control.
  # Missing sudo is not proof; no 4096-process exhaustion or quota assertion.
  @wip @slice-14 @ID-OFFLINE-08 @engine
  Scenario: The first workload process has the host non-root identity without elevation
    Given a CI offline checkout with the cached image and identity capability and setuid probes
    And misleading host identity environment and an unrelated supplementary group are recorded as fixture inputs
    When the product runs the offline identity probe as its first executable
    Then the process and its child have exactly the effective host UID and GID without host supplementary groups
    And the runtime has no effective permitted inheritable ambient or bounding capabilities and NoNewPrivs is one
    And root identity and privileged mount attempts fail and the setuid probe cannot elevate
    And the process reads an effective pids limit of 4096 without any claimed memory or CPU quota
    And the product confirms owned cleanup without host chown or unrelated file changes

  # Given prepares a writable checkout marker and an empty mountpoint below it,
  # then a read-only fixture bind there backed by an outside synthetic sentinel.
  # Record the native mount and its readable nonce before and after both runs;
  # absence of a working nested mount cannot prove nonrecursive isolation.
  # When invokes the product twice with independent session nonces. Each probe
  # observes kernel read-only root mount flags AND attempts an image write (for
  # example creating /slice-14-root-marker). A permission error without the
  # read-only mount observation is insufficient. Successful checkout and tmpfs
  # writes provide controls.
  # HOME/TMPDIR remain /tmp; use different paths within it to distinguish the
  # home and temp markers. Verify tmpfs type/options and owner identity, not only
  # environment strings. Run two receives run one's names and must attempt reads;
  # it sees the exact persisted checkout nonce and no prior temporary nonce.
  # The underlying empty directory, not the host mount's sentinel, is visible.
  # Audit both full IDs, mounts and volume inventory before unmounting the fixture.
  # Harness unmount/removal cannot establish ephemeral product teardown.
  @wip @slice-14 @ID-OFFLINE-09 @engine
  Scenario: Checkout edits persist while writable home and temporary files are ephemeral
    Given a CI offline checkout with the cached image and a filesystem probe
    When the product runs the offline filesystem probe twice with distinct session nonces
    Then both processes observe a read-only image root and fail to modify image files
    And both processes can write the checkout and their tmpfs HOME and TMPDIR as the host UID GID
    And the second process sees the first checkout edit but none of its temporary or home files
    And native inspection and mount observations show only the approved checkout host source and container-owned temporary storage
    And neither worker sees the outside marker from the fixture's nested host mount
    And both exact worker IDs and their ephemeral storage are absent after product cleanup with no created persistent volume

  # Given seeds known-readable synthetic host-only files outside checkout, a
  # disposable host home, protected approval/ownership paths, harmless Unix
  # listeners and a live host process sentinel with a nonce-bearing identity.
  # Host controls read/connect before and after; worker-local files and a Unix
  # listener exercise the same probe operations in tmpfs. Pass host paths as data,
  # never mounts or forwarded environment. Use a harmless fixture socket for the
  # SSH-agent case, never a real agent or credentials; any engine-socket connect
  # success fails without sending a request. Refuse/report unexpected sensitive
  # access without logging its bytes. Symlinks placed in checkout may point at
  # outside sentinel paths but must not expose their host targets in the worker.
  # Compare /proc namespace device/inode identities against independent host
  # observations, and identify the process by nonce/start identity, not a PID
  # alone that could coincide inside the worker. No host-namespace join by a probe.
  # Actual attempts and successful local controls are required for every denial.
  @wip @slice-14 @ID-OFFLINE-10 @engine
  Scenario: Host files sockets and namespaces are inaccessible to a running worker
    Given a CI offline checkout with the cached image and outside host sentinels and socket listeners
    When the product runs the offline host-access probe
    Then the probe completes its worker-local file and socket controls but cannot read host sentinels or protected approval and ownership records
    And no host home engine socket SSH agent or fixture host socket is reachable from the worker
    And PID IPC UTS mount and network namespace identities differ from the host without access to the host process sentinel
    And host sentinels and unrelated resources are unchanged after confirmed owned cleanup

  # Given creates trusted, fixture-owned dual-stack listeners and an isolated
  # control topology with no public egress, using only the same cached image.
  # Synthetic private/gateway/link-local metadata addresses belong to that
  # topology; no real cloud metadata, public endpoint or external DNS is queried.
  # Control containers/networks are harness resources, never product resources.
  # Record exact addresses/ports and require both TCP and UDP over IPv4 and IPv6,
  # including literal destination, host gateway, private destination and synthetic
  # metadata paths. No proxy variables or client cooperation are needed for denial.
  # TCP success means a complete nonce exchange; UDP needs a nonce response AND
  # listener receipt. send() success is not delivery, timeout alone is not denial.
  # Each phase attempts all routes before emitting its complete framed report.
  # First-instruction means the explicit probe's earliest application operations,
  # before a blocking handshake, not before language-runtime initialization. No
  # image entrypoint/init/setup precedes it. Then use a bounded live barrier to
  # inspect the running worker and trigger phase two. Early product exit is data.
  # Mirror host/control loopback destinations in their own namespaces; at the
  # same port the worker can exchange its own nonce but never the host's nonce.
  # No control is allowed to attach or repair a subject worker's network.
  @wip @slice-14 @ID-OFFLINE-11 @engine
  Scenario: No external network path exists from the first workload instruction
    Given a CI offline checkout with the cached image and controlled IPv4 IPv6 TCP UDP gateway private and synthetic metadata listeners
    When trusted controls and the product's first-instruction and running network probes attempt every fixture route
    Then every trusted route control succeeds before and after its worker attempt
    And every first-instruction and running worker attempt is denied with no matching listener receipt
    And the worker can reach its own loopback control but no host loopback listener at the same port
    And independent inspection observes NetworkMode none before start and while running with no port publication or extra network attachment
    And the product confirms owned cleanup while preserving all unrelated network resources

  # Given seeds unique fixture-only DNS names, nonce-bearing expected answers,
  # controlled resolvers and a disposable TLS trust root for the HTTPS control.
  # Exercise system lookup without replacing its resolver, explicit UDP/TCP DNS
  # to IPv4/IPv6 resolvers, Docker 127.0.0.11, an alternate 127/8 address and HTTPS
  # DNS. A/AAAA names and per-phase nonces exclude caches and hosts-file success.
  # Capture effective system resolver targets; the isolated control topology
  # must serve those target addresses without public forwarding. Never rewrite
  # the subject's resolver file, daemon DNS settings or frozen permissions to
  # make it testable. Docker's embedded DNS control uses the fixture bridge and
  # a fixture-owned name; other loopback resolvers live in the control namespace.
  # HTTPS uses a real TLS request with the fixture CA and expected nonce answer;
  # certificate failure is not network denial. Trust only this synthetic CA in
  # the probe, not the host store; pin the controlled endpoint to avoid relying
  # on the very DNS path under test. Do not disable TLS verification.
  # Resolver controls must return the answer before and after the corresponding
  # worker attempts; NXDOMAIN, timeout or a broken server is not a control pass.
  # Use the same earliest-operation/live-phase protocol as 11 and require the
  # complete query/phase report plus zero matching resolver receipts. Localhost
  # and hosts-file resolution alone is allowed and is not external DNS traffic.
  @wip @slice-14 @ID-OFFLINE-12 @engine
  Scenario: Offline execution has no external DNS or alternate resolver path
    Given a CI offline checkout with the cached image and controlled system Docker loopback TCP UDP and HTTPS DNS resolvers
    When trusted controls and the product's first-instruction and running DNS probes query unique fixture names
    Then each resolver control returns its expected nonce-bearing answer before and after the worker attempt
    And the worker receives no external answer through system DNS 127.0.0.11 alternate loopback direct TCP UDP or HTTPS DNS
    And no controlled resolver receives a matching worker query at either execution phase
    And the DNS probes actually execute and the product confirms owned cleanup

  # Given prepares the primary/child probe and an unrelated same-image control
  # worker with a different ownership tuple. When runs the product: the primary
  # writes an edit, spawns a long-lived child and waits for its nonce heartbeat,
  # then emits the trailing frame and exits with the example status. The child
  # has no voluntary exit before the fixture deadline. Independent observation
  # ties its live process/start identity to this full worker ID before primary
  # exit; absence of a child at setup or merely stale heartbeat data cannot pass.
  # After the actual primary exit require descendant disappearance and full-ID
  # absence before the wrapper result. Normal PID-namespace teardown may kill
  # descendants at primary exit; the spec does not require them to outlive it.
  # Do not confuse a coincidentally reused host PID or a killed harness process
  # with product teardown. Require the control worker still live with new nonce
  # heartbeats before harness cleanup. Check original approval/snapshot bytes,
  # ownership completion and persisted edit separately from the exit number.
  # Slice-13's local contract owns durability ordering; a DELETE alone is not it.
  @wip @slice-14 @ID-OFFLINE-14 @engine
  Scenario Outline: Primary exit determines the result and ends the whole owned worker
    Given a CI offline checkout with the cached image and a primary probe exiting <status> with a surviving child
    When the product runs that offline foreground probe
    Then the product returns exactly <status> after the observed primary exit and confirmed durable owned cleanup
    And the trailing workload frame is delivered and the descendant no longer runs
    And native Docker confirms the exact worker ID absent while the unrelated same-image worker remains
    And checkout edits approvals snapshots and unrelated resources remain intact

    Examples:
      | status |
      | 0      |
      | 7      |
      | 1      |
      | 2      |
      | 3      |
      | 70     |
      | 75     |

  # Given prepares a removal-only fault rule, not an observed workload result.
  # When forwards real create/attach/start/wait/inspect to native Docker, captures
  # the actual primary exit, then rejects EVERY product DELETE for that owned
  # full ID with synthetic HTTP 503/body marker. Keep inspection working and
  # hold the fault until all product observations are complete. No manufactured
  # wait result or engine-wide outage. A fault-free run of the same probe/status
  # supplies the control and must complete cleanup; its resource is distinct.
  # Independently observe the faulted full ID stopped but PRESENT, the retained
  # owned record and unmodified approval/snapshot. No absent/completed tombstone.
  # Require the real rule run.cleanup-temporary (classification 75) on stderr,
  # distinct from known child exit: 0 maps to wrapper 75; 7 and 75 stay unchanged.
  # Retain the existing safe protected intent reference, not a made-up public ID
  # or unimplemented stop retry. A truthful exited workload is allowed; a stopped
  # success claim means completed session cleanup and must be absent here.
  # Diagnostics and ownership records exclude raw argv/environment, response
  # bodies and copied output; the approved snapshot and deliberate streams are
  # separate data. Clear the fault only for identity-checked harness cleanup after
  # assertions; its removal cannot turn the product failure into a successful run.
  # Engine loss/cancellation remains slice-12, public status/stop retry slice-11.
  @wip @slice-14 @ID-OFFLINE-16 @engine
  Scenario Outline: Native workload exit is retained when required removal fails
    Given a CI offline checkout with the cached image and a removal-only transient Engine API fault
    When the product runs an offline probe that writes a checkout edit and exits <child>
    Then the actual workload exit is <child> and the product returns <result> with a separate temporary cleanup failure
    And native Docker still observes the exact stopped worker and its protected ownership record before harness cleanup
    And the diagnostic carries the safe protected intent reference without the synthetic body marker or a stopped-success claim
    And the checkout edit and unrelated resources remain intact

    Examples:
      | child | result |
      | 0     | 75     |
      | 7     | 7      |
      | 75    | 75     |
