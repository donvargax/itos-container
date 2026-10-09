@phase-1
Feature: Shell completion
  itos-container completes on bash, zsh, fish and PowerShell.
  itos-container completion <shell> prints a short script for that shell, which asks
  the binary itself what fits, through a hidden command, __complete, taking the words
  typed so far, the last the one being completed (empty for a new word). __complete
  answers from kong's model of the command line, so the scripts never go stale: one
  candidate a line, then a last line saying what the shell does beyond them, ":files"
  to complete file names or ":none". Candidates come from kong, including only values
  kong declares. The scripts name this project's own binary. The scenarios ask
  __complete, as a script would, on the three systems; no shell runs in them.

  @ID-COMPL-01 @count-cli
  Scenario: a new word at the top completes to the commands, the hidden one left out
    When itos-container runs with "__complete ''"
    Then it exits with code 0
    And its standard output lists "count"
    And its standard output lists "version"
    And its standard output lists "completion"
    And its standard output does not list "__complete"

  @ID-COMPL-02 @count-cli
  Scenario: a flag of a command completes from its name's start
    When itos-container runs with "__complete count notes.txt --js"
    Then it exits with code 0
    And its standard output lists "--json"
    And the last line of its standard output is ":none"

  # count's argument is a path, so the shell is the one that knows the file names.
  @ID-COMPL-03 @count-cli
  Scenario: a command's argument is left to the shell's completion of file names
    When itos-container runs with "__complete count ''"
    Then it exits with code 0
    And the last line of its standard output is ":files"

  @ID-COMPL-04 @count-cli
  Scenario Outline: completion prints a script for <shell> that asks itos-container
    When itos-container runs with "completion <shell>"
    Then it exits with code 0
    And its standard output says "__complete"
    And its standard output says "itos-container"

    Examples:
      | shell      |
      | bash       |
      | zsh        |
      | fish       |
      | powershell |

  @ID-COMPL-05 @count-cli
  Scenario: completion refuses a shell it has no script for with exit 2, naming it
    When itos-container runs with "completion tcsh"
    Then it exits with code 2
    And its error output says "tcsh"

  @ID-COMPL-06 @count-cli
  Scenario: the help names completion and not __complete
    When itos-container runs with "--help"
    Then it exits with code 0
    And its standard output says "completion"
    And its standard output does not say "__complete"

  @ID-COMPL-07 @count-cli
  Scenario: completion handles a shell with no current word
    When itos-container runs with "__complete"
    Then it exits with code 0
    And its standard output lists "count"
    And its standard output does not list "__complete"
