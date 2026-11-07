# Slice-19: refuse existing checkout sockets and defer continuous enforcement

The person settled q-30 on 2026-11-07 at approximately 01:50 UTC. This decision
overrides slice-19's earlier requirement that a checkout containing host Unix
sockets must run successfully while all socket communication is denied.

For milestone 1, run must inspect the checkout before container creation and
refuse any Unix socket it finds, at any depth, including `.git`. Traversal must
not follow symlinks outside the checkout. Refusal uses the existing typed
isolation outcome (`run.isolation-unverified`, exit 1); no worker is created.
The checkout bind, literal argv, approved image and private worker IPC remain.

This is a startup check only. Host sockets created or swapped into the checkout
after inspection, including during a run, remain reachable from the worker.
The scan does not close that race or continuously isolate host IPC. Security
guidance, the threat model and run's isolation help must state this limit.

This overrides arc42's **Constraints**, **Context and scope**, and **Foreground
run and its failure paths** where their host/network isolation target would
otherwise imply continuous denial of checkout host sockets. The startup check
belongs before **Create and own**, with filesystem access behind an adapter
port. No kernel bootstrap, alternative runtime, new mount or dependency is
authorized for this milestone.

Reword ID-SOCKETS-01 as startup refusal and obtain one fresh guarded CI red run
before implementation. Its earlier connectivity red remains historical evidence,
not a substitute for this requested run. Keep ID-SOCKETS-02/03 `@wip`, remove
their slice-19 tags and retain their assertions for the deferred
`p1-checkout-socket-kernel-enforcement` work in milestone 2. The
[research handoff](../plans/checkout-socket-enforcement.md) retains Landlock and
gVisor questions without selecting or implementing either.
