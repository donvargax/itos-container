# This repository's own notes

`itos go` prints these notes after its coordinator guide. Keep them specific to this template.

## Start of a session

- Read `PLAN.md` and this branch's actual `itos.yaml`. The Go foundation has private example
  code and its Go/code-proof gates, but no public CLI, godog runner or binary release job yet.
- Root is `main`; stacks and features remain real projects. Merge common changes downward,
  never rebase published history. Made-project binary releases are not template release tags.
- Infrastructure is harvested from the owner's itos-template and itos repositories. Preserve
  provenance and real upstream identities; do not globally substitute the owner's handle.
- A Renovate configuration does not prove App installation or bot activity. Verify before
  claiming the automation runs.

## Briefs and proof

- Name the item, spec, exact harvest files and neighboring checks. The coordinator writes the
  user-facing docs; each branch's claims must match its implemented stage.
- The count example uses standard-library Unicode whitespace and logical lines. Its additive
  property uses newline-terminated pieces, not arbitrary concatenation.
- The proof base is the parent of the item's actual first commit, specs included. Adding tests
  can stale imported-code snapshots: refresh the records the gate requires, not the gate.
- This branch's proof covers `internal/`; add `cmd/` with the public CLI, not as an empty placeholder.
- Pick a representative smoke scenario when the CLI runner makes each feature live. Keep live
  scenarios as explained wip on a revert; never delete them to force the revert through.
- Never inherit another project's mutation cache or generator-domain fixtures.

## Lessons

New lessons name a recorded date, last-seen date and exit item (or permanent); keep at most ten.
Report an upstream-tool gap upstream after checking releases and existing reports, rather than
working around it in this template.
