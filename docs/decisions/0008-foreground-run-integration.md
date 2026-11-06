# Foreground run integration

The arc42 section “Foreground run and its failure paths” described the lifecycle
but still said the ownership implementation and evidence were absent. This
increment updates that section and replaces run's temporary startup refusal.

Keep slice-9's unstarted-only engine capability and cleanup restrictions intact.
A separate session Runner composes the same durable intent store with a run-capable
Docker SDK connection. It pins the approved workspace, creates with literal argv,
records the independently inspected full ID, checks the actual runtime boundary,
attaches before start, observes exit, drains streams and confirms owned cleanup.
The frozen binding supplies workspace/environment; current checkout requests and
approval cannot change a selected session. No new dependency is added.

Override image entrypoint, command, identity and working directory. Disable image
health checks and the daemon's optional init wrapper so no image/helper process
precedes the supplied workload. Disable daemon workload logging; the attached
streams are the deliberate output channel. Preserve approved image environment
except HOME/TMPDIR, which always use the worker tmpfs. Check the actual create
result before starting and retain the original protected reference on uncertainty.

Startup and teardown use separate bounded contexts. Waiting for an acknowledged
workload exit is not a workload lifetime limit. No status/stop retry is advertised;
those interfaces, cancellation and controller-loss proof remain slice-11/12 work.

The coordinator supplied assertion-level red evidence for ID-OFFLINE-06/07/13
at ba71f14 in CI run 42138046011 on 2026-11-06. This increment activates those
three cases. The other slice-10 scenarios retain their existing WIP status and
still require their own step definitions and red/green evidence. Local HTTP
fixtures cover the superseded startup-refusal scenarios without claiming native
runtime isolation. Only the outer loop runs the engine tier and changed-code proof.
