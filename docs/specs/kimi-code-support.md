# Kimi Code Agent Support

Related requirement: `requirements/2026-07-17_kimi-code-support_WIP.md`

## TL;DR

Add Kimi Code (`~/.kimi-code/sessions/`, wire protocol 1.4) as a new
agent `kimicode`, following the established add-an-agent changeset
(reference commits `63247f5f`, `9ae1a5f0`). New parser
`internal/parser/kimicode.go` + registry entry + three sync-engine
touchpoints + frontend metadata/resume + CLI help + README + tests.
The legacy `kimi` parser (old kimi-cli protocol) is untouched; the two
coexist.

## 已锁定

- Agent identity: `AgentKimiCode AgentType = "kimicode"`,
  DisplayName `"Kimi Code"`, EnvVar `KIMI_CODE_DIR`, ConfigKey
  `kimi_code_dirs`, DefaultDirs `[".kimi-code/sessions"]`, IDPrefix
  `"kimicode:"`, FileBased, single directory (no WatchSubdirs —
  recursive watch of the sessions root).
- Canonical session ID (one-way-door, see 方案): main session
  `kimicode:session_<uuid>`; subagent session
  `kimicode:session_<uuid>:<agent-id>`. The `session_<uuid>` form is
  what `kimi --session` / `kimi export` accept (verified 2026-07-17
  against `kimi export`: bare uuid rejected, `session_<uuid>` works).
- Subagent sessions are in scope: discovered from
  `agents/<agent-id>/wire.jsonl`, `RelationshipType=RelSubagent`,
  `ParentSessionID` = canonical main-session ID. (User-approved)
- Non-user `context.append_message` origins (`hook_result`,
  `injection`, `skill_activation`, `system_trigger`,
  `background_task`) render as context cards (`IsSystem: true`,
  `SourceType: "system"`, `SourceSubtype: <origin.kind>`).
  (User-approved)
- Resume command in scope: `kimi --session session_<uuid>` (Kilo
  precedent; frontend `RESUME_AGENTS` entry strips prefix and any
  subagent suffix). (User-approved)
- Token accounting consumes `usage.record` events only (verified 1:1
  with `step.end` on all 21 real wire files: 169 = 169), mapped to
  `ParsedUsageEvent`; session aggregates via
  `applyUsageEventTokenTotals` (`TokenAggregateUsageEvents`).
  Defensive fallback: if a file has zero `usage.record` but
  `step.end.usage` exists, consume `step.end` usage instead.
- Session enrichment from sibling `state.json`: `title` →
  `SessionName`, `workDir` → `Cwd` + `Project` (base name),
  `forkedFrom` → `ParentSessionID` + `RelFork` (main wire only).
  Missing/corrupt `state.json` degrades gracefully (Project falls back
  to the `wd_<name>_<hash>` directory name, stripped).

## 可自由裁量

- Frontend accent color for `kimicode` (pick an unused
  `var(--accent-*)`; `accent-pink` is taken by legacy `kimi`).
- Exact fixture file layout inside tests.

## 待决策

无。

## 边界

### Goals

- Auto-discover all Kimi Code sessions (main + subagent) under the
  default or configured sessions dir; parse into sessions, messages,
  tool calls/results, thinking, context cards, and per-turn usage
  events.
- Full-text search, session detail, usage/cost CLI + API work for
  `kimicode` like any other agent.
- Copyable resume command `kimi --session session_<uuid>`.
- Unit tests for parser, registry, sync classification, frontend
  utils; requirements entry per repo convention.

### Non-goals

- No changes to the legacy `kimi` parser behavior.
- No `kimi migrate` (legacy kimi-cli → kimi-code) interop.
- No parsing of `llm.request`, `llm.tools_snapshot`, `permission.*`,
  `plan_mode.*`, `tools.*`, `config.update`, `forked` top-level events
  into messages (skipped).
- No use of `~/.kimi-code/server/`, `user-history/`, `logs/`,
  `session_index.jsonl`, or `workspaces.json` as data sources
  (`state.json` + wire files are sufficient).
- Cost display depends on LiteLLM pricing for the session's model
  (e.g. `kimi-code/kimi-for-coding`); if unpriced, tokens show and
  cost stays unset (Droid precedent).

### Constraints

