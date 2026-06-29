-- Sessions table
CREATE TABLE IF NOT EXISTS sessions (
    id          TEXT PRIMARY KEY,
    project     TEXT NOT NULL,
    machine     TEXT NOT NULL DEFAULT 'local',
    agent       TEXT NOT NULL DEFAULT 'claude',
    first_message TEXT,
    display_name TEXT,
    session_name TEXT,
    started_at  TEXT,
    ended_at    TEXT,
    message_count INTEGER NOT NULL DEFAULT 0,
    user_message_count INTEGER NOT NULL DEFAULT 0,
    file_path   TEXT,
    file_size   INTEGER,
    file_mtime  INTEGER,
    file_inode  INTEGER,
    file_device INTEGER,
    file_hash   TEXT,
    local_modified_at TEXT,
    parent_session_id TEXT,
    relationship_type TEXT NOT NULL DEFAULT '',
    total_output_tokens INTEGER NOT NULL DEFAULT 0,
    peak_context_tokens INTEGER NOT NULL DEFAULT 0,
    has_total_output_tokens INTEGER NOT NULL DEFAULT 0,
    has_peak_context_tokens INTEGER NOT NULL DEFAULT 0,
    is_automated INTEGER NOT NULL DEFAULT 0,
    tool_failure_signal_count INTEGER NOT NULL DEFAULT 0,
    tool_retry_count INTEGER NOT NULL DEFAULT 0,
    edit_churn_count INTEGER NOT NULL DEFAULT 0,
    consecutive_failure_max INTEGER NOT NULL DEFAULT 0,
    outcome TEXT NOT NULL DEFAULT 'unknown',
    outcome_confidence TEXT NOT NULL DEFAULT 'low',
    ended_with_role TEXT NOT NULL DEFAULT '',
    final_failure_streak INTEGER NOT NULL DEFAULT 0,
    signals_pending_since TEXT,
    compaction_count INTEGER NOT NULL DEFAULT 0,
    mid_task_compaction_count INTEGER NOT NULL DEFAULT 0,
    context_pressure_max REAL,
    health_score INTEGER,
    health_grade TEXT,
    has_tool_calls INTEGER NOT NULL DEFAULT 0,
    has_context_data INTEGER NOT NULL DEFAULT 0,
    data_version INTEGER NOT NULL DEFAULT 0,
    cwd TEXT NOT NULL DEFAULT '',
    git_branch TEXT NOT NULL DEFAULT '',
    source_session_id TEXT NOT NULL DEFAULT '',
    source_version TEXT NOT NULL DEFAULT '',
    parser_malformed_lines INTEGER NOT NULL DEFAULT 0,
    is_truncated INTEGER NOT NULL DEFAULT 0,
    deleted_at  TEXT,
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    termination_status TEXT,
    secret_leak_count INTEGER NOT NULL DEFAULT 0,
    secrets_rules_version TEXT NOT NULL DEFAULT '',
    llm_title TEXT NOT NULL DEFAULT '',
    llm_summary TEXT NOT NULL DEFAULT '',
    llm_keywords TEXT NOT NULL DEFAULT '',
    llm_embedding BLOB,
    llm_embedding_dim INTEGER NOT NULL DEFAULT 0,
    enriched_at TEXT NOT NULL DEFAULT '',
    enriched_msg_count INTEGER NOT NULL DEFAULT 0,
    enrich_model TEXT NOT NULL DEFAULT '',
    enrich_status TEXT NOT NULL DEFAULT '',
    enrich_error TEXT NOT NULL DEFAULT ''
);

