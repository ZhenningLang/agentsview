# Spec v2: 近似有效输出速度(approx tok/s)指标(Analytics 卡片 + SessionVitals + Speed 趋势页)

> v2 = spec review 后修订版,吸收 B1–B7 全部 blocker 与 major。
> 归档意见见 spec_review.md。

## 0. 指标口径(SSOT,全部实现引用此节)

### 0.1 定义与命名(B1 修订)

**每条合格 assistant 消息的近似速率:**

```
rate_i = output_tokens_i / gen_window_sec_i
gen_window_sec_i = ts_i - ts_prev(i)   (同 session 内按 ordinal 的直接前驱消息,任意 role)
```

**命名统一为 "Output speed (approx.)" / 近似有效输出速度。**
禁止使用 lower-bound / 下界字样:各 parser 的 timestamp 语义不一
(Claude 家族≈完成时刻;OpenCode/Kilo 用 time_created;Piebald 用
created_at;Antigravity 取较早时刻),分母可能偏短或偏长,方向不定。
UI tooltip 固定文案要点:
- 由消息级时间戳差推得,含首 token 等待、排队,部分 agent 含工具时间;
- 适合**同 agent 跨时间**对比好/坏;跨 agent 对比仅供参考;
- 非解码速率。

用户已确认「只要大概对、好坏能指示」,本口径以此为准
(REQUIREMENT.md 的"下界"措辞随本节修订,见 Boundary decisions)。

### 0.2 合格条件

计入 rate 样本需全部满足(作用于**当前**消息,不作用于前驱):

- `role='assistant'` 且 `has_output_tokens=1` 且 `output_tokens >= 32`
  (常量 `minSpeedOutputTokens = 32`)
- 本条与前驱 timestamp 均非空且可解析(RFC3339/RFC3339Nano,容带 offset)
- `0 < gen_window_sec <= 1800`(常量 `maxSpeedWindowSec = 1800`;
  恰好 1800 计入,负值/0/超窗丢弃)
- session 首条消息(无前驱)不计入

### 0.3 SQL 形状(B2 修订,三后端同形)

**两层结构,LAG 必须先于一切过滤:**

```sql
WITH seq AS (
  SELECT m.session_id, m.ordinal, m.role, m.timestamp,
         m.output_tokens, m.has_output_tokens, m.model,
         LAG(m.timestamp) OVER (
           PARTITION BY m.session_id ORDER BY m.ordinal
         ) AS prev_ts
  FROM messages m           -- 全 role、全时间,不加任何 WHERE
)
SELECT ... FROM seq
WHERE role='assistant' AND has_output_tokens=1
  AND output_tokens >= 32
  AND prev_ts IS NOT NULL AND timestamp IS NOT NULL
  AND <window/time-range/cohort 过滤,仅作用于当前行>
```

内层允许按 session 集合裁剪(`WHERE m.session_id IN (...)`,不改变
session 内序列完整性),**不允许**在内层加 role/token/时间过滤。
时间范围过滤作用于**当前消息 timestamp**,半开区间 `[since, until)`;
前驱允许落在 since 之前。

分桶:`bucket_start = floor(epoch(timestamp) / bucketSec) * bucketSec`,
bucketSec ∈ {900, 3600, 86400},UTC epoch;前端按本地时区渲染。
SQLite 用 `strftime('%s', ...)`,PG 用 `extract(epoch from ...)`,
DuckDB 用 `epoch(...)`;parity 测试对桶边界与 1800s 边界给 1s 容差、
rate 给 1e-9 相对容差(minor 修订)。

### 0.4 聚合(B3 修订)

- percentile 一律复用 `percentileFloat`(排序后离散取位,非插值;
  SSOT 写明,QA 覆盖偶数/小样本)。
- **消息级聚合**(velocity 卡片、trend 点):对 rate 样本取 p50/p95 + n。
- **session 级统计量**:`session_rate = Σ output_tokens / Σ gen_window_sec`
  (仅合格消息,ratio-of-sums)。
- **baseline 与 session 比较必须同统计量**:
  baseline = 同 agent 近 30 天(相对**请求时刻**,排除当前 session)
  所有「合格 session」(合格消息数 ≥5)的 `session_rate` 分布的 p50。
  即 session-ratio 对 session-ratio 分布 p50,阈值才有意义。
