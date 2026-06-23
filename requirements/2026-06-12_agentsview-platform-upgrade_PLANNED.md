# Agentsview Platform Upgrade Requirement

## Goal

Upgrade the forked `agentsview` project into the primary local-first platform for browsing, searching, and managing coding-agent sessions, with first-class Kilo support and a browser-friendly resume workflow.

## Background

The existing `mantis` project has useful terminal-oriented features, but `agentsview` is a broader platform: it already has a local SQLite archive, parser registry, sync pipeline, web UI, FTS-backed search, session viewer, and analytics. The intended direction is to build on `agentsview` rather than recreating those platform capabilities inside `mantis`.

## Core Requirements

1. Kilo first-class support

- Add Kilo as a distinct supported agent, not as OpenCode.
- Discover Kilo sessions from `~/.local/share/kilo/` by default, with environment/config override following existing agentsview conventions.
- Read `kilo.db` and index session metadata, messages, parts, tools, and searchable text into agentsview's local archive.
- Store Kilo sessions with a stable `kilo:` ID prefix to avoid collisions with OpenCode and other agents.
- Preserve agent identity as `agent = "kilo"` for filtering, analytics, usage, and UI badges.
- Reuse/genericize OpenCode-family SQLite parsing where safe, but avoid hard-coded `opencode:` prefixes, agent names, and db file names.
- Do not read Kilo auth/account secrets as part of normal session indexing.

2. Full session search

- Global search must cover all indexed agents, including Kilo.
- Search must include message text and session title/name/first-message fields.
- Kilo message text must be indexed into `messages` and FTS so browser search can find real Kilo transcript content.
- Search results should preserve agent/project/session context and remain filterable by agent where the existing UI supports it.

3. Session title rewrite

- Support user-controlled session title override in agentsview's own archive/UI layer.
- Do not write title changes back into original agent databases/files by default.
- Manual title edit should be the first implementation target.
- If an LLM-assisted title suggestion is added later, it must be explicitly triggered and reviewable, not silently run across all sessions.
- Title display/search should prefer user override, then agent-provided session name/title, then first message.

4. Browser-friendly resume workflow

- The browser UI cannot directly `--resume` into a terminal session.
- Instead, for each session expose the appropriate resume command in the UI, such as a copyable command field/button.
- Kilo sessions should show a command equivalent to resuming that session from a terminal.
- Claude/OpenCode/Codex/etc. should use their agent-specific command conventions where available.
- If an agent's resume command is unknown or unsupported, show that clearly instead of pretending it can resume.
- Do not attempt to execute local terminal commands from the browser.

## Non-Goals For Initial Delivery

- Do not migrate all `mantis` features immediately.
- Do not implement destructive operations such as deleting original agent sessions unless already supported safely by agentsview.
- Do not push to remote or create releases automatically.
- Do not modify secrets, auth files, or production resources.
- Do not make the browser execute resume commands directly.

## Acceptance Criteria

- `agentsview` includes Kilo in its supported agent registry/configuration.
- A local Kilo database with `session`, `message`, and `part` tables is parsed into agentsview sessions/messages.
- Indexed Kilo transcript text is searchable through the existing search path.
- Kilo sessions are distinguishable from OpenCode sessions in stored data and UI/API output.
- UI/API exposes a resume command string for sessions whose agent supports a known resume command.
- Manual title override persists in agentsview's archive and is used in display/search without modifying original agent data.
- Tests cover Kilo parsing, ID prefixing, search indexing, resume command rendering, and title override behavior.
- Local verification commands pass, including Go tests relevant to parser/db/server and frontend build/tests where affected.

## Browser Resume Constraint

Because the browser cannot attach to or resume an interactive terminal process, the UI must present commands for users to copy and run manually. This is a product requirement, not a workaround. The implementation should make this explicit and safe.

## Suggested Phases

1. Kilo ingestion and parser support.
2. Search/index verification and UI surfacing for Kilo.
3. Resume command API/UI.
4. Manual session title override API/UI.
5. Final acceptance and documentation update.