-- Messages table with ordinal for efficient range queries
CREATE TABLE IF NOT EXISTS messages (
    id             INTEGER PRIMARY KEY,
    session_id     TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    ordinal        INTEGER NOT NULL,
    role           TEXT NOT NULL,
    content        TEXT NOT NULL,
    thinking_text  TEXT NOT NULL DEFAULT '',
    timestamp      TEXT,
    has_thinking   INTEGER NOT NULL DEFAULT 0,
    has_tool_use   INTEGER NOT NULL DEFAULT 0,
    content_length INTEGER NOT NULL DEFAULT 0,
    is_system      INTEGER NOT NULL DEFAULT 0,
    model TEXT NOT NULL DEFAULT '',
    token_usage TEXT NOT NULL DEFAULT '',
    context_tokens INTEGER NOT NULL DEFAULT 0,
    output_tokens INTEGER NOT NULL DEFAULT 0,
    has_context_tokens INTEGER NOT NULL DEFAULT 0,
    has_output_tokens INTEGER NOT NULL DEFAULT 0,
    claude_message_id TEXT NOT NULL DEFAULT '',
    claude_request_id TEXT NOT NULL DEFAULT '',
    source_type TEXT NOT NULL DEFAULT '',
    source_subtype TEXT NOT NULL DEFAULT '',
    source_uuid TEXT NOT NULL DEFAULT '',
    source_parent_uuid TEXT NOT NULL DEFAULT '',
    is_sidechain INTEGER NOT NULL DEFAULT 0,
    is_compact_boundary INTEGER NOT NULL DEFAULT 0,
    UNIQUE(session_id, ordinal)
);

-- Stats table maintained by triggers
CREATE TABLE IF NOT EXISTS stats (
    key   TEXT PRIMARY KEY,
    value INTEGER NOT NULL DEFAULT 0
);

INSERT OR IGNORE INTO stats (key, value) VALUES ('session_count', 0);
INSERT OR IGNORE INTO stats (key, value) VALUES ('message_count', 0);

-- Triggers for stats maintenance
CREATE TRIGGER IF NOT EXISTS sessions_insert_stats AFTER INSERT ON sessions BEGIN
    UPDATE stats SET value = value + 1 WHERE key = 'session_count';
END;

CREATE TRIGGER IF NOT EXISTS sessions_delete_stats AFTER DELETE ON sessions BEGIN
    UPDATE stats SET value = value - 1 WHERE key = 'session_count';
END;

CREATE TRIGGER IF NOT EXISTS messages_insert_stats AFTER INSERT ON messages BEGIN
    UPDATE stats SET value = value + 1 WHERE key = 'message_count';
END;

CREATE TRIGGER IF NOT EXISTS messages_delete_stats AFTER DELETE ON messages BEGIN
    UPDATE stats SET value = value - 1 WHERE key = 'message_count';
END;

-- Indexes
CREATE INDEX IF NOT EXISTS idx_sessions_ended
    ON sessions(ended_at DESC, id);
CREATE INDEX IF NOT EXISTS idx_sessions_project
    ON sessions(project);
CREATE INDEX IF NOT EXISTS idx_sessions_machine
    ON sessions(machine);
CREATE INDEX IF NOT EXISTS idx_messages_session_ordinal
    ON messages(session_id, ordinal);
CREATE INDEX IF NOT EXISTS idx_messages_velocity
    ON messages(session_id, ordinal, role, timestamp, content_length);
CREATE INDEX IF NOT EXISTS idx_messages_session_role
    ON messages(session_id, role);

CREATE INDEX IF NOT EXISTS idx_sessions_parent
    ON sessions(parent_session_id)
    WHERE parent_session_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_sessions_file_path
    ON sessions(file_path)
    WHERE file_path IS NOT NULL;

-- Analytics indexes
CREATE INDEX IF NOT EXISTS idx_sessions_started
    ON sessions(started_at);
CREATE INDEX IF NOT EXISTS idx_sessions_message_count
    ON sessions(message_count);
CREATE INDEX IF NOT EXISTS idx_sessions_user_message_count
    ON sessions(user_message_count);
CREATE INDEX IF NOT EXISTS idx_sessions_agent
    ON sessions(agent);

