@phase-1
Feature: Inspect the restricted single-worker manifest
  Doctor reads a development-environment request without executing checkout code.
  A valid manifest is not approval or proof that the host can run an isolated worker.
  Sandbox permissions still come from the strict offline policy and host-owned approval.

  # Slice-2 specifies inspection only. Follow docs/charter.md, the restricted-manifest
  # section of docs/plans/container-pilot.md, docs/CONFIG.md and docs/CLI.md.
  # Implement doctor as its own vertical slice, following internal/count; reusable
  # manifest validation belongs outside command domains for doctor/approve/run to share.
  # Do not add behavior to count or introduce a Dev Containers CLI dependency.
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
  # Discovery is relative to the effective checkout root supplied by the extension seam:
  # exactly one of .devcontainer/devcontainer.json and .devcontainer.json must exist.
  # Both is an error even for identical bytes or an invalid candidate; no precedence,
  # parent search, recursive search, YAML policy override, or unrestricted fallback.
  # A candidate and every component below the root must be real directories/a regular
  # file, never symlinks (even in-tree or dangling ones) or special files. Check the
  # candidates without executing helpers. Root identity itself is slice-3's contract.
  #
  # Supported fields are case-sensitive and exhaustive:
  # - image: required string, exactly the policy's validated digest-pinned worker image.
  #   No image from metadata, host variables, a tag, or an implicit default. Reuse the
  #   policy's image-reference validation; do not write another reference parser here.
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
  # - containerUser and remoteUser: optional strings, each exactly the policy's positive
  #   decimal worker UID, without leading zeroes, names, or :group suffixes. Both default
  #   to that UID; neither can select a second identity or override an image/user check.
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
  # Image metadata is read from the exact cached worker image through the engine port,
  # never by starting it. An absent devcontainer.metadata label is allowed. If present,
  # its string must be strict JSON (no comments/trailing commas), an array of objects,
  # without duplicate decoded keys at any depth. Only name and customizations.vscode,
  # constrained as above, are allowed in each object. They are reported as unused and
  # never merged with checkout settings. Even operational fields supported in the visible
  # manifest (image, workspaceFolder, containerEnv, containerUser, remoteUser) are refused
  # in metadata, as are every hook and unsupported permission. [] and [{}] are allowed.
  # Inspect all entries, not just the first. A missing/unreadable cached image is slice-3's
  # readiness failure, never permission to assume metadata is absent or pull/build an image.
  #
  # Public JSON fragment on successful manifest inspection:
  # manifest = {valid:true, path:<relative candidate>, effective:{name, image,
  # workspaceFolder, containerEnv, containerUser, remoteUser}, unused:[<JSON pointers>],
  # image_metadata:{present:<bool>, unused:[<label JSON pointers>]}}.
  # The six effective fields always include defaults. unused pointers are sorted;
  # report /customizations/vscode once, and /<index>/name or /<index>/customizations/vscode
  # for metadata. Ignored values never enter effective. Other doctor checks and overall
  # success, including approval readiness, belong to their slices: valid manifest
  # assertions do not equate this fragment to permission to run or require overall ok=true.
  #
  # On manifest refusal, schema=1, ok=false, manifest.valid=false and no manifest.effective.
  # Each problem has rule, exit, nonempty message/fix, and path (a decoded-key JSON pointer;
  # the root is ""). File problems have file=<relative candidate>; discovery problems have
  # file="". Image-label problems instead have image=<policy image> and
  # label="devcontainer.metadata". No raw input values/lines/commands appear in diagnostics.
  # Configuration refusals below all have exit=2; together they return 2. Do not stop at
  # the first independent semantic problem. No problem order is a second public API.
  # Human output explains the candidate/field and how to correct it, and explicitly says
  # editor customizations/accepted metadata are unused. JSON is one stdout object;
  # logs stay on stderr. Tests check typed problems rather than freezing error prose.
  #
  # New fixture steps required by this spec (none exist in features/steps_test.go yet):
  # "a manifest inspection checkout" creates an ordinary temporary checkout with a
  # slice-1-valid offline policy, worker UID/GID 1000:1000, and the image below. It writes
  # .devcontainer/devcontainer.json containing only that image. A stateful local Engine
  # API fake advertises compatible cached image content with no metadata label, through
  # slice-3's supported local endpoint selection. It cannot reach a real Docker socket.
  # Use the real binary/adapters, not a test-only flag, production-code import or canned
  # doctor result. Resolve policy/endpoint fixture spellings from those sibling specs;
  # this file does not introduce an alternate policy schema or remote-engine exception.
  # Clear inherited engine/account variables and place HOME, XDG and engine configuration
  # in the fixture's temporary directories; never inspect a caller's login or engine state.
  # The fixture starts with no approval; this suite does not write host approval records.
  # It records checkout/control-state bytes and modes, engine mutations, and invocations
  # of PATH traps for docker, devcontainer and checkout helpers, all in its temp directory.
  # Trap programs only record invocation; never delegate to the host's tools.
  #
  # Fixture image (synthetic, served only by the local fake; never pull it):
  # registry.example/worker@sha256:1111111111111111111111111111111111111111111111111111111111111111
  # All remaining Given steps only write/remove/move fixture files, set raw member/label
  # bytes or create directories/symlinks/special-file fixtures. They never call doctor or
  # assert validation. A manifest "member ... has JSON value" edits that single member;
  # "manifest text is" preserves the exact doc string, including duplicate keys and escapes.
  # The unsafe-path fixture supplies valid bytes behind links, and a dangling-link case;
  # outside targets remain inside the test's temporary parent, never a real host secret.
  # "host execution sentinels" installs harmless recording scripts/traps and snapshots
  # state AFTER setup, without running any checkout script. It also seeds .git/hooks and
  # editor tasks with those scripts, so an accidental helper call is observable.
  #
  # New Then steps parse actual stdout, never inspect code. "reports a valid manifest"
  # checks one schema=1 object and manifest.valid/path; "JSON value at ... is" compares a
  # decoded value exactly. "reports ... at ... with exit ..." checks ok=false, the invalid
  # manifest, absence of effective, and a complete matching problem including file or
  # image/label subjects. "inspection has no host side effects" compares recorded bytes/
  # modes, absent execution markers, no trap invocations and no mutating engine requests;
  # read-only inspection requests may occur. Merely checking that a container is absent
  # would miss a pull/build or a started-then-removed resource.
  #
  # Reuse the existing run, run-with-environment, exit and output-containment steps.
  # All scenarios are non-engine black-box tests against the local fake, runnable without
  # Docker. They prove validation/inspection, not runtime isolation. Implementers commit
  # the missing steps red first, keep these @wip in that commit, then remove @wip for
  # failing-then-passing runs. Today doctor is unknown: every scenario below must first
  # fail its manifest-specific Then assertion, not setup, undefined steps or exit 2 alone.

  Background:
    Given a manifest inspection checkout

  # Discovery and syntax. Both allowed locations resolve the same defaults; comments
  # and trailing commas must not require a second parser or rewrite the source file.
  @wip @slice-2 @ID-MANIFEST-01
  Scenario Outline: Discover the sole supported manifest and accept JSON with comments
    Given the policy selects worker identity "<identity>"
    And the manifest is located only at "<file>"
    And the manifest text is:
      """
      {
        // One cached worker, never a build recipe.
        "image": "registry.example/worker@sha256:1111111111111111111111111111111111111111111111111111111111111111",
        /* Deliberately use trailing commas in an object and an array. */
        "customizations": {"vscode": {"extensions": ["example.editor",],},},
      }
      """
    And host execution sentinels are installed
    When itos-container runs with "doctor --json"
    Then its JSON output reports a valid manifest at "<file>"
    And its JSON value at "/manifest/effective" is:
      """
      {"name":null,"image":"registry.example/worker@sha256:1111111111111111111111111111111111111111111111111111111111111111","workspaceFolder":"/workspace","containerEnv":{},"containerUser":"<uid>","remoteUser":"<uid>"}
      """
    And its JSON value at "/manifest/image_metadata" is:
      """
      {"present":false,"unused":[]}
      """
    And inspection has no host side effects

    Examples:
      | file                            | identity  | uid  |
      | .devcontainer/devcontainer.json  | 1000:1000 | 1000 |
      | .devcontainer.json               | 1000:1000 | 1000 |
      | .devcontainer/devcontainer.json  | 1234:2345 | 1234 |

  @wip @slice-2 @ID-MANIFEST-02
  Scenario: Missing candidates do not trigger discovery elsewhere or a default worker
    Given neither manifest candidate exists
    And valid decoy manifests exist at "devcontainer.json", ".devcontainer/devcontainer.jsonc" and ".devcontainer/other/devcontainer.json"
    And a valid decoy manifest exists in the checkout's parent directory
    When itos-container runs with "doctor --json"
    Then its JSON output reports "manifest.missing" at "" with exit 2
    And it exits with code 2

  @wip @slice-2 @ID-MANIFEST-03
  Scenario Outline: Refuse two manifest candidates without choosing precedence
    Given a second manifest candidate exists with "<contents>"
    When itos-container runs with "doctor --json"
    Then its JSON output reports "manifest.ambiguous" at "" with exit 2
    And it exits with code 2

    Examples:
      | contents                           |
      | the same valid bytes as the first  |
      | malformed JSON                     |

  # Reject a FIFO without opening it for a blocking read. The step runner must have a
  # bounded command timeout so an incorrect implementation fails instead of hanging CI.
  @wip @slice-2 @ID-MANIFEST-04
  Scenario Outline: Refuse unsafe manifest candidates without following or executing them
    Given the manifest candidate has the layout "<layout>"
    And host execution sentinels are installed
    When itos-container runs with "doctor --json"
    Then its JSON output reports "manifest.path" at "" with exit 2
    And it exits with code 2
    And inspection has no host side effects

    Examples:
      | layout                                      |
      | file symlink to an in-checkout file           |
      | file symlink outside the checkout            |
      | dangling file symlink                        |
      | .devcontainer directory symlink              |
      | directory at .devcontainer/devcontainer.json  |
      | FIFO at .devcontainer/devcontainer.json       |

  @wip @slice-2 @ID-MANIFEST-05
  Scenario Outline: Reject malformed JSON and JSON5 extensions before interpreting fields
    Given the manifest text is:
      """
      <text>
      """
    When itos-container runs with "doctor --json"
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
  @wip @slice-2 @ID-MANIFEST-06
  Scenario Outline: Refuse duplicate decoded keys at every depth
    Given the manifest text is:
      """
      <text>
      """
    When itos-container runs with "doctor --json"
    Then its JSON output reports "manifest.duplicate" at "<path>" with exit 2
    And it exits with code 2

    Examples:
      | text                                                                    | path                                      |
      | {"image":"one","image":"two"}                                           | /image                                    |
      | {"image":"one","\u0069mage":"two"}                                      | /image                                    |
      | {"containerEnv":{"MODE":"one","M\u004fDE":"two"}}                        | /containerEnv/MODE                        |
      | {"customizations":{"vscode":{"settings":{"a":[{"x":1,"x":2}]}}}}         | /customizations/vscode/settings/a/0/x      |

  # A recognized unsafe field is not made acceptable by false, [] or {}. Unknown fields
  # fail too, including misspellings and fields from the unrestricted devcontainer spec.
  @wip @slice-2 @ID-MANIFEST-07
  Scenario Outline: Refuse unsupported execution and permission fields
    Given the manifest member "<field>" has JSON value <value>
    When itos-container runs with "doctor --json"
    Then its JSON output reports "manifest.unsupported" at "/<field>" with exit 2
    And it exits with code 2

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

  @wip @slice-2 @ID-MANIFEST-08
  Scenario Outline: Validate the types and values of supported manifest fields
    Given the manifest member "<field>" has JSON value <value>
    When itos-container runs with "doctor --json"
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

  @wip @slice-2 @ID-MANIFEST-09
  Scenario Outline: Refuse an image that does not exactly match the policy's pinned worker
    Given the manifest member "image" has JSON value <image>
    When itos-container runs with "doctor --json"
    Then its JSON output reports "manifest.image" at "/image" with exit 2
    And it exits with code 2

    Examples:
      | image                                                                                                     |
      | null                                                                                                      |
      | 42                                                                                                        |
      | ""                                                                                                        |
      | "registry.example/worker:latest"                                                                          |
      | "registry.example/worker@sha256:1111"                                                                     |
      | "registry.example/worker@sha256:2222222222222222222222222222222222222222222222222222222222222222"             |
      | "registry.example/other@sha256:1111111111111111111111111111111111111111111111111111111111111111"              |

  @wip @slice-2 @ID-MANIFEST-10
  Scenario: Require an explicit image instead of choosing a default
    Given the manifest has no "image" member
    When itos-container runs with "doctor --json"
    Then its JSON output reports "manifest.image" at "/image" with exit 2
    And it exits with code 2

  # Workspace and identity are constraints on the one policy-selected worker, not ways
  # to choose host paths, extra mounts, an elevated identity or a second user.
  @wip @slice-2 @ID-MANIFEST-11
  Scenario Outline: Refuse workspace locations outside the fixed checkout layout
    Given the manifest member "workspaceFolder" has JSON value <folder>
    When itos-container runs with "doctor --json"
    Then its JSON output reports "manifest.workspace" at "/workspaceFolder" with exit 2
    And it exits with code 2

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

  @wip @slice-2 @ID-MANIFEST-12
  Scenario Outline: Refuse a workspace directory reached through a symlink or a file
    Given the checkout workspace entry "src" has layout "<layout>"
    And the manifest member "workspaceFolder" has JSON value "/workspace/src"
    And host execution sentinels are installed
    When itos-container runs with "doctor --json"
    Then its JSON output reports "manifest.workspace" at "/workspaceFolder" with exit 2
    And it exits with code 2
    And inspection has no host side effects

    Examples:
      | layout                          |
      | symlink to an in-checkout directory |
      | symlink outside the checkout     |
      | regular file                    |

  @wip @slice-2 @ID-MANIFEST-13
  Scenario Outline: Neither manifest user can change the policy's non-root identity
    Given the manifest member "<field>" has JSON value <user>
    When itos-container runs with "doctor --json"
    Then its JSON output reports "manifest.identity" at "/<field>" with exit 2
    And it exits with code 2

    Examples:
      | field         | user        |
      | containerUser | "root"      |
      | containerUser | "0"         |
      | containerUser | "1001"      |
      | containerUser | "1000:0"    |
      | containerUser | "01000"     |
      | remoteUser    | "root"      |
      | remoteUser    | "0"         |
      | remoteUser    | "1001"      |
      | remoteUser    | "worker"    |
      | remoteUser    | "1000:1000" |

  # These are literal, synthetic data, not credentials. A host marker is deliberately
  # different from the written dollar expression and must never enter effective settings.
  @wip @slice-2 @ID-MANIFEST-14
  Scenario: Preserve literal environment values and resolve an existing checkout subdirectory
    Given a checkout directory "src" exists
    And the manifest member "name" has JSON value "worker display"
    And the manifest member "workspaceFolder" has JSON value "/workspace/src"
    And the manifest member "containerUser" has JSON value "1000"
    And the manifest member "remoteUser" has JSON value "1000"
    And the manifest member "containerEnv" has JSON value {"APP_MODE":"offline","EMPTY":"","TEXT":"$SLICE_2_HOST $(touch slice-2-ran) `touch slice-2-ran` // literal /* literal */","LINES":"first\nsecond"}
    And host execution sentinels are installed
    When itos-container runs with "doctor --json" and the environment "SLICE_2_HOST=host-only-marker"
    Then its JSON output reports a valid manifest at ".devcontainer/devcontainer.json"
    And its JSON value at "/manifest/effective" is:
      """
      {"name":"worker display","image":"registry.example/worker@sha256:1111111111111111111111111111111111111111111111111111111111111111","workspaceFolder":"/workspace/src","containerEnv":{"APP_MODE":"offline","EMPTY":"","TEXT":"$SLICE_2_HOST $(touch slice-2-ran) `touch slice-2-ran` // literal /* literal */","LINES":"first\nsecond"},"containerUser":"1000","remoteUser":"1000"}
      """
    And its standard output does not say "host-only-marker"
    And its error output does not say "host-only-marker"
    And inspection has no host side effects

  @wip @slice-2 @ID-MANIFEST-15
  Scenario Outline: Reject reserved environment namespaces regardless of case or empty values
    Given the manifest member "containerEnv" has JSON value {"<name>":""}
    When itos-container runs with "doctor --json"
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

  @wip @slice-2 @ID-MANIFEST-16
  Scenario Outline: Refuse variable-substitution syntax instead of expanding host or container values
    Given the manifest member "<field>" has JSON value <value>
    When itos-container runs with "doctor --json" and the environment "SLICE_2_HOST=host-only-marker"
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
  @wip @slice-2 @ID-MANIFEST-17
  Scenario Outline: Refuse every lifecycle hook while its runtime semantics remain unspecified
    Given the manifest member "<field>" has JSON value <value>
    And host execution sentinels are installed
    When itos-container runs with "doctor --json"
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
  @wip @slice-2 @ID-MANIFEST-18
  Scenario: Report recognized editor customizations as unused without executing their data
    Given the manifest member "customizations" has JSON value {"vscode":{"extensions":["example.editor"],"settings":{"example.command":"./slice-2-record-execution","example.variable":"${localEnv:SLICE_2_HOST}"}}}
    And host execution sentinels are installed
    When itos-container runs with "doctor --json" and the environment "SLICE_2_HOST=host-only-marker"
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

  @wip @slice-2 @ID-MANIFEST-19
  Scenario Outline: Refuse unknown editor namespaces and malformed recognized customizations
    Given the manifest member "customizations" has JSON value <value>
    When itos-container runs with "doctor --json"
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

  # The visible name must remain null even when metadata supplies one. A syntactically
  # acceptable label is never a second source of effective settings or commands.
  @wip @slice-2 @ID-MANIFEST-20
  Scenario Outline: Inspect permitted image metadata and report it as unused without merging
    Given the cached fixture image's devcontainer.metadata label is:
      """
      <label>
      """
    And host execution sentinels are installed
    When itos-container runs with "doctor --json"
    Then its JSON output reports a valid manifest at ".devcontainer/devcontainer.json"
    And its JSON value at "/manifest/effective/name" is:
      """
      null
      """
    And its JSON value at "/manifest/image_metadata" is:
      """
      {"present":true,"unused":<unused>}
      """
    And inspection has no host side effects

    Examples:
      | label                                                                                               | unused                                        |
      | []                                                                                                  | []                                            |
      | [{}]                                                                                                | []                                            |
      | [{"name":"image display"},{"customizations":{"vscode":{"extensions":["example.editor"]}}}]            | ["/0/name","/1/customizations/vscode"]         |

  # The whole label is checked, including later entries. Metadata has a stricter
  # descriptive-only field profile than the visible manifest, and cannot supply a user.
  @wip @slice-2 @ID-MANIFEST-21
  Scenario Outline: Refuse malformed or operational image metadata before accepting the manifest
    Given the cached fixture image's devcontainer.metadata label is:
      """
      <label>
      """
    And host execution sentinels are installed
    When itos-container runs with "doctor --json"
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

  # Show all independent mistakes, not only the first map iteration's result.
  @wip @slice-2 @ID-MANIFEST-22
  Scenario: Report independent manifest problems together with typed paths and one refusal code
    Given the manifest member "containerEnv" has JSON value {"PATH":"","APP_MODE":false}
    And the manifest member "remoteUser" has JSON value "0"
    And the manifest member "privileged" has JSON value false
    When itos-container runs with "doctor --json"
    Then its JSON output reports "manifest.value" at "/containerEnv/APP_MODE" with exit 2
    And its JSON output reports "manifest.reserved" at "/containerEnv/PATH" with exit 2
    And its JSON output reports "manifest.unsupported" at "/privileged" with exit 2
    And its JSON output reports "manifest.identity" at "/remoteUser" with exit 2
    And its JSON output gives each problem a rule, a message and a fix
    And it exits with code 2

  # Synthetic marker only. Parser errors must not echo a source line, and semantic
  # errors must not dump the rejected command, environment or label value.
  @wip @slice-2 @ID-MANIFEST-23
  Scenario Outline: Diagnose rejected manifest input without disclosing raw values
    Given the manifest inspection input "<source>" has the exact text:
      """
      <text>
      """
    When itos-container runs with "doctor --json"
    Then its JSON output reports "<rule>" at "<path>" with exit 2
    And its standard output does not say "slice-2-private-marker"
    And its error output does not say "slice-2-private-marker"
    And it exits with code 2

    Examples:
      | source   | text                                                            | rule                 | path                 |
      | manifest | {"name":"slice-2-private-marker"                                 | manifest.syntax      |                      |
      | manifest | {"initializeCommand":"echo slice-2-private-marker"}               | manifest.unsupported | /initializeCommand   |
      | metadata | [{"initializeCommand":"echo slice-2-private-marker"}]             | manifest.metadata    | /0/initializeCommand |
      | metadata | [{"name":"slice-2-private-marker"                                 | manifest.metadata    |                      |

  @wip @slice-2 @ID-MANIFEST-24
  Scenario: Plain doctor output identifies the manifest and explains unused editor data
    Given the manifest member "customizations" has JSON value {"vscode":{"extensions":["example.editor"]}}
    And the cached fixture image's devcontainer.metadata label is:
      """
      [{"name":"image display"}]
      """
    And host execution sentinels are installed
    When itos-container runs with "doctor"
    Then its standard output says ".devcontainer/devcontainer.json"
    And its standard output says "customizations.vscode"
    And its standard output says "devcontainer.metadata"
    And its standard output says "unused"
    And inspection has no host side effects
