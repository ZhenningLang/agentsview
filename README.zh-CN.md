# agentsview

[English](README.md) | **简体中文**

浏览、搜索你所有 AI 编程 agent 的会话，并统计 token 花费。单一二进制，无需账号，一切尽在本地。

<p align="center">
  <img src="https://agentsview.io/screenshots/dashboard.png" alt="分析仪表盘" width="720">
</p>

## 为什么用 agentsview

你的 AI 编程会话分散在十几个工具、目录和文件格式里，agentsview 把它们归拢到一处：

- **一个档案库收纳所有 agent** —— 自动发现 30+ 种 agent（Claude Code、Codex、Cursor、Kimi、Copilot
  等）的会话，索引进本地 SQLite 数据库，带 FTS5 全文搜索。
- **成本与 token 统计** —— 跨**所有** agent 的按天、按模型费用统计，缓存感知的计价方式。是 ccusage 类工具的本地化快速替代。
- **本地优先** —— 单二进制、无账号、无云端。服务器只绑定 `127.0.0.1`；除非你显式推送（PostgreSQL、DuckDB 或
  Gist），数据不会离开你的机器。

## 安装

```bash
# macOS / Linux
curl -fsSL https://agentsview.io/install.sh | bash

# Windows
powershell -ExecutionPolicy ByPass -c "irm https://agentsview.io/install.ps1 | iex"
```

