@phase-1
Feature: Refuse checkout files that share writable host inodes
  A checkout-only mount can still expose an outside file through a hard link.
  Read-only file permissions do not protect an inode owned by the worker's UID:
  the worker can chmod it and write through either name. Inspection, approval
  and launch must refuse this layout without repairing the person's files.

  # Authorities: slice-20's registry rationale, docs/charter.md, the pilot's
  # checkout-only mount boundary and AC-04/05/07/25, and architecture.md, Slices.
  # Extend shared readiness domain/port/disk contracts, consumed by the existing
  # doctor, approve and run slices and session launch validation. Recovery keeps
  # decision 0012's independence from mutable checkout contents. No slice imports
  # another; cmd assembles adapters. Domain tests use I/O-free porttest fakes and
  # the same contracts as adapters. Reuse Go's pinned standard-library filesystem
  # APIs and the already pinned golang.org/x/sys v0.48.0 for unavailable calls.
  # No new dependency, Git subprocess, custom parser, or production test switch.
  # OS is an input value; preserve the existing exact-package import allowlists.

  # Decision: refuse, never automatically copy, unlink, chmod or chown. Every
  # regular file below the selected checkout, recursively including .git,
  # hidden, ignored and untracked paths, must have exactly one filesystem link
  # at admission. Conservatively refuse even when all known names are inside
  # the checkout: a list of local aliases does not prove there is no outside
  # name. Directory link counts are not regular-file link counts. A read-only
  # regular file with one link is allowed; an independently copied/reflinked
  # inode is allowed. This spec requires no particular copy/reflink mechanism.
  # Inspect metadata through the pinned checkout without following symlinks or
  # reading linked file contents to decide sharing. Existing symlink, policy,
  # manifest and Git-layout rules remain in force. Never consult Git's tracked
  # files or ignore rules to prune the inspection. An unreadable subtree or
  # unavailable link-count observation cannot be treated as safe.
  #
  # A pre-existing multiple link reports doctor.checkout.hardlink, exit 1,
  # with the checkout root as its safe path subject. Explain hard links and
  # direct the person to prepare an independent checkout without shared file
  # inodes; do not suggest approval or chmod as a fix. Ordinary doctor/approve
  # preserve the schema-1 problem envelope; run preserves the same prerequisite
  # diagnostic on stderr and empty stdout. An unreadable subtree uses existing
  # doctor.checkout.unavailable, exit 3. Both refuse before engine connections
  # and before consent, snapshot publication, registration or worker creation.
  # File-only doctor modes retain their existing scope and make no layout claim.
  #
  # Approval never caches layout safety. Recheck on each invocation, after a
  # consent wait, after engine inspection and at the existing launch validation
  # boundaries before create/start. A link introduced while inspection is paused
  # is doctor.checkout.changed, exit 1; after review it is approve.changed, exit 1.
  # A late unsafe layout after durable ownership uses run.ownership-refused,
  # exit 1, never starts the worker, and still cleans only its owned resources.
  # Source bytes/link counts do not enter the approval fingerprint; replacing a
  # shared file with an independent copy does not by itself need new approval.
  #
  # This preserves independent host inodes for an admitted worker under the
  # pilot's trusted-host assumption and the sole checkout mount. Hard links the
  # worker creates wholly inside that mount cannot acquire an outside inode.
  # They do not block cleanup, even though a later run conservatively refuses
  # the resulting multiply linked layout. No continuous host-filesystem monitor
  # or protection from a trusted host deliberately adding outside aliases to a
  # running worker is claimed. Overlapping workers and binding the actual mount
  # to the validated root remain slice-16/slice-17's separate requirements.

  # Enumeration is bounded even when every inode is independent. Each full scan,
  # including Unchanged rechecks, reads batches of at most 128 entries and shares
  # a 100,000-entry allowance and a 10-second monotonic deadline across the whole
  # tree. Count every returned entry, including symlinks and directories. The
  # root is depth zero; allow descendants through directory depth 64. Check time
  # between filesystem operations; this does not promise to interrupt a blocked
  # kernel filesystem call. No detached scanner may outlive a refusal.
  # Exhaustion cannot establish safety: initial inspection reports the existing
  # doctor.checkout.unavailable (exit 3); revalidation fails closed through its
  # existing changed/ownership refusal. Never prune or accept a partial scan.
  # Reuse pinned Go standard-library directory batching and monotonic time;
  # no library, subprocess, user-configurable limit or production test switch.
  # Adapter tests exercise exact entry/depth boundaries, cumulative accounting,
  # bounded reads and deterministic elapsed-time exhaustion without sleeps.

  # Fixture and step handoff:
  # Reuse doctor/approval checkout creation, the independent approval oracle,
  # seeded approval state, local owned foreground API, helper sentinels, generic
  # CLI invocation, doctor/approval problem assertions and terminal refusal.
  # Those steps exist in doctor_steps_test.go, approval_steps_test.go,
  # run_preflight_steps_test.go and steps_test.go. All phrases containing
  # "hard-link" below are NEW, including actions/assertions, not undefined-step
  # evidence. The late-start and native assertions are new too.
  #
  # Every fixture lives below one disposable parent on one filesystem, with
  # private HOME/XDG state and a non-root effective UID. "outside" creates a
  # second name under a sibling source directory that is never mounted; "inside"
  # creates a second name elsewhere inside the checkout. Use real os.Link, not
  # synthetic stat results. Preserve existing policy/manifest/config bytes; create
  # other named files with synthetic data. The loose object and pack paths model
  # a local clone's shared storage directly, without running Git. An "ignored"
  # path also gets an ordinary .gitignore entry. Set the requested mode before
  # linking. Record device/inode/link counts for both names and root/.git identity
  # independently; Givens only prepare/capture, Thens check those observations.
  #
  # The hard-link checkpoint records bytes, modes, owners, device/inode/link
  # counts, mtime and all entries in checkout/outside/host state, excluding API
  # journals and atime. Record again after intentional fixture changes before
  # resuming a paused child. Assertions distinguish those changes from product
  # effects and catch new files, repairs or lost links. They never reset an
  # outside sentinel or discard an earlier process result. Never follow symlinks
  # or open special files to snapshot them. Helper markers stay absent.
  #
  # The unreadable-subtree Given writes .slice-20-closed/nested/file, links it
  # outside, captures its known entries, then makes .slice-20-closed mode 0000.
  # Its new action/checkpoint must not use doctorTree/runTree's recursive read of
  # that subtree, which would fail in setup. Observe the inaccessible directory's
  # metadata and the outside alias; restore access only after recording the
  # product result and directory mode, then inspect its known contents. Invoke
  # doctor --json, approve --json --fingerprint <independent exact fingerprint>,
  # or run -- /bin/true as named. Use the successful local foreground API for run.
  #
  # Local run actions use startRun/runPreflightProcess so the API knows the child
  # PID and literal argv, not runDoctor alone. A hard-link checkpoint initializes
  # that run fixture when needed. Barriers wait for the response/prompt OR process
  # exit with a finite deadline and always retain early exits for Then. No sleeps
  # as synchronization and no product injection flags. Local API fixtures have
  # no connection to Docker and never execute the supplied workload.

  @slice-20 @ID-HARDLINK-01 @linux @local-engine-fixture
  Scenario Outline: Doctor refuses multiply linked files anywhere under the checkout
    Given a doctor checkout
    And a local Engine API fixture
    And executable marker fixtures for Git hooks, fsmonitor, credential helpers, builds and PATH tools
    And the hard-link fixture links "<path>" to an "<alias>" name with mode "<mode>"
    And the hard-link fixture files are checkpointed
    When itos-container runs with "doctor --json"
    Then its JSON output has a doctor problem "doctor.checkout.hardlink" with exit 1
    And the hard-link diagnostic names the checkout and explains independent file inodes
    And the hard-link observations prove the two names shared an inode before invocation
    And the Engine API journal is empty
    And no endpoint connection was attempted
    And no executable marker was created
    And the hard-link fixture trees and link relationships are unchanged

    Examples:
      | path                                                                 | alias   | mode |
      | src/file.txt                                                         | outside | 0600 |
      | untracked/deep/file.txt                                              | outside | 0600 |
      | ignored/deep/cache                                                   | outside | 0600 |
      | .hidden/deep/file                                                    | outside | 0600 |
      | .git/HEAD                                                            | outside | 0600 |
      | .git/config                                                          | outside | 0600 |
      | .git/objects/aa/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa               | outside | 0444 |
      | .git/objects/aa/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa               | outside | 0600 |
      | .git/objects/pack/pack-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa.pack | outside | 0444 |
      | itos-container.yaml                                                  | outside | 0600 |
      | .devcontainer/devcontainer.json                                      | outside | 0600 |
      | src/file.txt                                                         | inside  | 0600 |

  # This must fail closed, not silently skip the part that hides a shared inode.
  @slice-20 @ID-HARDLINK-02 @linux @local-engine-fixture
  Scenario Outline: An unreadable subtree cannot establish checkout independence
    Given an approval checkout and isolated host state
    And a local owned foreground Engine API fixture
    And a seeded matching approval at generation "1"
    And the hard-link fixture hides a linked file below an unreadable subtree
    When the hard-link fixture invokes "<command>" with valid arguments
    Then the hard-link command reports "doctor.checkout.unavailable" with exit 3
    And the Engine API journal is empty
    And no endpoint connection was attempted
    And the hard-link unreadable subtree and outside alias were not repaired or changed
    And the hard-link fixture host state is unchanged

    Examples:
      | command |
      | doctor  |
      | approve |
      | run     |

  # The retry also catches overbroad refusal of directories (which normally have
  # several links), read-only regular files, or bytes identical to outside files.
  @slice-20 @ID-HARDLINK-03 @linux @local-engine-fixture
  Scenario: An independent copy restores readiness without changing approved settings
    Given an approval checkout and isolated host state
    And a seeded matching approval at generation "1"
    And the hard-link fixture links ".git/objects/aa/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" to an "outside" name with mode "0444"
    And the hard-link fixture files are checkpointed
    When doctor inspects the hard-link checkout, the fixture replaces its linked entry with an independent copy, and doctor inspects again
    Then the first hard-link inspection reports "doctor.checkout.hardlink" with exit 1
    And the second hard-link inspection is ready with the original matching approval and fingerprint
    And only the second hard-link inspection contacted the engine and used inspection requests only
    And the hard-link copy has identical initial bytes and mode but a different inode and one link
    And the hard-link outside file and approval are unchanged apart from the fixture's removed alias

  @slice-20 @ID-HARDLINK-04 @linux @local-engine-fixture
  Scenario Outline: Exact consent and a prior approval cannot authorize a shared checkout inode
    Given an approval checkout and isolated host state
    And approval state is "<state>"
    And the hard-link fixture links "src/file.txt" to an "outside" name with mode "0600"
    And the hard-link fixture files are checkpointed
    When approve runs without a terminal with the independently calculated exact fingerprint and "--json"
    Then it reports prerequisite problem "doctor.checkout.hardlink" with exit 1 before requesting consent
    And the hard-link diagnostic names the checkout and explains independent file inodes
    And the Engine API journal is empty
    And no endpoint connection was attempted
    And the hard-link fixture trees and link relationships are unchanged

    Examples:
      | state    |
      | missing  |
      | matching |

  # No prior approval, so terminal approve must reach its real y/N prompt. The
  # action adds an outside alias without changing bytes, settings or root identity
  # after the review, captures that fixture edit, and answers yes through the PTY.
  @slice-20 @ID-HARDLINK-05 @linux @local-engine-fixture
  Scenario: A link added during terminal review invalidates that review
    Given an approval checkout and isolated host state
    And an independent hard-link fixture file at "src/file.txt" with mode "0600"
    And the hard-link fixture files are checkpointed
    When terminal approve shows its review, the hard-link fixture links "src/file.txt" outside, and then answers yes
    Then it reports terminal approval refusal "approve.changed" with exit 1 after showing the review
    And the hard-link action reached the review before adding the alias
    And no approval record or frozen input is produced
    And no engine mutation or image acquisition was attempted
    And the hard-link fixture trees and link relationships are unchanged

  # Seed consent before adding the alias. File contents are not part of approval;
  # matching root/.git identities and fingerprints must not bypass fresh checks.
  @slice-20 @ID-HARDLINK-06 @linux @local-engine-fixture
  Scenario Outline: Run rechecks hard links added after approval before any startup effect
    Given an approval checkout and isolated host state
    And a local owned foreground Engine API fixture
    And a seeded matching approval at generation "1"
    And the hard-link fixture links "<path>" to an "outside" name with mode "<mode>"
    And the hard-link fixture files are checkpointed
    When itos-container runs with "run -- /bin/true"
    Then run reports preflight problem "doctor.checkout.hardlink" with exit 1 and subject "checkout path"
    And the hard-link diagnostic names the checkout and explains independent file inodes
    And the Engine API journal is empty
    And no endpoint connection was attempted
    And the hard-link fixture trees and link relationships are unchanged

    Examples:
      | path                                                   | mode |
      | .git/objects/aa/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa | 0444 |
      | ignored/deep/cache                                     | 0600 |
      | .devcontainer/devcontainer.json                        | 0600 |

  # Both edits leave root/.git identities, bytes and approved settings unchanged.
  # "add alias" links the existing file outside; "replace with linked inode"
  # installs a different inode with the same bytes/mode and an outside alias.
  # Pause the first image response after local validation; do not re-seed approval.
  @slice-20 @ID-HARDLINK-07 @linux @local-engine-fixture
  Scenario Outline: Revalidate file independence after engine inspection
    Given an approval checkout and isolated host state
    And a local owned foreground Engine API fixture
    And a seeded matching approval at generation "1"
    And an independent hard-link fixture file at "src/file.txt" with mode "0600"
    And the hard-link fixture files are checkpointed
    When "<command>" runs and the hard-link fixture performs "<edit>" before releasing its image response
    Then the hard-link command reports "doctor.checkout.changed" with exit 1
    And the hard-link action reached the image barrier before installing the shared inode
    And the hard-link observations retain the original checkout and Git directory identities
    And no engine mutation or image acquisition was attempted
    And the hard-link fixture trees and link relationships are unchanged

    Examples:
      | command | edit                      |
      | doctor  | add alias                 |
      | doctor  | replace with linked inode |
      | run     | add alias                 |
      | run     | replace with linked inode |

  # Pause the successful local foreground API's attach response after it has
  # observed durable ownership of the full worker ID, before returning the 101
  # upgrade to run. Add the alias then resume. The API would accept Start; it
  # must record a forbidden attempt rather than enforce the product's decision.
  # Cleanup uses original protected ownership, not another checkout admission.
  @slice-20 @ID-HARDLINK-08 @linux @local-engine-fixture
  Scenario: A late hard link prevents start without stranding the owned worker
    Given an approval checkout and isolated host state
    And a local owned foreground Engine API fixture
    And a seeded matching approval at generation "1"
    And an independent hard-link fixture file at "src/file.txt" with mode "0600"
    And the hard-link fixture files are checkpointed
    When run reaches owned attachment and the hard-link fixture links "src/file.txt" outside before releasing attachment
    Then run refuses with "run.ownership-refused" and exit 1 without any worker start request
    And the hard-link action reached attachment after durable full-ID ownership
    And the unstarted full-ID worker is removed with confirmed absence and completed protected cleanup
    And the hard-link checkout and outside file are unchanged apart from the fixture's added alias
    And the original approval and unrelated resources are unchanged

  # Runtime evidence only in guarded CI, using README.md's cached native fixture
  # and request recorder. All native phrases here are NEW, reusing the existing
  # offline runtime fixture infrastructure. Seed consent independently before
  # adding the alias; never invoke approve to arrange a passing run. Setup,
  # observation and cleanup all retain @engine. No local engine invocation.
  #
  # The trusted test probe is copied as an independent executable into the temp
  # checkout. It chmods the selected 0444 file to 0600, opens that SAME inode and
  # overwrites a synthetic marker, never replaces the pathname with a new file.
  # It also creates a hard link wholly inside /workspace, writes through it and
  # leaves both names as checkout edits. Invoke it by explicit argv, never on the
  # host. The native recorder allows normal startup and records all attempts.
  # After writing, the probe emits a readiness frame and waits for stdin release
  # so the harness can independently inspect the live worker. Race that frame
  # against process exit and the fixture deadline; an early refusal is a result
  # for Then, never a missing-handshake setup failure.
  #
  # Retain the first result and outside-file observations before repair. The
  # fixture then unlinks only the checkout name and installs a byte-for-byte
  # independent copy with the original mode, retaining root/.git identities,
  # approval, endpoint and image. Retry the SAME probe without new consent.
  # Independently inspect each live control worker and its mount/UID boundaries,
  # both outside sentinel observations, the checkout edit and final full-ID
  # absence before harness cleanup. No broken probe/engine may count as denial.
  @slice-20 @ID-HARDLINK-09 @linux @engine
  Scenario Outline: A shared host file is protected while an independent writable copy remains useful
    Given a CI-only hard-link checkout with a cached native worker and isolated seeded approval
    And the hard-link fixture links "<path>" to an "outside" name with mode "0444"
    And the native hard-link write probe and independent file observations are prepared
    When run attempts the hard-link write probe, the fixture replaces the checkout link with an independent copy, and run retries the probe
    Then the first native hard-link run reports "doctor.checkout.hardlink" with exit 1 before any worker creation or start
    And the outside file's bytes mode owner and inode stayed unchanged after each run
    And the second native hard-link run executes the probe as the approved UID and GID with only the checkout host mount
    And the second probe writes through its in-checkout hard link and exits with code 0
    And the second run preserves those checkout edits and confirms full-ID worker absence despite the new in-checkout hard link
    And neither run changes approval or unrelated resources or acquires an image

    Examples:
      | path                                                   |
      | .git/objects/aa/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa |
      | src/file.txt                                           |

  # Red-first handoff, not evidence from this spec-only commit:
  # Commit complete step definitions while these remain @wip. For 01-08 remove
  # @wip only for local assertion-level red runs, then implement and show green.
  # For 09 the coordinator dispatches its exact ID at the step commit in the
  # guarded engine tier; only CI supplies red/green, never the agent's host.
  # Today checkout.go only inspects embedded Git entries and directory identity;
  # it does not inspect regular-file link counts or enumerate ordinary subtrees.
  # Each case is expected to fail its FIRST Then: 01/02/03/04/06/07 get success
  # instead of the specified refusal; 05 records approval despite the added link;
  # 08 starts the fixture worker; 09 admits the native write to a shared inode.
  # Every outline row must fail at that assertion, not in setup or an undefined
  # step. Keep both phases of 03/09: their refusal fails before the fix and their
  # positive control rules out rejecting all checkouts or a nonworking probe.
  # Preserve adjacent doctor/approval/run and foreground recovery scenarios.

  @wip @slice-20 @ID-HARDLINK-10 @linux @local-engine-fixture
  Scenario Outline: Checkout enumeration refuses excessive size or depth before side effects
    Given an approval checkout and isolated host state
    And a local owned foreground Engine API fixture
    And a seeded matching approval at generation "1"
    And the checkout enumeration fixture exceeds the "<budget>" budget
    And the hard-link fixture files are checkpointed
    When the hard-link fixture invokes "<command>" with valid arguments
    Then the hard-link command reports "doctor.checkout.unavailable" with exit 3
    And the Engine API journal is empty
    And no endpoint connection was attempted
    And the hard-link fixture trees and link relationships are unchanged

    Examples:
      | command | budget  |
      | doctor  | entries |
      | doctor  | depth   |
      | approve | depth   |
      | run     | depth   |

  @wip @slice-20 @ID-HARDLINK-11 @linux @local-engine-fixture
  Scenario Outline: Checkout revalidation has the same enumeration budgets
    Given an approval checkout and isolated host state
    And a local owned foreground Engine API fixture
    And a seeded matching approval at generation "1"
    And the hard-link fixture files are checkpointed
    When "<command>" runs and the checkout exceeds the depth budget before its image response
    Then the hard-link command reports "doctor.checkout.changed" with exit 1
    And the checkout budget action reached the image barrier
    And no engine mutation or image acquisition was attempted
    And the hard-link fixture trees and link relationships are unchanged

    Examples:
      | command |
      | doctor  |
      | run     |
