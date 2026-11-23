---
status: accepted
date: 2026-11-23
---

# Real credentials for container runs: a subscription token where the provider issues one, else its auth file shared

## Context and Problem Statement

Decision 0025's question 5 (docs/decisions/0025-credentials-passed-in.md:493): may real credentials be used for container runs, the person's smoke check included, and in what form does each harness get its credential?

Asked as q-92.

## Considered Options

- A long-lived subscription token where the provider issues one, otherwise the provider's auth file mounted and shared with the host.
- Every harness shares the host's own login file, Claude's ~/.claude/.credentials.json included (the line of 2026-11-20).
- Keep real credentials barred until accounts, profiles, the SSH key and the inspection CA are settled (decision 0025's question 5 as asked).

## Decision Outcome

The person, 2026-11-23: real credentials are allowed for container runs, in one of two forms per provider. Where the provider issues a long-lived subscription token, the run gets that token: for Claude, a claude setup-token token, read from CLAUDE_CODE_OAUTH_TOKEN, as slice-31's approved token. Otherwise the run gets the provider's auth file mounted, shared with the host and refreshed in place: for Codex, which has no subscription token, its auth.json. itos-container is told which credential each harness gets. A credentials-manager integration is needed but not started; until it exists, credential sources are files, one secret per file, mode 0600, in a directory of mode 0700, so nothing waits on it.

### Consequences

This answers decision 0025's question 5 (docs/decisions/0025-credentials-passed-in.md:493): real-credential runs, the person's smoke check included, are no longer barred. Synthetic scenarios stay as they are, and no scenario uses a real credential (docs/decisions/STANDING.md, the scenario line). It replaces the Claude half of the 2026-11-20 standing line (docs/charter.md:72): Claude in the container no longer shares the host's ~/.claude/.credentials.json; it gets the setup-token, which slice-31 stages read-only at /run/itos-container/credentials/claude-token for the workload to export. Codex keeps the host's Codex home mounted read-write as a directory, with the worker's auth.json a symlink into it, as decision 0033 binds and masks it. A run's settings name each harness's credential; the controller never picks one. Credential sources stay files, one secret per file, 0600, in a 0700 directory; a credentials-manager integration is a later idea that nothing waits on.
