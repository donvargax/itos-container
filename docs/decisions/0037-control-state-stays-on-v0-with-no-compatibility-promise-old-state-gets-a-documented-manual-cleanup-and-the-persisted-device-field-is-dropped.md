---
status: accepted
date: 2026-11-24
---

# Control state stays on v0 with no compatibility promise: old state gets a documented manual cleanup, and the persisted device field is dropped

## Context and Problem Statement

p2-control-state-upgrade-contract: does itos-container owe existing v0.1.0 control state an upgrade path, a migration or a documented manual cleanup, and does a schema bump drop the persisted binding device field or does a stable filesystem ID replace it?

Asked as q-96, about p2-control-state-upgrade-contract.

## Considered Options

- A migration step that upgrades v0.1.0 control state in place
- A documented manual cleanup of old v0.1.0 state, with no migration
- Keep the persisted binding device field, or replace it with a stable filesystem ID
- Drop the persisted binding device field with a schema bump, with no replacement

## Decision Outcome

The person, 2026-11-24: itos-container stays on v0 until the design is final, so it makes no backwards-compatibility promise. Breaking changes to state or schema ship with a documented manual cleanup, never a migration, so old v0.1.0 state gets documented manual cleanup. A schema bump drops the persisted binding device field, with no filesystem-ID replacement.

### Consequences

itos-container makes no backwards-compatibility promise while it is on v0. p2-control-state-upgrade-contract specifies a documented manual cleanup for v0.1.0 session records and for roots no catalog has seen, not a migration and not a new recovery command. The control-state schema bump removes the binding device field, so no record or fixture computes it, and no statfs or other filesystem ID takes its place. A later breaking change to state or schema follows the same rule until v1.