- Repo rules: testify for new tests, `t.TempDir()`, table-driven where
  sensible, `go fmt`/`go vet` clean, `make test` green, commit every
  turn with conventional messages.
- Backend parity: parser feeds the shared `db.Store`; no
  backend-specific work.
- The SQLite archive is append/migrate-only; a new agent triggers the
  normal full-resync path, no manual migration.

## 场景化推演

| Scenario | Actor / Context | Step-by-step path | System touchpoints | Exposed issue | Requirement / Contract |
|---|---|---|---|---|---|
| 浏览已有会话 | 用户装了 Kimi Code 并跑过若干会话 | `agentsview serve` → 首次同步发现 `~/.kimi-code/sessions/wd_*/session_*/agents/*/wire.jsonl` → 列表出现 "Kimi Code" 会话 → 打开详情 | DiscoverFunc → processKimiCode → db.Store → REST → SPA | 项目名若取 `wd_<name>_<hash>` 目录名将带 hash 噪音 | Project 优先取 `state.json` 的 `workDir` base name；缺失时回退目录名并剥离 `wd_` 前缀与 `_<hash>` 后缀 |
| 活跃会话增量同步 | 用户一边跑 Kimi Code 一边看 UI | watcher 捕获 wire.jsonl 写入 → 路径分类命中 kimi-code 规则 → 重解析 → SSE 推送 | engine.fileToDiscovered (kimi block at `internal/sync/engine.go:705` 旁) → processKimiCode → SSE | wire 路径深度（5 段）与旧 kimi（3 段）不同，分类规则必须独立 | 分类契约：rel parts == 5 且 `parts[2]=="agents"` 且 `parts[4]=="wire.jsonl"` |
| 损坏/边界数据 | 会话目录残缺（无 state.json、空 wire、畸形行、未知事件） | 同步到残缺目录 | parser | 解析崩溃或产生空会话会污染列表 | Contract cases：无 state.json 仍解析 wire；空消息 → 返回 nil 跳过；非 JSON 行计入 MalformedLines 并跳过；未知事件类型跳过不炸 |

## 方案

单一推荐方案，无竞争性候选（改动清单由仓库既有惯例唯一确定）。
唯一真正的单向门是 canonical session ID 形状：

- **A（选择）**: `kimicode:session_<uuid>`，subagent 追加 `:<agent-id>`。
  resume 命令零加工（`stripIdPrefix` 后即为 `kimi --session` 接受的
  ID）；FindSource 扫描一层 `wd_*` 目录定位，O(workspaces) 可接受。
- **B**: 旧 kimi 风格 `kimicode:<ws-hash>:<session-uuid>`。反超条件：
  若 session uuid 仅 workspace 内唯一则需此方案——但 uuid v4 全局
  唯一，且 `kimi --session` 只认 `session_<uuid>`，B 会让 resume
  与 `session usage` 的 ID 提取多一层特例。不选。
- **C**: `kimicode:<ws-dir>:<session-uuid>` 保留可读名。反超条件：
  人肉调试时更容易认 workspace；但 ws 目录名带 hash 后缀一样不可
  读，且 ID 更长、冒号段数更多。不选。

不可逆性评估：改 ID 方案后既有 kimi-code 会话会以新 ID 重新入库
（full resync 重建），数据不丢，但旧 ID 的书签/链接失效。A 与
Kilo 先例（raw ID 无目录前缀、resume 去前缀）同构，锁定 A。

### 事件消费映射（parser 核心契约）

