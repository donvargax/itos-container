@phase-1
Feature: Run through the approved egress proxy
  An allowlist run sends HTTPS only through its owned proxy to exactly the
  approved names. Ignoring proxy variables gives the worker no route out.
  Offline runs keep network none. Approval and a running session's frozen
  permissions are different things: changing one cannot widen the other.

  # Slice-27, specified against main and the slice-26 and T-40 branches.
  # Authorities: charter milestone two step 1; contract AC-02/04/09..14/16/18/38;
  # decisions 0022 (T-38), 0023 (policy), 0025 (q-52); architecture.md Slices;
  # arc42's Egress run; approval.feature's Slice-26 block; T-40's phase-1 entry.
  # This spec owns this file only. Implementation changes the neighbouring live
  # scenarios below in its feat commit, never in the red step commit.
  #
  # q-52, the person, 2026-11-09 18:15 UTC: ask for consent again only when the
  # settings fingerprint changes. An egress allowlist or credential grant needs
  # the person at a real terminal. --fingerprint is accepted only for offline
  # settings without credentials. For a ready grant candidate it always refuses
  # approve.terminal-required, exit 1, even if matching, malformed or supplied at
  # a terminal; its fix names bare "itos container approve" at a terminal. Bare
  # nonterminal/JSON grant approval keeps approve.confirmation-required with only
  # that interactive fix. No keys, signatures or human-authentication claim.
  # An unsandboxed host agent can fake a terminal; docs/security.md must say so.
  # Slice-26's current branch already changes ID-APPROVAL-45 to this refusal,
  # adds interactive consent in 42/43 and no second prompt in 46. Preserve those
  # changes on integration, including 47's precedence and the offline controls.
  # Credential syntax/delivery belongs to slice-29/30/31/33, not this feature.
  #
  # Carry forward matching allowlist-v1 approvals, including ones recorded before
  # run's gate lifts. Keep their canonical bytes, fingerprint and generation.
  # The gate lifting changes no approved setting and creates no new profile.
  # These records already bind the list, CONNECT/443/first-hello boundary and
  # proxy image; q-52 is enforced before they can be written. Reordering or IDNA,
  # case or trailing-dot respelling keeps consent; adding/removing/replacing a
  # name, changing mode/version or the pinned proxy reference changes it.
  # A local mismatch says exactly "run approval changed at <pointers>; use itos
  # container approve", with sorted comma-separated JSON pointers and no values.
  # Local mismatches precede even an engine connection. Keep slice-26's shorter
  # changed message when atomic selection has no comparison available.
  #
  # Required supersessions, stated in the implementing feat commit's body:
  # - Remove ID-POLICY-62 and ID-APPROVAL-51's temporary version-2 refusals.
  #   EGRESS-01/02/03 replace them with changed, allowlist and offline run cases.
  # - Remove the remaining version-2 gate in readiness/domain/inspect.go and
  #   cmd's opt-in distinction. ID-POLICY-60/61 and the three policy.feature
  #   comments about only file-only doctor reading version 2 are already removed
  #   or corrected by slice-26: ensure none survive integration. Replace the
  #   corresponding zero-value gate test, retaining malformed-policy precedence.
  # - Remove "Run cannot start this profile yet: it refuses every version 2
  #   policy for now." from presentation and its scenario rows, including
  #   ID-APPROVAL-42, 43 and 46. Retain every grant/handshake/log disclosure.
  # - Extend run.feature's inspection-only journal and the shared
  #   doctorAllowedJournal/doctorAllowedRequest assertion behind "no engine
  #   mutation or image acquisition was attempted". Allowlist fixtures may also
  #   permit GET /images/<pinned proxy reference>/json without a query, only when
  #   image inspection is permitted. Other image paths, queries, mutations and
  #   acquisition stay forbidden; allowImage=false still permits neither image.
  #   Preserve empty-journal approval refusals, offline worker-image-only journals,
  #   argv/JSON precedence and existing identity safeguards. This shared assertion
  #   is reused in EGRESS-04/05/06/08/20, not just run.feature.
  # Update CLI/CONFIG/run help for approved offline or allowlist runs; keep the
  # hidden proxy command out of help/completion. In the same implementation PR,
  # record the gate removal and runtime observation handoff in docs/decisions,
  # update arc42's building blocks/runtime/status, and restore Solution strategy's
  # doctor/approve/run agreement on accepted inputs and effective settings.
  # Ordinary doctor and approve now check egress prerequisites read-only too.
  # docs/security.md repeats the status limitations below, q-52 and the existing
  # late-checkout-socket gap. No claim that this closes that separate IPC route.
  #
  # Dependencies beyond the registry's slice-26, T-39 and T-40: the coordinator
  # must schedule p1-acceptance-static-binary and p1-egress-refusal-log-writer
  # before implementation can complete. The former builds ordinary and coverage
  # fixtures with CGO_ENABLED=0, as releases do. T-40 refuses a grant without a
  # RefusalLog and retains resources on copy failure; bind the real streaming
  # writer to both Runner and Recovery. No nil, discard writer or test fake in
  # production. Slice-28 keeps log sequence/gap reporting, status counts, hostile
  # display escaping, proxy-failure and recovery scenarios. This item binds the
  # existing failure closure and normal cleanup; it does not waive them until 28.
  #
  # Architecture and libraries: extend run/doctor/approve/status via their own
  # domain/port/adapter siblings and shared readiness/approval/session APIs.
  # Add internal/slices/egressproxy/command only, a thin handler over egress's
  # Proxy port. Only cmd assembles it with egress/tcp and supplies stdout as its
  # Log. Cmd is the only place joining policy's concrete canonicalisation and
  # egress. Egress imports no policy or session engine; no slice imports another.
  # T-38/q-51 chooses our existing standard-library proxy, with no interception
  # code and no new library/service. Reuse pinned Go 1.27.3 (debug/elf in an
  # adapter), moby client v0.6.2/api v1.56.1, x/term v0.46.0 and creack/pty v1.1.24.
  # Preserve notices; do not widen domain/port imports for executable inspection.
  #
  # The product proxy reference is exactly
  # docker.io/library/busybox@sha256:5624bb6777cf469b1fb50ae3f981b15d2715ee2fffaf20b9998cfb7843726835
  # as slice-26 binds it. Inspect that cached image, its config, linux/amd64 and
  # digest attestation; never pull/build or use the worker's image as a fallback.
  # Missing image uses doctor.image.missing (exit 3), with this exact fix:
  # docker pull docker.io/library/busybox@sha256:5624bb6777cf469b1fb50ae3f981b15d2715ee2fffaf20b9998cfb7843726835
  # Other unusable proxy observations use doctor.egress-image, exit 3, subject
  # that reference, with guidance to restore its verified cached linux/amd64 image.
  # No engine response body or substitute image value is printed.
  # Require native Docker >=28.0 and negotiated API >=1.48 for allowlist only:
  # doctor.egress-engine, exit 3, subject DOCKER_HOST, fix naming both minimums.
  # Unknown/malformed versions refuse too; actual post-create verification stays
  # mandatory because a version alone cannot prove isolated bridge support.
  #
  # Resolve the controller's own executable, not PATH or a checkout-supplied path.
  # Require a readable regular static Linux amd64 ELF outside the pinned checkout;
  # inspect ELF headers without executing helpers. Wrong class/machine, PT_INTERP
  # or a dynamic dependency, unreadable/uninspectable input and a resolved path
  # inside the checkout refuse doctor.egress-executable, exit 3, before create,
  # with guidance to use the installed static Linux amd64 release outside it.
  # The existing hard-link/socket/checkout scans remain. Offline needs none of
  # these proxy prerequisites. OS/architecture are values in domain tests.
  #
  # Snapshot-to-session handoff: Names is the frozen policy's canonical names,
  # one per line without final newline, not the raw YAML nor current approval.
  # Egress.ImageID is the freshly inspected proxy CONFIG ID, not the manifest
  # digest in proxy.image. Preserve that observation durably with the original
  # session binding before any create; recovery must recover the same ID without
  # consulting current policy, current approval or a replacement cached image.
  # It is runtime evidence, not a new permission or fingerprint input. Keep the
  # canonical approval snapshot unchanged; version any extended execution record
  # and retain old offline recovery. Never infer a config ID from a manifest hash.
  # The implementation's decision record must document that durable mapping.
  #
  # T-40's exact launch remains: pin/scan/anchor, owned isolated internal IPv4
  # bridge with no gateway, owned IPv4 egress bridge, owned proxy, verify/start/
  # ready, then verified worker. Proxy has both networks, ip_forward=0, no ports,
  # only the read-only /itos-container executable bind, fixed UID:GID and all
  # T-38 restrictions. Its Cmd is /itos-container __egress-proxy -- <each name>;
  # Env is only the inert image's defaults. Worker has only the internal network
  # and its existing checkout/tmpfs mounts and restrictions. HTTP_PROXY,
  # HTTPS_PROXY and lowercase forms name http://<proxy internal IPv4>:3128;
  # NO_PROXY/no_proxy are localhost,127.0.0.1,::1. No worker engine access, added
  # route, capability, published port, fallback, proxy restart or policy reload.
  # Preserve 10 s calls/120 s startup/30 s cleanup, no workload lifetime limit.
  # Bind T-40's watcher and cleanup even before slice-28's native failure proof.
  # run.proxy-failed and run.proxy-exited use exit 1 unless an observed nonzero
  # workload exit wins; unknown exits stay unknown and cleanup errors stay separate.
  #
  # Plain doctor names allowlist-v1, the exact canonical list and proxy reference,
  # CONNECT/443 without interception and the isolated internal network. Its JSON
  # keeps schema 1 and existing effective settings; inspection.ready requires the
  # new observations. File-only doctor remains file-only and makes no such claim.
  # For status, derive each grant from that session's validated original snapshot,
  # even if current requests/approval have changed or disappeared. Add an optional
  # sessions[].egress object {profile:"allowlist-v1",allow:[names],limitations:[text]}.
  # Offline entries omit it. The limitations are these four strings, in order;
  # plain status prints them beside the session's profile and network.allow lines:
  #   No TLS interception: the proxy verifies the CONNECT name and the first ClientHello's SNI.
  #   It cannot see the HTTP Host inside TLS or anything after that hello, a second hello after a HelloRetryRequest included, so fronting remains a limit, as does a hello the upstream reads differently, and it cannot stop data sent to an allowed service.
  #   Clients that omit SNI or send ECH, and SSH over 443, are refused.
  #   The proxy logs each refusal, but a stored log is no proof that every refusal was kept: a log that ends early reads as a complete one.
  # Report facts for both live and completed sessions, never re-open consent or
  # claim a stored count proves completeness. No new log count/path in this slice.
  #
  # Fast tests beneath these scenarios (no engine):
  # - In cmd, feed real policy.Inspect with its real IDNA adapter to
  #   egress/domain.NewAllowlist; do not copy either implementation into the test.
  #   Include every accepted policy hostname vector, mixed case, trailing ASCII
  #   and Unicode dots, IDNA A/U labels, hyphens/digits, 63-byte labels and total
  #   253-byte names, plus a fixed deterministic generated corpus. For every
  #   accepted policy result, egress accepts the WHOLE list and admits each exact
  #   name:443. Assert independent expected names for the named vectors so that
  #   skipping inputs or normalising twice cannot hide drift. Invalid raw variants
  #   stay refused by egress. This is the fast cross-engine drift check; no new
  #   domain-to-domain import, random per-push fuzz run or real-engine dependency.
  # - Cmd tests hold non-nil Names at every inspector, the hidden handler's real
  #   adapter, Engine.Executable and both real RefusalLog bindings. Handler tests
  #   use egress/port/porttest, preserve every argv name after --, log only records
  #   to stdout, put safe typed failures on stderr and return nonzero on Serve
  #   failure. An unknown command is not red evidence for a handler test.
  # - Shared executable-observation contracts run against an I/O-free fake and
  #   the real adapter over synthetic ELF files. Test runtime image handoff,
  #   persisted original config ID, restart/recovery and refusal of missing or
  #   conflicting observations; never extend an approval by writing observation
  #   data. Canonical snapshots and prior offline records remain byte-compatible.
  # - Run session/approval/readiness/egress contracts and cmd tests. Neighbours are
  #   policy, approval, run, offline, lifecycle, recovery, sockets, hardlinks and
  #   anchors, with native cases only in their CI tier. Extend the existing engine
  #   shard file selection/contract to include this whole file in the other shard;
  #   never let new @engine scenarios be silently unassigned. No gate weakening.
  #
  # Step handoff: generic runs/exit/output, approval checkout/seed/record checks,
  # run preflight checkpoint/journal assertions and terminal approval steps exist
  # in approval_steps_test.go and run_preflight_steps_test.go. Slice-26 supplies
  # request/edit/oracle and review-list steps. Every phrase specific to "egress"
  # below is NEW unless it is one of those generic assertions. Add definitions
  # in a red test commit, never implement production behaviour in that commit.
  # EGRESS-11 also needs the NEW step "the active approval record is snapshotted".
  # After the generation-1 assertion and before the second invocation, refresh
  # recordBefore and recordInfo from that stored record, as approvalCapture does:
  # bytes (including generation), inode and mtime. Never seed or rewrite approval
  # here, recapture during the second approvalTerminal, or relax any of
  # approvalRecordUnchanged's byte, os.SameFile or mtime comparisons.
  # The local egress fixture extends the recorded HTTP Engine API fixture with
  # T-40's networks, proxy stdout-ready/exit and two independent image responses.
  # Its default daemon is 28.0/API 1.48; the worker's image and the proxy's image
  # have DIFFERENT config IDs so swapping them cannot pass. Fault scripts change
  # only their named observation. Multi-command steps capture each invocation's
  # output/exit/journal independently (doctor --json, approve --json, then run);
  # all Then assertions apply to all three, not just the last response. The
  # exact changed-list assertion compares the message after the normal CLI prefix
  # and rule, and checks both output streams for either list's names.
  # Givens only write policy/independent canonical approval or original session
  # records, set response/fault scripts, copy fixture executables and snapshot
  # files. They never require a successful product approval or launch. An earlier
  # approval is seeded in slice-26's exact format, with no invented provenance.
  # The completed status fixture seeds original runtime evidence too. No fake
  # Docker client or source-text assertion stands in for the actual CLI/adapter.
  # EGRESS-03 supplies a native offline-compatible pre-28 daemon and no proxy
  # image; the binary is copied, not hard-linked, inside the temporary checkout.
  # EGRESS-20 uses the same copy with an allowlist and healthy engine/image facts.
  # It executes only that built test binary, never checkout hooks or workload
  # code on the host. This holds the real command-to-executable-check wiring;
  # EGRESS-07 covers ELF inputs that an OS loader could not execute as a CLI.
  #
  # All CI Givens provision ONLY trusted test controls, private temporary checkout
  # and state, a synthetic approval, and the worker probe. Product launch happens
  # in When, whose nonzero/timeout result is captured for Then, not a setup error.
  # Existing engineFixtureAllowed guards every native access, not just @engine.
  # Use the release-shaped static product and prepared pinned fixture images.
  # No real model, account, credential, public endpoint, host resolver/firewall
  # rewrite or product test bypass. TLS servers generate ephemeral test key
  # material in memory; only public trust/certificates reach the client fixture.
  # Endpoint networks use addresses outside T-39's refused table (11.22.33.0/24
  # is an example), not documentation addresses it rejects. After the worker
  # reaches its sentinel barrier, the harness attaches the proxy to the endpoint
  # fixture network as decision 0022 allows. Never alter the worker's network or
  # the product's two networks; record this extra harness attachment separately.
  #
  # First-instruction denial probes execute before that barrier, and running
  # probes after it. Every route/resolver has nonce-bearing successful independent
  # controls before and after, distinct from the worker nonce and its own loopback
  # controls. Keep controls in disposable namespaces as offline isolation does.
  # For Docker DNS use real embedded DNS and an alias on an OUTSIDE fixture network,
  # never an alias on the worker's internal network: resolving the owned proxy is
  # allowed and is not an escape. Check no unauthorized external query/answer,
  # not absence of 127.0.0.11. Exercise TCP and UDP; reopen topology if q-50's
  # no-forwarding premise fails. An unavailable control fails, never skips.
  # Request/hello tables run on one worker per scenario, with separate connections
  # and a known-good tunnel before and after each bad attempt. Send raw CONNECT
  # text so the client cannot canonicalise a spelling before the proxy sees it.
  # Observe peer certificate, upstream receipts/bytes, worker progress and native
  # configuration independently. Proxy denial happens before dial for bad heads;
  # a bad hello may open upstream TCP but sends zero upstream bytes and none back
  # after 200. No timeout/empty log alone proves a boundary or a completed probe.
  # Normal product cleanup must confirm owned resources absent; harness cleanup
  # is a safety net and cannot satisfy that assertion or erase failure evidence.
  #
  # These are expected reds, not measured evidence. On main the version-2 gate
  # precedes all run behaviour; on slice-26 it still does, while doctor/approve
  # omit runtime prerequisite checks and keep the temporary warning. T-39/T-40
  # are unbound. Each runtime scenario first requires an actual successful worker
  # or positive tunnel, so "refuse every run" cannot satisfy its negative probes.
  # Non-engine reds run with @wip removed after definitions exist; real-engine
  # reds use the guarded CI selector, then ordinary green after implementation.

  # Approval comparison and the gate's replacement.
  @wip @slice-27 @ID-EGRESS-01 @local-engine-fixture @linux
  Scenario Outline: Run explains a changed approved list before opening the engine
    Given an approval checkout and isolated host state
    And the checkout requests the allowlist "api.example.com registry.example.org"
    And a seeded matching approval at generation "7"
    And the egress request is edited to "<request>"
    And the run preflight files are snapshotted
    When itos-container runs with "run -- fixture-workload"
    Then run reports preflight problem "run.approval-changed" with exit 1 and subject ""
    And the egress refusal says exactly "run approval changed at <pointers>; use itos container approve" without either list's values
    And the Engine API journal is empty
    And no endpoint connection was attempted
    And it never prompts or records approval
    And run preflight has changed no checkout or host state

    Examples:
      | request                                             | pointers                                                      |
      | add other.example                                   | /policy/network/allow                                         |
      | remove registry.example.org                         | /policy/network/allow                                         |
      | replace api.example.com with api.example.net        | /policy/network/allow                                         |
      | replace api.example.com with xn--pi-6kc.example.com | /policy/network/allow                                         |
      | version 2 offline                                   | /policy/network/allow, /policy/network/mode, /profile, /proxy |

  @wip @slice-27 @ID-EGRESS-02 @local-engine-fixture @linux
  Scenario Outline: A remembered grant supplies the exact proxy list without new consent
    Given an approval checkout and isolated host state
    And an egress-capable local Engine API fixture
    And the checkout requests the allowlist "api.example.com xn--bcher-kva.example"
    And a seeded matching approval at generation "7"
    And the egress request has equivalent spelling "<spelling>"
    When itos-container runs with "run -- fixture-workload '' 'two words' --json"
    Then the local egress run completes the owned worker with the exact literal argv and exit zero
    And the proxy create uses exactly the frozen canonical list and independently inspected proxy config ID
    And the protected run snapshot is exactly the independently calculated approval at generation "7"
    And the original proxy config ID survives reopening the protected session binding
    And the active approval bytes and generation are unchanged
    And it never prompts or records approval
    And the local egress journal confirms ordered ownership verification readiness and complete owned cleanup

    Examples:
      | spelling                                               |
      | unchanged from the approval written before run support |
      | reversed list order and comments                       |
      | API.Example.COM. and BÜCHER.example                    |

  @wip @slice-27 @ID-EGRESS-03 @local-engine-fixture @linux
  Scenario: Version two offline runs without any egress prerequisite
    Given an approval checkout and isolated host state
    And a local owned foreground Engine API fixture
    And the checkout requests offline at policy version "2"
    And a seeded matching approval at generation "1"
    And the egress-only prerequisites are unavailable while offline readiness is supported
    When itos-container runs with "run -- fixture-workload"
    Then run completes the inspected owned foreground worker with exit zero
    And the worker uses network none without proxy variables or egress resources
    And no proxy image or executable inspection was attempted
    And the local foreground journal contains only inspections and owned worker operations
    And the active approval bytes and generation are unchanged

  # Read-only readiness and delivery. All fault cases have 02/04 as controls.
  @wip @slice-27 @ID-EGRESS-04 @local-engine-fixture @linux
  Scenario: Doctor describes the executable allowlist profile without granting it
    Given an approval checkout and isolated host state
    And an egress-capable local Engine API fixture
    And the checkout requests the allowlist "api.example.com xn--bcher-kva.example"
    When ordinary doctor inspects the egress request in plain and JSON modes
    Then doctor reports ready only after inspecting both pinned images and the installed static executable
    And plain doctor names the exact list and proxy image with CONNECT on 443 and an isolated internal network
    And both doctor outputs report the same independently calculated allowlist candidate with approval missing
    And no engine mutation or image acquisition was attempted
    And the approval checkout and host state are unchanged

  @wip @slice-27 @ID-EGRESS-05 @local-engine-fixture @linux
  Scenario Outline: An absent cached proxy image names its exact pull without acquiring it
    Given an approval checkout and isolated host state
    And an egress-capable local Engine API fixture
    And the checkout requests the allowlist "api.example.com"
    And a seeded matching approval at generation "1"
    And only the pinned proxy image is absent from the egress fixture
    When itos-container runs with "<command>"
    Then the egress prerequisite reports "doctor.image.missing" with exit 3 and the pinned proxy reference as subject
    And its fix names exactly "docker pull docker.io/library/busybox@sha256:5624bb6777cf469b1fb50ae3f981b15d2715ee2fffaf20b9998cfb7843726835"
    And no engine mutation or image acquisition was attempted
    And it never prompts or records approval
    And the approval checkout and host state are unchanged

    Examples:
      | command                 |
      | doctor                  |
      | doctor --json           |
      | approve --json          |
      | run -- fixture-workload |

  @wip @slice-27 @ID-EGRESS-06 @local-engine-fixture @linux
  Scenario Outline: Egress refuses a daemon that cannot supply the isolated bridge
    Given an approval checkout and isolated host state
    And an egress-capable local Engine API fixture
    And the checkout requests the allowlist "api.example.com"
    And a seeded matching approval at generation "1"
    And the egress fixture advertises Docker "<version>" with negotiated API "<api>"
    When doctor approve and run inspect that egress request
    Then each reports "doctor.egress-engine" with exit 3 and guidance naming Docker 28.0 and API 1.48
    And no engine mutation or image acquisition was attempted
    And the approval checkout and host state are unchanged

    Examples:
      | version | api  |
      | 27.5.1  | 1.47 |
      | 28.0.0  | 1.47 |
      | unknown | 1.48 |

  # A wrong-architecture executable cannot itself run the CLI. Reach the actual
  # observation adapter with ELF fixture bytes and the same readiness contract,
  # not a flag to make the product pretend its own executable has another shape.
  @slice-27 @ID-EGRESS-07 @linux
  Scenario Outline: Proxy executable observations refuse unsafe delivery
    Given an egress executable contract with a static Linux amd64 control and defect "<defect>"
    When the shared egress readiness contract inspects both inputs through the fake and real observation adapter
    Then each backend admits the outside static control and refuses the defect as "doctor.egress-executable" with exit 3
    And the defect fix names the installed static Linux amd64 release outside the checkout
    And neither inspection executes a helper or changes a file

    Examples:
      | defect                                |
      | a resolved executable in the checkout |
      | a symlink resolving into the checkout |
      | ELF with PT_INTERP                    |
      | ELF with a dynamic dependency         |
      | ELF for another machine               |
      | ELF with the wrong class              |
      | truncated ELF                         |
      | unavailable executable observation    |

  @wip @slice-27 @ID-EGRESS-20 @local-engine-fixture @linux
  Scenario Outline: A checkout-writable controller cannot supply the trusted proxy executable
    Given an approval checkout and isolated host state
    And an egress-capable local Engine API fixture
    And the checkout requests the allowlist "api.example.com"
    And a seeded matching approval at generation "1"
    And a copy of the built product executable inside that temporary checkout
    When that copied executable runs with "<command>"
    Then the egress prerequisite reports "doctor.egress-executable" with exit 3 and guidance to use the installed release outside the checkout
    And no engine mutation or image acquisition was attempted
    And the copied executable checkout and original protected records are unchanged

    Examples:
      | command                 |
      | doctor --json           |
      | approve --json          |
      | run -- fixture-workload |

  @wip @slice-27 @ID-EGRESS-08 @local-engine-fixture @linux
  Scenario Outline: Unusable cached proxy content cannot substitute for the approved pin
    Given an approval checkout and isolated host state
    And an egress-capable local Engine API fixture
    And the checkout requests the allowlist "api.example.com"
    And a seeded matching approval at generation "1"
    And only the proxy image observation has defect "<defect>"
    When doctor approve and run inspect that egress request
    Then each reports "doctor.egress-image" with exit 3 and the pinned proxy reference as subject
    And no engine mutation or image acquisition was attempted
    And the approval checkout and host state are unchanged

    Examples:
      | defect                           |
      | another attested manifest digest |
      | missing image config             |
      | invalid image config ID          |
      | linux arm64                      |
      | a declared image volume          |

  # Report the grant of this session even after the current request changes.
  @wip @slice-27 @ID-EGRESS-09 @local-engine-fixture @linux
  Scenario Outline: Status discloses the frozen grant and its handshake and log limits
    Given original protected egress and offline sessions with phase "<phase>"
    And the current policy and approval no longer describe either original session
    When status inspects those sessions in plain and JSON modes
    Then the egress entry names only its original profile and allowlist with all four specified limitations in order
    And JSON carries the same facts in that entry's egress object while the offline entry omits it
    And status neither claims TLS interception nor that all refusals were stored
    And status writes no state requests no consent and changes no engine resource

    Examples:
      | phase                          |
      | running with controller absent |
      | stopped with cleanup complete  |

  @wip @slice-27 @ID-EGRESS-10 @local-engine-fixture @linux
  Scenario Outline: A daemon reporting weaker network controls never starts the worker
    Given an approval checkout and isolated host state
    And an egress-capable local Engine API fixture
    And the checkout requests the allowlist "api.example.com"
    And a seeded matching approval at generation "1"
    And the egress fixture returns boundary defect "<defect>"
    When itos-container runs with "run -- fixture-workload"
    Then run reports the observed egress boundary mismatch as "run.isolation-unverified" with exit 1
    And no worker instruction is started and no fallback network or proxy restart is requested
    And every created owned resource is recorded before its confirmed cleanup

    Examples:
      | defect                                |
      | internal bridge ignores isolated mode |
      | internal bridge has an IPAM gateway   |
      | internal bridge enables IPv6          |
      | proxy has IPv4 forwarding enabled     |
      | proxy executable mount is writable    |
      | worker has an additional network      |
      | worker has a published port           |

  # q-52's refusal scenarios stay in approval.feature; this holds the presentation
  # change that becomes truthful only when slice-27 implements the runnable grant.
  @wip @slice-27 @ID-EGRESS-11 @local-engine-fixture @linux
  Scenario: Terminal consent describes a runnable grant and matching settings do not ask again
    Given an approval checkout and isolated host state
    And an egress-capable local Engine API fixture
    And the checkout requests the allowlist "api.example.com registry.example.org"
    When bare approve runs at a terminal and receives "y" after the review
    Then the terminal review describes allowlist-v1 without a temporary run refusal before exactly one y/N prompt
    And the review before its prompt lists exactly the requested host names
    And the sole active approval is the exact candidate at generation "1" in protected host state
    Given the active approval record is snapshotted
    When bare approve runs at a terminal with no answer available
    Then it reports matching approval without prompting and exits with code 0
    And its standard output does not say "Run cannot start this profile yet"
    And the active approval bytes and generation are unchanged

  # Native boundary proof: every case uses the product, not the TCP adapter alone.
  @wip @slice-27 @ID-EGRESS-12 @engine @linux
  Scenario: An approved canonical name reaches the fake TLS endpoint through the owned proxy
    Given a CI egress checkout with policy spellings "API.Example.COM. BÜCHER.example" and matching protected approval
    And independent TLS endpoints for the canonical names with nonce receipts
    When the product runs the egress probe through its proxy variables and explicit CONNECT connections
    Then each canonical name completes TLS with the upstream's own certificate and exchanges its nonce
    And native observations show exactly the specified worker and proxy restrictions and two product-owned networks
    And the proxy argv contains only the frozen canonical list and its image is the inspected pinned content
    And the worker's loopback service works through the specified NO_PROXY values
    And the product returns the observed workload exit and confirms all its owned resources absent

  @wip @slice-27 @ID-EGRESS-13 @engine @linux
  Scenario: CONNECT uses exact names and port text instead of suffixes addresses or client cooperation
    Given a CI egress checkout allowing "api.example.com xn--bcher-kva.example" and matching protected approval
    And independent allowed and unlisted TLS aliases sharing a fixture address
    When the product's worker sends these raw proxy requests between successful canonical controls:
      | method  | target                            |
      | CONNECT | API.example.com:443               |
      | CONNECT | api.example.com.:443              |
      | CONNECT | example.com:443                   |
      | CONNECT | api.example.com.other.example:443 |
      | CONNECT | other.example:443                 |
      | CONNECT | xn--pi-6kc.example.com:443        |
      | CONNECT | <endpoint-ipv4>:443               |
      | CONNECT | [<endpoint-ipv6>]:443             |
      | CONNECT | api.example.com:80                |
      | CONNECT | api.example.com:0443              |
      | GET     | http://api.example.com/path       |
      | CONNECT | api.example.com:443/path          |
    Then every surrounding canonical control completes its TLS nonce exchange
    And every raw request receives 403 with an empty body and closure without an upstream dial for that request
    And the worker completes every table row and the product confirms owned cleanup

  @wip @slice-27 @ID-EGRESS-14 @engine @linux
  Scenario: The first ClientHello must match CONNECT before any tunnel byte passes
    Given a CI egress checkout allowing "api.example.com" and matching protected approval
    And an independent TLS endpoint recording every byte before handshake
    When the product's worker follows successful CONNECT responses with these inputs between valid TLS controls:
      | input                                   |
      | ClientHello with other.example SNI      |
      | ClientHello with API.example.com SNI    |
      | ClientHello with api.example.com. SNI   |
      | ClientHello without SNI                 |
      | ClientHello with ECH extension 0xfe0d   |
      | plain HTTP bytes                        |
      | an SSH banner                           |
      | a truncated ClientHello followed by EOF |
    Then every valid control verifies the upstream certificate and exchanges its nonce
    And every invalid input closes after 200 with zero upstream bytes and no proxy-generated TLS bytes returned
    And the worker completes every table row and the product confirms owned cleanup

  @wip @slice-27 @ID-EGRESS-15 @engine @linux
  Scenario: The worker cannot bypass the proxy from its first instruction or while running
    Given a CI egress checkout and matching protected approval with independent route controls
    When its first-instruction and running probes ignore proxy variables and attempt these routes:
      | route                                                           |
      | direct IPv4 TCP 443 to the allowed endpoint                     |
      | direct IPv6 TCP 443 to a controlled endpoint                    |
      | direct TCP on another port                                      |
      | UDP including QUIC-shaped traffic                               |
      | the host gateway and a host service bound to all IPv4 addresses |
      | private IPv4 and IPv6 destinations                              |
      | synthetic metadata IPv4 and IPv6 destinations                   |
      | host loopback and alternate loopback addresses                  |
      | forwarding via the proxy's internal address without CONNECT     |
    Then the worker has executed both probe phases and a canonical TLS control succeeds through the proxy
    And every route's independent before and after controls exchange their distinct nonces
    And every worker bypass attempt is denied with no matching destination receipt
    And native inspection shows no worker default route outside its isolated network and proxy IPv4 forwarding disabled
    And the product confirms owned cleanup without changing the unrelated control resources

  @wip @slice-27 @ID-EGRESS-16 @engine @linux
  Scenario: Docker DNS provides no unauthorized resolution path out of the internal network
    Given a CI egress checkout and matching protected approval with independent DNS controls
    When its first-instruction and running probes query unique outside names by these paths:
      | path                                                    |
      | the worker's system resolver for A and AAAA             |
      | Docker 127.0.0.11 over UDP and TCP for an outside alias |
      | controlled external DNS over UDP and TCP                |
      | alternate loopback resolvers                            |
      | direct HTTPS DNS                                        |
      | CONNECT to an unlisted HTTPS DNS name                   |
    Then the worker has executed both DNS phases and its allowed proxy-resolved TLS control succeeds
    And each DNS control returns its nonce-bearing answer before and after the worker attempt
    And no worker probe obtains an outside answer or delivers its query to a controlled outside resolver
    And the product confirms owned cleanup without changing host DNS or unrelated controls

  @wip @slice-27 @ID-EGRESS-17 @engine @linux
  Scenario: A listed name that resolves privately cannot turn the proxy into a private route
    Given a CI egress checkout allowing "public.example private.example" and matching protected approval
    And proxy-visible public-range and private-range fixture aliases with independent TLS controls
    When the product's worker CONNECTs to each listed name on 443
    Then public.example completes the verified TLS nonce exchange
    And the private endpoint's independent control succeeds before and after the worker attempt
    And private.example receives 403 with no connection to its private endpoint
    And the product confirms owned cleanup without changing the fixture endpoints

  @wip @slice-27 @ID-EGRESS-18 @engine @linux
  Scenario: Editing and approving a wider list cannot change the running proxy
    Given a CI egress checkout allowing "first.example" and matching protected approval
    And independent TLS endpoints for first.example and later.example
    When the product starts the worker and it attempts both names before an edit after a wider request and after terminal approval of that request
    Then the original worker reaches first.example in every phase and later.example is refused in every phase
    And the proxy argv original session fingerprint and protected snapshot remain unchanged
    When the original run exits and the product starts a fresh run with the newly approved list
    Then the fresh worker reaches both names through a newly owned proxy using the new fingerprint
    And neither run requests consent and both confirm their owned cleanup

  @wip @slice-27 @ID-EGRESS-19 @engine @linux
  Scenario: An allowed redirect cannot grant its unlisted destination
    Given a CI egress checkout allowing "download.example" and matching protected approval
    And download.example returns a TLS redirect to an unlisted independently reachable fixture endpoint
    When the product's worker follows that redirect using the owned proxy
    Then the first verified TLS request receives the expected redirect
    And the unlisted endpoint's independent control succeeds before and after the worker attempt
    And the redirected CONNECT receives 403 without an upstream connection to the unlisted endpoint
    And the frozen allowlist stays unchanged and the product confirms owned cleanup
