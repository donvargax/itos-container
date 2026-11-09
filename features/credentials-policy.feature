@phase-1
Feature: Named credential requests and read-only source inspection
  A policy names host files; it contains no credential value and grants no access.
  A person can inspect the request and its source metadata without reading secrets,
  executing checkout code, contacting an engine or recording approval.

  # Authorities: docs/charter.md, docs/architecture.md (Slices), CONFIG.md,
  # CLI.md and T-47's docs/decisions/0025-credentials-passed-in.md on its branch.
  # Draft against main 168eb7c9ba9800919a1d2a9c1d8affe0eadc2416,
  # slice-25 12ea980afcb5474e08da96699b68c19b103b63c3 and
  # T-47 389f5585b64557f545ce05c88a914c9461c3f88f (record 01cfa825).
  # This spec waits for those policy/decision changes; it does not copy their code.
  # Both T-47 and slice-25 are prerequisites. The coordinator must retain T-47
  # and add slice-25 to the registry dependency list before implementation.
  # docs/decisions/0026-credential-source-inspection.md amends T-47's source
  # location rule and increment table now, with the corresponding arc42 changes.
  #
  # T-47 supplies version 3: version 2 plus optional credentials, a nonempty list
  # of closed objects containing exactly kind and source. Kinds are case-sensitive
  # claude-token and github-token, each at most once. There is no separate name,
  # target, environment, inline token, command or Codex kind. T-47 question 1 owns
  # the Codex/charter conflict; adding a Codex kind is not this item's decision.
  # Versions 1/2 still refuse credentials as unknown. Omitting credentials means
  # no grants and omits the member from effective policy, including version 3.
  # Display valid entries sorted by kind; diagnostic indices use written order.
  # Network defaults and version 2's allowlist rules carry into version 3 intact.
  #
  # A source is a literal absolute slash-separated path of at most 4096 UTF-8
  # bytes, with at least one component after /. No empty, . or .. components,
  # trailing slash, backslash or ASCII control (U+0000..001F or U+007F). Never
  # trim, clean, expand ~ or variables, or evaluate text. Tilde/dollar characters
  # inside an already absolute path are literal filenames, not interpolation.
  # Policy parsing never opens or stats a named source. Existing YAML budgets,
  # structural refusals and no-partial-policy behavior apply before projection.
  #
  # New policy rules, exit 2, with file itos-container.yaml and RFC 6901 path:
  # policy.credentials_empty     /credentials, present but empty list
  # policy.credential_kind       /credentials/<i>/kind, unknown string kind
  # policy.credential_duplicate  /credentials/<i>/kind, repeated known kind
  # policy.credential_source     /credentials/<i>/source, invalid lexical path
  # Missing fields use policy.required, wrong types policy.type, and extra keys
  # policy.unknown_key. Do not cascade through a wrongly typed list or entry.
  # Check kind and source independently; unknown kinds never count as duplicates.
  # Collect independent problems without echoing rejected values or parser errors.
  # Schema-specific fixes name the declared version; unsupported-version fixes
  # name versions 1, 2 and 3. Plain paths use the existing Go-string escaping;
  # decoded JSON preserves the literal path. No contents or content hash is output.
  #
  # Decision 0026 resolves a scope omission in T-47's increment table:
  # introduce doctor --credentials-only, scope "credentials", for policy plus
  # source metadata only. --policy-only and --manifest-only keep their existing
  # scopes and never inspect host sources. The three flags are mutually exclusive.
  # This adds no approval or delivery: ordinary doctor, approve and run still
  # refuse a valid version 3 with policy.version at /version, exit 2, before the
  # manifest, source inspection, engine or state publication. Invalid version 3
  # reports its own policy problems first. The gate's fix names doctor --policy-only
  # and doctor --credentials-only. Slice-30 owns lifting doctor/approve's gate,
  # binding grants and preventing run before delivery. Never accept version 3 as
  # an offline approval by dropping credentials. Preserve any sibling version-2 gate.
  #
  # The new mode parses policy first, then observes sources using the effective
  # host UID and pinned checkout/state roots, without creating either state or
  # approval. It needs no manifest, Git helper, image or engine. On Linux amd64,
  # reuse the readiness source observation T-47 requires; no separate validator.
  # Observe each source in written order, collect one first failure per source:
  # 1. O_PATH/O_NOFOLLOW at every component; refuse any symlink, dangling included.
  # 2. Every ancestor is root- or host-owned, with no group/other write bits unless
  #    root-owned and sticky, matching approval/disk.directory's ancestor rule.
  # 3. Compare the source and every source ancestor's device/inode with BOTH
  #    protected roots AND every entry beneath them. A match is location refusal.
  #    Enumerate the pinned checkout and existing protected state tree completely,
  #    including .git, hidden, ignored and untracked entries. Cross directory mount
  #    points and inspect mounted file identities; neither device changes nor
  #    repeated directory identities permit skipping entries. Do not follow
  #    symlinks or open file data. Pin directories and check identity when opening
  #    each child. A failed/incomplete enumeration or observed replacement refuses
  #    with unavailable, never a safe result from the partial inventory. A known
  #    identity match still reports location even if another subtree is unavailable.
  #    Reuse readiness's bounded traversal primitives: batches of at most 128,
  #    100000 entries total across both trees, directory depth 64 from each root
  #    (depth 0), and one 10-second monotonic deadline across the scan. Count/depth
  #    limits are inclusive; exceeding them or reaching the deadline refuses.
  #    Time checks between synchronous calls do not preempt blocked kernel calls.
  #    No background scan
  #    continues after refusal, and no earlier invocation's inventory is reused.
  #    This detects bind aliases of protected descendants and individual files
  #    even when source ancestry contains neither root and link count is one.
  #    It also detects an outside source bound INTO either protected tree. It does
  #    not reject an unrelated inode just for sharing a pathname prefix or device.
  #    The checkout remains the only authorized worker host directory mount.
  # 4. Require a regular host-UID-owned file, no group/other permission bit,
  #    one link, and size 1..65536 inclusive, in that order. Owner read permission
  #    is not required for this metadata-only mode; delivery checks are later work.
  # 5. A source device/inode already used by another valid grant is refused.
  # An absent protected state root is not created. An inaccessible existing
  # boundary or descendant refuses, rather than being omitted from the inventory.
  # Existing state-location/identity failures retain their shared rules and exits.
  # The mode does not establish approval readiness or inspect another project's
  # admission history or record contents; the state walk observes metadata only.
  # Slice-21's controls still govern later approval and run. With no grants there
  # is no source/exposure scan. Later consumers re-observe before use: this is an
  # observation of the pinned trees, not continuous protection against host changes.
  #
  # Source rules are located at /credentials/<written-index>/source in the policy.
  # credential.missing (3): a source or ordinary ancestor is absent.
  # credential.unavailable (3): unavailable metadata/identity or incomplete protected scan.
  # credential.symlink (1): a symbolic link at any component, even dangling.
  # credential.ancestor (1): unsafe ancestor ownership or write permissions.
  # credential.location (1): an identity exposed in checkout or protected state.
  # credential.regular (1): a non-regular source.
  # credential.owner (1): a regular source belongs to a different UID, including root.
  # credential.permissions (1): any group/other read, write or execute permission.
  # credential.links (1): link count is not exactly one.
  # credential.size (1): outside 1..65536 bytes.
  # credential.duplicate (1): a later grant names an already used source inode.
  # Each problem has nonempty message/fix, file, path and exit. Safe source paths
  # may be displayed, but never source contents or wrapped OS errors. Highest exit
  # wins. Only successful policy projection allows source observations.
  #
  # Metadata JSON is one schema:1 object, scope:"credentials", ok, problems and
  # credentials (always an array). Each credential has exactly kind, source and
  # target; targets are /run/itos-container/credentials/<kind>, fixed by T-47.
  # The list is sorted by kind and remains visible on source failure. With no
  # grants it is []. Policy failures use the existing policy refusal envelope,
  # without partial effective data. No approval fingerprint or readiness claim.
  # Plain output names "Credentials only", the same grants and locations, and
  # "Credential contents, manifest, engine and approval were not checked".
  # ITOS_JSON and explicit --no-json follow the existing doctor convention.
  #
  # Architecture/library handoff for the implementer:
  # Extend internal/engine/policy's domain/port for pure schema rules, and
  # internal/engine/readiness's domain/port/disk for shared source observation.
  # Doctor stays internal/slices/doctor/{command,domain,port,inspection}; only cmd
  # binds adapters. Use the already pinned YAML v4 and x/sys/unix, plus Go's
  # standard library; no new dependency or handwritten YAML/filesystem library.
  # I/O-free port/porttest fakes and a shared contract must cover foreign source
  # and ancestor owners, root-owned sticky ancestors, inaccessible components,
  # unavailable stat data, and stable checked descriptor identity during path
  # replacement. Run that contract against fake and disk adapter; use adapter-local
  # observation seams for privileged metadata, never a production env override.
  # For EACH protected tree the shared contract must include root-directory binds,
  # descendant-directory binds, and single-file binds exposed outside the tree,
  # plus outside directories/files bound into descendants of the tree. All use
  # one-link regular files and safe modes/owners. Descendant/file and inverse
  # aliases have outside source ancestry with neither protected root identity;
  # root-directory aliases exercise the inherited root check as well.
  # Also include a distinct-inode same-device control, unreadable/disappearing
  # descendants, metadata failure,
  # directory substitution, and entry/depth/time limits at and beyond the bounds.
  # Alias results must be location; incomplete scans must be unavailable, with a
  # known match taking precedence. Do not precompute those decisions in fixtures.
  # Test directory and file mount crossings, including a device change below a
  # protected root; a scan stopping at a mount point must fail the contract.
  # The simulated alias cases prove the shared observation contract, not native
  # bind-mount or engine behavior. No privileged mount is needed by these tests.
  # Preserve first-failure order and all independent sources in those contracts.
  #
  # Publish docs/schemas/itos-container-v3.schema.json beside unchanged v1/v2,
  # matching this closed data shape and documenting byte limits and host checks
  # that JSON Schema's character-count constraints cannot fully express.
  # Implementation updates docs/CONFIG.md, docs/CLI.md and doctor help. Decision
  # 0026 and its arc42 amendment are part of this spec, not deferred to that PR.
  # ID-POLICY-15 on slice-25 currently refuses version 3: the feat commit must move
  # that unsupported-version example to 4 and explain why. Update the corresponding
  # policy unit/port-contract fixtures and version-list diagnostic assertions,
  # including ID-POLICY-77, without changing versions 1/2's existing meanings.
  # Neighbors: policy.feature (slice-1/25), parsing.feature, manifest.feature,
  # doctor.feature, approval.feature, run.feature and slice-21's control-state.feature.
  # Red/green here is local and non-engine; no Docker, real token, model or account.
  # The implementer commits steps red first with these @wip tags still present,
  # then removes @wip for assertion-failing runs before implementing. Undefined
  # steps are not red evidence. This drafting commit makes no executed-red claim.

  Background:
    Given policy inspection has isolated host state and a recording engine endpoint

  # Policy shape and presentation. No path below names a real credential.
  @wip @slice-29 @ID-CREDENTIALS-01
  Scenario: version 3 displays both named requests as a set without executing helpers
    Given the policy file contains:
      """
      version: 3
      manifest: .devcontainer/devcontainer.json
      network: {mode: allowlist, allow: [API.Example.COM.]}
      credentials:
        - {kind: github-token, source: /synthetic-sources/github}
        - {kind: claude-token, source: /synthetic-sources/claude}
      """
    And inspection helper sentinels are installed
    And the policy inspection files are snapshotted
    When itos-container runs with "doctor --policy-only --json"
    Then it exits with code 0
    And its policy JSON contains this effective policy:
      """
      {"version":3,"manifest":".devcontainer/devcontainer.json","network":{"mode":"allowlist","allow":["api.example.com"]},"credentials":[{"kind":"claude-token","source":"/synthetic-sources/claude"},{"kind":"github-token","source":"/synthetic-sources/github"}]}
      """
    And the policy inspection files are unchanged
    And no inspection helper sentinel was executed
    And the engine recorder has no connections

  # A version-3 positive control makes the legacy-compatibility rows red today.
  @wip @slice-29 @ID-CREDENTIALS-02
  Scenario Outline: omitting credentials keeps the offline defaults and creates no grant
    Given the policy file contains:
      """
      {version: 3, manifest: a.json}
      """
    When itos-container runs with "doctor --policy-only --json"
    Then it exits with code 0
    And its policy JSON contains this effective policy:
      """
      {"version":3,"manifest":"a.json","network":{"mode":"offline"}}
      """
    Given the policy file contains:
      """
      {version: <version>, manifest: a.json}
      """
    When itos-container runs with "doctor --policy-only --json"
    Then it exits with code 0
    And its policy JSON contains this effective policy:
      """
      {"version":<version>,"manifest":"a.json","network":{"mode":"offline"}}
      """
    And the engine recorder has no connections

    Examples:
      | version |
      | 1       |
      | 2       |

  @wip @slice-29 @ID-CREDENTIALS-03
  Scenario: plain policy inspection shows literal sources without expanding or granting them
    Given the policy file contains:
      """
      version: 3
      manifest: a.json
      credentials:
        - {kind: github-token, source: '/synthetic sources/ñ/~/$HOME/$(touch forbidden)'}
        - {kind: claude-token, source: /synthetic-sources/claude}
      """
    When itos-container runs with "doctor --policy-only"
    Then its standard output is exactly:
      """
      Policy only
      version: 3
      manifest: a.json
      network.mode: offline
      credentials.claude-token.source: /synthetic-sources/claude
      credentials.github-token.source: /synthetic sources/ñ/~/$HOME/$(touch forbidden)
      Host readiness and approval were not checked
      """
    And it exits with code 0
    And the engine recorder has no connections

  @wip @slice-29 @ID-CREDENTIALS-04
  Scenario Outline: credential entries are typed closed objects with both fields required
    Given the policy file contains:
      """
      version: 3
      manifest: a.json
      credentials: <value>
      """
    When itos-container runs with "doctor --policy-only --json"
    Then its policy JSON reports exactly these problems:
      | rule   | path   | exit |
      | <rule> | <path> | 2    |
    And it exits with code 2
    And the engine recorder has no connections

    Examples:
      | value                                                            | rule                     | path                  |
      | []                                                               | policy.credentials_empty | /credentials          |
      | null                                                             | policy.type              | /credentials          |
      | {}                                                               | policy.type              | /credentials          |
      | false                                                            | policy.type              | /credentials          |
      | token                                                            | policy.type              | /credentials          |
      | [null]                                                           | policy.type              | /credentials/0        |
      | [claude-token]                                                   | policy.type              | /credentials/0        |
      | [{source: /synthetic-sources/a}]                                 | policy.required          | /credentials/0/kind   |
      | [{kind: claude-token}]                                           | policy.required          | /credentials/0/source |
      | [{kind: null, source: /synthetic-sources/a}]                     | policy.type              | /credentials/0/kind   |
      | [{kind: false, source: /synthetic-sources/a}]                    | policy.type              | /credentials/0/kind   |
      | [{kind: claude-token, source: null}]                             | policy.type              | /credentials/0/source |
      | [{kind: claude-token, source: 42}]                               | policy.type              | /credentials/0/source |
      | [{kind: claude-token, source: /synthetic-sources/a, token: x}]   | policy.unknown_key       | /credentials/0/token  |
      | [{kind: claude-token, source: /synthetic-sources/a, target: /a}] | policy.unknown_key       | /credentials/0/target |
      | [{kind: claude-token, source: /synthetic-sources/a, name: work}] | policy.unknown_key       | /credentials/0/name   |

  @wip @slice-29 @ID-CREDENTIALS-05
  Scenario Outline: only the two decided token kinds are accepted
    Given the policy file contains:
      """
      version: 3
      manifest: a.json
      credentials: [{kind: '<kind>', source: /synthetic-sources/a}]
      """
    When itos-container runs with "doctor --policy-only --json"
    Then its policy JSON reports exactly these problems:
      | rule                   | path                | exit |
      | policy.credential_kind | /credentials/0/kind | 2    |
    And it exits with code 2

    Examples:
      | kind            |
      | codex-auth      |
      | codex-auth-file |
      | CLAUDE-TOKEN    |
      | github          |
      |                 |

  @wip @slice-29 @ID-CREDENTIALS-06
  Scenario Outline: repeated kinds are located at the later written entry
    Given the policy file contains:
      """
      version: 3
      manifest: a.json
      credentials:
        - {kind: <kind>, source: /synthetic-sources/first}
        - {kind: <kind>, source: /synthetic-sources/second}
      """
    When itos-container runs with "doctor --policy-only --json"
    Then its policy JSON reports exactly these problems:
      | rule                        | path                | exit |
      | policy.credential_duplicate | /credentials/1/kind | 2    |
    And it exits with code 2

    Examples:
      | kind         |
      | claude-token |
      | github-token |

  @wip @slice-29 @ID-CREDENTIALS-07
  Scenario Outline: a source must be an unambiguous literal absolute path
    Given the policy file contains:
      """
      version: 3
      manifest: a.json
      credentials: [{kind: claude-token, source: <source>}]
      """
    When itos-container runs with "doctor --policy-only --json"
    Then its policy JSON reports exactly these problems:
      | rule                     | path                  | exit |
      | policy.credential_source | /credentials/0/source | 2    |
    And it exits with code 2

    Examples:
      | source             |
      | ''                 |
      | token              |
      | ./token            |
      | ../token           |
      | '~/token'          |
      | '$HOME/token'      |
      | ' /vault/token'    |
      | /                  |
      | //vault/token      |
      | /vault//token      |
      | /vault/./token     |
      | /vault/../token    |
      | /vault/token/      |
      | 'C:/vault/token'   |
      | "/vault/\\\\token" |
      | "/vault/\\u0000"   |
      | "/vault/\\u0009"   |
      | "/vault/\\u000a"   |
      | "/vault/\\u001f"   |
      | "/vault/\\u007f"   |

  # New Given writes version 3, manifest a.json and one claude-token source:
  # / followed by repeated UTF-8 text and ASCII padding to exactly the given
  # decoded byte length, with no other slash. It creates no source on disk.
  # New Then compares the complete effective policy to that generated literal.
  @wip @slice-29 @ID-CREDENTIALS-08
  Scenario Outline: source length counts UTF-8 bytes and accepts the exact limit
    Given a credential policy source repeats "<text>" to exactly 4096 bytes
    When itos-container runs with "doctor --policy-only --json" and a 5 second deadline
    Then it exits with code 0
    And its policy JSON preserves the generated credential source exactly
    And it finishes before the deadline
    Given a credential policy source repeats "<text>" to exactly 4097 bytes
    When itos-container runs with "doctor --policy-only --json" and a 5 second deadline
    Then its policy JSON reports exactly these problems:
      | rule                     | path                  | exit |
      | policy.credential_source | /credentials/0/source | 2    |
    And it exits with code 2
    And it finishes before the deadline
    And the engine recorder has no connections

    Examples:
      | text |
      | a    |
      | ñ    |

  @wip @slice-29 @ID-CREDENTIALS-09
  Scenario: independent credential problems retain written locations and reveal no rejected values
    Given the policy file contains:
      """
      version: 3
      manifest: a.json
      credentials:
        - {kind: github-token, source: /synthetic-sources/github}
        - {kind: claude-token, source: /synthetic-sources/claude}
        - {kind: github-token, source: 'itos-synthetic-invalid-source'}
        - {kind: itos-synthetic-unknown-kind, source: false}
      """
    When itos-container runs with "doctor --policy-only --json"
    Then its policy JSON reports exactly these problems:
      | rule                        | path                  | exit |
      | policy.credential_duplicate | /credentials/2/kind   | 2    |
      | policy.credential_source    | /credentials/2/source | 2    |
      | policy.credential_kind      | /credentials/3/kind   | 2    |
      | policy.type                 | /credentials/3/source | 2    |
    And it exits with code 2
    And its standard output does not say "itos-synthetic-invalid-source"
    And its error output does not say "itos-synthetic-invalid-source"
    And its standard output does not say "itos-synthetic-unknown-kind"
    And its error output does not say "itos-synthetic-unknown-kind"

  # Structural errors already fail before version dispatch. Start with a valid
  # v3 control so this scenario cannot be green on a binary that rejects all v3.
  @wip @slice-29 @ID-CREDENTIALS-10
  Scenario: credential entries do not bypass YAML duplicate-key checks
    Given the policy file contains:
      """
      {version: 3, manifest: a.json, credentials: [{kind: claude-token, source: /synthetic-sources/a}]}
      """
    When itos-container runs with "doctor --policy-only --json"
    Then it exits with code 0
    Given the policy file contains:
      """
      version: 3
      manifest: a.json
      credentials:
        - kind: claude-token
          source: /synthetic-sources/a
          source: /synthetic-sources/b
      """
    When itos-container runs with "doctor --policy-only --json"
    Then its policy JSON reports exactly these problems:
      | rule                 | path                  | exit |
      | policy.duplicate_key | /credentials/0/source | 2    |
    And it exits with code 2

  @wip @slice-29 @ID-CREDENTIALS-11
  Scenario Outline: older policy versions cannot acquire credential semantics
    Given the policy file contains:
      """
      {version: 3, manifest: a.json, credentials: [{kind: github-token, source: /synthetic-sources/a}]}
      """
    When itos-container runs with "doctor --policy-only --json"
    Then it exits with code 0
    Given the policy file contains:
      """
      {version: <version>, manifest: a.json, credentials: [{kind: github-token, source: /synthetic-sources/a}]}
      """
    When itos-container runs with "doctor --policy-only --json"
    Then its policy JSON reports exactly these problems:
      | rule               | path         | exit |
      | policy.unknown_key | /credentials | 2    |
    And it exits with code 2
    And the engine recorder has no connections

    Examples:
      | version |
      | 1       |
      | 2       |

  # These three fixture/endpoint steps already exist on slice-25. The manifest
  # fixture leaves a private checkout and valid pinned-image JSON, no real engine.
  @wip @slice-29 @ID-CREDENTIALS-12
  Scenario: manifest-only validation accepts version 3 without checking a source
    Given a manifest inspection checkout
    And the manifest inspection policy contains:
      """
      version: 3
      manifest: .devcontainer/devcontainer.json
      credentials: [{kind: claude-token, source: /synthetic-missing-source/claude}]
      """
    And manifest inspection is given the recording engine endpoint
    And host execution sentinels are installed
    When itos-container runs with "doctor --manifest-only --json"
    Then its JSON output reports a valid manifest at ".devcontainer/devcontainer.json"
    And it exits with code 0
    And inspection has no host side effects
    And the engine recorder has no connections

  # New fixture steps for metadata and command gates below:
  # - "a private credential inspection fixture" creates a standalone scratch
  #   checkout with .git, a valid manifest and separate private HOME/state/source
  #   directories, and reuses the Background's loopback engine recorder. It selects only
  #   these paths for the child; no ambient credential/provider variables survive.
  #   Helpers and hooks are recording sentinels, never real Git/docker programs.
  # - "credential source ... has layout ..." creates only the specified fixture.
  #   A private regular file is host-owned 0600, one link, with a fresh synthetic
  #   marker as in T-47. Other layouts change exactly the named property. Missing
  #   means an absent leaf under an existing safe parent. No FIFO writer is started.
  # - "the credential fixture requests these sources" writes v3 policy, offline
  #   network and the selected manifest, substituting fixture absolute paths into
  #   {kind,source} entries in table order. The table's source name is a test alias,
  #   not a new policy key. No Given invokes doctor or judges product output.
  # - "keeps requested grants" leaves that list; "keeps none grants" removes the
  #   credentials key without changing the version. "manifest is absent" removes
  #   only the fixture manifest, leaving its policy path and checkout intact.
  # - Snapshot/observer setup records no-follow entries, metadata and known bytes
  #   before observation, never opening a FIFO or following a fixture symlink.
  #   Use Linux inotify on ordinary sources/targets to record data-open/access;
  #   O_PATH metadata handles are permitted. An independent file read proves the
  #   observer positive control. Close/drain it before harness verification reads.
  # - Grant assertions compare kind, exact resolved source and fixed target in
  #   sorted order. Failure assertions check scope/ok, complete located problem
  #   sets and retained grants, not merely an exit or a substring.
  # - Side-effect assertions compare checkout/state/source entries, modes, owners
  #   and bytes to setup, plus zero helper execution and zero engine connections.
  #   They do not open missing, inaccessible or special files to take snapshots.
  #   Sources are metadata-only where bytes cannot be read; setup retains the bytes.
  # - Marker absence scans raw marker, SHA-256 and base64 (all three offsets) in
  #   stdout/stderr and every checkout/state file, with a separate positive control
  #   seeded with the marker and every searched encoding outside both trees.
  #   Source files and observer controls are excluded.
  #   This proves controller inspection only, not arbitrary workload/log redaction.
  # All new native metadata scenarios are Linux-only, non-engine; no privileged
  # chown, host mount, real source or production test switch is needed for setup.
  # Pin the native fixtures below /tmp for the root-owned sticky ancestor control.

  @wip @slice-29 @ID-CREDENTIALS-13 @linux
  Scenario Outline: valid version 3 remains gated from ordinary doctor approval and run
    Given a private credential inspection fixture
    And credential source "token" has layout "private regular file"
    And the credential fixture requests these sources:
      | kind         | source |
      | claude-token | token  |
    And the credential fixture keeps "<grants>" grants
    And the credential fixture manifest is absent
    And credential inspection effects are snapshotted
    When itos-container runs with "doctor --policy-only --json"
    Then it exits with code 0
    When itos-container runs with "<command>"
    Then the credential fixture command reports exactly "policy.version" at "/version" with exit 2
    And its error output says "doctor --policy-only"
    And its error output says "doctor --credentials-only"
    And credential inspection has no side effects or helper execution
    And the engine recorder has no connections

    Examples:
      | grants    | command                                                                                       |
      | requested | doctor --json                                                                                 |
      | none      | doctor --json                                                                                 |
      | requested | approve --fingerprint 0000000000000000000000000000000000000000000000000000000000000000 --json |
      | none      | approve --fingerprint 0000000000000000000000000000000000000000000000000000000000000000 --json |
      | requested | run -- credential-fixture-never-executed                                                      |
      | none      | run -- credential-fixture-never-executed                                                      |

  @wip @slice-29 @ID-CREDENTIALS-14
  Scenario Outline: invalid version 3 reports its policy problem before any later inspection
    Given the policy file contains:
      """
      {version: 3, manifest: missing.json, credentials: []}
      """
    When itos-container runs with "<command>"
    Then its doctor JSON reports exactly these policy problems:
      | rule                     | path         | exit |
      | policy.credentials_empty | /credentials | 2    |
    And it exits with code 2
    And the engine recorder has no connections

    Examples:
      | command                          |
      | doctor --policy-only --json      |
      | doctor --manifest-only --json    |
      | doctor --credentials-only --json |
      | doctor --json                    |

  @wip @slice-29 @ID-CREDENTIALS-15
  Scenario: doctor help distinguishes credential metadata from file-only and approval scopes
    When itos-container runs with "doctor --help"
    Then its standard output says "--credentials-only"
    And its standard output says "metadata"
    And its standard output says "without reading credential contents"
    And its standard output says "does not approve"
    And it exits with code 0
    And the engine recorder has no connections

  # Source metadata inspection. The observer is armed after fixture setup and
  # before either CLI invocation; its positive control is outside product trees.
  @wip @slice-29 @ID-CREDENTIALS-16 @linux
  Scenario: metadata inspection shows both grants without reading or retaining either token
    Given a private credential inspection fixture
    And credential source "github" has layout "private regular file"
    And credential source "claude" has layout "private regular file"
    And the credential fixture requests these sources:
      | kind         | source |
      | github-token | github |
      | claude-token | claude |
    And credential inspection effects are snapshotted
    And credential source data access is observed with an independent read control
    When itos-container runs with "doctor --policy-only --json"
    Then it exits with code 0
    When itos-container runs with "doctor --credentials-only --json"
    Then it exits with code 0
    And its credential JSON reports exactly these grants and no problems:
      | kind         | source | target                                       |
      | claude-token | claude | /run/itos-container/credentials/claude-token |
      | github-token | github | /run/itos-container/credentials/github-token |
    And the source observer saw its control read and no credential data open or read
    And no credential marker or content hash appears in inspection output checkout or state
    And credential inspection has no side effects or helper execution
    And the engine recorder has no connections

  # New size/mode Given writes synthetic bytes of exactly this length, then
  # chmods only this fixture leaf. A one-byte boundary file cannot contain the
  # full marker; the marker absence probe belongs to 16, which uses full markers.
  @wip @slice-29 @ID-CREDENTIALS-17 @linux
  Scenario Outline: private regular sources need metadata but not content readability
    Given a private credential inspection fixture
    And credential source "token" is a regular file of <bytes> bytes with mode "<mode>"
    And the credential fixture requests these sources:
      | kind   | source |
      | <kind> | token  |
    And credential inspection effects are snapshotted
    When itos-container runs with "doctor --credentials-only --json"
    Then it exits with code 0
    And its credential JSON reports exactly these grants and no problems:
      | kind   | source | target                                 |
      | <kind> | token  | /run/itos-container/credentials/<kind> |
    And credential inspection has no side effects or helper execution
    And the engine recorder has no connections

    Examples:
      | kind         | bytes | mode |
      | claude-token | 1     | 0600 |
      | github-token | 65536 | 0400 |
      | claude-token | 64    | 0200 |
      | github-token | 64    | 0000 |

  # These failures must not open a special file or wait for a FIFO writer.
  # Source layouts remain lexically valid: --policy-only accepts all of them.
  @wip @slice-29 @ID-CREDENTIALS-18 @linux
  Scenario Outline: unavailable linked and non-regular sources fail without reading them
    Given a private credential inspection fixture
    And credential source "token" has layout "<layout>"
    And the credential fixture requests these sources:
      | kind         | source |
      | claude-token | token  |
    And credential inspection effects are snapshotted
    When itos-container runs with "doctor --policy-only --json"
    Then it exits with code 0
    When itos-container runs with "doctor --credentials-only --json" and a 5 second deadline
    Then its credential JSON reports exactly these source problems and retains the requested grants:
      | rule   | path                  | exit   |
      | <rule> | /credentials/0/source | <exit> |
    And it exits with code <exit>
    And it finishes before the deadline
    And credential inspection has no side effects or helper execution
    And the engine recorder has no connections

    Examples:
      | layout                              | rule                   | exit |
      | absent leaf                         | credential.missing     | 3    |
      | absent ancestor                     | credential.missing     | 3    |
      | directory                           | credential.regular     | 1    |
      | FIFO with no writer                 | credential.regular     | 1    |
      | Unix socket with no listener        | credential.regular     | 1    |
      | leaf symlink to private source      | credential.symlink     | 1    |
      | dangling leaf symlink               | credential.symlink     | 1    |
      | parent symlink to private directory | credential.symlink     | 1    |
      | dangling parent symlink             | credential.symlink     | 1    |
      | regular file used as a parent       | credential.unavailable | 3    |

  # Each bit stands alone so a check for only readability cannot pass.
  @wip @slice-29 @ID-CREDENTIALS-19 @linux
  Scenario Outline: any group or other source permission is refused
    Given a private credential inspection fixture
    And credential source "token" is a regular file of 64 bytes with mode "<mode>"
    And the credential fixture requests these sources:
      | kind         | source |
      | github-token | token  |
    And credential inspection effects are snapshotted
    When itos-container runs with "doctor --credentials-only --json"
    Then its credential JSON reports exactly these source problems and retains the requested grants:
      | rule                   | path                  | exit |
      | credential.permissions | /credentials/0/source | 1    |
    And it exits with code 1
    And credential inspection has no side effects or helper execution
    And the engine recorder has no connections

    Examples:
      | mode |
      | 0640 |
      | 0620 |
      | 0610 |
      | 0604 |
      | 0602 |
      | 0601 |

  # Layouts place sources in the selected checkout or managed itos-container
  # state tree, including .git and hidden descendants. Roots themselves are
  # included to pin location-before-regular-file precedence. No state is approved.
  @wip @slice-29 @ID-CREDENTIALS-20 @linux
  Scenario Outline: a credential source cannot reside in checkout or protected state
    Given a private credential inspection fixture
    And credential source "token" has layout "<layout>"
    And the credential fixture requests these sources:
      | kind         | source |
      | claude-token | token  |
    And credential inspection effects are snapshotted
    When itos-container runs with "doctor --credentials-only --json"
    Then its credential JSON reports exactly these source problems and retains the requested grants:
      | rule                | path                  | exit |
      | credential.location | /credentials/0/source | 1    |
    And it exits with code 1
    And credential inspection has no side effects or helper execution
    And the engine recorder has no connections

    Examples:
      | layout                                    |
      | checkout root                             |
      | private file at checkout root             |
      | private file below checkout .git          |
      | private file below checkout hidden folder |
      | protected state root                      |
      | private file below protected state        |

  # New contract Given stores raw metadata/topology and a v3 claude-token request
  # for source alias "token"; it calls no product code. It supplies Linux amd64
  # and a non-root host UID as values. All files have one link, 0600 mode, host
  # ownership and nonempty synthetic data. "distinct outside file"
  # uses a different inode on the same device, with a shared pathname prefix.
  # Each bind layout supplies the identities a bind exposes, not a symlink or a
  # second hard link. The source spelling stays outside both protected trees.
  # Root binds alias the selected root outside; descendant binds alias its private
  # subdirectory outside; file binds alias its private/token outside. The inverse
  # layouts mount an outside directory/file below that root's hidden descendant.
  # New When extends the existing test-only cmd/itos-container assembly pattern:
  # real policy parser, readiness source validator, doctor handler and presenter,
  # with an I/O-free metadata/topology fake beneath validation. The runner sends
  # raw fixture input and observes output; it imports no product package. Fixtures
  # cannot inject a ready result or a preselected credential failure. Use the same
  # cases in the fake/disk shared contract via adapter-local observation seams.
  # Reuse the compiled contract binary and its per-scenario coverage handling.
  # The first invocation proves that the new mode accepts distinct identities;
  # the second must reject the alias with the exact located diagnostic.
  @wip @slice-29 @ID-CREDENTIALS-29
  Scenario Outline: metadata inspection refuses bind aliases of protected descendants and files
    Given a credential observation fixture for "<boundary>" with "distinct outside file"
    When the credential observation contract runs in JSON mode
    Then it exits with code 0
    And its credential JSON reports exactly these grants and no problems:
      | kind         | source | target                                     |
      | claude-token | token  | /run/itos-container/credentials/claude-token |
    Given a credential observation fixture for "<boundary>" with "<layout>"
    When the credential observation contract runs in JSON mode
    Then its credential JSON reports exactly these source problems and retains the requested grants:
      | rule                | path                  | exit |
      | credential.location | /credentials/0/source | 1    |
    And it exits with code 1
    And the engine recorder has no connections

    Examples:
      | boundary        | layout                                    |
      | checkout        | root directory bound outside              |
      | checkout        | descendant directory bound outside        |
      | checkout        | single descendant file bound outside      |
      | checkout        | outside directory bound into a descendant |
      | checkout        | outside file bound into a descendant      |
      | protected state | root directory bound outside              |
      | protected state | descendant directory bound outside        |
      | protected state | single descendant file bound outside      |
      | protected state | outside directory bound into a descendant |
      | protected state | outside file bound into a descendant      |

  # New Given makes one empty private descendant, then removes all its permission
  # bits. The native non-root fixture retains its setup inventory; snapshots treat
  # that directory as metadata-only and cleanup restores access after assertions.
  # No source ancestry intersects either tree: only a complete exposure scan can
  # establish safety. Skipping the unreadable directory must not produce success.
  @wip @slice-29 @ID-CREDENTIALS-30 @linux
  Scenario Outline: an incomplete protected-tree scan cannot declare a source outside it
    Given a private credential inspection fixture
    And credential source "token" has layout "private regular file"
    And the credential fixture requests these sources:
      | kind         | source |
      | claude-token | token  |
    And the credential fixture "<boundary>" has an unreadable descendant
    And credential inspection effects are snapshotted
    When itos-container runs with "doctor --credentials-only --json"
    Then its credential JSON reports exactly these source problems and retains the requested grants:
      | rule                   | path                  | exit |
      | credential.unavailable | /credentials/0/source | 3    |
    And it exits with code 3
    And credential inspection has no side effects or helper execution
    And the engine recorder has no connections

    Examples:
      | boundary        |
      | checkout        |
      | protected state |

  # The root-owned sticky /tmp ancestor of every private fixture is a positive
  # control for the inherited directory rule. User-owned sticky is no exception.
  # Parent modes are 0720, 0702 and 01777 respectively; only fixture paths change.
  @wip @slice-29 @ID-CREDENTIALS-21 @linux
  Scenario Outline: a private source also needs safe ancestors
    Given a private credential inspection fixture
    And credential source "token" has layout "<layout>"
    And the credential fixture requests these sources:
      | kind         | source |
      | claude-token | token  |
    And credential inspection effects are snapshotted
    When itos-container runs with "doctor --credentials-only --json"
    Then its credential JSON reports exactly these source problems and retains the requested grants:
      | rule                | path                  | exit |
      | credential.ancestor | /credentials/0/source | 1    |
    And it exits with code 1
    And credential inspection has no side effects or helper execution
    And the engine recorder has no connections

    Examples:
      | layout                                                       |
      | private source below group-writable parent                   |
      | private source below other-writable parent                   |
      | private source below user-owned world-writable sticky parent |

  # Same path means the same inode with one link: this reaches the cross-grant
  # check rather than the independent multiple-hard-link refusal in 23.
  @wip @slice-29 @ID-CREDENTIALS-22 @linux
  Scenario: different kinds cannot share one source inode
    Given a private credential inspection fixture
    And credential source "token" has layout "private regular file"
    And the credential fixture requests these sources:
      | kind         | source |
      | github-token | token  |
      | claude-token | token  |
    When itos-container runs with "doctor --credentials-only --json"
    Then its credential JSON reports exactly these source problems and retains the requested grants:
      | rule                 | path                  | exit |
      | credential.duplicate | /credentials/1/source | 1    |
    And it exits with code 1
    And the engine recorder has no connections

  # Exposure through a checkout hard link is location refusal before link count;
  # an outside-only second link still reaches the independent links rule.
  @wip @slice-29 @ID-CREDENTIALS-23 @linux
  Scenario Outline: source exposure, link count and size bound metadata acceptance
    Given a private credential inspection fixture
    And credential source "token" has layout "<layout>"
    And the credential fixture requests these sources:
      | kind         | source |
      | claude-token | token  |
    And credential inspection effects are snapshotted
    When itos-container runs with "doctor --credentials-only --json"
    Then its credential JSON reports exactly these source problems and retains the requested grants:
      | rule   | path                  | exit |
      | <rule> | /credentials/0/source | 1    |
    And it exits with code 1
    And credential inspection has no side effects or helper execution
    And the engine recorder has no connections

    Examples:
      | layout                                          | rule                |
      | private source with a hard link in checkout      | credential.location |
      | private source with a hard link outside checkout | credential.links    |
      | private regular file of zero bytes               | credential.size     |
      | private regular file of 65537 bytes              | credential.size     |

  @wip @slice-29 @ID-CREDENTIALS-24 @linux
  Scenario: source failures are collected without hiding grants or disclosing contents
    Given a private credential inspection fixture
    And credential source "missing" has layout "absent leaf"
    And credential source "exposed" has layout "world-readable regular file"
    And the credential fixture requests these sources:
      | kind         | source  |
      | github-token | missing |
      | claude-token | exposed |
    And credential inspection effects are snapshotted
    And credential source data access is observed with an independent read control
    When itos-container runs with "doctor --credentials-only --json"
    Then its credential JSON reports exactly these source problems and retains the requested grants:
      | rule                   | path                  | exit |
      | credential.missing     | /credentials/0/source | 3    |
      | credential.permissions | /credentials/1/source | 1    |
    And it exits with code 3
    And the source observer saw its control read and no credential data open or read
    And no credential marker or content hash appears in inspection output checkout or state
    And credential inspection has no side effects or helper execution
    And the engine recorder has no connections

  # The first successful invocation prevents a preexisting unknown-flag usage
  # refusal from satisfying the combination assertion. The second command must
  # identify a scope conflict, not reject credentials-only as an unknown option.
  @wip @slice-29 @ID-CREDENTIALS-25 @linux
  Scenario Outline: explicit inspection scopes cannot be combined
    Given a private credential inspection fixture
    And credential source "token" has layout "private regular file"
    And the credential fixture requests these sources:
      | kind         | source |
      | claude-token | token  |
    When itos-container runs with "doctor --credentials-only --json"
    Then it exits with code 0
    And its credential JSON reports exactly these grants and no problems:
      | kind         | source | target                                       |
      | claude-token | token  | /run/itos-container/credentials/claude-token |
    Given credential inspection effects are snapshotted
    When itos-container runs with "doctor <flags> --json"
    Then its JSON output names the problem "usage"
    And it exits with code 2
    And its error output says "cannot be combined"
    And credential inspection has no side effects or helper execution
    And the engine recorder has no connections

    Examples:
      | flags                                            |
      | --credentials-only --policy-only                 |
      | --credentials-only --manifest-only               |
      | --credentials-only --policy-only --manifest-only |

  # New plain assertion resolves fixture source paths, requires exactly one line
  # per kind in sorted order, and applies the same escaping as policy inspection.
  @wip @slice-29 @ID-CREDENTIALS-26 @linux
  Scenario: credential metadata honors JSON selection and explains the plain inspection scope
    Given a private credential inspection fixture
    And credential source "token" has layout "private regular file"
    And the credential fixture requests these sources:
      | kind         | source |
      | github-token | token  |
    When itos-container runs with "doctor --credentials-only" and the environment "ITOS_JSON=1"
    Then it exits with code 0
    And its credential JSON reports exactly these grants and no problems:
      | kind         | source | target                                       |
      | github-token | token  | /run/itos-container/credentials/github-token |
    When itos-container runs with "doctor --credentials-only --no-json" and the environment "ITOS_JSON=1"
    Then it exits with code 0
    And its plain credential output lists exactly the requested grants
    And its standard output says "Credentials only"
    And its standard output says "Credential contents, manifest, engine and approval were not checked"
    And the engine recorder has no connections

  # New Givens change only the selected policy/version and remove the fixture's
  # otherwise empty managed state root, retaining private XDG_STATE_HOME. They
  # do not call a product command or open the person's real state directory.
  @wip @slice-29 @ID-CREDENTIALS-27 @linux
  Scenario Outline: metadata mode with no grants creates no state and inspects no sources
    Given a private credential inspection fixture
    And the credential fixture policy version is <version> with no credentials section
    And the credential fixture protected state root is absent
    And credential inspection effects are snapshotted
    When itos-container runs with "doctor --credentials-only --json"
    Then it exits with code 0
    And its credential JSON reports exactly these grants and no problems:
      | kind | source | target |
    And credential inspection has no side effects or helper execution
    And the engine recorder has no connections

    Examples:
      | version |
      | 1       |
      | 2       |
      | 3       |

  # A string-prefix check would reject these safe siblings. The sibling sources
  # are distinct private directories/inodes, never symlinks or mount aliases.
  @wip @slice-29 @ID-CREDENTIALS-28 @linux
  Scenario Outline: a sibling path sharing a protected name prefix is not inside that tree
    Given a private credential inspection fixture
    And credential source "token" has layout "<layout>"
    And the credential fixture requests these sources:
      | kind         | source |
      | claude-token | token  |
    When itos-container runs with "doctor --credentials-only --json"
    Then it exits with code 0
    And its credential JSON reports exactly these grants and no problems:
      | kind         | source | target                                       |
      | claude-token | token  | /run/itos-container/credentials/claude-token |
    And the engine recorder has no connections

    Examples:
      | layout                                                   |
      | private source in checkout-name-suffix sibling directory |
      | private source in state-name-suffix sibling directory    |
