-- 迁移: 000095_wika_freshness
-- 说明: 新增 Wika 知识保鲜扫描任务和问题明细表。

DO $$ BEGIN RAISE NOTICE '[Migration 000095] Creating freshness checks'; END $$;

CREATE TABLE IF NOT EXISTS freshness_checks (
    id BIGSERIAL PRIMARY KEY,
    tenant_id BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    kb_id VARCHAR(36) NOT NULL REFERENCES knowledge_bases(id) ON DELETE CASCADE,
    trigger VARCHAR(32) NOT NULL
        CHECK (trigger IN ('manual', 'knowledge_updated', 'suggestion_applied', 'schedule')),
    status VARCHAR(24) NOT NULL DEFAULT 'running'
        CHECK (status IN ('running', 'completed', 'failed')),
    checked_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_freshness_checks_kb_created
    ON freshness_checks(kb_id, checked_at DESC);

DO $$ BEGIN RAISE NOTICE '[Migration 000095] Creating freshness check items'; END $$;

CREATE TABLE IF NOT EXISTS freshness_check_items (
    id BIGSERIAL PRIMARY KEY,
    check_id BIGINT NOT NULL REFERENCES freshness_checks(id) ON DELETE CASCADE,
    tenant_id BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    kb_id VARCHAR(36) NOT NULL REFERENCES knowledge_bases(id) ON DELETE CASCADE,
    knowledge_id VARCHAR(36) NOT NULL REFERENCES knowledges(id) ON DELETE CASCADE,
    issue_type VARCHAR(32) NOT NULL
        CHECK (issue_type IN ('expired', 'expiring', 'stale', 'low_quality', 'low_confidence')),
    severity VARCHAR(16) NOT NULL
        CHECK (severity IN ('high', 'medium', 'low')),
    suggested_action VARCHAR(64) NOT NULL,
    status VARCHAR(24) NOT NULL DEFAULT 'open'
        CHECK (status IN ('open', 'resolved', 'ignored')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_freshness_check_items_knowledge
    ON freshness_check_items(knowledge_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_freshness_check_items_status
    ON freshness_check_items(status, issue_type, created_at DESC);

CREATE UNIQUE INDEX IF NOT EXISTS ux_freshness_check_items_per_check_issue
    ON freshness_check_items(check_id, knowledge_id, issue_type);
