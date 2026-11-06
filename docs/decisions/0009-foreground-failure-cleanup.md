# Foreground failure and cleanup authority

PR #5 exposed three ways foreground execution could leave an owned worker behind:
forwarding errors were hidden behind workload wait, catchable host signals killed
the controller, and changes to writable `.git` invalidated cleanup authority.
Stdin read errors could also be mistaken for successful EOF.

This decision updates arc42's “Foreground run and its failure paths” and “Recovery”
sections, and supersedes 0008's deferral of all cancellation handling. The settled
signal policy remains unchanged: forward the first SIGINT or SIGTERM once, allow
10 seconds of grace, then SIGKILL, within the shared 30-second teardown budget.
An observed nonzero workload exit wins. An interruption without such an exit uses
the existing execution-unavailable failure (75), including a handler exiting zero.
Repeated signals remain captured until owned teardown finishes. Slice-12 retains
native signal/cancellation/controller-loss proof and the remaining budget cases.

The run slice's signals adapter captures process signals; cmd binds its context to
the thin command handler. The session domain reacquires protected ownership and
the original daemon before sending either signal. Each signal and removal requires
fresh full-ID inspection and matching protected identity. Signal delivery alone
never establishes exit. Teardown retains its deadline across reconnection, grace,
escalation and removal. No new dependency or public recovery command is introduced.

Attachments expose forwarding failures independently of final output drain.
The domain monitors these while waiting for workload exit and cancels that wait on
failure, then attempts bounded cleanup. The SDK adapter preserves stdin read/write,
half-close and stdout/stderr forwarding errors. Successful EOF alone does not end
the workload. A drain cannot erase an already reported error.

Launch continues to validate checkout identity at all existing boundaries.
Run cleanup instead opens a protected-evidence transaction that validates the
original snapshot, record and effective host identity without opening the checkout.
Writable checkout identity cannot authorize or veto removal. The original selected
socket/daemon and full resource identity still must match; protected-state tampering
continues to refuse mutation. Slice-9's unstarted-only Open/cleanup remains unchanged.

ID-OFFLINE-17/18/19 are local black-box regressions using the real binary and an
HTTP Engine API fixture. At test commit 2ce7bec, output failures exceeded the harness
deadline, stdin failure returned zero, checkout changes returned cleanup-ownership,
and interrupts terminated the controller without a signal request. These are
controller observations, not native runtime or isolation evidence. The original
slice-10/13/14 scenario split remains in force.
