# T-8 foundation mutation baseline

Measured on Linux amd64 with `tools/bin/pinned itos-cc` 0.47.2. The initial
measurement used `go version go1.27.3-X:nodwarf5 linux/amd64`; the CI recovery
below uses standard Go 1.27.2, selected by the workflow from `go.mod`.
This is evidence for the admitted Go foundation on that platform, not other
platforms or container isolation.
No real-engine scenarios ran. Pins, dependencies, hooks, CI, proof paths and
thresholds remain unchanged.

## Initial measurement

The full fresh run at `92b1c497fbfdb742dfe5833c361e901b0d303085` exited 1:
15 files, 69 function records (48 with sites and 21 without), 225 mutants,
221 killed, four survived, no uncovered mutants, and 30 uncovered executable
blocks. No file was reported not built.
The strict cached check also exited 1 with those same failures.

The survivors, at that initial tree, were:

| Site | Mutation | Disposition |
| --- | --- | --- |
| `cmd/itos-container/main.go:76:15`, `run` | `>` to `>=` | Equivalent: empty arguments have already become `--help`. |
| `internal/cli/cli.go:120:10`, `report` | `0` to `1` | Test an error group with no leaves, whose aggregate code is zero. |
| `internal/release/build.go:183:14`, `extract` | `!=` to `==` | Test rejection of an archive larger than a pipe buffer; the producer must be drained before waiting. |
| `internal/release/release.go:74:58`, `Newest` | `>` to `>=` | Equivalent: the comparator returns zero only for identical tag strings. |

The uncovered blocks were reported at these initial-tree lines:

| File | Lines |
| --- | --- |
| `cmd/itos-container/main.go` | 74, 102, 151 |
| `internal/cli/completion.go` | 113 |
| `internal/release/build.go` | 81, 95, 105, 112, 121, 125, 164, 177, 180, 184, 202, 205, 214, 217, 224, 228 |
| `internal/release/releasetest/releasetest.go` | 28, 78, 81, 90, 103, 106, 110, 114, 123 |
| `internal/version/version.go` | 45 |

## Changes

Refactor commit `d6a3281b6f9c3e9bcbb9b2015d7801ef67b969ef` separates terminal
detection, build metadata and release tool execution from their callers. Kong
options exercise invalid models and unhandled commands. A narrow fixture helper
interface lets tests record fatal failures. Default behavior and executable
error paths remain intact; no sites were removed to avoid measurement.

Test commit `bf637d977264bd7fdecf756ea2ecda7a883a5d57` adds deterministic checks
for the listed errors, error aggregation, completion after `--`, and untagged
fixtures. Filesystem failures use actual conflicting paths; archive draining
uses a real child process. Terminal logging no longer skips under redirected
test runners. Focused package tests passed. No behavior scenario was changed or
added, so there are no red/green scenario steps to report.

The repaired run at the test commit killed 223 mutants, left only the two
equivalent survivors, and reported no uncovered mutants or executable blocks.
Exception commit `ac3d7a69c18a53ee80809dda6ee37fcdcc2f4f22` records those two
through the pinned tool's `mutation except` command. Their current identities
are `cmd/itos-container/main.go:77:15` in `run` (hash `62a146aba98870d1`,
function-relative line 21) and `internal/release/release.go:74:58` in `Newest`
(hash `169323ab8c351ad2`, function-relative line 4), both `>` to `>=`.
The exact identities and reasons are in `itos-cc.yaml` for the person's review.

## Reproduction

Run from the repository root, without replacing the pinned launcher. Select
the standard toolchain and clear the host's Go experiment setting first:

```sh
export GOTOOLCHAIN=go1.27.2 GOEXPERIMENT=''
tools/bin/pinned itos-cc mutation run --all-tests --mutate-all --no-coverage-cache --fail-uncovered --no-annotate cmd internal
tools/bin/pinned itos-cc mutation check --fail-uncovered --json cmd internal
tools/bin/pinned itos-cc mutation check --since 4ac0cc4b017feb219ba1645ca4263a7dd26870fd --fail-uncovered --json cmd internal
```

The range base is the parent of T-8's actual first specification commit,
`1536aa563c94175e761b5d2186a7bc3f322e29fb`. The full run is the foundation
baseline; the range check is additional proof.

## Original local result

All three commands above exited 0 after the last source, test and exception
change. The measured tree was `ac3d7a69c18a53ee80809dda6ee37fcdcc2f4f22`; its
source and tests are those of `bf637d977264bd7fdecf756ea2ecda7a883a5d57`.
Proof commit `6e6d4c9601a8fac9f829f1f88dd243faff3fd677` records the 15 generated
`.metrics/mutate/` files by explicit path, including the refreshed legacy
fixture snapshot. No coverage scratch/cache output was committed.

