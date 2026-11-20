# Credential brokering: pass in, broker or mint

## Status

Proposed, for the person's decision, 2026-11-20. Amended the same day: the person corrected
the Codex recommendation (question 3), because an API key bills the OpenAI API, not the
ChatGPT subscription, and their rule is subscriptions first, with API credits only as the
fallback when a weekly limit is reached.

This is research under the [charter](../charter.md)'s "Not yet", which lists a capability
broker as a research direction. It selects nothing to build. It changes no code, scenario,
dependency, charter passage or contract passage. Every source below is a public page read on
or before 2026-11-20. Claims marked [unconfirmed] were not checked against a primary source.
Claims marked (analysis) are this document's own reasoning.

If the person accepts a direction, a decision record follows in `docs/decisions/` with the
next free number, and the charter or the [pilot contract](container-pilot.md) changes only
by the person's hand.

## Problem

The goal of itos-container is to let a coding agent run unattended without trusting it. The
worker is hostile by assumption: arbitrary code, hostile dependency scripts,
prompt-injected instructions ([contract](container-pilot.md), "Goal and limits").

The charter's second milestone, step 2, passes credentials in: "passed in, never injected,
never in the checkout". [Decision 0025](../decisions/0025-credentials-passed-in.md) defines
the delivery. The controller copies a token file into private memory-backed staging and binds
it read-only into the worker. [Decision 0029](../decisions/0029-credential-approval.md)
binds the grants in approval. The worker then reads the real credential. Today that means:

- Claude: the person's long-lived token from `claude setup-token`, exported by the
  workload as `CLAUDE_CODE_OAUTH_TOKEN`.
- Codex: the host's own ChatGPT login. `auth.json` lives in a dedicated host directory,
  mounted read-write, shared with the host's Codex through a symlink (q-72).
- GitHub: a token file, exported as `GH_TOKEN` and used by git.

A hostile worker that reads a credential can copy it out. The egress proxy limits where
it can send it, but not what it sends to an allowed host. A copied credential then works
from anywhere, after the run, until it expires or the person revokes it.

Credential brokering means the worker never holds the real secret. A trusted component
outside the worker adds it to requests on the way out. This document asks whether
itos-container should do that, how, and for which credential.

## What brokering can and cannot stop

Brokering, and short-lived credentials, stop one thing: a copied secret that keeps working
after the run. Nothing else on this list.

No option stops these. They hold for every design below.

- **Use during the run.** A compromised agent can use whatever it reaches while it runs.
  With a broker it cannot read the secret, but it can make authenticated requests through
  the broker, in the credential's full scope, for as long as the run lasts. The broker is a
  credential oracle (analysis). Substrate says its actors "cannot read, snapshot, or
  exfiltrate" the credential [16]; that holds for the value, not for its use.
- **Data exfiltration through any allowed host.** Anthropic's guide says it plainly:
  "Within your proxy's allowlist, the agent can send data to any allowed domain" [2]. The
  model API receives prompts and file contents. GitHub accepts pushes, issues, comments and
  gists. A package registry may accept a publish.
- **Damage within scope.** A token that can push can force-push or delete a branch during
  the run. Scope, not brokering, limits this.
