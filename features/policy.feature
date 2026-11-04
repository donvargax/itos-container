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
  # Before YAML decoding, refuse source lines starting with % (optionally after
  # a byte-order mark) as one root policy.syntax problem. This reserves directive
  # lines, including %TAG and %YAML, so tag prefixes cannot amplify parser
  # allocations before the representation limits run. Check the whole input,
  # including later documents, with LF, CR, CRLF, NEL, LS and PS line breaks.
  # This source restriction also applies inside multiline quoted scalars; indent
  # such continuation lines to keep literal % text. Inline and indented percent
  # text, comments and the permitted built-in tags remain ordinary YAML data.
  #
  # Use main's T-14 pin, go.yaml.in/yaml/v3 v3.0.5, reviewed on 2026-11-04 at
  # https://pkg.go.dev/go.yaml.in/yaml/v3@v3.0.5. The YAML organization maintains
  # the stable v3 branch for security fixes; its node API retains keys, tags,
  # anchors and locations for strict validation before typed decoding.
  # https://github.com/yaml/go-yaml/blob/main/README.md documents that maintenance
  # policy and v4's still-unreleased API. Prefer this stable parser to a handwritten
  # YAML parser; its permissive decoding is not our validation contract. Pin it in
  # go.mod/go.sum during implementation and preserve its MIT/Apache-2.0 notices.
  #
  # T-14 lands before slice-1 continues: use its layout and import boundaries from
  # tasks/phase-1.yaml. These settled changes concern layout only; the closed
  # version 1 policy above stays unchanged, with no image or identity keys.
  # doctor lives in internal/slices/doctor/{command,domain,port,<adapter>}.
  # Reusable policy validation lives in internal/engine/policy/{domain,port,<adapter>},
  # shared by doctor/approve/run without one slice importing another's domain.
  # Move this branch's existing doctor and policy code and tests to those paths
  # when integrating T-14; do not leave a parallel implementation at the old paths.
  #
  # A slice imports no other slice and no adapter, including its own; cmd/ binds
  # adapters to ports. An adapter may import its own domain and port. Ports do no
  # I/O and import only ports. Domains use T-14's pure standard-library allowlist,
  # their own ports and shared engine domains/ports, never adapters or UI packages.
  # Each port has a port/porttest contract suite run against its fake and its
  # adapter; fakes and porttest packages are test-only, never production imports.
  # Doctor's failure classifier and policy presentation live beside the doctor
  # slice. cmd/ supplies the classifier to internal/cli, which imports no packages
  # from this repository. Apply this to the policy code already in internal/cli.
  # YAML nodes, file access, JSON, rule IDs and exit codes stay outside the domain.
  # No Docker library, socket, real engine or runtime isolation test is needed here.
  # Read the policy without following its final symbolic link and require the
  # opened file to be regular. Never wait for a FIFO writer or read a device.
  # Limit policy input to 65536 bytes (64 KiB), checking the opened file's size and
  # bounding the read even if it grows. Refuse links (including dangling links),
  # non-regular files and larger inputs as policy.unreadable before parsing.
  # Standard-library file APIs and bounded readers suffice; no new library is needed.
  # Checkout/engine identity and approval-time substitution defenses still belong
  # to slice-3; manifest contents/metadata belong to slice-2. This bounded policy
  # read does not settle secure checkout identity or approval.
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
  # policy.unreadable           unreadable, linked, non-regular or oversized policy file
  # policy.empty                no document content, including comments only
  # policy.syntax               malformed YAML/invalid tagged scalar/directive line
  # policy.multiple_documents   more than one document, even an empty trailing one
  # policy.complexity           representation exceeds depth, node or path budget
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
  # Stop at unreadable, empty or malformed input, or multiple documents. Before
  # structural validation, preflight the decoded representation with no per-node
  # allocation: at most 32 levels and 2048 nodes, counting the root at depth 1,
  # every mapping key/value and sequence element, and each alias reference once.
  # Do not count the document wrapper or follow alias targets. Also limit the sum
  # of representation path lengths to 65536 bytes, after RFC 6901 escaping.
  # The root path is empty. Each scalar mapping key adds its decoded spelling
  # (regardless of type) to both its key and value paths; a complex key adds no
  # component to either. Sequence elements add their decimal index. Count each
  # node's full path, including ancestors and slash separators, without building
  # path strings. This conservative budget bounds repeated diagnostic locations,
  # including aliases below long keys; scalar values do not extend paths.
  # Standard-library byte lengths and counters suffice; no dependency is needed.
  # Refuse any excess with exactly one policy.complexity problem at root, before allocating
  # diagnostic paths or converting values (including scalar validation).
  # Within those budgets, collect structural problems before schema validation.
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
  # Numeric validation must not expand powers of ten from untrusted exponents,
  # even below unknown fields. Preserve exact finite-number meanings regardless
  # of exponent magnitude; no rounding, overflow or new syntax cutoff is allowed.
  # Escape diagnostic locations on stderr with Go string escapes (without outer
  # quotes), including controls, backslashes and quotes. Keep printable Unicode
  # readable and JSON paths lossless after JSON decoding and RFC 6901 escaping.
  # Apply the same escaping to the manifest in successful human output, including
  # C1 controls and Unicode line separators. Preserve the decoded manifest in JSON.
  # The coordinator's parser-wide budgets supersede the previous requirement to
  # inspect arbitrarily deep trees. Keep the shared traversal path within those
  # budgets, preserving diagnostic locations. The tagged 6000-level review input
  # and earlier untagged 6000/8000-level inputs must be refused before that walk.
  # Prove these limits through the CLI's refusal rule and a bounded completion
  # deadline, with inputs at each limit still reaching structural/schema checks.
  # Do not assert heap size, GC traces or measured allocations: those depend on
  # the Go runtime and its environment, not the policy contract.
  #
  # Layout handoff: features/steps_test.go and features/policy_steps_test.go now
  # define every step below, and all 28 scenarios are live. Preserve their IDs,
  # examples, assertions and live tags through the move; no new behaviour or step
  # definitions are needed for this layout correction. Verify the same scenarios
  # before and after moving code, with T-14's checks enforcing package boundaries
  # and the shared porttest suites checking fake/adapter agreement.
  # The original red-first baseline was the CLI refusing doctor with usage/exit 2;
  # failures belonged at the first policy assertion or expected successful exit,
  # never at setup or an undefined step. That is history, not the current CLI.

  Background:
    # New Given: create private host-state directories outside the scenario tree
    # and a loopback connection recorder, and give only that endpoint to the child
    # through DOCKER_HOST. Clear competing engine/context overrides. No real engine
    # or socket is used; this setup makes no assertion about the product.
    Given policy inspection has isolated host state and a recording engine endpoint

  # Missing network data defaults closed, but nothing defaults a policy or version.
  @slice-1 @ID-POLICY-01
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

  @slice-1 @ID-POLICY-02
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

  @slice-1 @ID-POLICY-03
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

  @slice-1 @ID-POLICY-04
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

  @slice-1 @ID-POLICY-05
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

  @slice-1 @ID-POLICY-06
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
  @slice-1 @ID-POLICY-07
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

  @slice-1 @ID-POLICY-08
  Scenario: a policy path that is a directory reports an unreadable requirement
    Given an empty folder "itos-container.yaml"
    When itos-container runs with "doctor --policy-only --json"
    Then its policy JSON reports exactly these problems:
      | rule              | path | exit |
      | policy.unreadable |      | 3    |
    And it exits with code 3

  # The empty-file Given writes zero bytes; it does not call the command or assert.
  @slice-1 @ID-POLICY-09
  Scenario: an empty policy cannot supply a version by default
    Given the policy file is empty
    When itos-container runs with "doctor --policy-only --json"
    Then its policy JSON reports exactly these problems:
      | rule         | path | exit |
      | policy.empty |      | 2    |
    And it exits with code 2

  @slice-1 @ID-POLICY-10
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

  @slice-1 @ID-POLICY-11
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

  @slice-1 @ID-POLICY-12
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

  @slice-1 @ID-POLICY-13
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

  @slice-1 @ID-POLICY-14
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

  @slice-1 @ID-POLICY-15
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
  @slice-1 @ID-POLICY-16
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
  @slice-1 @ID-POLICY-17
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

  @slice-1 @ID-POLICY-18
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

  @slice-1 @ID-POLICY-19
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

  @slice-1 @ID-POLICY-20
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

  @slice-1 @ID-POLICY-21
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
  @slice-1 @ID-POLICY-22
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
  @slice-1 @ID-POLICY-28
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

  @slice-1 @ID-POLICY-23
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

  @slice-1 @ID-POLICY-24
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

  @slice-1 @ID-POLICY-25
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

  @slice-1 @bug-1 @ID-POLICY-29
  Scenario Outline: diagnostic paths cannot inject terminal controls or extra lines
    Given the policy file contains:
      """
      version: 1
      manifest: a.json
      "bad\n\r\t\b\f\u0000\u001b[31m\u007f\u0085\u2028\u2029~/ñ\\\"": ignored
      """
    When itos-container runs with "doctor --policy-only <format>"
    Then its error output is exactly:
      """
      itos-container: itos-container.yaml/bad\n\r\t\b\f\x00\x1b[31m\x7f\u0085\u2028\u2029~0~1ñ\\\": Field is not supported; remove fields absent from the version 1 schema
      """
    And it exits with code 2
    When itos-container runs with "doctor --policy-only --json"
    Then its policy JSON has this problem path encoded as JSON:
      """
      "/bad\n\r\t\b\f\u0000\u001b[31m\u007f\u0085\u2028\u2029~0~1ñ\\\""
      """
    And it exits with code 2

    Examples:
      | format |
      |        |
      | --json |

  @slice-1 @ID-POLICY-30
  Scenario Outline: extreme finite exponents retain their exact schema classification
    Given the policy file contains:
      """
      <document>
      """
    When itos-container runs with "doctor --policy-only --json" and a 5 second deadline
    Then it finishes before the deadline
    And its policy JSON reports exactly these problems:
      | rule   | path   | exit |
      | <rule> | <path> | 2    |
    And it exits with code 2

    Examples:
      | document                                                                     | rule               | path     |
      | {version: 1e1000001, manifest: a.json}                                          | policy.version     | /version |
      | {version: 1e-1000001, manifest: a.json}                                         | policy.type        | /version |
      | {version: 1e999999999999999999999, manifest: a.json}                            | policy.version     | /version |
      | {version: 1e-999999999999999999999, manifest: a.json}                           | policy.type        | /version |
      | {version: 0e999999999999999999999, manifest: a.json}                            | policy.version     | /version |
      | {version: 1, manifest: a.json, extra: [!!float 1e999999999999999999999]}         | policy.unknown_key | /extra   |

  # This small input used to expand each number into a million-digit integer or
  # denominator before discarding the unknown list. The deadline bounds the child
  # process in the harness, so the assertion fails without hanging the test runner.
  @slice-1 @ID-POLICY-31
  Scenario Outline: repeated numeric exponents cannot amplify policy inspection work
    Given the policy file contains 1024 numeric list values spelled "<number>" below an unknown field
    When itos-container runs with "doctor --policy-only --json" and a 5 second deadline
    Then it finishes before the deadline
    And its policy JSON reports exactly these problems:
      | rule               | path   | exit |
      | policy.unknown_key | /extra | 2    |
    And it exits with code 2

    Examples:
      | number     |
      | 1e1000000  |
      | 1e-1000000 |

  # A normal target and a dangling target demonstrate no-follow without ever
  # exposing the unfixed reader to an unbounded device or a blocking FIFO.
  @slice-1 @bug-2 @ID-POLICY-32
  Scenario Outline: policy symbolic links are refused without reading their targets
    Given the file "target.yaml" contains "version: 1", "manifest: a.json"
    And the policy file is a symbolic link to "<target>"
    When itos-container runs with "doctor --policy-only --json" and a 5 second deadline
    Then it finishes before the deadline
    And its policy JSON reports exactly these problems:
      | rule              | path | exit |
      | policy.unreadable |      | 3    |
    And it exits with code 3

    Examples:
      | target       |
      | target.yaml  |
      | missing.yaml |

  @slice-1 @bug-2 @ID-POLICY-33
  Scenario: oversized policy data is refused before parsing
    Given a valid policy is padded with spaces to 65537 bytes
    When itos-container runs with "doctor --policy-only --json" and a 5 second deadline
    Then it finishes before the deadline
    And its policy JSON reports exactly these problems:
      | rule              | path | exit |
      | policy.unreadable |      | 3    |
    And it exits with code 3

  @slice-1 @bug-3 @ID-POLICY-34
  Scenario Outline: successful policy output escapes terminal controls and preserves JSON data
    Given the policy file contains:
      """
      version: 1
      manifest: "config/ñ\u009b2J\u009dtitle\u009c\u0085\u2028\u2029\".json"
      """
    When itos-container runs with "doctor <mode>"
    Then its standard output is exactly:
      """
      Policy only
      version: 1
      manifest: config/ñ\u009b2J\u009dtitle\u009c\u0085\u2028\u2029\".json
      network.mode: offline
      Host readiness and approval were not checked
      """
    And it exits with code 0
    When itos-container runs with "doctor <mode> --json"
    Then its policy JSON contains this effective policy:
      """
      {"version":1,"manifest":"config/ñ\u009b2J\u009dtitle\u009c\u0085\u2028\u2029\".json","network":{"mode":"offline"}}
      """
    And it exits with code 0

    Examples:
      | mode          |
      |               |
      | --policy-only |

  # Keep the review's hostile inputs, but observe only their bounded refusal.
  @slice-1 @bug-4 @ID-POLICY-35
  Scenario Outline: deeply nested unknown policy data is refused before traversal
    Given the policy file contains <depth> nested sequences below an unknown field
    When itos-container runs with "doctor --policy-only --json" and a 5 second deadline
    Then it finishes before the deadline
    And its policy JSON reports exactly these problems:
      | rule              | path | exit |
      | policy.complexity |      | 2    |
    And it exits with code 2
    And the engine recorder has no connections

    Examples:
      | depth |
      | 6000  |
      | 8000  |

  @slice-1 @ID-POLICY-36
  Scenario Outline: representation depth is bounded before structural validation
    Given the policy file has representation depth 33 using "<shape>"
    When itos-container runs with "doctor --policy-only --json" and a 5 second deadline
    Then it finishes before the deadline
    And its policy JSON reports exactly these problems:
      | rule              | path | exit |
      | policy.complexity |      | 2    |
    And it exits with code 2
    Given the policy file has representation depth 32 using "<shape>"
    When itos-container runs with "doctor --policy-only --json" and a 5 second deadline
    Then it finishes before the deadline
    And its policy JSON reports exactly these problems:
      | rule   | path   | exit |
      | <rule> | <path> | 2    |
    And it exits with code 2
    And the engine recorder has no connections

    Examples:
      | shape        | rule                  | path   |
      | sequences    | policy.unknown_key    | /extra |
      | mappings     | policy.unknown_key    | /extra |
      | complex keys | policy.non_string_key |        |

  @slice-1 @ID-POLICY-37
  Scenario Outline: representation node count is bounded including mapping keys
    Given the policy file has 2049 representation nodes using "<shape>"
    When itos-container runs with "doctor --policy-only --json" and a 5 second deadline
    Then it finishes before the deadline
    And its policy JSON reports exactly these problems:
      | rule              | path | exit |
      | policy.complexity |      | 2    |
    And it exits with code 2
    Given the policy file has 2048 representation nodes using "<shape>"
    When itos-container runs with "doctor --policy-only --json" and a 5 second deadline
    Then it finishes before the deadline
    And its policy JSON reports exactly these problems:
      | rule   | path   | exit |
      | <rule> | <path> | 2    |
    And it exits with code 2
    And the engine recorder has no connections

    Examples:
      | shape     | rule               | path   |
      | sequences | policy.unknown_key | /extra |
      | mappings  | policy.type        |        |

  @slice-1 @ID-POLICY-38
  Scenario: custom tags at every depth are refused before collecting diagnostic paths
    Given the policy file contains 6000 nested tagged sequences below an unknown field
    When itos-container runs with "doctor --policy-only --json" and a 5 second deadline
    Then it finishes before the deadline
    And its policy JSON reports exactly these problems:
      | rule              | path | exit |
      | policy.complexity |      | 2    |
    And it exits with code 2
    And the engine recorder has no connections

  @slice-1 @ID-POLICY-39
  Scenario Outline: tag directives cannot amplify node construction before policy limits
    Given the policy file reuses a 32768 byte tag prefix on <count> scalars in "<position>"
    When itos-container runs with "doctor --policy-only --json" and a 5 second deadline
    Then it finishes before the deadline
    And its policy JSON reports exactly these problems:
      | rule          | path | exit |
      | policy.syntax |      | 2    |
    And it exits with code 2
    And the engine recorder has no connections

    Examples:
      | count | position          |
      | 4678  | first document    |
      | 4678  | after initial BOM |
      | 4677  | second document   |

  @slice-1 @ID-POLICY-40
  Scenario Outline: directive lines are refused even without tag expansion
    Given the policy file contains:
      """
      <directive>
      ---
      version: 1
      manifest: a.json
      """
    When itos-container runs with "doctor --policy-only --json"
    Then its policy JSON reports exactly these problems:
      | rule          | path | exit |
      | policy.syntax |      | 2    |
    And it exits with code 2

    Examples:
      | directive                         |
      | %YAML 1.1                         |
      | %TAG !e! tag:example:              |
      | %TAG !! tag:yaml.org,2002:         |
      | %TAG ! tag:yaml.org,2002:          |

  # This is the review's 65147-byte, 2048-node shallow self-alias reproduction.
  # Check output length before comparing diagnostics so a regression reports a
  # small assertion failure instead of copying its amplified output into logs.
  @wip @slice-1 @ID-POLICY-41
  Scenario Outline: shallow aliases cannot amplify long diagnostic paths
    Given the policy file has 2045 self-aliases below a key of 59000 slashes
    When itos-container runs with "doctor --policy-only <format>" and a 5 second deadline
    Then its combined policy output is at most 4096 bytes
    And it finishes before the deadline
    And it exits with code 2
    And its error output is exactly:
      """
      itos-container: itos-container.yaml: Policy representation is too complex; reduce the policy to at most 32 levels, 2048 nodes and 65536 aggregate path bytes
      """
    And the engine recorder has no connections

    Examples:
      | format |
      |        |
      | --json |

  @wip @slice-1 @ID-POLICY-42
  Scenario Outline: aggregate path limits count escaped bytes and accept the boundary
    Given the policy file has an explicit key repeating "<text>" <count> times with suffix "xx"
    When itos-container runs with "doctor --policy-only --json" and a 5 second deadline
    Then its combined policy output is at most 4096 bytes
    And it finishes before the deadline
    And its policy JSON reports exactly these problems:
      | rule              | path | exit |
      | policy.complexity |      | 2    |
    And it exits with code 2
    Given the policy file has an explicit key repeating "<text>" <count> times with suffix "x"
    When itos-container runs with "doctor --policy-only --json" and a 5 second deadline
    Then it finishes before the deadline
    And its policy JSON reports exactly these problems:
      | rule            | path     | exit |
      | policy.required | /version | 2    |
    And it exits with code 2
    And the engine recorder has no connections

    Examples:
      | text | count |
      | a    | 32766 |
      | /    | 16383 |
      | ~    | 16383 |
      | ñ    | 16383 |

  # New Given: create only harness-owned helper executables on the child PATH for
  # git, docker, devcontainer, sh, bash and npm. Each records execution and exits;
  # none delegates to a host tool. Also place a checkout generator and a manifest
  # with initializeCommand writing that recorder. Snapshot checkout/host-state
  # file bytes and entries after setup, with recorder storage outside both trees.
  # The Then steps compare observations; they never set up or validate policy.
  @slice-1 @ID-POLICY-26
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
  @slice-1 @ID-POLICY-27
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
