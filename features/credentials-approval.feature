@phase-1
Feature: Review and remember credential grants without delivering them
  Approval binds the files a worker would receive, never their contents.
  The person reviews a credential grant at a terminal. A changed grant needs fresh
  consent, and status describes the session's frozen grant, not a later request.

  # Authorities: docs/charter.md, docs/plans/container-pilot.md AC-02/03/04/29/36,
  # docs/architecture.md (Slices), docs/decisions/0025-credentials-passed-in.md
  # and q-52. Read the dependency specs, not their older registry summaries:
  # slice-29 at 5fff7908139202e2adc0c0bb6261e61907decf76 supplies
  # features/credentials-policy.feature and decision 0026-credential-source-inspection;
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
  # Fixture/step handoff. All credential-approval-specific phrases below are NEW.
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
  @wip @slice-30 @ID-CREDAPPROVAL-01 @local-engine-fixture
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

  @wip @slice-30 @ID-CREDAPPROVAL-02 @local-engine-fixture
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

  @wip @slice-30 @ID-CREDAPPROVAL-03 @local-engine-fixture
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

  @wip @slice-30 @ID-CREDAPPROVAL-04 @local-engine-fixture
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
  @wip @slice-30 @ID-CREDAPPROVAL-15 @local-engine-fixture
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

  @wip @slice-30 @ID-CREDAPPROVAL-16 @local-engine-fixture
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
  @wip @slice-30 @ID-CREDAPPROVAL-05 @local-engine-fixture
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

  @wip @slice-30 @ID-CREDAPPROVAL-06 @local-engine-fixture
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

  @wip @slice-30 @ID-CREDAPPROVAL-09 @local-engine-fixture
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

  @wip @slice-30 @ID-CREDAPPROVAL-23 @local-engine-fixture
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
  @wip @slice-30 @ID-CREDAPPROVAL-07 @local-engine-fixture
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

  @wip @slice-30 @ID-CREDAPPROVAL-08 @local-engine-fixture
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

  @wip @slice-30 @ID-CREDAPPROVAL-10 @local-engine-fixture
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

  @wip @slice-30 @ID-CREDAPPROVAL-11 @local-engine-fixture
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

  @wip @slice-30 @ID-CREDAPPROVAL-25 @local-engine-fixture
  Scenario: A valid source rotation during review does not bind the old contents or inode
    Given a credential approval checkout with private synthetic sources
    And the credential approval request is "claude" on "offline"
    When the credential terminal review is displayed then the fixture applies "replace claude-a with a different valid private inode" before answering yes
    Then the sole active approval is the exact candidate at generation "1" in protected host state
    And it exits with code 0
    And credential observations read no source data and leak no synthetic marker

  @wip @slice-30 @ID-CREDAPPROVAL-26 @local-engine-fixture
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
  @wip @slice-30 @ID-CREDAPPROVAL-12 @local-engine-fixture
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

  # The API fixture pauses at image inspection after the first metadata check.
  # A premature policy gate must reach Then as a failed readiness assertion.
  @wip @slice-30 @ID-CREDAPPROVAL-13 @local-engine-fixture
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

  @wip @slice-30 @ID-CREDAPPROVAL-14 @local-engine-fixture
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
  @wip @slice-30 @ID-CREDAPPROVAL-17 @local-engine-fixture
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

  @wip @slice-30 @ID-CREDAPPROVAL-18 @local-engine-fixture
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

  @wip @slice-30 @ID-CREDAPPROVAL-19
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

  @wip @slice-30 @ID-CREDAPPROVAL-20
  Scenario: Frozen grants survive later request and approval replacement unchanged
    Given a credential freeze contract with independently seeded matching grants at generation "4"
    When the real service selects and publishes frozen input then the fixture replaces the request and active approval
    Then the selected and persisted snapshot contain the exact original canonical settings bytes fingerprint project and generation
    And changing caller data or returned values cannot alter the stored or independently returned grants
    And a new selection using generation "4" refuses as superseded without another snapshot
    And the credential freeze contract reads no source data and creates no worker or staged material

  # Status reads protected historical authority. Seeded absence avoids depending
  # on a delivered worker or on T-40's network implementation.
  @wip @slice-30 @ID-CREDAPPROVAL-21 @local-engine-fixture
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
  @wip @slice-30 @ID-CREDAPPROVAL-22 @local-engine-fixture
  Scenario: A current grant cannot fill in an invalid frozen snapshot
    Given an independently seeded stopped credential session on "offline" with both frozen grants
    And the current request and active approval name different valid grants
    When status is invoked then the fixture removes the frozen credentials member recomputes its hash and invokes status again
    Then the first status succeeds with exactly the original frozen grants
    And the second status reports "recovery.invalid-state" with exit 2 and no session entry
    And neither status reads current grants or repairs protected records

  @wip @slice-30 @ID-CREDAPPROVAL-24
  Scenario: Approval help explains the credential consent rule and its host-agent limit
    When itos-container runs with "approve --help"
    Then approval help says credential and network grants require terminal review and refuse every fingerprint option
    And approval help retains the offline no-credential fingerprint route and unchanged-settings no-prompt rule
    And approval help says an unsandboxed host agent can fake a terminal and approval records what not who
    And it exits with code 0