| wire event | 处理 |
|---|---|
| `metadata` | 读 `protocol_version`/`created_at`；startTime 候选 |
| `config.update`, `tools.*`, `permission.*`, `plan_mode.*`, `llm.request`, `llm.tools_snapshot`, `forked` | 跳过 |
| `turn.prompt` | flush 当前 assistant 段 → RoleUser 消息（`input[]` 中 `type=="text"` 拼接）；首个非空为 `FirstMessage` |
| `turn.steer` | 同 `turn.prompt`（字段形状以实现时真实 fixture 为准） |
| `turn.cancel` | context 卡片（`SourceSubtype: "turn_cancel"`） |
| `context.append_message` | `origin.kind=="user"` 跳过（与 `turn.prompt` 去重）；其余 origin → context 卡片 |
| `step.begin` | 跟踪当前 `turnId`/`step` |
| `content.part` (`text`/`think`) | 累积进当前 assistant 段（think → `ThinkingText` + `HasThinking`） |
| `tool.call` | 累积 `ParsedToolCall`（`toolCallId`/`name`/`args` → `InputJSON`，`Category=NormalizeToolCategory`）+ 展示文本（沿用 `formatKimiToolUse` 风格） |
| `tool.result` | flush assistant 段 → RoleUser + `ToolResults` 消息（output 为字符串直取，否则取 raw JSON；防御性处理错误形态） |
| `step.end` | `finishReason` → 最近 assistant 消息 `StopReason`；`usage` 仅在无 `usage.record` 回退路径中消费 |
| `usage.record` | → `ParsedUsageEvent{Model, InputTokens: inputOther, OutputTokens: output, CacheCreationInputTokens: inputCacheCreation, CacheReadInputTokens: inputCacheRead, OccurredAt: time, DedupKey: 位置序号}` |

Assistant 段 flush 触发：`turnId` 变化、`tool.result`、EOF。时间戳
取各事件 `time`（epoch 毫秒）；`StartedAt`/`EndedAt` 为全文件
min/max。

### Premise collapse

1. `If usage.record 与 step.end 在 protocol 1.4 全量 1:1 配对（本机
   169:169 已验证），只消费 usage.record 不丢 token。If does not
   hold, 部分 turn 的 token 丢失，usage 统计偏低。` → 防御回退：
   文件级 0 条 usage.record 且 step.end 带 usage 时改消费
   step.end.usage（contract case 进测试）。
2. `If wire 顶层/loop 事件字段名在 protocol_version 1.x 内稳定，
   parser 输出完整。If does not hold, 新版 Kimi Code 会话解析缺
   消息。` → parser 对未知事件一律跳过、不校验版本号 fail-fast；
   本机 21 个真实 wire 文件已枚举全部事件类型（spike: verified）。
3. 数据形态边界（空会话/无 state.json/无 usage/畸形行/单消息会
   话）→ 全部转为 fixture contract cases，随实现验证
   （spike-before-implement，以测试落地）。

## 风险与验证

最大风险是事件契约理解偏差（真实会话里消息序/重复/缺失）。缓解：
fixture 直接镜像本机真实 wire 结构（smoke 会话 19 行为最小样本），
acceptance 阶段用真实 `~/.kimi-code` 全量数据跑 serve 核对。

- Inner-loop verifier: `go test ./internal/parser/ -run KimiCode`、
  `go test ./internal/sync/ -run KimiCode`、`go test ./internal/parser/ -run TestAgentByPrefix`、
  `cd frontend && npx vitest run src/lib/utils/agents.test.ts src/lib/utils/resume.test.ts`、
  `go fmt ./... && go vet ./...`、`make test`。
- Acceptance verifier: `make build` 后以
  `AGENTSVIEW_DATA_DIR=$(mktemp -d)` 启动 `./agentsview serve`
  （默认发现真实 `~/.kimi-code`），验证：
  `GET /api/v1/sessions?agent=kimicode` 返回 ≥20 条；
  `agentsview usage daily --agent kimicode` 有输出；
  `agentsview session usage kimicode:session_10077c19-bdf3-414e-8967-699a42301c9b`
  报告 output=80、peak context=16098（smoke 会话 47+33 /
  max(1436+14592, 226+15872)）；详情页含 context 卡片与 resume 命令。
  剩余风险：跨机器/跨版本 wire 差异无法本机证伪，靠宽松解析兜底。

## 实施步骤

1. `internal/parser/types.go` + `types_test.go` — 加 `AgentKimiCode`
   常量与 Registry 条目（DiscoverFunc/FindSourceFunc 指向新函数）—
   验证：`go test ./internal/parser/ -run TestAgentByPrefix`。
2. `internal/parser/kimicode.go` — `DiscoverKimiCodeSessions`、
   `FindKimiCodeSourceFile`、`ParseKimiCodeSession`（返回
   `*ParseResult`，Droid 风格签名）— 验证：
   `go test ./internal/parser/ -run KimiCode`。
3. `internal/parser/kimicode_test.go` — 上文 contract cases
   fixtures（t.TempDir 构造目录树）— 验证：同上。
