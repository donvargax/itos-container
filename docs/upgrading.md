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
a version beside stale hashes. The pinned itos-cc upgrade is separate work under T-4.

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
