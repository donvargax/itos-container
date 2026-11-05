@phase-1
Feature: Run invocation refusals
  Run requires an explicit nonempty executable after -- and refuses wrapper JSON
  before attempting startup. These local cases introduce the run command; they
  prove no container isolation or successful workload execution.

  # Authorities: docs/plans/offline-run-lifecycle.md, Arguments and JSON (T-11,
  # AC-21/22), docs/CLI.md, and docs/architecture/arc42.md, slice-7. The future
  # implementation belongs in internal/slices/run under the Slices rules. Reuse
  # the pinned kong v1.16.1 and generic UI; no engine client belongs in this item.
  # Domain tests supplement these black-box cases with I/O-free port fakes and
  # zero resource calls; a recorder alone is not runtime isolation evidence.
  # All cases are local, without @engine. Core dispatch is represented by its
  # inherited ITOS_JSON=1 at the extension entry point, never the installed itos.
  # Keep every scenario @wip for the implementer's red step-definition commit.
  # Undefined steps are not red evidence. Remove @wip for assertion-level runs
  # only after those steps exist, before implementing, then demonstrate green.

  # Existing policy fixtures are reused only as harness infrastructure. They
  # create private host directories, a TCP connection recorder (no daemon), and
  # harmless git/docker/devcontainer/sh/bash/npm executables that record calls.
  # PATH names only those executables; the generated manifest contains a host
  # hook sentinel, not permission to run it. No sandbox policy or approval exists.
  # The snapshot records checkout/host-state bytes and modes after setup. These
  # Givens only prepare fixtures; all effect checks are explicit Then steps.
  Background:
    Given policy inspection has isolated host state and a recording engine endpoint
    And inspection helper sentinels are installed
    And the policy inspection files are snapshotted

  # Required-command guidance must beat the old generic unknown-command error
  # and missing-policy errors. A later nonempty argument cannot replace an empty
  # executable, even when it looks like a wrapper flag or another separator.
  # NEW assertion: "its standard output is empty" checks zero bytes.
  @ID-RUN-01 @slice-7
  Scenario Outline: Run requires a nonempty executable after the separator
    When itos-container runs with "<arguments>"
    Then its error output says "run requires a nonempty command after --"
    And its error output says "itos container run -- <command>"
    And its error output says "itos-container:"
    And it exits with code 2
    And its standard output is empty
    And the engine recorder has no connections
    And no inspection helper sentinel was executed
    And the policy inspection files are unchanged

    Examples:
      | arguments           |
      | run                 |
      | run --              |
      | run -- ''           |
      | run -- '' git       |
      | run -- '' --json    |
      | run -- '' --help    |
      | run -- '' -- git    |

  # T-11 requires the separator, not merely a positional command. Do not infer
  # a shell or silently move arguments across the missing separator.
  @ID-RUN-02 @slice-7
  Scenario: Run refuses a command supplied without the separator
    When itos-container runs with "run git status"
    Then its error output says "run requires a nonempty command after --"
    And its error output says "itos container run -- <command>"
    And its error output says "itos-container:"
    And it exits with code 2
    And its standard output is empty
    And the engine recorder has no connections
    And no inspection helper sentinel was executed
    And the policy inspection files are unchanged

  # Existing When steps capture redirected streams. Redirection never enables
  # batch execution. Wrapper options are accepted on either side of "run", up
  # to --. A workload --no-json cannot cancel a wrapper or inherited request.
  # NEW assertion below parses the entire stdout as exactly one JSON object,
  # with schema 1, ok false, and exactly one problem with the given rule and
  # numeric exit. Require nonblank message and fix; message explains run's JSON
  # refusal and fix directs removal of both --json and ITOS_JSON=1. Allow additive
  # fields per CLI.md, but no other stdout or second document. Wording is human
  # guidance, not byte-for-byte JSON serialization. Stderr stays separate.
  @ID-RUN-03 @slice-7
  Scenario Outline: Run refuses wrapper and inherited JSON with redirected streams
    When itos-container runs with "<arguments>" and the environment "ITOS_JSON=<inherited>"
    Then its run JSON reports exactly one refusal with rule "run.json-unsupported" and exit 2
    And it exits with code 2
    And its error output says "itos-container:"
    And its error output says "run"
    And its error output says "JSON"
    And the engine recorder has no connections
    And no inspection helper sentinel was executed
    And the policy inspection files are unchanged

    Examples:
      | arguments                  | inherited |
      | --json run -- git           | 0         |
      | run --json -- git           | 0         |
      | run -- git                  | 1         |
      | run --json -- git --json    | 0         |
      | run --json -- git --no-json | 0         |
      | run -- git --json           | 1         |
      | run -- git --no-json        | 1         |
      | run -- --no-json            | 1         |
      | --json run -- git --no-json | 1         |

  # NEW When step: the same literal argv/environment as runWith, but stdin and
  # stdout are a real pseudoterminal; stderr is captured independently. Allocate
  # and verify the terminal before launching the binary, drain its output, and
  # close/reap it with a bounded harness deadline. Do not fake TTY detection or
  # merge stderr into the JSON channel. A fixture failure is a failure, no skip.
  # Select github.com/creack/pty v1.1.24 for this test-only fixture on the initial
  # Linux target: maintained PTY allocation avoids handwritten ioctl/session
  # plumbing. Preserve its MIT notice; pin it when the implementer adds steps.
  # Source: https://github.com/creack/pty/releases/tag/v1.1.24 . Existing pinned
  # golang.org/x/term v0.46.0 can verify terminal descriptors. This selection
  # adds no runtime dependency, engine access, or additional host support claim.
  # @linux excludes this fixture before setup on other platforms; portable
  # refusal cases still run there. Allocation failures on Linux remain failures.
  @ID-RUN-04 @slice-7 @linux
  Scenario Outline: Run refuses JSON at a terminal as it does with redirected streams
    When itos-container runs at a terminal with "<arguments>" and the environment "ITOS_JSON=<inherited>"
    Then its run JSON reports exactly one refusal with rule "run.json-unsupported" and exit 2
    And it exits with code 2
    And its error output says "itos-container:"
    And its error output says "run"
    And its error output says "JSON"
    And the engine recorder has no connections
    And no inspection helper sentinel was executed
    And the policy inspection files are unchanged

    Examples:
      | arguments                  | inherited |
      | --json run -- git           | 0         |
      | run --json -- git --no-json | 0         |
      | run -- git --no-json        | 1         |

  # Interface choice for competing invocation failures: the requested JSON
  # refusal wins over missing argv/separator. Emit the single T-11 refusal,
  # never a usage object, combined problem list, or a plain-text-only error.
  @ID-RUN-05 @slice-7
  Scenario Outline: A JSON request is refused even when the command is missing
    When itos-container runs with "<arguments>" and the environment "ITOS_JSON=<inherited>"
    Then its run JSON reports exactly one refusal with rule "run.json-unsupported" and exit 2
    And it exits with code 2
    And its error output says "itos-container:"
    And its error output says "run"
    And its error output says "JSON"
    And the engine recorder has no connections
    And no inspection helper sentinel was executed
    And the policy inspection files are unchanged

    Examples:
      | arguments        | inherited |
      | --json run       | 0         |
      | run --json --    | 0         |
      | run --json -- '' | 0         |
      | run              | 1         |
      | run --           | 1         |
      | run -- ''        | 1         |
      | run --json git   | 0         |

  # Slice-8 preflight and frozen publication (AC-02/03/04/05/06/38/41).
  # Authorities: the charter, arc42 Foreground run steps 1/2, decision 0003,
  # features/approval.feature's canonical settings/storage/frozen-input contracts,
  # and the coordinator's settled slice-8 decisions. Preserve slice-7's JSON and
  # argv precedence. Reuse strict policy/manifest/readiness and slice-4's identity,
  # canonicalization and approval selection; do not add a second resolver/loader.
  #
  # Refusal ordering and journal contract:
  # Invalid invocation/input and approval failures decidable without a daemon
  # leave the Engine API journal EMPTY. Inspect local checkout identity, files,
  # effective host IDs and protected active approval first. Compare locally known
  # canonical fields with that record before engine access; this is a preliminary
  # refusal check, never enough to establish matching approval. Missing, corrupt,
  # unsafe or locally changed approval must not need a reachable daemon.
  # In particular, a changed manifest image reference is locally known: refuse
  # before inspecting either image. A missing cached image or changed inspected
  # image config digest with the SAME requested reference needs engine inspection.
  # Never fill unobserved engine/image fields from approval to claim a match.
  # Checks needing daemon/image observations MAY inspect. Permit only HEAD/GET
  # ping, GET version/info and GET inspection of the exact requested image, with
  # the existing SDK negotiation. No mutation, pull, build, alternate image or
  # helper execution is permitted, even on a failed/redirected request. Revalidate
  # checkout/socket/daemon identities as readiness already requires. Preflight
  # refusals before publication publish no snapshot and never prompt, write consent
  # or repair unsafe state. The later unimplemented-startup refusal follows a
  # successful preflight and DOES retain its durable snapshot (ID-RUN-13/19).
  # ID-RUN-08/09/10 require an empty journal; ID-RUN-11/12 require a nonempty
  # inspection-only journal. ID-RUN-14 names the required class for each change.
  # An empty journal cannot satisfy an inspection-only expectation. Check each
  # invocation's own journal, not the combined requests of a multi-run scenario.
  # Empty-journal process cases also assert zero connections to the selected
  # Unix API fixture, so connecting without sending a request cannot pass.
  #
  # Effective UID identifies the host user; root is refused. Effective UID/GID
  # override all image Config.User values. Both manifest users must equal that
  # UID or be omitted and resolve to it. Never use USER/LOGNAME/UID/GID/SUDO_* as
  # identity, look up image usernames, inherit supplementary groups or chown.
  #
  # Presentation choice: preserve shared prerequisite rule IDs, subjects, exits,
  # messages and fixes (decision 0004), including doctor.* names; do not flatten
  # them into run.approval-invalid. Plain run stderr, prefixed itos-container:,
  # identifies the rule, safe field/path/image subject and actionable guidance.
  # No new JSON mode. Empty stdout holds before any workload exists. Missing,
  # locally changed, invalid and superseded approval retain exit 1 and explicit
  # itos container approve guidance. Identity root/mismatch exit 1; unavailable
  # IDs exit 3, with the existing approval.identity.* rules. Never print raw
  # rejected values, argv, host environment or engine response bodies.
  #
  # Publication choice: a snapshot is immutable material for later sessions,
  # not a session ID, ownership record, current-approval substitute or run permit.
  # Store it at snapshots/<project>/<generation>.json under the SAME protected
  # itos-container state root as approval. project/generation come only from
  # slice-4's successful atomic Freeze, never from argv or checkout path text.
  # Reuse its canonical envelope {schema:1,project,generation,fingerprint,settings}
  # and validation/size/depth rules, with settings the exact selected canonical
  # object. Store no timestamp, argv, inherited environment or workload output.
  # This choice avoids allocating lifecycle/session IDs before their owning spec.
  # Later session records may reference this material; retention here cannot
  # authorize a future run without full fresh preflight and current approval.
  #
  # Apply slice-4's outside-checkout, pinned/no-follow, ownership, safe-ancestor,
  # link-count and regular-file rules to snapshot paths too: effective-UID-owned
  # 0700 directories, 0600 files, independent of umask. Never chmod/chown existing
  # unsafe state. Serialize publication per project; write a private temporary
  # file, check complete writes, fsync it, atomically publish, fsync the parent
  # and persist any newly created ancestor directory entries before returning a
  # usable result. An existing identical, safely validated snapshot is reusable
  # without rewriting its bytes/inode/mtime; a conflicting or malformed record
  # refuses, never overwrites. Concurrent publishers cannot expose partial bytes
  # or replace one another's valid snapshot. Re-establish durability on reuse
  # before returning, including after a previous parent-fsync uncertainty.
  #
  # Publication consumes only the independent Frozen value. Return the exact
  # frozen value and durable snapshot reference to run's later startup boundary;
  # no checkout reread or current.json alias may supply that handoff. Freeze's
  # atomic active-generation check is the selection point: replacement before it
  # refuses as superseded, replacement after it cannot change selected settings.
  # Later approval or request edits neither rewrite A nor widen A's handoff.
  # A fresh invocation must revalidate even when A's snapshot already exists.
  #
  # Snapshot path/ownership/link failures use run.snapshot.unsafe, corrupt or
  # conflicting records run.snapshot.invalid (exit 1); publication I/O failures
  # use run.snapshot.write (exit 75). Give safe host-state guidance, never suggest
  # chmod/chown or automatically approve. Before publication, failure leaves no
  # final snapshot; after rename/parent-fsync uncertainty it may leave a COMPLETE
  # record but returns no usable snapshot/handoff and never claims success.
  # Existing approval and unrelated snapshots stay unchanged. No resource call
  # can follow either refusal. Temporary files are never accepted as snapshots.
  #
  # Scope and implementation boundary:
  # Extend internal/slices/run/{command,domain,port,<named adapter siblings>}.
  # Shared preflight composition, frozen values and protected snapshot storage
  # belong in the existing approval/readiness engines under the Slices rules.
  # Keep all I/O behind ports, assemble only in cmd, and run shared contracts
  # against I/O-free port/porttest fakes and actual adapters. Reuse pinned
  # github.com/moby/moby/client v0.6.2 and api v1.56.1 for inspection, existing
  # parsers/canonical codec/store protections, Go 1.27.3 file/JSON/hash APIs and
  # golang.org/x/sys v0.48.0 for unavailable no-follow/locking calls. No new
  # dependency, hand-written parser/client, allowlist change or production test
  # switch. Pass OS as a value. No worker create/start, intent/recovery schema,
  # stop/status promise, resource budget change, hooks or runtime proof here.
  #
  # Successful preflight still reaches the exit-3 unimplemented-startup refusal
  # and existing doctor --policy-only guidance; that guidance is not permission.
  # Slice-8 supersedes ONLY the blanket unchanged-host-state expectations of
  # ID-RUN-06/07 and ID-APPROVAL-26: the implementer replaces them with exact
  # snapshot-only changes, preserving argv ownership, no engine mutation/pull,
  # no checkout/approval rewrite, no helpers, empty stdout and the interim refusal.
  # Their unrelated policy fixture must be snapshotted/compared with the expected
  # new snapshot accounted for. Do not re-tag or weaken those live scenarios.
  # Sibling feature edits belong to implementation, not this specification commit.
  #
  # Fixture and new-step handoff:
  # Existing approval checkout/seed, image Config.User, misleading identity env,
  # generic CLI, refusal, journal and sentinel steps are reused unchanged. All
  # run-preflight/snapshot-specific phrases below are NEW. The Background's TCP
  # trap is separate from the Unix HTTP Engine API journal, never a substitute.
  # @local-engine-fixture selects Linux amd64 before setup; no real Docker or
  # operator state is used. Seed canonical approvals directly from the independent
  # fixture oracle, never by invoking approve, and snapshot only AFTER all edits.
  # New checkpoint steps capture bytes, mode, owner, links, inode and mtime of
  # checkout/approval/outside targets; ignore access times and API journals.
  # Refusal assertions compare the entire before/after tree, including absence
  # of any new snapshot or lock, and absence of all helper markers/chown effects.
  # Positive assertions allow only the specified snapshot subtree/locking entries.
  # Allow parent-directory mtime changes and link-count increases due to the new
  # subdirectories only; preserve existing directory modes, owners and inodes.
  # Preserve every existing regular file's bytes, mode, owner, links, inode and
  # mtime. Never follow symlinks or open FIFOs while checkpointing; observe them
  # with lstat/readlink like slice-4.
  # The NEW child-umask Given records only a launch option. Apply it to the product
  # child with the existing trusted test-wrapper pattern, preserving literal argv;
  # do not change the test process's umask or create fixtures under that mask.
  #
  # Local-defect Givens change one named fixture field/file only. Local approval
  # conditions below mean absent state, a copy only inside checkout, malformed
  # current.json, a different literal APP environment, historical A plus active B,
  # or 0644 current.json as named. For "manifest image differs from A", seed A
  # first, then change only manifest.image to the same repository with a different
  # valid sha256 digest. Keep current.json and the API fixture's cached image A
  # unchanged; no replacement image is prepared or acquired. This is a local
  # approval mismatch even though inspecting the replacement would fail readiness.
  # No Given calls the product or asserts it.
  # Engine-defect Givens modify only the synthetic API responses after seeding
  # approval: blank info.ID; version product Podman; version/info architecture
  # arm64; image 404; RepoDigests containing only a different digest; image arm64;
  # missing Config; or devcontainer.metadata [{"containerUser":"1000"}].
  # Read-only journal assertions require at least one inspection request
  # where stated, and reject EVERY other request, even one answered with failure.
  # Require version/info observations for daemon refusals and the exact pinned
  # image inspection for image refusals; do not require image access after an
  # earlier daemon refusal or fix the number of SDK negotiation/recheck calls.
  # Paused-response actions wait for the barrier OR process exit with a bounded
  # harness deadline; missing inspection must reach Then, never hang in setup.
  # Replacements are wholly inside the temporary parent and captured before resume.
  #
  # A run-preflight problem assertion checks the actual process exit, rule, subject,
  # cause and fix on stderr, empty stdout and no startup-success/prompt claim.
  # Subject aliases "checkout path", "requested image" and "snapshot path" mean
  # the actual fixture root, pinned image reference and protected snapshot path;
  # compare their safely escaped rendering, never those literal alias words.
  # Exact wording is not fixed beyond existing required guidance. Snapshot
  # assertions decode the WHOLE record and independently calculate bytes/hash,
  # project, generation, resolved UID/GID, modes/owner and placement; finding any
  # file or searching for a substring is insufficient. Check the immutable record
  # after each invocation, not only after the last one. Multi-run When steps
  # capture each result and observation even when the first invocation refuses.
  # ID-RUN-14 deletes or truncates only current.json for its approval-state edits;
  # it leaves the first snapshot intact. Capture the edited state before run two,
  # so an unchanged-approval assertion never attributes the fixture edit to run.
  # Its "manifest requests another digest" edit changes only manifest.image as
  # above. "cached image config digest" instead changes only the API's image Id
  # to another valid sha256 digest, retaining the manifest reference and attested
  # RepoDigests. The latter therefore requires inspection to find the mismatch.
  # Partition API requests by invocation; run one's inspections cannot hide a
  # forbidden call or supply a missing observation in run two's journal.
  # The second-run assertion's "empty" class also requires zero connections in
  # that invocation; run one's connection count is excluded, not reset globally.
  #
  # Contract scenarios use a test-only cmd assembly of the REAL run preflight and
  # shared approval/snapshot contracts, like the existing slice-4 contract probe.
  # Root/unavailable IDs are supplied through identity ports, never privilege or
  # environment overrides. Selection/handoff barriers are port-bound test support,
  # not product flags. Exercise publication faults with the same suite against
  # the memory fake AND filesystem adapter in a temporary tree; adapter-specific
  # I/O fault seams must drive real file writes/fsync/rename, not fake verdicts.
  # Feature steps read subprocess results, never import/read production source.
  # Thin compileable unavailable stubs are allowed in the red step commit.
  # Every action retains early failure as data and reaches its Then assertion.
  # The A/B contract actions install B's valid request and active record before
  # the fresh invocation. For the post-selection action, mutate the caller's
  # input and copies returned to consumers during the When, retain the original
  # handoff, and observe both snapshots; the Then steps only compare observations.
  # "No checkout reads after selection" applies within each invocation, not to
  # the next invocation's required fresh preflight. Contract positive publication
  # and independence controls run against both memory and disk stores as faults do.
  # ID-RUN-18 captures the first result and visible record BEFORE clearing the
  # fault, then retries full fresh preflight against the same store/state. The
  # cleared-fault retry is a positive control, not permission to retry a failed
  # write inside the first invocation. Record file and required directory fsyncs
  # on the fake and real adapter; a retry handoff must follow successful durability
  # operations, even when the first attempt left a complete reusable snapshot.
  # Compare that record's bytes/inode/mtime across retry; never delete it to force
  # the fresh-file path. A failure before any first snapshot exists still reaches
  # Then; it must not become a setup failure in the retry or visibility checks.
  #
  # Commit steps red first with @wip; run each example locally with @wip removed
  # and require assertion-level red, then green after implementation. No @engine
  # case is added. Expected failures below are a handoff, NOT observed red proof.

  # Current run collapses all prerequisite diagnostics into approval invalid.
  @wip @slice-8 @ID-RUN-08 @local-engine-fixture
  Scenario Outline: Run preserves local validation refusals before engine inspection
    Given an approval checkout and isolated host state
    And a seeded matching approval at generation "1"
    And run preflight has local defect "<defect>"
    And the run preflight files are snapshotted
    When itos-container runs with "run -- fixture-workload"
    Then run reports preflight problem "<rule>" with exit <exit> and subject "<subject>"
    And the Engine API journal is empty
    And no endpoint connection was attempted
    And run preflight has changed no checkout or host state
    And no inspection helper sentinel was executed

    Examples:
      | defect                                      | rule                       | exit | subject                     |
      | policy is missing                           | policy.missing             | 2    | itos-container.yaml         |
      | policy requests network mode online         | policy.network_mode        | 2    | /network/mode               |
      | selected manifest is missing                | manifest.missing           | 2    | .devcontainer/devcontainer.json |
      | manifest requests initializeCommand         | manifest.unsupported       | 2    | /initializeCommand          |
      | manifest has a duplicate image key          | manifest.duplicate         | 2    | /image                      |
      | containerUser differs from effective UID    | approval.identity.mismatch | 1    | containerUser               |
      | remoteUser differs from effective UID       | approval.identity.mismatch | 1    | remoteUser                  |
      | embedded .git is a symlink outside checkout  | doctor.checkout.path       | 1    | checkout path               |

  # Today even missing/corrupt approval contacts the synthetic daemon; the empty
  # journal is the first failing assertion, paired with the precise refusal.
  @wip @slice-8 @ID-RUN-09 @local-engine-fixture
  Scenario Outline: Locally decidable approval failures need no daemon
    Given an approval checkout and isolated host state
    And run preflight has local approval condition "<condition>"
    And the run preflight files are snapshotted
    When itos-container runs with "run -- fixture-workload"
    Then the Engine API journal is empty
    And no endpoint connection was attempted
    And run refuses approval "<reason>" with exit 1 and names "itos container approve"
    And its error output says "run.approval-<reason>"
    And its standard output is empty
    And run preflight has changed no checkout or host state
    And no inspection helper sentinel was executed

    Examples:
      | condition                              | reason  |
      | no host approval                       | missing |
      | approval only inside the checkout      | missing |
      | truncated active approval              | invalid |
      | manifest environment differs from A    | changed |
      | manifest image differs from A          | changed |
      | historical A with active B             | changed |
      | active approval has mode 0644          | invalid |

  # The real run entry currently flattens these failures, and inspects the engine
  # before resolving host identity. The contract first checks the specific cause.
  @wip @slice-8 @ID-RUN-10
  Scenario Outline: Run refuses unusable effective host identity before engine access
    Given a run preflight contract fixture with effective UID "<uid>" and GID "<gid>"
    When the run preflight contract receives valid explicit argv
    Then the run preflight contract reports "<rule>" with exit <exit>
    And the run preflight contract made no engine call, snapshot write or startup handoff

    Examples:
      | uid         | gid         | rule                          | exit |
      | 0           | 1000        | approval.identity.root        | 1    |
      | unavailable | 1000        | approval.identity.unavailable | 3    |
      | 1000        | unavailable | approval.identity.unavailable | 3    |

  # These refusals need inspection; an empty journal would be the wrong oracle.
  # Current run returns generic approval invalid instead of each shared cause.
  @wip @slice-8 @ID-RUN-11 @local-engine-fixture
  Scenario Outline: Engine and cached image refusals preserve their cause without acquisition
    Given an approval checkout and isolated host state
    And a seeded matching approval at generation "1"
    And run preflight has engine observation "<observation>"
    And the run preflight files are snapshotted
    When itos-container runs with "run -- fixture-workload"
    Then run reports preflight problem "<rule>" with exit <exit> and subject "<subject>"
    And the run Engine API journal contains inspection requests only
    And no engine mutation or image acquisition was attempted
    And run preflight has changed no checkout or host state
    And no inspection helper sentinel was executed

    Examples:
      | observation                       | rule                   | exit | subject                  |
      | missing daemon ID                 | doctor.engine.identity | 1    | DOCKER_HOST              |
      | unsupported engine product        | doctor.engine.unsupported | 1 | DOCKER_HOST              |
      | engine reports linux arm64        | doctor.engine.platform | 1    | DOCKER_HOST              |
      | requested image returns not found | doctor.image.missing   | 3    | requested image          |
      | image attests a different digest  | doctor.image.digest    | 1    | requested image          |
      | image reports linux arm64         | doctor.image.platform  | 1    | requested image          |
      | image response has no Config      | doctor.image.inspect   | 75   | requested image          |
      | metadata requests containerUser   | manifest.metadata      | 2    | /0/containerUser         |

  # Existing readiness detects these races, but run loses their diagnostic.
  @wip @slice-8 @ID-RUN-12 @local-engine-fixture
  Scenario Outline: Run refuses identity substitution during inspection before freezing
    Given an approval checkout and isolated host state
    And a seeded matching approval at generation "1"
    And run preflight pauses the Engine API before its image response
    And the run preflight files are snapshotted
    When run starts and the fixture replaces "<identity>" before releasing the image response
    Then run reports preflight problem "<rule>" with exit 1 and subject "<subject>"
    And the run Engine API journal contains inspection requests only
    And run preflight has changed no checkout or host state apart from the fixture replacement
    And no engine mutation or image acquisition was attempted

    Examples:
      | identity            | rule                    | subject       |
      | checkout root       | doctor.checkout.changed | checkout path |
      | embedded .git       | doctor.checkout.changed | checkout path |
      | daemon ID           | doctor.engine.changed   | DOCKER_HOST   |
      | socket device/inode | doctor.engine.changed   | DOCKER_HOST   |

  # Current run discards Frozen and writes nothing; the snapshot assertion fails.
  # Image user is excluded from canonical settings; changing it cannot choose UID.
  @wip @slice-8 @ID-RUN-13 @local-engine-fixture
  Scenario Outline: Approved preflight publishes exact settings with host identity
    Given an approval checkout and isolated host state
    And the run manifest users are "<users>"
    And the inspected image Config.User is "<image-user>"
    And misleading USER, LOGNAME, UID, GID, SUDO_UID and SUDO_GID fixture values
    And a seeded matching approval at generation "1"
    And the run child has umask "<umask>"
    And the run preflight files are snapshotted
    When itos-container runs with "run -- fixture-workload '' 'two words' --json"
    Then the protected run snapshot is exactly the independently calculated approval at generation "1"
    And the snapshot resolves both manifest users to the effective UID and binds the effective GID
    And run reports that approval matched before refusing unimplemented offline startup with exit 3
    And its error output says "doctor --policy-only"
    And its standard output is empty
    And it never prompts or records approval
    And only the protected run snapshot subtree was added without chown or checkout changes
    And the Engine API journal contains only ping, version, info and the requested image inspection
    And no inspection helper sentinel was executed

    Examples:
      | users                | image-user | umask |
      | omitted              |            | 0000  |
      | omitted              | 0:0        | 0000  |
      | effective host UID   | root       | 0000  |
      | effective host UID   | 9999:8888  | 0000  |
      | omitted              |            | 0777  |
      | omitted              | 0:0        | 0777  |
      | effective host UID   | root       | 0777  |
      | effective host UID   | 9999:8888  | 0777  |

  # A cached snapshot must never become an alternate approval/readiness source.
  # Capture the first run's snapshot before editing; today it is absent.
  @wip @slice-8 @ID-RUN-14 @local-engine-fixture
  Scenario Outline: A later run revalidates even when a frozen snapshot exists
    Given an approval checkout and isolated host state
    And a seeded matching approval at generation "1"
    And the run preflight files are snapshotted
    When run is invoked, the fixture changes "<change>", and run is invoked again
    Then the first run published the exact protected snapshot for generation "1"
    And the second run reports "<rule>" with exit <exit> and journal "<journal>"
    And the first snapshot and active approval were not rewritten by either run
    And the second run published no snapshot and made no startup handoff
    And neither run attempted engine mutation, image acquisition or helper execution

    Examples:
      | change                           | rule                 | exit | journal         |
      | active approval is removed       | run.approval-missing | 1    | empty           |
      | active approval is truncated     | run.approval-invalid | 1    | empty           |
      | literal manifest environment     | run.approval-changed | 1    | empty           |
      | manifest requests another digest | run.approval-changed | 1    | empty           |
      | daemon ID at the same endpoint   | run.approval-changed | 1    | inspection only |
      | cached image config digest       | run.approval-changed | 1    | inspection only |
      | requested image becomes absent   | doctor.image.missing | 3    | inspection only |

  # Exercise run's actual selection-to-publication ordering, not just slice-4's
  # already-proven Select refusal. A fresh B result must publish as a control.
  @wip @slice-8 @ID-RUN-15
  Scenario: Superseded selection cannot publish while a fresh selection can
    Given a run preflight contract with independently seeded approvals A and B
    When approval B replaces A after run inspects A but before frozen selection and a fresh run selects B
    Then the fresh run publishes exactly B and returns its durable frozen handoff
    And the stale run reports "run.approval-superseded" with exit 1 and approve guidance
    And the stale run published no snapshot and made no startup handoff
    And neither contract run attempted engine mutation or image acquisition

  # After Freeze succeeds, later changes cannot alter snapshot or returned input.
  # The controller's subsequent phase receives the durable value, not file paths
  # from which to reconstruct settings. No real workload is run by this contract.
  @wip @slice-8 @ID-RUN-16
  Scenario Outline: Snapshot and handoff retain selected settings after later edits
    Given a run preflight contract with independently seeded approvals A and B
    When run selects A, the fixture makes "<change>" before publication and handoff, and a fresh run follows with B's approved inputs
    Then the published snapshot and startup handoff contain exactly A's canonical bytes and approval identifiers
    And the snapshot and retained handoff are unchanged by the fixture's caller-input and returned-buffer mutations
    And no checkout settings are read after frozen selection
    And a fresh run of approved B publishes B without changing A
    And neither contract run attempted engine mutation or image acquisition

    Examples:
      | change                                         |
      | policy now requests forbidden online networking |
      | manifest now requests a forbidden extra mount   |
      | active approval is replaced by B                |

  # Only the snapshots subtree is unsafe; valid approval remains readable.
  # Today run ignores this subtree and reaches exit 3 instead of this refusal.
  @wip @slice-8 @ID-RUN-17 @local-engine-fixture
  Scenario Outline: Snapshot publication refuses unsafe or conflicting existing state
    Given an approval checkout and isolated host state
    And a seeded matching approval at generation "1"
    And the run snapshot path has "<defect>"
    And the run preflight files are snapshotted
    When itos-container runs with "run -- fixture-workload"
    Then run reports preflight problem "<rule>" with exit 1 and subject "snapshot path"
    And run preflight has changed no checkout or host state
    And no engine mutation or image acquisition was attempted
    And no inspection helper sentinel was executed

    Examples:
      | defect                               | rule                 |
      | snapshots directory is a symlink      | run.snapshot.unsafe  |
      | project directory is a symlink        | run.snapshot.unsafe  |
      | project directory has mode 0777       | run.snapshot.unsafe  |
      | final record is a symlink             | run.snapshot.unsafe  |
      | final record is a FIFO                | run.snapshot.unsafe  |
      | final record has another hard link    | run.snapshot.unsafe  |
      | final record has mode 0644            | run.snapshot.unsafe  |
      | final record is truncated JSON        | run.snapshot.invalid |
      | final record contains other settings  | run.snapshot.invalid |

  # Same contract runs against memory and disk, with real run error translation.
  # Current run has no publication stage, so cannot report a publication failure.
  @wip @slice-8 @ID-RUN-18
  Scenario Outline: Failed durable publication never yields a usable startup input
    Given a run snapshot contract with publication failure "<failure>"
    When approved run preflight attempts publication through each store implementation and fresh preflight retries after the fault is cleared
    Then each first run reports "run.snapshot.write" with exit 75 and returns no durable handoff
    And each first attempt leaves final snapshot "<visible>" and no partial snapshot is accepted
    And each retry returns exactly A's frozen handoff only after durable snapshot publication
    And any complete snapshot left by the first attempt is reused without changing its bytes, inode or mtime
    And active approval, prior snapshots and outside targets remain unchanged
    And no failed attempt made a later startup call
    And neither attempt made an engine mutation or image acquisition call

    Examples:
      | failure                    | visible                    |
      | private file creation      | absent                     |
      | short write                | absent                     |
      | file fsync                 | absent                     |
      | rename                     | absent                     |
      | parent directory fsync     | absent or complete A       |
      | new ancestor fsync         | absent or complete A       |

  # This positive control prevents a publisher from satisfying all fault cases
  # by always refusing. Compare each process's result and independently read file.
  @wip @slice-8 @ID-RUN-19 @local-engine-fixture
  Scenario Outline: Repeated and concurrent preflights reuse one complete immutable snapshot
    Given an approval checkout and isolated host state
    And a seeded matching approval at generation "1"
    And the run preflight files are snapshotted
    When two runs for the same approval execute "<schedule>" and a third run follows
    Then all three runs observe the same exact protected snapshot for generation "1"
    And all three reach only the unimplemented-startup refusal with exit 3 and empty stdout
    And the final snapshot is never partial and its inode and mtime remain unchanged after publication
    And the active approval bytes and generation are unchanged
    And only the protected run snapshot subtree was added without chown or checkout changes
    And no engine mutation or image acquisition was attempted
    And no inspection helper sentinel was executed

    Examples:
      | schedule     |
      | sequentially |
      | concurrently |

  # The registry and arc42 explicitly require valid argv to refuse until later
  # increments. Interface choice: classify unavailable offline startup as a
  # missing requirement (exit 3), explain that it is not implemented, and point
  # to policy-only doctor as inspection, never as permission or a run fallback.
  # Slice-4 supersedes the original pre-prerequisite refusal. Valid argv now
  # checks a valid approved temporary checkout, preserving the same argument
  # boundary checks and the refusal to launch an unimplemented worker.
  @ID-RUN-06 @slice-7 @local-engine-fixture
  Scenario: A valid command refuses while offline startup is not implemented
    Given an approval checkout and isolated host state
    And a seeded matching approval at generation "1"
    And the policy inspection files are snapshotted
    When itos-container runs with "run -- git status"
    Then its error output says "offline startup is not implemented"
    And its error output says "doctor --policy-only"
    And its error output says "itos-container:"
    And it exits with code 3
    And its standard output is empty
    And no engine mutation or image acquisition was attempted
    And the approval checkout and host state are unchanged
    And the engine recorder has no connections
    And no inspection helper sentinel was executed
    And the policy inspection files are unchanged

  # Acceptance of workload flags must reach the same interim refusal, not help,
  # JSON, flag validation or empty-command handling. ITOS_JSON is cleared by the
  # clean harness environment. This proves option ownership only; actual argv
  # preservation (including empty arguments) and worker execution belong to the
  # later CI real-engine run slice, not to this refusal-only increment.
  # The first word after the first separator is the executable, even when it is
  # --json, --help or --. It is nonempty syntax; do not resolve it on the host.
  # Later separators remain workload arguments and never restart wrapper parsing.
  @ID-RUN-07 @slice-7 @local-engine-fixture
  Scenario Outline: Arguments after the separator belong only to the workload
    Given an approval checkout and isolated host state
    And a seeded matching approval at generation "1"
    And the policy inspection files are snapshotted
    When itos-container runs with "<arguments>"
    Then its error output says "offline startup is not implemented"
    And its error output says "doctor --policy-only"
    And its error output says "itos-container:"
    And it exits with code 3
    And its standard output is empty
    And no engine mutation or image acquisition was attempted
    And the approval checkout and host state are unchanged
    And the engine recorder has no connections
    And no inspection helper sentinel was executed
    And the policy inspection files are unchanged

    Examples:
      | arguments                     |
      | run -- git --json              |
      | run -- git --no-json           |
      | run -- git --help              |
      | run -- git --version           |
      | run -- git --bogus             |
      | run -- git -- '' 'two words' - |
      | run -- --json                 |
      | run -- --help                 |
      | run -- --                     |
      | run -- git -- --help          |
