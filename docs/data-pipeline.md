# PMHive 数据管道设计（v0.7 行业数据壁垒）

> 状态：设计阶段（v0.7 实现）
> 作者：PMHive
> 上游：v0.5 已交付 Tier1 SocialScraper + Tier2 Web Crawler
> 下游：v0.8 PRD 模板库 + 长期记忆检索

---

## 1. 顶层设计：为什么做数据管道

PMHive 当前 (v0.5) 已经具备：
- ✅ Web 搜索（DDG / Tavily / Jina）
- ✅ 单页抓取（Jina Reader）
- ✅ 简易 BFS 爬虫（`internal/crawler`）
- ✅ Reddit 社区聆听（Atom RSS）

**仍然不具备**：行业级结构化数据库。每次任务都现搜现抓，**报告质量受当下 web 噪音限制**，没有沉淀。

**底层逻辑**：
> 真正的护城河不是"有 agent"，是"有 agent 看不到的数据"。
> 让 LLM 写得比别人好，前提是它"知道得比别人多"。

**目标**：建一套 **结构化、定期刷新、向量化可检索** 的行业数据库，让 PMHive 的所有 agent 在跑任务前先 RAG 召回一次内部知识，**质量天花板被数据厚度撑高**。

---

## 2. 数据源矩阵（按 ROI 排序）

| 优先级 | 数据源 | 内容类型 | 抓取方式 | 法律/技术风险 | 月成本估算 |
|---|---|---|---|---|---|
| **P0** | **Product Hunt** | 新品发布 / 投票 / 评论 | 官方 GraphQL API（free 1000 req/day）| 低 — 有官方 API | $0 |
| **P0** | **Hacker News** | 技术圈讨论 / Show HN | Algolia HN API（公开免费）| 低 | $0 |
| **P0** | **Reddit OAuth** | 互动数据完整版 | OAuth API 60 req/min | 低 | $0 |
| **P0** | **GitHub Trending / Repos** | 开源项目动向 | GitHub REST/GraphQL 5000 req/h（有 token） | 低 | $0 |
| **P1** | **G2 评论** | B2B SaaS 真实用户评价 | 爬虫（无官方 API）| 中 — 灰色，需 robots.txt 谨慎 | $0–$50（代理）|
| **P1** | **Crunchbase** | 公司融资 / 团队 | 官方 API（贵）或爬 | 高 — 反爬强 | $300+/月 |
| **P1** | **SimilarWeb 趋势** | 流量估算 | 官方 API（贵）或浏览器抓 | 中 | $100+/月 |
| **P1** | **小红书 / 知乎** | 中文用户口碑 | 爬虫（cookie + 签名）| 高 — 需账号轮换 | $50–$200（代理）|
| **P2** | **App Store / 应用宝评论** | 移动端评价 | 官方 API（免费）| 低 | $0 |
| **P2** | **微博 / Twitter 公开** | 实时动态 | X API $200+/月 | 低 | $200+ |
| **P2** | **公司官网 / 定价页** | 自家定价 / 功能 | v0.5 crawler 定时跑 | 低 | $0 |

**P0 全部免费、低风险、官方 API 友好** —— 第一阶段先把 P0 打通形成基础数据层。

---

## 3. 架构

