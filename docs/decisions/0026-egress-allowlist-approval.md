# Bind allowlist approval while keeping run gated

Accepted for slice-26, 2026-11-09. Authorities are the charter's second milestone,
the pilot's AC-02, T-38's proxy boundary and the person's q-52 decision, as detailed
in decision 0025. No new dependency is needed: reuse Go JSON/SHA-256, the existing
terminal adapter and policy's pinned IDNA adapter.

Canonical schema 1 gains `allowlist-v1`, binding the version-2 network mode, sorted
canonical host names and T-38's pinned proxy image. Names are a set in the request
and one array value in differences. Any permission or proxy-image change needs new
consent. Stored records share policy's lexical canonical-name check, without IDNA
mapping on read. Another valid proxy digest reads as changed, not invalid.

Offline keeps its existing profile and version-1 canonical bytes. A version-2
offline request retains version 2 in the canonical policy: moving between versions
needs consent rather than making doctor and approval print different policies.
The envelope schema stays 1; older binaries reject the unknown allowlist profile.

This overrides the Solution strategy sentence in arc42 requiring doctor, approve
and run to accept the same inputs. Doctor and approve explicitly enable version-2
readiness; run keeps the default refusal before manifest, state and engine access.
The cmd assembly test requires Names on both file-only and ordinary Inspectors and
checks that only doctor/approve opt in. Slice-27 owns lifting the gate with proof.

Approval's domain owns the shared decoded-value difference walk. Presentation
uses its old/new values; preflight carries only sorted paths to run. The local
comparison omits unobserved engine/image facts, and the later comparison includes
them. Selection failure without a comparison retains the existing generic refusal.

The review and plain approval status disclose the full requested list, handshake
limits and incomplete-log limitation. They explicitly say run cannot start this
profile yet. No owned proxy or network is created here. An intervening release
could publish this approval UI before runtime support lands; release timing remains
a question for the coordinator and the person.

Under q-52, a ready network grant refuses every fingerprint option with
`approve.terminal-required`, before syntax or equality checks. Bare nonterminal or
JSON approval retains `approve.confirmation-required`, with only interactive
approval in the fix. Terminal matching approvals neither prompt nor rewrite.
Credential support will widen this condition. An unsandboxed host agent can fake a
terminal; detection is not human authentication, and no keys or signatures are added.

ID-POLICY-60/61's interim doctor/approve refusals are replaced by the slice-26
approval scenarios. ID-POLICY-62 and all invalid-policy checks remain. Test
checkpoints `1df32bb` and `0199609` retain work-in-progress scenarios. With those
tags removed for the run, all 91 local rows failed at behavior assertions before
implementation. Version-2 rows failed candidate, comparison, review or diagnostic
assertions; the version-1 run rows failed at the missing changed-path text; help
failed at its old offline-only description. No undefined steps or setup failures
count as evidence. Tests use temporary state and synthetic Engine APIs only;
CI owns changed-code proof and all real-engine evidence.

The red run's first failing assertions, grouped by ID and outline rows:

| ID-APPROVAL | Rows | Failing behavior assertion |
| --- | --- | --- |
| 36, 37, 41, 51 | 2, 1, 1, 2 | Doctor reports the independent fingerprint and complete effective settings. |
| 38 | 4 | Both inspections report matching approval and identical independent fingerprints. |
| 39, 40 | 5, 6 | The second inspection reports changed approval and a different independent fingerprint. |
| 42, 44, 49, 57 | 1, 1, 2, 2 | The transcript shows complete settings and restrictions before one prompt. |
| 43 | 3 | The transcript shows the listed old/new network differences before one prompt. |
| 45, 47 | 5, 8 | The command reports `approve.terminal-required`, exit 1. |
| 46 | 1 | Matching approval succeeds without prompting. |
| 48 | 3 | Terminal consent reports `approve.changed`, exit 1. |
| 50 | 25 | Doctor reports `approval.state.invalid`, exit 1. |
| 52 | 2 | Run refuses changed approval and names `itos container approve`. |
| 53 | 1 | Run names `/manifest/containerEnv, /manifest/workspaceFolder` in its changed diagnostic. |
| 54 | 1 | Run names `/image/id` in its changed diagnostic. |
| 55 | 4 | Approve retains each prerequisite's rule and exit before consent. |
| 56 | 2 | Help says `Review and remember exact settings`. |
| 58 | 4 | Review escapes C1/bidirectional controls in current/prior environment values. |
| 59 | 5 | Bare redirected/JSON approval reports `approve.confirmation-required`, exit 1. |

All 91 rows pass after implementation, together with the existing approval-help
regression. Scoped race tests pass for the changed packages and the cmd assembly.
The full local feature suite also passes without the race detector. Fresh CI
changed-code proof and native-engine checks remain the coordinating loop's work.
