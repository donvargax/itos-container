---
status: accepted
date: 2026-11-23
---

# The next control-lock holder deletes orphaned temporaries in protected state

## Context and Problem Statement

May the next control-lock holder delete orphaned temporaries in protected host state automatically: the .intent-<nonce>.tmp a killed publisher leaves, and the copy temporaries that decision 0022 says nothing sweeps (docs/decisions/0022-egress-proxy.md:824)?

Asked as q-91, about p3-registration-before-intent.

## Considered Options

- Delete orphaned temporaries automatically: the next control-lock holder removes them on durable evidence.
- Keep decision 0022's line: nothing sweeps a temporary, and deleting anything from protected state waits for p3-session-state-retention.

## Decision Outcome

The person, 2026-11-23: yes. Orphaned temporaries in protected host state, the .intent-<nonce>.tmp a killed publisher leaves and the copy temporaries decision 0022 says nothing sweeps, may be deleted automatically by the next control-lock holder on durable evidence. How long completed sessions' real state is kept stays with p3-session-state-retention.

### Consequences

This narrows decision 0022's line (docs/decisions/0022-egress-proxy.md:824-828), which says a temporary can be left behind, nothing sweeps it, and deleting from protected state is p3-session-state-retention's decision. That holds no longer for orphaned temporaries: a .intent-<nonce>.tmp, alone or as a second link of its record, and the refusal log's copy temporaries. The real state of completed sessions, the session records and egress-refusals.jsonl included, is not narrowed: how long it is kept stays p3-session-state-retention's decision. The evidence must be durable, so a name alone is never proof, as p3-registration-before-intent already requires, and a live publisher's temporary is never removed. That item's safe retirement applies this decision and its scenario checks both.
