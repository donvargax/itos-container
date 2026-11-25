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

The ordinary non-engine suite runs four independent scenarios at once, each with its own
checkout, state, processes, recording engine and assertions. Compiled test helpers are shared
only after a serialized build and atomic publication outside the scenario checkouts. Engine runs,
red selections and focused harness controls stay serial. No scenario deadline or assertion is
shortened or omitted to fit the budget.

Commands run in a clean environment: no `GIT_*`, `ITOS_*`, `ITOS_CONTAINER_*`, `GITHUB_*`,
`GH_*` or `CI` variable of the caller's, no global or system git config, a fixed author, and none
of the caller's claude, itos or its extensions on the PATH. Either `GOCOVERDIR` or
`ITOS_CC_TEST_COVERDIR` requests a covered product build. The harness reads the latter before
filtering `ITOS_` variables and gives each scenario its own `GOCOVERDIR` for collection.

Contract scenarios (metadata, identity, publication and frozen-input IDs) run a test-only
`cmd/itos-container` assembly built as a Go test binary, with in-memory file data and the real
domain, parser and adapters. The runner sends raw input, asserts actual output, imports no
production package and supplies no engine fake or product injection flag; file scenarios still
run the product binary. `TestManifestContractCoverage`, `TestApprovalContractCoverage` and
`TestApprovalKernelCoverage` require each such ID's counters, in separate directories, to show
executed product blocks; they skip under `-scenarios` and manual red selections. The older-kernel
approval fixture runs the product under umask `0777` as well as `0022`, and the harness moves each
child's private coverage files unchanged into the scenario's collection directory.

The socket-backend fixture lets a world choose what the kernel answers to the Landlock version
query, at the system call and never through a product seam. A world that pins an answer
(`pinKernelAnswer`: "ABI <n>" for n from 0 to 2147483647 with no sign or leading zero, "ENOSYS",
"EOPNOTSUPP" or "EPERM"; any other text is refused) starts each later invocation through a helper
mode of this test binary. The helper installs a seccomp user-notification filter on
`landlock_create_ruleset` and execs the program in the same process, its argv, environment and
descriptors unchanged; the harness answers the version query from the world's current answer and
any other call with `EPERM` (`landlockRecord`). A route applies `superviseLandlock` to its
`exec.Cmd` before `Start` and ends it after `Wait` (`runEnv` does), and a route that outlives the
started process first extends supervision until the listener reports `POLLHUP`. A world that pins
nothing starts its program directly. `doctorAPI` reports `DefaultRuntime` `runc` and, by default,
the inventory "runc only"; `chooseRuntimeInventory` names another. `TestSocketBackendFixture`
holds both. On Linux amd64 it never skips: a host under a live seccomp listener refuses the filter
with `EBUSY`, so it fails there. Elsewhere it skips, and `pinKernelAnswer` refuses every pin, so a
world never runs unfiltered while counting as pinned
(`TestSocketBackendPinRefusedWhereUnsupported`).

## Running them

```sh
go test ./features -count=1                         # live non-engine scenarios and harness tests
go test ./features -count=1 -scenarios='^@ID-CLI-'  # matching live non-engine scenarios
GOCOVERDIR=$d go test ./features -count=1           # non-engine coverage in existing directory $d
ITOS_CC_TEST_COVERDIR=$d go test ./features -count=1 # non-engine coverage in $d/<ID>
itos tests smoke run scenario                      # eligible smoke IDs; engine IDs explicitly skip
```

Run the full feature suite without `-race`: a full-package race run once consumed 22 GB and
starved the machine. Use `-race` only on changed focused tests with an explicit `-run`, such as
`go test -race ./features -count=1 -run '^TestFeatureMemoryRetention$'`; for changed production
packages select only those packages. A test killed at the memory limit needs its memory use
fixed, not a higher limit. Do not repeat the historical full-feature race command in ADR 0020.
`itos task T-35` runs the full suite and the focused regression in both modes, each within
4,194,304 KiB peak RSS. Godog keeps matched step handlers and their worlds in its results, so the
initializer clears each scenario's world after cleanup; a harness change must not retain scenario
snapshots (`TestFeatureMemoryRetention`).

