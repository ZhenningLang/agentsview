# Agentsview 上游收敛与模型演进审计（2026-07-13）

> [推断] 本报告中的建议、优先级和迁移路线，是基于已核实 Git、代码与官方文档证据形成的工程裁决，不是上游承诺。

## 1. 一页结论

这个 fork 需要继续演进。原因不是“模型名字更新了”：模型字段是普通字符串，新模型 ID 通常可以透传。真正需要处理的是 Claude/Codex 会话格式、token 与 pricing、OpenAI generation API，以及上游已经替换的 parser 底座。

不建议直接 merge。fork 独有 124 个提交，上游独有 335 个提交，双方都深改了 parser、sync、schema、server 和 frontend，涉及 1,508 个上游变更文件。

也不建议逐个 cherry-pick 335 个提交。这会把同一套架构迁移拆成大量相互依赖的小补丁，长期维护成本太高。

“智能合并”的单位应该是行为合同和能力模块。先建立真实样本与 parse-diff 验收，再替换底座。

## 2. 模型更新到底影响哪里

模型名字段在 fork 里是普通字符串，不是枚举。新模型 ID，例如 `claude-sonnet-5`、`gpt-5.6-terra`，通常不会因为模型名本身导致 parser 或存储失败。

风险主要在三个地方。

**Content block 解析。** Claude parser 只显式处理 `text`、`thinking`、`tool_use`、`tool_result` 四种 block（`internal/parser/content.go:17-90`）。Codex parser 的顶层记录、response item、content block 也使用固定分支（`internal/parser/codex.go`）。新 block 类型或新 token 字段更容易造成消息消失、统计遗漏或 enrichment 产出为空。

**Pricing fallback。** `internal/pricing/fallback.go:3-133` 没有 `gpt-5.6`、`gpt-5.6-terra`、`gpt-5.6-luna`、`claude-sonnet-5`。每日 usage 对无法定价的模型按零费率累计，但没有充分暴露“成本不完整”；单 session usage 会暴露 unpriced model。两条路径语义不一致。

**LLM enrichment。** 当前实现固定调用 `chat/completions`，请求固定发送 `response_format: json_object` 与 `temperature: 0.2`，响应只读取 `choices[0].message.content`（`internal/llm/client.go:53-86,168-180,204-218`）。HTTP 429 不在 retry 条件内（`internal/llm/client.go:242-247`）。OpenAI 官方文档说明 Chat Completions 仍受支持，但新项目推荐 Responses API。

是否需要原生 Anthropic Messages API 是产品边界，不是 Claude 模型发布自动带来的需求。

## 3. fork 与上游分叉程度

| 维度 | 数据 |
|---|---|
| 共同基线 | `56218f2`（2026-06-12） |
| fork HEAD | `9451552`，独有 124 个提交 |
| 上游 HEAD | `2066ea3`，独有 335 个提交 |
| 上游改动 | 1,508 个文件，约 +407,087 / -33,925 行 |

双方都深改了 parser、sync、schema、server 和 frontend。上游完成了 provider facade 整体迁移、conversation-unit semantic search、read-only MCP server、stable export contract。fork 则增加了 memory、Kilo、Droid、LLM enrichment、skill governance 和 vault 等能力。

上游 `internal/skills` 与 fork `internal/skills` 路径相同，但领域语义不同。这是明确的命名冲突。

上游 recall 表和生命周期不能直接作为第二套 memory 系统引入。可以借鉴 ranking、evidence window、context safety，但数据合同必须继续服从 fork 的 raw-preserving memory 设计。

## 4. 建议引入清单

### 现在做（Slice 0）

