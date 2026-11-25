# Slice-41 OpenCode host smoke check

The person's host smoke check for slice-41's spec (pull request 73), run on 2026-11-20 on
the person's Linux amd64 host, outside any container, with their own OpenCode Go login.
The person approved the real login for this check only. The key was never printed,
copied or logged: runs that needed it read the host's own data directory in place.

This records what the installed OpenCode does. It changes no spec, scenario, code or
approval profile; slice-41's drafter re-drafts from it. It is not container evidence and
claims no OpenCode support.

## Method

`$T` is an empty scratch directory outside any checkout, and `$T/proj` is an empty
directory with a `.git` folder, standing in for a checkout. Credential-free runs use
isolated homes:

```sh
iso() { env -i PATH=/usr/bin:/bin HOME=$T/home XDG_CONFIG_HOME=$T/xdg/config \
  XDG_DATA_HOME=$T/xdg/data XDG_STATE_HOME=$T/xdg/state XDG_CACHE_HOME=$T/xdg/cache \
  TMPDIR=$T "$@"; }
```

The spec's profile environment, from its runtime decisions:

```sh
PROFILE='OPENCODE_CONFIG_CONTENT={"autoupdate":false,"share":"disabled"}
OPENCODE_DISABLE_AUTOUPDATE=true OPENCODE_DISABLE_DEFAULT_PLUGINS=true
OPENCODE_DISABLE_LSP_DOWNLOAD=true OPENCODE_DISABLE_MODELS_FETCH=true'
```

Network use was seen through a local CONNECT proxy that logs only the host and port of
each request, never headers or bodies, named by `HTTPS_PROXY` and `HTTP_PROXY`. OpenCode
honours both: every outbound request, loopback included, went through it. Source
references are to the `v2.2.1` tag of `anomalyco/opencode` (the repository `sst/opencode`
now redirects there).

## Findings

### 1. The installed version is 2.2.1, not the spec's target 1.19.0

`opencode --version` printed `opencode v2.2.1` (`/usr/bin/opencode`). Version 2 is a
restructured code base: the spec's loader reference, `packages/opencode/src/config/config.ts`
lines 447-506 at v1.19.0, has no counterpart path in 2.2.1. The loader now lives in
`packages/core/src/config.ts`. Every finding below is for 2.2.1. Nothing here was run
against 1.19.0.

### 2. The headless command line works as the spec writes it

`opencode run --help` and `opencode --help` list `--standalone` ("Run with a private
server instead of the background service"), `--format default|json` and
`--model provider/model#variant`. Without `--standalone`, `run` connects to the
background service, whose environment is not the caller's.

- The real run exited 0, and stdout held one JSON line, a `text` event whose text was
  `OK`:

  ```sh
  cd $T/proj && env -i PATH=/usr/bin:/bin HOME=$T/home XDG_CONFIG_HOME=$T/real/config \
    XDG_DATA_HOME=$HOME/.local/share XDG_STATE_HOME=$T/real/state \
    XDG_CACHE_HOME=$T/real/cache TMPDIR=$T \
    OPENCODE_CONFIG_CONTENT='{"autoupdate":false,"share":"disabled"}' \
    OPENCODE_DISABLE_AUTOUPDATE=true OPENCODE_DISABLE_MODELS_FETCH=true \
    opencode --standalone --print-logs --log-level error run --format json \
    --model opencode-go/glm-5.3-flash "Reply with the single word OK."
  ```

  `$T/real/config` is empty, so the host's own configuration and plugins were not
  loaded. `XDG_DATA_HOME` is the host's own data directory (`$HOME` here is the person's
  real home), where the login lives, read in place.
- `cd $T/proj && iso $PROFILE opencode --standalone run --format json --model
  opencode-go/none "say ok"` exited 1 with one JSON line,
  `{"type":"error",…,"error":{"type":"provider.no-route","message":"Model unavailable:
  opencode-go/none"}}`. A failing request exits non-zero.
