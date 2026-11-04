@phase-1
Feature: Strict offline sandbox policy
  A policy requests permissions; parsing it cannot approve or start a sandbox.
  doctor --policy-only inspects itos-container.yaml in the effective working
  directory, without searching parents or reading core ITOS_CONFIG as policy.
  It validates only policy data, not manifest contents, host readiness or approval.
  Ordinary doctor applies the same policy validation before those later checks.

  # Specification decisions for slice-1, bounded by docs/charter.md,
  # docs/plans/container-pilot.md, docs/CONFIG.md and docs/CLI.md.
  #
  # Version 1 is a closed object with precisely these keys, in display order:
  # - version: required integer, exactly 1, no default. Numerically integral YAML
  #   numbers such as 1.0 mean the same number; strings are never coerced.
  # - manifest: required nonempty string, no default or discovery fallback.
  #   Example: .devcontainer/devcontainer.json. It is relative to the policy file,
  #   uses / separators, and is preserved literally, including spaces and Unicode.
  #   Refuse an absolute/drive-qualified path, any backslash, an empty, . or ..
  #   component, or any ASCII control character (U+0000..001F or U+007F).
  #   Do not clean, trim, expand variables/tilde, evaluate commands or open the
  #   referenced manifest during --policy-only. A literal path is not a mount grant.
  # - network: optional object, default {mode: offline}; its only key is mode,
  #   an optional string with the sole value/default offline. {} has that default.
  # Null is a value, never omission. No image, identity, resource, mount, account,
  # ingress, egress, proxy, command, include or extension keys exist in version 1.
  # Fixed runtime hardening is not configurable through an unknown policy key.
  # A new key needs a new policy version; no forward-compatible x- exception.
  # Publish this data contract as docs/schemas/itos-container-v1.schema.json when
  # implementing, with closed objects and the same requirements and defaults.
  # YAML syntax restrictions below supplement that JSON Schema, not vice versa.
  #
  # Read one UTF-8 YAML document using JSON-compatible values and YAML 1.2 core
  # scalar meanings. Comments, quoted keys/values, flow/block collections and
  # block strings are spellings, not execution. Non-specific ! means as if absent.
  # Permit JSON-type tags (!!map, !!seq, !!str, !!int, !!float, !!bool, !!null)
  # only for values of that type; reject custom/non-JSON tags and nonfinite numbers.
  # Do not inherit timestamp objects or YAML 1.1 boolean coercions. A plain date,
  # yes or on is a string. Structural checks visit every mapping/list, even below
  # an unknown field, without resolving aliases or merging mappings.
  #
  # Select go.yaml.in/yaml/v3 v3.0.6, checked on 2026-11-04 with the module proxy
  # and https://pkg.go.dev/go.yaml.in/yaml/v3@v3.0.6. The YAML organization maintains
  # the stable v3 branch for security fixes; its node API retains keys, tags,
  # anchors and locations for strict validation before typed decoding.
  # https://github.com/yaml/go-yaml/blob/main/README.md documents that maintenance
  # policy and v4's still-unreleased API. Prefer this stable parser to a handwritten
  # YAML parser; its permissive decoding is not our validation contract. Pin it in
  # go.mod/go.sum during implementation and preserve its MIT/Apache-2.0 notices.
  #
  # doctor is a new vertical slice, following internal/count's command/domain/port/
  # adapter layout. Reusable policy validation belongs in its own shared engine
  # package with domain-owned ports and fakes, outside doctor/approve/run domains.
  # YAML nodes, file access, JSON, rule IDs and exit codes stay outside the domain.
  # No Docker library, socket, real engine or runtime isolation test is needed here.
  # Safe checkout identity, symlink/substitution defenses and engine identity belong
  # to slice-3; manifest contents/metadata belong to slice-2. These regular-file
  # fixtures settle lexical paths only, not secure filesystem identity or approval.
  #
  # --json and inherited ITOS_JSON=1 request exactly one stdout object. Explicit
  # --no-json wins over the environment. Success has schema:1, ok:true,
  # scope:"policy", problems:[] and policy containing the complete effective object.
  # Failure has schema:1, ok:false, problems and no policy. --policy-only failures
  # also have scope:"policy"; ordinary doctor's other output remains its own spec.
  # Each problem has rule, exit, nonempty message/fix and flat file/path members.
  # file is "itos-container.yaml"; path is an RFC 6901 JSON Pointer ("" at root).
  # For a non-string key, path identifies the containing object, not the bad key.
  # Escape ~ and / in pointer components. Array indices are zero based.
  #
  # Stable rules (all exit 2 except policy.unreadable, exit 3):
  # policy.missing              required policy file absent
  # policy.unreadable           policy exists but cannot be read, including a directory
  # policy.empty                no document content, including comments only
  # policy.syntax               malformed YAML/invalid tagged scalar
  # policy.multiple_documents   more than one document, even an empty trailing one
  # policy.duplicate_key        repeated decoded string key
  # policy.non_string_key       mapping key is not a string
  # policy.anchor               an anchor declaration, even unused
  # policy.alias                an alias reference
  # policy.merge_key            <<, quoted or unquoted, is never a field or merge
  # policy.tag                  custom or non-JSON tag
  # policy.non_json_number      NaN/infinity
  # policy.required             required field absent
  # policy.type                 wrong JSON type (fractional version included)
  # policy.version              an integer version other than 1
  # policy.unknown_key          key absent from this version's closed schema
  # policy.network_mode         string mode other than offline
  # policy.path                 invalid lexical manifest path
  #
  # Stop at unreadable, empty or malformed input, or multiple documents. Otherwise
  # collect structural problems throughout the tree before schema validation.
  # Recognize merge keys before generic key/tag checks, reporting only merge_key
  # for that key; an alias is reported once without visiting its target.
  # Structural errors prevent schema projection. Then require an object and a valid
  # supported version before interpreting the remaining fields. For version 1,
  # collect independent schema problems; don't cascade into a wrongly typed value.
  # Never return a partial effective policy on failure. Problem order is not API.
  # The invocation's exit is the highest problem exit; policy success is exit 0.
  # Plain output identifies the policy-only scope and effective defaults. Failures
  # go to stderr with the itos-container: prefix, location and corrective advice.
  # Never include source lines, offending scalar values or raw parser errors in
  # diagnostics/logs. Synthetic markers below are not credentials.
  #
  # Implementer handoff: existing steps cover process runs (with environment),
  # exits, output containment, one/two-line files and empty folders. New steps here
  # are plainly new: policy docstrings/empty bytes, inspection-host fixtures,
  # working-directory/core-config selection, effective-policy/problem assertions, snapshots
  # and execution/engine recorder assertions. Write those steps in a test commit
  # while all scenarios remain @wip, then remove @wip only for the local red run.
  # The current CLI refuses doctor with usage/exit 2; the first policy assertion
  # or expected successful exit must fail, not setup or an undefined step.

  Background:
    # New Given: create private host-state directories outside the scenario tree
    # and a loopback connection recorder, and give only that endpoint to the child
    # through DOCKER_HOST. Clear competing engine/context overrides. No real engine
    # or socket is used; this setup makes no assertion about the product.
    Given policy inspection has isolated host state and a recording engine endpoint

  # Missing network data defaults closed, but nothing defaults a policy or version.
  @wip @slice-1 @ID-POLICY-01
  Scenario: minimal policy exposes every effective default without approving a run
    Given the policy file contains:
      """
      version: 1
      manifest: .devcontainer/devcontainer.json
      """
    When itos-container runs with "doctor --policy-only --json"
    Then it exits with code 0
    And its policy JSON contains this effective policy:
      """
      {"version":1,"manifest":".devcontainer/devcontainer.json","network":{"mode":"offline"}}
      """
    And the engine recorder has no connections

  @wip @slice-1 @ID-POLICY-02
  Scenario Outline: permitted YAML spellings retain the same offline policy
    Given the policy file contains:
      """
      <document>
      """
    When itos-container runs with "doctor --policy-only --json"
    Then it exits with code 0
    And its policy JSON contains this effective policy:
      """
      {"version":1,"manifest":".devcontainer/devcontainer.json","network":{"mode":"offline"}}
      """

    Examples:
      | document                                                                                                                             |
      | {version: 1, manifest: .devcontainer/devcontainer.json, network: {mode: offline}}                                                       |
      | {"version": 1, "manifest": ".devcontainer/devcontainer.json", "network": {}}                                                            |
      | {manifest: '.devcontainer/devcontainer.json', version: 1} # order does not authorize anything                                           |
      | {version: 1.0, manifest: .devcontainer/devcontainer.json}                                                                               |
      | {version: 1e0, manifest: .devcontainer/devcontainer.json}                                                                               |
      | ! {version: ! 1, manifest: ! .devcontainer/devcontainer.json, network: ! {mode: ! offline}}                                             |
      | !!map {version: !!int 1, manifest: !!str .devcontainer/devcontainer.json, network: !!map {mode: !!str offline}}                          |

  @wip @slice-1 @ID-POLICY-03
  Scenario: plain policy inspection names its scope and effective defaults
    Given the policy file contains:
      """
      version: 1
      manifest: .devcontainer/devcontainer.json
      """
    When itos-container runs with "doctor --policy-only"
    Then it exits with code 0
    And its standard output says "Policy only"
    And its standard output says "network.mode: offline"
    And its standard output says "manifest: .devcontainer/devcontainer.json"
    And its standard output says "Host readiness and approval were not checked"

  @wip @slice-1 @ID-POLICY-04
  Scenario: the extension JSON request is honored and an explicit flag can decline it
    Given the policy file contains:
      """
      version: 1
      manifest: .devcontainer/devcontainer.json
      """
    When itos-container runs with "doctor --policy-only" and the environment "ITOS_JSON=1"
    Then it exits with code 0
    And its policy JSON contains this effective policy:
      """
      {"version":1,"manifest":".devcontainer/devcontainer.json","network":{"mode":"offline"}}
      """
    When itos-container runs with "doctor --policy-only --no-json" and the environment "ITOS_JSON=1"
    Then it exits with code 0
    And its standard output says "Policy only"
    And its standard output says "Host readiness and approval were not checked"

  @wip @slice-1 @ID-POLICY-05
  Scenario: doctor help explains the limited validation mode without reading policy
    When itos-container runs with "doctor --help"
    Then its standard output says "--policy-only"
    And it exits with code 0
    And its standard output says "itos-container.yaml"
    And its standard output says "schema"
    And its standard output says "scope"
    And its standard output says "Exit codes:"
    And its standard output says "0  policy validation succeeded"
    And its standard output says "2  invalid policy or command line"
    And its standard output says "3  policy cannot be read"
    And the engine recorder has no connections

  @wip @slice-1 @ID-POLICY-06
  Scenario: a missing policy never becomes an unrestricted default
    When itos-container runs with "doctor --policy-only --json"
    Then its policy JSON reports exactly these problems:
      | rule           | path | exit |
      | policy.missing |      | 2    |
    And it exits with code 2

  # These are literal fixture writes, not discovery performed by a Given. The
  # child directory is the effective directory core itos would dispatch into.
  # The new working-directory Given changes only the harness's next process cwd.
  # The core-config Given sets ITOS_CONFIG to the absolute fixture filename,
  # matching the extension protocol, without loading that file as configuration.
  @wip @slice-1 @ID-POLICY-07
  Scenario: policy lookup neither searches parents nor substitutes core configuration
    Given the policy file contains:
      """
      version: 1
      manifest: .devcontainer/devcontainer.json
      """
    And an empty folder "child"
    And the file "child/itos.yaml" contains "version: 1", "manifest: .devcontainer/devcontainer.json"
    And the core configuration environment names "child/itos.yaml"
    And the command working directory is "child"
    When itos-container runs with "doctor --policy-only --json"
    Then its policy JSON reports exactly these problems:
      | rule           | path | exit |
      | policy.missing |      | 2    |
    And it exits with code 2

  @wip @slice-1 @ID-POLICY-08
  Scenario: a policy path that is a directory reports an unreadable requirement
    Given an empty folder "itos-container.yaml"
    When itos-container runs with "doctor --policy-only --json"
    Then its policy JSON reports exactly these problems:
      | rule              | path | exit |
      | policy.unreadable |      | 3    |
    And it exits with code 3

  # The empty-file Given writes zero bytes; it does not call the command or assert.
  @wip @slice-1 @ID-POLICY-09
  Scenario: an empty policy cannot supply a version by default
    Given the policy file is empty
    When itos-container runs with "doctor --policy-only --json"
    Then its policy JSON reports exactly these problems:
      | rule         | path | exit |
      | policy.empty |      | 2    |
    And it exits with code 2

  @wip @slice-1 @ID-POLICY-10
  Scenario Outline: comments and document markers alone are not policy data
    Given the policy file contains:
      """
      <document>
      """
    When itos-container runs with "doctor --policy-only --json"
    Then its policy JSON reports exactly these problems:
      | rule         | path | exit |
      | policy.empty |      | 2    |
    And it exits with code 2

    Examples:
      | document                   |
      | # there is no policy here  |
      | ---                        |
      |                            |

  @wip @slice-1 @ID-POLICY-11
  Scenario Outline: malformed YAML is a configuration error
    Given the policy file contains:
      """
      <document>
      """
    When itos-container runs with "doctor --policy-only --json"
    Then its policy JSON reports exactly these problems:
      | rule          | path | exit |
      | policy.syntax |      | 2    |
    And it exits with code 2

    Examples:
      | document                                                                        |
      | {version: 1, manifest: [                                                         |
      | {version: !!int not_an_integer, manifest: .devcontainer/devcontainer.json}         |

  @wip @slice-1 @ID-POLICY-12
  Scenario Outline: a second YAML document is refused even when it is empty
    Given the policy file contains:
      """
      version: 1
      manifest: .devcontainer/devcontainer.json
      ---
      <second>
      """
    When itos-container runs with "doctor --policy-only --json"
    Then its policy JSON reports exactly these problems:
      | rule                      | path | exit |
      | policy.multiple_documents |      | 2    |
    And it exits with code 2

    Examples:
      | second                   |
      | version: 1               |
      | # empty second document  |

  @wip @slice-1 @ID-POLICY-13
  Scenario Outline: policy data must be an object with explicit required fields
    Given the policy file contains:
      """
      <document>
      """
    When itos-container runs with "doctor --policy-only --json"
    Then its policy JSON reports exactly these problems:
      | rule   | path   | exit |
      | <rule> | <path> | 2    |
    And it exits with code 2

    Examples:
      | document                                       | rule            | path      |
      | null                                           | policy.type     |           |
      | []                                             | policy.type     |           |
      | offline                                        | policy.type     |           |
      | {}                                             | policy.required | /version  |
      | {manifest: .devcontainer/devcontainer.json}     | policy.required | /version  |
      | {version: 1}                                   | policy.required | /manifest |

  @wip @slice-1 @ID-POLICY-14
  Scenario Outline: values are checked without coercion or null defaults
    Given the policy file contains:
      """
      <document>
      """
    When itos-container runs with "doctor --policy-only --json"
    Then its policy JSON reports exactly these problems:
      | rule        | path   | exit |
      | policy.type | <path> | 2    |
    And it exits with code 2

    Examples:
      | document                                                                         | path          |
      | {version: '1', manifest: .devcontainer/devcontainer.json}                          | /version      |
      | {version: true, manifest: .devcontainer/devcontainer.json}                         | /version      |
      | {version: null, manifest: .devcontainer/devcontainer.json}                         | /version      |
      | {version: 1.5, manifest: .devcontainer/devcontainer.json}                          | /version      |
      | {version: [], manifest: .devcontainer/devcontainer.json}                           | /version      |
      | {version: 1, manifest: null}                                                      | /manifest     |
      | {version: 1, manifest: 42}                                                        | /manifest     |
      | {version: 1, manifest: [.devcontainer/devcontainer.json]}                          | /manifest     |
      | {version: 1, manifest: a.json, network: null}                                      | /network      |
      | {version: 1, manifest: a.json, network: offline}                                   | /network      |
      | {version: 1, manifest: a.json, network: []}                                        | /network      |
      | {version: 1, manifest: a.json, network: {mode: null}}                              | /network/mode |
      | {version: 1, manifest: a.json, network: {mode: false}}                             | /network/mode |
      | {version: 1, manifest: a.json, network: {mode: [offline]}}                         | /network/mode |

  @wip @slice-1 @ID-POLICY-15
  Scenario Outline: unsupported policy versions are not guessed or downgraded
    Given the policy file contains:
      """
      version: <version>
      manifest: .devcontainer/devcontainer.json
      """
    When itos-container runs with "doctor --policy-only --json"
    Then its policy JSON reports exactly these problems:
      | rule           | path     | exit |
      | policy.version | /version | 2    |
    And it exits with code 2

    Examples:
      | version |
      | 0       |
      | -1      |
      | 2       |

  # Empty collections and false do not make unsupported permissions permissible.
  @wip @slice-1 @ID-POLICY-16
  Scenario Outline: unknown and deferred fields are refused at every schema level
    Given the policy file contains:
      """
      <document>
      """
    When itos-container runs with "doctor --policy-only --json"
    Then its policy JSON reports exactly these problems:
      | rule               | path   | exit |
      | policy.unknown_key | <path> | 2    |
    And it exits with code 2

    Examples:
      | document                                                            | path             |
      | {version: 1, manifest: a.json, typo: true}                             | /typo            |
      | {version: 1, manifest: a.json, x-note: ignored}                        | /x-note          |
      | {version: 1, manifest: a.json, mounts: []}                             | /mounts          |
      | {version: 1, manifest: a.json, privileged: false}                      | /privileged      |
      | {version: 1, manifest: a.json, accounts: []}                           | /accounts        |
      | {version: 1, manifest: a.json, ingress: []}                            | /ingress         |
      | {version: 1, manifest: a.json, proxy: {}}                              | /proxy           |
      | {version: 1, manifest: a.json, include: other.yaml}                    | /include         |
      | {version: 1, manifest: a.json, network: {egress: []}}                   | /network/egress  |
      | {version: 1, manifest: a.json, network: {x-note: ignored}}             | /network/x-note  |
      | {version: 1, manifest: a.json, 'a~/b': true}                           | /a~0~1b          |

  # Structural checks also inspect lists inside an unknown field before decoding
  # could discard it; error paths must identify the repeated decoded key.
  @wip @slice-1 @ID-POLICY-17
  Scenario Outline: duplicate keys cannot silently select a value
    Given the policy file contains:
      """
      <document>
      """
    When itos-container runs with "doctor --policy-only --json"
    Then its policy JSON reports exactly these problems:
      | rule                 | path   | exit |
      | policy.duplicate_key | <path> | 2    |
    And it exits with code 2

    Examples:
      | document                                                                        | path          |
      | {version: 1, version: 1, manifest: a.json}                                         | /version      |
      | {version: 1, 'version': 2, manifest: a.json}                                       | /version      |
      | {version: 1, manifest: a.json, network: {mode: offline, mode: bridge}}              | /network/mode |
      | {version: 1, manifest: a.json, extra: [{name: one, name: two}]}                     | /extra/0/name |

  @wip @slice-1 @ID-POLICY-18
  Scenario Outline: non-string mapping keys are refused without stringifying them
    Given the policy file contains:
      """
      <document>
      """
    When itos-container runs with "doctor --policy-only --json"
    Then its policy JSON reports exactly these problems:
      | rule                  | path   | exit |
      | policy.non_string_key | <path> | 2    |
    And it exits with code 2

    Examples:
      | document                                                    | path     |
      | {version: 1, manifest: a.json, 7: value}                       |          |
      | {version: 1, manifest: a.json, network: {true: value}}          | /network |
      | {version: 1, manifest: a.json, extra: [{[a, b]: value}]}        | /extra/0 |

  @wip @slice-1 @ID-POLICY-19
  Scenario: anchors aliases and merge keys are all rejected before schema projection
    Given the policy file contains:
      """
      version: &format 1
      manifest: &unused .devcontainer/devcontainer.json
      '<<': {mode: offline}
      network:
        <<: {mode: offline}
        mode: *format
      """
    When itos-container runs with "doctor --policy-only --json"
    Then its policy JSON reports exactly these problems:
      | rule             | path          | exit |
      | policy.anchor    | /version      | 2    |
      | policy.anchor    | /manifest     | 2    |
      | policy.merge_key | /<<           | 2    |
      | policy.merge_key | /network/<<   | 2    |
      | policy.alias     | /network/mode | 2    |
    And it exits with code 2

  @wip @slice-1 @ID-POLICY-20
  Scenario Outline: tags and numbers cannot introduce values outside JSON data
    Given the policy file contains:
      """
      <document>
      """
    When itos-container runs with "doctor --policy-only --json"
    Then its policy JSON reports exactly these problems:
      | rule   | path   | exit |
      | <rule> | <path> | 2    |
    And it exits with code 2

    Examples:
      | document                                                          | rule                   | path         |
      | {version: 1, manifest: !include a.json}                             | policy.tag             | /manifest    |
      | {version: 1, manifest: !!binary YQ==}                               | policy.tag             | /manifest    |
      | {version: 1, manifest: !!timestamp 2026-11-04}                       | policy.tag             | /manifest    |
      | {version: 1, manifest: a.json, extra: [!!set {a: null}]}             | policy.tag             | /extra/0     |
      | {version: .nan, manifest: a.json}                                  | policy.non_json_number | /version     |
      | {version: 1, manifest: a.json, extra: [.inf]}                        | policy.non_json_number | /extra/0     |
      | {version: 1, manifest: a.json, extra: [-.inf]}                       | policy.non_json_number | /extra/0     |

  @wip @slice-1 @ID-POLICY-21
  Scenario Outline: no alternate network mode enables a wider milestone
    Given the policy file contains:
      """
      version: 1
      manifest: .devcontainer/devcontainer.json
      network: {mode: '<mode>'}
      """
    When itos-container runs with "doctor --policy-only --json"
    Then its policy JSON reports exactly these problems:
      | rule                | path          | exit |
      | policy.network_mode | /network/mode | 2    |
    And it exits with code 2

    Examples:
      | mode    |
      | bridge  |
      | host    |
      | proxy   |
      | none    |
      | Offline |
      |         |

  # A date or yes/on without quotes remains a string under the core scalar rules.
  # These filenames request no host expansion; policy-only does not open them.
  @wip @slice-1 @ID-POLICY-22
  Scenario Outline: relative manifest paths remain literal data
    Given the policy file contains:
      """
      version: 1
      manifest: <value>
      """
    And the policy inspection files are snapshotted
    When itos-container runs with "doctor --policy-only --json" and the environment "SLICE_1_DIR=elsewhere"
    Then it exits with code 0
    And its policy JSON contains this effective policy:
      """
      {"version":1,"manifest":"<path>","network":{"mode":"offline"}}
      """
    And the policy inspection files are unchanged
    And the engine recorder has no connections

    Examples:
      | value                                     | path                                  |
      | 'config files/entorno-ñ.json'               | config files/entorno-ñ.json            |
      | '${SLICE_1_DIR}/devcontainer.json'          | ${SLICE_1_DIR}/devcontainer.json       |
      | '~/devcontainer.json'                      | ~/devcontainer.json                   |
      | '$(touch slice-1-executed).json'            | $(touch slice-1-executed).json         |
      | 2026-11-04                                 | 2026-11-04                            |
      | on                                        | on                                    |
      | yes                                       | yes                                   |

  # The block scalar's strip chomping supplies a path without a trailing newline.
  @wip @slice-1 @ID-POLICY-28
  Scenario: a block string is another spelling of a literal manifest path
    Given the policy file contains:
      """
      version: 1
      manifest: |-
        config files/devcontainer.json
      """
    When itos-container runs with "doctor --policy-only --json"
    Then it exits with code 0
    And its policy JSON contains this effective policy:
      """
      {"version":1,"manifest":"config files/devcontainer.json","network":{"mode":"offline"}}
      """

  @wip @slice-1 @ID-POLICY-23
  Scenario Outline: invalid lexical paths are refused rather than cleaned or expanded
    Given the policy file contains:
      """
      version: 1
      manifest: <value>
      """
    When itos-container runs with "doctor --policy-only --json"
    Then its policy JSON reports exactly these problems:
      | rule        | path      | exit |
      | policy.path | /manifest | 2    |
    And it exits with code 2

    Examples:
      | value                             |
      | ''                                |
      | '/tmp/manifest.json'              |
      | 'C:/manifest.json'                |
      | 'C:manifest.json'                 |
      | 'config\\manifest.json'            |
      | '../manifest.json'                |
      | 'config/../manifest.json'          |
      | './manifest.json'                 |
      | 'config/./manifest.json'           |
      | 'config//manifest.json'            |
      | 'config/'                         |
      | "config/\u0000manifest.json"       |
      | "config/\\nmanifest.json"          |
      | "config/\u007fmanifest.json"       |

  @wip @slice-1 @ID-POLICY-24
  Scenario: independent schema problems are returned together without a partial policy
    Given the policy file contains:
      """
      version: 1
      manifest: null
      network:
        mode: bridge
        egress: []
      accounts: []
      """
    When itos-container runs with "doctor --policy-only --json"
    Then its policy JSON reports exactly these problems:
      | rule                | path            | exit |
      | policy.type         | /manifest       | 2    |
      | policy.network_mode | /network/mode   | 2    |
      | policy.unknown_key  | /network/egress | 2    |
      | policy.unknown_key  | /accounts       | 2    |
    And it exits with code 2

  @wip @slice-1 @ID-POLICY-25
  Scenario Outline: invalid input diagnostics identify the problem without echoing values
    Given the policy file contains:
      """
      <document>
      """
    When itos-container runs with "doctor --policy-only --json"
    Then its policy JSON reports exactly these problems:
      | rule   | path   | exit |
      | <rule> | <path> | 2    |
    And it exits with code 2
    And its standard output does not say "SYNTHETIC_POLICY_MARKER"
    And its error output does not say "SYNTHETIC_POLICY_MARKER"
    When itos-container runs with "doctor --policy-only"
    Then it exits with code 2
    And its error output says "itos-container:"
    And its error output says "itos-container.yaml"
    And its error output does not say "SYNTHETIC_POLICY_MARKER"
    And its standard output does not say "SYNTHETIC_POLICY_MARKER"

    Examples:
      | document                                                                             | rule                | path          |
      | {version: 1, manifest: a.json, network: {mode: SYNTHETIC_POLICY_MARKER}}                 | policy.network_mode | /network/mode |
      | {version: 1, manifest: a.json, unexpected: SYNTHETIC_POLICY_MARKER}                      | policy.unknown_key  | /unexpected   |
      | {version: 1, manifest: [SYNTHETIC_POLICY_MARKER                                         | policy.syntax       |               |

  # New Given: create only harness-owned helper executables on the child PATH for
  # git, docker, devcontainer, sh, bash and npm. Each records execution and exits;
  # none delegates to a host tool. Also place a checkout generator and a manifest
  # with initializeCommand writing that recorder. Snapshot checkout/host-state
  # file bytes and entries after setup, with recorder storage outside both trees.
  # The Then steps compare observations; they never set up or validate policy.
  @wip @slice-1 @ID-POLICY-26
  Scenario: policy-only inspection leaves checkout and host state unchanged and runs no helpers
    Given the policy file contains:
      """
      version: 1
      manifest: .devcontainer/devcontainer.json
      network: {mode: offline}
      """
    And inspection helper sentinels are installed
    And the policy inspection files are snapshotted
    When itos-container runs with "doctor --policy-only --json"
    Then it exits with code 0
    And its policy JSON contains this effective policy:
      """
      {"version":1,"manifest":".devcontainer/devcontainer.json","network":{"mode":"offline"}}
      """
    And the policy inspection files are unchanged
    And no inspection helper sentinel was executed
    And the engine recorder has no connections

  # Ordinary doctor's success/readiness contract belongs to slice-3. Invalid policy
  # must nevertheless reach these same typed refusals before manifest or engine work.
  # This assertion checks the policy problem without imposing policy-only's scope.
  @wip @slice-1 @ID-POLICY-27
  Scenario: ordinary doctor refuses invalid policy before inspecting other requirements
    Given the policy file contains:
      """
      version: 1
      manifest: .devcontainer/devcontainer.json
      network: {mode: bridge}
      """
    And inspection helper sentinels are installed
    And the policy inspection files are snapshotted
    When itos-container runs with "doctor --json"
    Then its doctor JSON reports exactly these policy problems:
      | rule                | path          | exit |
      | policy.network_mode | /network/mode | 2    |
    And it exits with code 2
    And the policy inspection files are unchanged
    And no inspection helper sentinel was executed
    And the engine recorder has no connections