4. `internal/sync/engine.go` — 三处接线：fileToDiscovered 分类块
   （~line 705 旧 kimi 块旁）、process 分发 `case parser.AgentKimiCode`
   （~line 3446）、`processKimiCode`（镜像 processDroid，计算文件
   hash）、project 推导 case（~line 6652，四级父目录取 wd 名）—
   验证：`go test ./internal/sync/ -run KimiCode`。
5. `internal/sync/kimicode_test.go` — 分类/process/project 推导 —
   验证：同上。
6. `frontend/src/lib/utils/agents.ts` + `agents.test.ts` —
   KNOWN_AGENTS 加 kimicode — 验证：vitest。
7. `frontend/src/lib/utils/resume.ts` + `resume.test.ts` —
   `RESUME_AGENTS["kimicode"]`（raw ID 截掉 `:<agent-id>` 后缀）—
   验证：vitest。
8. `frontend/src/lib/components/settings/AgentDirSettings.svelte` —
   label 表加 `kimicode: "Kimi Code"` — 验证：`make frontend` 构建。
9. `cmd/agentsview/cli.go` — env help 加 `KIMI_CODE_DIR` 行；
   `scripts/e2e-server.sh` — 加 `KIMI_CODE_DIR=$EMPTY_DIR`；
   `README.md` — Supported Agents 表加 `| Kimi Code |
   ~/.kimi-code/sessions/ |`（紧随 Kimi 行）— 验证：`make build`。
10. 全量验证 + acceptance（上方验证节）+ requirements 条目
    `git mv` 到 `_DONE` — 验证：`make test && go vet ./...`。

步骤 2-3 与 4-5 按 `/dev-tdd` 节奏（先 fixture 测试后实现）；6-9 为
纯接线，可与 4-5 同批。全部完成后按仓库规则逐 turn commit。

```yaml
# spec-contract
checks:
  - "agentsview serve 首次同步后 GET /api/v1/sessions?agent=kimicode 返回本机全部 Kimi Code 会话（≥20 条，含 subagent）"
  - "agentsview usage daily --agent kimicode 输出 token 统计行"
  - "agentsview session usage kimicode:session_10077c19-bdf3-414e-8967-699a42301c9b 报告 output=80、peak context=16098"
  - "会话详情 API 返回 user/assistant/tool/context 卡片消息与 usage 数据"
  - "前端 resume 命令为 kimi --session session_<uuid>（main 与 subagent 会话均如此）"
  - "parser 单测覆盖：基本会话、subagent、fork、context 卡片去重、无 state.json、畸形行、空会话、turn.steer、usage 回退路径"
non_goals:
  - "不改动 legacy kimi parser 行为"
  - "不做 kimi migrate 互操作"
  - "不解析 llm.request/permission.*/plan_mode.*/tools.*/config.update/forked 为消息"
  - "不消费 server/、user-history/、logs/、session_index.jsonl、workspaces.json"
validation_commands:
  - "go test ./internal/parser/ -run KimiCode -v"
  - "go test ./internal/sync/ -run KimiCode -v"
  - "make test"
  - "go fmt ./... && go vet ./..."
  - "cd frontend && npx vitest run src/lib/utils/agents.test.ts src/lib/utils/resume.test.ts"
  - "make build && AGENTSVIEW_DATA_DIR=$(mktemp -d) ./agentsview serve"
locked_decisions:
  - "AgentType=kimicode, KIMI_CODE_DIR, kimi_code_dirs, .kimi-code/sessions, prefix kimicode:"
  - "canonical ID: kimicode:session_<uuid>[ :<agent-id>]（方案 A）"
  - "subagent 会话纳入本次切片"
  - "非 user origin 的 context.append_message 渲染为 context 卡片"
  - "token 只消费 usage.record，step.end.usage 仅作零 usage.record 时的回退"
  - "resume 命令 kimi --session session_<uuid>"
derisk_spikes:
  - type: "2 第三方契约"
    question: "wire 协议 1.4 的事件类型与字段全集是什么"
    method: "枚举本机 21 个真实 wire.jsonl 的全部顶层/loop 事件类型与 origin 分布"
    status: "verified"
  - type: "4 数据形态边界"
    question: "空/残缺/畸形会话目录是否会导致解析崩溃或脏数据"
    method: "fixture contract cases（无 state.json、空 wire、畸形行、未知事件、无 usage.record）"
    status: "spike-before-implement"
```
