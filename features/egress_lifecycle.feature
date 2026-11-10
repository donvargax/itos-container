@phase-1
Feature: Egress refusal evidence and owned failure recovery
  The person can read safe refusal metadata after a run has ended.
  A failed proxy never grants another route or leaves an unattended workload
  under a living controller. Cleanup and a later stop account for every owned
  resource without discarding the log or touching another session.

  # Slice-28 specification only; no step definitions or implementation here.
  # Authorities: charter milestone two step 1; contract AC-17/22/23/28/36;
  # decisions 0022 and 0025; architecture.md Slices; arc42's Egress run,
  # Recovery, Bounded operations and Diagnostics and streaming.
  # Read against main be5b830, slice-26 497a41c, slice-27 02e7714 and T-40
  # ae9b027. T-39's proxy exists on main. T-40's schema-3 lifecycle, watcher,
  # stdout reader and RefusalLog protocol exist on its branch. Slice-27's
  # egress.feature is a spec, not implemented runtime support. Its approval,
  # executable, image, topology and frozen-grant rules remain prerequisites.
  # This item owns only this feature file; never edit sibling specs to fit it.
  #
  # Dependency handoff: p1-acceptance-static-binary and the already recorded
  # p1-egress-refusal-log-writer must precede slice-27. T-40 refuses a nil writer
  # and retains the stopped proxy on copy failure, so a discard writer cannot
  # stand in for that task. The writer task owns streaming private publication
  # beyond approval Publish's 1 MiB limit and its fake/adapter contract. Slice-27
  # binds it to Runner AND Recovery. This item adds sequence-gap interpretation,
  # the public report and integrated failure/recovery proof. The coordinator
  # schedules T-40 -> writer -> slice-27 -> slice-28 and reconciles 0022/arc42's
  # older assignment of the writer to slice-28; no registry edits in this spec.
  #
  # Settled log contract, from 0022: stdout only, one schema-1 JSON object per
  # line, at most 2 KiB. seq starts at ready=1 and increases across ALL events.
  # time is RFC3339 UTC. Events are ready, refused, upstream-failed and log-full.
  # A refused/upstream-failed record carries only method/target/port when 0022's
  # lexical rules allow them, plus its enumerated reason. Keep original case,
  # trailing dot and port text such as 0443; target is the host portion, with
  # port separate, not the whole authority. Never resolve a refused name.
  # Do not write client addresses, Go errors, URLs, userinfo, headers, bodies,
  # SNI or tunnel bytes. An allowed tunnel produces no connection record.
  # Before 200: policy/address refusal is 403; DNS/connect failure is 502;
  # both have Content-Length: 0 and Connection: close. After 200: a rejected
  # ClientHello closes with no upstream payload and no proxy-generated TLS.
  # This preserves q-49/q-51's first-hello check without interception or a CA.
  #
  # Copy after worker absence, proxy SIGKILL, its not-running wait and fresh
  # inspection; never take a live tail as the final copy. Omit ready from the
  # published file, but include it in sequence validation. Keep every other
  # available record, its original seq and order. Publish at mode 0600 below
  # the protected state root, sessions/<lookup>/<public-id>/egress-refusals.jsonl,
  # using the writer's fsync/rename/parent-fsync protocol. Never put this file in
  # the checkout or copy proxy stderr into it. A failed read or publication
  # retains the stopped proxy, its log, networks, anchor and exact stop retry.
  # A successful copy with a gap still releases resources: retry cannot recover
  # bytes dockerd lost. Removal, repeated stop and new runs never delete the file.
  # No writable proxy mount, terminal log record, retention policy or lost-tail
  # guarantee is added. The existing disclosed lost-tail limit remains verbatim.
  #
  # Public report choices made here, extending slice-27's sessions[].egress:
  # add refusal_log {path, state, records, missing}. path is the absolute
  # protected destination for this session, never a path from current policy.
  # state is pending until a committed copy exists, stored for a copy without
  # observed gaps, gapped for one with gaps, or unknown when publication or
  # reading its evidence cannot be confirmed. records is null for pending/unknown,
  # otherwise the number of published lines (log-full counts, ready does not).
  # missing is [] or ordered inclusive {first,last} sequence ranges. A pending
  # path is a destination, not a claim that a file exists; no invented zero.
  # Offline entries omit egress. A completed empty copy has records=0, even
  # for a proxy that exited before writing anything; absence of a later seq
  # cannot reveal a lost tail. No counter claims total attempts or completeness.
  # Plain status prints the same path, state and stored count or missing ranges;
  # preserve slice-27's profile/list and all four limitations. Do not dump log
  # lines or contact the proxy to manufacture a live count. Status is read-only.
  # Read through the protected session binding and a port, never unchecked paths.
  # Reopening status after removal or controller death must retain these facts.
  # Durably bind the validated report to its committed copy, never just cache it
  # in the controller. Detect a missing initial seq=1 BEFORE filtering ready;
  # its deliberate omission from the published file is not itself a gap. The
  # publication protocol must not expose a new file with an old clean report.
  # If the file is missing/unreadable when required, report recovery.temporary
  # (75) and unknown evidence, never a clean zero or fabricate resource presence.
  # Quote paths and other hostile display text with approval's control/bidi/C1
  # escaping; JSON preserves the value as a properly encoded string.
  #
  # A committed gap reports run.refusal-log-gap from run and
  # recovery.refusal-log-gap from stop/status, each problem exit 1, with session
  # ID, path and inclusive ranges, and guidance that lost records cannot be
  # recovered by retry. These are this spec's diagnostic/exit choices, not a
  # new cleanup failure. Observed nonzero workload exit still wins for run.
  # Cleanup may be complete alongside gapped evidence. No stop retry remains
  # when every resource is gone. Repeated stop preserves the gap and returns 1
  # without rewriting the file or acquiring new authority. If removal also
  # fails, keep BOTH diagnoses and the real cleanup retry; do not replace one
  # with the other or call the session complete. Existing primary failure,
  # cancellation and nonzero workload precedence remain; otherwise gap is 1.
  #
  # Failure closure and timing are inherited, not new tunables. No proxy
  # restart, reconnecting worker, fallback network, policy reload or watchdog.
  # An absent/invalid readiness record or an exited proxy before worker create
  # refuses startup. A complete bad/EOF ready stream is run.proxy-failed (1);
  # a held readiness call expires as run.call-timeout (75), or the existing
  # startup-timeout if the total expires first. An observed runtime proxy exit
  # is run.proxy-exited (1), even exit zero; preserve the actual nonzero worker
  # exit and separately report the proxy cause. Engine/transport loss retains
  # its existing distinct classification and unknown exit where appropriate.
  # The worker gets existing SIGTERM/grace/SIGKILL teardown on proxy exit.
  # Controller SIGINT/SIGTERM keeps slice-12's first-signal rules and observed
  # exit precedence. Teardown's own proxy kill is never run.proxy-exited.
  # Calls/startup/grace/teardown remain 10/120/10/30 seconds in product builds.
  # Native fixtures may use ONLY the existing q-40 harness copy (1/3/1/30),
  # and assertions use that copy's budgets; no product flag or environment seam.
  # Controller SIGKILL leaves the original frozen worker/proxy running (A3).
  # Fresh stop never starts or restarts either; it uses original ownership and
  # full IDs, not current approval. Reverse removal is worker, stopped/copied
  # proxy, egress network, internal network, anchor. Each absence is confirmed.
  #
  # AC-36 scope must not hide ADR 0025's open question 6. Under 0022 a marker
  # placed in a syntactically valid hostname is persisted verbatim. This spec
  # proves exclusion from the listed payload/error fields, not universal secret
  # absence or credential-bearing hostname redaction. Do not invent hashing,
  # omission or a new log target grammar here. The person must settle that
  # representation/guarantee conflict before slice-31/33 claim full log absence.
  # Their adversarial hostname-marker probe remains required, including both
  # persisted copies; it must not be dropped or called a passing absence check.
  #
  # Shared absence step belongs to this item (triage 2026-11-09), not a task:
  # `the synthetic markers are absent from tree "<name>"` and
  # `the synthetic markers are absent from output "<name>"` use ONE helper.
  # Register a nonempty set of fresh ASCII synthetic markers per world. Search
  # raw bytes and base64 representations at all three input alignments, including
  # padded/unpadded and standard/URL alphabets, across streaming chunk boundaries.
  # The tree form covers every regular file, hidden/Git files included, with
  # bounded streaming memory; unreadable/missing input, walk failure or no
  # registered markers fails instead of passing vacuously. Do not follow links
  # outside the declared tree or silently skip an unexpected special entry.
  # Output means explicitly captured bytes, including binary Engine API output;
  # it must exist even if legitimately empty. Diagnostics identify the surface
  # and encoding, never echo a marker or the matching content.
  # Generalise offline_runtime_assertions_test.go's host-marker check,
  # lifecycle_native_assertions_test.go's lifecycleSafeState and the marker
  # portion of offline_isolation_results_test.go's safeDiagnostic through it.
  # Keep their non-marker assertions and existing argv/environment/output cases.
  # Use complete known markers, not base64 of the old SYNTHETIC prefix alone.
  # Ordinary harness regression tests, not product scenarios, plant each form
  # in a separate output and a positive-control tree and require the ACTUAL
  # absence assertion to fail, then require a clean control to pass. Include
  # all three base64 alignments and a match crossing a read boundary; each
  # scenario also verifies the scanner on a separate planted control before
  # judging its real surfaces. Keep controls outside the scanned product tree.
  # Slice-31/33 reuse these steps for inspect, records, logs and checkout bytes.
  # No broad scan for real credentials and no generated marker evidence commits.
  #
  # Implementation placement: run/status/stop keep thin command siblings and
  # consume shared engine/session domain/port APIs. Log parsing/metadata access
  # uses named adapters behind ports; pure sequence decisions stay in domain.
  # Extend the existing RefusalLog/RefusalCopy/report and storage contracts,
  # with an I/O-free porttest fake and the same contract against the adapter.
  # Egress imports no session or policy engine; no slice imports another.
  # Only cmd assembles adapters and supplies classifiers. No additional package
  # outside the vertical slices/engines and no domain/port I/O import exception.
  # Reuse Go 1.27.3's JSON/base64/streaming APIs and the pinned Moby
  # client v0.6.2/API v1.56.1; q-51 chose the existing standard-library proxy.
  # No new dependency, service, handwritten Docker protocol or engine CLI.
  # Update CLI/help/security and arc42 in the implementation with a decision
  # naming the public evidence report and any affected architecture sections.
  # Preserve existing offline record compatibility and every live scenario.
  #
  # Step handoff: every phrase below is NEW, except slice-27's specified
  # `a CI egress checkout allowing ... and matching protected approval`.
  # Reuse its fixtures rather than defining duplicate regular expressions.
  # The existing offline_runtime_*, lifecycle_*, recovery_* and approval helpers
  # supply process capture, protected seeds, independent inspection, barriers,
  # publication observation, retry parsing and fault forwarding, not new Givens
  # disguised as assertions. A Given writes only temporary inputs, original
  # protected bindings/records, clocks and scripted responses or trusted probes;
  # no Given requires product approval/run/status/stop success. Seed matching
  # consent independently; never approve against a real state folder.
  # Whens run product commands, including any control run, and keep premature
  # exit, timeout and missed barrier as observations for the named Then. Missing
  # steps are the implementer's red test commit, never red evidence themselves.
  #
  # Local cases use the real CLI with a recorded HTTP Engine API or existing cmd
  # contract assembly with port fakes and the actual disk writer. Observe every
  # call and publication, and script faults without inventing successful product
  # actions. Use controlled time for exact bounds beneath the scenarios; no long
  # wall-clock sleeps. Copy failure tests include the shared contract's Begin,
  # Write, Commit and Close/timeout races; sequence tests include chunked input,
  # first/middle gaps, ready filtering, >1 MiB and an empty valid copy. No local
  # daemon; only @engine cases may use one, guarded by engineFixtureAllowed.
  #
  # Native cases run only in CI, using slice-27's static product, pinned cached
  # images, endpoint controls and allowed-address fixture network. TLS keys stay
  # in fixture memory. The extra proxy-to-endpoint attachment is test-owned and
  # separately recorded; it never changes the worker or product networks. Record
  # actual full IDs, starts, waits, removals and final independently observed
  # absence. Fault forwarding may withhold/lose real replies or refuse a named
  # request, never fabricate successful native creates/starts/removals. No host
  # DNS/firewall/daemon change, public endpoint, model, account or credential.
  # Independent nonce controls must work before/after a denied probe; an empty
  # endpoint log, failed setup or unexecuted probe is not enforcement evidence.
  # A proxy killed before ready is killed at the real start-response barrier,
  # and a held ready stream holds only the controller's log response. Teardown
  # log reads remain available so the test can distinguish closure from leakage.
  # Assert product results before bounded, identity-checked harness cleanup.
  #
  # Neighbours: slice-27's EGRESS-02/09/10/12..19, approval's grant controls,
  # OFFLINE-14..23, lifecycle cancellation/loss, recovery/foreground recovery,
  # anchors and overlap, plus egress/session/writer contracts and cmd binding
  # tests. Reuse existing engine shards, assigning this WHOLE file to the other
  # shard with its selection contract; no orphaned @engine case. Local features
  # run without whole-package race detection; focused helper tests may use race.
  # Required gates run through hooks/CI. No local mutations or native runs.
  #
  # Expected reds are specification predictions, not measured runs. Main and
  # slice-26 still refuse version-2 run before launch; seeded status fixtures
  # have no refusal_log report. T-40 alone has no command/writer binding. After
  # prerequisites land, status counts/gaps remain new and are asserted in every
  # native case; closure already built by T-40/27 is preserved as part of the
  # same case, not manufactured into a failure. The implementer records each
  # assertion-level red after its step-only commit: locally with @wip removed
  # for local cases, CI's guarded selector for native ones. If a whole case is
  # already green, report the spec defect rather than weakening it or faking red.

  # Read-only evidence, including the difference between no copy and zero rows.
  @wip @slice-28 @ID-EGRESSLIFE-01 @local-engine-fixture @linux
  Scenario Outline: Status describes this session's stored evidence without inventing a live count
    Given an original protected egress session with evidence fixture "<fixture>" and a separate offline session
    And the current policy and approval differ from both original sessions
    When fresh status processes inspect those sessions in plain and JSON modes
    Then the egress refusal log report has state "<state>" records "<records>" and missing ranges "none"
    And the report names that session's protected log destination and all frozen egress limitations
    And the offline entry omits egress and no status prints request records or claims a complete history
    And both status invocations return zero without writing state or mutating resources

    Examples:
      | fixture                                   | state   | records |
      | running proxy without a published copy    | pending | null    |
      | completed copy after a ready-only stream  | stored  | 0       |
      | completed copy with three refusal records | stored  | 3       |

  # Known loss is separate from resource cleanup and survives another process.
  @wip @slice-28 @ID-EGRESSLIFE-02 @local-engine-fixture @linux
  Scenario Outline: A hole in the copied sequence is reported after owned resources are released
    Given a local egress lifecycle fixture for "<operation>" with observed workload exit "<workload>"
    And its proxy stdout contains ready sequence 1 and request sequences 2 5 and 7
    When the product completes that egress operation and fresh status and stop follow
    Then the operation reports "<rule>" with missing ranges "3-4,6-6" and process exit <exit>
    And the committed log holds exactly request sequences 2 5 and 7 without ready or renumbering
    And fresh status and repeated stop report recovery.refusal-log-gap with exit 1 and the same gapped three-record report
    And cleanup is complete with no retry and every owned resource is confirmed absent
    And repeated stop changes neither the committed log nor completed records or engine resources

    Examples:
      | operation | workload | rule                     | exit |
      | run       | 0        | run.refusal-log-gap       | 1    |
      | run       | 17       | run.refusal-log-gap       | 17   |
      | stop      | unknown  | recovery.refusal-log-gap  | 1    |

  # Copy faults keep the source; a committed gap plus removal fault keeps both
  # diagnoses. A fresh process, not the failed controller's memory, retries.
  @wip @slice-28 @ID-EGRESSLIFE-03 @local-engine-fixture @linux
  Scenario Outline: Failed log preservation or removal retains the exact recovery work
    Given a local egress lifecycle fixture with an observed zero workload exit and fault "<fault>"
    When run tears down and a separate status inspects the retained session
    Then run reports "<diagnoses>" with exit <exit> and the original public session's exact stop retry
    And status reports refusal evidence "<evidence>" separately from incomplete cleanup
    And the stopped proxy and every not-yet-released dependency retain their original owned recovery identity
    And no partial destination is reported as a committed clean log and no later resource is removed past the fault
    When the fault is removed and a fresh stop process retries twice
    Then the original log is committed without duplicate records and owned cleanup completes
    And the retry reports the same sequence gaps if any without treating them as cleanup work
    And checkout edits approval snapshots and unrelated resources remain unchanged

    Examples:
      | fault                               | diagnoses                                  | exit | evidence |
      | refusal copy cannot begin           | run.cleanup-temporary                      | 75   | pending  |
      | proxy log transport fails mid-copy  | run.cleanup-temporary                      | 75   | pending  |
      | refusal copy write fails            | run.cleanup-temporary                      | 75   | pending  |
      | refusal copy file fsync fails       | run.cleanup-temporary                      | 75   | pending  |
      | refusal copy rename fails           | run.cleanup-temporary                      | 75   | pending  |
      | refusal copy parent fsync fails     | run.cleanup-temporary                      | 75   | unknown  |
      | gapped copy then proxy remove fails | run.refusal-log-gap,run.cleanup-temporary   | 1    | gapped   |

  @wip @slice-28 @ID-EGRESSLIFE-04 @local-engine-fixture @linux
  Scenario Outline: Recovery never adopts or removes a foreign egress resource
    Given protected egress recovery records with a committed two-record log and ownership defect "<defect>"
    And independently recorded foreign containers networks and another project's live session
    When fresh status and stop inspect the original session
    Then both report recovery.not-owned with exit 1 while preserving the stored two-record log report
    And neither operation signals removes adopts or rewrites the conflicting resource
    And original recovery authority and the exact retry remain without a cleanup-complete claim
    And every foreign resource and the other project's progress remain unchanged

    Examples:
      | defect                                    |
      | proxy full ID differs from its observation |
      | proxy image differs from the frozen ID    |
      | proxy ownership label differs             |
      | egress network full ID differs            |
      | egress network ownership label differs    |
      | internal network ownership label differs  |
      | name-only candidate conflicts with full ID |

  # Path display is hostile input even though the host owns the state folder.
  @wip @slice-28 @ID-EGRESSLIFE-05 @local-engine-fixture @linux
  Scenario Outline: Status and gap diagnostics escape the refusal log path without changing its identity
    Given an original protected egress session with a gapped log under a state-root name containing "<text>"
    When fresh status processes inspect that session in plain and JSON modes
    Then plain status and its recovery.refusal-log-gap diagnostic escape the complete log path without terminal controls
    And the JSON refusal log path decodes to the original filesystem path with the same stored count and gaps
    And the log and every protected record remain byte-for-byte unchanged

    Examples:
      | text                     |
      | newline and ANSI escape  |
      | C1 U+0085                |
      | bidi override U+202E     |

  # Native metadata, with upstream failure distinct from policy refusal.
  # The fixture makes dns.example resolve NXDOMAIN in a disposable proxy-side
  # DNS control and connect.example resolve to a reachable closed port 443.
  # DNS fault injection is confined to the disposable proxy's network namespace
  # by the harness after readiness, never a product option or resolver override
  # on the host. The allowed-name control uses that same DNS path successfully.
  # It changes neither host DNS nor the product's address rule. Pre/post DNS
  # and connection controls prove these exact faults, not a broken fixture.
  @wip @slice-28 @ID-EGRESSLIFE-06 @engine @linux
  Scenario: Every refused request retains safe metadata and separates policy from upstream failure
    Given a CI egress checkout allowing "api.example.com dns.example connect.example" and matching protected approval
    And independent allowed TLS DNS-failure and connect-failure controls with request receipts
    When the product's worker sends these requests between verified allowed TLS controls and exits zero:
      | method  | target                  | response | event           | reason        |
      | CONNECT | unlisted.example:443    | 403      | refused         | not-listed    |
      | CONNECT | API.example.com:443     | 403      | refused         | not-canonical |
      | CONNECT | api.example.com.:443    | 403      | refused         | not-canonical |
      | CONNECT | api.example.com:0443    | 403      | refused         | port          |
      | CONNECT | dns.example:443         | 502      | upstream-failed | dns           |
      | CONNECT | connect.example:443     | 502      | upstream-failed | connect       |
    Then every control and table row has an independent completion receipt with the specified empty response and closure
    And the protected log contains exactly one safe record per row with the original method target port and reason
    And its schema times and consecutive sequences match the proxy stdout captured before removal except for ready
    And fresh plain and JSON status name that log with six stored records and the lost-tail limitation
    And run returns zero only after the product confirms owned cleanup without changing the fixture endpoints

  # AC-36's payload fields; no marker is used as a valid CONNECT hostname.
  # Each row has a unique marker. Verify sent/received probe receipts by length
  # and digest, not by printing payloads. The allowed TLS request reaches the
  # in-memory endpoint; refused targets/hellos never send their payload upstream.
  # Engine-failure bodies are confined to the named cleanup fault, not workload
  # stdout/stderr. The tree scan excludes fixture sources/positive controls.
  @wip @slice-28 @ID-EGRESSLIFE-07 @engine @linux
  Scenario: Payload and failure markers never enter proxy logs or controller status
    Given a CI egress checkout allowing "api.example.com" and matching protected approval
    And an independent TLS endpoint and fresh synthetic payload markers for these surfaces and expected log reasons:
      | surface                                                  | reason      |
      | CONNECT userinfo before api.example.com:443                | malformed   |
      | CONNECT a/path?token=marker:443                            | malformed   |
      | CONNECT api.example.com:443/path?token=marker              | malformed   |
      | GET http://api.example.com/path?token=marker               | not-connect |
      | unlisted CONNECT headers including Proxy-Authorization    | not-listed  |
      | accepted TLS URL query headers and body                   | no record   |
      | mismatched ClientHello SNI                                | tls-name    |
      | non-TLS bytes after CONNECT                               | not-tls     |
      | engine error body during the first proxy removal          | no record   |
    When the product runs every marker probe and fresh stop retries the one removal fault before status in both formats
    Then independent receipts prove every marker probe and its expected acceptance or refusal actually occurred
    And both persisted proxy log copies contain only the permitted metadata with the expected refusal reasons
    And the synthetic markers are absent from output "proxy stdout and Docker log frames"
    And the synthetic markers are absent from tree "protected product state"
    And the synthetic markers are absent from output "controller diagnostics and all plain and JSON status"
    And fresh status names the retained log with seven independently counted records after complete owned cleanup

  # SIGINT/TERM rows use a cooperative worker exiting 17 after its receipt;
  # nonzero precedence is real engine evidence, not a manufactured 130/143.
  @wip @slice-28 @ID-EGRESSLIFE-08 @engine @linux
  Scenario Outline: Normal failure and interruption preserve the final refusal before reverse teardown
    Given a CI egress lifecycle probe with a retained checkout edit and ending "<ending>"
    And unrelated control resources and an independent allowed TLS endpoint
    When run starts the probe exchanges its control and last refused CONNECT then receives that ending
    Then the real worker receipts and engine exit require process exit <exit> with only the expected cancellation diagnostic
    And product teardown confirms worker absence then stopped proxy then committed log then proxy egress network internal network and anchor absence
    And the last acknowledged refusal is in the private published log before the proxy is removed
    And fresh status reports that log with one stored record and cleanup complete without a proxy-failure diagnostic
    And the checkout edit original approval and unrelated resources survive unchanged

    Examples:
      | ending                      | exit |
      | primary exits zero          | 0    |
      | primary exits 17            | 17   |
      | controller receives SIGINT  | 17   |
      | controller receives SIGTERM | 17   |

  # Each faulted invocation is paired with a separate healthy run in When.
  # Capturing a failure at readiness cannot count as a successful no-worker test
  # unless the product actually created/started/verified the original proxy.
  @wip @slice-28 @ID-EGRESSLIFE-09 @engine @linux
  Scenario Outline: A proxy that never becomes ready cannot start the workload
    Given a CI egress startup fixture with readiness fault "<fault>" and a separate healthy control
    When run executes the healthy control and then the faulted invocation
    Then the control completes a verified TLS exchange and fresh status reports its stored log
    And the faulted run reaches the named proxy barrier and reports "<rule>" with exit <exit> within the existing budgets
    And no worker is created or started and no fallback route proxy restart or duplicate resource is requested
    And only created owned resources are removed in reverse order with a retained log report visible to fresh status

    Examples:
      | fault                                         | rule             | exit |
      | proxy killed after start before ready delivery | run.proxy-failed | 1    |
      | ready response held until its call expires     | run.call-timeout | 75   |

  # Probe remains busy after a successful allowed exchange; it records TERM or
  # ignores it. A teardown observation, not the harness kill, determines exit.
  @wip @slice-28 @ID-EGRESSLIFE-10 @engine @linux
  Scenario Outline: Losing the running proxy stops the workload without a new route
    Given a CI egress failure probe whose worker "<response>" after a successful tunnel and one refusal
    And independent direct-route controls remain reachable outside the worker
    When run starts the probe and the harness kills only its independently identified proxy
    Then native observations show the original running worker and proxy before the kill and run reports run.proxy-exited
    And the worker receives the existing termination sequence with observed workload exit <exit> and process exit <process>
    And worker probes during the remaining grace deliver no direct-route nonce while the outside controls still succeed
    And no network attachment fallback proxy restart or additional worker start is requested
    And fresh status reports the original one-record log after confirmed reverse owned cleanup

    Examples:
      | response                   | exit | process |
      | exits zero on SIGTERM       | 0    | 1       |
      | exits 17 on SIGTERM         | 17   | 17      |
      | ignores SIGTERM until kill  | 137  | 137     |

  # Sequential requests avoid treating an unacknowledged final refusal as a
  # client failure; 0022 permits the cap-triggering connection to close. Send
  # 20,000 distinct attempts with bounded concurrency/memory, not giant output.
  @wip @slice-28 @ID-EGRESSLIFE-11 @engine @linux
  Scenario: Exhausting the refusal budget stops the proxy and preserves its earliest attempts
    Given a CI egress cap probe with an allowed TLS control and a worker that exits zero on SIGTERM
    When run starts it and the worker completes a control then sends 20000 refused CONNECT attempts
    Then the actual proxy exits nonzero at the refusal cap and run reports run.proxy-exited with process exit 1
    And the published log contains 20000 request records followed by one log-full record with original consecutive sequences
    And the earliest and final request metadata remain without rotation truncation or ready in the published file
    And fresh status reports 20001 stored records and the lost-tail limitation after complete owned cleanup

  # Native removal failure uses the recording forwarder's named one-shot error;
  # it does not remove resources in the harness and then claim product cleanup.
  @wip @slice-28 @ID-EGRESSLIFE-12 @engine @linux
  Scenario Outline: A failed egress removal is recoverable by the exact public stop command
    Given a CI egress cleanup probe with one refusal and a one-shot removal fault on "<resource>"
    And another project's live worker proxy and networks with independent progress receipts
    When run exits zero and encounters that removal fault
    Then run reports run.cleanup-temporary with exit 75 and exactly the original public session's stop retry
    And fresh status names the committed one-record log and pending cleanup with the original resource identities
    And removal has stopped at the fault without deleting later dependencies or the other project's resources
    When a fresh process executes that exact retry and then repeats it
    Then the first stop confirms every original owned resource absent and the second performs no mutation
    And both statuses retain the same log count and path without rewriting its bytes
    And both projects' edits approvals and the other project's progress are preserved

    Examples:
      | resource         |
      | proxy            |
      | egress network   |
      | internal network |

  # A3 keeps both alive under the frozen grant after controller SIGKILL. The
  # worker attempts the same routes/names before and after loss, with controls.
  @wip @slice-28 @ID-EGRESSLIFE-13 @engine @linux
  Scenario: Controller death preserves frozen egress and a fresh stop saves refusals from both sides of the loss
    Given a CI egress controller-loss probe with allowed and refused names and independently reachable route controls
    When run starts it the worker records one refusal and the harness kills only the controller
    Then the original worker and proxy continue under the same frozen allowlist with no restart or widened route
    And the worker completes its allowed controls and second refusal while all direct-route attempts remain denied
    When fresh status inspects the abandoned session and a fresh stop removes it before status runs again
    Then the first status reports controller absent with a pending log and the exact stop candidate
    And the final status reports the original two-record log with cleanup complete after reverse full-ID removal
    And the log contains both acknowledged refusals and unrelated resources and checkout edits are preserved

  # A name-only miss cannot fence a delayed accepted create. Cover that race
  # beneath this native proof in the local contract: keep recovery incomplete
  # until the original full ID is found, saved and independently revalidated.
  # No worker should yet exist at these barriers, so workload_exit stays null.
  @wip @slice-28 @ID-EGRESSLIFE-14 @engine @linux
  Scenario Outline: Controller loss during egress creation leaves original resources recoverable without starting work
    Given a CI egress startup fixture that withholds the accepted "<resource>" create reply
    When run reaches that native create barrier the controller is killed and a fresh stop reconciles the original session
    Then the durable intent precedes the real create and recovery saves the matching full ID before any removal
    And no worker instruction proxy restart or replacement resource is started by recovery
    And fresh status reports cleanup complete with unknown workload exit and refusal evidence "<evidence>"
    And all created original resources are confirmed absent in reverse order without changing unrelated resources

    Examples:
      | resource         | evidence |
      | internal network | pending  |
      | egress network   | pending  |
      | proxy            | stored   |
