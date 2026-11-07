# Slice-21 red-first handoff

Slice-21 is **not implemented**. This branch contains committed step definitions
for all eleven IDs in [control-state.feature](../../features/control-state.feature),
with all scenarios still `@wip`. No production coordination, account-home lookup,
catalog, exposure refusal or bootstrap protocol has been added. There is no green
scenario evidence, CI result or runtime-isolation claim for this item.

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

## Test assembly and remaining work

`cmd/itos-container/control_state_probe_test.go` supplies a temporary account-home
host port in the test executable. That method is intentionally unused by the
baseline production code. The same host is supplied to the existing public
recovery test assembly. `features/control_state_trace_test.go` extends the existing
child-only syscall observer to record directory creation, locks, publication and
fsync, and inject only the specified filesystem failures. No production testing
flag was introduced.

The binding oracle currently represents schema 1 with `uid`, `path`, `complete`
and ordered `ancestors` entries (`path`, `device`, `inode`). `self.json` binds the
fixed tree including its coordination-directory identity; other filenames hash
the JSON-encoded absolute managed path. These are fixture protocol choices, not
an implemented storage format. The behavioral scenarios remain authoritative.

Implementation still needs the shared ports, I/O-free fake and disk contracts;
effective-account lookup; stable account-home descriptor exclusion; conservative
legacy reservation discovery; durable self-binding and pending/completed location
claims; and atomic approval/registration integration. Before wiring that lookup,
isolate the existing approval/run harnesses too: their child HOME values alone
must never select the person's real account-home anchor.

The later assertions have not yet run against an implementation. Validate their
positive controls and retry observations while implementing; do not treat skipped
steps as passing evidence. Complete green evidence, the required architecture
decision and arc42 updates, and the feature commit remain outstanding. The loop
still owns pushing, CI proof and item closure.
