# Slice-boundary fixtures

`tools/bin/check-slice-layout` runs the full checkout lint, then this executable
against the same generator and pinned depguard. `imports.json` describes compilable
probes materialised in disposable modules. Each negative requires the named importer,
the exact denied import and a depguard diagnostic. Compilation failures, configuration
errors, timeouts and other linters cannot satisfy it. Clean controls must have no issues.
Fixture modules reuse the project's dependencies; none run an engine or network workload.

The matrix covers domain I/O families, own and foreign adapters, command/UI imports,
cross-slice domains, adapter ports and external tests, rapid, nested packages, lookalike
names, a shared engine, domain-only and port-only slices, and a newly added slice and
adapter. A different fixture module path checks derivation from go.mod. Configuration
generation is repeated to check determinism; its output lives outside each source tree.

Coverage fixtures invoke a copy of the actual coverage script and discovery helper.
They require separate figures for count, inspect and inspect's nested domain; a 66.6%
inspect domain must fail beside fully covered domains. Untested adapter/helper/fake
statements do not change those figures. Empty discovery and a domain with no executable
statements must fail with their specific diagnostics.

## Recorded red run

At test commit `20ad60d`, before changing the rules, this command exited 1 with
46 failed assertions:

```sh
go run ./tools/testdata/slice-layout "$PWD" original
```

Representative results (the full matrix specifies all import paths):

| Probe | Importer and expected import | Original result |
| --- | --- | --- |
| `domain-os`, `domain-os-exec`, `domain-net`, `domain-net-http` | `internal/inspect/domain/probe.go`; `os`, `os/exec`, `net`, `net/http` independently | Expected depguard refusal; got none |
| `domain-own-adapter` | `internal/inspect/domain/probe.go`; `internal/inspect/socket` | Expected depguard refusal; got none |
| `adapter-own-domain` | `internal/count/input/probe.go`; `internal/count/domain` | Expected depguard refusal; got none |
| `handler-foreign-domain` | `internal/inspect/command/probe.go`; `internal/count/domain` | Expected depguard refusal; got none |
| `rapid-domain-test`, `rapid-nested-domain-test` | New domain test packages; `pgregory.net/rapid` | Unexpected `import 'pgregory.net/rapid' is not allowed from list 'rapid'` |
| `coverage-low`, `coverage-each` | Three separate domain packages | Only `./internal/count/domain 100.0%` printed; missing discovery |
| `coverage-empty` | No domain packages | Tried the nonexistent count domain instead of reporting empty discovery |
| `coverage-no-statements` | Additional empty domain | Only count measured; missing empty-domain refusal |

`preserve-count-os`, `preserve-count-net` and `preserve-disk-domain` already produced
their intended depguard diagnostics. They are preservation controls. The existing rapid
restrictions and eight clean controls also passed. No missing executable or undefined
scenario step was counted as red evidence.

After enforcement, all 56 import probes and four coverage probes pass through
`itos task T-10`, including the complete checkout lint. For example, `domain-own-adapter`
now reports `import 'github.com/donvargax/itos-container/internal/inspect/socket' is not
allowed from list 'internal/inspect/domain'` with depguard's port-boundary explanation.