The full run freshly judged all 225 mutants, reusing none: 223 killed, two
excepted, no other survivors and no uncovered mutants or executable blocks.
All 73 function records have complete strict coverage evidence, including 25
without mutation sites. The whole-scope check reports 48 functions with sites.

| Scope | Files | Function records | Mutants | Killed | Excepted |
| --- | ---: | ---: | ---: | ---: | ---: |
| `cmd/itos-container` and `internal/cli` | 5 | 33 | 117 | 116 | 1 |
| `internal/count` | 6 | 13 | 23 | 23 | 0 |
| `internal/release` | 3 | 20 | 71 | 70 | 1 |
| `internal/version` | 1 | 7 | 14 | 14 | 0 |

The range check reports four files and seven functions with sites, with 41
killed and one equivalent exception. Strict coverage also checks changed
functions without sites. Neither full nor range checking reported any file
as not built on Linux amd64. Counts describe this measurement, not thresholds
for future changes.

The configured commit hooks accepted the commits. The whole test suite passed
in the coverage and unmutated baseline stages. Pushing, hosted CI validation
of the actual pushed head, review of the two equivalence reasons, and item
closure remained with the coordinating loop at that handoff.

## CI recovery

The [landing run at 7e64b02](https://github.com/donvargax/itos-container/actions/runs/41832816369)
failed strict proof with `mutation.coverage-stale` on `@go-env`, and its Windows
test job failed `TestExtractReportsPipeAndStartFailures/executable_missing`.
Linux and macOS tests passed. The failed run is the coordinator's supplied
evidence; this recovery session could not fetch its logs because the GitHub
connection required reauthentication.

Test commit `ed380259f39e8b0d410eda4eaf701c143bf78ffd` recognizes both specific
missing-executable errors. Windows performs extension lookup before starting
the process and returns `exec.ErrNotFound`; Unix returns `os.ErrNotExist` from
process start. Other errors, including a nil result, still fail the assertion.
The focused release test passed on Linux with standard Go 1.27.2. No production
code or behavior scenarios changed; Windows execution remains with platform CI.

The old proof passes on the customized host toolchain, but switching to standard
Go 1.27.2 reproduces `mutation.coverage-stale` on `@go-env` before any edits.
The pinned itos-cc fingerprints `GOVERSION` and `GOEXPERIMENT`, among other build
settings. The [pinned setup-go implementation](https://github.com/actions/setup-go/blob/b7ad1dad31e06c5925ef5d2fc7ad053ef454303e/src/installer.ts#L645)
selects the `toolchain go1.27.2` directive in `go.mod`. Remeasuring with that
standard compiler repairs the evidence instead of changing the freshness check.

The recovery environment is Linux amd64 for both host and target, Go 1.27.2,
`CGO_ENABLED=1`, `GOAMD64=v1`, and empty `GOFLAGS`, `GOEXPERIMENT`, `GOARM` and
`GO386`. `GOTOOLCHAIN=go1.27.2` selects that compiler locally; itos-cc fingerprints
the resolved compiler version rather than how it was selected. Use the exports
above for local checks and coordinator landing commands on this host. Its default
customized compiler cannot validate proof made by a different Go environment.

The full fresh command in Reproduction exited 0 against source/test commit
`ed380259f39e8b0d410eda4eaf701c143bf78ffd`, as did both cached checks. The exact
previously failing landing range also passed:

```sh
tools/bin/pinned itos-cc mutation check --since 2110a1d52a6939c2ef3171ddbb5e55e0d6a39f2e --fail-uncovered --json cmd internal
```

Proof commit `e3738730433c749de1c84f561708c4b9e3f569c7` saves all 15 generated
snapshot files. The run freshly judged all 225 mutants with no reuse: 223 killed,
two existing equivalent exceptions, no other survivors and no uncovered mutants
or executable blocks. All 73 function records have complete strict coverage,
including 25 without sites. No file was reported not built. Both range checks
report four files, seven functions with sites, 41 kills and one exception. The
scope totals in the original result remain the same.

The regenerated snapshots differ only in the changed test's fingerprints and
the resolved Go environment fingerprint; mutation outcomes and coverage blocks
are unchanged. No evidence was hand-edited and no coverage scratch output was
staged. The full run's unmutated coverage and baseline tests passed. The
coordinator must push and validate the actual head, including Windows execution,
before closing the item. This session neither pushed nor launched hosted CI.
