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

  # Slice-25 added policy version 2, which made this table's example 2 untrue. Its feat
  # commit changed that example to 3 and said so; the rule checked here did not change.
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
      | 3       |

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

  # Slice-25: an exact host-name egress allowlist (charter, second milestone, step 1). A
  # policy only requests it: nothing here approves it, starts a proxy or opens a route.
  # Slice-26 binds it in approval, T-39 and T-40 build the proxy and its network, and
  # slice-27 runs through them. Every decision below belongs to this spec.
  #
  # A new key needs a new policy version (docs/CONFIG.md rule 7 and the decisions at the
  # top of this file), so the allowlist is version 2. Version 1 keeps its written meaning
  # in every command: ID-POLICY-16 and ID-POLICY-21 stay as they are, and offline approvals
  # keep their identity. Version 2 is version 1's closed object with a wider network:
  # - mode: optional string, offline (still the default) or allowlist, never case folded.
  # - allow: list of strings, required and nonempty in allowlist mode. In offline mode it
  #   is refused even when empty, so a forgotten mode cannot look like a granted list, and
  #   its entries are not interpreted. Strings rather than objects: per-destination
  #   settings such as TLS modes would need a new version anyway.
  # The effective network is {mode: offline} or {mode: allowlist, allow: [names]}. allow is
  # absent when offline, so the offline object is the same in both versions. Names appear
  # once each, in ascending byte order of their canonical spelling: reordering changes
  # nothing, and slice-26 can bind them as a set. Plain output gives one network.allow line
  # per name in that order. Canonical names are ASCII and need no terminal escaping.
  #
  # Each entry is checked in this order. The check is lexical: no DNS lookup, public-suffix
  # list or list of special names. What a listed name may resolve to belongs to the proxy
  # (contract AC-12), so a name such as host.internal is not refused here.
  # 1. At most 1024 bytes of decoded UTF-8, before IDNA, because punycode encoding is
  #    quadratic: a 64 KiB name of distinct characters took x/net/idna four to six seconds
  #    on 2026-11-09. A character mapping to one ASCII character takes at most 4 bytes, so
  #    any 253-byte name can be spelled in 1012; only ignored code points can reach the limit.
  # 2. idna.Lookup.ToASCII: lower case, UTS 46 non-transitional mapping (full-width forms,
  #    ideographic dots, ignored code points), STD3 host-name characters, hyphen and Bidi
  #    rules and valid A-labels. An error refuses the entry. Never show the error's text:
  #    it quotes the label.
  # 3. Remove one trailing dot, after mapping, so an ideographic final dot counts as well.
  # 4. At least two labels, each of 1 to 63 bytes, at most 253 bytes in all. Lookup checks
  #    none of this: it accepts .example.com, example.com.. and a 64-byte label, and it
  #    turns xn--.example into .example.
  # 5. The final label is not all digits, nor 0x followed only by hexadecimal digits.
  #    Address parsers read those as IPv4 (127.1, 0x7f.0.0.1, 1.2.3.0X4) and Lookup passes
  #    them. IPv6 already fails step 2 on its colons.
  # A look-alike keeps its own A-label, so it stays visibly different from what it imitates.
  # Nothing is trimmed: surrounding space is refused like any other character outside a name.
  #
  # New stable rules, all exit 2. Entry rules sit at /network/allow/<index>, the others at
  # /network/allow:
  # policy.host_name       entry is not a full DNS host name
  # policy.host_duplicate  entry's canonical name repeats an earlier entry's
  # policy.allow_empty     empty list in allowlist mode
  # policy.allow_offline   list present in offline mode
  # An absent list in allowlist mode is policy.required and a wrong type is policy.type. A
  # wrong mode, or a mode of the wrong type, reports only the mode, whatever the list beside
  # it holds: allow means nothing without a mode. Only canonical names can repeat, so a
  # refused entry is never a duplicate of another. Independent problems are collected as in
  # ID-POLICY-24, and diagnostics never echo a rejected value. The existing depth, node and
  # path budgets bound the list; it has no limit of its own.
  #
  # A problem's fix depends on the version read. For a version 2 file under doctor
  # --policy-only, no fix names version 1, and policy.network_mode's says "use offline or
  # allowlist mode". Version 1 keeps its words exactly (ID-POLICY-29, 75 and 77) and never
  # offers a mode it refuses. Where no supported version was read, the fix names both: "use
  # policy version 1 or 2" for an unsupported number, and "use the field type defined by
  # policy version 1 or 2" for a wrongly typed version or a root that is not an object.
  # Ordinary doctor says the same (ID-POLICY-77), though it refuses version 2 for now: these
  # words describe the format, and version 2's own refusal then says what to do next.
  #
  # Only doctor --policy-only and --manifest-only read version 2 for now. Ordinary doctor,
  # approve and run refuse a valid version 2 file with the existing policy.version at /version,
  # exit 2, whatever its mode, before the manifest, the engine or host state. Canonical settings
  # v1 (features/approval.feature) hardcode profile offline-v1 and network offline, so without
  # this refusal approve would record offline settings for a request that asks for network,
  # and an offline approval would appear to match it. On all three commands the refusal's
  # fix names doctor --policy-only, so a person can still see what the file asks for. A version
  # 2 file that fails validation is refused there for its own problems instead, as any invalid
  # policy is (ID-POLICY-78): the person mends the file, then learns where it can be read.
  # Slice-26 lifts the refusal for doctor and approve once approval can bind version 2, and
  # must keep run from starting a version 2 policy until slice-27 can.
  #
  # Library decision, checked 2026-11-09: golang.org/x/net/idna, pinned at golang.org/x/net
  # v0.60.0, BSD-3-Clause, maintained by the Go team; net/http uses the same Lookup profile
  # for host names. golang.org/x/tools v0.50.0 already puts v0.59.0 in the module graph, and
  # v0.60.0 requires the same versions, so no other pin moves; x/text v0.42.0 joins go.mod as
  # indirect. Not v0.59.0: govulncheck lists five HTTP/2 advisories against that module
  # version, fixed in v0.60.0. It still lists two against x/net/html (GO-2026-6866,
  # GO-2026-6860), a package nothing here imports, so no symbol is reached. Their fix,
  # v0.62.0, would also move x/sys, x/term, x/sync, x/telemetry and x/tools, and the idna
  # package is byte-identical in all three. Add x/net's and x/text's notices to
  # THIRD_PARTY_NOTICES.md. The library is outside the domain allowlist, so canonicalising
  # sits behind a policy port with an adapter at internal/engine/policy/idna, an I/O-free
  # porttest fake and the shared contract against both; cmd assembles it. Steps 1 and 3 to
  # 5, duplicates and sorting are pure domain code. port.Value must start retaining list
  # items, which it drops today. The charter's no-dependency rule (q-51) binds the proxy, and
  # this adapter is no part of it: canonical names are plain ASCII, so the proxy can compare
  # them without any IDNA of its own.
  #
  # No new slice: this adds to what exists. internal/engine/policy takes the rules, that
  # port and its adapter. The doctor slice's command presents the list, plain and JSON, and
  # cmd/itos-container/main.go assembles the adapter where it assembles the policy source.
  # The refusal goes in internal/engine/readiness/domain/inspect.go, after policy.Inspect:
  # ordinary doctor, approve and run share that call, and --policy-only and --manifest-only
  # read the policy through the doctor slice's inspection package instead.
  #
  # The implementing pull request also updates docs/CONFIG.md and docs/CLI.md for the new
  # field. That includes the opening paragraphs that call network egress deferred, which
  # p1-docs-admit-milestone-two-egress leaves to this item: they say instead that a policy can
  # request an allowlist, and that nothing approves or grants it yet. The doctor command's help
  # line stops saying "strict offline policy". The pull request publishes
  # docs/schemas/itos-container-v2.schema.json beside the unchanged v1 schema. It records a
  # decision that names and updates the one arc42 text this overrides, the internal/engine/policy
  # row's "Strict offline schema", as the charter's "Architecture is a hypothesis" asks. arc42's
  # "Proxy egress ... deferred" text stays: this slice builds no proxy, and T-38's record owns it.
  # Its feat commit also moves what pins version 2 as refused to version 3, and says so:
  # ID-POLICY-15's example 2, TestVersionIsRequiredAndExact in
  # internal/engine/policy/domain/policy_test.go and PoliciesContract's last fixture in
  # internal/slices/doctor/port/porttest/contract.go. Only a feat or a fix may change a
  # live scenario's example (the moves rule).
  #
  # Scenarios below reuse existing steps except these, which only write fixtures or read
  # output. Each of the first three writes version 2, manifest a.json and mode allowlist:
  # - "the policy file allows one host name of <count> distinct characters": one entry, that
  #   many consecutive code points from U+4E00 upward followed by .example.
  # - "the policy file allows "<name>" padded with soft hyphens to <size> bytes": one
  #   double-quoted entry, the name then raw U+00AD until its text is exactly that size.
  # - "the policy file allows <count> generated host names": entries n0.example,
  #   n1.example and so on, one a line.
  # - "the manifest inspection policy contains:" and "the checkout policy contains:" write
  #   the DocString over itos-container.yaml in the manifest or approval checkout.
  # - "manifest inspection is given the recording engine endpoint" sets DOCKER_HOST in the
  #   manifest world's environment to the recorder's address. manifestEnv removes the value
  #   policyEnv set, so without this no dial from --manifest-only could reach the recorder.
  # Red first: today every version 2 policy is refused as policy.version at /version, exit
  # 2, so each scenario fails at its first Then that reads the outcome; deadline and output
  # size steps pass. Where the final answer is that same refusal, the scenario runs
  # --policy-only first so that it can fail before the work. ID-POLICY-77 fails instead at
  # today's fix, which names only version 1. This spec's red evidence used scratch steps, so
  # the first test commit adds the steps above and repeats the red run.

  @slice-25 @ID-POLICY-43
  Scenario: an allowlist policy shows its effective host names and runs nothing
    Given the policy file contains:
      """
      version: 2
      manifest: .devcontainer/devcontainer.json
      network:
        mode: allowlist
        allow:
          - registry.example.org
          - API.Example.COM.
          - bücher.example
      """
    And inspection helper sentinels are installed
    And the policy inspection files are snapshotted
    When itos-container runs with "doctor --policy-only --json"
    Then it exits with code 0
    And its policy JSON contains this effective policy:
      """
      {"version":2,"manifest":".devcontainer/devcontainer.json","network":{"mode":"allowlist","allow":["api.example.com","registry.example.org","xn--bcher-kva.example"]}}
      """
    And the policy inspection files are unchanged
    And no inspection helper sentinel was executed
    And the engine recorder has no connections

  @slice-25 @ID-POLICY-44
  Scenario: plain inspection lists each canonical host name in order
    Given the policy file contains:
      """
      version: 2
      manifest: .devcontainer/devcontainer.json
      network:
        mode: allowlist
        allow:
          - registry.example.org
          - API.Example.COM.
          - bücher.example
      """
    When itos-container runs with "doctor --policy-only"
    Then its standard output is exactly:
      """
      Policy only
      version: 2
      manifest: .devcontainer/devcontainer.json
      network.mode: allowlist
      network.allow: api.example.com
      network.allow: registry.example.org
      network.allow: xn--bcher-kva.example
      Host readiness and approval were not checked
      """
    And it exits with code 0

  # The \u0430 row starts with a Cyrillic letter: a look-alike never merges with the Latin
  # name. faß pins non-transitional mapping, which keeps ß instead of turning it into ss.
  # The soft hyphen is an ignored code point. A numeric or hexadecimal label is refused only
  # in final place, and there 0x counts only with hexadecimal digits after it, so 0x1f.example
  # and a.0xg stay names. The three longest rows sit at the DNS limits: a 63-byte label, a
  # 253-byte name, and that name with a trailing dot, which does not count towards the 253.
  @slice-25 @ID-POLICY-45
  Scenario Outline: each host name is canonicalised before it is shown
    Given the policy file contains:
      """
      version: 2
      manifest: a.json
      network: {mode: allowlist, allow: [<spelling>]}
      """
    When itos-container runs with "doctor --policy-only --json"
    Then it exits with code 0
    And its policy JSON contains this effective policy:
      """
      {"version":2,"manifest":"a.json","network":{"mode":"allowlist","allow":["<canonical>"]}}
      """

    Examples:
      | spelling              | canonical             |
      | api.example.com       | api.example.com       |
      | API.Example.COM       | api.example.com       |
      | api.example.com.      | api.example.com       |
      | bücher.example        | xn--bcher-kva.example |
      | BÜCHER.Example.       | xn--bcher-kva.example |
      | XN--BCHER-KVA.example | xn--bcher-kva.example |
      | 日本。example。           | xn--wgv71a.example    |
      | faß.example           | xn--fa-hia.example    |
      | "\u0430pple.example" | xn--pple-43d.example  |
      | example。com           | example.com           |
      | ＡＰＩ.example.com       | api.example.com       |
      | "exa\u00ADmple.com"  | example.com           |
      | 123.example           | 123.example           |
      | a-b.example           | a-b.example           |
      | a.b                   | a.b                   |
      | 0x1f.example          | 0x1f.example          |
      | a.0xg                 | a.0xg                 |
      | aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa.example | aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa.example |
      | aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa.bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb.ccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc.ddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd | aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa.bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb.ccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc.ddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd |
      | aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa.bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb.ccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc.ddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd. | aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa.bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb.ccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc.ddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd |

  # The byte order puts a-b.example before a.example: - is 0x2d and . is 0x2e. A sort by
  # labels would differ. Numeric spellings of the version mean the same number, as at version 1.
  @slice-25 @ID-POLICY-46
  Scenario Outline: reordered and respelled lists have one effective allowlist
    Given the policy file contains:
      """
      <document>
      """
    When itos-container runs with "doctor --policy-only --json"
    Then it exits with code 0
    And its policy JSON contains this effective policy:
      """
      {"version":2,"manifest":"a.json","network":{"mode":"allowlist","allow":["a-b.example","a.example","b.example"]}}
      """

    Examples:
      | document                                                                                                                                           |
      | {version: 2, manifest: a.json, network: {mode: allowlist, allow: [a-b.example, a.example, b.example]}}                                                |
      | {version: 2, manifest: a.json, network: {mode: allowlist, allow: [b.example, a.example, a-b.example]}}                                                |
      | {manifest: a.json, network: {allow: [B.EXAMPLE., 'a.example', "A-B.example"], mode: allowlist}, version: 2} # order grants nothing                    |
      | {version: 2.0, manifest: a.json, network: {mode: allowlist, allow: [a.example, b.example, a-b.example]}}                                              |
      | {version: 20e-1, manifest: a.json, network: {mode: allowlist, allow: [a.example, b.example, a-b.example]}}                                            |
      | ! {version: ! 2, manifest: ! a.json, network: ! {mode: ! allowlist, allow: ! [! a.example, ! b.example, ! a-b.example]}}                               |
      | !!map {version: !!int 2, manifest: !!str a.json, network: !!map {mode: !!str allowlist, allow: !!seq [!!str a.example, !!str b.example, !!str a-b.example]}} |

  @slice-25 @ID-POLICY-47
  Scenario Outline: offline stays the default at version 2 and brings no list
    Given the policy file contains:
      """
      <document>
      """
    When itos-container runs with "doctor --policy-only --json"
    Then it exits with code 0
    And its policy JSON contains this effective policy:
      """
      {"version":2,"manifest":"a.json","network":{"mode":"offline"}}
      """
    When itos-container runs with "doctor --policy-only"
    Then it exits with code 0
    And its standard output says "network.mode: offline"
    And its standard output does not say "network.allow"

    Examples:
      | document                                                 |
      | {version: 2, manifest: a.json}                           |
      | {version: 2, manifest: a.json, network: {}}              |
      | {version: 2, manifest: a.json, network: {mode: offline}} |

  @slice-25 @ID-POLICY-48
  Scenario: the same network request stays refused at version 1
    Given the policy file contains:
      """
      version: 2
      manifest: a.json
      network: {mode: allowlist, allow: [api.example.com]}
      """
    When itos-container runs with "doctor --policy-only --json"
    Then it exits with code 0
    Given the policy file contains:
      """
      version: 1
      manifest: a.json
      network: {mode: allowlist, allow: [api.example.com]}
      """
    When itos-container runs with "doctor --policy-only --json"
    Then its policy JSON reports exactly these problems:
      | rule                | path           | exit |
      | policy.network_mode | /network/mode  | 2    |
      | policy.unknown_key  | /network/allow | 2    |
    And it exits with code 2

  # Canonicalising comes first, so a disguised spelling is judged by what it maps to.
  # The defect column only explains each row. The longest rows are one byte past the DNS
  # limits, and the ü row is a 60-character label whose A-label takes 66 bytes.
  @slice-25 @ID-POLICY-49
  Scenario Outline: only a full DNS host name enters the allowlist
    Given the policy file contains:
      """
      version: 2
      manifest: a.json
      network: {mode: allowlist, allow: [<value>]}
      """
    When itos-container runs with "doctor --policy-only --json"
    Then its policy JSON reports exactly these problems:
      | rule             | path             | exit |
      | policy.host_name | /network/allow/0 | 2    |
    And it exits with code 2

    Examples:
      | defect                               | value                     |
      | literal IPv4                         | '127.0.0.1'               |
      | IPv4 with a trailing dot             | '127.0.0.1.'              |
      | short IPv4                           | '127.1'                   |
      | hexadecimal IPv4                     | '0x7f.0.0.1'              |
      | capital hexadecimal final label      | '1.2.3.0X4'               |
      | IPv4 as one number                   | '2130706433'              |
      | IPv4 in full-width digits            | '１２７.０.０.１'           |
      | IPv4 with ideographic dots           | '1。2。3。4'               |
      | numeric final label                  | 'example.123'             |
      | hexadecimal final label              | 'example.0x1f'            |
      | bare hexadecimal prefix              | 'example.0x'              |
      | IPv6 loopback                        | '::1'                     |
      | bracketed IPv6                       | '[::1]'                   |
      | IPv6                                 | '2001:db8::1'             |
      | wildcard alone                       | '*'                       |
      | leading wildcard                     | '*.example.com'           |
      | trailing wildcard                    | 'example.*'               |
      | inner wildcard                       | 'ex*mple.com'             |
      | full-width wildcard                  | '＊.example.com'           |
      | question mark                        | '?.example.com'           |
      | suffix shortcut                      | '.example.com'            |
      | suffix shortcut with two dots        | '..example.com'           |
      | two trailing dots                    | 'example.com..'           |
      | empty inner label                    | 'api..example.com'        |
      | empty A-label                        | 'xn--.example'            |
      | single label                         | example                   |
      | top-level domain alone               | com                       |
      | localhost                            | localhost                 |
      | localhost with a trailing dot        | 'localhost.'              |
      | single label with an ideographic dot | 'example。'                |
      | port                                 | 'example.com:443'         |
      | another port                         | 'example.com:8443'        |
      | full-width colon and port            | 'example.com：443'         |
      | bracketed IPv6 and port              | '[::1]:443'               |
      | URL                                  | 'https://example.com'     |
      | URL with a path                      | 'https://example.com/'    |
      | path                                 | 'example.com/path'        |
      | scheme-relative URL                  | '//example.com'           |
      | user information                     | 'user@example.com'        |
      | query                                | 'example.com?x=1'         |
      | fragment                             | 'example.com#f'           |
      | percent encoding                     | '%65xample.com'           |
      | empty                                | ''                        |
      | one space                            | ' '                       |
      | one dot                              | '.'                       |
      | leading space                        | ' example.com'            |
      | trailing space                       | 'example.com '            |
      | inner space                          | 'exa mple.com'            |
      | two names in one entry               | 'a.example,b.example'     |
      | newline                              | "example.com\\n"          |
      | tab                                  | "exa\\tmple.com"          |
      | NUL                                  | "exa\u0000mple.com"       |
      | zero-width joiner                    | "exa\u200Dmple.com"       |
      | underscore                           | '_dmarc.example.com'      |
      | leading hyphen                       | '-example.com'            |
      | trailing hyphen                      | 'example-.com'            |
      | hyphens in places three and four     | 'ab--cd.example'          |
      | malformed A-label                    | 'xn--a.example'           |
      | A-label of a refused name            | 'xn--a-.example'          |
      | A-label prefix on Unicode            | 'xn--bücher.example'      |
      | mixed right-to-left label            | "\u05D0a.example"         |
      | 64-byte label                        | 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa.example' |
      | 254-byte name                        | 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa.bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb.ccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc.dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd' |
      | A-label over 63 bytes                | 'üüüüüüüüüüüüüüüüüüüüüüüüüüüüüüüüüüüüüüüüüüüüüüüüüüüüüüüüüüüü.example' |

  @slice-25 @ID-POLICY-50
  Scenario Outline: a name repeated after canonicalisation is refused at its later entry
    Given the policy file contains:
      """
      version: 2
      manifest: a.json
      network: {mode: allowlist, allow: [<names>]}
      """
    When itos-container runs with "doctor --policy-only --json"
    Then its policy JSON reports exactly these problems:
      | rule                  | path   | exit |
      | policy.host_duplicate | <path> | 2    |
    And it exits with code 2

    Examples:
      | names                                 | path             |
      | api.example.com, api.example.com      | /network/allow/1 |
      | api.example.com, API.EXAMPLE.COM.     | /network/allow/1 |
      | bücher.example, xn--bcher-kva.example | /network/allow/1 |
      | example.com, example。com              | /network/allow/1 |
      | example.com, "exa\u00ADmple.com"     | /network/allow/1 |
      | a.example, b.example, a.example       | /network/allow/2 |

  # A list in offline mode is refused whatever it holds: its entries are not interpreted.
  # Null is a value here as everywhere, never omission.
  @slice-25 @ID-POLICY-51
  Scenario Outline: the list and the mode must agree
    Given the policy file contains:
      """
      <document>
      """
    When itos-container runs with "doctor --policy-only --json"
    Then its policy JSON reports exactly these problems:
      | rule   | path           | exit |
      | <rule> | /network/allow | 2    |
    And it exits with code 2

    Examples:
      | document                                                                 | rule                 |
      | {version: 2, manifest: a.json, network: {mode: allowlist}}                | policy.required      |
      | {version: 2, manifest: a.json, network: {mode: allowlist, allow: []}}     | policy.allow_empty   |
      | {version: 2, manifest: a.json, network: {allow: [a.example]}}             | policy.allow_offline |
      | {version: 2, manifest: a.json, network: {allow: []}}                      | policy.allow_offline |
      | {version: 2, manifest: a.json, network: {mode: offline, allow: [a.example]}} | policy.allow_offline |
      | {version: 2, manifest: a.json, network: {mode: offline, allow: []}}       | policy.allow_offline |
      | {version: 2, manifest: a.json, network: {mode: offline, allow: ['*']}}    | policy.allow_offline |
      | {version: 2, manifest: a.json, network: {mode: offline, allow: null}}     | policy.allow_offline |

  # A wrong mode reports only the mode, however good or bad the list beside it, so nothing
  # may judge the list before the mode is known. The unquoted 127.1 is a YAML number, so it
  # fails on its type before any name check.
  @slice-25 @ID-POLICY-52
  Scenario Outline: version 2 network values keep their types and modes
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
      | document                                                                       | rule                | path             |
      | {version: 2, manifest: a.json, network: {mode: allowlist, allow: null}}          | policy.type         | /network/allow   |
      | {version: 2, manifest: a.json, network: {mode: allowlist, allow: a.example}}     | policy.type         | /network/allow   |
      | {version: 2, manifest: a.json, network: {mode: allowlist, allow: {a.example: true}}} | policy.type     | /network/allow   |
      | {version: 2, manifest: a.json, network: {mode: allowlist, allow: [null]}}        | policy.type         | /network/allow/0 |
      | {version: 2, manifest: a.json, network: {mode: allowlist, allow: [443]}}         | policy.type         | /network/allow/0 |
      | {version: 2, manifest: a.json, network: {mode: allowlist, allow: [127.1]}}       | policy.type         | /network/allow/0 |
      | {version: 2, manifest: a.json, network: {mode: allowlist, allow: [true]}}        | policy.type         | /network/allow/0 |
      | {version: 2, manifest: a.json, network: {mode: allowlist, allow: [[a.example]]}} | policy.type         | /network/allow/0 |
      | {version: 2, manifest: a.json, network: {mode: allowlist, allow: [{name: a.example}]}} | policy.type   | /network/allow/0 |
      | {version: 2, manifest: a.json, network: {mode: allowlist, allow: [a.example, 7]}} | policy.type        | /network/allow/1 |
      | {version: 2, manifest: a.json, network: null}                                    | policy.type         | /network         |
      | {version: 2, manifest: a.json, network: allowlist}                               | policy.type         | /network         |
      | {version: 2, manifest: a.json, network: [a.example]}                             | policy.type         | /network         |
      | {version: 2, manifest: a.json, network: {mode: null, allow: [a.example]}}        | policy.type         | /network/mode    |
      | {version: 2, manifest: a.json, network: {mode: [allowlist], allow: [a.example]}} | policy.type         | /network/mode    |
      | {version: 2, manifest: a.json, network: {mode: bridge, allow: [a.example]}}      | policy.network_mode | /network/mode    |
      | {version: 2, manifest: a.json, network: {mode: proxy, allow: [a.example]}}       | policy.network_mode | /network/mode    |
      | {version: 2, manifest: a.json, network: {mode: Allowlist, allow: [a.example]}}   | policy.network_mode | /network/mode    |
      | {version: 2, manifest: a.json, network: {mode: '', allow: [a.example]}}          | policy.network_mode | /network/mode    |
      | {version: 2, manifest: a.json, network: {mode: Offline}}                         | policy.network_mode | /network/mode    |
      | {version: 2, manifest: a.json, network: {mode: bridge, allow: ['*', 7]}}         | policy.network_mode | /network/mode    |
      | {version: 2, manifest: a.json, network: {mode: bridge, allow: []}}               | policy.network_mode | /network/mode    |
      | {version: 2, manifest: a.json, network: {mode: bridge, allow: 7}}                | policy.network_mode | /network/mode    |
      | {version: 2, manifest: a.json, network: {mode: null, allow: ['*', 7]}}           | policy.type         | /network/mode    |
      | {version: 2, manifest: a.json, network: {mode: null, allow: []}}                 | policy.type         | /network/mode    |
      | {version: 2, manifest: a.json, network: {mode: null, allow: 7}}                  | policy.type         | /network/mode    |

  # The plain step guards a fix that would point at the wrong schema. Both policy.type's and
  # policy.unknown_key's fix name version 1 today, and ID-POLICY-77 and ID-POLICY-29 hold those
  # words exactly for version 1, so the text must depend on the version actually read.
  @slice-25 @ID-POLICY-53
  Scenario Outline: version 2 stays closed and keeps version 1's other checks
    Given the policy file contains:
      """
      <document>
      """
    When itos-container runs with "doctor --policy-only --json"
    Then its policy JSON reports exactly these problems:
      | rule   | path   | exit |
      | <rule> | <path> | 2    |
    And it exits with code 2
    When itos-container runs with "doctor --policy-only"
    Then it exits with code 2
    And its error output does not say "version 1"

    Examples:
      | document                                                                         | rule               | path            |
      | {version: 2, manifest: a.json, network: {mode: allowlist, allow: [a.example], egress: []}} | policy.unknown_key | /network/egress |
      | {version: 2, manifest: a.json, network: {mode: allowlist, allow: [a.example], hosts: []}}  | policy.unknown_key | /network/hosts  |
      | {version: 2, manifest: a.json, allow: [a.example]}                                 | policy.unknown_key | /allow          |
      | {version: 2, manifest: a.json, proxy: {}}                                          | policy.unknown_key | /proxy          |
      | {version: 2, manifest: a.json, accounts: []}                                       | policy.unknown_key | /accounts       |
      | {version: 2, manifest: a.json, x-note: ignored}                                    | policy.unknown_key | /x-note         |
      | {version: 2}                                                                       | policy.required    | /manifest       |
      | {version: 2, manifest: null}                                                       | policy.type        | /manifest       |
      | {version: 2, manifest: '../a.json'}                                                | policy.path        | /manifest       |
      | {version: 2, manifest: a.json, network: {mode: allowlist, allow: [7]}}             | policy.type        | /network/allow/0 |

  # Today's fix says use offline mode. Version 2 also accepts allowlist, and version 1 refuses
  # it, so one text for both versions misleads one of them.
  @slice-25 @ID-POLICY-75
  Scenario: a wrong mode's fix names the modes its version accepts
    Given the policy file contains:
      """
      version: 2
      manifest: a.json
      network: {mode: bridge}
      """
    When itos-container runs with "doctor --policy-only"
    Then it exits with code 2
    And its error output says "use offline or allowlist mode"
    Given the policy file contains:
      """
      version: 1
      manifest: a.json
      network: {mode: bridge}
      """
    When itos-container runs with "doctor --policy-only"
    Then it exits with code 2
    And its error output says "use offline mode"
    And its error output does not say "allowlist"

  # The words are literal on purpose: a correct "version 1 or 2" has no separate "version 2"
  # for a looser step to find. The exact step holds version 1's policy.type words, which no
  # other scenario pins: "says" would pass a build that wrote "version 1 or 2" there as well.
  @slice-25 @ID-POLICY-77
  Scenario Outline: a fix names both versions only where no supported version was read
    Given the policy file contains:
      """
      <document>
      """
    When itos-container runs with "doctor --policy-only"
    Then it exits with code 2
    And its error output says "<fix>"
    When itos-container runs with "doctor"
    Then it exits with code 2
    And its error output says "<fix>"
    Given the policy file contains:
      """
      version: 1
      manifest: null
      """
    When itos-container runs with "doctor --policy-only"
    Then its error output is exactly:
      """
      itos-container: itos-container.yaml/manifest: Value has the wrong type; use the field type defined by policy version 1
      """
    And it exits with code 2
    And the engine recorder has no connections

    Examples:
      | document                         | fix                                                 |
      | {version: 3, manifest: a.json}   | use policy version 1 or 2                           |
      | {version: 0, manifest: a.json}   | use policy version 1 or 2                           |
      | {version: '2', manifest: a.json} | use the field type defined by policy version 1 or 2 |
      | {version: 2.5, manifest: a.json} | use the field type defined by policy version 1 or 2 |
      | []                               | use the field type defined by policy version 1 or 2 |

  # The last entry repeats the first exactly. A refused entry has no canonical name to
  # repeat, so it is one more policy.host_name and never a policy.host_duplicate.
  @slice-25 @ID-POLICY-54
  Scenario: independent version 2 problems are returned together without a partial policy
    Given the policy file contains:
      """
      version: 2
      manifest: null
      network:
        mode: allowlist
        allow:
          - '*.example.com'
          - a.example
          - A.EXAMPLE.
          - a.example.
          - 'a.example:443'
          - 7
          - '*.example.com'
        egress: []
      accounts: []
      """
    When itos-container runs with "doctor --policy-only --json"
    Then its policy JSON reports exactly these problems:
      | rule                  | path             | exit |
      | policy.type           | /manifest        | 2    |
      | policy.host_name      | /network/allow/0 | 2    |
      | policy.host_duplicate | /network/allow/2 | 2    |
      | policy.host_duplicate | /network/allow/3 | 2    |
      | policy.host_name      | /network/allow/4 | 2    |
      | policy.type           | /network/allow/5 | 2    |
      | policy.host_name      | /network/allow/6 | 2    |
      | policy.unknown_key    | /network/egress  | 2    |
      | policy.unknown_key    | /accounts        | 2    |
    And it exits with code 2

  # The marker is lowercase because canonicalising and the library's errors lower a name.
  # A refused name never reaches output, even when another entry repeats it exactly.
  @slice-25 @ID-POLICY-55
  Scenario Outline: allowlist diagnostics locate the entry without echoing its value
    Given the policy file contains:
      """
      <document>
      """
    When itos-container runs with "doctor --policy-only --json"
    Then its policy JSON reports exactly these problems:
      | rule   | path   | exit |
      | <rule> | <path> | 2    |
    And it exits with code 2
    And its standard output does not say "synthetic-policy-marker"
    And its error output does not say "synthetic-policy-marker"
    When itos-container runs with "doctor --policy-only"
    Then it exits with code 2
    And its error output says "itos-container: itos-container.yaml<path>:"
    And its error output does not say "synthetic-policy-marker"
    And its standard output does not say "synthetic-policy-marker"

    Examples:
      | document                                                                                                          | rule                  | path             |
      | {version: 2, manifest: a.json, network: {mode: allowlist, allow: ['https://synthetic-policy-marker.example/']}}     | policy.host_name      | /network/allow/0 |
      | {version: 2, manifest: a.json, network: {mode: allowlist, allow: ['-synthetic-policy-marker.example']}}             | policy.host_name      | /network/allow/0 |
      | {version: 2, manifest: a.json, network: {mode: allowlist, allow: [synthetic-policy-marker.example, synthetic-policy-marker.example]}} | policy.host_duplicate | /network/allow/1 |
      | {version: 2, manifest: a.json, network: {mode: synthetic-policy-marker, allow: [a.example]}}                        | policy.network_mode   | /network/mode    |
      | {version: 2, manifest: a.json, network: {mode: offline, allow: [synthetic-policy-marker.example]}}                  | policy.allow_offline  | /network/allow   |

  # 21600 distinct characters of 3 bytes each are the longest such name the 64 KiB file
  # limit admits. Encoding it takes the library four to six seconds here and refusing it
  # first takes 4 ms, so the 2 second deadline tells a limit checked before IDNA from one
  # checked after it, where the usual 5 seconds would tell nothing. A correct build has five
  # hundred times the room, so a slow machine does not flake it, but a machine over twice as
  # fast would miss the wrong build. So the implementing commit also adds a domain unit test,
  # which holds that order on any machine: the names port's fake records no call for an entry
  # over the limit. ID-POLICY-57 holds the limit's exact value without any timing.
  @slice-25 @ID-POLICY-56
  Scenario: a hostile host name is refused before canonicalising can amplify it
    Given the policy file allows one host name of 21600 distinct characters
    When itos-container runs with "doctor --policy-only --json" and a 2 second deadline
    Then it finishes before the deadline
    And its combined policy output is at most 4096 bytes
    And its policy JSON reports exactly these problems:
      | rule             | path             | exit |
      | policy.host_name | /network/allow/0 | 2    |
    And it exits with code 2
    And the engine recorder has no connections

  # Both entries canonicalise to short names. Only the limit on the raw text tells them
  # apart, and an implementation that measures after mapping accepts both.
  @slice-25 @ID-POLICY-57
  Scenario: the length limit counts what canonicalising would remove
    Given the policy file allows "ab.example" padded with soft hyphens to 1024 bytes
    When itos-container runs with "doctor --policy-only --json"
    Then it exits with code 0
    And its policy JSON contains this effective policy:
      """
      {"version":2,"manifest":"a.json","network":{"mode":"allowlist","allow":["ab.example"]}}
      """
    Given the policy file allows "abc.example" padded with soft hyphens to 1025 bytes
    When itos-container runs with "doctor --policy-only --json"
    Then its policy JSON reports exactly these problems:
      | rule             | path             | exit |
      | policy.host_name | /network/allow/0 | 2    |
    And it exits with code 2

  # Eleven fixed nodes leave 2037 of the 2048 for entries: the root, three keys and values
  # at the top, and two keys and values in network. The deadline bounds duplicate detection.
  @slice-25 @ID-POLICY-58
  Scenario: only the representation budget limits how many names a list holds
    Given the policy file allows 2037 generated host names
    When itos-container runs with "doctor --policy-only --json" and a 5 second deadline
    Then it finishes before the deadline
    And it exits with code 0
    Given the policy file allows 2038 generated host names
    When itos-container runs with "doctor --policy-only --json" and a 5 second deadline
    Then it finishes before the deadline
    And its policy JSON reports exactly these problems:
      | rule              | path | exit |
      | policy.complexity |      | 2    |
    And it exits with code 2

  # The endpoint Given is what makes the recorder step able to fail here: see the step list
  # above. The sentinel Given follows the policy file, because it snapshots the checkout.
  @slice-25 @ID-POLICY-59
  Scenario: manifest inspection reads a version 2 policy and goes on to its manifest
    Given a manifest inspection checkout
    And manifest inspection is given the recording engine endpoint
    And the manifest inspection policy contains:
      """
      version: 2
      manifest: .devcontainer/devcontainer.json
      network: {mode: allowlist, allow: [api.example.com]}
      """
    And host execution sentinels are installed
    When itos-container runs with "doctor --manifest-only --json"
    Then its JSON output reports a valid manifest at ".devcontainer/devcontainer.json"
    And it exits with code 0
    And inspection has no host side effects
    And the engine recorder has no connections

  # A refused list stops manifest inspection at the policy, like any other policy problem.
  # The manifest is valid, so a build that let the list through would report it instead.
  @slice-25 @ID-POLICY-76
  Scenario: manifest inspection stops at a refused version 2 list
    Given a manifest inspection checkout
    And manifest inspection is given the recording engine endpoint
    And the manifest inspection policy contains:
      """
      version: 2
      manifest: .devcontainer/devcontainer.json
      network: {mode: allowlist, allow: ['*']}
      """
    When itos-container runs with "doctor --manifest-only --json"
    Then its policy JSON reports exactly these problems:
      | rule             | path             | exit |
      | policy.host_name | /network/allow/0 | 2    |
    And it exits with code 2
    And the engine recorder has no connections

  # The next three scenarios keep a version 2 request away from canonical settings v1. Each
  # file edit is the only difference from an approved or approvable offline checkout, so
  # refusing later than the policy stage would reach the engine or the approval record. The
  # rows naming an absent manifest catch a refusal between the manifest stage and the engine:
  # such a build reports that manifest instead of policy.version. Each scenario's fix step
  # reads the words a person acts on: JSON stdout for doctor and approve, stderr for run.
  @slice-25 @ID-POLICY-60 @local-engine-fixture
  Scenario Outline: ordinary doctor refuses version 2 even beside a matching offline approval
    Given an approval checkout and isolated host state
    And a seeded matching approval at generation "1"
    And the checkout policy contains:
      """
      <policy>
      """
    When itos-container runs with "doctor --policy-only --json"
    Then it exits with code 0
    When itos-container runs with "doctor --json"
    Then its doctor JSON reports exactly these policy problems:
      | rule           | path     | exit |
      | policy.version | /version | 2    |
    And it exits with code 2
    And its standard output says "doctor --policy-only"
    And the Engine API journal is empty
    And the active approval bytes and generation are unchanged
    And the checkout and host control state are unchanged

    Examples:
      | policy                                                                                                        |
      | {version: 2, manifest: .devcontainer/devcontainer.json, network: {mode: allowlist, allow: [api.example.com]}} |
      | {version: 2, manifest: .devcontainer/devcontainer.json}                                                       |
      | {version: 2, manifest: absent/devcontainer.json, network: {mode: allowlist, allow: [api.example.com]}}        |
      | {version: 2, manifest: absent/devcontainer.json}                                                              |

  # In the good-manifest rows the fingerprint is the one the offline settings would have, the
  # exact approval a build that ignored the list would accept. The tree step does the looking
  # for frozen input: the record step beside it stops once the record is absent.
  @slice-25 @ID-POLICY-61 @local-engine-fixture
  Scenario Outline: approve cannot record offline settings for a version 2 request
    Given an approval checkout and isolated host state
    And the checkout policy contains:
      """
      <policy>
      """
    When itos-container runs with "doctor --policy-only --json"
    Then it exits with code 0
    When approve runs without a terminal with a well-formed candidate fingerprint as JSON
    Then it reports prerequisite problem "policy.version" with exit 2 before requesting consent
    And its standard output says "doctor --policy-only"
    And no approval record or frozen input is produced
    And the approval checkout and host state are unchanged
    And the Engine API journal is empty

    Examples:
      | policy                                                                                                        |
      | {version: 2, manifest: .devcontainer/devcontainer.json, network: {mode: allowlist, allow: [api.example.com]}} |
      | {version: 2, manifest: .devcontainer/devcontainer.json}                                                       |
      | {version: 2, manifest: absent/devcontainer.json, network: {mode: allowlist, allow: [api.example.com]}}        |
      | {version: 2, manifest: absent/devcontainer.json}                                                              |

  # Contract AC-02: a change to network permissions refuses startup, with no self-approval.
  @slice-25 @ID-POLICY-62 @local-engine-fixture @linux
  Scenario Outline: run refuses version 2 even beside a matching offline approval
    Given an approval checkout and isolated host state
    And a seeded matching approval at generation "1"
    And the checkout policy contains:
      """
      <policy>
      """
    And the run preflight files are snapshotted
    When itos-container runs with "doctor --policy-only --json"
    Then it exits with code 0
    When itos-container runs with "run -- fixture-workload"
    Then run reports preflight problem "policy.version" with exit 2 and subject "/version"
    And its error output says "doctor --policy-only"
    And the Engine API journal is empty
    And no endpoint connection was attempted
    And the active approval bytes and generation are unchanged
    And run preflight has changed no checkout or host state

    Examples:
      | policy                                                                                                        |
      | {version: 2, manifest: .devcontainer/devcontainer.json, network: {mode: allowlist, allow: [api.example.com]}} |
      | {version: 2, manifest: .devcontainer/devcontainer.json}                                                       |
      | {version: 2, manifest: absent/devcontainer.json, network: {mode: allowlist, allow: [api.example.com]}}        |
      | {version: 2, manifest: absent/devcontainer.json}                                                              |

  # The refusal above follows validation, so an invalid file reports what is wrong with it. A
  # build that refused every version 2 file first would report policy.version instead. Either
  # order exits 2 before the manifest, so this settles which answer a person gets, not a safety
  # property. Each row's first run fails today, when every version 2 file gets policy.version.
  @slice-25 @ID-POLICY-78 @local-engine-fixture @linux
  Scenario Outline: ordinary commands report an invalid version 2 file's own problem
    Given an approval checkout and isolated host state
    And the checkout policy contains:
      """
      <policy>
      """
    When itos-container runs with "doctor --json"
    Then its doctor JSON reports exactly these policy problems:
      | rule   | path   | exit |
      | <rule> | <path> | 2    |
    And it exits with code 2
    When approve runs without a terminal with a well-formed candidate fingerprint as JSON
    Then it reports prerequisite problem "<rule>" with exit 2 before requesting consent
    When itos-container runs with "run -- fixture-workload"
    Then run reports preflight problem "<rule>" with exit 2 and subject "<path>"
    And the Engine API journal is empty
    And no endpoint connection was attempted

    Examples:
      | policy                                                                                            | rule                | path             |
      | {version: 2, manifest: .devcontainer/devcontainer.json, network: {mode: allowlist, allow: ['*']}} | policy.host_name    | /network/allow/0 |
      | {version: 2, manifest: .devcontainer/devcontainer.json, network: {mode: bridge, allow: ['*']}}    | policy.network_mode | /network/mode    |

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
  @slice-1 @ID-POLICY-41
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

  @slice-1 @ID-POLICY-42
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
