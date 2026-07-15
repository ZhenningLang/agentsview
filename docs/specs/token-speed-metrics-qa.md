# QA v2: 近似有效输出速度(approx tok/s)指标

## 自动化验证项(进 verify.sh)

- **QA1** `internal/db` 速率计算单测(spec §0.2/§0.3):固定 fixture
  断言 per-message rate、p50/p95(含偶数样本、n=1..4 小样本 p95,
  验证 percentileFloat 离散取位语义)、样本数。丢弃边界:负 delta、
  delta=0、delta>1800(恰 1800 计入)、output_tokens<32(恰 32 计入)、
  has_output_tokens=0、timestamp 空、session 首条。
  **LAG 层级用例(B2)**:前驱为 user、前驱为不合格 assistant(短输出/
  无 token)时窗口仍取直接前驱;当前消息在 since 内但前驱在 since 外
  时样本保留;RFC3339Nano 与带 offset 时间戳可解析。
- **QA2** 分桶与归组单测:15m/hour/day 桶边界(UTC epoch 对齐,跨桶
  归属);group_by=agent/model 归组;**top-8 规则**:n 降序、并列 key
  升序、其余合并重算 percentile 且 `is_other=true`、空 model→"unknown"、
  group_by=model 跨 agent 合并。
- **QA3** velocity 扩展单测:overall/by_agent/by_complexity 序列化后
  JSON 路径含 `output_tok_per_sec_p50/p95`(指针,n<5→null)与
  `speed_n`;**cohort 一致性(B4)**:构造 AnalyticsFilter(时间/agent
  过滤)断言 speed 样本与既有 velocity 使用同一 session 集合;既有
  字段值回归不变;complexity 分组与 by_complexity 同分桶。
- **QA4** speed-trend handler 测试(B5):默认值(hour/agent/7d)、
  非法 bucket/group_by/时间格式→400、since>=until→400、范围>90d→400、
  [since,until) 半开边界、未知 agent→空 series、空数据→`series:[]`、
  n<5 点 p50/p95 为 null 且点保留、响应 JSON 形状与 spec §1.5 示例一致。
- **QA5** SessionTiming.speed 单测(B3):session ratio-of-sums 正确;
  三态 JSON(spec §1.6):合格消息<5→`speed:null`、baseline 合格
  session<10→`baseline_p50:null` 且 `baseline_n` 为真实计数、充足态
  数值正确;baseline 为同 agent 请求时刻回溯 30 天、排除当前 session、
  以「合格 session 的 session_rate 分布 p50」计算(同统计量断言);
  baseline TTL 缓存命中时不重查(可用查询计数桩断言)。
- **QA6** 三后端 parity(B6/B7):`internal/backendcontract` 新增 speed
  合同,同 fixture 在 SQLite/PostgreSQL/DuckDB 断言相同输出(rate 相对
  容差 1e-9、桶边界 1s 容差),覆盖空数据、null 形状、排序、other 折叠。
  **PG 侧必须实跑**:verify.sh full tier 执行 `make test-postgres`
  (docker compose 起容器),不可用则 verify 直接 fail,不允许
  「编译通过即算过」。DuckDB 合同随常规 `make test` 跑。
- **QA7** 前端单测:分档边界(ratio=0.5 前后、0.79/0.8、insufficient
  分支)、tok/s 格式化;speed store fetch 成功/失败;**每个展示面的
  null/insufficient 渲染各一例**(velocity 卡、By Agent 列、By Size 列、
  SessionVitals 行、trend 断线);控件变更(group_by/bucket/range)
  产生正确请求参数(mock service 断言)。
- **QA8** 全量门禁:`make test` + `make vet` + `make lint` +
  `cd frontend && npm test` 全绿;`go fmt ./...` 无 diff。
- **QA9** 静态守护(non-goal 保险):schema.sql 与迁移路径无 diff、
  parser/resync 路径无 diff、`frontend/package-lock.json` 无新增依赖
  条目(verify.sh 里 git diff 路径检查)。

## 人工验证项(complete 前贴给用户)

- **QA10** 真实库(~/.agentsview/sessions.db)起服务,**含对账**:
  a) Analytics 页 Velocity 区显示 tok/s p50/p95 卡片,By Agent /
     By Size 表有 tok/s 列;
  b) 打开一个近期 session,SessionVitals 显示 Output speed 行与
     vs 30d median 分档色;
  c) Speed 页:agent/model、bucket 15m/hour/day、范围 24h/7d/30d
     全部可切且反映到请求;URL 直达/reload 保持参数;hover 有
     p50/p95/n;n<5 断线;
  d) 三处口径文案均为 approx 措辞,无 lower-bound 字样;
  e) **对账(major 修订)**:抽 1 个固定 session,列出其合格消息的
     output_tokens、前后 timestamp、被过滤消息及原因,手算
     session_rate 与 API 返回一致;并检查 p50<=p95、无 NaN/Inf、
     Analytics/Speed 页/SessionVitals 三处同 cohort 数值互洽;
     量级(个位~两位数 tok/s)仅作观察项记录。
  f) baseline 查询实测耗时记录(<200ms 目标)。
- **QA11** requirements/2026-07-15_token-speed-metrics_ACTIVE.md 存在,
  含 goal / locked / open / acceptance,指向 spec。
- **QA12** Boundary decisions 在交付 summary 中列出(B1 措辞变更、
  B7 DuckDB 纳入)。
