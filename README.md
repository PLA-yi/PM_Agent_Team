# 🐝 PMHive — AI 产品经理 Agent 集群（v0.1 MVP）

面向 SaaS PM 的多 Agent 集群，**30 秒** 交付以前 2 天的竞品调研工作。

> 前后端分离：Go 1.22+ 后端 + React/Vite 前端。多 Agent 协作通过 SSE 实时流式推送。
> 默认零外部依赖（mock 模式开箱即跑），加上 API key 即切真实 LLM/搜索/抓取。

---

## 30 秒 quickstart

```bash
# 1) 启动后端（默认 :8080，mock 模式：无 API key 也能跑）
cd server && go run ./cmd/server

# 2) 另开终端启动前端（:5173，已配 vite proxy 到 :8080）
cd web && npm install && npm run dev

# 3) 浏览器打开 http://localhost:5173
#    输入"国内 AI 笔记类产品" → 启动 → 看 5 个 Agent 并行 → 拿带引用的报告
```

切真实模式：

```bash
cp .env.example .env       # 填 OPENROUTER_API_KEY、TAVILY_API_KEY
cd server && set -a && source ../.env && set +a && go run ./cmd/server
```

---

## 调研结论与产品定位

**市场定位**：B 端 SaaS · AI 产品经理 Agent 集群 · 中国/出海 SaaS PM 群体
**MVP 三件套**（按 ROI）：
1. **竞品调研报告生成**（v0.1 已实现）
2. 用户访谈记录分析（v0.1 计划中）
3. PRD 自动起草（v0.1 计划中）

**护城河**：项目级长期记忆 + 行业数据管道 + Go 多 Agent 并发 + 工具链集成

**目标定价**：Starter $29 / Pro $79 / Team $59 座/月

完整调研三份原始报告：
- `/Users/yijiawei/.claude/plans/purrfect-conjuring-aho-agent-a84f993e0aae5134d.md` — 市场调研
- `/Users/yijiawei/.claude/plans/purrfect-conjuring-aho-agent-a3e3ed10cb85829d7.md` — 技术选型
- `/Users/yijiawei/.claude/plans/purrfect-conjuring-aho-agent-aafa42dea51f32a34.md` — 产品设计

总方案：`/Users/yijiawei/.claude/plans/purrfect-conjuring-aho.md`

---

## 多 Agent 编排（场景 1：竞品调研）

```
PM 输入: 赛道/产品名
       │
       ▼
[Coordinator]
       │
       ├──▶ [Planner]   生成调研大纲 + 5 个候选竞品
       │
       ├─┬─▶ [Search·Tavily]    并行
       │ └─▶ [Scraper·Jina]     抓官网/定价
       │
       ├──▶ [Extractor]  LLM 抽结构化字段（功能/定价/亮点/差评）
       │
       ├──▶ [Analyzer]   SWOT + 横向对比 + 差异化机会
       │
       └──▶ [Writer]     渲染 Markdown 报告 + 引用 [n]

   每一步都通过 stream.Bus 推 SSE 事件，前端 timeline 实时回放。
```

7 个 Agent，5 个 Stage（其中 researching 阶段并行 2 个）。
单次任务 mock 模式 < 1 秒；接 Claude Sonnet 4.6 + Tavily 实测 2-5 分钟。

---

## 技术栈

| 层 | 选型 | 备注 |
|---|---|---|
| 后端 | Go 1.22+ 标准库 net/http | SSE 走标准库 Flusher，零中间件 |
| LLM | OpenRouter (Claude Sonnet 4.6) | mock fallback，可切 DeepSeek/Qwen |
| 搜索 | Tavily ($3/1K) | mock fixture |
| 抓取 | Jina Reader（免费）/ Firecrawl | mock fixture |
| Store | 内存（v0.1） | schema 已就绪 → 升 Postgres + pgvector + River |
| 前端 | Vite + React 18 + TypeScript | strict 模式，0 warning |
| UI | Tailwind 3 + react-markdown | 暗色主题 |
| 通信 | EventSource (SSE) | 支持 Last-Event-ID 断线重连 |

