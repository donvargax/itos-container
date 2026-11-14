@phase-1
Feature: Named credential requests and read-only source inspection
  A policy names host files; it contains no credential value and grants no access.
  A person can inspect the request and its source metadata without reading secrets,
  executing checkout code, contacting an engine or recording approval.

  # Authorities: docs/charter.md, docs/architecture.md (Slices), CONFIG.md,
  # CLI.md and T-47's docs/decisions/0025-credentials-passed-in.md, now landed.
  # Revised after the review of f2b0e58. Slice-25's version 2 and helpers remain
  # prerequisites; this spec does not copy their code.
  # Both T-47 and slice-25 have landed; no dependency change is needed.
  # docs/decisions/0027-credential-source-inspection.md amends T-47's source
  # location rule and increment table, with the corresponding arc42 changes.
  # This revision withdraws the shared-state traversal and supplies its bounded
  # mount-projection replacement and race argument in BOTH decisions and arc42.
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
  # Decision 0027 resolves a scope omission in T-47's increment table:
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
  # approval. It reads that policy only through a pin of the effective root, the
  # checkout the sources are excluded from, never from another working directory
  # (ID-CREDENTIALS-56 to 58). It needs no manifest, Git helper, image or engine.
  # On Linux amd64,
  # reuse the readiness source observation T-47 requires; no separate validator.
  # Observe each source in written order, collect one first failure per source:
  # 1. O_PATH/O_NOFOLLOW at every component; refuse any symlink, dangling included.
  # 2. Every ancestor is root- or host-owned, with no group/other write bits unless
  #    root-owned and sticky, matching approval/disk.directory's ancestor rule.
  # 3. Compare the source and every source ancestor's device/inode with BOTH
  #    protected roots AND every entry in the CHECKOUT. A match is location refusal.
  #    Enumerate the pinned checkout completely,
  #    including .git, hidden, ignored and untracked entries. Cross directory mount
  #    points and inspect mounted file identities; neither device changes nor
  #    repeated directory identities permit skipping entries. Do not follow
  #    symlinks or open file data. Pin directories and check identity when opening
  #    each child. A failed/incomplete enumeration or observed replacement refuses
  #    with unavailable, never a safe result from the partial inventory. A known
  #    identity match still reports location even if another subtree is unavailable.
  #    Reuse readiness's bounded traversal primitives: batches of at most 128,
  #    100000 checkout entries, directory depth 64 from its root
  #    (depth 0), and one 10-second monotonic deadline across ALL source observation. Count/depth
  #    limits are inclusive; exceeding them or reaching the deadline refuses.
  #    Time checks between synchronous calls do not preempt blocked kernel calls.
  #    No background scan continues after refusal, and no earlier inventory is reused.
  #    This detects bind aliases of protected descendants and individual files
  #    even when source ancestry contains neither root and link count is one.
  #    It also detects an outside source bound INTO the checkout. It does
  #    not reject an unrelated inode just for sharing a pathname prefix or device.
  #    In BOTH mount observations, also compare every source/ancestor coordinate
  #    to the validated checkout coordinate subtree, using the same device and
  #    component ancestry. A match is credential.location. The non-recursive
  #    worker bind exposes checkout backing directories/files hidden under host
  #    submounts, so inventory of the host-visible entries alone is insufficient.
  #    For STATE, never enumerate or stat unrelated records. Decision 0027's
  #    mount projections reserve the state root's filesystem-coordinate subtree
  #    and the roots of all mounts at/below its path, including file mounts,
  #    different devices and covered backing subtrees. A descriptor's coordinate
  #    uses its statx STATX_MNT_ID, that row's decoded root and relative pathname.
  #    q-57 drops st_dev equality with the row's device. Capture descriptor mount
  #    IDs before mountinfo and re-read them afterwards; a change refuses.
  #    Device plus component ancestry determines membership, not path-prefix or
  #    device alone. This catches state descendants bound outside and outside
  #    sources bound into state, even with one link and neither root in ancestry.
  #    Rewalk descriptor paths with no-follow and compare device/inode/mount ID.
  #    Take complete mount tables before and after, recheck pins and mappings,
  #    and refuse unavailable/changed relevant evidence, never retry to obtain
  #    a pass. Unrelated mounts/record writes do not invalidate the observation.
  #    Non-path mount roots (nsfs net/mnt handles and //deleted bind roots)
  #    remain in the table as unprojectable, with no filesystem coordinate.
  #    Refuse unavailable when a held source/boundary descriptor uses one or
  #    its mountpoint is at, below or covering protected state; ignore unrelated
  #    unprojectable rows when judging an independent source.
  #    The coordinator's 2026-11-10 04:42 UTC decision also refuses overlay
  #    and every FUSE filesystem (fuse, fuse.*, fuseblk, fuseblk.*): mountinfo does not map their backing paths.
  #    A source/ancestor descriptor using one, or a row at, below or covering
  #    checkout or either state root, refuses with credential.filesystem (3).
  #    Check both observations, including covered rows and absent state roots.
  #    Ignore unrelated stacking rows; do not parse lowerdir/upperdir options.
  #    After the review of 8e5088e the same refusal covers nfs and nfs4, matched
  #    exactly: the kernel prints "/" as the root of every NFS mount row, so a
  #    row cannot place a second mount of an export subdirectory in its tree.
  #    Known identity/location matches keep precedence. q-57 still governs
  #    descriptor mount identity; no st_dev equality requirement is restored.
  #    Defer any initial table error to the location check, preserving earlier
  #    source symlink/ancestor refusal precedence.
  #    Either complete observation's source match is location; otherwise require
  #    two complete exclusions and unchanged source/boundary mappings/identities.
  #    Each table is at most 8 MiB, 16384 rows, 65535 bytes per line excluding LF;
  #    EOF and the final LF are required. Paths are at most 4096 bytes, joined
  #    coordinates 8192, fdinfo 64 KiB with EOF. Bounds are inclusive, shared by
  #    both grants, and overflow/malformed/missing evidence is unavailable.
  #    These are topology limits; state history never consumes the checkout cap.
  #    An absent root reserves its projected coordinate under a pinned safe
  #    ancestor. Safe first creation is accepted after final validation at that
  #    same coordinate. A replaced existing root retains shared boundary refusal.
  #    The checkout remains the only authorized worker host directory mount.
  # 4. Require a regular host-UID-owned file, no group/other permission bit,
  #    one link, and size 1..65536 inclusive, in that order. Owner read permission
  #    is not required for this metadata-only mode; delivery checks are later work.
  #    Recheck held source metadata before success. An extra link in state or
  #    outside both trees is credential.links without any state enumeration.
  # 5. A source device/inode already used by another valid grant is refused.
  # An absent protected state root is not created. An inaccessible existing
  # boundary refuses; an unreadable unrelated STATE descendant is immaterial.
  # Existing state-location/identity failures retain their shared rules and exits.
  # That includes a selected state location that is not valid UTF-8, refused as
  # approval.state.location (2) with a relative one, before any path is opened.
  # Checkout/state overlap in either direction, including bind coordinates and
  # pinned identities, is state.path before any scan. Compare checkout and its
  # descendant mounts with every reserved state subtree; never enumerate an alias.
  # Unavailable boundary mapping evidence prevents inventory. Validate the checkout
  # descriptor before testing stacking rows, preserving mount_id/unavailable rules.
  # Absolute ITOS_ROOT paths are cleaned like the shared checkout pin; source
  # literals keep their stricter lexical rules and are never cleaned.
  # The mode does not establish approval readiness or inspect another project's
  # admission history or record contents. Normal publication/removal does not
  # change the reserved subtrees or an independent source's metadata; no record
  # disappearance can invalidate an inventory because none is made for state.
  # Slice-21's controls still govern later approval and run. With no grants there
  # is no source/exposure scan. Later consumers re-observe before use: this is an
  # observation, not an atomic snapshot or continuous protection against trusted
  # host mount/rename/link changes between or after the checks. Later delivery
  # reads the retained checked descriptor, never a fresh source pathname.
  #
  # Source rules are located at /credentials/<written-index>/source in the policy.
  # credential.missing (3): a source or ordinary ancestor is absent.
  # credential.unavailable (3): unavailable metadata, checkout scan or state projection.
  # credential.mount_id (3): unavailable STATX_MNT_ID; name it and require kernel support.
  # credential.filesystem (3): overlay/FUSE backing aliases cannot be excluded.
  #   The same rule refuses paths on NFS, whose mount rows hide the mounted subdirectory.
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
  # standard library. Select github.com/moby/sys/mountinfo v0.7.2 for its bounded-
  # reader-compatible kernel-format parser; preserve its Apache-2.0 notices.
  # Pin it during implementation, not this spec; no new service or custom parser.
  # Validate its numeric/identity output and supply no early-stopping filter.
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
  # CHECKOUT descendants, metadata failure, directory substitution and checkout
  # entry/depth/time limits at and beyond the bounds. State contracts instead
  # cover mount projection, complete topology bounds, concurrent publication and
  # removal, unreadable unrelated records and >100000 retained entries. All
  # exact topology bounds must succeed before their one-over-limit refusals.
  # Include escaped mount paths, nested mounts, covered backing subtrees, absent
  # state safely created during inspection, unrelated mount changes and a final
  # source stat observing a newly added state hard link (credential.links).
  # Alias results must be location; incomplete required observations unavailable,
  # with a known match taking precedence. Do not precompute decisions in fixtures.
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
  # 0027 and its arc42 amendment are part of this spec, not deferred to that PR.
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
  @slice-29 @ID-CREDENTIALS-01
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
  @slice-29 @ID-CREDENTIALS-02
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

  @slice-29 @ID-CREDENTIALS-03
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

  @slice-29 @ID-CREDENTIALS-04
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

  @slice-29 @ID-CREDENTIALS-05
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

  @slice-29 @ID-CREDENTIALS-06
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

  @slice-29 @ID-CREDENTIALS-07
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
  @slice-29 @ID-CREDENTIALS-08
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

  @slice-29 @ID-CREDENTIALS-09
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
  @slice-29 @ID-CREDENTIALS-10
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

  @slice-29 @ID-CREDENTIALS-11
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
  @slice-29 @ID-CREDENTIALS-12
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

  # Missing-source success in 12 cannot detect metadata calls whose errors are
  # discarded. Reuse 29/31's raw observation fixture and per-invocation call log
  # to prove both file-only scopes leave credential observation untouched.
  # The new manifest Given supplies .devcontainer/devcontainer.json with the
  # existing pinned fixture image and valid workspace observations, and updates
  # only the policy's manifest path. It preserves the v3 request, source, topology
  # and selected failure; it neither invokes validation nor supplies its result.
  # The new parameterized When passes the literal doctor argv through the real
  # scope dispatcher in the same cached test-only command assembly as 29/31.
  # All scopes receive the same ports; the runner must not omit the observer or
  # choose a validator/presenter instead of letting the command select its scope.
  # Reset call logs at command entry, never in Then. Counts cover failed calls too.
  # The new scope Then checks schema:1, ok:true, the named scope and problems:[].
  # For policy it compares the complete v3 effective policy, including the literal
  # requested source; for manifest it checks the selected path, valid:true and
  # the fixture's complete effective manifest, without inventing a grants field.
  # No-source-metadata excludes every source path stat/open and source ancestor
  # walk. No-checkout-traversal excludes the exposure inventory, not the policy
  # and selected manifest/workspace reads that file-only inspection requires.
  # No-state-mount-table means no topology read. The credentials-only invocation
  # then proves the unchanged fault and both observers are reachable; merely
  # discarding observation errors in a file-only mode cannot pass the zero counts.
  # Today each row fails at the first exit-0 assertion: version 3 is unsupported.
  # The implementer adds these steps in the red-first test commit, not this spec.
  @slice-29 @ID-CREDENTIALS-36 @linux
  Scenario Outline: file-only scopes never observe requested credential sources or exposure
    Given a credential observation fixture for "<boundary>" with "<layout>"
    And credential observation includes a valid selected manifest
    When the credential observation contract runs "doctor <flag> --json"
    Then it exits with code 0
    And its credential observation JSON reports a valid "<scope>" inspection
    And the credential observation recorded no source metadata checkout traversal or state mount-table calls
    When the credential observation contract runs in JSON mode
    Then its credential JSON reports exactly these source problems and retains the requested grants:
      | rule                   | path                  | exit |
      | credential.unavailable | /credentials/0/source | 3    |
    And it exits with code 3
    And the credential observation recorded "source metadata" calls
    And the credential observation recorded "<operation>" calls
    And the engine recorder has no connections

    Examples:
      | flag            | scope    | boundary        | layout                  | operation          |
      | --policy-only   | policy   | checkout        | unreadable descendant   | checkout traversal |
      | --policy-only   | policy   | protected state | unavailable mount table | state mount-table  |
      | --manifest-only | manifest | checkout        | unreadable descendant   | checkout traversal |
      | --manifest-only | manifest | protected state | unavailable mount table | state mount-table  |

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

  @slice-29 @ID-CREDENTIALS-13 @linux
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

  @slice-29 @ID-CREDENTIALS-14
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

  @slice-29 @ID-CREDENTIALS-15
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
  # The selected manifest is absent: successful source inspection must not
  # depend on manifest validation, even when both grants are requested.
  @slice-29 @ID-CREDENTIALS-16 @linux
  Scenario: metadata inspection shows both grants without reading or retaining either token
    Given a private credential inspection fixture
    And credential source "github" has layout "private regular file"
    And credential source "claude" has layout "private regular file"
    And the credential fixture requests these sources:
      | kind         | source |
      | github-token | github |
      | claude-token | claude |
    And the credential fixture manifest is absent
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
  @slice-29 @ID-CREDENTIALS-17 @linux
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
  @slice-29 @ID-CREDENTIALS-18 @linux
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
  @slice-29 @ID-CREDENTIALS-19 @linux
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
  @slice-29 @ID-CREDENTIALS-20 @linux
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
  # State topology supplies actual mountinfo text, descriptor paths/mount IDs,
  # per-component metadata and before/after observations, never a computed set
  # of protected coordinates. The production adapter parser and projection code
  # must participate. A root's device can differ from a mounted descendant's.
  # New When extends the existing test-only cmd/itos-container assembly pattern:
  # real policy parser, readiness source validator, doctor handler and presenter,
  # with an I/O-free metadata/topology fake beneath validation. The runner sends
  # raw fixture input and observes output; it imports no product package. Fixtures
  # cannot inject a ready result or a preselected credential failure. Use the same
  # cases in the fake/disk shared contract via adapter-local observation seams.
  # Reuse the compiled contract binary and its per-scenario coverage handling.
  # The first invocation proves that the new mode accepts distinct identities;
  # the second must reject the alias with the exact located diagnostic.
  @slice-29 @ID-CREDENTIALS-29 @linux
  Scenario Outline: metadata inspection refuses bind aliases of protected descendants and files
    Given a credential observation fixture for "<boundary>" with "distinct outside file"
    When the credential observation contract runs in JSON mode
    Then it exits with code 0
    And its credential JSON reports exactly these grants and no problems:
      | kind         | source | target                                       |
      | claude-token | token  | /run/itos-container/credentials/claude-token |
    Given a credential observation fixture for "<boundary>" with "<layout>"
    When the credential observation contract runs in JSON mode
    Then its credential JSON reports exactly these source problems and retains the requested grants:
      | rule                | path                  | exit |
      | credential.location | /credentials/0/source | 1    |
    And it exits with code 1
    And the engine recorder has no connections

    Examples:
      | boundary        | layout                                            |
      | checkout        | root directory bound outside                      |
      | checkout        | descendant directory bound outside                |
      | checkout        | single descendant file bound outside              |
      | checkout        | outside directory bound into a descendant         |
      | checkout        | outside file bound into a descendant              |
      | protected state | root directory bound outside                      |
      | protected state | descendant directory bound outside                |
      | protected state | single descendant file bound outside              |
      | protected state | outside directory bound into a descendant         |
      | protected state | outside file bound into a descendant              |
      | protected state | descendant on another device bound outside        |
      | protected state | descendant with escaped mount paths bound outside |
      | protected state | covered state backing directory bound outside     |

  # New Given makes one empty private descendant, then removes all its permission
  # bits. The native non-root fixture retains its setup inventory; snapshots treat
  # that directory as metadata-only and cleanup restores access after assertions.
  # Only checkout enumeration is required. State's old row mandated the withdrawn
  # traversal; 35 below replaces it with the correct independent-source success.
  @slice-29 @ID-CREDENTIALS-30 @linux
  Scenario: an incomplete checkout scan cannot declare a source outside it
    Given a private credential inspection fixture
    And credential source "token" has layout "private regular file"
    And the credential fixture requests these sources:
      | kind         | source |
      | claude-token | token  |
    And the credential fixture "checkout" has an unreadable descendant
    And credential inspection effects are snapshotted
    When itos-container runs with "doctor --credentials-only --json"
    Then its credential JSON reports exactly these source problems and retains the requested grants:
      | rule                   | path                  | exit |
      | credential.unavailable | /credentials/0/source | 3    |
    And it exits with code 3
    And credential inspection has no side effects or helper execution
    And the engine recorder has no connections

  @slice-29 @ID-CREDENTIALS-35 @linux
  Scenario: unreadable unrelated state records cannot block a distinct private source
    Given a private credential inspection fixture
    And credential source "token" has layout "private regular file"
    And the credential fixture requests these sources:
      | kind         | source |
      | claude-token | token  |
    And the credential fixture "protected state" has an unreadable descendant
    And credential inspection effects are snapshotted
    When itos-container runs with "doctor --credentials-only --json"
    Then it exits with code 0
    And its credential JSON reports exactly these grants and no problems:
      | kind         | source | target                                     |
      | claude-token | token  | /run/itos-container/credentials/claude-token |
    And credential inspection has no side effects or helper execution
    And the engine recorder has no connections

  # New activity Given schedules another project's raw state operations after
  # acquiring the first mount table but before returning that observation to the
  # validator. Publication writes a temp
  # file then renames it over an approval, intent or snapshot; cleanup unlinks a
  # session record. A barrier holds that first observation until the writes finish,
  # so the writer runs inside inspection even if a known alias refuses early.
  # The combined activity also removes a temp file before a hypothetical stat.
  # The Then checks the operation log and that the barrier was reached inside the
  # command, not before it. No Given invokes a command or asserts its result.
  # Repeat each activity with a state alias so ignoring state cannot pass.
  # The fake schedules raw metadata changes; the shared disk contract performs
  # these same writes in isolated temporary state through adapter-local barriers.
  # Do not require unchanged unrelated records or modify the production binary.
  @slice-29 @ID-CREDENTIALS-32 @linux
  Scenario Outline: another project's state publications and cleanup do not block inspection or hide aliases
    Given a credential observation fixture for "protected state" with "distinct outside file"
    And credential observation schedules unrelated state activity "<activity>"
    When the credential observation contract runs in JSON mode
    Then it exits with code 0
    And its credential JSON reports exactly these grants and no problems:
      | kind         | source | target                                       |
      | claude-token | token  | /run/itos-container/credentials/claude-token |
    And the credential observation completed the scheduled state activity during inspection
    And the credential observation recorded no state record enumeration or metadata visits
    Given a credential observation fixture for "protected state" with "<alias>"
    And credential observation schedules unrelated state activity "<activity>"
    When the credential observation contract runs in JSON mode
    Then its credential JSON reports exactly these source problems and retains the requested grants:
      | rule                | path                  | exit |
      | credential.location | /credentials/0/source | 1    |
    And it exits with code 1
    And the credential observation completed the scheduled state activity during inspection
    And the credential observation recorded no state record enumeration or metadata visits
    And the engine recorder has no connections

    Examples:
      | activity                                  | alias                                     |
      | approval publication                      | descendant directory bound outside        |
      | intent publication                        | single descendant file bound outside      |
      | snapshot publication                      | outside directory bound into a descendant |
      | session record cleanup                    | outside file bound into a descendant      |
      | publication and disappearing temp entries | descendant directory bound outside        |

  # New retained-history Given supplies 100001 ordinary unrelated records under
  # other projects, without changing the mount table. Enumerations are available
  # as lazy batches, not a precomputed count returned to the validator; a caller
  # doing the withdrawn scan would visit them and exceed its budget. The Then
  # verifies the setup count and zero product visits. Run against fake and disk
  # observation seams; do not allocate a 100001-entry object graph or capture its
  # contents for each row. This is observation-contract, not native mount proof.
  @slice-29 @ID-CREDENTIALS-33 @linux
  Scenario Outline: retained unrelated state history does not consume inspection budgets or hide aliases
    Given a credential observation fixture for "protected state" with "distinct outside file"
    And the credential observation state retains 100001 unrelated entries
    When the credential observation contract runs in JSON mode
    Then it exits with code 0
    And its credential JSON reports exactly these grants and no problems:
      | kind         | source | target                                       |
      | claude-token | token  | /run/itos-container/credentials/claude-token |
    And the credential observation left all 100001 unrelated entries unvisited
    Given a credential observation fixture for "protected state" with "<alias>"
    And the credential observation state retains 100001 unrelated entries
    When the credential observation contract runs in JSON mode
    Then its credential JSON reports exactly these source problems and retains the requested grants:
      | rule                | path                  | exit |
      | credential.location | /credentials/0/source | 1    |
    And it exits with code 1
    And the credential observation left all 100001 unrelated entries unvisited
    And the engine recorder has no connections

    Examples:
      | alias                                     |
      | descendant directory bound outside        |
      | single descendant file bound outside      |
      | outside directory bound into a descendant |
      | outside file bound into a descendant      |

  # Raw evidence faults are inputs, not canned validator results. Table limits
  # use generated mount rows/bytes; the final-table fault is injected only on
  # the second read. Descriptor/path changes keep a safe file at the old path
  # so identity revalidation, not a missing-file diagnostic, must catch them.
  # "source mount mapping changed" changes its root without making a known
  # location match. Complete-data controls include exactly every topology bound,
  # safe first state creation and unrelated mount churn in the shared contract.
  @slice-29 @ID-CREDENTIALS-34 @linux
  Scenario Outline: incomplete or inconsistent namespace evidence cannot establish state exclusion
    Given a credential observation fixture for "protected state" with "distinct outside file"
    When the credential observation contract runs in JSON mode
    Then it exits with code 0
    And its credential JSON reports exactly these grants and no problems:
      | kind         | source | target                                       |
      | claude-token | token  | /run/itos-container/credentials/claude-token |
    Given a credential observation fixture for "protected state" with "distinct outside file"
    And credential observation has namespace evidence fault "<fault>"
    When the credential observation contract runs in JSON mode
    Then its credential JSON reports exactly these source problems and retains the requested grants:
      | rule                   | path                  | exit |
      | credential.unavailable | /credentials/0/source | 3    |
    And it exits with code 3
    And the credential observation recorded no state record enumeration or metadata visits
    And the engine recorder has no connections

    Examples:
      | fault                                   |
      | mount table unreadable                  |
      | final mount table unreadable            |
      | truncated final mount row               |
      | malformed mount row                     |
      | duplicate mount ID                      |
      | descriptor mount row missing            |
      | source pathname now names another inode |
      | source mount mapping changed            |
      | 16385 mount rows                        |
      | mount table exceeds 8 MiB               |
      | mount row exceeds 65535 bytes           |
      | fdinfo mount ID unavailable             |
      | observation deadline reached            |

  # The root-owned sticky /tmp ancestor of every private fixture is a positive
  # control for the inherited directory rule. User-owned sticky is no exception.
  # Parent modes are 0720, 0702 and 01777 respectively; only fixture paths change.
  @slice-29 @ID-CREDENTIALS-21 @linux
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
  @slice-29 @ID-CREDENTIALS-22 @linux
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
  @slice-29 @ID-CREDENTIALS-23 @linux
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
      | layout                                             | rule                |
      | private source with a hard link in checkout        | credential.location |
      | private source with a hard link in protected state | credential.links    |
      | private source with a hard link outside checkout   | credential.links    |
      | private regular file of zero bytes                 | credential.size     |
      | private regular file of 65537 bytes                | credential.size     |

  @slice-29 @ID-CREDENTIALS-24 @linux
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
  @slice-29 @ID-CREDENTIALS-25 @linux
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
  @slice-29 @ID-CREDENTIALS-26 @linux
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
  # Reuse 30's unreadable descendant and metadata-only snapshot setup. With no
  # grants, that unrelated checkout directory must not prevent success. The
  # observation contract in 31 also rejects a scan whose errors are discarded.
  @slice-29 @ID-CREDENTIALS-27 @linux
  Scenario Outline: metadata mode with no grants creates no state and inspects no sources
    Given a private credential inspection fixture
    And the credential fixture policy version is <version> with no credentials section
    And the credential fixture protected state root is absent
    And the credential fixture "checkout" has an unreadable descendant
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

  # Extend 29's raw observation fixture: checkout's "unreadable descendant"
  # supplies an entry-read error; state's "unavailable mount table" supplies a
  # procfs read error, never an unrelated record failure. Roots remain pinnable.
  # Supply raw observations/errors, never a credential decision.
  # The new policy-version Given changes only the version and removes credentials;
  # it preserves topology, source file and the selected observation failure.
  # The same real command assembly runs both invocations. Its test-only observer
  # records source metadata, checkout traversal and state mount-table calls below
  # validation, including failed calls, from command entry through return. Record
  # each operation separately and reset logs before each invocation, not in Then.
  # The positive control must record the selected operation; with no grants,
  # require zero calls in all three categories. Root identity pinning is allowed;
  # directory-entry reads, descendant walks and mount-table reads are not.
  # Export observations through test-only output separate from the
  # product JSON, and run the same cases against the fake/disk observation contract.
  @slice-29 @ID-CREDENTIALS-31 @linux
  Scenario Outline: no-grant inspection makes no source checkout scan or state projection calls
    Given a credential observation fixture for "<boundary>" with "<layout>"
    When the credential observation contract runs in JSON mode
    Then its credential JSON reports exactly these source problems and retains the requested grants:
      | rule                   | path                  | exit |
      | credential.unavailable | /credentials/0/source | 3    |
    And it exits with code 3
    And the credential observation recorded "<operation>" calls
    Given the credential observation policy version is <version> with no credentials section
    When the credential observation contract runs in JSON mode
    Then it exits with code 0
    And its credential JSON reports exactly these grants and no problems:
      | kind | source | target |
    And the credential observation recorded no source metadata checkout traversal or state mount-table calls
    And the engine recorder has no connections

    Examples:
      | boundary        | layout                  | operation          | version |
      | checkout        | unreadable descendant   | checkout traversal | 1       |
      | checkout        | unreadable descendant   | checkout traversal | 2       |
      | checkout        | unreadable descendant   | checkout traversal | 3       |
      | protected state | unavailable mount table | state mount-table  | 1       |
      | protected state | unavailable mount table | state mount-table  | 2       |
      | protected state | unavailable mount table | state mount-table  | 3       |

  # A string-prefix check would reject these safe siblings. The sibling sources
  # are distinct private directories/inodes, never symlinks or mount aliases.
  @slice-29 @ID-CREDENTIALS-28 @linux
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

  # Review regressions use only raw topology and metadata. The covered root is
  # a different filesystem; the alias still reaches its hidden backing tree.
  @slice-29 @ID-CREDENTIALS-37 @linux
  Scenario: an overmounted state root still reserves its backing subtree
    Given a credential observation fixture for "protected state" with "covered state root backing directory bound outside"
    When the credential observation contract runs in JSON mode
    Then its credential JSON reports exactly these source problems and retains the requested grants:
      | rule                | path                  | exit |
      | credential.location | /credentials/0/source | 1    |
    And it exits with code 1

  @slice-29 @ID-CREDENTIALS-38 @linux
  Scenario Outline: the final source stat must complete before the observation deadline
    Given a credential observation fixture for "protected state" with "distinct outside file"
    And the final credential stat completes at <elapsed> nanoseconds
    When the credential observation contract runs in JSON mode
    Then its credential JSON reports exactly these source problems and retains the requested grants:
      | rule                   | path                  | exit |
      | credential.unavailable | /credentials/0/source | 3    |
    And it exits with code 3

    Examples:
      | elapsed     |
      | 10000000000 |
      | 10000000001 |

  @slice-29 @ID-CREDENTIALS-39 @linux
  Scenario Outline: credential inspection uses checkout ancestor permissions from the shared checkout pin
    Given a credential observation fixture for "checkout" with "distinct outside file"
    And the credential checkout parent is "<layout>"
    When the credential observation contract runs in JSON mode
    Then it exits with code 0
    And its credential JSON reports exactly these grants and no problems:
      | kind         | source | target                                     |
      | claude-token | token  | /run/itos-container/credentials/claude-token |

    Examples:
      | layout         |
      | group writable |
      | foreign owned  |

  @slice-29 @ID-CREDENTIALS-40 @linux
  Scenario Outline: credential checkout boundary refusals keep their shared diagnostic and subject
    Given a credential observation fixture for "checkout" with "distinct outside file"
    And the credential checkout parent is "<layout>"
    When the credential observation contract runs in JSON mode
    Then the credential boundary problem is "<rule>" at "/projects/checkout"
    And it exits with code <exit>

    Examples:
      | layout  | rule                        | exit |
      | symlink | doctor.checkout.path        | 1    |
      | missing | doctor.checkout.unavailable | 3    |

  @slice-29 @ID-CREDENTIALS-41 @linux
  Scenario: a separately selected state location cannot expose the fixed account control state
    Given a credential observation fixture for "protected state" with "fixed account state file"
    When the credential observation contract runs in JSON mode
    Then its credential JSON reports exactly these source problems and retains the requested grants:
      | rule                | path                  | exit |
      | credential.location | /credentials/0/source | 1    |
    And it exits with code 1

  @slice-29 @ID-CREDENTIALS-42
  Scenario: doctor help names the credential source refusal exit codes
    When itos-container runs with "doctor --help"
    Then it exits with code 0
    And its standard output says "unsafe credential source metadata or location"
    And its standard output says "missing credential source or unavailable credential evidence"

  # q-57 replaces the st_dev/mountinfo device equality with descriptor statx
  # mount IDs, sampled before and after mountinfo. Synthetic subvolume devices
  # differ at every pin, including checkout and state, not only at the source.
  @slice-29 @ID-CREDENTIALS-43 @linux
  Scenario: subvolume device numbers do not prevent credential mount mapping
    Given a credential observation fixture for "protected state" with "distinct outside file"
    And credential descriptors report subvolume devices distinct from mountinfo
    When the credential observation contract runs in JSON mode
    Then it exits with code 0
    And its credential JSON reports exactly these grants and no problems:
      | kind         | source | target                                     |
      | claude-token | token  | /run/itos-container/credentials/claude-token |
    And the credential observation recorded no state record enumeration or metadata visits

  @slice-29 @ID-CREDENTIALS-44 @linux
  Scenario: a descriptor mount ID cannot change across the mount-table read
    Given a credential observation fixture for "protected state" with "distinct outside file"
    And the credential source mount ID changes during the first mount-table read
    When the credential observation contract runs in JSON mode
    Then its credential JSON reports exactly these source problems and retains the requested grants:
      | rule                   | path                  | exit |
      | credential.unavailable | /credentials/0/source | 3    |
    And it exits with code 3

  @slice-29 @ID-CREDENTIALS-45 @linux
  Scenario: missing statx mount identity has an actionable diagnostic
    Given a credential observation fixture for "protected state" with "distinct outside file"
    And the credential source has no STATX_MNT_ID evidence
    When the credential observation contract runs in JSON mode
    Then its credential JSON reports exactly these source problems and retains the requested grants:
      | rule                | path                  | exit |
      | credential.mount_id | /credentials/0/source | 3    |
    And it exits with code 3
    And its error output says "STATX_MNT_ID"

  @slice-29 @ID-CREDENTIALS-46 @linux
  Scenario Outline: unrelated namespace and deleted mounts do not invalidate credential inspection
    Given a credential observation fixture for "protected state" with "distinct outside file"
    And credential mount tables include root "<root>" at "/run/namespaces/unrelated"
    When the credential observation contract runs in JSON mode
    Then it exits with code 0
    And its credential JSON reports exactly these grants and no problems:
      | kind         | source | target                                     |
      | claude-token | token  | /run/itos-container/credentials/claude-token |
    And the credential observation recorded no state record enumeration or metadata visits
    Given credential mount tables include root "<root>" at "/state/private/namespace"
    When the credential observation contract runs in JSON mode
    Then its credential JSON reports exactly these source problems and retains the requested grants:
      | rule                   | path                  | exit |
      | credential.unavailable | /credentials/0/source | 3    |
    And it exits with code 3

    Examples:
      | root               |
      | net:[4026537384]   |
      | mnt:[4026537385]   |
      | /replaced//deleted |

  @slice-29 @ID-CREDENTIALS-47 @linux
  Scenario Outline: checkout inside either state boundary is refused before inventory
    Given a credential observation fixture for "protected state" with "distinct outside file"
    And the credential checkout is inside "<boundary>" state
    When the credential observation contract runs in JSON mode
    Then its JSON output names the problem "approval.state.path"
    And it exits with code 1
    And the credential observation recorded no source metadata checkout traversal or state mount-table calls
    And the credential observation recorded no state record enumeration or metadata visits

    Examples:
      | boundary |
      | selected |
      | account  |

  @slice-29 @ID-CREDENTIALS-48 @linux
  Scenario Outline: credential inspection cleans absolute checkout paths like the shared checkout pin
    Given a private credential inspection fixture
    And credential source "token" has layout "private regular file"
    And the credential fixture requests these sources:
      | kind         | source |
      | claude-token | token  |
    And the credential checkout environment uses "<spelling>" spelling
    When itos-container runs with "doctor --credentials-only --json"
    Then it exits with code 0
    And its credential JSON reports exactly these grants and no problems:
      | kind         | source | target                                     |
      | claude-token | token  | /run/itos-container/credentials/claude-token |
    And the engine recorder has no connections

    Examples:
      | spelling         |
      | trailing slash   |
      | repeated slash   |
      | dot component    |
      | parent component |

  # Rewalks only revalidate a mapping; their temporary handles must not pile
  # up across source ancestors or observations. Original pins remain held.
  @slice-29 @ID-CREDENTIALS-49 @linux
  Scenario: a deeply nested credential source fits the host descriptor limit
    Given a credential observation fixture for "protected state" with "distinct outside file"
    And the credential source has 30 additional ancestor directories
    And the credential filesystem permits at most 1024 open handles
    When the credential observation contract runs in JSON mode
    Then it exits with code 0
    And its credential JSON reports exactly these grants and no problems:
      | kind         | source | target                                     |
      | claude-token | token  | /run/itos-container/credentials/claude-token |
    And the credential observation recorded no state record enumeration or metadata visits

  # Linux permits an empty mount source: exactly that field, following the
  # separator and filesystem type, can be empty. Other empty fields still fail.
  @slice-29 @ID-CREDENTIALS-50 @linux
  Scenario: an unrelated mount with an empty source does not block credential inspection
    Given a credential observation fixture for "protected state" with "distinct outside file"
    And credential mount tables include an unrelated mount with an empty source
    When the credential observation contract runs in JSON mode
    Then it exits with code 0
    And its credential JSON reports exactly these grants and no problems:
      | kind         | source | target                                     |
      | claude-token | token  | /run/itos-container/credentials/claude-token |
    And the credential observation recorded no state record enumeration or metadata visits

  # Raw stacking observations reproduce the leak without mounting anything:
  # the lower source and merged view share an inode but have different devices.
  @slice-29 @ID-CREDENTIALS-51 @linux
  Scenario Outline: credential inspection refuses unprojectable stacking filesystems
    Given a credential observation fixture for "protected state" with "distinct outside file"
    And credential observation exposes the source through "<filesystem>" at "<point>"
    When the credential observation contract runs in JSON mode
    Then its credential JSON reports exactly these source problems and retains the requested grants:
      | rule                  | path                  | exit |
      | credential.filesystem | /credentials/0/source | 3    |
    And it exits with code 3
    And its standard output says "Overlay or FUSE backing paths cannot be excluded from credential exposure"
    And the credential observation recorded no state record enumeration or metadata visits

    Examples:
      | filesystem  | point                                      |
      | overlay     | /checkout                                  |
      | fuse.bindfs | /checkout                                  |
      | overlay     | /state                                     |
      | fuse.bindfs | /state                                     |
      | overlay     | /account/.local/state/itos-container        |
      | fuse.bindfs | /account/.local/state/itos-container        |
      | overlay     | /checkout-sources                          |
      | fuse.bindfs | /checkout-sources                          |
      | overlay     | /checkout/private                          |
      | fuse.bindfs | /checkout/private                          |
      | overlay     | /state/private                             |
      | fuse.bindfs | /state/private                             |

  @slice-29 @ID-CREDENTIALS-54 @linux
  Scenario Outline: credential inspection refuses FUSE types with and without subtypes
    Given a credential observation fixture for "protected state" with "distinct outside file"
    And credential observation exposes the source through "<filesystem>" at "<point>"
    When the credential observation contract runs in JSON mode
    Then its credential JSON reports exactly these source problems and retains the requested grants:
      | rule                  | path                  | exit |
      | credential.filesystem | /credentials/0/source | 3    |
    And it exits with code 3
    And its standard output says "Overlay or FUSE backing paths cannot be excluded from credential exposure"
    And the credential observation recorded no state record enumeration or metadata visits

    Examples:
      | filesystem  | point                               |
      | fuse        | /checkout                           |
      | fuseblk     | /checkout                           |
      | fuseblk.ntfs | /checkout                          |
      | fuse        | /state                              |
      | fuseblk     | /state                              |
      | fuseblk.ntfs | /state                             |
      | fuse        | /account/.local/state/itos-container |
      | fuseblk     | /account/.local/state/itos-container |
      | fuseblk.ntfs | /account/.local/state/itos-container |
      | fuse        | /checkout-sources                   |
      | fuseblk     | /checkout-sources                   |
      | fuseblk.ntfs | /checkout-sources                  |
      | fuse        | /checkout/private                   |
      | fuseblk     | /checkout/private                   |
      | fuseblk.ntfs | /checkout/private                  |
      | fuse        | /state/private                      |
      | fuseblk     | /state/private                      |
      | fuseblk.ntfs | /state/private                     |

  # Bind coordinates and pinned identities must establish the boundary before
  # inventory. The checkout spelling must never enumerate protected records.
  @slice-29 @ID-CREDENTIALS-52 @linux
  Scenario Outline: aliased checkout and state overlap is refused before inventory
    Given a credential observation fixture for "protected state" with "distinct outside file"
    And the credential checkout aliases "<boundary>" state through "<layout>"
    When the credential observation contract runs in JSON mode
    Then its JSON output names the problem "approval.state.path"
    And it exits with code 1
    And the credential observation recorded no checkout traversal
    And the credential observation recorded no state record enumeration or metadata visits

    Examples:
      | boundary | layout           |
      | selected | root bind        |
      | selected | descendant bind  |
      | selected | ancestor bind    |
      | selected | descendant mount |
      | selected | inverse bind     |
      | account  | root bind        |
      | account  | descendant bind  |
      | account  | ancestor bind    |
      | account  | descendant mount |
      | account  | inverse bind     |

  @slice-29 @ID-CREDENTIALS-53 @linux
  Scenario Outline: unrelated stacking mounts do not hide missing descriptor evidence
    Given a credential observation fixture for "protected state" with "distinct outside file"
    And credential descriptors have "<fault>" beside an unrelated "<filesystem>" mount
    When the credential observation contract runs in JSON mode
    Then its credential JSON reports exactly these source problems and retains the requested grants:
      | rule   | path                  | exit |
      | <rule> | /credentials/0/source | 3    |
    And it exits with code 3
    And the credential observation recorded no state record enumeration or metadata visits

    Examples:
      | fault                           | filesystem  | rule                   |
      | missing mount ID                | overlay     | credential.mount_id    |
      | missing mount ID                | fuse.portal | credential.mount_id    |
      | unavailable checkout descriptor | overlay     | credential.unavailable |
      | unavailable checkout descriptor | fuse.portal | credential.unavailable |

  # A non-recursive checkout bind exposes backing entries hidden from the host
  # inventory by submounts. The outside alias has one link and no shared root
  # ancestor. Only filesystem coordinates reveal its checkout membership.
  # First/final rows move the alias mapping outside the checkout in the other
  # observation; either complete observation's match must keep location precedence.
  @slice-29 @ID-CREDENTIALS-55 @linux
  Scenario Outline: covered checkout entries remain excluded from credential sources
    Given a credential observation fixture for "checkout" with "distinct outside file"
    When the credential observation contract runs in JSON mode
    Then it exits with code 0
    And its credential JSON reports exactly these grants and no problems:
      | kind         | source | target                                     |
      | claude-token | token  | /run/itos-container/credentials/claude-token |
    Given a credential observation fixture for "checkout" with "distinct outside file"
    And the credential source aliases a covered checkout "<kind>" in the "<observation>" observation
    When the credential observation contract runs in JSON mode
    Then its credential JSON reports exactly these source problems and retains the requested grants:
      | rule                | path                  | exit |
      | credential.location | /credentials/0/source | 1    |
    And it exits with code 1
    And the credential observation recorded "checkout traversal" calls
    And the credential observation recorded no state record enumeration or metadata visits
    And the engine recorder has no connections

    Examples:
      | kind      | observation |
      | directory | both        |
      | file      | both        |
      | directory | first       |
      | file      | first       |
      | directory | final       |
      | file      | final       |

  # Review of 027305f (the thread at cmd/itos-container/main.go:144): the policy
  # and the checkout boundary must name one tree. The effective root is absolute
  # ITOS_ROOT when set, otherwise the working directory, as ordinary doctor
  # resolves it. This mode pins that root with the shared checkout pin, reads
  # itos-container.yaml only through the pin, as ordinary doctor does, and holds
  # the pin until the source observation returns. It never opens the policy of a
  # different working directory, so one checkout's requests are never judged
  # against another tree. --policy-only and --manifest-only keep reading the
  # working directory: they observe no source and have no second root.
  # A root that cannot be pinned refuses before any policy is read with the
  # shared doctor.checkout.root (2), .unavailable (3), .path (1) or .changed (1)
  # diagnostic, its path the root as given, in the credentials envelope with
  # credentials [] because no request is known. This holds with and without
  # grants: pinning the root identity is not a source or exposure scan.
  # The new Given creates a sibling folder outside checkout, sources and state,
  # writes there a valid version 3 policy requesting a private 0600 one-link
  # file of that folder, and makes it the child's working directory only.
  # ITOS_ROOT and the checkout's own policy are untouched. Before the fix that
  # request was inspected against the checkout and accepted.
  @slice-29 @ID-CREDENTIALS-56 @linux
  Scenario: credential inspection reads the effective root's policy from another working directory
    Given a private credential inspection fixture
    And credential source "token" has layout "private file at checkout root"
    And the credential fixture requests these sources:
      | kind         | source |
      | claude-token | token  |
    And the credential command runs in another directory with its own policy and private source
    And credential inspection effects are snapshotted
    When itos-container runs with "doctor --credentials-only --json"
    Then its credential JSON reports exactly these source problems and retains the requested grants:
      | rule                | path                  | exit |
      | credential.location | /credentials/0/source | 1    |
    And it exits with code 1
    And credential inspection has no side effects or helper execution
    And the engine recorder has no connections

  # The review's reproduction: only the working directory holds a policy.
  @slice-29 @ID-CREDENTIALS-57 @linux
  Scenario: an effective root without a policy never borrows the working directory's
    Given a private credential inspection fixture
    And the credential fixture checkout has no policy file
    And the credential command runs in another directory with its own policy and private source
    And credential inspection effects are snapshotted
    When itos-container runs with "doctor --credentials-only --json"
    Then the credential fixture command reports exactly "policy.missing" at "" with exit 2
    And credential inspection has no side effects or helper execution
    And the engine recorder has no connections

  # The root Given sets only ITOS_ROOT. "relative" is the checkout's literal
  # relative name from the working directory, "absent" an unused absolute name
  # beside the checkout, and "symbolic link" an absolute link to the checkout.
  # Each already reported the same rule, beside the working directory's request;
  # that request must now be absent because its policy is never read.
  @slice-29 @ID-CREDENTIALS-58 @linux
  Scenario Outline: an effective root that cannot be pinned refuses before any policy is read
    Given a private credential inspection fixture
    And the credential command runs in another directory with its own policy and private source
    And the credential effective root is "<root>"
    And credential inspection effects are snapshotted
    When itos-container runs with "doctor --credentials-only --json"
    Then the credential boundary problem is "<rule>" with exit <exit> at the effective root and no grant is reported
    And it exits with code <exit>
    And credential inspection has no side effects or helper execution
    And the engine recorder has no connections

    Examples:
      | root          | rule                        | exit |
      | relative      | doctor.checkout.root        | 2    |
      | absent        | doctor.checkout.unavailable | 3    |
      | symbolic link | doctor.checkout.path        | 1    |

  # Review of 8e5088e (the thread at credentials.go:59): the selected state
  # location was checked for being absolute, never for being valid UTF-8. Linux
  # opens such a path normally, so this mode reported success for a selection
  # that ordinary doctor refuses as approval.state.location (exit 2), where it
  # promises the shared state-location rules and exits. The selected state path
  # must be absolute and valid UTF-8 before any boundary is walked. Both
  # XDG_STATE_HOME and its HOME fallback are selections.
  # The new Given points the selection at an existing private tree, mode 0700
  # and owned by the user, whose path holds the byte 0xff: nothing but its
  # encoding can refuse it. Before the fix both examples exited 0.
  @slice-29 @ID-CREDENTIALS-59 @linux
  Scenario Outline: a selected state path that is not valid UTF-8 keeps the shared state-location refusal
    Given a private credential inspection fixture
    And credential source "token" has layout "private regular file"
    And the credential fixture requests these sources:
      | kind         | source |
      | claude-token | token  |
    And the credential fixture selects state through "<selection>" with a path that is not valid UTF-8
    And credential inspection effects are snapshotted
    When itos-container runs with "doctor --credentials-only --json"
    Then its JSON output names the problem "approval.state.location"
    And it exits with code 2
    And credential inspection has no side effects or helper execution
    And the engine recorder has no connections

    Examples:
      | selection      |
      | XDG_STATE_HOME |
      | HOME fallback  |

  # Review of 8e5088e (the thread at credential_projection.go:207): a mount
  # row's root is the coordinate's backing path, and for NFS it is not a path.
  # The kernel prints "/" for every nfs and nfs4 row (nfs_show_path), whatever
  # export subdirectory the mount or bind is rooted at. A protected subdirectory
  # mounted a second time therefore projects beside its tree, not inside it,
  # and no pinned identity matches either: state is never enumerated, and a
  # checkout entry hidden under a host submount is never inventoried.
  # Refuse nfs and nfs4 with credential.filesystem (3) under the conditions of
  # overlay and FUSE: a source or source-ancestor descriptor on such a row, or
  # such a row at, below or covering checkout or either state root.
  # The new Given mounts one export at <point> and a second time, rooted at
  # <subdirectory>, over the source's parent directory. Both rows share the
  # export's device and report root "/". A checkout subdirectory is also
  # covered by a host submount, as in ID-CREDENTIALS-55, so the inventory sees
  # only the covering entry. Before the fix all six examples exited 0.
  @slice-29 @ID-CREDENTIALS-60 @linux
  Scenario Outline: credential inspection refuses NFS mounts, whose rows hide the mounted subdirectory
    Given a credential observation fixture for "protected state" with "distinct outside file"
    And the credential source is a second "<filesystem>" mount of "<subdirectory>" from the export mounted at "<point>"
    When the credential observation contract runs in JSON mode
    Then its credential JSON reports exactly these source problems and retains the requested grants:
      | rule                  | path                  | exit |
      | credential.filesystem | /credentials/0/source | 3    |
    And it exits with code 3
    And its standard output says "nor can paths on NFS"
    And the credential observation recorded no state record enumeration or metadata visits

    Examples:
      | filesystem | subdirectory                                  | point    |
      | nfs4       | /account/.local/state/itos-container/sessions | /account |
      | nfs        | /account/.local/state/itos-container/sessions | /account |
      | nfs4       | /state/private                                | /        |
      | nfs        | /state/private                                | /        |
      | nfs4       | /checkout/private                             | /        |
      | nfs        | /checkout/private                             | /        |
