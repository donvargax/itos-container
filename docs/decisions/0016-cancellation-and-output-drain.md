# Cancellation outcomes and concurrent final output drain

This updates arc42's “Foreground run and its failure paths”, “State and result
interpretation” and “Bounded operations”. It supersedes 0009's provisional
execution-unavailable result for cancellation and serial drain-before-removal.
The coordinator's slice-12 decisions retain slice-11 as the dependency and shared
termination implementation. LIFECYCLE-15 remains with slice-15 after slice-14.

Registered run passes the original cancellation grace deadline through shared
recovery. The first signal is sent once to the freshly revalidated owned primary;
delivery time consumes the ten seconds. A lost acknowledgement cannot repeat that
signal. Only fresh owned running state permits escalation at the original deadline.
Explicit stop retains its established slice-11 behavior and thirty-second total.

Cancellation is reported separately with its first signal. Every observed nonzero
exit wins. Observed zero with successful streaming and cleanup returns zero; an
unknown exit retains wrapper 130 for SIGINT or 143 for SIGTERM. Neither a signal
request nor removal invents a primary exit. A fresh engine wait may supply an
authoritative result when the worker exited before cleanup's first inspection.
Transport loss reports `run.engine-lost` and unknown outcome unless an exit was
separately observed. Existing classified ownership and isolation refusals retain
precedence over later signals.

Final output drains concurrently with owned stopping and removal. A blocked drain
expires within ten seconds and reports `run.stream-timeout`, separately from the
workload exit and cleanup. On expiry it closes the owned attachment immediately,
so engine exit observation cannot remain blocked on an unread output sink until
cleanup returns. Closing the attachment does not wait for caller I/O; caller-owned
stream handles are never closed. Diagnostic
writes use only the original teardown's remaining time; an unread stderr cannot
keep the controller alive. The existing seven-field recovery record retains a
safe typed stream-timeout in `last_problem`, including after confirmed cleanup.
This does not reopen a stopped session or change its immutable workload result.

This also clarifies arc42's “Recovery” admission rules alongside decision 0014.
A completed admission certificate remains valid when its matching stopped public
record retains `stream-timeout`. Admission still refuses mismatched intent,
non-stopped state, and every other retained failure beside a completion certificate.
The output diagnostic neither releases an incomplete reservation nor invalidates
durably completed resource cleanup. The preservation audit uses the same specified
distinction while continuing to check the exact certificate's identity and safety.

A reconnect that reports the local engine unavailable is retryable recovery
failure. When protected state remains writable, retain failed progress and the
original unknown workload result, so a restored status reports the surviving
worker under the same session binding and a fresh stop can finish cleanup.

The arc42 work-item table follows q-11's settled order: slice-12 consumes
slice-11's shared termination contract. Controller-loss isolation evidence is
slice-15's LIFECYCLE-15, after slice-12 and slice-14.

OFFLINE-19 and OFFLINE-20 now expect observed zero to win and require the separate
cancellation diagnostic. Their signal-count, escalation, trailing-byte, nonzero
exit and owned cleanup assertions remain. Local tests replace the superseded
serial-drain assertion with a blocked-drain check that observes removal and lock
availability before drain completes.

The step checkpoint d0579bf supplied local LIFECYCLE-14 red on both outline rows
at the typed stream-timeout assertion. The implementation passes both rows with
port fakes and the protected-record adapter over memory storage. Native red comes
from coordinator runs 42297748701 and 42304766391; 11 and 16 remain accepted
slice-11 regressions. Native green and changed-code proof belong to CI at the
pushed head. This checkpoint does not claim completion of the remaining local
preflight, accepted-mutation and finite-call scenarios, or runtime host support.

No dependency or record schema is added. The Linux-only registry interprets the
selected Linux root with slash-path rules, so portable fake-backed recovery checks
do not accidentally use Windows host path semantics. Linux diagnostic signal
numbers stay behind the existing Linux value check while all test builds compile.
