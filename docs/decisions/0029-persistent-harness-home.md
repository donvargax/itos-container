# Retain an approved harness home with consented retirement

This was decision 0028 when q-73 was answered; it became 0029 when main added
decision 0028 for egress runtime wiring. The selected design is unchanged.

**Selected by the coordinator in q-73, 2026-11-16 01:50 UTC, for the person to
ratify; unimplemented and unproven.** The charter's second milestone, step 3,
permits a volume or tmpfs for the harness. The coordinator selected this
per-checkout persistent named volume and terminal-consented reset/deletion on
the person's standing instruction. No further storage-lifetime answer is owed
before implementation. This record claims no engine feasibility, passing runtime
evidence or supported host combination.

## Context and architecture exception

The first-milestone A2 choice in [arc42](../architecture/arc42.md) excludes
persistent worker state. A harness needs a private writable home; retaining it
lets later approved runs resume state without mounting the host home or placing
it in the checkout. Tmpfs would discard that state at every session ending.
Retention also needs an explicit escape from missing, unusable and obsolete
storage. Policy toggling, routine cleanup and a name-only observation must not
silently discard state or replace it with an empty home.

For the opt-in second-milestone home profile only, this decision overrides A2's
persistent-volume exclusion in **Goals**, **Context and scope**, **Building-block
view**, **Runtime view / Foreground run and its failure paths**, **State and result
interpretation**, **Recovery**, **Deployment view**, **Decisions** and **Glossary**.
Those sections are updated in this specification revision. The omitted-home
profile keeps its existing tmpfs and exact-mount contract. Init, lifecycle hooks,
shared caches, host-home mounts, automatic migration and pruning remain excluded.
The charter and pilot contract are unchanged: ordinary session stop continues to
preserve project state. Only an explicit, separately consented home operation
discards the selected storage.

## Decision

[The home scenarios and their contract](../../features/harness-home.feature)
are the detailed specification, including policy shape, canonical profiles,
diagnostics, public results, deadlines and fixture obligations.

An optional `home: {storage: volume}` grants one named Docker local volume at
`/home/itos`, read/write with copying disabled, root owned by the approved numeric
UID/GID and mode `0700`. Fixed HOME/XDG mappings point inside it; `/tmp` remains a
separate executable tmpfs. No host, checkout or image-home copy seeds the volume.
Harness-specific seeding belongs to slice-35/36. Initialize only a new owned
volume on the unstarted worker through bounded Engine archive operations, and
independently verify ownership, root metadata, mounts and environment before start.
No root helper, image entrypoint, host mountpoint traversal or injected argv runs.

The scope binds full approved checkout identity, host UID/GID and original local
daemon/endpoint. Image, approval generation and XDG state-root changes do not
change it. Protected generation records, a current-generation binding, claims
and one persistent scope lock live under the fixed account-home coordination
boundary. The random 32-byte token is the home ID and generated volume-name
suffix. Durable intent precedes allocation; a protected complete tuple of daemon,
name, CreatedAt, local driver/scope, empty options and exact ownership labels
precedes mounting. Docker supplies no immutable volume ID. A lost create reply
retains uncertainty; there is no replacement create based on name-only absence.

Every home-consuming session claims its exact generation durably before any
allocation or mount can be issued, across selected state roots. Only confirmed
worker absence or fenced proof of no worker create releases its claim. Claims
survive controller death, exited/unstarted workers and unavailable observations.
Ordinary run cleanup and session stop remove session resources and retain the
home, including partly initialized storage. Status distinguishes home observation
from completion of ephemeral cleanup and preserves known or unknown workload exits.

### Person-controlled recovery and reclamation

1. `itos container status --homes` reads the current host UID's protected catalog,
   including obsolete generations and tombstones. It needs no current checkout,
   requests or approval and never scans the daemon for similarly labelled objects.
   Original GID, checkout and engine bindings remain visible after identity changes.
   Entries on a different daemon remain unknown until the person selects that
   original local engine; no command silently switches engines.
2. `itos container stop --home <id> --reset` or `--delete` selects one full home ID,
   without a session positional argument. Show the original identity, action,
   volume, references and permanent data loss, then require terminal y/N consent.
   There is no unattended yes, fingerprint, force or bulk path. JSON keeps its
   single result on stdout and the terminal review on stderr. This is deliberate
   consent, not proof of humanity: a host agent can fake a terminal. Agents do not
   use these commands on real homes.
3. After consent, revalidate the reviewed binding and complete resource tuple
   under the scope lock. Any known session claim refuses with `home-in-use`;
   unsafe or uncertain references also refuse. The person must first recover the
   session with ordinary stop, then consent again. Home management never stops
   workers. A missing volume is not evidence that a claim has gone away.