-- Session-level usage events. These complement message-level
-- messages.token_usage rows for agents that only expose aggregate
-- session accounting.
CREATE TABLE IF NOT EXISTS usage_events (
    id INTEGER PRIMARY KEY,
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    message_ordinal INTEGER,
    source TEXT NOT NULL,
    model TEXT NOT NULL,
    input_tokens INTEGER NOT NULL DEFAULT 0,
    output_tokens INTEGER NOT NULL DEFAULT 0,
    cache_creation_input_tokens INTEGER NOT NULL DEFAULT 0,
    cache_read_input_tokens INTEGER NOT NULL DEFAULT 0,
    reasoning_tokens INTEGER NOT NULL DEFAULT 0,
    cost_usd REAL,
    cost_status TEXT NOT NULL DEFAULT '',
    cost_source TEXT NOT NULL DEFAULT '',
    occurred_at TEXT,
    dedup_key TEXT NOT NULL DEFAULT ''
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_usage_events_dedup
    ON usage_events(session_id, source, dedup_key)
    WHERE dedup_key != '';
CREATE INDEX IF NOT EXISTS idx_usage_events_session
    ON usage_events(session_id);
CREATE INDEX IF NOT EXISTS idx_usage_events_occurred
    ON usage_events(occurred_at);

-- Tool calls table
CREATE TABLE IF NOT EXISTS tool_calls (
    id         INTEGER PRIMARY KEY,
    message_id INTEGER NOT NULL
        REFERENCES messages(id) ON DELETE CASCADE,
    session_id TEXT NOT NULL
        REFERENCES sessions(id) ON DELETE CASCADE,
    tool_name  TEXT NOT NULL,
    category   TEXT NOT NULL,
    tool_use_id TEXT,
    input_json  TEXT,
    skill_name  TEXT,
    result_content_length INTEGER,
    result_content        TEXT,
    subagent_session_id TEXT
);

CREATE INDEX IF NOT EXISTS idx_tool_calls_session
    ON tool_calls(session_id);
CREATE INDEX IF NOT EXISTS idx_tool_calls_session_category
    ON tool_calls(session_id, category);
-- idx_tool_calls_message backs the ON DELETE CASCADE from
-- messages(id). Without it SQLite full-scans tool_calls per
-- deleted message row, which makes ReplaceSessionMessages
-- O(messages * tool_calls) and stalls sync once tool_calls
-- grows large.
CREATE INDEX IF NOT EXISTS idx_tool_calls_message
    ON tool_calls(message_id);
CREATE INDEX IF NOT EXISTS idx_tool_calls_category
    ON tool_calls(category);
CREATE INDEX IF NOT EXISTS idx_tool_calls_skill
    ON tool_calls(skill_name)
    WHERE skill_name IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_tool_calls_subagent
    ON tool_calls(subagent_session_id)
    WHERE subagent_session_id IS NOT NULL;

-- Tool result events table: canonical chronological tool outputs.
CREATE TABLE IF NOT EXISTS tool_result_events (
    id                       INTEGER PRIMARY KEY,
    session_id               TEXT NOT NULL
        REFERENCES sessions(id) ON DELETE CASCADE,
    tool_call_message_ordinal INTEGER NOT NULL,
    call_index               INTEGER NOT NULL DEFAULT 0,
    tool_use_id              TEXT,
    agent_id                 TEXT,
    subagent_session_id      TEXT,
    source                   TEXT NOT NULL,
    status                   TEXT NOT NULL,
    content                  TEXT NOT NULL,
    content_length           INTEGER NOT NULL DEFAULT 0,
    timestamp                TEXT,
    event_index              INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_tool_result_events_session
    ON tool_result_events(session_id);
CREATE INDEX IF NOT EXISTS idx_tool_result_events_call
    ON tool_result_events(
        session_id,
        tool_call_message_ordinal,
        call_index,
        event_index
    );

-- Insights table for AI-generated activity insights
CREATE TABLE IF NOT EXISTS insights (
    id          INTEGER PRIMARY KEY,
    type        TEXT NOT NULL,
    date_from   TEXT NOT NULL,
    date_to     TEXT NOT NULL,
    project     TEXT,
    agent       TEXT NOT NULL,
    model       TEXT,
    prompt      TEXT,
    content     TEXT NOT NULL,
    created_at  TEXT NOT NULL
        DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE INDEX IF NOT EXISTS idx_insights_lookup
    ON insights(type, date_from, project);

CREATE INDEX IF NOT EXISTS idx_insights_created
    ON insights(created_at DESC);

-- Pinned messages table
CREATE TABLE IF NOT EXISTS pinned_messages (
    id          INTEGER PRIMARY KEY,
    session_id  TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    message_id  INTEGER NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    ordinal     INTEGER NOT NULL,
    note        TEXT,
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    UNIQUE(session_id, message_id)
);

CREATE INDEX IF NOT EXISTS idx_pinned_session
    ON pinned_messages(session_id);
-- idx_pinned_message backs the ON DELETE CASCADE from messages(id).
-- The UNIQUE(session_id, message_id) constraint creates an index
-- ordered (session_id, message_id), which the FK lookup on
-- message_id alone cannot use (leftmost-prefix rule).
CREATE INDEX IF NOT EXISTS idx_pinned_message
    ON pinned_messages(message_id);
CREATE INDEX IF NOT EXISTS idx_pinned_created
    ON pinned_messages(created_at DESC);

-- Starred sessions: persists user star/unstar decisions
CREATE TABLE IF NOT EXISTS starred_sessions (
    session_id TEXT PRIMARY KEY REFERENCES sessions(id) ON DELETE CASCADE,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

-- Excluded sessions: tracks session IDs that were permanently
-- deleted by the user so the sync engine does not re-import them.
CREATE TABLE IF NOT EXISTS excluded_sessions (
    id         TEXT PRIMARY KEY,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
-- Skipped files cache: persists skip decisions for files that
-- produced no session (non-interactive, parse errors) so they
-- survive process restarts without re-parsing.
CREATE TABLE IF NOT EXISTS skipped_files (
    file_path  TEXT PRIMARY KEY,
    file_mtime INTEGER NOT NULL
);

-- Remote skip cache: tracks file mtimes per remote host
-- for SSH sync incremental optimization.
CREATE TABLE IF NOT EXISTS remote_skipped_files (
    host       TEXT NOT NULL,
    path       TEXT NOT NULL,
    file_mtime INTEGER NOT NULL,
    PRIMARY KEY (host, path)
);

CREATE TABLE IF NOT EXISTS worktree_project_mappings (
    id          INTEGER PRIMARY KEY,
    machine     TEXT NOT NULL,
    path_prefix TEXT NOT NULL,
    project     TEXT NOT NULL,
    enabled     INTEGER NOT NULL DEFAULT 1,
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    updated_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    UNIQUE(machine, path_prefix)
);

CREATE INDEX IF NOT EXISTS idx_worktree_project_mappings_match
    ON worktree_project_mappings(machine, enabled, path_prefix);

CREATE INDEX IF NOT EXISTS idx_worktree_project_mappings_project
    ON worktree_project_mappings(machine, project);

-- PG sync state: stores watermarks for push sync
CREATE TABLE IF NOT EXISTS pg_sync_state (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

-- Model pricing for cost calculation
CREATE TABLE IF NOT EXISTS model_pricing (
    model_pattern    TEXT PRIMARY KEY,
    input_per_mtok   REAL NOT NULL DEFAULT 0,
    output_per_mtok  REAL NOT NULL DEFAULT 0,
    cache_creation_per_mtok REAL NOT NULL DEFAULT 0,
    cache_read_per_mtok     REAL NOT NULL DEFAULT 0,
    updated_at       TEXT NOT NULL
        DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

-- Git aggregation TTL cache: memoizes `git log --numstat` and
-- `gh pr list` results per (repo, author, window) tuple so
-- repeated `agentsview stats` invocations don't re-shell out.
CREATE TABLE IF NOT EXISTS git_cache (
    cache_key   TEXT PRIMARY KEY,          -- sha256(repo|author|since|until|kind)
    kind        TEXT NOT NULL,             -- 'log' | 'pr'
    payload     TEXT NOT NULL,             -- JSON-encoded result
    computed_at TEXT NOT NULL              -- RFC3339
);

-- Secret findings: persisted detections from internal/secrets.
-- Located by natural coordinates (no row IDs) so findings survive the
-- full-resync orphan copy. Only redacted values are stored.
CREATE TABLE IF NOT EXISTS secret_findings (
    id              INTEGER PRIMARY KEY,
    session_id      TEXT NOT NULL
        REFERENCES sessions(id) ON DELETE CASCADE,
    rule_name       TEXT NOT NULL,
    confidence      TEXT NOT NULL,
    location_kind   TEXT NOT NULL,
    message_ordinal INTEGER NOT NULL,
    call_index      INTEGER,
    event_index     INTEGER,
    match_start     INTEGER NOT NULL,
    match_end       INTEGER NOT NULL,
    match_index     INTEGER NOT NULL,
    redacted_match  TEXT NOT NULL,
    rules_version   TEXT NOT NULL,
    created_at      TEXT NOT NULL
        DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE INDEX IF NOT EXISTS idx_secret_findings_session
    ON secret_findings(session_id);
CREATE INDEX IF NOT EXISTS idx_secret_findings_rule
    ON secret_findings(rule_name);

-- Skills dimension table: slowly-changing reference data synced from
-- the coding-skills catalog (catalog.json + per-skill SKILL.md
-- frontmatter). This is NOT session/message data: it is populated by a
-- dedicated SkillSyncer that runs independently of the session sync
-- engine, so it never touches the sessions/messages/tool_calls fact
-- domain or the stats triggers above. Full-replace semantics on sync.
CREATE TABLE IF NOT EXISTS skills (
    name                 TEXT PRIMARY KEY,
    catalog_path         TEXT NOT NULL DEFAULT '',
    resolved_path        TEXT NOT NULL DEFAULT '',
    domain               TEXT NOT NULL DEFAULT '',
    role                 TEXT NOT NULL DEFAULT '',
    migration_state      TEXT NOT NULL DEFAULT '',
    migration_canonical  TEXT NOT NULL DEFAULT '',
    description          TEXT NOT NULL DEFAULT '',
    frontmatter_name     TEXT NOT NULL DEFAULT '',
    description_tokens   INTEGER NOT NULL DEFAULT 0,
    tokenizer            TEXT NOT NULL DEFAULT '',
    catalog_present      INTEGER NOT NULL DEFAULT 0,
    file_present         INTEGER NOT NULL DEFAULT 0,
    health_error_count   INTEGER NOT NULL DEFAULT 0,
    source_mtime         INTEGER NOT NULL DEFAULT 0,
    prompt               TEXT NOT NULL DEFAULT '',
    prompt_tokens        INTEGER NOT NULL DEFAULT 0,
    synced_at            TEXT NOT NULL
        DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE INDEX IF NOT EXISTS idx_skills_domain ON skills(domain);
CREATE INDEX IF NOT EXISTS idx_skills_role ON skills(role);

-- Skill health findings: one row per detected issue. skill_name is
-- nullable for catalog-level findings (e.g. orphaned directories).
CREATE TABLE IF NOT EXISTS skill_health (
    id          INTEGER PRIMARY KEY,
    skill_name  TEXT,
    check_type  TEXT NOT NULL,
    severity    TEXT NOT NULL DEFAULT 'warn',
    message     TEXT NOT NULL DEFAULT '',
    detail      TEXT NOT NULL DEFAULT '',
    detected_at TEXT NOT NULL
        DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE INDEX IF NOT EXISTS idx_skill_health_skill
    ON skill_health(skill_name);
CREATE INDEX IF NOT EXISTS idx_skill_health_type
    ON skill_health(check_type);

-- Memory dimension table: read-only mirror of the user-memory SSOT
-- (~/.dotfiles/memory/user/*.md). Like skills it is slowly-changing
-- reference data populated by a dedicated MemorySyncer that runs
-- independently of the session sync engine, so it never touches the
-- sessions/messages/tool_calls fact domain or the stats triggers above.
-- Full-replace semantics on sync. rel_path is the file path relative to
-- the memory directory and serves as the stable natural key.
CREATE TABLE IF NOT EXISTS memory (
    rel_path       TEXT PRIMARY KEY,
    source         TEXT NOT NULL DEFAULT 'cross-agent',
    title          TEXT NOT NULL DEFAULT '',
    date           TEXT NOT NULL DEFAULT '',
    problem_type   TEXT NOT NULL DEFAULT '',
    type           TEXT NOT NULL DEFAULT '',
    status         TEXT NOT NULL DEFAULT '',
    origin_session TEXT NOT NULL DEFAULT '',
    origin_project TEXT NOT NULL DEFAULT '',
    body           TEXT NOT NULL DEFAULT '',
    body_tokens    INTEGER NOT NULL DEFAULT 0,
    source_mtime   INTEGER NOT NULL DEFAULT 0,
    synced_at      TEXT NOT NULL
        DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE INDEX IF NOT EXISTS idx_memory_problem_type
    ON memory(problem_type);
CREATE INDEX IF NOT EXISTS idx_memory_type ON memory(type);
CREATE INDEX IF NOT EXISTS idx_memory_status ON memory(status);
-- NOTE: idx_memory_source is intentionally NOT created here. On an
-- existing database the memory table predates the `source` column, and
-- this schema runs (in init) BEFORE migrateColumns adds that column, so
-- indexing memory(source) here fails with "no such column: source" and
-- aborts startup. The index is created in migrateColumns() right after the
-- ALTER TABLE ... ADD COLUMN source migration, which covers both fresh and
-- migrated databases.

-- Vault dimension tables: read-only mirror of dev workflow run records
-- discovered by scanning configured roots for .long-loop/<slug>/
-- directories. Like skills and memory this is slowly-changing reference
-- data populated by a dedicated VaultSyncer that runs independently of the
-- session sync engine, so it never touches the sessions/messages/tool_calls
-- fact domain or the stats triggers above. Full-replace semantics on sync.
--
-- A run is either a dev-long-run (multi-phase, has phases/ + metrics.jsonl)
-- or a dev-complete (single workspace, no phases/metrics). The skill column
-- distinguishes them; a missing skill field defaults to "dev-long-run".
-- Missing optional files (phases, metrics, acceptance, stuck) mark a run
-- incomplete rather than failing it.
CREATE TABLE IF NOT EXISTS vault_run (
    slug           TEXT PRIMARY KEY,
    skill          TEXT NOT NULL DEFAULT '',
    state          TEXT NOT NULL DEFAULT '',
    branch         TEXT NOT NULL DEFAULT '',
    goal           TEXT NOT NULL DEFAULT '',
    repo_root      TEXT NOT NULL DEFAULT '',
    workspace_path TEXT NOT NULL DEFAULT '',
    source_path    TEXT NOT NULL DEFAULT '',
    -- Workspace-level acceptance.json snapshot (nullable: no file = unknown).
    acceptance_ok   INTEGER,
    acceptance_exit INTEGER,
    synced_at      TEXT NOT NULL
        DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

-- One row per phase directory of a dev-long-run. verify_* come from
-- phases/<id>/verify.json, stuck_* from phases/<id>/stuck.json. Nullable
-- integers use -1 sentinels avoided in favor of NOT NULL DEFAULT 0 with a
-- presence flag; here we keep them nullable so "no verify file" is
-- distinguishable from "verify exit 0".
CREATE TABLE IF NOT EXISTS vault_phase (
    id                     INTEGER PRIMARY KEY,
    run_slug               TEXT NOT NULL
        REFERENCES vault_run(slug) ON DELETE CASCADE,
    phase_id               TEXT NOT NULL DEFAULT '',
    verify_ok              INTEGER,
    verify_exit            INTEGER,
    stuck_consecutive_fail INTEGER,
    stuck_fingerprint      TEXT
);

-- One row per metrics.jsonl event record (header rows excluded).
-- dev-complete runs have no metrics so emit no rows here.
CREATE TABLE IF NOT EXISTS vault_metric (
    id          INTEGER PRIMARY KEY,
    run_slug    TEXT NOT NULL
        REFERENCES vault_run(slug) ON DELETE CASCADE,
    ts          TEXT NOT NULL DEFAULT '',
    event       TEXT NOT NULL DEFAULT '',
    phase       TEXT NOT NULL DEFAULT '',
    ok          INTEGER,
    exit        INTEGER,
    fingerprint TEXT
);

CREATE INDEX IF NOT EXISTS idx_vault_run_skill ON vault_run(skill);
CREATE INDEX IF NOT EXISTS idx_vault_phase_run
    ON vault_phase(run_slug);
CREATE INDEX IF NOT EXISTS idx_vault_metric_run
    ON vault_metric(run_slug);
CREATE INDEX IF NOT EXISTS idx_vault_metric_event
    ON vault_metric(event);
