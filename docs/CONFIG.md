# Configuration format guidelines

These are format and validation requirements for the planned `itos-container.yaml`
sandbox policy. The [charter](charter.md) selects the milestone from the binding
[pilot contract](plans/container-pilot.md); they take precedence over upstream
examples. Strict policy validation is implemented through `doctor --policy-only`;
the inherited count, version and completion commands remain. Manifest, host readiness,
approval and runtime isolation are not implemented by this policy slice.

The milestone is an offline owned run with no network, an approved hardened container,
the checkout as its only host mount, a non-root identity and owned cleanup. Linux amd64
with native Docker is a target requiring passing runtime evidence, not a supported-host
claim. Network egress, credentials/accounts, network Git, LAN ingress, Compose and
other platforms remain deferred. Describing the wider pilot does not authorize its
later slices or settle an open security decision. Real-engine scenarios run only in CI.

Files owned by other tools retain their formats: `itos.yaml` configures core itos's
workflow, and the restricted `devcontainer.json` is JSON-with-comments, not YAML.
Container policy keys do not belong in `itos.yaml`. The host's `ITOS_CONFIG` names core
configuration, never the sandbox policy; [CLI.md](CLI.md) describes the extension seam
and the limits on flags and environment settings.

## Sources and comparison

Compared on **2026-11-04**, using each inspected main revision and the last commit
that changed each guideline, not the arrival date of a local copy:

