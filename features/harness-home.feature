@phase-1
Feature: An approved persistent home for one checkout's harness
  A run can keep its harness state in a private container-managed home.
  Repeated runs of that checkout reuse it; another checkout cannot read it.
  Session cleanup retains the home without retaining a worker or temporary files.

  # Authorities: charter milestone two step 3; container-pilot AC-02/03/06/07/23/25/28;
  # architecture.md Slices; arc42 Building-block view, Foreground run, Recovery,
  # Deployment and A2; approval.feature, offline.feature, anchors.feature and
  # recovery.feature. This is slice-34's spec, not implementation or runtime proof.
  #
  # Storage selected by coordinator q-73, 2026-11-16 01:50 UTC, for the person
  # to ratify on their standing instruction; no further answer blocks building:
  # opt-in named local volume, persistent per approved
  # checkout identity, host UID/GID and local daemon. This follows the registry's
  # recommendation so a later harness can resume across runs. A tmpfs home would
  # lose those sessions. No shared cache, host-home mount, checkout state directory,
  # automatic migration or prune is introduced. Ordinary session stop and launch
  # failure paths retain even an empty or partly initialized home.
  # The person remains able to remove engine resources outside this CLI; a missing
  # recorded volume is reported, never silently replaced with an empty home.
  # Explicit person-controlled reset/deletion below supplies recovery and reclaims
  # obsolete homes; disabling/re-enabling policy is never consent to discard data.
  #
  # Request and approval decisions:
  # - Add optional home: {storage: volume} to supported policy versions 1/2/3.
  #   This additive opt-in does not change any omitted-home policy or its canonical
  #   bytes. The home object is closed, storage is required and exactly "volume".
  #   Reject null, non-object, empty, unknown fields and every other storage value.
  #   Use policy.type/required/unknown_key and new policy.home_storage (exit 2),
  #   with exact /home or /home/storage subjects; existing duplicate/alias/depth
  #   and size rules still hold. No source, target, driver, options or name input.
  # - File-only doctor returns normalized home: {storage: volume} when requested;
  #   it performs no state/engine/home inspection. Recognizing home never lifts
  #   an existing version, egress or credential delivery gate. Version 1 offline
  #   fixtures establish this slice independently of those other implementations.
  # - Effective approval adds top-level home exactly
  #   {storage:"volume",target:"/home/itos",lifetime:"project",mode:"0700"}; its
  #   owner is already bound by host.uid/gid. No additional policy.home copy is
  #   needed in canonical settings. Use offline-home-v1 / allowlist-home-v1 and,
  #   when credential profiles are supported, offline-credentials-home-v1 /
  #   allowlist-credentials-home-v1. The corresponding no-home profiles and bytes
  #   stay unchanged. No engine volume name, contents or creation time is approved.
  # - Doctor and approve show storage, target, owner, persistence after exit/stop,
  #   per-checkout scope, no host-home access, and no implicit deletion. Changes
  #   include /home and /profile in the existing sorted differences. Adding or
  #   removing home needs fresh approval; old fingerprints cannot authorize it.
  #   Preserve existing terminal/fingerprint rules: home alone is not a network
  #   or credential grant. Inspecting or approving never allocates a volume.
  # - Selection freezes home like other settings. A later request/approval change
  #   cannot change an already selected run. Disabling home leaves its retained
  #   volume alone; re-enabling under the same identity reuses its original data.
  #   Image/environment/approval-generation changes do not reset that data.
  #
  # Runtime decisions:
  # - Mount the owned local volume read/write at /home/itos, with NoCopy true,
  #   no driver options, subpath or host bind source. Initialize its root once
  #   as the approved numeric UID:GID, mode 0700. HOME=/home/itos, TMPDIR=/tmp;
  #   XDG_CONFIG_HOME, XDG_DATA_HOME, XDG_STATE_HOME and XDG_CACHE_HOME are literal
  #   /home/itos/.config, /home/itos/.local/share, /home/itos/.local/state and
  #   /home/itos/.cache. The home profile binds these fixed mappings; the worker
  #   can create their directories as needed. These controller values override
  #   image defaults. Existing manifest reserved environment names stay reserved.
  #   The initial volume contains no image-home,
  #   host-home or checkout copy. Slice-35/36 own harness-specific seeding later.
  # - Initialize only the new owned volume through bounded Engine API operations
  #   on the unstarted, durably owned worker: controller-generated directory
  #   metadata via archive copy preserving numeric UID/GID, never an image
  #   entrypoint, root helper, shell,
  #   setup hook or host traversal of Docker's mountpoint. Reuse requires the
  #   root still to be a directory with the approved owner/mode; refuse a mismatch
  #   without recursive chmod/chown, following symlinks or overwriting contents.
  #   Reconcile incomplete initialization only when no worker could have started.
  # - Inspect actual mounts, volume identity, root metadata and environment before
  #   start, not merely create arguments. Verify directory kind/mode through
  #   Engine path metadata and UID/GID through its archive's first root header,
  #   bounded to 64 KiB including extended headers, then close the stream. Use
  #   archive/tar, reject a non-directory root and never extract the archive,
  #   consume file bodies or require a full home inventory for admission.
  #   No new workload instruction executes before existing ownership and boundary
  #   checks plus this verification. No argv wrapper or injected harness flag.
  # - /tmp remains a separate rw,exec,nosuid,nodev,mode=1777 executable tmpfs: moby
  #   prepends noexec to every tmpfs list and only a later exec token clears it,
  #   so exec is named explicitly. No memory
  #   quota is claimed. Native proof copies an ELF probe there and execs that file
  #   directly (not `sh /tmp/script`), then observes a fresh /tmp on a later run.
  #   Executability already exists: ID-HOME-13 fails first on the new persistent
  #   home behavior; its /tmp assertion is deliberately a regression control.
  # - Preserve checkout anchoring, read-only image root, effective non-root UID/GID,
  #   dropped capabilities, no-new-privileges and all network/credential rules.
  #   The home is never mounted into a proxy or another project's worker. Persistent
  #   state is worker-controlled data, never host approval or recovery authority.
  #
  # Ownership and recovery decisions:
  # - Scope key is SHA-256 of existing canonical JSON for
  #   {schema:1,project,uid,gid,engine:{endpoint,id}}. project is approval's full
  #   checkout identity, not its basename or Git remote. Engine/GID/checkout
  #   identity changes select another home; image, generation and XDG state-root
  #   changes do not. Never transplant data or adopt a similarly labelled volume.
  #   The random token is also the public home ID for one storage generation;
  #   the scope key stays stable across an explicitly consented reset.
  # - Keep one protected current-generation binding, retained generation records
  #   and a persistent allocation lock per key under
  #   the existing fixed account-home coordination boundary, independent of chosen
  #   XDG_STATE_HOME. This is host control metadata, never the harness home itself.
  #   Apply existing no-follow, owner, mode, single-link, strict-codec and atomic
  #   durable-publication rules; run never scans other projects or trusts labels
  #   alone. Only explicit status --homes enumerates this UID's home catalog.
  # - Allocate a random 32-byte token and name itos-container-home-<64 lower hex>.
  #   Publish intent before VolumeCreate. Refuse an occupied random name, with no
  #   adoption, deletion or redraw. Inspect and durably record the concrete tuple
  #   before mounting: original daemon, name, CreatedAt, local driver/local scope,
  #   empty options and exact labels binding kind=harness-home, scope and token.
  #   Docker volumes have no container-style immutable ID: do not invent one.
  #   A reconstructed controller checks this complete tuple against protected
  #   ownership, then records initialization before worker start. Include the
  #   home reference and exact tuple in that session's protected ownership record.
  # - Serialize allocation/reconciliation across state roots. Existing checkout
  #   admission still forbids simultaneous workers for the same checkout. Do not
  #   hold a project-state transaction across engine calls or a foreground wait.
  #   Order is session operation, home allocation, then short state transaction;
  #   never acquire another session's operation lock. Home allocation completes
  #   before start and its lock is released before foreground waiting.
  # - No create retry after a lost reply. A name-only not-found cannot fence a late
  #   allocation: retain intent and the session's executable stop retry. A later
  #   stop may reconcile the exact matching volume and durably retain it, never
  #   start a worker, allocate another volume or erase initialization uncertainty.
  #   A known pre-create failure cancels the reservation without engine mutation.
  # - Normal exit, nonzero exit, cancellation, failed startup and stop remove only
  #   owned session resources, confirming worker and checkout-anchor absence as
  #   before. Never issue a home VolumeRemove or remove-volumes/prune operation.
  #   This prohibition is for session cleanup; only explicit home management below
  #   can remove a retained home. Presence after cleanup is not a cleanup leak.
  #   Missing or substituted retained storage is never automatically repaired.
  # - Status stays read-only and adds home:{storage:"volume",state,name} only for
  #   home-bearing sessions. state is present, absent or unknown from independently
  #   checked ownership/inspection; name is the validated recorded generated name.
  #   A pending allocation must be reconciled before cleanup is complete. Once
  #   allocation is durably accounted for, home absence does not prevent removal
  #   of an independently owned worker or make session-resource cleanup incomplete.
  #   An unavailable/mismatched home observation remains a typed problem, never a
  #   fabricated present value. Legacy sessions retain their existing response.
  # - Run uses run.home-refused (1) for changed/missing recorded ownership, unsafe
  #   metadata or an occupied name, run.home-unavailable (75) for unavailable or
  #   uncertain operations/publication, and existing run.isolation-unverified (1)
  #   for an observed wrong worker boundary. Recovery uses its existing not-owned,
  #   invalid-state, unsafe-state, temporary and environment rules. Preserve known
  #   workload exits and exact `itos container stop <id>` retries; null stays null.
  #   Do not expose home file contents or raw engine responses in records/logs.
  # - Existing 10 s call, 120 s aggregate startup, 30 s teardown/stop and 10 s
  #   status budgets include all home locks, calls and publications. Reconnects
  #   cannot reset them; expiry fences further start/mutation. No workload timeout.
  #
  # Person-controlled home management (selected, part of this slice):
  # - status --homes lists only this effective host UID's protected home catalog,
  #   across its checkouts and XDG state roots, including obsolete generations and
  #   deleted tombstones. This explicit mode is separate from project session
  #   status; no daemon-wide discovery or label adoption. It works outside the old
  #   checkout even after its Git identity/path or the person's GID changes. Read
  #   original protected bindings, not current YAML, manifest, approval or Git.
  #   Missing catalog is an empty read-only result, never bootstrap or repair.
  # - Its schema-1 JSON is {schema:1,ok,problems:[],homes:[]}, sorted by home ID.
  #   Each validated entry has id, scope, project, uid, gid, engine:{endpoint,id},
  #   name, state (present/absent/unknown), disposition
  #   (retained/reset-pending/delete-pending/reset/deleted/awaiting-allocation),
  #   successor (home ID or null), and references (sorted session IDs).
  #   Plain output gives the same safe metadata, never contents. Missing/unsafe
  #   records produce typed problems, not trusted entries or partial success.
  #   Inspect only the selected pinned local daemon when it matches the original;
  #   another daemon's entry stays unknown with recovery.environment (3) and
  #   guidance to select that original local engine, never an automatic reconnect.
  # - Add mutually exclusive stop --home <home-id> --reset and
  #   stop --home <home-id> --delete, without a session positional argument.
  #   A full 64-lower-hex ID selects exactly one protected generation; no name,
  #   prefix, --all, --force, --yes, fingerprint or policy-derived consent.
  #   Invalid combinations/IDs return recovery.usage (2) before state/engine I/O.
  #   Session stop retains its old interface and never offers home destruction.
  # - Review the action, exact ID, original checkout/UID/GID/daemon, volume name,
  #   observed presence, references and permanent loss of ALL harness contents.
  #   Require the person at a terminal to answer y/N after this review; no/empty/
  #   EOF/cancellation returns recovery.consent-required (1) without mutation.
  #   Redirected input cannot consent even if it contains y. JSON does not bypass
  #   consent: terminal review/prompt goes to stderr; stdout is one schema-1 home
  #   result. Terminal presence is not proof of humanity; unsandboxed agents can
  #   fake a terminal, as with approve. Agents must not manage real retained homes.
  # - Hold no lock across consent. Reacquire the scope lock and revalidate the
  #   reviewed generation/binding, selected socket/daemon and full volume tuple
  #   before persisting the action and again immediately before VolumeRemove.
  #   A changed review refuses, never retargets consent. Current UID must own the
  #   original protected record; old GID/checkout identity need not still be current.
  #   Their original values remain binding. Never trust labels/name alone, read
  #   home contents or follow its root symlinks. Owned bad root metadata may be
  #   discarded by consent; it may not be repaired for reuse without that consent.
  # - Under that same scope lock, publish a durable per-generation session claim
  #   before any home allocation/mount can be issued. Claims include protected
  #   original state-root/session references across XDG selections. Release only
  #   after full worker absence or fenced proof no worker create was issued.
  #   Controller death, an exited worker, a missing volume or an unavailable state
  #   root never releases a claim. Incomplete, unsafe or uncertain references block
  #   management; return recovery.home-in-use (1) for known claims and the existing
  #   typed unsafe/invalid/temporary failure otherwise. Name the exact session stop
  #   retry and original state-root selection, without stopping it automatically.
  # - No home reset/delete signals/removes a worker. The person first uses ordinary
  #   session stop, then requests fresh home management consent. New run claims,
  #   generation checks before mount/start and management use the same scope lock;
  #   a durable pending action fences new claims through controller death.
  #   Lock contention or a pending action yields run.home-unavailable (75), never
  #   reuse. Order stays session operation, scope, short state transaction. Management
  #   takes no session operation lock and no transaction spans an Engine call.
  # - Persist consented action and original ownership durably before deletion.
  #   Remove only the freshly matching owned volume with force=false, never prune,
  #   remove-volumes, a backing host path, image, anchor or another generation.
  #   In-use responses remain temporary. Confirm absence independently, then
  #   publish completion durably; a DELETE reply or old tombstone alone is not proof.
  #   A fully recorded home already confirmed absent needs no delete call, but
  #   still needs consent, zero claims and durable retirement. A pending allocation
  #   with no concrete tuple cannot be retired from name-only absence: reconcile
  #   its original session first, never fence an uncertain late create by guessing.
  # - Reset retires this exact generation and publishes one fresh unallocated
  #   successor token under the same scope. It creates no volume or worker. Only
  #   a later run with matching current approval may allocate and initialize it;
  #   the old marker must be gone. Retain original records and session outcomes.
  #   Repeating reset on the retired ID reports that same successor with no new
  #   mutation, even after the successor acquired data. It never resets the latest
  #   generation by implication. A different action on that retired ID refuses.
  #   Reserve the successor token in the durable reset intent; recover publication
  #   of that same token/current binding after a crash, never draw another token.
  # - Delete retires the selected generation without a successor and disables that
  #   scope. Even reapproval or toggling home cannot recreate it: run.home-refused
  #   names the deleted home ID and consented reset route. A fresh consented reset
  #   of that deleted ID may publish its sole successor. Thus obsolete checkout,
  #   GID or daemon scopes can be reclaimed without affecting the current scope.
  #   Explicit deletion of a missing recorded home also disables its scope.
  # - Pending/failed deletion keeps the original tuple/action and launch fence.
  #   Fresh status reports uncertainty and exactly [itos,container,stop,--home,id,
  #   --reset|--delete] as retry for that action; each unfinished retry requires
  #   renewed terminal review/consent and original daemon/tuple revalidation.
  #   Lost removal reply may complete after confirmed absence; a same-named
  #   replacement is refused, never deleted. Completion fsync failure stays pending
  #   until durability is re-established. Completed retries make no engine mutation
  #   or new successor, and cannot touch a newer generation. Never erase tombstones.
  # - Management uses recovery.not-owned/unsafe-state/invalid-state/environment/
  #   temporary/internal as in recovery.feature. Successful results use the home
  #   envelope with exactly the selected entry, plus retry (argv or null) per entry;
  #   status --homes uses the same field. Exit 0 means durable requested retirement,
  #   not workload success. New consent/home-in-use failures are exit 1. Status has
  #   10 s total; review inspection has 10 s, and a fresh 30 s mutation total starts
  #   only after consent. Human input waiting holds no engine handle or state lock.
  #   Calls remain at most 10 s within totals; expiry forbids later mutation or
  #   completion publication. No reset/deletion changes sandbox approval or grants.
  #
  # Architecture decision accompanies this specification, not later implementation:
  # docs/decisions/0030-persistent-harness-home.md updates the named arc42 sections
  # now, with all home behavior explicitly proposed and unproven.
  # Extend shared internal/engine/session domain/port/disk/docker responsibilities
  # for home allocation, binding, claims and consented retirement; policy and
  # approval keep shared validation/canonicalization. Integrate through existing
  # run/doctor/approve/status/stop slices, never slice-to-slice imports. Only cmd
  # assembles. Use I/O-free porttest fakes and identical fake/adapter contracts.
  # Override arc42's Goals/Context A2 deferral only for this opt-in second-milestone
  # profile; amend Building-block view, Foreground run, Recovery, Deployment and
  # Decisions to distinguish retained home from ephemeral session resources.
  # Omitted-home offline.feature/lifecycle.feature/recovery.feature promises remain
  # true. Their exact-mount oracles must gain a separate home-profile branch, never
  # an arbitrary-extra-volume allowance. Keep ordinary engine cases credential-free.
  # Reuse pinned Moby client v0.6.2/api v1.56.1, Go 1.27.4 archive/tar and existing
  # x/sys v0.48.0 adapters; no new library/service, engine CLI or dependency pin.
  # This draft changes this feature and its architecture decision only; no bindings
  # or product implementation, and no runtime, red/green or mutation evidence.
  #
  # Step handoff: EVERY phrase below is NEW, deliberately home-specific. Read
  # offline_launch_steps_test.go, offline_local_steps_test.go and
  # offline_runtime_steps_test.go for subprocess, recorder and positive controls;
  # anchors.feature/recovery.feature define the recovery contract reused here.
  # Add steps alone in the red test commit with @wip retained. Feature steps only
  # run the CLI or test-only cmd assembly and inspect public output, protected
  # artifacts and independent engine observations; no product imports or source
  # inspection as an oracle. A new live feature needs a smoke-manifest selection.
  #
  # Givens create temporary Git checkouts, raw policies/manifests, synthetic image
  # metadata, independent canonical approval/snapshot/ownership records, sentinels,
  # faults and barriers. None calls product approve/run/status/stop. Test account
  # home AND HOME/XDG/engine selections are isolated, including the fixed catalog.
  # For runtime fixtures CI alone creates/seeds disposable engine resources and
  # runs marker probes: no real harness, account, credential or model endpoint.
  # Both local backends means memory ports and real disk/SDK against a recording
  # Unix HTTP fixture, through the existing command test assembly. Simulated
  # storage metadata proves decisions, not kernel isolation or executable mounts.
  # When steps collect early refusal, missing barrier and child exit as results,
  # then reach Then; they never require product success to construct later inputs.
  # Positive controls use independent sibling fixtures with the same requested
  # home profile, not a legacy no-home success that could conceal a home refusal.
  # Native probes report errors without hiding them in setup or skipping a row.
  # Harness teardown may delete only its independently tracked disposable test
  # volumes AFTER assertions; product cleanup is observed before harness teardown.
  # Non-engine red runs are local; every @engine red/green run is CI-only. Expected
  # baseline failures below are source-based predictions, not executed red evidence.

  # Policy and approval. All rows fail first because home is currently unknown.
  @slice-34 @ID-HOME-01
  Scenario Outline: Policy accepts only the explicit volume home request
    Given a home policy fixture using version <version> and no credential grants
    When file-only doctor inspects an omitted home, a volume home and the malformed home requests
    Then the volume request succeeds with exactly the normalized home storage and the omitted request keeps its old output
    And malformed home requests have the specified rules and subjects without state or engine access

    Examples:
      | version |
      | 1       |
      | 2       |
      | 3       |

  # Malformed inputs above: null/[]/"volume" -> policy.type /home;
  # {} -> policy.required /home/storage; {storage: null} -> policy.type
  # /home/storage; {storage: tmpfs} and {storage: host} -> policy.home_storage
  # /home/storage; {storage: volume, source: /outside} -> policy.unknown_key
  # /home/source. Duplicate home/storage keys keep the parser's existing rule.

  # Expected red is the requested-home review assertion, not a missing prompt.
  @slice-34 @ID-HOME-02 @local-engine-fixture
  Scenario: Review binds home persistence without creating storage
    Given a home approval fixture with an independently seeded legacy approval and a requested volume home
    When ordinary doctor and terminal approve with consent inspect the requested home
    Then both show the exact home settings and changed fingerprint and approval records the independently expected home profile
    And review explains retention and project scope and sorted home differences before consent
    And no home volume or worker is created and checkout and host-home sentinels are unchanged

  # Expected red is changed-approval refusal and the matching home control:
  # the baseline rejects the new request/record before either behavior.
  @slice-34 @ID-HOME-03 @local-engine-fixture
  Scenario: Adding and removing a home both require approval
    Given home approval mismatch fixtures for adding and removing the home request
    When run attempts each changed request then each independently approved matching request and finally an independently approved re-enabled home
    Then each mismatch reports changed approval with exit 1 before resource mutation and each matching control reaches its requested runtime boundary
    And disabling home preserves its retained volume and re-enabling selects the original volume

  # Frozen-input and local lifetime. Red at each first Then: no home profile,
  # no home mount or no home ownership operation exists in the baseline.
  @slice-34 @ID-HOME-04 @local-engine-fixture
  Scenario: A selected run keeps its home grant after requests change
    Given a home launch fixture with approved home settings A and independently approved omitted-home settings B
    When run selects A and the fixture publishes B before create and then invokes a fresh run through both local backends
    Then the first worker uses A's exact home volume and environment and the fresh worker uses B's legacy tmpfs home
    And A's frozen snapshot and retained home remain unchanged with no post-selection request reads

  @slice-34 @ID-HOME-05 @local-engine-fixture
  Scenario Outline: Fresh controllers reuse one durably owned initialized home
    Given a home launch fixture with synthetic approved UID <uid> GID <gid> and no home record or volume and two sequential runs
    When fresh run controllers execute both runs through both local backends
    Then one volume is allocated after durable intent and owned before mounting and initialized before the first worker starts
    And both workers use the same inspected volume and approved UID GID with HOME at /home/itos and TMPDIR at /tmp
    And all four XDG home directories resolve inside the private home despite conflicting image defaults
    And the second run never initializes or overwrites existing home contents and cleanup removes only session resources

    Examples:
      | uid  | gid  |
      | 1000 | 1000 |
      | 2001 | 3001 |

  @slice-34 @ID-HOME-06 @local-engine-fixture
  Scenario Outline: Home startup failure never launches or silently falls back
    Given a home launch fixture with fault "<fault>" and an independent successful home control
    When both home launches finish through both local backends and explicit recovery follows after the fault is cleared
    Then the faulted launch reports run.home-unavailable with exit 75 and no start while its home control succeeds
    And owned session resources are cleaned or retained with exact recovery guidance and no home is deleted or allocated twice

    Examples:
      | fault                              |
      | intent publication fails           |
      | volume create reply is lost        |
      | owned home publication fails       |
      | root initialization fails          |
      | initialized home publication fails |
      | home inspection is unavailable     |

  # Refusal rows include a positive home control, so an old generic refusal
  # cannot pass. Fixtures independently seed each described record/object defect.
  @slice-34 @ID-HOME-07 @local-engine-fixture
  Scenario Outline: A home name or label alone confers no reuse authority
    Given a retained home fixture with defect "<defect>" and an independent matching home control
    When fresh run controllers attempt home reuse through both local backends
    Then the defective home is refused before start with run.home-refused and exit 1 while the matching home control succeeds
    And no foreign or missing home is adopted repaired replaced removed or read for contents

    Examples:
      | defect                                    |
      | occupied freshly allocated random name    |
      | recorded volume is absent                 |
      | same name with different creation time    |
      | another checkout scope in labels          |
      | another daemon in protected ownership     |
      | nonlocal driver                           |
      | nonempty driver options                   |
      | root owner differs from approved UID GID  |
      | root mode is not 0700                     |
      | home reference belongs to another project |

  @slice-34 @ID-HOME-08 @local-engine-fixture
  Scenario Outline: Home boundary inspection precedes every worker start
    Given a home launch fixture whose worker inspection has "<defect>" and an independent correct home control
    When the home boundary launches are attempted through both local backends
    Then the defective launch reports run.isolation-unverified with exit 1 and no start while the correct home control succeeds
    And the owned unstarted worker is removed and the retained home and unrelated resources remain intact

    Examples:
      | defect                              |
      | wrong home volume                   |
      | host bind in place of home volume   |
      | read-only home mount                |
      | image copy enabled on home mount    |
      | home target inside checkout         |
      | HOME inherited from image           |
      | XDG_STATE_HOME inherited from image |
      | TMPDIR points into persistent home  |
      | another unapproved mount            |

  # Recovery and bounded operations. Independently seeded home-aware records
  # are not accepted by the old codec: first Then is red, never a setup assertion.
  @slice-34 @ID-HOME-09 @local-engine-fixture
  Scenario Outline: Stop removes session resources and retains the recorded home
    Given an independently seeded home-bearing session with state "<state>" and changed current requests and a legacy session control
    When fresh status and stop processes inspect and clean that session twice through both local backends
    Then status reports the original retained home and stop confirms session cleanup while preserving the original workload outcome
    And the home bytes and ownership record remain intact with no home removal initialization or new worker
    And repeated stop makes no destructive engine call and legacy sessions keep their existing output

    Examples:
      | state                         |
      | unstarted after failed launch |
      | exited with observed code 7   |
      | running after controller loss |

  @slice-34 @ID-HOME-10 @local-engine-fixture
  Scenario: Recovery accounts for a delayed home allocation without deleting it
    Given a home-bearing session with a durable allocation intent and an independently delayed successful volume create and a separate replacement-volume control
    When stop observes name-only absence and later fresh stop observes the matching volume through both local backends
    Then the first stop reports temporary uncertainty with its exact retry and the later stop durably accounts for the retained home and completes session cleanup
    And neither stop allocates or starts anything or deletes the home and a same-named replacement is refused by a separate control

  @slice-34 @ID-HOME-11 @local-engine-fixture
  Scenario Outline: Home work shares the existing operation budgets
    Given a home lifecycle fixture delaying "<operation>" and an independent timely home control
    When the home operations exhaust their remaining aggregate budget through both local backends
    Then the delayed home operation reports temporary failure within its original budget and the timely home control completes
    And no late observation causes start mutation or completion publication after expiry and recovery retains the original home intent

    Examples:
      | operation                      |
      | allocation lock during startup |
      | volume create during startup   |
      | initialization during startup  |
      | home inspection during status  |
      | reconciliation during stop     |

  # Reuse the lifecycle assembly's caller deadlines and recorder barriers, never
  # long sleeps or production timeout switches. SDK contexts carry the lesser
  # call/phase deadline; adapters retain late handles under existing budget rules.
  @slice-34 @ID-HOME-12 @local-engine-fixture
  Scenario: Home allocation is shared across selected host state roots
    Given two home launch fixtures for the same checkout and daemon with different XDG state roots and an allocation barrier
    When concurrent run controllers contend and a later run follows after the first worker is gone through both local backends
    Then only one home is allocated and only one worker is admitted at a time and the later run reuses that home
    And no state-root change resets ownership or initializes existing home contents

  # Home management. Every binding is new. Givens independently encode retained
  # and retired generations, claims and engine objects, never run setup through
  # the product. When collects every early return; the first Then must assert the
  # specified home result, never merely a nonzero exit or absence of deletion.
  @slice-34 @ID-HOME-16 @local-engine-fixture
  Scenario: Retained and obsolete homes remain discoverable outside their checkouts
    Given a home catalog fixture with current obsolete deleted and other-user homes across state roots and an independent empty catalog
    When fresh plain and JSON status --homes inspect both catalogs outside the recorded checkouts through both local backends
    Then the owned catalog reports the exact sorted home metadata and the empty catalog succeeds without creating state
    And obsolete GID checkout and daemon bindings remain discoverable while other-user entries and all home contents stay private
    And another daemon is unknown with recovery.environment and neither catalog is changed or used to discover engine resources by labels

  @slice-34 @ID-HOME-17 @local-engine-fixture
  Scenario Outline: Discarding home state requires consent to one reviewed action
    Given independent owned home management fixtures with response "<response>" and a terminal consenting control for each action
    When reset and delete are requested in plain and JSON modes through both local backends
    Then each unconsented request reports recovery.consent-required with exit 1 and changes no home record claim or engine resource while its consenting control completes
    And each consenting review names the exact action home and original binding and warns of permanent loss before accepting consent
    And JSON stdout contains only the home result and no prompt and no approval record or checkout data changes

    Examples:
      | response           |
      | terminal no        |
      | terminal empty     |
      | terminal EOF       |
      | terminal cancelled |
      | redirected yes     |

  # A valid consenting control prevents generic legacy argument rejection from
  # satisfying these refusal rows. Publication faults precede any engine mutation.
  @slice-34 @ID-HOME-24 @local-engine-fixture
  Scenario Outline: Invalid or unproven home actions cannot discard storage
    Given an independently recorded home with management request condition "<condition>" and an independent valid consenting control
    When home management is attempted through both local backends
    Then the request reports <rule> with exit <exit> without volume removal or successor publication while the control completes
    And usage errors perform no state or engine access and failed publication or lock expiry preserves original ownership within the mutation budget

    Examples:
      | condition                             | rule                  | exit |
      | truncated home ID                     | recovery.usage        | 2    |
      | both reset and delete                 | recovery.usage        | 2    |
      | home ID and session ID                | recovery.usage        | 2    |
      | home ID without action                | recovery.usage        | 2    |
      | unattended yes override               | recovery.usage        | 2    |
      | action intent fsync fails             | recovery.temporary    | 75   |
      | scope lock exhausts the budget        | recovery.temporary    | 75   |
      | pending allocation with name-only 404 | recovery.temporary    | 75   |
      | delayed create landed at intent       | recovery.temporary    | 75   |
      | claim state root unavailable          | recovery.temporary    | 75   |
      | unsafe session claim                  | recovery.unsafe-state | 1    |

  @slice-34 @ID-HOME-18 @local-engine-fixture
  Scenario Outline: Consented reset recovers an unusable home without automatic replacement
    Given an independently recorded unreferenced home that is "<condition>" and an independently approved matching run
    When run is attempted before consent then terminal reset completes and fresh run and repeated reset of the old ID follow through both local backends
    Then the first run reports "<before>" without replacement and reset durably retires exactly that generation before any new allocation
    And only the later approved run initializes one fresh successor with no old marker and repeated reset leaves the successor marker unchanged
    And original records remain as tombstones and the reset command creates no volume worker or approval and changes no unrelated state

    Examples:
      | condition                          | before                       |
      | present with a retained marker     | retained marker with exit 0  |
      | missing after external removal     | run.home-refused with exit 1 |
      | deleted by prior consent           | run.home-refused with exit 1 |
      | owned with unsafe worker root mode | run.home-refused with exit 1 |

  @slice-34 @ID-HOME-19 @local-engine-fixture
  Scenario Outline: Explicit deletion reclaims obsolete homes without disabling another scope
    Given independent old and current home generations differing by "<identity>" with no claims and missing-volume and current-scope deletion controls
    When the person selects each original daemon deletes only the chosen home with consent and retries through both local backends
    Then exactly the selected owned storage is absent and its scope is durably disabled while the other home retains its marker and remains reusable
    And repeated deletion and deletion of confirmed missing storage issue no volume removal and preserve the original ownership tombstone
    And an approved run for a deleted scope is refused despite policy toggling until a separate consented reset permits one fresh home

    Examples:
      | identity          |
      | host GID          |
      | checkout identity |
      | local daemon      |

  @slice-34 @ID-HOME-20 @local-engine-fixture
  Scenario Outline: Home destruction requires exact ownership at the mutation boundary
    Given a home management fixture with substitution "<defect>" and an independent valid owned control
    When reset and delete receive terminal consent and the substitution is exercised after review and immediately before removal through both local backends
    Then the defective request reports <rule> with exit <exit> before removal or retirement while the valid control completes
    And no substituted resource or protected binding is adopted repaired deleted or read for contents

    Examples:
      | defect                             | rule                   | exit |
      | another host UID in the record     | recovery.not-owned     | 1    |
      | replaced protected generation      | recovery.not-owned     | 1    |
      | symlinked home record              | recovery.unsafe-state  | 1    |
      | missing original ownership tuple   | recovery.invalid-state | 2    |
      | same name different creation time  | recovery.not-owned     | 1    |
      | wrong scope or token labels        | recovery.not-owned     | 1    |
      | changed driver or options          | recovery.not-owned     | 1    |
      | replaced selected socket or daemon | recovery.not-owned     | 1    |
      | unavailable original daemon        | recovery.temporary     | 75   |

  # Test both race orders with deterministic barriers: a launch claims while the
  # prompt waits, and a launch attempts a claim after pending deletion is durable.
  # Each invocation's own operation budget applies; never sleep to order events.
  # The uncertain-create row releases its delayed matching resource before session
  # stop; neither controller death nor name-only absence supplies that evidence.
  @slice-34 @ID-HOME-21 @local-engine-fixture
  Scenario Outline: Home management fences launches and protects every retained session claim
    Given a home shared across state roots with reference "<reference>" and barriers at consent and durable retirement intent
    When reset and delete contend with launch then ordinary stop recovers the referenced session and fresh consented management is retried through both local backends
    Then the referenced home is refused with recovery.home-in-use before any home mutation and no management attempt stops a worker
    And a launch claiming during consent prevents retirement while a durable pending action prevents a later mount or start
    And only confirmed session cleanup releases the claim so fresh consent can retire the exact home without changing workload knowledge

    Examples:
      | reference                                 |
      | running with live controller              |
      | running after controller death            |
      | exited worker not yet removed             |
      | unstarted owned worker                    |
      | uncertain worker create                   |
      | paused launch before worker create        |
      | running with a held session operation lock |

  # The replacement control substitutes a foreign same-named volume after the
  # first confirmed removal and before retry. Hold absence inspection after lost
  # replies so that the first attempt must actually retain uncertainty.
  @slice-34 @ID-HOME-22 @local-engine-fixture
  Scenario Outline: Interrupted home retirement remains bounded and recoverable by exact ID
    Given an unreferenced owned home with management fault "<fault>" and independent timely and replacement-volume controls
    When consented reset and delete encounter the fault and fresh status and consented retries follow through both local backends
    Then the first action reports recovery.temporary within its original budget or loses its controller without claiming completion and the timely control completes
    And fresh status retains the original action ownership and exact home retry and pending retirement refuses new run claims
    And after the fault clears retry confirms absence and durable completion without repeated successful removal or more than one successor
    And the replacement-volume control is refused before deletion and no expiry permits a late mutation or completion publication

    Examples:
      | fault                                                 |
      | removal reply lost and absence inspection unavailable |
      | completion publication fsync fails                    |
      | controller dies after durable action intent           |
      | controller dies after removal                         |
      | engine removal consumes remaining budget              |
      | engine reports volume in use                          |

  # Native evidence. First assertions require the absent persistent-home behavior;
  # /tmp execution, denied host access and teardown are explicit regression controls.
  @slice-34 @ID-HOME-13 @engine
  Scenario: Non-root runs retain home state and execute temporary binaries
    Given a CI home checkout with a cached approved image for the actual non-root controller UID GID and isolated host-home sentinels
    When two fresh product runs write then read a home marker and directly execute a copied ELF probe from /tmp
    Then both probes report the approved IDs and a writable private /home/itos and the second reads the first home marker
    And each temporary ELF executes successfully while the second run cannot read the first temporary marker
    And independent inspection finds only the approved home checkout and tmpfs mounts with no host-home or image-home copy
    And host-home sentinels and checkout ownership stay unchanged and both workers and anchors are absent while the home remains

  @slice-34 @ID-HOME-14 @engine
  Scenario: Equal checkout names never share harness state
    Given two CI home checkouts with equal basenames and the same approved image and host UID GID
    When their product workers write distinct private home markers and each checkout is run again
    Then each later worker reads only its own retained home marker from a different owned volume
    And neither worker can read the other home or the synthetic host-home sentinel and a same-home read control succeeds
    And cleanup preserves both checkout edits and both homes while removing both sessions

  # Controller death cannot supply retry output: use the independently captured
  # protected registration, then explicitly invoke fresh status and stop.
  @slice-34 @ID-HOME-15 @engine
  Scenario Outline: Home state survives every session ending without preserving a worker
    Given a CI home checkout with a cached image and a probe that writes a home marker before "<ending>"
    When the product session ends that way and fresh product status discovers the independently recorded session ID then fresh product stop recovers it and status inspects it again
    Then fresh status identifies the original retained home and confirms worker and anchor absence without inventing a workload exit
    When a later independently approved product run reads the home and temporary markers
    Then the later worker reads the original home marker and cannot read the old temporary marker
    And no product operation deletes a home or changes unrelated resources and the later worker is also cleaned

    Examples:
      | ending                    |
      | normal exit zero          |
      | observed exit seven       |
      | controller cancellation   |
      | controller death and stop |

  # Slice-40 depends on T-48 for staging and recovery, slice-34 for the private
  # home and slice-30 for source checks and terminal consent. The coordinator
  # selects implementation independently of the Claude and Codex schedules.
  # ADR 0025 question 5 reserves live credential use, including a smoke check,
  # for the person's decision.
  # Every account, key, database, approval and endpoint below is a synthetic fixture.
  #
  # Slice-40 extends internal/slices/doctor for inspection, internal/slices/approve
  # for consent and internal/slices/run for delivery under the frozen grant.
  # internal/slices/status reports that grant and internal/slices/stop recovers
  # the recorded session. internal/engine/policy validates the request and
  # internal/engine/readiness checks the host source and revalidates its identity.
  # internal/engine/approval records and freezes the canonical grant.
  # internal/engine/session owns staging, mount verification and removal of the
  # worker's login on exit or recovered stop. Only cmd assembles the adapters.
  #
  # One legacy grant identifies the pair (absolute auth.json path, provider ID)
  # in the approval for this checkout. A legacy API-key entry has no independent
  # account ID. The person controls which key occupies that provider slot.
  # A matching approval permits a fresh copy on each requested run. Each copy
  # lives only as long as its worker. Provider permissions still apply in full.
  #
  # Use one optional opencode-auth entry in version 4's credentials list.
  # The closed entry requires kind, source and provider. Source names the
  # trusted host account home's .local/share/opencode/auth.json by its literal
  # absolute path. File-only inspection checks path syntax without a home lookup.
  # Provider is one nonempty case-sensitive literal provider ID. Wildcards and
  # implicit selection are invalid. Earlier versions keep their existing forms.
  # Version 4 is also the format in slice-32's draft. Each kind has its own
  # delivery gate. A request containing an unavailable kind fails as a whole.
  # ID-POLICY-15 and ID-POLICY-77 use 4 as an unsupported version. When 4 is
  # implemented, use the next unsupported version there and update the advice.
  #
  # Bind kind, source, provider and the fixed read-only worker target
  # /run/itos-container/credentials/opencode-auth in the canonical grant.
  # The fingerprint excludes source bytes, their hashes and inode metadata.
  # Only run reads the checked descriptor, after matching frozen approval and
  # slice-30's full source revalidation. Run selects the legacy type "api"
  # entry for that provider and projects its nonempty string key into OpenCode
  # 2.3.2's auth import format. The staged JSON is an array with one account.
  # Its only fields are a fresh worker-local id independent of the key,
  # integrationID equal to the approved provider, label "itos-container",
  # active true and value containing exactly type "key" and the selected key.
  # T-48 supplies the owned memory-backed staging, mount checks and teardown.
  #
  # The workload imports the staged account inside the worker with
  # opencode --standalone auth import /run/itos-container/credentials/opencode-auth
  # OpenCode 2.3.2 keeps the current account active when the worker database
  # already has that integration. After import the workload takes integrationID
  # and id from the staged account and selects it in the same worker store with
  # opencode --standalone auth switch <integrationID> <id>
  # Only after both commands succeed does it send a provider request with
  # opencode --standalone run --model <integrationID>/<fixture-model> <prompt>.
  # The probe keeps the worker live for inspection, rotation or session end.
  # https://opencode.ai/docs/cli/commands/#auth
  # https://github.com/anomalyco/opencode/blob/v2.3.2/packages/schema/src/credential.ts
  # https://github.com/anomalyco/opencode/blob/v2.3.2/packages/cli/src/commands/handlers/auth/import.ts#L21-L36
  # https://github.com/anomalyco/opencode/blob/v2.3.2/packages/cli/src/commands/handlers/auth/switch.ts
  #
  # OpenCode v2 imports legacy credentials during migration and saves new or
  # updated logins in SQLite. Removing auth.json alone can leave a usable login.
  # https://opencode.ai/docs/cli/providers/#storage
  # The person still needs to decide whether to support SQLite-only accounts.
  # If so, which supported host operation identifies one approved account among
  # several for the same provider and exports its current key or refreshed
  # tokens? Who owns OAuth refresh, concurrent updates and revocation?
  # Pending that decision the file grant reads only the named legacy slot.
  # A missing file or rejected stale key cannot authorize database fallback,
  # interactive login, a host callback or copying changes back to the host.
  # A worker must never receive the host database, its sidecars or data directory.
  #
  # Givens write independent policy/approval fixtures, private temporary host
  # stores and distinct generated markers. Both the trusted account home and
  # HOME/XDG resolve to the fixture. Clients use isolated configuration and
  # only the accounts seeded for that scenario. Run steps capture early CLI
  # exits for Then. Only @engine rows use native workers, in CI. They invoke the
  # released OpenCode 2.3.2 client against an allowlisted fake HTTPS provider
  # configured for the fixture's provider IDs. It records request IDs, synthetic
  # principals and authentication outcomes, and discards headers and key bytes.
  # Only product delivery supplies the granted key to a worker. No model is contacted.
  # https://github.com/anomalyco/opencode/releases/tag/v2.3.2
  # 01 and 02 use existing steps. 03 reuses terminal approval. 04 reuses
  # command invocation and approval refusal. 05 reuses command invocation and
  # the source observer assertion. 09 reuses command invocation and the exit
  # code check. The remaining steps need red-first bindings.

  @slice-40 @ID-OPENCODELOGIN-01
  Scenario: File-only inspection identifies the exact OpenCode provider requested
    Given policy inspection has isolated host state and a recording engine endpoint
    And the policy file contains:
      """
      version: 4
      manifest: a.json
      credentials:
        - kind: opencode-auth
          source: /fixture-account/.local/share/opencode/auth.json
          provider: opencode-go
      """
    When itos-container runs with "doctor --policy-only --json"
    Then its policy JSON contains this effective policy:
      """
      {"version":4,"manifest":"a.json","network":{"mode":"offline"},"credentials":[
        {"kind":"opencode-auth","source":"/fixture-account/.local/share/opencode/auth.json","provider":"opencode-go"}
      ]}
      """
    And it exits with code 0
    And the engine recorder has no connections

  @slice-40 @ID-OPENCODELOGIN-02
  Scenario Outline: A grant cannot request an implicit provider or a second OpenCode login
    Given policy inspection has isolated host state and a recording engine endpoint
    And the policy file contains:
      """
      version: 4
      manifest: a.json
      credentials: <entries>
      """
    When itos-container runs with "doctor --policy-only --json"
    Then its policy JSON reports exactly these problems:
      | rule   | path   | exit |
      | <rule> | <path> | 2    |
    And it exits with code 2
    And the engine recorder has no connections

    Examples:
      | entries                                                                                                                           | rule                        | path                    |
      | [{kind: opencode-auth, source: /a/auth.json}]                                                                                      | policy.required             | /credentials/0/provider |
      | [{kind: opencode-auth, source: /a/auth.json, provider: '*'}]                                                                       | policy.credential_provider  | /credentials/0/provider |
      | [{kind: opencode-auth, source: /a/auth.json, provider: opencode-go}, {kind: opencode-auth, source: /a/auth.json, provider: other}] | policy.credential_duplicate | /credentials/1/kind     |

  @wip @slice-40 @ID-OPENCODELOGIN-03 @local-engine-fixture
  Scenario: Terminal approval shows which OpenCode key the worker can use
    Given a disposable OpenCode checkout requesting "opencode-go" from a private auth.json with two provider keys
    And its network allowlist names the fake provider for the terminal review
    And two fresh unapproved fixtures each request only "opencode-auth" for the same provider under an offline policy
    When bare approve runs at a terminal and receives "y" after the review
    Then the review before consent names the checkout, provider, absolute source and read-only worker target
    And it explains the one-run copy, full provider permissions, rotation of the host file entry and exposure through workload output or allowed hosts
    And approval records exactly the requested grant without opening either credential store for data
    And neither the review nor the record contains a synthetic key or its hash
    When the first offline fixture tries approve without a terminal with its independently calculated exact fingerprint
    And the second offline fixture tries bare approve with yes on redirected stdin
    Then fingerprint approval reports approve.terminal-required and redirected approval reports approve.confirmation-required
    And both unattended attempts exit 1 without recording a grant or staging a login

  @wip @slice-40 @ID-OPENCODELOGIN-04 @local-engine-fixture
  Scenario Outline: Run requires approval for this checkout and this provider
    Given a disposable OpenCode checkout requesting "opencode-go" from a private auth.json with two provider keys
    And its only protected approval is "<approval>"
    When itos-container runs with "run -- opencode-grant-probe"
    Then run refuses approval "<rule>" with exit 1 and names "itos container approve"
    And the OpenCode refusal leaves source data unread and creates no staged login or worker

    Examples:
      | approval                                   | rule                 |
      | absent                                     | run.approval-missing |
      | for the other provider at the same path    | run.approval-changed |
      | for the same slot in another checkout only | run.approval-missing |

  # Seed a host database in every row. The wrong-path file, symlink target and
  # group-readable file each hold a valid entry for the requested provider.
  # Start the observers after setup and before run. Watching the target catches
  # an open through the link. Count a staged file even if run later unlinks it.
  # In the mixed row both sources are usable and approved. The fixture supports
  # opencode-auth delivery and leaves github-token delivery gated.
  @wip @slice-40 @ID-OPENCODELOGIN-05 @local-engine-fixture
  Scenario Outline: Run refuses the whole credential request when it cannot deliver a grant
    Given a disposable checkout whose version 4 policy requests the "opencode-go" file grant with "<condition>"
    And the fixture independently records matching approval for the entire request
    And the source observer watches <unopened sources> with an independent read control
    When itos-container runs with "run -- opencode-grant-probe"
    Then run exits with code <exit> and reports <refusal>
    And throughout the run the engine recorder counted zero worker create requests and the staging recorder counted zero login file creations
    And the source observer saw its control read and no credential data open or read
    And no synthetic key appears in the run diagnostics or records

    Examples: Unusable OpenCode source or provider entry
      | condition                                                  | refusal                                                   | exit | unopened sources                                   |
      | auth.json absent with the selected key only in host SQLite | "credential.missing" for the approved source path         | 3    | the host database                                  |
      | a different private auth.json named in request and grant   | "credential.opencode_source" for the approved source path | 1    | the host database and the wrong-path auth.json     |
      | a symbolic link in place of auth.json                      | "credential.symlink" for the approved source path         | 1    | the host database and the symlink target           |
      | group-readable auth.json                                   | "credential.permissions" for the approved source path     | 1    | the host database and the group-readable auth.json |
      | auth.json with malformed JSON                              | "run.opencode-entry" for the approved source path         | 1    | the host database                                  |
      | auth.json has only a valid API-key entry for OpenCode-Go   | "run.opencode-entry" for the approved source path         | 1    | the host database                                  |
      | duplicate JSON members for the selected provider           | "run.opencode-entry" for the approved source path         | 1    | the host database                                  |
      | duplicate key fields inside the selected entry             | "run.opencode-entry" for the approved source path         | 1    | the host database                                  |
      | an empty key for the selected provider                     | "run.opencode-entry" for the approved source path         | 1    | the host database                                  |
      | a selected OAuth entry that needs refresh                  | "run.opencode-entry" for the approved source path         | 1    | the host database                                  |

    Examples: A recognized kind whose delivery is gated
      | condition                                          | refusal                                                                              | exit | unopened sources                                                               |
      | valid opencode-auth followed by valid github-token | "policy.version" at /version and identifies github-token as unavailable for delivery | 2    | the host database, the requested auth.json and the requested github-token file |

  # The cooperative probe uses the delivered grant. Its read and write checks
  # report booleans. Independent controls outside the observed trees validate
  # each marker search (raw and base64 at all three offsets). q-56 leaves
  # deliberate disclosures by a workload in refused hostnames to
  # p1-platform-log-redaction. Inspect Env, argv, labels and mounts while each
  # worker is live. Include raw create requests, protected approvals, snapshots
  # and recovery state, and both plain and JSON output from doctor and status.
  # Teardown publishes each session's protected refusal log after its proxy stops.
  # After exit, copy the original volume's database and sidecars as in 08
  # before opening SQLite or OpenCode. Select the pre-existing ID explicitly
  # so its saved key can be tested even if cleanup cleared the active selection.
  @wip @slice-40 @ID-OPENCODELOGIN-06 @engine
  Scenario: The worker receives only the selected OpenCode provider key
    Given two CI checkouts with isolated homes, one granted "opencode-go" and one granted "other-provider" from the same private auth.json
    And the fixture host has distinct markers for those two keys, an extra field and its OpenCode database
    And the first checkout's retained home has a different active "opencode-go" account whose key is stored there only in SQLite
    And an independent OpenCode control uses a copy of that home made before delivery
    And the fake provider registers the two granted keys and the retained account's key under three distinct synthetic principals
    When each granted worker runs "opencode --standalone auth import /run/itos-container/credentials/opencode-auth"
    And each switches to the credential ID in its staged file before requesting its provider with OpenCode at the allowlisted fake endpoint
    And each worker sends its proxy a CONNECT request on port 443 for a distinct harmless hostname outside its allowlist
    And the control requests "opencode-go" with OpenCode using its existing selection
    Then the fake provider authenticates both workers and attributes every authenticated request from each to its granted principal
    And the provider authenticates the control as the pre-existing "opencode-go" principal and receives no attempt with that key from either granted worker
    And the first worker's retained database contains both accounts with the staged credential ID active for "opencode-go"
    And each worker reads a staged file matching the single-account import format for its approved provider and key with no extra host field
    And an attempted write to each staged file fails
    And neither worker can read the other checkout's granted key, original auth.json, host database or its sidecars
    And native mount inspection finds only the staged read-only credential file among credential mounts and both host stores retain their bytes
    And the marker checks find no synthetic marker in either worker's create request or live inspect data, its checkout, proxy stdout or engine logs
    And the same checks find none in either checkout's approval or live session records, doctor or status output or run diagnostics so far
    And the same checks detect each marker and encoding in its independent control
    When both granted workers exit normally and their run commands return
    Then the same marker checks rescan every file in both checkout trees and find no synthetic marker
    And immediate read-only inspection of the first checkout's original retained database finds the pre-existing account ID and no row for that worker's staged credential ID
    And the first worker's granted key is absent from that database and its SQLite sidecars
    And each session's protected egress-refusals.jsonl contains the refusal for the harmless hostname its worker requested
    And for both runs the same marker checks find no synthetic markers in the full refusal log, final diagnostics or final session records
    When the first checkout selects matching approval with only the credential grant removed
    And a fresh worker on that same volume selects the pre-existing "opencode-go" account by ID and requests the fake provider without importing credentials
    And the independent control repeats its request from the pre-delivery copy
    Then the fake provider authenticates the fresh worker and the independent control as the pre-existing "opencode-go" principal

  # The fake provider recognizes two key generations for one synthetic principal.
  # The fake provider identifies the generation on each request without logging
  # the key. One checkout runs three workers in sequence under the same approval.
  # Capture the ID from each worker's staged file before that worker exits.
  @wip @slice-40 @ID-OPENCODELOGIN-07 @engine
  Scenario: Host rotation supplies a fresh key only to the next granted run
    Given a CI OpenCode file grant and a fake provider configured to accept its selected API key
    When the first worker runs "opencode --standalone auth import /run/itos-container/credentials/opencode-auth", switches to the staged credential ID and requests the fake provider with OpenCode
    Then the fake provider accepts the original key for the selected principal
    When the first worker exits and the fixture revokes its key, registers a replacement at the provider and saves it only in host SQLite
    And a second granted worker repeats the import, switch and request with the stale host auth.json
    Then the fake provider rejects the stale key and OpenCode reports an authentication failure without falling back to another login
    When the host writes the replacement key into the selected auth.json entry in place while the second worker is live
    Then a separate reader in that worker reopens "/run/itos-container/credentials/opencode-auth" and finds the original key for the provider and no replacement key
    When OpenCode retries the request in the second worker
    Then the fake provider still receives and rejects the old key
    When the second worker exits and a third granted worker repeats the import, switch and request under the same approval
    Then the fake provider accepts the replacement for the same principal
    And all three runs use the same approval fingerprint
    And the credential IDs read from the three workers' staged files are pairwise distinct
    And no run refreshes or writes either host store

  # Inspect the retained home as soon as run returns on normal exit or fresh
  # stop finishes after controller loss. Take a read-only copy before changing
  # approval, launching another worker or opening OpenCode. Capture the database
  # and any -wal, -shm and -journal files before opening SQLite. Search the
  # copied bytes for the key and query the copy for the account and saved session.
  # The only cleanup between observations is product exit or recovered stop.
  # A separate disposable SQLite-only client proves a surviving database entry
  # can still authenticate after its legacy file is gone.
  @wip @slice-40 @ID-OPENCODELOGIN-08 @engine
  Scenario Outline: Reusing the same OpenCode home cannot reuse a finished run's login
    Given a CI checkout with an OpenCode Go file grant, a retained home and an allowlisted fake provider that keeps the key valid
    And the retained OpenCode database contains an unrelated saved session with a known ID and title
    And an independent OpenCode control has the same synthetic login saved only in SQLite
    When the granted worker runs "opencode --standalone auth import /run/itos-container/credentials/opencode-auth"
    And it switches to the staged credential ID, sends a distinct message in the saved session with OpenCode and writes a home marker
    And the SQLite-only control makes its own request to the fake provider
    Then the fake provider authenticates that worker as the selected principal and the SQLite-only control also authenticates
    And OpenCode lists the imported account from the worker's effective SQLite store and the seeded session in that store contains the new message
    When the granted session ends by "<ending>"
    Then immediate read-only inspection finds no imported account in the retained OpenCode database
    And the granted key is absent from the database, its SQLite sidecars and the rest of the retained OpenCode data
    And that database still contains the seeded session with its original ID, title and the message from the granted worker
    And the same inspection finds the saved account and its key in the independent SQLite-only control
    And independent inspection confirms the original worker and its staged login are gone
    When the checkout selects matching approval with only the credential grant removed
    And fresh OpenCode attempts the same provider request without an import using the same home, configuration and provider route
    And the SQLite-only control repeats its request
    Then the second worker reads the first home marker from the same volume and reports missing or rejected authentication
    And the route succeeds for an unauthenticated health check and the provider accepts no authenticated operation from the second worker
    And the independent SQLite-only control still authenticates as the selected principal

    Examples:
      | ending                              |
      | normal exit                         |
      | controller loss followed by stop    |

  # Each row starts from its own session with all other resources accounted for.
  # The engine answers not-found for the full worker ID in the confirmed rows.
  # In the unconfirmed rows only inspection of that ID is unavailable until the
  # final stop retry. The worker that never imported the login has a writable
  # home throughout. Only the full-ID inspection result changes between retries.
  # Inspect read-only copies as in 08. A present login requires the staged
  # credential ID's row to contain the granted key. An absent login requires
  # both a missing row and absence of the key throughout the retained home,
  # including the database and its SQLite sidecars.
  # As in HOME-12, changing the host state root still selects the original volume.
  # Keep each row's grant-free policy and approval fixed across all three runs.
  # /bin/true can exit successfully even with the database read-only.
  @wip @slice-40 @ID-OPENCODELOGIN-09 @local-engine-fixture
  Scenario Outline: Incomplete OpenCode cleanup blocks home reuse until the original session is recovered
    Given a synthetic OpenCode session in the original host XDG_STATE_HOME with an independently recorded owned worker and a retained home
    And the worker acquired the read-only login mount before the host staging pathname was unlinked
    And the worker <import history> the staged account before controller loss
    And a second, empty host XDG_STATE_HOME for the same checkout, account home, UID/GID and daemon
    And the retained worker database contains an unrelated session with a known ID, title and message
    And the retained database, its directory and any SQLite sidecars are <access> to cleanup
    And the checkout now requests a different OpenCode provider
    When the fixture snapshots the retained home and probes the store with cleanup's permissions before the first stop
    Then read-only inspection of the snapshot reports the staged login as <login>
    And the probe finds the database, its directory and any SQLite sidecars <access>
    And the engine recorder shows the original worker acquired the login mount and a separate lookup finds its host staging pathname absent
    When a fresh stop for that session runs in the original host XDG_STATE_HOME with worker absence <confirmation>
    And fresh status inspects the session under the same conditions
    Then status shows the frozen OpenCode provider and reports the original worker as "<resource>"
    And the engine recorder shows worker absence was <confirmation> for the recorded full ID during the first stop
    And the first stop exits nonzero and both commands report incomplete cleanup for the original session
    And each command gives exactly the retry argv "itos", "container", "stop" and the original session ID
    When the checkout selects a version 1 offline policy for the original home
    And the fixture independently records a matching grant-free approval for that checkout and policy in each host XDG_STATE_HOME
    And the run controller selects the <run root> host XDG_STATE_HOME
    And itos-container runs with "run -- /bin/true"
    Then the grant-free run exits nonzero and the engine recorder counts zero worker start requests during that run
    And read-only inspection of the retained home still reports the staged login as <login>
    When a fresh controller executes the reported stop retry in the original host XDG_STATE_HOME with worker absence still <confirmation>
    Then that retry exits nonzero, reports incomplete cleanup and gives the same stop retry argv for the original session
    When the fixture lifts any write restriction it set on the retained database, its directory and its SQLite sidecars
    Then an independent probe with cleanup's permissions opens the retained database file read-write and closes it without writing
    And read-only inspection of the retained home still reports the staged login as <login>
    When the run controller selects the <run root> host XDG_STATE_HOME
    And itos-container runs with "run -- /bin/true"
    Then the grant-free run exits nonzero and the engine recorder counts zero worker start requests during that run
    And read-only inspection of the retained home still reports the staged login as <login>
    When a fresh controller in the original host XDG_STATE_HOME executes the reported stop retry with the engine answering not-found for the recorded full worker ID
    Then immediate read-only inspection finds no imported account in the retained OpenCode database
    And the granted key is absent from the database, its SQLite sidecars and the rest of the retained OpenCode data
    And the same database still contains the unrelated session with its original ID, title and message
    And the retry exits 0 and reports cleanup complete for the original session
    And the engine recorder shows the retry confirmed absence by the original full worker ID
    And independent inspection confirms the worker and staging are absent
    And the original home volume is still present and the host login is byte-for-byte intact
    And no stop starts a worker, rereads a host credential or invents a workload exit
    When the run controller selects the <run root> host XDG_STATE_HOME
    And itos-container runs with "run -- /bin/true"
    Then it exits with code 0
    And the engine recorder shows that run started a new worker with the original volume mounted at "/home/itos" and no credential mounts

    Examples:
      | import history | access    | login   | confirmation | resource | run root |
      | imported       | read-only | present | confirmed    | absent   | original |
      | imported       | read-only | present | confirmed    | absent   | second   |
      | imported       | read-only | present | unconfirmed  | unknown  | original |
      | never imported | writable  | absent  | unconfirmed  | unknown  | second   |

  # Management runs only on independently tracked disposable CI homes. Observe
  # active-worker refusal and a successful control before harness teardown.
  # For external absence, the fixture removes its tracked original volume only
  # after product stop confirmed worker absence, before management's new review.
  # The obsolete case selects the original scope explicitly after an identity
  # change; the other checkout's home is never a reclamation candidate.
  @slice-34 @ID-HOME-23 @engine
  Scenario Outline: A person can retire native home storage after its session is gone
    Given two CI home checkouts with retained markers one live worker and a disposable terminal for action "<action>"
    When home management is attempted while the worker lives then ordinary stop completes and the chosen action receives fresh consent
    Then the live attempt reports recovery.home-in-use with its worker and home intact and the consented attempt durably retires only the selected home
    And native inspection confirms the original volume absent and the other checkout still reads its retained marker before harness teardown
    When a later independently approved run and repeated management of the old ID are attempted
    Then reset permits one fresh empty home while delete refuses allocation until a separate consented reset and old-ID retries never remove successor data

    Examples:
      | action                        |
      | reset present home            |
      | reset externally missing home |
      | delete obsolete home          |
