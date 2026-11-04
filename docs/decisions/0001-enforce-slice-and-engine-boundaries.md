# Enforce slice and engine boundaries by package role

Accepted for foundation task T-14, 2026-11-04.

## Problem and decision

The inherited depguard rules named only count. New slices, ports and unclassified
packages could import I/O or another slice without a refusal. A generated list
would make enforcement depend on discovery and would still miss import-free
packages. The generic UI also depended directly on count's typed failures.

Supersede `docs/architecture.md`, "Slices", both its `internal/<command>` layout
and its adapter-only-port sentence. Commands use `internal/slices/<command>`;
shared engines use `internal/engine/<name>`. Static role globs apply to new names.
Adapters may use their own domain's values, avoiding needless mappers. Ports
retain their own values and import only ports; upstream permission for a port to
import a domain is deliberately not adopted. No generated assembly or DI framework
is introduced. Count's classifier moves to its command, supplied by cmd to the UI.

Depguard holds exact production pure-package allowlists and import categories.
A Go package-graph checker holds owner/layer arrows, rejects unclassified packages
and production test support, and inspects linked dependencies independently. It
checks real Go comments for depguard suppressions. Tests follow the project
arrows; only their standard-library helpers and eligible fakes gain allowances.
Generated files are subject to the same rules. Inspection errors cannot pass.

Shared Files contracts prevent the fake and real adapters drifting. Coverage
continues to hold each domain to 80%, discovering nested slice and engine domains
from Go packages. Compile-first fixtures prove allowed and refused arrows and
coverage failures. They run only when enforcement inputs change, selected from
staged paths or the CI range, including deleted and renamed paths.

## Sources and dependencies

The design adapts itos-cc ADR-0047 and ADR-0051, its architecture-check selectors
and FilesContract factory pattern, inspected at
`bcb081778820602cbc968f8759c7b5a2d0effdc6`; itos ADR-0053 and its static engine
rules were inspected at `8ee479efb43b940ad3729d72f8cb842d7aca36b7`.
No upstream legacy exceptions or generated assembly are copied.

Compile-first mechanics come from this project's AGPL-3.0 T-10 harness at
`f6f339c0acd6c0901ca7ea0a172edd0241d24ab2`; its generator and obsolete
adapter/domain prohibition do not land. The adapted deny-prefix test retains
itos-cc's AGPL-3.0 license, provided alongside it in
`tools/testdata/architecture/licenses/itos-cc.txt`. This does not relicense the
source projects or the product's existing AGPL-3.0 code.

Tooling parses YAML with the specified `go.yaml.in/yaml/v3 v3.0.5`, maintained by
the YAML organization, preserving its MIT/Apache notices in its module
redistribution. The reusable tooling import keeps the pin visible to `go mod tidy`.
This adds no runtime sandbox policy parser, service, linter or container library.

## Evidence

The initial test commit is `11e686a63125e21cac5108d58e849a7bee71da2e`.
Against inherited enforcement, 50 expected-refusal assertions failed after their
modules compiled: pure-domain exclusions, ports, ownership/layer arrows, UI and
leaf boundaries, unclassified packages, test support and suppressions. Two valid
nested domain rapid controls were also refused by the old rule. Existing product
scenarios remain preservation checks, unchanged. The full compile-first matrix
and task acceptance checks run through the configured hooks; mutation proof uses
the item's first-spec parent, not the take or implementation commit.

The initial matrix contains 132 import/layout controls, plus coverage, deny-prefix,
inspection, linked-support and disposable Git-selection tests. Follow-up red
probes caught two external-test self imports, three misplaced nested support
packages and two unresolved test dependencies that ordinary `go list` accepted.
The checker now validates test dependency loading as well as its import arrows.

Review added 125 compile-first controls. Before enforcement, 97 assertions failed
without infrastructure errors: command and porttest I/O, hidden import path
elements, ignored domain/port files, suppression spelling variants, unknown
top-level packages, and the coordinator-approved pure imports. The remaining
controls preserved existing allowances and refusals. Ignored files also compile
by explicit filename before inspection. Both data files now run in the normal
fixture matrix. Production command/porttest I/O uses the itos ADR-0053
`store-ports-io` pattern, extended to syscall, x/sys, ioutil, filepath and plugin.
The coordinator approved context, fmt, strconv, sort, slices, maps, unicode and
time contract types in domains/ports; their pure APIs complement bytes, errors
and strings. This supersedes the earlier three-package-only starting allowlist.