| Project and inspected revision | Pinned sources | Last guideline changes (UTC) |
| --- | --- | --- |
| itos, `772cef549d90b9cca7170a2252ad85fc09bd880a` | [CLI](https://github.com/donvargax/itos/blob/772cef549d90b9cca7170a2252ad85fc09bd880a/docs/CLI.md), [extension protocol](https://github.com/donvargax/itos/blob/772cef549d90b9cca7170a2252ad85fc09bd880a/docs/extensions.md) | CLI: `50dfa0bcc8c0632aaa0d0041206e2c97cf9ae24f`, 2026-10-27 03:01:23. No CONFIG.md at this revision. |
| itos-cc, `9361616f7204a236e809417726ad107ca4f9b805` | [CLI](https://github.com/donvargax/itos-cc/blob/9361616f7204a236e809417726ad107ca4f9b805/docs/CLI.md), [CONFIG](https://github.com/donvargax/itos-cc/blob/9361616f7204a236e809417726ad107ca4f9b805/docs/CONFIG.md) | CLI: `8a3f31cd899be858fa7eaea02ce9f05572d54346`, 2026-10-27 10:10:36. CONFIG: `405b36205f8a38a27260f69afe554412015077ec`, 2026-10-27 06:39:05. |
| itos-template, `1533aee2ea3331816e793faaeaabd3f55f83bcaa` | [CLI](https://github.com/donvargax/itos-template/blob/1533aee2ea3331816e793faaeaabd3f55f83bcaa/docs/CLI.md), [CONFIG](https://github.com/donvargax/itos-template/blob/1533aee2ea3331816e793faaeaabd3f55f83bcaa/docs/CONFIG.md) | CLI: `79301eba51b9768a27906c720e39c49b08491e67`, 2026-10-12. CONFIG: `2d8bc3da85fd0aa8b6cc86f2b40b526c9726ffd3`, 2026-10-07. |

itos-cc has the newest edits. Adopt its separation of format policy from implemented
loader behaviour and retain strict JSON-data validation. Its loader's compatibility
exceptions are project-specific, not sandbox permissions. The inherited CONFIG.md
matched the template copy; CLI.md retained older render/merge guidance and lacked the
template's later pre-v1 compatibility rule. Upstream decision numbers and implementation
gaps are not local specifications or evidence.

These rules retain donvargax's attribution and the owner's configuration principles,
carried through itos-template and itos-cc. The owner permitted reuse of itos and
itos-template infrastructure here under AGPL-3.0; preserve original and dependency licenses
and notices. See [LICENSE](../LICENSE) and [CONTRIBUTORS.md](../CONTRIBUTORS.md).

| Key | External guideline source |
| --- | --- |
| K8S | [Kubernetes API Conventions](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md), including lists of named objects and primitive types. |
| AIP | [Google API Improvement Proposals](https://google.aip.dev), especially 126 (enums), 140 (names), 180 (compatibility) and 193 (errors). |
| CLIG | [Command Line Interface Guidelines: configuration](https://clig.dev/#configuration). |
| JSON | [RFC 8259](https://www.rfc-editor.org/rfc/rfc8259), the JSON data model. |
| YAML | [YAML 1.2.2](https://yaml.org/spec/1.2.2/), data syntax subject to the restrictions below. |
| SCHEMA | [JSON Schema](https://json-schema.org), for a versioned editor and producer contract. |
| OWNER | The owner's visible-defaults, explicit-values and no-embedded-language principles, attributed in the pinned upstream guides above. |

## Requests, approval and frozen sessions

Treat the checkout policy and restricted manifest as **untrusted requests**. Neither
authorizes itself. Keep these project requests under version control; approval and
session records belong outside the checkout. `doctor`, `approve` and `run` must share
strict parsing, validation and effective-value resolution without executing checkout
code or repository helpers.
Host-user approval is explicit, held in host-owned state outside the writable checkout,
and covers the reviewed effective policy and supported manifest settings. Include
effective defaults and applicable differences from prior approval in that review.

`run` must refuse missing or malformed policy, unsupported options, invalid values and
absent, mismatched or superseded approval before starting resources. Missing policy or
manifest must never fall back to unrestricted startup. A valid file alone is not an
approval. Use a frozen effective session configuration in trusted material the worker
cannot alter; do not re-read newly edited values as authorization during startup.
Editing checkout files or recording another approval cannot widen a live session.
New permissions require a new approved run after stopping the old session.

The restricted manifest describes one image-based worker under the same boundary.
It cannot authorize host commands, builds, Features, Compose services, extra mounts,
runtime security flags, host environment interpolation or credential sourcing. It
cannot configure or replace the trusted guard/proxy or independently publish ports.
Image metadata must not introduce unreviewed commands, identity or permissions.
Exact supported fields, JSON-with-comments syntax, metadata treatment and in-container
lifecycle semantics need specifications; no Dev Containers CLI fallback is authorized.

## Format policy

The requirements below guide behaviour specifications. Slice-1 selects the maintained
`go.yaml.in/yaml/v3 v3.0.5` node parser, pinned in go.mod/go.sum, for structural validation
before projection. Its YAML 1.1 scalar extensions are constrained to YAML 1.2 core
meanings. Its MIT/Apache notices are preserved in THIRD_PARTY_NOTICES.md and releases.
Manifest parsing remains a separate specification.

### Implemented version 1

`doctor --policy-only` reads exactly `itos-container.yaml` in the effective working
directory, without searching parents or using `ITOS_CONFIG`. The policy must be a
regular file of at most 65536 bytes (64 KiB). The reader opens without following
the final symbolic link, checks the opened file, and bounds the actual read even
if it grows. Symbolic links (including dangling links), non-regular files and
oversized input report `policy.unreadable` with exit 3 before parsing. This does
not establish checkout identity or approval-time substitution protection.

Before YAML decoding, source lines beginning with `%`, optionally after a
byte-order mark, are refused with one root `policy.syntax` problem and exit 2.
This reserves directive lines, including `%TAG` and `%YAML`, so a tag prefix
cannot be expanded into every node before the representation limits run. The
check covers the entire input, including later documents and all YAML line
breaks (LF, CR, CRLF, NEL, LS and PS). The restriction also applies to multiline
quoted scalars: indent a continuation line containing literal percent text.
Inline and indented percent text and permitted built-in tags remain supported.

After YAML syntax and single-document checks, a preflight limits the decoded
representation to 32 levels and 2048 nodes. The root is depth 1; mapping keys,
mapping values and sequence elements each count as nodes one level below their
parent. Alias references count once without following their targets; the document
wrapper does not count. Either excess reports one `policy.complexity` problem at
the root with exit 2, before diagnostic collection or value conversion. This
bounds validation of hostile data, including custom tags below unknown fields.

Successful human output escapes manifest controls, quotes and Unicode line
separators using Go string escapes without outer quotes. Printable Unicode stays
readable; JSON retains the original decoded manifest value. The
[versioned schema](schemas/itos-container-v1.schema.json) describes the closed data
contract; the YAML restrictions below also apply. The complete behaviour is specified
in [policy.feature](../features/policy.feature).

| Key | Type and requirement | Default |
| --- | --- | --- |
| version | Required integer, exactly 1; integral numeric spellings are equivalent | None |
| manifest | Required nonempty literal relative path, such as .devcontainer/devcontainer.json | None |
| network | Optional closed object containing only mode | {mode: offline} |
| network.mode | Optional string, exactly offline | offline |

Null never means omission. Paths use slash separators and preserve spaces and Unicode.
Absolute or drive-qualified paths, backslashes, empty/dot/dot-dot components and ASCII
controls are refused. There is no trimming, expansion or referenced-file read in policy
inspection. Structural errors prevent schema projection; an invalid version prevents
interpreting other fields. Independent version 1 errors are collected. No partial
effective policy is returned. Safe filesystem identity belongs to the later host slice.

### Data and validation

1. Use JSON's data model: objects, arrays, strings, numbers, booleans and null, with
   YAML as the policy file's syntax (JSON, YAML). Quotes, comments and block/flow styles
   are data spellings. The non-specific `!` tag is allowed as a data spelling, read as
   if absent; it builds no object and expands nothing.
2. Reject duplicate keys, unknown keys, non-string keys, multiple documents, anchors,
   aliases, merge keys and non-JSON/custom tags before using configuration or starting
   resources. Apply these checks throughout nested mappings and lists as well as at
   the root. JSON-type tags may only express JSON values; no tag can construct an
   arbitrary object. A permissive decoder that silently drops keys is insufficient.
3. Validate every value before using any: types, accepted values, references, patterns
   and defaults must all satisfy the specified schema. Refuse unsupported options
   rather than ignore them or substitute a weaker runtime (K8S, OWNER).
4. Report actionable problems with the key's path from the top, including list indices
   where relevant. Collect validation problems where possible so authors can correct
   them together; an unreadable or malformed document may prevent further validation
   (AIP 193). Exact rule IDs and diagnostics belong to later behaviour specifications;
   their JSON/exit mapping stays in the UI as [CLI.md](CLI.md) requires.
5. Make every default explicit and visible in documentation and the reviewed effective
   configuration. Values come from written data or documented defaults, not guesses
   about the host. No default can approve a permission (OWNER, CLIG).
6. Embed no language, interpolation engine, shell evaluation or host-side setup in the
   policy reader. Authors may generate plain JSON data themselves; the controller
   reads that output, never executes a checkout generator. A data spelling is not
   permission to run a command (OWNER).

The newer itos-cc loader accepts ignored `x-` keys, treats absent/empty files as empty
configuration, reads a missing `version` as version 1 and adds keys within version 1
before its 1.0 release. **None of those exceptions is adopted here.** Unknown sandbox
keys remain errors; an absent or empty policy cannot start a session, and the version
must be explicit. Its shell-string test commands and environment inheritance likewise
do not establish sandbox argv or credential rules.

### Versions and shape

7. Begin the policy with an integer `version` and reject unsupported versions instead
   of guessing their meaning. A new key needs a format version that defines it;
   retained earlier versions must keep their written meaning. Removing or renaming a
   key, or changing a value's meaning, is a breaking change requiring migration
   guidance and the release treatment in [CLI.md](CLI.md) (K8S, AIP 180). The initial
   policy/schema version is assigned by slice-1 above.
8. Use `snake_case` keys consistently. Represent ordered named collections as lists
   of objects with names rather than maps keyed by name, so order and object shape
   remain explicit (AIP 140, K8S).
9. Prefer an enum string over a boolean when another state may be needed, and a list
   over a scalar when multiple values may be needed (K8S, AIP 126).
10. Keep values whole: commands use argv lists, never strings to split or implicitly
    evaluate with a shell. This principle does not select lifecycle hooks or authorize
    host command execution.
11. Express paths relative to the configuration file with `/` separators. This is
    representation guidance, not a decision about safe checkout identity, symlinks,
    manifest discovery, output locations or host-state storage. Those require the
    pilot's security specifications before implementation.
12. Keep secrets out of repository configuration, examples, generated files and
    diagnostics. General advice about environment variables or secret stores does
    not authorize host credential sourcing or forwarding. Credentials/accounts are
    outside the offline milestone; later work needs its own reviewed contract.

### Documentation, defaults and writing

13. Document every supported key's type, requirements, default and example before
    implementing it. Examples must reflect specified behaviour; this page provides
    the settled version 1 catalog above (OWNER).
14. Make the effective configuration, including all defaults, reviewable without
    executing checkout code or revealing secrets. This is a required capability for
    doctor/approval design; policy-only inspection shows effective defaults (OWNER, CLIG).
15. Publish a JSON Schema versioned with the settled format so editors and producers
    can validate it (SCHEMA). Version 1 is linked above.
    Schema validation alone cannot establish host approval or runtime isolation.
16. Make generated writes stable and deterministic, in the format's defined key
    order, one key per line, using only the permitted JSON-compatible data. Preserve
    meaning on read-back and avoid unrelated diff churn. `init` must not overwrite
    existing files; its non-runnable examples and paths still need specifications.

## Remaining specification work

The pilot leaves approval canonicalization/fingerprint and
storage, safe checkout/engine identity, manifest discovery/output paths, reserved
environment names, image metadata, lifecycle hooks, resource/time limits and
cancellation details open. Resolve them through the assigned specifications and the
person's security decisions, not an illustrative configuration here.

Format policy is not loader compliance. Implement each required behaviour with
black-box scenarios and red-first step definitions; use domain-owned ports and fakes,
and run real-engine scenarios only in CI. The policy slice
supplies no runtime isolation evidence.
