@phase-1
Feature: An explicitly selected terminal for an owned foreground run
  Run can attach a terminal to the literal foreground command without changing
  its approved isolation or ownership. Terminal input belongs to that command.
  Leaving or losing the client restores the host terminal when possible and
  finishes bounded owned cleanup, preserving the observed workload result.

  # Authorities: docs/charter.md's interactive milestone, the pilot's foreground
  # command contract, docs/architecture.md (Slices), T-56's terminal contract,
  # and docs/decisions/0016-cancellation-and-output-drain.md (slice-12).
  # Slice-37 implements these cases. Local red evidence rejects --tty before
  # implementation. The coordinator accepted the fourteen engine rows' launch
  # assertions at 23270b6 (dispatch 43837837169, job 133198815706): exit 2,
  # unknown flag --tty. Their later steps first execute in implementation CI;
  # inspect that green run's complete step list for skipped assertions.
  #
  # Interface decisions:
  # - Only run --tty -- <argv> requests terminal mode. Without --tty, preserve
  #   noninteractive behavior even when stdin/stdout are terminals. No automatic
  #   selection, short alias, detach escape sequence or implicit shell is added.
  #   After --, every word, including --tty, belongs to the workload unchanged.
  # - Terminal mode requires terminal stdin AND stdout; stderr may be redirected.
  #   Stdin is the sole initial-size and resize authority, even if stdout names
  #   a different terminal. Acquire stdin's T-56 lease, then manage distinct
  #   stdout's state through the shared terminal port before preflight. If both
  #   descriptors name the SAME terminal, save/acquire/restore it once; separate
  #   file descriptors do not imply separate terminals. Do not discover /dev/tty,
  #   use the parent's terminal or manufacture a terminal for a pipe.
  #   Supplied /dev/tty aliases share the underlying terminal's state even when
  #   their filesystem identities differ from the supplied /dev/pts descriptor.
  # - Invocation order is existing JSON refusal, explicit nonempty argv, terminal
  #   eligibility/acquisition, then existing approved preflight/startup. JSON
  #   retains run.json-unsupported, exit 2 and one schema-1 refusal on stdout.
  #   ITOS_JSON=1 still refuses, including with --no-json as run does today;
  #   this item does not change that inherited-request rule. JSON, argv and
  #   eligibility refusals acquire no raw lease and make no engine connection,
  #   consent or state write. Failed acquisition must roll back every terminal
  #   already changed, including stdin when acquiring distinct stdout fails.
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
  # Save/restore each host terminal's own exact state, including nondefault flags.
  # While attached, stdin's canonical input, echo and signal generation are off.
  # Before forwarding any attachment bytes, disable stdout's output processing
  # (OPOST, including ONLCR's LF-to-CRLF conversion), even on a distinct terminal.
  # The fixture must not pre-disable it on behalf of the controller. Stdout's
  # dimensions, including zero, neither refuse acquisition nor resize the worker;
  # only stdin's dimensions are measured and validated for the session.
  # Forward bytes without line buffering, filtering, echoing or interpreting
  # Ctrl-C (03), Ctrl-D (04) or an escape sequence as a controller command.
  # The daemon applies escape-sequence handling to every TTY attach and cannot
  # disable it: with no detachKeys request it swallows Docker's default ctrl-p,
  # ctrl-q as a detach and drops the attach while the worker keeps running. Run
  # therefore pins one long per-run escape value on the attach request; that
  # exact byte sequence alone detaches, the recorded limitation, and every other
  # input, bytes 10 11 included, reaches the workload unchanged. The local
  # recorder models the daemon's escape handling with moby/term (test-only pin
  # v0.5.2), the library the daemon itself runs, so RUNTTY-20 proves the pinned
  # value locally and RUNTTY-21 against the real engine.
  # The worker's own terminal line discipline may turn a byte into its own signal
  # or EOF; this is distinct from a host signal or local terminal hangup.
  # Worker stdout/stderr share its PTY and reach controller stdout as ONE raw
  # stream, without Docker multiplex headers/demultiplexing. Controller diagnostics
  # remain on stderr. Noninteractive streams keep their existing separation.
  # Initial columns/rows come from stdin before creation and must be visible at
  # the workload's first instruction. Changed sizes reach only this owned worker;
  # resize bursts may coalesce, but the last valid size must reach it. Never swap
  # columns/rows or use COLUMNS/LINES, 80x24 or a terminal from another invocation.
  # Both terminals' state ownership and resize/input observers outlive startup;
  # an expired startup context must not end a healthy, quiet running command.
  #
  # Lifecycle decisions:
  # Restore every acquired terminal on every return, including partial acquisition,
  # preflight/start/stream failure and external cancellation. Never apply stdin's
  # saved settings to distinct stdout, or skip one restoration because the other
  # failed. Finish all terminal-state ownership and its owned observation;
  # close only adapter-owned handles, not caller-owned files. A restoration
  # failure is separately reported as run.terminal-restore (75), never hidden
  # by the original failure or called a successful restoration. When a terminal
  # has vanished, restoration may succeed or fail: require the attempt and an
  # honest result, not a platform-specific errno. A usable terminal must regain
  # its exact saved state. No terminal payload is logged to protected records.
  # On exit or host cancellation, retain raw output until the final bounded drain
  # ends, including buffered output and shutdown diagnostics. Restore as soon as
  # drain finishes or its original bound expires, independently of owned cleanup.
  # Report restoration failures within the original teardown deadline and the
  # existing finite diagnostic-call bound, even when stderr is an unread pipe.
  #
  # Passive local hangup of either terminal, including while input/output are idle
  # or input is unread, is run.terminal-disconnected (75). Restore the other
  # terminal if still usable. A controlling-terminal SIGHUP must not terminate
  # the controller before it can handle that disconnect. That protection lasts
  # the whole --tty run through diagnostic delivery: the terminal lease ends its
  # hangup subscription with the disconnect report, so the command's own signal
  # capture consumes any later hangup signal instead of dying to it while owned
  # teardown is still running. RUNTTY-22 delivers that late signal explicitly.
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
  # reuse and extend internal/engine/terminal's T-56 contract for stdout state
  # ownership, rollback and hangup without making stdout a dimensions source.
  # The current Provider couples raw mode to size validation/observation: do not
  # bind it unchanged to stdout or bind two providers to one underlying terminal.
  # Extend the shared fake/native contract for both descriptor layouts and failed
  # acquisition/restoration; reuse the pinned raw-state primitives. Domain
  # decisions and terminal/session values stay behind ports; only cmd binds adapters.
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
  # "Distinct" explicitly allocates two PTY pairs, binds stdin to one slave and
  # stdout to the other, and retains BOTH handles for separate state comparisons.
  # Seed different nondefault termios states, with stdout OPOST and ONLCR enabled
  # and no other output transformations. Do not set either terminal raw in setup.
  # Stdin starts at 113 by 37; distinct stdout starts at 79 by 23 unless the
  # scenario supplies another size.
  # RUNTTY-15's When first writes a fixed LF to the stdout slave and captures the
  # translated control at its master before launching the controller. Capture
  # the later attachment payload there without CR/LF normalization or trimming.
  # The recorder's raw payload includes lone LF, CRLF, lone CR and header-like
  # binary bytes. It sends them only after attachment/start, withholds exit until
  # the resize/input barriers finish, and retains the independently supplied bytes.
  # Change stdout to 151 by 49 and notify only the child controller of SIGWINCH;
  # collect a later input/progress exchange before changing stdin to 167 by 53
  # and notifying it again. Missing progress/early exit is a Then failure, not
  # a setup failure. No stdout-originated size may appear in the engine journal.
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
  @slice-37 @ID-RUNTTY-01 @linux @local-engine-fixture
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
  @slice-37 @ID-RUNTTY-02 @linux @local-engine-fixture
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
  @slice-37 @ID-RUNTTY-03 @linux @local-engine-fixture
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
  @slice-37 @ID-RUNTTY-04 @linux @local-engine-fixture
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
  @slice-37 @ID-RUNTTY-05 @linux @local-engine-fixture
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

  # The fault is an operation failure after stdin entered raw mode, not a
  # preselected command result. Keep both handles usable and require a retry
  # after clearing the fault. Native failure injection belongs at the test seam;
  # the same rollback contract also runs against the I/O-free terminal fake.
  @slice-37 @ID-RUNTTY-16 @linux
  Scenario Outline: Failure to acquire distinct stdout rolls back stdin before startup
    Given a terminal-run contract fixture with distinct terminal stdin and stdout
    And the fixture schedules "<fault>" after stdin enters raw mode
    When terminal run attempts acquisition then retries after clearing the fault through the applicable local backends
    Then the first terminal run reports "run.terminal-unavailable" with exit 3 before any engine connection or state write
    And the first attempt restores each changed terminal to its own exact saved state and ends all terminal observers
    And the retry starts with stdin size 113 by 37 delivers exact raw output and exits zero after owned cleanup
    And both attempts leave both caller terminal handles usable with their exact original settings and approval intact

    Examples:
      | fault                                    |
      | reading distinct stdout state fails      |
      | acquiring distinct stdout raw mode fails |

  # Native transport. The trusted probe observes TTY/size before changing its
  # worker terminal settings, then uses raw output for an exact byte oracle.
  # Alternate writes to stdout/stderr, including bytes resembling an eight-byte
  # Docker framing header, catch accidental demultiplexing or CR/LF conversion.
  # Distinct stdout retains OPOST/ONLCR until the controller acquires it; the
  # probe changes only its worker PTY, never either host terminal.
  @slice-37 @ID-RUNTTY-06 @engine @linux
  Scenario Outline: The foreground command receives a terminal and literal argv from its first instruction
    Given a CI terminal-run fixture with initial size 113 by 37 and a reporting probe that exits <exit>
    And the CI fixture binds stdin and stdout to <terminals> terminals with saved nondefault settings
    When terminal run launches the probe with an empty argument spaces and literal wrapper-looking flags
    Then the probe observes all three worker descriptors as terminals and initial size 113 by 37
    And the probe observes exactly the supplied argv frozen workspace identity environment and isolation
    And both worker output streams reach terminal stdout as the exact ordered raw payload without framing or a controller banner
    And protected session records contain no terminal input or output payload
    And terminal run returns the observed exit <exit> with exact restoration of each host terminal's own saved state
    And independent inspection confirms owned worker and anchor removal while checkout edits approval and foreign resources survive

    Examples:
      | terminals | exit |
      | shared    | 0    |
      | shared    | 17   |
      | distinct  | 0    |
      | distinct  | 17   |

  # A native two-PTY byte oracle needs only the local Engine API recorder. The
  # pre-run LF control proves output translation was enabled; an stdin-only
  # lease then fails exact output even if TTY startup otherwise works. Zero
  # stdout dimensions also catch reusing an input-sized lease for output.
  @slice-37 @ID-RUNTTY-15 @linux @local-engine-fixture
  Scenario Outline: Distinct output terminals preserve bytes and restore both saved states
    Given a local terminal-run fixture with distinct terminal stdin and stdout
    And the fixture stdin has size 113 by 37 and stdout has size <columns> by <rows>
    When the fixture measures stdout newline translation then runs a raw terminal payload with exit <exit> while resizing stdout then stdin
    Then the pre-run stdout control converts LF to CRLF and terminal stdout exactly matches the raw engine payload
    And terminal run creates a TTY worker at stdin size 113 by 37 and resizes it only to stdin size 167 by 53
    And the stdout-only size change to 151 by 49 leaves the run live for a later input and progress exchange
    And terminal run returns the observed exit <exit> with exact restoration of each host terminal's own saved state
    And both caller terminal handles remain usable and only owned resources are confirmed absent
    And terminal run preserves approval checkout and outside sentinels

    Examples:
      | columns | rows | exit |
      | 79      | 23   | 0    |
      | 0       | 0    | 17   |

  # Keep the lease beyond successful startup; quiet time exceeds the configured
  # startup/call limits before resize. Observe the intermediate size before the
  # burst, then the burst's final size; do not demand every intermediate event.
  @slice-37 @ID-RUNTTY-07 @engine @linux
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
  @slice-37 @ID-RUNTTY-08 @engine @linux
  Scenario: Raw control keys remain workload bytes and do not disconnect the client
    Given a CI terminal-run fixture with a raw input-reporting probe
    When terminal run reaches the raw probe ready frame and the harness writes bytes 03 04 41 without a newline then a final release frame
    Then the probe records bytes 03 04 41 exactly once before the release frame and remains live until that frame
    And the host terminal adds no echo and terminal run sends no engine signal or cancellation diagnostic
    And terminal run returns zero restores exact host settings and confirms owned cleanup

  # Docker's daemon cannot disable escape handling on a TTY attach (see the
  # transport decisions above). The fixture's keystroke after the detach attempt
  # models the client's next write, which the dropped attach fails; a swallowed
  # 10 11 leaves the workload without its bytes and the run failed, not exited.
  @slice-37 @ID-RUNTTY-20 @linux @local-engine-fixture
  Scenario: The daemon's default detach bytes reach the workload as input
    Given a local terminal-run fixture with shared terminal stdin and stdout
    When the fixture writes the daemon's default detach bytes and one further keystroke to the attached workload
    Then the workload receives exactly bytes 10 11 65 66 unfiltered
    And terminal run returns the observed exit 0 with exact restoration of each host terminal's own saved state
    And both caller terminal handles remain usable and only owned resources are confirmed absent

  # Same oracle as RUNTTY-08 against the real daemon's own escape handling:
  # bytes 10 11 must not detach the attach, echo or cancel the controller.
  @slice-37 @ID-RUNTTY-21 @engine @linux
  Scenario: Docker's default detach escape remains workload input on a real engine
    Given a CI terminal-run fixture with a raw input-reporting probe
    When terminal run reaches the raw probe ready frame and the harness writes bytes 10 11 65 without a newline then a final release frame
    Then the probe records bytes 10 11 65 exactly once before the release frame and remains live until that frame
    And the host terminal adds no echo and terminal run sends no engine signal or cancellation diagnostic
    And terminal run returns zero restores exact host settings and confirms owned cleanup

  # A separate canonical/ISIG worker proves actual interactive Ctrl-C semantics.
  # It installs its own SIGINT handler, records once, continues and accepts a
  # later line. An engine kill request or host cancellation cannot supply proof.
  @slice-37 @ID-RUNTTY-09 @engine @linux
  Scenario: The worker terminal interprets Ctrl-C without cancelling the controller
    Given a CI terminal-run fixture with a canonical signal-handling probe
    When terminal run reaches the canonical probe ready frame and the harness writes byte 03 followed by a release line after the signal receipt
    Then the probe records exactly one terminal-generated SIGINT and accepts the later release line
    And terminal run sends no engine signal and reports no controller cancellation
    And terminal run returns zero restores exact host settings and confirms owned cleanup

  # Failure paths. The local recorder's barriers distinguish attach failure from
  # accepted start with a lost reply. Always retain/reconcile accepted effects.
  # Each expected typed failure is missing today because --tty never gets here.
  # Both terminals have been acquired before these preflight/startup faults;
  # distinct rows must compare each restored state against its own saved copy.
  @slice-37 @ID-RUNTTY-10 @linux @local-engine-fixture
  Scenario Outline: Startup failure restores the host terminal without launching later phases
    Given a local terminal-run fixture with <terminals> terminal stdin and stdout
    And the terminal-run fixture schedules "<fault>"
    When the terminal fixture invokes "run --tty -- fixture-workload"
    Then terminal run reports "<rule>" with exit <exit> and no later startup phase
    And terminal run restores each host terminal's own exact saved state and ends all terminal observers
    And accepted resources are reconciled only under the original ownership with incomplete recovery retained when necessary
    And terminal run preserves approval checkout and outside sentinels

    Examples:
      | terminals | fault                                         | rule                      | exit |
      | shared    | missing approval                              | run.approval-missing      | 1    |
      | shared    | created worker reports TTY disabled           | run.isolation-unverified  | 1    |
      | shared    | created worker reports different initial size | run.isolation-unverified  | 1    |
      | shared    | attach fails before start                     | run.execution-unavailable | 75   |
      | shared    | start accepted but its reply times out        | run.call-timeout          | 75   |
      | shared    | host SIGTERM during preflight                 | run.cancelled             | 143  |
      | distinct  | missing approval                              | run.approval-missing      | 1    |
      | distinct  | attach fails before start                     | run.execution-unavailable | 75   |
      | distinct  | host SIGTERM during preflight                 | run.cancelled             | 143  |

  # These faults are scheduled after a ready TTY run, except restore failure
  # which follows the stated authoritative exit. The controller receives facts,
  # not a preselected return code. Unknown exit rows withhold authoritative wait
  # results even if cleanup succeeds. A healthy same-path control must also run.
  # For live-failure rows, release any authoritative exit only after the failure;
  # it cannot be dismissed as an operation attempted after an already known exit.
  # For output blockage/read error keep the existing stream diagnostic instead
  # of relabelling it as terminal loss. Engine resize timeout uses the call budget.
  # Distinct-terminal restoration faults allocate two terminals; only the named
  # terminal's restoration fails. The other must regain its exact saved state.
  @slice-37 @ID-RUNTTY-11 @linux
  Scenario Outline: Terminal failures retain their cause exit precedence and bounded cleanup
    Given a terminal-run contract fixture with fault "<fault>" and authoritative exit "<child>"
    When terminal run reaches the fault and finishes through the applicable local backends alongside a healthy control
    Then terminal run reports "<rule>" separately from workload outcome "<child>" and returns <result>
    And terminal teardown restores or reports the failed restoration without waiting for input output or unread resize events
    And a failed restoration of one distinct terminal does not prevent exact restoration of the other usable terminal
    And owned stop drain and cleanup share the original budgets without duplicate signals or late resizes
    And the healthy terminal control returns zero restores exact settings and confirms owned cleanup

    Examples:
      | fault                              | child   | rule                      | result |
      | terminal observation fails         | unknown | run.terminal-unavailable  | 75     |
      | resized terminal has zero rows     | 17      | run.terminal-unavailable  | 17     |
      | running worker resize fails        | 0       | run.terminal-resize       | 75     |
      | running worker resize times out    | unknown | run.call-timeout          | 75     |
      | terminal output blocks during exit | 17      | run.stream-timeout        | 17     |
      | terminal input read fails          | unknown | run.execution-unavailable | 75     |
      | saved terminal restoration fails   | 0       | run.terminal-restore      | 75     |
      | saved terminal restoration fails   | 17      | run.terminal-restore      | 17     |
      | distinct stdin restoration fails   | 0       | run.terminal-restore      | 75     |
      | distinct stdin restoration fails   | 17      | run.terminal-restore      | 17     |
      | distinct stdout restoration fails  | 0       | run.terminal-restore      | 75     |
      | distinct stdout restoration fails  | 17      | run.terminal-restore      | 17     |

  # Host signals are deliberately different from typing a control byte. Deliver
  # only to this child controller after ready; an external SIGINT still cancels
  # while host ISIG is off. The worker's handler exit remains authoritative.
  @slice-37 @ID-RUNTTY-12 @engine @linux
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
  # In distinct-terminal rows close only the named master; leave the other pair
  # usable to prove its exact restoration. Stdout loss while idle needs passive
  # observation, even though only stdin supplies worker dimensions.
  @slice-37 @ID-RUNTTY-13 @engine @linux
  Scenario Outline: Losing the terminal client stops its owned workload even without stream activity
    Given a CI terminal-run fixture with a "<probe>" probe and "<input>" terminal input
    And the CI fixture binds stdin and stdout to <terminals> terminals with saved nondefault settings
    When terminal run reaches the ready frame and the harness closes the <lost> terminal master while leaving the engine reachable
    Then terminal run reports "run.terminal-disconnected" and returns authoritative workload exit <exit> within bounded teardown
    And the worker receives one owned SIGTERM with escalation "<escalation>" at the original grace deadline only while still owned and running
    And terminal restoration is attempted and any failure is reported separately without blocking cleanup
    And every still-usable host terminal regains its own exact saved state
    And independent inspection confirms owned worker and anchor removal while checkout edits approval and foreign resources survive

    Examples:
      | terminals | lost   | probe            | input       | escalation    | exit |
      | shared    | shared | SIGTERM-handling | idle        | no SIGKILL    | 17   |
      | shared    | shared | signal-ignoring  | idle        | one SIGKILL   | 137  |
      | shared    | shared | signal-ignoring  | unread byte | one SIGKILL   | 137  |
      | distinct  | stdin  | SIGTERM-handling | idle        | no SIGKILL    | 17   |
      | distinct  | stdout | signal-ignoring  | idle        | one SIGKILL   | 137  |

  # The lease's hangup subscription ends with the disconnect report, before
  # owned cleanup finishes. A shell that resends SIGHUP to its jobs after the
  # window hangs up must not kill the controller mid-teardown: the run still
  # reports the disconnect, restores and completes owned removal. The fixture
  # holds the revalidated worker's wait briefly so the late signal provably
  # lands while teardown is still running.
  @slice-37 @ID-RUNTTY-22 @wip @linux @local-engine-fixture
  Scenario: A late hangup signal cannot kill the controller during owned teardown
    Given a local terminal-run fixture with shared terminal stdin and stdout
    When the fixture closes the client master and delivers SIGHUP after owned teardown has begun
    Then terminal run reports "run.terminal-disconnected" and returns the workload exit 17
    And both caller terminal handles remain usable and only owned resources are confirmed absent

  # Disconnect cannot manufacture an exit or ownership authority. The cleanup
  # failure row retains observed 17, then a fresh status/stop after the fixture
  # clears removal proves recovery. The foreign-identity row never removes that
  # resource, even on retry; the engine-loss row recovers the original resource
  # only after connectivity returns. Each When must observe a ready TTY run
  # before inducing disconnect; Givens only schedule those future observations.
  @slice-37 @ID-RUNTTY-14 @linux
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

  # Regression probes use only disposable host PTYs and the local recorder.
  @slice-37 @ID-RUNTTY-17 @linux @local-engine-fixture
  Scenario Outline: Controlling-terminal aliases restore one shared terminal exactly
    Given a local terminal-run fixture with shared terminal stdin and stdout
    When the fixture runs with <endpoint> addressed through the controlling-terminal alias
    Then terminal run returns the observed exit 0 with exact restoration of each host terminal's own saved state
    And the terminal invocation uses raw attachment output on stdout and separate controller diagnostics on stderr
    And both caller terminal handles remain usable and only owned resources are confirmed absent

    Examples:
      | endpoint |
      | stdin    |
      | stdout   |

  @slice-37 @ID-RUNTTY-18 @linux @local-engine-fixture
  Scenario Outline: Final terminal output stays raw while owned cleanup proceeds
    Given a local terminal-run fixture with <terminals> terminal stdin and stdout
    When the fixture delivers a final raw output frame after workload exit during owned removal
    Then terminal stdout contains the exact initial and final raw frames
    And terminal run returns the observed exit 0 with exact restoration of each host terminal's own saved state
    And both caller terminal handles remain usable and only owned resources are confirmed absent

    Examples:
      | terminals |
      | shared    |
      | distinct  |

  @slice-37 @ID-RUNTTY-19 @linux
  Scenario Outline: Restoration diagnostics cannot extend the original teardown deadline
    Given a terminal-run contract fixture with blocked restoration diagnostics and authoritative exit "<child>"
    When terminal run reaches the fault and finishes through the applicable local backends alongside a healthy control
    Then terminal run returns <result> within the remaining original teardown budget despite blocked restoration diagnostics
    And the healthy terminal control returns zero restores exact settings and confirms owned cleanup

    Examples:
      | child | result |
      | 0     | 75     |
      | 17    | 17     |
