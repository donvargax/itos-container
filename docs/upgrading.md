# Upgrading

## Manifest inspection

Ordinary `doctor` now validates the policy-selected manifest after policy validation.
Callers that previously used it to check policy alone must use `doctor --policy-only`.
`doctor --manifest-only` explicitly requests file and workspace validation without
image metadata, host readiness or approval checks. Neither mode authorizes a run.

## Tooling

Read the current branch's configuration and tool comments before moving a pin. Use
`itos upgrade <version>` for supported migration: it updates the repository pin and checksum,
applies published configuration migrations, updates a managed install script when present, and
refreshes an existing itos rules block. Read its remaining "By hand" steps. The command lists
workflow action references and version inputs but does not edit them; reconcile those manually.

For the move from 7.0.0-rc.1 to 8.45.3, the pin and configuration were already upgraded.
T-3 aligns both workflows with that pin using
`donvargax/itos@0a8c1bcfce6ae7b67b39197ee3990a90343f35ac # v8.45.3`, removing each action's
`with/version` override. Verify the SHA against the
[upstream release tag](https://api.github.com/repos/donvargax/itos/git/ref/tags/v8.45.3), not just
the comment. The pinned [action](https://github.com/donvargax/itos/blob/0a8c1bcfce6ae7b67b39197ee3990a90343f35ac/action.yml)
resolves that SHA to its release without a version input; the repository pin chooses the itos
that runs. Keep the existing pin checksum, CI steps, and `itos verify` / `itos ci run` calls.

Preserve verified checksums when upgrading tools fetched by `tools/bin/pinned`; never update only
a version beside stale hashes. The itos-cc migration under T-4 is recorded below.

Go dependencies are locked in go.mod/go.sum. Use the project's gates to check module
verification and tidy-diff, and review code and vulnerability results before an update lands.
The Go toolchain is pinned too: its standard library is part of the resulting code, so security
patches matter even when module versions stay unchanged.

The owner's tools can be pinned on release. Other updates normally wait seven days, with
security fixes following the existing security-update policy. Renovate's committed configuration
does not by itself prove its GitHub App is installed or the bot is running.

`itos ci run` alone judges fresh changed-code proof through `proof.code`, as specified by T-13.
Agents commit and stop: no local mutation runs, no generated mutation or coverage records in Git,
and no evidence restored into CI from another machine. The ignored `.metrics/mutate/` and
`.metrics/coverage/` directories are not handoff artifacts. Kill survivors with tests and cover
uncovered changed lines; never redraw to escape a failure. Only truly equivalent mutants may
be excepted with a reason for the person to review; uncovered code needs tests, never exceptions.
A workflow/docs-only change touches no `proof.code` path and fabricates no mutation results.
The no-start message from `itos ci plan --whole` is not a proof pass. Scenario red-first
obligations remain separate and unchanged.

Numbered `itos work add --kind slice|task|bug`, `itos task add`, and `itos work promote` mint their
IDs; do not prescribe numbered IDs by hand. Stage only explicit paths and commit through
`itos commit`. The bans on `--no-verify`, `commit -a`, and `git add -A` still apply.

A plain `itos push` does not publish tags; deliberate tag publication requires `--tag`. The
existing release workflow creates its tag locally, uploads a draft, then publishes it through
GitHub, which creates the remote tag. It does not call `itos push`. Preserve that sequence and
the nonempty-version condition. T-3's migration did not enable releases; T-25 separately
authorizes the caller described below, without a tag-push step.

## Binary releases (T-25)

On a push to `main`, CI calls the reusable release workflow only after `ci`, `engine` and the
automatic Linux `platform` job all succeed at that SHA. Failed, cancelled or skipped prerequisites
do not qualify. Pull requests and manual dispatch never publish; manual macOS/Windows jobs are
not release prerequisites. Only the release caller and reusable job have write permissions.
The workflow checks out that SHA with complete history and tags, calculates a version, then
creates a local tag, builds a GoReleaser draft, attests the archives, generates notes and publishes.
GitHub creates the remote tag on publication; the tools themselves only read and calculate.

The first releasable history selects `v0.1.0`, including fix-only and breaking-only histories.
With a stable reachable release tag, breaking changes take major precedence, then features
minor, then fixes patch. Otherwise a changed linked module or Go toolchain selects a patch;
prose, test-only dependencies and tool updates alone do not release. Empty `next` skips
publication. Shallow history, read errors and a module-major mismatch fail before any tag.

The five Linux/macOS/Windows archives and SHA-256 checksums distribute binaries; they do not
establish container-host support. Template-generator releases are separate. This guidance
enables the workflow but is not evidence that a tag or published release already exists.

The helpers and git-cliff configuration come from the owner's
[itos-template revision 79301eba](https://github.com/donvargax/itos-template/tree/79301eba51b9768a27906c720e39c49b08491e67),
inspected alongside [itos's release workflow](https://github.com/donvargax/itos/blob/0dfbdf2e80d1bafb08a884edaa3171c5bbfe2a14/.github/workflows/release.yml).
They reuse this project's `internal/release` under T-25's narrow q-44 architecture exception.
Attribution and AGPL-3.0 permission remain in force. Git-cliff stays at 2.14.2 with the template's
four verified archive hashes; acquisition uses the existing binary asset API and checksum check.

Before the first real tag, run `itos task T-25`. Its final check runs the workflow's actual
`go run ./tools/bin/release-version` at the candidate HEAD, then builds both helpers and runs
them in a complete local clone with no tags. It passes the calculated last/next to notes,
verifies every commit link against the selected range, and repeats notes after creating a
fixture tag only in that disposable clone. Notes must match before and after the fixture tag;
neither helper may change HEAD, refs or working files, in either source or clone. No archive
build or publication occurs. Report the candidate SHA and exact last/next/bump/range; the
coordinator alone verifies the eventual pushed main run and released assets.

## itos-cc 0.5.0 to 0.47.2

T-4 pins the launcher to [0.47.2](https://github.com/donvargax/itos-cc/releases/tag/v0.47.2).
Its five archive hashes were verified on 2026-11-04 against the
[release manifest](https://github.com/donvargax/itos-cc/releases/download/v0.47.2/checksums.txt),
whose SHA-256 is `7ebfae6b1f2193b79425289af3a436e4ee25efe27e6ee11b04f49ac984f9ef30`,
and the release API's asset digests. Keep Linux and macOS amd64/arm64 and Windows amd64;
these tool downloads do not expand supported container hosts. The launcher still verifies an
archive before extraction and supports `--path`; on Linux its cache miss installs
`.tools/itos-cc-0.47.2/itos-cc`. Other tools keep their existing pins.

T-13 replaces T-4's cached checking and sampling with the counted-run contract in the
[0.47.2 README](https://github.com/donvargax/itos-cc/blob/v0.47.2/README.md). CI executes this
`proof.code.check` template through the existing launcher:

```sh
tools/bin/pinned itos-cc mutation run --count 10 --since {base} --seed {base} --fail-uncovered=lines --json cmd internal
```

Core itos supplies `{base}` from the judged CI range for both `--since` and `--seed`; neither
HEAD nor an item's take/specification commit replaces it. The maximum is ten sites across
changed functions, not ten per function or a full baseline. The same base and eligible sites
give the same draw; changing the candidate set can change it. `--fail-uncovered=lines` requires
measured coverage of added or changed lines, including functions without sites, superseding
the earlier whole-function requirement. Counted runs write no snapshot, annotation or coverage
cache. No all-tests override or cached sample is part of this proof. Task checks use the
runner's shell tools directly; RTK is not a CI dependency.

`itos-cc.yaml` now lists live scenarios through `itos tests list scenario --tsv` and preserves
the two T-8 equivalent-mutant exceptions and their reasons. The omitted format version means
version 1. The strict parser rejects unknown keys, anchors, aliases, merge keys, non-JSON tags,
non-string keys and extra documents. T-36 makes the harness collect the listed scenarios in
one run when itos-cc sets `ITOS_CC_TEST_COVERDIR`, writing atomic product counters to
`<root>/<ID>`. Both the CLI and command test assembly use covered builds. The per-ID root
takes precedence over `GOCOVERDIR`, whose aggregate route remains available. See the
[feature coverage contract](../features/README.md#product-coverage) for routing, secondary
worlds and cleanup. This replaces reliance on the collector's individual-test fallback
without changing the pinned tool or its mutation contract.

The existing engine CI run also collects and checks its selected IDs. Platform jobs keep
running all tests without unused coverage output. Local collection is not code proof:
T-36 closure still requires a fresh pinned-itos-cc CI run on the actual pushed head over
a production-changing judged range, reporting "the listed tests ran in" under 600 seconds,
with no missing-ID error. The coordinator records its URL, SHA, base, count and elapsed
time. A failed or missing collection, cached result or no-code-change skip is not proof.

T-4's results are historical. Its launcher/docs range from
`ebd187e7f0722f65d9aadda7a13d27048e4e03f6` passed with no source files, proving compatibility
rather than a Go function. Its inherited snapshot held three `Example` mutants in
`internal/release/releasetest/releasetest.go`, but the upgraded sample found no eligible cache:
**zero mutants were sampled**. T-8 later recorded a full foundation measurement and two
equivalent exceptions. Its [report](proof/T-8-foundation-baseline.md) preserves those results,
the toolchain-freshness failure and recovery. T-13 removes the generated records from the
current tree; Git history retains them. The historical reproduction recipes are not current
requirements, and neither the new sampled proof nor an empty range establishes a full baseline.

The intervening releases also changed mutation's plain output and several nonzero exit codes
([0.16.0](https://github.com/donvargax/itos-cc/releases/tag/v0.16.0)). Our callers neither parse
the plain output nor distinguish those error codes; exit status remains the verdict.