| 上游提交或能力 | 理由 | 引入方式 |
|---|---|---|
| `800c19b6` parser output validation | 集中校验 parser 输出，减少静默数据损失 | 手工移植，适配 fork 类型和三后端合同 |
| `ccf2c4da` LiteLLM offline pricing | 替换手写 fallback，缩小新模型定价缺口 | 引入 snapshot 与生成链，保留 fork custom pricing |
| Cost completeness | 让每日 usage 暴露 unpriced model，与单 session 语义对齐 | fork 自行实现 |
| `293342a2`、`1fc423a7`、`9bee8a3c`、`a4584de4`、`87cf12e1` | Codex fork replay、late token、goal context、custom tool 的正确性修复 | 以上游提交为行为来源，在 fork 上手工移植 |
| `18f3aceb` | 支持 `CLAUDE_CONFIG_DIR` | 手工移植 |
| `16707586` | Claude 文件 stat 未变化但内容原地改写时用 hash 检测 | 手工移植 |
| `59a4d055` | Claude companion layout、路径 containment、外置 tool result 数据保真 | 手工移植，不依赖 provider facade |
| DOMPurify `3.4.11` | 跟进上游安全更新 | fork 直接升级依赖并验证前端 |

这些条目不是先把 `upstream/main` 合进来。提交号只用来定位上游行为，实现在 fork 当前代码上重做。

### 底座迁移（Slice 2）

| 上游提交或能力 | 裁决 |
|---|---|
| Provider facade 迁移链：`6c8407ec`、`1039ef66`、`736e782f`、`8a4ae8c5`、`9fd61a07`、`ebee2ccc`、`98ef093c`、`56e3d0f0` | 作为整体架构迁移，不零散 cherry-pick；迁移前必须先有脱敏真实 corpus 和 parse-diff 基线 |

### 协议演进（Slice 3）

| 能力 | 裁决 |
|---|---|
| OpenAI Responses adapter | 可独立于 parser provider facade 实现；给 enrichment 增加显式 Responses adapter，同时保留 Chat Completions |
| Anthropic Messages adapter | 暂不默认引入；只有直连 Anthropic API 成为产品需求时再立项 |

### 底座稳定后做（Slice 4）

| 上游提交或能力 | 裁决 |
|---|---|
| `16e63c74` read-only MCP server | 价值明确，待 SessionService 与权限边界稳定后引入 |
| `725a3d0c`、`94f1488f` stable export/evidence contract | 待 parser、pricing、project identity 和三后端合同稳定后引入 |
| `6c25213e`、`43c1e826` semantic/hybrid search | 用上游 conversation-unit embeddings 与 citations 替换 fork 的 session-level in-memory semantic search；只能保留一个 session vector SSOT |
| daemon、HTTP remote sync、i18n/kit-ui shell | 分别处理后台 worker 生命周期、私密数据排除和 fork 页面重接线后再引入 |

### 明确不做

| 项目 | 理由 |
|---|---|
| 直接 merge `upstream/main` | 变更范围与语义冲突不可控 |
| 第二套 memory store | fork 已有完整 memory 合同，上游 recall 表语义冲突 |
| 照抄上游 data version | fork 有自己的 schema migration 路径 |
| 同时保留两套 session vector 生命周期 | 会出现索引 SSOT、刷新时机和查询结果竞争 |
| 默默改变 orphan subagent 策略 | 现有 fork 测试与上游行为不同，需要单独产品决策 |

## 5. 智能合并方法

合并单位不是 commit，而是行为合同和能力模块。

每个合并单元需要经过四步：

1. 从上游提交中提取目标行为、数据合同和测试场景。
2. 在 fork 分支上手工实现，处理命名冲突、schema 差异和三后端 parity。
3. 用 parse-diff（上游 `4592129b` 及后续）对脱敏真实 session corpus 做重解析比对。
4. 验证 memory、Kilo、Droid、LLM enrichment 等 fork 合同没有退化。

没有真实样本的合并属于盲改，不进入底座迁移。

## 6. 分阶段路线

**Slice 0：基础修补。** 引入 parser validation、offline pricing、cost completeness、Claude/Codex P0 correctness、`CLAUDE_CONFIG_DIR`、Claude companion layout、DOMPurify 更新。这些能力可以逐项手工移植。

**Slice 1：建立验收基础设施。** 建立脱敏真实 session corpus，接入 parse-diff 和 anomaly evidence。这是后续底座替换的质量门。

