# Architecture fixtures

Compile every disposable module, including tests, before checking the pinned depguard
policy and package graph. Import refusals name the importer, dependency and rule; tool
errors never satisfy a negative test. Run through `tools/bin/architecture-check --fixtures`.
The matrix stays under testdata so ordinary `go test ./...` does not launch the linter.

T-14's pinned-tool acquisition uses GitHub's binary asset API with release asset
IDs and the existing version/checksum pins. Slice-17 run 42470010588 failed before
the matrix when the browser download route returned six HTTP 504s. A local GET
reproduced that failure; the official asset API returned the archive with the
existing SHA-256. This changes acquisition, not the policy or fixture verdicts.
The command helper keeps stdout values separate from stderr diagnostics, so a
recovered download retry cannot become part of the linter's executable path.
Exhausted retries, failed commands and checksum mismatches still fail the gate.

Before the correction, `TestPinnedAssetRedirect` failed to obtain a binary asset,
and `TestSuccessfulCommandKeepsStderrOutOfValue` failed because the returned path
included the retry diagnostic. The focused checks cover media-type negotiation,
redirects, retries, exact path output, checksum rejection and failure diagnostics;
the commit hook runs the unchanged compile-first policy matrix.

`review-cases.json` specifies T-14's follow-up refusals and approved pure imports;
TestPolicy always runs it alongside `cases.json`. Before enforcement, the new data
was appended temporarily to `cases.json` and run with the original checker: 97 assertions failed
after compilation, with no infrastructure errors. Ignored-file probes also compile
by explicit filename, independently of their build constraints.

`thread-cases.json` records the three subsequent PR findings. Temporarily including
it in TestPolicy's input list and running `TestPolicy/thread-` against the pre-fix
checker produced 15 assertion failures after compilation: six trailing-comma
suppressions escaped, eight valid build-constrained porttest helpers were refused,
and a valid two-module workspace failed inspection. Three targeted/prose comment
controls passed. The test commit left the new data separate; TestPolicy now includes
it in the permanent matrix. Workspace probes explicitly enable their
disposable go.work file for compilation, lint and inspection.

For the initial red evidence only, `T14_BASELINE=1 go test ./tools/testdata/architecture
-run TestPolicy -count=1 -v` judges the inherited linter alone. Missing graph enforcement
is reported as an expected-refusal assertion, never as a successful negative proof.

Compile-first mechanics are adapted from this project's AGPL-3.0 T-10 fixture harness
(f6f339c0acd6c0901ca7ea0a172edd0241d24ab2); no generator or generated policy is used.
The graph and contract patterns follow itos-cc ADR-0047/0051 and itos ADR-0053.