- 样本门槛:消息级聚合 n<5 → p50/p95 输出 null(JSON 用指针);
  session 合格消息 <5 → speed 对象为 null;baseline 合格 session <10
  → baseline 字段为 null。

### 0.5 好/坏分档(前端常量,minor 修订区间)

`ratio = session_rate / baseline_p50`:
- `ratio >= 0.8` → 正常(默认色)
- `0.5 <= ratio < 0.8` → 偏慢(黄)
- `ratio < 0.5` → 明显慢(红)
- speed 或 baseline 为 null → 灰 "insufficient data"

## 1. 后端设计

### 1.1 共享层(internal/db)

新文件 `speed.go` + `speed_test.go`:

- `SpeedSample{SessionID, Agent, Model, BucketStart int64, Rate float64,
  OutputTokens int}`(B4:带 SessionID,供 cohort join 与 session 聚合)
- `SpeedTrendQuery{Since, Until time.Time; BucketSec int64;
  GroupBy string; Agent string}`
- 共享纯 Go 聚合器(三后端复用,仿 AssembleTiming):
  - `AggregateSpeedTrend(samples, groupBy) []SpeedTrendSeries`
    series 规则(major 修订):按窗口内样本数 n 降序取 top 8,并列按
    key 升序;其余样本合并重算 percentile 为一条 series,标
    `is_other: true`(不用魔法 key,避免与真实名冲突);
    model 为空 → key `"unknown"`;`group_by=model` 时同名 model 跨
    agent 合并(这是 by-model 视图的语义)。
  - `AggregateSpeedStats(samples) SpeedStats{P50, P95 *float64, N int}`

### 1.2 Store 接口(B7 修订:三后端)

- `db.Store` 接口新增
  `GetSpeedTrend(ctx, SpeedTrendQuery) (SpeedTrendResponse, error)`。
- **SQLite、PostgreSQL、DuckDB 三个实现全部提供**,SQL 同形
  (§0.3),scan 进同一 `SpeedSample`,复用共享聚合器。
- `internal/backendcontract` 契约测试新增 speed 合同:同 fixture 在
  三后端断言相同输出(含空数据、null 形状、排序、other 折叠)。

### 1.3 velocity 扩展(B4 修订:cohort 一致)

- `VelocityOverview` / `VelocityBreakdown` 的 overview 结构新增:
  `output_tok_per_sec_p50 *float64`、`output_tok_per_sec_p95 *float64`、
  `speed_n int`(JSON 路径见 §1.6 示例;n<5 → null)。
- **cohort 与现有 velocity 完全一致**:speed 样本查询按
  `AnalyticsFilter` 生成的同一 session 集合(同 buildWhere +
  filteredSessionIDs)做内层 session 裁剪;不单独按消息时间过滤——
  与现有 velocity「session 入选则其全部消息参与」语义对齐。
  complexity 分组复用现有 by_complexity 的 message_count 分桶,
  按 SessionID 归组。

### 1.4 SessionTiming 扩展

`SessionTiming` 新增 `Speed *SessionSpeed`:

```go
type SessionSpeed struct {
    TokPerSec   float64  `json:"tok_per_sec"`
    SampleN     int      `json:"sample_n"`
    BaselineP50 *float64 `json:"baseline_p50"` // null: baseline 不足
    BaselineN   int      `json:"baseline_n"`   // 合格 session 数,恒返回真实值
}
```

- session 合格消息 <5 → `speed: null`(整个对象)。
- baseline 合格 session <10 → `baseline_p50: null`,`baseline_n` 仍给真实值。
- baseline 窗口:请求时刻回溯 30 天,排除当前 session。
- **性能合同(major 修订)**:GetSessionTiming 在 SSE 刷新路径高频调用,
  baseline 结果按 `(backend, agent)` 做进程内 TTL 缓存(5 分钟,容量
  上限 64,server 层实现,三后端共享);单次 timing 请求最多新增
  2 条查询(session speed + baseline miss 时),真实库(~42 万消息)
  baseline 查询目标 <200ms,QA 实测记录。

### 1.5 新 endpoint:GET /api/v1/analytics/speed-trend(B5 修订)

参数契约:

| 参数 | 类型/格式 | 必填 | 默认 | 校验(违反→400) |
|---|---|---|---|---|
| since | RFC3339 | 否 | until − 7d | 可解析 |
| until | RFC3339 | 否 | now (UTC) | 可解析 |
| bucket | `15m\|hour\|day` | 否 | `hour` | 枚举 |
| group_by | `agent\|model` | 否 | `agent` | 枚举 |
| agent | agent type 串 | 否 | 空=全部 | 无(未知值→空结果) |

追加校验:`since >= until` → 400;`until - since > 90d` → 400。
过滤按当前 assistant 消息 timestamp,`[since, until)`。

响应示例:

```json
{
  "bucket_sec": 3600,
  "group_by": "agent",
  "since": "2026-07-08T07:00:00Z",
  "until": "2026-07-15T07:00:00Z",
  "series": [
    {
      "key": "claude",
      "is_other": false,
      "points": [
        {"t": 1752562800, "p50": 42.1, "p95": 96.3, "n": 87},
        {"t": 1752566400, "p50": null, "p95": null, "n": 3}
      ]
    },
    {"key": "(rest)", "is_other": true, "points": []}
  ]
}
```

- 点仅在 n≥1 时输出;n<5 时 p50/p95 为 null(点保留,前端断线并在
  tooltip 显示 n)。空数据 → `series: []`。
- SQLite/PG/DuckDB 三个 serve 路径都接。

### 1.6 velocity / timing 响应 JSON 路径示例(major 修订)

velocity(现有嵌套结构上 additive,非破坏性变更):

```json
{
  "overall": { "...": "...", "output_tok_per_sec_p50": 38.2,
               "output_tok_per_sec_p95": 88.0, "speed_n": 1240 },
  "by_agent": [
    {"label": "claude", "overview": { "...": "...",
      "output_tok_per_sec_p50": 41.0, "output_tok_per_sec_p95": 90.1,
      "speed_n": 800 }}
  ]
}
```

(以现有序列化结构为准:若 breakdown 指标嵌套在 `overview` 字段下,
新字段进同一层;实现时以 `internal/db/analytics.go` 现结构对齐,
QA 按序列化后 JSON 路径断言。)

timing 三态:

```json
{ "speed": {"tok_per_sec": 35.4, "sample_n": 12,
            "baseline_p50": 44.0, "baseline_n": 63} }
{ "speed": {"tok_per_sec": 35.4, "sample_n": 12,
            "baseline_p50": null, "baseline_n": 4} }
{ "speed": null }
```

## 2. 前端

- `VelocityMetrics.svelte`:Overview 加 "Output speed p50 / p95 (approx.)"
  两张卡(null → "insufficient data");By Agent / **By Size** 表均加
  tok/s 列;label tooltip 用 §0.1 固定文案。
- `SessionVitals.svelte`:新增 "Output speed" 行:
  `X tok/s · vs 30d median`,按 §0.5 分档着色;null 分支灰色文案。
- 新页面 `lib/components/speed/SpeedPage.svelte`:
  - router 加 `speed` 路由 + 侧边栏入口(仿 trends/usage 接线;
    支持 URL 直达与 reload 保持参数)
  - 折线图:x=桶,y=p50;n<5 断线;hover tooltip 显示 p50/p95/n;
    group_by agent|model 切换、bucket 15m/hour/day 切换、范围
    24h/7d/30d 切换(默认 7d + hour + agent),每个控件变更必须
    反映到请求参数
  - 图表复用现有 trends/usage 页图表模式,**不新增依赖**
    (package-lock 无新条目,QA 静态检查)
- API client types + speed store(仿 analytics store)。

## 3. 仓库约定

- `requirements/2026-07-15_token-speed-metrics_ACTIVE.md`:指向本 spec,
  goal / locked(口径 §0、三后端 parity、三展示面)/ open(分桶 UI、
  时段热力图 backlog)/ acceptance(qa.md QA1–QA12)。

## 4. 明确不做

- 不改 schema、无迁移、不触发 resync(QA 静态检查守护)。
- 不做输出长度分桶 UI(仅 §0.2 最小过滤;backlog)。
- 不做真 TTFT/解码速率、不做代理拦截。
- 不加前端新依赖。
- 不做 per-agent timestamp 语义修正(v1 接受近似;backlog:为有完成
  时刻证据的 agent 标注更高置信度)。

