-- 回滚迁移: 000095_wika_freshness。

DO $$ BEGIN RAISE NOTICE '[Migration 000095] Dropping freshness tables'; END $$;

DROP INDEX IF EXISTS ux_freshness_check_items_per_check_issue;
DROP INDEX IF EXISTS idx_freshness_check_items_status;
DROP INDEX IF EXISTS idx_freshness_check_items_knowledge;
DROP TABLE IF EXISTS freshness_check_items;

DROP INDEX IF EXISTS idx_freshness_checks_kb_created;
DROP TABLE IF EXISTS freshness_checks;
