# Slice-8 verification

Slice-8 implements local-first preflight and immutable frozen settings. It does not
start workloads, acquire images, create engine resources or establish isolation.

## Red-first evidence

The step commit `76688ab` kept all slice-8 scenarios `@wip`. Commit `c095115`
corrected the observer to lstat unsafe entries before reading them and retained the
shared diagnostic wording. With only `@wip` temporarily removed, the following
local selection ran against that pre-implementation code:

```sh
go test ./features -v -count=1 -scenarios='^@ID-RUN-(08|09|1[0-9])$'
```

All 70 examples failed at behavior assertions, with no undefined or setup failures:

| IDs | First failing assertion |
| --- | --- |
| RUN-08, RUN-10, RUN-11, RUN-12, RUN-17 | Required typed preflight problem/exit/subject |
| RUN-09 | Empty Engine API journal |
| RUN-13, RUN-14, RUN-19 | Exact protected snapshot exists |
| RUN-15 | Fresh B selection publishes and returns a durable handoff |
| RUN-16 | Published snapshot and handoff contain selected A |
| RUN-18 | Publication failure returns exit 75 and no usable handoff |

The sandbox maps `/tmp` ownership to UID 65534, which the existing protected-store
rules reject. The valid red run and subsequent disk checks ran outside that ownership
mapping, using disposable host-owned directories and synthetic Unix HTTP fixtures.
No real engine, Docker socket or operator approval state was used.

## Implementation checks

The run selection and ID-APPROVAL-26 pass with exact snapshot-only effects. Neighboring
local doctor and approval scenarios pass. Focused tests exercise the pinned local
checkpoint, local canonical comparison, frozen-envelope validation, no handoff after
refusal, memory/disk publication contracts, and filesystem publication/reuse faults.
The contract observer compares device/inode and modification time, excluding access
time as specified. Each process has its own API journal; concurrent requests are
attributed through Unix peer PID.

ID-RUN-15, ID-RUN-16 and ID-RUN-18 retain `@linux` on their ID tag lines; all slice-8
scenarios using the real disk store carry that tag. The spec already includes the
coordinator's distinction between empty pre-engine journals and inspection-only
engine-dependent refusals. Neither decision was reopened.

CI alone judges fresh changed-code coverage and mutation proof. There is no local
mutation result or runtime isolation evidence. The coordinating loop owns push, CI
watching and closure.
