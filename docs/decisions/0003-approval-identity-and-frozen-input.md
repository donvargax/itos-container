# Slice-4 approval, host identity and frozen input

## Authority and status

Specified on 2026-11-04 from the coordinator's settled slice-4 assignment and the
person's q-2 answer. This records a specification, not implementation or runtime
evidence. [The scenarios](../../features/approval.feature) carry every behavior
decision and the implementer's red-first handoff. The charter's offline scope holds.

## Decision

The coordinator assigns effective host identity, UID resolution and frozen snapshot
input to slice-4. The effective UID identifies the host user; root is refused.
Both manifest users resolve to that UID and explicit different UIDs are refused.
The worker uses the effective host UID/GID, overriding image Config.User without
username lookup or chown. File-only manifest inspection still reports omitted users
as unspecified. This supersedes ID-DOCTOR-29's image-user comparison when built.

Approval follows q-2's terminal settings/difference review and y/N consent. The
optional exact `--fingerprint` route is available without a terminal; bare approve
without a terminal refuses. Doctor exposes the fingerprint. Host agents can use
the option or fake a terminal, so approval records what was approved, not who.

The spec defines canonical JSON SHA-256, a single active generation per host user
and checkout, protected host storage, atomic replacement and frozen-input selection.
It chooses standard-library JSON/hash operations and existing pinned terminal and
OS libraries, adding no dependency. Schema, canonical fields, storage paths,
failure rules, concurrency and independent fixture oracles live in the feature's
comments so the implementer does not need to infer them from this summary.

Slice-4 wires run's approval refusal and guidance while keeping approved startup
unimplemented. This supersedes ID-RUN-06/07's placement of the interim refusal
before prerequisite inspection. Their argument-preservation assertions remain
required with approved temporary fixtures. Successful run, durable session snapshot
publication and actual worker isolation remain later slices' responsibilities.

## Architecture sections clarified

This overrides the allocation of effective UID work to slices 3/8 in arc42's
**Building-block view**, **Runtime view / Foreground run**, **Deployment view** and
**Decisions**. The matching table entries now assign resolution and frozen input
to slice-4; slice-8 consumes that input and publishes the session snapshot.
It refines **Cross-cutting concepts / Approval and ownership** and the approval row
in **Risks and open questions** with the linked executable specification.

## Consequences and evidence still required

Implementers must update the explicitly superseded doctor/run expectations in the
same implementation, preserving syntax, metadata, argument and no-effect checks.
Specification-only commits do not claim failing-then-passing evidence. The step
commit must first make every new scenario fail at its stated behavior assertion.

Local CLI cases use temporary state and an HTTP Engine API fixture, never Docker.
Contract cases cover root/unavailable identities and publication faults without
privileged processes or production test switches. Shared contracts run against
fakes and adapters. Frozen-input independence and storage placement do not prove
worker inaccessibility: slice-10 owes that runtime evidence in CI.
