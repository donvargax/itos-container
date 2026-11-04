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
slice-4 owns image/UID approval comparisons. The later manifest security repair below
adds inspection budgets; runtime resource quotas remain outside this item. No
host-support claim is made.

## Per-ID coverage preparation repair

The first CI proof stopped before judging a mutant because `ID-MANIFEST-29`
exported no per-ID coverage. The metadata scenarios' separate `go test -c`
assembly lacked the coverage flags already used by the product binary. It now
uses `-cover -covermode=atomic -coverpkg=./...` when `GOCOVERDIR` is set. Go's
exit hooks export the counters on both successful and refused metadata inputs;
the ordinary build and presenter output are unchanged.

`TestManifestContractCoverage` runs each of the four metadata IDs separately,
converts its exported counters with `go tool covdata textfmt`, and requires
executed product blocks in the metadata domain, JSON parser, reference validator
and doctor presenter. Before the repair, all four IDs passed their scenarios but
failed every coverage assertion. Afterward, the regression passed, including
JSON/plain output and refusal exits. Its temporary coverage files are not committed.
Filtered scenario invocations skip this whole-suite regression so unrelated
scenarios cannot contribute to a selected ID's mutation result.

The focused regression and `go test ./features ./cmd/itos-container -count=1`
passed locally. A separate filtered `ID-MANIFEST-29` run with `GOCOVERDIR` set
also exported executed blocks in all four product components. This is a
test-harness repair for slice-2, with no changed product
behavior or scenario expectations. No local mutation run was made; the coordinator
must rerun the full changed-code proof in CI at the new pushed head.

## Bounded manifest inspection security repair

Review thread `PRRT_kwDOVCoNUM6z1uOX` identified the unbounded regular-file read
and recursive JSON representation. Commit `a48f82f` first committed scenarios
`ID-MANIFEST-31` through `ID-MANIFEST-33` and their raw-input steps with `@wip`.
With those tags removed, the following run failed all 13 examples at the typed
refusal assertion, after 39 setup/execution steps passed and with no undefined steps:

```sh
go test ./features -count=1 -scenarios='^@ID-MANIFEST-(31|32|33)$'
```

The oversized, deep, wide and long-path complete inputs returned valid settings
instead of a refusal. The unfinished deep manifest returned `manifest.syntax`
instead of the required early `manifest.complexity` refusal. No scenario assertion
or input changed after that run; the implementation followed the new limits.

The reader now consumes at most 65537 bytes from its already checked descriptor,
returning no data when the 65536-byte limit is exceeded. A nonrecursive token
preflight bounds both parser modes to depth 32 and 2048 nodes before HuJSON builds
its recursive tree. A tree preflight limits aggregate escaped path lengths to
65536 bytes before conversion and duplicate diagnostics. Supplied metadata also
has its byte limit checked before copying into parser bytes. Existing schema,
image/UID requests and metadata provenance remain unchanged.

All 13 new examples pass within the complete live non-engine suite: 460 scenarios
and 2992 steps passed. Manifest engine, doctor slice and command assembly package
tests pass. Tests cover exact acceptance/rejection boundaries, read errors and
a reader that continually grows (only 65537 bytes consumed), literal/comment
punctuation, unfinished input, object keys, escaped paths and array paths. Files
and Parser shared contracts cover the budgets against the fake and real adapters.

The reported CI preparation failure at `6dce7f6` predates the existing repair
`7487e17`. Its `TestManifestContractCoverage` regression passed again before this
repair and now includes the new metadata budget ID. All five metadata IDs export
executed validator, parser, reference and presenter blocks independently. No
local mutation run or generated proof artifact is included; the outer loop must
run fresh changed-code proof in CI at the pushed head.
