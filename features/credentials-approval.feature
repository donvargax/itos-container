@phase-1
Feature: Review and remember credential grants without delivering them
  Approval binds the files a worker would receive, never their contents.
  The person reviews a credential grant at a terminal. A changed grant needs fresh
  consent, and status describes the session's frozen grant, not a later request.

  # Authorities: docs/charter.md, docs/plans/container-pilot.md AC-02/03/04/29/36,
  # docs/architecture.md (Slices), docs/decisions/0025-credentials-passed-in.md
  # and q-52. Read the dependency specs, not their older registry summaries:
  # slice-29 at 5fff7908139202e2adc0c0bb6261e61907decf76 supplies
  # features/credentials-policy.feature and decision 0027-credential-source-inspection;
  # slice-26 at 5d302ac225b153dfa4822ed50d5f7d784928c3f5 supplies
  # features/approval.feature and decision 0026-egress-allowlist-approval.
  # These are drafting baselines, not a request to reset either dependency.
  # Implement only after both dependencies and slice-29's source-exclusion proof.
  # No Codex kind, delivery, staging, account call, live credential or new runtime
  # isolation claim belongs here. T-47's unresolved delivery/account/logging questions
  # remain with their owners. This spec neither answers them nor changes the charter.
  #
  # Canonical contract, extending slice-26's schema-1 settings and envelope:
  # credentials, when present, is one top-level array of closed {kind,source,target}
  # objects, strictly ascending by kind. There is no separate grant name and no
  # duplicate credentials member under policy. The two kinds and literal source
  # path grammar are slice-29's. Current targets are exactly:
  # claude-token -> /run/itos-container/credentials/claude-token
  # github-token -> /run/itos-container/credentials/github-token
  # A nonempty set selects offline-credentials-v1 or allowlist-credentials-v1.
  # Its policy.version is 3. Keep the base profile's network/proxy/host/checkout/
  # manifest/image settings, JSON encoding, project identity and generation rules.
  # Canonical bytes contain paths, not source bytes, their hashes, inode, size,
  # permissions or timestamps. Reordering grants and replacing a still-valid file
  # at the same path do not change the fingerprint. Revalidate metadata nonetheless.
  # With no grants omit credentials altogether, retaining offline-v1/allowlist-v1
  # and every existing byte for versions 1/2. Version 3 without grants uses those
  # same profiles but keeps declared policy.version 3; a version-only change is
  # exactly /policy/version. Existing approvals never silently gain credentials.
  #
  # Stored records remain closed and canonical, with independently verified hashes.
  # Credential profiles require version 3 and a nonempty ascending set of known
  # kinds and valid sources. Base profiles reject credentials, even [] or null.
  # The stored target must be a literal absolute path with exactly one nonempty
  # filename under /run/itos-container/credentials/, using source's lexical/byte
  # restrictions. Targets must be distinct. A different valid historical target
  # is readable but changed, like slice-26's different valid proxy digest: a release
  # moving a target requires new consent, not discarded history. Only the current
  # fixed targets can match a new candidate. Invalid/unknown fields, kinds, versions,
  # targets, ordering and duplicate grants are approval.state.invalid, exit 1.
  # The difference walk treats the entire credentials array as one /credentials
  # value, never indices. An absent member is null in old/new differences. Adding
  # the first/removing the last grant also changes /profile. Other fields retain
  # their own sorted paths. Doctor and approval review show complete old/new arrays.
  #
  # Readiness and staged run boundary:
  # Ordinary doctor and approve now accept valid version 3, including no grants.
  # Reuse slice-29's shared metadata observer, including its bounded checkout walk,
  # state mount projections, safe ancestors and source identity/link checks; do not
  # recreate it or scan other projects' state records. Observe before engine access,
  # revalidate after engine inspection and repeat inspection after terminal consent,
  # including the unchanged-approval path. Never open source data. Preserve located
  # credential.* rules/exits and written policy indices before consent; invalid
  # policy still wins before source inspection. A source failure leaves requested
  # grants visible, inspection.ready=false and no usable approval fingerprint.
  # A change observed after the review uses existing approve.changed, exit 1.
  # A safe replacement/rotation at the same source path is not a settings change.
  # File-only and credentials-only scopes keep slice-29's distinct contracts.
  # At every source observation, compare the policy pin's checkout (device,inode)
  # with the final checkout node of the independent no-symlink boundary walk.
  # Both components must match; equal path text or a stable second walk is not
  # enough. These are descriptor identities, not mountinfo device numbers (q-57).
  # A mismatch is doctor.checkout.changed, exit 1, at the effective root, before
  # a usable candidate or consent; after terminal consent it is approve.changed.
  # Check before engine access, after engine inspection and after consent, also
  # for remembered grants. The shared credentials-only observer checks it too.
  # This closes decision 0027's explicitly accepted slice-29 read-only gap when
  # slice-30 admits grants; it does not claim atomic host observations or bind a
  # later run mount. Keep source inode rotation distinct from checkout substitution.
  #
  # Share slice-29's inclusive 4095-byte path bound with ordinary doctor/approve:
  # an effective root of 4096 bytes is doctor.checkout.root, exit 2; a selected
  # itos-container state root of 4096 bytes is approval.state.location, exit 2.
  # Judge the effective path text before walking any boundary or source, not from
  # a failed descriptor read. Root validation precedes opening policy; state
  # validation follows valid policy and precedes source observation. Count UTF-8
  # bytes, including the selected state's appended /itos-container component.
  # Preserve absolute-then-clean root handling and state selection/fallback rules.
  # Both the message and fix for an overlong path must name the 4095-byte limit
  # and the offending boundary; retain absoluteness/encoding guidance as applicable.
  # Ordinary doctor/approve apply this lexical bound even with no grants;
  # credentials-only keeps its no-grant no-scan result, and file-only scopes still
  # do not inspect host boundaries. Public run's version-3
  # delivery gate stays first; carry these checks into run when its gate lifts.
  # The filesystem choice is settled by q-64 (the person, 2026-11-15), confirmed
  # by the coordinator on 2026-11-16 00:10 UTC. Replace the shared observer's named
  # refusal list with an allowlist: accept exactly ext4, btrfs and xfs; every other
  # filesystem type refuses with credential.filesystem, exit 3. NFS (nfs, nfs4)
  # and 9p remain refused outright. Apply this in both mount observations to source
  # and ancestor descriptors and rows at, below or covering checkout or either
  # protected state root, including covered rows and absent state. Unrelated rows
  # do not refuse an independent source; retain mapping and known-location
  # precedence. The diagnostic's fix names ext4, btrfs and xfs. This shared rule
  # also applies to credentials-only; file-only modes still do not inspect sources.
  # The filesystem decision is no longer an implementation prerequisite to ask.
  # Slice-61 supersedes this filesystem paragraph at its feat commit; until then
  # it describes main. See the slice-61 block after ID-CREDAPPROVAL-29.
  #
  # Run keeps slice-29's early policy.version gate for ALL version 3 requests in
  # this increment, even without credentials. Preserve any slice-27 version-2 gate
  # change independently. This deliberately does not move approval/state reads
  # before a delivery refusal. ID-CREDAPPROVAL-17 first proves new approval succeeds,
  # then proves gated run still refuses before manifest/source/engine/state access.
  # ID-CREDAPPROVAL-18 exercises the public changed-approval refusal by removing all
  # grants and returning to version 1. ID-CREDAPPROVAL-19 exercises remaining grant
  # changes through the real shared local preflight and run error classifier with
  # raw port observations; it does not turn off the public gate. Slice-31/33 must
  # carry those comparisons into public run when they lift their delivery gates.
  # Their lifting alone must not request consent again for unchanged canonical grants.
  #
  # Supersession at implementation, never in this draft:
  # Replace only doctor/approve rows of slice-29 ID-CREDENTIALS-13 with the ordinary
  # admission/refusal coverage here. Keep its run rows and invalid-policy precedence.
  # Slice-26 ID-APPROVAL-50's two rows treating policy version 3 under base profiles
  # as invalid become valid no-grant records, covered by ID-CREDAPPROVAL-04/26 and
  # the codec contract. All other corruption rows remain. Explain these intentional
  # transitions in the feat commit. Preserve older no-grant fingerprints and all
  # control-state ownership, admission, store atomicity and recovery checks.
  #
  # Consent and display decisions:
  # Any current credential grant requires terminal stdin AND stdout and plain mode.
  # Every supplied --fingerprint, even at a terminal, empty, malformed, matching,
  # stale or already approved, refuses with approve.terminal-required, exit 1,
  # after readiness but before fingerprint syntax/equality or matching shortcuts.
  # Bare redirected/JSON approval retains approve.confirmation-required, exit 1.
  # Both fixes name bare "itos container approve" and a terminal, never suggest
  # --fingerprint for grants. --no-json overrides inherited JSON, but not the need
  # for terminals. The existing y/yes, decline and concurrent-generation rules hold.
  # An unchanged terminal approval prints its grants/disclosure without prompting
  # or rewriting bytes, inode, mtime or generation. Consent depends on the CURRENT
  # request: returning to credential-free version 1 permits its exact fingerprint
  # route even if the previous record contained grants. There are no keys/signatures.
  #
  # The exact credential profile line is the base restriction line with its profile
  # name replaced and just these two clauses changed:
  # "checkout-only host mount at /workspace" becomes
  # "checkout-only host directory mount at /workspace plus read-only credential files under /run/itos-container/credentials";
  # "no ports, grants or hooks" becomes "no ports or hooks".
  # Offline retains "network none"; allowlist retains "internal network with no
  # route out" and all slice-26 host-list/CONNECT/SNI/TLS/ECH/fronting/log-loss words.
  # For version 3 replace only the old temporary version-2 run warning with the
  # version-3 warning below. Present "Effective settings:", complete canonical
  # settings, profile restrictions, any network disclosure, sorted credential lines,
  # credential disclosure, sorted old/new differences, then the one y/N prompt.
  # A credential line is "credentials: <kind> source=<JSON string> target=<JSON string>".
  # JSON string display additionally escapes C1/bidi/nonprinting Unicode; neither
  # stored bytes nor structured JSON are normalized. The disclosure is exactly:
  # "This profile grants credentials. Each listed file will be readable by the worker; approval does not limit the token's provider permissions."
  # "Each run copies the current source bytes to memory-backed staging, retains the copy for that worker, and drops it at teardown; the original source is never written."
  # "Workload output and traffic are not redacted."
  # An allowlist credential profile adds these two complete lines:
  # "The worker can send credential bytes to any allowed host."
  # "A refused hostname can contain credential bytes and is kept in proxy and host refusal logs; credential absence from those logs is not established."
  # Approval review/status then says:
  # "Credential delivery is not available in this increment; run still refuses version 3 before manifest, engine or host state access."
  # Plain matching approve repeats this complete disclosure without a prompt.
  # Update approve help, docs/CLI.md, docs/CONFIG.md and docs/security.md with these
  # limits and q-52: an unsandboxed host agent can fake a terminal; approval records
  # what, not who. Detection does not authenticate a person. No new security decision
  # or dependency is selected; reuse the dependencies pinned by slices 26 and 29.
  #
  # Public status adds credentials to each credential-bearing session's JSON entry,
  # the exact sorted {kind,source,target} array from its validated protected snapshot.
  # Omit that member for no-grant sessions, preserving their existing schema-1 shape.
  # Plain status shows the same sorted lines and credential disclosure (including
  # the allowlist warning when applicable), labeled with the session ID. It does
  # not print approval's temporary run warning as a claim about a recorded worker.
  # It never reads current policy/manifest, active approval or source contents or
  # requires sources still to exist. Invalid protected snapshots retain recovery's
  # existing refusal; a current valid request cannot repair or supply missing grants.
  # No new resource, progress/staging field, restart or stop behavior belongs here.
  # Seeded historical sessions test presentation only, not successful delivery.
  #
  # Architecture: approve/doctor/run/status remain their existing vertical slices.
  # Canonicalization, comparison, publication and freeze belong to engine/approval;
  # metadata to engine/readiness; frozen recovery projection to engine/session.
  # Put shared display behavior behind engine ports/adapters; no slice imports a
  # sibling and only cmd assembles adapters. Domains/ports gain no I/O imports or
  # new allowlist exception. Run shared contracts against I/O-free porttest fakes
  # and the canonical/disk adapters; feature runners import no production packages.
  # Any implementation finding that changes arc42 requires its accompanying ADR
  # and section update, per the charter. This spec follows its credential design.
  #
  # Fixture/step handoff. credential_approval_steps_test.go already registers the
  # private checkout/request, candidate, profile and read-only steps used by 01;
  # credential_approval_help_test.go registers 24's existing consent/host-agent
  # assertions. Reuse these definitions. The other credential-specific phrases,
  # including 24's complete disclosure assertion, still need definitions.
  # Reuse approval_steps_test.go, approval_fixture_test.go, approval_terminal_test.go
  # and slice-26 approval_network_steps_test.go for generic invocation, independent
  # records, differences, transcript timing and engine journals. Extend their oracle
  # with the contract above, never ask the product to manufacture expected records.
  # Do not reuse slice-29's credentials-only checkout unmodified: it intentionally
  # lacks ordinary readiness. The new fixture composes approvalCheckout's valid Git,
  # manifest, non-root UID and recording HTTP Engine API with slice-29's source
  # observer support in ONE private temporary home/state, never the operator's.
  # Sources claude-a, claude-b and github-a are distinct one-link mode-0600 files
  # under a mode-0700 directory outside checkout/protected state. Each holds only
  # "itos-synthetic-credential-" plus 32 random lowercase hex digits. Network
  # "allowlist" means exactly api.example.com and registry.example.org; no traffic
  # leaves the fixture. Default policy version is 3, with no inline credential bytes.
  # Grant request shorthand is only fixture data:
  # none -> omitted credentials; claude -> claude-token at claude-a;
  # github -> github-token at github-a; both -> both of those, written github first;
  # repointed claude -> claude-token at claude-b; swapped sources -> both kinds with
  # their source paths exchanged. Expected target always follows KIND, not filename.
  # "checkout alias" adds a hard link to the outside source inside the checkout;
  # location wins before the link-count rule. No test makes a host bind mount.
  # "prior and requested ... filenames" seeds generation 3 at one safe literal
  # path and writes a different current path containing the same specified controls.
  # Each record-defect fixture starts with an otherwise valid profile; the missing
  # proxy case first builds a valid allowlist-credentials-v1 record, then removes
  # only proxy. Malformed semantic records still have correct canonical hashes.
  # Given steps only write these inputs, seed independently canonical records or
  # schedule faults. They never invoke approve/doctor or assert product behavior.
  #
  # When steps retain early exits and all results for Then, even when a multi-action
  # sequence first refuses; PTYs wait for prompt OR process exit with a bounded
  # deadline. No assert or missing-result dereference in setup/action may mask red.
  # File changes at a prompt/engine barrier use explicit synchronization, not sleeps.
  # A distinct marker-positive control proves raw and all three base64-offset scans
  # work. Scan captured output/errors, all Engine API request/response bodies and
  # checkout/protected-state bytes; exclude only source files, positive control and
  # harness journals holding synthetic fixture data. Never print markers on failure.
  # Reuse slice-29's product-process data-access observer, armed only around product
  # invocations, with its positive control, to prove zero source data opens/reads.
  # Mode 0000 sources are a second control, not a substitute for observing attempts.
  # Baseline snapshots/watch setup must not themselves be counted as product reads.
  #
  # "read-only" compares checkout/source/state bytes, modes, owner, inode and mtime
  # after deliberate fixture edits; engine journal permits inspection only. Successful
  # consent may create/replace only its protected approval and existing lock metadata.
  # "no publication" checks BOTH approvals and snapshots, not just current.json.
  # This is a new assertion: the older approvalNoRecord helper does not check snapshots.
  # Recovery fixtures seed valid protected public/intent/snapshot/admission/binding
  # records directly, as recovery_state_steps_test.go does, using a stopped worker
  # durably absent and no controller. No run is needed to arrange a status case.
  # Contract-only cases invoke a test-only cmd assembly of the REAL preflight/freeze
  # services with raw observations and independently seeded data, no canned verdicts
  # or production gate override. Run the same store contract against fake and disk.
  # New admission-contract steps below extend that assembly with real doctor and
  # approve handlers, policy/manifest parsers, source validator, canonical codec
  # and consent service. Porttest fakes supply raw pins, paths, mount rows, engine
  # metadata and private store inputs; they never supply readiness or refusal
  # verdicts. Use the same public options and terminal semantics as the CLI.
  # Record calls and all command results even on early refusal. Each multi-run
  # case starts from independent missing-approval state, so a successful control
  # cannot supply authority or publication to a later refusal.
  # All unvaried relevant mount rows use ext4. Extend the slice-29 raw-observation
  # fixtures, whose default tmpfs is now refused; never relabel native tmpfs as
  # ext4 evidence, skip an unsupported filesystem or add a production override.
  # Native positive CLI fixtures need a named filesystem; test-only observation
  # assemblies establish the logic on other hosts, not native filesystem support.
  # Extend the shared fake/disk contracts for these cases. At implementation,
  # reconcile decision 0027's old read-only root-comparison exception and length
  # diagnostic note, and arc42's Credential source location, with this contract.
  #
  # Expected red is at each scenario's first Then, documented in the commit. The
  # implementation agent must commit missing/extended steps with @wip still present,
  # then obtain assertion-level red with @wip removed for the run before building.
  # This drafting commit has no executed-red or runtime proof claim. Every case is
  # local, with no Docker or real credential. Do not race the whole features package.
  # Neighbors: approval.feature (especially 36..59), credentials-policy.feature
  # (13/14 and metadata cases), policy/manifest/doctor, run-preflight, recovery,
  # control-state and slice-27's staged network refusals. Preserve the existing
  # shared codec/store/snapshot/source/recovery contracts and assembly checks.

  # Canonical requests, remembered identity and strict record decoding.
  @slice-30 @ID-CREDAPPROVAL-01 @local-engine-fixture
  Scenario Outline: Ordinary doctor exposes the complete credential candidate without consent
    Given a credential approval checkout with private synthetic sources
    And the credential approval request is "both" on "<network>"
    When itos-container runs with "doctor --json"
    Then doctor reports the independently calculated credential fingerprint and complete effective settings
    And its credential profile is "<profile>" with exactly the sorted grants and fixed targets
    And doctor reports approval "missing" and names "itos container approve"
    And it exits with code 0
    And credential inspection is read-only with no publication or source data access

    Examples:
      | network   | profile                  |
      | offline   | offline-credentials-v1   |
      | allowlist | allowlist-credentials-v1 |

  @slice-30 @ID-CREDAPPROVAL-02 @local-engine-fixture
  Scenario Outline: Grant order and source rotation do not change what was approved
    Given a credential approval checkout with private synthetic sources
    And the credential approval request is "both" on "offline"
    And a seeded matching approval at generation "4"
    When credential doctor inspects before and after "<edit>"
    Then both inspections report matching approval and identical independently calculated fingerprints
    And the active approval bytes and generation are unchanged
    And credential observations read no source data and leak no synthetic marker

    Examples:
      | edit                                                           |
      | reverse credential entries and reorder keys with YAML comments |
      | use equivalent JSON escapes in the source paths                |
      | rotate both source contents in place within the permitted size |
      | replace claude-a with a different valid private inode          |

  @slice-30 @ID-CREDAPPROVAL-03 @local-engine-fixture
  Scenario Outline: Any change to a credential grant invalidates the active approval
    Given a credential approval checkout with private synthetic sources
    And the credential approval request is "<prior>" on "offline"
    And a seeded matching approval at generation "4"
    And the credential approval request is "<current>" on "offline"
    When itos-container runs with "doctor --json"
    Then doctor reports the independently calculated credential fingerprint and complete effective settings
    And its approval status is "changed"
    And its approval differences are exactly "<pointers>" with the prior and current values
    And the active approval bytes and generation are unchanged
    And credential inspection is read-only with no publication or source data access

    Examples:
      | prior  | current           | pointers              |
      | none   | claude            | /credentials /profile |
      | claude | both              | /credentials          |
      | both   | github            | /credentials          |
      | claude | repointed claude  | /credentials          |
      | claude | github            | /credentials          |
      | both   | swapped sources   | /credentials          |
      | both   | none              | /credentials /profile |

  @slice-30 @ID-CREDAPPROVAL-04 @local-engine-fixture
  Scenario Outline: Moving a no-grant policy to version 3 changes only the declared version
    Given a credential approval checkout with private synthetic sources
    And the credential approval request is "none" on "<network>"
    And the credential approval policy version is <old>
    And a seeded matching approval at generation "2"
    And the credential approval policy version is 3
    When itos-container runs with "doctor --json"
    Then doctor reports the independently calculated credential fingerprint and complete effective settings
    And the no-grant profile is "<profile>" with no credentials member
    And its approval status is "changed"
    And its approval differences are exactly "/policy/version" with the prior and current values
    And the legacy approval bytes and generation are unchanged
    When terminal approval is attempted and doctor then inspects the version 3 no-grant record
    Then both results report matching no-grant approval at generation "3" with the independent fingerprint
    And the no-grant profile is "<profile>" with no credentials member

    Examples:
      | old | network   | profile      |
      | 1   | offline   | offline-v1   |
      | 2   | offline   | offline-v1   |
      | 2   | allowlist | allowlist-v1 |

  # Recompute hashes after each seeded semantic defect; a hash failure must not mask it.
  @slice-30 @ID-CREDAPPROVAL-15 @local-engine-fixture
  Scenario Outline: A canonical hash cannot legitimize an invalid stored grant
    Given a credential approval checkout with private synthetic sources
    And the credential approval request is "both" on "offline"
    And a seeded credential approval with defect "<defect>" and its recomputed fingerprint
    When itos-container runs with "doctor --json"
    Then it reports approval problem "approval.state.invalid" with exit 1
    And its approval status is "invalid"
    And the active approval bytes and generation are unchanged
    And credential inspection is read-only with no publication or source data access

    Examples:
      | defect                                             |
      | credential profile without credentials             |
      | credential profile with empty credentials          |
      | credential profile with null credentials           |
      | credentials under offline-v1                       |
      | credential profile at policy version 2             |
      | unknown credential kind                            |
      | duplicate credential kind                          |
      | grants out of kind order                           |
      | relative source path                               |
      | source containing a dot-dot component              |
      | target outside the reserved credential directory   |
      | duplicate target                                   |
      | extra value member containing synthetic fixture data |
      | allowlist credential profile without proxy         |

  @slice-30 @ID-CREDAPPROVAL-16 @local-engine-fixture
  Scenario: A valid historical target is a permission change rather than corrupt history
    Given a credential approval checkout with private synthetic sources
    And the credential approval request is "claude" on "offline"
    And a seeded credential approval at generation "3" with target "/run/itos-container/credentials/claude-token-v0"
    When itos-container runs with "doctor --json"
    Then doctor reports the independently calculated credential fingerprint and complete effective settings
    And its approval status is "changed"
    And its approval differences are exactly "/credentials" with the prior and current values
    And the active approval bytes and generation are unchanged

  # Review content is checked as complete ordered lines before input, not substrings
  # such as "token scope" that would also pass a false claim of restricted scope.
  @slice-30 @ID-CREDAPPROVAL-05 @local-engine-fixture
  Scenario Outline: A yes follows a complete review of the files and their limits
    Given a credential approval checkout with private synthetic sources
    And the credential approval request is "both" on "<network>"
    When bare approve runs at a terminal and receives "y" after the review
    Then the credential review shows complete settings and the exact profile restrictions before one y/N prompt
    And the credential review lists exactly the sorted kinds sources and fixed targets before consent
    And the credential review gives every disclosure for "<network>" in the specified order before consent
    And it exits with code 0
    And the sole active approval is the exact candidate at generation "1" in protected host state
    And credential consent writes only approval state and reads no source data
    And no engine mutation or image acquisition was attempted

    Examples:
      | network   |
      | offline   |
      | allowlist |

  @slice-30 @ID-CREDAPPROVAL-06 @local-engine-fixture
  Scenario Outline: Re-approval reviews the current grants and full differences before replacement
    Given a credential approval checkout with private synthetic sources
    And the credential approval request is "<prior>" on "offline"
    And a seeded matching approval at generation "7"
    And the credential approval request is "<current>" on "offline"
    When bare approve runs at a terminal and receives " YES " after the review
    Then the transcript shows differences "<pointers>" with old and new values before exactly one y/N prompt
    And the credential review describes exactly the current request including its applicable disclosure
    And the sole active approval is the exact candidate at generation "8" in protected host state
    And it exits with code 0
    And credential consent writes only approval state and reads no source data

    Examples:
      | prior  | current          | pointers              |
      | none   | claude           | /credentials /profile |
      | claude | both             | /credentials          |
      | both   | github           | /credentials          |
      | claude | repointed claude | /credentials          |
      | both   | none             | /credentials /profile |

  @slice-30 @ID-CREDAPPROVAL-09 @local-engine-fixture
  Scenario Outline: Remembered grants need no second prompt but remain visible
    Given a credential approval checkout with private synthetic sources
    And the credential approval request is "both" on "<network>"
    And a seeded matching approval at generation "4"
    When bare approve runs at a terminal with no answer available
    Then it reports matching approval without prompting and exits with code 0
    And the plain approval result lists exactly the current grants and complete credential disclosure
    And the active approval bytes and generation are unchanged
    And credential inspection is read-only with no publication or source data access

    Examples:
      | network   |
      | offline   |
      | allowlist |

  @slice-30 @ID-CREDAPPROVAL-23 @local-engine-fixture
  Scenario Outline: Literal source paths cannot alter the terminal review
    Given a credential approval checkout with private synthetic sources
    And the credential approval request is "claude" on "offline"
    And the prior and requested credential source filenames contain "<text>"
    When credential approval reviews the changed path at a terminal and doctor then inspects as JSON
    Then the credential review escapes the prior and current paths before consent and preserves their exact decoded JSON values
    And both commands succeed with the independently calculated current credential fingerprint
    And no shell-looking text is executed
    And credential observations read no source data and leak no synthetic marker

    Examples:
      | text                                         |
      | Unicode C1 and bidirectional controls         |
      | quotes dollar signs backticks and U+2028      |

  # q-52 must be exercised with OFFLINE credentials: the network-only predicate in
  # slice-26 cannot accidentally satisfy these rows. Fingerprints come from fixture
  # data, even if ordinary doctor cannot yet produce a candidate.
  @slice-30 @ID-CREDAPPROVAL-07 @local-engine-fixture
  Scenario Outline: No fingerprint option consents to a credential grant
    Given a credential approval checkout with private synthetic sources
    And the credential approval request is "claude" on "offline"
    And approval state is "<state>"
    When credential approve supplies fingerprint "<fingerprint>" with "<streams>" in "<mode>"
    Then it reports approval problem "approve.terminal-required" with exit 1
    And the credential refusal names terminal "itos container approve" without offering a fingerprint route
    And it never prompts or records approval
    And credential inspection is read-only with no publication or source data access

    Examples:
      | state    | fingerprint | streams        | mode                      |
      | missing  | exact       | redirected     | plain                     |
      | missing  | exact       | redirected     | --json                    |
      | matching | exact       | both terminals | plain                     |
      | missing  | mismatching | redirected     | --json                    |
      | missing  | malformed   | both terminals | plain                     |
      | matching | empty       | redirected     | inherited ITOS_JSON=1     |
      | missing  | exact       | redirected     | ITOS_JSON=1 with --no-json |

  @slice-30 @ID-CREDAPPROVAL-08 @local-engine-fixture
  Scenario Outline: Piped yes and JSON cannot replace terminal credential consent
    Given a credential approval checkout with private synthetic sources
    And the credential approval request is "github" on "offline"
    And approval state is "<state>"
    When approve runs at a terminal in "<mode>" with streams "<streams>"
    Then it reports approval problem "approve.confirmation-required" with exit 1
    And the credential refusal names terminal "itos container approve" without offering a fingerprint route
    And it never prompts or records approval
    And credential inspection is read-only with no publication or source data access

    Examples:
      | state    | mode                      | streams                      |
      | missing  | plain                     | redirected stdin             |
      | missing  | plain                     | redirected stdout            |
      | matching | plain                     | redirected stdin and stdout  |
      | missing  | --json                    | both terminals               |
      | matching | inherited ITOS_JSON=1     | both terminals               |
      | missing  | ITOS_JSON=1 with --no-json | redirected stdout            |

  @slice-30 @ID-CREDAPPROVAL-10 @local-engine-fixture
  Scenario Outline: Declining the reviewed credential grant writes nothing
    Given a credential approval checkout with private synthetic sources
    And the credential approval request is "claude" on "offline"
    When bare approve runs at a terminal and receives "<answer>" after the review
    Then it reports terminal approval refusal "approve.declined" with exit 1 after showing the review
    And credential inspection is read-only with no publication or source data access

    Examples:
      | answer |
      | n      |
      |        |
      | EOF    |

  @slice-30 @ID-CREDAPPROVAL-11 @local-engine-fixture
  Scenario Outline: Consent cannot follow changed grants or newly unsafe sources
    Given a credential approval checkout with private synthetic sources
    And the credential approval request is "claude" on "offline"
    When the credential terminal review is displayed then the fixture applies "<edit>" before answering yes
    Then it reports terminal approval refusal "approve.changed" with exit 1
    And the credential refusal publishes neither approval nor frozen input
    And the deliberately edited credential fixture is otherwise unchanged

    Examples:
      | edit                                |
      | add the github grant                |
      | remove the last grant               |
      | repoint claude to claude-b           |
      | remove claude-a                      |
      | make claude-a group-readable         |
      | replace claude-a with a symlink      |

  @slice-30 @ID-CREDAPPROVAL-25 @local-engine-fixture
  Scenario: A valid source rotation during review does not bind the old contents or inode
    Given a credential approval checkout with private synthetic sources
    And the credential approval request is "claude" on "offline"
    When the credential terminal review is displayed then the fixture applies "replace claude-a with a different valid private inode" before answering yes
    Then the sole active approval is the exact candidate at generation "1" in protected host state
    And it exits with code 0
    And credential observations read no source data and leak no synthetic marker

  @slice-30 @ID-CREDAPPROVAL-26 @local-engine-fixture
  Scenario: Removing the last grant restores the offline fingerprint route for the current request
    Given a credential approval checkout with private synthetic sources
    And the credential approval request is "both" on "offline"
    And a seeded matching approval at generation "3"
    And the credential approval request is "none" on "offline"
    And the credential approval policy version is 1
    When approve runs without a terminal with the independently calculated exact fingerprint and "--json"
    Then it reports matching approval for that fingerprint without a prompt and exits with code 0
    And the no-grant profile is "offline-v1" with no credentials member
    And the sole active approval is the exact candidate at generation "4" in protected host state
    And credential consent writes only approval state and reads no source data

  # Initial failures retain slice-29's rule/location and outrank consent. The first
  # written grant is github here, so claude's source pointer must remain index 1.
  @slice-30 @ID-CREDAPPROVAL-12 @local-engine-fixture
  Scenario Outline: Ordinary consumers revalidate sources even with a remembered grant
    Given a credential approval checkout with private synthetic sources
    And the credential approval request is "both" on "offline"
    And a seeded matching approval at generation "2"
    And credential source "claude-a" is changed to "<defect>"
    When the credential readiness command "<command>" is invoked
    Then credential readiness reports "<rule>" at "/credentials/1/source" with exit <exit> before consent
    And the requested grants remain visible without a usable approval candidate
    And the active approval bytes and generation are unchanged
    And credential inspection is read-only with no publication or source data access

    Examples:
      | defect         | command                         | rule                   | exit |
      | missing        | doctor --json                   | credential.missing     | 3    |
      | group-readable | doctor --json                   | credential.permissions | 1    |
      | missing        | bare terminal approve           | credential.missing     | 3    |
      | group-readable | approve --fingerprint=malformed --json | credential.permissions | 1 |
      | checkout alias | bare terminal approve           | credential.location    | 1    |

  # A stable observer can inspect tree B while the policy pin still holds tree A.
  # Keep both paths identical, each walk internally stable and all source checks
  # otherwise safe. Change only the named descriptor-identity component at the
  # named checkpoint; do not trigger a same-walk race or an unrelated layout fault.
  # The pin and its later checks continue to describe A. Raw observer nodes and
  # their rewalks describe B consistently, so only the cross-pin comparison catches
  # the mismatch. The matching control retains both components, including the
  # q-57 case where descriptor devices differ from mountinfo's device field.
  # Before implementation the ordinary consumers hit policy.version and the
  # credentials-only mismatch succeeds; neither satisfies the first Then.
  @slice-30 @ID-CREDAPPROVAL-27
  Scenario Outline: Credential admission binds the source walk to the checkout that supplied policy
    Given a credential admission observation fixture requesting "claude" with independent matching policy and source-checkout pins
    When the admission contract runs "<command>" with matching pins then with a "<component>" disagreement at "<checkpoint>"
    Then the mismatching admission reports "<rule>" with exit 1 and no usable approval candidate
    And the matching control succeeds with exactly the independently calculated grants
    And the disagreement journal identifies the policy pin and observed checkout node at the specified checkpoint
    And the mismatching admission publishes neither approval nor frozen input and reads no source data
    And the mismatching admission performs no engine access or consent after detecting the disagreement

    Examples:
      | command                          | component | checkpoint               | rule                    |
      | doctor --json                    | device    | before engine inspection | doctor.checkout.changed |
      | doctor --json                    | inode     | before engine inspection | doctor.checkout.changed |
      | doctor --json                    | device    | after engine inspection  | doctor.checkout.changed |
      | doctor --json                    | inode     | after engine inspection  | doctor.checkout.changed |
      | terminal approve with yes        | device    | before engine inspection | doctor.checkout.changed |
      | terminal approve with yes        | inode     | before engine inspection | doctor.checkout.changed |
      | terminal approve with yes        | device    | after engine inspection  | doctor.checkout.changed |
      | terminal approve with yes        | inode     | after engine inspection  | doctor.checkout.changed |
      | terminal approve with yes        | device    | after terminal consent   | approve.changed         |
      | terminal approve with yes        | inode     | after terminal consent   | approve.changed         |
      | doctor --credentials-only --json | device    | before source inspection | doctor.checkout.changed |
      | doctor --credentials-only --json | inode     | before source inspection | doctor.checkout.changed |

  # These are raw path/metadata contract inputs, not fabricated kernel readlink
  # results. Build clean absolute paths from components within NAME_MAX; count
  # bytes, not characters. A selected-state length includes /itos-container.
  # The 4095-byte control has all other valid readiness inputs; its child paths
  # are served through pinned capabilities. Each 4096-byte case must refuse from
  # the path text without needing that path to exist or a source to be accessible.
  # Call journals distinguish policy pinning from boundary/source observation.
  # For root refusal even policy pin/open must be absent; state refusal permits
  # the valid policy read but no credential boundary/source filesystem calls.
  # Existing credentials-only length refusal still fails the new diagnostic
  # assertion; ordinary consumers either hit the version gate or lack this bound.
  @slice-30 @ID-CREDAPPROVAL-28
  Scenario Outline: Admission names the 4095-byte limit before walking an overlong root or state path
    Given a credential admission observation fixture requesting "<request>" with independent matching policy and source-checkout pins
    When the admission contract runs "<command>" with the "<boundary>" path at 4095 bytes then at 4096 bytes
    Then the 4096-byte admission reports "<rule>" with exit 2 and both its message and fix name the boundary and the 4095-byte limit
    And the 4095-byte control succeeds with exactly the independently calculated grants
    And the overlong path is refused from its text before any boundary or source observation and before engine access or consent
    And the overlong-path call journal follows the specified root or state validation order
    And the 4096-byte admission publishes neither approval nor frozen input and reads no source data

    Examples:
      | command                          | request | boundary       | rule                    |
      | doctor --json                    | claude  | effective root | doctor.checkout.root    |
      | terminal approve with yes        | claude  | effective root | doctor.checkout.root    |
      | doctor --json                    | claude  | selected state | approval.state.location |
      | terminal approve with yes        | claude  | selected state | approval.state.location |
      | doctor --json                    | none    | effective root | doctor.checkout.root    |
      | terminal approve with yes        | none    | effective root | doctor.checkout.root    |
      | doctor --json                    | none    | selected state | approval.state.location |
      | terminal approve with yes        | none    | selected state | approval.state.location |
      | doctor --credentials-only --json | claude  | effective root | doctor.checkout.root    |
      | doctor --credentials-only --json | claude  | selected state | approval.state.location |

  # Live until slice-61's feat commit deletes it; ID-CREDAPPROVAL-31 to 35 replace it.
  # q-64 is settled, not a prompt to examine and extend another refusal list.
  # For each invocation replace just the relevant row's filesystem field; keep
  # descriptors, mount IDs, coordinates, ownership and source bytes unchanged.
  # Both mount observations are complete. "first" and "final" change only that
  # table, so a single disallowed observation must suffice; no mapping change is
  # an alternative reason to refuse. Covered and absent-state rows remain relevant.
  # Each case has ext4, btrfs and xfs positive controls and the named refusal.
  # Shared fake/disk contracts cross all placements with both observations and
  # add unrelated disallowed rows (accepted) plus mapping/location precedence.
  @slice-30 @ID-CREDAPPROVAL-29
  Scenario Outline: Credential admission accepts only the three named filesystems
    Given a credential admission observation fixture requesting "claude" with independent matching policy and source-checkout pins
    When the admission contract runs "<command>" with ext4 btrfs xfs then "<filesystem>" at "<placement>" in the "<observation>" mount observation
    Then only ext4 btrfs and xfs succeed and the other filesystem reports "credential.filesystem" at "/credentials/0/source" with exit 3
    And the filesystem refusal fix names ext4 btrfs and xfs for source checkout and state paths
    And each accepted control reports exactly the independently calculated grants
    And the filesystem refusal retains the requested grants without a usable approval candidate
    And the filesystem refusal publishes neither approval nor frozen input and reads no source data
    And the filesystem refusal precedes engine access and consent without enumerating state records

    Examples:
      | command                          | filesystem | placement                    | observation |
      | doctor --json                    | cifs       | source                       | first       |
      | terminal approve with yes        | smb3       | source ancestor              | final       |
      | doctor --json                    | ceph       | checkout root                | first       |
      | terminal approve with yes        | virtiofs   | checkout descendant          | final       |
      | doctor --json                    | ecryptfs   | covered checkout row         | first       |
      | terminal approve with yes        | zfs        | selected state root          | final       |
      | doctor --json                    | tmpfs      | fixed account state root     | first       |
      | terminal approve with yes        | ext3       | absent selected state root   | final       |
      | doctor --credentials-only --json | unlistedfs | covered fixed state row      | first       |
      | doctor --credentials-only --json | EXT4       | source                       | final       |
      | terminal approve with yes        | nfs        | source                       | first       |
      | doctor --json                    | nfs4       | selected state descendant    | final       |
      | terminal approve with yes        | 9p         | fixed state descendant       | first       |

  # ======================================================================
  # Slice-61: credential.filesystem is a warning in doctor and run status.
  #
  # Authority and the superseded text. The person's call of 2026-11-26 is
  # decision 0041
  # (docs/decisions/0041-record-credential-filesystem-type-as-a-warning-on-the-trusted-local-host.md:20),
  # and its standing line is docs/decisions/STANDING.md:126-128. It applies
  # the 2026-11-24 rule that a security control must stop a real outsider.
  # The local machine, its repositories and its accounts are trusted. A
  # filesystem type describes the person's own layout, and the approved
  # workload already receives the model credential. Decision 0041 replaces
  # the filesystem-type refusal of q-64 (tasks/asks.yaml) and of decision
  # 0027's 2026-11-10 04:42 UTC amendment with its NFS and 9p extensions.
  # Detection stays, and nothing else is reopened. STANDING.md already holds
  # the person's line, so slice-61 does not edit STANDING.md; every other
  # standing line, including STANDING.md:123-125 on credentials passed in
  # per command, stays in force.
  #
  # Which text governs. Until slice-61's feat commit, ID-CREDAPPROVAL-29 and
  # this file's header paragraph on filesystems describe main and stay live.
  # This block governs the implementation, and the feat commit makes the
  # supersessions listed below. These scenarios do not change and keep
  # passing byte for byte, because none of them reaches a detected row (rule
  # 1): ID-CREDAPPROVAL-01 to 28 and 30, and every ID-CREDENTIALS scenario
  # not listed below. Apart from ID-CREDENTIALS-46, every row they use is
  # ext4.
  #   ID-CREDENTIALS-46 stays unchanged and gives no warning in either half.
  #   Its first half's nsfs row at "/run/namespaces/unrelated" is not
  #   relevant. Its second half's nsfs row at "/state/private/namespace" is
  #   below the selected state, but its unprojectable root makes the boundary
  #   projection fail, so the observation detects nothing (rule 1) and the
  #   source still refuses with credential.unavailable and no
  #   credential_warnings member.
  #   ID-CREDAPPROVAL-30 is slice-30's scenario, already live on main, with
  #   defined steps (credentials_observation_steps_test.go). host.unsupported
  #   still returns before any pin, walk or mount table, so it carries no
  #   warning. It has no @local-engine-fixture tag on purpose: it is a
  #   raw-observation contract with no CLI or engine fixture.
  # After slice-61 it is no longer true that "nfs, nfs4 and 9p remain refused
  # outright": they warn like every other type outside the three.
  #
  # Rule 1, detection is unchanged. A row is relevant exactly as today, in
  # both mount observations of one source observation, in either of two ways.
  #   First, it is the row of a source or source-ancestor descriptor.
  #   Second, its mount point is at, below or covering the checkout root or
  #   either protected state root (selected or fixed account). Covered rows
  #   and an absent state root are included.
  # A relevant row is detected when its type is not exactly ext4, btrfs or
  # xfs. The match is byte-exact and case-sensitive, so EXT4, tmpfs, overlay,
  # fuse, fuse.*, fuseblk, fuseblk.*, nfs, nfs4, 9p, cifs, zfs and any other
  # text are all detected. Unrelated rows stay harmless.
  # Detection happens exactly where today's filesystem check runs: only in an
  # observation whose boundary projection succeeded. When the boundary
  # projection fails (the early return at the top of the readiness domain's
  # credentialScan.project), the observation detects no row and gives no
  # warning, whatever its mount tables hold, and its refusal is today's.
  #
  # Rule 2, no refusal. A detected row never refuses and never makes evidence
  # unavailable. Inspection continues with every other check unchanged. It
  # maps source and boundary descriptors through the detected row's device
  # and root like any other row. No problem ever carries
  # credential.filesystem, so its CredentialDiagnostic case goes. Source
  # precedence is now credential.location, then credential.mount_id, then
  # credential.unavailable, then regular, owner, permissions, links and
  # size, then duplicate. Boundary refusals (host.unsupported, checkout.*,
  # state.*) keep their order and return before warnings exist, so they carry
  # none. A source refusal after a successful boundary projection keeps the
  # warnings its observation found (ID-CREDAPPROVAL-33); a refusal from a
  # failed boundary projection has none (ID-CREDENTIALS-46). The consequence is the person's accepted risk. An
  # alias that overlay, FUSE, NFS or 9p hides from mountinfo and from
  # descriptor identities is accepted, with its warning (ID-CREDAPPROVAL-34
  # and 35).
  #
  # Rule 3, the warning. Each detected row gives the closed object
  # {"rule":"credential.filesystem","path":P,"filesystem":F}.
  #   P is the row's decoded mount point when that is valid UTF-8 holding no
  #   U+0000 to U+001F or U+007F and is at most 512 bytes long. A mount point
  #   that fails the first two conditions gives the text "(unprintable)",
  #   whatever its length. A printable mount point longer than 512 bytes gives
  #   the text "(too long)". The 512-byte cap is what keeps rule 6's record
  #   within the session parser's limit.
  #   F is the row's decoded type when that is 1 to 64 bytes, each byte from
  #   0x21 to 0x7E. Otherwise F is "(unprintable)".
  # Rows that give an equal (P, F) give one warning. This holds across both
  # mount observations and every relevance: a tmpfs row at "/" that covers
  # checkout, both states and a source ancestor is one warning. Warnings are
  # sorted by P's bytes, then F's bytes, and the first 8 are kept. The
  # number of the rest is the omitted count, from 1 to 32760, because two
  # tables of at most 16384 rows each hold at most 32768 distinct rows.
  # A command reports the warnings of its last completed source observation.
  # The shared inspector (readiness domain Inspect) observes sources twice:
  # once before Engine.Open and again after engine inspection. Ordinary
  # doctor and the run preflight both report the second observation, which
  # is the one view.Inspection.Credentials holds. Approve reports the last
  # observation it made, and credentials-only reports its only one.
  #
  # Rule 4, doctor and approve output. Readiness port gains the type
  # CredentialWarning {Rule, Path, Filesystem string} and a fixed-storage list
  # type: 8 CredentialWarning entries, a used-entry count and the omitted
  # count. Fixed storage keeps every type that holds the list comparable with
  # ==, as Binding.Credentials [2] already is. CredentialResult carries this
  # list beside Issues. The following holds only when
  # there is at least one warning; with none, every byte of today's output is
  # unchanged, and the credentials-only envelope keeps its 5 members.
  #  - doctor --json and doctor --credentials-only --json add the top-level
  #    member "credential_warnings" (the array). They add
  #    "credential_warnings_omitted" (a JSON integer) only when the count is
  #    not zero. ok, the exit code, inspection.ready, the candidate and its
  #    fingerprint are exactly those the same inputs give on ext4.
  #  - In plain and JSON modes, doctor, doctor --credentials-only and approve
  #    write one standard-error line per warning through cli.UI.Line, where
  #    Q is Go's strconv.Quote:
  #    itos-container: warning credential.filesystem <Q(P)> on <Q(F)>: credential exposure through filesystems other than ext4, btrfs and xfs is not checked
  #    When the omitted count is not zero, they then write:
  #    itos-container: warning credential.filesystem: <n> more filesystem warnings omitted
  #    Doctor writes these lines after its credential problem lines and before
  #    its approval problem lines and "Review with itos container approve".
  #    Ordinary doctor prints its credential problems through its approval
  #    readiness presentation (presentApprovalReadiness, with c.Presentation,
  #    in doctor/command), so that presentation and the plain readiness one
  #    build this order. Credentials-only writes them after its problem
  #    lines, in presentCredentials. Approve writes
  #    them after its result, outside its review, prompt, candidate and record.
  #    Standard output is unchanged in every mode.
  #  - Warnings are not settings. A filesystem change alone never changes a
  #    fingerprint or an approval status, and approval binds no warning.
  # The admission assembly (cmd/itos-container/credential_admission_contract_test.go)
  # builds doctor.CLI with JSON fixed at true today, so every doctor row runs
  # in JSON mode. The red step commit must change it to set doctor.CLI.JSON
  # from whether the row's argv contains "--json", so the plain doctor and
  # doctor --credentials-only rows reach the product's plain presentation.
  # The new step "the varied admission succeeds exactly like its ext4 control
  # apart from its credential warnings" then checks the mode of both runs: for
  # a doctor row without "--json" it fails when the trimmed standard output
  # of the control or the varied run is valid JSON (encoding/json Valid), and
  # for a row with "--json" it fails unless each is one JSON object.
  #
  # Rule 5, run records them. The run preflight's selection carries the
  # warnings as fresh execution evidence, like ProxyImageID. Approval port's
  # Durable gains one CredentialWarnings field of rule 4's fixed-storage
  # list type (approval port already imports readiness port). The run
  # slice's domain sets it in PreflightContext
  # (internal/slices/run/domain/preflight.go), after snapshot publication and
  # beside ProxyImageID, by copying view.Inspection.Credentials' list when
  # view.Inspection.Credentials is not nil. That list comes from the source
  # observation made after engine inspection (rule 3). Approval domain
  # Preflight still returns only its View and is unchanged.
  # The session store's first intent publication copies them into the
  # record's top-level "credential_warnings" and
  # "credential_warnings_omitted", with doctor JSON's shapes and presence
  # rules. That publication is Store.New or Store.Prepare, in both the
  # porttest memory store and the disk store, and the publication case
  # below (TestSessionStoreCredentialWarningPublication) proves it on both
  # against bytes the test reads itself. Record.ValidTransition keeps
  # both members equal for the record's whole life, including the schema-4
  # staging save in session domain Runner.prepareCredentials. A save that
  # adds, changes or drops them is refused. Public run on main still refuses
  # version 3 with policy.version before any observation, so
  # ID-CREDAPPROVAL-17 is unchanged. ID-CREDAPPROVAL-36 therefore proves this
  # rule through the admission assembly's real preflight and the real
  # session store. Whichever item lifts run's version-3 gate (slice-31) must
  # pass the preflight's Durable to the store unchanged; that alone records
  # the warnings.
  #
  # Rule 6, strict records. Session codec JSON.Decode accepts both members in
  # intent schemas 1 to 4, each present member adding one to the field count.
  # No schema number changes (decision 0037: v0, no compatibility promise).
  #  - credential_warnings is an array of 1 to 8 closed objects, each with
  #    exactly three string members: rule, path and filesystem.
  #  - rule is exactly "credential.filesystem".
  #  - path is "(unprintable)", "(too long)", or at most 512 bytes of valid
  #    UTF-8 holding no U+0000 to U+001F or U+007F, written as "/" or as "/"
  #    followed by nonempty components, none of them "." or "..".
  #  - filesystem is 1 to 64 bytes, each byte from 0x21 to 0x7E.
  #  - Entries are strictly ascending by path bytes, then filesystem bytes.
  #  - credential_warnings_omitted is a JSON integer from 1 to 32760, in
  #    decimal with no sign, fraction, exponent or leading zero. It is
  #    present only beside exactly 8 warnings.
  # Anything else is invalid.
  # Record size. No size limit changes. Decode refuses any record over 65536
  # bytes, and it parses through manifest.Parser, whose port MaxBytes is also
  # 65536. That parser is shared with manifests, so it is not touched. A
  # record carrying the warnings must therefore encode within 65536 bytes,
  # and rule 3's 512-byte path cap guarantees it. Encode uses json.Marshal,
  # which writes each "<", ">" or "&" as a six-byte \u escape, and that is
  # the worst expansion for these members. The worst case of each part is
  # then:
  #  - one entry, at most 40 + 6*512 + 16 + 6*64 + 2 = 3514 bytes;
  #  - eight entries with their separators, plus the member name and
  #    brackets, at most 28144 bytes;
  #  - the omitted member, at most 36 bytes more.
  # Both warning members together therefore encode to at most 28180 bytes.
  # Every other member the product writes is a hexadecimal ID, a fixed name,
  # a /proc descriptor path, a /dev/shm staging path, an engine endpoint that
  # connected, or a distribution/reference image reference, and all of these
  # stay far below the remaining 37356 bytes.
  # Parser budgets. manifest.Parser's jsonc adapter also refuses a document
  # deeper than 32, with more than 2048 nodes, or whose JSON-pointer paths
  # total more than 65536 bytes (internal/engine/manifest/jsonc/limits.go:11-15,
  # where every key, value and array element is a node and every key and
  # value is charged its full path). The warning members stay inside each:
  #  - depth: the deepest warning value sits at depth 4 (record, array,
  #    entry, string);
  #  - nodes: 1 key and 1 array, then per entry 1 object and 3 keys with 3
  #    values, so 2 + 8*7 = 58, plus 2 for the omitted member: 60 nodes;
  #  - path bytes: "/credential_warnings" charged as key and value (40), per
  #    entry 22 for "/credential_warnings/<i>" plus 54, 54 and 66 for its
  #    rule, path and filesystem keys and values (196, so 1568 for eight),
  #    and 56 for "/credential_warnings_omitted": 1664 bytes.
  # Size case. cmd/itos-container/session_store_test.go gains
  # TestSessionCodecCredentialWarningSize. It needs the real session codec,
  # sc.JSON{Approval: canonical.JSON{}, Parser: manifestjson.Parser{}} as
  # sessionStoreFixture composes it, because neither session porttest's fake
  # Codec nor the codec package's parser fake runs manifest.Parser's budgets,
  # and only cmd assembles adapters. It builds a schema-4
  # record with home, egress, credential staging and anchor at their longest
  # valid values, and with these two members at their longest valid values:
  #  - engine_endpoint is "unix://" followed by a 108-byte absolute path, "/"
  #    and then 107 "&" bytes. 108 bytes is Linux's sun_path size, which Go's
  #    unix connect refuses to exceed, and readiness records only an
  #    endpoint that connected. Each "&" encodes as a six-byte escape.
  #  - image_reference is a canonical name:tag@digest reference that
  #    distribution/reference accepts (manifest/reference Valid): a 255-byte
  #    name (its NameTotalLengthMax), a 128-byte tag and a sha256 digest.
  # It adds the eight longest warnings (ID-CREDAPPROVAL-37) and an omitted
  # count of 32760, and checks two things: Encode writes at most 65536
  # bytes, and Decode, which runs manifest.Parser's byte, depth, node and
  # path budgets, returns an equal record. If that case cannot pass, stop
  # and report; do not raise a limit. Because no record the
  # product writes can exceed the limit, the store gains no size refusal,
  # which could never be reached.
  # Publication case. The item's earlier disk-store test passed on a tree
  # without the implementation. StoreContract compares a reopened record only
  # with the record New returned, and CredentialRecoveryStoreContract starts
  # from a ready-made record and never calls New or Prepare, so a store that
  # drops the warnings passes both. Rule 5's store evidence is therefore
  # TestSessionStoreCredentialWarningPublication in
  # cmd/itos-container/session_store_test.go, with subtests "disk" and
  # "memory". Both use the real session codec composed as above, and one
  # frozen snapshot: a canonical approval record, built as sessionStoreFixture
  # builds its snapshot, whose candidate holds the one approved claude-token
  # grant at /vault/claude-a.
  #  - "disk" is sessionStoreFixture's real sd.Store on real protected
  #    storage, holding that snapshot. It reads the published bytes with
  #    os.ReadFile from <state>/itos-container/intents/<project>/<token>.json,
  #    a path the test computes from its own state root. The fixture's
  #    existing skip conditions stay.
  #  - "memory" is session porttest's Store with that codec, its Snapshots
  #    seeded with the same snapshot bytes and its UID, GID and Root set from
  #    the same fixture. It reads the published bytes from
  #    Records[port.Reference(project, token)].
  # Each subtest makes four publications, each with a fresh token: New and
  # Prepare with the warning Durable, then New and Prepare with the plain one.
  # The warning Durable is the fixture's Durable whose CredentialWarnings
  # holds exactly one entry {credential.filesystem, "/vault", "fuse.sshfs"}
  # with omitted count 0; the plain Durable has an empty list. The test
  # writes its expectation itself as the literal
  #    [{"rule":"credential.filesystem","path":"/vault","filesystem":"fuse.sshfs"}]
  # and never derives an expected value from what New or Prepare returned.
  # For each publication:
  #  1. New or Prepare succeeds, and the test closes the intent.
  #  2. The published bytes are one JSON object. For the warning Durable its
  #     "credential_warnings" member, decoded with encoding/json, equals the
  #     literal decoded the same way, and it has no
  #     "credential_warnings_omitted" member. For the plain Durable it has
  #     neither member.
  #  3. Open, then after Close OpenCleanup, of that reference each succeed
  #     and return a record whose warning list has exactly the test's own
  #     entries and omitted count: the one entry and 0, or no entry and 0.
  # Red and green. On a tree where Durable.CredentialWarnings, the record's
  # list field and the codec's two members exist but a store's New and
  # Prepare do not copy Durable's list into the record, that store's subtest
  # fails at check 2 for the warning Durable, because the published bytes
  # lack "credential_warnings". A codec that writes the members but does not
  # read them back fails check 3. Both subtests pass once each store's
  # publication copies the list and the codec writes and reads it.
  # Comparability and encoding. Record keeps == and != (porttest memory
  # store, ValidTransition). It holds the warnings in one field of rule 4's
  # fixed-storage list type (8 entries, the used-entry count and the omitted
  # count), tagged json:"-", which publication fills from Durable's list. No
  # port gains a MarshalJSON or UnmarshalJSON method and no port imports
  # encoding/json: the port allowlist in .golangci.yml has no encoding/json,
  # as internal/engine/session/disk/refusals.go:162-163 also notes. The
  # session codec, an adapter, writes and reads the two wire members itself:
  #  - JSON.Encode marshals a codec-local struct that embeds port.Record and
  #    adds "credential_warnings" (a slice of codec-local {rule, path,
  #    filesystem} entries, omitempty) and "credential_warnings_omitted" (an
  #    int, omitempty), filled from the field's used entries and omitted
  #    count. encoding/json promotes the embedded record's members first and
  #    writes the two members last, only when present.
  #  - JSON.Decode checks both members on manifest.Parser's value as rule 6
  #    says, unmarshals into port.Record as today (the json:"-" field stays
  #    zero), then unmarshals the same bytes into a codec-local struct holding
  #    only the two members and copies them into the record's field.
  # A record without warnings therefore encodes byte for byte as today.
  # Store publication and recovery both refuse warnings on a session whose frozen
  # snapshot holds no credential grant. That rule has a session port contract
  # case on fake and disk; status reports it as recovery.invalid-state.
  #
  # Rule 7, status. Recovery projects the validated record's warnings into
  # RecoveryEntry's JSON members "credential_warnings" and
  # "credential_warnings_omitted", with the same shapes and presence rules.
  # Plain status writes these lines after the session's credential grant lines:
  #    session <id> warning credential.filesystem <Q(P)> on <Q(F)>: credential exposure through filesystems other than ext4, btrfs and xfs was not checked
  # and, when the omitted count is not zero:
  #    session <id> warning credential.filesystem: <n> more filesystem warnings omitted
  # Status reads no current source, policy or approval to produce them.
  #
  # Rule 8, help. Doctor's help gains this line after its two --credentials-only
  # lines:
  #    "    Filesystems other than ext4, btrfs and xfs give credential.filesystem warnings, not refusals."
  # No exit code changes. Exit 3's help line never named the filesystem rule.
  #
  # Supersession at the feat commit, explained in its body:
  #  - Delete ID-CREDAPPROVAL-29, and the four step definitions only it uses
  #    ("only ext4 btrfs and xfs succeed ...", "the filesystem refusal fix
  #    names ...", "the filesystem refusal retains ...", "the filesystem
  #    refusal precedes ..."). Replace this file's header paragraph, from
  #    "The filesystem choice is settled by q-64" to "prerequisite to ask.",
  #    with a pointer to this block. In the fixture handoff, "whose default
  #    tmpfs is now refused" becomes "whose default tmpfs now warns".
  #  - In credentials-policy.feature, delete ID-CREDENTIALS-51, 54, 60 and 61.
  #    ID-CREDAPPROVAL-31, 34 and 35 replace them, and no number is reused.
  #    Replace its header lines on overlay, FUSE, NFS and 9p (from "The
  #    coordinator's 2026-11-10 04:42 UTC decision" to "instead is the person's
  #    open decision, recorded in decision 0027.") and its "credential.filesystem
  #    (3)" rule lines with a pointer to this block.
  #  - Turn the refusal cases of readiness porttest
  #    credential_stacking_contract.go and credential_export_contract.go into
  #    warning cases, for both fake and disk. Add rule 6's size case and
  #    rule 5's publication case to cmd/itos-container/session_store_test.go,
  #    as named above; StoreContract and CredentialRecoveryStoreContract stay
  #    as they are.
  #  - Leave docs/decisions/STANDING.md unchanged. Its line at 126-128 is
  #    the person's call (decision 0041) and already states this rule.
  #  - Insert this paragraph into decision 0027, directly after the q-64
  #    section that ends "...credential-delivery support claim.":
  #    "Amended by the person, **2026-11-26**, in
  #    [decision 0041](0041-record-credential-filesystem-type-as-a-warning-on-the-trusted-local-host.md)
  #    and built by slice-61, applying the rule of
  #    2026-11-24 that a security control must stop a real outsider: the local
  #    machine, its repositories and its accounts are trusted, a filesystem
  #    type describes the person's own layout, and the approved workload
  #    already receives the credential. A filesystem type outside ext4, btrfs
  #    and xfs therefore no longer refuses. This supersedes the refusal in the
  #    2026-11-10 04:42 UTC amendment, in the amendments after the reviews of
  #    `8e5088e` and `2ac05a4`, and in q-64; their detection stays. In both
  #    mount observations the shared observer still detects every other type
  #    on source and ancestor descriptors and on rows at, below or covering the
  #    checkout or either protected state root, covered rows and absent state
  #    included, and reports each as a `credential.filesystem` warning naming
  #    the row's mount point and type. Inspection continues with every other
  #    check, so an alias such a filesystem hides is accepted with its warning;
  #    every other source or boundary refusal still applies. Doctor and
  #    approve print the warnings, approval binds none, and run records them
  #    in the session intent record for status. Their exact shape is in
  #    slice-61's block of
  #    [credentials-approval.feature](../../features/credentials-approval.feature)."
  #  - In decision 0029, append "The person's
  #    [decision 0041](0041-record-credential-filesystem-type-as-a-warning-on-the-trusted-local-host.md)
  #    (2026-11-26), built by slice-61, makes that allowlist's refusal a
  #    `credential.filesystem` warning; see decision 0027's amendment of that
  #    date." to its first paragraph. Replace its paragraph beginning "The
  #    source observer accepts only ext4, btrfs and xfs" with: "The source
  #    observer detects every type other than ext4, btrfs and xfs in both
  #    mount observations, under decision 0027's source/ancestor and
  #    checkout/state relevance conditions, and since slice-61 reports each as
  #    a `credential.filesystem` warning, not a refusal. Unrelated rows stay
  #    harmless; missing mapping and known location precedence stay in force.
  #    Synthetic observations exercise the complete matrix; they do not relabel
  #    a native unsupported filesystem or establish native bind-mount support."
  #  - Make docs/CONFIG.md's overlay/FUSE/NFS/9p paragraph and arc42's
  #    "Credential source location" section say what rules 1 to 3 say.
  #    docs/CLI.md gives rule 4's JSON members and lines, rule 7's status
  #    members and lines, and rule 8's help line. docs/security.md discloses
  #    the accepted risk in rule 2. The 0027 amendment names the arc42 section
  #    it changes, as the charter requires.
  #
  # Architecture. Detection and shaping belong to engine/readiness's domain,
  # and the list type to its port. Durable's field belongs to
  # engine/approval's port, and the run slice's domain fills it. The record, codec,
  # recovery projection and presentation to engine/session. Doctor and
  # approve stay thin and print through cli.UI. No slice imports a sibling,
  # only cmd assembles adapters, and domains and ports gain no I/O import. No
  # dependency is added.
  #
  # Fixtures and steps. Reuse these existing steps:
  #  - "a credential admission observation fixture requesting ..."
  #  - the ext4 btrfs xfs When of ID-CREDAPPROVAL-29
  #  - "each accepted control reports exactly the independently calculated grants"
  #  - "credential observation exposes the source through ..."
  #  - "the credential source is a second ... mount ..."
  #  - the ID-CREDAPPROVAL-21 and 22 status steps
  #  - "itos-container runs with", "it exits with code" and "its standard output says"
  # The admission fixture (credentialAdmissionNodes) has the checkout at
  # /checkout, selected state at /state, fixed account state at
  # /account/.local/state/itos-container and sources under /vault. Row 1 is
  # "/" on ext4. A placement adds row 2 with the varied type in the named
  # observation, and a covered placement adds an ext4 row 3 over it at the
  # same mount point. The expected warning path for a placement is row 2's
  # mount point, as in each table. Every other step below is new. "The varied
  # admission" means the last run of a When, and "its controls" means the
  # earlier runs of that When. Givens only write fixture inputs. Never ask
  # the product for an expected value; compute it independently.
  #
  # Red before the work: each scenario's first Then fails at its behaviour
  # assertion, because today the varied input refuses with
  # credential.filesystem, exit 3, or the record holding warnings is invalid.
  # ID-CREDAPPROVAL-33's location rows are the exception: they reach their
  # refusal today and fail at the warning assertion. Commit the step
  # definitions red first with @wip in place; for the evidence run, remove
  # @wip locally, never in a commit. Every case is local, with no engine
  # and no real credential. The implementer may commit freely in its own
  # workspace; the host makes the landing commits.

  # The varied run must match its ext4 control in exit code 0, in the
  # independently calculated candidate and fingerprint (or credentials-only
  # grants), in its standard output, and in its journal counts for engine
  # opens, prompts, approval writes, frozen publications and source data
  # reads. JSON objects are compared after removing the two warning members.
  # Its standard error is the control's plus rule 4's lines, placed as rule 4
  # says. This replaces ID-CREDAPPROVAL-29's matrix row for row, with plain rows added.
  @wip @slice-61 @ID-CREDAPPROVAL-31
  Scenario Outline: A credential filesystem outside ext4, btrfs and xfs warns and inspection continues
    Given a credential admission observation fixture requesting "claude" with independent matching policy and source-checkout pins
    When the admission contract runs "<command>" with ext4 btrfs xfs then "<filesystem>" at "<placement>" in the "<observation>" mount observation
    Then the varied admission succeeds exactly like its ext4 control apart from its credential warnings
    And the varied admission reports exactly the credential warnings "<path> on <filesystem>" with omitted count 0
    And the varied admission reports no credential.filesystem problem
    And each accepted control reports exactly the independently calculated grants
    And the accepted controls report no credential warning
    And the varied admission reads no source data and enumerates no state record

    Examples:
      | command                          | filesystem   | placement                  | observation | path                                         |
      | doctor --json                    | cifs         | source                     | first       | /vault/claude-a                              |
      | terminal approve with yes        | smb3         | source ancestor            | final       | /vault                                       |
      | doctor --json                    | ceph         | checkout root              | first       | /checkout                                    |
      | terminal approve with yes        | virtiofs     | checkout descendant        | final       | /checkout/subtree                            |
      | doctor --json                    | ecryptfs     | covered checkout row       | first       | /checkout                                    |
      | terminal approve with yes        | zfs          | selected state root        | final       | /state                                       |
      | doctor --json                    | tmpfs        | fixed account state root   | first       | /account/.local/state/itos-container         |
      | terminal approve with yes        | ext3         | absent selected state root | final       | /state                                       |
      | doctor --credentials-only --json | unlistedfs   | covered fixed state row    | first       | /account/.local/state/itos-container         |
      | doctor --credentials-only --json | EXT4         | source                     | final       | /vault/claude-a                              |
      | terminal approve with yes        | nfs          | source                     | first       | /vault/claude-a                              |
      | doctor --json                    | nfs4         | selected state descendant  | final       | /state/subtree                               |
      | terminal approve with yes        | 9p           | fixed state descendant     | first       | /account/.local/state/itos-container/subtree |
      | doctor                           | overlay      | checkout descendant        | first       | /checkout/subtree                            |
      | doctor                           | fuse.sshfs   | source ancestor            | final       | /vault                                       |
      | doctor --credentials-only --json | fuseblk.ntfs | selected state root        | first       | /state                                       |
      | doctor --json                    | fuse         | source ancestor            | first       | /vault                                       |

  # Layouts. Each starts from the all-ext4 admission fixture, whose tables
  # hold "1 0 0:1 / / rw - ext4 disk rw". The rows below are given as
  # mountinfo text and go into both tables unless a layout names one table.
  # Every added row has its own mount ID and device from 0:2 upward, root
  # "/", and parent 1 unless stated. Mount points are written in mountinfo's
  # escaped form, because the fixture's Mounts strings pass through
  # json.Marshal, and the shared parser (moby/sys/mountinfo v0.7.2) decodes
  # octal escapes in both the mount point and the type. "Nodes move" means
  # that the nodes at the mount point and below it take that row's mount
  # ID, as credentialAdmissionFilesystem does for a placement. Every other
  # node stays on mount ID 1.
  #  - "tmpfs at / beside the ext4 root". Row 1 becomes two rows, in this
  #    order: "2 0 0:2 / / rw - tmpfs tmpfs rw" and then
  #    "1 2 0:1 / / rw - ext4 disk rw". The ext4 row covers the tmpfs row, and
  #    every node stays on the visible mount ID 1.
  #  - "fuse.sshfs at the source ancestor and overlay below the checkout".
  #    Two rows are added: "2 1 0:2 / /vault rw - fuse.sshfs sshfs rw", with
  #    nodes moving to it, and the node-free
  #    "3 1 0:3 / /checkout/subtree rw - overlay overlay rw".
  #  - "overlay on nine mounts below the checkout". For k from 1 to 9, the
  #    node-free row "<k+1> 1 0:<k+1> / /checkout/m<k> rw - overlay overlay rw".
  #  - "a 65-byte type at the checkout root".
  #    "2 1 0:2 / /checkout rw - <type> disk rw", where the type is "x"
  #    repeated 65 times. Nodes move.
  #  - "a type holding an escaped space at the checkout root". The same row
  #    with the type field written fuse\040x, which decodes to "fuse x".
  #    Nodes move.
  #  - "a mount point holding byte 0xff below the checkout". The node-free row
  #    "2 1 0:2 / /checkout/\377 rw - overlay overlay rw". Its mount point is
  #    written as a backslash and the three digits 377, which decodes to
  #    "/checkout/" followed by byte 0xff.
  #  - "overlay then fuse at the checkout root".
  #    "2 1 0:2 / /checkout rw - overlay overlay rw" in the first table; the
  #    same row with type fuse in the final table. Nodes move.
  #  - "overlay at the checkout root in both observations". The overlay row
  #    just described, in both tables. Nodes move.
  #  - "a 512-byte mount point below the checkout". The node-free row
  #    "2 1 0:2 / {512-byte point} rw - overlay overlay rw". {512-byte point}
  #    is "/checkout/", then 200 "a", "/", 200 "a", "/" and 100 "a": exactly
  #    512 bytes, the longest P rule 3 keeps.
  #  - "a 513-byte mount point below the checkout". The same row, with a last
  #    component of 101 "a": 513 bytes.
  # A cell lists the expected warnings as "P on F", joined by "; ", in order.
  # {512-byte point} in a cell stands for the path defined above.
  @wip @slice-61 @ID-CREDAPPROVAL-32
  Scenario Outline: Credential filesystem warnings are deduplicated, sorted, capped and printable
    Given a credential admission observation fixture requesting "claude" with independent matching policy and source-checkout pins
    When the admission contract runs "<command>" with the all-ext4 control then with filesystem layout "<layout>"
    Then the varied admission succeeds exactly like its ext4 control apart from its credential warnings
    And the varied admission reports exactly the credential warnings "<warnings>" with omitted count <omitted>
    And the varied admission reports no credential.filesystem problem

    Examples:
      | command                          | layout                                                         | warnings                                                                                                                                                                 | omitted |
      | doctor --json                    | tmpfs at / beside the ext4 root                                | / on tmpfs                                                                                                                                                               | 0       |
      | doctor                           | fuse.sshfs at the source ancestor and overlay below the checkout | /checkout/subtree on overlay; /vault on fuse.sshfs                                                                                                                      | 0       |
      | doctor --credentials-only --json | overlay on nine mounts below the checkout                      | /checkout/m1 on overlay; /checkout/m2 on overlay; /checkout/m3 on overlay; /checkout/m4 on overlay; /checkout/m5 on overlay; /checkout/m6 on overlay; /checkout/m7 on overlay; /checkout/m8 on overlay | 1       |
      | doctor                           | overlay on nine mounts below the checkout                      | /checkout/m1 on overlay; /checkout/m2 on overlay; /checkout/m3 on overlay; /checkout/m4 on overlay; /checkout/m5 on overlay; /checkout/m6 on overlay; /checkout/m7 on overlay; /checkout/m8 on overlay | 1       |
      | doctor --json                    | a 65-byte type at the checkout root                            | /checkout on (unprintable)                                                                                                                                               | 0       |
      | doctor --credentials-only        | a type holding an escaped space at the checkout root           | /checkout on (unprintable)                                                                                                                                               | 0       |
      | doctor --json                    | a mount point holding byte 0xff below the checkout             | (unprintable) on overlay                                                                                                                                                 | 0       |
      | doctor --credentials-only --json | overlay then fuse at the checkout root                         | /checkout on fuse; /checkout on overlay                                                                                                                                  | 0       |
      | doctor                           | overlay at the checkout root in both observations              | /checkout on overlay                                                                                                                                                     | 0       |
      | doctor --json                    | a 512-byte mount point below the checkout                      | {512-byte point} on overlay                                                                                                                                              | 0       |
      | doctor                           | a 513-byte mount point below the checkout                      | (too long) on overlay                                                                                                                                                    | 0       |

  # The defect is applied to the claude-a node after the overlay placement:
  #  - "group-readable mode 0640": mode 0100640.
  #  - "two links": link count 2.
  #  - "owner UID 1001": UID 1001.
  #  - "an alias inside the checkout": a node /checkout/alias with claude-a's
  #    device and inode, one link, mode 0100600 and UID 1000.
  # Before the work, the permissions, links and owner rows refuse with
  # credential.filesystem and fail the first Then. The location rows already
  # report credential.location and fail at the warning assertion.
  @wip @slice-61 @ID-CREDAPPROVAL-33
  Scenario Outline: An independent source refusal still applies beside a filesystem warning
    Given a credential admission observation fixture requesting "claude" with independent matching policy and source-checkout pins
    When the admission contract runs "<command>" with "overlay" at "source ancestor" in the "first" mount observation and claude-a "<defect>"
    Then the varied admission reports "<rule>" at "/credentials/0/source" with exit <exit> and no usable approval candidate
    And the varied admission reports exactly the credential warnings "/vault on overlay" with omitted count 0
    And the varied admission reports no credential.filesystem problem
    And the refused admission publishes neither approval nor frozen input and reads no source data

    Examples:
      | command                          | defect                       | rule                   | exit |
      | doctor --json                    | group-readable mode 0640     | credential.permissions | 1    |
      | terminal approve with yes        | group-readable mode 0640     | credential.permissions | 1    |
      | terminal approve with yes        | two links                    | credential.links       | 1    |
      | doctor --credentials-only --json | owner UID 1001               | credential.owner       | 1    |
      | doctor --json                    | an alias inside the checkout | credential.location    | 1    |
      | terminal approve with yes        | an alias inside the checkout | credential.location    | 1    |

  # ID-CREDENTIALS-51 and 54's layouts, now accepted with their warning. In
  # each, overlay or FUSE shows the source's inode under another device, so
  # no identity or coordinate places it in a protected tree (rule 2's
  # accepted risk). The source is /checkout-sources/token, and the fixture's
  # "/" row is ext4. If a layout instead meets another refusal, stop and
  # report it; do not change the fixture.
  @wip @slice-61 @ID-CREDAPPROVAL-34 @linux
  Scenario Outline: Stacking filesystems that hide a source alias are accepted with a warning
    Given a credential observation fixture for "protected state" with "distinct outside file"
    And credential observation exposes the source through "<filesystem>" at "<point>"
    When the credential observation contract runs in JSON mode
    Then it exits with code 0
    And its credential JSON reports exactly the claude-token grant at "token", no problems and the credential warnings "<point> on <filesystem>" with omitted count 0
    And its error output says "warning credential.filesystem"
    And the credential observation recorded no state record enumeration or metadata visits

    Examples:
      | filesystem   | point                                |
      | overlay      | /checkout                            |
      | fuse.bindfs  | /checkout-sources                    |
      | fuseblk.ntfs | /state/private                       |
      | fuse         | /account/.local/state/itos-container |

  # ID-CREDENTIALS-60 and 61's layouts, now accepted with their warnings. The
  # export's row at <point> is relevant to a protected root, and its second
  # mount at /checkout-sources is the source's ancestor row. The source's
  # second-mount coordinate and identity never fall inside a protected tree.
  # Stop and report if another refusal occurs.
  @wip @slice-61 @ID-CREDAPPROVAL-35 @linux
  Scenario Outline: NFS and 9p exports that hide a source alias are accepted with warnings
    Given a credential observation fixture for "protected state" with "distinct outside file"
    And the credential source is a second "<filesystem>" mount of "<subdirectory>" from the export mounted at "<point>"
    When the credential observation contract runs in JSON mode
    Then it exits with code 0
    And its credential JSON reports exactly the claude-token grant at "token", no problems and the credential warnings "<warnings>" with omitted count 0
    And the credential observation recorded no state record enumeration or metadata visits

    Examples:
      | filesystem | subdirectory                                  | point    | warnings                                          |
      | nfs4       | /checkout/private                             | /        | / on nfs4; /checkout-sources on nfs4              |
      | nfs        | /state/private                                | /        | / on nfs; /checkout-sources on nfs                |
      | nfs4       | /account/.local/state/itos-container/sessions | /account | /account on nfs4; /checkout-sources on nfs4       |
      | 9p         | /checkout/private                             | /        | / on 9p; /checkout-sources on 9p                  |
      | 9p         | /account/.local/state/itos-container/sessions | /account | /account on 9p; /checkout-sources on 9p           |

  # The API fixture pauses at image inspection after the first metadata check.
  # A premature policy gate must reach Then as a failed readiness assertion.
  @slice-30 @ID-CREDAPPROVAL-13 @local-engine-fixture
  Scenario Outline: A source becoming unsafe during engine inspection cannot become a candidate
    Given a credential approval checkout with private synthetic sources
    And the credential approval request is "claude" on "offline"
    And the fixture will make claude-a group-readable while image inspection is paused
    When the credential readiness command "<command>" is invoked
    Then credential readiness reports "credential.permissions" at "/credentials/0/source" with exit 1 before consent
    And the requested grants remain visible without a usable approval candidate
    And the credential refusal publishes neither approval nor frozen input
    And no engine mutation or image acquisition was attempted

    Examples:
      | command               |
      | doctor --json         |
      | bare terminal approve |

  @slice-30 @ID-CREDAPPROVAL-14 @local-engine-fixture
  Scenario Outline: Approval observes metadata without opening or copying credential contents
    Given a credential approval checkout with private synthetic sources
    And the credential approval request is "both" on "offline"
    And both credential sources have mode "0000"
    And the credential data-access observer and marker scanner have positive controls
    When the credential readiness command "<command>" is invoked
    Then credential readiness succeeds with the complete independently calculated candidate
    And the product opens and reads no credential source data
    And no synthetic marker occurs in command output engine traffic checkout or protected state
    And only the command's permitted approval publication occurs

    Examples:
      | command                             |
      | doctor --json                       |
      | terminal approve with yes           |
      | terminal approve with inherited JSON overridden by --no-json and yes |

  # Public run stays closed; shared comparison/freeze proof does not authorize it.
  @slice-30 @ID-CREDAPPROVAL-17 @local-engine-fixture
  Scenario Outline: An approved credential profile still cannot deliver anything in this increment
    Given a credential approval checkout with private synthetic sources
    And the credential approval request is "<request>" on "<network>"
    When terminal credential approval is attempted then the manifest is removed and run is invoked with literal argv
    Then the terminal attempt records the exact credential candidate at generation "1"
    And the subsequent run reports "policy.version" at "/version" with exit 2
    And that run opens no manifest source data engine connection or protected state
    And that run publishes no snapshot intent session or staged credential material
    And the approved record and all existing fixture resources remain unchanged by run

    Examples:
      | request | network   |
      | claude  | offline   |
      | github  | offline   |
      | both    | allowlist |

  @slice-30 @ID-CREDAPPROVAL-18 @local-engine-fixture
  Scenario: Removing all grants does not let run silently use an older credential approval
    Given a credential approval checkout with private synthetic sources
    And the credential approval request is "both" on "offline"
    And a seeded matching approval at generation "5"
    And the credential approval request is "none" on "offline"
    And the credential approval policy version is 1
    When itos-container runs with "run -- credential-fixture-never-executed"
    Then run refuses approval "changed" with exit 1 and names "itos container approve"
    And the run change diagnostic lists exactly "/credentials /policy/version /profile"
    And run makes no engine connection and publishes no approval snapshot or session
    And the active approval bytes and generation are unchanged

  @slice-30 @ID-CREDAPPROVAL-19
  Scenario Outline: Shared run preflight explains every grant change before engine inspection
    Given a credential preflight contract with independently approved "<prior>" and requested "<current>" grants
    When the real local preflight and run refusal presentation consume the fixture observations
    Then the contract refuses changed approval with exit 1 naming "itos container approve" and exactly "<pointers>"
    And the contract makes no engine observation approval write or frozen publication

    Examples:
      | prior  | current          | pointers              |
      | none   | claude           | /credentials /profile |
      | claude | both             | /credentials          |
      | both   | github           | /credentials          |
      | claude | repointed claude | /credentials          |
      | claude | github           | /credentials          |
      | both   | none             | /credentials /profile |

  @slice-30 @ID-CREDAPPROVAL-20
  Scenario: Frozen grants survive later request and approval replacement unchanged
    Given a credential freeze contract with independently seeded matching grants at generation "4"
    When the real service selects and publishes frozen input then the fixture replaces the request and active approval
    Then the selected and persisted snapshot contain the exact original canonical settings bytes fingerprint project and generation
    And changing caller data or returned values cannot alter the stored or independently returned grants
    And a new selection using generation "4" refuses as superseded without another snapshot
    And the credential freeze contract reads no source data and creates no worker or staged material

  # Slice-61 rule 5, through the admission assembly's "preflight" route: the
  # real run domain Preflight over the shared readiness, as for ID-19, with
  # the approved record equal to the request. For the first ext4 run and then
  # the varied run, the When does the following. It hands the returned Durable
  # to Store.New of session porttest's memory Store, which uses the real
  # session codec.JSON and is seeded with the snapshot that Preflight
  # published. It keeps the first published record bytes. It then saves the
  # store's next valid transition (intent to owned, with a full container ID).
  # Last, it attempts three saves: one adding a warning to the ext4 record,
  # one changing the varied record's filesystem text, and one dropping its
  # warnings. The disk store's publication is proven by
  # TestSessionStoreCredentialWarningPublication, and its saves use the same
  # Record.ValidTransition (see the slice-61 block). Public run is untouched. Preflight
  # becomes ready only after Engine.Open, so each successful run opens the
  # fake engine once; the last Then compares the assembly's engine.Opens for
  # the varied run with the ext4 run's.
  @wip @slice-61 @ID-CREDAPPROVAL-36
  Scenario Outline: Run's preflight carries filesystem warnings into the session intent record it publishes
    Given a credential admission observation fixture requesting "claude" with independent matching policy and source-checkout pins
    When the admission contract runs the real local preflight for the approved claude grant with ext4 then "<filesystem>" at "<placement>" in the "<observation>" mount observation and publishes each first session intent
    Then the varied preflight succeeds and its selection carries exactly the credential warnings "<path> on <filesystem>" with omitted count 0
    And the varied first intent record carries exactly those credential warnings and no omitted count member
    And the ext4 first intent record has neither credential warning member
    And the next valid save keeps the recorded warnings byte for byte while saves that add change or drop them are refused
    And the preflight contract reads no source data and opens the engine exactly as often as its ext4 control

    Examples:
      | filesystem | placement              | observation | path                                         |
      | overlay    | source                 | first       | /vault/claude-a                              |
      | fuse.sshfs | checkout descendant    | final       | /checkout/subtree                            |
      | nfs4       | selected state root    | first       | /state                                       |
      | 9p         | fixed state descendant | final       | /account/.local/state/itos-container/subtree |

  # Status reads protected historical authority. Seeded absence avoids depending
  # on a delivered worker or on T-40's network implementation.
  @slice-30 @ID-CREDAPPROVAL-21 @local-engine-fixture
  Scenario Outline: Status shows frozen grants even after requests approvals and sources disappear
    Given an independently seeded stopped credential session on "<network>" with both frozen grants
    And the current request is invalid the active approval is removed and both original sources are absent
    When credential session status is invoked in plain and JSON modes
    Then both status results succeed with the original fingerprint and exactly the frozen grants
    And plain credential status shows the frozen disclosure without claiming new delivery is available
    And status neither reads mutable requests approvals or source data nor changes protected records

    Examples:
      | network   |
      | offline   |
      | allowlist |

  # First prove a valid credential snapshot is readable. An always-invalid decoder
  # must not pass the refusal half before this work exists.
  # Rebuild the protected fingerprint references after the semantic edit, so an
  # unrelated intent/snapshot fingerprint mismatch cannot satisfy the second Then.
  @slice-30 @ID-CREDAPPROVAL-22 @local-engine-fixture
  Scenario: A current grant cannot fill in an invalid frozen snapshot
    Given an independently seeded stopped credential session on "offline" with both frozen grants
    And the current request and active approval name different valid grants
    When status is invoked then the fixture removes the frozen credentials member recomputes its hash and invokes status again
    Then the first status succeeds with exactly the original frozen grants
    And the second status reports "recovery.invalid-state" with exit 2 and no session entry
    And neither status reads current grants or repairs protected records

  # Slice-61 rules 6 and 7. The new Given adds the members to the seeded
  # intent record directly, as run would have published them. The record's
  # schema stays the seeded one. A cell lists "P on F" warnings joined by
  # "; ", and omitted count 0 means no omitted member. Paths need not match
  # the sources, because status never consults current paths. The Given
  # re-captures the protected tree (recoveryCapture) after its edit, so the
  # reused last Then compares against the seeded record as it stands. The
  # Given writes the members as json.Marshal does, so "<", ">" and "&" become
  # six-byte \u escapes, as in a record that run published.
  # Two named cells cover the size bound in rule 6:
  #  - "{eight longest}" is eight warnings. Warning k, for k from 1 to 8, has
  #    path "/" followed by 510 "<" and the digit k (512 bytes) and
  #    filesystem "<" repeated 64 times. With omitted count 32760, this is
  #    the largest warning content rule 6 allows, at its worst escaping.
  #  - "{eight longest} in a 65536-byte record" is the same warnings. The
  #    Given then inserts spaces before the record's final "}" until the file
  #    is exactly 65536 bytes, which is the largest record Decode accepts.
  # Before the work the decoder refuses the extra members, so status reports
  # recovery.invalid-state and the first Then fails.
  @wip @slice-61 @ID-CREDAPPROVAL-37 @local-engine-fixture
  Scenario Outline: Status shows the filesystem warnings a credential session recorded
    Given an independently seeded stopped credential session on "<network>" with both frozen grants
    And the seeded intent record also carries the credential warnings "<warnings>" with omitted count <omitted>
    When credential session status is invoked in plain and JSON modes
    Then both status results succeed with the original fingerprint and exactly the frozen grants
    And JSON status lists exactly the recorded credential warnings and omitted count for that session
    And plain status prints exactly the recorded credential warning lines after the session's credential grant lines
    And status neither reads mutable requests approvals or source data nor changes protected records

    Examples:
      | network   | warnings                                                                                                                         | omitted |
      | offline   | /vault on fuse.sshfs                                                                                                             | 0       |
      | allowlist | / on tmpfs; /checkout on overlay                                                                                                 | 0       |
      | offline   | /m1 on overlay; /m2 on overlay; /m3 on overlay; /m4 on overlay; /m5 on overlay; /m6 on overlay; /m7 on overlay; /m8 on overlay | 3       |
      | offline   | (unprintable) on (unprintable)                                                                                                   | 0       |
      | allowlist | (too long) on overlay                                                                                                            | 0       |
      | offline   | {eight longest}                                                                                                                  | 32760   |
      | offline   | {eight longest} in a 65536-byte record                                                                                           | 32760   |

  # Slice-61 rule 6, by ID-22's pattern. The first status proves that the
  # seeded valid warnings decode. Then the fixture rewrites only the intent
  # record's warning members with the named defect. "Eight" and "nine" mean
  # /m1 to /m8 or /m9 on overlay. Nothing else is hashed or referenced, so
  # nothing but the defect can make the second status refuse. Like ID-22's
  # When, the new When keeps the first JSON status as the only entry of
  # w.approval.results, re-captures the protected tree after the defect, and
  # leaves the second status as the current result for the reused Thens.
  # The two size defects are the next byte above ID-CREDAPPROVAL-37's
  # largest valid cells:
  #  - "a 513-byte path" replaces the path with "/vault/" followed by 506 "a".
  #  - "the eight longest warnings in a 65537-byte record" writes ID-37's
  #    {eight longest} with omitted count 32760, then pads with spaces before
  #    the final "}" to exactly 65537 bytes. Each member is valid, so only
  #    the size can make this second status refuse.
  # Before the work the first status already refuses and the first Then fails.
  @wip @slice-61 @ID-CREDAPPROVAL-38 @local-engine-fixture
  Scenario Outline: A malformed recorded filesystem warning makes the session record invalid
    Given an independently seeded stopped credential session on "offline" with both frozen grants
    And the seeded intent record also carries the credential warnings "/vault on fuse.sshfs" with omitted count 0
    When status is invoked then the fixture applies the recorded warning defect "<defect>" and invokes status again
    Then the first status succeeds with exactly the original frozen grants
    And the first status lists exactly the seeded credential warnings
    And the second status reports "recovery.invalid-state" with exit 2 and no session entry
    And neither status reads current grants or repairs protected records

    Examples:
      | defect                                          |
      | nine warnings without an omitted count          |
      | two warnings out of order                       |
      | a repeated warning                              |
      | an empty warnings array                         |
      | a rule other than credential.filesystem         |
      | an extra member in the warning                  |
      | a relative path                                 |
      | a path with a dot-dot component                 |
      | a filesystem containing a space                 |
      | a 65-byte filesystem                            |
      | an omitted count of 2 beside one warning        |
      | an omitted count of 0 beside eight warnings     |
      | an omitted count of 2 without warnings          |
      | an omitted count written as a string beside eight warnings |
      | a 513-byte path                                 |
      | the eight longest warnings in a 65537-byte record |

  # The consent/host-agent assertions already pass before slice-30. Keep them as
  # regression checks; the missing exposure disclosure is this scenario's red.
  # Help must preview all three credential disclosure lines, both allowlist-only
  # lines and the version-3 delivery warning specified above. Label the allowlist
  # limits as such and describe delivery as prospective, preserving the run gate.
  # The new assertion checks each complete statement after joining whitespace,
  # like credentialApprovalHelpContains; terminal wrapping must not cause red.
  @slice-30 @ID-CREDAPPROVAL-24
  Scenario: Approval help explains credential exposure, consent and its host-agent limit
    When itos-container runs with "approve --help"
    Then approval help gives every specified credential disclosure and the version 3 delivery warning
    And approval help says credential and network grants require terminal review and refuse every fingerprint option
    And approval help retains the offline no-credential fingerprint route and unchanged-settings no-prompt rule
    And approval help says an unsandboxed host agent can fake a terminal and approval records what not who
    And it exits with code 0

  # Slice-61 rule 8. The phrase stays on one help line, so wrapping cannot split it.
  @wip @slice-61 @ID-CREDAPPROVAL-39
  Scenario: Doctor help says other credential filesystems warn rather than refuse
    When itos-container runs with "doctor --help"
    Then it exits with code 0
    And its standard output says "Filesystems other than ext4, btrfs and xfs give credential.filesystem warnings, not refusals"

  @slice-30 @ID-CREDAPPROVAL-30
  Scenario Outline: Unsupported credential inspection hosts refuse before reading the policy pin identity
    Given a credential observation fixture for "checkout" with "distinct outside file"
    And the credential observation host is "<os>" with architecture "<architecture>"
    When the credential observation contract runs in JSON mode
    Then it exits with code 1
    And its JSON output names the problem "doctor.host.unsupported"
    And the credential observation recorded no pinned checkout identity reads
    And the credential observation recorded no source metadata checkout traversal or state mount-table calls

    Examples:
      | os      | architecture |
      | windows | amd64        |
      | darwin  | amd64        |
      | linux   | arm64        |