```
┌──────────────────────────────────────────────────────────────────┐
│                    Sources (P0 优先实现)                          │
│  Product Hunt · HN Algolia · Reddit OAuth · GitHub · 自家爬虫     │
└────────────────────┬─────────────────────────────────────────────┘
                     │ pull (定时 cron / 事件触发)
                     ▼
┌──────────────────────────────────────────────────────────────────┐
│                    Ingestion Layer (Go)                           │
│  internal/ingest/                                                 │
│   ├── source/         (每个数据源一个 connector)                   │
│   ├── normalizer.go   (统一 schema：Entity{type,name,desc,...})    │
│   ├── deduper.go      (按 entity_id + content_hash 去重)           │
│   └── scheduler.go    (River cron: P0 每 6h, P1 每 24h)            │
└────────────────────┬─────────────────────────────────────────────┘
                     │ insert
                     ▼
┌──────────────────────────────────────────────────────────────────┐
│                    Storage Layer (Postgres)                       │
│  ┌─────────────────────────────────────────────────────────────┐ │
│  │ entities   (产品/公司/人物 — 去重 + 版本化)                   │ │
│  │ content    (评论/帖子/文章 — 关联 entity_id)                  │ │
│  │ embeddings (pgvector(1536) — 关联 content_id)                 │ │
│  │ snapshots  (定时抓的快照，diff 触发警报)                       │ │
│  └─────────────────────────────────────────────────────────────┘ │
└────────────────────┬─────────────────────────────────────────────┘
                     │ query (RAG)
                     ▼
┌──────────────────────────────────────────────────────────────────┐
│                    Retrieval Layer                                │
│  internal/retrieval/                                              │
│   ├── semantic.go     (pgvector cosine + Reranker)                │
│   ├── lexical.go      (Postgres FTS / tsvector)                   │
│   └── hybrid.go       (RRF: semantic + lexical 分数融合)           │
└────────────────────┬─────────────────────────────────────────────┘
                     │ 注入 Agent context
                     ▼
┌──────────────────────────────────────────────────────────────────┐
│                    Existing Agent Pipelines (v0.4)                │
│  Planner / Search / Extractor / Analyzer / Writer                 │
│  跑任务前先 RAG 召回 5-10 条相关 chunk 作为额外上下文                │
└──────────────────────────────────────────────────────────────────┘
```

---

## 4. 数据库 Schema（增量 migration）

```sql
-- 002_data_pipeline.sql

-- 实体（产品 / 公司 / 人物）
CREATE TABLE entities (
    id           BIGSERIAL PRIMARY KEY,
    type         TEXT NOT NULL CHECK (type IN ('product','company','person','category')),
    canonical    TEXT NOT NULL,         -- 规范名（小写 + 连字符）
    name         TEXT NOT NULL,
    aliases      TEXT[],                -- 别名（"思源笔记" / "SiYuan" / "siyuan-note"）
    homepage     TEXT,
    description  TEXT,
    metadata     JSONB,                 -- 行业、定价、tag
    first_seen   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (type, canonical)
);

-- 内容（评论 / 帖子 / 新品发布）
CREATE TABLE content (
    id            BIGSERIAL PRIMARY KEY,
    entity_id     BIGINT REFERENCES entities(id) ON DELETE SET NULL,
    source        TEXT NOT NULL,        -- producthunt / hn / reddit / github / g2 / ...
    source_id     TEXT NOT NULL,        -- 原平台 ID
    url           TEXT NOT NULL,
    author        TEXT,
    title         TEXT,
    body          TEXT NOT NULL,
    body_hash     TEXT NOT NULL,        -- SHA1(body) 去重
    published_at  TIMESTAMPTZ,
    fetched_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    engagement    JSONB,                -- {likes, comments, shares}
    lang          TEXT,
    UNIQUE (source, source_id)
);
CREATE INDEX content_entity_idx ON content (entity_id, published_at DESC);
CREATE INDEX content_fts_idx    ON content USING gin (to_tsvector('simple', title || ' ' || body));

-- 向量化片段（chunk-level embedding）
CREATE TABLE embeddings (
    id          BIGSERIAL PRIMARY KEY,
    content_id  BIGINT NOT NULL REFERENCES content(id) ON DELETE CASCADE,
    chunk_index INT  NOT NULL,
    chunk_text  TEXT NOT NULL,
    embedding   vector(1536) NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (content_id, chunk_index)
);
CREATE INDEX embeddings_hnsw ON embeddings USING hnsw (embedding vector_cosine_ops);

-- 快照（用于 diff 触发警报）
CREATE TABLE snapshots (
    id           BIGSERIAL PRIMARY KEY,
    entity_id    BIGINT NOT NULL REFERENCES entities(id) ON DELETE CASCADE,
    snapshot_kind TEXT NOT NULL,        -- pricing / homepage / changelog / repo_stats
    content_hash TEXT NOT NULL,
    payload      JSONB NOT NULL,
    captured_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX snapshots_entity_kind_idx ON snapshots (entity_id, snapshot_kind, captured_at DESC);
```

