# itos-container: the plan

This is a template that is a real project, not a folder of placeholders. Its code and tooling
are minimal examples of the rules a made project starts with. The generator is
[itos-template](https://github.com/donvargax/itos-template); this repository does not copy its
rendering, manifest-reading or update domain code.

## Shape

The default/root branch is `main`. `stack/go` branches off it; `go/cli` branches off the stack.
Common changes merge downward without rebasing published history. The supported combinations
will be `go` and `go + cli`, proved by rendering both and running their checks.

The manifest's literals are real values: `itos-container` for the project name, with its five
case forms; `donvargax` for the attribution owner; and
`github.com/donvargax/itos-container` for the module path. A render substitutes answers after
merging branches. AGPL-3.0 is this copy's license; owner attribution is an answer, not a secret.

## Order of work

1. Root: shared docs, fresh itos data, git conventions, Renovate configuration and root-only CI.
2. Go stack: a working Go project, its locked dependencies, private counting domain, Files port,
   read-only disk adapter, in-memory fake, example/property tests and gates applicable to its code.
   It has no public CLI or scenario runner yet.
3. CLI feature: Kong UI, a thin handler over the counting domain and black-box scenarios. Joined-
   pieces properties respect word boundaries; arbitrary concatenation is not promised additive.
4. Template CI: render and check both combinations, with pinned gitleaks marked as a credential
   scan. Only after this passes is the template advertised as ready.

Keep the completed Go/CLI project's machinery whole: three-platform CI, itos gates, code proof,
rapid properties, godog scenarios, pinned tools, Renovate and attested binary releases. Attach
each part to the branch where its prerequisites exist. Root alone cannot build a binary or
measure a nonexistent domain; the finished branches must not retain weaker placeholder gates.

Template releases and the generator's adoption/update workflow are separate later work. Binary
release tooling carried by a made CLI project is not a template-release protocol.

## Provenance

Infrastructure is harvested from the owner's itos and itos-template projects rather than
rewritten. The owner authorized AGPL-3.0 distribution of their copied work for this template.
Preserve original provenance and applicable third-party notices. Do not replace real upstream
tool and preset identities when substituting project identity.
