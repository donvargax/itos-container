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