---

## 5. Source Connector 接口

每个数据源实现 `Connector`，调度器统一调用：

```go
package ingest

type Connector interface {
    Source() string                         // "producthunt" / "hn" / ...
    Pull(ctx context.Context, since time.Time) ([]RawItem, error)
    NormalizeEntity(item RawItem) (*Entity, error)  // 提取/规范化实体
    NormalizeContent(item RawItem) (*Content, error)
}

type RawItem struct {
    SourceID string
    URL      string
    PulledAt time.Time
    Payload  any  // 平台原始 JSON
}
```

**P0 connector 工作量**（每个 ~150 LOC）：
- `producthunt.go` — GraphQL `posts(first: 50, order: VOTES)`
- `hn_algolia.go` — GET `https://hn.algolia.com/api/v1/search?tags=story`
- `reddit_oauth.go` — OAuth client_credentials → `/r/{sub}/top.json`
- `github_trending.go` — GitHub Search Repos `/search/repositories?q=created:>...&sort=stars`

---

## 6. 调度（River cron）

复用 v0.5 已规划的 River 队列：

```go
// internal/jobs/cron.go
client.Cron(ctx, river.PeriodicJobOpts{
    Name:     "ingest_producthunt",
    Schedule: river.PeriodicInterval(6 * time.Hour),
    Args:     IngestArgs{Source: "producthunt"},
})
client.Cron(ctx, river.PeriodicJobOpts{
    Name:     "ingest_hn",
    Schedule: river.PeriodicInterval(2 * time.Hour),
    Args:     IngestArgs{Source: "hn"},
})
// ...
```

每次 ingest job：
1. 查 `MAX(content.fetched_at) WHERE source=X` 作为增量 watermark
2. 调 connector.Pull(since=watermark)
3. 规范化 + 去重（content.body_hash UNIQUE）
4. enqueue `embed_chunks` job（异步生成 embedding）

---

## 7. Embedding 生成

```go
// internal/jobs/embed_chunks.go
func (w *EmbedWorker) Work(ctx context.Context, job *river.Job[EmbedArgs]) error {
    content := store.GetContent(job.Args.ContentID)
    chunks := chunkText(content.Body, 500, 50) // 500 char + 50 overlap
    for i, chunk := range chunks {
        vec, err := w.LLM.Embed(ctx, chunk)
        if err != nil { return err }
        store.InsertEmbedding(content.ID, i, chunk, vec)
    }
}
```

**Embedding provider**：
- 选 1536 维兼容模型（业界主流尺寸）
- 通过 LLM 抽象层切换 provider，无需改业务代码
- 国内合规可选其他 1536 维国产模型

---

## 8. 检索（hybrid RAG）

```go
// internal/retrieval/hybrid.go
type Hit struct {
    ContentID int64
    Chunk     string
    Score     float64
    Source    string
}

func (h *Hybrid) Search(ctx context.Context, query string, k int) ([]Hit, error) {
    queryVec, _ := h.LLM.Embed(ctx, query)
    semantic, _ := h.SemanticSearch(ctx, queryVec, k*3)  // pgvector cosine
    lexical, _  := h.LexicalSearch(ctx, query, k*3)      // Postgres FTS
    return rrFusion(semantic, lexical, k), nil           // RRF (k=60)
}
```

**Reciprocal Rank Fusion (RRF)** 简单有效，比单点 cosine 提 15-20% recall。

---

## 9. Agent 集成

每个 pipeline 在 Planner 阶段后插入 **RAG 召回 stage**：

