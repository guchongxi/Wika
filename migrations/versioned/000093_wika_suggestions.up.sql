-- 迁移: 000093_wika_suggestions
-- 说明: 新增 Wika 个人知识推荐到团队的预审表和知识溯源表。

DO $$ BEGIN RAISE NOTICE '[Migration 000093] Creating knowledge_suggestions'; END $$;

CREATE TABLE IF NOT EXISTS knowledge_suggestions (
    id BIGSERIAL PRIMARY KEY,
    source_tenant_id BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    source_kb_id VARCHAR(36) NOT NULL REFERENCES knowledge_bases(id) ON DELETE CASCADE,
    source_knowledge_id VARCHAR(36) NOT NULL REFERENCES knowledges(id) ON DELETE CASCADE,
    target_tenant_id BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    target_kb_id VARCHAR(36) NOT NULL REFERENCES knowledge_bases(id) ON DELETE RESTRICT,
    submitter_id VARCHAR(64) NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    idempotency_key VARCHAR(128),
    source_content_hash VARCHAR(128) NOT NULL,
    reason TEXT,
    ai_decision VARCHAR(32) NOT NULL
        CHECK (ai_decision IN ('approved', 'needs_confirmation', 'rejected')),
    ai_confidence NUMERIC(4,3) NOT NULL DEFAULT 0
        CHECK (ai_confidence BETWEEN 0 AND 1),
    ai_review JSONB NOT NULL DEFAULT '{}'::jsonb,
    corrected_title TEXT,
    corrected_content TEXT,
    corrected_tags JSONB NOT NULL DEFAULT '[]'::jsonb,
    change_summary JSONB NOT NULL DEFAULT '[]'::jsonb,
    human_decision VARCHAR(32)
        CHECK (human_decision IS NULL OR human_decision IN ('approved', 'needs_confirmation', 'rejected')),
    human_reviewer_id VARCHAR(64) REFERENCES users(id) ON DELETE RESTRICT,
    human_comment TEXT,
    final_decision VARCHAR(32) NOT NULL
        CHECK (final_decision IN ('approved', 'needs_confirmation', 'rejected')),
    status VARCHAR(32) NOT NULL
        CHECK (status IN ('ai_reviewed', 'pending_human', 'rejected', 'applied')),
    auto_apply_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    policy_version INT NOT NULL DEFAULT 1,
    result_knowledge_id VARCHAR(36) REFERENCES knowledges(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    reviewed_at TIMESTAMPTZ,
    applied_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_knowledge_suggestions_source
    ON knowledge_suggestions(source_tenant_id, source_kb_id, source_knowledge_id);

CREATE INDEX IF NOT EXISTS idx_knowledge_suggestions_target_status
    ON knowledge_suggestions(target_tenant_id, target_kb_id, status, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_knowledge_suggestions_submitter
    ON knowledge_suggestions(submitter_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_knowledge_suggestions_human_reviewer
    ON knowledge_suggestions(human_reviewer_id, reviewed_at DESC)
    WHERE human_reviewer_id IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS ux_knowledge_suggestions_open
    ON knowledge_suggestions(source_knowledge_id, target_tenant_id)
    WHERE status IN ('ai_reviewed', 'pending_human');

CREATE UNIQUE INDEX IF NOT EXISTS ux_knowledge_suggestions_idempotency
    ON knowledge_suggestions(submitter_id, target_tenant_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL AND idempotency_key <> '';

DO $$ BEGIN RAISE NOTICE '[Migration 000093] Creating knowledge_lineage'; END $$;

CREATE TABLE IF NOT EXISTS knowledge_lineage (
    id BIGSERIAL PRIMARY KEY,
    source_knowledge_id VARCHAR(36) NOT NULL REFERENCES knowledges(id) ON DELETE CASCADE,
    target_knowledge_id VARCHAR(36) NOT NULL REFERENCES knowledges(id) ON DELETE CASCADE,
    source_tenant_id BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    target_tenant_id BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    mode VARCHAR(24) NOT NULL DEFAULT 'copy'
        CHECK (mode IN ('copy', 'reference', 'snapshot')),
    suggestion_id BIGINT NOT NULL REFERENCES knowledge_suggestions(id) ON DELETE CASCADE,
    created_by VARCHAR(64) NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_knowledge_lineage_source
    ON knowledge_lineage(source_knowledge_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_knowledge_lineage_target
    ON knowledge_lineage(target_knowledge_id, created_at DESC);

CREATE UNIQUE INDEX IF NOT EXISTS ux_knowledge_lineage_suggestion_target
    ON knowledge_lineage(suggestion_id, target_knowledge_id);
