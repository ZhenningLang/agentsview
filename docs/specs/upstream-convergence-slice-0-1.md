# Spec: 上游收敛 Slice 0-1

> 状态：实施中
> 决策来源：`docs/upstream-convergence-audit-2026-07-13.md`

## TL;DR

先移植低耦合的数据正确性与兼容修复，再建立真实会话对拍底座。
本阶段不合并 `upstream/main`，不迁移 provider facade，不复制上游
`dataVersion`。

## 已锁定

1. 上游提交号只用于定位行为和测试场景，代码在当前 fork 上手工实现。
2. `CLAUDE_PROJECTS_DIR` > `claude_project_dirs` >
   `CLAUDE_CONFIG_DIR/projects` > `$HOME/.claude/projects`。
3. Codex custom tool 新数据立即按新行为解析；历史会话的统一重解析延后到
   Slice 0 全部 parser correctness 修复完成后，只提升一次 fork-owned
   `dataVersion`。
4. Claude same-stat rewrite 检测只在 size、mtime、file identity 均未变化时读取
   hash；真实 append 不进入该路径。
5. Kilo、Droid、memory、LLM secret 与显式 side-effect 合同不得退化。

## 边界

### Goals

- G1 支持 `CLAUDE_CONFIG_DIR`，并让本地、SSH、pg service 提示使用同一优先级。
- G2 Codex `custom_tool_call` 与 `custom_tool_call_output` 进入结构化 tool timeline。
- G3 相同 size、mtime、inode/device 的 Claude 原地改写不再保留陈旧消息。
- G4 DOMPurify 更新到上游安全修复版本 `3.4.11`。
- G5 后续集中式 parser validation、pricing、剩余 P0 修复与 parse-diff 有明确验收门。

### Non-goals

- 不迁移 provider facade、daemon、MCP、remote sync 或 UI shell。
- 不在首批修复中提升 `dataVersion`。
- 不引入原生 Anthropic Messages API 或 OpenAI Responses adapter。
- 不修改数据库 schema。

## Phase

### Phase 0A：低耦合兼容修复

- DOMPurify `3.4.11`。
- `CLAUDE_CONFIG_DIR` 本地、SSH、pg service 行为。
- Codex custom tool call/output。
- Claude same-stat rewrite hash tie-breaker。

验收：定向测试、前端 markdown sanitization 测试、`go fmt ./...`、
`go vet ./...`、`make test-short`、前端 check/build。

回滚：本阶段无 schema 与 data version 变化，可以按功能提交逐个 revert。

### Phase 0B：集中式 parser output validation

以 `800c19b6` 为行为来源，在 full、incremental、usage persistence seam 统一清洗
控制字符、非法 role、异常 token/model/timestamp，并输出 anomaly 证据。

验收：SQLite/PostgreSQL/Cockroach 可观察结果 parity；现有合法 fixture 字节级不变；
对抗 fixture 被清洗并计数。

回滚：无 schema 变化时 revert；如最终需要持久 anomaly 字段，另立 migration phase。

### Phase 0C：pricing 与成本完整性

引入上游 LiteLLM offline snapshot 生成与嵌入链，保留 custom pricing 覆盖层。
每日 usage 与单 session usage 都必须暴露 unpriced models 或 complete-cost 状态。

验收：新旧模型、provider prefix、未知模型、custom override、离线启动测试；
涉及 PostgreSQL/Cockroach pricing/usage parity 时运行 `make test-postgres`。

回滚：恢复旧 fallback 与 API shape；不删除已缓存 pricing row。

### Phase 0D：剩余 Claude/Codex P0 correctness

移植 fork replay、late token counts、goal context、Claude companion layout 等修复。
全部行为稳定后只提升一次 fork-owned `dataVersion`，触发一次全量 resync。

验收：持久 SQLite archive 在重扫后保留 session 数据；目标 fixture 获得新行为；
非目标 agents 与 orphan sessions 不丢失。

回滚：代码前滚优先；SQLite 不原地删除或重建。若新 parser 有问题，回退二进制并
保留 archive，修正后重新 resync。

### Phase 1：智能合并验收底座

建立脱敏 Claude、Codex、Kilo、Droid corpus；适配上游 parse-diff，使 full、
incremental 与 stored output 能逐字段对拍，并分类 live-write race 与预批准差异。

验收：corpus 来源和脱敏规则可审计；无未解释差异；Kilo/Droid preserve contracts
进入固定 acceptance suite。

## 场景化推演

| Scenario | 路径 | 暴露问题 | Contract |
|---|---|---|---|
| Claude 自定义配置根 | 只设置 `CLAUDE_CONFIG_DIR`，本地与 SSH 同步 | 默认目录仍指向 `~/.claude` | 三个入口都解析到 `<root>/projects` |
| Codex exec apply_patch | JSONL 含 custom call 与文本 output | timeline 丢失编辑动作 | 生成 Edit tool call 并关联 completed result |
| Claude 原地改写 | 文件内容变、size/mtime/inode 不变 | freshness fast path 错误 skip | stored hash 不同则 full replace |
| 未知新模型 | model ID 不在 pricing catalog | token 有、成本被当成完整零值 | API 暴露 unpriced/不完整状态 |

## Premise Collapse

- If `CLAUDE_CONFIG_DIR` only affects implicit defaults, current precedence can be
  preserved. If this does not hold, explicit config may be silently overridden.
- If stored `file_hash` is refreshed after Claude incremental append, hash tie-breaker
  identifies real rewrites. If this does not hold, unchanged sessions will be
  repeatedly full-parsed.
- If the scrubbed corpus covers current real Claude/Codex/Kilo/Droid shapes,
  parse-diff is a useful migration gate. If this does not hold, provider migration can
  still regress unseen formats.

## 验收与回滚门

- Inner-loop：行为级测试先红后绿；定向 package tests。
- Acceptance：真实或脱敏 fixture 经 parser -> DB -> API 关键路径验证。
- Holdout：至少一份不参与实现设计的 Claude 与 Codex 样本。
- 每个 Phase 独立提交；无对应测试和回滚说明不得进入下一 Phase。

```yaml
# spec-contract
checks:
  - "CLAUDE_CONFIG_DIR precedence is identical across local config, SSH discovery, and pg service warnings"
  - "Codex custom tool calls preserve edit input and link output result events"
  - "Claude same-stat in-place rewrites replace stale stored messages"
  - "Unknown models remain visible and incomplete pricing is explicit"
  - "Kilo, Droid, and raw-preserving memory contracts do not regress"
non_goals:
  - "Direct merge of upstream/main"
  - "Provider facade migration in Slice 0-1"
  - "Native Anthropic Messages support"
validation_commands:
  - "go fmt ./..."
  - "go vet ./..."
  - "make test-short"
  - "npm test --prefix frontend"
  - "npm run check --prefix frontend"
  - "npm run build --prefix frontend"
locked_decisions:
  - "One fork-owned dataVersion bump after all Slice 0 parser correctness changes"
  - "No destructive SQLite migration"
derisk_spikes:
  - type: "数据形态边界"
    question: "Does the scrubbed corpus cover current real Claude/Codex/Kilo/Droid records?"
    method: "Collect and redact real local samples, then compare stored and reparsed outputs"
    status: "spike-before-implement"
```
