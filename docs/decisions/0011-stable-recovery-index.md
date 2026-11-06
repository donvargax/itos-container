# Stable discovery for original recovery authority

The security review of slice-11 (PRRT_kwDOVkB0ws60fqg0) identified a mismatch
between foreground cleanup and fresh recovery. A worker could replace `.git`,
changing the approval project key. After controller death, status and stop
looked in a different protected directory before loading the original evidence.

This decision refines arc42's **Recovery** section and supersedes decision 0010's
project-keyed public registry path and RECOVERY-11's Git-identity veto. Public
registrations now live at `sessions/<lookup>/<id>/record.json`. The lookup is
lowercase hex SHA-256 of the UTF-8 concatenation of
`itos-container/session-index/v1`, NUL, canonical decimal host UID, NUL, and the
canonical absolute checkout path. The state root, registry, records and lock
objects retain their existing host ownership, modes, no-follow checks and
durability requirements. No extra index file or global scan is needed.

The path locates evidence; it does not authorize cleanup. Every registration
retains its original approval project and intent reference. Fresh recovery loads
the original strict snapshot and intent, checks the host UID/GID and original
canonical root, and revalidates the protected evidence, selected daemon and full
worker identity before every mutation. Transplanting a valid registration into
another checkout's index cannot authorize removal. Different checkout paths,
including equal basenames, remain independent.

Recovery pins the selected root's ancestors without inspecting mutable `.git`
contents. A worker may replace, remove or redirect `.git` without hiding its
session or vetoing cleanup. The checkout's parent and protected host index are
outside its mount; host-user actions remain trusted. Launch still validates the
complete current checkout identity. Approval, snapshots and intents keep their
existing project keys. No automatic migration of pre-interface artifacts or
restart authority is added.

RECOVERY-33 exercises fresh commands after four Git changes. RECOVERY-11 retains
foreign-root refusal, including an otherwise-valid registration transplanted
into that root's index. The existing concurrency, durability, daemon and full-ID
cases continue to apply. These local tests use disposable filesystem and HTTP
fixtures, not native runtime isolation evidence.
