@phase-1 @linux
Feature: Foreground failures retain owned cleanup authority
  Slice-10 review regressions exercise the product binary against a local HTTP
  engine fixture. Deadline and trailing-output cases use the real command through
  a test-only cmd assembly for short budgets and drain barriers. They prove
  controller decisions, not native signal delivery
  or isolation; slice-12 and slice-14 retain their CI runtime proof obligations.
  Use the existing SDK and standard-library signal/stream support, with no dependency.
  Forward the first SIGINT or SIGTERM once, allow 10 seconds, then SIGKILL.
  Repeated signals do not interrupt the 30-second owned teardown budget.
  An observed nonzero worker exit wins; otherwise interrupted execution fails.
  Observed exit drains trailing output within the uncanceled teardown budget,
  including after interruption. A worker may exit without consuming all stdin;
  a closed engine input connection must not hide its observed exit or output.
  This also holds after the 120-second startup deadline; exit reconciliation
  has a fresh 10-second budget bound to the original protected session and daemon.
  Host input read failures and output forwarding failures still fail execution.
  Launch still requires fresh checkout identity. Removal requires protected
  session/snapshot, current host user, original daemon and full resource identity,
  independently of later changes to the writable checkout.
  A host interrupt never replaces a classified launch or wait refusal. Before
  owned launch, host signals retain their normal abort behavior: preflight must
  stop without publishing a snapshot or starting a worker.

  @slice-10 @ID-OFFLINE-17
  Scenario Outline: Host stream failures cannot strand the foreground worker
    Given an approval checkout and isolated host state
    And a seeded matching approval at generation "1"
    And a foreground recovery fixture for "<fault>"
    When the foreground recovery command runs
    Then the foreground recovery command reports stream failure and completes owned cleanup

    Examples:
      | fault  |
      | stdout |
      | stderr |
      | stdin  |

  @slice-10 @ID-OFFLINE-18
  Scenario Outline: Writable checkout changes cannot veto owned cleanup
    Given an approval checkout and isolated host state
    And a seeded matching approval at generation "1"
    And a foreground recovery fixture for "<fault>"
    When the foreground recovery command runs
    Then the foreground recovery command succeeds with confirmed owned cleanup

    Examples:
      | fault        |
      | replaced git |
      | missing git  |

  @slice-10 @ID-OFFLINE-19
  Scenario Outline: Foreground interrupts reach bounded owned teardown
    Given an approval checkout and isolated host state
    And a seeded matching approval at generation "1"
    And a foreground recovery fixture for "<fault>"
    When the foreground recovery command runs
    Then the foreground recovery command forwards once and completes bounded owned cleanup

    Examples:
      | fault           |
      | handled SIGINT  |
      | handled SIGTERM |
      | ignored SIGINT  |
      | ignored SIGTERM |

  @slice-10 @ID-OFFLINE-20
  Scenario Outline: Interrupted worker exit preserves trailing output
    Given an approval checkout and isolated host state
    And a seeded matching approval at generation "1"
    And a foreground recovery fixture with "<signal>" and trailing output after exit <exit>
    When the foreground recovery command runs
    Then the foreground recovery command preserves both trailing streams and the interrupted exit
    And the foreground recovery command has confirmed owned cleanup

    Examples:
      | signal  | exit |
      | SIGINT  | 0    |
      | SIGTERM | 0    |
      | SIGINT  | 23   |
      | SIGTERM | 23   |

  @slice-10 @ID-OFFLINE-21
  Scenario Outline: Worker exit does not require consuming all redirected stdin
    Given an approval checkout and isolated host state
    And a seeded matching approval at generation "1"
    And a foreground recovery fixture with exit <exit> and unread redirected stdin
    When the foreground recovery command runs
    Then the foreground recovery command preserves the worker exit despite unread stdin
    And the foreground recovery command has confirmed owned cleanup

    Examples:
      | exit |
      | 0    |
      | 23   |

  @slice-10 @ID-OFFLINE-23
  Scenario Outline: An interrupt preserves an already classified run refusal
    Given a foreground interrupt contract with "<outcome>" at "<phase>"
    When the foreground interrupt contract runs
    Then the foreground interrupt contract retains "<outcome>" with exit 1 and owned cleanup

    Examples:
      | phase  | outcome              |
      | launch | isolation-unverified |
      | launch | ownership-refused    |
      | wait   | ownership-refused    |

  @slice-10 @ID-OFFLINE-24
  Scenario Outline: Host signals abort preflight before snapshot publication
    Given an approval checkout and isolated host state
    And a seeded matching approval at generation "1"
    And the run preflight is held at fresh image inspection
    When the run receives "<signal>" during preflight
    Then preflight aborts before release without a snapshot or worker

    Examples:
      | signal  |
      | SIGINT  |
      | SIGTERM |

  # Hold an acknowledged workload wait beyond the launch connection's observed
  # deadline, shortened only by the test assembly. Domain tests use virtual time
  # to enforce the full production budgets. Closing stdin precedes the wait reply;
  # a drain barrier releases trailing output after the reconciled wait.
  @slice-10 @ID-OFFLINE-22
  Scenario Outline: Unread stdin after the startup deadline preserves exit and output
    Given an approval checkout and isolated host state
    And a seeded matching approval at generation "1"
    And a foreground recovery fixture with exit <exit> and unread stdin after the startup deadline
    When the foreground recovery command runs
    Then the foreground recovery command preserves the late worker exit and trailing streams
    And the foreground recovery command has confirmed owned cleanup

    Examples:
      | exit |
      | 0    |
      | 23   |