- The first real attempt, with the full profile environment and the host's data
  directory, exited 1 with one JSON error, `"UnexpectedStatus: 500"`, before any
  provider was contacted (the proxy saw only the private server's loopback request). The
  repeat, without `OPENCODE_DISABLE_DEFAULT_PLUGINS` and `OPENCODE_DISABLE_LSP_DOWNLOAD`
  and with `--print-logs --log-level error`, succeeded. 2.2.1 reads neither variable
  (finding 4), so they are unlikely to be the cause, and the isolated run with the full
  profile returned its ordinary error, not 500. The cause was not found. It was not rerun, to keep real
  runs to the approved check.

### 3. Configuration load order

`opencode --standalone debug config` lists the configuration sources in load order.
`cd $T/proj && iso OPENCODE_CONFIG=$T/explicit.json
OPENCODE_CONFIG_CONTENT='{"autoupdate":false,"share":"disabled"}' opencode --standalone
debug config`, with an `opencode.json` in the global directory, in `$T/proj` and in
`$T/proj/.opencode`, listed:

1. the global file, `$XDG_CONFIG_HOME/opencode/opencode.json` (`OPENCODE_CONFIG_DIR`
   replaces that directory);
2. `OPENCODE_CONFIG`, the explicit file;
3. the project's `opencode.json`, found by walking up from the working directory;
4. the project's `.opencode/opencode.json`;
5. `OPENCODE_CONFIG_CONTENT`, the inline document.

`Config.load` (`packages/core/src/config.ts`, lines 243-316) gives the full order:
managed policies, integrations, remote well-known configuration, global, explicit,
project files, `.opencode` directories, inline content, then managed settings. Its
comment says managed settings "load last so they override everything". The managed
directory on Linux is `/etc/opencode` (`packages/core/src/config/discovery.ts`, line 31).
The host has none. So the inline content overrides the global, explicit and project
layers, and only managed settings override it, as the spec says of 1.19.0. The account
layer the spec names for 1.19.0 does not appear as its own layer in 2.2.1. The merged
values were not read back: `opencode api config.get` returns the same source list.

`debug config` also shows that 2.2.1 rewrites the legacy `autoupdate` key on load:
`{"autoupdate":false}` became `{"update":"disable"}`, and `true` became `"auto"`
(`packages/core/src/config/normalize.ts`, lines 55-77). `share` keeps
`manual|auto|disabled`.

### 4. What each variable the spec relies on does in 2.2.1

Every `OPENCODE_DISABLE_*` name in the 2.2.1 source, from `grep -rn --include=*.ts -o
'OPENCODE_DISABLE_[A-Z_]*' .` over the tag's source tree: `AUTOUPDATE`, `CHANNEL_DB`,
`CLAUDE_CODE`, `CLAUDE_CODE_SKILLS`, `EMBEDDED_WEB_UI`, `FFF`, `FILEWATCHER`,
`FREE_MODELS`, `MODELS_FETCH`, `PROJECT_CONFIG`.

- **`OPENCODE_DISABLE_AUTOUPDATE`**: read. With `1` or `true`, the update check is
  skipped before anything else (`packages/cli/src/services/updater.ts`, line 534).
  Without it, the check reads the update policy only from the global directory's
  `config.json`, `opencode.json` and `opencode.jsonc` (lines 122-171). It never reads the
  merged configuration, so the inline `autoupdate:false` does not reach the updater.
  This variable is the setting that disables updates, and the inline key does not.
  The headless run without the variable made no update-check request (the no-profile
  run in finding 5), so `run` does not seem to check for updates at all.
- **`OPENCODE_DISABLE_MODELS_FETCH`**: read. It turns off the models catalogue fetch
  (`packages/cli/src/server-process.ts`, line 120), and OpenCode uses its bundled
  catalogue instead. Seen on the wire: without it, a headless run connected to
  `models.opencode.ai:443`. With it, no such request was made.
- **`OPENCODE_DISABLE_DEFAULT_PLUGINS`**: not read anywhere in 2.2.1. It has no effect.
- **`OPENCODE_DISABLE_LSP_DOWNLOAD`**: not read anywhere in 2.2.1. It has no effect, and
  no language-server download code was found in the core, server or CLI packages.
- **`OPENCODE_CONFIG_CONTENT`**: read as the inline layer (finding 3,
  `server-process.ts` line 133).

### 5. Startup network use, under the profile

- Credential-free, no profile variables, isolated homes, `--model opencode-go/none`:
  `models.opencode.ai:443` by CONNECT, then plain HTTP requests to `127.0.0.1:1234`,
  `127.0.0.1:11434` and `127.0.0.1:8000`, the default origins of the bundled LM Studio,
  Ollama and vLLM providers (`packages/core/src/plugin/provider/lmstudio.ts` line 27,
  `ollama.ts` line 48, `vllm.ts` line 18). They were refused, since nothing listens there.
- The same run with the full profile: the catalogue request was gone, and the three
  loopback probes remained. No variable in the profile stops them.
- The real run: `opencode.ai:443`, the OpenCode Go provider, twice, and the three
  loopback probes. It also connected to `api.individual.githubcopilot.com:443`, because
  the host's login file also holds a GitHub Copilot login. OpenCode tried to sync that
  provider's models, and the request failed with 403 (logged as the warning
  `failed to sync GitHub Copilot models`). A worker
  given the whole login file under an allowlist of the provider's name only would
  produce refusals for each such host.
- Through an allowlisting CONNECT proxy, the loopback probes are plain HTTP proxy
  requests, not CONNECTs, so the egress proxy would refuse them as non-CONNECT requests
  rather than log them as refused names.

## Not established

- The spec's smoke step 4, that the approved image holds no managed configuration, needs
  the person's approved image, and no image was used.
- Whether these results hold at 1.19.0: no 1.19.0 binary was run.
- The cause of the one 500 (finding 2).
