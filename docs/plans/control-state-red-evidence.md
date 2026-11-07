# Slice-21 red-first handoff

Coordinator q-31 (2026-11-07, approximately 16:25 UTC) accepted the red evidence
below and authorized implementation. All 45 examples use recording HTTP engine
fixtures, not `@engine`; no CI red run is required. The production change and
[architecture decision](../decisions/0019-control-state-publication.md) implement
the common account-home transaction, durable catalog and separation checks. All
45 examples now pass locally with `@wip` removed. CI proof and closure remain with
the coordinating loop; this adds no runtime-isolation claim.

## Executed red evidence

At `14d525c802fadf623910536efc3a1daba97908e1`, temporarily remove `@wip` from
this feature and run:

```sh
go test ./features -count=1 -run='TestControlStateTrace|TestFeatures' -scenarios='^@ID-CONTROL-'
```

All 45 examples failed at their first behavior assertion. All 97 setup/action
steps passed, 45 assertion steps failed and 142 later steps were skipped. There
were no undefined steps or setup failures. The tags were restored after the run;
no scenario text or expected behavior changed.

| IDs | Examples | First assertion failure |
| --- | ---: | --- |
| CONTROL-01–05 | 19 | Doctor/approve returned 0 instead of `approval.state.exposed`, exit 1. The worker/reservation checks succeeded before testing the refusal. |
| CONTROL-06 | 1 | The worker acquired admission while approval publication was held. |
| CONTROL-07 | 11 | Both commands returned 0 instead of the specified tracking refusal (exit 1 or 3). |
| CONTROL-08 | 4 | Approval returned 0 instead of `approval.state.write`, exit 75. |
| CONTROL-09 | 2 | No location binding matched the independently observed filesystem identity. |
| CONTROL-10 | 1 | A bootstrap directory was created before account-home exclusion. |
| CONTROL-11 | 7 | Approval returned 0 instead of `approval.state.write`, exit 75. |

`TestControlStateTrace` passed its positive observation and seven bootstrap-fault
controls in the same run. The parent-fsync fault requires an existing coordination
directory, keeping ordinary approval storage healthy on the baseline. Concurrent
processes have separate trace/PID filenames; CONTROL-06's earlier journal collision
was fixed before this final red run.

The following neighboring run also passed after the test changes:

```sh
go test ./features -count=1 -run='TestControlStateTrace|TestFeatures' -scenarios='^@ID-OVERLAP-(01|02|04|10|11|12)$'
```

Each test commit passed its configured hooks, including lint and vulnerability
checks. No local mutation run or generated coverage evidence was produced. The
filesystem scenarios ran outside the agent sandbox because its remapped ancestor
ownership would cause an unrelated protected-path refusal. All product commands
used disposable fixture state and recording Unix HTTP Engine API servers, never
a real daemon or the person's approval state.

## Test assembly and implementation

`cmd/itos-container/control_state_probe_test.go` supplies a temporary account-home
host port in the test executable. The production account-home port now consumes that observation. The same
isolated account lookup is supplied to the other approval, run and public
recovery test assemblies. `features/control_state_trace_test.go` extends the existing
child-only syscall observer to record directory creation, locks, publication and
fsync, and inject only the specified filesystem failures. No production testing
flag was introduced.

The binding oracle currently represents schema 1 with `uid`, `path`, `complete`
and ordered `ancestors` entries (`path`, `device`, `inode`). `self.json` binds the
fixed tree including its coordination-directory identity; other filenames hash
the JSON-encoded absolute managed path. The implementation uses this binding format and validates its identities,
required fields and unique keys. The behavioral scenarios remain authoritative.

Shared memory/disk contracts now cover read-only inspection, exposure, publication
failure/retry, retained protection, cross-root exclusion and replacement. Adapter
tests cover bootstrap filesystem failures, malformed bindings and account lookup.
The previously skipped positive controls, post-recovery publication, retry durability,
read-only inspection and no-rewrite observations now run successfully.

Neighboring fixtures now provide disposable account homes and independently seeded
catalogs when their premise is current approved session state. Multi-project fixtures
share one account home. CONTROL retains its explicit missing-catalog and legacy
fixtures. The plain diagnostic observer recognizes doctor's existing CLI prefix
without relaxing exact rule/message/fix checks. An occupied-public-ID fixture now
uses valid completed ownership, keeping its assertion focused on collision at
registration instead of malformed-state rejection during inspection.

No local mutation run or generated proof evidence is committed. The loop owns the
actual pushed-head CI proof, push, review and closure.

The final affected-scenario run passed all 560 selected examples (including all
45 CONTROL examples), with the observer controls included:

```sh
go test ./features -count=1 -run='TestControlStateTrace|TestFeatures' -scenarios='^@ID-(CONTROL|APPROVAL|OVERLAP|RUN|RECOVERY|DOCTOR)-'
```

Affected approval/session storage, domain, fake, account lookup and command-assembly
unit tests also passed. These local filesystem checks used unremapped host ownership
and exclusively disposable state plus recording HTTP engines. Configured gates run
through the feature commit's hooks; fresh changed-code proof remains CI-only.

## CI fixture regressions after integration

At `11b539f`, both doctor harness controls failed before reaching their assertions:
their worlds omitted the module root, so the isolated session-probe build searched
for `features/cmd/itos-container`. Each fixture now supplies `moduleRoot()` as the
scenario harness does. The journal and tree-change assertions are unchanged.

The catalog symlink fixture renamed its original binding to a `-old` filename
inside the catalog. This also introduced an unknown catalog entry. Depending on
filesystem directory order, inspection correctly refused that entry as
`state.invalid` before observing the symlink's `state.path`. The original fixture
failed 3 of 10 repetitions with temporary state on the checkout filesystem, while
40 repetitions on `/tmp`'s tmpfs passed. Move the symlink target outside the catalog
so this row exercises only substitution; the separate unknown-name row continues
to require `state.invalid`. Production refusal rules and assertions are unchanged.

After these fixture corrections, all `TestDoctor*` controls and all cases of
`TestControlBindingsRefuseMalformedOrReplacedEvidence` pass. The corrected symlink
case also passes 40 repetitions on the checkout filesystem. Filesystem checks use
unremapped ancestor ownership: the agent sandbox otherwise produces unrelated
`state.unsafe` refusals. All command state is disposable and engine observations
use recording HTTP fixtures. No real approval state, daemon or mutation proof is
used by this follow-up.
