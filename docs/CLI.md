# CLI design guidelines

These rules describe the planned `itos container` extension. The [charter](charter.md)
selects the milestone from the binding [pilot contract](plans/container-pilot.md); both
take precedence over upstream examples. The current [entry point](../cmd/itos-container/main.go)
exposes strict policy validation through `doctor --policy-only` and invocation refusals
through `run`, alongside the inherited count, version and completion commands. Runtime
isolation is not implemented.

The first milestone is an offline owned run: explicit argv in an approved, hardened
container with no network, the checkout as its only host mount, a non-root identity
and owned resources cleaned up on exit. Linux amd64 with native Docker is the target,
requiring passing runtime evidence before any support claim. Network egress,
credentials/accounts, network Git, LAN ingress, Compose and other platforms are
deferred. Describing later pilot commands here does not authorize their implementation
or settle an open security decision. Real-engine scenarios run only in CI; agents
never receive the Docker socket.

## Sources and comparison

Compared on **2026-11-04**, using each inspected main revision and the last commit
that changed each guideline, not the date a copy arrived here:

| Project and inspected revision | Pinned sources | Last guideline changes (UTC) |
| --- | --- | --- |
| itos, `772cef549d90b9cca7170a2252ad85fc09bd880a` | [CLI](https://github.com/donvargax/itos/blob/772cef549d90b9cca7170a2252ad85fc09bd880a/docs/CLI.md), [extension protocol](https://github.com/donvargax/itos/blob/772cef549d90b9cca7170a2252ad85fc09bd880a/docs/extensions.md) | CLI: `50dfa0bcc8c0632aaa0d0041206e2c97cf9ae24f`, 2026-10-27 03:01:23. No CONFIG.md at this revision. |
| itos-cc, `9361616f7204a236e809417726ad107ca4f9b805` | [CLI](https://github.com/donvargax/itos-cc/blob/9361616f7204a236e809417726ad107ca4f9b805/docs/CLI.md), [CONFIG](https://github.com/donvargax/itos-cc/blob/9361616f7204a236e809417726ad107ca4f9b805/docs/CONFIG.md) | CLI: `8a3f31cd899be858fa7eaea02ce9f05572d54346`, 2026-10-27 10:10:36. CONFIG: `405b36205f8a38a27260f69afe554412015077ec`, 2026-10-27 06:39:05. |
| itos-template, `1533aee2ea3331816e793faaeaabd3f55f83bcaa` | [CLI](https://github.com/donvargax/itos-template/blob/1533aee2ea3331816e793faaeaabd3f55f83bcaa/docs/CLI.md), [CONFIG](https://github.com/donvargax/itos-template/blob/1533aee2ea3331816e793faaeaabd3f55f83bcaa/docs/CONFIG.md) | CLI: `79301eba51b9768a27906c720e39c49b08491e67`, 2026-10-12. CONFIG: `2d8bc3da85fd0aa8b6cc86f2b40b526c9726ffd3`, 2026-10-07. |

itos-cc has the newest guideline edits. Adopt its clearer distinction between design
policy and implemented behaviour, and its typed problems with stable rule IDs, per-problem
exit codes and flat subject fields. Its exit-code choices and command-specific exceptions
remain its own. The inherited CONFIG.md matched the template copy; the inherited CLI.md
still used render/merge examples and lacked that copy's later pre-v1 compatibility rule.
Neither upstream's implementation gaps nor its numbered decisions become local facts.

These guidelines retain attribution to donvargax and the owner's permitted reuse of
itos and itos-template infrastructure. Preserve the original projects' and dependencies'
licenses and notices; see [LICENSE](../LICENSE) and [CONTRIBUTORS.md](../CONTRIBUTORS.md).

| Key | External guideline source |
| --- | --- |
| CLIG | [Command Line Interface Guidelines](https://clig.dev), especially help, naming, arguments and flags, output, errors, configuration and future-proofing. |
| GNU-CLI | [Standards for Command Line Interfaces](https://www.gnu.org/prep/standards/html_node/Command_002dLine-Interfaces.html). |
| GNU-VER | [Version output](https://www.gnu.org/prep/standards/html_node/_002d_002dversion.html). |
| GNU-HELP | [Help output](https://www.gnu.org/prep/standards/html_node/_002d_002dhelp.html). |
| GNU-ERR | [Formatting Error Messages](https://www.gnu.org/prep/standards/html_node/Errors.html). |
| POSIX | [POSIX.1-2024 Utility Syntax Guidelines](https://pubs.opengroup.org/onlinepubs/9799919799/basedefs/V1_chap12.html#tag_12_02). |
| COBRA | [Cobra command concepts](https://github.com/spf13/cobra/blob/main/README.md#concepts), as naming guidance, not a parser selection. |
| PROBLEMS | [RFC 9457](https://www.rfc-editor.org/rfc/rfc9457), the model for per-problem status and extension members used by itos-cc. This is a CLI object, not an HTTP response. |

## Extension dispatch and environment

Core itos's existing extension seam finds `itos-container` on `PATH` for
`itos container`, passes the remaining arguments unread and returns the extension's
exit status. A built-in command wins over an extension; dispatch alone supplies no
isolation. `itos help container` dispatches `itos-container --help`.

Flags before `container` belong to itos. Arguments after it belong to the extension.
After the `--` of `run` or `exec`, every word belongs to the workload: no further wrapper
flag parsing, expansion, joining into a shell string or injected agent permission flags.

The host protocol supplies the following to the extension/controller, not to the worker:

| Variable | Host protocol meaning |
| --- | --- |
| `ITOS_ROOT` | Absolute effective working directory after an itos `--root` before `container`; the extension runs in that directory. This alone does not establish safe checkout identity. |
| `ITOS_CONFIG` | Absolute core itos configuration path, which may not exist. It never names `itos-container.yaml`. Container keys do not belong in `itos.yaml`. |
| `ITOS_JSON` | `1` when itos received `--json` before `container`, otherwise unset by the host protocol. Planned control commands honor it as a JSON request. |
| `ITOS_BIN` | The invoking itos executable for callbacks. |
| `ITOS_VERSION` | That executable's version; the itos launcher uses it to keep callbacks on the same version. |

Core dispatch inherits the host environment. That is not permission to import it into
the worker. Proposed product-specific settings use the `ITOS_CONTAINER_` prefix, in
uppercase with underscores. This naming rule defines no new variable or flag. Where a
future specification allows a setting through several interfaces, its precedence is
flag, environment, then file (CLIG). No override can grant permissions, bypass host-user
approval or widen a running session. See [configuration boundaries](CONFIG.md).

## Planned command contract

The full commands in this table remain planned. Doctor currently validates only policy,
including when called without `--policy-only`; its output names that limited scope.
Run currently validates the invocation and refuses startup as described below.
The charter determines which slices may be built next.

| Command after `itos container` | Required behaviour |
| --- | --- |
| `init` | Prepare non-runnable policy and single-worker manifest examples without overwriting existing files. Output paths remain unspecified. |
| `doctor` | Inspect strict policy, the restricted manifest, selected local engine, cached images/metadata, checkout layout, identity, preparation and approval. Inspect only; do not execute checkout code or change resources. |
| `approve` | Show effective permissions, differences from prior approval and applicable weaker-mode warnings; record explicit host-user consent in host-owned state outside the writable checkout. Approval cannot start a session. |
| `run -- <argv...>` | Require an explicit command, validate matching approval and start an approved foreground session with bounded startup. Preserve every argument boundary; use no implicit shell. |
| `exec <session> -- <argv...>` | Run an explicit command only in a live owned session, under its frozen policy, identity, mounts and restrictions. Never extend its foreground lifetime. |
| `status` | Report owned sessions, state, effective policy identity and recovery information without secrets. |
| `stop <session>` | Stop and remove only the named session's owned resources, preserving checkout edits and project state. Finish interrupted cleanup without pruning foreign resources. |

`init`, ordinary `doctor` and `approve` never execute checkout code, install hooks, log
in, pull/build images or start a session. Host inspection must not invoke
repository-configured helpers. `run` must refuse missing, incompatible or unapproved
cached image content; it cannot pull/build an image as a fallback.

The pilot separately proposes `doctor --probe`: an explicit request for bounded,
disposable runtime probes using trusted test material, with no agent or real credentials
and no checkout-supplied scripts. It is not part of ordinary inspection, and its
real-engine scenarios run only in CI.

There is no detached or keep-alive mode. Primary workload exit refuses new `exec`
requests and tears down active commands and session resources. Owned recovery records
support inspection and cleanup, never an unattended agent restart. Record ownership
before workload execution; ambiguous or foreign resources cannot be removed merely
because their labels resemble an owned session. Cleanup preserves checkout edits and
project state; exact recovery storage and retry details still need specifications.

## Implemented policy inspection

`itos container doctor --policy-only` validates `itos-container.yaml` in the effective
working directory without executing helpers, contacting an engine, reading the manifest
or changing checkout/host state. Ordinary doctor applies the same policy validation;
manifest, host readiness and approval checks remain for later slices.

`--json` or `ITOS_JSON=1` prints exactly one object; `--no-json` overrides that environment.
The object contains `schema: 1`, `ok`, `scope: "policy"` and `problems`. Success includes
`policy` with all effective defaults; failure omits it. Every problem has `rule`, `exit`,
`message`, `fix`, `file: "itos-container.yaml"` and an RFC 6901 `path` (empty at the root).
Rule IDs are specified in [policy.feature](../features/policy.feature). Exit 0 means valid
policy, 2 means invalid policy/usage, and 3 means unreadable policy. Multiple problems
use the highest exit. Diagnostics never include source lines, raw parser errors or
rejected values. Valid policy is neither host readiness nor approval.

## Implemented run refusals

`itos container run -- <command> [arguments...]` requires a nonempty executable after
the first separator. Missing or empty commands and commands supplied without `--`
exit 2 with invocation guidance on stderr and no stdout. Words after the separator
belong only to the workload, including `--json`, `--help`, empty strings and further
separators; they cannot enable or cancel wrapper JSON.

A wrapper `--json` before or after `run`, or inherited `ITOS_JSON=1`, takes precedence
over missing-command errors. Run emits exactly one schema-1 object with `ok: false`
and one `run.json-unsupported` problem containing `exit: 2`, a message and guidance
to remove both requests. The diagnostic also goes to stderr. Terminal and redirected
streams receive the same refusal.

A valid invocation currently exits 3 with no stdout, explaining that offline startup
is not implemented. `doctor --policy-only` is suggested only for policy inspection.
These refusals read no policy, manifest or approval state, contact no engine, and
execute no helper or workload. They prove invocation handling, not isolation or
successful argv delivery to a worker. Later startup slices must supersede this interim
refusal explicitly while preserving the invocation rules.

## Names, help and arguments

These are design requirements, not assertions that the container commands exist:

- Use a short lowercase executable name, lowercase dash-separated subcommands, singular
  noun groups and imperative actions. Avoid overlapping or ambiguous names. Do not add
  an implicit action or accept subcommand abbreviations; an alias requires an explicit
  specification (POSIX, CLIG, COBRA).
- Show help for the bare executable, `--help`, `help <command>`, `<command> --help` and
  `-h` within the extension's arguments. Help goes to stdout with exit 0; groups name
  their subcommands. Include safe examples, the
  [issue-report address](https://github.com/donvargax/itos-container/issues), JSON shape
  or its documented exception, and exit codes (CLIG, GNU-HELP).
- `--version` and `version` print the same first line, `itos-container <version>`, and
  `commit <full commit id>` when known (GNU-CLI, GNU-VER). An unknown command exits 2;
  suggest the intended command when possible.
- Give flags long names and reserve short names for common options. Use standard names
  when applicable, consistently across commands. Flags modify an action rather than
  select another one; mentioning a conventional flag does not add it to the interface.
- Accept `--flag=value` and `--flag value`. Refuse unknown flags, missing values,
  switch values and repeated once-only flags with exit 2. Bind a value only to its
  option and never give an option an optional value. Validate every argument before
  using it. Keep declarations in a shared command model (CLIG, POSIX).
- Accept wrapper flags in any position within the extension's arguments before its
  workload separator. `--` ends wrapper options; the workload may itself contain
  `--help`, `--json`, `-` or more separators. Where a control command specifies stream
  input/output, `-` means stdin/stdout; it does not bypass required policy validation.
- Prompt only when stdin and stdout are terminals. Specify explicit noninteractive
  inputs before introducing a prompt; never treat a missing terminal as consent.
  Approval must remain a deliberate host-user action, not a general force override.
- Do not make CI depend on a network update check. An eventual check needs an opt-out;
  this rule creates no update-check feature (CLIG).

## Output and the contract with scripts

Scripts rely on exit codes, structured output excluding `message` and `fix`, and the
data of written files. Plain output, messages, fixes and file comments are for people
and may change. Stable rule IDs and their meanings are part of the structured contract
(itos and itos-cc). Do not turn prose into a second machine interface.

For planned noninteractive control commands, `--json` or inherited `ITOS_JSON=1`
requests exactly one stdout object with `schema: 1`, `ok` and typed problems on failure,
including usage/configuration failures. JSON schema 1 evolves by adding compatible
keys. Each problem carries a stable dotted lowercase `rule`, its own `exit`, and
human-readable `message` and `fix`. Subject fields sit beside these as flat members;
`rule`, `exit`, `message` and `fix` are reserved. Behaviour specifications must define
the exact problem IDs and multiple-problem exit mapping; none are invented here.

Main control output goes to stdout; diagnostics, logs and progress go to stderr. If
plain output resembles data, direct scripts to JSON with a stderr hint. Error lines
start with `itos-container:` and explain the problem and next action, without dumping
raw commands or secrets (GNU-ERR, CLIG). Use no colour. `NO_COLOR` is a presentation
convention, not authority to forward host variables to a worker.

The pilot's proposed exception is to **refuse JSON mode for interactive `run` and
`exec`**, including inherited `ITOS_JSON=1`, before workload startup. Never mix workload
terminal output with a JSON object. Specify that refusal's diagnostic in its behaviour
scenarios; this document does not assign it a new rule ID.

### Wrapper and workload exit codes

The sources disagree. itos and its extension conventions retain the following
wrapper codes, which the pilot binds this project to:

| Code | Wrapper meaning |
| --- | --- |
| 0 | Success. |
| 1 | Policy or check refusal. |
| 2 | Usage, configuration or unknown-command error. |
| 3 | Missing environment requirement. |
| 70 | Unclassified internal error. |
| 75 | Temporary failure that may succeed when repeated unchanged. |

itos-cc's newer [Square-style codes](https://github.com/square/exit) use 80 for
usage/configuration, 81 for unknown commands, 82 for missing requirements and 100 for
internal errors, with interrupt 130 as its own additional decision. Those numbers and
its precedence/signal rules are not adopted here merely because its guideline is newer.

Once a workload starts, preserve its exit status. Report cleanup errors separately;
if the workload succeeds but required cleanup fails, return nonzero and retain owned
recovery information. The inherited rule against passing through helper-tool codes
does not override workload exit preservation. A workload's numeric status need not
mean the same thing as a wrapper code. Exact cleanup, timeout, signal forwarding and
cancellation outcomes require scenarios; no new code is chosen here.

Classify controller/helper failures deliberately. Keep exit codes, JSON, problem IDs
and their mapping in the UI, never the domain. Help and tested behaviour must agree on
the codes they promise. Existing counting behaviour is unchanged.

## Compatibility and completion

Renamed commands/flags, changed exit codes and incompatible structured/file data are
breaking changes. Group ready breaking changes and document migration; use a major
release from 1.0 and a minor release before 1.0, as the newer upstream guidance does.
Avoid compatibility shims: before v1 drop a retired name; from v1 a retired name can
refuse with exit 2 and identify its replacement rather than execute an old interface.
No upstream previous-release checker or other compliance machinery is claimed to run
here. New behaviour needs this repository's scenario-first work and existing gates.

Preserve the inherited completion design: `completion <shell>` emits a short stub for
bash, zsh, fish or PowerShell that calls the same executable's hidden `__complete`.
Use the command model rather than a second command list. Completion receives words
after the executable, with an empty final word indicating a new word; it emits one
candidate per line and one final `:files` or `:none` instruction. Document installation
in the stub. Suggest declared commands, flags and values only; do not inspect or execute
checkout policy to generate candidates. This does not claim completion of future
container commands before their implementation.

Update these rules with a specified interface change. The version 1 policy schema is
specified; safe filesystem discovery, approval identities, lifecycle hooks, resource
limits and cancellation semantics remain open in the pilot.
[CONFIG.md](CONFIG.md) records the configuration boundary.