Four follow-up assertions reproduced wholly excluded domain/port packages, which
`go list ./...` omits entirely. A source-directory check now catches those as well
as reported `IgnoredGoFiles`. An excluded adapter helper remains permitted,
including `internal/engine/group/docker/domain`, which inherits the adapter role.
The lib fixture retains its os/exec import and cmd wiring; its assertion requires
the layout path/reason, not an unrelated import-rule diagnostic.

Three subsequent review threads added 18 compile-first fixtures. Against the
pre-fix checker, six trailing-comma suppressions escaped, eight build-constrained
porttest helpers were wrongly refused, and a valid two-module workspace failed
inspection. Three targeted/prose comment controls passed. The test-first commit
is `cea5b82`; the enforcement commit enables these fixtures in the normal matrix.
Suppression parsing now accepts a trailing comma. Both ignored-file checks use
the pure package role, preserving the distinct test-support role of porttest.
Module loading takes its identity from the packages inspected by `go list ./...`,
validates every package against it, and still resolves test dependencies; it no
longer assumes `go list -m -json` returns a single object in workspace mode.

T-13 supersedes the local snapshot proof. The merge from main removes all
`.metrics/mutate` snapshots, including those moved or added by T-14. Main's
`proof.code.check` runs a fresh seeded sample in CI and measures changed-line
coverage. Agents run no local mutation proof; the coordinating loop verifies
the actual pushed head. The old snapshots are not acceptance evidence.

The initial proof exposed assertion branches in `FilesContract` that conforming
adapters never exercised. Faulty port implementations now prove the suite detects
wrong bytes, errors and paths, repeated-read failures, and shared storage. A
recording reporter exercises those same assertions; the storage check mutates
every byte so partially overlapping returns also fail. These are tests of the
port's existing promise, not new counting behaviour.

`itos task T-14` passed after the final implementation commit: the combined
structural check, explicit fixture matrix, three `TestFilesContract` entry points,
per-domain coverage (count 100.0%) and the full `RAPID_CHECKS=1` product suite,
including unchanged black-box scenarios. Commit hooks passed lint and vulnerability
checks. The historical snapshot check is superseded by T-13's fresh CI proof.
Push and verification of the actual pushed head remain the coordinator's work.

<details>
<summary>Initial compiled fixtures: failing assertions before enforcement</summary>

Each module compiled first. These assertions failed against the inherited
linter; the final matrix passes their corresponding refusals and controls.

