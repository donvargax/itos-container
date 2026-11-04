# itos-container: the plan

Read [docs/charter.md](docs/charter.md) first: it selects the scope this project may build
from the binding [pilot contract](docs/plans/container-pilot.md). Those documents are
authoritative; this plan describes the selected milestone and the work still needed.

`itos-container` is being built as the `itos container` extension to run a coding agent
and every tool it executes inside a local container sandbox for one checkout. Core itos
retains work routing, commit rules and CI proof.

## Current state

The repository has an inherited Go counting CLI, its domain-owned Files port, disk and
standard-input adapters, in-memory fake, unit/property tests and black-box scenarios.
The entry point exposes count, version and completion. Container commands and isolation
are not implemented yet; the counting example is no evidence of a sandbox.

T-3 completed the itos 8 workflow migration and T-4 upgraded the pinned itos-cc. These
are tool migrations, not completion of the foundation or sandbox. The inherited Go
checks and release tooling cover multiple platforms, not container-host support. T-4's
cached mutation sample had zero eligible records; it did not establish a fresh baseline.

## Order of work

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

Foundation work has named task checks. Behaviour work starts with scenarios and red step
commits. Domain tests use an engine fake; scenarios needing a real engine run only in CI.
Agents never receive the Docker socket. Agents commit their work; the coordinating loop
pushes, marks pull requests ready and closes items after the pushed head is green and its
required proof passes.

## Milestone limits

The first milestone targets Linux amd64 with native Docker. Even that combination has no
support claim until passing runtime evidence exists. Proxy egress, credentials/accounts,
Git over the network, LAN ingress, Compose services and other platforms remain outside
this milestone. The broader pilot contract does not authorize building them here yet.

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
