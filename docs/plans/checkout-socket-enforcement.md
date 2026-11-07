# Slice-19 enforcement handoff

## Status and accepted evidence

This is an implementation constraint and proposal for the coordinator, not an
accepted architecture decision. Product denial is not implemented. The three
[socket scenarios](../../features/sockets.feature) retain their assertions and
`@wip` tags until an implementation exists.

The coordinator accepted these engine runs; they were not executed locally:

| Scenarios | Run and tested revision | Accepted failure |
| --- | --- | --- |
| ID-SOCKETS-01 and ID-SOCKETS-03 | [42312676619](https://github.com/donvargax/itos-container/actions/runs/42312676619), `1662753` | The worker connected/sent to host sockets, including all five operations and nested paths. |
| ID-SOCKETS-02 | [42323471709](https://github.com/donvargax/itos-container/actions/runs/42323471709), `69b02a8` | The worker reached sockets published after launch, with working positive controls. |

Earlier startup failures and the interrupted private seqpacket control were not
accepted red evidence. They do not require another dispatch now. Implementation
and ordinary green engine CI, without the red selector, remain outstanding.

## Cause in the current product

[`CreateRun`](../../internal/engine/session/docker/run.go) bind-mounts the entire
checkout at `/workspace`, read/write, and launches the supplied argv directly.
Its only security option is `no-new-privileges`; network and IPC namespaces do
not remove the host pathname socket inodes from that bind. The accepted engine
failures demonstrate actual access, including sockets created after launch.
`VerifyRun` and `safeHost` inspect the configured boundary but currently require
no kernel enforcement for these socket operations.

Scanning, masking known socket paths, changing permissions, or rejecting the
checkout would not meet the specified late-publication/replacement cases and
successful file-edit controls. A blanket socket/connect/send ban would also
break the required worker-private controls.

## Enforcement and startup constraint

The spec preserves the existing bind and approved image, forbids extra mounts
and privilege, and says an image entrypoint/helper must not precede the probe.
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
kernel silently, or count startup refusal as a successful socket scenario.

## Proposed coordinator decision

Select and authorize a trusted pre-exec enforcement path. The proposed direction
is mandatory kernel Landlock enforcement with ABI 9 or later, installed before
any untrusted instruction, followed by execution of the unchanged user argv.
The kernel policy, not cooperation by the workload, must hold the boundary.

That choice must specify how the trusted installer reaches the worker without
being replaceable through the writable checkout, how it fits the approved-image
and no-extra-mount constraints, and whether the prohibition on a preceding helper
is amended or enforcement is instead supplied by a reviewed container runtime.
It must also select a CI host that actually supports the required ABI, define
verification and the existing isolation-refusal mapping, and preserve cleanup,
literal argv, file edits and private IPC. No runtime/helper dependency or changed
image is selected by this note.

After that decision, record the architecture override, implement behind the
session ports, test refusal and verification with fakes/adapter fixtures, remove
`@wip`, and obtain all three ordinary engine passes plus neighboring offline-run
checks. The accepted red evidence remains valid. Until then, the socket escape
is still present; no production mitigation or support claim is made.
