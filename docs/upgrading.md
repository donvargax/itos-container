# Upgrading

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

`itos ci run` enforces changed-code proof through `proof.code`. When code or relevant test imports
change, record fresh mutation results with the pinned itos-cc using all-tests, fail-uncovered and
no-annotate, and commit `.metrics/mutate/` before the coordinator pushes. Use the parent of the
item's actual first commit, specifications included, as the proof base; do not assume its take
commit is first. Kill survivors with tests; only truly equivalent mutants may be excepted, with
a reason for review. Uncovered mutants need tests. Keep the proof configuration and thresholds.
A workflow/docs-only change touches no `proof.code` path and needs no fabricated mutation records.
The no-start message from `itos ci plan --whole` is not a proof pass.

Numbered `itos work add --kind slice|task|bug`, `itos task add`, and `itos work promote` mint their
IDs; do not prescribe numbered IDs by hand. Stage only explicit paths and commit through
`itos commit`. The bans on `--no-verify`, `commit -a`, and `git add -A` still apply.

A plain `itos push` does not publish tags; deliberate tag publication requires `--tag`. The
existing release workflow creates its tag locally, uploads a draft, then publishes it through
GitHub, which creates the remote tag. It does not call `itos push`. Preserve that sequence and
the nonempty-version condition; this migration neither enables releases nor adds a tag-push step.

This branch has no CLI binary release yet. Template releases and the generator's adoption/update
workflow are separate from made-project binary release tooling and are not claimed here.

## itos-cc 0.5.0 to 0.47.2

T-4 pins the launcher to [0.47.2](https://github.com/donvargax/itos-cc/releases/tag/v0.47.2).
Its five archive hashes were verified on 2026-11-04 against the
[release manifest](https://github.com/donvargax/itos-cc/releases/download/v0.47.2/checksums.txt),
whose SHA-256 is `7ebfae6b1f2193b79425289af3a436e4ee25efe27e6ee11b04f49ac984f9ef30`,
and the release API's asset digests. Keep Linux and macOS amd64/arm64 and Windows amd64;
these tool downloads do not expand supported container hosts. The launcher still verifies an
archive before extraction and supports `--path`; on Linux its cache miss installs
`.tools/itos-cc-0.47.2/itos-cc`. Other tools keep their existing pins.

The [0.47.2 README](https://github.com/donvargax/itos-cc/blob/v0.47.2/README.md) and
[intervening releases](https://github.com/donvargax/itos-cc/releases) preserve our commands:

```sh
tools/bin/pinned itos-cc mutation sample --count 10
tools/bin/pinned itos-cc mutation check --since <base> --fail-uncovered --json cmd internal
```

Keep the cached check, its scope and thresholds. A counted mutation run is a different operation;
`--fail-uncovered=lines` would narrow the existing whole-function requirement. The task checks
use the runner's shell tools directly; RTK is not a CI dependency.

`itos-cc.yaml` remains valid unchanged: `mutation.exceptions: []`, with the omitted format
version meaning version 1. The stricter parser rejects unknown keys, anchors, aliases, merge
keys, non-JSON tags, non-string keys and extra documents. This file uses none of them. No
exclusion, exception, `mutation.tests` or `mutation.copy` setting is needed for this upgrade.

[0.7.0](https://github.com/donvargax/itos-cc/releases/tag/v0.7.0) introduced strict Go coverage
evidence and whole-suite freshness. Strict `--fail-uncovered` needs fresh evidence for every
judged Go function, even one with no mutation sites, and fails uncovered executable blocks as
well as uncovered mutants. Whole-suite outcomes depend on the suite and configured support
files. [0.18.0](https://github.com/donvargax/itos-cc/releases/tag/v0.18.0) changed function
hashes to ignore comments; [0.47.0](https://github.com/donvargax/itos-cc/releases/tag/v0.47.0)
invalidates earlier strict coverage evidence. Old results can therefore become stale without a
source edit. Our empty exception list needs no `mutation except --rehash` migration.

When a changed-code check requires stale evidence, remeasure it with the repository launcher:

```sh
tools/bin/pinned itos-cc mutation run --since <base> --all-tests --fail-uncovered --no-annotate cmd internal
```

Commit only the resulting project-local `.metrics/mutate/` records. Kill survivors with tests;
uncovered code needs tests, never an exception. Do not hand-edit evidence or import another
project's cache. T-4's base is `ebd187e7f0722f65d9aadda7a13d27048e4e03f6`, the parent of its first
specification commit, `786ea2f2cb1f75351e30c4b75c37d98437b3f418`. Its launcher/docs changes touch
no `proof.code` path: the range check passes with no source files, proving compatibility rather
than any Go function. They require no new mutation records.

Sampling reruns at most ten fresh recorded outcomes, skipping stale or missing results, with a
seed derived from HEAD. The inherited snapshot contains three `Example` mutants in
`internal/release/releasetest/releasetest.go`; after this upgrade the sample reports no cached
mutant, so **zero mutants were sampled**. Passing that check does not refresh the baseline or
prove the whole codebase. A full foundation baseline remains separate work.

The intervening releases also changed mutation's plain output and several nonzero exit codes
([0.16.0](https://github.com/donvargax/itos-cc/releases/tag/v0.16.0)). Our callers neither parse
the plain output nor distinguish those error codes; exit status remains the verdict.
