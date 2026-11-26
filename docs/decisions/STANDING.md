# Standing decisions

The settled calls in force in this repository, one line each, newest wins. Read this whole
page before drafting or deciding. A design, brief or proposal that contradicts a line goes
back unless it names a new fact; then it goes to the person as a reopening, with that fact.
Superseded calls are left out; the records keep that history. A line's source is its
`path:line`; read the record there for the detail. A call the person made outranks one a
coordinator made. A change to a line is the person's, and it replaces the line.

## The person's calls

- Local sandboxes share the host's own harness login files, refreshed in place. Separate
  logins, API keys and credential brokering are for remote sandboxes only; invent no threat
  model for the local case. 2026-11-20. `docs/charter.md:143`
- Codex uses the ChatGPT subscription, not an API key. 2026-11-20. `docs/charter.md:148`
- Approval of the host OpenCode auth.json shows all its providers and grants the whole file.
  Run shares it writable at OpenCode's normal login path for host refreshes. 2026-11-26.
  `docs/decisions/0040-share-the-approved-whole-opencode-login-file-at-its-normal-path.md:20`
- Container runs may use real credentials, one form per provider: a long-lived subscription
  token where the provider issues one, else its auth file mounted, shared with the host and
  refreshed in place. Claude gets a `claude setup-token` token, read from
  `CLAUDE_CODE_OAUTH_TOKEN`, as slice-31's approved token. Codex, with no subscription token,
  gets the host's Codex home mounted read-write as a directory, with the worker's `auth.json`
  a symlink into it. itos-container is told which credential each harness gets. 2026-11-23.
  `docs/decisions/0035-real-credentials-for-container-runs-a-subscription-token-where-the-provider-issues-one-else-its-auth-file-shared.md:22`
- Credential sources are files, one secret per file, mode 0600, in a 0700 directory, until a
  credentials-manager integration exists; nothing waits on it. 2026-11-23.
  `docs/decisions/0035-real-credentials-for-container-runs-a-subscription-token-where-the-provider-issues-one-else-its-auth-file-shared.md:22`
- The Codex grant binds the mounted directory's and its lock's device and inode; a changed
  identity at the same path needs the person's consent again. This reopens 0029's rotation
  and no-inode lines for the Codex home only. 2026-11-21.
  `docs/decisions/0033-codex-home-identity-and-masks.md:15`
- Inside the read-write Codex home mount, `config.toml` and `AGENTS.md` are read-only binds,
  `sessions/` and the history files sit behind empty tmpfs, and `auth.json` and its lock stay
  writable. A run notes an unknown file there as a finding, not a stop. A CI scenario proves
  the refresh through the symlink with those masks. 2026-11-21.
  `docs/decisions/0033-codex-home-identity-and-masks.md:35`,
  `docs/decisions/0033-codex-home-identity-and-masks.md:51`
- GitHub credentials stay on the host, and the host pushes and lands. A worker holds no
  GitHub token and no SSH key: an empty `gh` configuration and no SSH agent. If an agent must
  reach GitHub, it gets a one-hour token for one repository. 2026-11-17.
  `docs/charter.md:149`
- Runs use the person's own user and existing logins: no service accounts, no logins made for
  agents. 2026-11-17. `docs/charter.md:153`
- Network and filesystem isolation, egress proxying, and credential injection or brokering
  for agent runs are this project's job. 2026-11-18. `docs/charter.md:155`
- MicroVMs come later, on several cloud providers. No Docker Sandboxes trial. 2026-11-20.
  `docs/charter.md:157`
- Cursor Bugbot is the pull request review gate; Codex is still asked, best effort.
  2026-11-19. `docs/charter.md:161`
- Caps on cost, turns, minutes, attempts and cycles are soft: passing one logs a warning and
  work never stops or holds. Only the no-progress guard, bounded retries and a hang guard
  stop. 2026-11-17. `docs/charter.md:168`, `docs/decisions/0024-engine-tier-shards.md:92`
- No exact-match allowlists of commands, questions or tool calls for agent runs: record what
  agents do and judge it against the item's scope; only an escalation reaches the person.
  2026-11-18. `docs/charter.md:165`
- Edits to `.github/workflows` go through a normal pull request and this repository's own
  gates, made by a worker in this repository. 2026-11-20. `docs/charter.md:163`
- CPU is precious and tokens matter less: the shared Go build cache, no `-count=1` by
  default, never rerun a suite on a commit that already ran. 2026-11-17.
  `docs/charter.md:171`
- Briefs name the start files, the fake and the test command; the repository keeps a short
  `MAP.md`. 2026-11-20. `docs/charter.md:173`
