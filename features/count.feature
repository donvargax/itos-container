@phase-1
Feature: Count
  What a person of a project made from this template observes from
  itos-container count <file>: the logical lines and the Unicode-whitespace words the
  Go foundation already counts, with - reading standard input and no implicit default
  action. A human run prints the counts on stdout and the use-json hint on stderr, and
  never the input's contents. --json prints one object, schema 1, with ok, file (the
  argument as given, - for standard input), lines and words, and it prints the hint
  nowhere. Every failure prints its own object, ok false, with problems carrying rule,
  message and fix. The exit code comes from the kind of the error, in the UI's sealed
  switch alone: 2 for a usage failure, 3 for an input that is missing
  (COUNT_INPUT_MISSING) or otherwise unreadable, a directory included
  (COUNT_INPUT_UNREADABLE). A kind no case classifies is the 70 of an unclassified bug,
  never a child status passed through, and count itself never refuses with 1. No
  arguments and --help print help on stdout with exit 0, while count without its file is
  a usage failure. --help and --version are actions; --json is a negatable switch with
  ITOS_CONTAINER_JSON, the project's name in upper case. -- ends the options and flags
  are accepted in any position.

  # A file whose last line has no newline still counts, CRLF is one separator and
  # punctuation stays inside a word, so the counts a person reads are the foundation's
  # and not a second rule written in the UI.
  @ID-COUNT-01 @count-cli
  Scenario: count prints a file's lines and words and hints at --json
    Given the file "notes.txt" contains "one two three", "four five"
    When itos-container runs with "count notes.txt"
    Then it exits with code 0
    And its standard output says "notes.txt: lines 2, words 5"
    And its error output says "itos-container: for scripts, use --json"

  # The counts are for people, never the file's own text: an input holding a secret
  # must not reach the output.
  @ID-COUNT-02 @count-cli
  Scenario: count never prints what it read
    Given the file "secret.txt" contains "hunter2"
    When itos-container runs with "count secret.txt"
    Then it exits with code 0
    And its standard output does not say "hunter2"

  @ID-COUNT-03 @count-cli
  Scenario: count reads standard input where the argument is -
    Given standard input holds "one two three"
    When itos-container runs with "count -"
    Then it exits with code 0
    And its standard output says "-: lines 1, words 3"

  @ID-COUNT-04 @count-cli
  Scenario: --json prints one object with the file, its lines and its words
    Given the file "notes.txt" contains "one two three", "four five"
    When itos-container runs with "count notes.txt --json"
    Then it exits with code 0
    And its JSON output gives file "notes.txt", 2 lines and 5 words
    And its error output does not say "--json"

  # Standard input is named - in the object too, so a script needs no second rule to
  # tell where the counts came from.
  @ID-COUNT-05 @count-cli
  Scenario: --json names standard input as the argument it was given
    Given standard input holds "one two three"
    When itos-container runs with "count - --json"
    Then it exits with code 0
    And its JSON output gives file "-", 1 lines and 3 words

  # A switch that can be declined, never --json [yes|no].
  @ID-COUNT-06 @count-cli
  Scenario: --no-json declines the switch and prints the plain output
    Given the file "notes.txt" contains "one two three", "four five"
    When itos-container runs with "count notes.txt --json --no-json"
    Then it exits with code 0
    And its standard output says "notes.txt: lines 2, words 5"

  @ID-COUNT-07 @count-cli
  Scenario: ITOS_CONTAINER_JSON turns the JSON on for a shell
    Given the file "notes.txt" contains "one two three", "four five"
    When itos-container runs with "count notes.txt" and the environment "ITOS_CONTAINER_JSON=1"
    Then it exits with code 0
    And its JSON output gives file "notes.txt", 2 lines and 5 words

  # -- ends the options and an option-argument is never read as a flag, so a file
  # whose name starts with a dash is still counted. --json comes before the --,
  # which ends the options: after it --json is a file's name, not a switch.
  @ID-COUNT-08 @count-cli
  Scenario: -- ends the options so a name starting with - is a file
    Given the file "-draft.txt" contains "one two"
    When itos-container runs with "count --json -- -draft.txt"
    Then it exits with code 0
    And its JSON output gives file "-draft.txt", 1 lines and 2 words

  # bufio.Scanner's default token is 64 KiB: a line past it must still be one line and
  # be counted whole, or the answer would quietly depend on the length of a line.
  @ID-COUNT-09 @count-cli
  Scenario: a line longer than a scanner's token limit is counted whole
    Given the file "long.txt" holds a line of 100000 characters
    When itos-container runs with "count long.txt --json"
    Then it exits with code 0
    And its JSON output gives file "long.txt", 1 lines and 1 words

  # Exit 3 is the environment's, not a refusal: the input the person named is not
  # there. The rule ID is what a script matches on, not the sentence.
  @ID-COUNT-10 @count-cli
  Scenario: a missing file exits 3 with COUNT_INPUT_MISSING
    When itos-container runs with "count absent.txt --json"
    Then it exits with code 3
    And its JSON output names the problem "COUNT_INPUT_MISSING"
    And its error output says "absent.txt"

  # A directory exists and still cannot be read as bytes; a platform says so
  # differently, the kind and the code are the same.
  @ID-COUNT-11 @count-cli
  Scenario: a directory exits 3 with COUNT_INPUT_UNREADABLE
    Given an empty folder "adirectory"
    When itos-container runs with "count adirectory --json"
    Then it exits with code 3
    And its JSON output names the problem "COUNT_INPUT_UNREADABLE"

  # With --json even a failure is one object, and every problem says what to do next,
  # not only what went wrong: a script reads the rule and prints the fix.
  @ID-COUNT-12 @count-cli
  Scenario: a JSON failure carries a rule, a message and a fix for each problem
    When itos-container runs with "count absent.txt --json"
    Then it exits with code 3
    And its JSON output gives each problem a rule, a message and a fix

  @ID-COUNT-13 @count-cli
  Scenario: count without its file is a usage failure
    When itos-container runs with "count"
    Then it exits with code 2
    And its error output says "count"
    And its error output does not say "EOL"

  # No implicit default action: with nothing to do the program says what it can do,
  # on stdout, and succeeds.
  @ID-COUNT-14 @count-cli
  Scenario: no arguments print the help on stdout and exit 0
    When itos-container runs with ""
    Then it exits with code 0
    And its standard output says "count"
    And its standard output says "completion"
    And its error output does not say "count"

  # The help of a command gives the shape of its --json output and every exit code it
  # can return, so the two agree without a reader holding the source. The wording below
  # is what the help must hold, because a scenario checks it and a script reads it.
  @ID-COUNT-15 @count-cli
  Scenario: count's help names its JSON shape and its exit codes
    When itos-container runs with "count --help"
    Then it exits with code 0
    And its standard output says "--json prints one object: schema 1, ok true, file the argument, lines and words."
    And its standard output says "Exit codes:"
    And its standard output says "0  success"
    And its standard output says "2  the command line is wrong"
    And its standard output says "3  the input is missing or cannot be read"