- **Spend.** A compromised agent can burn quota or money on any model credential it uses.
- **Domain fronting with today's proxy.** The proxy checks the CONNECT name and the SNI. It
  cannot see the HTTP Host inside TLS ([security.md](../security.md), "Allowlist egress and
  its limits"). A worker can reach another tenant of an allowed shared host. Only a proxy
  that terminates TLS can compare the Host header with the SNI (option B).

What brokering changes beyond that, with today's design as the baseline:

- The [refusal-log conflict](../decisions/0025-credentials-passed-in.md#provisioning-redaction-and-the-refusal-log-conflict)
  (decision 0025, question 6) loses its weight for a brokered credential. A refused
  hostname can only carry a placeholder, and a placeholder is useless away from the broker.
- A credential the worker never holds cannot be left in harness state, the checkout or a
  persistent home volume.

So the useful questions per credential are: how long does a copy work, what can it do, and
what does closing that gap cost.

## Option A: keep passing in

This is the charter as written. The worker reads the real credential from a staged
read-only file.

**Threat it stops.** The credential is never in the checkout, the image, the container
configuration or `docker inspect`. The controller never logs it or stores it in state.
Another project's credentials are not reachable. The worker has no route out except the
proxy, which resolves and dials only listed names, so it cannot send the credential to an
unlisted host or through DNS. The staged copy is gone after teardown.

**Threat it leaves.** A copy sent to an attacker-readable sink on an allowed host, or
through fronting, works after the run:

- The `setup-token` token lasts one year. It "can only make model requests" [3]. A copy
  gives model use on the person's plan for that time.
- An API key works until the person revokes it.
- A GitHub personal access token works until its expiry. Fine-grained tokens default to 30
  days and can be non-expiring [37].
- The shared Codex login is the worst case (analysis). The worker can read the refresh
  token, which keeps working until revoked. It can also replace `auth.json` in the shared
  directory with another login. The host's own Codex then runs under that login. What an
  attacker could see of the host's later sessions through that account is not known
  [unconfirmed]. At the least the worker can break the host's login.

**Cost.** None new. The staging seam exists (decision 0025, "T-48 implementation status").
Slice-31 and slice-33 still owe their real-engine evidence.

**Login changes.**

| Login | What the worker gets |
| --- | --- |
| Claude subscription | The `setup-token` file, exported as `CLAUDE_CODE_OAUTH_TOKEN`. The `/login` file `~/.claude/.credentials.json` [3] is not passed in. |
| Claude API key | A new kind, exported as `ANTHROPIC_API_KEY`. Decision 0025 has no such kind yet. |
| Codex ChatGPT login | The shared read-write directory holding `auth.json` (charter, q-72). |
| Codex API key | A new kind, exported as `OPENAI_API_KEY` or `CODEX_API_KEY` [20][21]. OpenAI calls API keys "the recommended default for automation" [20]. |
| gh and git | The `github-token` file, exported as `GH_TOKEN` [28]; git reads it through a credential helper [33]. |

**MicroVM fit.** Weak (analysis). The staged bind under `/dev/shm` is Docker-specific; a
microVM would need the file through its own init, a shared filesystem or vsock. Worse, a
long-lived personal token would then sit inside a VM on a cloud provider's host, in its
memory and possibly its snapshots.

## Option B: broker with TLS interception

The worker holds a placeholder. A proxy outside the worker terminates TLS for allowed
hosts, finds the placeholder in a header and swaps in the real value. It forwards over a
new, verified TLS connection. This is how most products that broker do it: Docker
Sandboxes [10][12][13], srt's experimental `tlsTerminate` [15], Agent Substrate [16][17],
iron-proxy [18], Codex cloud's network secrets [19], Modal [49] and E2B [50].

**Threat it stops.** The real credential never enters the worker. A copy of the
placeholder is useless off the proxy, provided the proxy swaps it only for the hosts it
belongs to (srt's `injectHosts` [15]). With TLS terminated, the proxy also sees each
request. It can compare the Host header with the SNI, which closes fronting. It can refuse
methods or paths, such as gist creation on `api.github.com` (analysis). That is a real
gain beyond credentials.

**Threat it leaves.** Use during the run, through the proxy. A destination that echoes
headers can hand the real value back (analysis); Tailscale Aperture blocks TRACE for this
reason [51] [unconfirmed]. The proxy's CA signs certificates the worker trusts.
Its private key must never reach the worker, and the bundle must be read-only.

**Cost.** The highest of all options.

- *Code.* TLS termination, per-host certificate minting, HTTP/1.1 and HTTP/2 parsing,
  header rewriting, unbuffered streaming for server-sent events, WebSocket handling for
  Codex, verified upstream TLS, and fail-closed handling of every parse error. Refresh
  responses carry new real tokens in their JSON bodies, so the proxy must rewrite bodies
  too (below). Today's proxy is about 300 lines and "holds no CA, certificate or key"
  ([decision 0022](../decisions/0022-egress-proxy.md)).
- *New trust.* A planted CA, per run, trusted by every program in the worker. Each client
  reads trust differently, so one combined bundle (system roots plus the CA) must reach all
  of them at once [17]:
  - Claude Code adds a CA through `NODE_EXTRA_CA_CERTS` [4].
  - Codex reads `CODEX_CA_CERTIFICATE`, then `SSL_CERT_FILE` [23]. A user report says its
    chat and WebSocket paths rejected a MITM CA despite these variables [24]
    [unconfirmed for current versions].
  - gh is Go. On Linux `SSL_CERT_FILE` replaces the default bundle rather than adding to
    it [17].
  - git needs `GIT_SSL_CAINFO` [17]. Python requests needs `REQUESTS_CA_BUNDLE` [17].
  - A client that pins certificates fails. It can only be tunnelled opaquely, and then
    nothing can be injected [15][47]. No pinning was found for the three CLIs, but none was
    tested against a forged CA [unconfirmed].
- *Refresh endpoints.* An OAuth login refreshes against its own host, and the proxy must
  intercept that too, or the placeholder refresh token gets nowhere. Claude Code's "OAuth
  token exchange, refresh, and revocation also go to `platform.claude.com`" [4]. Codex
  refreshes at `https://auth.openai.com/oauth/token` [21]. The proxy must replace the real
  tokens in each response with placeholders before the client stores them. Docker's
  Claude kit does exactly this [10].
- *The Codex placeholder must look like a JWT.* Codex parses `id_token` as a JWT and fails
  if it does not parse. It reads the access token's expiry if that is a JWT [22]. A
  placeholder `auth.json` therefore needs well-formed fake JWTs with plausible plan and
  account claims. Docker's Codex kit ships a placeholder `auth.json` [11]; its format was
  not found [unconfirmed].
- *gh has two header forms.* API requests send `Authorization: token <token>` [29]. git
  over HTTPS through `gh auth git-credential` sends Basic authentication with username
  `x-access-token` and the token as password, base64-encoded [30]. The proxy must decode,
  swap and re-encode the Basic form as well.
- *Header-only.* Every product named above swaps headers only. Clients that put a secret in a
  body need body substitution, which Codex's own proxy offers as an opt-in [27].
- *Building blocks.* Anthropic names Envoy's `credential_injector`, mitmproxy, Squid and
  LiteLLM [2]. Envoy's own reference gives that filter an "unknown security posture" and
  says it suits only setups "where both downstream and upstream are trusted" [48]; a
  hostile worker is not.
- *Known failure in a shipped product.* In Docker Sandboxes a global OAuth binding silently
  overrides every credential sent to `api.anthropic.com`, even a real API key [14].

**The charter's rule.** TLS interception may come later; "if it does, the proxy first moves
to a repository and project of its own" (charter, step 1). Decision 0022 repeats it: no
interception code exists in the tree. Option B therefore starts with that move, before any
interception code is written. The pilot contract once favoured mitmdump with per-host
inspection ("Agreed scope"); the charter and q-51 chose a non-intercepting proxy instead.

**Login changes.**

| Login | What changes |
| --- | --- |
| Claude subscription | A placeholder `~/.claude/.credentials.json` with prefix-shaped sentinels such as `sk-ant-oat01-proxy-managed` and the real expiry [10]. The proxy intercepts `api.anthropic.com` and `platform.claude.com` and re-masks refresh responses. Ruled out for this project by Anthropic's terms; see below. |
| Claude API key | `apiKeyHelper` printing a placeholder, because an environment key outranks the OAuth file [10]. The proxy swaps `x-api-key` for `api.anthropic.com`. |
| Codex ChatGPT login | A placeholder `auth.json` with fake JWTs. The proxy intercepts the model host and `auth.openai.com` and re-masks refreshes. The model host for ChatGPT logins was not documented on the pages read [unconfirmed]. OpenAI's terms on this were not researched [unconfirmed]. |
| Codex API key | A placeholder in `OPENAI_API_KEY`; the proxy swaps the bearer header for `api.openai.com`. |
| gh and git | A placeholder in `GH_TOKEN`; the proxy swaps `token` headers for `api.github.com` and Basic headers for `github.com`. |

**MicroVM fit.** Good technically (analysis). The proxy sits outside the VM, so it works
on any provider. Docker Sandboxes runs it with microVMs [13]. But the CA bundle must enter
every guest per run, and the proxy becomes a per-run, credential-holding, TLS-terminating
component on every provider.

## Option C: broker without interception, through a base URL

The tool is configured to call the broker's own endpoint in plain HTTP. The broker strips
what the client sent, adds the real credential and opens TLS upstream to one fixed host.
This is TLS origination, not interception. No CA is planted and no certificate is forged.
Anthropic's guide recommends exactly this for the Claude API: point `ANTHROPIC_BASE_URL`
at a plaintext proxy, which "can inspect and modify them (including injecting
credentials), then forwards to the real API" [2]. Nango's proxy has this shape [39], as do
LLM gateways such as LiteLLM [43][44] and Tailscale Aperture [51]. Claude Code on the web
keeps git credentials outside its sandbox behind a git proxy that holds the real token and
limits pushes to the session's repository and branch [52][53].

**Threat it stops.** The real credential never enters the worker. The worker holds a
per-run broker token. The broker accepts only that token, only for its one route, and only
during the run. A copy is useless after teardown, and useless before it from anywhere the
broker cannot be reached. The broker has a fixed upstream per route, so a worker cannot
use it to reach other hosts. Features like Nango's `Base-Url-Override` [39] must not exist
here.

**Threat it leaves.** Use during the run through the broker. Hosts the tool calls without
the base URL. A redirected configuration.

- *Hosts that bypass the base URL.* Claude Code calls `api.anthropic.com` directly for the
  WebFetch safety check, feature flags and the fast-mode check. It calls
  `platform.claude.com`, `claude.ai` and `sso.claude.com` for login and refresh,
  `mcp-proxy.anthropic.com` for connectors, Datadog hosts for telemetry, and update hosts
  [4][7][54]. `CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1` turns off updates, telemetry,
  error reports and feature flags, but not the WebFetch check [5][7]. Codex refreshes at
  `auth.openai.com` [21]; its other calls were not enumerated [unconfirmed]. Each such host
  is either refused by the allowlist or allowed without a credential. A placeholder sent
  there is harmless (analysis).
- *A hostile checkout redirecting the configuration.* The checkout is writable by the
  worker. Claude Code applies a project's `.claude/settings.json` `env` block, which can
  set `ANTHROPIC_BASE_URL` [4] [CLI ordering unconfirmed]. A repository's `.git/config`
  can define `url.<base>.insteadOf` and `credential.helper` [33][34]. Codex ignores some
  keys in a project's `.codex/config.toml`, including `chatgpt_base_url`; the full list
  was not read [25] [unconfirmed]. The configuration must be pinned outside the checkout:
  a managed Claude settings file in the image with `allowedProviders: ["customEndpoint"]`,
  which refuses any other endpoint from v2.1.285 [6]; Codex configuration in a `CODEX_HOME`
  the checkout cannot write; git configuration from the environment or command line rather
  than the repository (analysis). Claude Code already follows this rule for its own
  sandbox: it honours credential masking only from user, managed or `--settings` scope,
  never from a repository's `.claude/` [8]. A redirect that escapes the pin sends only the
  placeholder, and only to a host the allowlist admits. The worst case is that model
  traffic, not the credential, goes to that host (analysis).

**Cost.**

- *Code.* A small reverse proxy: one route per credential, a fixed upstream, header
  replacement, unbuffered streaming, upstream TLS through the standard library (analysis).
  It needs `net/http`, which decision 0022 keeps out of the egress proxy. So the broker is
  a separate component from the egress proxy, not a feature of it.
- *Operations.* One more owned container (or process) per run beside the proxy, joining the
  internal network for the worker and the egress network for its upstream. It enters
  intent, teardown and recovery like the proxy (analysis).
- *New trust.* No CA. The broker holds the real credential in memory for the run.
- *Third-party gateways.* Nango's repository is licensed under the Elastic License 2.0
  [38]. Its limits on managed services and licence keys are further restrictions that
  AGPL-3.0 section 10 forbids [41], so its code cannot be vendored into this repository.
  AGPL-3.0 section 5 lets a covered work sit in an aggregate with separate works [41], so a
  separately installed Nango service may be possible. ELv2's limit bites when the software
  is offered to others as a managed service [42]. Nango's scoped keys are per environment,
  not per connection [40]. Its required `Connection-Id` and `Provider-Config-Key` headers
  do not match the paths Claude Code and Codex call [39] [untested end to end]. LiteLLM's
  licence was not checked [unconfirmed]. A gateway service is also a new dependency with
  a network service, which the charter says to stop and ask about.

**Login changes.**

| Login | What changes |
| --- | --- |
| Claude subscription | No gain. With only `ANTHROPIC_BASE_URL` set, "a saved claude.ai login remains the active credential" and the gateway receives the real OAuth token [6]. A broker that holds the subscription token and injects it is intermediating, which Anthropic's terms bar (below). |
| Claude API key | `ANTHROPIC_BASE_URL=http://<broker>:<port>` and `ANTHROPIC_AUTH_TOKEN=<per-run broker token>` [5]. The broker replaces it with `x-api-key`. The gateway serves `/v1/messages` [54]. A plain `http://` base URL is not stated in Anthropic's docs [unconfirmed]; LiteLLM's guide uses one [44]. Remote Control is disabled off `api.anthropic.com` [5]. |
| Codex ChatGPT login | No gain. `requires_openai_auth = true` sends the client's own ChatGPT token to the custom base URL [26]. A broker that held the login and ran the refresh is untested [unconfirmed]. |
| Codex API key | A custom provider with `base_url = "http://<broker>"`, `env_key` naming a variable that holds the per-run broker token, `wire_api = "responses"` and `supports_websockets = false` [25][26]. Pinned in a `CODEX_HOME` outside the checkout. |
| gh | No clean fit. gh has no documented plain-HTTP knob for `github.com`. `api_host` takes a bare host name on HTTPS port 443 [31], so the broker would need a certificate gh trusts: a planted CA, which defeats this option. Only `github.localhost` uses plain HTTP [32] [unconfirmed for gh itself]. gh stays on A or D. |
| git | `url."http://<broker>/github.com/".insteadOf "https://github.com/"` is valid git configuration [34]. git then speaks smart HTTP to the broker, which adds the token upstream. Proxying smart HTTP was not tested [unconfirmed]. |

**MicroVM fit.** Good (analysis). The broker runs outside the guest and the guest reaches
it over a virtual network or vsock. No CA enters the guest. A broker on the person's
machine serving a VM in a cloud would need an authenticated path between them.

## Option D: short-lived, scoped credentials, without brokering

The worker holds a real credential, but a lease: minted for this run, narrow in scope and
short in life. A trusted component outside the worker holds the long-lived secret and mints
the lease.

**Threat it stops.** A copy expires. A GitHub App installation token lasts one hour, can be
limited to chosen repositories and permissions, and can be revoked with
`DELETE /installation/token` [35][36]. Revoking at teardown ends it with the run.

**Threat it leaves.** The worker holds the lease and can copy it. The copy works from
anywhere until it expires or is revoked. A broker that answers a mounted socket also lets
the worker mint more leases while the socket is attached; the endpoint is the authority
(analysis, from the endpoint pattern in [45]).

**Cost, per mechanism.**

- *GitHub App installation tokens.* The person registers a GitHub App and installs it on
  the repositories the sandbox may touch [35]. Its private key is a new long-lived secret
  on the host. Only the controller reads it; it is never staged. The controller signs a JWT
  with it and asks GitHub for a token, scoped to the one repository the approval names and
  the permissions it needs [35]. That is a new network call from the host controller,
  which today makes none (analysis). Delivery can reuse the existing staging seam: the
  minted token becomes the `github-token` file. A git credential helper over a mounted
  socket [33] would let long runs refresh, but the contract excludes socket forwarding
  ("Goal and limits"), so a socket needs the person's exception.
- *Anthropic Workload Identity Federation.* The workload presents a short-lived OIDC JWT
  and receives a short-lived token bound to a service account. Lifetime is 60 seconds to
  24 hours, default one hour [9]. It needs an organization with Console admin access, so
  API billing, not a subscription [9]. The broker would act as the OIDC issuer, with a
  public HTTPS issuer or inline JWKS [9]. Claude Code supports it, but not in bare mode,
  and `ANTHROPIC_API_KEY` must be unset because it outranks federation [3][9].
- *`apiKeyHelper` with a TTL.* Claude Code runs a helper command and refreshes at
  `CLAUDE_CODE_API_KEY_HELPER_TTL_MS` [3][5]. This is plumbing, not a lease. A plain API
  key has no minter, so the helper would return the same long-lived key. It helps only with
  a minter such as federation, or with option C's broker token. Codex's
  `[model_providers.<id>.auth]` command, with `refresh_interval_ms`, is the same plumbing
  [25].
- *AWS-style credential endpoints.* SDKs fetch temporary keys from a local endpoint named
  by `AWS_CONTAINER_CREDENTIALS_FULL_URI`, sending a bearer token [45], or run a
  `credential_process` command that returns an `Expiration` [46]. This is the best-proven
  interface for a generic lease endpoint. Its content matters here only for AWS-hosted
  models (analysis).

**Login changes.**

| Login | What changes |
| --- | --- |
| Claude subscription | Nothing can be minted. It stays on A. |
| Claude API key | Federation for an organization; the worker gets a JWT file and the env names in [9]. |
| Codex ChatGPT login | No first-party short-lived minting was found. Stays on A. |
| Codex API key | No first-party short-lived minting was found [unconfirmed beyond the pages read]. Stays on A. |
| gh and git | An installation token in the `github-token` file, exported as `GH_TOKEN`, read by git through a credential helper. gh's own refreshable credentials were not examined under this model [unconfirmed]. |

**MicroVM fit.** The best (analysis). Cloud providers give each VM a workload identity, which
federation accepts directly [9]. A lease that leaks from a VM, its memory or a snapshot
expires on its own. Minting stays on the control side, whatever the provider.

## Option E: a mix per credential

Each credential gets the cheapest option that closes the gap that matters for it. The
options compose: D's minted GitHub token rides A's staging seam; C's broker token can come
from D's helper plumbing. The recommendation below is a mix.

## Anthropic's terms and the Claude subscription

Anthropic's legal page says developers "may not collect, store, or intermediate Claude.ai
credentials or session tokens", and may not "route requests through Free, Pro, Max, or
Ultra plan credentials on behalf of their users". It permits "an end user [signing] in to
the unmodified Claude Code binary with their own Claude subscription", and lets customers
keep their own API keys in "a secrets manager" [1].

This project cannot settle a legal question; this is not legal advice. Its reading
(analysis):

- Option A passes the person's own token to the unmodified binary in the person's own
  container. The controller copies it into staging and never sends it anywhere. That is
  the closest case to what the page permits.
- Options B and C hold a subscription token in a proxy and put it on requests the worker
  makes. That is what the page calls intermediating. Docker's Claude kit does it [10];
  whatever arrangement covers Docker is not public. A public AGPL tool should not.
- API keys may be brokered. Holding them in a broker is the "secrets manager" case.

So for a Claude subscription, brokering is not an option for this project. Only A applies.

## Comparison

| | A: pass in | B: interception | C: base URL | D: short-lived |
| --- | --- | --- | --- | --- |
| Real secret in the worker | Yes | No | No | A lease |
| A copy works after the run | Until expiry or revocation | No | No | Until lease expiry; revoked at teardown |
| Closes fronting | No | Yes | No | No |
| New code | None | Large: TLS, HTTP/2, streaming, body re-masking | Small reverse proxy, separate from the egress proxy | Minting client; revoke at teardown |
| New trust | None | Planted CA in every client | Broker holds keys in memory | Long-lived minting key on the host; controller network call |
| Claude subscription | Yes | Barred by terms | Barred by terms | Not mintable |
| Claude API key | Yes | Yes | Yes, documented [2][6] | Federation, organizations only |
| Codex ChatGPT login | Yes, shared read-write | Fake JWTs, refresh re-mask, untested | No gain | Not mintable |
| Codex API key | Yes | Yes | Yes, documented [25] | No minter found |
| gh | Yes | Yes, two header forms | Needs a CA; no fit | Yes, App token |
| git | Yes | Yes, Basic re-encode | `insteadOf`, untested | Yes, App token |
| Own repository first | No | Yes, by the charter | Not by the rule's text | No |
| MicroVM fit | Weak | Good, costly | Good | Best |

## Is a repository of its own required?

The charter requires it only for TLS interception, and before any interception code is
written. Option B triggers it.

Option C does not intercept. It plants no CA and forges no certificate; the client chooses
to talk to the broker. The rule's text does not cover it. But it adds a credential-holding
HTTP component, and decision 0022 keeps `net/http` out of the egress proxy. So if C comes,
it should be a separate component with its own decision record, and the egress proxy
should stay as it is. Whether C also moves out is the person's call (question 6).

Option D changes no proxy. Minting lives in the host controller, behind a port with a fake,
like the engine. It needs no move.

## Recommendation

Adopt option E as two small pieces: **A for model credentials, D for GitHub.** Build no
interception. Record C as the next step for API keys only, not built now.

1. **Keep A as the base, as already designed.** Slice-31 and slice-33 land as decision 0025
   describes. Nothing else can carry a Claude subscription, because brokering one is barred
   [1]. Any brokering design would still need A for that login.
2. **Claude: the `setup-token` token passed in.** Its copy works for a year, but only for
   model requests [3]. That limits the damage to the person's plan usage. A Claude API key
   is a second kind on the same seam, for people who bill by API.
3. **Codex: default to a ChatGPT login of the sandbox's own.** The charter's shared
   read-write ChatGPT login is the largest gap in today's design. The worker can read a
   refresh token that does not expire on its own, and can replace the host's login. The
   default becomes a separate login kept in the run's persistent harness home
   ([decision 0030](../decisions/0030-persistent-harness-home.md)), made once there with
   device login (`codex login --device-auth` [20]), and never the host's `auth.json`. It keeps
   billing on the ChatGPT subscription, the person's rule. It closes the host-substitution
   threat: the worker never sees the host's file. It leaves the sandbox's own refresh token
   readable by the worker (A's gap, for that login only). An API key on the same seam stays
   as an explicit fallback kind, for when the subscription's weekly limit is reached. The
   shared host login is no longer offered.

   *Do two logins of one ChatGPT account refresh independently?* Not stated by OpenAI. Each
   `codex login` writes its own `auth.json` [20], and a refresh replaces the refresh token,
   which then works once [55][56]. OpenAI's own guidance is one `auth.json` per runner, never
   one file across machines or concurrent jobs [57]. So two separate logins should keep
   separate token chains (analysis). But an open Codex bug reports one account on three hosts,
   each with its own `auth.json`, where logging in on one host "appears to invalidate" the
   refresh tokens on the others [58], with no maintainer answer yet; a maintainer has said
   Codex is not designed for several concurrent logins on one system [59]. Independence is
   therefore likely but unconfirmed. The first increment must test it on the real account,
   host and sandbox both refreshing, before it is the default. If the logins do interfere,
   the failure is a logged-out run or host, not a leaked credential: the run stops and asks
   for a new device login.
4. **GitHub: an installation token minted per run (D).** A's real gap here is that a copied
   personal token keeps write access after the run, for days or months [37]. GitHub is the
   credential whose misuse others see: pushes, pull requests, comments. A one-hour token,
   limited to the approved repository and permissions, and revoked at teardown, closes that
   gap [35][36]. It reuses the staging seam, so the worker sees the same `github-token`
   file. The approval binds the App, the installation, the repository and the permissions,
   from the policy, never from the checkout's Git configuration. The fine-grained personal
   token stays as the fallback for people without an App.
5. **No option B.** It costs the most: a planted CA per client, HTTP/2 and streaming
   parsing, body re-masking for refreshes, JWT-shaped Codex placeholders, two gh header
   forms, and a move to its own repository first. Its credential gain over A is reachable
   for API keys through C without a CA, and is barred for the Claude subscription. Its one
   unique gain, closing fronting, is an egress question, not a credential one, and belongs
   in its own idea if the person wants it.
6. **C later, if the person wants API keys never inside the worker.** It is the documented
   Anthropic pattern [2] and fits microVMs. The gap it would close is an API key copied out
   and used until revoked. It waits until A and D have runtime evidence.
7. **For microVMs on cloud providers, move toward D.** Long-lived personal tokens should not
   go into cloud VMs. Federation [9] and installation tokens carry across providers. Build
   nothing for this now.

**Gaps this leaves, stated plainly.** A copied `setup-token` token works for model requests
for up to a year. A copied sandbox Codex refresh token works until it, or the sandbox's next
refresh, rotates it; the host's Codex login is never exposed. A copied API key, when that
fallback is chosen, works until revoked. A copied GitHub token works for up
to an hour, or until teardown when revocation succeeds. A GitHub run longer than an hour
loses access unless a refresh design follows. Use during the run, data exfiltration through
allowed hosts and fronting remain, as for every option.

**Increments, if accepted.** Each is specified by black-box scenarios first, against fakes;
no scenario uses a real account (charter).

1. Slice-31 and slice-33 as designed.
2. A sandbox-only Codex ChatGPT login in the harness home, logged in once by device login,
   replacing the shared read-write directory and answering decision 0025's question 1 for
   unattended runs. Its first test is whether host and sandbox logins refresh independently.
   A Codex API-key kind on the same seam follows as the explicit fallback.
3. A GitHub App kind: policy and approval binding, a minting port with an I/O-free fake, a
   fake GitHub endpoint in CI, revocation at teardown recorded in recovery.
4. Later, and only on the person's word: a refresh design for long GitHub runs, then C.

## Questions for the person

Each lists the recommended answer first.

1. **Direction.** Recommended: accept option E as above, A for model credentials and D for
   GitHub, no interception, C deferred. Alternatives: keep A alone with least-scope rules
   (decision 0025, question 4); or start option B, which first moves the proxy to its own
   repository.
2. **Claude login for unattended runs.** Recommended: the `setup-token` token passed in, as
   chartered, with an API key as a second kind. Never broker the subscription. Alternative:
   API keys only, which removes any doubt about the terms at the cost of API billing.
3. **Codex login.** Recommended (the person's correction, 2026-11-20): a sandbox-only
   ChatGPT login by default, kept in the harness home, logged in once there by device login,
   never the host's `auth.json`; it keeps the subscription and closes host substitution. An
   API key stays as an explicit fallback kind for when the weekly limit is reached.
   Alternatives: an API key by default, which bills the API instead of the subscription; or
   keep the shared login as the default, as q-72 set. This changes charter step 2. Open
   point: whether two logins of one account refresh independently is unconfirmed (see the
   recommendation) and is the first thing to test.
4. **GitHub.** Recommended: GitHub App installation tokens minted per run, limited to the
   approved repository and permissions, revoked at teardown, with the host controller
   calling GitHub to mint and revoke. Alternative: require a fine-grained personal token
   for one repository with an expiry the person sets, and accept that a copy works until
   then.
5. **GitHub runs longer than an hour.** Recommended: the first increment promises one hour
   and says so; refresh comes later in its own design. Alternatives: a credential helper
   over a per-run socket, which needs an exception to the contract's ban on socket
   forwarding; or rewriting the staged file in place from a descriptor the controller
   keeps (analysis, untested).
6. **Where a base-URL broker would live.** Recommended: C is not interception, so the
   charter's move rule does not apply; it would be its own component and decision record,
   never part of the egress proxy. Alternative: treat any credential-holding proxy like
   interception and move it to its own repository first.
7. **TLS interception.** Recommended: rule it out for this repository; if it is wanted, for
   credentials or for fronting, it starts as its own project, as the charter says.
   Alternative: keep it open as research only.
8. **MicroVMs on cloud providers.** Recommended: record D (federation, installation tokens)
   as the credential direction for cloud VMs, and pass no long-lived personal token into
   one. Build nothing now. Alternative: decide when the microVM work starts.

## Sources

1. Claude Code, legal and compliance: https://code.claude.com/docs/en/legal-and-compliance
2. Claude Agent SDK, secure deployment: https://code.claude.com/docs/en/agent-sdk/secure-deployment
3. Claude Code, authentication: https://code.claude.com/docs/en/authentication
4. Claude Code, network configuration: https://code.claude.com/docs/en/network-config
5. Claude Code, environment variables: https://code.claude.com/docs/en/env-vars
6. Claude Code, LLM gateway overview: https://code.claude.com/docs/en/llm-gateway
7. Claude Code, connecting to a gateway: https://code.claude.com/docs/en/llm-gateway-connect.md
8. Claude Code, sandboxing: https://code.claude.com/docs/en/sandboxing
9. Claude platform, Workload Identity Federation: https://platform.claude.com/docs/en/manage-claude/workload-identity-federation
10. Docker Sandboxes Claude kit: https://hub.docker.com/r/sbx/claude-kit
11. Docker Sandboxes Codex kit: https://hub.docker.com/r/sbx/codex-kit
12. Docker Sandboxes, credentials: https://docs.docker.com/ai/sandboxes/security/credentials/
13. Docker Sandboxes, isolation: https://docs.docker.com/ai/sandboxes/security/isolation/
14. Docker sbx-releases issue 793: https://github.com/docker/sbx-releases/issues/793
15. Anthropic sandbox-runtime README: https://github.com/anthropic-experimental/sandbox-runtime/blob/main/README.md
16. Agent Substrate, egress credential injection: https://github.com/agent-substrate/substrate/blob/main/docs/guides/egress-credential-injection.md
17. Agent Substrate, egress trust bundle: https://github.com/agent-substrate/substrate/blob/main/docs/guides/egress-trust-bundle.md
18. Hermes Agent, iron-proxy: https://hermes-agent.nousresearch.com/docs/user-guide/egress/iron-proxy
19. Codex cloud environments: https://learn.chatgpt.com/docs/environments/cloud-environments
20. Codex authentication: https://learn.chatgpt.com/docs/auth
21. Codex source, login auth manager: https://github.com/openai/codex/blob/main/codex-rs/login/src/auth/manager.rs
22. Codex source, token data: https://github.com/openai/codex/blob/main/codex-rs/login/src/token_data.rs
23. Codex pull request 14239, CA certificate precedence: https://github.com/openai/codex/pull/14239
24. Codex issue 6849, MITM CA rejected: https://github.com/openai/codex/issues/6849
25. Codex advanced configuration: https://learn.chatgpt.com/docs/config-file/config-advanced
26. Codex configuration reference: https://learn.chatgpt.com/docs/config-file/config-reference
27. Codex network proxy README: https://github.com/openai/codex/blob/main/codex-rs/network-proxy/README.md
28. GitHub CLI, environment: https://cli.github.com/manual/gh_help_environment
29. GitHub CLI source, HTTP client: https://github.com/cli/cli/blob/trunk/api/http_client.go
30. GitHub CLI source, git credential helper: https://github.com/cli/cli/blob/trunk/pkg/cmd/auth/gitcredential/helper.go
31. GitHub CLI, configuration: https://cli.github.com/manual/gh_config
32. go-gh REST client tests: https://github.com/cli/go-gh/blob/trunk/pkg/api/rest_client_test.go
33. Git, gitcredentials: https://git-scm.com/docs/gitcredentials
34. Git, url configuration: https://raw.githubusercontent.com/git/git/master/Documentation/config/url.adoc
35. GitHub, generating an installation access token: https://docs.github.com/en/apps/creating-github-apps/authenticating-with-a-github-app/generating-an-installation-access-token-for-a-github-app
36. GitHub REST, revoking an installation access token: https://docs.github.com/en/rest/apps/installations?apiVersion=2022-11-28#revoke-an-installation-access-token
37. GitHub, managing personal access tokens: https://docs.github.com/en/authentication/keeping-your-account-and-data-secure/managing-your-personal-access-tokens
38. Nango licence: https://github.com/NangoHQ/nango/blob/master/LICENSE
39. Nango proxy reference: https://nango.dev/docs/reference/backend/http-api/proxy/get
40. Nango API keys reference: https://nango.dev/docs/reference/backend/http-api/api-keys
41. GNU Affero General Public License v3.0: https://www.gnu.org/licenses/agpl-3.0.txt
42. Elastic License 2.0 FAQ: https://www.elastic.co/licensing/elastic-license/faq
43. LiteLLM virtual keys: https://docs.litellm.ai/docs/proxy/virtual_keys
44. LiteLLM, Claude Code tutorial: https://docs.litellm.ai/docs/tutorials/claude_responses_api
45. AWS SDKs, container credential provider: https://docs.aws.amazon.com/sdkref/latest/guide/feature-container-credentials.html
46. AWS CLI, sourcing credentials with an external process: https://docs.aws.amazon.com/cli/latest/userguide/cli-configure-sourcing-external.html
47. mitmproxy, ignoring domains: https://docs.mitmproxy.org/stable/howto/ignore-domains/
48. Envoy credential injector API: https://www.envoyproxy.io/docs/envoy/latest/api-v3/extensions/filters/http/credential_injector/v3/credential_injector.proto
49. Modal, sandbox secret injection: https://modal.com/docs/guide/sandbox-secret-injection
50. E2B, internet access: https://docs.e2b.dev/sandbox/internet-access
51. Tailscale Aperture, how it works: https://tailscale.com/docs/aperture/how-aperture-works
52. Anthropic engineering, Claude Code sandboxing: https://anthropic.com/engineering/claude-code-sandboxing
53. Claude Code on the web: https://code.claude.com/docs/en/claude-code-on-the-web
54. Claude Code, LLM gateway protocol: https://code.claude.com/docs/en/llm-gateway-protocol.md
55. Codex issue 41973, refresh token revoked; single-use refresh tokens: https://github.com/openai/codex/issues/41973
56. Codex Security issue 2243, a shared single-use refresh token: https://github.com/openai/codex-security/issues/2243
57. Codex, maintain account auth in CI/CD: https://learn.chatgpt.com/docs/auth/ci-cd-auth
58. Codex issue 17340, refresh token already used, across hosts: https://github.com/openai/codex/issues/17340
59. Codex issue 9634, refresh token already used: https://github.com/openai/codex/issues/9634
