@phase-1
Feature: Execute literal argv in a live owned foreground session
  Exec attaches another command to an existing worker under its frozen grant.
  Finishing that command leaves the foreground session running. Ending the
  foreground session ends every exec; exec never keeps a worker alive.

  # Slice-38 specification only. Authorities: charter milestone two step 4;
  # container-pilot AC-03/06/21/22/24/36; CLI.md's separator contract; arc42's
  # Building blocks, Recovery and Bounded operations; decisions 0010 and 0016.
  # Implement internal/slices/exec/{command,domain,port,...} with shared session
  # operations in internal/engine/session. No slice imports run or stop. Only
  # cmd assembles adapters; domains use ports and I/O-free porttest fakes.
  # Reuse pinned kong v1.16.1, Moby client v0.6.2/API v1.56.1 and stdcopy;
  # no new dependency, shell command builder or host process-signalling library.
  # TTY selection, raw mode and resize belong to slice-39. These invocations use
  # redirected streams, TTY false, no detach mode and no detach escape sequence.
  #
  # Termination decision, settled here before implementation:
  # Moby ExecAttach sends /exec/<id>/start; closing its stream is not a kill.
  # There is no exec-signal endpoint. After start may have been sent, cancellation,
  # a detected client/stream disconnect or uncertain startup stops the ENTIRE
  # owned session through shared recovery. No host PID signalling, extra killer
  # exec, injected shell/supervisor or privileged helper. Send the first SIGINT
  # or SIGTERM once to the freshly revalidated full worker ID (PID 1), with ten
  # seconds of grace from cancellation and SIGKILL if still owned and running,
  # within thirty seconds total including locks, drain and diagnostics. A
  # disconnect uses SIGTERM. Do not claim that a signal was delivered to the
  # exec process; terminating/removing the container terminates its processes.
  # Repeated signals never reset either budget. Normal exec exit never stops
  # the primary. Before any start request, cancellation leaves the session alone.
  # Help and cancellation/disconnect diagnostics disclose session-wide teardown.
  #
  # Disconnect means an observed non-EOF input error, broken output sink, or
  # lost attachment without an authoritative exit. Ordinary stdin EOF half-closes
  # input only. Abrupt controller death (e.g. SIGKILL) retains arc42 A3: isolation
  # remains until explicit stop; no watchdog or invented exit is promised here.
  # A missing/unknown primary controller cannot authorize a new exec. This does
  # not change run controller-loss policy or claim automatic SIGKILL recovery.
  #
  # Admission uses the protected original registration, intent and frozen input,
  # current host identity, original socket/daemon and independently inspected full
  # worker ID, never labels/name/PID alone or current approval. Require controller
  # present, durable state running and a fresh running primary. Revalidate under
  # the session operation lock before exec create and again before exec start;
  # validate the returned exec's ID and owning container too. Release the lock
  # while waiting/draining. Stop and foreground exit fence subsequent start.
  # Neither exec nor recovery rewrites the primary's observed exit with an exec
  # exit. Exec cannot acquire a new container, network, mount, image or grant.
  #
  # Startup has the existing 120-second total and ten-second finite-operation
  # budget, including state locks, inspection, exec create and start/attach.
  # A healthy running exec has no workload timeout. Exec inspection polls are
  # individually bounded; output drain is bounded by ten seconds. If cleanup is
  # needed, drain runs concurrently within the original thirty-second total.
  # An accepted/lost start is never retried, detached or replaced by another exec.
  # Only an authoritative stopped observation for this started exec with a
  # PRESENT exit code gives a workload result. Missing/null exit is unknown:
  # pinned client.ExecInspect flattens a missing ExitCode to zero, so the adapter
  # must preserve wire presence rather than treating that default as evidence.
  # Observed nonzero exec exit wins; observed zero wins only with successful
  # streams and required cleanup. Unknown exit returns 130/143 for SIGINT/TERM,
  # otherwise 75 with exec.workload-exit-unknown unless another cause applies.
  # Cleanup failure is separate, with the original session ID and exact same-root
  # `itos container stop <id>` retry. Confirmed cleanup has no retry. Never print
  # raw API bodies, argv, environment, stream bytes or synthetic secret markers
  # in controller diagnostics or persisted state; deliberate streams stay separate.
  #
  # Harness handoff: every step below is NEW except the generic "it exits with
  # code", "its error output says" and "its standard output is empty" assertions
  # in steps_test.go/run_steps_test.go. New Givens only build disposable fixtures,
  # seed protected records/leases and program observers. They never call exec or
  # assert product behavior. Local cases run the CLI through the command assembly
  # with I/O-free fakes AND the disk/Moby adapter against a recording HTTP fixture,
  # as recovery_run_steps_test.go and the session contracts do. No local Docker.
  # Literal argv comes from decoded JSON directly to a subprocess argument array,
  # not commandWords or a host shell. SESSION in invocation examples expands to
  # a fixture's full 64-digit lowercase hexadecimal public ID, never a worker ID.
  # Native Givens only provision trusted probes, synthetic approval and disposable
  # files. When launches run, observes its barrier, then attempts exec. Record
  # premature product exit/missing exec readiness and assert it in Then; do not
  # fail a setup step because exec is not implemented. Every native assertion
  # requires positive exec readiness/output evidence, not just absence of leaks.
  # Harness cleanup follows assertions and cannot supply product cleanup evidence.
  #
  # All scenarios remain @wip. The implementer commits complete steps red first,
  # runs local rows with @wip removed for that run and obtains native red only in
  # guarded CI. Each first Then below must fail: main.go currently has no exec
  # command, so it emits generic usage instead of an exec diagnostic or receipt.
  # This source-based prediction is NOT executed red evidence. Neighbours:
  # run.feature; offline argv/streams/frozen-grant/cleanup rows; recovery ownership,
  # concurrent stop and unknown-exit rows; lifecycle budgets/signals/drain rows;
  # anchors and egress cleanup. Slice-39 consumes this termination contract.

  # Invocation validation precedes any state, policy, approval or engine access.
  # There is exactly one public session ID before -- and a nonempty executable
  # after it. A later argument cannot replace an empty executable. No abbreviations,
  # names, worker IDs, environment/user overrides or missing separator inference.
  @ID-EXEC-01 @wip @slice-38 @linux
  Scenario Outline: Exec requires a full session ID and an explicit command
    Given an exec invocation fixture with isolated state and access recorders
    When the exec CLI receives "<arguments>" with ITOS_JSON "0"
    Then its error output says "itos container exec <session-id> -- <command>"
    And it exits with code 2
    And its standard output is empty
    And exec invocation validation accessed no session state or engine and changed no files

    Examples:
      | arguments                         |
      | exec                              |
      | exec -- probe                     |
      | exec short -- probe               |
      | exec ../session -- probe          |
      | exec SESSION other -- probe       |
      | exec SESSION                      |
      | exec SESSION probe                |
      | exec SESSION --                   |
      | exec SESSION -- '' later          |
      | exec --user root SESSION -- probe |

  # Match run's JSON rule: wrapper --json or literal ITOS_JSON=1 wins over
  # missing ID/argv and a workload --no-json cannot cancel it. Inherited JSON
  # still refuses with wrapper --no-json, as run does. No stream is opened.
  # The new assertion parses ALL stdout as one schema-1 object, ok false,
  # exactly one problem with rule/exit plus nonempty message and fix. The fix
  # names removal of --json and ITOS_JSON=1; stderr also explains the refusal.
  @ID-EXEC-02 @wip @slice-38 @linux
  Scenario Outline: Exec refuses JSON before session lookup or workload parsing
    Given an exec invocation fixture with isolated state and access recorders
    When the exec CLI receives "<arguments>" with ITOS_JSON "<inherited>"
    Then exec reports exactly one JSON refusal "exec.json-unsupported" with exit 2 and removal guidance
    And exec invocation validation accessed no session state or engine and changed no files

    Examples:
      | arguments                              | inherited |
      | --json exec SESSION -- probe           | 0         |
      | exec --json SESSION -- probe --no-json | 0         |
      | exec SESSION -- probe --json           | 1         |
      | exec --no-json SESSION -- probe        | 1         |
      | exec                                   | 1         |
      | exec --json SESSION -- ''              | 0         |

  # The fixture holds a primary-controller lease unless the row changes it.
  # Foreign/unsafe rows alter one fact in otherwise valid original authority;
  # it retains before-snapshots and unrelated resources. No current checkout
  # config is needed to seed a session. A wrong-user row uses injected host
  # identity in the command assembly, never an actual account or credentials.
  @ID-EXEC-03 @wip @slice-38 @linux
  Scenario Outline: Exec admits only an unambiguous live owned session
    Given an exec admission fixture with condition "<condition>"
    When the exec CLI requests a probe in the fixture session through both local backends
    Then exec refuses with "<rule>" and exit <exit> before creating or starting an exec
    And the refused exec changed no session record, worker, unrelated resource or checkout file

    Examples:
      | condition                          | rule                     | exit |
      | unregistered public ID             | exec.not-owned           | 1    |
      | worker ID used as public ID        | exec.not-owned           | 1    |
      | another project with same basename | exec.not-owned           | 1    |
      | different effective host user      | exec.not-owned           | 1    |
      | conflicting intent and snapshot    | exec.not-owned           | 1    |
      | foreign worker with copied labels  | exec.not-owned           | 1    |
      | original daemon replaced           | exec.not-owned           | 1    |
      | symlinked protected record         | exec.state-unsafe        | 1    |
      | malformed protected record         | exec.state-invalid       | 2    |
      | protected state unreadable         | exec.state-unavailable   | 3    |
      | preparing                          | exec.session-not-running | 1    |
      | ready                              | exec.session-not-running | 1    |
      | stopping                           | exec.session-not-running | 1    |
      | stopped                            | exec.session-not-running | 1    |
      | failed                             | exec.session-not-running | 1    |
      | running record but primary exited  | exec.session-not-running | 1    |
      | running record but worker absent   | exec.session-not-running | 1    |
      | primary controller absent          | exec.session-not-running | 1    |
      | primary controller probe unknown   | exec.temporary           | 75   |
      | original engine unavailable        | exec.engine-unavailable  | 3    |

  # Observation barriers mutate only fixture authority, after a successful
  # admission inspection. The healthy sibling control must start and exit 23;
  # a fixture that prevents all exec calls cannot prove these revalidations.
  @ID-EXEC-04 @wip @slice-38 @linux
  Scenario Outline: Exec revalidates authority before each launch mutation
    Given an exec admission fixture that substitutes "<authority>" at "<boundary>"
    When the exec attempt and its unchanged control run through both local backends
    Then the changed attempt reports "exec.not-owned" with exit 1 and the control exits 23
    And no exec start or cleanup mutation targets substituted authority and no late reply resumes launch

    Examples:
      | authority                                    | boundary           |
      | protected intent                             | before exec create |
      | full worker ID and labels                    | before exec create |
      | pinned engine socket                         | before exec start  |
      | original daemon identity                     | before exec start  |
      | protected intent                             | before exec start  |
      | returned exec belongs to another container   | before exec start  |
      | returned exec ID differs from the created ID | before exec start  |

  # The recorder emits separate output channels and an explicit exit only for
  # the exec request it actually received. The primary remains live throughout.
  # Empty, whitespace, Unicode, quote and shell-looking argv must survive exactly;
  # --help/--json/--no-json/-- after the separator are workload data only.
  @ID-EXEC-05 @wip @slice-38 @linux
  Scenario Outline: Exec preserves literal argv streams and its own exit status
    Given a live exec fixture whose primary stays running and whose exec exits <exit>
    And the exec argument array is JSON
      """
      ["/workspace/exec-probe", "", "two words", "tab\there", "line\nbreak", "café", "\"quoted\"", "*", "$HOME", "$(touch slice-38-host-marker)", ";", "> slice-38-host-marker", "--help", "--json", "--no-json", "--"]
      """
    When the exec CLI sends that array and finite stdin through both local backends
    Then the recorded exec argv and separate output bytes match exactly and exec exits <exit>
    And exec starts exactly once without a shell or added flags and no host helper marker exists
    And the original primary remains running with its unchanged grant and unknown primary exit
    And exec help explains literal argv and that cancellation or disconnect stops the whole session

    Examples:
      | exit |
      | 0    |
      | 23   |

  # Seed an original non-root UID:GID, workspace subdirectory and literal env,
  # including the existing reserved HOME/TMPDIR/proxy values for that profile.
  # Changes below affect only current request/approval or caller environment.
  # Exec inherits existing container environment and mounts; it does not import
  # the caller's env, reread credential sources or provision new credentials.
  @ID-EXEC-06 @wip @slice-38 @linux
  Scenario Outline: Exec uses the frozen grant even after current inputs change
    Given a live exec fixture with original frozen settings and "<change>"
    When the exec CLI requests a settings probe through both local backends
    Then exec succeeds with only the original UID GID workspace environment mounts and network restrictions
    And no exec operation reads current approval or credential bytes, acquires images or creates session resources
    And the original grant, primary result and unrelated resources are unchanged

    Examples:
      | change                                      |
      | invalid policy and manifest                 |
      | replacement approval with wider permissions |
      | current approval removed                    |
      | caller environment contains override values |
      | offline policy changed to allowlist         |
      | allowlist policy widened with another host  |

  # Cancel before any start may be sent: no running exec exists to terminate.
  # The command assembly injects an actual cancelled context at the named port
  # boundary; it does not bypass the exec handler or invent a returned result.
  @ID-EXEC-07 @wip @slice-38 @linux
  Scenario Outline: Cancellation before exec start leaves the primary alone
    Given a live exec fixture with "<signal>" scheduled at "<boundary>"
    When the exec CLI attempts its probe through both local backends
    Then exec reports "exec.cancelled" with "<signal>" and exits <exit> without an exec start
    And no signal or removal reaches the primary and its session remains usable

    Examples:
      | signal  | boundary                       | exit |
      | SIGINT  | before session lookup          | 130  |
      | SIGTERM | after exec create before start | 143  |

  # EXEC-08/09/10 use five harmless, distinct synthetic values:
  # slice-38-argv-marker in a literal probe operand, slice-38-env-marker in the
  # caller's environment ONLY, slice-38-api-body-marker in raw engine replies,
  # and slice-38-stdout-marker / slice-38-stderr-marker in separate probe streams.
  # The marker Given only prepares inputs, response bodies and stream schedules;
  # it never invokes exec. Frozen inputs start marker-free and must stay intact.
  # The HTTP fixture puts the API marker in an ignored inspection-response field
  # and in the message field of each API error. Fake failures expose the same
  # underlying fault, never a precomputed CLI result. The marked start-error row
  # accepts start then returns HTTP 500 with that message; removal-unavailable
  # rows return HTTP 503 with it. Record delivery of those marked error bodies
  # and require it in Then, so an unconsumed fixture cannot prove exclusion.
  #
  # The EXEC-08 healthy control delivers both marked streams. EXEC-09/10 deliver
  # exact marked stdout/stderr prefixes before the scheduled event, including
  # before breaking an output sink; record missing receipts for Then to reject.
  # Capture these deliberate prefixes separately by their observed byte offsets.
  # All remaining CLI output is controller output: never strip marker strings
  # globally or exempt the whole stderr stream, which could hide a copied marker.
  # The exclusion assertion checks exec diagnostics plus all fresh status/stop
  # stdout/stderr and every persisted control-state file, including registration,
  # intent, locks and any new controller log/artifact. Keep fixture recordings
  # outside that state root. Capture files before clearing faults and again after
  # status/retry; a successful retry cannot erase evidence of an earlier leak.
  # Check every captured version for all five markers and their raw source
  # payloads. The unchanged frozen snapshot and deliberate workload prefixes
  # remain available; neither is deleted/redacted to make these assertions pass.
  # These marker setup, delivery and exclusion steps are NEW, as are the other
  # exec-specific steps; RECOVERY-20 and OFFLINE-15 are neighbouring contracts.
  #
  # No retry of either mutation. Lost exec-create acknowledgement cannot have
  # started work; lost start/attach acknowledgement CAN. The latter must stop
  # the session even if no exec bytes or running observation ever arrive. Each
  # fault row pairs with a healthy probe to reject fixture-only failures.
  @ID-EXEC-08 @wip @slice-38 @linux
  Scenario Outline: Exec bounds launch uncertainty without retrying a possible command
    Given a live exec fixture with launch fault "<fault>" and a healthy control
    And the exec fixture stages distinct synthetic argv caller-environment API-body stdout and stderr markers
    When both exec attempts run through the local backends with deadline observations
    Then the faulty exec reports "<rule>" with exit 75 within its operation and startup budgets and the control exits 23
    And exec start was never retried and the faulty session is "<session>"
    And a late engine reply cannot start work or change the reported result
    And every injected marked API error was delivered to the exec controller before it reported failure
    And the healthy exec control delivers the exact marked stdout and stderr and both frozen snapshots remain unchanged
    And exec controller and recovery diagnostics and all captured control-state files contain none of the synthetic markers or raw payloads

    Examples:
      | fault                                                   | rule                 | session                                          |
      | operation lock does not become available                | exec.call-timeout    | still running                                    |
      | create accepted but response lost                       | exec.start-uncertain | still running                                    |
      | create ignores cancellation past ten seconds            | exec.call-timeout    | still running                                    |
      | start accepted but attach response lost                 | exec.start-uncertain | durably cleaned within thirty seconds of failure |
      | start accepted but marked API error returned            | exec.start-uncertain | durably cleaned within thirty seconds of failure |
      | start ignores cancellation past ten seconds             | exec.call-timeout    | durably cleaned within thirty seconds of failure |
      | original 120-second startup budget expires before start | exec.startup-timeout | still running                                    |

  # Keep the engine available for cleanup, with exact controlled exec exit
  # observations. "null" and "missing" are distinct wire bodies, not zero.
  # "Started" Givens program responses and event schedules only; When invokes
  # exec and delivers the event after its start request. Exit observations follow
  # the event, including a known exit learned during disconnect cleanup. A known
  # exec zero with complete cleanup wins the signal, while missing result cannot
  # borrow primary exit 0. A stopped exec with missing exit data but healthy
  # streams needs no session teardown: it reports uncertainty and leaves the
  # primary running. An unknown result alone is not a disconnect.
  @ID-EXEC-09 @wip @slice-38 @linux
  Scenario Outline: Exec preserves observed exits and keeps unknown results honest
    Given a started exec fixture with event "<event>" and exit observation "<observed>"
    And the exec fixture stages distinct synthetic argv caller-environment API-body stdout and stderr markers
    When the exec CLI settles the event through both local backends
    Then exec exits <exit> with separate diagnostic "<rule>" and primary and exec results remain distinct
    And the fixture session is "<session>" and no exec exit is invented
    And exec delivered the exact marked stdout and stderr before the event and preserved the frozen snapshot
    And exec controller and recovery diagnostics and all captured control-state files contain none of the synthetic markers or raw payloads

    Examples:
      | event                                           | observed | exit | rule                       | session         |
      | SIGINT                                          | 23       | 23   | exec.cancelled             | durably cleaned |
      | SIGTERM                                         | 0        | 0    | exec.cancelled             | durably cleaned |
      | SIGINT                                          | missing  | 130  | exec.cancelled             | durably cleaned |
      | SIGTERM                                         | null     | 143  | exec.cancelled             | durably cleaned |
      | output sink failure                             | 0        | 75   | exec.stream-failed         | durably cleaned |
      | attachment transport lost                       | 23       | 23   | exec.engine-lost           | durably cleaned |
      | primary exit and exec removed before inspection | missing  | 75   | exec.workload-exit-unknown | durably cleaned |
      | exec stopped with missing exit field            | missing  | 75   | exec.workload-exit-unknown | still running   |
      | exec stopped with null exit field               | null     | 75   | exec.workload-exit-unknown | still running   |

  # Missing exec alone never proves worker absence. For incomplete cleanup retain
  # protected original ownership and a usable retry. Changing ownership forbids
  # mutation, even after the exec has run. Observed exec 23 still wins the refusal.
  # The fresh retry happens after clearing only the injected fault/substitution;
  # it consumes original authority without current approval and preserves edits.
  @ID-EXEC-10 @wip @slice-38 @linux
  Scenario Outline: Exec cleanup failure keeps original recovery authority
    Given a started exec fixture with observed exit <observed> and cancellation cleanup fault "<fault>"
    And the exec fixture stages distinct synthetic argv caller-environment API-body stdout and stderr markers
    When exec cancellation completes and fresh status inspects the session through both local backends
    Then exec exits <exit> and reports "<rule>" separately with its original session ID and exact stop retry
    And incomplete cleanup is not reported as stopped and no foreign resource is changed
    And every injected marked API error was delivered to the exec controller before it reported failure
    And exec delivered the exact marked stdout and stderr before the event and preserved the frozen snapshot
    And exec controller and recovery diagnostics and all captured control-state files contain none of the synthetic markers or raw payloads
    And after the fault is cleared the printed stop retry confirms cleanup without substituting the exec exit for the primary result or losing checkout edits
    And exec controller and recovery diagnostics and all captured control-state files contain none of the synthetic markers or raw payloads

    Examples:
      | observed | fault                                        | exit | rule                    |
      | 0        | worker removal unavailable                   | 75   | exec.cleanup-incomplete |
      | 23       | worker removal unavailable                   | 23   | exec.cleanup-incomplete |
      | 23       | ownership replaced before signal             | 23   | exec.not-owned          |
      | 0        | engine unavailable through teardown deadline | 75   | exec.teardown-timeout   |

  # Barriers release the operation lock between transitions. An independent
  # stop wins before the selected mutation; a delayed positive observation may
  # not undo stopped state. No lock is held over foreground wait or stream drain.
  @ID-EXEC-11 @wip @slice-38 @linux
  Scenario Outline: A completed stop fences an exec continuation
    Given a live exec fixture with a stop barrier at "<boundary>"
    When exec pauses and a separate stop completes before the exec continuation resumes through both local backends
    Then exec refuses with "exec.session-not-running" and exit 1 and the independent stop exits 0
    And no subsequent exec start, worker restart or state rewrite occurs after durable cleanup

    Examples:
      | boundary                            |
      | after admission before exec create  |
      | after exec create before exec start |

  # Native evidence uses a trusted static probe copied to the disposable
  # checkout, executed only in the worker. Run's primary sends a nonce receipt
  # and waits for a release file. Exec prints its complete argv, UID/GID, cwd,
  # namespaces and env, proves stdin EOF, and writes separate output tails.
  # Collect its first output before sending EOF/releasing it: buffering until
  # exit fails. One row remains healthy longer than 120 seconds; no startup
  # budget may become an exec lifetime limit. Before exec, change policy and
  # manifest to request root, another cwd/env and network, and replace approval
  # with a different synthetic generation. These edits never reconfigure run.
  # Check non-root identity, read-only root, zero capabilities, NNP set and matching
  # mount/network namespace and mounted inode against the running primary;
  # actual write denial and writable-checkout controls supplement inspection.
  # No assertion relies on a real credential, model, account or external service.
  @ID-EXEC-12 @wip @slice-38 @engine @linux
  Scenario Outline: Native exec preserves argv streams frozen restrictions and foreground lifetime
    Given a CI exec checkout with frozen subdirectory settings and a trusted reporting probe
    And the exec argument array is JSON
      """
      ["/workspace/exec-probe", "", "two words", "tab\there", "line\nbreak", "café", "\"quoted\"", "*", "$HOME", "$(touch slice-38-host-marker)", ";", "> slice-38-host-marker", "--help", "--json", "--no-json", "--"]
      """
    When run reaches its primary barrier and exec receives that array and finite stdin with exit <exit> after <seconds> seconds
    Then the native exec receipt has the exact argv and frozen settings with timely separate streams and exit <exit>
    And stdin EOF leaves the primary and exec alive until the probe finishes and all output tails arrive
    And the native exec has the primary's restricted identity and namespaces and cannot write the image root
    And a second exec succeeds in the same still-running session without changing its grant or primary result
    And releasing the primary returns its own exit 17 and confirms owned cleanup with checkout edits preserved

    Examples:
      | exit | seconds |
      | 0    | 0       |
      | 23   | 121     |

  # Primary and exec both ignore termination and publish progress receipts.
  # First await actual exec readiness, then signal the CLI; inject repeats
  # during grace. Container-level signal observations target only the primary.
  # Independent worker absence and ceased exec progress prove termination.
  @ID-EXEC-13 @wip @slice-38 @engine @linux
  Scenario Outline: Cancelling a native exec tears down its whole owned session
    Given a CI exec checkout with a stubborn primary and stubborn exec probe plus an unrelated running session
    When run and exec reach their barriers and the exec client receives "<signal>" followed by repeated signals
    Then native exec readiness precedes one "<signal>" to the owned primary and bounded escalation and session cleanup
    And the exec client reports cancellation and session-wide teardown with its observed exec exit or the unknown-exit fallback <fallback>
    And neither exec nor its descendants remain after cleanup and the unrelated session keeps making progress
    And checkout edits and unrelated state survive and fresh status confirms the target session is stopped

    Examples:
      | signal  | fallback |
      | SIGINT  | 130      |
      | SIGTERM | 143      |

  # Break ONLY the exec client's selected stream after a positive readiness
  # receipt. For attachment loss, a CI forwarding recorder drops that connection
  # but keeps independent engine access available; it never invents engine replies.
  # A real write attempt establishes a broken sink. An unread healthy sink is
  # instead a bounded drain test, not evidence of a disconnect. Stderr failure
  # cannot demand a diagnostic on the broken descriptor or block teardown.
  @ID-EXEC-14 @wip @slice-38 @engine @linux
  Scenario Outline: A detected native exec disconnect cannot leave its command running
    Given a CI exec checkout with a stubborn primary and a continuously reporting exec probe
    When run and exec reach their barriers and the harness breaks "<stream>" while keeping engine control available
    Then the started exec and primary are terminated and owned cleanup completes within thirty seconds of detected disconnect
    And the exec client returns a nonzero observed exec exit or wrapper 75 without claiming successful execution
    And closing the attachment alone is not counted as termination and no helper exec or host PID signal is used

    Examples:
      | stream                    |
      | exec stdout reader        |
      | exec stderr reader        |
      | exec attachment transport |

  # Run two active execs: one must not monopolize the operation lock or delay
  # stop. The primary exits 17 on release, or ignores TERM for explicit stop.
  # Each exec writes a readiness nonce and ongoing descendant progress. After
  # ending the session, a third exec request must be refused, never resurrect it.
  # Observed per-exec nonzero exits win; if removal wins inspection, report 75
  # and exec.workload-exit-unknown, not the primary's 17 or stop's zero.
  @ID-EXEC-15 @wip @slice-38 @engine @linux
  Scenario Outline: Primary exit and concurrent stop end every active native exec
    Given a CI exec checkout with a gated primary and two long-running exec probes
    When both execs reach their barriers and the session ends by "<event>" while one exec output drain is blocked
    Then both native execs started and session cleanup completes within thirty seconds without waiting for their natural exit
    And both exec results remain separate from the primary and stop results and output drain is bounded by ten seconds
    And every exec descendant is gone and a subsequent exec is refused with "exec.session-not-running"
    And the checkout edit survives and no session resource is restarted or leaked

    Examples:
      | event                       |
      | primary exits 17            |
      | independent concurrent stop |

  # Actual run creates the target in its own disposable project. The caller is
  # another project for the foreign row; the stopped row waits for run cleanup.
  # Try the same probe successfully in a live owned control, so generic parsing
  # failure or a probe that cannot run cannot masquerade as the required refusal.
  @ID-EXEC-16 @wip @slice-38 @engine @linux
  Scenario Outline: Native exec refuses foreign and ended sessions without adopting them
    Given two CI exec checkouts with independent synthetic grants and trusted probes
    When the product attempts exec against a "<target>" session and a live owned control
    Then the target exec reports "<rule>" with exit 1 and the owned control returns its exact probe receipt and exit 23
    And no probe ran in the refused target and its resources and records were neither adopted nor changed

    Examples:
      | target  | rule                     |
      | foreign | exec.not-owned           |
      | stopped | exec.session-not-running |
