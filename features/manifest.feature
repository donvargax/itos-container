@phase-1
Feature: Inspect the restricted single-worker manifest
  Doctor reads a development-environment request without executing checkout code.
  A valid manifest is not approval or proof that the host can run an isolated worker.
  Sandbox permissions still come from the strict offline policy and host-owned approval.

  # Slice-2 specifies inspection only. Follow docs/charter.md, the restricted-manifest
  # section of docs/plans/container-pilot.md, docs/CONFIG.md and docs/CLI.md.
  # T-14 lands first. Extend slice-1's internal/slices/doctor; do not create a second
  # doctor slice. Shared validation and the image-reference validator belong to
  # internal/engine/manifest/{domain,port,<adapter>}, reused by doctor/approve/run.
  # Follow T-14's why in tasks/phase-1.yaml: a slice imports no adapter or other slice;
  # cmd/ binds adapters. An adapter may import its own domain and port. Each port has
  # a port/porttest contract suite run against its in-memory fake and its adapter.
  # JSON/parser/library types and file I/O stay in adapters. The domain owns validation;
  # public rule IDs, exits and JSON presentation stay beside doctor. No engine reaches a slice.
  # Do not add behavior to count or introduce a Dev Containers CLI dependency.
  # The order is slice-1, slice-2, slice-3. No container engine is needed for this slice.
  #
  # Parser decision, reviewed 2026-11-04:
  # github.com/tailscale/hujson v0.0.0-20260727124030-b80ff77dac4f
  # https://github.com/tailscale/hujson/tree/b80ff77dac4f3b606c8bf0a28a84b33e68d07f60
  # Tailscale maintains this Go parser; its lossless syntax tree retains object members
  # and offsets, allowing duplicate checks before decoding loses them. It implements
  # JSON plus line/block comments and trailing commas, without a JSON5 language or a
  # checkout-executing CLI. Its Go 1.26 requirement fits this project's Go 1.27 toolchain.
  # Pin this version when implementation adds the dependency and preserve its BSD-3-Clause
  # license and notices. This spec adds no dependency to go.mod. Use the syntax tree to
  # reject duplicate decoded keys in every object, including otherwise unused editor data;
  # the parser alone is not a schema validator. Do not strip comments with a custom lexer.
  #
  # Reference-parser decision, reviewed 2026-11-04:
  # github.com/distribution/reference v0.6.0, with github.com/opencontainers/go-digest
  # v1.0.0 as its pinned dependency; preserve their Apache-2.0 licenses and notices.
  # https://github.com/distribution/reference/releases/tag/v0.6.0
  # https://github.com/distribution/reference/blob/v0.6.0/reference.go
  # Use this maintained reference grammar rather than a handwritten image parser.
  # The manifest engine owns the validation contract and a reference adapter wrapping
  # Parse, requiring a named, digested reference and a sha256 digest of 64 lowercase
  # hexadecimal digits. Register sha256 support in that adapter. Preserve the literal
  # reference: do not add a registry, namespace, tag or digest, trim it or resolve it.
  # A tag together with a digest is allowed; a tag alone is not pinned. This performs
  # no registry request and establishes neither image availability nor approval.
  # Pin these modules when implementing; this specification changes no dependencies.
  #
  # Slice-1's closed policy has version, manifest and network, with no image or UID.
  # Read exactly its manifest path relative to the effective checkout root supplied
  # by the extension seam. There is no automatic discovery, candidate precedence,
  # parent search or fallback. Other conventional manifest files are irrelevant.
  # A selected path and every component below the root must be real directories/a regular
  # file, never symlinks (even in-tree or dangling ones) or special files. Check the
  # selected path without executing helpers. Root identity itself is slice-3's contract.
  #
  # doctor --manifest-only validates policy, then the selected manifest and workspace,
  # without engine access, image metadata lookup, host readiness or approval checks.
  # Its success has schema=1, scope="manifest", ok=true, problems=[] and exit 0;
  # manifest refusals keep scope="manifest" and return 2. Policy failures preserve
  # slice-1's rules/exits and short-circuit manifest validation. --policy-only keeps
  # its existing policy-only behavior; combining it with --manifest-only is usage 2.
  # Honor ITOS_JSON=1 and explicit --no-json as slice-1 does. Help and plain success
  # identify the limited scope and say metadata, host readiness and approval were not
  # checked. Ordinary doctor reuses this file validation before slice-3 readiness;
  # slice-3 supplies inspected metadata, never a default absent label on lookup failure.
  # This deliberately supersedes ID-POLICY-34's ordinary-doctor success example: it
  # supplies no manifest, so ordinary doctor must now refuse the missing file. Keep
  # its --policy-only terminal-escaping and JSON-preservation regression intact when
  # implementing the scope change; do not retain policy-only success for ordinary doctor.
  #
  # Supported fields are case-sensitive and exhaustive:
  # - image: required digest-pinned string validated by this engine's reference validator.
  #   No image from metadata, host variables or an implicit default. Any syntactically
  #   valid pinned request is accepted here; slice-4 compares it with host approval.
  # - name: optional nonempty display string without ASCII control characters; default
  #   null. It grants no authority and is never a checkout, engine or approval identity.
  # - workspaceFolder: default /workspace, the controller-selected checkout mount target.
  #   Otherwise /workspace/<relative-directory>, with slash-separated nonempty components,
  #   no dot/dot-dot components, backslash, NUL, or trailing slash. The corresponding
  #   checkout directory must already exist, with no symlink component. Never create it,
  #   use it as a host mount source, or select an extra mount from it.
  # - containerEnv: object of literal string values, default {}. Names match
  #   [A-Za-z_][A-Za-z0-9_]*; values may be empty or contain whitespace but never NUL.
  #   Names are not sourced from the host. Reserved checks use ASCII case folding.
  #   Exact reserved names: HOME, USER, LOGNAME, PATH, SHELL, ENV, BASH_ENV, SHELLOPTS,
  #   BASHOPTS, TMP, TEMP, TMPDIR, HTTP_PROXY, HTTPS_PROXY, ALL_PROXY, NO_PROXY,
  #   SSL_CERT_FILE, SSL_CERT_DIR, NODE_EXTRA_CA_CERTS, REQUESTS_CA_BUNDLE, CURL_CA_BUNDLE,
  #   OPENAI_API_KEY, ANTHROPIC_API_KEY, GOOGLE_APPLICATION_CREDENTIALS.
  #   Reserved prefixes: ITOS_, DOCKER_, CONTAINER_, XDG_, LD_, DYLD_, SSH_, GIT_, GH_,
  #   GITHUB_, AWS_, AZURE_, GOOGLE_, GCLOUD_, CLAUDE_, ANTHROPIC_, OPENAI_, OPENCODE_.
  #   These are controller, identity, trust, tool-loading or account namespaces, not a
  #   general secret detector. Configuration must contain no credentials; accepting a
  #   literal application variable never authorizes host credential sourcing.
  # - containerUser and remoteUser: optional positive decimal UID strings matching
  #   [1-9][0-9]*, without leading zeroes, names, signs, whitespace or :group suffixes.
  #   Each omitted value is null (unspecified), not a host/image/policy-derived UID.
  #   Explicit null is invalid. Preserve decimal text without machine-integer coercion.
  #   Validate each request independently; slice-4 resolves omissions and compares both
  #   with the one host-approved non-root identity. Acceptance here grants no identity.
  # - customizations: optional object containing only vscode; vscode is an object with
  #   only extensions (array of nonempty strings) and settings (object of opaque JSON
  #   data). All of vscode is explicitly unused, never installed, interpreted or expanded.
  #   Missing editor data contributes no effective setting. Unknown namespaces/keys fail.
  # No other field, including x- extensions or $schema, is silently accepted.
  #
  # In effective fields, reject every string value containing ${ before field-specific value
  # checks: no devcontainer variable substitution, including container/local workspace
  # variables. Ordinary $NAME, $(...), backticks and comment-looking text inside allowed
  # strings stay literal. No shell, interpolation engine or host lookup runs while reading.
  # Syntax and decoded-key duplicates precede semantic validation. Then collect independent
  # semantic problems in JSON-pointer order; a rejected parent need not produce child errors.
  #
  # Lifecycle decision: this inspection profile supports NO lifecycle hooks yet.
  # Reject initializeCommand, onCreateCommand, updateContentCommand, postCreateCommand,
  # postStartCommand, postAttachCommand and waitFor, including empty or argv values.
  # The pilot leaves hook ordering, repeat behavior and runtime failure/cancellation
  # semantics open. T-11 and the person's security decisions must settle those before a
  # later spec admits a hook. Do not accept a command now and silently ignore it or run it
  # on the host. This slice does not invent runtime protections or claim AC-42 proof.
  #
  # Metadata validation accepts the devcontainer.metadata label's presence and raw value
  # as given data, plus the requested image reference for diagnostic attribution. There
  # is no Engine API, endpoint selection or cached-image lookup in this validator or its
  # fixtures. Slice-3 reads the label and calls this same engine API. An explicitly
  # absent label is allowed; not-yet-inspected is distinct from absent. If present,
  # its string must be strict JSON (no comments/trailing commas), an array of objects,
  # without duplicate decoded keys at any depth. Only name and customizations.vscode,
  # constrained as above, are allowed in each object. They are reported as unused and
  # never merged with checkout settings. Even operational fields supported in the visible
  # manifest (image, workspaceFolder, containerEnv, containerUser, remoteUser) are refused
  # in metadata, as are every hook and unsupported permission. [] and [{}] are allowed.
  # Inspect all entries, not just the first. A missing/unreadable cached image remains
  # slice-3's readiness failure; it cannot supply an absent-label validation input.
  #
  # Public JSON fragment on successful manifest inspection:
  # manifest = {valid:true, path:<selected relative path>, effective:{name, image,
  # workspaceFolder, containerEnv, containerUser, remoteUser}, unused:[<JSON pointers>],
  # image_metadata:{checked:<bool>, present:<bool or null>, unused:[<label JSON pointers>]}}.
  # --manifest-only always reports checked=false, present=null, unused=[]; it makes no
  # claim about an image label. Supplied metadata yields checked=true and its presence.
  # The six effective fields always include defaults. unused pointers are sorted;
  # report /customizations/vscode once, and /<index>/name or /<index>/customizations/vscode
  # for metadata. Ignored values never enter effective. Other doctor checks and overall
  # success, including approval readiness, belong to their slices: this limited mode's
  # ok=true is configuration validity only, never permission to run.
  #
  # On manifest refusal, schema=1, ok=false, manifest.valid=false and no manifest.effective.
  # Each problem has rule, exit, nonempty message/fix, and path (a decoded-key JSON pointer;
  # the root is ""). Build each pointer from decoded key segments, escaping ~ as ~0 and
  # / as ~1 within each segment, including duplicate keys and image-label problems.
  # A slash in a key is not a nesting boundary. File problems have file=<selected path>;
  # a missing file still names the selected policy path. Image-label problems instead have
  # image=<requested manifest image> and
  # label="devcontainer.metadata". No raw input values/lines/commands appear in diagnostics.
  # Configuration refusals below all have exit=2; together they return 2. Do not stop at
  # the first independent semantic problem. No problem order is a second public API.
  # Human output explains the candidate/field and how to correct it, and explicitly says
  # editor customizations/accepted metadata are unused. Plain refusal diagnostics go to
  # stderr and identify the candidate file or image label without echoing raw values,
  # just as in JSON mode. JSON is one stdout object; logs stay on stderr. Tests check
  # typed problems or diagnostic subjects rather than freezing error prose.
  #
  # New manifest fixture steps are required; existing generic run/output steps remain usable.
  # "a manifest inspection checkout" creates an ordinary temporary checkout and writes
  # itos-container.yaml as {version: 1, manifest: .devcontainer/devcontainer.json,
  # network: {mode: offline}}, with no image or identity keys. It writes the selected
  # manifest containing only the image below. There is no engine fixture or endpoint.
  # File scenarios use the real binary/adapters, never a production-code import or
  # canned doctor result. The --manifest-only option is a public data-validation mode,
  # not a test-only input channel. It never accepts metadata through flags or environment.
  # Clear inherited engine/account variables and place HOME, XDG and engine configuration
  # in the fixture's temporary directories; never inspect a caller's login or engine state.
  # The fixture starts with no approval; this suite does not write host approval records.
  # It records checkout/control-state bytes and modes and invocations
  # of PATH traps for docker, devcontainer and checkout helpers, all in its temp directory.
  # Trap programs only record invocation; never delegate to the host's tools.
  #
  # Fixture image (synthetic reference text only; never pull or inspect it):
  # registry.example/worker@sha256:1111111111111111111111111111111111111111111111111111111111111111
  # All remaining Given steps only write/remove/move fixture files, set raw member/label
  # input data or create directories/symlinks/special-file fixtures. They never call doctor or
  # assert validation. A manifest "member ... has JSON value" edits that single member;
  # "manifest text is" preserves the exact doc string, including duplicate keys and escapes.
  # The unsafe-path fixture supplies valid bytes behind links, and a dangling-link case;
  # outside targets remain inside the test's temporary parent, never a real host secret.
  # "the policy selects manifest" changes only that policy member; location steps move
  # the file and update the member. Unsafe layouts modify that selected location, not
  # always the nested default. A workspace entry can name several components: an
  # intermediate-symlink
  # layout links its first component to a directory containing the remaining path.
  # All symlink targets other than explicit dangling cases exist and contain valid data,
  # so accepting an in-checkout link or checking only the final component is detectable.
  # "host execution sentinels" installs harmless recording scripts/traps and snapshots
  # state AFTER setup, without running any checkout script. It also seeds .git/hooks and
  # editor tasks with those scripts, so an accidental helper call is observable.
  # Do not call slice-1's installPolicySentinels unchanged: it overwrites the selected
  # manifest with initializeCommand. Preserve all manifest/policy bytes and label data.
  # Snapshots use no-follow entry information: record link targets and special-file types,
  # never traverse symlinks or open a FIFO/device. Read bytes only from regular files;
  # the unsafe-layout Given must finish before doctor runs. Keep recorder output separate
  # from the snapshotted trees, and compare it in the side-effect assertion.
  #
  # New Then steps parse actual stdout, never inspect code. "reports a valid manifest"
  # checks one schema=1 object and manifest.valid/path, plus scope="manifest", ok=true
  # and problems=[] for --manifest-only. "JSON value at ... is" compares a
  # decoded value exactly. "reports ... at ... with exit ..." checks ok=false, the invalid
  # manifest, absence of effective, and a complete matching problem including file or
  # image/label subjects. "inspection has no host side effects" compares recorded bytes/
  # modes, absent execution markers and no trap invocations. No engine request is needed.
  # Domain tests additionally use fakes to prove the file-only path calls no readiness
  # dependency; absence of a container alone is not evidence of read-only behavior.
  #
  # Metadata scenarios exercise the shared validator's input/output contract, not doctor
  # engine integration. "the supplied devcontainer.metadata label" sets raw data only;
  # "the manifest inspection contract runs" passes it to the real manifest validator.
  # Bind real parsing/reference adapters and in-memory file data at a test-only cmd/
  # assembly, preserving T-14's import arrows. Use the same doctor presenter for the
  # JSON/plain assertions, capturing its actual output and returned exit for the Then
  # steps; no fabricated verdict, Engine API fake or production metadata
  # injection flag. Keep the feature runner free of production imports. Domain tests use
  # the owning ports' fakes; porttest suites run against fakes and actual adapters so a
  # fake cannot silently change parsing semantics. Slice-3 owns binary-to-engine evidence.
  #
  # Reuse the existing run, run-with-environment, exit and output-containment steps.
  # Reuse the run-with-deadline and deadline assertion steps for unsafe-file cases.
  # File scenarios are black-box CLI tests; metadata cases are validator contract tests.
  # All run without Docker and prove validation/inspection, not runtime isolation.
  # Implementers commit the missing steps red first, keep these @wip in that commit,
  # then remove @wip for
  # failing-then-passing runs. Slice-1's doctor and T-14's layout are already present;
  # doctor currently validates policy only, with no manifest option or metadata validator.
  # Make the red tests compile and reach each manifest-specific assertion;
  # missing APIs, undefined steps,
  # unknown-command errors or exit 2 alone are not failing-then-passing proof.

  Background:
    Given a manifest inspection checkout

  # Policy-selected paths and syntax. Every location resolves the same defaults; comments
  # and trailing commas must not require a second parser or rewrite the source file.
  @slice-2 @ID-MANIFEST-01
  Scenario Outline: Read the policy-selected manifest and accept JSON with comments
    Given the manifest is located only at "<file>"
    And the manifest text is:
      """
      {
        // One requested worker, never a build recipe.
        "image": "registry.example/worker@sha256:1111111111111111111111111111111111111111111111111111111111111111",
        /* Deliberately use trailing commas in an object and an array. */
        "customizations": {"vscode": {"extensions": ["example.editor",],},},
      }
      """
    And host execution sentinels are installed
    When itos-container runs with "doctor --manifest-only --json"
    Then its JSON output reports a valid manifest at "<file>"
    And its JSON value at "/manifest/effective" is:
      """
      {"name":null,"image":"registry.example/worker@sha256:1111111111111111111111111111111111111111111111111111111111111111","workspaceFolder":"/workspace","containerEnv":{},"containerUser":null,"remoteUser":null}
      """
    And its JSON value at "/manifest/image_metadata" is:
      """
      {"checked":false,"present":null,"unused":[]}
      """
    And it exits with code 0
    And inspection has no host side effects

    Examples:
      | file                           |
      | .devcontainer/devcontainer.json |
      | .devcontainer.json              |
      | config files/worker.json        |

  @slice-2 @ID-MANIFEST-02
  Scenario: A missing selected manifest never triggers fallback discovery or a default worker
    Given the policy selects manifest "config/worker.json"
    And the selected manifest does not exist
    And valid decoy manifests exist at ".devcontainer/devcontainer.json" and ".devcontainer.json"
    And valid decoy manifests exist at "devcontainer.json", ".devcontainer/devcontainer.jsonc" and ".devcontainer/other/devcontainer.json"
    And a valid decoy manifest exists in the checkout's parent directory
    When itos-container runs with "doctor --manifest-only --json"
    Then its JSON output reports "manifest.missing" at "" with exit 2
    And it exits with code 2

  @slice-2 @ID-MANIFEST-03
  Scenario Outline: Inspect only the selected manifest even when another conventional file exists
    Given the manifest is located only at "<file>"
    And the unselected conventional manifest exists with "<contents>"
    When itos-container runs with "doctor --manifest-only --json"
    Then its JSON output reports a valid manifest at "<file>"
    And it exits with code 0

    Examples:
      | file                           | contents                          |
      | .devcontainer/devcontainer.json | the same valid bytes as the first |
      | .devcontainer/devcontainer.json | malformed JSON                    |
      | .devcontainer.json              | malformed JSON                    |

  # The mode is a user-facing validation scope. It must remain usable when host
  # readiness is added, and must not claim that metadata or approval was inspected.
  @slice-2 @ID-MANIFEST-28
  Scenario: Explain the manifest-only scope and honor the extension JSON request
    When itos-container runs with "doctor --help"
    Then its standard output says "--manifest-only"
    And it exits with code 0
    When itos-container runs with "doctor --manifest-only" and the environment "ITOS_JSON=1"
    Then its JSON output reports a valid manifest at ".devcontainer/devcontainer.json"
    And its JSON value at "/scope" is:
      """
      "manifest"
      """
    And it exits with code 0
    When itos-container runs with "doctor --manifest-only --no-json" and the environment "ITOS_JSON=1"
    Then its standard output says "Manifest only"
    And its standard output says "Image metadata, host readiness and approval were not checked"
    And it exits with code 0
    When itos-container runs with "doctor --manifest-only --policy-only"
    Then its error output says "--manifest-only"
    And its error output says "--policy-only"
    And it exits with code 2

  # Reject a FIFO without opening it for a blocking read. The five-second test deadline
  # bounds an incorrect reader; it does not define a product parsing/resource budget.
  @slice-2 @ID-MANIFEST-04
  Scenario Outline: Refuse unsafe manifest candidates without following or executing them
    Given the manifest is located only at "<file>"
    And the manifest candidate has the layout "<layout>"
    And host execution sentinels are installed
    When itos-container runs with "doctor --manifest-only --json" and a 5 second deadline
    Then its JSON output reports "manifest.path" at "" with exit 2
    And it finishes before the deadline
    And it exits with code 2
    And inspection has no host side effects

    Examples:
      | file                           | layout                                     |
      | .devcontainer/devcontainer.json | file symlink to an in-checkout file        |
      | .devcontainer/devcontainer.json | file symlink outside the checkout         |
      | .devcontainer/devcontainer.json | dangling file symlink                     |
      | .devcontainer/devcontainer.json | .devcontainer directory symlink           |
      | .devcontainer/devcontainer.json | dangling .devcontainer directory symlink  |
      | .devcontainer/devcontainer.json | directory at the selected candidate       |
      | .devcontainer/devcontainer.json | FIFO at the selected candidate            |
      | .devcontainer.json              | file symlink to an in-checkout file        |
      | .devcontainer.json              | file symlink outside the checkout         |
      | .devcontainer.json              | dangling file symlink                     |
      | .devcontainer.json              | directory at the selected candidate       |
      | .devcontainer.json              | FIFO at the selected candidate            |

  @slice-2 @ID-MANIFEST-05
  Scenario Outline: Reject malformed JSON and JSON5 extensions before interpreting fields
    Given the manifest text is:
      """
      <text>
      """
    When itos-container runs with "doctor --manifest-only --json"
    Then its JSON output reports "<rule>" at "" with exit 2
    And it exits with code 2

    Examples:
      | text                    | rule            |
      |                         | manifest.syntax |
      | // only a comment       | manifest.syntax |
      | {                       | manifest.syntax |
      | {} {}                   | manifest.syntax |
      | {"name":"unterminated}  | manifest.syntax |
      | {name:"worker"}         | manifest.syntax |
      | {'name':'worker'}       | manifest.syntax |
      | {"name":NaN}            | manifest.syntax |
      | {"name":0x10}           | manifest.syntax |
      | {/* unterminated        | manifest.syntax |
      | []                      | manifest.value  |
      | null                    | manifest.value  |
      | "worker"                | manifest.value  |

  # Duplicate detection uses decoded keys, recursively, before a map could discard one.
  # The settings object is unused but must still have one unambiguous JSON meaning.
  @slice-2 @ID-MANIFEST-06
  Scenario Outline: Refuse duplicate decoded keys at every depth
    Given the manifest text is:
      """
      <text>
      """
    When itos-container runs with "doctor --manifest-only --json"
    Then its JSON output reports "manifest.duplicate" at "<path>" with exit 2
    And it exits with code 2

    Examples:
      | text                                                                    | path                                      |
      | {"image":"one","image":"two"}                                           | /image                                    |
      | {"image":"one","\u0069mage":"two"}                                      | /image                                    |
      | {"containerEnv":{"MODE":"one","M\u004fDE":"two"}}                        | /containerEnv/MODE                        |
      | {"customizations":{"vscode":{"settings":{"a":[{"x":1,"x":2}]}}}}         | /customizations/vscode/settings/a/0/x      |
      | {"customizations":{"vscode":{"settings":{"a/b~c":1,"a\\u002fb~c":2}}}}  | /customizations/vscode/settings/a~1b~0c    |

  # A recognized unsafe field is not made acceptable by false, [] or {}. Unknown fields
  # fail too, including misspellings and fields from the unrestricted devcontainer spec.
  @slice-2 @ID-MANIFEST-07
  Scenario Outline: Refuse unsupported execution and permission fields
    Given the manifest member "<field>" has JSON value <value>
    And host execution sentinels are installed
    When itos-container runs with "doctor --manifest-only --json"
    Then its JSON output reports "manifest.unsupported" at "/<field>" with exit 2
    And it exits with code 2
    And inspection has no host side effects

    Examples:
      | field               | value                                      |
      | build               | {"dockerfile":"Dockerfile"}                |
      | dockerFile          | "Dockerfile"                               |
      | features            | {}                                         |
      | dockerComposeFile   | "compose.yaml"                             |
      | service             | "worker"                                   |
      | runServices         | []                                         |
      | mounts              | []                                         |
      | workspaceMount      | "source=/tmp,target=/workspace,type=bind"   |
      | runArgs             | []                                         |
      | privileged          | false                                      |
      | capAdd              | []                                         |
      | securityOpt         | []                                         |
      | init                | true                                       |
      | forwardPorts        | [3000]                                     |
      | appPort             | 3000                                       |
      | portsAttributes     | {}                                         |
      | otherPortsAttributes | {}                                        |
      | overrideCommand     | false                                      |
      | shutdownAction      | "none"                                     |
      | updateRemoteUserUID  | true                                       |
      | userEnvProbe        | "loginInteractiveShell"                    |
      | remoteEnv           | {"MODE":"test"}                            |
      | hostRequirements    | {}                                         |
      | extensions          | ["example.editor"]                         |
      | settings            | {}                                         |
      | Image               | "ignored-misspelling"                      |
      | x-worker            | {}                                         |
      | $schema             | "https://example.invalid/schema.json"      |

  @slice-2 @ID-MANIFEST-08
  Scenario Outline: Validate the types and values of supported manifest fields
    Given the manifest member "<field>" has JSON value <value>
    When itos-container runs with "doctor --manifest-only --json"
    Then its JSON output reports "manifest.value" at "<path>" with exit 2
    And it exits with code 2

    Examples:
      | field         | value                      | path               |
      | name          | null                       | /name              |
      | name          | 42                         | /name              |
      | name          | ""                         | /name              |
      | name          | "worker\\ncontrol"         | /name              |
      | workspaceFolder | false                    | /workspaceFolder   |
      | containerEnv  | []                         | /containerEnv      |
      | containerEnv  | null                       | /containerEnv      |
      | containerEnv  | {"MODE":true}              | /containerEnv/MODE |
      | containerEnv  | {"MODE":null}              | /containerEnv/MODE |
      | containerEnv  | {"MODE":"a\u0000b"}        | /containerEnv/MODE |
      | containerEnv  | {"9MODE":"test"}           | /containerEnv/9MODE |
      | containerEnv  | {"APP-MODE":"test"}        | /containerEnv/APP-MODE |
      | containerEnv  | {"":"test"}                | /containerEnv/     |
      | containerUser | 1000                       | /containerUser     |
      | remoteUser    | null                       | /remoteUser        |

  @slice-2 @ID-MANIFEST-09
  Scenario Outline: Refuse an image that is not a valid digest-pinned reference
    Given the manifest member "image" has JSON value <image>
    When itos-container runs with "doctor --manifest-only --json"
    Then its JSON output reports "manifest.image" at "/image" with exit 2
    And it exits with code 2

    Examples:
      | image                                                                                                     |
      | null                                                                                                      |
      | 42                                                                                                        |
      | ""                                                                                                        |
      | "registry.example/worker:latest"                                                                          |
      | "registry.example/worker@sha256:1111"                                                                     |
      | "registry.example/worker"                                                                                 |
      | "registry.example/worker@sha256:gggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggg"             |
      | "registry.example/worker@sha256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"             |
      | "registry.example/Worker@sha256:1111111111111111111111111111111111111111111111111111111111111111"               |
      | "https://registry.example/worker@sha256:1111111111111111111111111111111111111111111111111111111111111111"       |
      | "registry.example/worker@sha512:1111"                                                                       |
      | "sha256:1111111111111111111111111111111111111111111111111111111111111111"                                    |
      | " registry.example/worker@sha256:1111111111111111111111111111111111111111111111111111111111111111"              |
      | "registry.example/worker@sha256:1111111111111111111111111111111111111111111111111111111111111111 "              |

  # Ordinary doctor must apply the same refusal before slice-3 engine checks.
  @slice-2 @ID-MANIFEST-10
  Scenario Outline: Require an explicit image instead of choosing a default
    Given the manifest has no "image" member
    When itos-container runs with "doctor <mode> --json"
    Then its JSON output reports "manifest.image" at "/image" with exit 2
    And it exits with code 2

    Examples:
      | mode            |
      | --manifest-only |
      |                 |

  # Different valid requests are not approval mismatches. A supplied UID does not
  # fill in the other optional UID or grant permission to run either one.
  # An unqualified image name remains literal; validation must not add docker.io/library.
  @slice-2 @ID-MANIFEST-27
  Scenario Outline: Accept pinned image and UID requests without host approval
    Given the manifest member "image" has JSON value "<image>"
    And the manifest member "<user_field>" has JSON value "1001"
    When itos-container runs with "doctor --manifest-only --json"
    Then its JSON output reports a valid manifest at ".devcontainer/devcontainer.json"
    And its JSON value at "/manifest/effective/image" is:
      """
      "<image>"
      """
    And its JSON value at "/manifest/effective/containerUser" is:
      """
      <container_uid>
      """
    And its JSON value at "/manifest/effective/remoteUser" is:
      """
      <remote_uid>
      """
    And it exits with code 0

    Examples:
      | image                                                                                                   | user_field    | container_uid | remote_uid |
      | registry.example/worker@sha256:2222222222222222222222222222222222222222222222222222222222222222             | containerUser | "1001"        | null       |
      | registry.example/other@sha256:1111111111111111111111111111111111111111111111111111111111111111              | remoteUser    | null          | "1001"     |
      | registry.example:5000/team/worker:release@sha256:1111111111111111111111111111111111111111111111111111111111111111 | containerUser | "1001"    | null       |
      | worker@sha256:1111111111111111111111111111111111111111111111111111111111111111                               | remoteUser    | null          | "1001"     |

  # Workspace and identity are requests for one worker, not ways
  # to choose host paths, extra mounts, an elevated identity or a second user.
  @slice-2 @ID-MANIFEST-11
  Scenario Outline: Refuse workspace locations outside the fixed checkout layout
    Given the manifest member "workspaceFolder" has JSON value <folder>
    And host execution sentinels are installed
    When itos-container runs with "doctor --manifest-only --json"
    Then its JSON output reports "manifest.workspace" at "/workspaceFolder" with exit 2
    And it exits with code 2
    And inspection has no host side effects

    Examples:
      | folder                      |
      | ""                          |
      | "/"                         |
      | "/tmp"                      |
      | "/workspace-other"          |
      | "relative"                  |
      | "/workspace/.."             |
      | "/workspace/../tmp"          |
      | "/workspace/./src"           |
      | "/workspace//src"            |
      | "/workspace/src/"            |
      | "/workspace\\\\src"         |
      | "/workspace/a\u0000b"        |
      | "/workspace/not-created"     |

  @slice-2 @ID-MANIFEST-12
  Scenario Outline: Refuse a workspace directory reached through a symlink or a file
    Given the checkout workspace entry "<entry>" has layout "<layout>"
    And the manifest member "workspaceFolder" has JSON value "/workspace/<entry>"
    And host execution sentinels are installed
    When itos-container runs with "doctor --manifest-only --json"
    Then its JSON output reports "manifest.workspace" at "/workspaceFolder" with exit 2
    And it exits with code 2
    And inspection has no host side effects

    Examples:
      | entry      | layout                                                |
      | src        | symlink to an in-checkout directory                    |
      | src        | symlink outside the checkout                          |
      | src        | dangling symlink                                      |
      | src        | regular file                                          |
      | src/nested | intermediate symlink to an in-checkout directory       |
      | src/nested | intermediate symlink outside the checkout             |

  @slice-2 @ID-MANIFEST-13
  Scenario Outline: Refuse user values that are not positive decimal UIDs
    Given the manifest member "<field>" has JSON value <user>
    When itos-container runs with "doctor --manifest-only --json"
    Then its JSON output reports "manifest.identity" at "/<field>" with exit 2
    And it exits with code 2

    Examples:
      | field         | user        |
      | containerUser | "root"      |
      | containerUser | "0"         |
      | containerUser | "-1"        |
      | containerUser | "+1000"     |
      | containerUser | " 1000"     |
      | containerUser | "1000 "     |
      | containerUser | "1.0"       |
      | containerUser | ""          |
      | containerUser | "1000:0"    |
      | containerUser | "01000"     |
      | remoteUser    | "root"      |
      | remoteUser    | "0"         |
      | remoteUser    | "01000"     |
      | remoteUser    | "-1"        |
      | remoteUser    | "+1000"     |
      | remoteUser    | " 1000"     |
      | remoteUser    | "1.0"       |
      | remoteUser    | ""          |
      | remoteUser    | "worker"    |
      | remoteUser    | "1000:1000" |

  # These are literal, synthetic data, not credentials. A host marker is deliberately
  # different from the written dollar expression and must never enter effective settings.
  # UID 1 and values beyond uint64 prove decimal syntax without an assumed host UID range;
  # approval/readiness still own whether a requested identity can be used by the worker.
  @slice-2 @ID-MANIFEST-14
  Scenario Outline: Preserve literal environment values and resolve an existing checkout subdirectory
    Given a checkout directory "src" exists
    And the manifest member "name" has JSON value "worker display"
    And the manifest member "workspaceFolder" has JSON value "/workspace/src"
    And the manifest member "containerUser" has JSON value "<container_uid>"
    And the manifest member "remoteUser" has JSON value "<remote_uid>"
    And the manifest member "containerEnv" has JSON value {"APP_MODE":"offline","EMPTY":"","TEXT":"$SLICE_2_HOST $(touch slice-2-ran) `touch slice-2-ran` // literal /* literal */","LINES":"first\nsecond"}
    And host execution sentinels are installed
    When itos-container runs with "doctor --manifest-only --json" and the environment "SLICE_2_HOST=host-only-marker"
    Then its JSON output reports a valid manifest at ".devcontainer/devcontainer.json"
    And its JSON value at "/manifest/effective" is:
      """
      {"name":"worker display","image":"registry.example/worker@sha256:1111111111111111111111111111111111111111111111111111111111111111","workspaceFolder":"/workspace/src","containerEnv":{"APP_MODE":"offline","EMPTY":"","TEXT":"$SLICE_2_HOST $(touch slice-2-ran) `touch slice-2-ran` // literal /* literal */","LINES":"first\nsecond"},"containerUser":"<container_uid>","remoteUser":"<remote_uid>"}
      """
    And its standard output does not say "host-only-marker"
    And its error output does not say "host-only-marker"
    And inspection has no host side effects

    Examples:
      | container_uid        | remote_uid           |
      | 1                    | 1                    |
      | 1000                 | 1000                 |
      | 1234                 | 1234                 |
      | 1001                 | 1002                 |
      | 18446744073709551616 | 18446744073709551617 |

  @slice-2 @ID-MANIFEST-15
  Scenario Outline: Reject reserved environment namespaces regardless of case or empty values
    Given the manifest member "containerEnv" has JSON value {"<name>":""}
    When itos-container runs with "doctor --manifest-only --json"
    Then its JSON output reports "manifest.reserved" at "/containerEnv/<name>" with exit 2
    And it exits with code 2

    Examples:
      | name                           |
      | HOME                           |
      | USER                           |
      | LOGNAME                        |
      | PATH                           |
      | SHELL                          |
      | ENV                            |
      | BASH_ENV                       |
      | SHELLOPTS                      |
      | BASHOPTS                       |
      | TMP                            |
      | TEMP                           |
      | TMPDIR                         |
      | HTTP_PROXY                     |
      | http_proxy                     |
      | HTTPS_PROXY                    |
      | ALL_PROXY                      |
      | NO_PROXY                       |
      | SSL_CERT_FILE                  |
      | SSL_CERT_DIR                   |
      | NODE_EXTRA_CA_CERTS             |
      | REQUESTS_CA_BUNDLE              |
      | CURL_CA_BUNDLE                  |
      | OPENAI_API_KEY                 |
      | ANTHROPIC_API_KEY              |
      | GOOGLE_APPLICATION_CREDENTIALS |
      | ITOS_CONTAINER_ROOT            |
      | itos_custom                    |
      | DOCKER_HOST                    |
      | CONTAINER_HOST                 |
      | XDG_CONFIG_HOME                |
      | LD_PRELOAD                     |
      | DYLD_INSERT_LIBRARIES          |
      | SSH_AUTH_SOCK                  |
      | GIT_CONFIG_COUNT               |
      | GH_TOKEN                       |
      | GITHUB_TOKEN                   |
      | AWS_PROFILE                    |
      | AZURE_CONFIG_DIR               |
      | GOOGLE_CLOUD_PROJECT           |
      | GCLOUD_PROJECT                 |
      | CLAUDE_CONFIG_DIR              |
      | ANTHROPIC_BASE_URL             |
      | OPENAI_BASE_URL                |
      | OPENCODE_CONFIG               |

  @slice-2 @ID-MANIFEST-16
  Scenario Outline: Refuse variable-substitution syntax instead of expanding host or container values
    Given the manifest member "<field>" has JSON value <value>
    When itos-container runs with "doctor --manifest-only --json" and the environment "SLICE_2_HOST=host-only-marker"
    Then its JSON output reports "manifest.interpolation" at "<path>" with exit 2
    And it exits with code 2
    And its standard output does not say "host-only-marker"
    And its error output does not say "host-only-marker"

    Examples:
      | field           | value                                      | path               |
      | name            | "${localWorkspaceFolderBasename}"          | /name              |
      | image           | "${localEnv:SLICE_2_HOST}"                  | /image             |
      | workspaceFolder | "${localWorkspaceFolder}"                  | /workspaceFolder   |
      | workspaceFolder | "${containerWorkspaceFolder}"              | /workspaceFolder   |
      | containerUser   | "${localEnv:UID}"                          | /containerUser     |
      | remoteUser      | "${containerEnv:USER}"                     | /remoteUser        |
      | containerEnv    | {"MODE":"${localEnv:SLICE_2_HOST}"}         | /containerEnv/MODE |
      | containerEnv    | {"MODE":"${containerEnv:PATH}"}             | /containerEnv/MODE |
      | containerEnv    | {"MODE":"${HOME}"}                          | /containerEnv/MODE |
      | containerEnv    | {"MODE":"${devcontainerId}"}                | /containerEnv/MODE |

  # No hook is a supported command yet. Empty, array and object forms cannot bypass
  # that decision; initializeCommand must not run even before reporting its refusal.
  @slice-2 @ID-MANIFEST-17
  Scenario Outline: Refuse every lifecycle hook while its runtime semantics remain unspecified
    Given the manifest member "<field>" has JSON value <value>
    And host execution sentinels are installed
    When itos-container runs with "doctor --manifest-only --json"
    Then its JSON output reports "manifest.unsupported" at "/<field>" with exit 2
    And it exits with code 2
    And inspection has no host side effects

    Examples:
      | field                | value                         |
      | initializeCommand    | "./slice-2-record-execution"  |
      | initializeCommand    | ["./slice-2-record-execution"] |
      | onCreateCommand      | ["./slice-2-record-execution"] |
      | updateContentCommand | ["./slice-2-record-execution"] |
      | postCreateCommand    | ["./slice-2-record-execution"] |
      | postStartCommand     | ["./slice-2-record-execution"] |
      | postAttachCommand    | ["./slice-2-record-execution"] |
      | postCreateCommand    | ""                            |
      | postCreateCommand    | []                            |
      | postCreateCommand    | {"parallel":["true"]}          |
      | waitFor              | "postCreateCommand"           |

  # Editor settings are opaque data. Recognizing the namespace is not consent to
  # install extensions, expand an editor variable or invoke task/customization code.
  @slice-2 @ID-MANIFEST-18
  Scenario: Report recognized editor customizations as unused without executing their data
    Given the manifest member "customizations" has JSON value {"vscode":{"extensions":["example.editor"],"settings":{"example.command":"./slice-2-record-execution","example.variable":"${localEnv:SLICE_2_HOST}"}}}
    And host execution sentinels are installed
    When itos-container runs with "doctor --manifest-only --json" and the environment "SLICE_2_HOST=host-only-marker"
    Then its JSON output reports a valid manifest at ".devcontainer/devcontainer.json"
    And its JSON value at "/manifest/unused" is:
      """
      ["/customizations/vscode"]
      """
    And its JSON value at "/manifest/effective/containerEnv" is:
      """
      {}
      """
    And its standard output does not say "host-only-marker"
    And its error output does not say "host-only-marker"
    And inspection has no host side effects

  @slice-2 @ID-MANIFEST-19
  Scenario Outline: Refuse unknown editor namespaces and malformed recognized customizations
    Given the manifest member "customizations" has JSON value <value>
    When itos-container runs with "doctor --manifest-only --json"
    Then its JSON output reports "<rule>" at "<path>" with exit 2
    And it exits with code 2

    Examples:
      | value                                      | rule                 | path                                  |
      | []                                         | manifest.value       | /customizations                       |
      | {"otherEditor":{}}                          | manifest.unsupported | /customizations/otherEditor           |
      | {"vscode":null}                             | manifest.value       | /customizations/vscode                |
      | {"vscode":{"command":"true"}}               | manifest.unsupported | /customizations/vscode/command        |
      | {"vscode":{"extensions":"example.editor"}}  | manifest.value       | /customizations/vscode/extensions     |
      | {"vscode":{"extensions":[""]}}              | manifest.value       | /customizations/vscode/extensions/0   |
      | {"vscode":{"settings":[]}}                  | manifest.value       | /customizations/vscode/settings       |

  # Absence is an explicit input, not the default result of skipping image inspection.
  @slice-2 @ID-MANIFEST-29
  Scenario: Distinguish a supplied absent label from metadata that was never inspected
    Given the supplied devcontainer.metadata label is absent
    When the manifest inspection contract runs with supplied metadata in JSON mode
    Then its JSON output reports a valid manifest at ".devcontainer/devcontainer.json"
    And its JSON value at "/manifest/image_metadata" is:
      """
      {"checked":true,"present":false,"unused":[]}
      """
    And it exits with code 0

  # The visible name must remain null even when metadata supplies one. A syntactically
  # acceptable label is never a second source of effective settings or commands.
  @slice-2 @ID-MANIFEST-20
  Scenario Outline: Inspect permitted image metadata and report it as unused without merging
    Given the supplied devcontainer.metadata label is:
      """
      <label>
      """
    And host execution sentinels are installed
    When the manifest inspection contract runs with supplied metadata in JSON mode
    Then its JSON output reports a valid manifest at ".devcontainer/devcontainer.json"
    And its JSON value at "/manifest/effective/name" is:
      """
      null
      """
    And its JSON value at "/manifest/image_metadata" is:
      """
      {"checked":true,"present":true,"unused":<unused>}
      """
    And it exits with code 0
    And inspection has no host side effects
    When the manifest inspection contract runs with supplied metadata in plain mode
    Then its standard output says "devcontainer.metadata"
    And its standard output says "unused"
    And it exits with code 0
    And inspection has no host side effects

    Examples:
      | label                                                                                               | unused                                        |
      | []                                                                                                  | []                                            |
      | [{}]                                                                                                | []                                            |
      | [{"name":"image display"},{"customizations":{"vscode":{"extensions":["example.editor"]}}}]            | ["/0/name","/1/customizations/vscode"]         |

  # The whole label is checked, including later entries. Metadata has a stricter
  # descriptive-only field profile than the visible manifest, and cannot supply a user.
  @slice-2 @ID-MANIFEST-21
  Scenario Outline: Refuse malformed or operational image metadata before accepting the manifest
    Given the supplied devcontainer.metadata label is:
      """
      <label>
      """
    And host execution sentinels are installed
    When the manifest inspection contract runs with supplied metadata in JSON mode
    Then its JSON output reports "manifest.metadata" at "<path>" with exit 2
    And it exits with code 2
    And inspection has no host side effects

    Examples:
      | label                                                   | path                  |
      |                                                         |                       |
      | not-json                                                |                       |
      | {}                                                      |                       |
      | null                                                    |                       |
      | [null]                                                  | /0                    |
      | [/*comment*/{}]                                         |                       |
      | [{},]                                                   |                       |
      | [{"name":"one","name":"two"}]                          | /0/name               |
      | [{"name":"one","n\u0061me":"two"}]                     | /0/name               |
      | [{"customizations":{"vscode":{"settings":{"x":1,"x":2}}}}] | /0/customizations/vscode/settings/x |
      | [{"customizations":{"vscode":{"settings":{"a/b~c":1,"a\\u002fb~c":2}}}}] | /0/customizations/vscode/settings/a~1b~0c |
      | [{"name":42}]                                           | /0/name               |
      | [{"customizations":{"otherEditor":{}}}]                 | /0/customizations/otherEditor |
      | [{"image":"other"}]                                     | /0/image              |
      | [{"workspaceFolder":"/tmp"}]                            | /0/workspaceFolder    |
      | [{"containerEnv":{"MODE":"hidden"}}]                    | /0/containerEnv       |
      | [{"containerUser":"1000"}]                              | /0/containerUser      |
      | [{"remoteUser":"root"}]                                 | /0/remoteUser         |
      | [{"initializeCommand":"./slice-2-record-execution"}]     | /0/initializeCommand  |
      | [{"postCreateCommand":["./slice-2-record-execution"]}]    | /0/postCreateCommand  |
      | [{"mounts":[]}]                                         | /0/mounts             |
      | [{"runArgs":[]}]                                        | /0/runArgs            |
      | [{"privileged":true}]                                   | /0/privileged         |
      | [{"name":"display"},{"capAdd":["SYS_ADMIN"]}]             | /1/capAdd             |
      | [{"unknown":false}]                                     | /0/unknown            |

  # Exercise diagnostics at the same validator/presenter boundary as the other label
  # cases. No CLI environment variable or engine lookup supplies the hostile input.
  @slice-2 @ID-MANIFEST-30
  Scenario Outline: Supplied metadata diagnostics identify the label without echoing values
    Given the supplied devcontainer.metadata label is:
      """
      <text>
      """
    When the manifest inspection contract runs with supplied metadata in JSON mode
    Then its JSON output reports "manifest.metadata" at "<path>" with exit 2
    And its standard output does not say "slice-2-private-marker"
    And its error output does not say "slice-2-private-marker"
    When the manifest inspection contract runs with supplied metadata in plain mode
    Then its error output says "devcontainer.metadata"
    And its error output says "itos-container:"
    And its standard output does not say "slice-2-private-marker"
    And its error output does not say "slice-2-private-marker"

    Examples:
      | text                                                 | path                 |
      | [{"initializeCommand":"echo slice-2-private-marker"}] | /0/initializeCommand |
      | [{"name":"slice-2-private-marker"                     |                      |

  # Show all independent mistakes, not only the first map iteration's result.
  @slice-2 @ID-MANIFEST-22
  Scenario: Report independent manifest problems together with typed paths and one refusal code
    Given the manifest member "containerEnv" has JSON value {"PATH":"","APP_MODE":false}
    And the manifest member "remoteUser" has JSON value "0"
    And the manifest member "privileged" has JSON value false
    When itos-container runs with "doctor --manifest-only --json"
    Then its JSON output reports "manifest.value" at "/containerEnv/APP_MODE" with exit 2
    And its JSON output reports "manifest.reserved" at "/containerEnv/PATH" with exit 2
    And its JSON output reports "manifest.unsupported" at "/privileged" with exit 2
    And its JSON output reports "manifest.identity" at "/remoteUser" with exit 2
    And its JSON output gives each problem a rule, a message and a fix
    And it exits with code 2

  # A key can contain a slash or tilde without naming a child field. Escaped pointer
  # segments keep the refusal's subject unambiguous for scripts and remediation tools.
  @slice-2 @ID-MANIFEST-25
  Scenario Outline: Keep diagnostic paths distinct from literal slashes and tildes in keys
    Given the manifest member "<field>" has JSON value <value>
    When itos-container runs with "doctor --manifest-only --json"
    Then its JSON output reports "manifest.unsupported" at "<path>" with exit 2
    And it exits with code 2

    Examples:
      | field                  | value                 | path                                |
      | customizations/vscode  | {}                    | /customizations~1vscode              |
      | customizations~1vscode | {}                    | /customizations~01vscode             |
      | customizations         | {"vs/code~1":{}}       | /customizations/vs~1code~01          |

  # Synthetic marker only. Parser errors must not echo a source line, and semantic
  # errors must not dump the rejected command or environment value. The PATH
  # example is a rejected literal value, never a change to the controller's environment.
  @slice-2 @ID-MANIFEST-23
  Scenario Outline: Diagnose rejected manifest input without disclosing raw values
    Given the manifest inspection input "<source>" has the exact text:
      """
      <text>
      """
    When itos-container runs with "doctor --manifest-only --json"
    Then its JSON output reports "<rule>" at "<path>" with exit 2
    And its standard output does not say "slice-2-private-marker"
    And its error output does not say "slice-2-private-marker"
    And it exits with code 2

    Examples:
      | source   | text                                                            | rule                 | path                 |
      | manifest | {"name":"slice-2-private-marker"                                 | manifest.syntax      |                      |
      | manifest | {"initializeCommand":"echo slice-2-private-marker"}               | manifest.unsupported | /initializeCommand   |
      | manifest | {"containerEnv":{"PATH":"slice-2-private-marker"}}               | manifest.reserved    | /containerEnv/PATH   |

  # A separate plain-output path must not reintroduce parser source snippets or rejected
  # values. Check the diagnostic's subject before its exit code: an unknown option also
  # exits 2, but cannot identify this file. Existing containment steps suffice;
  # the wording around those subjects remains free to change. Use the same exact-input
  # fixture as ID-MANIFEST-23, with no new setup behavior or assertion step definitions.
  @slice-2 @ID-MANIFEST-26
  Scenario Outline: Plain manifest refusals identify their source without disclosing raw values
    Given the manifest inspection input "<source>" has the exact text:
      """
      <text>
      """
    When itos-container runs with "doctor --manifest-only"
    Then its error output says "<subject>"
    And its error output says "itos-container:"
    And its standard output does not say "slice-2-private-marker"
    And its error output does not say "slice-2-private-marker"
    And it exits with code 2

    Examples:
      | source   | text                                                  | subject                        |
      | manifest | {"name":"slice-2-private-marker"                       | .devcontainer/devcontainer.json |
      | manifest | {"initializeCommand":"echo slice-2-private-marker"}    | .devcontainer/devcontainer.json |
      | manifest | {"containerEnv":{"PATH":"slice-2-private-marker"}}    | .devcontainer/devcontainer.json |

  @slice-2 @ID-MANIFEST-24
  Scenario: Plain doctor output identifies the manifest and explains unused editor data
    Given the manifest member "customizations" has JSON value {"vscode":{"extensions":["example.editor"]}}
    And host execution sentinels are installed
    When itos-container runs with "doctor --manifest-only"
    Then its standard output says ".devcontainer/devcontainer.json"
    And its standard output says "customizations.vscode"
    And its standard output says "unused"
    And inspection has no host side effects
