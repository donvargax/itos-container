# Run the frozen allowlist through the owned proxy

Accepted for slice-27, 2026-11-15. This implements the charter's second-milestone
egress step with the existing T-39 proxy, T-40 lifecycle and T-51 refusal-log writer.
T-52 already supplies static acceptance binaries. No new library, service, keys or
signatures are introduced; reuse the pinned standard-library proxy and adapters.

This supersedes decision 0026's temporary exception to arc42's Solution strategy.
Doctor, approve and run accept the same valid version-1 and version-2 settings and
use the same readiness inspector. Remove ID-POLICY-62 and ID-APPROVAL-51's temporary
refusals and the review's "Run cannot start this profile yet" line. EGRESS-01/02/03
replace those gates with changed-list, approved allowlist and offline run evidence.
Version 3 remains gated for its later credential increments; malformed-policy
precedence is unchanged. Existing matching allowlist-v1 approvals are honoured;
the runtime gate was not an approved setting, so no profile or fingerprint changes.

Allowlist readiness observes native Docker >=28.0 and negotiated API >=1.48, both
cached images and the controller's own regular static Linux amd64 ELF outside the
checkout. Unknown versions and unavailable observations refuse. The proxy reference
remains the exact pin approved by slice-26. Its config ID is observed independently
from its manifest digest and the worker's config ID. Image acquisition, helpers,
substitute images and fallback networks are unavailable. Post-create inspection
still verifies every network and container boundary before worker start.

Only cmd joins policy's canonical names and egress's independent list check, binds
the hidden egressproxy command to egress/tcp, and binds the real RefusalLog to both
Runner and Recovery. Proxy stdout holds only JSON log records. Typed failures go
to stderr; unknown errors never expose their payload. The private command is absent
from public help and completion. Proxy failure/exits yield exit 1 unless an observed
nonzero workload exit takes precedence; cleanup remains independently reported.

## Durable runtime observation

The approval envelope, canonical snapshot and fingerprint remain byte-compatible.
The run handoff carries the freshly observed proxy config ID separately. The session
codec derives Names from the validated original snapshot, joined with newline and
no final newline, never from mutable policy or current approval.

Before publishing an intent or creating any engine resource, session/disk publishes
`intents/<project>/<token>.egress.json` as compact canonical JSON with exactly
`proxy_image_id` and `schema: 1`. The existing protected directory machinery provides
exclusive publication, file fsync and directory durability. Storage admits this
sidecar only in the intent collection, with a complete lowercase 64-digit token.
The main intent/progress schemas remain unchanged. A crash before intent publication
can leave an inert sidecar; it grants no resource authority by itself.

Every reopening joins the original snapshot and sidecar under the protected intent
lock. Missing, invalid or conflicting evidence refuses; current policy, approval or
cached images cannot repair or replace it. Held transactions also compare the full
binding, so changing the observation cannot silently transfer an existing handle.
Offline records retain their old representation and require no runtime observation.
The same mapping supports status and fresh-process recovery.

Status reports each session's original allowlist-v1 grant and all four specified
handshake/log limitations, including completed sessions, independently of current
requests and approval. It does not reopen consent, claim TLS interception or infer
log completeness. docs/security.md repeats these limits, the unsandboxed-host-agent
terminal limitation and the separate late-checkout-socket gap.

## Evidence boundary

The red step commits precede implementation. All 43 local rows had assertion-level
red at their checkpoint; EGRESS-07 was implemented separately. CI run 43531516406 at
5478523 supplies accepted first-Then red for EGRESS-12 through EGRESS-19. Local runs
use temporary state, a recorded HTTP Engine API, synthetic ELF observations and
independent approval bytes. The policy-to-egress canonicalisation drift check uses
both real adapters in cmd without an engine.

The full non-engine feature suite passes in 340 seconds without race detection,
serialized under the machine lock. The affected approval/readiness/session and
command contracts pass, as do scoped race checks for changed packages and the
journal regression. These results include the version-3 gate regression.

Real-engine green, sampled changed-code proof and the other shard's measured time
belong to CI at the pushed head. No local result establishes native isolation.
If that shard keeps warning above 180 seconds and the person judges it persistent, move
egress.feature whole to a fourth shard and update its inventory pins; never split it.
Slice-28 adds native log/failure/recovery proof. Runtime wiring does not waive
T-40's failure closure or T-51's requirement to retain resources on log-copy failure.