也可以安装 **AgentsView 桌面应用**（macOS / Windows）：从
[GitHub Releases](https://github.com/kenn-io/agentsview/releases) 下载，或用
Homebrew：

```bash
brew install --cask agentsview
```

或者直接跑 Docker 镜像 —— 见下文「Docker」一节。

## 快速上手

```bash
agentsview serve           # 同步会话并启动 web UI
agentsview usage daily     # 打印按天费用汇总
```

首次运行时，agentsview 会发现机器上所有受支持 agent 的会话，同步进本地 SQLite 档案库，并在
`http://127.0.0.1:8080` 提供 web UI。之后由文件 watcher 加每 15 分钟一次的周期同步保持档案最新，UI 通过 SSE
实时更新。

完整文档见 **[agentsview.io](https://agentsview.io)**。

## 功能一览

### 浏览和搜索所有会话

| 仪表盘                                                        | 会话查看器                                                              |
| ------------------------------------------------------------- | ----------------------------------------------------------------------- |
| ![Dashboard](https://agentsview.io/screenshots/dashboard.png) | ![Session viewer](https://agentsview.io/screenshots/message-viewer.png) |

| 搜索                                                            | 活动热力图                                                |
| --------------------------------------------------------------- | --------------------------------------------------------- |
| ![Search](https://agentsview.io/screenshots/search-results.png) | ![Heatmap](https://agentsview.io/screenshots/heatmap.png) |

- **全文搜索**：覆盖所有消息内容（SQLite FTS5）。
- **实时更新**：活跃会话收到新消息时通过 SSE 即时刷新。
- **键盘优先**导航 —— `j`/`k`、`[`/`]`、`Cmd+K` 搜索，按 `?` 查看全部快捷键。
- **上下文事件**：持久化的系统消息、resume/中断标记、命令消息、stop-hook 反馈等渲染为紧凑的可展开卡片，与普通
  user/assistant 对话回合区分开。agentsview 只能展示 agent 真正落盘的上下文；从未写入磁盘的运行时 prompt 或 hook
  无法还原。
- **Resume 命令**：UI 为支持的 agent 展示可复制的终端命令。它不会替你执行本地命令或接管终端；resume 行为未知的 agent
  会显示为不支持，而不是编造一条命令。
- **导出**：会话可导出为独立 HTML，或发布到 GitHub Gist。
- Web UI 支持 English 和简体中文。

### 统计 token 用量与费用

`agentsview usage` 跨**所有**编程 agent 统计 token 消耗与算力成本 —— 不只是 Claude
Code。因为会话数据已经索引在 SQLite 里，查询比在每次运行时重新解析原始会话文件的工具快 100 倍以上。

```bash
agentsview usage daily                         # 最近 30 天（默认）
agentsview usage daily --breakdown             # 按模型细分
agentsview usage daily --agent claude --since 2026-04-01
agentsview usage daily --all --json            # 脚本调用
agentsview usage statusline                    # shell 提示符用的一行摘要
```

- 自动按 LiteLLM 费率计价（带离线兜底）
- 感知 prompt 缓存的成本计算（cache 创建 / 读取 token）
- Droid 会话会从可选的 `.settings.json` 的 `tokenUsage` 块摄取 token 总量；无定价的自定义模型仍会报告 token
  数，并列在 `unpriced_models` 下
- 时区感知的日期分桶（`--timezone`），支持日期与 agent 过滤（`--since`、`--until`、`--all`、`--agent`）
- 可独立使用 —— 无需启动服务器

单会话数据：

```bash
agentsview session usage <id>                  # token + 成本估算
agentsview session usage <id> --format json
```

同样的数据也可通过 HTTP 获取：`GET /api/v1/sessions/{id}/usage`。会话存在时即使没有 token 或成本数据也返回
`200`；会话不存在返回 `404`。已废弃的别名 `agentsview token-use <id>` 仍可用，同样会报告成本估算。

### 分析

```bash
agentsview stats                 # 人类可读摘要，最近 28 天
agentsview stats --format json   # 带版本的 v1 schema，供下游工具消费
```

`agentsview stats` 输出窗口范围内的总量、会话原型（automation 与 quick/standard/deep/marathon）、时长
/ 用户消息数 / 峰值上下文 / 每回合工具数的分布，以及缓存经济性、工具 / 模型 / agent 构成和按小时的时间分布。Git
衍生的结果指标是可选的（在大仓库上可能很慢）：`--include-git-outcomes`（commits / 代码行数 / 改动文件数）和
`--include-github-outcomes`（经 `gh` 统计 PR 数，会同时开启 git 指标）。

Web UI 另提供活动热力图、工具用量与速度图表、项目分布、token 生成速度图表和趋势视图。

### 可选的 LLM 功能

LLM 功能默认关闭。在 web UI（Settings）或 `config.toml` 的 `[llm]` 段配置 provider：

- `agentsview enrich` —— 对本地会话做离线 LLM
  富化（`--all`、`--project`、`--force`、`--limit`）。
- **语义搜索** —— 配置 embedding provider 后，UI 中可使用向量检索。

### 密钥泄露扫描

```bash
agentsview secrets scan    # 全规则集扫描并持久化结果
agentsview secrets list    # 列出发现（默认脱敏）
```

扫描结果在 web UI 中同样可见。

### 多机与团队场景

- **SSH 远程同步** —— `agentsview sync --host <机器>` 从远程机器拉取会话文件；配置里的
  `[[remote_hosts]]` 可扇出到多台。
- **PostgreSQL** —— 推送到共享实例做团队仪表盘，见下文「PostgreSQL 同步」。
- **DuckDB** —— 构建可移植的分析镜像文件，或通过 Quack 协议提供远程只读访问，见下文「DuckDB 镜像与 Quack」。

## 支持的 agent

agentsview 会自动发现以下所有 agent 的会话：

| Agent              | 会话目录                                                                      |
| ------------------ | ----------------------------------------------------------------------------- |
| Claude Code        | `~/.claude/projects/`                                                         |
| Codex              | `~/.codex/sessions/`、`~/.codex/archived_sessions/`                           |
| Copilot            | `~/.copilot/`                                                                 |
| Gemini             | `~/.gemini/`                                                                  |
| Droid              | `~/.factory/sessions/`                                                        |
| Kilo               | `~/.local/share/kilo/`                                                        |
| OpenCode           | `~/.local/share/opencode/`                                                    |
| OpenHands CLI      | `~/.openhands/conversations/`                                                 |
| Cursor             | `~/.cursor/projects/`                                                         |
| Amp                | `~/.local/share/amp/threads/`                                                 |
| iFlow              | `~/.iflow/projects/`                                                          |
| Zencoder           | `~/.zencoder/sessions/`                                                       |
| Command Code       | `~/.commandcode/projects/`                                                    |
| OpenClaw           | `~/.openclaw/agents/`                                                         |
| QClaw              | `~/.qclaw/agents/`                                                            |
| Kimi               | `~/.kimi/sessions/`                                                           |
| Kiro               | `~/.kiro/sessions/cli/`、`~/.local/share/kiro-cli/`                           |
| Kiro IDE           | 各 OS 配置根目录下的 `Kiro/User/globalStorage/kiro.kiroagent/`                |
| Cortex Code        | `~/.snowflake/cortex/conversations/`                                          |
| Hermes Agent       | `~/.hermes/sessions/`                                                         |
| WorkBuddy          | `~/.workbuddy/projects/`                                                      |
| Pi                 | `~/.pi/agent/sessions/`                                                       |
| Qwen Code          | `~/.qwen/projects/`                                                           |
| Forge              | `~/.forge/`                                                                   |
| Piebald            | `~/.local/share/piebald/`（Linux；其他 OS 为各自的应用数据目录）              |
| Warp               | 各 OS 的应用数据目录（如 Linux `~/.local/state/warp-terminal/`）              |
| Positron Assistant | `~/Library/Application Support/Positron/User/`（macOS）                       |
| Zed                | `~/Library/Application Support/Zed/`（macOS）、`~/.local/share/zed/`（Linux） |
| VSCode Copilot     | 各 OS 的 VS Code 用户数据目录（Code、Insiders、VSCodium）                     |
| Antigravity        | `~/.gemini/antigravity/`                                                      |
| Antigravity CLI    | `~/.gemini/antigravity-cli/`（见下方说明）                                    |
| Claude.ai          | 非文件型 —— 用 `agentsview import --type claude-ai <path>` 导入               |
| ChatGPT            | 非文件型 —— 用 `agentsview import --type chatgpt <path>` 导入                 |

每个目录都可以用环境变量覆盖 —— 各 agent 的变量名见
[配置文档](https://agentsview.io/configuration/)。支持多目录配置，例如：

```toml
droid_sessions_dirs = ["/factory/sessions/a", "/factory/sessions/b"]
kilo_dirs = ["/Users/me/.local/share/kilo", "/Volumes/work/kilo"]
```

补充说明：

- **Droid** 解析会读取 JSONL 转录文件，以及（存在时）同级的 `<session-id>.settings.json`
  文件获取用量总计（`inputTokens`、`outputTokens`、cache 创建 / 读取 token、thinking token）。归档会话
  ID 带 `droid:` 前缀。源文件是只读输入；agentsview 把解析结果存在自己的档案库里，绝不回写。
- **Kilo** 归档 ID 带 `kilo:` 前缀；resume 命令使用去掉前缀的原始
  ID：`kilo --session <session-id>`。
- **Claude.ai / ChatGPT** 没有落盘的会话文件；用 `agentsview import --type <类型> <path>`
  导入对话导出包。

### Antigravity CLI：高分辨率转录

Antigravity CLI 会话在磁盘上有两种格式。新版本把对话轨迹存为 SQLite `.db` 文件，agentsview 直接索引；旧版本把
assistant 回合和工具调用存为 AES-GCM 加密的 `.pb` 文件，对这些会话 agentsview 回退到**摘要模式** —— 使用
`history.jsonl` 里你的 prompt 加上 `brain/` 下的纯文本产物（plan、walkthrough、checkpoint）。

要为旧的 `.pb` 会话解锁完整转录，可以配合 agentsview 运行
[agy-reader](https://github.com/mjacobs/agy-reader)。它与本地 Antigravity
守护进程通信，解密每段对话，并在加密的 `.pb` 文件旁写一个 `<uuid>.trajectory.json` sidecar。agentsview 的文件
watcher 会自动发现 sidecar 并用它替代摘要模式 —— 无需重启：

```bash
go install github.com/mjacobs/agy-reader@latest
agy-reader --sync     # 为存量会话生成 sidecar
agy-reader --watch    # ……或在工作中持续生成
```

Sidecar 只留在你的机器上。agentsview 不会为生成或读取它们发出任何出站请求，并把 sidecar 视为不可信的结构化输入 —— 信任模型见
[SECURITY.md](SECURITY.md)。

## CLI 速览

| 命令                                          | 作用                                                                                  |
| --------------------------------------------- | ------------------------------------------------------------------------------------- |
| `agentsview serve`                            | 启动服务器与 web UI（默认端口 8080）                                                  |
| `agentsview sync`                             | 只同步会话数据不起服务器（`--host` 走 SSH）                                           |
| `agentsview usage daily` / `usage statusline` | token 费用报告                                                                        |
| `agentsview stats`                            | 窗口范围分析（默认最近 28 天）                                                        |
| `agentsview session ...`                      | `list`、`get`、`messages`、`tool-calls`、`search`、`usage`、`watch`、`export`、`sync` |
| `agentsview import --type <类型> <path>`      | 导入 Claude.ai / ChatGPT 导出包                                                       |
| `agentsview enrich`                           | 离线 LLM 富化（需 `[llm]` 配置）                                                      |
| `agentsview secrets scan` / `secrets list`    | 扫描会话中泄露的密钥                                                                  |
| `agentsview prune`                            | 删除符合过滤条件的会话（先用 `--dry-run` 预览）                                       |
| `agentsview projects` / `health`              | 列出项目；查看会话健康信号                                                            |
| `agentsview pg ...`                           | PostgreSQL：`push`、`status`、`serve`、`service`                                      |
| `agentsview duckdb ...`                       | DuckDB：`push`、`status`、`serve`、`quack serve`                                      |
| `agentsview update` / `version`               | 自更新与版本信息                                                                      |
| `agentsview openapi`                          | 打印 REST API 的 OpenAPI 3.1 schema                                                   |

完整 flag 列表见 `agentsview <命令> --help`。

## 远程 / 端口转发访问

agentsview 绑定 loopback，并校验请求的 `Host` 头以防御 DNS rebinding 攻击。当你通过 SSH
端口转发、反向代理或远程开发环境（exe.dev、Codespaces、Coder、WSL2）访问时，浏览器发出的 `Host` 不在服务器信任列表里，API
请求会被拒绝并返回 `403 Forbidden`（响应体会说明修复方法）。

重启服务器时把 `--public-url` 设为你在浏览器里打开的那个 origin：

```bash
# 浏览器经 `ssh -L 18080:127.0.0.1:8080 host` 打开 http://127.0.0.1:18080
agentsview serve --public-url http://127.0.0.1:18080

# 浏览器打开转发后的域名
agentsview serve --public-url https://your-workspace.exe.dev
```

用 `--public-origin`（可重复或逗号分隔）信任额外的浏览器 origin。如果要把 UI 暴露到 loopback 之外，请同时开启
`--require-auth`。

## Docker

```bash
docker run --rm -p 127.0.0.1:8080:8080 \
  -v agentsview-data:/data \
  -v "$HOME/.claude/projects:/agents/claude:ro" \
  -v "$HOME/.forge:/agents/forge:ro" \
  -e CLAUDE_PROJECTS_DIR=/agents/claude \
  -e FORGE_DIR=/agents/forge \
  ghcr.io/kenn-io/agentsview:latest
```

容器化的 agentsview 只能发现你显式挂载进容器、并用对应环境变量指向的目录里的会话。仓库附带 `docker-compose.prod.yaml`
作为生产示例：

```bash
docker compose -f docker-compose.prod.yaml up -d
```

注意：

- 容器以 root 运行，`/data` 建议用命名卷而不是宿主机 bind mount；若必须 bind mount，先按期望的属主预建目录，避免在你的
  home 目录留下 root 拥有的文件。
- 示例只把 UI 发布到 loopback（`127.0.0.1`）。要暴露到 localhost 之外，请开启 `--require-auth`
  并有意地发布端口。
- 设 `PG_SERVE=1` 可把启动命令切换为 `agentsview pg serve`（配合 `AGENTSVIEW_PG_URL` 指向你的实例）。

## PostgreSQL 同步

把会话数据推送到共享的 PostgreSQL 实例，用于团队仪表盘：

```bash
agentsview pg push       # 推送本地数据到 PG
agentsview pg serve      # 从 PG 提供 web UI（只读）
```

想让共享库持续保持最新而不必手动 `pg push`，可以跑自动推送守护进程。它监视你的会话目录，在新会话写入后很快推送，并以周期兜底：

```bash
agentsview pg push --watch                 # 前台运行，Ctrl-C 停止
agentsview pg push --watch --debounce 1m   # 自定义合并窗口
agentsview pg push --watch --interval 5m   # 自定义兜底周期
```

作为 OS 服务常驻运行（macOS 用 launchd，Linux 用 `systemd --user`）：

```bash
agentsview pg service install     # 生成单元并启用 + 启动
agentsview pg service status      # 查看服务状态（另有 start/stop）
agentsview pg service logs -f     # 跟踪服务日志
agentsview pg service uninstall   # 停止并移除
```

守护进程读取与 `pg push` 相同的 `[pg]` 配置，因此 PostgreSQL DSN
必须写在配置文件里。配置文件含有凭据，请收紧权限：`chmod 600 ~/.agentsview/config.toml`。

**Linux 无头机器：** systemd `--user` 服务在登出后会停止，除非启用
lingering：`loginctl enable-linger "$USER"`。部署细节见
[PostgreSQL 文档](https://agentsview.io/postgresql/)。

## DuckDB 镜像与 Quack

DuckDB 是镜像后端，不替代本地 SQLite 档案库。`agentsview serve` 仍然把数据主摄入
SQLite。需要可移植的分析文件、从镜像只读本地服务、或通过 DuckDB 的 Quack 协议远程只读访问时，才用 DuckDB。

```bash
agentsview duckdb push          # 把 SQLite 镜像进 DuckDB
agentsview duckdb status        # 查看镜像同步状态
agentsview duckdb serve         # 从 DuckDB 提供 web UI（只读）
agentsview duckdb quack serve   # 通过 Quack 暴露本地 DuckDB 文件
```

`duckdb serve` 读取 `[duckdb].path` 或 `AGENTSVIEW_DUCKDB_PATH`。要从远程 Quack 端点服务，改设
`AGENTSVIEW_DUCKDB_URL` 和 `AGENTSVIEW_DUCKDB_TOKEN`。Quack 仍是很新的协议，因此 agentsview
保持保守默认值：本地 Quack 服务只绑 loopback、要求 token，非 loopback 的明文 HTTP 除非显式
`--allow-insecure` 一律拒绝。远程使用请优先选 TLS URL，或把 Quack 放在带认证的隧道 / 代理后面。

三种后端模式一览：

- **SQLite**：主本地档案库 —— 文件同步、FTS5 搜索、可写 UI。
- **PostgreSQL**：可选的团队共享后端 —— 从 SQLite 推送，只读服务。
- **DuckDB**：可选的镜像文件或 Quack 端点 —— 从 SQLite 推送，只读服务。DuckDB 搜索目前走子串 / 正则兜底；SQLite
  FTS5 仍是主服务的索引搜索路径。

## 隐私

agentsview 会在服务器启动时及运行中每 24 小时向 PostHog 发送一个有限的匿名 `daemon_active` 遥测事件，事件的
`DistinctId` 使用一个稳定的随机安装 ID。事件包含 `application=agentsview`、应用版本、commit、OS 和 CPU
架构，并带 `$process_person_profile=false` 与
`$geoip_disable=true`。不包含会话、项目、prompt、文件路径、账号或机器标识。可用
`AGENTSVIEW_TELEMETRY_ENABLED=0` 或 `TELEMETRY_ENABLED=0` 关闭遥测。无论环境变量如何，Go
测试二进制中遥测一律硬性关闭。

所有会话数据都留在你的机器上。服务器默认绑定 `127.0.0.1`。更新检查是可选的，可用 `--no-update-check` 关闭。

## 开发

需要 Go 1.26+（CGO）和 Node.js 22+。

```bash
make dev            # Go 服务器（开发模式）
make frontend-dev   # Vite dev server（与 make dev 并行运行）
make build          # 构建内嵌前端的二进制
make install        # 安装到 ~/.local/bin
```

```bash
make test           # Go 测试（CGO_ENABLED=1 -tags "fts5,kit_posthog_disabled"）
make lint           # golangci-lint + NilAway
make e2e            # Playwright E2E 测试
make bench-backends # 对比 SQLite、DuckDB、PostgreSQL 读取（需要 Docker）
```

pre-commit 钩子使用 [prek](https://github.com/j178/prek)：clone 后运行 `make lint-tools`
和 `make install-hooks`。

### 项目结构

```
cmd/agentsview/     CLI 入口
internal/           Go 包（config、db、parser、server、sync、postgres）
frontend/           Svelte 5 SPA（Vite、TypeScript）
desktop/            Tauri 桌面壳
```

## 文档

完整文档见 **[agentsview.io](https://agentsview.io)**：
[Quick Start](https://agentsview.io/quickstart/) --
[Usage Guide](https://agentsview.io/usage/) --
[CLI Reference](https://agentsview.io/commands/) --
[Configuration](https://agentsview.io/configuration/) --
[Architecture](https://agentsview.io/architecture/)

## 致谢

灵感来自 Andy Fischer 的
[claude-history-tool](https://github.com/andyfischer/ai-coding-tools/tree/main/claude-history-tool)
和 Simon Willison 的
[claude-code-transcripts](https://github.com/simonw/claude-code-transcripts)。

## 许可证

MIT
