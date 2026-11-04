@phase-1
Feature: Bounded policy parsing
  Doctor must be able to refuse hostile policy data without first finishing an
  arbitrarily deep parse. The file, parser, representation and diagnostic limits
  form one bounded inspection, including input that never completes a document.

  # Slice-5 extends slice-1's features/policy.feature, inspected at
  # dfcd6156ffd753acb6fc628ef04e804969b8ae60. Integrate slice-1 before implementing
  # this spec: this branch alone still has no doctor or policy step definitions.
  # This separate area/file avoids changing another drafter's feature file.
  #
  # Keep the closed version 1 schema, all permitted YAML data spellings, no-follow
  # regular-file read, 65536-byte input cap, directive-line refusal, exact numeric
  # semantics and safe diagnostics from slice-1. No resource keys or environment
  # overrides are added. In particular, aliases are never expanded, merge keys
  # never merged, and tag prefixes never expanded from directives. Quoted text,
  # block scalars and comments are not collections, aliases or directives.
  #
  # Bound the whole path, rather than adding another list of hostile strings:
  # - Admit at most 65536 source bytes in total, not per document or parse attempt.
  #   Keep the existing size, UTF-8 and directive checks before parser creation.
  # - During scanning/parsing, allow at most 32 simultaneously open flow
  #   collections and 32 active block-collection indentation levels. Count a
  #   mapping or sequence once, including collections used as mapping keys.
  #   These are parser-stack limits, separate from representation depth below;
  #   a mixed block/flow input must satisfy both and the representation limit.
  #   Stop on the first excess, even if no closing delimiter ever arrives.
  # - Parse nodes, not expanded Go values. Source-sized scalar/tag/anchor text
  #   and bounded parser stacks must not become copies of alias targets,
  #   expanded tag prefixes, exponent-sized numbers or per-ancestor strings.
  #   The source cap bounds the width/text of the intermediate node tree; the
  #   parser-stack cap bounds construction depth before that tree is complete.
  # - After the first document is complete, apply slice-1's aggregate limits
  #   before requesting a second document or converting values: 32 representation
  #   levels, 2048 nodes and 65536 bytes of full escaped representation paths.
  #   Keep slice-1's counting rules, including keys, indices, aliases counted once
  #   without following targets, and both key/value paths. Counters do not reset
  #   for siblings, unknown fields, tags, aliases or structural errors.
  # - A second-document check uses the same bounded parser, never an unrestricted
  #   decoder. A complete second document still means policy.multiple_documents;
  #   a parser-stack excess while reading it means policy.complexity. Never
  #   construct policy values from it or retry with a permissive parser.
  # - Only within those bounds collect structural/schema problems and render
  #   diagnostics, using slice-1's path budget and compact exact-number handling.
  #   No stage may expand data first and check its budget afterwards.
  #
  # Parser-stack or representation exhaustion returns exactly one root
  # policy.complexity problem, exit 2, no partial policy, and slice-1's safe
  # corrective advice. It takes precedence over later syntax/multiple-document
  # errors. Syntax encountered before exhaustion remains policy.syntax. The
  # bounded whole-source UTF-8/directive checks retain their earlier precedence.
  # This explicitly supersedes slice-1's parse-both-documents-before-limits order;
  # it changes no existing live example's expected result. The new mixed-failure
  # cases below distinguish early refusal from an after-decoding tree walk.
  #
  # Library decision, verified 2026-11-04: use go.yaml.in/yaml/v4 v4.0.0 for the
  # policy adapter, replacing its v3 use. The released module resolves to
  # f2f9563503f11b7a60da0c532175dcc693ce8d5a and provides LoaderLimitsPlugin's
  # CheckDepth during scanning, whereas v3.0.5 exposes no configurable parser
  # depth hook. Prefer the maintained parser's hook to writing a YAML pre-parser
  # or counting punctuation in raw bytes. Pin go.mod/go.sum when implementing;
  # retain the independent tooling v3 pin and all upstream license notices.
  # Sources reviewed (the module download was checksum verified):
  # https://pkg.go.dev/go.yaml.in/yaml/v4@v4.0.0
  # https://github.com/yaml/go-yaml/blob/v4.0.0/plugin/loaderlimits/plugin.go
  # https://github.com/yaml/go-yaml/blob/v4.0.0/internal/libyaml/scanner.go
  # Record an adapter-owned exhaustion outcome in the hook; the library may
  # reformat its error, so never classify by matching raw parser error prose.
  # Decode to nodes and retain slice-1's own strict scalar
  # and structural checks; a v4 default is not permission to change the schema.
  # AliasNone disables the library's alias limit; it does NOT forbid aliases.
  # Never use it as an alias prohibition or enable external alias-data sources.
  #
  # Keep this in the reusable internal/engine/policy adapter and contracts, with
  # doctor as its existing vertical slice. No slice-to-slice import, generic UI
  # dependency or new standalone parser engine is required. Run the unchanged
  # Source contract against fake/adapter, plus focused adapter tests for bounded
  # construction and every failure-precedence edge. Preserve POLICY-01..42.
  # This is a structural resource bound, not a promised RSS/CPU quota or a new
  # worker/startup timeout. Those contract decisions remain outside this item.
  # The five-second child deadline below is a harness guard; do not assert heap
  # size, allocations, GC traces or wall-clock performance of successful parses.
  # No engine access is needed; red/green evidence belongs to the local tier.
  #
  # Step handoff: reuse slice-1's isolation, deadline, JSON, output-size and
  # snapshot/sentinel steps. Add only the fixture writers described below and
  # the append-DocString step. Each Given writes bytes or prepares observations;
  # none calls doctor, parses the fixture or asserts product behaviour.
  # On this branch, import those reusable steps via slice-1 integration before
  # the implementer's red test commit. Unknown-command/undefined-step failures
  # are not evidence for this item. Against the inspected slice-1 implementation,
  # every scenario below is expected to fail at its first exact-problem assertion:
  # the current decoder returns
  # policy.syntax after parsing to the unfinished tail instead of complexity.

  Background:
    Given policy inspection has isolated host state and a recording engine endpoint

  # New Given writes one of these exact byte constructions, with LF endings:
  # sequences: repeat("[", count) + "0\n"
  # mappings: repeat("{a: ", count) + "0\n"
  # tagged sequences: repeat("!x [", count) + "0\n"
  # complex keys: "? " + repeat("[", count) + "0\n"
  # For "second document", prepend "version: 1\nmanifest: a.json\n---\n".
  # For "first document", prepend nothing. Never add closing delimiters.
  # Both 33 and 32 openings are malformed: only 33 exhausts the parser stack.
  @slice-5 @ID-PARSING-01
  Scenario Outline: unfinished flow collections exhaust the parser before a syntax tail
    Given the policy file has 33 unclosed "<shape>" flow collections in the "<position>"
    When itos-container runs with "doctor --policy-only --json" and a 5 second deadline
    Then it finishes before the deadline
    And its combined policy output is at most 4096 bytes
    And its policy JSON reports exactly these problems:
      | rule              | path | exit |
      | policy.complexity |      | 2    |
    And it exits with code 2
    Given the policy file has 32 unclosed "<shape>" flow collections in the "<position>"
    When itos-container runs with "doctor --policy-only --json" and a 5 second deadline
    Then it finishes before the deadline
    And its policy JSON reports exactly these problems:
      | rule          | path | exit |
      | policy.syntax |      | 2    |
    And it exits with code 2
    And the engine recorder has no connections

    Examples:
      | shape            | position        |
      | sequences        | first document  |
      | mappings         | first document  |
      | tagged sequences | first document  |
      | complex keys     | first document  |
      | sequences        | second document |
      | mappings         | second document |
      | tagged sequences | second document |
      | complex keys     | second document |

  # New Given writes count lines: repeat("  ", i) + "a:\n" for mappings,
  # or repeat("  ", i) + "-\n" for sequences, with i from 0 to count-1.
  # Append repeat("  ", count) + "[\n" to leave the final value unclosed.
  # The second-document prefix is the same as above. A scalar's indentation is
  # not an additional block collection, and indentation columns are not levels.
  @slice-5 @ID-PARSING-02
  Scenario Outline: unfinished indented collections share the bounded parsing contract
    Given the policy file has 33 "<shape>" block levels with an unfinished value in the "<position>"
    When itos-container runs with "doctor --policy-only --json" and a 5 second deadline
    Then it finishes before the deadline
    And its combined policy output is at most 4096 bytes
    And its policy JSON reports exactly these problems:
      | rule              | path | exit |
      | policy.complexity |      | 2    |
    And it exits with code 2
    Given the policy file has 32 "<shape>" block levels with an unfinished value in the "<position>"
    When itos-container runs with "doctor --policy-only --json" and a 5 second deadline
    Then it finishes before the deadline
    And its policy JSON reports exactly these problems:
      | rule          | path | exit |
      | policy.syntax |      | 2    |
    And it exits with code 2
    And the engine recorder has no connections

    Examples:
      | shape     | position        |
      | mappings  | first document  |
      | sequences | first document  |
      | mappings  | second document |
      | sequences | second document |

  # New Given writes a complete first document with the specified aggregate:
  # nodes: "version: 1\nmanifest: a.json\nextra: [" +
  #        repeat("0,", count-7) + "]\n" (seven fixed representation nodes).
  # paths: "? '" + repeat("a", count/2-1) + "'\n: 0\n" for the even counts
  #        below (key and value each charge a slash and count/2-1 UTF-8 bytes).
  # New append Given appends DocString bytes and one LF to itos-container.yaml.
  # At the exact limit, the malformed second document still reports syntax;
  # above it, first-document exhaustion must stop inspection before that tail.
  @slice-5 @ID-PARSING-03
  Scenario Outline: exhausted representation budgets stop inspection before another document
    Given the policy file has a complete document using <excess> aggregate "<budget>"
    And the following bytes are appended to the policy file:
      """
      ---
      [
      """
    When itos-container runs with "doctor --policy-only --json" and a 5 second deadline
    Then it finishes before the deadline
    And its combined policy output is at most 4096 bytes
    And its policy JSON reports exactly these problems:
      | rule              | path | exit |
      | policy.complexity |      | 2    |
    And it exits with code 2
    Given the policy file has a complete document using <limit> aggregate "<budget>"
    And the following bytes are appended to the policy file:
      """
      ---
      [
      """
    When itos-container runs with "doctor --policy-only --json" and a 5 second deadline
    Then it finishes before the deadline
    And its policy JSON reports exactly these problems:
      | rule          | path | exit |
      | policy.syntax |      | 2    |
    And it exits with code 2
    And the engine recorder has no connections

    Examples:
      | budget     | excess | limit |
      | nodes      | 2049   | 2048  |
      | path bytes | 65538  | 65536 |

  # This uses the same exhaustion through ordinary doctor, and then a successful
  # policy-only control. The quoted path contains 40 literal [ characters and
  # the comment contains apparent YAML syntax; neither consumes nesting.
  # Existing sentinel/snapshot Givens leave helper executables, a manifest and
  # observations in the scratch checkout/isolated host state, never real tools.
  @slice-5 @ID-PARSING-04
  Scenario: early parsing refusal is read-only and leaves literal YAML data usable
    Given the policy file has 33 unclosed "sequences" flow collections in the "first document"
    And inspection helper sentinels are installed
    And the policy inspection files are snapshotted
    When itos-container runs with "doctor --json" and a 5 second deadline
    Then it finishes before the deadline
    And its combined policy output is at most 4096 bytes
    And its doctor JSON reports exactly these policy problems:
      | rule              | path | exit |
      | policy.complexity |      | 2    |
    And it exits with code 2
    And the policy inspection files are unchanged
    And no inspection helper sentinel was executed
    Given the policy file contains:
      """
      # [[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[ &a *a !x %TAG
      version: !!int 1
      manifest: '[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[.json'
      network: ! {mode: !!str offline}
      """
    And the policy inspection files are snapshotted
    When itos-container runs with "doctor --policy-only --json" and a 5 second deadline
    Then it finishes before the deadline
    And it exits with code 0
    And its policy JSON contains this effective policy:
      """
      {"version":1,"manifest":"[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[.json","network":{"mode":"offline"}}
      """
    And the policy inspection files are unchanged
    And no inspection helper sentinel was executed
    And the engine recorder has no connections
