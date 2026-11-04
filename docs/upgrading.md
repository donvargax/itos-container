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
non-string keys and extra documents. The existing harness builds with coverage when itos-cc
sets `GOCOVERDIR`; its documented individual-test fallback collects each listed scenario's
counters alongside own-test coverage. A failed or missing collection is not proof. Coverage
is produced in the `ci` job; platform jobs keep running all tests without unused coverage output.

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
