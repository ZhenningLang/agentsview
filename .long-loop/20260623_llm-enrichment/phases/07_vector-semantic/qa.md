# Phase 07 QA — 向量语义检索

## 自动化验证

QA1. 向量编解码与余弦排序正确。

命令：

```bash
go test -tags "fts5,kit_posthog_disabled" ./internal/search ./internal/db -run 'Test.*(Embedding|Semantic|Cosine|Vector)' -count=1
```

结果：pass。

输出摘要：

```text
ok  	go.kenn.io/agentsview/internal/search	0.509s
ok  	go.kenn.io/agentsview/internal/db	0.895s
```

QA2. `SessionEmbeddings` 是 `db.Store` 三后端 parity 能力。

命令：

```bash
go test -tags "fts5,kit_posthog_disabled" ./internal/db ./internal/duckdb ./internal/backendcontract -run 'Test.*(StoreContract|SessionEmbeddings|BackendContract)' -count=1
make test-postgres
```

结果：pass。

输出摘要：

```text
ok  	go.kenn.io/agentsview/internal/db	0.532s
ok  	go.kenn.io/agentsview/internal/duckdb	1.799s
?   	go.kenn.io/agentsview/internal/backendcontract	[no test files]
Container agentsview-postgres-1 Healthy
ok  	go.kenn.io/agentsview/internal/postgres	25.062s
```

验收覆盖：SQLite/DuckDB/PostgreSQL 编译通过并有行为级 oracle；SQLite store contract、DuckDB `TestDuckDBStoreContract/session_embeddings`、PG pgtest `TestStoreSessionEmbeddings` 覆盖只返回非 deleted、有 embedding/dim 的 session，project filter 生效，vector round-trip 与 `llm_embedding_dim` 一致。

QA3. 富化成功时可选写入 embedding，embed 未配置或不支持时文本富化照常成功。

命令：

```bash
go test -tags "fts5,kit_posthog_disabled" ./internal/enrich ./internal/db ./internal/llm -run 'Test.*(Embed|Embedding|Enrich)' -count=1
```

结果：pass。

输出摘要：

```text
ok  	go.kenn.io/agentsview/internal/enrich	0.871s
ok  	go.kenn.io/agentsview/internal/db	1.669s
ok  	go.kenn.io/agentsview/internal/llm	2.045s
```

验收覆盖：`cfg.Embed.Model != ""` 且 mock embed 成功时写入 `llm_embedding` / `llm_embedding_dim`；`ErrEmbeddingsUnsupported` 时 `enrich_status=ok`，文本字段保留成功写入，embedding 不写；provider 返回 NaN/Inf 等不可编码向量时丢弃 embedding，文本富化仍保持 `ok` 且 `llm_embedding_dim=0`。

QA4. semantic search HTTP disabled 主线正确，DeepSeek-only 不外发 embedding 请求。

命令：

```bash
go test -tags "fts5,kit_posthog_disabled" ./internal/server -run 'Test.*(Semantic|Search)' -count=1
```

结果：pass。

输出摘要：

```text
ok  	go.kenn.io/agentsview/internal/server	1.605s
```

验收覆盖：`/api/v1/search/semantic` 在 `llm.enabled=true` 但 `Embed.Model==""` 时返回 `disabled=true`、`count=0`，mock provider 未被调用；`/api/v1/search/semantic/status` 返回 `available=false`；配置独立无 API key embed provider 时 status/search 可用且请求不携带 chat Authorization；remote semantic 请求在 provider 调用前返回 403。

QA5. semantic search enabled 时按 cosine top-K 返回 session 结果。

命令：

```bash
go test -tags "fts5,kit_posthog_disabled" ./internal/server ./internal/search -run 'Test.*Semantic' -count=1
```

结果：pass。

输出摘要：

```text
ok  	go.kenn.io/agentsview/internal/server	0.580s
ok  	go.kenn.io/agentsview/internal/search	0.472s
```

验收覆盖：mock `/embeddings` query vector `[]float32{1,0}`，session vectors 按 cosine 降序返回；`project=proj` 与 `k=1` 生效；返回项 `ordinal=-1`、snippet 为 `Semantic match`。

QA6. 前端语义 mode gating 与搜索 store 行为正确。

命令：

```bash
npm --prefix frontend test -- --run src/lib/api/llm.test.ts src/lib/stores/search.test.ts src/lib/components/command-palette/CommandPalette.test.ts
```

结果：pass。

输出摘要：

```text
Test Files  3 passed (3)
Tests  29 passed (29)
```

验收覆盖：semantic status/helper、search store `keyword|semantic` mode、disabled response 自动隐藏 semantic mode、CommandPalette 仅 `semanticAvailable` 时显示 semantic toggle。

QA7. phase 交付的常规回归验证通过。

命令：

```bash
go fmt ./...
make test
make vet
npm --prefix frontend run check
bash .long-loop/20260623_llm-enrichment/phases/07_vector-semantic/verify.sh
```

结果：pass。

输出摘要：

```text
bash .long-loop/20260623_llm-enrichment/phases/07_vector-semantic/verify.sh
...
ok  	go.kenn.io/agentsview/internal/postgres	25.062s
ok  	go.kenn.io/agentsview/internal/postgres	5.474s
ok  	go.kenn.io/agentsview/internal/web	4.779s
go vet -tags fts5 ./...
svelte-check found 0 errors and 0 warnings
Manual QA reminder: QA8 DeepSeek-only UI hidden and QA9 real embed provider E2E are operator-controlled.
```

补充事实：首次把 `npm --prefix frontend test -- --run` 与 `make test-postgres`、`make test`、`make vet` 并行执行时，前端全量测试出现 10 个失败，集中在既有 `events`/`usage` 超时与 `highlight-fences` 样式断言。随后单独重跑同一前端全量命令通过：

```text
Test Files  76 passed (76)
Tests  1283 passed (1283)
```

## 人工验证

QA8. DeepSeek-only 默认配置下语义入口隐藏。

目的：验证已锁定边界“DeepSeek 无 embeddings，默认禁用；未另配 `[llm.embed]` 不暴露语义入口”。

操作：

1. 使用 `[llm] enabled=true`、DeepSeek `base_url/model/api_key`，但不设置 `[llm.embed].model` 启动本地 web。
2. 打开应用并打开命令面板搜索。
3. 输入长度不少于 3 的关键词。

观察：

1. 搜索控件只显示普通关键词/排序能力，不显示可点击的语义 mode。
2. 后端日志不出现 embedding 请求错误；前端无错误占位。
3. 普通关键词搜索仍能返回结果。

QA9. 另配 embed provider 时语义召回端到端可用。

目的：验证 optional path：用户提供可用 `[llm.embed]` 后，语义搜索能通过 embedding + cosine 返回 session。

操作：

1. 配置一个可用的 `[llm.embed] base_url/api_key/model`。
2. 准备至少两个已富化并写入 embedding 的 session，其中一个与查询语义更接近。
3. 打开命令面板，切到语义 mode，输入语义查询。

观察：

1. 语义 mode 可见且可选择。
2. 返回结果按相似度排序，预期更接近的 session 排在前面。
3. 点击结果进入对应 session；因 `ordinal=-1`，不会跳到错误消息位置。

QA9 不作为默认 run 阻塞项，因为它依赖用户另配真实 embedding provider。
