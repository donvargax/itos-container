# CLI design guidelines

These are the rules for the itos-template command line. Each rule names its source. They are
itos's (github.com/donvargax/itos, `docs/CLI.md`, as of 2026-10-06), kept where they hold for any
CLI, with what itos learned applying them. When itos-template does not follow a rule yet, the rule
says so.

Use these rules when you add or change a command, a flag, an exit code or an output. A decision
record in `docs/decisions/` can change a rule; change this document in the same commit.

## Sources

| Key      | Source                                                                                                                                                         |
| -------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| CLIG     | Command Line Interface Guidelines, <https://clig.dev>. The anchor after the key names the section, for example CLIG `#help`.                                   |
| GNU-CLI  | GNU Coding Standards, "Standards for Command Line Interfaces", <https://www.gnu.org/prep/standards/html_node/Command_002dLine-Interfaces.html>                 |
| GNU-VER  | GNU Coding Standards, "--version", <https://www.gnu.org/prep/standards/html_node/_002d_002dversion.html>                                                       |
| GNU-HELP | GNU Coding Standards, "--help", <https://www.gnu.org/prep/standards/html_node/_002d_002dhelp.html>                                                             |
| GNU-ERR  | GNU Coding Standards, "Formatting Error Messages", <https://www.gnu.org/prep/standards/html_node/Errors.html>                                                  |
| POSIX    | POSIX.1-2024 Base Definitions, section 12.2, "Utility Syntax Guidelines", <https://pubs.opengroup.org/onlinepubs/9799919799/basedefs/V1_chap12.html#tag_12_02> |
| COBRA    | Cobra README, "Concepts", <https://github.com/spf13/cobra/blob/main/README.md#concepts>                                                                        |
| ITOS     | itos's decision 35, only machine output is the contract: <https://github.com/donvargax/itos/tree/main/docs/decisions>                                          |

## The contract with scripts

Scripts can rely on three things only (ITOS):

- The exit code.
- The `--json` output, less every key named `message` or `fix`. These keys hold the same sentences
  as the plain output.
- The files that itos-template writes.

All plain output is for people. It can change in any release. A script reads `--json` and the exit
code, never the plain output and never a `message`. Once there is a release, CI runs the last
release's scenarios against the new binary; a change to the contract is a breaking change, and a
breaking change makes a major release.

### Exit codes

| Code | Meaning                                                                                                                   |
| ---- | ------------------------------------------------------------------------------------------------------------------------- |
| 0    | Success.                                                                                                                  |
| 1    | A check said no: a render's check failed, a merge left conflicts, a template refused a combination.                       |
| 2    | A usage error or a manifest or config error.                                                                              |
| 3    | The environment is missing something: a tool, a template that cannot be reached, a git repository.                        |
| 70   | An internal error that no code classified. Report it.                                                                     |
| 75   | A temporary failure. The same command can pass when you run it again with no change, for example after a network failure. |

These codes follow grep and diff (0 yes, 1 no, 2 trouble) and the BSD `sysexits.h` values
`EX_SOFTWARE` for 70 and `EX_TEMPFAIL` for 75 (CLIG `#the-basics`: map the non-zero codes to the
most important failure modes).

## Rules

### Command names and grammar

1. Keep the program name short and lowercase. (POSIX guidelines 1 and 2; CLIG `#naming`.)
2. Write a subcommand name in lowercase, with dashes between words. (CLIG `#naming`.)
3. Name a group of commands with a noun, and an action in a group with a verb in the imperative.
   (CLIG `#subcommands`; COBRA `#concepts`.)
4. Use the singular for a group name.
5. Do not give two commands similar names or overlapping meanings. (CLIG `#subcommands`.)
6. Do not name a command with an everyday verb when the same verb in a request can point to a
   different command: an agent picks a command by its name before it reads the help. (Learned in
   itos: "I need to ask someone this" meant one command, and the word "ask" pointed to another.)
7. Do not add an implicit default subcommand, one that runs an action when given no subcommand.
   (CLIG `#future-proofing`.)
8. Do not accept an abbreviation of a subcommand. Make an alias only when you name it explicitly.
   (CLIG `#future-proofing`.)

### Help and version

9. Show help for the program alone, `--help`, `help <command>` and `<command> --help`, and for
   `-h` in any position. Write help to stdout and exit 0. (CLIG `#help`; GNU-HELP.)
10. In the help of each command, give the shape of its `--json` output and its exit codes.
11. Support `--version` and `version`, printing the same. The first line of the output is
    `itos-template <version>`; a second line, `commit <full commit id>`, follows when the build
    knows the commit it was built from. (GNU-CLI; GNU-VER; CLIG `#arguments-and-flags`.)
12. For an unknown command, exit 2. If you can guess the command the person meant, name it.
    (CLIG `#help`.)
13. For a group with no subcommand, name the subcommands the group takes.
14. End the help with an example or two and the address for issue reports. (CLIG `#help`;
    GNU-HELP.)

### Flags and arguments

