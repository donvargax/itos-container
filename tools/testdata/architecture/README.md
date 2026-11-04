# Architecture fixtures

Compile every disposable module, including tests, before checking the pinned depguard
policy and package graph. Import refusals name the importer, dependency and rule; tool
errors never satisfy a negative test. Run through `tools/bin/architecture-check --fixtures`.
The matrix stays under testdata so ordinary `go test ./...` does not launch the linter.

`review-cases.json` specifies T-14's follow-up refusals and approved pure imports.
For red evidence, append these cases to `cases.json` temporarily and run TestPolicy
with the current compiled checker, then restore the original matrix. The enforcement
commit connects both files to the normal fixture run. This records compile-valid
expectations before changing either the linter policy or the graph checker.

For the initial red evidence only, `T14_BASELINE=1 go test ./tools/testdata/architecture
-run TestPolicy -count=1 -v` judges the inherited linter alone. Missing graph enforcement
is reported as an expected-refusal assertion, never as a successful negative proof.

Compile-first mechanics are adapted from this project's AGPL-3.0 T-10 fixture harness
(f6f339c0acd6c0901ca7ea0a172edd0241d24ab2); no generator or generated policy is used.
The graph and contract patterns follow itos-cc ADR-0047/0051 and itos ADR-0053.