> v0.1 默认用内存 store + worker pool 实现零依赖体验；
> 生产路径切到 **Postgres + pgvector + River queue**：`docker-compose.yml`、`server/migrations/001_init.sql` 已就绪。

---

## 项目结构

```
.
├── README.md                       # 本文档
├── docker-compose.yml              # postgres + pgvector（生产路径）
├── .env.example                    # 配置模板
├── Makefile
├── server/                         # Go 后端
│   ├── go.mod
│   ├── cmd/server/main.go
│   ├── internal/
│   │   ├── api/                    # HTTP + SSE handler
│   │   ├── agent/                  # 7 个 Agent + Coordinator
│   │   ├── llm/                    # OpenRouter + Mock
│   │   ├── tools/                  # Tavily + Jina + mocks
│   │   ├── store/                  # Memory store (Postgres-ready interface)
│   │   ├── jobs/                   # Worker pool（River-ready）
│   │   └── stream/                 # 任务事件 Bus
│   └── migrations/001_init.sql
└── web/                            # React 前端
    ├── package.json
    ├── vite.config.ts
    ├── tailwind.config.js
    └── src/
        ├── App.tsx                 # 三栏布局
        ├── components/
        │   ├── TaskList.tsx
        │   ├── AgentTimeline.tsx   # Agent 协作流（实时）
        │   └── ReportPreview.tsx   # Markdown 报告 + 引用
        └── lib/api.ts              # SSE / REST 客户端
```

---

## API 速查

| 方法 | 路径 | 说明 |
|---|---|---|
| GET  | `/healthz` | 健康检查 |
| POST | `/api/tasks` | 创建任务（body: `{"input":"...", "scenario":"competitor_research"}`）|
| GET  | `/api/tasks` | 列表 |
| GET  | `/api/tasks/{id}` | 任务状态 |
| GET  | `/api/tasks/{id}/stream` | **SSE** 实时事件流 |
| GET  | `/api/tasks/{id}/report` | 拉报告（含 sources） |
| GET  | `/api/tasks/{id}/traces` | Agent trace 全量回放 |

SSE 事件格式：
```
id: 7
event: stage_start
data: {"task_id":"...","seq":7,"agent":"coordinator","step":"stage_start","payload":{...}}
```

---

## 测试

```bash
cd server && go test ./... -race
```

覆盖：
- `internal/llm` — mock 路由 + factory fallback
- `internal/tools` — mock searcher / scraper
- `internal/stream` — pub/sub + history replay + 并发安全
- `internal/agent` — **整个 7-Agent pipeline 跑通**（mock 模式）
- `internal/api` — **HTTP → worker → SSE → report 端到端 E2E**

---

## v0.1 已实现 vs 待办

✅ **已实现**
- 7 个 Agent + Coordinator 编排（Eino 等价语义，零外部依赖）
- 长任务异步 worker + SSE 流式推送
- 三栏 React UI：任务列表 / Agent timeline / Markdown 报告 + 引用
- Mock 模式开箱即用，0 API key 也能完整 demo

🟡 **下一步**（v0.2）
- 接 Postgres + pgvector + River（schema/migration 已就绪）
- 接入 Eino Graph 替换内置 orchestrator（`internal/agent/coordinator.go` 接口已对齐）
- 用户访谈分析 + PRD 起草两个场景
- 报告 block-based 可编辑（Notion 风格）
- 任务 diff / 增量追问

---

## 风险与已知 trade-off

| 项 | 选择 | 原因 |
|---|---|---|
| 任务存储 | 内存 | v0.1 demo 优先；进程重启丢数据。生产换 PG。 |
| Agent 框架 | 自研轻量 orchestrator | Eino API 不熟、避免赌依赖；接口已对齐方便后续替换。 |
| 队列 | 内存 worker pool | 同上；River schema 已存在，迁移成本低。 |
| 并发 | bridge goroutine + ctx 取消 | 已通过 `-race`，无 stale 状态覆盖问题。 |

---

🐝 _Built with attention by PMHive multi-agent system._
