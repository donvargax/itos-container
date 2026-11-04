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

## Changed-code proof

Use f21767fae205d4a0ceed4ad70873a9f062eb1e1f, the parent of the item's actual first
commit 8f6fd61 (promotion), as the base. This includes the scenario specification
bcf219b and all subsequent implementation; no take commit is assumed.

```sh
tools/bin/pinned itos-cc mutation run \
  --since f21767fae205d4a0ceed4ad70873a9f062eb1e1f \
  --all-tests --fail-uncovered --no-annotate --workers 4 --json cmd internal
```

The pinned tool is itos-cc 0.47.2. The final run exited 0 with ok true: 154 mutants
ran freshly (zero reused), 153 were killed, one was excepted, zero survived and zero
were uncovered. No uncovered executable code or other problems were reported. The
full-suite coverage and baseline runs passed, including all live policy scenarios and
the inherited counting, CLI and completion scenarios.

The one exception predates this item: main replaces empty argv with --help before
checking len(args) > 0, making >= 0 equivalent there. The tool renewed only its function
hash after adding doctor; it retains the same reason in itos-cc.yaml for human review.

Generated project-local evidence is committed under .metrics/mutate/ by explicit path.
No evidence was hand-edited or copied from another project. No source annotations were
written. CI and the final push remain the coordinator's responsibility.
