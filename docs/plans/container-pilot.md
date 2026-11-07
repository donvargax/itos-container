# itos-container pilot contract

## Status and sources

Draft for review, 2026-10-09. This records the planning conversation; it is not an
implemented interface, executable policy, security certification or authorization to
run with credentials. No ledger items or GitHub updates accompany this draft.

The starting proposal is [issue #1](https://github.com/donvargax/itos-container/issues/1).
Its upstream evidence is pinned to itos revision
`517ea82f3d9493bfd966b7587d02be6921024091`:

- [Container extension design](https://github.com/donvargax/itos/blob/517ea82f3d9493bfd966b7587d02be6921024091/docs/plans/container-extension.md).
- [Existing devcontainer and limits](https://github.com/donvargax/itos/blob/517ea82f3d9493bfd966b7587d02be6921024091/.devcontainer/README.md).
- [Executable extension contract](https://github.com/donvargax/itos/blob/517ea82f3d9493bfd966b7587d02be6921024091/docs/extensions.md).

The additional local experiment is
`a local Claude Code devcontainer spike`. Its proxy separation
and verification scripts are useful starting points, not runtime isolation proof. The
shown worker can bypass proxy environment variables; Squid allows broad suffixes and
private destinations; the firewall is not wired into startup; and the image grants
passwordless sudo. No scripts or containers were run during this review. Preserve
applicable licenses and notices if implementation later reuses any source.

Dependency references:

- [Mitmproxy interception selectors](https://docs.mitmproxy.org/stable/howto/ignore-domains/),
  [event hooks](https://docs.mitmproxy.org/stable/api/events.html) and
  [streaming/blocklist limitations](https://docs.mitmproxy.org/stable/overview/features/).
- [Rancher Desktop Docker API requirements](https://docs.rancherdesktop.io/how-to-guides/using-testcontainers).
- [GitHub SSH over port 443](https://docs.github.com/en/authentication/troubleshooting-ssh/using-ssh-over-the-https-port).
- [Vite+ environment downloads](https://viteplus.dev/guide/env),
  [package management](https://viteplus.dev/guide/install) and
  [Docker image privileges](https://viteplus.dev/guide/docker).
- [Dev Container manifest reference](https://containers.dev/implementors/json_reference/),
  including image selection, lifecycle commands and inherited image metadata.

## Goal and limits

Run OpenCode or Claude Code CLI and all their executable tools in a local container
sandbox for one checkout. A person should be able to edit code, install project
dependencies, commit through prepared hooks, push through approved GitHub access,
inspect a failed CI run and expose an approved development port to their LAN.

The extension owns preparation, approval, container lifecycle, mounts, network policy
and isolated state. Core itos retains work routing, commit rules and CI proof.

Assume arbitrary code execution inside the worker, including hostile dependency scripts
and prompt-injected instructions. Trust the host user, host controller, approved images,
guard/proxy and local container engine. The checkout is intentionally writable and is
not protected from deletion or destructive edits. Containers are not a defense against
the host administrator or a kernel/container escape.

**Known first-milestone limit (q-30, 2026-11-07):** host pathname Unix sockets
created or swapped into the writable checkout during a run remain reachable
from the worker. Such a listener can expose host actions or networking despite
the worker's offline network namespace. Slice-19 is limited to refusing sockets
found before worker creation, at any depth including `.git`, without following
symlinks outside the checkout. That check is specified but not implemented yet
and cannot close the race after inspection. Continuous enforcement is deferred
to milestone 2 under `p1-checkout-socket-kernel-enforcement`; see the
[accepted scope decision](../decisions/0013-checkout-socket-milestone-limit.md)
and [research handoff](checkout-socket-enforcement.md). This is a known gap in
the isolation target, not a claim that startup scanning establishes it.

Hooks, Git configuration and agent instructions are workflow safeguards, not the network
or filesystem isolation boundary. Isolation must hold when the worker invokes real Git,
ignores proxy variables or runs a different program.

After the worker changes a checkout, its hooks, builds, plugins and dependency installs
must run inside the sandbox until the person reviews the changes. Host-side inspection
must not execute repository-configured helpers. `doctor` is not a host test/build runner.

Not included: cloud execution, remote engines, Kubernetes orchestration, linked
worktrees, extra host-directory mounts, host home or socket forwarding, arbitrary Docker
flags, model selection, work scheduling, Docker-in-Docker or full traffic capture.
The pilot has one project worker. Project services through Docker Compose are deferred;
the separate trusted guard/proxy remains part of itos's isolation infrastructure.

## Agreed scope

| Area | Planning decision |
| --- | --- |
| Proxy | Favor headless mitmdump with a trusted policy addon; do not start with Squid or build a new Rust proxy on presumed performance grounds. |
| TLS | Approve inspection or passthrough per HTTPS destination. Never downgrade automatically. |
| Approval | Separate host-side `approve` command; approval and session policy live outside the writable checkout. |
| Images | Project-supplied, digest-pinned worker and trusted guard/proxy images. |
| Manifest | A restricted, image-based devcontainer.json profile describes one worker. Go consumes the supported fields; no Dev Containers CLI dependency is required. |
| Project services | Defer Docker Compose support. The manifest cannot configure or replace the trusted guard/proxy. |
| Agents | Include OpenCode and Claude Code CLI in the worker. |
| Authentication | User-wide login sources with explicit project grants and project-isolated runtime state. Do not mount the sources directly. |
| Git | Include HTTPS/API access and SSH. Use a dedicated sandbox SSH identity, not the person's normal private key or host SSH agent. |
| Lifecycle | Foreground `run`; tear down session resources when the command exits, preserving edits and isolated state. |
| Ingress | No publishing by default; support explicitly approved development ports on loopback or a LAN address. |
| Mounts | The checkout is the only host filesystem mount into the worker. Generate or copy provisioned material into isolated container-managed storage; do not bind-mount additional host paths. |
| Platforms | Docker API first. Target Linux, macOS and Windows; Rancher Desktop with Moby is a first-class macOS pilot target alongside Docker Desktop, contingent on runtime evidence. |
| Logging | Safe policy/connection metadata only. No full URLs, headers, bodies, prompts or traffic dumps by default. |
| Tools | Go, Node.js, npm/pnpm, Vite+, Python/uv, Git/SSH, gh and itos. Exact versions remain to be selected. |

Support is a tested combination of host, engine and image architecture, not a property
of a compiled CLI. Native Docker on Linux, Rancher Desktop/Moby on macOS and Docker
Desktop on macOS/Windows require runtime evidence. AMD64 and ARM64 images need their
own validation; Apple Silicon cannot be covered solely by an AMD64 build. Rancher's
containerd mode, Podman and untested combinations remain unsupported initially.

## Restricted devcontainer manifest

Use a restricted devcontainer.json profile for the worker's development environment,
and keep sandbox permissions in itos-container.yaml. The manifest requests an image,
workspace layout, environment and supported in-container setup commands; it does not
grant permissions. itos supplies the guard/proxy, network topology, mounts, account
provisioning, security restrictions and foreground lifecycle.

The pilot supports an image-based worker only. Dockerfile builds, Dev Container
Features, dockerComposeFile/service/runServices and arbitrary Docker flags are excluded.
Later Compose support needs its own restricted service/network/mount contract; do not
pass checkout-supplied Compose files directly to the engine.

The following field profile is proposed; exact value constraints and lifecycle-command
semantics must be specified before implementation:

| Field group | Proposed handling |
| --- | --- |
| name and image | Use a display name and an explicitly approved, digest-pinned worker image. The manifest cannot substitute an unapproved image. |
| workspaceFolder | Permit only a validated location within the mounted checkout; itos selects the mount source and target. |
| containerEnv | Permit literal values subject to reserved-variable checks; no host environment interpolation or credential sourcing through the manifest. |
| containerUser/remoteUser | Support only configurations consistent with itos's approved non-root identity; no elevation or separate unrestricted user. |
| Selected in-container lifecycle commands | Run only after the isolation boundary is ready, under the worker identity and frozen permissions. Exact supported hooks, ordering, failure handling and repeat behavior remain open. |
| Recognized editor customizations | Explicitly report as unused by this headless runner. Do not install editor extensions or execute customization data. |

Reject unsupported execution/security fields with actionable errors before starting
resources. In particular, reject host-side initializeCommand, extra mounts,
workspaceMount, runArgs, privileged, capAdd and securityOpt. The manifest cannot change
session lifetime or publish ports independently of the sandbox policy. Do not interpret
forwardPorts as Docker publishing: the specification distinguishes forwarding from
publishing, and approved LAN bindings remain an itos concern.

Account for devcontainer.metadata image labels in the supported-field contract. Do not
automatically merge hidden permissions, commands or user settings into the runtime.
Define whether supported metadata is consumed or explicitly reported as unused, and
include every effective setting in approval. Check unsupported image metadata before
startup rather than validating only the visible checkout file.

Parse the supported JSON-with-comments manifest syntax with a reviewed library rather
than implementing the full Dev Containers CLI. Strict validation and variable-expansion
rules must be shared by doctor, approve and run. Files remain untrusted after approval:
startup uses a frozen effective configuration, not newly edited commands or values.
Missing or unsupported manifests must never trigger an unrestricted fallback.

## Proposed command contract

The executable is `itos-container`, dispatched through the existing `itos container`
extension seam. Use the documented effective root and configuration environment; do
not add container keys to core `itos.yaml`.

| Command | Proposed behavior |
| --- | --- |
| `init` | Prepare non-runnable example sandbox policy and a single-worker manifest without overwriting existing files. Exact output paths remain to be specified. Do not install hooks, log in, pull/build images or start resources. |
| `doctor` | Inspect strict sandbox policy and the supported manifest profile, the selected local engine, cached images/metadata, checkout layout, identity, preparation and approval. Do not execute repository code or change resources. |
| `doctor --probe` | Explicitly run bounded disposable runtime probes, with no agent or real credentials. Probes use trusted test material, not checkout-supplied scripts. |
| `approve` | Show the effective permissions, differences from the prior approval and weaker-mode warnings, then explicitly record host-user consent. No container startup or repository-code execution. |
| `run -- <argv...>` | Start an approved session and attach to the supplied command. Require argv for the pilot; do not evaluate an implicit shell or inject permission-skipping agent flags. |
| `exec <session> -- <argv...>` | Attach another command while the owned session's foreground workload is running, with the same frozen policy, identity, mounts and restrictions. Do not extend its lifetime. |
| `status` | Show owned sessions, state, effective policy identity and recovery information without secrets. |
| `stop <session>` | Stop and remove only that session's owned resources. Preserve checkout edits, account sources and project state. |

There is no detached/keep-alive mode in the pilot. When the foreground workload exits,
refuse new exec commands and tear down the session, including any active exec commands.
`status` inspects live sessions or interrupted cleanup; `stop` cancels a live session or
finishes that cleanup. Recovery records do not authorize restarting an agent unattended.

The exact configuration schema and account-management commands are not settled. Strict
parsing must reject unknown/duplicate fields, unsupported options and invalid values
before starting resources. An absent configuration must never create an unrestricted
default session.

For noninteractive control commands, follow itos's JSON convention: one object on stdout
with `schema: 1`, `ok` and typed problems; diagnostic logs go to stderr. Wrapper refusals
follow itos exit conventions. Proposed interactive `run`/`exec` behavior is to refuse JSON
mode rather than mix terminal output with a JSON document.

Once a workload starts, preserve its exit status. Report cleanup errors separately; if
the workload succeeds but required cleanup fails, return nonzero. Define timeout,
signal-forwarding and cancellation details in the scenarios before implementation.

## Approval and session state

The checkout YAML and devcontainer manifest request configuration; neither can authorize
itself. Approval binds the effective sandbox policy and supported manifest settings to
the host user and canonical checkout identity, selected local engine,
image digests, worker identity, account grants, writable state, ingress bindings and
network destinations/protocols/TLS modes. Names, paths and defaults must be resolved for
review, without executing checkout code. A folder basename alone is not a project ID.

Store approval and control records in host-owned state outside the checkout. Serialize
the approved policy into trusted session material the worker cannot alter. The canonical
representation, fingerprint, safe path handling and storage layout need specification
and tests; parsing a different policy at approval and startup is unacceptable.

`run` refuses absent, mismatched or superseded approval. Once running, the session keeps
its frozen policy. Editing YAML, the manifest or recording new approval does not widen
an existing session; stop and start a new one. An in-container restart must not gain
newly requested permissions.

Proposed states are preparing, ready, running, stopping, stopped and failed. Do not mark
a session ready until every required boundary is verified. Startup/teardown waits are
bounded. Record concrete resource IDs and ownership before a worker can run, and
reconcile them after controller interruption. Labels alone are not permission to remove
an arbitrary resource; a mismatched or ambiguous session is refused.

On normal exit, remove the worker, guard/proxy and session network. If cleanup fails,
retain a recovery record and print the session ID with an exact retry command. Never
prune unrelated Docker resources or discard edits/profiles as a recovery shortcut.

## Runtime and network boundary

This section states required experiment outcomes, not guarantees the current code
provides. If a host/engine cannot enforce the required boundary, mark that combination
unsupported and refuse startup. Do not substitute a weaker topology or a warning.

The trusted guard/proxy has no checkout, agent-state volume, Docker socket or worker
control API. It applies default-deny controls before any worker command, including
dependency installers or agent startup plugins. The worker has no administrative
capabilities, sudo, Docker/SSH-agent sockets or host namespaces; require
`no-new-privileges`, a read-only image root and explicit writable checkout/state/temp
storage. Temporary storage is container-owned, not the host's `/tmp`.

The precise guard/worker network topology is a blocking experiment, not established
here. It must prevent direct worker egress independently of proxy environment variables,
and remain closed during setup failure, crash, restart and controller loss. A proxy
failure stops the workload; there is no unrestricted fallback network. Mitmproxy's
`allow_hosts`/`ignore_hosts` select interception, not permission. Its HTTP blocklist alone
is explicitly unsuitable against an active adversary. Addon load/runtime errors must
not leave a listening proxy forwarding traffic without policy.

Enforce exact approved destinations and ports, normalize hostnames consistently and
reject literal-IP/suffix-wildcard shortcuts. Resolve destinations through the trusted
path; validate the addresses actually used, including CNAMEs, changing answers, IPv6 and
private/special-use results. Restrict the worker's DNS and block embedded Docker DNS,
external resolvers, unauthorized UDP/QUIC and direct-address bypasses. Unavailable
controls cause startup refusal. Do not permit an entire CDN/storage suffix to fix a
redirect; each new destination needs approval.

| Destination mode | Required checks and limitation |
| --- | --- |
| HTTPS inspection | Check CONNECT target, available TLS identity metadata and each decrypted HTTP request's destination/Host/authority before forwarding it or its body. Validate upstream TLS. Reject inconsistent identities, malformed requests and unauthorized protocol changes. |
| HTTPS passthrough | Restrict the approved tunnel destination/port and require the handshake checks the selected engine can prove. Encrypted HTTP authority remains invisible, so this does not meet full request-host enforcement. Missing/unverifiable SNI or ECH must not silently weaken the approved contract. |
| SSH tunnel | Permit only explicitly approved SSH destinations/ports. SSH on port 443 is not HTTPS and has no TLS inspection mode. This must not enable a generic tunnel to other hosts or ports. |

Passthrough is an explicit weaker exception. A destination requiring full HTTP-authority
enforcement cannot use it. Approval and `status` must disclose the distinction. Protect
the inspection CA private key from the worker; provide only its public trust certificate
where required, without modifying the host's trust store.

Hostname enforcement does not prevent data being sent to an approved service. It also
does not protect tokens that the worker legitimately receives. TLS passthrough and SSH
must not be advertised as equivalent to inspecting every HTTP request.

Ingress publishing is separate from egress permission. Explicit loopback/LAN bind
addresses and ports need approval; a LAN binding is not permission for outbound access
to the whole private subnet. Return traffic for approved inbound connections must work
without introducing that broad exception. A bind address does not authenticate clients.
Do not publish guard/proxy administration or engine APIs. Public/all-interface exposure
and access-control requirements remain outside the initial LAN contract.

## Credentials and Git

Keep one login source per host user, agent/provider and account, reusable by explicitly
approved projects. Do not share agent settings, plugins, history or a writable credential
store between projects. Provision only the selected authentication material through
supported mechanisms into isolated state; do not mount host homes or copy an entire
OpenCode database to obtain one account.

Provision credentials, public CA certificates and other generated configuration by
copying or generating only the required material into isolated container-managed
storage. This is not an exception allowing host-file mounts or a generic host bridge.
The worker cannot directly read or write the host-side source or approval records.

Safe provisioning, OAuth callback paths, refresh ownership and account-source updates
are unresolved. If neither CLI offers a suitable supported path, report that limitation
rather than invent a generic host shell/HTTP bridge. The worker must not be able to
replace another project's credentials or change its own account grant. Account access
remains as broad as the supplied credentials; approval does not reduce provider scopes.

Use a dedicated sandbox SSH identity rather than the person's normal key. Its approval
and provisioning follow the account-source model. Supplied private keys and tokens are
readable by the worker, and reusing one identity across projects shares its account
exposure. GitHub HTTPS/API authentication is separate from SSH authentication.

Preserve SSH remotes. For GitHub.com, evaluate container-owned SSH configuration and a
proxy tunnel to the supported `ssh.github.com:443` endpoint. Use verified host keys and
fail normal client connections on unknown/changed identity. These client checks are
not the isolation boundary: a malicious client disabling its own checks must still be
unable to reach unapproved destinations. Other SSH providers and GitHub Enterprise
require separate destination/transport validation.

Supply minimal container-owned Git identity/configuration, not the entire host Git
configuration. Validate prepared hook paths and protect Git configuration/hook-entry
mounts where the design requires them; hook prerequisites run inside the sandbox.
Do not claim that these safeguards make hook-skipping impossible. CI still judges
commits, and preparation must never silently execute or install checkout-controlled
hooks on the host.

Project state persists after `stop`. The explicit contracts for granting/revoking
accounts, deleting profiles, rotating the sandbox SSH key and the inspection CA must be
settled before real credentials are used. No cleanup command deletes them implicitly.

## Worker tools and downloads

Bake pinned agent CLIs, the global Vite+ CLI and baseline tools into the project-supplied
worker image. Project-declared Node/package-manager versions may be downloaded later
into project-isolated state. Do not use Vite+'s official image unchanged: its documented
passwordless sudo conflicts with the worker boundary.

The proposed initial image-acquisition path is explicit host-operator preparation:
project releases identify exact worker/guard digests, architectures and provenance; the
person uses the trusted local engine to pull those images before startup. Image
distribution, verification and update details still need specification. Control commands
only inspect cached images and refuse missing, wrong-digest or incompatible images;
they do not pull/build them automatically. Host/engine image acquisition does not add
registry permissions to worker egress. An image update requires new approval, and local
build recipes require human review before execution outside the sandbox.

For a normal npm/pnpm workflow, initial destination candidates are `registry.npmjs.org`
and `nodejs.org` on HTTPS port 443. Vite+'s docs describe Node downloads from
`nodejs.org/dist` and automatic package-manager provisioning. These candidates are not
a validated complete allowlist. Image-build/bootstrap downloads are a separate trust
operation from runtime dependency downloads; a host build is not silently part of
`doctor` or `run`.

Validate actual clients, certificate trust and redirects for Vite+ environment setup,
`vp install`, dependency/native-addon builds and project hooks. Test the Go/Python
workflows too. Custom registries, git/tarball dependencies, browser downloads and new
mirrors request additional permissions through the same approval process. Running an
installer never grants access to hosts merely named in package metadata.

Install scripts may execute arbitrary code, but only inside the worker with the same
restrictions. System-library requirements need a reviewed image update, not sudo at
runtime. Persist downloaded tools/caches within project state; no shared writable cache
between unrelated projects. Do not disable signature/integrity verification to make a
download work through the proxy.

Stream agent responses and large downloads rather than buffer full bodies. Enforcement
must run before forwarding starts. Default proxy logging records only approved safe
metadata; do not retain headers, request bodies, full URLs/query strings or flow dumps.
Prove this with synthetic sensitive markers and error paths, not real credentials.

## Acceptance matrix

These are proposed acceptance labels, not existing scenario IDs or passing tests. Write
black-box scenarios and domain tests when the corresponding implementation slice is
specified. Runtime probes must exercise the effective boundary; checking Dockerfile
strings or a successful image build is insufficient.

Use disposable repositories, controlled allowed/denied endpoints, isolated test
resources and synthetic markers. Pair denied probes with positive controls so a broken
network is not mistaken for enforcement. Distinguish policy refusal from DNS failure,
timeouts and unavailable engines. No valuable credentials enter the early probes.

### Preparation, approval and ownership

| Label | Probe | Required result |
| --- | --- | --- |
| AC-01 | Missing/malformed config, duplicate/unknown fields or unsupported options | Refuse before agent/session resources are started. |
| AC-02 | Missing approval or a change to images, identity, accounts, ingress or network permissions | Refuse startup and explain the change; no self-approval. |
| AC-03 | Worker edits YAML and tries to restart, exec or change trusted policy material | Existing policy stays frozen; no widened access. |
| AC-04 | Invoke init, doctor and approve against a checkout containing executable hooks/plugins | No checkout code runs, hooks install, image pull/build or account login occurs. |
| AC-05 | Remote engine, linked worktree, symlink/path substitution, home/socket or extra-directory mount attempt | Refuse unsupported layouts and unapproved sources before workload execution. |
| AC-06 | Two projects with equal basenames; wrong-user/foreign/ambiguous session IDs | State stays separate; mutations refuse the foreign or ambiguous resource. |
| AC-38 | Missing, incompatible or wrong-digest cached images; unapproved image substitution/update | Refuse workload startup without auto-pull/build or widened worker permissions; only the explicitly approved image content may run. |
| AC-39 | Supported single-worker manifest with an approved image and constrained workspace/environment | Resolve one worker under the approved policy; retain the itos-managed guard/proxy and fixed security restrictions. |
| AC-40 | Compose, builds, Features, host commands, extra mounts, runtime flags or host environment interpolation | Refuse unsupported fields before starting resources or executing commands; no host-side setup or fallback to the Dev Containers CLI. |
| AC-41 | Hidden image metadata and manifest changes after approval | No unreviewed commands, users or permissions enter the effective configuration; changes invalidate approval for the next run and cannot alter a live session. |

### Worker and network isolation

| Label | Probe | Required result |
| --- | --- | --- |
| AC-07 | Read an unmounted host sentinel/home/socket; inspect host namespaces; request sudo or capabilities | Access is denied; host privilege is unavailable. |
| AC-08 | Write image files, protected Git entries, guard policy or inspection private key | Writes/access are denied where protected; policy stays unchanged. |
| AC-09 | Normal approved HTTPS request in each mode | Request works and the reported guarantee matches the approved mode. |
| AC-10 | Ignore proxy variables; use direct IPv4/IPv6, host gateway, metadata/private destinations or another local route | Unapproved egress is denied independently of client cooperation. |
| AC-11 | Normal DNS, Docker 127.0.0.11, external UDP/TCP DNS, alternate loopback addresses and unauthorized DoH destinations | No unauthorized resolution path or destination is reachable. |
| AC-12 | CNAMEs, rotating answers, rebinding/private answers and mixed public/private address sets | Only policy-valid addresses actually used are permitted; no private-destination bypass. |
| AC-13 | Allowed-host spelling variants, suffix lookalikes, literal IPs, alternate ports and shared-address destinations | Canonical exact policy is enforced; denied destinations stay denied. |
| AC-14 | CONNECT/SNI mismatch, missing/unverifiable identity or ECH | Enforce the approved mode's explicit contract or refuse; never silently downgrade. |
| AC-15 | Inspected HTTP Host/authority/URI mismatch, reused connections and HTTP/2 streams | Check every request before forwarding headers/body; mismatch is refused. |
| AC-16 | Non-HTTP bytes, protocol upgrades, arbitrary CONNECT, UDP/QUIC and malformed requests | Only explicitly supported approved protocols work; no generic fallback escape. |
| AC-17 | Proxy/addon startup error, runtime exception, crash and restart; missing network control | No unrestricted forwarding window; refuse startup or stop the worker. |
| AC-18 | Allowed redirect followed by an unapproved download/log-storage destination | Stop at the unapproved destination without widening policy. |
| AC-19 | Approved GitHub.com SSH tunnel/identity plus different hosts/ports/protocols and disabled client safeguards | The approved identity works through ssh.github.com:443 with unchanged GitHub.com SSH remotes; other destinations, including unvalidated Enterprise/other-provider endpoints, remain denied regardless of client flags. |
| AC-20 | TLS inspection client trust, upstream certificate failure, passthrough and pinned-client incompatibility | Correct trust behavior; invalid upstream TLS is refused and failed inspection never auto-bypasses. |

### Lifecycle, state and ingress

| Label | Probe | Required result |
| --- | --- | --- |
| AC-21 | Literal argv containing spaces, metacharacters and shell-looking text | Preserve argument boundaries; no implicit shell evaluation. |
| AC-22 | Success, nonzero exit, cancellation and required-cleanup failure | Preserve workload outcome; surface cleanup failures and an owned recovery ID. |
| AC-23 | Controller interruption, guard loss, partial startup and repeated stop | Reconcile/clean only owned resources; retain edits/state; no unrestricted restart. |
| AC-24 | Exec in live/stopped/foreign sessions and foreground workload exit during exec | Only the live owned session accepts commands under unchanged policy; primary exit refuses new exec and tears down active exec commands without extending session lifetime. |
| AC-25 | UID 1000 and a different UID/GID, read-only root and fresh writable state | Checkout/profile/cache/temp operations work without root or host-wide chown. |
| AC-26 | No ingress approval, then explicit loopback and LAN bindings | Only approved bindings are reachable; control/engine APIs remain unpublished. |
| AC-27 | Approved LAN client connection and attempted worker-initiated private-network access | Approved request/return traffic works without broad private-network egress. |
| AC-28 | Stop one of two concurrent projects/sessions and provoke port collisions | The other session stays intact; collisions fail with bounded, recoverable cleanup. |

### Agents, credentials and useful work

| Label | Probe | Required result |
| --- | --- | --- |
| AC-29 | Selected user-wide account provisioning; another project/account/state; attempted source modification | Only approved material is provisioned; no central-store/home exposure or cross-project mutation. |
| AC-30 | OpenCode and Claude Code login, expiration/refresh, cancellation and account revocation | Supported flows work within policy; source updates remain trusted and failures are explicit. |
| AC-31 | Sandbox SSH identity and HTTPS/API credentials; wrong/changed host key | Expected Git/gh operations work; normal clients reject bad host identity; no host-agent/key mount. |
| AC-32 | Edit, commit through prepared hooks and push in a disposable consumer repo | Changes persist and hook code executes inside the worker, not on the host. |
| AC-33 | Fetch pinned itos and retrieve an actual failed CI log through its redirect chain | Useful workflow succeeds under reviewed destinations or a separately specified narrow retrieval design. |
| AC-34 | Fresh Vite+ Node/PM provisioning, npm/pnpm install/check/build, Go and Python dependencies | Approved downloads and verification work; extra hosts are refused, not inferred permissions. |
| AC-35 | Agent streaming, concurrent requests and large dependency/release downloads in both TLS modes | Measure first-byte/token latency, CPU, memory and completion; no unintended full-body buffering. |
| AC-36 | Synthetic secret markers in URLs/headers/bodies and failure diagnostics | Proxy/controller persisted logs/status contain only permitted metadata, not the markers or flow dumps. |
| AC-37 | Repeat runtime probes across declared host/engine/architecture combinations | Claim support only for combinations with recorded passing runtime evidence. |
| AC-42 | Supported in-container lifecycle command succeeds, fails or attempts denied egress before agent launch | Run only after isolation is ready under frozen policy; failures block subsequent setup/agent launch and trigger owned cleanup. |

## Experiments before implementation

These experiments establish feasibility before production interfaces and ledger slices
are finalized. They are not authorized or executed by creating this document.

1. **Proxy boundary:** a disposable worker and trusted guard/mitmdump setup, with no
   credentials. Prove direct-egress denial, exact policy, inspection/passthrough
   distinctions, typed SSH tunneling and failure closure. First resolve how addon
   failures disable forwarding. Include DNS/IPv6, authority mismatch and restart probes.
2. **Client compatibility and performance:** exercise streaming, Vite+ downloads, Git
   SSH/HTTPS and release redirects. Measure against a direct controlled baseline;
   define performance budgets from the intended workload, not a language comparison.
   Consider a Rust alternative only if it passes the same security contract and gives a
   measured useful improvement.
3. **Credential lifecycle:** investigate supported minimal provisioning/refresh for
   both CLIs, the dedicated SSH identity and gh. Start with synthetic/test accounts.
   Define consent, CA/key ownership, callback exposure, revocation and deletion before
   a person explicitly authorizes a live-account trial.
4. **Local-engine portability:** run the same probes on native Docker, Rancher/Moby and
   Docker Desktop, including ARM64. Validate bind mounts, ownership, trusted-control
   separation, LAN forwarding and interrupted cleanup. Engine-name similarity is not
   proof of equivalent behavior.

Record versions/digests, host/engine/architecture, exact probe commands, expected and
actual outcomes, and redacted evidence. Keep failures visible; an unavailable control or
client incompatibility changes the proposed support contract, not a test into a skip.

## Implementation slices

Candidate sequence after review and experiment results, not work already entered in
the ledger:

1. **Project foundation:** replace inherited template claims in the roadmap/agent notes,
   reconcile CI/version assumptions and plan fresh baseline proof. Before the first
   tracked spec/change, explicitly set the tracking boundary to the amended seed
   `23b8ce15410ec9431f5a1fe39a973d9cf0a3d8de`, install required hooks and regenerate agent
   rules. Existing-config `itos init` does not set that boundary automatically. The
   first actual project task starts at T-1; do not recreate template completion claims.
2. **Configuration, doctor and approval:** strict domain policy, restricted single-worker
   devcontainer manifest, host-state port and reviewable permissions, without running
   checkout code. Select existing YAML, JSON-with-comments and Docker libraries before
   specifying adapters; preserve the ports/fakes pattern. Do not implement Compose yet.
3. **Offline owned run:** explicit argv, hardened mounts/identity, recorded ownership,
   bounded startup, foreground exit and safe cleanup. Prove actual isolation before
   any network/credential feature goes live.
4. **Approved proxy egress:** add the experimentally validated guard, mode-specific
   policy, SSH path, metadata logging and denial/failure runtime tests. Keep offline
   mode working rather than making egress implicit.
5. **Session recovery and LAN ingress:** complete exec/status/stop and approved binding
   behavior, including interrupted controllers and foreign-resource refusal.
6. **Accounts and real workflows:** implement the validated grants/profiles/refresh
   contract; prove both agents, Git hooks, Vite+ dependencies, release downloads and
   failed CI inspection under explicit live-account consent.
7. **Supported-platform/release proof:** automate image builds and isolation probes in
   this project's CI as features arrive, then collect the remaining Desktop/ARM64
   evidence before advertising those combinations. Preserve pins and provenance.

Specify each behavior with black-box scenarios before building it; task-only foundation
work has named checks. Commit behavior steps red first once tracking/hooks are ready.
Use domain-owned ports and stateful fakes; keep CLI, engine/filesystem adapters, JSON and
exit-code mapping outside the domain. Do not delete the counting example or weaken its
existing gates merely to make the new project pass.

## Decisions still open

- Exact schema, approval canonicalization/storage and checkout/engine identity rules.
- Manifest discovery/output paths, supported JSON-with-comments syntax, field/value
  constraints, reserved environment variables, image-metadata handling and lifecycle
  hook semantics. Compose services remain later work, not part of this pilot.
- Concrete guard/proxy topology and fail-closed handling of addon/runtime failures.
- Which passthrough handshake guarantees can be enforced, and how unverifiable clients
  are refused without misrepresenting HTTP-authority protection.
- Supported minimal auth provisioning/refresh mechanisms, account-management commands,
  callback paths, revocation/deletion and CA/SSH-key rotation.
- Worker/guard versions, digests and provenance; final download destinations, redirects
  and TLS trust configuration for each pinned client.
- Image distribution, host-operator acquisition instructions and digest/architecture
  verification; no automatic pull/build path is part of the pilot control commands.
- Resource/timeout limits, performance budgets and platform-specific cancellation behavior.
- Precise initially validated OS/engine/architecture matrix and LAN publishing constraints.
- Failed Actions-log redirects: narrowly approved destinations versus a separately
  reviewed run/artifact-specific retrieval design. A generic host fetch bridge is excluded.

Review this contract and matrix first. Then authorize the smallest experiments, update
the GitHub proposal if desired, and create implementation work. None of those actions
is implied by accepting a draft file.
