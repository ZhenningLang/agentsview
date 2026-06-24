# Phase 06 QA — Frontend

## 自动化验证

QA1. LLM API helpers match the Phase 05 frontend contract.

Command:

```bash
cd frontend && npm run test -- --run src/lib/api/llm.test.ts
```

Result: pass.

Output summary:

```text
Test Files  1 passed (1)
Tests       4 passed (4)
```

QA2. Title selection logic preserves original title semantics and only uses
`llm_title` under the persisted toggle.

Command:

```bash
cd frontend && npm run test -- --run src/lib/utils/session-title.test.ts src/lib/stores/ui.test.ts src/lib/components/sidebar/SessionItem.test.ts
```

Result: pass.

Output summary:

```text
Test Files  3 passed (3)
Tests       60 passed (60)
```

QA3. Balance chip renders only when supported and amount is present.

Command:

```bash
cd frontend && npm run test -- --run src/lib/components/layout/AppHeader.test.ts
```

Result: pass.

Output summary:

```text
Test Files  1 passed (1)
Tests       6 passed (6)
```

QA4. Enrichment settings UI loads status, triggers a run, refreshes counts,
surfaces backend errors, and disables the trigger in read-only mode.

Command:

```bash
cd frontend && npm run test -- --run src/lib/components/settings/LLMEnrichmentSettings.test.ts
```

Result: pass.

Output summary:

```text
Test Files  1 passed (1)
Tests       5 passed (5)
```

QA4b. Sidebar index exposes `llm_title` from local read backends.

Command:

```bash
CGO_ENABLED=1 go test -tags "fts5,kit_posthog_disabled" ./internal/db ./internal/duckdb ./internal/postgres
```

Result: pass.

Output summary:

```text
ok   go.kenn.io/agentsview/internal/db       8.466s
ok   go.kenn.io/agentsview/internal/duckdb   11.796s
ok   go.kenn.io/agentsview/internal/postgres (cached)
```

QA4c. PostgreSQL integration tests cover sidebar index parity when Docker is
available.

Command:

```bash
make test-postgres
```

Result: pass.

Output summary:

```text
ok   go.kenn.io/agentsview/internal/postgres 21.759s
```

QA5. Svelte and TypeScript validation pass for the frontend.

Command:

```bash
cd frontend && npm run check
```

Result: pass.

Output summary:

```text
svelte-check found 0 errors and 0 warnings
```

QA6. Frontend regression tests pass for changed and adjacent behavior.

Command:

```bash
cd frontend && npm run test
```

Result: pass.

Output summary:

```text
Test Files  76 passed (76)
Tests       1278 passed (1278)
```

Final verifier:

```bash
bash .long-loop/20260623_llm-enrichment/phases/06_frontend/verify.sh
```

Result: pass. The script executed QA1-QA6 plus backend QA4b/QA4c in order and
exited 0.

## 人工验证

QA7. 标题开关在列表和详情页按预期切换。

**目的**：验证 R2，用户可以在原标题和 LLM 标题之间切换；未富化 session 回退原标题；不改变 rename/display_name 语义。

**操作**：

1. 启动本地 server 和前端，准备至少两个 session：一个有 `llm_title`，一个 `llm_title` 为空。
2. 打开 Sessions 列表，观察默认标题。
3. 打开 LLM title 开关。
4. 在列表里分别查看有 `llm_title` 和无 `llm_title` 的 session。
5. 打开有 `llm_title` 的 session 详情页，查看 breadcrumb/detail title。
6. 对该 session 触发 rename，观察输入框默认值。
7. 关闭 LLM title 开关，再次观察列表和详情标题。

**观察**：

1. 默认关闭时，列表和详情显示原有 `display_name` / first message fallback。
2. 开启后，有 `llm_title` 的 session 在列表和详情显示 LLM 标题。
3. 开启后，无 `llm_title` 的 session 仍显示原有标题，不显示空白。
4. Rename 输入框仍使用 `display_name` 或原 first-message fallback，不使用 LLM 标题。
5. 关闭后，列表和详情恢复原有标题。

QA8. 余额 chip 只在支持且已配置时显示，失败/不支持时静默隐藏。

**目的**：验证 R4，DeepSeek 余额展示正确；未配置、远程、失败或不支持 provider 不出现错误占位。

**操作**：

1. 本地模式配置 `[llm].enabled=true`、DeepSeek base URL 和 API key，打开应用。
2. 查看 header 搜索区域附近是否出现余额 chip。
3. 改成未配置 API key 或 `[llm].enabled=false` 后重启/刷新。
4. 使用 remote connection 配置访问同一前端，或模拟远程 server URL。
5. 使用一个不支持余额的 provider/base URL，刷新页面。

**观察**：

1. DeepSeek 支持且返回金额时，header 显示紧凑余额 chip，如 `CNY 12.34` 或等价格式。
2. 未配置、disabled、remote、provider 不支持或 provider 请求失败时，不渲染 chip，也不留下空白占位或错误 badge。
3. 页面 console 不包含 API key 或 secret 文本。

QA9. 富化入口能展示状态、触发批次并反馈错误。

**目的**：验证 R3 前端入口，用户能看到富化覆盖状态并手动触发增量富化；禁用或配置错误可见。

**操作**：

1. 在本地模式打开 Settings 中的 LLM enrichment 区域。
2. 观察状态卡片或计数行。
3. 在 LLM 配置有效时点击触发按钮。
4. 等待请求完成并观察结果和状态刷新。
5. 将 LLM disabled 或移除 API key 后刷新，再点击触发按钮或观察禁用态。
6. 在 read-only/remote 模式打开同一区域。

**观察**：

1. 状态区显示 total、enriched、pending、skipped-too-short、no-content、errors。
2. 触发时按钮进入 loading/disabled 状态，避免重复点击。
3. 成功后显示本批次 enriched/skipped/no-content/errors/candidates 和耗时，并刷新状态计数。
4. disabled/missing-key 等后端 rejection 显示明确错误文本。
5. read-only/remote 模式不显示可用的触发按钮，或按钮禁用并解释原因。

QA10. 桌面和移动布局没有新增横向溢出或不可点击控件。

**目的**：验证 Phase 06 的视觉与响应式边界，尤其是 header 新增余额 chip、标题开关和 Settings 富化区。

**操作**：

1. 在桌面宽度打开 Sessions、session detail、Settings。
2. 截图保存桌面状态，包括 header 余额 chip、标题开关和富化区。
3. 将 viewport 调整到移动宽度（约 390px），打开相同页面。
4. 截图保存移动状态。
5. 在移动宽度点击 header 常用按钮、标题开关入口、Settings 触发按钮。

**观察**：

1. 桌面 header 不挤压搜索框到不可识别状态，余额 chip 不遮挡其他按钮。
2. 移动宽度没有横向滚动条。
3. Header 控件仍可点击，触摸目标没有明显重叠。
4. Settings 富化区内容换行正常，计数和按钮不溢出容器。
5. UI 使用现有 theme token，无渐变、glass 或 emoji。
