# T-8 foundation mutation baseline

Measured on Linux amd64 with `tools/bin/pinned itos-cc` 0.47.2 and
`go version go1.27.3-X:nodwarf5 linux/amd64`. This is evidence for the admitted
Go foundation on that platform, not other platforms or container isolation.
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

Run from the repository root, without replacing the pinned launcher:

```sh
tools/bin/pinned itos-cc mutation run --all-tests --mutate-all --no-coverage-cache --fail-uncovered --no-annotate cmd internal
tools/bin/pinned itos-cc mutation check --fail-uncovered --json cmd internal
tools/bin/pinned itos-cc mutation check --since 4ac0cc4b017feb219ba1645ca4263a7dd26870fd --fail-uncovered --json cmd internal
```

The range base is the parent of T-8's actual first specification commit,
`1536aa563c94175e761b5d2186a7bc3f322e29fb`. The full run is the foundation
baseline; the range check is additional proof.

## Final result

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
closure remain with the coordinating loop; there is no hosted CI run to cite
from this implementation session.
