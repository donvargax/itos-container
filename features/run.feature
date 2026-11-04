@phase-1
Feature: Run invocation refusals
  Run requires an explicit nonempty executable after -- and refuses wrapper JSON
  before attempting startup. These local cases introduce the run command; they
  prove no container isolation or successful workload execution.

  # Authorities: docs/plans/offline-run-lifecycle.md, Arguments and JSON (T-11,
  # AC-21/22), docs/CLI.md, and docs/architecture/arc42.md, slice-7. The future
  # implementation belongs in internal/slices/run under the Slices rules. Reuse
  # the pinned kong v1.16.1 and generic UI; no engine client belongs in this item.
  # Domain tests supplement these black-box cases with I/O-free port fakes and
  # zero resource calls; a recorder alone is not runtime isolation evidence.
  # All cases are local, without @engine. Core dispatch is represented by its
  # inherited ITOS_JSON=1 at the extension entry point, never the installed itos.
  # Keep every scenario @wip for the implementer's red step-definition commit.
  # Undefined steps are not red evidence. Remove @wip for assertion-level runs
  # only after those steps exist, before implementing, then demonstrate green.

  # Existing policy fixtures are reused only as harness infrastructure. They
  # create private host directories, a TCP connection recorder (no daemon), and
  # harmless git/docker/devcontainer/sh/bash/npm executables that record calls.
  # PATH names only those executables; the generated manifest contains a host
  # hook sentinel, not permission to run it. No sandbox policy or approval exists.
  # The snapshot records checkout/host-state bytes and modes after setup. These
  # Givens only prepare fixtures; all effect checks are explicit Then steps.
  Background:
    Given policy inspection has isolated host state and a recording engine endpoint
    And inspection helper sentinels are installed
    And the policy inspection files are snapshotted

  # Required-command guidance must beat the old generic unknown-command error
  # and missing-policy errors. A later nonempty argument cannot replace an empty
  # executable. NEW assertion: "its standard output is empty" checks zero bytes.
  @ID-RUN-01 @wip @slice-7
  Scenario Outline: Run requires a nonempty executable after the separator
    When itos-container runs with "<arguments>"
    Then its error output says "run requires a nonempty command after --"
    And its error output says "itos container run -- <command>"
    And its error output says "itos-container:"
    And it exits with code 2
    And its standard output is empty
    And the engine recorder has no connections
    And no inspection helper sentinel was executed
    And the policy inspection files are unchanged

    Examples:
      | arguments     |
      | run           |
      | run --        |
      | run -- ''     |
      | run -- '' git |

  # T-11 requires the separator, not merely a positional command. Do not infer
  # a shell or silently move arguments across the missing separator.
  @ID-RUN-02 @wip @slice-7
  Scenario: Run refuses a command supplied without the separator
    When itos-container runs with "run git status"
    Then its error output says "run requires a nonempty command after --"
    And its error output says "itos container run -- <command>"
    And its error output says "itos-container:"
    And it exits with code 2
    And its standard output is empty
    And the engine recorder has no connections
    And no inspection helper sentinel was executed
    And the policy inspection files are unchanged

  # Existing When steps capture redirected streams. Redirection never enables
  # batch execution. Wrapper options are accepted on either side of "run", up
  # to --. A workload --no-json cannot cancel a wrapper or inherited request.
  # NEW assertion below parses the entire stdout as exactly one JSON object,
  # with schema 1, ok false, and exactly one problem with the given rule and
  # numeric exit. Require nonblank message and fix; message explains run's JSON
  # refusal and fix directs removal of both --json and ITOS_JSON=1. Allow additive
  # fields per CLI.md, but no other stdout or second document. Wording is human
  # guidance, not byte-for-byte JSON serialization. Stderr stays separate.
  @ID-RUN-03 @wip @slice-7
  Scenario Outline: Run refuses wrapper and inherited JSON with redirected streams
    When itos-container runs with "<arguments>" and the environment "ITOS_JSON=<inherited>"
    Then its run JSON reports exactly one refusal with rule "run.json-unsupported" and exit 2
    And it exits with code 2
    And its error output says "itos-container:"
    And its error output says "run"
    And its error output says "JSON"
    And the engine recorder has no connections
    And no inspection helper sentinel was executed
    And the policy inspection files are unchanged

    Examples:
      | arguments                  | inherited |
      | --json run -- git           | 0         |
      | run --json -- git           | 0         |
      | run -- git                  | 1         |
      | run --json -- git --json    | 0         |
      | run --json -- git --no-json | 0         |
      | run -- git --json           | 1         |
      | run -- git --no-json        | 1         |
      | --json run -- git --no-json | 1         |

  # NEW When step: the same literal argv/environment as runWith, but stdin and
  # stdout are a real pseudoterminal; stderr is captured independently. Allocate
  # and verify the terminal before launching the binary, drain its output, and
  # close/reap it with a bounded harness deadline. Do not fake TTY detection or
  # merge stderr into the JSON channel. A fixture failure is a failure, no skip.
  # Select github.com/creack/pty v1.1.24 for this test-only fixture on the initial
  # Linux target: maintained PTY allocation avoids handwritten ioctl/session
  # plumbing. Preserve its MIT notice; pin it when the implementer adds steps.
  # Source: https://github.com/creack/pty/releases/tag/v1.1.24 . Existing pinned
  # golang.org/x/term v0.46.0 can verify terminal descriptors. This selection
  # adds no runtime dependency, engine access, or additional host support claim.
  @ID-RUN-04 @wip @slice-7
  Scenario Outline: Run refuses JSON at a terminal as it does with redirected streams
    When itos-container runs at a terminal with "<arguments>" and the environment "ITOS_JSON=<inherited>"
    Then its run JSON reports exactly one refusal with rule "run.json-unsupported" and exit 2
    And it exits with code 2
    And its error output says "itos-container:"
    And its error output says "run"
    And its error output says "JSON"
    And the engine recorder has no connections
    And no inspection helper sentinel was executed
    And the policy inspection files are unchanged

    Examples:
      | arguments                  | inherited |
      | --json run -- git           | 0         |
      | run --json -- git --no-json | 0         |
      | run -- git --no-json        | 1         |

  # Interface choice for competing invocation failures: the requested JSON
  # refusal wins over missing argv/separator. Emit the single T-11 refusal,
  # never a usage object, combined problem list, or a plain-text-only error.
  @ID-RUN-05 @wip @slice-7
  Scenario Outline: A JSON request is refused even when the command is missing
    When itos-container runs with "<arguments>" and the environment "ITOS_JSON=<inherited>"
    Then its run JSON reports exactly one refusal with rule "run.json-unsupported" and exit 2
    And it exits with code 2
    And its error output says "itos-container:"
    And its error output says "run"
    And its error output says "JSON"
    And the engine recorder has no connections
    And no inspection helper sentinel was executed
    And the policy inspection files are unchanged

    Examples:
      | arguments        | inherited |
      | --json run       | 0         |
      | run --json --    | 0         |
      | run --json -- '' | 0         |
      | run              | 1         |
      | run --           | 1         |
      | run -- ''        | 1         |
      | run --json git   | 0         |

  # The registry and arc42 explicitly require valid argv to refuse until later
  # increments. Interface choice: classify unavailable offline startup as a
  # missing requirement (exit 3), explain that it is not implemented, and point
  # to policy-only doctor as inspection, never as permission or a run fallback.
  # This interim refusal precedes policy/manifest/approval access. Later startup
  # slices must explicitly supersede this interim contract in their specs and
  # behaviour commits; invocation refusals above remain required.
  @ID-RUN-06 @wip @slice-7
  Scenario: A valid command refuses while offline startup is not implemented
    When itos-container runs with "run -- git status"
    Then its error output says "offline startup is not implemented"
    And its error output says "doctor --policy-only"
    And its error output says "itos-container:"
    And it exits with code 3
    And its standard output is empty
    And the engine recorder has no connections
    And no inspection helper sentinel was executed
    And the policy inspection files are unchanged

  # Acceptance of workload flags must reach the same interim refusal, not help,
  # JSON, flag validation or empty-command handling. ITOS_JSON is cleared by the
  # clean harness environment. This proves option ownership only; actual argv
  # preservation (including empty arguments) and worker execution belong to the
  # later CI real-engine run slice, not to this refusal-only increment.
  @ID-RUN-07 @wip @slice-7
  Scenario Outline: Arguments after the separator belong only to the workload
    When itos-container runs with "<arguments>"
    Then its error output says "offline startup is not implemented"
    And its error output says "doctor --policy-only"
    And its error output says "itos-container:"
    And it exits with code 3
    And its standard output is empty
    And the engine recorder has no connections
    And no inspection helper sentinel was executed
    And the policy inspection files are unchanged

    Examples:
      | arguments                     |
      | run -- git --json              |
      | run -- git --no-json           |
      | run -- git --help              |
      | run -- git --version           |
      | run -- git --bogus             |
      | run -- git -- '' 'two words' - |