4. Publish the exact action durably before non-forced removal of only that owned
   volume. Recheck ownership immediately before removal and confirm absence
   independently afterward. A substituted name/tuple refuses. A fully recorded
   but already missing volume needs no deletion call, yet still requires consent,
   no claims and durable retirement. Unreconciled allocation cannot be forgotten.
5. Reset retires the old generation and publishes exactly one unallocated successor
   under the same scope. Its token is reserved in the durable reset intent, so
   retries cannot draw another. Only a later approved run allocates it. A retry of
   reset on the old ID returns the same successor and never removes successor data.
   Delete instead disables that scope without a successor; reapproval or policy
   toggling cannot recreate it. A separate consented reset of the deleted ID can
   enable its sole successor. Old checkout/GID/daemon scopes can therefore be
   reclaimed without touching current storage or transplanting its contents.
6. Pending actions fence new claims through controller loss. Retain original
   ownership, action and retry on failure; an unfinished retry requires renewed
   consent and fresh validation. Keep tombstones and session results. No ambiguous
   delete response, failed fsync or expired operation claims completion. Completed
   retries cannot repeat deletion or affect a newer generation.

No lock spans the prompt. Lock order stays session operation, home scope, short
state transaction; management acquires no session operation lock. Transactions
never span engine calls and the scope lock is released before foreground waiting.
Calls remain bounded to 10 seconds; status/review inspection uses 10 seconds,
startup 120 seconds and session cleanup/management mutation 30 seconds. Management's
mutation total begins after consent; expiry fences later mutations/publication.

## Placement and dependencies

Extend shared `internal/engine/session/{domain,port,disk,docker,...}` for storage,
claims, protected generation records, recovery and retirement. Policy and approval
keep shared validation/canonicalization. Existing run, doctor, approve, status and
stop slices integrate the ports, with consent I/O behind stop's own named adapter;
no slice imports another. Only cmd assembles. Porttest fakes remain I/O-free and
share contracts with the real adapters. No architecture import allowance changes.

Reuse pinned Moby client v0.6.2/API v1.56.1, Go 1.27.4 `archive/tar` and existing
x/sys v0.48.0 descriptor/storage primitives. They already provide the required
Engine, metadata, locking and durable-publication mechanisms; no new dependency,
custom Engine client, engine CLI or network service is selected.

## Evidence and consequences

HOME-01 through HOME-24 remain `@wip`. HOME-01 and all four native scenarios have
step bindings, as does HOME-02; the remaining non-engine bindings are an
implementation handoff.
The coordinator accepted HOME-13's assertion-level red in dispatch
`43797097484`, offline engine job `133068276167`, at
`68be535a07039b29ae294f7d72e4be00b6881e6f`: one selected row failed in 2.9 seconds
at its first observation, "run 1 did not provide the approved persistent private
home: exit 2", with `policy.unknown_key` for the home key. Setup succeeded.
Commit `191a919e440839dd00d5d1d04140ed58c9f1c5ca` binds HOME-14, HOME-15 and
HOME-23. The coordinator accepted their assertion-level red in dispatch
`43815326065`, offline engine job `133126139882`, at
`5fe482e10dbfa42e20036e4b95064aa90077f146`: all eight selected rows completed
and failed at the first product run with `policy.unknown_key` for
`itos-container.yaml/home`. No step was undefined and no fixture failed. These
observations satisfy the native red prerequisite for runtime implementation;
no further red dispatch or storage decision is owed. Steps after those first
observations have not executed yet and need inspection in the native green run.

Givens must independently seed temporary inputs/records and isolated account-home
state without invoking product approval/run/recovery. Both memory and real
disk/SDK-with-recorder paths must exercise the same decisions, with positive
home controls on refusal cases.

HOME-13/14/15/23 require CI-only real-engine red/green evidence, including non-root
initialization, executable temporary binaries, cross-project separation, retained
state, controller-death recovery, active-session refusal and exact native retirement.
HOME-15 explicitly discovers ownership and invokes stop after controller death;
it does not depend on output from a killed process. No real harness/account is used.
Legacy offline/anchor/recovery checks remain required; exact-mount oracles gain a
separate approved-home branch, never permission for arbitrary extra volumes.

Persistent bytes remain hostile worker data and consume space until consented
retirement. They confer no approval or cleanup authority. Lost protected ownership
or irreconcilable engine uncertainty cannot authorize automated reclamation;
host-administrator intervention is outside this command's guarantee. The tuple and
Engine-based initialization are design hypotheses until native evidence passes.
