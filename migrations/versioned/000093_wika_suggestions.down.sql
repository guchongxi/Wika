-- 回滚迁移: 000093_wika_suggestions。

DO $$ BEGIN RAISE NOTICE '[Migration 000093] Dropping knowledge_lineage'; END $$;

DROP INDEX IF EXISTS ux_knowledge_lineage_suggestion_target;
DROP INDEX IF EXISTS idx_knowledge_lineage_target;
DROP INDEX IF EXISTS idx_knowledge_lineage_source;
DROP TABLE IF EXISTS knowledge_lineage;

DO $$ BEGIN RAISE NOTICE '[Migration 000093] Dropping knowledge_suggestions'; END $$;

DROP INDEX IF EXISTS ux_knowledge_suggestions_idempotency;
DROP INDEX IF EXISTS ux_knowledge_suggestions_open;
DROP INDEX IF EXISTS idx_knowledge_suggestions_human_reviewer;
DROP INDEX IF EXISTS idx_knowledge_suggestions_submitter;
DROP INDEX IF EXISTS idx_knowledge_suggestions_target_status;
DROP INDEX IF EXISTS idx_knowledge_suggestions_source;
DROP TABLE IF EXISTS knowledge_suggestions;
