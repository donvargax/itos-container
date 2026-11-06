@phase-1 @linux
Feature: Foreground failures retain owned cleanup authority
  Slice-10 review regressions exercise the product binary against a local HTTP
  engine fixture. They prove controller decisions, not native signal delivery
  or isolation; slice-12 and slice-14 retain their CI runtime proof obligations.
  Use the existing SDK and standard-library signal/stream support, with no dependency.
  Forward the first SIGINT or SIGTERM once, allow 10 seconds, then SIGKILL.
  Repeated signals do not interrupt the 30-second owned teardown budget.
  An observed nonzero worker exit wins; otherwise interrupted execution fails.
  Observed exit drains trailing output within the uncanceled teardown budget,
  including after interruption. A worker may exit without consuming all stdin;
  a closed engine input connection must not hide its observed exit or output.
  Host input read failures and output forwarding failures still fail execution.
  Launch still requires fresh checkout identity. Removal requires protected
  session/snapshot, current host user, original daemon and full resource identity,
  independently of later changes to the writable checkout.

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

  @wip @slice-10 @ID-OFFLINE-20
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

  @wip @slice-10 @ID-OFFLINE-21
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
