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

## Not yet

Egress through a proxy, credentials and accounts, Git over the network from inside the
container, LAN ingress, Compose services, platforms other than Linux amd64 with native
Docker, and any support claim beyond what a scenario proves. The runtime directions in
`PLAN.md` (isolates, WASM, a restricted shell, a capability broker) are research: record
findings as ideas, build none of them in this milestone.

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

- Work only on items this milestone needs. When nothing is ready, specify the next idea
  the milestone needs, in the order above.
- Agents commit; only the loop pushes, marks a pull request ready, and closes an item.
- Stop and ask the person on: an open question (`itos decision`), a change to this charter
  or to the contract, a new dependency with a network service, a security property the
  contract leaves open ("Decisions still open"), or two attempts on one item that move
  nothing.
