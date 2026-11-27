---
status: accepted
date: 2026-11-26
---

# Deliver one approved OpenCode Go key to OpenCode 2.2.1

## Context and Problem Statement

How does slice-40 deliver an approved login to OpenCode 2.2.1, which stores credentials and sessions in SQLite?

Asked as q-103, about slice-40.

## Considered Options

- Give the supported client the approved OpenCode Go provider key.
- Keep whole-file delivery for an older client that uses auth.json.

## Decision Outcome

On 2026-11-26 the person selected credential-only access for OpenCode 2.2.1. Run gives OpenCode the one approved OpenCode Go key through a provider-key input that this version reads. The host SQLite login store and the host sessions are never shared with the workload.

### Consequences

This replaces the whole-file delivery for slice-40 and the later-provider consent described in decision 0042. Each run and its status report only the delivered provider name. Scenarios use synthetic keys to verify the supported provider input, exclusion of host login and session data, and absence of key values from records and status.

## More Information

Supersedes ADR-0040.
