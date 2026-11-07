# Recovery without writable checkout access

Slice-11 review PRRT_kwDOVkB0ws60jWo- showed that decision 0011 still allowed a
worker to hide its surviving session. Recovery pinned the checkout root before
opening the protected index. Removing read or search permissions from the writable
root made fresh status and stop refuse before loading the original authority.

This decision supersedes that root-pinning requirement in decision 0011 and
arc42's **Recovery** section. Discovery hashes the host UID and lexically cleaned
absolute effective root path without opening the checkout or resolving its current
contents. A relative or empty root refuses. The original launch already established
the canonical path stored in the protected snapshot. A different path selects a
different index; recovery neither follows a checkout alias nor scans other indexes.

The index is still only a locator. Opening a session validates the original strict
record, intent and snapshot, including the host UID/GID and the snapshot's exact
root match. Protected ancestors, file ownership, modes, links and operation locks
retain their existing checks. Each destructive request still revalidates the
original daemon and full resource identity. A transplanted registration or matching
resource labels cannot grant authority. Launch retains full checkout validation.

RECOVERY-36 invokes fresh status and stop from outside a disposable checkout after
its root becomes unreadable or inaccessible. It requires discovery, owned cleanup,
durable completion and preservation of unrelated resources and original state.
The commands must leave the worker-selected permissions untouched; the harness
restores them only after both commands finish. RECOVERY-11 and RECOVERY-13 retain
the foreign-root and substituted-authority refusals. These are local filesystem
and HTTP-fixture tests, not new native-engine isolation evidence.
