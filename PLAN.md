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

## Security and runtime ideas to explore

These are research directions, not selected dependencies, promised protections or additions to
the bootstrap sequence above. The aim is to reduce what an agent or model-generated program can
access, and explore a lighter runtime where the harness supports it. Separate the harness's
runtime from the sandbox that executes its tools; either can change without moving the other.

Start with a threat model for untrusted repository content, generated code and tool calls.
Filesystem access, network destinations, credentials and resource budgets need explicit limits
enforced outside guest code. Keep credentials behind a narrow broker where possible, and treat
each exposed tool as authority that isolation alone cannot make safe.

- Explore V8 isolates in a Workers-style runtime for generated JavaScript and possibly the
  agent loop itself. Compare their security and operating costs with hardened containers or
  VMs rather than assuming isolates are stronger. Cloudflare's
  [Workers security model](https://developers.cloudflare.com/workers/reference/security-model/)
  combines isolates with process sandboxing, mediated APIs and side-channel defenses; embedding
  V8 alone does not reproduce those protections.
- Consider WASM for portable tools or harness components, with narrowly granted host imports
  or WASI capabilities and bounded memory and execution. Compilation does not grant Unix process,
  filesystem or network compatibility, and permissive host imports can undo the intended limits.
  JavaScript can run directly in V8; a WASM build is not a prerequisite for that path.
- Evaluate Vercel's [just-bash](https://justbash.dev/) as a restricted shell tool with a virtual
  filesystem and network disabled unless explicitly allowed. It may cover inspection and text
  processing without exposing a real shell. It is not a VM or a replacement for arbitrary native
  builds; filesystem adapters and custom commands must not quietly restore host authority.
- Study [OpenCode Code Mode](https://opencode.ai/v2/docs/tools#execute) as a stretch idea for
  composing tool calls in generated JavaScript through a small catalog. Verify its actual
  execution boundary before reusing it. Tool orchestration and V8 isolation address different
  concerns: a Code Mode interface could sit inside an isolate, with authorization still enforced
  on every host tool call. Compare that combination with a smaller isolate-only interface.

A possible runtime split is to keep closed harnesses such as Claude Code in hardened containers
and adapt open-source harnesses such as pi or OpenCode to an isolate runtime where feasible.
Choose by required capabilities, not license alone. OpenCode V2 already documents a
[workerd SDK](https://opencode.ai/v2/docs/build/sdk/cloudflare) for Durable Objects that replaces
local filesystem and process services; investigate that route before attempting a WASM port.
Compatibility for pi and Claude Code is unproven here. Using a Claude model API from an isolate
is a separate question from running the Claude Code harness there.

### Working hypothesis and first experiment

Favor isolated orchestration plus disposable execution over replacing containers wholesale or
porting every harness to WASM. Keep a hardened container baseline while testing this split:

```text
Harness -> permission/capability broker -> restricted tools
                                       -> disposable native-code runner
```

The broker must enforce permissions independently of the harness and generated code. An agent
that can request a build does not thereby need unrestricted machine access. Giving an isolate
a broadly privileged shell tool would defeat much of the intended restriction.

Start with an OpenCode agent turn in workerd using scoped file tools, restricted network access
and, if compatible, just-bash. Its virtual filesystem could hold proposed edits for a trusted
component to validate and apply; it must not be the sole containment boundary. Delegate native
builds and tests to a disposable container without model-provider credentials. Keep Code Mode
optional until the smaller tool interface works, and investigate WASM for individual components
only where it solves a demonstrated need.

Prove session recovery, rejection of forbidden file reads and network requests, and cancellation
and termination of runaway work. Check resource exhaustion and ensure secrets stay outside
guest-visible state. Record unsupported plugins, subprocess and terminal needs, escape and
side-channel risks, and patching responsibilities before selecting a runtime or making security
claims. The experiment should show whether the hybrid improves containment without requiring
every harness to use the same runtime.
Any adopted option belongs on a real runtime branch with its own checks, not a template switch.

## Provenance

Infrastructure is harvested from the owner's itos and itos-template projects rather than
rewritten. The owner authorized AGPL-3.0 distribution of their copied work for this template.
Preserve original provenance and applicable third-party notices. Do not replace real upstream
tool and preset identities when substituting project identity.
