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
  # An image reference alone is not its distinct config identity.
  #
  # Cleanup needs a safely loaded intent plus the matching current user, selected
  # endpoint/daemon identity and validated frozen binding. Re-pin/check the socket
  # using slice-3's identity rules; do not trust a replacement because its URL is
  # unchanged. With an owned ID, inspect that full ID and compare name, labels and
  # image before deletion; never fall back to deleting a same-named replacement.
  # With only intent after a lost create response, inspect the exact random name,
  # then re-inspect the returned full ID and require the same identity tuple.
  # A single matching, never-started resource can be reconciled and removed by
  # that ID. No record, mismatch, conflicting observations or unexpected running
  # resource refuses mutation. Never adopt a known pre-existing foreign resource.
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
  @wip @slice-9 @ID-SESSION-01 @linux
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

  # First Then needs a specific new persistence failure, not today's refusal.
  @wip @slice-9 @ID-SESSION-02 @linux
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
  @wip @slice-9 @ID-SESSION-13 @linux
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
  @wip @slice-9 @ID-SESSION-14 @linux
  Scenario: Fresh preflights reusing a snapshot allocate independent ownership
    Given an approved session contract fixture with two distinct entropy results
    When two sequential full preflight, create and cleanup cycles reuse the same approved snapshot through each local backend
    Then each operation owns a different random-named container with a separate durable intent
    And both intents bind the same unchanged approved snapshot and their own full container IDs
    And cleanup of either operation never mutates the other's resource or record
    And both cleanups complete after their own ID absence and durable absent recording

  # Seed unsafe intent paths after a safe approval/snapshot; refusal must come
  # from ownership storage, not preflight. Faulted paths are never followed.
  # Wrong-owner fixture supplies a different current UID through the host-identity
  # port; the real adapter must reject the actual record's owner. Never chown or
  # elevate a test. Other rows run real filesystem faults.
  # Stage/ID rows change only stage and container_id in an otherwise valid,
  # independently seeded record. Each has a matching never-started resource, so
  # accepting the contradictory record could authorize deletion. The unsupported
  # stage is the literal "running", not malformed JSON or a running worker.
  @wip @slice-9 @ID-SESSION-03 @linux
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
      | record owned by another effective UID | ownership-unsafe  |
      | truncated record                      | ownership-invalid |
      | duplicate container_id field          | ownership-invalid |
      | unsupported schema                    | ownership-invalid |
      | intent stage with a container ID       | ownership-invalid |
      | owned stage without a container ID     | ownership-invalid |
      | absent stage without a container ID    | ownership-invalid |
      | unsupported stage                     | ownership-invalid |
      | snapshot fingerprint differs          | ownership-invalid |
      | name differs from filename token      | ownership-invalid |

  # First Then fails today: no create/owned-ID publication exists.
  @wip @slice-9 @ID-SESSION-04 @linux
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

  # Lost response and a malformed success both hide the ID from the product.
  # Reopened cleanup must discover it without a second create or memory handoff.
  @wip @slice-9 @ID-SESSION-05 @linux
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
  @wip @slice-9 @ID-SESSION-06 @linux
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
  @wip @slice-9 @ID-SESSION-07 @linux
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

  # Here concrete identity was independently recorded before absence. A resource
  # reusing its name must survive; unlike ID-SESSION-06, the old ID is sufficient.
  @wip @slice-9 @ID-SESSION-08 @linux
  Scenario: Cleanup of a confirmed absent ID is repeatable without touching a name replacement
    Given a session recovery fixture with a recorded absent container and a same-named foreign replacement
    When two new controllers successively clean the recorded ID through each local backend
    Then both cleanups return cleanup-complete after confirming the recorded ID absent
    And no container create or later engine mutation was attempted
    And the first cleanup durably records absence and the second preserves that record's bytes, inode and mtime
    And the same-named replacement and unrelated state are unchanged

  # The first Then distinguishes absence proof from DELETE acknowledgement.
  # Lost DELETE with independent not-found is a positive control for reconciliation.
  @wip @slice-9 @ID-SESSION-09 @linux
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
  @wip @slice-9 @ID-SESSION-10 @linux
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

  # This CI observation is required in addition to local ordering/fault contracts.
  # Today the positive observed-create assertion fails; an empty audit cannot pass.
  @wip @slice-9 @ID-SESSION-11 @engine
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
  @wip @slice-9 @ID-SESSION-12 @engine
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
