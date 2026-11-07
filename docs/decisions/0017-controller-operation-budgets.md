# Whole-invocation deadlines and finite controller operations

This updates arc42's building blocks, “Foreground run and its failure paths”
and “Bounded operations”. It completes the local budget and preflight portion
left pending by 0013. The coordinator's q-11 order and accepted native evidence
remain unchanged: slice-12 consumes slice-11; slice-15 owns LIFECYCLE-15 after
slice-12 and slice-14.

The run command starts its 120-second startup scope before preflight. Signal
capture in cmd covers that inspection, frozen publication, launch, teardown and
diagnostic delivery. Readiness and approval expose contextual inspection without
changing doctor's existing inspection contract. Cancellation stops the next
startup phase and reports the first signal as 130 or 143 when no workload exit
was observed. Attachment alone does not establish that start was attempted.
An accepted create or start retains its original protected identity even when
its response is lost. No timed-out mutation is retried.

The shared `internal/engine/budget` engine owns phase and finite-call deadlines.
Its ports carry the retained handle lifetime and safe typed timeout names. It
uses the already selected standard-library context and monotonic time APIs;
there is no new dependency or configurable duration. Each finite operation is
capped at the lesser of ten seconds and the remaining phase. Monotonic deadline
comparison makes phase expiry win at a coincident call deadline, even before
the timer goroutine publishes cancellation. Successful progress never renews
startup. Acknowledged foreground waits remain outside that startup lifetime.

Handle establishment has a shorter context than the acquired handle's phase
lifetime. Both SDK requests and storage handles preserve that distinction.
Readiness's native SDK adapter accepts a context for each request; cancellation
reaches the HTTP request, rather than merely abandoning its response. The same
context contract runs against its memory fake and a local Unix HTTP fixture.

An uninterruptible filesystem operation can finish after the controller stops
waiting. Its result cannot resume launch or replace the reported cause. Late
acquired handles are closed. A pending storage operation keeps its lock until
it returns, so another controller cannot overlap a publication whose result is
unknown. Visible records after failed fsync remain visible-but-unconfirmed;
the wrapper preserves the adapter's observation without claiming durability or
overwriting a completed tombstone. A fresh recovery invocation revalidates it.

Status retains its ten-second total; stop retains its thirty-second total.
Lock acquisition may make several ten-second attempts within the same stop
deadline. These are lock observations, never repeated signal, create, start or
removal mutations. Foreground teardown still uses slice-11's shared termination
contract, with the original grace and teardown deadlines.

OFFLINE-24 now requires typed cancellation while inspection remains blocked,
preserving its prompt abort, zero snapshot publication and zero engine mutation
assertions. The concurrent-stop fixture retains its eleven-second lock hold
using two healthy calls of six and five seconds, preserving every assertion
while respecting the finite-call limit.

Local step checkpoints are df8b8e6, 9b9ebd3 and e67210a. Assertion-level red
covered preflight/intent stalls, startup reset, start timeout classification,
pre-launch cancellation, accepted-create cancellation and stop's finite calls.
LIFECYCLE-04's complete rows also pass against 41529b1 after collecting already
dispatched diagnostic writes without advancing time; they retain slice-10's
teardown/exit controls, slice-11 recovery and the earlier slice-12 timeout
reporting. The accepted-start row of 07, both 10 rows and 05's status rows are
verified regressions. All 37 local rows of 01–07, 10 and 14 pass together.
The coordinator supplied all 18 native cases green in run 42312330532 at
41529b1. Fresh CI and changed-code proof at the new pushed head remain the
coordinating loop's responsibility; this decision adds no host-support claim.
