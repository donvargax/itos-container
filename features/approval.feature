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
  # Slice-26's block below adds the first network grant approval can record, and no route.
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
  # Canonical settings v1 (offline-v1's complete field set, all members always present;
  # slice-26's block below adds the allowlist-v1 profile and the members only it has):
  # {schema:1, profile:"offline-v1", host:{uid,gid},
  #  checkout:{root,device,inode,git_device,git_inode},
  #  engine:{endpoint,id,os,architecture},
  #  policy:{version:1 or 2,manifest,network:{mode:"offline"}},
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
  # Plain doctor escapes non-printing Unicode in effective settings just like
  # approval review; its structured JSON retains the exact decoded values.
  #
  # Bare approve requires terminal stdin AND stdout and plain mode. It shows all
  # canonical settings, fixed offline restrictions, unused-field notices, and on
  # re-approval sorted JSON-pointer differences with old/new values, before "y/N".
  # Escape terminal controls in every untrusted value without changing stored/JSON
  # values. This includes Unicode C1 and bidirectional controls in current settings,
  # prior values and plain approval status. Render non-printing Unicode as JSON
  # escapes, preserving printable Unicode and the exact canonical bytes for hashing.
  # Only a complete line y or yes (ASCII case-insensitive, trimmed) consents;
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

  # Slice-26: approval binds the egress allowlist, and any change to it needs a new approval
  # (charter, second milestone, step 1; contract AC-02 and "Approval and session state").
  # Slice-25 lets a policy request a list (features/policy.feature, ID-POLICY-43 to 78, on its
  # branch until it lands). T-38's record, docs/decisions/0022-egress-proxy.md, on main and still
  # proposed, gives this slice three jobs: name the network-granting profile, bind the proxy's
  # image digest, and put the handshake guarantee and the refusal log's limit in approve's
  # presentation. If the person overturns that record, the profile's text and the proxy member
  # change with it. This slice covers records, presentation and refusal only. Nothing starts a
  # proxy, and while run refuses version 2 nothing here opens a route. Every decision below
  # belongs to this spec.
  #
  # No new slice: this adds to what exists. internal/engine/approval takes the profile, valid(),
  # the changed-field comparison and the review and status lines. internal/engine/policy exports
  # the host-name shape check that valid() calls, and internal/engine/readiness takes the gate's
  # field. internal/slices/run/domain takes only ApprovalRefusal's wording, from what the view
  # carries. The approve and doctor slices change only through cmd/itos-container's assembly, which
  # also holds the help line.
  #
  # Canonical settings v1 gain a second profile:
  # {schema:1, profile:"allowlist-v1", host:{uid,gid},
  #  checkout:{root,device,inode,git_device,git_inode},
  #  engine:{endpoint,id,os,architecture},
  #  policy:{version:2,manifest,network:{mode:"allowlist",allow:[names]}},
  #  proxy:{image:"docker.io/library/busybox@sha256:5624bb6777cf469b1fb50ae3f981b15d2715ee2fffaf20b9998cfb7843726835"},
  #  manifest:{name,image,workspaceFolder,containerEnv,containerUser,remoteUser},
  #  image:{reference,id,os,architecture}}
  # Every other member means what it means under offline-v1. allow is slice-25's effective
  # list: canonical names, each once, in ascending byte order, so a reordered or respelled file
  # keeps its fingerprint. It is compared as one value, as containerEnv is: any change to it is
  # one difference at /policy/network/allow with the whole old and new arrays, null where a side
  # has none. Neither allow nor proxy exists under offline-v1, whose bytes do not change: a
  # version 1 file keeps its profile, fingerprint and record, and ID-APPROVAL-01's independent
  # oracle holds that. schema stays 1 in the settings and the envelope, because the profile says
  # what a record grants. An older binary refuses allowlist-v1 as approval.state.invalid, which
  # fails closed.
  #
  # allowlist-v1 binds T-38's boundary, which no field varies: the worker's offline-v1
  # restrictions except network none, an internal network with no route out, one owned proxy,
  # CONNECT on port 443 only, no TLS interception, and a first ClientHello whose SNI equals the
  # CONNECT name. The list and the profile together are the contract's "network
  # destinations/protocols/TLS modes". If what the proxy enforces changes, the profile takes a
  # new name, which needs new consent.
  #
  # proxy.image is T-38's inert root filesystem, T-20's pinned reference. It is a product
  # constant with one definition, in the approval codec or its port: no policy, manifest, flag
  # or environment changes it, and ID-POLICY-53 already refuses a proxy key. Approval binds the
  # literal reference and inspects nothing. No command here uses the image, so doctor and approve
  # add no Engine API request, which ID-APPROVAL-36's journal step holds. Slice-27 owns checking
  # that it is cached and naming the exact pull. A record that bound another digest is valid and
  # reads as changed at /proxy/image, so a later release that moves the constant needs a new
  # approval.
  #
  # policy.version is the version the file declared, the value doctor --policy-only shows as
  # effective. So a version 2 offline file is profile offline-v1, policy.version 2, no list and
  # no proxy. Moving an approved version 1 file to version 2 offline changes only /policy/version
  # and needs one new approval, which shows that one difference. Decided here rather than
  # normalising the field to 1: otherwise two commands would print different effective policies
  # for one file, and asking once more grants nothing.
  #
  # A stored record is valid only as one of these, checked in valid() in
  # internal/engine/approval/canonical/json.go. Anything else is approval.state.invalid:
  # - offline-v1: policy.version 1 or 2, mode offline, no allow and no proxy.
  # - allowlist-v1: policy.version 2, mode allowlist, proxy.image a name followed by @sha256: and
  #   64 lower-case hexadecimal digits, and allow nonempty and strictly ascending by bytes, each
  #   name with the shape slice-25's checks end with: at least two labels of 1 to 63 lower-case
  #   letters, digits and hyphens, at most 253 bytes in all, and a final label neither all digits
  #   nor 0x plus hexadecimal digits. This is a lexical check that never calls the names port: a
  #   stored name is compared, never mapped. The second half of slice-25's canonicalHost is that
  #   check, so it moves to an exported function in internal/engine/policy/domain, which both call:
  #   a copy could drift, and a narrower one would call an approval invalid as soon as it was
  #   written.
  # valid() and the candidate constructor above it are the two places in json.go that hardcode
  # version 1 and offline today; the constructor reads policy.Network.Mode and never infers it
  # from the list. porttest.Ready's policy has no network, so that fixture gains its offline mode.
  # Approval adds no limit on the list of its own, and needs none: 2037 names of 253 bytes take
  # under 530 KB and a 64 KiB manifest escapes to under 400 KB, so no list the policy admits
  # produces a record over the store's 1 MiB limit.
  #
  # Run keeps refusing every valid version 2 file until slice-27, whatever its mode: with
  # policy.version at /version, before the manifest, the engine and host state, and in the same
  # words, as ID-POLICY-62 holds. It has no proxy to start. The branch that refuses, which slice-25
  # put in InspectBeforeEngine in internal/engine/readiness/domain/inspect.go, also stops doctor
  # and approve, which share that call. It moves behind an Inspector field whose zero value
  # refuses. cmd sets the field in the doctor and approve assemblies and in no other, never for
  # run, so a forgotten wiring refuses instead of opening, and
  # TestValidVersion2PolicyIsRefusedBeforeAnyLaterStage keeps holding the zero value. An offline
  # approval can no longer match an allowlist request, since profile and fingerprint differ, and
  # even a mis-wired gate opens no route: session's adapters keep NetworkMode none and VerifyRun
  # enforces it.
  #
  # "Run refuses with the change explained" (the registry's why, contract AC-02) meets
  # ID-POLICY-62: a version 2 file never reaches run's approval comparison, so run cannot explain
  # a changed list until slice-27 lifts that refusal. Doctor and approve explain it now, and
  # slice-27's spec must prove run does. What run explains here is every comparison it can reach:
  # run.approval-changed names the changed canonical fields, sorted and comma separated, as "run
  # approval changed at <pointers>; use itos container approve". Pointers only, never values:
  # ApprovalRefusal carries no request contents, containerEnv can hold anything, and doctor and
  # approve already show old and new values safely. The comparison before the engine leaves out
  # /engine and /image, which it has not observed (LocalMatches), and the one after inspection
  # includes them. Selection can also answer changed, for a record rewritten at the same generation
  # between inspection and Select (internal/engine/approval/disk/store.go). It has no comparison at
  # hand, so that refusal keeps today's words, "run approval changed; use itos container approve",
  # and never reads "changed at ;".
  #
  # One walk finds what changed for all three commands, since arc42's Solution strategy gives
  # matching to one shared responsibility. presentation's differences moves into
  # internal/engine/approval/domain, over decoded values, so it needs neither encoding/json nor
  # reflect, which a domain may not import, and presentation calls it from there. PreflightCodec
  # gains the comparison run needs: canonical decodes both sides, blanks /engine and /image for the
  # one before the engine, and returns that walk's paths. The view carries them to
  # internal/slices/run/domain, which reads no JSON.
  #
  # The implementing commit also adds tests for what no scenario here can hold, each at the tier
  # that can see it, since a domain test may not import the codec. An allowlist candidate and a
  # record with another list, which the black box cannot reach while run refuses version 2:
  # canonical's own test gets /policy/network/allow from the local comparison, and approval's
  # domain test, over the fake codec, has Preflight carry the codec's paths onto the view without
  # opening the engine. A selection that answers changed, which ID-RUN-12's race step could reach by
  # rewriting the record at the same generation, but which passes before the work and after it
  # because its words do not change, so it can be no red evidence: run's domain test gets today's
  # words. The Unused field lines, which no scenario reads for either profile: presentation_test.go
  # also holds them for an allowlist-v1 candidate, after the disclosure and before the differences.
  #
  # Bare approve's review of an allowlist-v1 candidate prints these lines in this order, then the
  # unused fields, the differences and the y/N prompt as it does today:
  #   Effective settings: <canonical settings, escaped as today>
  #   allowlist-v1: checkout-only host mount at /workspace; UID:GID override; no supplementary groups; read-only root; dropped capabilities; no-new-privileges; internal network with no route out; no ports, grants or hooks; ephemeral tmpfs; pids limit 4096
  #   This profile grants network. The worker can reach only these host names, through an owned proxy, by CONNECT on port 443:
  #   network.allow: <name>, one line per name in list order
  #   No TLS interception: the proxy verifies the CONNECT name and the first ClientHello's SNI.
  #   It cannot see the HTTP Host inside TLS or anything after that hello, a second hello after a HelloRetryRequest included, so fronting remains a limit, as does a hello the upstream reads differently, and it cannot stop data sent to an allowed service.
  #   Clients that omit SNI or send ECH, and SSH over 443, are refused.
  #   The proxy logs each refusal, but a stored log is no proof that every refusal was kept: a log that ends early reads as a complete one.
  #   Run cannot start this profile yet: it refuses every version 2 policy for now.
  # The handshake and log lines are T-38's disclosure, and slice-27 repeats them in status and
  # docs/security.md. Plain approve status prints the same lines from allowlist-v1: down, after
  # the canonical settings, so an unchanged approval is told too. Slice-27's feat commit removes
  # the last line and the rows that read it, in the commit that makes the lines above it true of a
  # run: they speak of the proxy in the present tense, which only that last line keeps honest until
  # then. An offline-v1 candidate keeps its review and status exactly at either version, including
  # "Effective offline settings:" and "network none", which ID-APPROVAL-44 and ID-APPROVAL-57 hold.
  # JSON gains nothing: effective already carries the profile, the list and the proxy image, and
  # approve's JSON stays one object with no prose. Ordinary doctor prints the same canonical
  # settings as today. Canonical names are ASCII, and a stored one passes valid() first, so the
  # lines this block adds need no further escaping. The canonical settings and the differences can
  # hold anything a manifest holds, so ID-APPROVAL-58 holds them as ID-APPROVAL-33 does for offline.
  #
  # q-52 (the person, 2026-11-09): only changed settings ask for approval again. A grant
  # with an egress allowlist or credentials requires the person at a real terminal.
  # --fingerprint is only for offline grants without credentials. For an exact network
  # fingerprint, refuse approve.interactive-required (exit 1), with a fix naming
  # "itos container approve" and "terminal". Bare nonterminal or JSON network approval
  # gives the same refusal, never suggesting --fingerprint as a way around it. Fingerprint
  # syntax and mismatch checks still precede this refusal. Matching terminal approval
  # neither prompts nor rewrites. No keys or signatures. docs/security.md records the
  # remaining gap: an unsandboxed host agent can fake a terminal.
  #
  # What this makes untrue. ID-POLICY-60 and ID-POLICY-61 (ordinary doctor and approve refuse
  # version 2) were written to last until this slice. The feat commit removes both and says so,
  # as only a feat or a fix may. Their behaviour moves here: a matching offline approval beside an
  # allowlist request is ID-APPROVAL-40's first row, the offline fingerprint ID-APPROVAL-47's first
  # row and the absent manifest ID-APPROVAL-55's last row. ID-POLICY-62, ID-POLICY-77 and
  # ID-POLICY-78 stay as they are. The same commit renames this feature, whose title says offline,
  # and corrects the comments that still state the old rule: three in features/policy.feature
  # ("Ordinary doctor says the same (ID-POLICY-77), though it refuses version 2 for now", the
  # paragraph "Only doctor --policy-only and --manifest-only read version 2 for now", and "The next
  # three scenarios" above ID-POLICY-60), the one at the refusal in
  # internal/engine/readiness/domain/inspect.go, which moves with the gate, and the one above
  # TestValidVersion2PolicyIsRefusedBeforeAnyLaterStage in inspect_policy_test.go beside it, which
  # says canonical settings v1 describe an offline profile.
  #
  # The implementing pull request also updates docs/CLI.md and docs/CONFIG.md, which say that
  # nothing approves a list and that ordinary doctor and approve refuse version 2, and the approve
  # help line (ID-APPROVAL-56). It records a decision for the profile, the policy.version choice,
  # the gate and run's wording, and brings three texts of arc42 up to date, as the charter's
  # "Architecture is a hypothesis" asks: the internal/engine/approval row, T-38's slice-26 row, and
  # Solution strategy's sentence that doctor, approve and run "must agree on accepted/rejected
  # inputs and effective settings". The gate contradicts that sentence until slice-27, because
  # doctor and approve accept a valid version 2 file that run refuses, so the decision names that
  # section and says why. docs/security.md records q-52 here; runtime disclosure remains with slice-27.
  #
  # A nil names port panics on the first allowlist, in a file a hostile checkout controls. Both
  # sites are assembled in cmd/itos-container/main.go, and the gate above re-wires one of them, so
  # the feat commit adds a test in cmd/itos-container over that assembly: every Inspector and
  # inspection.Policies it builds has Names set, and only doctor's and approve's read version 2.
  # The scenarios reach each site through the binary; this test fails where the wiring is.
  #
  # Scenarios below reuse existing steps except these, which only write fixtures or read output:
  # - "the checkout requests the allowlist "<names>"" writes version 2, the fixture's manifest
  #   path, mode allowlist and the names one a line, then records them for the oracle and
  #   recaptures the tree as approvalSeed does. The names are given canonical and ascending, and
  #   the step refuses others. "the checkout requests an allowlist of <count> generated host
  #   names" does the same with n0.example, n1.example and so on, sorted by bytes for the oracle,
  #   and "the checkout requests an allowlist of one host name of 253 bytes" with labels of 63, 63,
  #   63 and 61 bytes.
  # - "the checkout requests offline at policy version "<n>"" writes that version and the manifest
  #   path, with no network key. "then" may follow "checkout" in all four: it reads better after
  #   a seed and means the same.
  # - approvalOracle takes the profile, version, network and proxy from that request, never from
  #   the product. Its digest is the features package's own busyboxFixturePin. A scenario with no
  #   such request keeps today's version 1 offline settings.
  # - "a seeded allowlist approval at generation "<g>" that bound another proxy image digest"
  #   seeds the oracle's settings with another digest of the same form, and a fingerprint of
  #   those settings.
  # - "approve runs without a terminal with the independently calculated fingerprint of
  #   "<settings>" as JSON" hashes a variant of the oracle's settings, each row's variant named
  #   by the row.
  # - "its approval differences are exactly "<pointers>" with the prior and current values" reads
  #   the last JSON's whole list against the oracle, as the single-pointer step does. "the
  #   transcript shows differences "<pointers>" with old and new values before exactly one y/N
  #   prompt" reads the review the way approvalReview(true) does, in the pointers' order.
  # - "the transcript shows all effective settings and the worker's restrictions before exactly
  #   one y/N prompt" is approvalReview(false)'s check under a name that fits both profiles.
  # - "the review before its prompt says, in this order:" and "the review before its prompt does
  #   not say "<text>"" read the transcript before y/N, each row after the one above it. "its
  #   standard output says, in this order:" reads standard output the same way.
  # - "the review before its prompt lists exactly the requested host names" reads every line of the
  #   transcript before y/N that begins "network.allow: ", in order, against the oracle's list, so a
  #   name missing, added, moved or taken from the old record fails. "its standard output lists
  #   exactly the requested host names" reads standard output the same way.
  # - "its approval status is "<status>"" is approvalStatus alone.
  # - "the host state is unchanged" is approvalSameTree without the entries under the checkout, for
  #   a fixture that edits the checkout itself. It is what looks for frozen input in ID-APPROVAL-48,
  #   where the record step stops once the record is absent and approvalSameTree cannot run.
  # These existing steps gain values. approvalEdit: "reorder the allowlist", "respell the
  # allowlist's names" (API.Example.COM. and BÜCHER.example), "write the allowlist in flow style
  # with comments and reordered keys", "write the policy version as 2.0", "add the host name
  # <name>", "remove the host name <name>", "change the host name <old> to <new>", where the
  # look-alike row's new name is the Cyrillic spelling of api.example.com, canonically
  # xn--pi-6kc.example.com, and "request ..." as the Givens above. approvalDefect: the rows of
  # ID-APPROVAL-50, each recomputing the fingerprint so that only record validity can catch it, and
  # each breaking one rule. A row about one bad name lists it between 0.example and z.example, in
  # byte order, so that a check of only the first name or only the last fails: the upper-case name
  # is b.EXAMPLE, the wildcard a.*.example, the single label localhost, the literal address
  # 192.0.2.1, the hexadecimal final label host.0x7f, the empty label a..example, and every label of
  # the 254-byte name is within 63 bytes. The proxy image with a short digest has 63 digits, the
  # upper-case one is the pinned digest in upper case, and the one with no name is @sha256: and the
  # pinned digest. "approval input has prerequisite defect" gains "the selected manifest is absent"
  # and now recaptures the tree after any defect, so that ID-APPROVAL-55's tree step reads what
  # approve did and not what the fixture did. "run preflight has engine observation": "cached image
  # reports another config digest". approvalUnicodeReview finds the settings on the line for the
  # oracle's profile, "Effective settings: " under allowlist-v1 and "Effective offline settings: "
  # under offline-v1, so ID-APPROVAL-33 keeps reading what it reads today.
  # Red first: today every valid version 2 file is refused as policy.version, exit 2, before
  # anything else reads it, so each scenario fails at its first Then that reads the outcome. Where
  # the first answer is that same refusal, the scenario goes on to doctor so that it can fail
  # before the work. A version 1 file keeps its behaviour by design, so a row that reads one starts
  # from an approval today's record check refuses.

  @wip @slice-26 @ID-APPROVAL-36 @local-engine-fixture
  Scenario Outline: Doctor exposes the exact allowlist candidate without granting approval
    Given an approval checkout and isolated host state
    And the checkout requests the allowlist "api.example.com registry.example.org"
    When itos-container runs with "doctor <format>"
    Then doctor reports the independently calculated fingerprint and complete effective settings
    And doctor reports approval "missing" and names "itos container approve"
    And it exits with code 0
    And no engine mutation or image acquisition was attempted
    And the approval checkout and host state are unchanged

    Examples:
      | format |
      |        |
      | --json |

  @wip @slice-26 @ID-APPROVAL-37 @local-engine-fixture
  Scenario: Offline at policy version 2 keeps the offline profile
    Given an approval checkout and isolated host state
    And the checkout requests offline at policy version "2"
    When itos-container runs with "doctor --json"
    Then doctor reports the independently calculated fingerprint and complete effective settings
    And doctor reports approval "missing" and names "itos container approve"
    And it exits with code 0
    And the approval checkout and host state are unchanged

  # The second name has a Unicode spelling, so the respelling row can change every name.
  @wip @slice-26 @ID-APPROVAL-38 @local-engine-fixture
  Scenario Outline: Equivalent allowlist requests keep one approval
    Given an approval checkout and isolated host state
    And the checkout requests the allowlist "api.example.com xn--bcher-kva.example"
    And a seeded matching approval at generation "1"
    When doctor inspects before and after the equivalent edit "<edit>"
    Then both inspections report matching approval and identical independently calculated fingerprints
    And the active approval bytes and generation are unchanged

    Examples:
      | edit                                                       |
      | reorder the allowlist                                      |
      | respell the allowlist's names                              |
      | write the allowlist in flow style with comments and reordered keys |
      | write the policy version as 2.0                            |

  # A parent name and a look-alike are other names, not spellings of a listed one.
  @wip @slice-26 @ID-APPROVAL-39 @local-engine-fixture
  Scenario Outline: Any change to a host name needs a new approval
    Given an approval checkout and isolated host state
    And the checkout requests the allowlist "api.example.com registry.example.org"
    And a seeded matching approval at generation "1"
    When doctor inspects before and after the effective edit "<edit>"
    Then the second inspection reports changed approval and a different independently calculated fingerprint
    And its approval differences are exactly "/policy/network/allow" with the prior and current values
    And the active approval bytes and generation are unchanged

    Examples:
      | edit                                                         |
      | add the host name other.example                              |
      | remove the host name registry.example.org                    |
      | change the host name registry.example.org to registry.example.net |
      | change the host name api.example.com to example.com          |
      | change the host name api.example.com to its Cyrillic look-alike |

  # The third and fourth rows narrow what the file asks for: a new approval is needed that way
  # too. The last two pin the policy.version decision above.
  @wip @slice-26 @ID-APPROVAL-40 @local-engine-fixture
  Scenario Outline: Moving between network modes or policy versions needs a new approval
    Given an approval checkout and isolated host state
    And the checkout requests <from>
    And a seeded matching approval at generation "1"
    When doctor inspects before and after the effective edit "<to>"
    Then the second inspection reports changed approval and a different independently calculated fingerprint
    And its approval differences are exactly "<pointers>" with the prior and current values
    And the active approval bytes and generation are unchanged

    Examples:
      | from                            | to                                    | pointers                                                                   |
      | offline at policy version "1"   | request the allowlist api.example.com | /policy/network/allow /policy/network/mode /policy/version /profile /proxy |
      | offline at policy version "2"   | request the allowlist api.example.com | /policy/network/allow /policy/network/mode /profile /proxy                 |
      | the allowlist "api.example.com" | request offline at policy version 2   | /policy/network/allow /policy/network/mode /profile /proxy                 |
      | the allowlist "api.example.com" | request offline at policy version 1   | /policy/network/allow /policy/network/mode /policy/version /profile /proxy |
      | offline at policy version "1"   | request offline at policy version 2   | /policy/version                                                            |
      | offline at policy version "2"   | request offline at policy version 1   | /policy/version                                                            |

  # No file can change the digest, so a record from a release that bound another one stands in
  # for the release that moves the constant.
  @wip @slice-26 @ID-APPROVAL-41 @local-engine-fixture
  Scenario: A different proxy image digest needs a new approval
    Given an approval checkout and isolated host state
    And the checkout requests the allowlist "api.example.com"
    And a seeded allowlist approval at generation "1" that bound another proxy image digest
    When itos-container runs with "doctor --json"
    Then doctor reports the independently calculated fingerprint and complete effective settings
    And doctor reports approval "changed" and names "itos container approve"
    And its approval differences are exactly "/proxy/image" with the prior and current values
    And it exits with code 0
    And the active approval bytes and generation are unchanged

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

  # The egress allowlist under review. The rows of the first scenario are the words a person
  # consents to: every line of the block above ID-APPROVAL-36 but the settings themselves, whole
  # and in its order, because a fragment would also pass a line that says the opposite, such as
  # "SSH over 443 is allowed".
  @wip @slice-26 @ID-APPROVAL-42 @local-engine-fixture
  Scenario: Review shows the exact host names and the profile that grants network before a yes stores approval
    Given an approval checkout and isolated host state
    And the checkout requests the allowlist "api.example.com registry.example.org"
    When bare approve runs at a terminal and receives "y" after the review
    Then the transcript shows all effective settings and the worker's restrictions before exactly one y/N prompt
    And the review before its prompt says, in this order:
      | text |
      | Effective settings: |
      | allowlist-v1: checkout-only host mount at /workspace; UID:GID override; no supplementary groups; read-only root; dropped capabilities; no-new-privileges; internal network with no route out; no ports, grants or hooks; ephemeral tmpfs; pids limit 4096 |
      | This profile grants network. The worker can reach only these host names, through an owned proxy, by CONNECT on port 443: |
      | network.allow: api.example.com |
      | network.allow: registry.example.org |
      | No TLS interception: the proxy verifies the CONNECT name and the first ClientHello's SNI. |
      | It cannot see the HTTP Host inside TLS or anything after that hello, a second hello after a HelloRetryRequest included, so fronting remains a limit, as does a hello the upstream reads differently, and it cannot stop data sent to an allowed service. |
      | Clients that omit SNI or send ECH, and SSH over 443, are refused. |
      | The proxy logs each refusal, but a stored log is no proof that every refusal was kept: a log that ends early reads as a complete one. |
      | Run cannot start this profile yet: it refuses every version 2 policy for now. |
    And the review before its prompt lists exactly the requested host names
    And the review before its prompt does not say "network none"
    And the review before its prompt does not say "Effective offline settings"
    And it exits with code 0
    And the sole active approval is the exact candidate at generation "1" in protected host state
    And the checkout is unchanged and only approval state was written
    And no engine mutation or image acquisition was attempted

  # The profile words and the names come from the request, whatever the record says: the first two
  # rows change the list, where a build that read the record would name the old hosts under "only
  # these host names", and the last widens an offline approval, the one place such a build would
  # tell a person offline. The rows reach to the disclosure's last line, so a re-approval that cuts
  # it short fails.
  @wip @slice-26 @ID-APPROVAL-43 @local-engine-fixture
  Scenario Outline: Re-approval reviews the old and new network before replacing the active record
    Given an approval checkout and isolated host state
    And the checkout requests <prior>
    And a seeded matching approval at generation "7"
    And the checkout then requests <current>
    When bare approve runs at a terminal and receives " YES " after the review
    Then the transcript shows differences "<pointers>" with old and new values before exactly one y/N prompt
    And the review before its prompt says, in this order:
      | text                              |
      | allowlist-v1:                     |
      | This profile grants network       |
      | No TLS interception               |
      | Run cannot start this profile yet |
    And the review before its prompt lists exactly the requested host names
    And the review before its prompt does not say "network none"
    And the sole active approval is the exact candidate at generation "8" in protected host state
    And it exits with code 0

    Examples:
      | prior                                 | current                                         | pointers                                                                   |
      | the allowlist "a.example b.example"   | the allowlist "a.example b.example c.example"   | /policy/network/allow                                                      |
      | the allowlist "a.example b.example"   | the allowlist "a.example"                       | /policy/network/allow                                                      |
      | offline at policy version "1"         | the allowlist "a.example"                       | /policy/network/allow /policy/network/mode /policy/version /profile /proxy |

  @wip @slice-26 @ID-APPROVAL-44 @local-engine-fixture
  Scenario: Narrowing an approved allowlist to offline reviews the offline profile
    Given an approval checkout and isolated host state
    And the checkout requests the allowlist "api.example.com"
    And a seeded matching approval at generation "3"
    And the checkout then requests offline at policy version "1"
    When bare approve runs at a terminal and receives "y" after the review
    Then the transcript shows all effective settings and the worker's restrictions before exactly one y/N prompt
    And the transcript shows differences "/policy/network/allow /policy/network/mode /policy/version /profile /proxy" with old and new values before exactly one y/N prompt
    And the review before its prompt says, in this order:
      | text                       |
      | Effective offline settings |
      | offline-v1:                |
      | network none               |
    And the review before its prompt does not say "This profile grants network"
    And its standard output does not say "This profile grants network"
    And its standard output does not say "network.allow:"
    And it exits with code 0
    And the sole active approval is the exact candidate at generation "4" in protected host state

  # The offline review and status at each version, against an approval of the other. The status
  # follows the review on the same terminal, so the standard output step reads both. A version 1
  # file with no record would pass today and could not fail before the work; here the first row
  # fails at the version 2 request and the second at the version 2 record today's check refuses.
  @wip @slice-26 @ID-APPROVAL-57 @local-engine-fixture
  Scenario Outline: An offline request keeps the offline review and status at either policy version
    Given an approval checkout and isolated host state
    And the checkout requests offline at policy version "<approved>"
    And a seeded matching approval at generation "1"
    And the checkout then requests offline at policy version "<requested>"
    When bare approve runs at a terminal and receives "y" after the review
    Then the transcript shows all effective settings and the worker's restrictions before exactly one y/N prompt
    And the transcript shows differences "/policy/version" with old and new values before exactly one y/N prompt
    And the review before its prompt says, in this order:
      | text                                       |
      | Effective offline settings                 |
      | offline-v1:                                |
      | network none                               |
      | /policy/version: <approved> -> <requested> |
    And the review before its prompt does not say "This profile grants network"
    And its standard output does not say "This profile grants network"
    And it exits with code 0
    And the sole active approval is the exact candidate at generation "2" in protected host state

    Examples:
      | approved | requested |
      | 1        | 2         |
      | 2        | 1         |

  @wip @slice-26 @ID-APPROVAL-45 @local-engine-fixture
  Scenario Outline: The optional exact fingerprint refuses an allowlist grant without a terminal
    Given an approval checkout and isolated host state
    And the checkout requests the allowlist "api.example.com registry.example.org"
    And approval state is "<state>"
    When approve runs without a terminal with the independently calculated exact fingerprint and "<mode>"
    Then it reports approval problem "approve.interactive-required" with exit 1
    And its guidance names "itos container approve" and "terminal"
    And it never prompts or records approval
    And the approval checkout and host state are unchanged
    And no engine mutation or image acquisition was attempted

    Examples:
      | state    | mode                      |
      | missing  | plain                     |
      | missing  | --json                    |
      | missing  | inherited ITOS_JSON=1     |
      | missing  | ITOS_JSON=1 with --no-json |
      | matching | --json                    |

  # Unchanged settings need no second consent. The terminal status still discloses
  # the full grant, and the remembered record is not rewritten.
  @wip @slice-26 @ID-APPROVAL-46 @local-engine-fixture
  Scenario: Plain approval status says what the allowlist profile grants and cannot do
    Given an approval checkout and isolated host state
    And the checkout requests the allowlist "api.example.com registry.example.org"
    And a seeded matching approval at generation "1"
    When bare approve runs at a terminal with no answer available
    Then it reports matching approval without prompting and exits with code 0
    And the active approval bytes and generation are unchanged
    And its standard output says, in this order:
      | text |
      | "profile":"allowlist-v1" |
      | allowlist-v1: checkout-only host mount at /workspace; UID:GID override; no supplementary groups; read-only root; dropped capabilities; no-new-privileges; internal network with no route out; no ports, grants or hooks; ephemeral tmpfs; pids limit 4096 |
      | This profile grants network. The worker can reach only these host names, through an owned proxy, by CONNECT on port 443: |
      | network.allow: api.example.com |
      | network.allow: registry.example.org |
      | No TLS interception: the proxy verifies the CONNECT name and the first ClientHello's SNI. |
      | It cannot see the HTTP Host inside TLS or anything after that hello, a second hello after a HelloRetryRequest included, so fronting remains a limit, as does a hello the upstream reads differently, and it cannot stop data sent to an allowed service. |
      | Clients that omit SNI or send ECH, and SSH over 443, are refused. |
      | The proxy logs each refusal, but a stored log is no proof that every refusal was kept: a log that ends early reads as a complete one. |
      | Run cannot start this profile yet: it refuses every version 2 policy for now. |
    And its standard output lists exactly the requested host names
    And its standard output does not say "network none"
    And its standard output does not say "Effective offline settings"

  # The offline rows are what a build that ignored the list would accept, the list rows a build
  # that left a name out of the fingerprint, and the proxy rows one that left out the digest or
  # bound another.
  @wip @slice-26 @ID-APPROVAL-47 @local-engine-fixture
  Scenario Outline: A fingerprint for other network settings cannot approve the allowlist
    Given an approval checkout and isolated host state
    And the checkout requests the allowlist "api.example.com registry.example.org"
    When approve runs without a terminal with the independently calculated fingerprint of "<settings>" as JSON
    Then it reports approval problem "approve.fingerprint" with exit 1
    And no approval record or frozen input is produced
    And the approval checkout and host state are unchanged

    Examples:
      | settings                                      |
      | the same checkout offline at policy version 1 |
      | the same checkout offline at policy version 2 |
      | the list without registry.example.org         |
      | the list with other.example added             |
      | the list with no proxy member                 |
      | the list bound to another proxy image digest  |

  # A hostile workload can edit the file while the person reads the review.
  @wip @slice-26 @ID-APPROVAL-48 @local-engine-fixture
  Scenario Outline: Consent cannot approve an allowlist changed while the review was visible
    Given an approval checkout and isolated host state
    And the checkout requests the allowlist "api.example.com registry.example.org"
    When terminal approve shows its review, the fixture changes "<edit>", and then answers yes
    Then it reports terminal approval refusal "approve.changed" with exit 1
    And no approval record or frozen input is produced
    And the host state is unchanged

    Examples:
      | edit                                      |
      | add the host name other.example           |
      | remove the host name registry.example.org |
      | request offline at policy version 1       |

  # 2037 is what ID-POLICY-58 lets a list hold, and 253 bytes with 63-byte labels is the longest
  # name slice-25 admits: approval adds no limit of its own. The doctor half reads the record back,
  # so a valid() narrower than the policy fails there, on an approval approve has just written.
  @wip @slice-26 @ID-APPROVAL-49 @local-engine-fixture
  Scenario Outline: The longest list and the longest name the policy admits can be approved and still match
    Given an approval checkout and isolated host state
    And the checkout requests <request>
    When bare approve runs at a terminal and receives "y" after the review
    Then the transcript shows all effective settings and the worker's restrictions before exactly one y/N prompt
    And it exits with code 0
    And the sole active approval is the exact candidate at generation "1" in protected host state
    When itos-container runs with "doctor --json"
    Then doctor reports the independently calculated fingerprint and complete effective settings
    And its approval status is "matching"
    And it exits with code 0

    Examples:
      | request                                    |
      | an allowlist of 2037 generated host names  |
      | an allowlist of one host name of 253 bytes |

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

  # Each record is self-consistent, its fingerprint recomputed, so only the rules for what a
  # profile may hold can catch it. None can match a candidate, but a record no approval could have
  # written is invalid state, not a changed setting.
  @wip @slice-26 @ID-APPROVAL-50 @local-engine-fixture
  Scenario Outline: A self-consistent record that contradicts its profile fails closed
    Given an approval checkout and isolated host state
    And the checkout requests the allowlist "api.example.com registry.example.org"
    And a seeded matching approval with record defect "<defect>"
    When itos-container runs with "doctor --json"
    Then it reports approval problem "approval.state.invalid" with exit 1
    And the approval checkout and host state are unchanged

    Examples:
      | defect                                  |
      | allowlist names out of byte order       |
      | a repeated allowlist name               |
      | an empty allowlist                      |
      | an allowlist name in upper case         |
      | a wildcard allowlist name               |
      | a single-label allowlist name           |
      | a literal address in the allowlist      |
      | a hexadecimal final allowlist label     |
      | an empty allowlist label                |
      | an allowlist label of 64 bytes          |
      | an allowlist name of 254 bytes          |
      | no allowlist under allowlist-v1         |
      | mode offline under allowlist-v1         |
      | policy version 1 under allowlist-v1     |
      | policy version 3 under allowlist-v1     |
      | no proxy image under allowlist-v1       |
      | a proxy image without a digest          |
      | a proxy image with a short digest       |
      | a proxy image with an upper-case digest |
      | a proxy image digest with no name       |
      | profile offline-v1 with mode allowlist  |
      | an allowlist under offline-v1           |
      | a proxy image under offline-v1          |
      | policy version 3 under offline-v1       |
      | an unknown network profile              |

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
  @slice-4 @ID-APPROVAL-32 @local-engine-fixture
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
  Scenario: Matching approval reaches owned execution without asking again
    Given an approval checkout and isolated host state
    And a local owned foreground Engine API fixture
    And a seeded matching approval at generation "1"
    And the run preflight files are snapshotted
    When itos-container runs with "run -- fixture-workload --json"
    Then run completes the inspected owned foreground worker with exit zero
    And it never prompts or records approval
    And the local foreground journal contains only inspections and owned worker operations
    And the protected run snapshot is exactly the independently calculated approval at generation "1"
    And only protected snapshot intent and public session records were added without chown or checkout changes

  # The first half passes today: ID-POLICY-62 holds it beside an offline approval, and here the
  # approval is for this very request. The doctor half is what fails before the work.
  @wip @slice-26 @ID-APPROVAL-51 @local-engine-fixture @linux
  Scenario Outline: An approved version 2 request still cannot start a run
    Given an approval checkout and isolated host state
    And the checkout requests <request>
    And a seeded matching approval at generation "1"
    And the run preflight files are snapshotted
    When itos-container runs with "run -- fixture-workload"
    Then run reports preflight problem "policy.version" with exit 2 and subject "/version"
    And its error output says "doctor --policy-only"
    And the Engine API journal is empty
    And no endpoint connection was attempted
    And the active approval bytes and generation are unchanged
    And run preflight has changed no checkout or host state
    When itos-container runs with "doctor --json"
    Then doctor reports the independently calculated fingerprint and complete effective settings
    And its approval status is "matching"
    And it exits with code 0

    Examples:
      | request                                              |
      | the allowlist "api.example.com registry.example.org" |
      | offline at policy version "2"                        |

  # A version 1 file is the one request run can compare with an allowlist approval today. The
  # local comparison settles it, so no engine is needed.
  @wip @slice-26 @ID-APPROVAL-52 @local-engine-fixture @linux
  Scenario Outline: An approval for other network settings does not authorize run, and run says what changed
    Given an approval checkout and isolated host state
    And the checkout requests <approved>
    And a seeded matching approval at generation "1"
    And the checkout then requests offline at policy version "1"
    And the run preflight files are snapshotted
    When itos-container runs with "run -- fixture-workload"
    Then the Engine API journal is empty
    And no endpoint connection was attempted
    And run refuses approval "changed" with exit 1 and names "itos container approve"
    And its error output says "run.approval-changed"
    And its error output says "run approval changed at <pointers>;"
    And its error output does not say "api.example.com"
    And its standard output is empty
    And it never prompts or records approval
    And run preflight has changed no checkout or host state

    Examples:
      | approved                        | pointers                                                                       |
      | the allowlist "api.example.com" | /policy/network/allow, /policy/network/mode, /policy/version, /profile, /proxy |
      | offline at policy version "2"   | /policy/version                                                                |

  @wip @slice-26 @ID-APPROVAL-53 @local-engine-fixture @linux
  Scenario: Run names changed settings without their values
    Given an approval checkout and isolated host state
    And a seeded prior approval at generation "2" with a different application environment and workspace
    And the run preflight files are snapshotted
    When itos-container runs with "run -- fixture-workload"
    Then the Engine API journal is empty
    And run refuses approval "changed" with exit 1 and names "itos container approve"
    And its error output says "run approval changed at /manifest/containerEnv, /manifest/workspaceFolder;"
    And its error output does not say "/workspace/old"
    And its error output does not say "APP"
    And it never prompts or records approval
    And run preflight has changed no checkout or host state

  # The comparison after inspection is the other path to this refusal, and the one a changed
  # digest takes. Slice-27's allowlist runs will use both.
  @wip @slice-26 @ID-APPROVAL-54 @local-engine-fixture @linux
  Scenario: Run names a change it observed through read-only inspection
    Given an approval checkout and isolated host state
    And a seeded matching approval at generation "1"
    And run preflight has engine observation "cached image reports another config digest"
    And the run preflight files are snapshotted
    When itos-container runs with "run -- fixture-workload"
    Then the run Engine API journal contains inspection requests only
    And no engine mutation or image acquisition was attempted
    And run refuses approval "changed" with exit 1 and names "itos container approve"
    And its error output says "run approval changed at /image/id;"
    And it never prompts or records approval
    And run preflight has changed no checkout or host state

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

  # Lifting the version refusal must leave every later stage in place for a version 2 file. The
  # tree step does the looking for frozen input, as it did in ID-POLICY-61, whose absent manifest
  # row moves here: the record step beside it stops once the record is absent.
  @wip @slice-26 @ID-APPROVAL-55 @local-engine-fixture
  Scenario Outline: Approve applies every other prerequisite to a version 2 request
    Given an approval checkout and isolated host state
    And the checkout requests the allowlist "api.example.com"
    And approval input has prerequisite defect "<defect>"
    When approve runs without a terminal with a well-formed candidate fingerprint as JSON
    Then it reports prerequisite problem "<rule>" with exit <exit> before requesting consent
    And no approval record or frozen input is produced
    And the approval checkout and host state are unchanged
    And no engine mutation or image acquisition was attempted

    Examples:
      | defect                                 | rule                 | exit |
      | unsupported manifest initializeCommand | manifest.unsupported | 2    |
      | hidden image label requests a user     | manifest.metadata    | 2    |
      | cached image is absent                 | doctor.image.missing | 3    |
      | the selected manifest is absent        | manifest.missing     | 2    |

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

  # Help wraps to an inherited COLUMNS, so the run names a wide one: a narrow terminal must not
  # fail a correct build.
  @wip @slice-26 @ID-APPROVAL-56
  Scenario Outline: Approval help stops limiting approval to offline settings
    When itos-container runs with "<arguments>" and the environment "COLUMNS=200"
    Then its standard output says "Review and remember exact settings"
    And its standard output does not say "exact offline settings"
    And it exits with code 0

    Examples:
      | arguments      |
      | approve --help |
      | --help         |

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

  @slice-4 @ID-APPROVAL-33 @local-engine-fixture
  Scenario Outline: Terminal approval escapes Unicode controls in current settings and prior differences
    Given an approval checkout and isolated host state
    And approval review contains "<controls>" controls in the "<source>" environment
    When bare approve runs at a terminal and receives "y" after the review
    Then the terminal review escapes "<controls>" controls from the "<source>" environment before consent
    And it exits with code 0
    And the sole active approval is the exact candidate at generation "<generation>" in protected host state
    And any stored or JSON settings preserve the exact literal environment value

    Examples:
      | controls      | source  | generation |
      | C1            | current | 1          |
      | bidirectional | current | 1          |
      | C1            | prior   | 2          |
      | bidirectional | prior   | 2          |

  # A hostile manifest's controls must not reach the terminal while a person consents to a network
  # grant. "Effective settings:" is a new line, so ID-APPROVAL-33 cannot hold it. The step reads all
  # of standard output for raw controls, so the status after the yes is held too, and the prior rows
  # read the /manifest/containerEnv difference, so a review that drops it fails.
  @wip @slice-26 @ID-APPROVAL-58 @local-engine-fixture
  Scenario Outline: Terminal approval of an allowlist escapes Unicode controls in current settings and prior differences
    Given an approval checkout and isolated host state
    And the checkout requests the allowlist "api.example.com"
    And approval review contains "<controls>" controls in the "<source>" environment
    When bare approve runs at a terminal and receives "y" after the review
    Then the terminal review escapes "<controls>" controls from the "<source>" environment before consent
    And it exits with code 0
    And the sole active approval is the exact candidate at generation "<generation>" in protected host state
    And any stored or JSON settings preserve the exact literal environment value

    Examples:
      | controls      | source  | generation |
      | C1            | current | 1          |
      | bidirectional | current | 1          |
      | C1            | prior   | 2          |
      | bidirectional | prior   | 2          |

  @slice-4 @ID-APPROVAL-34 @local-engine-fixture
  Scenario Outline: Plain approval status escapes Unicode controls without changing approved settings
    Given an approval checkout and isolated host state
    And approval review contains "<controls>" controls in the "current" environment
    When approve runs without a terminal with the independently calculated exact fingerprint and ""
    Then the plain approval status escapes "<controls>" controls and preserves the effective settings
    And it exits with code 0
    And the sole active approval is the exact candidate at generation "1" in protected host state
    And any stored or JSON settings preserve the exact literal environment value

    Examples:
      | controls      |
      | C1            |
      | bidirectional |

  @slice-4 @ID-APPROVAL-35 @local-engine-fixture
  Scenario Outline: Doctor escapes Unicode controls only for plain approval display
    Given an approval checkout and isolated host state
    And the approval candidate contains "<controls>" controls in "<field>"
    When itos-container runs with "doctor"
    Then the plain approval status escapes "<controls>" controls and preserves the effective settings
    And doctor reports approval "missing" and names "itos container approve"
    And it exits with code 0
    And the approval checkout and host state are unchanged
    When itos-container runs with "doctor --json"
    Then doctor reports the independently calculated fingerprint and complete effective settings
    And it exits with code 0
    And the approval checkout and host state are unchanged

    Examples:
      | controls      | field        |
      | C1            | containerEnv |
      | bidirectional | containerEnv |
      | C1            | name         |
      | bidirectional | name         |

  @wip @slice-26 @ID-APPROVAL-59 @local-engine-fixture
  Scenario Outline: A network grant requires both review terminals and plain output
    Given an approval checkout and isolated host state
    And the checkout requests the allowlist "api.example.com"
    When approve runs at a terminal in "<mode>" with streams "<streams>"
    Then it reports approval problem "approve.interactive-required" with exit 1
    And its guidance names "itos container approve" and "terminal"
    And it never prompts or records approval
    And the approval checkout and host state are unchanged

    Examples:
      | mode                  | streams                           |
      | plain                 | redirected stdin and stdout       |
      | plain                 | terminal stdin, redirected stdout |
      | plain                 | redirected stdin, terminal stdout |
      | --json                | terminal stdin and stdout         |
      | inherited ITOS_JSON=1 | terminal stdin and stdout         |
