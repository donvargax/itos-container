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
tier and the live set. An empty expression selects the whole tier. Every OR branch retains
both restrictions, so neither an explicit ID nor `@wip` can bypass them.

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
`TestEngineWorkflow` parses the workflow and rejects broken fixtures without executing Docker.

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
bounds it to 30 seconds. No install/start, image pull/build, container creation or fallback
is attempted. The subsequent step alone enables the switch and runs this **CI-only** command:

```sh
ITOS_CONTAINER_ENGINE_TESTS=1 go test ./features -count=1 -v -timeout=5m -run '^TestFeatures$'
```

The step has a five-minute budget and the job a ten-minute budget. These are test-infrastructure
budgets, not product startup/resource limits. The step always selects every live engine case,
independent of smoke selections, changed IDs or a green baseline. Pending, undefined, failed
or skipped runtime steps fail; losing selected engine cases also fails.

There are currently no live engine scenarios. The preflight still runs, and the harness
explicitly skips with `no live @engine scenarios (count 0)`, including an all-`@wip` engine set.
This is infrastructure readiness, not sandbox or supported-host proof. Later behaviour specs
must settle runtime fixtures and image acquisition before adding native probes.

Mutation, own-test and scenario-coverage runs in `ci` remain engine-disabled, including
individual-test fallback and whole-suite collection. An excluded engine ID provides no
runtime or coverage evidence. Later behaviours require domain/port fake tests alongside
separate runtime evidence; uncovered code cannot be satisfied by enabling Docker in proof.

## Tags

| Tag               | Meaning                                                                              |
| ----------------- | ------------------------------------------------------------------------------------ |
| `@phase-<n>`      | The phase a scenario belongs to.                                                     |
| `@<item>`         | The work item that turns it green (`@count-cli`).                                     |
| `@ID-<AREA>-<nn>` | Stable scenario ID. Commits reference these. Never reuse or renumber them.           |
| `@bug-<n>`        | Reproduces a fixed bug. Added by `fix:` commits.                                     |
| `@wip`            | Written, not yet implemented. Excluded from both tiers, and a `feat` may not name it. |
| `@engine`         | Setup, hooks, steps or cleanup need a real engine. Runs only in the CI engine tier. |

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
  commit. The coordinator obtains assertion-level red evidence in CI with `@wip` removed
  for that run, then green evidence after implementation at the same permitted tier.
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
