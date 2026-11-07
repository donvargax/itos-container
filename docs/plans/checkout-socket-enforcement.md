# Checkout socket enforcement: milestone scope and deferred research

## Status and accepted evidence

The person settled q-30 on 2026-11-07 at approximately 01:50 UTC: defer kernel
enforcement and document the gap for milestone 1. The
[accepted decision](../decisions/0019-checkout-socket-milestone-limit.md) supersedes
the earlier proposal to block this milestone on continuous socket denial.

Slice-19 now requires startup refusal before create for any Unix socket in the
checkout, including `.git` and arbitrarily nested directories, without following
symlinks outside the checkout. Use typed isolation refusal
(`run.isolation-unverified`, exit 1) and create no worker. Socket detection is
filesystem I/O behind a port, using Go 1.27.3's standard-library file type and
traversal APIs; reuse the existing pinned native APIs where needed. No new
library or runtime is selected. The scan must not delete or chmod host sockets.

Sockets created or swapped in after inspection, including during a run, remain
reachable. This is a known limit, not a continuous denial claim. State it in
`docs/security.md`, the pilot threat model and run's help beside the isolation
properties. Keep the checkout bind, approved image, literal argv and private IPC.

ID-SOCKETS-01 is reworded as refusal; obtain one fresh guarded CI red run of that
ID before implementing it, then remove its `@wip` and require ordinary green CI.
The outline isolates each socket type and location so a root socket cannot mask
an incomplete traversal. Require exit 1, the typed diagnostic and no create
request or worker ID. A different startup failure is not attributable red.
After that red is accepted, implement the scan and add the help disclosure in
the behavior commit. No production refusal or help change is present yet.

ID-SOCKETS-02 and ID-SOCKETS-03 retain their denial assertions and `@wip` tags,
but no longer belong to slice-19. They are deferred to milestone 2 under
`p1-checkout-socket-kernel-enforcement`. Do not dispatch them for this rescope or
make their continuous-denial assertions pass by accepting startup refusal.

The coordinator accepted these engine runs; they were not executed locally:

| Scenarios | Run and tested revision | Accepted failure |
| --- | --- | --- |
| ID-SOCKETS-01 and ID-SOCKETS-03 | [42312676619](https://github.com/donvargax/itos-container/actions/runs/42312676619), `1662753` | The worker connected/sent to host sockets, including all five operations and nested paths. |
| ID-SOCKETS-02 | [42323471709](https://github.com/donvargax/itos-container/actions/runs/42323471709), `69b02a8` | The worker reached sockets published after launch, with working positive controls. |

Earlier startup failures and the interrupted private seqpacket control were not
accepted red evidence. The connectivity reds above remain valid historical
evidence of the gap. Q-30 nevertheless requires a fresh red of the reworded
ID-SOCKETS-01 only; the older assertions are not its new refusal assertion.

## Cause in the current product

[`CreateRun`](../../internal/engine/session/docker/run.go) bind-mounts the entire
checkout at `/workspace`, read/write, and launches the supplied argv directly.
Its only security option is `no-new-privileges`; network and IPC namespaces do
not remove the host pathname socket inodes from that bind. The accepted engine
failures demonstrate actual access, including sockets created after launch.
`VerifyRun` and `safeHost` inspect the configured boundary but currently require
no kernel enforcement for these socket operations.

Startup refusal mitigates existing sockets only. Scanning, masking known paths
or changing permissions cannot satisfy the deferred late-publication/replacement
cases. A blanket socket/connect/send ban would break worker-private controls.

## Deferred milestone-2 enforcement and startup constraint

The original continuous-enforcement spec preserves the existing bind and
approved image, forbids extra mounts and privilege, and says an image
entrypoint/helper must not precede the probe.
Arc42's [foreground run](../architecture/arc42.md#foreground-run-and-its-failure-paths)
likewise verifies before launch and then starts the literal argv. There is no
trusted pre-exec enforcement hook in this product's current startup path.

The mechanisms examined do not supply a configuration-only fix within that path:

- Classic seccomp filters inspect syscall argument values, not the socket
  pathname behind a pointer. Consequently a Docker seccomp argument rule cannot
  distinguish checkout destinations from private `/tmp` destinations. This follows
  from the kernel's [seccomp filter interface](https://docs.kernel.org/userspace-api/seccomp_filter.html).
- AppArmor's documented fine-grained Unix rules cover abstract/anonymous sockets;
  pathname sockets use file-access rules. Denying writes to every checkout path
  would break regular edits. A peer-label rule alone is therefore not an
  established solution for these pathname cases. See the upstream manual shipped
  by Debian, [Unix socket rules](https://manpages.debian.org/trixie/apparmor/apparmor.d.5.en.html#Unix_socket_rules).
  A custom Docker profile also needs a host-loaded policy, as described in
  [Docker's AppArmor documentation](https://docs.docker.com/engine/security/apparmor/).
- Landlock ABI 9 adds `LANDLOCK_ACCESS_FS_RESOLVE_UNIX`, which restricts resolution
  of pathname sockets created outside the sandbox while allowing sockets created
  within the same domain. This matches the intended distinction and inherits to
  children. It requires successful enforcement in the worker's ancestry before
  the workload runs. Calling `landlock_restrict_self` in the controller cannot
  restrict a worker spawned by the separately running Docker daemon. See the
  kernel's [Landlock interface](https://www.kernel.org/doc/html/latest/userspace-api/landlock.html).

These are interface/design findings, not runtime proof. In particular, neither
the CI worker's available Landlock ABI nor an enforcement bootstrap has been
verified. The current engine job selects `ubuntu-latest` without a Landlock
capability prerequisite. Do not infer an ABI from that runner label, replace the
kernel silently, or count startup refusal as continuous socket denial.

## Landlock and gVisor research retained for milestone 2

Landlock ABI 9 or later remains a candidate for a trusted pre-exec path. Research
must specify how an installer reaches the worker without replacement through the
writable checkout, fits the approved-image/no-extra-mount constraints, and runs
before any untrusted instruction. Restricting only the host controller cannot
constrain a worker launched by the daemon. Verify the actual kernel ABI and
fail-closed behavior; the runner label alone supplies neither.

[gVisor](https://gvisor.dev/docs/) is an alternative application-kernel direction.
Its [Docker integration](https://gvisor.dev/docs/user_guide/quick_start/docker/)
uses an installed `runsc` runtime, so adopting it changes host/runtime requirements.
Research its [filesystem boundary](https://gvisor.dev/docs/user_guide/filesystem/),
host Unix socket options and defaults, and
[application compatibility](https://gvisor.dev/docs/user_guide/compatibility/).
Do not assume that selecting it proves pathname socket denial or equivalent
checkout/file-edit and private-IPC behavior. No gVisor dependency or version is
selected, installed or tested here.

The deferred item must select a mechanism, record any architecture override and
pin new dependencies before implementation. It needs runtime evidence for the
complete stream/seqpacket/datagram inventory, late publication, replacement,
parent and child processes, path aliases, file edits and private IPC, plus
startup refusal on hosts where enforcement is unavailable. Keep ID-SOCKETS-02/03
as regression specifications; restore a successful-run existing-socket denial
case when that behavior is authorized. Until then, no continuous socket boundary
or new host-support claim is made.
