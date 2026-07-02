-- 回滚迁移: 000105_wika_token_usage

DO $$ BEGIN RAISE NOTICE '[Migration 000105] Dropping wika_token_usage_events'; END $$;

DROP INDEX IF EXISTS idx_wika_token_usage_events_tool_created;
DROP INDEX IF EXISTS idx_wika_token_usage_events_token_created;
DROP INDEX IF EXISTS idx_wika_token_usage_events_user_created;
DROP INDEX IF EXISTS idx_wika_token_usage_events_tenant_created;
DROP TABLE IF EXISTS wika_token_usage_events;

DO $$ BEGIN RAISE NOTICE '[Migration 000105] Dropping wika_token_usage_daily'; END $$;

DROP INDEX IF EXISTS idx_wika_token_usage_daily_tool_day;
DROP INDEX IF EXISTS idx_wika_token_usage_daily_token_day;
DROP INDEX IF EXISTS idx_wika_token_usage_daily_user_day;
DROP INDEX IF EXISTS idx_wika_token_usage_daily_tenant_day;
DROP TABLE IF EXISTS wika_token_usage_daily;