- The person hands over a goal and wants it driven to done: bring only large gaps, each with
  one recommendation, never start sheets or command lists. 2026-11-17. `docs/charter.md:178`
- Record design points as decided, proposed or open, each with its date. 2026-11-16.
  `docs/charter.md:180`
- Reports are in plain English. 2026-11-17. `docs/charter.md:181`
- The person is not against dependencies; judge each on its merits, with its reason in the
  spec and a pinned version. 2026-11-20. `docs/charter.md:182`, `docs/charter.md:119`
- The record of work is this repository's itos registry. 2026-11-15. `docs/charter.md:183`

## Scope

- Milestone one is an offline owned run, implementation slices 1 to 3; milestone two is a
  coding agent inside the sandbox, steps 1 to 5. 2026-11-09. `docs/charter.md:16`,
  `docs/charter.md:47`
- Not yet: LAN ingress, Compose services, platforms other than Linux amd64 with native
  Docker, the Docker socket for any worker, and any support claim beyond what a scenario
  proves. The runtime directions in `PLAN.md` are research only. `docs/charter.md:104`
- No agent receives the Docker socket; scenarios that need a real engine run in CI, never on
  the loop's host. `docs/charter.md:116`
- No scenario uses a real model, account or credential; harness traffic goes to a fake
  endpoint, and a real harness run is the person's smoke check. `docs/charter.md:101`
- The loop stops for the person on an open question, a change to the charter or contract, a
  new dependency with a network service, an open security property, or two attempts on one
  item that move nothing. `docs/charter.md:130`
- Only the loop pushes, marks a pull request ready and closes an item. `docs/charter.md:129`
- When the code shows an architecture section is wrong, record a decision in
  `docs/decisions/` naming the section in the same pull request. `docs/charter.md:40`

## Egress and credentials

- The proxy is our own, on Go's standard library with no dependencies, no `net/http` and no
  TLS interception; it checks that the ClientHello's SNI equals the CONNECT name and refuses
  missing SNI, non-TLS bytes and ECH. If interception ever comes, the proxy first moves to a
  project of its own. 2026-11-09. `docs/charter.md:60`,
  `docs/decisions/0022-egress-proxy.md:31`, `docs/decisions/0022-egress-proxy.md:111`
- CONNECT to listed host names on port 443 only, matched on the name the client gave, never
  a reverse lookup; every refusal is logged. `docs/charter.md:60`
- The worker sits on an internal network with an isolated bridge and no address on it; the
  proxy has its own egress network. There is no fallback network: nothing reconnects the
  worker or restarts the proxy. 2026-11-09. `docs/decisions/0022-egress-proxy.md:281`,
  `docs/decisions/0022-egress-proxy.md:377`
- Embedded Docker DNS cannot be removed; the rule is the result, no unauthorized resolution
  path. Provisional for the person's review. 2026-11-09.
  `docs/decisions/0022-egress-proxy.md:316`
- Policy version 1 is offline; version 2 adds an exact host-name allowlist, canonicalised and
  checked as full DNS names with no literal addresses, wildcards, suffix shortcuts or ports.
  A retained version keeps its written meaning. 2026-11-09.
  `docs/decisions/0023-policy-version-2-allowlist.md:15`,
  `docs/decisions/0023-policy-version-2-allowlist.md:33`
- Approval binds the allowlist and the pinned proxy image; any change needs new consent.
  2026-11-09. `docs/decisions/0026-egress-allowlist-approval.md:8`
- A network or credential grant is approved only at a real terminal with review and yes: every
  `--fingerprint` refuses with `approve.terminal-required`; bare non-terminal or JSON approval
  refuses with `approve.confirmation-required`. A terminal is not authentication. 2026-11-09.
  `docs/decisions/0025-credentials-passed-in.md:360`,
  `docs/decisions/0029-credential-approval.md:25`
- Doctor, approve and run accept the same version-1 and version-2 settings; version 3 stays
  gated until credential delivery lands. 2026-11-15.
  `docs/decisions/0028-egress-run-wiring.md:8`
- Credentials are passed in per command, never injected, never in the checkout or in
  `docker inspect`; approval binds kinds and paths, never contents. `docs/charter.md:69`,
  `docs/decisions/0029-credential-approval.md:11`
- On the trusted local host, a filesystem type that prevents excluding a backing alias raises
  a warning with the path and type in doctor and run status. Independent refusals still apply.
  2026-11-26. `docs/decisions/0041-record-credential-filesystem-type-as-a-warning-on-the-trusted-local-host.md:20`
