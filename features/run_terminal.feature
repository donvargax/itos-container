@phase-1
Feature: An explicitly selected terminal for an owned foreground run
  Run can attach a terminal to the literal foreground command without changing
  its approved isolation or ownership. Terminal input belongs to that command.
  Leaving or losing the client restores the host terminal when possible and
  finishes bounded owned cleanup, preserving the observed workload result.

  # Authorities: docs/charter.md's interactive milestone, the pilot's foreground
  # command contract, docs/architecture.md (Slices), T-56's terminal contract,
  # and docs/decisions/0016-cancellation-and-output-drain.md (slice-12).
  # This is slice-37's specification only. All cases remain @wip until defined
  # steps have assertion-level red and green at their stated tier. No steps or
  # implementation accompany this spec; the failures below are predictions.
  #
  # Interface decisions:
  # - Only run --tty -- <argv> requests terminal mode. Without --tty, preserve
  #   noninteractive behavior even when stdin/stdout are terminals. No automatic
  #   selection, short alias, detach escape sequence or implicit shell is added.
  #   After --, every word, including --tty, belongs to the workload unchanged.
  # - Terminal mode requires terminal stdin AND stdout; stderr may be redirected.
  #   Bind the T-56 provider to stdin: its saved state, dimensions and hangup are
  #   authoritative even if stdout names a different terminal. Do not discover
  #   /dev/tty, use the parent's terminal or manufacture a terminal for a pipe.
  # - Invocation order is existing JSON refusal, explicit nonempty argv, terminal
  #   eligibility/acquisition, then existing approved preflight/startup. JSON
  #   retains run.json-unsupported, exit 2 and one schema-1 refusal on stdout.
  #   ITOS_JSON=1 still refuses, including with --no-json as run does today;
  #   this item does not change that inherited-request rule. JSON, argv and
  #   eligibility refusals acquire no raw lease and make no engine connection,
  #   consent or state write. Failed acquisition must roll back partial changes.
  # - Missing terminal descriptors use run.terminal-required, exit 2, with guidance
  #   to use terminal stdin/stdout or omit --tty. Acquisition failures, including
  #   unsupported host terminal capability and zero/invalid initial dimensions,
  #   use run.terminal-unavailable, exit 3, with terminal-repair/retry guidance.
  #   Neither may silently fall back to noninteractive mode or an invented size.
  #   All preflight/startup work, including terminal acquisition, shares the
  #   existing startup deadline; terminal mode adds no separate startup budget.
  # - Terminal mode is a per-invocation attachment choice, not a permission grant
  #   or persisted transcript. Reuse the selected approval unchanged: no new
  #   policy setting, approval fingerprint input, mount, device grant, host
  #   environment passthrough or recovery schema. TERM may be set by the existing
  #   approved manifest; do not inherit TERM, COLUMNS or LINES from the controller.
  #
  # Transport decisions:
  # Save/restore the exact host input terminal state, including nondefault flags.
  # While attached, host canonical input, echo and signal generation are disabled.
  # Forward bytes without line buffering, filtering, echoing or interpreting
  # Ctrl-C (03), Ctrl-D (04) or an escape sequence as a controller command.
  # The worker's own terminal line discipline may turn a byte into its own signal
  # or EOF; this is distinct from a host signal or local terminal hangup.
  # Worker stdout/stderr share its PTY and reach controller stdout as ONE raw
  # stream, without Docker multiplex headers/demultiplexing. Controller diagnostics
  # remain on stderr. Noninteractive streams keep their existing separation.
  # Initial columns/rows come from stdin before creation and must be visible at
  # the workload's first instruction. Changed sizes reach only this owned worker;
  # resize bursts may coalesce, but the last valid size must reach it. Never swap
  # columns/rows or use COLUMNS/LINES, 80x24 or a terminal from another invocation.
  # The terminal lease and resize/input observers outlive successful startup;
  # an expired startup context must not end a healthy, quiet running command.
  #
  # Lifecycle decisions:
  # Restore on every return after acquisition, including preflight/start/stream
  # failure and external cancellation. Finish the lease's owned observation;
  # close only adapter-owned handles, not caller-owned files. A restoration
  # failure is separately reported as run.terminal-restore (75), never hidden
  # by the original failure or called a successful restoration. When a terminal
  # has vanished, restoration may succeed or fail: require the attempt and an
  # honest result, not a platform-specific errno. A usable terminal must regain
  # its exact saved state. No terminal payload is logged to protected records.
  #
  # Passive local hangup, including while input/output are idle or input is
  # unread, is run.terminal-disconnected (75). A controlling-terminal SIGHUP
  # must not terminate the controller before it can handle that disconnect.
  # A failed live terminal observation is run.terminal-unavailable (75), and a
  # failed engine resize is run.terminal-resize (75). These are controller
  # failures, not synthetic child exits or host SIGINT/SIGTERM cancellations.
  # A finite resize timeout retains run.call-timeout and its operation name.
  # Begin teardown on the first such failure; finish terminal observation and
  # attempt restoration promptly, without waiting for an input byte or blocked
  # output. Unread resize notifications must not hold teardown open. An engine
  # EOF/resize failure racing an authoritative exit is reconciled before claiming
  # failure; an already observed normal exit and deliberate lease close are not
  # new terminal disconnects. No resize is issued after teardown has begun.
  #
  # Use slice-12's shared termination path, not a second cleanup algorithm.
  # On terminal loss/failure send SIGTERM once to the freshly revalidated owned
  # live primary, allow the original 10 s grace INCLUDING delivery time, then
  # SIGKILL once only if fresh ownership and running state still permit it.
  # Keep the original 30 s total teardown and 10 s finite-call/final-drain bounds.
  # Drain and owned stopping/removal proceed concurrently; blocked terminal
  # output cannot prevent signal delivery, restoration or cleanup. Host-delivered
  # SIGINT/SIGTERM retain their existing first-signal forwarding, repeat handling,
  # run.cancelled diagnostic and unknown-exit wrapper 130/143 rules. A later
  # resize, signal, hangup or cleanup error cannot replace the first cause,
  # restart a deadline or trigger a duplicate signal/removal.
  #
  # Every observed nonzero workload exit wins, even over terminal/cleanup errors.
  # Observed zero with successful streams/cleanup/restoration returns zero after
  # ordinary exit or host cancellation. A genuine terminal/controller failure
  # still returns 75 with observed zero or unknown exit; a SIGTERM request or
  # successful removal is never proof of child exit 143, 137 or zero. Preserve
  # the separate typed failures and usable recovery identity when cleanup fails.
  # Unverifiable ownership or engine loss cannot authorize speculative cleanup;
  # retain the same protected records and allow later status/stop under the
  # existing rules. Controller SIGKILL/engine unavailability retains slice-12's
  # isolation-and-explicit-recovery limit; no watchdog or new guarantee is added.
  #
  # Implementation handoff:
  # Extend internal/slices/run and internal/engine/session under the Slices rules;
  # reuse internal/engine/terminal's Provider/Lease from T-56. Domain decisions
  # and terminal/session values stay behind ports; only cmd binds adapters.
  # Extend shared session fake/adapter contracts for TTY create/verification,
  # raw attachment, bounded resize and terminal completion. Keep porttest fakes
  # I/O-free; native terminal/session adapters own file, signal and SDK work.
  # Reuse existing pins: term v0.46.0, sys v0.48.0, Moby client v0.6.2/API
  # v1.56.1, and test-only creack/pty v1.1.24. The pinned SDK has ConsoleSize
  # (height,width) and ContainerResize; no dependency or termios/client rewrite.
  # Preserve all worker boundary checks, including TTY verification before start.
  # Record the terminal integration and merged-TTY-stream exception in arc42's
  # building-block/foreground views in implementation, with a decision naming
  # the affected sections; no architecture-policy exception is needed.
  #
  # Fixture/step handoff (ALL step phrases below are NEW):
  # A local terminal-run fixture writes an isolated approval checkout, manifest,
  # independent matching approval seed, outside sentinels and a before snapshot.
  # It installs the Unix HTTP Engine API recorder and disposable PTY pairs with
  # initial size 113 columns by 37 rows and saved nondefault terminal settings.
  # Where both descriptors are terminals, bind them to the same PTY slave by
  # default; keep an open harness handle for the final exact-state comparison.
  # A pipe endpoint is a disposable pipe, never the operator's terminal. Stderr
  # is captured separately. Misleading COLUMNS=7, LINES=9 and TERM values live
  # only in the child controller environment; expected worker environment stays
  # the independently seeded approved one. No Given invokes any product command,
  # approves real state, executes a workload or asserts product behavior.
  # Reuse features/run_steps_test.go's PTY mechanisms, run_preflight_steps_test.go
  # and offline_local_engine_test.go's seeds/journals, not their non-TTY framing.
  # The recorder models both raw TTY and multiplexed non-TTY attachments according
  # to actual create requests, retaining all requests and raw synthetic frames.
  # Do not teach it to accept missing TTY as a successful terminal run.
  #
  # A terminal-run contract fixture is a test-only cmd assembly of the real run
  # entry, registered session lifecycle and terminal ports. Run each local fault
  # through I/O-free fakes and a disposable PTY plus disk/SDK backend where the
  # real fault is controllable. Inject only observations/operations at port or
  # adapter test seams, never the expected result, a production flag or a new
  # environment bypass. Restore-error determinism may use the terminal fake;
  # native disappearance is also exercised by the engine disconnect case. Use
  # controlled clocks for exact 10/30 s fault budgets, retaining socket adapter
  # deadline checks. A fake clock cannot advance a real HTTP server's timer.
  #
  # CI fixtures extend lifecycle_native_* and offline_runtime_* infrastructure:
  # prepare a trusted Go terminal probe in the disposable checkout, to execute
  # ONLY in the CI worker using the independently inspected cached image. Use
  # synthetic bytes, a harmless persistent checkout marker, an independent engine
  # observer, and a separately owned foreign resource control. The probe reports
  # terminal descriptors, initial/current sizes, literal argv, bytes and signal
  # receipts; it can be quiet, raw, canonical, signal-handling or signal-ignoring.
  # Receipts after hangup use the checkout, not a vanished output channel. The
  # host controller runs in its own disposable controlling-terminal session so
  # master closure exercises SIGHUP as well as passive hangup without signalling
  # the harness or another worker. No host Docker socket enters the worker.
  #
  # All When steps collect early exits, missing ready frames and deadline expiry
  # as observations for Then. Race readiness barriers with child exit; never
  # fail setup or hang because the current binary refuses --tty. Harness-forced
  # kill/cleanup is not product teardown evidence. Take evidence before bounded,
  # identity-checked fixture cleanup; keep caller handles available for state
  # comparisons. Require stated healthy controls and real ready/progress evidence
  # for every successful launch, never vacuous absence. Invocation refusals do
  # not require a ready worker. Export coverage through the existing harness.
  # @local-engine-fixture means no daemon; @engine means guarded Linux amd64 CI
  # only. No agent runs a real engine scenario. Use the existing q-40 compiled
  # CI timing fixture for call/startup/grace (1/3/1 s); ordinary/release builds
  # keep 10/120/10 s. Never add a runtime timing override, shorten assertions or
  # repeat long real-time waits locally. PTY allocation failures fail, not skip.
  #
  # Neighbours to preserve: RUN-01..07 (argv/JSON), RUN-08..19 (preflight),
  # OFFLINE-02/03/06/07/13/15 and foreground_recovery.feature (streams, exit and
  # cleanup), lifecycle.feature and decision 0016 (budgets/signals/uncertainty),
  # recovery.feature (owned status/stop), and T-56's shared terminal contracts.
  # Existing noninteractive/JSON scenarios are unchanged. The implementer commits
  # defined steps red first with @wip retained; local assertion runs temporarily
  # remove @wip, while real-engine red/green comes only from coordinated CI.

  # Invocation selection. Baseline rejects the new wrapper flag; the first
  # positive TTY assertion distinguishes that from the unchanged plain control.
  @wip @slice-37 @ID-RUNTTY-01 @linux @local-engine-fixture
  Scenario Outline: TTY selection is explicit even when streams are terminals
    Given a local terminal-run fixture with terminal stdin and terminal stdout
    When the fixture invokes plain run and terminal run with workload arguments "<arguments>"
    Then only the terminal invocation creates and verifies a TTY worker with initial size 113 by 37
    And both invocations deliver exactly the literal workload arguments
    And the plain invocation keeps separate output streams and never changes terminal settings or resizes a worker
    And the terminal invocation uses raw attachment output on stdout and separate controller diagnostics on stderr
    And both invocations restore saved host settings and confirm only their own resources absent
    And both invocations preserve approval checkout and outside sentinels except for their owned session state

    Examples:
      | arguments                                      |
      | fixture-workload '' 'two words'                 |
      | fixture-workload --tty --json --no-json -- --help |

  # Eligibility is a usage refusal before preflight, including when the opposite
  # stream is a terminal. Today there is no run.terminal-required diagnostic.
  @wip @slice-37 @ID-RUNTTY-02 @linux @local-engine-fixture
  Scenario Outline: Explicit terminal mode refuses redirected input or output
    Given a local terminal-run fixture with <input> stdin and <output> stdout
    When the terminal fixture invokes "run --tty -- fixture-workload"
    Then terminal run reports "run.terminal-required" with exit 2 and terminal-or-omit-tty guidance
    And terminal run emits no stdout and changes no terminal settings checkout or host state
    And terminal run makes no engine connection or helper execution

    Examples:
      | input    | output   |
      | pipe     | terminal |
      | terminal | pipe     |
      | pipe     | pipe     |

  # Required argv still precedes terminal eligibility. The old unknown --tty
  # usage failure is not the specific existing command-required behavior.
  @wip @slice-37 @ID-RUNTTY-03 @linux @local-engine-fixture
  Scenario Outline: Terminal mode still requires an explicit nonempty executable
    Given a local terminal-run fixture with pipe stdin and pipe stdout
    When the terminal fixture invokes "<arguments>"
    Then terminal run reports "run.command-required" with exit 2 and explicit-command guidance
    And terminal run emits no stdout and changes no terminal settings checkout or host state
    And terminal run makes no engine connection or helper execution

    Examples:
      | arguments                         |
      | run --tty                         |
      | run --tty --                      |
      | run --tty -- '' 'later executable' |
      | run --tty fixture-workload        |

  # Keep RUN-03..05's full-document JSON oracle, including missing argv and
  # nonterminal combinations. A generic parser usage object must fail this Then.
  @wip @slice-37 @ID-RUNTTY-04 @linux @local-engine-fixture
  Scenario Outline: JSON refusal precedes terminal selection and command validation
    Given a local terminal-run fixture with <input> stdin and <output> stdout
    When the terminal fixture invokes "<arguments>" with inherited ITOS_JSON "<inherited>"
    Then terminal run emits exactly one schema-1 JSON refusal for "run.json-unsupported" and exits 2
    And terminal run explains removal of wrapper JSON and ITOS_JSON on its separate diagnostic channel
    And terminal run changes no terminal settings checkout or host state
    And terminal run makes no engine connection or helper execution

    Examples:
      | input    | output   | arguments                                  | inherited |
      | terminal | terminal | run --tty --json -- fixture-workload        | 0         |
      | terminal | terminal | --json run --tty -- fixture-workload        | 0         |
      | terminal | terminal | run --tty -- fixture-workload --no-json     | 1         |
      | terminal | terminal | run --tty --no-json -- fixture-workload     | 1         |
      | pipe     | pipe     | run --tty --json --                         | 0         |
      | pipe     | terminal | run --tty -- ''                            | 1         |

  # Native PTY metadata is bad, not a fake failure result. Retrying with a valid
  # measured size is a positive control; COLUMNS/LINES cannot supply a fallback.
  @wip @slice-37 @ID-RUNTTY-05 @linux @local-engine-fixture
  Scenario Outline: Unusable initial terminal dimensions refuse before startup
    Given a local terminal-run fixture with terminal stdin and terminal stdout
    And the fixture terminal has <columns> columns and <rows> rows
    When the terminal fixture invokes run with tty and retries after setting size 113 by 37
    Then the first terminal run reports "run.terminal-unavailable" with exit 3 before any engine connection or state write
    And the retry starts with the measured size 113 by 37 and exits zero after owned cleanup
    And both attempts leave the exact original terminal settings and approval intact

    Examples:
      | columns | rows |
      | 0       | 37   |
      | 113     | 0    |
      | 0       | 0    |

  # Native transport. The trusted probe observes TTY/size before changing its
  # worker terminal settings, then uses raw output for an exact byte oracle.
  # Alternate writes to stdout/stderr, including bytes resembling an eight-byte
  # Docker framing header, catch accidental demultiplexing or CR/LF conversion.
  @wip @slice-37 @ID-RUNTTY-06 @engine @linux
  Scenario Outline: The foreground command receives a terminal and literal argv from its first instruction
    Given a CI terminal-run fixture with initial size 113 by 37 and a reporting probe that exits <exit>
    When terminal run launches the probe with an empty argument spaces and literal wrapper-looking flags
    Then the probe observes all three worker descriptors as terminals and initial size 113 by 37
    And the probe observes exactly the supplied argv frozen workspace identity environment and isolation
    And both worker output streams reach terminal stdout as the exact ordered raw payload without framing or a controller banner
    And protected session records contain no terminal input or output payload
    And terminal run returns the observed exit <exit> with exact host terminal restoration
    And independent inspection confirms owned worker and anchor removal while checkout edits approval and foreign resources survive

    Examples:
      | exit |
      | 0    |
      | 17   |

  # Keep the lease beyond successful startup; quiet time exceeds the configured
  # startup/call limits before resize. Observe the intermediate size before the
  # burst, then the burst's final size; do not demand every intermediate event.
  @wip @slice-37 @ID-RUNTTY-07 @engine @linux
  Scenario: A quiet foreground terminal receives changed and latest dimensions
    Given a CI terminal-run fixture with initial size 113 by 37 and a quiet resize-reporting probe
    When terminal run remains quiet beyond startup and call budgets then the harness changes size to 151 by 49 and bursts through 79 by 23 to 167 by 53
    Then the running probe reports initial size 113 by 37 then 151 by 49 and finally 167 by 53 without a restart
    And the engine journal resizes only the verified owned worker and no resize follows teardown
    And a final input frame still reaches the probe and releases its normal zero exit
    And terminal run restores exact host settings and confirms owned cleanup without a cancellation diagnostic

  # The probe sets its OWN worker terminal raw before its ready frame; the host
  # is already raw. Sending 03/04 without newline must not need canonical flush,
  # trigger host SIGINT, end input forwarding or echo a second copy locally.
  @wip @slice-37 @ID-RUNTTY-08 @engine @linux
  Scenario: Raw control keys remain workload bytes and do not disconnect the client
    Given a CI terminal-run fixture with a raw input-reporting probe
    When terminal run reaches the raw probe ready frame and the harness writes bytes 03 04 41 without a newline then a final release frame
    Then the probe records bytes 03 04 41 exactly once before the release frame and remains live until that frame
    And the host terminal adds no echo and terminal run sends no engine signal or cancellation diagnostic
    And terminal run returns zero restores exact host settings and confirms owned cleanup

  # A separate canonical/ISIG worker proves actual interactive Ctrl-C semantics.
  # It installs its own SIGINT handler, records once, continues and accepts a
  # later line. An engine kill request or host cancellation cannot supply proof.
  @wip @slice-37 @ID-RUNTTY-09 @engine @linux
  Scenario: The worker terminal interprets Ctrl-C without cancelling the controller
    Given a CI terminal-run fixture with a canonical signal-handling probe
    When terminal run reaches the canonical probe ready frame and the harness writes byte 03 followed by a release line after the signal receipt
    Then the probe records exactly one terminal-generated SIGINT and accepts the later release line
    And terminal run sends no engine signal and reports no controller cancellation
    And terminal run returns zero restores exact host settings and confirms owned cleanup

  # Failure paths. The local recorder's barriers distinguish attach failure from
  # accepted start with a lost reply. Always retain/reconcile accepted effects.
  # Each expected typed failure is missing today because --tty never gets here.
  @wip @slice-37 @ID-RUNTTY-10 @linux @local-engine-fixture
  Scenario Outline: Startup failure restores the host terminal without launching later phases
    Given a local terminal-run fixture with terminal stdin and terminal stdout
    And the terminal-run fixture schedules "<fault>"
    When the terminal fixture invokes "run --tty -- fixture-workload"
    Then terminal run reports "<rule>" with exit <exit> and no later startup phase
    And terminal run restores the exact saved host terminal state and ends its terminal observers
    And accepted resources are reconciled only under the original ownership with incomplete recovery retained when necessary
    And terminal run preserves approval checkout and outside sentinels

    Examples:
      | fault                                         | rule                      | exit |
      | missing approval                              | run.approval-missing       | 1    |
      | created worker reports TTY disabled            | run.isolation-unverified  | 1    |
      | created worker reports different initial size | run.isolation-unverified  | 1    |
      | attach fails before start                     | run.execution-unavailable | 75   |
      | start accepted but its reply times out        | run.call-timeout          | 75   |
      | host SIGTERM during preflight                 | run.cancelled             | 143  |

  # These faults are scheduled after a ready TTY run, except restore failure
  # which follows the stated authoritative exit. The controller receives facts,
  # not a preselected return code. Unknown exit rows withhold authoritative wait
  # results even if cleanup succeeds. A healthy same-path control must also run.
  # For live-failure rows, release any authoritative exit only after the failure;
  # it cannot be dismissed as an operation attempted after an already known exit.
  # For output blockage/read error keep the existing stream diagnostic instead
  # of relabelling it as terminal loss. Engine resize timeout uses the call budget.
  @wip @slice-37 @ID-RUNTTY-11 @linux
  Scenario Outline: Terminal failures retain their cause exit precedence and bounded cleanup
    Given a terminal-run contract fixture with fault "<fault>" and authoritative exit "<child>"
    When terminal run reaches the fault and finishes through the applicable local backends alongside a healthy control
    Then terminal run reports "<rule>" separately from workload outcome "<child>" and returns <result>
    And terminal teardown restores or reports the failed restoration without waiting for input output or unread resize events
    And owned stop drain and cleanup share the original budgets without duplicate signals or late resizes
    And the healthy terminal control returns zero restores exact settings and confirms owned cleanup

    Examples:
      | fault                              | child   | rule                       | result |
      | terminal observation fails         | unknown | run.terminal-unavailable   | 75     |
      | resized terminal has zero rows     | 17      | run.terminal-unavailable   | 17     |
      | running worker resize fails        | 0       | run.terminal-resize        | 75     |
      | running worker resize times out    | unknown | run.call-timeout           | 75     |
      | terminal output blocks during exit | 17      | run.stream-timeout         | 17     |
      | terminal input read fails          | unknown | run.execution-unavailable  | 75     |
      | saved terminal restoration fails   | 0       | run.terminal-restore       | 75     |
      | saved terminal restoration fails   | 17      | run.terminal-restore       | 17     |

  # Host signals are deliberately different from typing a control byte. Deliver
  # only to this child controller after ready; an external SIGINT still cancels
  # while host ISIG is off. The worker's handler exit remains authoritative.
  @wip @slice-37 @ID-RUNTTY-12 @engine @linux
  Scenario Outline: External host cancellation retains its signal and observed workload result in terminal mode
    Given a CI terminal-run fixture with a signal-handling probe that exits <exit>
    When terminal run reaches the ready frame and the harness sends host signal "<signal>" then repeats SIGINT and SIGTERM during cleanup
    Then the probe records exactly one "<signal>" and terminal run returns the observed exit <exit>
    And terminal run reports "run.cancelled" with the first host signal separately from the workload result
    And terminal run restores exact host settings and finishes owned cleanup within the original teardown budget
    And later signals do not repeat forwarding restart grace or change foreign resources

    Examples:
      | signal  | exit |
      | SIGINT  | 0    |
      | SIGTERM | 17   |

  # Master closure, not host SIGKILL or test cancellation, is the action. Quiet
  # and unread-input cases prove passive disconnect observation. For the ignoring
  # probe, independently observe it alive before grace and exited after SIGKILL;
  # 137 must come from engine wait. The handled probe supplies its own exit 17.
  # The unread-byte variant queues a byte for a probe that never reads stdin;
  # whether forwarding has already consumed the local byte cannot hide hangup.
  @wip @slice-37 @ID-RUNTTY-13 @engine @linux
  Scenario Outline: Losing the terminal client stops its owned workload even without stream activity
    Given a CI terminal-run fixture with a "<probe>" probe and "<input>" terminal input
    When terminal run reaches the ready frame and the harness closes the client terminal master while leaving the engine reachable
    Then terminal run reports "run.terminal-disconnected" and returns authoritative workload exit <exit> within bounded teardown
    And the worker receives one owned SIGTERM with escalation "<escalation>" at the original grace deadline only while still owned and running
    And terminal restoration is attempted and any failure is reported separately without blocking cleanup
    And independent inspection confirms owned worker and anchor removal while checkout edits approval and foreign resources survive

    Examples:
      | probe            | input       | escalation    | exit |
      | SIGTERM-handling | idle        | no SIGKILL    | 17   |
      | signal-ignoring  | idle        | one SIGKILL   | 137  |
      | signal-ignoring  | unread byte | one SIGKILL   | 137  |

  # Disconnect cannot manufacture an exit or ownership authority. The cleanup
  # failure row retains observed 17, then a fresh status/stop after the fixture
  # clears removal proves recovery. The foreign-identity row never removes that
  # resource, even on retry; the engine-loss row recovers the original resource
  # only after connectivity returns. Each When must observe a ready TTY run
  # before inducing disconnect; Givens only schedule those future observations.
  @wip @slice-37 @ID-RUNTTY-14 @linux
  Scenario Outline: Terminal disconnect preserves uncertain outcomes and owned recovery
    Given a terminal-run contract fixture with disconnect condition "<condition>" and authoritative exit "<child>"
    When terminal run loses its client and later status and stop inspect the retained session after the fixture fault clears
    Then the first terminal run reports "run.terminal-disconnected" with workload outcome "<child>" and returns <result>
    And terminal run retains any cleanup failures and the original recovery identity without claiming an unobserved exit or unconfirmed absence
    And every signal resize and removal is confined to freshly verified ownership and no foreign resource is touched
    And the later recovery reports actual ownership and absence without restarting work or rewriting unknown history as success

    Examples:
      | condition                       | child   | result |
      | cleanup succeeds without a wait | unknown | 75     |
      | primary exits zero during stop  | 0       | 75     |
      | owned removal fails             | 17      | 17     |
      | engine becomes unreachable      | unknown | 75     |
      | inspected ownership changes     | unknown | 75     |
