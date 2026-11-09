# T-45: the engine tier runs in three shards of whole features, each inside 180 seconds

Recorded for T-45 on 2026-11-09. This overrides the part of [arc42](../architecture/arc42.md),
**Future behaviour evidence**, that describes one dedicated `Engine scenarios (Linux amd64)`
job enabling the engine switch and running the whole tier, and the unsharded 240-second
step budget [T-33](../../features/README.md) set. It changes where the tier's rows run and how
long the measured step may take. No product code, seam value, scenario, assertion, cleanup or
overlap drain bound changes, and the record is no runtime evidence and no support claim.

## Why

The engine step has run at its limit on slow runners since T-33 landed. The coordinator read
the step's time from public job metadata, without a token, for 33 main pushes from T-33's
merge (`4641ad4`) to T-42's (`19a188f`): 162 to 196 seconds on 11 runs and 199 to 245
seconds on 22. T-33's own merge took 238 seconds, about two seconds inside its budget. Run
`42635414385` took 245 seconds and ended in the test binary's four-minute timeout, exit code
2; the other slow failures read exit code 1, which the 240-second budget and a failed
scenario both give.

The title T-45 was promoted under blamed T-41. That premise is wrong. Before T-41 merged
(`d1099fc`) the runs split 5 and 8, at 162 to 173 and 199 to 242 seconds; after it, 6 and 14,
at 164 to 196 and 212 to 245. The wait T-41 lengthened, `finishLocalWorkloads` through
`overlapDrainBound`, runs only for a covered fixture that is not native, and both `@engine`
overlap scenarios use native fixtures. The step runs `TestFeatures` alone, so T-41's unit
tests never ran in it. From `966e456` to `d1099fc` those two files are all that changed. The
2 min 49 s, 3 min 44 s and 4 min 5 s of runs `42615366138`, `42617313777` and `42618748581`
read as fast and slow runners, not as a step. Nothing measured points at one step. The step runs 75 examples one at a time, 28 more than
the 47 T-33 measured, and compilation is a fraction of it: on four CPUs a cold compile of the
test binary took 21 seconds and the covered product build 16, and 4.7 and 0.8 seconds warm.
The rest is engine work under `-cover`, and under the ptrace syscall observer for recovery
processes, which a slow runner slows throughout.

Whether a newer runner image changed anything stays unconfirmed. [Decision 0020](0020-descriptor-backed-checkout-anchor.md)
records a saved engine log from run `42491204197`, earlier than these runs, that already shows
Ubuntu 26.04.1 and Docker 29.4.2, so the image had probably moved before the slowdown was
measured. The loop therefore records each shard's runner image and Docker version beside its
step time, and a later image change is not taken for a regression.

## What changes

[features/README.md](../../features/README.md) is the operational authority; the rule is T-33's
own. If the measured unsharded suite still exceeds 240 seconds, T-33 calls for whole-feature
shards on separate native runners with an exact parsed-example partition check. Main's did,
at 242, 244 and 245 seconds and in that timeout, so:

- The engine job becomes `engine-shard`, a matrix of three shards on separate native
  `ubuntu-latest` runners and daemons, each of whole feature files, with `fail-fast` off.
  **offline** is `offline.feature` (24 examples), **lifecycle** is `lifecycle.feature` (19,
  its four `ID-LIFECYCLE-09` rows still nested in `ID-LIFECYCLE-13`'s quiet row) and
  **other** is `anchors`, `checkout-overlap`, `doctor`, `hardlinks`, `recovery`, `session` and
  `sockets` (32, of which 12 are `ID-SOCKETS-01` refusals before any worker exists).
- Each shard's measured step fails above 180 seconds, a budget well under the four-minute
  limit, by the step's existing wrapper on every run. The job, step and Go test limits stay
  10, 5 and 4 minutes, and fixture preparation keeps its own. An estimate from the shares of
  examples and a cold compile, no measurement, puts the slowest shard between two and two
  and a half minutes on today's slowest runner.
- `engine` keeps its id and the stable name `Engine scenarios (Linux amd64)` that `ci`,
  `release` and the repository's settings use, and becomes an aggregate: it needs
  `engine-shard` alone, runs under `always()` plus the event guard for one minute, and its one
  step requires the shard job's result to be `success`. GitHub's contexts reference does not
  spell out a matrix job's result in `needs`; the gate takes success to mean that every shard
  succeeded, and the loop confirms that on the first red shard.
- The selector is the harness-only environment input `ITOS_CONTAINER_ENGINE_FEATURES`,
  whitespace-separated feature basenames set on the Engine scenarios step alone, read as the
  red input is, so tests hold it as data. Blank is the complete tier. `selectScenarios`
  refuses a nonempty value outside the engine tier, a name that is not a basename ending in
  `.feature`, a name given twice and a name no feature with an effective `@engine` case bears,
  before any build or fixture. The selection and the check of godog's filtered pickles read
  the shard's files alone, and the run reads the same files, because godog numbers pickles
  across the files of one read. Red IDs and `-scenarios` are still validated against the full
  inventory, and a shard whose red IDs all belong to others skips by name.
- No daemon, state or fixture output crosses runners. Each shard keeps what T-33 required of
  one engine job: the exact checkout, the pinned setup, the read-only preflight, guarded
  fixture preparation and cleanup, the step-local switch, `setsid --wait`,
  `-test.paniconexit0`, the timing records, the coverage check of its own IDs and the
  diagnostics. Exactly one engine switch remains, in that step, and manual red input reaches
  every shard's step.

`TestEngineShards` holds the partition against the real feature files. Move a whole feature
between shards only with measured shard times that show the need, and never split one.

## Alternatives left

- **Lowering the unsharded budget.** Today's suite took at most 180 seconds on 9 of those 33
  runs, so a check of it would pass about one run in four with nothing done, and no known wait
  would bring the slow runs under it.
- **Shorter harness-seam budgets.** The seam already holds call, startup and grace at 1/3/1
  seconds and the quiet observation at 4, and the engine steps' other waits end on their events,
  costing time only on a fault. T-42 showed a slow runner can cut LIFECYCLE-09's acknowledgement
  hold at the 1 second call budget; shorter budgets would leave slow runners less room still.
- **One shard per feature**, as T-33 wrote: nine engine runners a run beside the other jobs,
  while the loop keeps several runs going at once. Three keep each shard under half the
  examples at a third of the runners.
- **Raising a limit**, a larger runner, a retry or a reduced assertion. T-33 excludes the last
  three, and raising a limit is the person's call, not a fix.
- **Cutting the control transaction's repeated `Unchanged()` walks.** That needs the person's
  security review, and nothing measured points at them.