`-scenarios` takes a regular expression over each effective tag, `@` included. Godog's pinned
Gherkin parser supplies inheritance from Feature, Rule, Scenario Outline and Examples; each outline
row is selected independently. The expression is intersected with the selected tier, platform and
the live set, and an empty one selects the whole eligible tier. Every OR branch keeps these
restrictions, so explicit IDs, `@wip` and `@linux` cannot bypass them. The
[manual red selector](#manual-red-first-evidence) admits named `@wip` cases only in guarded CI.
`@linux` marks scenarios whose fixtures require a Linux test process; other platforms exclude
them before setup, and a failure on Linux is a failure, never a skip.

Invalid regexes, unreadable or malformed features and expressions matching no scenario's effective
tags fail. A selection containing only excluded cases explicitly skips `TestFeatures`, with the
reason, excluded IDs and count; a mixed selection runs eligible cases and logs exclusions. Use
`-v` to see successful-run logs and Go's skip report. Selection precedes the binary build,
scenario initialization, Background steps and any engine access. Each eligible scenario is a
subtest of `TestFeatures`, named after the scenario.

### Product coverage

Both the CLI and the shared command test assembly use `-cover -covermode=atomic -coverpkg=./...`
when coverage is requested. Plain and covered assemblies have separate cache paths; with neither
input, builds are plain, selected runs included, without missing-coverage warnings.

| Harness inputs | Product coverage destination |
| --- | --- |
| Neither variable | No product coverage |
| `GOCOVERDIR` only | Aggregate counters in that directory |
| `ITOS_CC_TEST_COVERDIR` only | `<root>/<ID>` for each selected scenario |
| Both variables | `<ITOS_CC_TEST_COVERDIR>/<ID>`; the per-ID root wins |

The harness resolves relative roots against its working directory (`features/`) and creates an
absolute destination before fixture setup starts product code. For per-ID collection each scenario
must have exactly one complete `@ID-AREA-number` tag; missing, ambiguous or malformed IDs and
unusable directories fail the run. Outline rows accumulate under their shared ID. The destination
stays on the scenario world and its secondary fixture worlds, so concurrent IDs and nested native
scenarios keep independent counters; CLI launches, contract probes, recovery controllers and the
umask helpers use it too, and an export failure fails cleanup. Scenario cleanup removes fixture
state but leaves coverage for the collector. Harness regressions use private temporary coverage
roots and restore the caller's inputs. `TestScenarioCoverageContract` checks this routing through
real hooks and binaries, requiring readable atomic metadata and counters with executed product
blocks; COUNT-01 alone must reach `internal/slices/count/domain/`. Native coverage and fresh
pinned itos-cc proof are CI evidence.

### The CI engine tier

Reserve `@engine` for any scenario whose setup, hooks, steps **or cleanup** needs a real
container engine. Ordinary whole-suite, smoke, ID and coverage runs exclude it even if Docker
environment variables or a socket are present. `ITOS_CONTAINER_ENGINE_TESTS` is a harness-only
switch, stripped from child commands by the `ITOS_` environment filtering:

- Unset, empty or `0` selects live non-engine scenarios.
- Exactly `1` selects only live engine scenarios, and requires `CI=true`,
  `GITHUB_ACTIONS=true` and an actual Linux amd64 test process.
- Any other nonempty value, or an enabled request outside that context, fails before setup.

These checks prevent accidental execution; they do not isolate hostile code. Agents never
receive a Docker socket and never run the engine tier locally or spoof the CI context.
`TestEngineSelection`, `TestEngineWorkflow` and `TestEngineFixtureWorkflow` hold the switch and
parse the workflow without executing Docker. No local engine or mutation run substitutes for
native CI evidence; the loop owns native runs and closure.

**Shards.** The tier runs as the matrix of the `engine-shard` job in `.github/workflows/ci.yml`.
Each shard is whole feature files on its own native `ubuntu-latest` runner and daemon, with its own
checkout of the exact `${{ env.TO }}` head, preflight, fixture preparation and cleanup: **offline**
(`offline.feature`, `harness-home.feature`, `credentials-run-native.feature`, which slice-31 brings and the shard skips and logs as pending until the file exists), **lifecycle** (`lifecycle.feature`, whose four
`ID-LIFECYCLE-09` rows stay nested in `ID-LIFECYCLE-13`'s quiet row), **other** (`anchors.feature`,
`checkout-overlap.feature`, `claude-home.feature`, `doctor.feature`, `exec.feature`, `hardlinks.feature`,
`recovery.feature`, `run_terminal.feature`, `session.feature`, `sockets.feature`; `claude-home.feature` is slice-36's and is skipped and
logged as pending until the file exists) and **egress** (`egress.feature`,
`credentials-run.feature`, which slice-31 brings and the shard skips and logs as pending until the
file exists, and `egress_lifecycle.feature`, which joins it when it exists). `pendingEngineFeatures`
in `features_test.go` is the one reviewed list of such names, each with its item and its one permitted shard; any other absent or `@engine`-less name
is refused. Nothing crosses runners, `fail-fast` is off
so a red shard hides no other, no feature is ever split, and shards run on main pushes, non-draft
pull requests and manual dispatch. Move a whole feature to another shard only on measured shard
times that show the need; the loop records each shard's step time, runner image and the
preflight's Docker version, so an image change is not taken for a regression.

The stable check name **`Engine scenarios (Linux amd64)`** belongs to the `engine` aggregate job.
It needs `engine-shard` alone, runs under `always()` plus the event guard for one minute, and its
one step, **Require every engine shard**, fails unless the shard job's result is `success`. `ci`
depends on the aggregate, runs with `always()` plus the event guard, and fails unless the engine
result is success; `release` needs it too. Remote branch protection stays the coordinator's.

`ITOS_CONTAINER_ENGINE_FEATURES` names a shard's files: a second harness-only input, read through
the same lookup as the red input (`readFeatureEnvironment`) and set only as the Engine scenarios
step's `${{ matrix.features }}`. It holds whitespace-separated feature file basenames; unset or
blank is the complete tier. Before any build or fixture, `selectScenarios` refuses a nonempty
value outside the engine tier, a name that is not a basename ending in `.feature`, a duplicate, and
a name no feature file with an effective `@engine` case, live or `@wip`, bears. It keeps only the
named files' rows, intersected with the tier, platform, `-scenarios` and red selection; red IDs and
a `-scenarios` expression are still validated against the full inventory. A red list matching only
other shards' rows leaves the explicit red skip naming those IDs, and a `-scenarios` expression
matching only other shards' rows leaves the plain count-0 skip. The selection, godog's
filtered-pickle check, the run and the nested schedule read the same files
(`scenarioSelection.paths`), since godog numbers pickles across one read. The
`ITOS_CONTAINER_ENGINE_` prefix keeps the variable from product children.
`TestEngineShards` holds this without an engine: the workflow names exactly the four shards with
their complete file sets, every feature with an engine case lands in exactly one shard, and
`ID-LIFECYCLE-09` and `-13` share lifecycle. It also refuses broken workflows (a dropped, doubled,
unknown or path-named feature, an empty or extra shard, a weakened aggregate or gate,
`continue-on-error`, `fail-fast: true`, the feature input at job level, a second engine switch).

**Preflight and step.** A read-only server preflight targets `unix:///var/run/docker.sock`,
clearing caller-selected Docker host/context and TLS/API overrides. Missing CLI or socket, denied
access, daemon failure, wrong server platform or timeout fails the job. It records only server
version, API version, OS, architecture and the tested commit, within a 29-second deadline plus a
1-second kill grace, and attempts no install, start, pull, build, container creation or fallback.
The Engine scenarios step compiles the test binary, creates a fresh coverage root beneath
`RUNNER_TEMP`, then alone enables the switch for this **CI-only** invocation (the workflow also
supplies the fixture environment):

```sh
go test -c -o "$RUNNER_TEMP/slice-11-features.test" ./features
ITOS_CC_TEST_COVERDIR=$(mktemp -d "$RUNNER_TEMP/T-36-engine-coverage.XXXXXX")
export ITOS_CC_TEST_COVERDIR
cd features
ITOS_CONTAINER_ENGINE_TESTS=1 setsid --wait "$RUNNER_TEMP/slice-11-features.test" -test.count=1 -test.v -test.timeout=4m -test.paniconexit0 -test.run '^TestFeatures$'
```

After the selected scenarios succeed, the same invocation checks that the coverage root holds
exactly their IDs and converts each with `go tool covdata`, requiring atomic coverage and executed
product blocks. The tests run in their own session (q-16); `-test.paniconexit0` keeps `go test`'s
protection against an early `os.Exit(0)`. `TestMain` logs catchable signals to stderr in the
engine tier and exits nonzero for terminating ones. The workflow streams both channels to the log
and `$RUNNER_TEMP/slice-11-features.log`, prints the binary's exit status, decodes exits of 128 plus
a signal number, and keeps the test's failure even when capture succeeds; a capture failure also
fails a successful run.

**Failure report.** Each shard's failure-only step runs `tools/bin/engine-failure-report` on that
log under `timeout 40`, then prints the last 50 kernel messages, `free -m`, the
`engine scenario timing:` records slowest first (all of them, so a timeout shows which rows took
the time) and the last 100 test-output lines before unconditional cleanup. The report exists
because job logs return 403 without a token and agents have no `gh`, while a failed job's
annotations are public: for each failure it prints one `::error title=<name>` annotation and a
`--- FAIL: <name>` block with godog's step error as text. That relies on an engine run executing
one scenario at a time (`runScenarios` clamps an engine or red selection to one,
`TestFeatureConcurrency`). It keeps ten annotations per step, cuts each text at 4096 bytes, and
reads everything from the checkout as data, never instructions. `TestEngineDiagnostics`,
`TestEngineFailureTimings` and `TestFeatureSignalDiagnostics` run the workflow shell and the failure
step against logs from real failing memory-only scenarios, so a Go or godog upgrade that changes a
log's shape fails there.

**Time limits.** The Go test timeout is four minutes, leaving time for Go's panic and goroutine
dump before the step limit. The workflow measures the entire Engine scenarios step, compilation
through output capture. Above 180 seconds, T-45's per-shard target made soft by T-69, it prints a
`::warning::` naming the shard and its time; above 600 seconds it also prints an `::error::` and
fails a run that otherwise passed. There is no watchdog: `-test.timeout` stops a real hang at
about 606 seconds, a failed run that keeps its own status. The Go test, step and job limits are 10,
11 and 15 minutes. Missing, invalid or backwards timing fails, and compilation, test, signal and
capture failures keep their original exit status. These are test-infrastructure budgets, not
product limits. With no red input the step selects every live engine case, independent of smoke
selections, changed IDs or a green baseline; a manual red list narrows it to the named eligible
IDs, live or `@wip`. Pending, undefined, failed or skipped runtime steps fail, as does losing
selected engine cases, and manual non-engine red runs apply the same completion checks.
`TestEngineSuiteBudget` runs the workflow's timing and failure wrapper against disposable
fixtures. Never raise a suite timeout, limit or assertion to pass.

**Scenario timing.** Each completed engine example streams an `engine scenario timing:` JSON
record after its final cleanup: feature URI, stable scenario ID, source row line, monotonic
duration in nanoseconds and outcome, no path or secret. The record of an `@ID-LIFECYCLE-09` row,
and of no other, also carries `signals`: one object per signal request in the journal
`lifecycleSignals` judges, with arrival, reply, status and cancellation, in nanoseconds after the
harness signalled the controller. One `engine suite timing:` summary gives counts, outcome and
elapsed time; nested row durations overlap and are not added. `TestEngineScenarioTiming` holds
this with memory-only steps and controlled time.

**Fixture budgets.** The engine harness builds its disposable product from a copy of `cmd`,
`internal`, `go.mod` and `go.sum` in which only `internal/engine/budget/domain/limits.go` differs:
call, startup and grace budgets of 1/3/1 seconds (q-40). Normal builds, releases and ordinary
contract tests keep production's 10/120/10 seconds (`TestProductionBudgetLimits`), and no product
environment variable, argument or policy can enable the fixture. The copy and binary live outside
the writable scenario checkouts. It is a copy, not `go build -overlay`, because with `-cover`
cmd/go ignores an overlay and the build cache could serve production limits for the same text. The
30-second teardown budget and process guards are unchanged. In the engine tier
`productProbeBinary` compiles the world's test assembly from the copy, so native rows keep 1/3/1.
The session contract assembly (`sessionProbeBinary`, used through `sessionRun` by `ID-SESSION-11`
and `-12`) is compiled from the checkout and keeps production's 10/120/10 even in the engine tier.
`TestEngineSuiteBudget` requires 1/3/1 from the first and 10/120/10 from the second. Both are
built with `-trimpath`, like the product, to share one build cache
(`TestProbeAssemblySharesTheProductBuildCache`).

**Lifecycle limits.** The quiet observation lasts at least four real seconds after readiness.
Repeated signals occur at 20 and 80 percent of grace; delayed acknowledgements take 60 percent.
SIGKILL cannot come early and may leave up to two seconds late (`engineEscalationTolerance`), but
never from a grace timer that the last repeat restarted (an immediate row, refused from 1.8 s
after cancellation) or that the first signal's acknowledgement restarted (a delayed row, refused
from the delay plus the grace after that signal's arrival). Observer I/O never holds the
recorder's lock. `TestLifecycleSignalWindow` pins these limits on journals it writes. **Known
gap:** a grace timer restarted only by the first repeat (a SIGINT at a fifth of grace) fires 200 ms
after the grace and is accepted in every row, as is a late start no event restarted;
`p2-grace-origin-fake-clock-row` holds both, not this suite. Engine runs selecting both
`ID-LIFECYCLE-09` and `-13` run the four escalation rows serially inside the quiet worker's
lifetime, each with its own checkout, controller and worker, and the quiet worker is cancelled only
after every nested assertion and cleanup; filtered selections missing either ID and manual red
runs stay serial.

**Native recorder.** Native readiness is one observation with a 35-second allowance
(`engineReadinessAllowance`) from the controller's start: the ready file, an independent
running/full-identity inspection and the complete `slice-12:ready` frame must arrive before it, and
later progress never restarts it. The recorder accounts for every worker it let the product create:
a controller that left, or a recorder in cleanup, gets HTTP 502 and no engine create, and an
admitted create outlives its controller until the engine answers or `engineCreateOutcomeAllowance`
(10 seconds) ends. Every step ends with `awaitCreateOutcomes`, so no Then judges an open exchange,
and cleanup removes only workers with the expected identity before releasing volumes.
`TestOfflineRecorderRecordsAbandonedCreate`, `TestOfflineRecorderCleanupRemovesAuthorizedWorkers`
and `TestAnchorRecoveryBarrierDiagnostics` hold these rules with in-memory engines; they are
harness checks, never native evidence.

**Settled operating rules.** Under q-63 a slow first create (the engine answers after the fixture's
1-second call; read the create's `forward-to-answer` interval) belongs to
`p2-first-engine-row-slow-create`: re-run the failed jobs, return a repeat of the signature to the
coordinator for another re-run, and change no code, limit, assertion or retry for it. When the
engine's time is inside the limit, no limit moves and the intervals go to the coordinator. A
`session registration` call timeout is a different signature: it goes to the coordinator, and
nobody raises the call or barrier limit, bypasses durable publication or retries the launch to
make it pass.

**Native rows.** Recovery processes run under a test-only Linux amd64 syscall observer
(`TestRecoveryTrace`); durability counts only a successful file fsync followed by the publication
directory's, never a failed fsync or file existence. Native isolation rows (`ID-LIFECYCLE-15`,
`ID-EGRESS-12` to `-19`) use trusted controls only: independent probe clients with per-attempt
nonces, positive controls before and after, and a worker attempt that passes only if it was made
and got no answer. Local harness checks (`TestEgressNativeStepsDefined`,
`TestIsolationRouteAttempts`, `TestAnchorRecovery*`) compile probes and reject invalid evidence
without executing them. Inspection evidence does not prove sandbox isolation.

**Proof and selection.** Mutation, own-test and scenario-coverage runs in `ci` stay
engine-disabled, individual-test fallback and whole-suite collection included. An excluded engine
ID provides no runtime or coverage evidence; later behaviours need domain/port fake tests
alongside separate runtime evidence, and enabling Docker in proof never satisfies uncovered code.
Code proof lists its tests through `tools/bin/scenario-list`, which uses the same tier/platform
selector as execution and preserves core itos's TSV files and fingerprints; every listed ID must
emit real product counters. A selected tree with no live engine scenarios, an all-`@wip` engine
set included, skips with `no live @engine scenarios (count 0)`.

### Cached fixture protocol

T-20's [helper](../tools/bin/engine-fixture) runs only in the engine shard job, after preflight.
Both `prepare` and `cleanup` refuse outside GitHub Actions on an actual Linux amd64 host, before
engine access. Agents never invoke these modes against an engine or spoof CI. The helper uses the
existing Docker CLI, shell/coreutils, curl, jq and tar and installs nothing. Every engine call
clears Docker host/context/TLS/API overrides, uses `unix:///var/run/docker.sock` explicitly and
supplies a disposable empty Docker configuration, without logins or credential helpers. It never
changes the daemon or its registry trust.

Preparation pulls two fixed Linux amd64 manifests, by these exact references:

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
volume or devcontainer metadata, and is supplied on stdin with an
[empty build context](https://docs.docker.com/build/concepts/context/#empty-context); no consumer
checkout recipe or helper is run. The helper uses the daemon's classic builder explicitly, and an
unavailable builder fails the job. BusyBox is a fixture, not the pilot's agent toolchain;
additional contents need their own behaviour specification.

Preparation is bounded to 180 seconds, with a separate 30-second cleanup attempt on failure. It:

1. Creates one uniquely named registry, recorded outside the checkout under `RUNNER_TEMP`,
   published on an allocated **127.0.0.1** port, with container-managed tmpfs storage and no host
   mounts, host networking, extra capabilities, privileged mode or restart policy.
   `OTEL_TRACES_EXPORTER=none` disables trace export; readiness has a 20-second limit.
2. Builds the reviewed recipe for `linux/amd64`, records the raw `--iidfile` ID and pushes to the
   attempt's unique loopback repository. It independently reads the config from a
   [daemon image export](https://docs.docker.com/reference/cli/docker/image/save/) by that ID and
   the single-platform manifest through the registry API, hashing exact bytes and requiring the
   manifest's config digest to match. It reports `store=graph-driver` when the built ID equals the
   config digest, or `store=containerd` when it equals the manifest digest, and rejects any other
   relationship or an index.
3. Removes only the recorded derived references, confirms the exact requested `repository@sha256`
   reference is absent, then [pulls that digest](https://docs.docker.com/reference/cli/docker/image/pull/)
   and requires its complete `RepoDigests` entry, matching image ID, Linux amd64 platform, numeric
   non-root user, empty entrypoint, no declared volumes and no `devcontainer.metadata`. Build/push
   results alone are not cached-image proof, and no digest or image ID is assumed stable across
   builds or image stores.
4. Runs a ten-second benign smoke command from the cached digest with pull disabled, network none,
   read-only root, all capabilities dropped, no-new-privileges, the recorded UID/GID and no host
   mounts, requiring the actual UID/GID, fixed marker and exit zero.
5. Removes the smoke container and registry, then inspects the cached digest again.
6. Warms the engine once per shard with one plain create/remove from that digest under the smoke's
   restrictions, never started, recording its name in `warmup.intent` and ID in `warmup.cid`,
   checking its identity before removal and exact absence after.
7. Follows with one product-shaped create/remove from `build.iid`, built by the shared
   definitions-only warm-up library as the offline `CreateRun` wire body with the product's
   restrictions. Its sole read-write mount is a named local volume with `NoCopy=true`, matching
   the product's checkout anchor, allocated with `type=none`, `o=bind` and a source through a held
   `/proc/<pid>/fd/<fd>` descriptor at a fresh empty directory under `RUNNER_TEMP`. The create
   uses curl with config and proxies disabled and only the local Unix Engine API socket, and never
   starts the container. Preexisting names are refused before `product.intent` and
   `product-volume.intent` are recorded; exact identity is required before removal, and the
   container goes before the volume. Transport and permission failures never count as absence, and
   foreign identity never authorizes removal. Only after both warm-ups pass are `image`,
   `image_id`, `uid` and `gid` published together as the sole step-output append.

Each complete warm-up, removal and absence verification included, is bound to ten seconds:
coreutils timeout sends TERM at nine seconds and KILL one second later, measured by the shared
[warm-up timing library](../tools/lib/engine-fixture-warmup.sh) on monotonic Linux uptime. Success
prints exactly two lines in order:

```text
engine warm-up timing: duration_ms=<integer> outcome=passed
engine product warm-up timing: duration_ms=<integer> outcome=passed
```

Each attempted phase prints one truthful `passed` or `failed`; preparation stops at the first
failure and invents no timing for an unrun phase. An unavailable or backwards clock gives
`duration_ms=unknown` and fails. Failures fail with an `engine-fixture: warm-up` diagnostic naming
plain or product and publish no fixture outputs. The failure trap and the final `always()` cleanup
both attempt the plain container, product container and anchor as well as the other resources,
recovering an interrupted create without a CID only by its recorded name, image ID, labels and
mounts. Warm-ups change no limit, assertion or row selection and add no retry.
`TestEngineFixtureWarmup`, `TestEngineFixtureWarmupProduct` and `TestEngineFixtureWorkflow` check
wiring, decoded requests and ownership fixtures with harmless commands; they supply no runtime
engine evidence.

`TestFeatures` also launches one **suite warm-up** per suite process (T-57), through
`runPreparedFeatures`, after selection admits engine rows and before any row setup. It keeps the
first attempt and its result, failure included; nested schedules and repeated invocations cannot
repeat it, and disabled, invalid and empty selections prepare none. It builds the original
checkout apart, so it carries production's 10/120/10 seconds, and invokes `run -- /bin/true`
through the actual `offlineAPI` recorder, within a 60-second compilation bound, the 45-second
watchdog and 30 seconds of owned cleanup; the rows keep 1/3/1. It removes its worker and anchor by
full identity and leaves unrelated objects and the shared image alone. Any failure stops the suite
before a scenario runs, with an `engine suite warm-up:` stage diagnostic that keeps the original
failure beside cleanup failures. It prints one line, with no request body, approval, environment
or host path, whose offsets come from the single create journal entry (the interval is
`Answered - Forwarded`):

```text
engine suite warm-up timing: arrival_ns=<n|missing> forwarded_ns=<n|missing> answer_ns=<n|missing> forward_to_answer_ns=<n|missing> outcome=<passed|failed>
```

Success requires ordered nonnegative timestamps, HTTP 201, an uncancelled create, workload
completion and verified cleanup. Warm-up adds no selected or completed scenario, and its coverage
goes to a private directory outside both collected destinations (`TestEngineSuiteWarmup`). A
still-slow first create is the coordinator's to act on under the settled operating rules above; a
successful first row never authorizes changing a limit, assertion, retry or selected row. Failures
from the push through output publication name their stage and expected and observed values (T-34;
[validation functions](../tools/lib/engine-fixture-checks.sh)). Under q-43 both image stores are
accepted while the config is verified independently. Captured-response tests and source guard
checks supply no native-engine evidence.

The Engine scenarios step receives these fixture inputs beside its tier switch and the optional
`ITOS_CONTAINER_RED_SCENARIOS` selector described below:

| Variable | Meaning |
| --- | --- |
| `ITOS_CONTAINER_ENGINE_FIXTURE_IMAGE` | Exact loopback repository and measured manifest digest |
| `ITOS_CONTAINER_ENGINE_FIXTURE_ID` | Daemon image ID: config digest on graph-driver, manifest digest on containerd |
| `ITOS_CONTAINER_ENGINE_FIXTURE_UID` | Runner's non-root effective UID |
| `ITOS_CONTAINER_ENGINE_FIXTURE_GID` | Runner's effective GID |

Future engine Given steps must reject absent or malformed inputs and independently inspect cached
content; they must not acquire a substitute. Ordinary selection must not read the inputs or
initialize fixture hooks. `ITOS_` filtering strips them from product children, which receive the
image reference through their disposable, normally validated manifest. Do not create approval
state or bypass policy, manifest or metadata validation. For an absent-image case, derive a
different well-formed digest in the same repository and verify its absence by read-only
inspection; never remove the shared positive fixture. The registry is already gone when scenarios
start, so product request-journal assertions must prove zero acquisition: an unavailable registry
is not that proof.

Each shard's final `always()` step retries cleanup after failures or cancellation and removes the
remaining derived image references on success, in a 30-second bound per attempt. Cleanup verifies
recorded IDs, names, image IDs and labels before removal, requires the recorded config identity
before image removal, checks every removal's own failure (Bash disables implicit `errexit` in
conditional paths), attempts all recorded resources, is idempotent, preserves an earlier failure,
fails an otherwise successful job when incomplete, and never prunes, removes shared pinned inputs
or deletes checkout files. A killed runner cannot promise cleanup: that run is incomplete, not
evidence.

CI logs record the tested commit, server version/API/platform, source pins, measured image
reference and config ID, cold-pull and smoke results, and cleanup verdicts, which the coordinator
inspects at the actual pushed head even with zero live engine scenarios. Static workflow checks
and shell syntax are not RepoDigest proof, and fixture success is not a passed doctor or runtime
scenario. Images, ownership records and generated test evidence are never committed.

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
Standard-library Unicode whitespace separates tokens; repeated IDs execute only once per eligible
scenario/outline row. Bare IDs, arbitrary tags, lowercase areas, commas, regexes, godog operators
and shell-looking text fail. Every ID must exist in the parsed inventory, even if other IDs match.
Combining a nonempty list with `-scenarios` fails before build/setup.

The workflow passes the input only to **Engine scenarios** and the existing macOS/Windows
**Tests** steps, as step-local `ITOS_CONTAINER_RED_SCENARIOS` data, never as a shell expression.
The harness reads it only after `CI=true`, `GITHUB_ACTIONS=true` and
`GITHUB_EVENT_NAME=workflow_dispatch` are established; local, PR, push and other contexts do not
read it, even if present and malformed. Unset, empty or whitespace-only input keeps the live-only
selection in both tiers. Agents never spoof this context or access an engine.

A nonempty list selects only its exact IDs, live or `@wip`, while keeping every tier and platform
restriction. Inherited Feature, Rule, Outline and Examples tags apply, and an outline ID selects
all eligible rows. The input never enables the engine. The same list may reach the manual platform
jobs and every engine shard: engine IDs skip on manual platforms; in an engine shard, non-engine
IDs and IDs left to another shard skip; and `@linux` cases stay excluded on macOS/Windows. A wholly
excluded selection reports its IDs, reasons and count, runs no setup and supplies no red evidence.
Eligible selections must finish all expected cases successfully for the test command to pass;
actual failures stay failed.

Draft PRs skip automatic CI. The coordinator dispatches on the item branch after it contains the
complete step commit and the workflow/harness changes, which must also be on the default branch
for GitHub manual dispatch.

1. The behaviour implementer commits complete step definitions in a `test` commit while cases
   remain `@wip`, retaining `@engine` wherever setup, steps or cleanup need it. The agent stops.
2. The loop publishes the item branch and invokes the coordinator-only command:

   ```sh
   gh workflow run ci.yml --ref <item> -f 'red_scenarios=<ids>'
   ```

   Substitute whitespace-separated complete tags for `<ids>`. No tag-removal commit, evidence
   branch, smoke edit or draft-guard exception is needed. Ordinary Linux and proof jobs keep their
   normal inputs; only the engine and manual macOS/Windows jobs receive this selector.
3. The coordinator records the **resolved tested SHA**, input IDs, run URL, runner/tier, scenario
   ID/outline row and failing **Then** assertion. Infrastructure, setup and undefined-step
   failures and empty or excluded selections are not red proof. A passing run is not red proof
   either; no forced failure or ignored test failure is permitted. Keep the failed run visible.
4. Implement, remove `@wip` in the behaviour commit and require green CI at the actual pushed head
   **with no red input**. Expected-red dispatches are never merge/closure prerequisites or
   substitutes for that green run. The loop alone publishes, dispatches, reads results and closes.

Non-engine cases that can run locally keep the ordinary local red-first procedure. This manual
route adds no product scenarios, supported-host claim or runtime evidence by itself: local and
harness checks (such as the `TestLifecycle*` ones) are not native evidence, and no container-host
support is claimed without CI runtime evidence.

## Tags

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
The ordinary smoke run may explicitly skip that ID because the required engine shards run all
live engine cases between them at the same head. Listing, footer validation and smoke-coverage rules remain
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
- **Cite evidence for a mount option claim.** When a spec (ledger entry, scenario comment, step
  oracle or fixture record) says an exact mount option string makes a mount executable, writable
  or read-only, or a path reachable, it cites the engine source that turns the requested options
  into the mount, or a native CI observation of the mounted result and the behaviour. An option
  left out of a request is not evidence: the engine may add, reorder or override options (moby
  prepends `noexec` to tmpfs). Local and harness checks are not native CI evidence, as stated
  below.

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
