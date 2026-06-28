-- 回滚迁移: 000092_wika_knowledge_state_access。

DO $$ BEGIN RAISE NOTICE '[Migration 000092] Dropping knowledge_access_daily'; END $$;

DROP INDEX IF EXISTS idx_knowledge_access_daily_knowledge_day;
DROP INDEX IF EXISTS idx_knowledge_access_daily_kb_day;
DROP TABLE IF EXISTS knowledge_access_daily;

DO $$ BEGIN RAISE NOTICE '[Migration 000092] Dropping wika_knowledge_state'; END $$;

DROP INDEX IF EXISTS ux_wika_knowledge_state_idempotency;
DROP INDEX IF EXISTS idx_wika_knowledge_state_kb_expires;
DROP INDEX IF EXISTS idx_wika_knowledge_state_tenant_kb_freshness;
DROP TABLE IF EXISTS wika_knowledge_state;
