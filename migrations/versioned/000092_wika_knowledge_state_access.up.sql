-- 迁移: 000092_wika_knowledge_state_access
-- 说明: 新增 Wika 知识状态和访问日聚合表。

DO $$ BEGIN RAISE NOTICE '[Migration 000092] Creating wika_knowledge_state'; END $$;

CREATE TABLE IF NOT EXISTS wika_knowledge_state (
    knowledge_id VARCHAR(36) PRIMARY KEY REFERENCES knowledges(id) ON DELETE CASCADE,
    tenant_id BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    kb_id VARCHAR(36) NOT NULL REFERENCES knowledge_bases(id) ON DELETE CASCADE,
    quality_score INT NOT NULL DEFAULT 0 CHECK (quality_score BETWEEN 0 AND 100),
    quality_breakdown JSONB NOT NULL DEFAULT '{}'::jsonb,
    freshness_status VARCHAR(24) NOT NULL DEFAULT 'fresh'
        CHECK (freshness_status IN ('fresh', 'expiring', 'expired', 'stale', 'low_quality', 'needs_review')),
    confidence_score NUMERIC(4,3) CHECK (confidence_score IS NULL OR confidence_score BETWEEN 0 AND 1),
    expires_at TIMESTAMPTZ,
    source_hash VARCHAR(128),
    source_updated_at TIMESTAMPTZ,
    idempotency_key VARCHAR(128),
    last_access_rollup_at TIMESTAMPTZ,
    review_status VARCHAR(24) NOT NULL DEFAULT 'none'
        CHECK (review_status IN ('none', 'needs_review', 'reviewed', 'deprecated')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_wika_knowledge_state_tenant_kb_freshness
    ON wika_knowledge_state(tenant_id, kb_id, freshness_status);

CREATE INDEX IF NOT EXISTS idx_wika_knowledge_state_kb_expires
    ON wika_knowledge_state(kb_id, expires_at)
    WHERE expires_at IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS ux_wika_knowledge_state_idempotency
    ON wika_knowledge_state(tenant_id, kb_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL;

DO $$ BEGIN RAISE NOTICE '[Migration 000092] Creating knowledge_access_daily'; END $$;

CREATE TABLE IF NOT EXISTS knowledge_access_daily (
    tenant_id BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    kb_id VARCHAR(36) NOT NULL REFERENCES knowledge_bases(id) ON DELETE CASCADE,
    knowledge_id VARCHAR(36) NOT NULL REFERENCES knowledges(id) ON DELETE CASCADE,
    day DATE NOT NULL,
    access_count BIGINT NOT NULL DEFAULT 0 CHECK (access_count >= 0),
    last_accessed_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (tenant_id, kb_id, knowledge_id, day)
);

CREATE INDEX IF NOT EXISTS idx_knowledge_access_daily_kb_day
    ON knowledge_access_daily(kb_id, day DESC);

CREATE INDEX IF NOT EXISTS idx_knowledge_access_daily_knowledge_day
    ON knowledge_access_daily(knowledge_id, day DESC);
