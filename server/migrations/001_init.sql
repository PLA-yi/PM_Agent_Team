-- PMHive schema v0.1
-- 业务库 + 向量库（pgvector），River 队列由 river migrate 单独管

CREATE EXTENSION IF NOT EXISTS vector;
CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- 任务：一次调研/PRD/访谈分析就是一个 task
CREATE TABLE IF NOT EXISTS tasks (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    scenario     TEXT NOT NULL CHECK (scenario IN ('competitor_research','interview_analysis','prd_drafting')),
    input        TEXT NOT NULL,
    status       TEXT NOT NULL DEFAULT 'queued' CHECK (status IN ('queued','running','succeeded','failed','cancelled')),
    stage        TEXT,                                -- planning / searching / extracting / analyzing / writing / done
    progress     INT  NOT NULL DEFAULT 0,             -- 0-100
    error        TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at  TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS tasks_created_idx ON tasks (created_at DESC);

-- Agent 协作 trace：每次 Agent 步骤记一行，前端 timeline 流式渲染
CREATE TABLE IF NOT EXISTS agent_traces (
    id          BIGSERIAL PRIMARY KEY,
    task_id     UUID NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    agent       TEXT NOT NULL,            -- coordinator / planner / search / scraper / extractor / analyzer / writer
    step        TEXT NOT NULL,            -- start / tool_call / tool_result / thought / message / done / error
    payload     JSONB,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS agent_traces_task_idx ON agent_traces (task_id, id);

-- 报告：最终产物（Markdown）+ 引用 + 可编辑 block 序列（v0.1 先用纯 Markdown）
CREATE TABLE IF NOT EXISTS reports (
    task_id     UUID PRIMARY KEY REFERENCES tasks(id) ON DELETE CASCADE,
    title       TEXT,
    markdown    TEXT NOT NULL,
    metadata    JSONB,                    -- 竞品矩阵结构化数据等
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 抓取来源：每条引用一行，前端 SourceCitation 用
CREATE TABLE IF NOT EXISTS sources (
    id          BIGSERIAL PRIMARY KEY,
    task_id     UUID NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    ref_index   INT  NOT NULL,            -- 报告里 [1] [2] 的角标
    url         TEXT NOT NULL,
    title       TEXT,
    snippet     TEXT,
    fetched_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (task_id, ref_index)
);

-- 向量化片段（v0.1 暂未用，预留 schema）
CREATE TABLE IF NOT EXISTS chunks (
    id          BIGSERIAL PRIMARY KEY,
    task_id     UUID NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    source_id   BIGINT REFERENCES sources(id) ON DELETE CASCADE,
    content     TEXT NOT NULL,
    embedding   vector(1536),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS chunks_embedding_idx
    ON chunks USING hnsw (embedding vector_cosine_ops);

-- updated_at 自动维护
CREATE OR REPLACE FUNCTION trg_set_updated_at() RETURNS TRIGGER AS $$
BEGIN NEW.updated_at = NOW(); RETURN NEW; END $$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS tasks_updated ON tasks;
CREATE TRIGGER tasks_updated BEFORE UPDATE ON tasks
    FOR EACH ROW EXECUTE FUNCTION trg_set_updated_at();

DROP TRIGGER IF EXISTS reports_updated ON reports;
CREATE TRIGGER reports_updated BEFORE UPDATE ON reports
    FOR EACH ROW EXECUTE FUNCTION trg_set_updated_at();
