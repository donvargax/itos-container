# Codex fixture

CI scaffolding for native real-Codex evidence; `tools/bin/engine-fixture prepare` builds it
(T-81). The image is published only to the disposable runner-local registry.

- Asset: `https://github.com/openai/codex/releases/download/rust-v0.167.0/codex-x86_64-unknown-linux-musl.tar.gz`
- API digest (`api.github.com/repos/openai/codex/releases/tags/rust-v0.167.0`):
  `sha256:9d64836106d5b374e90ee4454c7ebcde7ca7106558bd7e41a34b9f26ebda251a`, 122587028 bytes.
- Archive SHA-256: `9d64836106d5b374e90ee4454c7ebcde7ca7106558bd7e41a34b9f26ebda251a`.
- One member, the regular file `codex-x86_64-unknown-linux-musl` (mode 0755, 320575976 bytes),
  SHA-256 `cdcb45aaa01d0f14e298985e09465aebccd47f64ecd298aea44814af7b62bbda`.
- [`SHA256SUMS`](SHA256SUMS) pins the archive and the member; CI compares only against it.
  Any other name, a second member or a differing hash fails `codex_archive_verify`.
- The executable is never run here; `codex --version` belongs to a later item.
- Licence: Apache-2.0, <https://github.com/openai/codex/blob/rust-v0.167.0/LICENSE>.
