@phase-1
Feature: Review and record approval of exact offline settings
  Checkout files request permissions; approval records the reviewed effective settings
  outside the checkout. Approval is not evidence of human presence or runtime isolation.
  The host user is the effective UID, never an environment variable, and root is refused.

  # Authorities: docs/charter.md, docs/plans/container-pilot.md AC-02/03/04/06/41,
  # docs/architecture.md (Slices), arc42 A1 as revised by q-2, and the coordinator's
  # 2026-11-04 assignment of host identity, UID resolution and frozen input to slice-4.
  # q-2 replaces q-1's scriptable-only interaction. Bare terminal approve reviews
  # settings/differences and asks y/N; the person never types a hash. A host agent can
  # approve through --fingerprint or a faked terminal: approval records what, not who.
  # Carry that exact limitation into approve help and docs/security.md. There are no
  # credentials, network grants, hooks, persistent worker volumes or new runtime claims.
  #
  # Layout and dependency choice:
  # Implement internal/slices/approve/{command,domain,port,<named adapters>} and shared
  # internal/engine/approval/{domain,port,<named adapters>}. Reuse policy, manifest and
  # readiness contracts; doctor and run consume approval through their own ports.
  # Readiness never imports approval. Only cmd assembles adapters; no slice imports
  # another. Terminal/process/filesystem access, JSON and hashing stay behind ports,
  # with I/O-free port/porttest fakes and the same contracts against real adapters.
  # Use the pinned Go 1.27.3 standard library for JSON, SHA-256 and file operations,
  # existing golang.org/x/term v0.46.0 for terminal detection, and the already pinned
  # golang.org/x/sys v0.48.0 where no-follow/locking calls require it. PTY fixtures
  # reuse github.com/creack/pty v1.1.24. No new dependency or custom JSON lexer is
  # needed. Keep their licenses and notices. OS is an input value; only unavailable
  # calls belong in OS-specific files. Do not widen the domain/port allowlist.
  #
  # Supersession required when implementing, not a reason to weaken old checks:
  # ID-DOCTOR-29's comparison with image Config.User is replaced by host UID/GID
  # resolution below. Config.User is overridden, including empty/root/named values;
  # it cannot select the worker identity. Explicit manifest users must equal the
  # effective host UID; omissions resolve to it. Preserve file-only nulls, syntax
  # validation, metadata refusal and all other readiness checks. Existing doctor
  # fixtures must use the actual non-root test UID, not assume UID 1000. Ordinary
  # doctor gains approval inspection; ID-DOCTOR-02's blanket "approval ... not
  # established" wording becomes status-specific, while runtime remains unproven.
  # ID-RUN-06/07's interim refusal BEFORE prerequisites is superseded: valid argv
  # now checks approval, refuses with "itos container approve" when not approved,
  # and still refuses unimplemented startup after valid approval. Preserve their
  # argument-boundary assertions with valid, approved temporary fixtures and retain
  # ID-RUN-01..05's usage/JSON precedence. Do not add successful startup in slice-4.
  # Sibling feature edits belong to that implementation; this draft owns this file.
  #
  # Canonical settings v1 (the complete field set, all members always present):
  # {schema:1, profile:"offline-v1", host:{uid,gid},
  #  checkout:{root,device,inode,git_device,git_inode},
  #  engine:{endpoint,id,os,architecture},
  #  policy:{version:1,manifest,network:{mode:"offline"}},
  #  manifest:{name,image,workspaceFolder,containerEnv,containerUser,remoteUser},
  #  image:{reference,id,os,architecture}}
  # Use slice-3's inspected checkout/engine/image facts, including canonical root
  # and endpoint, daemon ID and both requested image reference and config digest.
  # host.uid/gid are decimal strings from effective UID/GID, and both manifest user
  # fields resolve to host.uid. No username/NSS lookup, supplementary-group import,
  # chown or image-user fallback. name defaults null, workspaceFolder /workspace,
  # containerEnv {}. Keep literal image reference text and environment values.
  # profile binds the settled offline boundary: checkout-only host mount at
  # /workspace, UID:GID override, no supplementary groups, read-only root, dropped
  # capabilities, no-new-privileges, network none, no ports/grants/hooks, ephemeral
  # tmpfs and pids limit 4096. Slice-10 still specifies exact tmpfs locations and
  # proves enforcement; this is its approved input, not a resource-create request.
  # Exclude API negotiation version, socket inode, timestamps, state path, Git
  # HEAD/branch/origin/source contents, raw file bytes, unused editor/label data,
  # image Config.User and inherited environment. Still validate unused metadata.
  #
  # Canonical JSON means recursively sorted string-key maps encoded as compact
  # Go encoding/json Marshal output with default HTML escaping, no whitespace or
  # trailing newline; arrays retain order. Only schema/version are JSON numbers;
  # identity integers are strings, no floating-point conversion or Unicode
  # normalization. Object ordering, comments and equivalent string escapes in the
  # requests cannot change it. Fingerprint is "sha256:" plus 64 lowercase hex digits
  # of SHA-256 of these exact UTF-8 bytes. It is not a signature or secret.
  # Independent fixtures calculate expected bytes/digests without calling the
  # production canonicalizer; include quotes, backslashes, <>& and U+2028 in vectors.
  #
  # Storage and supersession:
  # Use absolute nonempty XDG_STATE_HOME, otherwise absolute HOME/.local/state;
  # reject a nonempty relative XDG_STATE_HOME (do not silently fall back). HOME
  # selects a path only, never the host identity. Under it, itos-container and
  # managed subdirectories are effective-UID-owned mode 0700; regular records and
  # locks are 0600, independent of umask. Refuse unsafe existing ownership/modes;
  # do not repair them, follow links, or trust approval found in the checkout.
  # The managed tree must be outside the checkout. Open/pin and revalidate paths
  # without following symlinks, including ancestors; reject special files and
  # hard-linked records. Ancestors must not permit an untrusted user to substitute
  # the managed tree; a root-owned sticky /tmp above a private 0700 test parent is
  # allowed. A path substitution is a refusal, never permission to write its target.
  #
  # project = lowercase hex SHA-256 of canonical {schema:1,uid:host.uid,checkout}.
  # The ONLY active record is approvals/<project>/current.json beneath that tree.
  # Its canonical JSON is {schema:1,project,generation,fingerprint,settings}, with
  # generation an unpadded positive uint64 decimal string, settings exactly the
  # object above, and fingerprint recomputed on read. First approval has generation
  # "1"; a changed
  # approval increments it (overflow refuses as state.invalid). Same-settings
  # approval is a no-op. Engine/GID/settings changes replace this checkout's active
  # approval, never leave multiple choices.
  # No historical file or equal-basename checkout authorizes anything. Reverting
  # settings A after approving B needs new consent, even if A was once approved.
  # This does not defend against the trusted host user restoring/forging state.
  # Reject unknown/duplicate fields, noncanonical bytes, invalid schema/generation,
  # inconsistent project/host/checkout/fingerprint, truncated or >1 MiB records;
  # bound actual reads and JSON depth to 32. No partial record becomes authority.
  #
  # Serialize updates per project. Do not hold a lock while awaiting a person.
  # Capture the active generation for review; after consent re-inspect the inputs
  # and identities and compare exact canonical settings, then atomically compare
  # the active generation and publish. If either changed, refuse and ask for a
  # fresh review; do not silently approve the replacement. Write a private temp
  # file, fsync it, rename and fsync the parent; report success only after durable
  # publication. Failures never expose a partial approval. Before rename, preserve
  # the old record; after rename/fsync uncertainty, report failure and re-read the
  # actual complete record on the next invocation, never claim it stayed unchanged.
  # Concurrent same-generation writers cannot both replace it. Missing state is
  # read-only absence for doctor/run; only successful consent creates approval.
  #
  # Shared frozen-input contract (no public snapshot command): given current
  # validated settings and expected project/generation/fingerprint, atomically
  # verify the active approval and return an independent immutable value carrying
  # those three identifiers and the exact approved canonical settings bytes.
  # It reads no checkout files after selection. Mutation of caller data/returned
  # buffers or a later approval cannot change an already returned value. A fresh
  # selection with an old generation fails as superseded. Slice-8 writes the
  # session snapshot durably in protected host state and uses only that input;
  # slices 9/10 must never substitute later checkout reads or mount host control
  # state. Worker inability to access those records needs slice-10's CI runtime
  # proof; local path/snapshot tests here make no such isolation claim.
  #
  # CLI and review contract:
  # Ordinary doctor prints the fingerprint in plain output and as
  # approval:{status,fingerprint,effective,generation,differences} in schema-1 JSON. effective
  # is the canonical settings object; generation is null without a readable prior
  # record. status is missing, matching or changed; valid but unapproved inspection
  # remains exit 0 / ok=true / inspection.ready=true, with explicit approve guidance.
  # differences is [] without prior settings; otherwise sorted {path,before,after}
  # entries for changed canonical fields (containerEnv is compared as one map).
  # Invalid/unsafe state is a typed refusal, with approval.status invalid and the
  # validated candidate retained when available; it never reports matching.
  # File-only modes stay unchanged and never read host identity/state or emit an
  # approval fingerprint.
  #
  # Bare approve requires terminal stdin AND stdout and plain mode. It shows all
  # canonical settings, fixed offline restrictions, unused-field notices, and on
  # re-approval sorted JSON-pointer differences with old/new values, before "y/N".
  # Escape terminal controls in every untrusted value without changing stored/JSON
  # values. Only a complete line y or yes (ASCII case-insensitive, trimmed) consents;
  # blank, n, EOF and every other response decline (exit 1), without a write.
  # An unchanged matching approval returns 0 without prompting or rewriting.
  # Bare approve without both terminals refuses even if already approved; piped y
  # is not consent. --json or ITOS_JSON=1 never prompts, even at a terminal.
  # Those refusals name "--fingerprint <value>" and "itos container doctor".
  # Optional --fingerprint accepts only the exact current sha256:<hex> value; it
  # works without a terminal, never prompts and refuses stale/mismatched values.
  # Explicit --no-json overrides inherited JSON. No --yes or environment consent.
  #
  # Approval JSON is one object {schema:1,ok,problems,approval}; no prompt/prose on
  # stdout. Success approval has status matching, current fingerprint/effective
  # and generation. Problems have rule, exit, nonempty message/fix and safe field
  # or path subjects, not raw rejected content. Public approval rules/exits:
  # approval.identity.root=1, approval.identity.unavailable=3,
  # approval.identity.mismatch=1; approve.confirmation-required=1,
  # approve.declined=1, approve.fingerprint=1 (well-formed mismatch),
  # approve.fingerprint-format=2, approve.changed=1, approve.conflict=1;
  # approval.state.path/unsafe/invalid=1, approval.state.location=2,
  # approval.state.unavailable=3, approval.state.write=75.
  # Reuse policy/manifest/readiness typed rules and exits without a second parser.
  # Run's missing/changed/invalid/superseded approval refusal is exit 1 and names
  # "itos container approve"; it never prompts or writes approval. Valid approval
  # reaches only the existing offline-startup-unimplemented refusal (exit 3).
  # Preserve run's prior JSON and argv refusal precedence before all these checks.
  #
  # Fixture/step handoff (all approval-specific steps below are NEW):
  # "an approval checkout and isolated host state" builds slice-3's valid ordinary
  # Git checkout, policy, manifest and HTTP Engine API Unix-socket fixture, using
  # the actual non-root process UID/GID for explicit manifest users. It sets child
  # HOME/XDG/DOCKER_HOST to private temporary directories/socket, clears inherited
  # account/engine/identity variables, and records requests. It never contacts a
  # daemon. Snapshot checkout/state bytes, modes, owners and link targets only
  # after setup. API journals and execution markers are separate from snapshots.
  # No scenario may use the operator's state, actual credentials, sudo or Docker.
  # @local-engine-fixture selects Linux amd64 before setup; PTYs reuse slice-7's
  # real terminal allocation but run in the approval checkout with isolated env.
  # Match prompt OR early process exit with a bounded harness deadline, so missing
  # approve reaches Then instead of hanging/failing in setup. Multi-run When steps
  # retain every output/exit, even on failure; assertions belong only in Then.
  # If a prerequisite call in a multi-action or contract When fails, retain that
  # result and reach Then, rather than dereferencing absent data or asserting in
  # When. For controlled replacements, snapshot the replacement immediately after
  # the fixture creates it, before allowing the product to continue.
  #
  # "a seeded ... approval" writes the specified canonical record directly from
  # independent fixture values; no Given invokes approve/doctor or asserts their
  # result. "the exact fingerprint" comes from that independent fixture oracle,
  # not a successful product invocation required before the behavior assertion.
  # State defects/replacements always remain within the test's temporary parent.
  # Owner, root, unavailable UID and write-fault cases use a test-only cmd assembly
  # invoking the REAL shared approval contracts with in-memory ports (contract
  # scenarios), not a production injection flag or a privileged subprocess. Wire
  # the real store adapter into its shared contracts as well; fake-only fault
  # coverage is not evidence of filesystem atomicity. Feature runners import no
  # production code. Thin compileable stubs for new APIs in the red step commit
  # may return unavailable, never implement the behavior or fabricate verdicts.
  #
  # Reuse generic "itos-container runs with", exit/stdout/stderr steps and the
  # existing Engine API journal assertions after adapting fixture integration.
  # All other Then steps below need definitions reading actual outputs, records
  # or contract results; none read implementation source. A refusal assertion
  # checks the complete schema/problem/exit contract, not just nonzero/unknown
  # command. A review assertion checks ordered transcript events before input.
  # Positive store assertions decode the entire record and independently verify
  # bytes/hash, project/generation, modes/owner and sole active path. Unchanged
  # approval assertions also compare inode and mtime to detect same-byte rewrites.
  # No assertion accepts mere absence of effects as sufficient evidence of the
  # main behavior.
  # Commit steps red first with @wip, then temporarily remove @wip for local
  # assertion-level red and green. These expected failures are not evidence yet.

  # Fingerprints and host identity.
  @slice-4 @ID-APPROVAL-01 @local-engine-fixture
  Scenario Outline: Doctor exposes the exact candidate without granting approval
    Given an approval checkout and isolated host state
    When itos-container runs with "doctor <format>"
    Then doctor reports the independently calculated fingerprint and complete effective settings
    And doctor reports approval "missing" and names "itos container approve"
    And it exits with code 0
    And the approval checkout and host state are unchanged

    Examples:
      | format |
      |        |
      | --json |

  @slice-4 @ID-APPROVAL-02 @local-engine-fixture
  Scenario Outline: Equivalent requests keep the same canonical fingerprint
    Given an approval checkout and isolated host state
    And a seeded matching approval at generation "1"
    When doctor inspects before and after the equivalent edit "<edit>"
    Then both inspections report matching approval and identical independently calculated fingerprints
    And the active approval bytes and generation are unchanged

    Examples:
      | edit                                                        |
      | reorder YAML and JSON keys and add comments and whitespace   |
      | spell default workspace and empty environment explicitly    |
      | omit both manifest users that equal the effective host UID  |
      | change JSON string escaping without changing decoded values |
      | change unused editor and accepted image-label display data  |
      | change Git HEAD, origin and an ordinary source file         |
      | negotiate another supported Engine API version              |

  @slice-4 @ID-APPROVAL-03 @local-engine-fixture
  Scenario Outline: Changed effective settings need a new approval
    Given an approval checkout and isolated host state
    And a seeded matching approval at generation "1"
    When doctor inspects before and after the effective edit "<edit>"
    Then the second inspection reports changed approval and a different independently calculated fingerprint
    And its approval differences identify "<pointer>" with the prior and current values
    And the active approval bytes and generation are unchanged

    Examples:
      | edit                                            | pointer                    |
      | change the literal application environment       | /manifest/containerEnv     |
      | select an existing nested workspace              | /manifest/workspaceFolder  |
      | change the manifest display name                 | /manifest/name             |
      | select an identical manifest at another path     | /policy/manifest           |
      | request and inspect another pinned image digest  | /manifest/image            |
      | observe another config digest for the image      | /image/id                  |
      | observe another daemon at the same endpoint      | /engine/id                 |
      | select another outside socket for the same daemon| /engine/endpoint           |

  @slice-4 @ID-APPROVAL-04 @local-engine-fixture
  Scenario Outline: Resolve the worker from the effective host identity instead of image or environment
    Given an approval checkout and isolated host state
    And the approval manifest omits both users
    And the inspected image Config.User is "<imageUser>"
    And misleading USER, LOGNAME, UID, GID, SUDO_UID and SUDO_GID fixture values
    When itos-container runs with "doctor --json"
    Then the approval candidate uses the process effective UID and GID and resolves both manifest users to that UID
    And it exits with code 0
    And the approval checkout and host state are unchanged

    Examples:
      | imageUser |
      |           |
      | root      |
      | 0:0       |
      | 2000:3000 |
      | worker    |

  @slice-4 @ID-APPROVAL-05 @local-engine-fixture
  Scenario Outline: Refuse a manifest identity different from the effective host UID
    Given an approval checkout and isolated host state
    And "<field>" requests a positive UID different from the process effective UID
    When itos-container runs with "<command> --json"
    Then it reports approval problem "approval.identity.mismatch" with exit 1 and field "<field>"
    And no approval record or frozen input is produced
    And no engine mutation or image acquisition was attempted

    Examples:
      | command | field         |
      | doctor  | containerUser |
      | doctor  | remoteUser    |
      | approve | containerUser |
      | approve | remoteUser    |

  @slice-4 @ID-APPROVAL-06
  Scenario Outline: Root and unavailable host identities cannot yield an approval candidate
    Given an approval contract fixture with effective UID "<uid>" and GID "<gid>"
    When the approval identity contract resolves the candidate
    Then the contract refuses with "<failure>" and produces no candidate or store write

    Examples:
      | uid         | gid         | failure              |
      | 0           | 1000        | identity.root        |
      | unavailable | 1000        | identity.unavailable |
      | 1000        | unavailable | identity.unavailable |

  @slice-4 @ID-APPROVAL-31
  Scenario Outline: Bind both host IDs and support a non-default non-root identity
    Given a matching approval contract fixture for UID "1000" and GID "1000"
    When the approval contract resolves UID "<uid>" and GID "<gid>" with omitted manifest users
    Then it resolves worker identity "<uid>:<gid>" and reports approval "<status>"
    And its canonical fingerprint binds both independently supplied host IDs

    Examples:
      | uid  | gid  | status  |
      | 1000 | 1000 | matching|
      | 2000 | 3000 | missing |
      | 1000 | 3000 | changed |

  # Deliberate review and optional exact-fingerprint automation.
  @slice-4 @ID-APPROVAL-07 @local-engine-fixture
  Scenario: Review all effective settings before a terminal yes stores approval
    Given an approval checkout and isolated host state
    When bare approve runs at a terminal and receives "y" after the review
    Then the transcript shows all effective settings and offline restrictions before exactly one y/N prompt
    And it exits with code 0
    And the sole active approval is the exact candidate at generation "1" in protected host state
    And the checkout is unchanged and only approval state was written
    And no engine mutation or image acquisition was attempted

  @slice-4 @ID-APPROVAL-08 @local-engine-fixture
  Scenario Outline: Default refusal and declined review leave approval absent
    Given an approval checkout and isolated host state
    When bare approve runs at a terminal and receives "<answer>" after the review
    Then it reports terminal approval refusal "approve.declined" with exit 1 after showing the review
    And the approval checkout and host state are unchanged

    Examples:
      | answer |
      |        |
      | n      |
      | no     |
      | later  |
      | EOF    |

  @slice-4 @ID-APPROVAL-09 @local-engine-fixture
  Scenario: An unchanged approved checkout needs no second prompt
    Given an approval checkout and isolated host state
    And a seeded matching approval at generation "7"
    When bare approve runs at a terminal with no answer available
    Then it reports matching approval without prompting and exits with code 0
    And the active approval bytes and generation are unchanged

  @slice-4 @ID-APPROVAL-10 @local-engine-fixture
  Scenario: Re-approval reviews old and new values before replacing the active record
    Given an approval checkout and isolated host state
    And a seeded prior approval at generation "7" with a different application environment and workspace
    When bare approve runs at a terminal and receives " YES " after the review
    Then the transcript shows complete settings and sorted old-to-new differences before exactly one y/N prompt
    And the sole active approval is the exact candidate at generation "8" in protected host state
    And it exits with code 0

  @slice-4 @ID-APPROVAL-11 @local-engine-fixture
  Scenario Outline: Bare approve without a review terminal refuses and names the optional route
    Given an approval checkout and isolated host state
    And approval state is "<state>"
    When bare approve runs with "<streams>" and input "y"
    Then it reports approval problem "approve.confirmation-required" with exit 1
    And its guidance names "--fingerprint <value>" and "itos container doctor"
    And it never prompts or records approval
    And the approval checkout and host state are unchanged

    Examples:
      | state    | streams                          |
      | missing  | redirected stdin and stdout      |
      | matching | redirected stdin and stdout      |
      | missing  | terminal stdin, redirected stdout|
      | missing  | redirected stdin, terminal stdout|

  @slice-4 @ID-APPROVAL-12 @local-engine-fixture
  Scenario Outline: The optional exact fingerprint approves only the inspected candidate
    Given an approval checkout and isolated host state
    When approve runs without a terminal with the independently calculated exact fingerprint and "<mode>"
    Then it reports matching approval for that fingerprint without a prompt and exits with code 0
    And the sole active approval is the exact candidate at generation "1" in protected host state
    And its approval output follows the requested plain or JSON mode
    And no engine mutation or image acquisition was attempted

    Examples:
      | mode                       |
      | plain                      |
      | --json                     |
      | inherited ITOS_JSON=1      |
      | ITOS_JSON=1 with --no-json  |

  @slice-4 @ID-APPROVAL-13 @local-engine-fixture
  Scenario Outline: Malformed or stale fingerprints never grant consent
    Given an approval checkout and isolated host state
    When approve runs without a terminal with fingerprint "<value>" as JSON
    Then it reports approval problem "<rule>" with exit <exit>
    And no approval record or frozen input is produced

    Examples:
      | value                                      | rule                       | exit |
      | a valid SHA-256 fingerprint for old settings | approve.fingerprint        | 1    |
      | a SHA-256 fingerprint for another checkout   | approve.fingerprint        | 1    |
      | uppercase hexadecimal                       | approve.fingerprint-format | 2    |
      | a bare hexadecimal digest without sha256:   | approve.fingerprint-format | 2    |
      | an empty flag value                         | approve.fingerprint-format | 2    |
      | a short digest                              | approve.fingerprint-format | 2    |

  @slice-4 @ID-APPROVAL-14 @local-engine-fixture
  Scenario Outline: Consent cannot approve settings changed while the review was visible
    Given an approval checkout and isolated host state
    And a seeded prior approval at generation "2" with a different application environment and workspace
    When terminal approve shows its review, the fixture changes "<input>", and then answers yes
    Then it reports terminal approval refusal "approve.changed" with exit 1
    And the active approval bytes and generation are unchanged

    Examples:
      | input                        |
      | the policy manifest path     |
      | the manifest environment     |
      | the selected image config ID |

  @slice-4 @ID-APPROVAL-15 @local-engine-fixture
  Scenario Outline: Consent cannot follow substituted checkout or engine identities
    Given an approval checkout and isolated host state
    When terminal approve shows its review, the fixture replaces "<identity>", and then answers yes
    Then it reports terminal approval refusal "approve.changed" with exit 1
    And no approval record or frozen input is produced
    And the replacement target is unchanged

    Examples:
      | identity             |
      | checkout root        |
      | embedded .git        |
      | daemon ID            |
      | socket device/inode  |

  # Protected storage, project separation and supersession.
  @slice-4 @ID-APPROVAL-16 @local-engine-fixture
  Scenario Outline: A different checkout cannot borrow another project's approval
    Given two approval checkouts named "one/project" and "two/project" in isolated host state
    And a seeded matching approval for the first checkout
    And the second checkout has "<copy>"
    When doctor inspects the second checkout as JSON
    Then it reports the second checkout's independent fingerprint and approval "<status>"
    And the first checkout's approval remains unchanged
    And no approval record or frozen input is produced

    Examples:
      | copy                                                     | status  |
      | no copied record                                         | missing |
      | a copied approval inside its writable checkout            | missing |
      | the first record copied to the second project's host path | invalid |

  @slice-4 @ID-APPROVAL-17 @local-engine-fixture
  Scenario: Reverting to superseded settings does not reactivate historical approval
    Given an approval checkout and isolated host state
    And active approval B at generation "2" and a historical approval A at generation "1"
    And the checkout requests settings A
    When itos-container runs with "doctor --json"
    Then doctor reports approval "changed" and names "itos container approve"
    And its current approval generation is "2" and its candidate fingerprint is A
    And the active approval bytes and generation are unchanged

  @slice-4 @ID-APPROVAL-18 @local-engine-fixture
  Scenario Outline: Corrupt or inconsistent active records fail closed
    Given an approval checkout and isolated host state
    And a seeded matching approval with record defect "<defect>"
    When itos-container runs with "doctor --json"
    Then it reports approval problem "approval.state.invalid" with exit 1
    And the approval checkout and host state are unchanged

    Examples:
      | defect                                   |
      | truncated JSON                           |
      | duplicate decoded member                 |
      | unknown schema or member                 |
      | noncanonical JSON bytes                  |
      | invalid generation                       |
      | fingerprint does not hash settings       |
      | project does not hash host and checkout  |
      | settings belong to another effective UID |
      | record larger than 1 MiB                 |
      | record deeper than 32 levels             |

  @slice-4 @ID-APPROVAL-19 @local-engine-fixture
  Scenario Outline: Approval storage refuses unsafe paths without repairing or following them
    Given an approval checkout and isolated host state
    And approval storage has unsafe layout "<layout>"
    When approve runs without a terminal with the independently calculated exact fingerprint and "--json"
    Then it reports approval problem "approval.state.<failure>" with exit 1
    And every existing state entry and outside target is unchanged
    And no approval record or frozen input is produced

    Examples:
      | layout                                      | failure |
      | XDG_STATE_HOME beneath the checkout          | path    |
      | a symlinked state ancestor                   | path    |
      | itos-container is a symlink                  | path    |
      | approvals project directory is a symlink     | path    |
      | current.json is a symlink                   | path    |
      | current.json is a FIFO                      | unsafe  |
      | current.json has a second hard link         | unsafe  |
      | managed directory mode is 0777              | unsafe  |
      | current.json mode is 0644                   | unsafe  |

  @slice-4 @ID-APPROVAL-20 @local-engine-fixture
  Scenario Outline: Store approval under the selected host state root with private permissions
    Given an approval checkout and isolated host state
    And the child state location is "<location>" with umask "0000"
    When approve runs without a terminal with the independently calculated exact fingerprint and "--json"
    Then approval storage outcome is "<outcome>" at "<path>" with exit <exit>
    And all created managed directories are owned by the effective UID with mode 0700
    And all created records and locks are owned by the effective UID with mode 0600
    And no approval appears in the checkout or another host-state location

    Examples:
      | location                  | outcome                 | path                                      | exit |
      | absolute XDG_STATE_HOME   | matching                | XDG_STATE_HOME/itos-container              | 0    |
      | unset XDG_STATE_HOME      | matching                | HOME/.local/state/itos-container           | 0    |
      | empty XDG_STATE_HOME      | matching                | HOME/.local/state/itos-container           | 0    |
      | relative XDG_STATE_HOME   | approval.state.location | none                                      | 2    |

  # Linux 5.15/6.1 do not provide fchmodat2. A test-only child returns ENOSYS
  # for that syscall before execing the product; it uses only the temporary
  # checkout/state and synthetic Engine API. Initialization must still pin and
  # check the new inode, including when umask removes every directory permission.
  @slice-4 @ID-APPROVAL-32 @local-engine-fixture @wip
  Scenario Outline: First approval works without fchmodat2 regardless of umask
    Given an approval checkout and isolated host state
    When approve runs with the exact fingerprint without fchmodat2 and with umask "<umask>"
    Then it reports matching approval for that fingerprint without a prompt and exits with code 0
    And the sole active approval is the exact candidate at generation "1" in protected host state
    And all created managed directories are owned by the effective UID with mode 0700
    And all created records and locks are owned by the effective UID with mode 0600
    And the checkout is unchanged and only approval state was written
    And no engine mutation or image acquisition was attempted

    Examples:
      | umask |
      | 0022  |
      | 0777  |

  @slice-4 @ID-APPROVAL-21 @local-engine-fixture
  Scenario: Concurrent reviews cannot overwrite an approval recorded after their review began
    Given an approval checkout and isolated host state
    And a seeded prior approval at generation "4" with a different application environment and workspace
    When two terminal approvals review generation "4", the first answers yes, then the second answers yes
    Then the first records generation "5" and exits with code 0
    And the second reports "approve.conflict" with exit 1 without another write
    And the only active record is the first process's complete canonical approval

  @slice-4 @ID-APPROVAL-22
  Scenario Outline: Unsafe ownership and failed publication never yield false approval success
    Given an approval store contract fixture with failure "<failure>"
    When the approval store contract attempts to publish a consented replacement
    Then publication reports "<outcome>" and no successful approval result
    And the visible active record is "<visible>"
    And no partial record or outside-target write is visible

    Examples:
      | failure                                  | outcome           | visible                    |
      | managed directory owned by another UID   | state.unsafe      | the prior record           |
      | active file owned by another UID         | state.unsafe      | the prior record           |
      | state parent replaced after pinning      | state.path        | the prior record           |
      | cannot create the private temp file      | state.write       | the prior record           |
      | short write                              | state.write       | the prior record           |
      | temp file fsync fails                     | state.write       | the prior record           |
      | atomic rename fails                      | state.write       | the prior record           |
      | parent fsync fails after rename           | state.write       | a complete old or new record|

  # Frozen-input contracts are tested without implementing or launching run.
  @slice-4 @ID-APPROVAL-23
  Scenario: Frozen input owns the approved bytes independently of future edits and approvals
    Given a matching approval contract fixture for UID "2000" and GID "3000"
    When frozen input A is selected and the caller mutates its inputs and returned buffers before approval B is recorded
    Then the retained frozen input A still contains exactly its original canonical settings and approval identifiers
    And a fresh selection for B returns only B's independently owned canonical bytes
    And neither selection reads checkout data after choosing its approved record

  @slice-4 @ID-APPROVAL-24
  Scenario Outline: Frozen selection rejects missing, changed and superseded approval atomically
    Given an approval contract fixture with selection condition "<condition>"
    When the frozen-input contract selects the requested approval
    Then selection refuses with "<failure>" and returns no frozen input

    Examples:
      | condition                                  | failure    |
      | active record is absent                    | missing    |
      | candidate settings differ                  | changed    |
      | expected generation was superseded         | superseded |
      | active generation changes before final verification | superseded |
      | active record is invalid                   | invalid    |

  # Run observes approval; it cannot acquire it or launch in this slice.
  @slice-4 @ID-APPROVAL-25 @local-engine-fixture
  Scenario Outline: Run blocks unapproved settings and directs the person to approve
    Given an approval checkout and isolated host state
    And approval state is "<state>"
    When itos-container runs with "run -- fixture-workload"
    Then run refuses approval "<reason>" with exit 1 and names "itos container approve"
    And it never prompts or records approval
    And no engine mutation or image acquisition was attempted
    And the approval checkout and host state are unchanged

    Examples:
      | state                              | reason  |
      | missing                            | missing |
      | matching only in the checkout       | missing |
      | changed                            | changed |
      | historical A with active B          | changed |
      | invalid                            | invalid |

  @slice-4 @ID-APPROVAL-26 @local-engine-fixture
  Scenario: Matching approval reaches the interim startup refusal without asking again
    Given an approval checkout and isolated host state
    And a seeded matching approval at generation "1"
    When itos-container runs with "run -- fixture-workload --json"
    Then run reports that approval matched before refusing unimplemented offline startup with exit 3
    And it never prompts or records approval
    And no engine mutation or image acquisition was attempted
    And the approval checkout and host state are unchanged

  # Cross-command validation and honest help.
  @slice-4 @ID-APPROVAL-27 @local-engine-fixture
  Scenario Outline: Approve reuses strict validation and cannot consent around a failed prerequisite
    Given an approval checkout and isolated host state
    And approval input has prerequisite defect "<defect>"
    When approve runs without a terminal with a well-formed candidate fingerprint as JSON
    Then it reports prerequisite problem "<rule>" with exit <exit> before requesting consent
    And no approval record or frozen input is produced
    And no engine mutation or image acquisition was attempted

    Examples:
      | defect                                    | rule                       | exit |
      | unsupported manifest initializeCommand    | manifest.unsupported       | 2    |
      | hidden image label requests a user         | manifest.metadata          | 2    |
      | cached image is absent                    | doctor.image.missing       | 3    |
      | daemon identity changes during inspection | doctor.engine.changed      | 1    |
      | checkout root changes during inspection   | doctor.checkout.changed    | 1    |

  @slice-4 @ID-APPROVAL-28 @local-engine-fixture
  Scenario: Successful approval executes no checkout helpers and exposes no host control mount
    Given an approval checkout and isolated host state
    And executable marker fixtures for Git hooks, fsmonitor, credential helpers, builds and PATH tools
    When approve runs without a terminal with the independently calculated exact fingerprint and "--json"
    Then the sole active approval is the exact candidate at generation "1" in protected host state
    And no executable marker was created
    And the checkout is unchanged and only approval state was written
    And the approved mount boundary contains only the checkout and excludes host approval and snapshot paths
    And no engine mutation or image acquisition was attempted

  @slice-4 @ID-APPROVAL-29
  Scenario: Approval help explains the human flow and its host-agent limitation
    When itos-container runs with "approve --help"
    Then approval help describes terminal review, changes, y/N, remembered approval and optional exact --fingerprint
    And its standard output says "an agent on the host can approve"
    And its standard output says "faked terminal"
    And its standard output says "what was approved, not who"
    And its standard output says "doctor"
    And it exits with code 0

  @slice-4 @ID-APPROVAL-30 @local-engine-fixture
  Scenario Outline: JSON never prompts and terminal review safely renders literal values
    Given an approval checkout and isolated host state
    And a literal application environment value contains terminal controls and shell-looking text
    When approve runs at a terminal in "<mode>" with "<consent>"
    Then approval presentation is "<presentation>" with exit <exit>
    And no shell-looking text is executed
    And any stored or JSON settings preserve the exact literal environment value

    Examples:
      | mode                    | consent             | presentation                           | exit |
      | plain                   | y after review      | escaped review before consent          | 0    |
      | --json                  | no fingerprint      | confirmation-required JSON, no prompt   | 1    |
      | inherited ITOS_JSON=1    | no fingerprint      | confirmation-required JSON, no prompt   | 1    |
      | --json                  | exact fingerprint   | one matching JSON object, no prompt     | 0    |
