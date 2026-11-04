# Slice-1 validation evidence

The strict offline policy contract is features/policy.feature, IDs @ID-POLICY-01 through
@ID-POLICY-28. This work proves policy inspection only; it supplies no engine or runtime
isolation evidence.

## Red before implementation

Commit 0f00c9d88156dbfa6f227a4ac1ad2f511470ae47 added the step definitions while all
policy scenarios remained @wip. With @wip removed locally, this command failed:

```sh
go test ./features -count=1 -scenarios='^@ID-POLICY-'
```

All 105 examples failed at behavior assertions; 328 setup/action steps passed,
105 assertion steps failed, and 165 later steps were skipped. No step was undefined.

- POLICY-01, 02, 03, 04, 22, 26 and 28 failed at the expected successful exit.
- POLICY-05 failed because doctor help did not contain --policy-only.
- POLICY-06 through 21, 23, 24 and 25 failed at the exact policy-problem assertion.
- POLICY-27 failed at the ordinary-doctor policy-problem assertion.

The old CLI returned a usage error for doctor. No scenario names or assertions were
changed. Only @wip tags were removed when the implementation made the scenarios live.

## Passing implementation

Commit 9f6703bab4a1d6a1920c5500666804fb32bab9b6 introduced the doctor slice, shared
policy engine, YAML adapter, schema and dependency notices. All 105 policy examples
passed. Focused domain, parser and UI tests passed, including exact fractional
version checks, control/drive path boundaries, structural traversal and diagnostics.
The schema JSON and its path-pattern boundaries were checked separately. An ECMAScript
check exposed a schema-only mismatch for dot components following Unicode line
separators. Using a match across all characters in that lookahead fixed it; 27 positive
and negative path cases passed under Node 26.13.0. The Go path validator already refused
those components.

The itos commit hooks ran the configured lint and vulnerability checks successfully.
Mutation baselines use all tests, including inherited CLI/count/completion scenarios.
No real engine was invoked; the only endpoint in policy fixtures is a loopback recorder.

Commit 1da87f4 completed the [YAML 1.2 core scalar rules](https://yaml.org/spec/1.2.2/#1032-tag-resolution).
The tests for +0x1 and -0o7 failed as numbers before the correction and passed as literal
strings afterward. Two mismatched collection-tag cases killed the only new survivors
from the initial mutation run. No new exceptions were added.

## T-14 integration

The saved layout amendment landed as 58a39ded899554390be4b96b0229946543b8e728.
Merge 26a34c54300fe4de35e63f54734a1376ba4b269e moved doctor to
`internal/slices/doctor` and reusable policy validation to `internal/engine/policy`.
It also moved policy presentation beside doctor, kept concrete assembly in `cmd`,
and added shared Files, Source and Policies contracts against their fakes and adapters.
The merge records policy scenarios passing before and after the move.

The implementer rechecked the integrated tree at
66417f09248746ca2d0295d1999a3addbc2f5d43 on 2026-11-04. These commands passed:

```sh
go test ./features -count=1 -scenarios='^@ID-POLICY-'
go test ./internal/engine/policy/... ./internal/slices/doctor/... -count=1
go test ./features -count=1
```

All 28 policy scenarios remain live with unchanged assertions and examples. The full
feature run also preserves count, CLI and completion behavior. The package run includes
the shared port contracts. This is preservation evidence, not a new red-first claim.
The policy stays closed, with no image or identity keys, using T-14's YAML v3.0.5 pin.

## Changed-code proof belongs to CI

Earlier commits recorded a local itos-cc 0.47.2 run with 154 mutants: 153 killed and
one pre-existing equivalent exception. Those results predate T-14 integration and the
current proof policy; they do not establish proof for the integrated head. The old
generated caches were removed during integration and must not be restored into CI.

The binding workflow is now `AGENTS.md` and `itos.yaml`'s `proof.code`: CI runs the
pinned tool with core itos supplying the same judged base to `--since` and `--seed`,
sampling up to ten changed-function mutation sites and measuring changed-line coverage.
Agents make no local mutation run and commit no generated mutation or coverage evidence.
The evidence-document correction changes no `proof.code` path. The coordinator still
owns push, CI verification of the actual pushed head and item closure.