kong declares every command's flags in one place (decision 4), which gives rules 19 to 21 and 24
by construction; check each when a flag is added. kong refuses an unknown flag by itself;
`internal/cli`'s `Flags`, its mappers and a hook on kong's parse, refuse the rest of rule 20 in our
words: a flag with no value, a switch given a value and a once-only flag given twice. A flag is a
switch (a bool), once-only (a string) or repeatable (a slice); a flag of another kind is added
there first.

15. Give each flag a long form. Give a one-letter form only to the most common flags. (CLIG
    `#arguments-and-flags`; GNU-CLI.)
16. Use the standard name when one exists: `--json`, `-q`/`--quiet`, `-h`/`--help`, `--force`,
    `--version`. (CLIG `#arguments-and-flags`.)
17. Let a flag mean the same thing in every command. (CLIG `#subcommands`.)
18. Use a flag to change an action, never to select a different action. (COBRA `#concepts`.)
19. Accept `--flag=value` and `--flag value`. (CLIG `#arguments-and-flags`; GNU-CLI.)
20. Refuse an unknown flag, a flag with no value, a switch given a value and a once-only flag given
    twice, with exit 2. (CLIG `#robustness-guidelines`.)
21. Give an option-argument to its option only. Never read it as a global flag. (POSIX guidelines
    6 and 14.)
22. Do not make an option-argument optional. (POSIX guideline 7.) An offer that can be declined is
    a switch with its `--no-` pair (kong's negatable flags), never `--flag [no]`: itos broke this
    rule with `init --plugin [<scope>]` and paid for it.
23. Let `--` end the options, and let `-` mean stdin or stdout. (POSIX guidelines 10 and 13; CLIG
    `#arguments-and-flags`.)
24. Accept flags in any position. (CLIG `#arguments-and-flags`.)
25. Check each argument before you use it, and refuse a bad one with exit 2. (CLIG
    `#robustness-guidelines`.)

### Output

26. Write the main output to stdout. Write logs, progress and errors to stderr. (CLIG
    `#the-basics`.)
27. Print JSON only with `--json`. `--json` prints one object with `"schema": 1`, and a later
    release only adds keys to it. (CLIG `#output`; ITOS.)
28. When the plain output looks like data, write a line on stderr telling the reader to use
    `--json` in scripts.
29. With `--json`, print the object for every failure too, a usage error included: `"ok": false`
    and the rule ID of each problem. (CLIG `#output`; ITOS.)
30. Do not use colour, and pass `NO_COLOR` on to the programs itos-template runs. (CLIG `#output`,
    `#environment-variables`.)

### Errors and exit codes

31. Get the exit code from the kind of the error, in the UI alone (decision 17): the domain's and
    infra's errors are sealed sets, each kind given its code in one switch that a lint refuses to
    leave one out of, never a default; only an error no switch classified, a bug, exits 70. Read
    the kind of a failure of a program you run (git) from what it says, never pass its own code
    through. (CLIG `#the-basics`.)
32. Start each error line with `itos-template:`. Write it for people: say what happened and what
    to do next. Do not show a raw command line as the message. (GNU-ERR; CLIG `#errors`.)
33. Let the help and the code agree on each exit code; a scenario checks each one the help names.

### Environment variables

34. Start each environment variable itos-template reads with `ITOS_TEMPLATE_`, in uppercase with
    underscores, one per flag where a flag is worth setting once for a shell (kong's `env` tag).
    (CLIG `#environment-variables`.)
35. Read settings in this order: flag, then environment variable, then the config file. (CLIG
    `#configuration`.)
36. Do not let a run in CI depend on the network for an update check, and give an opt-out for the
    check. (CLIG `#future-proofing`, "Don't create a time bomb".)

### Prompts

37. Ask a question only when stdin and stdout are terminals. Give a flag for each question, so that
    a script never needs a terminal: `new` takes every answer as `--answer key=value`. (CLIG
    `#interactivity`.)

### Configuration

38. Keep a project's settings in a file in the repository, under version control: a template's
    manifest in the template, a made project's record of what it was rendered from in the project.
    (CLIG `#configuration`.)

### Entry points for other programs

39. itos runs itos-template as `itos template`, an extension. An entry point uses the protocol of
    the program that calls it: itos's extension protocol for `itos template`, this contract for a
    person or a script.

### Changing the interface

40. A rename of a command or a flag, or a change to an exit code, is a breaking change. Put the
    breaking changes that are ready into one major release together.
41. Do not keep code to stay compatible with an old interface: an old name exits 2 naming the new
    one. Only the previous-release check judges compatibility, against the contract above.

### Shell completion

42. `completion <shell>` prints a short completion stub for `bash`, `zsh`, `fish` or `powershell`.
    The stub asks the same executable through the hidden `__complete` command; it does not carry a
    second copy of the command model. `__complete` takes the words after the command, with the last
    word being completed and an empty last word meaning a new word. It prints one candidate per
    line, followed by exactly one final instruction: `:files` leaves file-name completion to the
    shell, and `:none` asks it to offer no further candidates. Candidates come from kong's command
    and flag model, including only values kong declares; values read from a template manifest are
    not completion candidates. The printed comments name where to install each stub; fish names
    its completions directory, and PowerShell names `$PROFILE`. (Decision 25.)
