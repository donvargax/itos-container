# Share inspection diagnostic contracts across commands

## Decision and scope

Slice-4 needs doctor and approve to report the same policy, manifest and readiness
failures without a slice importing or calling another slice. Keep the stable public
problem values and pure typed-issue translations beside their owning engine ports.
Doctor retains its presentation aliases; approval consumes the same translations.
The approval engine does not call the doctor slice, even through a callback.

This overrides **Cross-cutting concepts / Diagnostics and streaming** in
[arc42](../architecture/arc42.md), which previously placed all public rule and exit
mapping outside ports. The port contract now includes that deterministic translation,
like the existing manifest JSON-pointer helper. Domains still return typed failures;
commands and named presentation adapters still own terminal escaping, JSON encoding,
streams and exit emission. Port code performs no I/O and adds no standard-library
allowlist entry, parsing implementation or dependency.

## Why and evidence

A public prerequisite refusal is part of the shared inspection contract. Keeping its
translation in doctor would force cross-slice reuse or duplicate mappings in approve.
Moving the pure translation with the contract lets both consumers use exactly the
same rules and guidance while preserving the import direction and cmd-only adapter
assembly. Existing doctor diagnostic tests exercise the moved translations through
its aliases; policy, manifest, doctor and approval scenarios check their public output.

This is a presentation ownership refinement, not an approval authority or isolation
change. It does not alter the coordinator's consent, identity, state or frozen-input
decisions, and it makes no runtime support claim.
