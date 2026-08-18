# MCP server

`agentsview mcp` exposes your local session archive to an MCP client — Claude
Code, Claude Desktop, or any agent that speaks the Model Context Protocol —
as a set of read-only tools.

```bash
agentsview mcp            # stdio (the default; what a client config launches)
agentsview mcp --http 8085
```

## What it is not

The server is read-only by contract, not by accident:

- Every tool is a read. There is no write, no delete, and no sync trigger.
- All six tools carry the MCP `readOnlyHint` annotation, and a test asserts both
  the exact tool set and the annotation, so adding a mutating tool fails CI.
- Even the `--server` backend is built in read-only mode, so a tool could not
  issue a write to a remote server that would accept one.

The reason is prompt injection. An MCP client is an LLM acting on its own
judgement over text it just read; several of those texts are other agents'
transcripts. A mutation path from the tool surface into the archive would let
text stored in the archive rewrite the archive.

## Tools

| Tool | What it returns |
| --- | --- |
| `search_sessions` | Full-text search; one best-ranked snippet per matching session |
| `list_sessions` | Sessions filtered by project, agent, machine, git branch or date, newest first |
| `get_session_overview` | One session's metadata plus its first and last few messages |
| `get_session_messages` | One page of a transcript, ascending or descending |
| `list_session_tool_calls` | The tool calls made during one session, in transcript order |
| `get_usage_summary` | Token and cost totals over a date range, with breakdowns |

Shared conventions:

- **Paging.** `search_sessions` and `list_sessions` return `next_cursor`;
  `get_session_messages` returns `next_from`. Pass it back to continue; its
  absence means the results are exhausted.
- **Limits.** `limit` defaults to 20 for the listing tools and 50 for messages
  and tool calls, clamped to 100 and 200 respectively. `get_usage_summary`'s
  `top` defaults to 10, clamped to 50.
- **Truncation.** Long strings are cut at a rune boundary and flagged:
  message content at 2000 runes (`content_truncated`), search snippets at 400,
  tool-call input at 1000 (`input_truncated`).
- **System messages.** System-injected messages are excluded unless
  `include_system` is set.
- **Active sessions.** Sessions written within the last 10 minutes are withheld
  from `search_sessions` and `list_sessions`, and `excluded_active` says how
  many. The caller is usually an agent whose own transcript is being appended to
  this archive right now; that session would otherwise match every search for
  whatever the agent is currently working on. Set `include_active` to opt in.
- **`search_sessions` queries** are matched as a literal phrase, so punctuation
  and quotes are safe to pass through verbatim.

## Client configuration

The default transport is stdio, which is what MCP clients launch. For Claude
Code:

```bash
claude mcp add agentsview -- agentsview mcp
```

Or, in a client that takes JSON:

```json
{
  "mcpServers": {
    "agentsview": {
      "command": "agentsview",
      "args": ["mcp"]
    }
  }
}
```

Use an absolute path to the binary if the client does not inherit your `PATH`.

## Where the data comes from

`agentsview mcp` never opens `sessions.db` itself. The archive has a single
writable owner at a time, held by a lock in the data directory; an MCP server is
long-lived and started by a client, so opening the file here would make it a
second reader of a database another process is mid-transaction on.

Instead it picks one of three backends:

| Flag | Backend |
| --- | --- |
| *(none)* | The local `agentsview` daemon, started on demand |
| `--server <url>` | An already running agentsview server |
| `--pg` | The configured PostgreSQL mirror |

`--server` and `--pg` are mutually exclusive.

### Daemon wake-up (default)

With no backend flag, each tool call resolves a running daemon and talks HTTP to
it. If none is running, the command starts one through the same canonical
background start that `agentsview daemon start` uses, then waits for it to
publish a runtime record before issuing the call. A started process that has not
published a record yet is treated as not reachable — its port is unknown and
its database may still be migrating.

Resolution happens per call, not once at startup: a daemon can be stopped,
restarted or upgraded between two tool calls, and a cached address would leave
the client failing for the rest of the session.

If the daemon starts but never publishes a record, the tool call fails with a
pointer to `agentsview daemon status`.

### `--server`

```bash
agentsview mcp --server http://127.0.0.1:8080
agentsview mcp --server https://box.internal:8080 \
  --server-token-file ~/.agentsview/token
```

Reads from a server you already run instead of starting a daemon. The bearer
token comes from your config, or from `--server-token-file` when given.
A token file that exists but holds only whitespace is an error rather than "no
token": that is nearly always a half-written secret, and continuing
unauthenticated is the wrong recovery.

### `--pg`

Reads from the PostgreSQL mirror configured in `[pg]` (see the PostgreSQL
section of the README). Useful on a machine that has no local archive of its
own.

## HTTP transport

`--http` swaps stdio for MCP's streamable HTTP transport, for clients that
cannot spawn a process.

```bash
agentsview mcp --http 8085                       # binds 127.0.0.1:8085
agentsview mcp --http 0.0.0.0:8085 --http-allow-insecure   # requires a token
```

Three rules are enforced when the listener is resolved, before anything binds,
so a misconfiguration cannot start listening at all:

1. **A bare port binds loopback.** `8085` and `:8085` both mean
   `127.0.0.1:8085`. Go would read `:8085` as every interface; a port in a
   config file does not mean "publish my transcripts to the network".
2. **Leaving loopback is an explicit opt-in.** Any non-loopback bind address is
   refused unless you pass `--http-allow-insecure`. A hostname other than
   `localhost` is never treated as loopback even if it currently resolves there.
3. **Off loopback, a token is mandatory.** `--http-allow-insecure` says where to
   listen, never who may read. Binding a non-loopback address with no configured
   auth token is refused.

Authentication itself:

- The token is the same `auth_token` the rest of agentsview uses. On loopback it
  is optional unless `require_auth` is set, in which case a token is generated
  and printed once.
- Requests carry it as `Authorization: Bearer <token>`. A missing or wrong token
  gets a `401` with a `WWW-Authenticate` challenge. Both sides are hashed before
  the constant-time comparison, so neither the token nor its length leaks
  through response timing.
- Auth wraps the MCP handler itself, so it gates tool calls, not just the
  initialize handshake.
- The SDK's DNS-rebinding guard stays on: a request that arrives on the loopback
  socket carrying a non-loopback `Host` header is rejected, which is the
  shape of a browser page attacking a local tool server.
- The provisioned token is printed to **stderr**, never stdout, because
  stdout is the MCP stream in the default mode.

## Security limits

- Read-only, as described above; the tool inventory is asserted by test.
- The command loads config read-only: it does not migrate config files or
  persist a cursor secret as a side effect of a client launching it in the
  background.
- Everything stays on your machine. The server makes no outbound request beyond
  the backend you selected.
- Transcripts are untrusted input. Content that reaches an MCP client came from
  agent sessions and may contain injected instructions; see
  [SECURITY.md](../SECURITY.md) for the trust model.
- The HTTP transport has no TLS of its own. Off-loopback exposure should go
  through a reverse proxy or an SSH tunnel, with the bearer token still in
  place.
