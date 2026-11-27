---
status: accepted
date: 2026-11-24
---

# slice-40's redraft may reuse the earlier spec on slice-40-old and the partial implementation on slice-40-d9e5a2d

## Context and Problem Statement

slice-40: its spec is drafted fresh from current main now that its old branch is renamed slice-40-old. What may the drafter and the implementer reuse from the earlier attempt?

Asked as q-98, about slice-40.

## Considered Options

The options are those the question names.

## Decision Outcome

The coordinator, 2026-11-24: the earlier spec is kept on branch slice-40-old. Its scenarios are in the 'test: specify slice-40' commits ending at 4f52914 ('require worker absence before home reuse'). The drafter may reuse any of them that still match slice-40's why and decision 0035, where the one grant is the OpenCode Go provider entry from auth.json. The earlier implementer's work on two of those nine scenarios is commit d9e5a2d ('feat: read the OpenCode login request in policy version 4'), kept on branch slice-40-d9e5a2d. Once the new spec is accepted, the implementer may reuse it where the new spec keeps those two scenarios. Nothing from either branch is reused unread, and the new spec decides.

### Consequences

The drafter writes slice-40's spec fresh from current main and reads slice-40-old's scenarios before writing, keeping those that still fit. The implementer reads d9e5a2d before starting the two scenarios it covers. Both branches stay on origin until slice-40 lands.
