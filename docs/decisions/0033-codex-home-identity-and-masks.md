# Codex home: bound identity and masked host files

**Decided by the person, 2026-11-21**, answering T-76's hold (its P1-a and P1-b). These
are the person's calls, not proposals. They bind T-76's
`docs/decisions/0032-codex-shared-directory.md` and the Codex children after it. This record
takes 0033 because T-76's spec already names 0032 for its own record.

The charter's second milestone, step 2 (`docs/charter.md:76`), shares the host's Codex
home with the worker. That home is mounted read-write as a directory, and the worker's
`auth.json` is a symlink into it. Two points were left for the person: whether the grant binds
what it mounts, and what else the read-write mount lets the container reach.

## Identity binding (P1-a)

The Codex grant binds the identity, device and inode, of the mounted directory and of its
lock file, as T-76's spec drafts it. A different identity at the same path is a changed
grant, and the person must consent again. The person's reason is that they never remount
that directory, so a changed identity means something is wrong.

This reopens decision 0029's lines 17-18
(`docs/decisions/0029-credential-approval.md:17`) for the Codex home grant (`codex-auth`)
only:

- **Rotation:** "source rotation at an unchanged valid path is not a settings change" no
  longer holds when the directory's or the lock's identity changes.
- **Inode metadata:** "records never hold … inode metadata" no longer holds for those two
  identities.

The grant still binds neither `auth.json`'s own inode nor any bytes, so a refresh that
replaces `auth.json` keeps the approval. For every other credential kind, 0029 stands
unchanged.

## What the container reaches through the mount (P1-b)

The host's Codex home stays mounted read-write. Inside the mount, the files that let the
container reach the host are masked:

- `config.toml` and `AGENTS.md` are bound read-only.
- `sessions/` and the history files are covered by empty tmpfs mounts, so the container
  sees none of the host's other Codex sessions.
- `auth.json` and its lock stay writable, so a login refresh lands in the host's own file.

The container's Codex uses its own home and reaches the login through the symlink
(`docs/charter.md:78`), so it never writes the host's `config.toml`.

Each run notes any file in that directory it doesn't recognize. That is a finding in the
run's record, not a refusal: the run goes on.

## Proof

A scenario must prove two things together:

- The container's Codex still refreshes its login through the symlink, and the host's own
  `auth.json` receives the new copy.
- It runs with the masks in place.

A real engine is needed for that, so the scenario runs in CI and never on the loop's host
(`docs/decisions/STANDING.md:72`). Like every scenario, it uses the fake endpoint and no real
login (`docs/charter.md:101`).
