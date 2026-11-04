# Slice-2 validation evidence

This item implements the restricted manifest inspection contract in
`features/manifest.feature`, IDs `ID-MANIFEST-01` through `ID-MANIFEST-30`.
It adds no engine client, approval store or runtime isolation.

## Red before validation

Commit `ed3096023e52dd8083a20c77f599979b1f0f9259` added the step definitions,
isolated fixtures and test-only command assembly with all scenarios still `@wip`.
The first local run, with those tags removed, ran 260 examples and failed 260
assertions with no undefined steps. The file examples mostly received an unknown
`--manifest-only` option; those diagnostics are not claimed as validation evidence.
`ID-MANIFEST-28` failed at the help assertion requiring the new flag.

After declaring that flag, but before adding file validation, the same command
ran all 260 examples and failed at assertions, with 947 preceding steps passing
and no unknown-option diagnostics. The binary still returned its actual policy
inspection output. The 229 file examples established the following missing behavior:

| IDs | Failing assertion |
| --- | --- |
| 01, 03, 14, 18, 27, 28 | A valid manifest and its selected path must be reported. |
| 02, 04–13, 15–17, 19, 22, 23, 25 | A complete typed manifest refusal must identify the expected field and source. |
| 24 | Plain success must explain unused `customizations.vscode`. |
| 26 | Plain refusal must identify the selected manifest file. |

The metadata cases were then rerun separately after binding the actual file
validator, HuJSON/reference adapters, in-memory file port and doctor presenter in
the test-only `cmd/itos-container` assembly. The metadata entry point still returned
the file result without inspecting its supplied label. All 31 examples failed at
their metadata assertions; 128 preceding steps passed, with no undefined steps:

| IDs | Failing assertion |
| --- | --- |
| 20, 29 | `image_metadata` must record the supplied presence and unused pointers. |
| 21, 30 | Supplied malformed or operational metadata must produce a typed label refusal. |

The local commands were:

```sh
go test ./features -count=1 -scenarios='^@ID-MANIFEST-'
go test ./features -count=1 -scenarios='^@ID-MANIFEST-(20|21|29|30)$'
```

The file validator was implemented after its red run, and metadata validation after
its separate red run. No scenario assertions or example values were changed to fit
the implementation. All 30 manifest scenario IDs are now live.

## Passing checks and limits

All 260 manifest examples passed. The shared Files, Parser, References and doctor
Manifests contracts run against their memory-only fakes and actual adapters. Focused
tests cover policy short-circuiting, selected paths, literal settings, independent
problems, metadata presence, no merging, and terminal-safe diagnostics.

`go test ./... -count=1` passed, including all 432 live non-engine feature examples
and the inherited count, policy, CLI, completion and harness regressions. Its first
run caught a help-wording regression; the implementation retained the existing policy
exit descriptions and added the manifest meaning alongside them.

The specification explicitly supersedes `ID-POLICY-34`'s ordinary-doctor success
example. Its obsolete empty-mode row was removed; the policy-only terminal-escaping
and JSON-preservation row remains unchanged. A command assembly regression now proves
ordinary doctor refuses a missing selected manifest, and `ID-MANIFEST-10` proves it
also refuses a manifest with no image. Use `--policy-only` for the former scope.

No real engine or local mutation run was used. Fresh changed-line coverage and
sampled mutation proof remain CI's responsibility at the actual pushed head. Slice-3
must obtain image metadata and distinguish lookup failure from confirmed absence;
slice-4 owns image/UID approval comparisons. Slice-5 already records the outstanding
aggregate resource budget, including the manifest parser. This item invents no
resource limit or host-support claim.
