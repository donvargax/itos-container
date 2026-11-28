@phase-1
Feature: Deliver the approved Claude token to one owned run
  Run copies the approved source's current bytes to a private read-only worker file.
  Approval binds the grant, not its contents. Provisioning exposes no token in
  configuration, records or diagnostics, and owned teardown removes the copy.

  # Authorities: charter milestone two step 2, contract AC-02/03/22/23/28/29 and
  # AC-36 AS REVISED BY q-56, decisions 0025, 0027, 0029, 0035 and 0041,
  # architecture.md Slices, arc42 Credential run, q-52/67/69 and T-93's step rule.
  # Rows land in stages from slice-31's breakdown: T-95 lands 08 to 10 @wip with
  # their native steps and the shared harness; T-96 adds 11 and 12; the local rows
  # 01 to 07, 13 and 14 join with the items that own them. slice-31's complete
  # reviewed header, with the admission, recheck, presentation, raw-contract and
  # supersession rules those items carry, is this file at 48ce147.
  #
  # Decision 0035: container runs may use real credentials, the person's smoke
  # check included; Claude gets a claude setup-token token, staged as below for
  # the workload to export as CLAUDE_CODE_OAUTH_TOKEN. No scenario uses a real
  # credential, provider/model call, login, refresh or account: every source
  # here is synthetic. No Codex kind, GitHub delivery, automatic environment
  # export, harness init or new privilege belongs here.
  # Decision 0041: a source filesystem type that by itself prevents excluding a
  # backing alias raises credential.filesystem as a warning naming the path and
  # type, in doctor and in run status, and the run continues; an independently
  # observed prohibited location or any other independent refusal still applies.
  #
  # q-56 supersedes ADR 0025's open question 6 and blanket log-absence wording:
  # provisioning must not leak, but hostile workload output/traffic is unredacted.
  # A workload can put token bytes in a refused hostname; proxy stdout, Docker's
  # proxy log and the retained refusal log can then contain them. Verbatim target
  # logging remains. p1-platform-log-redaction owns scrubbing and the adversarial
  # hostname-marker probe; neither blocks these rows. Preserve that disclosure,
  # do not hash hostnames here, and do not claim general AC-36 closure.
  #
  # Delivery is T-48's existing mechanism, not a new session lifecycle:
  # retain checked descriptors, reopen /proc/self/fd/<n> rather than source names,
  # and copy once without trimming, including final newlines. Intent precedes
  # private tmpfs staging under /dev/shm/itos-container-<32 lowercase hex digits>,
  # directory 0700, file 0400, host UID. Bind ONLY the staged regular file
  # read-only at /run/itos-container/credentials/claude-token. The original
  # source, siblings and host staging directory are never mounted. Keep all
  # existing hardening, private IPC, worker /dev/shm, literal argv as the main
  # process and no restart. Never put source bytes in controller-set Env/argv,
  # Engine API bodies, labels, inspect, approvals, snapshots, intents, recovery,
  # diagnostics or files under checkout/state. The workload may explicitly read
  # the file and export CLAUDE_CODE_OAUTH_TOKEN; the controller does not do that
  # or call a shell on its behalf. After start wait and the applicable mount
  # detector, attempt staged unlink on every path; an acquired mount survives
  # unlink. Completion requires worker AND staging absence plus every existing
  # owned resource, never just a missing pathname. The staged file is that run's
  # own copy (decision 0035): rotating valid contents or inode at the approved
  # path between runs is not an approval change, never replaces a live worker's
  # copy, and reaches only a later run's staging.
  #
  # Sources contain only fresh "itos-synthetic-credential-" plus 32 random lower
  # hex digits, with optional padding/final newlines. Hashes are calculated by the
  # harness and cooperative worker only, never production approval/provisioning.
  # Hash-only probe reports include nonce, path, mode/UID and attempted operations;
  # absent reports cannot satisfy either positive or isolation assertions.
  # Scan raw markers and base64 at all three offsets, with positive-control
  # files for EVERY detector. Scan all request/response metadata (including error
  # bodies), full worker inspect, text/JSON status, doctor/approve output, run/stop
  # diagnostics, approval/snapshot/intent/recovery and all checkout/state files.
  # Checkout absence is also continuous, not only final: each When first arms a
  # recursive inotify write observer on every checkout directory (create,
  # modify, attrib, close-write, moves, delete and queue overflow) and proves it
  # with a fixture-made transient create, write and remove of its own. Later
  # fixture control writes go only to one fixture control directory and are
  # journaled by exact name. The cooperative probe and any child it runs write
  # nothing under /workspace, only under the worker's private tmpfs /tmp.
  # Sources, state roots and the account home lie outside every checkout, so
  # their fixture edits are not checkout events. The only fixture edits inside a
  # checkout are three policy edits, each a single
  # journaled operation on that checkout's exact itos-container.yaml path: 05's
  # policy row replaces it while run is held at its post-freeze barrier; 10
  # replaces the first project's after every earlier run's controller has
  # exited and before the later run starts; 12 unlinks it after the killed
  # controller has been reaped and before any fresh status or stop.
  # A replacement never writes through the policy path. The harness writes and
  # closes the new policy in a staging file in the private fixture root outside
  # every checkout, checks it shares the checkout's st_dev (else a fixture
  # blocker), records its inode and SHA-256, and then makes exactly one
  # rename(2) over the policy path. An unlink is exactly one unlink(2). The
  # harness's edit helper journals every mutating syscall it issues in a
  # bracket, with its path, and the bracket requires exactly one rename or one
  # unlink there and no open-for-write, write or truncate. That journal, not
  # the inotify count, attributes the fixture's own mutation.
  # One inotify instance per checkout keeps its events in order. The harness
  # brackets each edit with uniquely named begin and end sentinel creates in the
  # control directory, and in the same When calibrates the expected event kinds
  # by the identical operation on a calibration file there: one rename of a
  # staged file over an existing one, or one unlink. Inside a bracket the
  # policy path must show exactly that calibrated move-to or delete event and
  # no modify, close-write, attrib or create event. Every other writer is
  # accounted for by event kind: a write, truncate or recreation on that name
  # by anyone emits an event the calibrated sequence lacks, and coalescing can
  # merge repeats only into one such event, never into the expected one. A
  # second rename carries its own cookie and a second delete needs a create
  # between them, so neither coalesces into the calibrated event. Every other
  # writer is also held for the bracket: each product process the harness
  # launched is reaped or parked at its journaled, unreleased barrier, and each
  # live worker's cooperative probe is parked at a barrier that writes nothing.
  # After the end sentinel the policy path's inode and SHA-256 equal the staged
  # file's, or the path is absent after an unlink. The observer stays armed
  # throughout. Any other checkout event, an extra or out-of-bracket event on
  # the policy path, a missing or extra journaled mutation, a changed staged
  # result, a missing bracketed edit, an unjournaled name or an overflow fails
  # every provisioning-surface assertion, even when the final marker scan and
  # checkout comparison pass.
  # The red-first test commit also adds an engine-free harness regression,
  # TestCredentialRunPolicyEditBracket, run by ordinary go test over a
  # disposable checkout with the observer armed. Its negative controls open the
  # policy file once and make two writes through that one descriptor inside one
  # bracket: before a scheduled replacement, after it on the new file, and
  # before a scheduled unlink. Each must fail the bracket check. Its positive
  # control, the calibrated single replacement or unlink alone, must pass. It
  # tests the harness, so it passes before and after the work and is not
  # scenario red evidence.
  # Include proxy stdout, its Docker log and retained refusal log in 08's allowlist
  # row, whose ordinary refusal hostname contains NO marker. Require a nonempty
  # observed refusal and a successful controlled TLS exchange, not empty logs.
  # In 08 invoke doctor in plain/JSON and matching bare approve at a terminal
  # without an answer, retaining their output before run; collect plain/JSON
  # status while the hash probe waits after startup. Require successful results
  # and no new consent/publication. Do not scan absent command transcripts.
  # Fixture-owned sources, staged bytes while authorized, positive controls and
  # fixture-only journals containing inputs are excluded from searches of product
  # persistence; actual Engine API captures and product state are never excluded.
  # Workload transport carries only hashes in these cooperative probes. Failure
  # messages identify the leaking surface, never print markers or full records.
  #
  # @engine means guarded Linux amd64 native Docker CI ONLY, using the cached
  # image and non-root host UID/GID. Native fixture: one private 0700 root under
  # /var/tmp, as slice-29's credential fixture uses, holds the checkout, the
  # selected XDG_STATE_HOME, the source directory and the account home that the
  # existing engine probe assembly receives as SLICE_21_ACCOUNT_HOME. Only that
  # seam relocates the fixed <account home>/.local/state/itos-container boundary;
  # a private HOME or XDG_STATE_HOME does not, and the runner's real account
  # state is never used. Before any product command the Given independently
  # verifies, by its own statfs, statx(STATX_MNT_ID) and mountinfo parse and
  # never the product observer, for the source, checkout, selected state root,
  # fixed account state root and every ancestor of each: ext4, btrfs or xfs; a
  # mount ID naming exactly one row with a path root; no row of any other type
  # at, below or covering those four paths; every ancestor owned by root or the
  # host UID and not group/other-writable unless root-owned sticky; and /dev/shm
  # a root-owned sticky tmpfs. A failed verification fails that Given at its own
  # step (T-93), naming only path, type and UID: a fixture blocker, never red
  # evidence. The three types are required so doctor reports the source ready
  # with no credential.filesystem warning (decision 0041); the list is never
  # widened to admit a runner. Each @engine When then runs JSON doctor on every
  # fixture checkout before run. Doctor's readiness passes today, so it proves
  # the fixture clears shared readiness and an unrelated readiness refusal
  # cannot stand in for the credential red.
  #
  # Shard placement: T-77 put the whole file in the egress shard (ci.yml's egress
  # include and egressShardFiles); it is no longer pending once on the tree. Keep
  # four shards, whole files, the 180-second warning target and the 600-second
  # hang guard. Red dispatch and activation must each show the egress shard
  # selecting exactly this file's @engine rows (08 to 10: 3, 2 and 1 rows; with
  # T-96's 11 and 12: 3, 2, 1, 2 and 2) and no other shard selecting any.
  #
  # Helpers built by the harness run only inside disposable workers. No real
  # service/credential, pull/build, fallback image or agent Docker access. The
  # allowlist row reuses slice-27's isolated fake endpoint and positive controls.
  # Audit product effects before bounded identity-checked harness cleanup; that
  # cleanup is never evidence of product teardown. No credential bytes are
  # deliberately written into the checkout, even for a negative-control probe.
  #
  # A Given or When that cannot finish its own setup or action (a fixture,
  # barrier, readiness wait, timeout or harness engine call that fails) returns
  # that error at its own step (T-93). A When keeps for Then only what the
  # product was observed to do: exits, streams, probe reports, records and
  # engine observations, an early refusal included. Commit missing steps red
  # first with @wip retained; use only coordinator-dispatched CI for engine
  # red/green evidence. Today run refuses version 3 with policy.version at
  # /version, exit 2, before any engine contact, while doctor and approve accept
  # it, so 08 to 12's first Then, the fixture and doctor control, passes today
  # and they fail at the next Then. These are expected failures, not executed
  # evidence. No local mutation or whole-features race run. The implementer may
  # commit freely in its own workspace; the host makes the landing commits and
  # handles itos, submission, publication and review.

  # Real mount acceptance and cooperative provisioning absence, including a
  # nonempty ordinary refusal log in the allowlist row under q-56's limit.
  @wip @slice-31 @ID-CREDRUN-08 @engine @linux
  Scenario Outline: The approved workload alone reads a read-only staged Claude file
    Given a CI Claude checkout with a private synthetic source and matching "<network>" approval
    And the cooperative credential probe and every marker detector have independent positive controls
    When the product inspects with doctor and matching terminal approve then runs the credential probe through active status and workload exit <exit>
    Then the native fixture verification passed and JSON doctor reported every readiness check and the Claude source ready
    And the worker reports the exact source hash including final newlines from the fixed Claude path as the non-root host UID
    And native inspection and worker probes show one read-only mode-0400 regular Claude file and all existing hardening
    And write attempts fail and the original host source its sibling and host staging directory cannot be opened from the worker
    And no controller-set environment or argv contains a token or automatically exports CLAUDE_CODE_OAUTH_TOKEN
    And its network evidence is "<network evidence>"
    And every specified cooperative provisioning surface contains neither the raw marker nor any base64-offset encoding
    And staged paths are absent after acknowledged startup while the worker can still read the copy
    And run returns the observed workload exit <exit> after confirmed removal of the worker staging and all other owned resources
    And the checkout source approval and unrelated resources are unchanged

    Examples:
      | network   | exit | network evidence                                                            |
      | offline   | 0    | network none with no proxy or refusal log                                   |
      | offline   | 17   | network none with no proxy or refusal log                                   |
      | allowlist | 0    | successful controlled TLS and an ordinary denied hostname in all log copies |

  @wip @slice-31 @ID-CREDRUN-09 @engine @linux
  Scenario Outline: A live worker keeps its copy and the next run reads the rotated source
    Given a CI Claude checkout with a private synthetic source and matching "offline" approval
    And a second distinct synthetic source value is reserved for rotation by "<rotation>"
    When the product inspects with JSON doctor then starts a waiting hash probe then the fixture rotates the source and releases a second read
    Then the native fixture verification passed and JSON doctor reported every readiness check and the Claude source ready
    And both reads in that worker report the original hash and the source reports only the rotated value
    When that worker exits and the product runs a fresh hash probe without another approval
    Then the fresh worker reports the rotated hash through a newly staged Claude file
    And both runs keep the original approval fingerprint and generation without a consent prompt
    And both runs confirm complete owned cleanup and no cooperative provisioning leak

    Examples:
      | rotation                            |
      | rewriting the private file in place |
      | replacing it with a valid new inode |

  @wip @slice-31 @ID-CREDRUN-10 @engine @linux
  Scenario: One project's grant cannot supply another worker or a later no-grant run
    Given two independent CI Claude projects with distinct synthetic sources and separately matching approvals
    And a third independent approved version three project requests no credentials
    When the product inspects every project with JSON doctor then runs their probes concurrently and stops the first project while the other probes remain active
    Then the native fixture verification passed and JSON doctor reported every readiness check and each Claude source ready
    And each credential worker reports only its own source hash and cannot open the other project's source or staged path
    And the no-grant worker executes its probe and cannot open either token or the fixed Claude path
    And stopping the first project removes only its resources while the second still reads its original copy
    When the remaining probes finish and the fixture removes the first project's grant and independently seeds its matching no-grant approval
    And the product runs the no-grant token probe in the first project's same checkout and project identity
    Then that later run has a fresh session and its probe reports a nonce-bearing denial from the fixed Claude path
    And that later run executes without source reads staging credential mounts or residual token access
    And every run confirms its owned cleanup without modifying any original source or leaking a marker through provisioning

  # T-96: a lost start acknowledgement after mount acquisition (decision 0025,
  # LIFECYCLE-03's rule). The recorder forwards the first start and withholds
  # the engine's accepted reply until the controller's own call limit ends that
  # request, then closes the connection unanswered and refuses any repeated
  # start. The run's workload is the probe in detached mode: it reports nothing,
  # ignores SIGTERM, SIGINT and SIGHUP, and exits 23 only on the harness's
  # SIGUSR1. The recorder holds the controller's first worker-ending request,
  # for at most half the engine tier's call budget, until the harness has seen
  # the staged path absent, an exec in that worker has read the source hash
  # from the fixed path, and the workload has exited 23 by itself. That exit is
  # observed, never one the controller caused. The removal column refuses every
  # worker DELETE with a synthetic 503 until run exits; a fresh stop then
  # completes the cleanup run could not.
  @wip @slice-31 @ID-CREDRUN-11 @engine @linux
  Scenario Outline: A lost start acknowledgement after mount acquisition is reconciled from the original worker
    Given a CI Claude checkout with a private synthetic source and matching "offline" approval
    And the cooperative credential probe and every marker detector have independent positive controls
    And the run probe waits detached with its own positive control and a separate exec read plan is prepared
    When the product inspects with JSON doctor then runs the detached probe while the recorder loses the accepted start reply and answers worker removal "<removal>" then runs fresh status and any stop it requires
    Then the native fixture verification passed and JSON doctor reported every readiness check and the Claude source ready
    And the engine accepted exactly one create and one start of one worker and that start reply never reached the controller
    And the staged Claude path was absent before the first worker-ending request was forwarded and an exec in the running worker then read the exact source hash from the fixed Claude path as the non-root host UID
    And the workload exited 23 by itself and run returned 23 without a repeated start a replacement worker or a synthesized exit
    And run reports "<run cleanup>" cleanup for its registered session
    And fresh status reports that session with controller absent workload exit 23 credential staging "absent" and cleanup "<status cleanup>" in plain and JSON output
    And the final status after any required fresh stop reports complete cleanup and no worker staging or other owned resource remains
    And no provisioning surface of the recovery contains the raw marker or any base64-offset encoding
    And the source approval and unrelated resources are unchanged and the checkout differs only by the scenario's journaled policy edits

    Examples:
      | removal                  | run cleanup | status cleanup |
      | normally                 | complete    | complete       |
      | with 503 until run exits | incomplete  | pending        |

  # T-96: controller loss while the staged copy is still present (decision
  # 0025: stop removes staging by the intent's directory name after owner,
  # mode and expected-name checks). The recorder holds the engine's accepted
  # reply at the named barrier, and the harness SIGKILLs only the run
  # controller within that call and reaps it. The policy is then unlinked by
  # the journaled bracket before any fresh command, so status and stop work
  # from original protected authority alone. The detached workload never
  # exits by itself and stop runs with no controller present, so no workload
  # exit exists to report.
  @wip @slice-31 @ID-CREDRUN-12 @engine @linux
  Scenario Outline: A fresh controller recovers the staged credential after the run controller is killed at a held engine reply
    Given a CI Claude checkout with a private synthetic source and matching "offline" approval
    And the cooperative credential probe and every marker detector have independent positive controls
    And the run probe waits detached with its own positive control and a separate exec read plan is prepared
    When the product inspects with JSON doctor then runs the detached probe until the recorder holds the engine's reply "<barrier>" and the harness kills only the controller with SIGKILL
    And after reaping the controller the fixture unlinks the policy then the product runs fresh status then fresh stop then fresh status again
    Then the native fixture verification passed and JSON doctor reported every readiness check and the Claude source ready
    And the controller was killed while the engine had accepted the held "<barrier>" call and the worker "<worker state>"
    And fresh status before stop reports the original session with controller absent cleanup pending no workload exit credential staging "present" and the exact stop retry in plain and JSON output
    And fresh stop removes the original worker and its host staging directory and confirms complete cleanup
    And fresh status after stop reports complete cleanup credential staging "absent" and no workload exit or stop retry
    And no start restart or create followed the kill and status and stop read neither the source nor the active approval without a current policy
    And no provisioning surface of the recovery contains the raw marker or any base64-offset encoding
    And the source approval and unrelated resources are unchanged and the checkout differs only by the scenario's journaled policy edits

    Examples:
      | barrier                            | worker state                                                          |
      | after accepted create before reply | was created and never started                                         |
      | after accepted start before reply  | ran and an exec read the exact source hash from the still staged copy |
