# itos-container: the plan

Read [docs/charter.md](docs/charter.md) first: it selects the scope this project may build
from the binding [pilot contract](docs/plans/container-pilot.md). Those documents are
authoritative. This plan describes the selected milestones and the work still needed.

`itos-container` is being built as the `itos container` extension to run a coding agent
and every tool it executes inside a local container sandbox for one checkout. Core itos
retains work routing, commit rules and CI proof.

## Current state

v0.1.0 has been published. The entry point dispatches `doctor`, `approve`, `run`,
`status` and `stop`, alongside `count`, `version` and `completion`. Sandbox claims
are limited to what the scenarios prove. See the work registry (`itos work`) for
which second-milestone items have landed.

The inherited Go counting example retains its domain-owned Files port, disk and
standard-input adapters, in-memory fake, unit/property tests and black-box scenarios.

T-3 completed the itos 8 workflow migration and T-4 upgraded the pinned itos-cc. These
are tool migrations, not completion of the foundation or sandbox. The inherited Go
checks and release tooling cover multiple platforms, not container-host support. T-4's
cached mutation sample had zero eligible records; it did not establish a fresh baseline.

## Order of work

### First milestone: an offline owned run

1. **Project foundation:** complete project-specific guidance, adapt `docs/CLI.md` and
   `docs/CONFIG.md` to the planned extension and `itos-container.yaml`, comparing the
   current itos and itos-cc guidelines and using whichever is newer. Align CI branch
   conditions and establish fresh foundation proof in their separately recorded work.
   Preserve the counting example and its existing gates, pinned tools and release machinery.
2. **Configuration, doctor and approval:** specify and build the strict policy file,
   restricted single-worker devcontainer manifest and host-owned approval state without
   executing checkout code. Select maintained parsing and engine libraries in the specs
   before adding pinned dependencies; retain domain-owned ports and fakes.
3. **Offline owned run:** the planned target is `itos container run -- <argv>`, preserving
   explicit argument boundaries in an approved, hardened container with no network at all.
   Require the checkout as its only host mount, non-root identity, recorded resource
   ownership, bounded startup, foreground exit and safe cleanup of owned resources while
   preserving checkout edits and project state. Each property needs a black-box scenario
   that fails without it and passes with it before the milestone can be called complete.

### Second milestone: a coding agent inside the sandbox

1. [Egress through an allowlisting proxy][second-milestone].
2. [Credentials passed in, never injected, never in the checkout][second-milestone].
3. [A home for the harness][second-milestone].
4. [Interactive use][second-milestone].
5. [Late host sockets unreachable][second-milestone].

[second-milestone]: docs/charter.md#the-second-milestone-a-coding-agent-inside-the-sandbox

Foundation work has named task checks. Behaviour work starts with scenarios and red step
commits. Domain tests use an engine fake; scenarios needing a real engine run only in CI.
Agents never receive the Docker socket. Agents commit their work; the coordinating loop
pushes, marks pull requests ready and closes items after the pushed head is green and its
required proof passes.

## Milestone limits

The milestones target Linux amd64 with native Docker. Support claims require passing
runtime scenarios. The charter's deferred scope includes LAN ingress, Compose services,
other container platforms and giving any worker the Docker socket. GitHub credentials
stay on the host, which pushes and lands changes.

Assume hostile checkout code, arbitrary tool execution, dependency scripts and
prompt-injected instructions. Hooks and agent instructions are workflow safeguards, not
the filesystem or network isolation boundary. The checkout is intentionally writable;
it is not protected from destructive edits. Containers do not protect against host
administrators or kernel/container escapes.

## Deferred research

Isolates, WASM, restricted shells and a capability broker remain research ideas outside
this milestone. No dependency, experiment or runtime branch is selected or authorized
by this plan. Report findings as ideas for the coordinator; do not implement them here.

## Provenance

This project is AGPL-3.0 licensed with attribution to donvargax; see [LICENSE](LICENSE) and
[CONTRIBUTORS.md](CONTRIBUTORS.md). Infrastructure comes from the owner's
[itos](https://github.com/donvargax/itos) and
[itos-template](https://github.com/donvargax/itos-template) projects, with permission to
distribute that owner's copied work under AGPL-3.0 here. Preserve the original projects' and
third-party dependencies' licenses and notices; that permission does not relicense them.
