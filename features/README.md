# Feature files

Every `feat:` and `fix:` commit is driven by product scenarios in this folder. Harness
regression tests also live here as ordinary Go tests; their synthetic Gherkin is created in
temporary directories, outside the product scenario inventory. The harness is harvested from itos's
(github.com/donvargax/itos, `features/`) and itos-template's, without either one's own
generator behaviour.

## What goes in a feature file

Only behaviour a user of the made project can observe: what a command does to a folder, its exit
code, and what it prints.

Never here:

- code structure, dead code, dependencies, formatting (`gofmt`, `go vet`, golangci-lint,
  govulncheck);
- refactors and performance budgets (tasks in `tasks/`);
- internal logic (unit tests, next to the code).

## Black-box boundary

The steps (`*_test.go`, Go, run by [godog](https://github.com/cucumber/godog)) treat the binary as
a black box. `TestFeatures` builds it once a run from this tree (`itos-container.exe` on
Windows), stamped with a known version and commit, as a release's build is stamped with the
release's. Each scenario gets a scratch folder in a temporary directory, runs the binary in it
and reads its exit code, its output and the files it leaves. A step judges the program by its
command line alone: it never reads its source or calls its code to decide a result.

Commands run in a clean environment: no `GIT_*`, `ITOS_*`, `ITOS_CONTAINER_*`, `GITHUB_*`,
`GH_*` or `CI` variable of the caller's, no global or system git config, a fixed author, and none
of the caller's claude, itos or its extensions on the PATH. A `GOCOVERDIR` that exists builds the
binary with `-cover` and collects what each run did, for itos-cc to merge with `go test`'s.

Slice-2 also specifies a shared metadata-validator contract. Its four metadata
scenario IDs use a test-only `cmd/itos-container` assembly, built as a Go test binary,
with in-memory file data and the real parser, reference adapter, domain and doctor
presenter. The runner sends raw input and asserts actual output; it imports no
production packages and supplies no engine fake or product injection flag. File
scenarios still run the product binary. Slice-3 owns binary-to-engine evidence.
When `GOCOVERDIR` is set, the test assembly also builds with atomic coverage across
the module. Go's exit hooks export counters for both success and refusal exits.
`TestManifestContractCoverage` checks each metadata ID's exported product coverage
in a separate directory. It runs in the unfiltered suite; `-scenarios` and active manual red
selections skip these independent reruns so only selected product IDs execute. Empty or ignored red input
retains ordinary coverage; memory-only harness regressions still run.

Slice-4's identity, publication and frozen-input contract IDs use a test-only
`cmd/itos-container` assembly too. Their nested `go test` enables atomic coverage
across the module when `GOCOVERDIR` is set and passes that directory explicitly
through `-test.gocoverdir`, since `go test` supplies its own temporary directory.
`TestApprovalContractCoverage` requires executed approval-domain counters for each
of the five IDs in separate directories. Like the manifest regression, it skips
filtered and active manual red selections so they execute only the requested IDs.

The older-kernel approval fixture runs the product with umask `0777` as well as
`0022`. Go applies that umask to its coverage files too. Each child therefore
writes coverage to a private temporary subdirectory of the caller's `GOCOVERDIR`.
After it exits, the harness gives only those metadata/counter files owner access
and moves them unchanged into the caller's collection directory. The product's
umask and approval-state permissions remain as tested. Missing metadata or counters
fails collection. `TestApprovalKernelCoverage` runs both examples and requires
successful conversion with executed private-directory initialization coverage.
It preserves the same filtered, platform and engine selection boundaries.

## Running them

```sh
go test ./features -count=1                         # live non-engine scenarios and harness tests
go test ./features -count=1 -scenarios='^@ID-CLI-'  # matching live non-engine scenarios
GOCOVERDIR=$d go test ./features -count=1           # non-engine coverage in existing directory $d
itos tests smoke run scenario                      # eligible smoke IDs; engine IDs explicitly skip
```

`-scenarios` takes a regular expression over each effective tag, `@` included. Godog's
pinned Gherkin parser supplies inheritance from Feature, Rule, Scenario Outline and Examples;
each outline row is selected independently. The expression is intersected with the selected
tier, platform and the live set. An empty expression selects the whole eligible tier. Every OR
branch retains these restrictions, so explicit IDs, `@wip` and `@linux` cannot bypass them.
The separate [manual red selector](#manual-red-first-evidence) admits named `@wip` cases
only in guarded CI dispatch; empty input preserves the ordinary selection described here.

`@linux` marks scenarios whose fixtures require a Linux test process. The run terminal
examples use it for their real PTY fixture; portable invocation and redirected-stream
refusals still run on every platform. Other platforms exclude these cases before setup;
PTY allocation or verification failures on Linux remain failures, never skips.

Invalid regexes, unreadable/malformed features and expressions matching no scenario's effective
tags fail. A selection containing only excluded cases explicitly skips `TestFeatures`, with
the reason, excluded IDs and count. A mixed selection runs eligible cases and logs exclusions.
Use `-v` to see successful-run logs and Go's skip report. Selection precedes the binary build,
scenario initialization, Background steps and any engine access. Each eligible scenario is a
subtest of `TestFeatures`, named after the scenario.

### The CI engine tier

Reserve `@engine` for any scenario whose setup, hooks, steps **or cleanup** needs a real
container engine. Ordinary whole-suite, smoke, ID and coverage runs exclude it even if Docker
environment variables or a socket are present. `ITOS_CONTAINER_ENGINE_TESTS` is a harness-only
switch, stripped from child commands by the existing `ITOS_` environment filtering:

- Unset, empty or `0` selects live non-engine scenarios.
- Exactly `1` selects only live engine scenarios, and requires `CI=true`,
  `GITHUB_ACTIONS=true` and an actual Linux amd64 test process.
- Any other nonempty value, or an enabled request outside that context, fails before setup.

These checks prevent accidental execution; they do not isolate hostile code. Agents never
receive a Docker socket and never run the engine tier locally or spoof the CI context.
`TestEngineSelection` simulates context values with memory-only steps and hook counters;
`TestEngineWorkflow` and `TestEngineFixtureWorkflow` parse the workflow and reject broken
fixtures without executing Docker. The latter also checks the pinned recipe and helper guards.

The sole engine job in `.github/workflows/ci.yml` has the stable check name
**`Engine scenarios (Linux amd64)`** for the coordinator's repository settings. It runs on main
pushes, non-draft pull requests and manual dispatch, checking out the exact `${{ env.TO }}`
head on native `ubuntu-latest`. `ci` depends on it, runs with `always()` plus the event guard,
and explicitly fails if the engine result is anything other than success. Remote branch
protection settings remain the coordinator's responsibility.

A read-only server preflight targets `unix:///var/run/docker.sock`, clearing caller-selected
Docker host/context and TLS/API overrides. Missing CLI/socket, denied access, daemon failure,
wrong server platform or timeout fails the job. It records only server version, API version,
OS, architecture and the tested commit. Its 29-second deadline plus a 1-second kill grace
bounds it to 30 seconds. The preflight attempts no install/start, image pull/build, container
creation or fallback. A separate preparation step supplies the cached fixture described below.
The Engine scenarios step first compiles the test binary, then alone enables the switch
for this **CI-only** invocation (the workflow also supplies the fixture environment):

```sh
go test -c -o "$RUNNER_TEMP/slice-11-features.test" ./features
cd features
ITOS_CONTAINER_ENGINE_TESTS=1 setsid --wait "$RUNNER_TEMP/slice-11-features.test" -test.count=1 -test.v -test.timeout=7m -test.paniconexit0 -test.run '^TestFeatures$'
```

The engine tests run in their own session. Coordinator-reviewed run 42233015284
at `9af619c` traced the entire test process tree and found no product or harness
group/session signal, while the step's shell still died. The coordinator attributed
the sender to the external runner or Docker and accepted `setsid --wait` (q-16).
The workflow no longer runs strace, which consumed its five-minute budget.
The workflow streams both output channels to the log and
`$RUNNER_TEMP/slice-11-features.log`, prints the binary's exit status and decodes exits of
128 plus a signal number. It preserves the test's failure even when output capture succeeds;
a capture failure also fails a successful test run. A failure-only step prints the last
50 kernel messages, `free -m` and the last 100
test-output lines before unconditional cleanup.
The explicit `-test.paniconexit0` retains `go test`'s protection against an early `os.Exit(0)`.
TestMain logs catchable signals to stderr in the engine tier, including its PID, and exits
nonzero for terminating signals. SIGKILL and SIGSTOP cannot be caught; the supervising shell
and kernel output supply that evidence. These diagnostics address the coordinator's
2026-11-06 silent engine failures; the trace cleared product and harness signalling,
while each engine run must still establish its own runtime evidence.
`TestEngineDiagnostics` exercises the workflow shell with disposable compiler/program fixtures,
and `TestFeatureSignalDiagnostics` checks a child-only SIGTERM without engine access.

T-32 temporarily gives the step an eight-minute budget and the job a thirteen-minute budget.
The test binary's seven-minute timeout leaves time for Go's panic and goroutine dump before
the step limit. This coordinator-authorized stopgap is recorded in `weakened-gates.md` until
`p1-engine-suite-within-its-limit` speeds up the suite and restores the limits. These are
test-infrastructure budgets, not product startup/resource limits. With no red input the step
selects every live engine case, independent of smoke selections, changed IDs or a green
baseline. A manual red list narrows it to the named eligible IDs, live or `@wip`.
Pending, undefined, failed or skipped
runtime steps fail; losing selected engine cases also fails. Manual non-engine red runs
apply the same completion checks.

Doctor's ID-DOCTOR-23 and ID-DOCTOR-24 are live engine scenarios for cached-image
inspection and a missing digest. They run after preflight and fixture preparation,
followed by unconditional cleanup. If a selected tree has no live engine scenarios,
the harness explicitly skips with `no live @engine scenarios (count 0)`, including
an all-`@wip` engine set. Inspection evidence does not prove sandbox isolation.

Mutation, own-test and scenario-coverage runs in `ci` remain engine-disabled, including
individual-test fallback and whole-suite collection. An excluded engine ID provides no
runtime or coverage evidence. Later behaviours require domain/port fake tests alongside
separate runtime evidence; uncovered code cannot be satisfied by enabling Docker in proof.

Recovery's live ID-RECOVERY-21 through ID-RECOVERY-23 run fresh product processes
against the same guarded cached-image fixture. Their native recorder can inject a
removal-only failure or hold a removal barrier; it still validates concrete ownership
before forwarding mutations and records refused requests. A static worker probe supplies
exit 7 or a monotonic heartbeat. Independent observations precede harness cleanup, and
the printed retry is parsed into literal argv, never executed by a shell.

Those recovery processes run under a test-only Linux amd64 syscall observer compiled
with the already pinned `golang.org/x/sys`. Its child uses `CLONE_UNTRACED` so the outer
`strace` and the fsync observer do not compete for the same tracee; this changes neither
the child's group nor session. The observer logs child signal syscalls and reinjections
to `$SLICE_11_SIGNAL_TRACE` when supplied, so controller sends remain visible alongside
outer-traced harness sends. A local nested-tracer control asserts sender PID, absence of
session/group changes, child exit and actual fsync evidence. It traces its child and child threads,
correlating successful file fsync with the subsequently synced publication directory
and visible record inode. Failed fsyncs and file existence alone cannot prove durable
completion. The observer records only disposable fixture state; its journal is temporary
test data, not CI changed-code proof. Local `TestRecoveryTrace` controls exercise real
syscalls without engine access, including failure, ordering and controller death. These
controls do not provide native recovery red or green evidence; the coordinator must
still dispatch the three pending engine scenarios before their implementation.

Slice-15's live ID-LIFECYCLE-15 reuses that observer and slice-14's exact probe
clients and positive controls. One primary waits for atomically published checkout
challenges independently of stdin/attach. The harness collects a baseline, kills
only the recorded controller, waits for confirmed death, then generates a fresh
nonce for the post-loss round. A third round follows fresh status and precedes its
literal stop retry. Reports bind to the original native full ID and process start
identity; empty, cached or incomplete reports cannot establish isolation.
The combined network/DNS control retains the complete slice-14 inventory. Only
that isolated control reconnects to refresh real Docker DNS aliases for each new
nonce, retaining its full ID and addresses; its resolver file is restored inside
the control only. Preservation accounts for its generated endpoint ID/MAC while
comparing all other resource fields. No subject permission or resolver changes.
Listener receipts are checked through confirmed product removal. Stop must also
produce the existing file-fsync/publication/parent-fsync evidence. Local harness
checks compile the probe and reject invalid evidence without executing it.
Within each network or DNS phase, independent route clients run concurrently;
the complete report still waits for every attempt and preserves inventory order.
Client deadlines, per-attempt nonces, before/after controls and the ordering of
phases and controller-loss rounds are unchanged. This avoids accumulating each
route's denial wait inside the five-minute CI step. `TestIsolationRouteAttempts`
exercises only the scheduler with in-memory clients and virtual time on the host;
the real probe and clients still run only in the guarded engine tier.
The coordinator's guarded `red_scenarios=ID-LIFECYCLE-15` dispatch
[42390615242](https://github.com/donvargax/itos-container/actions/runs/42390615242)
at `e22aa6366a7bf024322d8b59f2b9f112c4cb26dd` passed all eight steps. Under q-32
(2026-11-07), this activates as regression coverage of merged slice-12, consuming
slice-11 recovery/termination and slice-14 boundary controls. The dispatch supplies
passing evidence, not an assertion-level red. The coordinating loop must still
obtain ordinary green CI without red input at the actual pushed activation head,
including the engine tier and any required changed-code proof, before closure.

Code proof obtains its listed tests through `tools/bin/scenario-list`. It uses the same
Gherkin tier/platform selector as execution, before any fixture setup, and preserves
core itos's TSV files and fingerprints. Thus ordinary Linux proof lists local Engine API
fixtures but excludes the native non-Linux ID-DOCTOR-22 and real-engine IDs. The manual
platform jobs and engine job still execute their eligible scenarios. Listing never
substitutes for per-ID coverage: every listed ID must emit real product counters.

### Cached fixture protocol

T-20's [helper](../tools/bin/engine-fixture) runs only in the existing engine job, after
preflight. Both `prepare` and `cleanup` refuse outside GitHub Actions on an actual Linux amd64
host, before engine access. Agents never invoke these modes against an engine or spoof CI.
The helper uses the existing Docker CLI, shell/coreutils, curl and jq; it installs nothing.
Every engine call clears Docker host/context/TLS/API overrides, uses
`unix:///var/run/docker.sock` explicitly and supplies a disposable empty Docker configuration,
without logins or credential helpers. It never changes the daemon or its registry trust.

Preparation pulls two fixed Linux amd64 manifests. The T-20 specification records their
Docker Hub metadata provenance, inspected on 2026-11-04:

| Input | Exact reference | Provenance |
| --- | --- | --- |
| Docker Official registry 3.0.0 (CNCF Distribution) | `docker.io/library/registry@sha256:5b12b22f21522fe69443079df04c0f3f42cb9977857f3c20404386652a6c6d8e` | [Tag metadata](https://hub.docker.com/v2/repositories/library/registry/tags/3.0.0) |
| Docker Official BusyBox 1.37.0-musl | `docker.io/library/busybox@sha256:5624bb6777cf469b1fb50ae3f981b15d2715ee2fffaf20b9998cfb7843726835` | [Tag metadata](https://hub.docker.com/v2/repositories/library/busybox/tags/1.37.0-musl) |

Tags identify the provenance only; CI never resolves them. The maintained
[Distribution registry](https://distribution.github.io/distribution/about/deploying/) provides
the push/pull round trip, and [BusyBox](https://hub.docker.com/_/busybox) supplies a small shell
and basic probes. No image or archive is published outside the disposable runner-local registry.
This acquisition is CI scaffolding: it adds no product acquisition, registry dependency or
permission for worker egress.

The reviewed [Dockerfile](testdata/engine-fixture/Dockerfile) takes the runner's numeric
effective UID/GID, refusing UID zero before build, sets `USER`, clears `ENTRYPOINT` and uses
`CMD ["/bin/true"]`. It has no `RUN`, `ADD`, `COPY`, bootstrap, extra environment, declared
volume or devcontainer metadata. The Dockerfile is supplied on stdin with an
[empty build context](https://docs.docker.com/build/concepts/context/#empty-context); no consumer
checkout recipe or helper is run. The helper uses the daemon's classic builder explicitly,
so no external builder or provenance index is introduced. An unavailable builder fails the job.
BusyBox is a fixture, not the pilot's full agent toolchain; additional contents need their
own behaviour specification.

The preparation protocol is bounded to 180 seconds, including pulls, build, push, pull and
smoke, with a separate 30-second cleanup attempt on failure. It performs these checks:

1. Create one uniquely named registry, recording its ID and attempt identity outside the
   checkout under `RUNNER_TEMP`. Publish container port 5000 on an allocated **127.0.0.1** port,
   discovered from that exact container. Registry storage is container-managed tmpfs; there
   are no host mounts, host networking, extra capabilities, privileged mode or restart policy.
   `OTEL_TRACES_EXPORTER=none` disables trace export. Readiness has a 20-second limit.
2. Build the reviewed recipe for `linux/amd64`, record the config ID and push to the attempt's
   unique loopback repository. Read the single-platform manifest through the registry API,
   hash its exact bytes, compare with `Docker-Content-Digest` and match its config digest to
   the built ID. Reject an index or a mismatch.
3. Remove only the recorded derived tag/digest references, confirm the exact requested
   `repository@sha256` reference is absent, then
   [pull that digest](https://docs.docker.com/reference/cli/docker/image/pull/). Inspect that
   exact reference and require its complete `RepoDigests` entry, matching config ID,
   Linux amd64 platform, numeric non-root user, empty entrypoint, no declared volumes and no
   `devcontainer.metadata`. Build/push results alone are not cached-image proof. The measured
   manifest digest and distinct config ID are retained together; neither is assumed stable.
4. Execute a ten-second bounded benign smoke command from the cached digest with pull disabled,
   network none, read-only root, all capabilities dropped, no-new-privileges, the recorded
   UID/GID and no host mounts. Require the actual UID/GID, fixed marker and exit zero.
5. Remove the smoke container and registry, discarding registry tmpfs, then inspect the cached
   digest again. Only then publish `image`, `image_id`, `uid` and `gid` as step outputs.

The Engine scenarios step receives these fixture inputs beside its tier switch and the
optional `ITOS_CONTAINER_RED_SCENARIOS` selector described below:

| Variable | Meaning |
| --- | --- |
| `ITOS_CONTAINER_ENGINE_FIXTURE_IMAGE` | Exact loopback repository and measured manifest digest |
| `ITOS_CONTAINER_ENGINE_FIXTURE_ID` | Distinct image config digest |
| `ITOS_CONTAINER_ENGINE_FIXTURE_UID` | Runner's non-root effective UID |
| `ITOS_CONTAINER_ENGINE_FIXTURE_GID` | Runner's effective GID |

Future engine Given steps must reject absent/malformed inputs and independently inspect cached
content; they must not acquire a substitute. Ordinary selection must not read the inputs or
initialize fixture hooks. Existing `ITOS_` filtering strips them from product children, which
receive the image reference through their disposable, normally validated manifest. Do not
create approval state or bypass policy/manifest or metadata validation. For an absent-image
case, derive a different well-formed digest in the same repository and verify its absence by
read-only inspection; never remove the shared positive fixture. The registry is already gone
when scenarios start. Product request-journal assertions still have to prove zero acquisition;
an unavailable registry is not that proof. Those steps and journals belong to the behaviour
items, including slice-3's doctor cases, not T-20.

Preparation attempts cleanup on failure. The engine job's final `always()` step retries cleanup
after failures/cancellation and removes the remaining derived image references on success.
Each attempt has a 30-second bound. Cleanup verifies recorded container IDs, names, image IDs
and attempt labels before removal; interrupted creates can be recovered by their recorded
unique names plus image and label. Image removal requires the recorded config identity.
Cleanup is idempotent, preserves an earlier failure and fails an otherwise successful job
when incomplete. It never prunes, removes shared pinned inputs, or deletes checkout files.
A killed runner cannot promise cleanup: that run is incomplete, not evidence.

CI logs record the tested commit, server version/API/platform, source pins, measured image
reference and config ID, expected/observed cold-pull and smoke results, and cleanup verdicts.
The coordinator must inspect the actual pushed head's run for these results even with zero
live engine scenarios. Static workflow checks and shell syntax are not RepoDigest proof;
fixture success is not a passed doctor/runtime scenario. Images, temporary ownership records
and generated test evidence are never committed.

#### Fixture sources and notices

These inputs retain their upstream licenses and notices; the project's AGPL-3.0 license does not
relicense them. The recipe changes only image configuration and removes no upstream files.

- Distribution 3.0.0 is [Apache-2.0](https://github.com/distribution/distribution/blob/v3.0.0/LICENSE).
  Retain its [source and notices](https://github.com/distribution/distribution/tree/v3.0.0),
  [official image build sources](https://github.com/distribution/distribution-library-image) and
  [bundled dependency inventory/notices](https://github.com/docker-library/repo-info/tree/master/repos/registry).
- BusyBox 1.37.0 is [GPL-2.0](https://busybox.net/license.html). Corresponding upstream source
  is [BusyBox 1.37.0](https://busybox.net/downloads/busybox-1.37.0.tar.bz2), including `LICENSE`;
  [official image sources](https://github.com/docker-library/busybox) retain its build provenance.
  Keep [musl's copyright and license notices](https://git.musl-libc.org/cgit/musl/tree/COPYRIGHT)
  and the [bundled component inventory](https://github.com/docker-library/repo-info/tree/master/repos/busybox).
  These source references do not authorize publishing the fixture as a product image.

### Manual red-first evidence

The optional `workflow_dispatch` string input `red_scenarios` supplies expected-red evidence
without changing feature files or smoke entries. It accepts whitespace-separated **complete**
scenario tags matching `^@ID-[A-Z]+-[0-9]+$`, for example `@ID-DOCTOR-01 @ID-DOCTOR-02`.
Standard-library Unicode whitespace separates tokens; repeated IDs execute only once per
eligible scenario/outline row. Bare IDs, arbitrary tags, lowercase areas, commas, regexes,
godog operators and shell-looking text fail. Every ID must exist in the parsed inventory,
even if other IDs match. Combining a nonempty list with `-scenarios` fails before build/setup.

The workflow passes the input only to **Engine scenarios** and the existing macOS/Windows
**Tests** steps, as step-local `ITOS_CONTAINER_RED_SCENARIOS` data. It is never a shell
expression. The harness reads it only after `CI=true`, `GITHUB_ACTIONS=true` and
`GITHUB_EVENT_NAME=workflow_dispatch` are established. Local, PR, push and other contexts
do not read it, even if present and malformed. Unset, empty or whitespace-only input keeps
today's live-only selection in both tiers. Agents never spoof this context or access an engine.

A nonempty list selects only its exact IDs, live or `@wip`, while retaining every tier and
platform restriction. Inherited Feature, Rule, Outline and Examples tags apply; an outline ID
selects all eligible rows. The input never enables the engine. The same list may reach all
three jobs: engine IDs skip on manual platforms, non-engine IDs skip in the engine job, and
`@linux` cases remain excluded on macOS/Windows. A wholly excluded selection reports its IDs,
reasons and count, runs no scenario setup and supplies no red evidence. Eligible selections must finish
all expected cases successfully for the test command to pass; actual failures stay failed.

Draft PRs skip automatic CI. The coordinator dispatches on the item branch after it contains
the complete step commit and T-21's workflow/harness changes. The existing workflow must also
be available on the default branch for GitHub manual dispatch.

1. The behaviour implementer commits complete step definitions in a `test` commit while cases
   remain `@wip`, retaining `@engine` wherever setup, steps or cleanup need it. The agent stops.
2. The loop publishes the item branch and invokes the coordinator-only command:

   ```sh
   gh workflow run ci.yml --ref <item> -f 'red_scenarios=<ids>'
   ```

   Substitute whitespace-separated complete tags for `<ids>`. No tag-removal commit, evidence
   branch, smoke edit or draft-guard exception is needed. Ordinary Linux and proof jobs retain
   their normal inputs; only the engine and manual macOS/Windows jobs receive this selector.
3. The coordinator records the **resolved tested SHA**, input IDs, run URL, runner/tier,
   scenario ID/outline row and failing **Then** assertion. Infrastructure, setup, undefined-step
   failures and empty/excluded selections are not red proof. A passing run is not red proof
   either; no forced failure or ignored test failure is permitted. Keep the failed run visible.
4. Implement, remove `@wip` in the behaviour commit and require green CI at the actual pushed
   head **with no red input**. Expected-red dispatches are never merge/closure prerequisites or
   substitutes for that green run. The loop alone publishes, dispatches, reads results and closes.

Non-engine cases that can run locally retain the ordinary local red-first procedure. This
manual route adds no product scenarios, supported-host claim or runtime evidence by itself.

Slice-12's native step checkpoint defines all 18 rows of `ID-LIFECYCLE-08`, `09`,
`11`, `12`, `13` and `16`. The complete `11` case and all four `16` rows are live
slice-11 recovery regressions, accepted by the coordinator at `9ac4f11` from runs
`42297748701` and `42298705984`. The implementation also activates `08`, `09`,
`12` and `13`; their native green run remains the coordinator's CI responsibility.
The trusted worker writes independent signal receipts and checkout edits. The recorder
keeps its full-ID mutation checks, streams acknowledged waits, delays only signal
acknowledgements and disconnects only product transports. The recovery observer records
successful file/directory fsyncs and controller signal syscalls; real pipe-capacity
observations establish output backpressure before cancellation. Initial readiness
and diagnostic bytes are preserved before releasing the output flood, freeing any
partially occupied pipe page. No further pipe read occurs until controller exit.
Inspection preserves nonblocking descriptors and collection has a two-second read
deadline. A missing barrier aborts immediately; collection caps teardown at 32 seconds
from the recorded cancellation/exit origin, including the two-second scheduling
tolerance, and os/exec capture waits are bounded after process death. Harness aborts
remain failures, never successful product cancellation or cleanup. One observed
125-second quiet control may be shared within the stream outline, as its spec permits.

Local `TestLifecycle*` checks verify step registration, probe compilation, recorder
forwarding/faults and rejection of incorrect observations. They do not execute the probe
or provide native red/green evidence. Run `42297748701` supplied accepted assertion
red for `08`, `09`, `12` and `13`'s quiet row. Its blocked stdout/stderr rows supplied
no evidence: the harness missed a barrier and hung in collection. After the harness
repair, coordinator run `42304766391` at `b4a8478` supplied accepted assertion-level
red for all four `13` rows. Do not raise suite timeouts. Separate red dispatches
keep the existing engine-job budget.
Record complete passing rows as earlier-slice regressions under q-6/q-11, including
exit 17 and LIFECYCLE-09, and assertion-level failures for missing behaviour. Keep
LIFECYCLE-15 with slice-15. Local `14` now runs the real run command through port
fakes and protected record adapters over memory storage, with controlled time.
Both rows failed at their typed stream-timeout assertion before implementation
and pass afterward. Local 01–07 and 10 now exercise the complete command entry and registered
recovery with controlled time. The combined 37-row local selection (including 14)
passes. Step checkpoints df8b8e6, 9b9ebd3 and e67210a preserve red-first
assertions and verified regressions; decision 0014 records the evidence. Native
signal evidence remains separate: coordinator run 42312330532 at 41529b1 passed
all 18 engine cases before these local budget changes. The loop owns fresh CI
and changed-code proof at the pushed head.

Review regression LIFECYCLE-17 covers interrupted checkout pinning, inspection,
revalidation and borrowed policy/manifest reads through the same command assembly.
Its ten rows failed at checkout lifetime assertions after the step checkpoint
`c274c49`, with `@wip` removed locally: pending reads lost their pins and late pins
were never released. They pass with closure serialized behind those operations;
the combined local lifecycle selection now has 47 passing rows. The controller
still returns at cancellation or its ten-second call bound, before the blocked
operation is released. Late completion cannot resume launch.

LIFECYCLE-09's independent liveness read starts immediately after cancellation,
with repeated signals still scheduled at two and eight seconds. Observer I/O does
not hold the request recorder's lock, and a delayed running reply remains evidence
for the same never-restarted primary. The journal still rejects SIGKILL before ten
seconds or after twelve. Controlled-time harness tests cover delayed replies and
early controller exit; fresh native-engine evidence remains CI-only.

LIFECYCLE-18/19 cover teardown diagnostics and primary-cause preservation through
both local backends. They freeze output at command return, before disposing of
abandoned writers, matching main's immediate exit. Checkpoints a020fe5/d606209
supplied eight assertion-level failures; the two blocked-stderr rows retained
existing slice-12 behavior. All ten pass after reserving reporting time inside
the original teardown total and keeping timeout separate from engine/wait loss.
Decision 0018 records the evidence; no native-engine or mutation run is implied.

LIFECYCLE-20 covers launch operations that ignore cancellation, including a
socket identity syscall. Step checkpoint `e932db6` supplied assertion-level red
for all twelve rows on both local backends with `@wip` removed for the run.
The controller exceeded its call, startup or cancellation plus teardown bound;
late attach results also leaked their handles. Finite connection calls now end
the controller wait independently, retain the connection until the operation
returns, and discard late attachments. All twelve rows pass without native
engine access; the existing 57 local lifecycle rows also pass. Fresh native
engine checks and changed-code proof remain the coordinating loop's work.

LIFECYCLE-21 covers an exited worker whose recovery wait ignores cancellation
before returning an exit code. Checkpoint `0347743` failed at the teardown-bound
Then on both local backends with `@wip` removed: the controller was still waiting
at the 31-second guard. The connection wrapper now bounds observation by its
caller's context while retaining the connection until the adapter finishes.
The scenario passes, including retained recovery, rejection of the late exit,
fresh owned cleanup and a healthy 121-second foreground wait. All live local
lifecycle scenarios pass; native-engine checks and changed-code proof remain
CI-only and belong to the coordinating loop.

## Tags

Slice-17 includes slice-18's accepted `anchors.feature` under coordinator q-36.
The seven native rows in `ID-ANCHORS-09` through `ID-ANCHORS-13` have registered
steps and remain `@wip` for their own guarded CI red dispatch. They use actual
product processes, the native request recorder, independently inspected volumes,
and the existing file/publication fsync observer. The trusted worker probe reports
inode, nonce, progress and denied control-record reads after controller loss.
Successful create/start responses and last-volume deletion have bounded barriers;
foreign-volume substitution occurs only after independently confirmed worker absence.
The recorder never fabricates successful native resource operations.

`TestAnchorRecovery*` checks registration, CI-only guards, probe compilation,
barriers and rejection of missing/incorrect observations without an engine.
These are harness checks, not native red evidence. The coordinator must obtain
assertion-level red for every row before recovery implementation; OFFLINE-29/30's
accepted red does not cover them. Dispatch individual IDs if needed to stay within
the existing engine-job budget. Local `ID-ANCHORS-01` through `08` still require
their step checkpoint and assertion-level red; no anchor scenario is activated here.

| Tag               | Meaning                                                                              |
| ----------------- | ------------------------------------------------------------------------------------ |
| `@phase-<n>`      | The phase a scenario belongs to.                                                     |
| `@<item>`         | The work item that turns it green (`@count-cli`).                                     |
| `@ID-<AREA>-<nn>` | Stable scenario ID. Commits reference these. Never reuse or renumber them.           |
| `@bug-<n>`        | Reproduces a fixed bug. Added by `fix:` commits.                                     |
| `@wip`            | Written, not yet implemented. Excluded by default; only named CI red selections admit it. A `feat` may not name it. |
| `@engine`         | Setup, hooks, steps or cleanup need a real engine. Runs only in the CI engine tier. |
| `@linux`          | Fixture requires Linux. Excluded before setup on other test platforms.             |

**IDs.** `<AREA>` is one word in capitals for the area of behaviour, and one feature file holds
one area: `COUNT` is `count.feature`, `CLI` is `cli.feature`, `COMPL` is `completion.feature`.
`<nn>` counts up within the area from `01`; a removed scenario's number is not given again. A new
area gets a new word and a new file.

## Commit rules

- `feat:` must add or change scenarios, or reference `@wip` ones it turns green. Footer:
  `Scenarios: ID-COUNT-01` (`itos commit --scenarios`).
- `fix:` must add a `@bug-<n>` scenario that failed before the fix, or reference an existing
  scenario that was failing. Same footer.
- The commit-msg hook checks that the referenced IDs exist and are live at the commit.
- Red first: a slice's steps go alone in a `test` commit, its scenarios still `@wip`
  (`itos commit --item count-cli`). For real-engine behaviour, retain `@wip` in the step
  commit. The coordinator obtains assertion-level red evidence through the scoped manual
  selector above, retaining the tags on disk, then green evidence after implementation with
  `@wip` removed and no red input. Ordinary no-input runs still exclude `@wip`.
  Undefined steps, setup failures and local engine runs are not acceptable evidence.

## The smoke set

The smoke set is the list in `smoke.yaml`: the scenarios every push runs, beside the ones its
commits name. The rule, which `itos tests smoke check scenario` checks:

- **Every feature file with a live scenario has at least one smoke scenario**, listed under the
  file with the reason it was chosen. A file has more only when the list says why (`more`).
- Every ID in the list is a live scenario of the file it is listed under.
- A smoke scenario is fast and central to its file.

So a `feat` that adds a feature file, or makes a `@wip` one live, picks its smoke scenario in the
same commit. This includes engine-only features: each still supplies a live smoke ID.
The ordinary smoke run may explicitly skip that ID because the required engine job runs all
live engine cases at the same head. Listing, footer validation and smoke-coverage rules remain
unchanged.

## Writing a scenario

- **Give a check its own wording.** godog matches a step's text whatever its keyword, so a `Then`
  phrased like an existing `Given` runs the setter and cannot fail.
- **A step is one line**, however long.
- **Assert what a user reads**: the exit code, a sentence of the output. Never more of the output
  than the behaviour is about.
- **Never start a description line with `@`.** Gherkin reads it as tags, and godog then refuses
  the file, failing every run of the features.
- **Say why beside the scenario** when a setup would make a reader ask: a comment above the tag
  line.

## The steps this branch's scenarios use

These are the ones the harness must define. Each is a regular expression over the step's text,
with the program named as the built binary is.

| Step                                                                                  | What it asserts                                                  |
| ------------------------------------------------------------------------------------- | ---------------------------------------------------------------- |
| `itos-container runs with "…"`                                                       | Runs the binary in the scratch folder with those words.          |
| `itos-container runs with "…" and the environment "NAME=value"`                     | The same, with that variable added to the clean environment.     |
| `it exits with code <n>`                                                              | The binary's exit status.                                         |
| `its standard output says "…"` / `does not say "…"` / `lists "…"` / `does not list "…"` | Containment, or one whole line equal to the text.                |
| `its error output says "…"` / `does not say "…"`                                      | The same, on stderr.                                              |
| `the first line of its standard output is "…" and the stamped version`                | The stamped version the harness built with.                      |
| `the second line of its standard output is "…" and the stamped commit`                | The stamped commit.                                               |
| `the last line of its standard output is "…"`                                          | The completion protocol's instruction line.                       |
| `its JSON output names the problem "…"`                                               | That the object lists a problem with that rule.                  |
| `its JSON output gives file "…", <n> lines and <n> words`                             | The success object's file, lines and words.                      |
| `its JSON output gives each problem a rule, a message and a fix`                      | That no problem is missing one of the three.                     |
| `the file "…" contains "…", "…"`                                                      | Each text is in the file.                                         |
| `the file "…" holds a line of <n> characters`                                         | A file written for the scenario, long enough to beat a token limit. |
| `standard input holds "…"`                                                            | The text the run reads from its standard input.                   |
| `an empty folder "…"`                                                                 | A folder in the scratch directory.                                |
