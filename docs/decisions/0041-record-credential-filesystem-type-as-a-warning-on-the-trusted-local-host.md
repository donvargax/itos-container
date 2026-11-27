---
status: accepted
date: 2026-11-26
---

# Record credential filesystem type as a warning on the trusted local host

## Context and Problem Statement

Should a credential filesystem type that cannot exclude a backing alias by itself refuse a run on the trusted local machine?

Asked as q-101, about slice-61.

## Considered Options

The options are those the question names.

## Decision Outcome

On 2026-11-26 the project owner directed that credential.filesystem become a recorded doctor and run warning. Detect the affected path and filesystem type, report both in doctor, and preserve the warning for status. Continue when filesystem type alone prevents excluding an alias, including on overlay, FUSE, NFS and 9p. The local machine, repositories and accounts are trusted. This applies the 2026-11-24 rule that a security refusal must address a real outsider.

### Consequences

This replaces the filesystem-type refusal in decision 0027’s 2026-11-10 amendment and later allowlist decision q-64. An independently observed prohibited location or other independent refusal still applies. Slice-61 updates the affected decision text, standing summary and scenarios.
