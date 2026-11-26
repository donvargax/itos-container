---
status: accepted
date: 2026-11-26
---

# Share the approved whole OpenCode login file at its normal path

## Context and Problem Statement

What does one OpenCode login grant authorize when the host auth.json holds more than one provider?

Asked as q-100, about slice-40.

## Considered Options

The options are those the question names.

## Decision Outcome

On 2026-11-26 the project owner’s local-login rule and decision 0035 were applied to OpenCode. The person approves the whole host OpenCode auth.json. Approval shows every provider it holds, including OpenCode Go and GitHub Copilot OAuth when both are present. Run makes that same file available writable at OpenCode’s normal login path so refreshes land on the host. A filtered copy would add maintenance without protecting against an outsider on the trusted local machine.

### Consequences

Slice-40 redrafts the approval and delivery scenarios around the file grant. The run supplies the normal login path for the literal workload. Decision 0039’s earlier provider-entry wording is replaced; its guidance about reusing earlier work still applies. Scenarios use synthetic provider entries.