**Slice 2：底座迁移。** 整体迁移 provider facade，不先合并 `upstream/main`。Kilo 使用上游 OpenCode-family provider，同时保持 `agent = "kilo"`、`kilo:` ID 与 resume command；Droid 适配成新 provider。

**Slice 3：协议演进。** 给 enrichment 增加显式 OpenAI Responses adapter，同时保留 Chat Completions。是否支持 Anthropic Messages API 另做产品决策。

**Slice 4：能力引入。** 底座稳定后引入 MCP、export/evidence contract，再评估 semantic/recall、daemon、remote sync、i18n/UI shell。

## 7. 不可牺牲合同

- **Memory raw-preserving：** assist-mem、CC-native、cross-agent 原始来源不被 canonical 替代；canonical 保留 provenance 与 covered refs。
- **Kilo 身份：** `agent = "kilo"`、稳定 `kilo:` ID 前缀、resume command。
- **Droid 身份：** `agent = "droid"`、稳定 `droid:` ID 前缀、settings token usage、tool metadata、session context events。
- **`llm_title` 唯一性：** `llm_title` 是唯一生成标题字段，不引入竞争字段。
- **安全边界：** LLM key 不明文返回，配置写入保留严格权限，执行端点保持本地可写模式边界。
- **领域隔离：** memory、skill governance、vault 不污染 session/message facts。
- **显式 opt-in：** backup、synthesis、enrichment 等外部调用或写文件行为保持显式 opt-in。

## 8. 验收标准

1. 相关 Go 与前端测试通过；Go 改动额外运行 `go fmt ./...` 和 `go vet ./...`。
2. 涉及 PostgreSQL/Cockroach 查询、schema、pricing、usage、search、export parity 的 Slice 必须运行 `make test-postgres`；其他 Slice 记录不适用依据。
3. parse-diff 对脱敏 corpus 的重解析结果不存在未解释差异；任何行为差异都要有预先批准的 contract case 和证据。
4. 不可牺牲合同以自动化 acceptance test 为主，人工抽样为补充。
5. 前端 session 列表、详情、搜索、analytics 进行 Playwright 或人工可观察验收。
6. Slice 2 额外要求 Kilo 与 Droid 在迁移前后的解析结果字段级一致。
7. SQLite 与 PostgreSQL/Cockroach 在受影响的查询、pricing、usage、search、export 路径保持可观察行为一致。

## 9. 证据与来源

| 编号 | 来源 | 用途 |
|---|---|---|
| E1 | `git merge-base HEAD upstream/main` = `56218f2` | 共同基线 |
| E2 | `git rev-list --left-right --count HEAD...upstream/main` = `124 335` | 双方独有提交数 |
| E3 | `git diff --stat HEAD...upstream/main` | 上游文件变更统计 |
| E4 | `internal/parser/content.go:17-90` | Claude content block 分支 |
| E5 | `internal/parser/codex.go` | Codex 解析分支 |
| E6 | `internal/pricing/fallback.go:3-133` | 定价 fallback 覆盖范围 |
| E7 | `internal/llm/client.go:53-86,168-180,204-218,242-247` | enrichment 实现与 retry 边界 |
| E8 | `https://developers.openai.com/api/docs/models` | OpenAI 当前模型列表 |
| E9 | `https://developers.openai.com/api/docs/guides/migrate-to-responses` | Responses API 官方迁移说明 |
| E10 | `https://platform.claude.com/docs/en/about-claude/models/overview` | Anthropic 当前模型列表与能力说明 |
| E11 | 第 4 节列出的上游提交 | 可移植能力与架构来源 |

## 10. 下一步裁决

[推断] 最稳的下一步不是开始 provider facade 迁移，而是先完成 Slice 0 与 Slice 1。这样可以先拿到数据正确性收益，也能为后续大迁移建立可证伪的反馈环。

等 corpus 与 parse-diff 基线稳定后，再把 provider facade 作为独立大型交付推进。
