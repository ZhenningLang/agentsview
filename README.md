# agentsview

**English** | [简体中文](README.zh-CN.md)

Browse, search, and track costs across all your AI coding agents. One binary, no
accounts, everything local.

<p align="center">
  <img src="https://agentsview.io/screenshots/dashboard.png" alt="Analytics dashboard" width="720">
</p>

## Why agentsview

Your AI coding sessions are scattered across a dozen tools, directories, and
file formats. agentsview pulls them into one place:

- **One archive for every agent** — auto-discovers sessions from 30+ agents
  (Claude Code, Codex, Cursor, Kimi, Copilot, ...) and indexes them into a local
  SQLite database with FTS5 full-text search.
- **Cost and token tracking** — daily and per-model costs across *all* your
  agents, with cache-aware pricing. A fast local replacement for ccusage-style
  tools.
- **Local first** — a single binary, no accounts, no cloud. The server binds to
  `127.0.0.1`; nothing leaves your machine unless you explicitly push it
  (PostgreSQL, DuckDB, or Gist).

## Install

```bash
# macOS / Linux
curl -fsSL https://agentsview.io/install.sh | bash

# Windows
powershell -ExecutionPolicy ByPass -c "irm https://agentsview.io/install.ps1 | iex"
```

Or install the **AgentsView desktop app** (macOS / Windows) from
[GitHub Releases](https://github.com/kenn-io/agentsview/releases), or via
Homebrew:

```bash
brew install --cask agentsview
```

Or run the Docker image — see [Docker](#docker).

## Quick start

```bash
agentsview serve           # sync sessions and serve the web UI
agentsview usage daily     # print a daily cost summary
```

On first run, agentsview discovers sessions from every supported agent on your
machine, syncs them into a local SQLite archive, and serves the web UI at
`http://127.0.0.1:8080`. From there, a file watcher plus a periodic sync every
15 minutes keeps the archive current, and the UI updates live via SSE.

Full documentation lives at **[agentsview.io](https://agentsview.io)**.

## Feature tour

### Browse and search every session

| Dashboard                                                     | Session viewer                                                          |
| ------------------------------------------------------------- | ----------------------------------------------------------------------- |
| ![Dashboard](https://agentsview.io/screenshots/dashboard.png) | ![Session viewer](https://agentsview.io/screenshots/message-viewer.png) |

| Search                                                          | Activity heatmap                                          |
| --------------------------------------------------------------- | --------------------------------------------------------- |
| ![Search](https://agentsview.io/screenshots/search-results.png) | ![Heatmap](https://agentsview.io/screenshots/heatmap.png) |

- **Full-text search** across all message content (SQLite FTS5).
- **Live updates** via SSE as active sessions receive new messages.
- **Keyboard-first** navigation — `j`/`k`, `[`/`]`, `Cmd+K` search, `?` lists
  all shortcuts.
- **Context events** — stored system messages, resume/interruption markers,
  command messages, and stop-hook feedback render as compact expandable cards,
  kept distinct from normal user/assistant turns. agentsview only displays
  context that the agent actually persisted; runtime prompts or hooks that are
  never written to disk cannot be reconstructed.
- **Resume commands** — the UI shows copyable terminal commands for supported
  agents. It never executes local commands or attaches to terminals itself;
  agents with unknown resume behavior are shown as unsupported rather than
  receiving a fabricated command.
- **Export** sessions as standalone HTML, or publish them to GitHub Gist.
- Web UI available in English and 简体中文.

### Track token usage and cost

`agentsview usage` tracks token consumption and compute costs across **all**
your coding agents — not just Claude Code. Because session data is already
indexed in SQLite, queries are over 100x faster than tools that re-parse raw
session files on every run.

```bash
agentsview usage daily                         # last 30 days (default)
agentsview usage daily --breakdown             # per-model breakdown
agentsview usage daily --agent claude --since 2026-04-01
agentsview usage daily --all --json            # for scripting
agentsview usage statusline                    # one-liner for shell prompts
```

- Automatic pricing via LiteLLM rates (with offline fallback)
- Prompt-caching-aware cost calculation (cache creation / read tokens)
- Droid sessions ingest token totals from each session's optional
  `.settings.json` `tokenUsage` block; unpriced custom models still report token
  counts and are listed under `unpriced_models`
- Timezone-aware date bucketing (`--timezone`), plus date and agent filtering
  (`--since`, `--until`, `--all`, `--agent`)
- Works standalone — no server required

Per-session numbers:

```bash
agentsview session usage <id>                  # tokens + cost estimate
agentsview session usage <id> --format json
```

The same data is available over HTTP at `GET /api/v1/sessions/{id}/usage`.
Existing sessions return `200` even when token or cost data is absent; missing
sessions return `404`. The deprecated alias `agentsview token-use <id>` still
works and also reports cost estimates.

### Analytics

```bash
agentsview stats                 # human-readable summary, last 28 days
agentsview stats --format json   # versioned v1 schema for downstream tools
```

`agentsview stats` reports window-scoped totals, session archetypes (automation
vs. quick/standard/deep/marathon), distributions for duration, user-message
count, peak context, and tools-per-turn, plus cache economics, tool/model/agent
mix, and an hourly temporal breakdown. Git-derived outcome metrics are opt-in
because they can be slow on large repos: `--include-git-outcomes`
(commits/LOC/files changed) and `--include-github-outcomes` (PR counts via `gh`,
also enables git outcomes).

The web UI adds activity heatmaps, tool-usage and velocity charts, project
breakdowns, token-generation-speed charts, and trend views.

### Optional LLM features

LLM features are off by default. Configure providers in the web UI (Settings) or
via `[llm]` in `config.toml`:

- `agentsview enrich` — offline LLM enrichment for local sessions (`--all`,
  `--project`, `--force`, `--limit`).
- **Semantic search** — vector search in the UI when an embedding provider is
  configured.

### Secrets scanning

```bash
agentsview secrets scan    # full-ruleset scan, persists findings
agentsview secrets list    # list findings (redacted by default)
```

Scan results are also visible in the web UI.

### Multi-machine and team setups

- **Remote sync over SSH** — `agentsview sync --host <machine>` pulls session
  files from remote machines; `[[remote_hosts]]` entries in the config fan out
  to many.
- **PostgreSQL** — push to a shared instance for team dashboards; see
  [PostgreSQL sync](#postgresql-sync).
- **DuckDB** — build a portable analytics mirror or serve remote read access
  over Quack; see [DuckDB mirror and Quack](#duckdb-mirror-and-quack).

## Supported agents

agentsview auto-discovers sessions from all of these:

| Agent              | Session Directory                                                           |
| ------------------ | --------------------------------------------------------------------------- |
| Claude Code        | `~/.claude/projects/`                                                       |
| Codex              | `~/.codex/sessions/`, `~/.codex/archived_sessions/`                         |
| Copilot            | `~/.copilot/`                                                               |
| Gemini             | `~/.gemini/`                                                                |
| Droid              | `~/.factory/sessions/`                                                      |
| Kilo               | `~/.local/share/kilo/`                                                      |
| OpenCode           | `~/.local/share/opencode/`                                                  |
| OpenHands CLI      | `~/.openhands/conversations/`                                               |
| Cursor             | `~/.cursor/projects/`                                                       |
| Amp                | `~/.local/share/amp/threads/`                                               |
| iFlow              | `~/.iflow/projects/`                                                        |
| Zencoder           | `~/.zencoder/sessions/`                                                     |
| Command Code       | `~/.commandcode/projects/`                                                  |
| OpenClaw           | `~/.openclaw/agents/`                                                       |
| QClaw              | `~/.qclaw/agents/`                                                          |
| Kimi               | `~/.kimi/sessions/`                                                         |
| Kimi Code          | `~/.kimi-code/sessions/`                                                    |
| Kiro               | `~/.kiro/sessions/cli/`, `~/.local/share/kiro-cli/`                         |
| Kiro IDE           | `Kiro/User/globalStorage/kiro.kiroagent/` under the per-OS config root      |
| Cortex Code        | `~/.snowflake/cortex/conversations/`                                        |
| Hermes Agent       | `~/.hermes/sessions/`                                                       |
| WorkBuddy          | `~/.workbuddy/projects/`                                                    |
| Pi                 | `~/.pi/agent/sessions/`                                                     |
| Qwen Code          | `~/.qwen/projects/`                                                         |
| Forge              | `~/.forge/`                                                                 |
| Piebald            | `~/.local/share/piebald/` (Linux; per-OS app-data dir elsewhere)            |
| Warp               | per-OS app-data dir (e.g. Linux `~/.local/state/warp-terminal/`)            |
| Positron Assistant | `~/Library/Application Support/Positron/User/` (macOS)                      |
| Zed                | `~/Library/Application Support/Zed/` (macOS), `~/.local/share/zed/` (Linux) |
| VSCode Copilot     | VS Code user-data dir on all OSes (Code, Insiders, VSCodium)                |
| Antigravity        | `~/.gemini/antigravity/`                                                    |
| Antigravity CLI    | `~/.gemini/antigravity-cli/` (see note below)                               |
| Claude.ai          | not file-based — `agentsview import --type claude-ai <path>`                |
| ChatGPT            | not file-based — `agentsview import --type chatgpt <path>`                  |

Each directory can be overridden with an environment variable — see the
[configuration docs](https://agentsview.io/configuration/) for the per-agent
variable names. Multi-directory setups are supported, for example:

```toml
droid_sessions_dirs = ["/factory/sessions/a", "/factory/sessions/b"]
kilo_dirs = ["/Users/me/.local/share/kilo", "/Volumes/work/kilo"]
```

Notes:

- **Droid** parsing reads the JSONL transcript and, when present, the sibling
  `<session-id>.settings.json` file for usage totals (`inputTokens`,
  `outputTokens`, cache creation/read tokens, thinking tokens). Archive session
  IDs use the `droid:` prefix. Source files are read-only inputs; agentsview
  stores parsed records in its own archive and never writes back.
- **Kilo** archive IDs use the `kilo:` prefix; the resume command strips it:
  `kilo --session <session-id>`.
- **Claude.ai / ChatGPT** have no on-disk session files; import conversation
  exports with `agentsview import --type <type> <path>`.

### Antigravity CLI: high-resolution transcripts

Antigravity CLI sessions appear in two on-disk formats. Newer releases store
conversation trajectories as SQLite `.db` files, which agentsview indexes
directly. Older releases stored assistant turns and tool calls in
AES-GCM-encrypted `.pb` files; for those sessions, agentsview falls back to
**summary mode** using your prompts from `history.jsonl` plus any plain-text
artifacts under `brain/` (plans, walkthroughs, checkpoints).

To unlock full transcripts for older `.pb` sessions, run
[agy-reader](https://github.com/mjacobs/agy-reader) alongside agentsview. It
talks to the local Antigravity daemon, decrypts each conversation, and writes a
`<uuid>.trajectory.json` sidecar next to the encrypted `.pb` file. agentsview's
file watcher detects the sidecar automatically and parses it in place of summary
mode — no restart needed:

```bash
go install github.com/mjacobs/agy-reader@latest
agy-reader --sync     # generate sidecars for existing sessions
agy-reader --watch    # ...or keep them fresh as you work
```

Sidecars stay on your machine. agentsview makes no outbound request to produce
or read them, and treats sidecars as untrusted structured input — see
[SECURITY.md](SECURITY.md) for the trust model.

## CLI at a glance

| Command                                       | What it does                                                                          |
| --------------------------------------------- | ------------------------------------------------------------------------------------- |
| `agentsview serve`                            | Start the server and web UI (default port 8080)                                       |
| `agentsview sync`                             | Sync session data without serving (`--host` for SSH)                                  |
| `agentsview usage daily` / `usage statusline` | Token cost reports                                                                    |
| `agentsview stats`                            | Window-scoped analytics (default: last 28 days)                                       |
| `agentsview session ...`                      | `list`, `get`, `messages`, `tool-calls`, `search`, `usage`, `watch`, `export`, `sync` |
| `agentsview import --type <type> <path>`      | Import Claude.ai / ChatGPT exports                                                    |
| `agentsview enrich`                           | Offline LLM enrichment (needs `[llm]` config)                                         |
| `agentsview secrets scan` / `secrets list`    | Detect leaked secrets in sessions                                                     |
| `agentsview prune`                            | Delete sessions matching filters (`--dry-run` first)                                  |
| `agentsview projects` / `health`              | List projects; show session health signals                                            |
| `agentsview pg ...`                           | PostgreSQL: `push`, `status`, `serve`, `service`                                      |
| `agentsview duckdb ...`                       | DuckDB: `push`, `status`, `serve`, `quack serve`                                      |
| `agentsview update` / `version`               | Self-update and version info                                                          |
| `agentsview openapi`                          | Print the REST API's OpenAPI 3.1 schema                                               |

Run `agentsview <command> --help` for the full flag list.

## Remote / forwarded access

agentsview binds to loopback and validates the request `Host` header to guard
against DNS-rebinding attacks. When you reach it through SSH port-forwarding, a
reverse proxy, or a remote dev environment (exe.dev, Codespaces, Coder, WSL2),
the browser sends a `Host` that the server does not recognize, so API requests
are rejected with `403 Forbidden` (the response body explains the fix).

Restart the server with `--public-url` set to the exact origin you open in the
browser:

```bash
# Browser opens http://127.0.0.1:18080 via `ssh -L 18080:127.0.0.1:8080 host`
agentsview serve --public-url http://127.0.0.1:18080

# Browser opens a forwarded hostname
agentsview serve --public-url https://your-workspace.exe.dev
```

Use `--public-origin` (repeatable or comma-separated) to trust additional
browser origins. If you expose the UI beyond loopback, also enable
`--require-auth`.

## Docker

```bash
docker run --rm -p 127.0.0.1:8080:8080 \
  -v agentsview-data:/data \
  -v "$HOME/.claude/projects:/agents/claude:ro" \
  -v "$HOME/.forge:/agents/forge:ro" \
  -e CLAUDE_PROJECTS_DIR=/agents/claude \
  -e FORGE_DIR=/agents/forge \
  ghcr.io/kenn-io/agentsview:latest
```

A containerized agentsview can only discover agent sessions from directories you
explicitly mount into the container and point at with the matching env var.
`docker-compose.prod.yaml` is included as a production example:

```bash
docker compose -f docker-compose.prod.yaml up -d
```

Notes:

- The container runs as root, so prefer a named volume for `/data` over a host
  bind mount; if you do bind-mount, pre-create the directory with the desired
  ownership to avoid root-owned files in your home directory.
- The examples publish the UI on loopback only (`127.0.0.1`). To expose it
  beyond localhost, enable `--require-auth` and publish the port intentionally.
- Set `PG_SERVE=1` to switch the startup command to `agentsview pg serve` (with
  `AGENTSVIEW_PG_URL` pointing at your instance).

## PostgreSQL sync

Push session data to a shared PostgreSQL instance for team dashboards:

```bash
agentsview pg push       # push local data to PG
agentsview pg serve      # serve web UI from PG (read-only)
```

To keep a shared database current without running `pg push` by hand, run the
auto-push daemon. It watches your session directories and pushes shortly after
new sessions are recorded, with a periodic floor as a safety net:

```bash
agentsview pg push --watch                 # foreground, Ctrl-C to stop
agentsview pg push --watch --debounce 1m   # custom coalesce window
agentsview pg push --watch --interval 5m   # custom floor interval
```

To run it unattended as an OS service (launchd on macOS, `systemd --user` on
Linux):

```bash
agentsview pg service install     # generate the unit, enable + start it
agentsview pg service status      # show manager status (also start/stop)
agentsview pg service logs -f     # follow the service log
agentsview pg service uninstall   # stop and remove
```

The daemon reads the same `[pg]` config as `pg push`, so the PostgreSQL DSN must
be set in your config file. Protect it, since it holds credentials:
`chmod 600 ~/.agentsview/config.toml`.

**Linux headless machines:** systemd `--user` services stop at logout unless
lingering is enabled: `loginctl enable-linger "$USER"`. See the
[PostgreSQL docs](https://agentsview.io/postgresql/) for setup.

## DuckDB mirror and Quack

DuckDB support is a mirror backend, not a replacement for the local SQLite
archive. `agentsview serve` still performs primary ingestion into SQLite. Use
DuckDB when you want a portable analytics file, read-only local serving from a
mirror, or remote read access through DuckDB's Quack protocol.

```bash
agentsview duckdb push          # mirror SQLite into DuckDB
agentsview duckdb status        # show mirror sync status
agentsview duckdb serve         # serve web UI from DuckDB (read-only)
agentsview duckdb quack serve   # expose the local DuckDB file over Quack
```

`duckdb serve` reads `[duckdb].path` or `AGENTSVIEW_DUCKDB_PATH`. To serve from
a remote Quack endpoint, set `AGENTSVIEW_DUCKDB_URL` and
`AGENTSVIEW_DUCKDB_TOKEN` instead. Quack is still a new protocol, so agentsview
keeps conservative defaults: local Quack serving binds to loopback, requires a
token, and rejects non-loopback plain HTTP unless `--allow-insecure` is
explicit. For remote use, prefer a TLS URL or put Quack behind an authenticated
tunnel/proxy.

Backend modes at a glance:

- **SQLite**: primary local archive — file sync, FTS5 search, writable UI.
- **PostgreSQL**: optional shared team backend — push from SQLite, serve
  read-only.
- **DuckDB**: optional mirror file or Quack endpoint — push from SQLite, serve
  read-only. DuckDB search currently uses substring/regex fallback; SQLite FTS5
  remains the indexed search path for primary local serving.

## Privacy

agentsview sends a limited anonymous `daemon_active` telemetry ping to PostHog
when the server starts and every 24 hours while it runs, using a stable random
install ID as the event `DistinctId`. The event includes
`application=agentsview`, app version, commit, OS, and CPU architecture, with
`$process_person_profile=false` and `$geoip_disable=true`. It does not include
session, project, prompt, file path, account, or machine identity. Disable
telemetry with `AGENTSVIEW_TELEMETRY_ENABLED=0` or `TELEMETRY_ENABLED=0`.
Telemetry is also hard-disabled in Go test binaries, regardless of environment.

All session data stays on your machine. The server binds to `127.0.0.1` by
default. The update check is optional and can be disabled with
`--no-update-check`.

## Development

Requires Go 1.26+ (CGO) and Node.js 22+.

```bash
make dev            # Go server (dev mode)
make frontend-dev   # Vite dev server (run alongside make dev)
make build          # build binary with embedded frontend
make install        # install to ~/.local/bin
```

```bash
make test           # Go tests (CGO_ENABLED=1 -tags "fts5,kit_posthog_disabled")
make lint           # golangci-lint + NilAway
make e2e            # Playwright E2E tests
make bench-backends # compare SQLite, DuckDB, and PostgreSQL reads (needs Docker)
```

Pre-commit hooks via [prek](https://github.com/j178/prek): run `make lint-tools`
and `make install-hooks` after cloning.

### Project layout

```
cmd/agentsview/     CLI entrypoint
internal/           Go packages (config, db, parser, server, sync, postgres)
frontend/           Svelte 5 SPA (Vite, TypeScript)
desktop/            Tauri desktop wrapper
```

## Documentation

Full docs at **[agentsview.io](https://agentsview.io)**:
[Quick Start](https://agentsview.io/quickstart/) --
[Usage Guide](https://agentsview.io/usage/) --
[CLI Reference](https://agentsview.io/commands/) --
[Configuration](https://agentsview.io/configuration/) --
[Architecture](https://agentsview.io/architecture/)

## Acknowledgements

Inspired by
[claude-history-tool](https://github.com/andyfischer/ai-coding-tools/tree/main/claude-history-tool)
by Andy Fischer and
[claude-code-transcripts](https://github.com/simonw/claude-code-transcripts) by
Simon Willison.

## License

MIT
