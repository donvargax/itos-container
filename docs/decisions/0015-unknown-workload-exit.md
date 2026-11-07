# Unknown workload exit after completed cleanup

Slice-24 implements [RUNEXIT-01/02](../../features/run_exit.feature). This decision
overrides arc42's **Recovery** section and decision 0010's **Review corrections
for deadlines and concurrent completion** where successful reconciliation allowed
run to return zero without observing the workload exit. Cleanup completion proves
resource removal, not command success.

The session lifecycle already carries exit observation separately from cleanup.
Keep that distinction through output draining and reconciliation, including a
concurrent stop that outlasts the ten-second stdin probe. At the run command
boundary, if no other failure determines the result and no workload exit was
observed, return wrapper exit 75 with `run.workload-exit-unknown`. Explain that
the workload exit is unknown and cleanup is complete. Print no cleanup retry
or automatic rerun advice. The existing launch, forwarding, ownership and cleanup
classifications retain precedence; an observed nonzero workload exit still wins,
including 75. An observed zero with successful forwarding and cleanup stays zero.

The fallback is presentation only. Do not persist it as a workload exit or last
problem, change completed record/intent bytes, repeat engine mutations, or change
stop/status success. No new dependency, budget, authority or platform claim is
introduced. Existing ports, codecs, fakes and the pinned Go standard library suffice.

The step checkpoint is `0ca41f9`. With RUNEXIT-01/02 temporarily live before
implementation, both Given/When pairs passed and both first Then assertions
failed with foreground exit 0 instead of wrapper 75. Both pass after the fix,
including separate trailing stdout/stderr, unchanged completed authority, repeated
stop and status, and independent controls for observed exits 0, 7 and 75 and
forwarding failures with unknown or observed exit 7. The delayed case observes
the operation lock across probe expiry at second 10 and completion at second 11.

RECOVERY-32/35 stay live, with their superseded exit-zero expectations corrected
to the specified unknown-result diagnostic. Their cleanup, nullable exit, ownership,
bounded reconciliation and drain checks remain. CI alone supplies changed-code
proof and runtime evidence; these local stateful-fake scenarios claim neither.
