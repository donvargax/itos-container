@phase-1
Feature: Owned unstarted worker creation and cleanup
  An approved frozen input can create one accountable, unstarted worker.
  Durable intent precedes create; trusted ownership authorizes removal by ID.
  Lost observations retain recovery evidence instead of permitting another create.

  # Authorities: docs/charter.md, docs/architecture.md (Slices), arc42's
  # Foreground run step 3 and Recovery, and offline-run-lifecycle.md's ownership
  # and cleanup contracts (AC-23 and uncertain-create/repeated-cleanup cases).
  # Slice-9 implements internal/engine/session/{domain,port,<named adapters>},
  # with I/O-free port/porttest fakes and shared fake/adapter contracts. Only cmd
  # assembles adapters. Reuse slice-8's approval.Durable and the existing strict
  # approval/readiness contracts, not a second preflight or approval resolver.
  #
  # This increment exposes its create/cleanup boundary through a TEST-ONLY cmd
  # assembly, following cmd/itos-container/run_contract_test.go. It adds no
  # public command, flag, environment injection, approval path or worker launch.
  # The assembly invokes real production session operations and reports their
  # results plus independently captured state/API observations. Feature steps
  # use that subprocess, never import product packages or inspect source.
  # Creation and cleanup are separate operations, so the caller can reconstruct
  # a controller between them. A successful create returns an owned-unstarted
  # result, never ready/running, a workload exit or permission to start.
  # Slice-10 wires this boundary into run with verified isolation and foreground
  # execution. Preserve all live run/approval scenarios and their snapshot-only
  # effects here. Slice-11 owns public session IDs, status/stop, recovery rendering
  # and concurrent command orchestration; this file promises no executable retry.
  # Slice-12 owns full cancellation, controller-loss and aggregate-budget proof.
  # The settled call/startup/cleanup bounds remain 10/120/30 seconds: accept a
  # caller deadline, cap calls at the lesser remaining budget, and never reset
  # it on reconnect. No unbounded poll, automatic create retry or watchdog.
  #
  # Dependency choice: reuse github.com/moby/moby/client v0.6.2 and api v1.56.1
  # for create/inspect/remove, the already selected native-local-engine checks,
  # Go 1.27.3 JSON/crypto/rand/hex/filesystem APIs in named adapters, and existing
  # golang.org/x/sys v0.48.0 for unavailable locking/no-follow operations. Keep
  # their notices. No new library, handwritten Engine client, Docker subprocess,
  # registry, network service or domain/port standard-library allowlist expansion.
  # Reuse safe storage primitives through engine ports; keep OS a supplied value.
  #
  # Ownership format choice (not a public session-ID format): generate 32 random
  # bytes through a port, encode 64 lowercase hex digits as an operation token,
  # and use worker name itos-container-<token>. Never derive it from checkout
  # text, PID, time, approval generation or an environment variable. Store one
  # schema-1 JSON record at intents/<project>/<token>.json under the SAME protected
  # host root as approvals/snapshots. Refuse entropy failure or an existing token
  # path; never overwrite/reuse another operation or retry create under a new name.
  # An entropy failure returns intent-identity-failed; an occupied token record
  # returns ownership-refused before any engine mutation. Distinct new operations
  # using the same approved snapshot still allocate independent tokens/records.
  # Token/path validation happens before file access. A token is a lookup key,
  # not authorization, and is distinct from slice-11's eventual public session ID.
  #
  # Required record fields are schema, project, generation, fingerprint, snapshot,
  # uid, gid, engine_endpoint, engine_id, name, image_reference, image_id,
  # container_id and stage. All except schema are strings. Snapshot is exactly
  # snapshots/<project>/<generation>.json, relative to the protected root; project,
  # generation, fingerprint, UID/GID, selected endpoint/daemon and both image
  # identities must agree with the independently validated frozen envelope.
  # Name binds the filename token. container_id is empty for stage "intent";
  # stage "owned" requires the full inspected container ID. Stage "absent" retains
  # that same ID after confirmed removal, or records the freshly reconciled ID
  # when an owned update failed. These are ownership progress markers,
  # not ready/running/stopped lifecycle claims. No argv, environment, output,
  # engine response body or duplicated settings are stored in this record.
  # Decode exactly one object, bounded to 65536 bytes and depth 16; reject unknown,
  # duplicate, missing or wrongly typed fields, trailing data, unsupported schema,
  # inconsistent bindings, and partial files. Use the existing strict JSON helpers.
  # Frozen settings remain separately protected and immutable as slice-8 specifies.
  #
  # Apply the approval/snapshot outside-checkout, pinned/no-follow, safe-ancestor,
  # effective-owner, single-link regular-file protections to every intent access.
  # New directories are 0700 and records 0600 regardless of umask. Never repair
  # unsafe state with chmod/chown or follow a substituted path. Serialize mutations
  # of one intent using pinned protected storage; revalidate records and engine
  # identity before each destructive action. The concrete lock API remains an
  # implementation detail for reuse by slice-11, not a new command-level protocol.
  # Publish a complete private file with full-write checks, file fsync, atomic
  # publication and parent/new-ancestor fsync BEFORE sending any create request.
  # Apply the same durability protocol to owned/absent updates without exposing
  # partial records or losing the preceding complete intent. Initial publication
  # is exclusive; later updates can only advance this same operation's identity.
  # A failed/uncertain write returns no successful handoff. Retain any complete
  # visible record, never accept a temporary file or delete evidence on failure.
  #
  # Create exactly one container from the frozen cached image, unstarted. This
  # increment's minimal request has the resolved host UID:GID, NetworkMode none,
  # read-only root, CapDrop ALL, no-new-privileges, pids limit 4096, restart no,
  # auto-remove false, no privileged/host namespaces, ports, mounts or volumes.
  # It has no need for writable worker storage because it NEVER starts. Slice-10
  # will add the approved checkout bind, tmpfs and literal foreground argv.
  # Use an inert /bin/true command with an explicitly empty entrypoint; image
  # acquisition, exec/start/attach, hooks, helpers, chown, prune, volume/network
  # creation and image removal are forbidden. This request is not isolation proof.
  # Labels io.itos-container.intent, io.itos-container.project and
  # io.itos-container.fingerprint carry the token/project/fingerprint. They only
  # corroborate the trusted record, never replace it. Read inspect observations
  # independently of the create reply before durably recording the concrete ID.
  # A container must have the exact name, labels and image config ID and remain
  # in Docker's created state with no start time/PID before owned-unstarted succeeds.
  # After an acknowledged create, the inspected full ID must also equal the
  # returned full ID; the create reply alone cannot establish those observations.
  # An image reference alone is not its distinct config identity.
  #
  # Cleanup needs a safely loaded intent plus the matching current user, selected
  # endpoint/daemon identity and validated frozen binding. Re-pin/check the socket
  # using slice-3's identity rules; do not trust a replacement because its URL is
  # unchanged. Recovery validates the ORIGINAL referenced frozen envelope, using
  # approval's strict codec without rerunning startup preflight. Changed, missing
  # or corrupt current approval and checkout settings do not revoke cleanup
  # authority or replace that envelope. A missing, unsafe or invalid original
  # snapshot refuses cleanup; a matching active approval is never a substitute.
  # This permits removal only, never another create, handoff or worker launch.
  # With an owned ID, inspect that full ID and compare name, labels and
  # image before deletion; never fall back to deleting a same-named replacement.
  # With only intent after a lost create response, inspect the exact random name,
  # then re-inspect the returned full ID and require the same identity tuple.
  # A single matching, never-started resource can be reconciled and removed by
  # that ID. No record, mismatch, conflicting observations or unexpected running
  # resource refuses mutation. Never adopt a known pre-existing foreign resource.
  # For this unstarted-only increment, Running=false alone is insufficient:
  # require created status, PID zero and unset StartedAt during cleanup too.
  # A worker that previously ran and exited needs the later run/stop contract.
  # Never issue a second create for this intent, including after reconstructing
  # the controller. Malformed successful create replies count as uncertain.
  #
  # Successful owned-ID persistence is required before returning owned-unstarted.
  # If that update fails, bounded cleanup may still use the durable pre-create
  # intent plus fresh matching inspections to remove by concrete ID; failed
  # persistence never authorizes starting. Keep the original write failure even
  # when removal succeeds. A persistent storage failure leaves the last complete
  # record for a later cleanup attempt, rather than pretending its update succeeded.
  #
  # Remove only the verified full ID, without force and without volume deletion.
  # A successful DELETE is not absence proof: inspect that ID again. An explicit
  # not-found for an already concrete ID confirms absence, including when DELETE's
  # response was lost. Engine unavailability, malformed replies, timeouts, identity
  # change or a still-present resource do not. Never delete a replacement by name.
  # Persist stage absent before returning cleanup-complete; retain that tombstone
  # in this increment. Later retention/GC belongs to a separate specification.
  # Repeated cleanup revalidates ownership/daemon, confirms the recorded ID absent
  # and returns complete without DELETE or rewriting an already durable tombstone.
  # Re-establish durability when an earlier absent update's fsync was uncertain.
  #
  # IMPORTANT uncertain-create distinction: name-not-found after a lost response
  # cannot prove a delayed/in-flight create will never appear. Keep stage intent,
  # return create-uncertain and do no replacement create or absent update. A later
  # explicit cleanup can reconcile the matching resource once visible. An ordinary
  # timeout or controller reconstruction does not fence the daemon's original
  # request. No automatic retry creates a second resource or discards this record.
  #
  # Outcome names below describe typed session-port results, not public rule IDs
  # or numeric exits: owned-unstarted, cleanup-complete, intent-write-failed,
  # ownership-write-failed, completion-write-failed, ownership-unsafe,
  # ownership-invalid, ownership-refused, intent-identity-failed, create-uncertain
  # and cleanup-incomplete. Failure results retain a safe relative intent reference
  # when one exists, separate cleanup certainty, and no usable startup handoff.
  # Diagnostics never include synthetic sensitive markers supplied in unrelated
  # argv/env/output/body fixtures; do not invent public stop guidance or a child exit.
  #
  # Fixture/step handoff: EVERY phrase below is NEW. All Givens only write inputs,
  # seed independently constructed approval/ownership envelopes or configure
  # fake/HTTP faults and observation barriers; none invokes/validates product work.
  # An approved session fixture contains a disposable checkout, strict offline
  # policy/manifest, independent approval seed, unrelated checkout/state sentinels
  # and an isolated engine. The When runs real slice-8 preflight to obtain Durable
  # before session creation. No approve invocation and no real operator state.
  # For recovery-only Givens, seed the record AND frozen envelope independently
  # and place the described resource in the fake/HTTP fixture. Never create an
  # "owned" fixture by asking the product under test to establish ownership.
  # Default recovery fixtures have stage owned and a matching unstarted resource;
  # name-only reconciliation rows instead seed stage intent with an empty ID.
  # The absent-ID fixture seeds stage owned with a concrete ID no longer present,
  # plus a separate same-named resource; it does not seed a completed tombstone.
  # For changed-input recovery, seed the original owned record, frozen envelope
  # and unstarted engine resource independently; then edit ONLY the specified
  # current input before the checkpoint. Approval replacement is an independently
  # encoded generation B, never an approve invocation. Keep checkout identity,
  # current host identity, engine and original snapshot unchanged. The missing
  # or unsafe snapshot rows keep a matching active approval to catch fallback;
  # a snapshot symlink points to a separately checkpointed outside sentinel.
  # For payload UID mismatch, keep actual file ownership valid and seed a mutually
  # consistent record/snapshot for another UID, so the host-identity comparison
  # (not corrupt JSON or file permissions) refuses. The socket-replacement Given
  # arms a barrier; its When replaces the socket after inspection but before
  # removal. Checkpoint that fixture change separately from product effects.
  # The completion-fault When also installs a same-named foreign replacement
  # after first removal and before the retry; absence checks still target the old ID.
  # Checkpoint after fixture edits; capture all action results, including early
  # unavailable returns, so missing behaviour fails at Then instead of setup.
  #
  # Local scenarios use the test-only cmd assembly. The same observation contract
  # exercises the in-memory engine/state fakes and production disk/SDK adapters
  # against a Unix HTTP Engine API fixture, without any real Docker connection.
  # @linux selects disk/no-follow/Unix-socket fixtures before setup; portable
  # domain/fake contracts remain ordinary unit tests. Fakes do no I/O, and tests
  # never reach across slice/adapter import boundaries. HTTP transport tests count
  # every attempted request, including refused requests, reconnects and retries.
  # Request recording/transport faults belong to adapters/test support, not a
  # product environment switch. Keep SDK acquisition and ambient configuration off.
  #
  # Each positive cycle captures the complete durable intent AT create, independent
  # container inspection BEFORE cleanup, owned-ID persistence, DELETE and the
  # subsequent ID-not-found observation and absent update, in order. Compare exact
  # independently calculated bindings, not substring matches or existence alone.
  # Positive create must actually occur: absence-only checks cannot satisfy it.
  # Fault seams execute real writes/fsync/rename except the named failing operation.
  # Pause/release actions wait for their barrier OR subprocess exit with a bounded
  # harness deadline; missing calls remain data for Then. Reconstruction discards
  # ALL in-memory IDs and service handles; a new subprocess receives only the
  # protected record location and host context. An independent observer may retain
  # the actual engine ID for assertions/final fixture cleanup, never feed it back
  # as product authority. Lost responses happen after a successful server-side
  # create/delete and drop the reply before the SDK receives it.
  # Persistent owned-update faults affect every attempt to publish that update;
  # preserve the intent while allowing independent absence observation. Clear a
  # fault only where When explicitly says so; do not suppress a real write error.
  #
  # @engine cases run ONLY in the existing guarded Linux amd64 CI tier, using
  # T-20's cached fixture reference/config ID/UID/GID and read-only independent
  # observations as features/doctor_runtime_test.go does. Prepare no new image,
  # registry or real approval; seed only disposable test approval after validating
  # fixture inputs. Extend the test-only request recorder with tightly scoped
  # create/inspect/remove forwarding; preserve doctor's read-only recorder. Record
  # unexpected start/acquisition calls BEFORE refusing them, so they fail Then.
  # Retain independently recorded IDs for bounded always-cleanup after failures,
  # verifying identity first; harness cleanup is never product-success evidence.
  # Pre-cleanup observations must show State.Status=created, Running=false,
  # Pid=0 and an unset StartedAt; post-cleanup inspect must be not-found. Audit
  # unrelated containers/images/networks/volumes and checkout/control-state sentinels.
  # Real-engine success proves only creation/ownership/removal, not runtime isolation.
  #
  # Red first: implement complete steps and a compileable unavailable session seam
  # in a test commit, keeping @wip. No success oracle belongs in a stub. Run local
  # cases with @wip temporarily removed; the coordinator dispatches @engine IDs
  # through the existing red_scenarios selector (decision 0002), never locally.
  # Then implement and remove @wip only with assertion-level red/green evidence.
  # New contract subprocesses must export product coverage with GOCOVERDIR using
  # the existing test-assembly pattern; ordinary proof stays engine-disabled.
  # Expected first failing assertions are called out below; none has run in this spec.

  # Positive ownership and cleanup; today the session boundary is unavailable.
  @slice-9 @ID-SESSION-01 @linux
  Scenario Outline: A durable intent owns an unstarted worker through confirmed cleanup
    Given an approved session contract fixture with child umask "<umask>"
    When the session contract creates one worker and cleans it through each local backend
    Then each backend returns owned-unstarted with the independently observed full container ID
    And each create follows the exact durable intent and precedes the durable owned record
    And each worker is inspected as never started before removal by its full ID
    And each cleanup completes only after ID absence and durable absent recording
    And session operations preserve unrelated resources, checkout edits, approval and frozen settings
    And no session operation starts work, acquires images or leaks sensitive fixture values

    Examples:
      | umask |
      | 0000  |
      | 0777  |

  # Slice-8 proves the handoff is frozen; session must also consume that exact
  # handoff. This is the SAME in-flight operation, not a new run permitted to
  # bypass fresh preflight. The Given prepares approved A and a mutation barrier;
  # it performs no preflight/create. A and B have distinct cached digest references
  # and image config IDs in the local fixture, with the same host and daemon.
  # B's approval envelope is independently encoded, never obtained by approving.
  # The When runs real preflight for A, captures its durable handoff, changes ONLY
  # the named current input, checkpoints the edited fixtures, then passes that
  # same handoff to production create and cleanup. Do not replace the handoff,
  # mutate A's snapshot or resolve B through another preflight in the harness.
  # Keep current host, checkout and socket identity unchanged; their revalidation
  # and safe loading of A's protected snapshot remain required and permitted.
  # Record policy/manifest/current-approval reads after handoff independently of
  # product outcomes. Compare the fake request and captured SDK request against
  # independent fixture values and EVERY minimal create setting listed above,
  # including the inert command, empty entrypoint, hardening and absent mounts.
  # Inspect the created worker's image config ID before cleanup; an unchanged
  # record alone cannot prove A's image was used. Checkpointed current inputs and
  # A's snapshot must survive cleanup. This is local request/ownership evidence,
  # not runtime isolation proof. Today the first Then fails because the session
  # boundary cannot create a worker or return owned-unstarted from A's handoff.
  @slice-9 @ID-SESSION-17 @linux
  Scenario Outline: Creation consumes the frozen handoff after current startup inputs change
    Given an approved session contract fixture with a post-handoff change "<change>"
    When the session contract completes A's preflight, applies the fixture change, then creates and cleans through each local backend
    Then each backend returns owned-unstarted after exactly one independently observed create using A's frozen image identity
    And each captured create request uses A's host UID:GID and the specified minimal unstarted configuration
    And the intent, owned and absent records retain exactly A's generation, fingerprint and snapshot binding
    And no policy, manifest or current approval is read again after A's durable handoff
    And each cleanup completes only after ID absence and durable absent recording
    And session operations preserve unrelated resources, checkout edits, approval and frozen settings
    And no session operation starts work, acquires images or leaks sensitive fixture values

    Examples:
      | change                                            |
      | policy now requests forbidden online networking    |
      | manifest now selects B's cached image              |
      | current approval is replaced by B                  |

  # Exercise the check BEFORE owned publication, separately from recovery's
  # identity checks. The Given only configures the approved fixture and a fault
  # on every container inspection after create; preflight remains valid. The
  # When performs one real production create, whose reply has the actual full ID.
  # The independent observer retains the actual never-started resource; only the
  # product-facing inspection is faulted. No fault is cleared during this action.
  # Unavailable, not-found or malformed inspection leaves create-uncertain;
  # a complete contradictory identity/state yields ownership-refused. Both retain
  # the original durable intent with an empty container_id for later revalidation,
  # without owned/absent publication or deletion on the strength of the reply.
  # Missing State is incomplete evidence, not an implicit never-started state.
  # ID-SESSION-01 is the matching-inspection positive control. Today the first
  # Then fails because neither acknowledged create nor its inspection exists.
  @slice-9 @ID-SESSION-16 @linux
  Scenario Outline: An acknowledged create needs independent inspection before ownership handoff
    Given an approved session contract fixture with post-create inspection "<inspection>"
    When the session contract attempts creation through each local backend
    Then each backend reports "<outcome>" after one acknowledged create and an attempted inspection of its returned full ID
    And the original durable intent remains unchanged with an empty container ID and no owned or absent update
    And no successful ownership handoff or worker start was returned or attempted
    And the post-create session journal contains inspection requests only
    And session operations preserve unrelated resources, checkout edits, approval and frozen settings

    Examples:
      | inspection                     | outcome           |
      | unavailable                    | create-uncertain  |
      | explicit not-found             | create-uncertain  |
      | malformed response             | create-uncertain  |
      | missing State                  | create-uncertain  |
      | different full container ID    | ownership-refused |
      | different name                 | ownership-refused |
      | different image config ID      | ownership-refused |
      | different intent label         | ownership-refused |
      | different project label        | ownership-refused |
      | different fingerprint label    | ownership-refused |
      | running state                  | ownership-refused |
      | exited state after starting    | ownership-refused |
      | created state with nonzero PID  | ownership-refused |
      | created state with StartedAt    | ownership-refused |

  # First Then needs a specific new persistence failure, not today's refusal.
  @slice-9 @ID-SESSION-02 @linux
  Scenario Outline: Intent publication must be durable before any create
    Given an approved session contract fixture with intent fault "<fault>"
    When the session contract attempts creation through each local backend
    Then each backend reports intent-write-failed without a successful ownership handoff
    And no container create or later mutation was attempted
    And intent publication leaves "<visible>" and never accepts partial or temporary records
    And session operations preserve unrelated resources, checkout edits, approval and frozen settings

    Examples:
      | fault                 | visible                    |
      | private file creation | no final record            |
      | short write           | no final record            |
      | file fsync            | no final record            |
      | atomic publication    | no final record            |
      | parent fsync          | absent or complete intent  |
      | new ancestor fsync    | absent or complete intent  |

  # Token faults are supplied only through the entropy port. An existing token
  # fixture belongs to another operation and must not be replaced or adopted.
  @slice-9 @ID-SESSION-13 @linux
  Scenario Outline: Failed or colliding intent identity cannot create an untracked worker
    Given an approved session contract fixture with token condition "<condition>"
    When the session contract attempts creation through each local backend
    Then each backend reports "<outcome>" without a successful ownership handoff
    And no container create or later mutation was attempted
    And no existing intent, frozen snapshot or outside target was rewritten

    Examples:
      | condition                           | outcome                |
      | entropy source unavailable          | intent-identity-failed |
      | generated token record already used | ownership-refused      |

  # The positive control prevents snapshot reuse from accidentally reusing an
  # ownership operation. This does not specify concurrent run/stop orchestration.
  @slice-9 @ID-SESSION-14 @linux
  Scenario: Fresh preflights reusing a snapshot allocate independent ownership
    Given an approved session contract fixture with two distinct entropy results
    When two sequential full preflight, create and cleanup cycles reuse the same approved snapshot through each local backend
    Then each operation owns a different random-named container with a separate durable intent
    And both intents bind the same unchanged approved snapshot and their own full container IDs
    And cleanup of either operation never mutates the other's resource or record
    And both cleanups complete after their own ID absence and durable absent recording

  # Seed unsafe intent paths after a safe approval/snapshot; refusal must come
  # from ownership storage, not preflight. Faulted paths are never followed.
  # Required unit test in the session store's disk adapter: through its injected
  # file operations, only the record's metadata reports another UID. Keep host
  # identity, ancestors, snapshot and record payload valid. Assert that record
  # validation is reached and the store refuses with ownership-unsafe. Never
  # change the current UID, chown or elevate the test. The filesystem-defect rows
  # below use real filesystem faults.
  # Stage/ID rows change only stage and container_id in an otherwise valid,
  # independently seeded record. Each has a matching never-started resource, so
  # accepting the contradictory record could authorize deletion. The unsupported
  # stage is the literal "running", not malformed JSON or a running worker.
  @slice-9 @ID-SESSION-03 @linux
  Scenario Outline: Unsafe or invalid ownership evidence authorizes no mutation
    Given a session recovery fixture whose ownership evidence has "<defect>"
    When a new session controller attempts cleanup through each local backend
    Then each backend reports "<outcome>" without a successful ownership handoff
    And no container create or later mutation was attempted
    And the rejected ownership evidence and all outside targets are unchanged

    Examples:
      | defect                                | outcome           |
      | intents directory symlink             | ownership-unsafe  |
      | project directory mode 0777            | ownership-unsafe  |
      | record symlink                        | ownership-unsafe  |
      | record FIFO                           | ownership-unsafe  |
      | record with another hard link         | ownership-unsafe  |
      | record mode 0644                      | ownership-unsafe  |
      | truncated record                      | ownership-invalid |
      | duplicate container_id field          | ownership-invalid |
      | unsupported schema                    | ownership-invalid |
      | intent stage with a container ID       | ownership-invalid |
      | owned stage without a container ID     | ownership-invalid |
      | absent stage without a container ID    | ownership-invalid |
      | unsupported stage                     | ownership-invalid |
      | snapshot fingerprint differs          | ownership-invalid |
      | name differs from filename token      | ownership-invalid |
      | referenced frozen snapshot missing    | ownership-invalid |
      | referenced frozen snapshot symlink    | ownership-unsafe  |

  # First Then fails today: no create/owned-ID publication exists.
  @slice-9 @ID-SESSION-04 @linux
  Scenario Outline: Failure to record a created ID prevents handoff but retains cleanup authority
    Given an approved session contract fixture with persistent owned-record fault "<fault>"
    When the session contract attempts creation and bounded cleanup through each local backend
    Then each backend reports ownership-write-failed after exactly one observed unstarted create
    And no successful ownership handoff or worker start was returned or attempted
    And cleanup uses the durable intent and fresh matching inspections before removal by full ID
    And the last complete ownership record remains until confirmed absence can be durably recorded
    And session operations preserve unrelated resources, checkout edits, approval and frozen settings

    Examples:
      | fault              |
      | short write        |
      | file fsync         |
      | atomic publication |
      | parent fsync       |

  # Compound failure must not lose either the write error or recovery authority.
  # The Given only prepares approved inputs and arms two faults. Create and its
  # first full-ID inspection succeed; the owned-record file fsync then fails
  # BEFORE publication. From that boundary onward, every product-facing engine
  # inspection is unavailable, including reconnect/negotiation; the independent
  # observer still sees the created worker. Keep both faults active until the
  # first create/cleanup attempt has returned and its state has been captured.
  # The creation result remains ownership-write-failed, with cleanup uncertainty
  # reported separately, never success or an erased primary error. The returned
  # safe relative intent reference must identify the surviving complete intent.
  # Only then clear both faults and discard every controller handle/in-memory ID.
  # The new subprocess receives that reference and host context, never the ID
  # retained by the observer. Its explicit cleanup reconciles by name and full ID;
  # it is not a new preflight or a create retry. Check the first attempt's unchanged
  # intent and still-present resource before the successful retry changes them.
  # Today the first Then fails: there is no session create, owned-record write or
  # cleanup result. Unavailable scaffolding cannot satisfy the observed-create
  # assertion; all steps below remain new work for the implementer's red commit.
  @slice-9 @ID-SESSION-18 @linux
  Scenario: An ownership write failure remains recoverable when immediate cleanup is unavailable
    Given an approved session contract fixture with owned-record fsync failure followed by unavailable cleanup inspection
    When creation and cleanup finish with both faults active, then a new controller retries cleanup after both faults are cleared through each local backend
    Then each backend retains ownership-write-failed with separate incomplete cleanup after exactly one observed unstarted create
    And the first attempt returns the exact surviving intent reference without a successful ownership handoff
    And the first cleanup attempts fresh engine inspection without removal or an absent update
    And the first attempt leaves the original complete intent unchanged and the created worker independently observable
    And the new controller reconciles the exact random name and rechecks its full ID against the durable intent
    And each cleanup completes only after ID absence and durable absent recording on the later attempt
    And no replacement create, worker start or foreign removal was attempted
    And session operations preserve unrelated resources, checkout edits, approval and frozen settings

  # Lost response and a malformed success both hide the ID from the product.
  # Reopened cleanup must discover it without a second create or memory handoff.
  @slice-9 @ID-SESSION-05 @linux
  Scenario Outline: A fresh controller reconciles a create whose concrete ID was not received
    Given an approved session contract fixture with create reply "<reply>"
    When creation is attempted and a new controller cleans the surviving intent through each local backend
    Then each backend records create-uncertain after exactly one successful engine create
    And no successful ownership handoff or worker start was returned or attempted
    And the new controller reconciles the exact random name and rechecks its full ID against the durable intent
    And each cleanup completes only after ID absence and durable absent recording
    And no replacement create, worker start or foreign removal was attempted

    Examples:
      | reply                          |
      | lost after server-side success |
      | success with an empty ID        |

  # A delayed create is allowed to appear only AFTER the first cleanup returns.
  # Early unknown/not-found must not discard the intent or authorize a duplicate.
  @slice-9 @ID-SESSION-06 @linux
  Scenario Outline: An uncertain create is not absent merely because it cannot yet be observed
    Given an approved session contract fixture with a delayed create and lookup "<lookup>"
    When creation and first cleanup finish before the delayed worker appears and a new controller then retries cleanup
    Then each local backend reports create-uncertain and retains the exact intent after the first cleanup
    And no successful ownership handoff or worker start was returned or attempted
    And the first cleanup makes no absent update, replacement create or resource removal
    And the later cleanup reconciles and removes exactly the originally delayed worker
    And each cleanup completes only after ID absence and durable absent recording on the later attempt

    Examples:
      | lookup            |
      | explicit not-found |
      | unavailable        |
      | malformed response |

  # Setup seeds trusted ownership independently, then changes only the named
  # observation. Same labels alone never make another resource removable.
  @slice-9 @ID-SESSION-07 @linux
  Scenario Outline: Cleanup refuses foreign or contradictory resource identity
    Given a session recovery fixture with identity condition "<condition>"
    When a new session controller attempts cleanup through each local backend
    Then each backend reports ownership-refused without a successful ownership handoff
    And no container create or later mutation was attempted
    And the ownership record and every candidate resource are retained unchanged

    Examples:
      | condition                                        |
      | matching name and labels without a trusted record |
      | record UID differs from the current host UID      |
      | selected daemon ID differs                       |
      | selected socket is replaced before removal        |
      | recorded ID has a different name                  |
      | matching name has a different image config ID     |
      | matching name has a different intent label        |
      | name inspection and ID inspection disagree        |
      | matching resource is already running              |
      | matching resource has exited after starting       |
      | created resource has a nonzero PID                |
      | created resource has a set StartedAt              |

  # A recorded ID authorizes inspection, not deletion without fresh evidence.
  # Seed a complete owned record, its original frozen envelope and a matching
  # never-started worker independently. Keep host, daemon and socket checks
  # healthy; fault only product-facing container inspections for the entire
  # cleanup attempt. The observer still sees the unchanged worker. No product
  # create or successful cleanup belongs in this Given, and no fault is cleared.
  # The malformed response is invalid JSON in a successful HTTP response; missing
  # State omits that whole object from an otherwise matching inspect response.
  # The memory backend supplies the corresponding unavailable/incomplete facts.
  # These are missing evidence, not contradictory identity or confirmed absence:
  # return cleanup-incomplete with the exact surviving safe intent reference.
  # Never use zero-value state, the record alone or a name lookup as a substitute.
  # ID-SESSION-01 supplies the healthy inspection control and ID-SESSION-08 the
  # explicit full-ID not-found control. ID-SESSION-09 faults observations AFTER
  # removal; this case must refuse to send DELETE in the first place. Today its
  # first Then fails because no session cleanup or container inspection exists.
  @slice-9 @ID-SESSION-19 @linux
  Scenario Outline: Incomplete inspection of an owned ID cannot authorize removal
    Given a session recovery fixture with pre-removal container inspection "<inspection>"
    When a new session controller attempts cleanup through each local backend
    Then each backend reports cleanup-incomplete after attempting to inspect the recorded full container ID
    And each cleanup returns the exact surviving intent reference without a successful ownership handoff
    And all container inspection requests target the recorded full ID without a name lookup
    And no container create or later mutation was attempted
    And the ownership record and every candidate resource are retained unchanged
    And session operations preserve unrelated resources, checkout edits, approval and frozen settings

    Examples:
      | inspection         |
      | unavailable        |
      | malformed response |
      | missing State      |

  # Recovery is not a new run: revalidating mutable startup inputs would strand
  # an owned resource after edits or reapproval. Today no cleanup boundary exists,
  # so the first Then must fail even though fixture ownership is complete.
  @slice-9 @ID-SESSION-15 @linux
  Scenario Outline: Cleanup retains frozen ownership when current startup inputs change
    Given a session recovery fixture with current input change "<change>"
    When a new session controller attempts cleanup through each local backend
    Then each backend returns cleanup-complete after removing only the original recorded worker and confirming its ID absent
    And the intent advances to durable absent with its original snapshot binding and full container ID
    And the original snapshot and changed current inputs remain unchanged by cleanup
    And no replacement create, worker start or foreign removal was attempted

    Examples:
      | change                                  |
      | checkout policy is now invalid          |
      | checkout manifest is now invalid        |
      | current approval is missing             |
      | current approval is corrupt             |
      | current approval is replaced by B        |

  # Here concrete identity was independently recorded before absence. A resource
  # reusing its name must survive; unlike ID-SESSION-06, the old ID is sufficient.
  @slice-9 @ID-SESSION-08 @linux
  Scenario: Cleanup of a confirmed absent ID is repeatable without touching a name replacement
    Given a session recovery fixture with a recorded absent container and a same-named foreign replacement
    When two new controllers successively clean the recorded ID through each local backend
    Then both cleanups return cleanup-complete after confirming the recorded ID absent
    And no container create or later engine mutation was attempted
    And the first cleanup durably records absence and the second preserves that record's bytes, inode and mtime
    And the same-named replacement and unrelated state are unchanged

  # The first Then distinguishes absence proof from DELETE acknowledgement.
  # Lost DELETE with independent not-found is a positive control for reconciliation.
  @slice-9 @ID-SESSION-09 @linux
  Scenario Outline: Removal completes only with an independent concrete-ID absence observation
    Given a session recovery fixture with removal result "<remove>" and following ID observation "<inspect>"
    When a new session controller attempts cleanup through each local backend
    Then each backend returns cleanup outcome "<outcome>" after inspecting the recorded ID
    And the ownership record has progress "<progress>" and no worker outcome is invented
    And only the fully revalidated recorded ID was submitted for removal
    And session operations preserve unrelated resources, checkout edits, approval and frozen settings

    Examples:
      | remove         | inspect            | outcome            | progress |
      | acknowledged   | still present      | cleanup-incomplete | owned    |
      | acknowledged   | unavailable        | cleanup-incomplete | owned    |
      | acknowledged   | malformed response | cleanup-incomplete | owned    |
      | response lost  | explicit not-found | cleanup-complete   | absent   |
      | response lost  | still present      | cleanup-incomplete | owned    |

  # A complete visible tombstone is not durable evidence after fsync failure.
  # Clearing the fault permits reconciliation; it does not justify retrying create.
  @slice-9 @ID-SESSION-10 @linux
  Scenario Outline: Failed completion persistence retains recovery and is repaired only after fresh absence proof
    Given a session recovery fixture with absent-record fault "<fault>"
    When cleanup is attempted, the fault is cleared and a new controller retries through each local backend
    Then each first cleanup reports completion-write-failed despite independently confirmed engine absence
    And the first attempt retains a complete owned or absent record without claiming durable completion
    And each retry confirms the same ID absent and completes only after successful record durability
    And neither retry creates a container nor deletes a same-named replacement

    Examples:
      | fault              |
      | short write        |
      | file fsync         |
      | atomic publication |
      | parent fsync       |

  # Review regression: recover only from independently seeded intent and frozen
  # input. Capture the complete record and successful durability events at DELETE,
  # before the absent update is attempted. Clear the absent fault only after the
  # first cleanup returns, then reconstruct without passing the observer's ID.
  @slice-9 @ID-SESSION-20 @linux @wip
  Scenario Outline: Reconciled identity survives removal when completion persistence fails
    Given an intent-only session recovery fixture with absent-record fault "<fault>"
    When cleanup is attempted, the fault is cleared and a new controller retries through each local backend
    Then each removal follows durable publication of the independently reconciled full ID
    And each first cleanup reports completion-write-failed despite independently confirmed engine absence
    And the first attempt retains a complete owned or absent record without claiming durable completion
    And each retry confirms the same ID absent and completes only after successful record durability
    And neither retry creates a container nor deletes a same-named replacement

    Examples:
      | fault              |
      | short write        |
      | file fsync         |
      | atomic publication |
      | parent fsync       |

  # Persisting reconciled ownership is a prerequisite to removal, including when
  # a prior publication was visible but its parent fsync failed. The first cleanup
  # keeps the fault active. Only the explicit later retry clears it.
  @slice-9 @ID-SESSION-21 @linux @wip
  Scenario Outline: Failed reconciled identity persistence keeps the worker recoverable
    Given an intent-only session recovery fixture with owned-record fault "<fault>"
    When cleanup is attempted, the fault is cleared and a new controller retries through each local backend
    Then each first cleanup reports ownership-write-failed and retains the independently observed worker without removal
    And each cleanup completes only after ID absence and durable absent recording on the later attempt
    And no replacement create, worker start or foreign removal was attempted

    Examples:
      | fault              |
      | short write        |
      | file fsync         |
      | atomic publication |
      | parent fsync       |

  # This CI observation is required in addition to local ordering/fault contracts.
  # Today the positive observed-create assertion fails; an empty audit cannot pass.
  @slice-9 @ID-SESSION-11 @engine
  Scenario: Native Docker creates an owned worker without starting it and confirms its removal
    Given a CI-only session fixture using the independently inspected T-20 cached image
    When the session contract creates an unstarted worker and cleans it using native Docker
    Then the CI observer sees one created worker whose full ID matches the durable ownership record
    And the CI request recorder observed the exact durable intent before forwarding create
    And native Docker reports that worker never started before product removal
    And native Docker independently confirms that exact ID absent after product cleanup
    And the protected record durably records that ID absent without a workload result
    And the CI audit preserves unrelated engine resources, checkout edits, approvals and snapshots
    And no session operation starts work, acquires images or leaks sensitive fixture values

  # Same real-engine boundary, with faults solely in test-only transport/storage.
  # The second controller gets no observer ID. Final harness cleanup is separate.
  @slice-9 @ID-SESSION-12 @engine
  Scenario Outline: Real uncertain creation remains recoverable without launching or duplicating a worker
    Given a CI-only session fixture using the independently inspected T-20 cached image
    And the CI session fixture injects "<fault>" after native Docker accepts create
    When creation fails and a fresh session controller cleans using only the surviving protected intent
    Then the CI creation result is "<outcome>" after exactly one independently observed unstarted create
    And no successful ownership handoff or worker start was returned or attempted
    And the fresh controller revalidates the intent, daemon, random name and concrete ID before product removal
    And native Docker independently confirms that exact ID absent after product cleanup
    And the protected record durably records that ID absent without a workload result
    And no replacement create, worker start or foreign removal was attempted
    And the CI audit preserves unrelated engine resources, checkout edits, approvals and snapshots

    Examples:
      | fault                      | outcome                |
      | lost create response       | create-uncertain       |
      | owned-record file fsync     | ownership-write-failed |
