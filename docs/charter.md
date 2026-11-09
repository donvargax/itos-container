# Charter

What this project builds, and the bounds an unattended agent loop works within here.
`docs/plans/container-pilot.md` is the binding contract; this page chooses what to build
from it first and what not to build yet. A change to this charter is the person's.

## Purpose

`itos-container` runs a coding agent and every tool it executes inside a local container
sandbox for one checkout, so the agent can work unattended without reaching the host: its
files, its credentials, its network. It is an itos extension, invoked as `itos container`;
core itos keeps work routing, commit rules and CI proof. Assume the workload is hostile:
arbitrary code, hostile dependency scripts, prompt-injected instructions (the contract's
"Goal and limits").

## The first milestone: an offline owned run

Done when `itos container run -- <argv>` runs a command in an approved, hardened container
with no network at all, the checkout as the only host mount, a non-root identity, and owned
resources torn down on exit, every one of those properties proven by a scenario that fails
without it. In the contract's terms: implementation slices 1 to 3.

1. **Foundation:** finish the move to itos 8 (`itos-8-upgrade`) and to the current itos-cc
   (`p1-itos-cc-upgrade`); replace the template claims
   inherited in `PLAN.md`, `README.md` and `AGENTS.md` with this project's; adapt
   `docs/CLI.md` and `docs/CONFIG.md` to `itos container`'s commands and
   `itos-container.yaml`, comparing with the current guidelines of itos and itos-cc, whichever
   is newer.
2. **Configuration, doctor and approval:** the strict policy file, the restricted
   single-worker devcontainer manifest, host-owned approval state, without executing
   checkout code.
3. **Offline owned run:** explicit argv, hardened mounts and identity, recorded ownership,
   bounded startup, foreground exit, safe cleanup.

Foundation leftovers (docs, tooling, CI polish) are taken only when they block the next
step above; otherwise the loop moves on to that step and they wait.

## Architecture is a hypothesis

`docs/architecture/arc42.md` (once it exists) is the current design, not a promise. Build from
its building-block view; when the code shows a section is wrong, do not diverge quietly: in the
same pull request, record a decision in `docs/decisions/` naming the section it overrides and
why, and update that section. A spec reviewer refuses a change that departs from the document
without one. Triage files a drift it sees elsewhere as its own idea, titled "Architecture
drift: …".

## The second milestone: a coding agent inside the sandbox

Current once `v0.1.0` is published. Milestone-one isolation items still open then (control
state, the flaky-test and suite-speed fixes) keep their place ahead of it.

Done when a coding agent's harness runs inside an approved container for one checkout and
reaches only the hosts it needs: Codex CLI headless (`codex exec`) first, then Claude Code
headless (`claude -p`) and interactive. Each property is proven by a scenario that fails
without it. The container spike (outside this repository) tried every piece by hand; its
findings are inputs, not evidence.

1. **Egress through an allowlisting proxy:** an owned proxy beside the worker, on an
   internal network, so the worker has no direct route out. It allows CONNECT to listed
   host names on port 443 only, with no TLS interception, matching the name the client gave
   (never a reverse-DNS lookup of an IP), and logs every refusal. The allowlist is part of
   the approved settings. The proxy is a small, maintained Go CONNECT proxy, chosen in the
   spec, not squid (the person, 2026-11-09). TLS interception may come later; if it does,
   the proxy first moves to a repository and project of its own.
2. **Credentials passed in, never injected, never in the checkout:** per command, so they
   never appear in the container's configuration or `docker inspect`. Claude's long-lived
   token from a host file the person keeps. Codex shares the host's own login: `auth.json`
   bind-mounted read-write as a single file (Codex rewrites it in place), plus its lock file
   when the host has one. A GitHub token for `gh` and pushes, passed the same way.
3. **A home for the harness:** a writable volume or tmpfs, never the host home; harness
   state seeded where its first run would otherwise stop or call home (Claude's onboarding
   flags; Codex with update checks, analytics and plugins off, and its own sandbox off because
   the container is the sandbox). Executable `/tmp`.
4. **Interactive use:** `run` and `exec` with a TTY (raw mode, resizes, Ctrl-C as input),
   built on slice-12's cancellation rules; a disconnected client never leaves the harness
   running inside.
5. **Late host sockets unreachable:** Landlock (ABI 9 or later) by default, applied by a
   small read-only init binary itos-container mounts as the worker's entrypoint before it
   runs the literal argv; gVisor (`runsc`) as the fallback; with neither, `run` only with an
   approval that records "late host sockets reachable", shown at approval and in `status`.
   Doctor names the backend a run will use, and the approval binds it. Prove the Landlock
   path on the person's machine until a CI runner with a new enough kernel exists (a
   weakened gate, recorded), and the gVisor path in hosted CI. Registry:
   `p1-checkout-socket-kernel-enforcement`.

No scenario uses a real model, account or credential: harness traffic goes to a fake
endpoint the proxy allows in tests. A real harness run is the person's smoke check.

## Not yet

LAN ingress, Compose services, platforms other than Linux amd64 with native Docker, giving
any worker the Docker socket, and any support claim beyond what a scenario proves. The
runtime directions in `PLAN.md` (isolates, WASM, a restricted shell, a capability broker)
are research: record findings as ideas, build none of them yet.

## How the work is done here

- Each behaviour is specified by black-box scenarios before it is built, committed red
  first; foundation work names a task with checks. The repository's rules (`AGENTS.md`,
  `itos guide work`, `itos guide commit`) hold for every agent.
- The domain talks to the container engine through a port. Agents test against its fake;
  a scenario that needs a real engine runs in CI, never on the host the loop runs on.
  No agent is given the Docker socket.
- Prefer an existing, maintained library to new code, chosen in the spec. Add a dependency
  only with its reason in the spec and a pinned version.
- Never weaken a gate, a check or a scenario to get green; never put a credential in a file.
- An idea goes in the registry (`itos work add`) with why it matters to this milestone, or
  it is dropped. An idea outside the milestone is recorded as such and not specified.

## Bounds for the loop

- Work only on items the current milestone needs. When nothing is ready, specify the next idea
  the milestone needs, in the order above.
- Agents commit; only the loop pushes, marks a pull request ready, and closes an item.
- Stop and ask the person on: an open question (`itos decision`), a change to this charter
  or to the contract, a new dependency with a network service, a security property the
  contract leaves open ("Decisions still open"), or two attempts on one item that move
  nothing.