## 5. Boundary facts / decisions

- Risk types: schema-contract(velocity/timing additive 字段、新
  speed-trend endpoint、Store 接口新方法)、data-source(新指标口径)、
  observability-routing(新页面路由)、shared-path(GetSessionTiming
  高频路径加 baseline,以 TTL 缓存约束)
- Callers: 前端 analytics/sessionTiming/新 speed store;三后端 serve
  路径;backendcontract 测试
- Contract cases: §1.5 参数表;null/insufficient 形状 §1.4/§1.5/§1.6;
  空数据空 series
- Data source: messages(timestamp, output_tokens, has_output_tokens,
  model, role, ordinal, session_id);无新写路径
- Metric route: GET /api/v1/analytics/speed-trend(新)
- **Boundary decision(B1)**: 放弃 REQUIREMENT"下界"措辞,改为
  「近似有效吞吐(approx.)」并覆盖全部 agent——依据用户原话
  「只需要大概对、好坏能指示、不需要精确数据」;严格下界需 per-agent
  timestamp 审计 + allowlist,会砍掉跨 agent 覆盖面,违背用户目标。
- **Boundary decision(B7)**: DuckDB 为生产接线后端
  (cmd/agentsview/duckdb.go、backendcontract),按仓库 Backend Parity
  规则纳入三后端同步实现,不留运行时缺口。

## 6. 风险

- timestamp 语义异质(B1):已通过命名/文案与「同 agent 对时间轴对比
  为主」的定位吸收;跨 agent 绝对值对比明示仅供参考。
- 窗口函数全扫:trend 有 90d 上限 + 时间过滤;velocity 沿用现有
  cohort 全扫先例;baseline 有 TTL 缓存 + 实测目标;慢于目标再谈
  物化(backlog)。
- DuckDB SQL 方言差异:epoch/时间函数不同,由 backendcontract 同
  fixture 断言兜底。

## 7. v3 口径修订(burst 合并 + 并发标注,2026-07-16)

用户确认后追加,消除并行造成的两类干扰:

### 7.1 burst 合并(消除会话内假样本)

30 天真实库探针:窗口 <2s 的样本占 6.5%,平均"速率" 530–1879 tok/s
(最高 57k),系同一 API 响应拆行 / 并行 tool 消息亚秒间隔的测量伪影;
Claude 30 天内 15% 的 assistant 行与前行共享同一 requestId。

**合并规则(`SpeedEventsFromMessages`,三后端 + velocity 共用):**

- 连续 assistant 行,间隔 < 2s(常量 `speedBurstGapSec`)归并为同一
  生成事件;非空 `claude_request_id` 相同的行跨更大间隔也强制归并
  (同一次 API 响应的碎片)。
- 合并事件:token 求和(仅 has_output_tokens 行)、窗口 = 首成员的
  LAG 前驱时间戳 → 末成员时间戳、model 取最后非空、bucket 取末成员。
- 资格阈值(≥32 token、0<window≤1800s)在**合并后**判定,碎片可以
  凑满阈值。
- 无 requestId 的 agent 仅按 2s 间隔归并。

### 7.2 并发标注(归因跨会话并行,不排除)

30 天内 72% 活跃分钟有 ≥2 个 session 同时在写(29% ≥4,峰值 22)。
跨会话并发导致的减速是真实体感,**不排除、只标注**:

- `speed-trend` 响应新增 `concurrency: [{t, sessions}]`,按相同
  bucket 统计全库(**忽略 agent 过滤**,机器级并行负载即混杂因子)
  写入任意消息的 distinct session 数。
- 前端:图底部灰色柱条(仅 sessions≥2 的桶,高度归一化,占绘图区
  ≤18%),tooltip 追加 "N parallel sessions" 行,图例注记
  "parallel sessions (all agents)"。

### 7.3 契约与测试

- `internal/backendcontract` speed 合同新增 burst session fixture
  (2s 内碎片 + requestId 跨 3s 归并 → 单事件)与 concurrency 断言,
  三后端同 fixture 同输出。
- 影响面:trend / velocity 扩展 / SessionVitals(session_rate 与
  baseline 均改用合并事件),同一函数,无口径分叉。
