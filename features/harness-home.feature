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
  #   OpenCode itself does: that `opencode --standalone run --format json` is
  #   accepted as written, where inline OPENCODE_CONFIG_CONTENT falls in the
  #   order of its configuration layers (the global, OPENCODE_CONFIG, project,
  #   account and managed layers included), and what each
  #   OPENCODE_DISABLE_* setting below switches off, each with its off/on
  #   control. This spec repeats none of those findings and claims no OpenCode
  #   behaviour beyond them; its scenarios test only what the product controls.
  # - Before building the opencode-v1 table, the implementer reads that record.
  #   If it shows that v2.2.1 rejects the argv above, that a value in the table
  #   below has no effect, or that the table needs a value it lacks, the
  #   implementer stops and reports to the coordinator. They do not adapt the
  #   table or the scenarios. Any change to the table is a feat commit with a
  #   new profile ID and a new approval. The stand-in rows keep their shape.
  #   Another OpenCode release needs its own smoke check before a new profile.
  # - In this slice the account layer has no source, because no OpenCode login
  #   is delivered (slice-40 owns it). Whatever managed configuration the
  #   approved image holds is in its read-only root filesystem, which the worker
  #   cannot write. It is the person's approved, digest-pinned input and may
  #   override the profile. The profile prevents unintended startup calls as a
  #   convenience. It is not an isolation property: the approved egress proxy
  #   stays the only route out and the egress boundary.
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
  #   [OpenCode] -> policy.home_harness /home/harnesses/0; [opencode, opencode]
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
  # - Doctor and approve show, before any consent: the harness name, profile
  #   opencode-v1, every environment name and value below, that OpenCode's
  #   configuration, data and state live only in /home/itos, that the argv and
  #   the checkout choose the provider and model, that a provider is reachable
  #   only through the approved allowlist (for an offline policy: that no
  #   provider is reachable), and that this setting delivers no OpenCode login.
  #   The difference lists add /home/harnesses in the existing sorted order.
  # - The harness profile is not a network or credential grant. q-52's rules are
  #   unchanged: an offline harness request without credentials may still be
  #   approved by its matching --fingerprint; allowlist or credential settings
  #   still need the person at a terminal. The fingerprint route shows no
  #   review (approve's domain skips ui.Review there), so ID-HOME-26 proves the
  #   disclosure through doctor and bare terminal approve: the review prints the
  #   profile, its environment and the /home/harnesses difference before the
  #   single y/N prompt. Inspecting or approving allocates no volume and creates
  #   no worker.
  # - Adding or removing the harness changes the fingerprint and needs a fresh
  #   approval. Run reports the local mismatch as egress.feature specifies,
  #   naming /home/harnesses, exit 1, before any engine connection. Selection
  #   freezes the profile with the other settings, as ID-HOME-04 does for home.
  #
  # Runtime decisions (profile opencode-v1):
  # - The controller sets exactly these worker environment values, sorted, beside
  #   the home mappings above, overriding any image default:
  #     OPENCODE_CONFIG_CONTENT={"autoupdate":false,"share":"disabled"}
  #     OPENCODE_DISABLE_AUTOUPDATE=true
  #     OPENCODE_DISABLE_DEFAULT_PLUGINS=true
  #     OPENCODE_DISABLE_LSP_DOWNLOAD=true
  #     OPENCODE_DISABLE_MODELS_FETCH=true
  #   The inline config value is those exact bytes, with no spaces or newline.
  #   Each name is one the person's v2.2.1 smoke record exercised with an
  #   off/on control. What each switches off on v2.2.1, and which configuration
  #   layers the inline keys win over or lose to, are that record's findings,
  #   not this spec's, as the target-release bullet above says. In ID-HOME-31
  #   the proxy refuses the startup-service names, which is a proxy property and
  #   never evidence of any setting's effect. The values are non-secret and
  #   appear in docker inspect.
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
  #   approved, digest-pinned input.
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
  # ID-HOME-31's startup-service names are refused by the proxy before any dial;
  # their independent controls are fixture aliases, never real hosts. This file
  # is already in the offline shard and features/smoke.yaml; ID-HOME-31 needs the
  # egress fixtures inside that shard, within its time limit. Non-engine red runs
  # are local; every @engine red and green run is CI-only, by dispatch.
  #
  # Expected red, from reading 16e5bdb, whose product code is main's at b880d99
  # (predictions, not executed evidence): the policy parser refuses
  # /home/harnesses as policy.unknown_key with exit 2. Every slice-41 row's first
  # Then fails on that captured result, never in a Given or When.
  #
  # The implementer may commit freely in its own workspace; the host makes the
  # landing commits and handles itos, submission, publication and review.
  @wip @slice-41 @ID-HOME-25
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
  @wip @slice-41 @ID-HOME-26 @local-engine-fixture
  Scenario: Review binds the OpenCode profile without creating storage
    Given an OpenCode approval fixture with an independently seeded approval of a volume home without harnesses and a requested OpenCode harness
    When ordinary doctor inspects the requested harness and bare approve runs at a terminal and receives "y" after the review
    Then doctor and the transcript before exactly one y/N prompt both show the OpenCode harness profile, its exact environment and the /home/harnesses difference with old and new values
    And the review before its prompt states where OpenCode state lives, who chooses provider and model, that no provider is reachable offline and that no login is delivered
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
  # and computes the difference. Engine connections are recorded at a fixture
  # engine endpoint outside preflight, as ID-EGRESS-01's "no endpoint
  # connection was attempted" is. Each mismatch makes no connection there, and
  # each matching control's connection must be recorded there and reach its
  # runtime boundary.
  @wip @slice-41 @ID-HOME-27 @local-engine-fixture
  Scenario: Adding and removing the OpenCode harness both require approval
    Given OpenCode harness approval mismatch checkouts with raw policies and independently encoded approval records for adding and removing the harness and matching controls
    When the itos-container binary runs each changed request and then each matching control through its production approval preflight
    Then each mismatch reports changed approval at /home/harnesses with exit 1 and the recording engine endpoint saw no connection
    And each matching control connects to the recording engine endpoint and reaches its requested runtime boundary with or without the OpenCode environment as approved

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
  @wip @slice-41 @ID-HOME-28 @local-engine-fixture
  Scenario: The worker gets the OpenCode profile and literal argv and nothing from the host
    Given an OpenCode launch fixture with conflicting image OPENCODE and XDG defaults, host OPENCODE XDG and HOME sentinels, host-home OpenCode configuration and login sentinels and two sequential runs of "opencode --standalone run --format json --model fake/fake-model say-ok"
    When fresh run controllers execute both runs through both local backends
    Then each worker's command is exactly the supplied argv and its environment holds exactly the opencode-v1 values and the home mappings over the image defaults
    And no worker environment, mount or archive copy carries a host sentinel or any OpenCode file
    And the second run reuses the same home without writing into it and host-home sentinels are unchanged

  # Each defect is independently injected into the created worker's observed
  # environment; the matching control has the same harness request. A value
  # equal to the approved bytes cannot be told from a controller-supplied one,
  # so every defect is missing or differs from those bytes. The two image rows
  # give the synthetic image a conflicting default and observe it left in
  # effect: OPENCODE_CONFIG_CONTENT={"autoupdate":true} and
  # OPENCODE_DISABLE_LSP_DOWNLOAD=false.
  @wip @slice-41 @ID-HOME-29 @local-engine-fixture
  Scenario Outline: Boundary inspection refuses a worker without the exact OpenCode profile
    Given an OpenCode launch fixture whose worker inspection has "<defect>" and an independent correct OpenCode control
    When the OpenCode boundary launches are attempted through both local backends
    Then the defective launch, whose observed environment differs from the approved opencode-v1 bytes, reports run.isolation-unverified with exit 1 and no start while the correct OpenCode control succeeds
    And the owned unstarted worker is removed and the retained home and unrelated resources remain intact

    Examples:
      | defect                                                        |
      | OPENCODE_CONFIG_CONTENT left at the image's conflicting value |
      | OPENCODE_DISABLE_MODELS_FETCH missing                         |
      | OPENCODE_DISABLE_AUTOUPDATE set to false                      |
      | OPENCODE_DISABLE_DEFAULT_PLUGINS missing                      |
      | OPENCODE_DISABLE_LSP_DOWNLOAD left at the image's false       |
      | OPENCODE_DISABLE_LSP_DOWNLOAD missing                         |
      | inline config with autoupdate enabled                         |

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

  # Slice-41's native rows. The first assertion needs the absent harness
  # request; output, exit, retained data, egress and cleanup are regression
  # controls in this profile, not new behaviour. The stand-in is installed in
  # the CI checkout as /workspace/opencode and named by that explicit path, so
  # its lookup depends on no image PATH and no reserved manifest PATH.
  @wip @slice-41 @ID-HOME-30 @engine
  Scenario: A headless OpenCode stand-in starts with only its isolated home and configuration
    Given a CI OpenCode checkout with a cached approved image for the actual non-root controller UID GID, an approved offline volume home with the OpenCode harness, host-home OpenCode configuration and login sentinels and a synthetic opencode stand-in installed at "/workspace/opencode"
    When two fresh product runs invoke the stand-in as "/workspace/opencode --standalone run --format json" with a prompt, the first exiting zero and the second exiting seven
    Then in both runs the stand-in reports its literal argv, the opencode-v1 environment and its configuration data and state directories inside a writable private /home/itos
    And it finds no host OpenCode configuration or login and the second run reads the state marker the first wrote under its OpenCode data directory
    And run's stdout is exactly the stand-in's JSON event lines with exit codes zero and seven and product diagnostics only on stderr
    And after each run its worker and anchor are absent while the home remains and the host-home sentinels and checkout ownership are unchanged

  # Reuses the egress native fixture (README, slice-27's engine rows): every
  # name, provider.example and the startup-service names alike, is a
  # controlled TLS alias of a fixture endpoint on a fixture network, so no
  # request needs ambient DNS and a proxy that forwarded a startup-service name
  # would reach a fixture receipt, never a public service. The stand-in stops at
  # the fixture's barrier file in its checkout; there the harness attaches only
  # the proxy to the fixture network, never the worker or the product's two
  # networks, and records that attachment apart. A run refused before its
  # worker starts is captured in When and fails the first Then. Generic
  # allowlist behaviour supplies the network assertions, so the nonce receipt
  # counts only after the stand-in has reported the opencode-v1 profile and the
  # fake model it received.
  @wip @slice-41 @ID-HOME-31 @engine @linux
  Scenario: OpenCode reaches only its approved fake provider and its startup services stay closed
    Given a CI OpenCode checkout allowing "provider.example" with an approved volume home with the OpenCode harness, matching protected approval and a synthetic opencode stand-in installed at "/workspace/opencode"
    And independent fixture TLS aliases with nonce receipts for "provider.example opencode.ai models.dev registry.npmjs.org" on a fixture network
    When the product runs "/workspace/opencode --standalone run --format json --model fake/fake-model say-ok" and at the stand-in's checkout barrier the harness attaches only the proxy to the fixture network
    Then the run is not refused before its worker starts and the stand-in reports its literal argv, the opencode-v1 environment and model "fake/fake-model" before it sends its model request
    And the fake provider receives exactly the stand-in's nonce request through the proxy variables and the stand-in prints its final JSON event on run's stdout
    And each startup-service name receives 403 without an upstream dial or a fixture receipt while its independent control succeeds
    And the product returns the observed workload exit and confirms its worker, proxy and both networks absent while the home remains

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
