# Upgrading

Read the current branch's configuration and tool comments before moving a pin. itos's repository
pin and workflow action move together through its supported upgrade command. Preserve verified
checksums when upgrading the tools fetched by `tools/bin/pinned`; never update only a version
beside stale hashes.

Go dependencies are locked in go.mod/go.sum. Use the project's gates to check module
verification and tidy-diff, and review code and vulnerability results before an update lands.
The Go toolchain is pinned too: its standard library is part of the resulting code, so security
patches matter even when module versions stay unchanged.

The owner's tools can be pinned on release. Other updates normally wait seven days, with
security fixes following the existing security-update policy. Renovate's committed configuration
does not by itself prove its GitHub App is installed or the bot is running.

Fresh mutation records are required when code or relevant test imports change. Use the item's
actual first-commit parent as the proof base; do not assume its take commit is first.

This branch has no CLI binary release yet. Template releases and the generator's adoption/update
workflow are separate from made-project binary release tooling and are not claimed here.
