---
status: proposed
date: 2026-11-24
---

# What evidence lets recovery retire an uncertain create

## Context and Problem Statement

What evidence lets status or stop retire an uncertain create, that is, finish its session and release its admission claim, when the create was sent or may have been sent and its outcome is unknown?

The cases are three. A daemon answers a container or network create with an error, an exhausted address pool for example. A VolumeCreate is sent, its reply is lost, and later lookups keep finding no volume. A controller dies after sending a create.

Today every engine create maps any error reply, and any reply that is lost or late, to an uncertain outcome; none is mapped to "not created". The worker create is internal/engine/session/docker/run.go:122-124, the proxy create is docker/proxy.go:40-42, and both egress network creates are docker/egress.go:41-43. The checkout anchor volume is docker/anchor.go:17-25 (port.MountUncertain) and the harness home volume is docker/home.go:24-27 (port.HomeUnavailable). The port states the rule at internal/engine/session/port/session.go:185: "Uncertain is not absence". The run publishes the durable "intent" stage before it sends the worker create, so a lost reply is uncertain from then on (domain/registered_run.go:437-441), and recovery keeps the session. A worker with no recorded ID that a name lookup cannot observe gives port.Uncertain (domain/recovery.go:183-191). An egress resource still at "intent" whose name is not found gives port.Incomplete (domain/egress_recovery.go:66-69). An anchor that was never seen and has no Created time gives port.Incomplete (domain/anchor.go:221-224). A home generation at "intent" whose name is not found gives port.Incomplete (domain/home.go:389-393). Each of these is retryable, so status and stop exit 75 with recovery.temporary every time, and no supported command clears the session. Its admission claim survives uncertain cleanup (internal/engine/session/disk/admission.go:56-58), so a nested checkout stays refused. arc42 says the same at docs/architecture/arc42.md:329-331. ID-HOME-10 (features/harness-home.feature:1093) covers only a volume that does appear later.

slice-59 (tasks/work-items.yaml:2806-2827) covers controllers killed between reservation and the first engine create. It retires a session only where durable evidence shows no create was issued, and its last sentence says: "Uncertain creates are out of scope: an engine error reply, or a create whose reply was lost." slice-60 and decision 0034 cover only orphaned .intent-<nonce>.tmp temporaries. Nothing else owns this question, and it is the person's: it is an open security property (docs/decisions/STANDING.md:82-84). Elapsed time, repeated name-only misses and a name-only 404 are not evidence that a create finished; a name that cannot be observed may still be created later. This record was written for T-83 and names no q-number; the coordinator asks the person.

## Considered Options

1. Keep today's rule. Nothing retires an uncertain create, and the only remedy is removing protected state by hand. The cost is a manual, unprotected repair, and until then the session, its admission claim and nested checkouts stay blocked.
2. Treat some engine error replies as proof that nothing was created. The Docker API does not document that an error reply means nothing was allocated, so each error class would need engine-source evidence for the pinned engine version, the rule features/README.md already applies to mount option strings. A lost reply gets no such evidence. The cost is a per-error, per-engine-version proof obligation, and the main uncertain cases stay unresolved.
3. Fence the exact name. Recovery creates its own labelled placeholder under the uncertain resource's name. If that succeeds, the original can no longer land under that name; if it conflicts, the original landed and recovery adopts it through the existing name and full-ID checks. This costs three things. The placeholder must be kept for an unknown time. A volume create on an existing name succeeds without a conflict, so this does not fence the anchor or home volumes. Name uniqueness for containers and networks also needs engine-source evidence. It sits close to the standing rule that no timed-out mutation is retried (docs/decisions/STANDING.md:160-161).
4. Consented retirement. At a real terminal only, stop shows the exact resource names it could not observe and asks y/N. On yes it publishes a durable tombstone that keeps each name and its labels, releases the admission claim and finishes the session. Every later stop of that project inspects the tombstoned names and removes a late arrival by full ID, after the existing ownership checks. Time, a count of misses, JSON and --fingerprint never retire anything. The cost is a new consent surface, which the charter does not yet authorize, so it needs the person's word in any case.

## Decision Outcome

Open: the person has not decided. Recorded 2026-11-24. This record is proposed in MADR's sense: it holds a recommendation, and the decision itself is open.

The recommendation is option 4, consented retirement. It is the only option that lets status or stop finish a session whose create outcome cannot be established without treating a name-only miss as proof. It keeps "Uncertain is not absence" as the only automatic rule: no time, miss count, JSON output or --fingerprint retires anything, and only the person's y/N at a real terminal does. It matches the consented, exact-generation retirement of harness homes (docs/decisions/0030-persistent-harness-home.md; docs/decisions/STANDING.md:126-129) and terminal-only consent (docs/decisions/STANDING.md:112-116). It keeps deletion from protected state an explicit act of the person, as p3-session-state-retention asks (tasks/work-items.yaml:947). Option 2 is refused until engine-source evidence for the pinned engine version exists. Option 3 is named as possible later automation, not a substitute for consent now.

### Consequences

Until the person answers, the code and today's rule stay as they are: every uncertain create remains retryable, and its session and admission claim remain. Whatever the person decides is built by a later item, which the coordinator proposes with scenarios written red first. Nothing in this record is runtime evidence or a support claim.