```text
expected refusal: internal/slices/newname/domain/probe.go imports os; rule domain; got none
expected refusal: internal/slices/newname/domain/probe.go imports os/exec; rule domain; got none
expected refusal: internal/slices/newname/domain/probe.go imports net; rule domain; got none
expected refusal: internal/slices/newname/domain/probe.go imports net/http; rule domain; got none
expected refusal: internal/slices/newname/domain/probe.go imports syscall; rule domain; got none
expected refusal: internal/slices/newname/domain/probe.go imports golang.org/x/sys/unix; rule domain; got none
expected refusal: internal/slices/newname/domain/probe.go imports encoding/json; rule domain; got none
expected refusal: internal/slices/newname/domain/probe.go imports flag; rule domain; got none
expected refusal: internal/slices/newname/domain/probe.go imports github.com/alecthomas/kong; rule domain; got none
expected refusal: internal/slices/newname/domain/nested/probe.go imports os; rule domain; got none
expected refusal: internal/slices/newname/port/probe.go imports os/exec; rule port; got none
expected refusal: internal/slices/newname/port/probe.go imports github.com/donvargax/itos-container/internal/slices/newname/domain; rule siblings; current linter produced []
expected refusal: internal/slices/newname/port/probe.go imports github.com/donvargax/itos-container/internal/slices/newname/portother; rule siblings; current linter produced []
expected refusal: internal/engine/shared/domain/nested/probe.go imports os; rule domain; got none
expected refusal: internal/engine/shared/port/probe.go imports os/exec; rule port; got none
expected refusal: internal/engine/shared/port/probe.go imports github.com/donvargax/itos-container/internal/engine/shared/domain; rule siblings; current linter produced []
expected refusal: internal/engine/shared/port/probe.go imports github.com/donvargax/itos-container/internal/engine/shared/portother; rule siblings; current linter produced []
expected refusal: internal/slices/count/domain/probe.go imports github.com/donvargax/itos-container/internal/slices/count/disk; rule siblings; current linter produced []
expected refusal: internal/slices/count/domain/probe.go imports github.com/donvargax/itos-container/internal/slices/countother/disk; rule siblings; current linter produced []
expected refusal: internal/slices/count/command/probe.go imports github.com/donvargax/itos-container/internal/slices/count/disk; rule siblings; current linter produced []
expected refusal: internal/slices/count/command/probe.go imports github.com/donvargax/itos-container/internal/slices/countother/disk; rule siblings; current linter produced []
expected refusal: internal/slices/count/domain/probe.go imports github.com/donvargax/itos-container/internal/slices/countother/domain; rule siblings; current linter produced []
expected refusal: internal/engine/shared/domain/probe.go imports github.com/donvargax/itos-container/internal/slices/count/domain; rule outside; current linter produced []
expected refusal: internal/slices/count/domain/probe_test.go imports github.com/donvargax/itos-container/internal/slices/countother/domain; rule siblings; current linter produced []
expected refusal: internal/engine/shared/domain/probe_test.go imports github.com/donvargax/itos-container/internal/slices/count/domain; rule outside; current linter produced []
expected refusal: internal/slices/count/domain/external_test.go imports github.com/donvargax/itos-container/internal/slices/countother/domain; rule siblings; current linter produced []
expected refusal: internal/engine/shared/domain/external_test.go imports github.com/donvargax/itos-container/internal/slices/count/domain; rule outside; current linter produced []
expected refusal: internal/slices/count/input/probe.go imports github.com/donvargax/itos-container/internal/slices/count/command; rule siblings; current linter produced []
expected refusal: internal/slices/count/input/probe.go imports github.com/donvargax/itos-container/internal/slices/count/disk; rule siblings; current linter produced []
expected refusal: internal/slices/count/disk/probe.go imports github.com/donvargax/itos-container/internal/slices/countother/domain; rule siblings; current linter produced []
expected refusal: internal/slices/count/disk/probe.go imports github.com/donvargax/itos-container/internal/cli; rule siblings; current linter produced []
expected refusal: internal/flat/probe.go imports ; rule unclassified; current linter produced []
expected refusal: internal/engine/shared/probe.go imports ; rule unclassified; current linter produced []
expected refusal: internal/engine/shared/adapters/disk/probe.go imports ; rule unclassified; current linter produced []
expected refusal: internal/engine/shared/command/probe.go imports ; rule unclassified; current linter produced []
expected refusal: internal/slices/count/probe.go imports ; rule unclassified; current linter produced []
expected refusal: pkg/public/probe.go imports ; rule unclassified; current linter produced []
expected refusal: internal/flat/probe.go imports os/exec; rule unclassified; got none
expected refusal: internal/cli/probe.go imports github.com/donvargax/itos-container/internal/slices/count/port; rule ui; got none
expected refusal: internal/cli/probe_test.go imports github.com/donvargax/itos-container/internal/slices/count/port; rule ui; got none
expected refusal: internal/version/probe.go imports github.com/donvargax/itos-container/internal/cli; rule version; got none
expected refusal: internal/release/probe.go imports github.com/donvargax/itos-container/internal/slices/count/port; rule release; got none
expected refusal: tools/probe/probe.go imports github.com/donvargax/itos-container/internal/version; rule toplevel; current linter produced []
expected refusal: internal/slices/count/domain/probe.go imports github.com/donvargax/itos-container/internal/slices/count/port/porttest; rule test support; current linter produced []
expected refusal: internal/engine/shared/domain/probe.go imports github.com/donvargax/itos-container/internal/engine/shared/port/porttest; rule test support; current linter produced []
expected refusal: internal/slices/count/domain/probe.go imports ; rule suppresses depguard; current linter produced []
expected refusal: internal/slices/count/domain/probe.go imports ; rule suppresses depguard; current linter produced []
expected refusal: internal/slices/count/domain/probe.go imports ; rule suppresses depguard; current linter produced []
expected refusal: internal/slices/count/domain/probe.go imports ; rule suppresses depguard; current linter produced []
expected refusal: internal/slices/count/domain/probe.go imports ; rule suppresses depguard; current linter produced []
allowed arrow refused: [{FromLinter:depguard Text:import 'pgregory.net/rapid' is not allowed from list 'rapid': rapid is used only by domain property tests Pos:{Filename:internal/slices/fresh/domain/nested/probe_test.go}}]
allowed arrow refused: [{FromLinter:depguard Text:import 'pgregory.net/rapid' is not allowed from list 'rapid': rapid is used only by domain property tests Pos:{Filename:internal/engine/fresh/domain/nested/probe_test.go}}]
```

</details>
