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

  # Slice-41: OpenCode starts headless in its isolated home and configuration.
  # The OpenCode counterpart of slice-35 (Codex), built on slice-34 (done): the
  # home, its HOME/XDG mappings and boundary inspection above are prerequisites,
  # not repeated. Its rows are ID-HOME-25 to 31, beside their topics' blocks: the
  # policy and approval rows here, the local runtime rows after ID-HOME-08 and the
  # native rows after ID-HOME-14. They sit in this file because they are this
  # area's behaviour and the file is already in the offline engine shard.
  # Authorities: charter milestone two steps 1 and 3 ("harness state seeded where
  # its first run would otherwise stop or call home"; "no scenario uses a real
  # model, account or credential"); container-pilot "Goal and limits" (OpenCode
  # in the worker; model selection is not included), "Agreed scope" (Agents,
  # Mounts) and "Credentials and Git"; decisions 0022 and 0030; architecture.md
  # Slices; egress.feature and offline.feature. Slice-41's spec adds these rows
  # and comments only: no bindings, product code or evidence. Slice-34's rows and
  # decisions above are unchanged.
  #
  # Scope and limits:
  # - No scenario installs or runs OpenCode itself: the charter makes a real
  #   harness run the person's smoke check, and no scenario uses a real model,
  #   account or credential. Native rows run a synthetic stand-in executable,
  #   installed in the CI checkout as /workspace/opencode and named by that
  #   explicit path in the literal argv, that reports what the worker gave it.
  #   These rows prove what the product controls: request, approval, the
  #   worker's environment and argv, isolation, egress and cleanup. They claim no
  #   OpenCode compatibility.
  # - Target release: OpenCode v2.2.1, the version installed on the person's
  #   host, pinned by the person on 2026-11-20 in place of v1.19.0 (which has no
  #   --standalone option). The person's one host smoke check on that release,
  #   run on their OpenCode Go login with a real credential approved for that
  #   check only, is done. Its record, docs/proof/slice-41-opencode-smoke.md
  #   (c8fab73, linked from the item at 1b44058), is the sole authority for what
  #   OpenCode itself does, and the opencode-v1 table below is drawn from it:
  #   `opencode --standalone run --format json` is accepted as written (its
  #   finding 2); v2.2.1 loads, in order, managed policies, integrations, remote
  #   well-known configuration, the global file, OPENCODE_CONFIG, project files,
  #   .opencode directories, inline OPENCODE_CONFIG_CONTENT and then managed
  #   settings, which override everything (finding 3); and of the
  #   OPENCODE_DISABLE_* names an earlier draft used, v2.2.1 reads only
  #   AUTOUPDATE and MODELS_FETCH (finding 4). This spec claims no OpenCode
  #   behaviour beyond that record; its scenarios test only what the product
  #   controls.
  # - Before building the opencode-v1 table, the implementer reads that record.
  #   The table follows it as of 2026-11-21. If the implementer finds the two
  #   disagreeing, they stop and report to the coordinator; they do not adapt
  #   the table or the scenarios. Any later change to the table is a feat commit
  #   with a new profile ID and a new approval. The stand-in rows keep their
  #   shape. Another OpenCode release needs its own smoke check before a new
  #   profile.
  # - v2.2.1 has no separate account configuration layer (finding 3), and this
  #   slice delivers no OpenCode login (slice-40 owns it). Managed settings load
  #   after the inline content and override it; on Linux they live in
  #   /etc/opencode. OPEN, 2026-11-21: whether the person's approved image holds
  #   any managed configuration. The record could not establish it, since no
  #   image was used, and the person has not yet checked it. Until the person
  #   settles it, this spec treats whatever managed configuration the approved
  #   image holds as the person's approved, digest-pinned input: it sits in the
  #   image's read-only root filesystem, which the worker cannot write, and it
  #   may override the profile. No row depends on the answer. The profile
  #   prevents unintended startup calls as a convenience. It is not an
  #   isolation property: the approved egress proxy stays the only route out and
  #   the egress boundary.
  # - The OpenCode login (auth.json under ~/.local/share/opencode) belongs to
  #   slice-40. This slice delivers no credential and no row needs one: the fake
  #   provider accepts unauthenticated synthetic requests.
  # - Run never chooses the provider or model. The contract leaves model
  #   selection out. The literal argv (`--model`) and the checkout's own
  #   opencode.json choose them. A provider is "authorized" only in that its name
  #   is on the approved egress allowlist (and, later, its login is granted by
  #   slice-40). The isolation boundary is the proxy, never OpenCode's config.
  #
  # Request and approval decisions:
  # - Add optional home.harnesses to the home object, in policy versions 1, 2 and
  #   3: a non-empty list of unique harness names. This slice accepts only
  #   "opencode"; slice-35/36 may add their own names. Canonical order is sorted.
  #   Omitting harnesses keeps every slice-34 output and canonical byte unchanged.
  #   Home's storage stays required and exactly volume.
  # - Malformed requests, each exit 2 with file-only doctor and no state or
  #   engine access: harnesses null, "opencode" or {} -> policy.type
  #   /home/harnesses; [] -> policy.home_harness /home/harnesses; [codex] or
  #   [OpenCode] -> policy.home_harness /home/harnesses/0 (slice-35, adding
  #   codex, replaces the [codex] example); [opencode, opencode]
  #   -> policy.home_harness /home/harnesses/1; [7] -> policy.type
  #   /home/harnesses/0; {harnesses: [opencode]} without storage -> the existing
  #   policy.required /home/storage. policy.home_harness is new; its fix names
  #   home.harnesses [opencode]. Duplicate keys, depth and size keep their rules.
  # - File-only doctor returns home: {storage: volume, harnesses: [opencode]}.
  # - The effective approval's home object gains
  #   harnesses:[{"name":"opencode","profile":"opencode-v1"}]. Approval profile
  #   names (offline-home-v1 and the rest) stay as slice-34 named them; the home
  #   object distinguishes harness settings. opencode-v1 binds exactly the
  #   environment table below, so changing that table needs a new profile ID and
  #   a new approval. An older binary's strict codec refuses such a record; that
  #   downgrade lockout belongs to p2-control-state-upgrade-contract.
  # - What each output shows, split by doctor mode so no new JSON field and no
  #   new difference output is needed:
  #   - `doctor --json` (ordinary doctor, as ID-HOME-02 runs it) gains no field.
  #     Its existing approval.effective carries the home object above, with
  #     harnesses [{"name":"opencode","profile":"opencode-v1"}]. Its existing
  #     approval.differences carries {"path":"/home/harnesses","before":null,
  #     "after":[that harness]} in the existing sorted order (an added key under
  #     an existing home object is reported at its own path), and
  #     approval.fingerprint is the new digest. JSON names the profile only. It
  #     does not list the environment, which the profile ID binds.
  #   - Plain `doctor` and bare approve's review print the existing shared home
  #     disclosure (presentation homeDisclosure). When harnesses are requested,
  #     it adds these lines after its existing three, in this order: "OpenCode
  #     harness: profile opencode-v1.", then one line per environment entry in
  #     the sorted order below, written NAME=value with the value JSON-escaped
  #     for display as the settings line already is, then the disclosure
  #     sentences: OpenCode's global configuration, data and state live in
  #     /home/itos; the checkout's own opencode.json and .opencode configuration
  #     also apply (loaded before the profile, which overrides them where they
  #     conflict); any managed configuration in the approved image
  #     (/etc/opencode) loads after the profile and overrides it; the image's
  #     other OPENCODE_* defaults stay in effect and can redirect or turn off
  #     configuration layers (OPENCODE_CONFIG, OPENCODE_CONFIG_DIR, which
  #     replaces the global directory, OPENCODE_DISABLE_PROJECT_CONFIG); the
  #     argv and the checkout choose the provider and model; a provider is
  #     reachable only through the approved allowlist (for an offline policy:
  #     no provider is reachable); and this setting delivers no OpenCode login.
  #     The wording of each sentence is the implementer's; the step checks for
  #     the stated facts by their fixed words (/home/itos, opencode.json,
  #     /etc/opencode, OPENCODE_CONFIG_DIR, provider, model, login).
  #   - Plain doctor gains no difference lines. Only the review prints the
  #     existing "path: before -> after" lines, so it adds
  #     "/home/harnesses: null -> [...]" in the existing sorted order.
  #   docs/CLI.md's "Implemented approval" gains one paragraph saying this.
  # - The harness profile is not a network or credential grant. q-52's rules are
  #   unchanged: an offline harness request without credentials may still be
  #   approved by its matching --fingerprint; allowlist or credential settings
  #   still need the person at a terminal. The fingerprint route shows no
  #   review (approve's domain skips ui.Review there), so ID-HOME-26 proves the
  #   disclosure through plain doctor and bare terminal approve: the review
  #   prints the profile, its environment and the /home/harnesses difference
  #   before the single y/N prompt. Inspecting or approving allocates no volume and creates
  #   no worker.
  # - Adding or removing the harness changes the fingerprint and needs a fresh
  #   approval. Run reports the local mismatch as egress.feature specifies,
  #   naming /home/harnesses, exit 1, before any engine connection. Selection
  #   freezes the profile with the other settings, as ID-HOME-04 does for home.
  #
  # Runtime decisions (profile opencode-v1):
  # - The controller sets exactly these worker environment values, sorted, beside
  #   the home mappings above, overriding any image default:
  #     OPENCODE_CONFIG_CONTENT={"share":"disabled"}
  #     OPENCODE_DISABLE_AUTOUPDATE=true
  #     OPENCODE_DISABLE_MODELS_FETCH=true
  #   The inline config value is those exact bytes, with no spaces or newline.
  #   What the smoke record supports for each value on v2.2.1:
  #   - OPENCODE_DISABLE_MODELS_FETCH turns off the models catalogue fetch. It
  #     is the one value with an off/on control seen on the wire: a headless
  #     run connected to models.opencode.ai:443 without it and made no such
  #     request with it (findings 4 and 5).
  #   - OPENCODE_DISABLE_AUTOUPDATE is read in the source and skips the update
  #     check before anything else (finding 4). No off/on control showed `run`
  #     checking for updates at all, so it is kept as a safeguard, not as an
  #     observed effect.
  #   - OPENCODE_CONFIG_CONTENT is read as the inline layer (findings 3 and 4).
  #     share "disabled" is a value v2.2.1's loader keeps (finding 3). No run
  #     showed a share request either way, so it too is a safeguard. The legacy
  #     autoupdate key is left out: v2.2.1 rewrites it to update, and the
  #     updater never reads the merged configuration (findings 3 and 4), so it
  #     would switch nothing off.
  #   - OPENCODE_DISABLE_DEFAULT_PLUGINS and OPENCODE_DISABLE_LSP_DOWNLOAD are
  #     left out: v2.2.1 reads neither (finding 4).
  #   - No value stops the bundled LM Studio, Ollama and vLLM providers'
  #     plain-HTTP probes of 127.0.0.1 ports 1234, 11434 and 8000 (finding 5).
  #     In the worker NO_PROXY names 127.0.0.1 (egress.feature), so a client
  #     that honours it probes the worker's own loopback; one that does not
  #     sends plain HTTP proxy requests, which the proxy refuses as it refuses
  #     ID-EGRESS-13's GET. Either way nothing leaves the worker. The profile
  #     claims nothing about these probes and no row tests them.
  #   In ID-HOME-31 the proxy refuses the startup-service name
  #   models.opencode.ai, which is a proxy property and never evidence of any
  #   setting's effect. The values are non-secret and appear in docker inspect.
  # - Seed by environment only: run writes no OpenCode file into the home, makes
  #   no archive copy beyond the root initialization above, and adds no mount.
  #   Every run reapplies the profile, so a repeat run needs no reseeding and
  #   never rewrites home contents. What the worker leaves under
  #   /home/itos/.config/opencode or .local/share/opencode is hostile worker
  #   data it starts with next time, as decision 0030 discloses.
  # - The argv stays literal: no wrapper, injected flag, model, prompt or
  #   format. Run does not check that argv[0] is opencode; the profile applies to
  #   whatever the workload runs. `opencode --standalone run --format json` is
  #   the person's argv, and its JSON event stream reaches run's stdout unchanged
  #   under offline.feature's stream rules: run adds no stdout bytes and writes
  #   its own diagnostics to stderr. The bare name resolves through the
  #   approved image's own PATH, which run does not change; manifest PATH stays
  #   reserved. The native rows therefore name the stand-in by its explicit
  #   checkout path, /workspace/opencode, as slice-34's probe does.
  # - The host's OPENCODE_*, XDG_* and HOME values never reach the worker. Run
  #   never reads or copies the host's ~/.config/opencode, ~/.local/share/opencode
  #   or ~/.local/state/opencode. Manifest containerEnv/remoteEnv already reserve
  #   the OPENCODE_ and XDG_ prefixes, which stays unchanged. Other OPENCODE_*
  #   defaults in the approved image are left alone: the image is the person's
  #   approved, digest-pinned input, and the review discloses that they stay
  #   in effect.
  # - Boundary inspection before start, beside the home inspection above, also
  #   verifies each opencode-v1 name has exactly its value in the created
  #   worker's environment. The check is the existing exact environment
  #   comparison in VerifyRun (internal/engine/session/docker/run.go,
  #   sameEnvironment), with the expected environment gaining the opencode-v1
  #   entries. Docker merges image defaults into the inspected Config.Env, so an
  #   inherited value is observable only when its bytes differ from the
  #   approved ones: each defect is an observed value that is missing or differs
  #   from the approved bytes, including the image's own conflicting value left
  #   in effect. Any such value is run.isolation-unverified, exit 1, with no
  #   start; the owned unstarted worker is removed and the home retained.
  #   Sessions without the harness keep their existing inspection.
  # - Startup stays bounded by the existing 10 s call and 120 s startup budgets.
  #   The profile adds no engine call. No workload, OpenCode-startup or output
  #   timeout is added: the contract sets no workload lifetime limit. Exit,
  #   failure, cancellation, cleanup and recovery stay as offline, lifecycle,
  #   egress and the home rows above specify them. The profile owns no resource,
  #   so status, stop and recovery records gain nothing for it.
  #
  # Architecture and libraries: policy and approval keep validation and
  # canonicalization (the home object and the opencode-v1 table); the shared
  # session engine applies the frozen environment in its docker adapter and
  # verifies it with the home boundary (home_boundary.go today). Doctor and
  # approve presentation go through the existing approval presentation package.
  # No slice imports another, only cmd assembles, porttest fakes do no I/O and
  # share contracts with adapters. No new package, dependency, library or
  # network service: reuse pinned moby client v0.6.2/api v1.56.1 and Go 1.27.4.
  # No arc42 departure is expected; if the code shows one, record a decision in
  # the same pull request. The implementation updates docs/CONFIG.md
  # (home.harnesses, opencode-v1) and docs/CLI.md, and adds home.harnesses to
  # the three published policy schemas, docs/schemas/itos-container-v1, -v2 and
  # -v3.schema.json. Each closes its home object with additionalProperties
  # false and would otherwise reject a request that file-only doctor accepts.
  #
  # Step handoff: every phrase in ID-HOME-25 to 31 is NEW except two existing
  # home steps: ID-HOME-05's "fresh run controllers execute both runs through
  # both local backends" and ID-HOME-08's "the owned unstarted worker is removed
  # and the retained home and unrelated resources remain intact". Their
  # definitions must also accept the OpenCode launch fixtures, without changing
  # what they assert for ID-HOME-05 and 08. Reuse the helpers
  # behind harness_home_policy_steps_test.go, harness_home_approval_steps_test.go,
  # harness_home_local_steps_test.go ("both local backends", as above) and the
  # harness_home_native_* and egress native fixtures. Add definitions alone in a
  # red test commit with @wip retained. Steps run the CLI or the test-only cmd
  # assembly and inspect public output, protected artifacts and independent
  # engine observations; never product imports or source as an oracle.
  # Givens create temporary Git checkouts, raw policies and manifests, synthetic
  # image metadata (with conflicting image OPENCODE_*/XDG defaults where named),
  # independently encoded approval, snapshot and ownership records, host-home
  # sentinel files and host environment sentinels. None calls product approve,
  # run, status or stop, and none asserts. A refused or failed product run is
  # captured in When and judged in Then. Positive controls use the same harness
  # request, never a no-harness success that could hide a harness refusal.
  # Native Givens provision only trusted controls, the stand-in, private state
  # and synthetic approval; no real model, account, credential, public endpoint
  # or resolver rewrite. Fake provider TLS uses in-memory ephemeral keys.
  # ID-HOME-31's startup-service name is refused by the proxy before any dial;
  # its independent control is a fixture alias, never a real host. This file
  # is already in the offline shard and features/smoke.yaml; ID-HOME-31 needs the
  # egress fixtures inside that shard, within its time limit. Non-engine red runs
  # are local; every @engine red and green run is CI-only, by dispatch.
  #
  # ID-HOME-28 and 29 need their own cases in TestHomeLocalContract, whose
  # dispatch fails any unbound ID. Their fixtures carry the harness in frozen
  # settings and snapshot bytes the step encodes itself. They never go through
  # canonical.JSON{}.Encode, which today silently drops the harnesses member.
  # ID-HOME-29's new When sets the same captured result that the reused
  # ID-HOME-08 Then reads.
  #
  # Expected red, from reading 16e5bdb, whose product code is main's at b880d99
  # (predictions, not executed evidence), always at the first Then, never in a
  # Given or When:
  # - ID-HOME-25, 26, 27, 30 and 31 parse the raw policy, which refuses
  #   /home/harnesses as policy.unknown_key with exit 2. The exception is
  #   ID-HOME-27's removing mismatch, whose policy has no harnesses: its
  #   approval record does carry them, and the strict record decode refuses
  #   it. Neither refusal names /home/harnesses as changed approval.
  # - ID-HOME-28 and 29 never parse policy: the local contract supplies the
  #   approval view in test code. On the disk backend the session codec's
  #   FrozenRecord re-encodes the frozen settings, drops the unknown harnesses
  #   member and refuses them as state.invalid before create. If a launch got
  #   past that, its created environment would lack the opencode-v1 values.
  #   Either way ID-HOME-28's environment Then fails, and ID-HOME-29's control
  #   half fails on every row.
  #
  # The implementer may commit freely in its own workspace; the host makes the
  # landing commits and handles itos, submission, publication and review.
  @slice-41 @ID-HOME-25
  Scenario Outline: Policy accepts only an explicit OpenCode harness on a volume home
    Given an OpenCode harness policy fixture using version <version> and no credential grants
    When file-only doctor inspects a volume home without harnesses, a volume home with the OpenCode harness and the malformed harness requests
    Then the OpenCode request succeeds with exactly the normalized home storage and harness and the request without harnesses keeps its slice-34 output
    And malformed harness requests have the specified rules and subjects without state or engine access

    Examples:
      | version |
      | 1       |
      | 2       |
      | 3       |

  # Approval status compares canonical settings, so a changed review and the
  # next generation alone do not show a changed digest. The fixture calculates
  # both fingerprints itself, as approval.feature's "independently calculated
  # fingerprint" does: the old one from the seeded no-harness settings and the
  # new one from the expected settings with the OpenCode harness. It requires
  # the two to differ, and requires doctor's reported fingerprint and the
  # persisted approval's to equal the new one.
  # Doctor runs twice, before approve: `doctor --json` is judged for the
  # effective home object, the /home/harnesses difference and the fingerprint;
  # plain `doctor` for the harness lines of the shared home disclosure. The
  # review transcript is judged for both. The decisions above ID-HOME-25 state
  # each output's lines.
  @slice-41 @ID-HOME-26 @local-engine-fixture
  Scenario: Review binds the OpenCode profile without creating storage
    Given an OpenCode approval fixture with an independently seeded approval of a volume home without harnesses and a requested OpenCode harness
    When JSON doctor and plain doctor inspect the requested harness and bare approve runs at a terminal and receives "y" after the review
    Then JSON doctor reports the independently expected home object and the /home/harnesses difference from null to the OpenCode harness
    And plain doctor and the transcript before exactly one y/N prompt both show the OpenCode harness profile and its exact environment lines, and the transcript shows the /home/harnesses difference with old and new values
    And plain doctor and the review before its prompt state where OpenCode's global configuration and state live, that the checkout's own OpenCode configuration applies, that managed configuration in the approved image overrides the profile, that the image's other OPENCODE defaults can redirect or turn off configuration layers, who chooses provider and model, that no provider is reachable offline and that no login is delivered
    And approval exits 0 and records the independently expected home object at the next generation
    And doctor and the persisted approval both carry the independently calculated new fingerprint, which differs from the seeded approval's fingerprint
    And no home volume or worker is created and checkout and host-home OpenCode sentinels are unchanged

  # Each mismatch must name /home/harnesses; each matching control must reach
  # its runtime boundary, so a generic policy refusal passes neither half.
  # Unlike ID-HOME-03's fixture, which sets the approval status in test code,
  # no status, view or difference is supplied by the test. As in ID-EGRESS-01,
  # each Given writes a temporary Git checkout with a raw itos-container.yaml
  # (adding or removing home.harnesses) and approval records whose JSON bytes
  # the step encodes itself, never through the product codec. The When runs the
  # itos-container binary's own `run`, so production approval preflight
  # (approval domain Preflight with the disk readiness, disk store and
  # canonical codec that cmd assembles) parses the policy, decodes the records
  # and computes the difference.
  # The engine endpoint is the plain doctor Engine API fixture that "an
  # approval checkout and isolated host state" starts (doctorLocalAPI,
  # features/doctor_engine_test.go) at each checkout's DOCKER_HOST. It counts
  # connections at its listener and journals every request, outside preflight.
  # It is never the offline or egress fixture, so the When runs the real binary
  # and not the session probe. Each mismatch leaves the listener with no
  # connection and the journal empty, as ID-EGRESS-01 judges. A matching
  # control's runtime boundary is run's read-only readiness inspection, which
  # run makes only after its local approval preflight has passed. The boundary
  # is reached when that control's journal holds GET /version, GET /info and
  # GET /images/<approved image>/json, and run reports no run.approval-changed.
  # The fixture answers every later request 403. Those requests and the run's
  # later outcome and exit are not judged here, and no worker environment is
  # judged: ID-HOME-28 proves the OpenCode environment.
  # Before the work the removing control (neither side has harnesses) is
  # slice-34's behaviour and already reaches the boundary, so it is a
  # regression control. The adding control fails at policy.unknown_key, exit 2,
  # with no connection.
  @slice-41 @ID-HOME-27 @local-engine-fixture
  Scenario: Adding and removing the OpenCode harness both require approval
    Given OpenCode harness approval mismatch checkouts with raw policies and independently encoded approval records for adding and removing the harness and matching controls
    When the itos-container binary runs each changed request and then each matching control through its production approval preflight
    Then each mismatch reports changed approval at /home/harnesses with exit 1 and the Engine API fixture saw no connection and journalled no request
    And each matching control reports no approval change and its Engine API fixture journal holds the readiness inspection of version, info and the approved image

  # Slice-40: deliver an approved OpenCode login to one container run. Its rows
  # are ID-HOME-32 to 38, beside their topics' blocks: policy, approval and the
  # run's approval comparison here, the local run rows after ID-HOME-29, the
  # recovery row after ID-HOME-10 and the native row after ID-HOME-31. They sit
  # in this file because they extend slice-41's OpenCode area and the file is
  # already in the offline engine shard; a new feature file with an @engine row
  # would need a shard in .github/workflows, which this item does not touch.
  # Authorities: charter milestone two step 2; decisions 0025 (staged binds,
  # /dev/shm, fixed targets), 0029 (approval binds kinds and paths, never
  # contents), 0035 (real credentials, one form per provider) and 0039 (this
  # redraft decides; the slice-40-old scenarios were not on this tree to read);
  # T-48's ledger (staging, exact mounts, owned recovery); slice-29's source
  # inspection and slice-30's approval contract in credentials-policy.feature
  # and credentials-approval.feature; slice-41's rows and record above;
  # architecture.md Slices. Slice-34's, slice-41's and slice-54's rows and
  # decisions stay unchanged except where a line below names one.
  #
  # What one grant authorizes (decided by the item, 2026-11-24; decision 0035):
  # - OpenCode issues no subscription token, so the one grant is the OpenCode Go
  #   provider entry of the person's own OpenCode auth.json, normally
  #   ~/.local/share/opencode/auth.json. That file may also hold other provider
  #   logins (the smoke record's finding 5 saw a GitHub Copilot login beside
  #   OpenCode Go). Only the opencode-go member is delivered. Every other member,
  #   and every member of the opencode-go entry other than type and key, stays
  #   on the host and appears in no worker file, request, record or output.
  # - Reconciling the standing lines (docs/decisions/STANDING.md:16 and :23):
  #   the source is the host's own login file, read in place on every run, so a
  #   host-side re-login reaches the next run with no copy to keep in step. The
  #   delivered file holds one secret. The entry is an API key, which OpenCode
  #   does not refresh, so nothing flows back to the host file and the worker
  #   gets no write path to it. A changed standing line would reopen this.
  # - Because run checks the content, the grant can deliver only an OpenCode Go
  #   API-key entry. A source path the checkout names that holds anything else
  #   (an SSH key, a Claude login) is refused at run, never delivered as is.
  #
  # Request (policy version 3 only; versions 1 and 2 still refuse credentials):
  # - credentials accepts a third kind, exactly the string opencode-go, at most
  #   once, with slice-29's closed {kind, source} entry and source grammar. This
  #   is additive within version 3, as slice-34 and slice-41 added home members
  #   to versions 1 to 3: no accepted file changes meaning, and it avoids the
  #   version 4 that slice-48 plans for Codex. Kinds sort as claude-token,
  #   github-token, opencode-go. OpenCode-Go, opencode, opencode-auth and
  #   OPENCODE-GO stay policy.credential_kind at /credentials/<i>/kind; a second
  #   opencode-go is policy.credential_duplicate at the later entry's kind.
  #   ID-CREDENTIALS-05's refused examples stay refused; its title, "only the two
  #   decided token kinds are accepted", goes stale, and the feat commit may
  #   retitle it. The grant needs no home or harness: it is a file at a fixed
  #   target, whichever home the request selects.
  # - File-only doctor JSON lists the grant in policy.credentials as
  #   {"kind":"opencode-go","source":<as written>} in the sorted order above;
  #   plain file-only doctor prints it in the existing credential line format.
  #   Parsing never stats or opens the source.
  # - Doctor and approve inspect the source by metadata only, with slice-29's
  #   observer and rules unchanged (credential.* at /credentials/<i>/source).
  #   They never read the content, so neither can tell whether the file holds an
  #   OpenCode Go entry; run reads it (below).
  #
  # Approval (slice-30's contract, one more kind):
  # - The canonical grant is {"kind":"opencode-go","source":<path>,
  #   "target":"/run/itos-container/credentials/opencode-go"}. Its profiles are
  #   slice-30's credential profiles (offline-credentials-v1,
  #   allowlist-credentials-v1 and their -home- forms); a new kind needs no new
  #   profile. The strict record codec accepts this kind and target. A stored
  #   opencode-go grant with any other target under /run/itos-container/
  #   credentials/ is readable history that differs from the current target, as
  #   slice-30 rules for every kind. Rotating the file at the same path keeps the
  #   fingerprint; adding, removing or repointing the grant changes /credentials.
  # - q-52 holds: the grant needs bare approve at a real terminal; every
  #   --fingerprint refuses with approve.terminal-required (exit 1).
  # - Plain doctor and bare approve's review print slice-30's credential line and
  #   its three disclosure lines unchanged, then, when an opencode-go grant is
  #   current, these three whole lines, case-sensitive, in this order:
  #     opencode-go: each run reads the source as OpenCode's auth.json and stages only its opencode-go entry, an API key; every other provider entry in that file stays on the host.
  #     opencode-go: the worker reads it read-only at /run/itos-container/credentials/opencode-go; OpenCode reads its login from $XDG_DATA_HOME/opencode/auth.json, so the workload links that path to it. Run writes no login into the home.
  #     opencode-go: a key changed in the source reaches the next run without new approval; a running worker keeps its copy and cannot write the source.
  #   When every current grant is opencode-go, slice-30's run warning
  #   (approval port CredentialGate, "Credential delivery is not available in
  #   this increment; ...") is replaced by exactly
  #     Run delivers the opencode-go grant.
  #   Any other grant set keeps the warning the tree has then. Plain status
  #   shows the same lines for an opencode-go session, as slice-30's shared
  #   disclosure already does; no row judges plain status.
  # - The decisions above ID-HOME-25 said "Run never reads or copies the host's
  #   ... ~/.local/share/opencode" and that slice-41's setting "delivers no
  #   OpenCode login". Both stay true of the harness setting. The one exception
  #   is the single source file an approved opencode-go grant names, which run
  #   reads and filters as below.
  #
  # Run (lifts the version 3 gate for this kind only):
  # - Run accepts a valid version 3 request whose credentials list is nonempty
  #   and every grant is opencode-go. Every other version 3 request keeps the
  #   gate the tree has then (today policy.version at /version, exit 2, before
  #   manifest, source, engine or state access, even with no grants); slice-31
  #   lifts claude-token separately. Lifting it requests no new consent for an
  #   unchanged canonical grant.
  # - Run then carries slice-30's ordinary doctor checks: the 4095-byte bounds,
  #   source metadata inspection before engine access and again after engine
  #   inspection, and the checkout identity comparison. A source failure reports
  #   doctor's located credential.* rule and exit, and nothing is created. Its
  #   approval comparison is local and precedes any engine connection, as
  #   ID-HOME-27's is; changed grants report run.approval-changed, exit 1,
  #   naming the sorted JSON pointers that differ.
  # - Staging is T-48's, unchanged in order: durable staging intent, a new
  #   /dev/shm/itos-container-<32 lower hex> directory (0700, host UID), one file
  #   named opencode-go (O_CREAT|O_EXCL|O_NOFOLLOW, mode 0400, host UID), one
  #   read-only HostConfig.Mounts bind of that file at the fixed target, exact
  #   boundary verification before start, and unlink of the file and directory
  #   once start returns and the checkout-mount detector has run. The worker
  #   keeps its acquired copy until it is removed. Nothing is ever written under
  #   the checkout, protected state, the home volume or the image.
  # - The copying adapter reads the checked source once, 1 to 65536 bytes, as
  #   T-48 does, and writes not those bytes but exactly
  #     {"opencode-go":{"type":"api","key":K}}
  #   with no whitespace and no final newline, where K is the key member's JSON
  #   string token exactly as written in the source, escapes included (never
  #   decoded and re-encoded). The source must be UTF-8 JSON text (RFC 8259)
  #   holding one value, an object, with no repeated member name at its top
  #   level or inside the opencode-go entry. Its opencode-go member must be an
  #   object whose type is exactly the string "api" and whose key is a string
  #   that decodes to at least one character. Other top-level members are
  #   parsed for syntax only, and the entry's other members are dropped. A
  #   source failing any of this is refused as run.credential-content, exit 1,
  #   located at itos-container.yaml/credentials/<written index>/source, whose
  #   fix names `opencode auth login` and OpenCode Go. Its message and fix name
  #   no source byte, member name other than opencode-go, offset or length. The
  #   staging intent was already durable, so the refusal cancels that
  #   reservation: partial staging is removed, no worker create is sent, owned
  #   session resources other than a retained home are removed, and no
  #   unfinished session is left for stop. The extraction is a pure function
  #   beside the copying adapter (internal/engine/session/disk), using the
  #   standard library's encoding/json token decoder; no new dependency. It is
  #   not a domain or port, so porttest fakes keep their own bytes. The local
  #   rows judge the content rule through the disk backend only.
  # - The session engine port, codec, porttest fake and docker observation gain
  #   the third kind, its fixed target and its staged file name. The strict
  #   session record evolves explicitly, as T-48 did (a new schema number if
  #   the shape changes, coordinated with the number main holds when this
  #   lands). Existing records keep their bytes and meaning and are still read.
  #   Recovery reconstructs staging authority from protected records alone, as
  #   T-48 specifies, now also for an opencode-go staging.
  # - Rotation: each run reads the source afresh, so a safe replacement at the
  #   same path, a new inode with valid metadata and a new key, is staged by the
  #   next run under the same approval and generation. A running session's copy
  #   is not updated. The target is read-only, so the worker cannot write it.
  # - T-48's credential-refused (short read, changed size) and
  #   credential-unavailable outcomes keep the classification the tree gives
  #   them; this slice adds only credential-content. No row tests them here.
  # - Status JSON carries the grant from the session's protected snapshot, as
  #   slice-30 specifies, and gains no staging field. Allowlist runs keep
  #   slice-30's refusal-log disclosure; no row here tests the refusal log
  #   (AC-36's marker probe belongs to p1-platform-log-redaction).
  #
  # Open and proposed points, each for the person:
  # - Proposed, 2026-11-26, by this drafter: delivery at the fixed /run target,
  #   with the workload linking $XDG_DATA_HOME/opencode/auth.json to it, and not
  #   a bind at /home/itos/.local/share/opencode/auth.json. A bind inside the
  #   retained home would make the runtime create root-owned mountpoint
  #   directories in the worker's own data path on a fresh home, and on a
  #   reused home would resolve worker-controlled paths: the unsafe
  #   retained-path work slice-56 owns for Codex. The fixed target reuses T-48
  #   and slice-30's target grammar unchanged.
  # - Open, 2026-11-26: the person's smoke check of a real OpenCode v2.2.1 run
  #   whose auth.json is that link to a read-only file holding only the
  #   opencode-go entry. The smoke record shows only that v2.2.1 reads its login
  #   from $XDG_DATA_HOME/opencode/auth.json in place (finding 2). Whether
  #   v2.2.1 writes that file at startup, and so fails on a read-only one, is
  #   not established. No scenario depends on it, and this spec claims no
  #   OpenCode behaviour beyond the record.
  # - Open, 2026-11-26, deferred: other provider logins in the same file (OAuth
  #   entries such as GitHub Copilot or an Anthropic login) are never delivered.
  #   Delivering one would be its own grant and item. Key expiry and account
  #   revocation (AC-30) are not detected: a revoked key fails in the workload's
  #   own provider request.
  #
  # Architecture: policy validates the kind; approval canonical and
  # presentation bind and disclose it; readiness gains run's opt-in for
  # deliverable kinds beside AllowVersion3 (doctor and approve keep theirs);
  # the run slice wires T-48's CredentialSources and CredentialStager through
  # its Runner; session domain, port, codec, disk and docker carry the third
  # kind; cmd assembles. No slice imports another, porttest fakes do no I/O
  # and share contracts with adapters. Reuse the pinned moby client v0.6.2/api
  # v1.56.1, golang.org/x/sys v0.48.0, moby/sys/mountinfo v0.7.2 and Go 1.27.4;
  # no new library, service or pin. The implementation updates docs/CONFIG.md
  # (the kind, its target, run's delivery and the home-read exception),
  # docs/CLI.md's approval text, docs/security.md's credential limits and the
  # kind enum of docs/schemas/itos-container-v3.schema.json. No arc42 departure
  # is expected; if the code shows one, record a decision in the same pull
  # request.
  #
  # Step handoff: every phrase in ID-HOME-32 to 38 is NEW. Reuse the helpers
  # behind ID-HOME-25 to 27 and 30 (harness_home_opencode_steps_test.go and
  # harness_home_opencode_native_test.go), slice-30's credential approval
  # fixture (credential_approval_steps_test.go: its private synthetic sources,
  # oracle and terminal transcript), the doctor Engine API fixture's journal and
  # connection count (doctor_engine_test.go), the home SDK recording endpoint
  # (cmd/itos-container/home_sdk_fixture_test.go) and slice-34's native home
  # fixtures. Local rows routed through TestHomeLocalContract need their own
  # cases there, whose dispatch fails any unbound ID. Add the definitions alone
  # in a red test commit with @wip kept.
  # Givens write temporary Git checkouts, raw policies and manifests, private
  # synthetic sources (mode 0600 files in 0700 directories on an accepted
  # filesystem, outside checkout and state), independently encoded approval,
  # snapshot and session records (never through the product codec) and
  # synthetic markers: each source member's secret value is a fresh random
  # marker, distinct per member. None calls product doctor, approve, run, status
  # or stop, and none asserts. Every product command is captured in When and
  # judged in Then. Every absence search has a positive control, the same
  # search finding the marker where it must be (the source, the staged bytes
  # observed at create or the worker's read), so an empty search cannot pass.
  # Assertions report the surface searched, never a marker. No real OpenCode,
  # model, account, credential or public endpoint. Non-engine red runs are
  # local; ID-HOME-38's red and green runs are CI-only, by dispatch. On a host
  # whose / or /tmp ancestors fail the ownership check
  # (p2-repair-platform-ci-fixtures), local runs stop before the behaviour
  # assertion: record that as an environment blocker, never as red evidence.
  #
  # Expected red, from reading main at 16c0a82 (predictions, not executed
  # evidence), always at the first Then, never in a Given or When: every
  # policy naming opencode-go is refused by policy.Inspect as
  # policy.credential_kind at /credentials/<i>/kind, exit 2, before any state,
  # source or engine access.
  # - ID-HOME-32: file-only doctor refuses the accepted request, so its first
  #   Then fails.
  # - ID-HOME-33: ordinary doctor --json refuses the request, with no
  #   approval.effective, so the first Then fails.
  # - ID-HOME-34: each changed request and its matching control are refused
  #   with policy.credential_kind, not run.approval-changed, so the first Then
  #   fails.
  # - ID-HOME-35 and 36: run refuses with policy.credential_kind before any
  #   engine request, so no create carries a staged file and the first Then
  #   fails, on the control as well as the defects.
  # - ID-HOME-37: the session codec refuses the seeded record's opencode-go
  #   staging kind, so status reports invalid protected state instead of the
  #   session and its grant, and the first Then fails.
  # - ID-HOME-38: run refuses before its worker exists, so the stand-in reports
  #   nothing and the first Then fails.
  #
  # The implementer may commit freely in its own workspace; the host makes the
  # landing commits and handles itos, submission, publication and review.
  @wip @slice-40 @ID-HOME-32
  Scenario: Policy version 3 accepts one OpenCode Go grant and versions 1 and 2 still refuse it
    Given an OpenCode Go grant policy fixture with a version 3 request, the same entry in version 1 and 2 files and the malformed OpenCode Go kinds
    When file-only doctor inspects each request in JSON and plain file-only doctor inspects the version 3 request
    Then the version 3 request succeeds with exactly its normalized credentials, holding the opencode-go grant as written after any other kinds, without the source being opened
    And the version 1 and 2 files keep their existing refusal of credentials and each malformed kind has exactly its specified rule and subject without state or engine access

  # ID-HOME-32's inputs, each with manifest a.json and a source under the
  # fixture's private directory. Accepted: version 3 with
  # [{kind: opencode-go, source: S}] and with [{kind: opencode-go, source: S},
  # {kind: github-token, source: T}] (judged sorted: github-token first).
  # Versions 1 and 2 with [{kind: opencode-go, source: S}] keep their existing
  # credentials refusal, policy.unknown_key at /credentials, exit 2, as
  # credentials-policy.feature specifies. Malformed version 3 kinds OpenCode-Go, opencode,
  # opencode-auth and OPENCODE-GO -> policy.credential_kind /credentials/0/kind;
  # two opencode-go entries -> policy.credential_duplicate /credentials/1/kind.
  # Each malformed file gives exactly that one problem, exit 2. The source path
  # S does not exist, so a request that succeeds shows parsing opened nothing.

  # The request is version 3 offline with no home and one opencode-go grant.
  # The fixture seeds an approval of the same checkout's version 3 request
  # without credentials (offline-v1, declared version 3), so the differences
  # are /credentials and /profile (to offline-credentials-v1), and
  # computes both fingerprints itself, requiring them to differ. The source is
  # a valid private auth.json whose members hold markers. Doctor and approve
  # read metadata only, so the row judges that no marker appears in any output
  # or record and that the source keeps its bytes, mode and inode.
  @wip @slice-40 @ID-HOME-33 @local-engine-fixture
  Scenario: Review binds the OpenCode Go grant and discloses what it delivers, at a terminal only
    Given an OpenCode Go approval fixture whose checkout requests an opencode-go grant over an independently seeded approval without credentials, with a private synthetic auth.json source
    When JSON doctor and plain doctor inspect the request, approve runs at a terminal with the seeded approval's fingerprint, and then bare approve runs at a terminal and receives "y" after the review
    Then JSON doctor reports the independently expected credentials with the opencode-go grant at its fixed target, the /credentials and /profile differences and the independently calculated new fingerprint
    And the fingerprint approval is refused with approve.terminal-required and exit 1 without a prompt and the seeded approval record is unchanged
    And plain doctor and the review before exactly one y/N prompt both show the opencode-go credential line, the credential disclosure, the three opencode-go lines in order and "Run delivers the opencode-go grant." in place of the run warning
    And terminal approval exits 0 and records the independently expected settings and new fingerprint at the next generation
    And no source byte or marker appears in any output or record, the source is unchanged, and no home volume, staging or worker is created

  # As ID-HOME-27: each checkout has a raw policy, independently encoded
  # approval records and its own doctor Engine API fixture at DOCKER_HOST, and
  # the When runs the itos-container binary's own run. Every source is valid.
  # Changed: adding (the record is the same version 3 request without
  # credentials; pointers /credentials /profile) and repointing (the record's
  # grant names another valid source; pointer /credentials). The matching
  # control's record carries the same grant and must reach the readiness
  # inspection (GET /version, GET /info and GET /images/<approved image>/json
  # journalled, no run.approval-changed); the fixture answers later requests
  # 403 and its later outcome is not judged. A run that kept the version 3
  # gate fails the control, and one that skipped the comparison fails the
  # changed rows.
  @wip @slice-40 @ID-HOME-34 @local-engine-fixture
  Scenario: Adding or repointing the OpenCode Go grant needs a new approval before run contacts the engine
    Given OpenCode Go grant run checkouts with raw policies and independently encoded approval records for adding and repointing the grant and a matching control
    When the itos-container binary runs each changed request and then the matching control through its production approval preflight
    Then each changed request reports changed approval naming exactly its sorted pointers with exit 1 and its Engine API fixture saw no connection and journalled no request
    And the matching control reports no approval change and its Engine API fixture journal holds the readiness inspection of version, info and the approved image

  # Slice-54: the person selects the Codex defaults explicitly and sees them
  # before approving. It is slice-35's first stage, built on slice-34 (done),
  # and its rows are ID-CODEXHOME-01 and 02. slice-55 seeds config.toml on each
  # run, slice-56 and 57 refuse unsafe or unverifiable seeding, slice-58 starts
  # pinned Codex, and slice-48 owns version 4's Codex credential entry. None of
  # that is specified here. This spec adds these rows and comments only: no
  # step definitions, product code or evidence. Slice-34's and slice-41's rows
  # and decisions above stay unchanged.
  # Authorities: charter milestone two step 3 ("harness state seeded where its
  # first run would otherwise stop or call home"); decision 0030; architecture.md
  # Slices; q-52 (decisions 0025:360 and 0029:25); the slice-54 registry entry.
  #
  # Request (policy versions 1, 2 and 3):
  # - home gains an optional member harness, whose only accepted value is the
  #   string codex. It is a separate member from slice-41's home.harnesses list.
  #   The list keeps its rules, and harnesses: [codex] is still policy.home_harness
  #   at /home/harnesses/0 with its existing fix. Slice-41's note that slice-35
  #   would replace that example no longer holds. A home may carry both members:
  #   {storage: volume, harness: codex, harnesses: [opencode]} is accepted, and
  #   each member is disclosed and bound on its own.
  # - harness is accepted only beside storage volume. The storage rules are
  #   slice-34's: a home with harness but no storage is policy.required
  #   /home/storage, and {storage: tmpfs, harness: codex} is policy.home_storage
  #   /home/storage. Each malformed input below gives exactly one problem: exit
  #   2, file itos-container.yaml, ok false, no policy member, and no state or
  #   engine access:
  #     harness: null, 7, true, [codex] or {kind: codex} -> policy.type /home/harness
  #     harness: "", Codex, CODEX, " codex", opencode or codex-v1
  #       -> policy.home_harness /home/harness
  #     {harness: codex} -> policy.required /home/storage
  #     {storage: tmpfs, harness: codex} -> policy.home_storage /home/storage
  #     {storage: volume, harness: codex, harness: codex} -> policy.duplicate_key
  #       /home/harness (the parser's existing rule)
  #   policy.type keeps its existing message and version-naming fix. At
  #   /home/harness, policy.home_harness has the message "Home harness is
  #   unsupported" and the fix "use home.harness codex, or omit harness". At
  #   /home/harnesses it keeps "use home.harnesses [opencode], or omit harnesses".
  #   An input with more than one defect is not judged here; the existing
  #   collection rules apply.
  # - The JSON of file-only doctor (doctor --policy-only --json) is
  #   {"schema":1,"ok":true,"scope":"policy","problems":[],"policy":P}, where P is
  #   {"version":N,"manifest":M,"network":{"mode":"offline"},"home":H}.
  #   H is {"storage":"volume","harness":"codex"}, plus "harnesses":["opencode"]
  #   when that list is requested. A version 3 file without credentials has no
  #   credentials member. Plain file-only doctor prints the line
  #   "home.harness: codex" right after "home.storage: volume" and before any
  #   "home.harnesses:" line.
  #
  # Manifest (doctor --manifest-only, ordinary doctor, approve and run share one
  # manifest inspection):
  # - Only when the same policy selects home.harness codex, a containerEnv key
  #   equal to CODEX_HOME under ASCII case folding (CODEX_HOME, codex_home,
  #   Codex_Home) is manifest.reserved at /containerEnv/<the key as written>.
  #   The problem has exit 2, the manifest's file and the existing message
  #   "Environment name is reserved" and fix. Run later sets CODEX_HOME itself
  #   (slice-55). Without that selection the key is ordinary, under no home, a
  #   plain volume home or the OpenCode list alike, and the effective
  #   containerEnv keeps it with its value.
  # - Only that exact name is reserved. CODEX_HOME_DIR, CODEX_SANDBOX, CODEXHOME
  #   and MY_CODEX_HOME stay ordinary under every selection, the Codex one
  #   included. The existing reserved names and prefixes are unchanged. Image
  #   metadata cannot carry containerEnv (it is descriptive only), so it gains
  #   no rule.
  #
  # Approval (doctor and approve):
  # - The effective approval's home object gains the member harness, exactly
  #   {"kind":"codex","seed":"codex-v1","target":"/home/itos/.codex"}. It is
  #   omitted when the harness is not selected, so every approval without it
  #   keeps its canonical bytes and fingerprint. The canonical encoding
  #   (internal/engine/approval/canonical/json.go) sorts keys recursively, so
  #   the home object's bytes with the harness are
  #   {"harness":{"kind":"codex","seed":"codex-v1","target":"/home/itos/.codex"},"lifetime":"project","mode":"0700","storage":"volume","target":"/home/itos"}.
  #   With the OpenCode list as well, "harnesses":[{"name":"opencode",
  #   "profile":"opencode-v1"}] sits between "harness" and "lifetime". The
  #   fingerprint is "sha256:" and the lower-hex SHA-256 of the whole canonical
  #   settings, as for every approval. Profile names stay as slice-34 named them
  #   (offline-home-v1 and the rest). The strict record codec accepts this exact
  #   object and refuses any other harness value as state.invalid. codex-v1
  #   names the seed that slice-55 specifies, so a different seed needs a new
  #   seed ID and a new approval. An older binary's codec refuses such a
  #   record; that downgrade lockout belongs to p2-control-state-upgrade-contract.
  # - doctor --json gains no field. approval.effective carries the home object
  #   above. approval.differences against an approval without the harness
  #   holds {"path":"/home/harness","before":null,"after":<that object>}. The
  #   existing differences are sorted by path in byte order, and when only the
  #   harness changes this is the only entry. approval.fingerprint is the new
  #   digest.
  # - Plain doctor and bare approve's review print the existing shared home
  #   disclosure (presentation homeDisclosure). When the Codex harness is
  #   selected, these six lines follow its existing three, each a whole line,
  #   case-sensitive, in this order, and before any OpenCode harness lines:
  #     Codex harness: seed codex-v1 at /home/itos/.codex.
  #     Each run sets CODEX_HOME=/home/itos/.codex and rewrites only config.toml there from the fixed codex-v1 seed.
  #     The seed turns off Codex update checks, analytics and plugins, and Codex's own sandbox; this container is the sandbox.
  #     Codex sessions, history and other files under /home/itos/.codex are hostile worker data kept for the next run; run does not read, reset or remove them.
  #     CODEX_HOME is reserved: the manifest's containerEnv cannot set it.
  #     No Codex login is delivered by this setting. The seed is no network or credential grant.
  #   Plain doctor prints no difference lines. Only the review prints the
  #   existing "path: before -> after" lines, so it adds
  #   /home/harness: null -> {"kind":"codex","seed":"codex-v1","target":"/home/itos/.codex"}
  #   in the sorted order above, before its single y/N prompt.
  # - The harness is not a network or credential grant, and q-52's rules are
  #   unchanged. An offline request with the harness and no credentials may
  #   still be approved by its matching --fingerprint, and allowlist or
  #   credential settings still need the person at a terminal. The fingerprint
  #   route shows no review, so ID-CODEXHOME-02 shows the disclosure through
  #   plain doctor and bare terminal approve.
  # - Adding or removing the harness changes the fingerprint. A fingerprint
  #   computed for the other side is stale: approve --fingerprint <stale> --json
  #   exits 1 with exactly one problem, approve.fingerprint, with no prompt, and
  #   leaves the approval record's bytes unchanged. The fresh fingerprint is the
  #   fixture's own digest of the expected settings with the harness.
  #   Inspecting or approving allocates no volume, creates no worker and reads
  #   no host Codex file.
  #
  # Run gate, until slice-55 lifts it: run cannot seed yet, so it never starts a
  # worker for a Codex selection.
  # - Readiness inspection (internal/engine/readiness/domain/inspect.go) refuses
  #   a valid policy that selects home.harness codex for run only, the same way
  #   it gates version 3 (AllowVersion3, set for doctor and approve in
  #   cmd/itos-container/main.go). Doctor and approve opt in; run does not. The
  #   problem is policy.home_harness, exit 2, file itos-container.yaml, path
  #   /home/harness, message "Home harness is not accepted by this command yet"
  #   and fix "omit home.harness to run, or review what it requests with doctor
  #   and approve". So run's stderr holds exactly
  #     itos-container: policy.home_harness "itos-container.yaml/home/harness": Home harness is not accepted by this command yet; omit home.harness to run, or review what it requests with doctor and approve
  #   and its stdout is empty.
  # - Run's order is unchanged around the gate: its argv and JSON refusals,
  #   the checkout pin, then the policy's own problems (a malformed policy
  #   reports those and never the gate), then the version 3 gate (a version 3
  #   file reports policy.version at /version, exit 2, with or without the
  #   harness), then the Codex gate, then the manifest (so a CODEX_HOME key is
  #   not reported under the gate), approval state and comparison, and the
  #   engine. The gate does not depend on approval: with no approval, a
  #   matching approval or an approval without the harness, run reports the
  #   gate, exit 2, and opens no engine connection.
  # - Removing: a request without the harness over an approval that carries it
  #   passes the gate. Run then reports the existing local mismatch, exactly
  #   "itos-container: run.approval-changed: run approval changed at
  #   /home/harness; use itos container approve", exit 1, before any engine
  #   connection. A request without the harness over no approval keeps the
  #   existing missing-approval refusal.
  #
  # Architecture and libraries: the policy engine validates home.harness
  # (port.Home gains Harness, JSON "harness" omitted when empty). The manifest
  # engine takes the selection's extra reserved name from its caller (readiness
  # and doctor's manifest-only inspection), so it imports no policy package.
  # Approval canonical and presentation bind and disclose the object, and the
  # readiness inspector gates run. No slice imports another, only cmd
  # assembles, and porttest fakes do no I/O. No new package, dependency or
  # service. The session codec and the run, status and stop slices are
  # untouched, because the gate stops every Codex selection before freezing.
  # The implementation updates docs/CONFIG.md (home.harness, codex-v1 as
  # disclosed, the run gate), docs/CLI.md's "Implemented approval", and the
  # home object of docs/schemas/itos-container-v1, -v2 and -v3.schema.json
  # (harness with const "codex"). Each schema closes home with
  # additionalProperties false. No arc42 departure is expected; if the code
  # shows one, record a decision in the same pull request.
  #
  # Step handoff: every phrase in ID-CODEXHOME-01 and 02 is NEW. Reuse the
  # helpers behind ID-HOME-25 to 27 (features/harness_home_opencode_steps_test.go):
  # isolatePolicy, homePolicyRun and homePolicyRefused's file-only checks
  # (that function judges w.home.policy[2:], so the new definitions keep their
  # own slice bounds); approvalCheckout, approvalSeed, approvalOracle with
  # approval.home set to the expected map, runDoctor, approvalTerminal,
  # seedControlLocations, runWith and the doctor Engine API fixture's
  # requests() and listener connection count (features/doctor_engine_test.go).
  # Givens write temporary Git checkouts, raw policies and manifests,
  # independently encoded approval records (never through the product codec)
  # and host-home Codex sentinels (~/.codex/config.toml, ~/.codex/auth.json and
  # ~/.codex/sessions/sentinel.jsonl under the isolated HOME). None calls
  # product doctor, approve or run, and none asserts. Every product run is
  # captured in When and judged in Then. The fixture computes both fingerprints
  # itself and treats a non-differing pair as a fixture error. Add the
  # definitions alone in a red test commit with @wip kept. Both rows are local:
  # neither needs a real engine, so their red and green runs are local, and
  # both stay out of TestHomeNativeStepDefinitions's selected IDs.
  #
  # Expected red, from reading main at 9743454 (predictions, not executed
  # evidence), always at the first Then, never in a Given or When:
  # - ID-CODEXHOME-01: file-only doctor refuses every harness input as
  #   policy.unknown_key /home/harness, exit 2, so the accepted Codex requests
  #   fail first. Later, CODEX_HOME is ordinary under every selection.
  # - ID-CODEXHOME-02: ordinary doctor --json refuses the requested policy as
  #   policy.unknown_key /home/harness, exit 2, with no approval.effective, so
  #   the first Then fails. Later, the removing checkout's record fails the
  #   strict decode (run.approval-invalid, not changed at /home/harness), and
  #   the adding and matching checkouts report policy.unknown_key, not the gate.
  #
  # The implementer may commit freely in its own workspace; the host makes the
  # landing commits and handles itos, submission, publication and review.
  @slice-54 @ID-CODEXHOME-01
  Scenario Outline: Policy selects the Codex harness only as an explicit volume home string and reserves CODEX_HOME only then
    Given a Codex harness policy fixture using version <version> and no credential grants
    When file-only doctor inspects the accepted Codex harness requests and the malformed Codex harness requests in JSON, and plain file-only doctor inspects the Codex request
    And manifest-only doctor inspects each CODEX_HOME spelling and the near names under the Codex, plain volume, OpenCode and omitted home selections
    Then each accepted Codex request succeeds with exactly its normalized home storage, harness and harnesses, and plain doctor prints home.harness codex after home.storage
    And each malformed Codex harness request has exactly its specified rule, subject and fix without state or engine access
    And under the Codex selection each CODEX_HOME spelling is manifest.reserved at its own containerEnv path with exit 2 and no effective manifest
    And under every other selection each CODEX_HOME spelling stays in the effective containerEnv with its value, and the near names stay under every selection, the Codex one included

    Examples:
      | version |
      | 1       |
      | 2       |
      | 3       |

  # ID-CODEXHOME-01's inputs. The accepted requests are
  # {storage: volume, harness: codex} and
  # {storage: volume, harness: codex, harnesses: [opencode]}; the malformed ones
  # are listed above. The manifests, each a valid pinned-image manifest whose
  # containerEnv holds only the entries named:
  #   {"CODEX_HOME":"/x"}, {"codex_home":"/x"}, {"Codex_Home":"/x"}, and the near
  #   names together {"CODEX_HOME_DIR":"/x","CODEX_SANDBOX":"seatbelt",
  #   "CODEXHOME":"/x","MY_CODEX_HOME":"/x"}.
  # The selections are home {storage: volume, harness: codex}, {storage:
  # volume}, {storage: volume, harnesses: [opencode]} and no home. Each of the
  # 16 pairs runs doctor --manifest-only --json. A reserved pair has exactly one
  # problem: manifest.reserved, exit 2, path /containerEnv/<key>, file the
  # manifest's path, manifest.valid false and no effective member. Every other
  # pair has ok true, no problems, and an effective containerEnv equal to the
  # manifest's entries.
  #
  # ID-CODEXHOME-02's fixture. Every checkout is version 1 offline, built by
  # "an approval checkout and isolated host state"'s helpers. The fixture
  # seeds an approval of a volume home without the
  # harness and requests {storage: volume, harness: codex}. Its other run
  # checkouts, each with its own Engine API fixture, are removing (the record
  # carries the harness, the request omits it), adding (the record omits it,
  # the request carries it), matching (both carry it) and the no-harness
  # control (neither does). The control must reach the readiness inspection
  # (GET /version, GET /info and GET /images/<approved image>/json journalled,
  # no run.approval-changed), so a gate that refuses every home cannot pass.
  # The fixture answers every later request 403, as ID-HOME-27's does, and the
  # control's later outcome is not judged.
  @slice-54 @ID-CODEXHOME-02 @local-engine-fixture
  Scenario: Doctor and approve disclose and bind the Codex harness, and run refuses it until seeding exists
    Given a Codex harness approval fixture whose checkout requests the Codex harness over an independently seeded approval of a volume home without it, with host-home Codex sentinels
    And Codex harness run checkouts with raw policies and independently encoded approval records for removing, adding and matching the harness and a no-harness control
    When JSON doctor and plain doctor inspect the requested Codex harness, approve runs without a terminal as JSON with the seeded approval's fingerprint, and then bare approve runs at a terminal and receives "y" after the review
    And the itos-container binary runs each Codex harness run checkout through its production approval preflight
    Then JSON doctor reports the independently expected home object with the Codex harness, the single /home/harness difference from null to it and the independently calculated new fingerprint
    And the stale fingerprint is refused with approve.fingerprint and exit 1 without a prompt and the seeded approval record is unchanged
    And plain doctor and the review before exactly one y/N prompt both show the six Codex harness disclosure lines in order after the home disclosure, and only the review shows the /home/harness difference line
    And terminal approval exits 0 and records the independently expected settings and new fingerprint at generation 2
    And no home volume or worker is created and checkout and host-home Codex sentinels are unchanged
    And the removing checkout reports changed approval at /home/harness with exit 1, the adding and matching checkouts report the Codex harness run gate with exit 2, and none of the three Engine API fixtures saw a connection
    And the no-harness control reports no approval change and its Engine API fixture journal holds the readiness inspection of version, info and the approved image

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

  # Slice-41's local runtime rows; its decisions sit above ID-HOME-25.
  # Only the disk backend judges the worker environment. There cmd's production
  # docker adapter creates and inspects the worker at the SDK recording endpoint
  # (home_sdk_fixture_test.go), and the step reads each worker's environment
  # from both the journalled create body and the inspection reply. The memory
  # fake records argv and mounts but no environment (porttest/run.go), so the
  # memory half judges only the literal argv, the mounts and archive copies,
  # and home reuse; no environment clause, in either Then, is judged there.
  # @linux keeps the row from passing on a process whose disk subtest is
  # skipped (home_local_contract_test.go), as the macOS and Windows jobs do.
  @slice-41 @ID-HOME-28 @local-engine-fixture @linux
  Scenario: The worker gets the OpenCode profile and literal argv and nothing from the host
    Given an OpenCode launch fixture with conflicting image OPENCODE and XDG defaults, host OPENCODE XDG and HOME sentinels, host-home OpenCode configuration and login sentinels and two sequential runs of "opencode --standalone run --format json --model fake/fake-model say-ok"
    When fresh run controllers execute both runs through both local backends
    Then each worker's command is exactly the supplied argv and on the disk backend its created and inspected environment holds exactly the opencode-v1 values and the home mappings over the image defaults
    And no worker environment, mount or archive copy carries a host sentinel or any OpenCode file
    And the second run reuses the same home without writing into it and host-home sentinels are unchanged

  # The row runs only through the disk backend. There cmd's production docker
  # adapter creates the worker and inspects it at the SDK recording endpoint
  # (home_sdk_fixture_test.go), so VerifyRun's sameEnvironment judges a real
  # inspection reply. The memory fake observes no environment and refuses only
  # by its Boundary flag, so it cannot tell a missing profile from a defect. No
  # row sets that flag. @linux keeps the scenario from passing on a run whose
  # only disk subtest was skipped.
  # The recorder leaves every create body unchanged. It alters only the
  # inspection reply for the defective launch's worker: a "missing" row removes
  # the named entry, an image row puts back the synthetic image's own
  # conflicting default (OPENCODE_CONFIG_CONTENT={"share":"manual"} or
  # OPENCODE_DISABLE_MODELS_FETCH=false), and the two "set to" rows replace the
  # value with the stated bytes ({"share":"auto"}, no spaces or newline, and
  # false). The control has the same harness request and
  # an unaltered reply. Its create body and its inspection reply must each hold
  # exactly the three opencode-v1 entries with the approved bytes, so a run that
  # never applies the profile fails every row on the control. A value equal to
  # the approved bytes cannot be told from a controller-supplied one, so every
  # defect is missing or differs from those bytes. Every row names a kept
  # opencode-v1 value, and each name has a missing row and a differing row.
  @slice-41 @ID-HOME-29 @local-engine-fixture @linux
  Scenario Outline: Boundary inspection refuses a worker without the exact OpenCode profile
    Given an OpenCode launch fixture whose worker inspection has "<defect>" and an independent correct OpenCode control
    When the OpenCode boundary launches are attempted through the disk backend's recording engine endpoint
    Then the defective launch, whose inspected environment differs from the approved opencode-v1 bytes, reports run.isolation-unverified with exit 1 and no start while the correct OpenCode control's created and inspected environments each hold exactly the opencode-v1 bytes and it succeeds
    And the owned unstarted worker is removed and the retained home and unrelated resources remain intact

    Examples:
      | defect                                                        |
      | OPENCODE_CONFIG_CONTENT left at the image's conflicting value |
      | OPENCODE_CONFIG_CONTENT missing                               |
      | OPENCODE_CONFIG_CONTENT set to {"share":"auto"}               |
      | OPENCODE_DISABLE_MODELS_FETCH missing                         |
      | OPENCODE_DISABLE_MODELS_FETCH left at the image's false       |
      | OPENCODE_DISABLE_AUTOUPDATE set to false                      |
      | OPENCODE_DISABLE_AUTOUPDATE missing                           |

  # Slice-40's local run rows; its decisions sit above ID-HOME-32. Both run
  # through the disk backend only, at the SDK recording endpoint
  # (home_sdk_fixture_test.go), with cmd's production readiness, staging and
  # docker adapters, real /dev/shm staging and the fixture's private source.
  # The memory fake keeps its own bytes, so it cannot judge the content rule.
  # @linux keeps the rows from passing on a process whose disk subtest is
  # skipped. At each worker create the recording endpoint reads the file the
  # bind's source names, before the controller unlinks it, and keeps those
  # bytes, mode and owner for the Then; that read is the positive control for
  # every marker search.
  # ID-HOME-35's source, mode 0600, written with two-space indentation as
  # OpenCode writes it, members in this order:
  #   github-copilot: {"type":"oauth","refresh":M1,"access":M2,"expires":0}
  #   anthropic:      {"type":"api","key":M3}
  #   opencode-go:    {"type":"api","label":M4,"key":K1}
  # where each M is a fresh marker as a JSON string and K1 is a fresh marker
  # whose JSON string token contains the six-byte escape backslash, u, 0, 0,
  # 2, d (a hyphen) where the decoded marker has its hyphen. Every marker search
  # looks for the decoded marker and for its token as written. The expected staged
  # bytes are {"opencode-go":{"type":"api","key":K1}}, K1's token as written.
  # Between the two runs the fixture writes a sibling file (0600) with K2 in
  # place of K1 and renames it over the source, a new inode at the same path.
  # The second run must stage K2 under the first run's approval generation
  # with no consent. The sibling checkout (version 1, the same home request,
  # no grant) is the absence control: its create has no credential bind.
  # Every run's argv is "/workspace/opencode --standalone run --format json"
  # and the endpoint lets each worker start and exit 0.
  @wip @slice-40 @ID-HOME-35 @local-engine-fixture @linux
  Scenario: Run stages only the OpenCode Go entry as a read-only file outside the engine configuration
    Given an OpenCode Go launch fixture with an approved version 3 volume home with the OpenCode harness and an opencode-go grant whose source holds an OpenCode Go entry with an extra member and two other provider logins, each with its own marker, a sibling approved checkout with the same home and no grant, and two sequential runs between which the fixture replaces the source at the same path with a new key
    When fresh run controllers execute both runs and the sibling's run through the disk backend's recording engine endpoint
    Then at each run's create the bind's staged file holds exactly the expected opencode-go bytes with that run's key, mode 0400 and the host UID, and the bind is read-only at /run/itos-container/credentials/opencode-go with no other credential bind
    And no create body, inspection reply, journalled request or response, protected record, status JSON, stdout, stderr, checkout file or archive copy holds any source member's marker, while status JSON lists the opencode-go grant at its fixed target
    And after each run its staging directory is gone, its worker and anchor are removed and the home is retained, and the second run used the same seeded approval generation with no new approval record
    And the sibling's create has no credential bind and no staging directory is made for it, and no run changes the source

  # ID-HOME-36's defective sources are each a mode 0600 file the fixture
  # writes before the run, holding a fresh marker wherever a value appears:
  # no opencode-go member ({"github-copilot":{"type":"oauth",...}}); an entry
  # of type "oauth"; an entry without key; key ""; key 7; entry "M"; two
  # opencode-go members; two key members in the entry; the text opencode-go=M;
  # a top-level array holding a valid object; a valid object followed by {};
  # a valid object whose key string holds the byte 0xff. The two metadata rows
  # start from a valid source and change it after the approval was seeded:
  # chmod 0644, or a second hard link in the same private directory. Those
  # refuse in run's readiness, before any engine request; the content rows
  # refuse at staging, after readiness and before any worker create. The
  # control is a sibling checkout with the same request and a valid source; it
  # must stage exactly its expected entry and exit 0, so a run that refused
  # every opencode-go grant fails every row.
  @wip @slice-40 @ID-HOME-36 @local-engine-fixture @linux
  Scenario Outline: Run refuses an OpenCode Go source it cannot deliver before any worker is created
    Given an OpenCode Go launch fixture whose approved opencode-go source has "<defect>" and an independent OpenCode Go control with a valid source
    When both OpenCode Go launches are attempted through the disk backend's recording engine endpoint
    Then the defective launch reports <rule> with exit 1 located at /credentials/0/source and journals no worker create, while the control's staged file holds exactly its expected opencode-go bytes and it exits 0
    And after the defective launch no staging directory and no unfinished session remain and no owned resource but a retained home is left
    And no marker of the defective source appears in its stdout, stderr, protected records or any journalled request

    Examples:
      | defect                               | rule                   |
      | no opencode-go member                | run.credential-content |
      | opencode-go entry of type oauth      | run.credential-content |
      | opencode-go entry without a key      | run.credential-content |
      | empty key                            | run.credential-content |
      | key that is a number                 | run.credential-content |
      | opencode-go entry that is a string   | run.credential-content |
      | repeated opencode-go member          | run.credential-content |
      | repeated key member in the entry     | run.credential-content |
      | text that is not JSON                | run.credential-content |
      | a top-level array                    | run.credential-content |
      | a second JSON value after the object | run.credential-content |
      | invalid UTF-8 in a string            | run.credential-content |
      | mode changed to 0644 after approval  | credential.permissions |
      | a second hard link after approval    | credential.links       |

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

  # Slice-40's recovery row; its decisions sit above ID-HOME-32. Disk backend
  # only, at the SDK recording endpoint, @linux as ID-HOME-35. The Given encodes
  # each session's protected records itself, never through the product codec:
  # a session of an approved opencode-go run whose controller was lost after
  # its staging was durably owned and its worker created, before any start
  # request, with its staging directory
  # /dev/shm/itos-container-<fresh token> (0700, host UID) holding only the file
  # opencode-go (0400, host UID, the expected entry bytes with a marker), and
  # its owned unstarted worker on the endpoint. The original source is deleted
  # before any When, so neither status nor stop can depend on it. The control
  # session is encoded the same way, but its staging directory also holds an
  # unexpected file named extra. T-48's recovery removes only the expected
  # names and refuses unexpected contents, so the control proves stop deletes
  # by exact name and never recursively.
  @wip @slice-40 @ID-HOME-37 @local-engine-fixture @linux
  Scenario: Fresh stop removes an OpenCode Go staging left by a lost controller and nothing else
    Given an independently seeded session of an approved opencode-go run whose controller is gone after staging, with its staged file, its owned unstarted worker and its source deleted, and a seeded control session whose staging directory also holds an unexpected file
    When fresh status and stop processes inspect and clean both sessions twice through the disk backend's recording engine endpoint
    Then status first reports the session with its opencode-go grant at its fixed target and its exact stop retry, and stop removes its worker and then its staged file and directory and records completion
    And the repeated stop makes no engine mutation and status reports the session complete, and no marker appears in any status or stop output
    And the control's stop records no completion and leaves its staging directory with both its files in place

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

  # Slice-41's native rows. The first assertion needs the absent harness
  # request; output, exit, retained data, egress and cleanup are regression
  # controls in this profile, not new behaviour. The stand-in is installed in
  # the CI checkout as /workspace/opencode and named by that explicit path, so
  # its lookup depends on no image PATH and no reserved manifest PATH.
  @slice-41 @ID-HOME-30 @engine
  Scenario: A headless OpenCode stand-in starts with only its isolated home and configuration
    Given a CI OpenCode checkout with a cached approved image for the actual non-root controller UID GID, an approved offline volume home with the OpenCode harness, host-home OpenCode configuration and login sentinels and a synthetic opencode stand-in installed at "/workspace/opencode"
    When two fresh product runs invoke the stand-in as "/workspace/opencode --standalone run --format json" with a prompt, the first exiting zero and the second exiting seven
    Then in both runs the stand-in reports its literal argv, the opencode-v1 environment and its configuration data and state directories inside a writable private /home/itos
    And it finds no host OpenCode configuration or login and the second run reads the state marker the first wrote under its OpenCode data directory
    And run's stdout is exactly the stand-in's JSON event lines with exit codes zero and seven and product diagnostics only on stderr
    And after each run its worker and anchor are absent while the home remains and the host-home sentinels and checkout ownership are unchanged

  # Reuses the egress native fixture (README, slice-27's engine rows): every
  # name, provider.example and the startup-service name alike, is a
  # controlled TLS alias of a fixture endpoint on a fixture network, so no
  # request needs ambient DNS and a proxy that forwarded the startup-service name
  # would reach a fixture receipt, never a public service. The stand-in stops at
  # the fixture's barrier file in its checkout; there the harness attaches only
  # the proxy to the fixture network, never the worker or the product's two
  # networks, and records that attachment apart. A run refused before its
  # worker starts is captured in When and fails the first Then. Generic
  # allowlist behaviour supplies the network assertions, so the nonce receipt
  # counts only after the stand-in has reported the opencode-v1 profile and the
  # fake model it received. The startup-service name is models.opencode.ai, the
  # catalogue host the smoke record saw on v2.2.1 (finding 5). opencode.ai is the
  # OpenCode Go provider's own host, not a startup service, and models.dev and
  # registry.npmjs.org were not seen on v2.2.1, so none of them is listed.
  @slice-41 @ID-HOME-31 @engine @linux
  Scenario: OpenCode reaches only its approved fake provider and its startup services stay closed
    Given a CI OpenCode checkout allowing "provider.example" with an approved volume home with the OpenCode harness, matching protected approval and a synthetic opencode stand-in installed at "/workspace/opencode"
    And independent fixture TLS aliases with nonce receipts for "provider.example models.opencode.ai" on a fixture network
    When the product runs "/workspace/opencode --standalone run --format json --model fake/fake-model say-ok" and at the stand-in's checkout barrier the harness attaches only the proxy to the fixture network
    Then the run is not refused before its worker starts and the stand-in reports its literal argv, the opencode-v1 environment and model "fake/fake-model" before it sends its model request
    And the fake provider receives exactly the stand-in's nonce request through the proxy variables and the stand-in prints its final JSON event on run's stdout
    And each startup-service name receives 403 without an upstream dial or a fixture receipt while its independent control succeeds
    And the product returns the observed workload exit and confirms its worker, proxy and both networks absent while the home remains

  # Slice-40's native row; its decisions sit above ID-HOME-32. Offline, so no
  # proxy or refusal log. The source is a private synthetic auth.json on the
  # runner (0600 in a 0700 directory on ext4, outside the checkout and state)
  # holding an opencode-go API entry and two other provider logins, each value
  # a fresh marker sharing one fixed prefix. The stand-in first stops at the
  # fixture's barrier file in its checkout, before it reads anything; there
  # the harness inspects the worker through the engine and checks the staging
  # directory the protected session record names, then releases it. The
  # stand-in, as the workload, then does what the review tells a workload to
  # do: it makes $XDG_DATA_HOME/opencode/auth.json a symlink to
  # /run/itos-container/credentials/opencode-go, reads its login through that
  # link, and tries to open the target for writing. It prints only the
  # SHA-256 of what it read, whether that held the marker prefix (the positive
  # control for the home search), whether the write was refused, whether the
  # link resolves and whether any file under /home/itos, symlinks not
  # followed, holds the marker prefix; it never prints a marker. For the
  # second run the fixture rewrites the request as version 1
  # with the same home and no grant and seeds its matching approval. The
  # first assertion needs delivery; the second run's absence, cleanup and the
  # retained home are regression controls. Docker inspection may report
  # paths, never values (decision 0025).
  @wip @slice-40 @ID-HOME-38 @engine @linux
  Scenario: The worker reads only the OpenCode Go entry, read-only, and nothing of it outlives the run
    Given a CI OpenCode checkout with a cached approved image for the actual non-root controller UID GID, an approved offline volume home with the OpenCode harness and an opencode-go grant, a private synthetic auth.json source holding an OpenCode Go entry and two other provider logins each with its own marker, and a synthetic opencode stand-in installed at "/workspace/opencode"
    When the product runs the stand-in, which links its OpenCode auth path to the delivered file, reads its login through that link and tries to write it, and at the stand-in's checkout barrier the harness inspects the worker and the recorded staging directory
    And the fixture rewrites the request without the grant with its matching approval and a second fresh product run invokes the stand-in again
    Then the first stand-in read through its link exactly the expected opencode-go bytes, by digest, holding the marker prefix, its write was refused and no file under /home/itos held the prefix
    And at the barrier the worker's inspection holds a read-only bind at /run/itos-container/credentials/opencode-go and no marker, and the recorded staging directory was already gone before the worker read its copy
    And the second stand-in found its link dangling and no file under /home/itos holding the marker prefix
    And no marker appears in either run's stdout or stderr, status, protected records or checkout, the source keeps its bytes, mode and inode, and after each run its worker, anchor and staging are absent while the home remains

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
