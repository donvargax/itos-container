# Working rules

Read `PLAN.md` first. This repository is itself the template; each branch stays a real project.
This branch has the Go foundation and a private counting example, not the public CLI yet.

The person's session coordinates with `itos go`. An implementing agent runs `itos guide work`,
takes its item with `itos work take`, does the work itself and starts no agents. Read the registry
with `itos work`; never edit owners or statuses by hand.

## Changes and commits

- Specify behavior before building it. The private foundation is a checked bootstrap task. The
  public CLI's features and fixes follow black-box scenarios when its entry point and runner
  arrive, with red steps committed first; never add unspecified behavior meanwhile.
- Other work names a ledger task. Decide each commit's type and paths before editing.
- Commit with `itos commit -F <file>`, never `git commit`. Bodies explain what changed and why;
  no body line starts with a word and a colon, or with `with #,`.
- Read `git status --short` before staging; stage only explicit file paths, never a whole folder,
  `.`, or with `git add -A` or `-u`.
- Never bypass hooks, force-push or rewrite published history. Push with `itos push` in the
  background, reading its output and watching CI without polling. Never pipe a commit or push.
- Gates run through their configured hooks and CI, not manually before committing. Fix a valid
  check's failure in the work, not by weakening the check; stop on a wrong gate.

## Code and template boundaries

- Keep thin entry points over domain-owned behavior through ports. The domain knows no UI,
  filesystem implementation, JSON or exit codes. Use our fakes in its tests, never mocks.
- Look for a library before building what one likely solves; settle the choice in the spec. The
  count example uses Go's standard-library whitespace tokenization, not a custom word parser.
- Code differing by OS takes the OS as a value. Build tags hold only a call unavailable on one
  OS, with no mutation site; never branch on runtime.GOOS inside logic.
- The additive counting property uses newline-terminated pieces. Arbitrary concatenation can
  join words and is not additive.
- Templates are real projects. Never introduce a template language or runtime switches merely
  to pick a stack or provider. Select branches before rendering.
- No credentials in answers, fixtures, logs or committed files. Go vulnerability checks are not
  credential scans; the final template-check piece adds and proves that separate protection.
- Preserve third-party notices. Owner permission for copied infrastructure does not relicense
  dependencies or the original source projects.

## Finishing

Close with `itos work done` only after the actual pushed head is green and its code proof passes.
Before the last push, record changed-code proof with the pinned itos-cc, all-tests,
fail-uncovered and no-annotate; commit `.metrics/mutate/`. Use the parent of the item's actual
first commit, specs included, not an assumed take commit. Kill each survivor with a test; only
truly equivalent mutants may be excepted with a reason for the person to review. An uncovered
mutant needs a test, never an exception. Never reshape code merely to remove mutation sites or
copy another project's cache. Generated rules below describe this branch's actual configuration.

<!-- itos:begin -->

## itos

Stage only your own files, by explicit path: untracked files you did not make are someone else's work in progress.

<!-- itos:end -->
