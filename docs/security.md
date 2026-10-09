# Security and trust boundaries

Use templates you trust: a made project contains their files and workflows. Do not put
credentials in answers, committed files, fixtures or logs. Owner and module answers identify a
public project, not authentication data.

The counting domain accesses input through its Files port. Its disk adapter reads files without
executing or modifying their contents. It is not a filesystem sandbox; a caller must decide
which paths it permits. The `count` CLI reads the path its own arguments name and writes no
file, so what a script may read is the script's decision, not the binary's.

Dependencies are locked with go.mod/go.sum and verified in CI. Tools keep their checked pins,
upstream identities, licenses and notices. Go's vulnerability scan examines used dependencies,
including test dependencies and the standard library. Mutation proof and its CI sample judge
recorded behavior; these controls do not isolate untrusted programs.

No render credential scan is installed at this stage. The template-check piece adds a pinned
scanner and proves every supported combination before claiming that protection. A template's
check commands can execute programs, so run them only in an environment appropriate to that
trust. itos's hooks and commit rules are workflow controls, not a security sandbox.