```go
// internal/agent/rag.go
type RAGRecaller struct {
    Hybrid *retrieval.Hybrid
    K      int  // 召回数
}

func (r RAGRecaller) Run(ctx context.Context, st *State, d Deps) error {
    hits, err := r.Hybrid.Search(ctx, st.Input, r.K)
    if err != nil { return err }
    // 把召回结果以"内部知识"形式注入 state，让后续 agent 能用
    st.InternalKnowledge = formatHits(hits)
    return nil
}
```

修改 `pipelines.go`：

```go
func DefaultCompetitorResearchPipeline() Coordinator {
    return Coordinator{
        Stages: []Stage{
            {Name: "planning",     Agents: []Agent{Planner{}}},
            {Name: "rag",          Agents: []Agent{RAGRecaller{K: 10}}},  // 新增
            {Name: "researching",  Agents: []Agent{Search{}, Scraper{}}},
            // ...
        },
    }
}
```

Extractor / Writer 的 system prompt 加：

> "## 内部知识库（PMHive 行业数据库）
> 以下是关于本主题的历史数据（来源：PH/HN/Reddit/GitHub），优先采信于现搜现抓：
> {InternalKnowledge}"

---

## 10. 风险与缓解

| 风险 | 缓解 |
|---|---|
| **法律风险**（爬 G2/小红书/Crunchbase） | P0 全部走官方 API；P1 仅在 robots.txt allow 时爬，且 per-domain ≤0.5 QPS；遵循 GDPR 个人数据脱敏 |
| **存储爆炸** | embedding 1536 维 × float32 = 6KB/chunk；P0 全量 1 年估算 ~50 GB，pgvector hnsw 索引 ~30 GB |
| **embedding 成本** | text-embedding-3-small $0.02/1M tokens；P0 月增 ~5M tokens = $0.10/月，可忽略 |
| **OAuth token 失效** | 每个 connector 实现 `Refresh()`，River retry 自动重新拉取 |
| **数据漂移** | 每周 sample 100 条人工 review；建 dashboard 监控 ingest 质量 |
| **冷启动** | 第一周 ingest **全量历史**（PH 历史 100k 产品 + HN 1 年 top stories），形成基础知识 |

---

## 11. 实施 Roadmap

| 阶段 | 内容 | Owner | 工作量 |
|---|---|---|---|
| v0.7-α | Schema migration + ingest 框架 + ProductHunt connector | 后端 | 3 天 |
| v0.7-β | HN + Reddit OAuth + GitHub connector | 后端 | 4 天 |
| v0.7-γ | Embedding 生成 + pgvector hybrid 检索 | 后端 | 3 天 |
| v0.7-δ | RAG 集成进 4 个 pipeline + system prompt 改造 | 后端 | 2 天 |
| v0.7-ε | 冷启动：全量历史回灌 + 数据质量 dashboard | 后端 + PM | 5 天 |
| **小计** | **v0.7 数据壁垒** | | **~17 工作日** |

---

## 12. 成功指标（v0.7 验收）

- [ ] P0 4 个 connector 每天稳定 ingest，错误率 < 1%
- [ ] 数据库累计 ≥ 50,000 content 行 + ≥ 200,000 embedding 行
- [ ] 任意 PM 关键词查询，hybrid retrieval Top-10 召回中**至少 3 条人工评估为相关**
- [ ] 同一份 PRD 调研任务，**接 RAG 后报告引用条数 ≥ 8**（v0.5 当前 4）
- [ ] 任务平均耗时控制在 v0.5 基线 ±15% 内（RAG 不能拖太慢）

---

## 13. 与现有代码的对接锚点

- 复用 `internal/crawler/` 做 P1 阶段的"自家公司官网定时快照"
- 复用 `internal/tools/social/Reddit` 做 OAuth 升级（Atom RSS → 完整 OAuth）
- 复用 `internal/agent/Coordinator` 把 RAG 作为新 Stage 插入既有 pipeline
- River queue 在 v0.5-P0b 引入；本设计假设 River 已就绪

---

> **结论**：数据管道是 PMHive 从"agent demo"晋级到"行业基础设施"的关键一跳。
> 不做 → 永远是通用 LLM 套壳；做了 → 12 个月内的护城河。
