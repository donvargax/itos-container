@phase-1
Feature: Distinguish an unknown foreground exit from completed cleanup
  A successful stop confirms cleanup, not the result of the command that ran.
  Run must not report success when neither controller observed the workload exit.

  # Authorities: docs/charter.md, container-pilot.md Proposed command contract,
  # offline-run-lifecycle.md Workload exits, and arc42 Recovery.
  # Slice-24 corrects the concurrent-stop result only. No new cancellation policy,
  # resource budget, recovery authority, dependency or runtime support is added.
  # Reuse the pinned standard library, existing registry/engine ports and fakes;
  # command presentation stays in the run slice, shared lifecycle in session.
  #
  # Result decision: after a launched workload, confirmed cleanup with no observed
  # exit must return wrapper exit 75 and rule run.workload-exit-unknown on stderr
  # when no other failure already determines the result. Explain that the workload
  # exit is unknown and cleanup is complete. This is a controller result, never a
  # synthetic child exit. Do not claim failed cleanup, print a cleanup retry or
  # automatically rerun work to recover its result. Run still refuses wrapper JSON.
  # Existing launch, forwarding, ownership and cleanup failures retain their own
  # classification; the unknown-result fallback must not replace those causes.
  # A known nonzero workload exit keeps precedence even when it equals 75. A known
  # zero exit with successful forwarding and cleanup still returns zero.
  #
  # Completed cleanup remains successful: stop and read-only status return zero,
  # schema 1, ok true, no problems, state stopped, resource absent, cleanup complete,
  # workload_exit null and retry null. The protected record also retains null;
  # do not store wrapper 75, zero, a guessed signal exit or last_problem merely
  # because run cannot report a workload result. Preserve completed record bytes
  # and the absent intent, and do not repeat create/start/signal/remove mutations.
  # Drain remaining stdout and stderr within the existing teardown/drain budgets,
  # even though run must return nonzero. Output is not evidence of an exit status.
  #
  # Integration correction: RECOVERY-32 and RECOVERY-35 currently delegate to
  # publicRecoveryStoppedWait/publicRecoveryDelayedStop, whose unknown-exit cases
  # require exit 0 and empty stderr. This spec supersedes only that success claim.
  # In the implementation fix, update those expectations and their misleading
  # "without a wait failure" wording/comments; retain both live scenarios and all
  # their cleanup, null-exit, ownership, bounded-reconciliation and drain checks.
  # Record this correction to arc42 Recovery and decision 0010's concurrent-
  # completion explanation in a decision naming the overridden section, and
  # update that section in the same implementation PR. No sibling file is changed
  # by this spec commit.
  #
  # All step phrases below are NEW. Reuse recovery_review_steps_test.go's command
  # subprocess seam and public_recovery_review_test.go/public_recovery_budget_test.go
  # fixture machinery, not their exit-zero assertions. Exercise the real run,
  # stop and status command handlers, Runner, registry and record codecs over
  # stateful I/O-free port fakes. Use Go testing/synctest for deterministic time
  # and lock barriers. No real engine, socket, host approval or production test
  # switch; all fixture state is disposable. Existing dependencies suffice.
  #
  # Each Given only prepares independent approval/snapshot inputs, empty session
  # storage, engine observations, separate stream markers and a barrier schedule.
  # The When invokes the commands and captures exits, streams, records, operation
  # order and timing for Then. Run creates its own registration/intent; only the
  # product stop may remove the worker and publish stopped/absent completion.
  # Never seed a completed record, substitute a Runner result or make the old
  # expectation test's PASS line the oracle. Capture early refusals/timeouts as
  # results; successful setup and defined steps are required before claiming red.
  #
  # In the fault case, stdin closure interrupts run's pending wait, no successful
  # wait response reaches either controller, and fresh owned observation permits
  # stop to remove the exited worker without learning its exit. The original full
  # worker ID and authority remain valid. Completed-stop timing makes the durable
  # tombstone available before run reconciles stdin closure. Pending-stop timing
  # holds stop's operation lock across that probe's 10 s deadline and completes
  # at 11 s on the controlled clock, inside the existing totals. Verify the actual
  # ordering, not merely elapsed time; do not turn the timeout into a new budget.
  # Release distinct trailing stdout/stderr markers only after stop's completion,
  # so seeing each once on its original stream proves run actually drained them.
  #
  # Positive controls are earlier slice-10/11 behavior, not separate red claims.
  # For each schedule use fresh fixtures with a valid wait response observed by
  # run or stop before removal: 0, 7 and 75 must produce run exits 0, 7 and 75,
  # preserve that exact workload_exit and omit run.workload-exit-unknown. Also
  # inject a forwarding/drain failure with unknown exit: keep exit 75 and existing
  # run.execution-unavailable, not the unknown-result fallback; cleanup stays
  # complete and workload_exit null. With observed exit 7 the same failure keeps
  # run exit 7 and its separate forwarding diagnostic. Do not hand-write exits
  # into completed records or conflate a default numeric field with observation.
  #
  # Before implementation both first Then assertions are expected to fail:
  # stoppedRun returns no observed exit and no failure, and command.Run falls
  # through to zero after complete cleanup. Commit complete steps while scenarios
  # remain @wip, then temporarily remove @wip for local assertion-level red evidence.
  # No undefined
  # step or already-passing control counts as red. When activated, add a RUNEXIT
  # smoke selection as required by features/README.md; CI owns changed-code proof.

  # Immediate reconciliation must separate successful cleanup from workload success.
  @slice-24 @ID-RUNEXIT-01 @linux
  Scenario: Run reports an unknown exit after a concurrent stop has completed
    Given an unknown-exit run fixture with stop completion "before stdin reconciliation"
    When the fixture runs the foreground command and concurrent stop then repeats stop and inspects JSON status
    Then the foreground run returns wrapper exit 75 for the unobserved workload exit
    And run reports run.workload-exit-unknown with completed cleanup and no cleanup failure or retry advice
    And both stops and fresh status succeed with the same stopped absent session cleanup complete workload exit null and retry null
    And run and repeated recovery preserve the completed record bytes and absent intent without further engine mutations
    And run drains each trailing marker once on its original stdout or stderr stream
    And observed-exit and forwarding-failure controls retain their specified result precedence

  # Retry after probe expiry must not reintroduce zero or discard trailing output.
  @slice-24 @ID-RUNEXIT-02 @linux
  Scenario: Run reports an unknown exit when concurrent stop outlasts the stdin probe
    Given an unknown-exit run fixture with stop completion "after the stdin reconciliation deadline"
    When the fixture runs the foreground command and concurrent stop then repeats stop and inspects JSON status
    Then the foreground run returns wrapper exit 75 for the unobserved workload exit
    And run reports run.workload-exit-unknown with completed cleanup and no cleanup failure or retry advice
    And run observes stop completion at controlled second 11 after its initial 10 second probe expires within the existing teardown total
    And both stops and fresh status succeed with the same stopped absent session cleanup complete workload exit null and retry null
    And run and repeated recovery preserve the completed record bytes and absent intent without further engine mutations
    And run drains each trailing marker once on its original stdout or stderr stream
    And observed-exit and forwarding-failure controls retain their specified result precedence
