---
status: accepted
date: 2026-11-26
---

# Keep the OpenCode file grant when providers change and record provider names per run

## Context and Problem Statement

Does approval of the whole OpenCode auth.json cover providers added to that file later, and what must each run and its status show?

Asked as q-102, about slice-40.

## Considered Options

The options are those the question names.

## Decision Outcome

The person confirmed on 2026-11-26 that the approved whole-file grant covers providers added to the host OpenCode auth.json later. Adding a provider requires no fresh approval. Each run records the provider names present for that run. Its status shows those recorded names. Both contain provider names only and never credential values.

### Consequences

Decision 0040 continues to govern writable delivery and approval of the whole file. Slice-40 scenarios use synthetic entries to prove that a later provider is covered and that each run reports the names it saw without exposing any credential values.
