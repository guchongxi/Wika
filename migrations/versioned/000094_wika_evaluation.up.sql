-- 迁移: 000094_wika_evaluation
-- 说明: 新增 Wika 评测数据集、QA 明细、评测 run 和 run case 明细。

DO $$ BEGIN RAISE NOTICE '[Migration 000094] Creating eval datasets'; END $$;

CREATE TABLE IF NOT EXISTS eval_datasets (
    id BIGSERIAL PRIMARY KEY,
    tenant_id BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    kb_id VARCHAR(36) NOT NULL REFERENCES knowledge_bases(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    created_by VARCHAR(64) NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_eval_datasets_tenant_kb
    ON eval_datasets(tenant_id, kb_id, created_at DESC);

CREATE TABLE IF NOT EXISTS eval_qa_items (
    id BIGSERIAL PRIMARY KEY,
    dataset_id BIGINT NOT NULL REFERENCES eval_datasets(id) ON DELETE CASCADE,
    question TEXT NOT NULL,
    expected_answer TEXT,
    expected_knowledge_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
    expected_chunk_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
    tags JSONB NOT NULL DEFAULT '[]'::jsonb,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    version INT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_eval_qa_items_dataset_enabled
    ON eval_qa_items(dataset_id, enabled, id);

CREATE TABLE IF NOT EXISTS eval_runs (
    id BIGSERIAL PRIMARY KEY,
    tenant_id BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    kb_id VARCHAR(36) NOT NULL REFERENCES knowledge_bases(id) ON DELETE CASCADE,
    dataset_id BIGINT NOT NULL REFERENCES eval_datasets(id) ON DELETE RESTRICT,
    dataset_version INT NOT NULL DEFAULT 1,
    trigger VARCHAR(32) NOT NULL CHECK (trigger IN ('manual', 'schedule')),
    status VARCHAR(32) NOT NULL CHECK (status IN ('pending', 'running', 'completed', 'failed')),
    mrr NUMERIC(8,6) NOT NULL DEFAULT 0,
    recall_at_5 NUMERIC(8,6) NOT NULL DEFAULT 0,
    ndcg_at_5 NUMERIC(8,6) NOT NULL DEFAULT 0,
    metrics JSONB NOT NULL DEFAULT '{}'::jsonb,
    search_config JSONB NOT NULL DEFAULT '{}'::jsonb,
    total INT NOT NULL DEFAULT 0 CHECK (total >= 0),
    failed INT NOT NULL DEFAULT 0 CHECK (failed >= 0),
    error_msg TEXT,
    created_by VARCHAR(64) NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_eval_runs_kb_created
    ON eval_runs(kb_id, created_at DESC);

CREATE TABLE IF NOT EXISTS eval_run_items (
    id BIGSERIAL PRIMARY KEY,
    run_id BIGINT NOT NULL REFERENCES eval_runs(id) ON DELETE CASCADE,
    qa_item_id BIGINT NOT NULL REFERENCES eval_qa_items(id) ON DELETE RESTRICT,
    rank INT NOT NULL DEFAULT 0,
    score NUMERIC(8,6) NOT NULL DEFAULT 0,
    hit BOOLEAN NOT NULL DEFAULT FALSE,
    first_hit_rank INT NOT NULL DEFAULT 0,
    retrieved_chunk_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
    retrieved_knowledge_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
    failure_reason TEXT,
    error_msg TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_eval_run_items_run
    ON eval_run_items(run_id, id);