- The harness home is a writable volume or tmpfs, never the host home; `/tmp` is executable.
  `docs/charter.md:85`. The opt-in per-checkout named volume was selected by the coordinator
  and waits for the person to ratify. 2026-11-16.
  `docs/decisions/0030-persistent-harness-home.md:7`
- Late host sockets: Landlock ABI 9 or later by default, gVisor as the fallback; with
  neither, run only on an approval that records "late host sockets reachable". `docs/charter.md:92`
- Milestone one refuses any Unix socket in the checkout at startup, `.git` included, and
  states that sockets created later stay reachable. 2026-11-07.
  `docs/decisions/0019-checkout-socket-milestone-limit.md:7`

## Run, recovery and approval

- Approval refuses root, uses the effective host UID/GID over the image user, and records
  what was approved, not who. 2026-11-04.
  `docs/decisions/0003-approval-identity-and-frozen-input.md:12`
- The worker's identity, entrypoint, command and working directory are overridden; image
  health checks and the daemon init wrapper are off; HOME and TMPDIR use the worker tmpfs.
  2026-11-05. `docs/decisions/0008-foreground-run-integration.md:15`
- The checkout is bound through a descriptor-backed anchor held until start, and the
  worker's view is compared with the validated identity. 2026-11-07.
  `docs/decisions/0020-descriptor-backed-checkout-anchor.md:25`
- Every regular file under the checkout has exactly one link; traversal is bounded to 128
  entries per batch, 100,000 entries, depth 64 and 10 seconds. 2026-11-06.
  `docs/decisions/0013-refuse-shared-checkout-inodes.md:9`,
  `docs/decisions/0013-refuse-shared-checkout-inodes.md:20`
- Strict ancestor or descendant checkouts are refused as `run.checkout-overlap`, across
  projects. 2026-11-06. `docs/decisions/0014-checkout-admission.md:69`
- Signal policy: forward the first SIGINT or SIGTERM once, 10 seconds of grace, then SIGKILL,
  inside a shared 30-second teardown budget. An observed nonzero workload exit wins.
  2026-11-05. `docs/decisions/0009-foreground-failure-cleanup.md:9`
- Cleanup completion proves removal, not command success: with no observed workload exit,
  run returns 75 with `run.workload-exit-unknown`. 2026-11-06.
  `docs/decisions/0015-unknown-workload-exit.md:13`
- Run ends cleanup at 29 seconds and reserves the last second for reporting; startup has a
  120-second scope; each finite call is capped at ten seconds; no timed-out mutation is
  retried; status keeps ten seconds and stop thirty. 2026-11-07.
  `docs/decisions/0018-teardown-diagnostics.md:9`,
  `docs/decisions/0017-controller-operation-budgets.md:9`
- Recovery locates a session by a hash of the host UID and the lexically cleaned checkout
  path, without opening the checkout; the index only locates, and every destructive request
  revalidates the original protected evidence. 2026-11-06.
  `docs/decisions/0012-recovery-without-checkout-access.md:8`
- `run --tty` selects terminal attachment; stdin alone supplies dimensions; the terminal is
  restored only after the final bounded drain. 2026-11-18.
  `docs/decisions/0031-foreground-terminal.md:10`
- The person: itos-container stays on v0 until the design is final, with no
  backwards-compatibility promise. A breaking change to state or schema ships with a
  documented manual cleanup, never a migration, so old v0.1.0 state gets a documented
  manual cleanup. The persisted binding device field is dropped with a schema bump, with no
  filesystem-ID replacement. 2026-11-24.
  `docs/decisions/0037-control-state-stays-on-v0-with-no-compatibility-promise-old-state-gets-a-documented-manual-cleanup-and-the-persisted-device-field-is-dropped.md:23`

## Code and CI

- Slices live under `internal/slices/<command>`, shared engines under
  `internal/engine/<name>`; a slice never imports another; slices share diagnostic
  contracts through engine ports. 2026-11-04.
  `docs/decisions/0001-enforce-slice-and-engine-boundaries.md:12`,
  `docs/decisions/0004-share-inspection-diagnostic-contracts.md:5`
- Red-first scenario evidence for engine rows comes from a manual CI dispatch with the
  `red_scenarios` input, not tag removal. 2026-11-04.
  `docs/decisions/0002-dispatch-scoped-red-evidence.md:12`
- The engine tier runs in shards of whole features; 180 seconds is a target that
  warns, 600 seconds a hang guard. 2026-11-20.
  `docs/decisions/0024-engine-tier-shards.md:92`
- Agents commit with `itos commit`, never bypass hooks, and never run gates by hand before
  committing. `AGENTS.md:29`, `AGENTS.md:33`, `AGENTS.md:35`
